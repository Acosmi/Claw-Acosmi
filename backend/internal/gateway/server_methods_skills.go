package gateway

// server_methods_skills.go — skills.* 方法处理器（全量实现）
// 对应 TS: src/gateway/server-methods/skills.ts (217L)
//
// 方法列表 (4):
//   skills.status, skills.bins, skills.install, skills.update
//
// 依赖:
//   agents/scope: ListAgentIds, ResolveDefaultAgentId, ResolveAgentWorkspaceDir
//   agents/skills: LoadSkillEntries, BuildWorkspaceSkillSnapshot, SkillEntry
//   config: ConfigLoader.LoadConfig, ConfigLoader.WriteConfigFile
//   infra: GetRemoteSkillEligibility
//   routing: NormalizeAgentID

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropic/open-acosmi/internal/agents/scope"
	"github.com/anthropic/open-acosmi/internal/agents/skills"
	"github.com/anthropic/open-acosmi/internal/argus"
	"github.com/anthropic/open-acosmi/internal/routing"
	"github.com/anthropic/open-acosmi/pkg/types"
)

// SkillsHandlers 返回 skills.* 方法映射。
func SkillsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"skills.status":        handleSkillsStatus,
		"skills.bins":          handleSkillsBins,
		"skills.install":       handleSkillsInstall,
		"skills.update":        handleSkillsUpdate,
		"skills.distribute":    handleSkillsDistribute,
		"skills.store.browse":  handleSkillsStoreBrowse,
		"skills.store.pull":    handleSkillsStorePull,
		"skills.store.refresh": handleSkillsStoreRefresh,
		"skills.store.link":    handleSkillsStoreLink,
	}
}

// ---------- skills.status ----------
// TS: skills.ts L70-102
// 获取指定 agent 的技能状态报告。

func handleSkillsStatus(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// 解析 agentId
	agentIDRaw, _ := ctx.Params["agentId"].(string)
	agentIDRaw = strings.TrimSpace(agentIDRaw)
	agentID := ""
	if agentIDRaw != "" {
		agentID = routing.NormalizeAgentID(agentIDRaw)
		// 验证 agent 是否存在
		knownAgents := scope.ListAgentIds(cfg)
		found := false
		for _, id := range knownAgents {
			if id == agentID {
				found = true
				break
			}
		}
		if !found {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unknown agent id \""+agentIDRaw+"\""))
			return
		}
	} else {
		agentID = scope.ResolveDefaultAgentId(cfg)
	}

	workspaceDir := scope.ResolveAgentWorkspaceDir(cfg, agentID)
	bundledDir := skills.ResolveBundledSkillsDir("")

	// 解析 docs/skills/ 目录用于 source 判定
	docsSkillsDir := skills.ResolveDocsSkillsDir(workspaceDir)
	syncedDir := ""
	if docsSkillsDir != "" {
		syncedDir = docsSkillsDir + "/synced"
	}

	// 加载所有技能条目
	entries := skills.LoadSkillEntries(workspaceDir, "", bundledDir, cfg)

	// 构建 SkillStatusEntry 列表（匹配 UI SkillStatusReport.skills 格式）
	type skillStatusEntry struct {
		Name               string                   `json:"name"`
		Description        string                   `json:"description"`
		Source             string                   `json:"source"`
		FilePath           string                   `json:"filePath"`
		BaseDir            string                   `json:"baseDir"`
		SkillKey           string                   `json:"skillKey"`
		Bundled            bool                     `json:"bundled,omitempty"`
		PrimaryEnv         string                   `json:"primaryEnv,omitempty"`
		Emoji              string                   `json:"emoji,omitempty"`
		Homepage           string                   `json:"homepage,omitempty"`
		Always             bool                     `json:"always"`
		Disabled           bool                     `json:"disabled"`
		BlockedByAllowlist bool                     `json:"blockedByAllowlist"`
		Eligible           bool                     `json:"eligible"`
		Distributed        bool                     `json:"distributed"`
		DistributedAt      string                   `json:"distributedAt,omitempty"`
		Requirements       map[string][]string      `json:"requirements"`
		Missing            map[string][]string      `json:"missing"`
		ConfigChecks       []map[string]interface{} `json:"configChecks"`
		Install            []map[string]interface{} `json:"install"`
	}

	skillEntries := make([]skillStatusEntry, 0, len(entries))
	for _, e := range entries {
		skillFile := ""
		if e.Skill.Dir != "" {
			skillFile = e.Skill.Dir + "/SKILL.md"
		}

		// 判断是否 bundled
		isBundled := false
		if bundledDir != "" && strings.HasPrefix(e.Skill.Dir, bundledDir) {
			isBundled = true
		}

		// 确定 source
		source := "workspace"
		if isBundled {
			source = "bundled"
		} else if syncedDir != "" && strings.HasPrefix(e.Skill.Dir, syncedDir) {
			source = "synced"
		} else if docsSkillsDir != "" && strings.HasPrefix(e.Skill.Dir, docsSkillsDir) {
			source = "docs"
		}

		// 检查是否被配置禁用
		disabled := false
		if cfg != nil && cfg.Skills != nil && cfg.Skills.Entries != nil {
			if sc, ok := cfg.Skills.Entries[e.Skill.Name]; ok && sc != nil {
				if sc.Enabled != nil && !*sc.Enabled {
					disabled = true
				}
			}
		}

		// 检查 VFS 分级状态
		distributed := false
		distributedAt := ""
		if vfs := ctx.Context.UHMSVFS(); vfs != nil {
			cat := skills.ResolveSkillCategory(e)
			if meta, err := vfs.ReadSystemMeta("skills", cat, e.Skill.Name); err == nil {
				if d, ok := meta["distributed"].(bool); ok && d {
					distributed = true
				}
				if t, ok := meta["distributed_at"].(string); ok {
					distributedAt = t
				}
			}
		}

		skillEntries = append(skillEntries, skillStatusEntry{
			Name:          e.Skill.Name,
			Description:   e.Skill.Description,
			Source:        source,
			FilePath:      skillFile,
			BaseDir:       e.Skill.Dir,
			SkillKey:      e.Skill.Name,
			Bundled:       isBundled,
			PrimaryEnv:    e.PrimaryEnv,
			Always:        false,
			Disabled:      disabled,
			Eligible:      e.Enabled && !disabled,
			Distributed:   distributed,
			DistributedAt: distributedAt,
			Requirements: map[string][]string{
				"bins":   {},
				"env":    {},
				"config": {},
				"os":     {},
			},
			Missing: map[string][]string{
				"bins":   {},
				"env":    {},
				"config": {},
				"os":     {},
			},
			ConfigChecks: []map[string]interface{}{},
			Install:      []map[string]interface{}{},
		})
	}

	// 追加 Argus 视觉子智能体技能条目
	if bridge := ctx.Context.ArgusBridge; bridge != nil {
		argusEntries := argus.BuildArgusSkillEntries(bridge.Tools())
		for _, ae := range argusEntries {
			skillEntries = append(skillEntries, skillStatusEntry{
				Name:               ae.Name,
				Description:        ae.Description,
				Source:             ae.Source,
				FilePath:           ae.FilePath,
				BaseDir:            ae.BaseDir,
				SkillKey:           ae.SkillKey,
				Bundled:            ae.Bundled,
				PrimaryEnv:         ae.PrimaryEnv,
				Emoji:              ae.Emoji,
				Always:             ae.Always,
				Disabled:           ae.Disabled,
				BlockedByAllowlist: ae.BlockedByAllowlist,
				Eligible:           ae.Eligible,
				Requirements:       ae.Requirements,
				Missing:            ae.Missing,
				ConfigChecks:       ae.ConfigChecks,
				Install:            ae.Install,
			})
		}
	}

	// 构建报告（匹配 UI SkillStatusReport 类型）
	report := map[string]interface{}{
		"workspaceDir":     workspaceDir,
		"managedSkillsDir": "",
		"skills":           skillEntries,
	}

	ctx.Respond(true, report, nil)
}

// ---------- skills.bins ----------
// TS: skills.ts L103-125
// 收集所有工作区的技能所需二进制列表。

func handleSkillsBins(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// TS: listWorkspaceDirs(cfg) — 收集所有 agent 的工作区目录
	workspaceDirs := listSkillsWorkspaceDirs(cfg)
	bins := make(map[string]bool)
	for _, wsDir := range workspaceDirs {
		entries := skills.LoadSkillEntries(wsDir, "", skills.ResolveBundledSkillsDir(""), cfg)
		for _, bin := range collectSkillBins(entries) {
			bins[bin] = true
		}
	}

	sorted := make([]string, 0, len(bins))
	for b := range bins {
		sorted = append(sorted, b)
	}
	sort.Strings(sorted)

	ctx.Respond(true, map[string]interface{}{
		"bins": sorted,
	}, nil)
}

// ---------- skills.install ----------
// TS: skills.ts L126-157
// 安装技能。

func handleSkillsInstall(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	name, _ := ctx.Params["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "skills.install requires name"))
		return
	}
	installID, _ := ctx.Params["installId"].(string)
	installID = strings.TrimSpace(installID)
	if installID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "skills.install requires installId"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	workspaceDir := scope.ResolveAgentWorkspaceDir(cfg, scope.ResolveDefaultAgentId(cfg))

	timeoutMs := 300_000
	if raw, ok := ctx.Params["timeoutMs"].(float64); ok && raw > 0 {
		timeoutMs = int(raw)
	}

	result := skills.InstallSkillFromSpec(skills.SkillInstallRequest{
		WorkspaceDir: workspaceDir,
		SkillName:    name,
		InstallID:    installID,
		TimeoutMs:    timeoutMs,
		Config:       cfg,
	})

	if !result.OK {
		ctx.Respond(false, result, NewErrorShape(ErrCodeServiceUnavailable, result.Message))
		return
	}
	ctx.Respond(true, result, nil)
}

// ---------- skills.update ----------
// TS: skills.ts L158-215
// 更新技能配置（enabled/apiKey/env）。

func handleSkillsUpdate(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	// FIND-5: TS 参数为 skillKey, 非 name
	skillKey, _ := ctx.Params["skillKey"].(string)
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "skills.update requires skillKey"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// 构建 skills 配置树
	if cfg.Skills == nil {
		cfg.Skills = &types.SkillsConfig{}
	}
	if cfg.Skills.Entries == nil {
		cfg.Skills.Entries = make(map[string]*types.SkillConfig)
	}

	current := cfg.Skills.Entries[skillKey]
	if current == nil {
		current = &types.SkillConfig{}
	}

	// 应用 enabled
	if enabled, ok := ctx.Params["enabled"].(bool); ok {
		current.Enabled = &enabled
	}

	// 应用 apiKey
	if apiKey, ok := ctx.Params["apiKey"].(string); ok {
		trimmed := strings.TrimSpace(apiKey)
		if trimmed != "" {
			current.APIKey = trimmed
		} else {
			current.APIKey = ""
		}
	}

	// 应用 env
	if envRaw, ok := ctx.Params["env"].(map[string]interface{}); ok {
		if current.Env == nil {
			current.Env = make(map[string]string)
		}
		for key, val := range envRaw {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			valStr, _ := val.(string)
			trimmedVal := strings.TrimSpace(valStr)
			if trimmedVal == "" {
				delete(current.Env, trimmedKey)
			} else {
				current.Env[trimmedKey] = trimmedVal
			}
		}
	}

	cfg.Skills.Entries[skillKey] = current

	// FIND-6: 写入配置文件 (TS: writeConfigFile)
	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write config: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":       true,
		"skillKey": skillKey,
		"config":   current,
	}, nil)
}

// ---------- 辅助函数 ----------

// listSkillsWorkspaceDirs 收集所有 agent 工作区目录。
// TS: listWorkspaceDirs(cfg)
func listSkillsWorkspaceDirs(cfg *types.OpenAcosmiConfig) []string {
	dirs := make(map[string]bool)
	agentIds := scope.ListAgentIds(cfg)
	for _, id := range agentIds {
		dirs[scope.ResolveAgentWorkspaceDir(cfg, id)] = true
	}
	// 始终包含默认 agent
	dirs[scope.ResolveAgentWorkspaceDir(cfg, scope.ResolveDefaultAgentId(cfg))] = true

	result := make([]string, 0, len(dirs))
	for d := range dirs {
		result = append(result, d)
	}
	sort.Strings(result)
	return result
}

// collectSkillBins 收集技能条目中需要的二进制列表。
// TS: collectSkillBins(entries)
func collectSkillBins(entries []skills.SkillEntry) []string {
	bins := make(map[string]bool)
	// SkillEntry 当前不含 metadata.requires.bins 字段
	// 若后续扩展 SkillEntry 增加 metadata，在此处提取
	// 目前返回空切片，与 TS 行为一致（仅在有 metadata 时才有 bins）
	_ = entries
	sorted := make([]string, 0, len(bins))
	for b := range bins {
		sorted = append(sorted, b)
	}
	sort.Strings(sorted)
	return sorted
}

// ---------- skills.store.browse ----------
// 浏览 nexus-v4 远程技能商店。

func handleSkillsStoreBrowse(ctx *MethodHandlerContext) {
	client := ctx.Context.SkillStoreClient
	if client == nil || !client.Available() {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "skill store not configured: set skills.store.url and skills.store.token in config"))
		return
	}

	category, _ := ctx.Params["category"].(string)
	keyword, _ := ctx.Params["keyword"].(string)

	items, err := client.Browse(strings.TrimSpace(category), strings.TrimSpace(keyword))
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "browse skill store: "+err.Error()))
		return
	}

	// 标记本地已安装的技能
	loader := ctx.Context.ConfigLoader
	if loader != nil {
		if cfg, cfgErr := loader.LoadConfig(); cfgErr == nil {
			agentID := scope.ResolveDefaultAgentId(cfg)
			workspaceDir := scope.ResolveAgentWorkspaceDir(cfg, agentID)
			bundledDir := skills.ResolveBundledSkillsDir("")
			localEntries := skills.LoadSkillEntries(workspaceDir, "", bundledDir, cfg)

			localNames := make(map[string]bool, len(localEntries))
			for _, e := range localEntries {
				localNames[e.Skill.Name] = true
			}

			for i := range items {
				if localNames[items[i].Key] || localNames[items[i].Name] {
					items[i].IsInstalled = true
				}
			}
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"skills": items,
	}, nil)
}

// ---------- skills.store.pull ----------
// 批量拉取远程技能到本地 docs/skills/synced/。

func handleSkillsStorePull(ctx *MethodHandlerContext) {
	client := ctx.Context.SkillStoreClient
	if client == nil || !client.Available() {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "skill store not configured"))
		return
	}

	// 解析 skillIds 参数
	var skillIDs []string
	if raw, ok := ctx.Params["skillIds"].([]interface{}); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				skillIDs = append(skillIDs, strings.TrimSpace(s))
			}
		}
	}
	if len(skillIDs) == 0 {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "skills.store.pull requires skillIds (non-empty string array)"))
		return
	}

	// 解析 docsSkillsDir
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	agentID := scope.ResolveDefaultAgentId(cfg)
	workspaceDir := scope.ResolveAgentWorkspaceDir(cfg, agentID)
	docsSkillsDir := skills.ResolveDocsSkillsDir(workspaceDir)
	if docsSkillsDir == "" {
		// fallback: 使用 workspaceDir/docs/skills
		docsSkillsDir = filepath.Join(workspaceDir, "docs", "skills")
	}

	results, errs := skills.BatchPull(client, skillIDs, docsSkillsDir)

	errStrings := make([]string, 0, len(errs))
	for _, e := range errs {
		errStrings = append(errStrings, e.Error())
	}

	ctx.Respond(true, map[string]interface{}{
		"results": results,
		"errors":  errStrings,
	}, nil)
}

// ---------- skills.store.refresh ----------
// 重新扫描 docs/skills/ 并返回最新技能列表。

func handleSkillsStoreRefresh(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	agentID := scope.ResolveDefaultAgentId(cfg)
	workspaceDir := scope.ResolveAgentWorkspaceDir(cfg, agentID)
	bundledDir := skills.ResolveBundledSkillsDir("")
	entries := skills.LoadSkillEntries(workspaceDir, "", bundledDir, cfg)

	summaries := make([]skills.SkillSummary, 0, len(entries))
	for _, e := range entries {
		summaries = append(summaries, skills.SkillSummary{
			Name:       e.Skill.Name,
			PrimaryEnv: e.PrimaryEnv,
		})
	}

	ctx.Respond(true, map[string]interface{}{
		"count":  len(summaries),
		"skills": summaries,
	}, nil)
}

// ---------- skills.store.link ----------
// 返回 nexus-v4 chat 端技能管理页面 URL。

func handleSkillsStoreLink(ctx *MethodHandlerContext) {
	// 从 config 读取 store URL
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	storeURL := ""
	if cfg != nil && cfg.Skills != nil && cfg.Skills.Store != nil {
		storeURL = strings.TrimRight(cfg.Skills.Store.URL, "/")
	}
	if storeURL == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "skills.store.url not configured"))
		return
	}

	page, _ := ctx.Params["page"].(string)
	page = strings.TrimSpace(page)

	var link string
	switch page {
	case "create":
		link = fmt.Sprintf("%s/skills/create", storeURL)
	case "manage":
		link = fmt.Sprintf("%s/skills/manage", storeURL)
	case "browse", "":
		link = fmt.Sprintf("%s/skills", storeURL)
	default:
		link = fmt.Sprintf("%s/skills", storeURL)
	}

	ctx.Respond(true, map[string]interface{}{
		"url": link,
	}, nil)
}
