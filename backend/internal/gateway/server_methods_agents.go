package gateway

// agents.* 方法处理器 — 对应 src/gateway/server-methods/agents.ts
//
// 提供 Agent 列表查询 + CRUD 功能。
// 依赖: scope.ListAgents, scope.ResolveAgentIdentity, scope.ResolveDefaultAgentId

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	"github.com/Acosmi/ClawAcosmi/internal/agents/workspace"
	"github.com/Acosmi/ClawAcosmi/internal/argus"
	"github.com/Acosmi/ClawAcosmi/internal/sessions"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// AgentsHandlers 返回 agents.* 方法处理器映射。
func AgentsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"agents.list":       handleAgentsList,
		"agents.create":     handleAgentsCreate,
		"agents.update":     handleAgentsUpdate,
		"agents.delete":     handleAgentsDelete,
		"agents.files.list": handleAgentsFilesList,
		"agents.files.get":  handleAgentsFilesGet,
		"agents.files.set":  handleAgentsFilesSet,
	}
}

// ---------- agents.list ----------

func handleAgentsList(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	defaultId := scope.ResolveDefaultAgentId(cfg)
	agentIds := scope.ListAgentIds(cfg)

	agents := make([]GatewayAgentRow, 0, len(agentIds)+3) // +3 for builtin subagents (argus, coder, media)
	for _, id := range agentIds {
		row := GatewayAgentRow{ID: id, Type: "agent"}

		entry := scope.ResolveAgentEntry(cfg, id)
		if entry != nil && entry.Name != "" {
			row.Name = entry.Name
		}

		identity := scope.ResolveAgentIdentity(cfg, id)
		if identity != nil {
			row.Identity = buildAgentIdentityRow(identity)
		}

		agents = append(agents, row)
	}

	// 追加子智能体条目（统一发现），传入已加载的 cfg 避免双重加载
	agents = appendSubagentRows(ctx, cfg, agents)

	mainKey := ""
	if cfg.Session != nil && cfg.Session.MainKey != "" {
		mainKey = cfg.Session.MainKey
	}

	sessionScope := "per-sender"
	if cfg.Session != nil && cfg.Session.Scope != "" {
		sessionScope = string(cfg.Session.Scope)
	}

	ctx.Respond(true, map[string]interface{}{
		"defaultId": defaultId,
		"mainKey":   mainKey,
		"scope":     sessionScope,
		"agents":    agents,
	}, nil)
}

// ---------- agents.create ----------
// 对应 TS agents.ts L185-L256

func handleAgentsCreate(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	rawName, _ := ctx.Params["name"].(string)
	rawName = strings.TrimSpace(rawName)
	if rawName == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "name is required"))
		return
	}

	agentId := scope.NormalizeAgentId(rawName)
	if agentId == "default" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, `"default" is reserved`))
		return
	}
	if reservedSubagentIDs[agentId] {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("%q is a reserved subagent ID", agentId)))
		return
	}

	// 检查是否已存在
	for _, id := range scope.ListAgentIds(cfg) {
		if id == agentId {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("agent %q already exists", agentId)))
			return
		}
	}

	// 解析 workspace
	wsDir, _ := ctx.Params["workspace"].(string)
	wsDir = strings.TrimSpace(wsDir)
	if wsDir == "" {
		wsDir = scope.ResolveAgentWorkspaceDir(cfg, agentId)
	}

	// 确保 workspace 目录存在
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to create workspace: "+err.Error()))
		return
	}

	// 确保 sessions 目录存在
	sessionsDir := sessions.ResolveAgentSessionsDir(agentId)
	_ = os.MkdirAll(sessionsDir, 0o755)

	// 确保引导文件存在
	workspace.EnsureAgentWorkspace(workspace.EnsureAgentWorkspaceParams{
		Dir:                  wsDir,
		EnsureBootstrapFiles: true,
	})

	// 写入 IDENTITY.md
	safeName := sanitizeIdentityLine(rawName)
	emoji := resolveOptionalStringParam(ctx.Params["emoji"])
	avatar := resolveOptionalStringParam(ctx.Params["avatar"])

	identityPath := filepath.Join(wsDir, string(workspace.BootstrapIdentity))
	var lines []string
	lines = append(lines, "", fmt.Sprintf("- Name: %s", safeName))
	if emoji != "" {
		lines = append(lines, fmt.Sprintf("- Emoji: %s", sanitizeIdentityLine(emoji)))
	}
	if avatar != "" {
		lines = append(lines, fmt.Sprintf("- Avatar: %s", sanitizeIdentityLine(avatar)))
	}
	lines = append(lines, "")
	appendToFile(identityPath, strings.Join(lines, "\n"))

	// 持久化到配置文件 — 对应 TS applyAgentConfig + writeConfigFile
	if err := persistAgentToConfig(ctx, agentId, rawName, wsDir); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to persist config: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":        true,
		"agentId":   agentId,
		"name":      rawName,
		"workspace": wsDir,
	}, nil)
}

// ---------- agents.update ----------
// 对应 TS agents.ts L257-L315

func handleAgentsUpdate(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	rawId, _ := ctx.Params["agentId"].(string)
	agentId := scope.NormalizeAgentId(strings.TrimSpace(rawId))
	if agentId == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agentId is required"))
		return
	}

	// 检查 agent 是否存在
	found := false
	for _, id := range scope.ListAgentIds(cfg) {
		if id == agentId {
			found = true
			break
		}
	}
	if !found {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("agent %q not found", agentId)))
		return
	}

	// 处理 workspace 更新
	wsDir := resolveOptionalStringParam(ctx.Params["workspace"])
	if wsDir != "" {
		_ = os.MkdirAll(wsDir, 0o755)
		workspace.EnsureAgentWorkspace(workspace.EnsureAgentWorkspaceParams{
			Dir:                  wsDir,
			EnsureBootstrapFiles: true,
		})
	}

	// 处理 avatar 更新
	avatar := resolveOptionalStringParam(ctx.Params["avatar"])
	if avatar != "" {
		targetWs := wsDir
		if targetWs == "" {
			targetWs = scope.ResolveAgentWorkspaceDir(cfg, agentId)
		}
		_ = os.MkdirAll(targetWs, 0o755)
		identityPath := filepath.Join(targetWs, string(workspace.BootstrapIdentity))
		appendToFile(identityPath, fmt.Sprintf("\n- Avatar: %s\n", sanitizeIdentityLine(avatar)))
	}

	// 持久化模型配置变更 — 对应 TS applyAgentConfig + writeConfigFile
	modelStr := resolveOptionalStringParam(ctx.Params["model"])
	if modelStr != "" || wsDir != "" {
		_ = updateAgentInConfig(ctx, agentId, modelStr, wsDir)
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":      true,
		"agentId": agentId,
	}, nil)
}

// ---------- agents.delete ----------
// 对应 TS agents.ts L316-L367

func handleAgentsDelete(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	rawId, _ := ctx.Params["agentId"].(string)
	agentId := scope.NormalizeAgentId(strings.TrimSpace(rawId))
	if agentId == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agentId is required"))
		return
	}
	if agentId == "default" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, `"default" cannot be deleted`))
		return
	}

	// 检查 agent 是否存在
	found := false
	for _, id := range scope.ListAgentIds(cfg) {
		if id == agentId {
			found = true
			break
		}
	}
	if !found {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("agent %q not found", agentId)))
		return
	}

	// deleteFiles 默认 true
	deleteFiles := true
	if v, ok := ctx.Params["deleteFiles"].(bool); ok {
		deleteFiles = v
	}

	// 从配置中移除 agent — 对应 TS pruneAgentConfig + writeConfigFile
	removedBindings := pruneAgentFromConfig(ctx, agentId)

	if deleteFiles {
		wsDir := scope.ResolveAgentWorkspaceDir(cfg, agentId)
		agentDir := scope.ResolveAgentDir(cfg, agentId)
		sessionsDir := sessions.ResolveAgentSessionsDir(agentId)

		// Best-effort 删除（TS 用 moveToTrash，Go 用 RemoveAll）
		_ = os.RemoveAll(wsDir)
		_ = os.RemoveAll(agentDir)
		_ = os.RemoveAll(sessionsDir)
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":              true,
		"agentId":         agentId,
		"removedBindings": removedBindings,
	}, nil)
}

// ---------- 辅助函数 ----------

func buildAgentIdentityRow(identity *types.IdentityConfig) *AgentIdentityRow {
	if identity == nil {
		return nil
	}
	row := &AgentIdentityRow{}
	if identity.Name != "" {
		row.Name = identity.Name
	}
	if identity.Theme != "" {
		row.Theme = identity.Theme
	}
	if identity.Emoji != "" {
		row.Emoji = identity.Emoji
	}
	if identity.Avatar != "" {
		row.Avatar = identity.Avatar
	}
	if row.Name == "" && row.Theme == "" && row.Emoji == "" && row.Avatar == "" {
		return nil
	}
	return row
}

func resolveConfigFromContext(ctx *MethodHandlerContext) *types.OpenAcosmiConfig {
	if ctx.Context.Config != nil {
		return ctx.Context.Config
	}
	if ctx.Context.ConfigLoader != nil {
		cfg, err := ctx.Context.ConfigLoader.LoadConfig()
		if err == nil {
			return cfg
		}
	}
	return nil
}

var whitespaceRe = regexp.MustCompile(`\s+`)

func sanitizeIdentityLine(value string) string {
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(value, " "))
}

func resolveOptionalStringParam(value interface{}) string {
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimSpace(s)
}

func appendToFile(path, content string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(content)
}

// reservedSubagentIDs 内置子智能体保留 ID，禁止用户通过 agents.create 创建同名 agent。
var reservedSubagentIDs = map[string]bool{
	"argus-screen": true,
	"oa-coder":     true,
	"oa-media":     true,
}

// ---------- 统一发现: 子智能体 → GatewayAgentRow ----------

// appendSubagentRows 将子智能体转换为 GatewayAgentRow 追加到 agents 列表。
// 复用 server_methods_subagent.go 中已有的 build*Entry 函数。
func appendSubagentRows(ctx *MethodHandlerContext, liveCfg *types.OpenAcosmiConfig, agents []GatewayAgentRow) []GatewayAgentRow {
	// State 可能为 nil（单元测试场景）
	var argusBridge *argus.Bridge
	if ctx.Context.State != nil {
		argusBridge = ctx.Context.State.ArgusBridge()
	}

	subEntries := []subagentEntry{
		buildArgusEntry(argusBridge),
		buildCoderEntry(ctx.Context.CoderConfirmMgr != nil, liveCfg),
		buildMediaEntry(ctx.Context.MediaSubsystem, liveCfg),
	}

	emojiMap := map[string]string{
		"argus-screen": "👁️",
		"oa-coder":     "🔧",
		"oa-media":     "🎬",
	}

	for _, se := range subEntries {
		configured := se.Configured
		builtinTrue := true
		row := GatewayAgentRow{
			ID:   se.ID,
			Name: se.Label,
			Identity: &AgentIdentityRow{
				Name:  se.Label,
				Emoji: emojiMap[se.ID],
			},
			Type:       "subagent",
			Status:     se.Status,
			Error:      se.Error,
			Provider:   se.Provider,
			Model:      se.Model,
			Configured: &configured,  // 始终序列化（false = 未配置）
			Builtin:    &builtinTrue, // 内置子智能体
		}
		agents = append(agents, row)
	}
	return agents
}

// ---------- Config 持久化辅助 (P0) ----------

// persistAgentToConfig 添加新 agent 到配置文件 — 对应 TS applyAgentConfig + writeConfigFile
func persistAgentToConfig(ctx *MethodHandlerContext, agentId, name, wsDir string) error {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		return nil // 无 loader 时静默跳过
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Agents == nil {
		cfg.Agents = &types.AgentsConfig{}
	}
	// 追加到 list
	cfg.Agents.List = append(cfg.Agents.List, types.AgentListItemConfig{
		ID:        agentId,
		Name:      name,
		Workspace: wsDir,
	})
	loader.ClearCache()
	return loader.WriteConfigFile(cfg)
}

// updateAgentInConfig 更新 agent 配置（model / workspace）
func updateAgentInConfig(ctx *MethodHandlerContext, agentId, modelStr, wsDir string) error {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		return nil
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Agents == nil {
		return nil
	}
	for i := range cfg.Agents.List {
		if cfg.Agents.List[i].ID == agentId {
			if modelStr != "" {
				cfg.Agents.List[i].Model = &types.AgentModelConfig{Primary: modelStr}
			}
			if wsDir != "" {
				cfg.Agents.List[i].Workspace = wsDir
			}
			break
		}
	}
	loader.ClearCache()
	return loader.WriteConfigFile(cfg)
}

// pruneAgentFromConfig 从配置中移除 agent — 对应 TS pruneAgentConfig
func pruneAgentFromConfig(ctx *MethodHandlerContext, agentId string) int {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		return 0
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		return 0
	}
	// 移除 agent list entry
	if cfg.Agents != nil {
		newList := make([]types.AgentListItemConfig, 0, len(cfg.Agents.List))
		for _, a := range cfg.Agents.List {
			if a.ID != agentId {
				newList = append(newList, a)
			}
		}
		cfg.Agents.List = newList
	}
	// 移除 bindings
	removed := 0
	if len(cfg.Bindings) > 0 {
		newBindings := make([]types.AgentBinding, 0, len(cfg.Bindings))
		for _, b := range cfg.Bindings {
			if b.AgentID == agentId {
				removed++
			} else {
				newBindings = append(newBindings, b)
			}
		}
		cfg.Bindings = newBindings
	}
	loader.ClearCache()
	_ = loader.WriteConfigFile(cfg)
	return removed
}

// ---------- agents.files.* handlers (P1) ----------
// 对应 TS agents.ts L368-L506

// allowedAgentFileNames — 对应 TS ALLOWED_FILE_NAMES
var allowedAgentFileNames = map[string]bool{
	"IDENTITY.md": true,
	"SOUL.md":     true,
	"TOOLS.md":    true,
	"README.md":   true,
}

func handleAgentsFilesList(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	rawID, _ := ctx.Params["agentId"].(string)
	agentId := scope.NormalizeAgentId(strings.TrimSpace(rawID))
	if agentId == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agentId is required"))
		return
	}

	wsDir := scope.ResolveAgentWorkspaceDir(cfg, agentId)
	var files []map[string]interface{}
	for name := range allowedAgentFileNames {
		fp := filepath.Join(wsDir, name)
		info, err := os.Stat(fp)
		entry := map[string]interface{}{"name": name, "path": fp}
		if err == nil {
			entry["exists"] = true
			entry["size"] = info.Size()
			entry["updatedAtMs"] = info.ModTime().UnixMilli()
		} else {
			entry["exists"] = false
		}
		files = append(files, entry)
	}
	ctx.Respond(true, map[string]interface{}{
		"agentId":   agentId,
		"workspace": wsDir,
		"files":     files,
	}, nil)
}

func handleAgentsFilesGet(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	rawID, _ := ctx.Params["agentId"].(string)
	agentId := scope.NormalizeAgentId(strings.TrimSpace(rawID))
	if agentId == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agentId is required"))
		return
	}
	name := strings.TrimSpace(readString(ctx.Params, "name"))
	if !allowedAgentFileNames[name] {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unsupported file %q", name)))
		return
	}
	wsDir := scope.ResolveAgentWorkspaceDir(cfg, agentId)
	fp := filepath.Join(wsDir, name)
	info, err := os.Stat(fp)
	if err != nil {
		ctx.Respond(true, map[string]interface{}{
			"agentId":   agentId,
			"workspace": wsDir,
			"file":      map[string]interface{}{"name": name, "path": fp, "missing": true},
		}, nil)
		return
	}
	content, err := os.ReadFile(fp)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read file: "+err.Error()))
		return
	}
	ctx.Respond(true, map[string]interface{}{
		"agentId":   agentId,
		"workspace": wsDir,
		"file": map[string]interface{}{
			"name":        name,
			"path":        fp,
			"missing":     false,
			"size":        info.Size(),
			"updatedAtMs": info.ModTime().UnixMilli(),
			"content":     string(content),
		},
	}, nil)
}

func handleAgentsFilesSet(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	rawID, _ := ctx.Params["agentId"].(string)
	agentId := scope.NormalizeAgentId(strings.TrimSpace(rawID))
	if agentId == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agentId is required"))
		return
	}
	name := strings.TrimSpace(readString(ctx.Params, "name"))
	if !allowedAgentFileNames[name] {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unsupported file %q", name)))
		return
	}
	content, _ := ctx.Params["content"].(string)
	wsDir := scope.ResolveAgentWorkspaceDir(cfg, agentId)
	_ = os.MkdirAll(wsDir, 0o755)
	fp := filepath.Join(wsDir, name)
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write file: "+err.Error()))
		return
	}
	info, _ := os.Stat(fp)
	ctx.Respond(true, map[string]interface{}{
		"agentId":   agentId,
		"workspace": wsDir,
		"file": map[string]interface{}{
			"name":        name,
			"path":        fp,
			"size":        info.Size(),
			"updatedAtMs": info.ModTime().UnixMilli(),
		},
	}, nil)
}

// suppress unused json import (used for future config serialization)
var _ = json.Marshal
