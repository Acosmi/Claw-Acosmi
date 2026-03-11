package scope

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- Agent 作用域 ----------

// TS 参考: src/agents/agent-scope.ts (193 行)

const DefaultAgentID = "default"

// ResolvedAgentConfig 解析后的 Agent 配置。
type ResolvedAgentConfig struct {
	Name         string
	Workspace    string
	AgentDir     string
	Model        *types.AgentModelConfig
	Skills       []string
	MemorySearch *types.MemorySearchConfig
	HumanDelay   *types.HumanDelayConfig
	Heartbeat    *types.HeartbeatConfig
	Identity     *types.IdentityConfig
	GroupChat    *types.GroupChatConfig
	Subagents    *types.AgentSubagentsConfig
	Sandbox      *types.AgentSandboxConfig
	Tools        *types.AgentToolsConfig
}

// NormalizeAgentId 标准化 agent ID（小写 + trim）。
// 与 internal/gateway/session_utils.go 保持一致。
func NormalizeAgentId(id string) string {
	return strings.TrimSpace(strings.ToLower(id))
}

// ---------- Agent 列表 ----------

// ListAgents 返回配置中的 Agent 列表。
func ListAgents(cfg *types.OpenAcosmiConfig) []types.AgentListItemConfig {
	if cfg == nil || cfg.Agents == nil {
		return nil
	}
	var result []types.AgentListItemConfig
	for _, entry := range cfg.Agents.List {
		if entry.ID != "" {
			result = append(result, entry)
		}
	}
	return result
}

// ListAgentIds 返回所有 Agent ID（去重）。
func ListAgentIds(cfg *types.OpenAcosmiConfig) []string {
	agents := ListAgents(cfg)
	if len(agents) == 0 {
		return []string{DefaultAgentID}
	}
	seen := make(map[string]bool)
	var ids []string
	for _, entry := range agents {
		id := NormalizeAgentId(entry.ID)
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []string{DefaultAgentID}
	}
	return ids
}

// ResolveDefaultAgentId 解析默认 Agent ID。
func ResolveDefaultAgentId(cfg *types.OpenAcosmiConfig) string {
	agents := ListAgents(cfg)
	if len(agents) == 0 {
		return DefaultAgentID
	}
	// 查找标记为 default 的 agent
	var defaults []types.AgentListItemConfig
	for _, a := range agents {
		if a.Default != nil && *a.Default {
			defaults = append(defaults, a)
		}
	}
	if len(defaults) > 0 {
		chosen := strings.TrimSpace(defaults[0].ID)
		if chosen != "" {
			return NormalizeAgentId(chosen)
		}
	}
	// 否则用第一个
	chosen := strings.TrimSpace(agents[0].ID)
	if chosen != "" {
		return NormalizeAgentId(chosen)
	}
	return DefaultAgentID
}

// ---------- Session Agent 解析 ----------

// agentKeyParsed 解析 "agent:<id>:<rest>" 结构。
type agentKeyParsed struct {
	AgentID string
	Rest    string
}

// ParseAgentSessionKey 解析 agent session key。
func ParseAgentSessionKey(key string) *agentKeyParsed {
	if !strings.HasPrefix(key, "agent:") {
		return nil
	}
	after := key[6:]
	idx := strings.Index(after, ":")
	if idx < 0 {
		return &agentKeyParsed{AgentID: after, Rest: ""}
	}
	return &agentKeyParsed{
		AgentID: after[:idx],
		Rest:    after[idx+1:],
	}
}

// ResolveSessionAgentIds 从 session key 解析出默认和会话 agent ID。
// TS 参考: agent-scope.ts L74-84
func ResolveSessionAgentIds(sessionKey string, cfg *types.OpenAcosmiConfig) (defaultAgentId, sessionAgentId string) {
	defaultAgentId = ResolveDefaultAgentId(cfg)
	sk := strings.TrimSpace(sessionKey)
	if sk == "" {
		return defaultAgentId, defaultAgentId
	}
	parsed := ParseAgentSessionKey(strings.ToLower(sk))
	if parsed == nil || parsed.AgentID == "" {
		return defaultAgentId, defaultAgentId
	}
	return defaultAgentId, NormalizeAgentId(parsed.AgentID)
}

// ResolveSessionAgentId 从 session key 解析 agent ID。
func ResolveSessionAgentId(sessionKey string, cfg *types.OpenAcosmiConfig) string {
	_, sessionAgentId := ResolveSessionAgentIds(sessionKey, cfg)
	return sessionAgentId
}

// ---------- Agent 配置解析 ----------

// ResolveAgentEntry 查找匹配的 Agent 条目。
func ResolveAgentEntry(cfg *types.OpenAcosmiConfig, agentId string) *types.AgentListItemConfig {
	id := NormalizeAgentId(agentId)
	for _, entry := range ListAgents(cfg) {
		if NormalizeAgentId(entry.ID) == id {
			return &entry
		}
	}
	return nil
}

// ResolveAgentConfig 解析完整的 Agent 配置。
func ResolveAgentConfig(cfg *types.OpenAcosmiConfig, agentId string) *ResolvedAgentConfig {
	entry := ResolveAgentEntry(cfg, agentId)
	if entry == nil {
		return nil
	}
	return &ResolvedAgentConfig{
		Name:         entry.Name,
		Workspace:    entry.Workspace,
		AgentDir:     entry.AgentDir,
		Model:        entry.Model,
		Skills:       entry.Skills,
		MemorySearch: entry.MemorySearch,
		HumanDelay:   entry.HumanDelay,
		Heartbeat:    entry.Heartbeat,
		Identity:     entry.Identity,
		GroupChat:    entry.GroupChat,
		Subagents:    entry.Subagents,
		Sandbox:      entry.Sandbox,
		Tools:        entry.Tools,
	}
}

// ---------- Agent 模型覆盖 ----------

// ResolveAgentModelPrimary 获取 Agent 的主模型覆盖。
func ResolveAgentModelPrimary(cfg *types.OpenAcosmiConfig, agentId string) string {
	ac := ResolveAgentConfig(cfg, agentId)
	if ac == nil || ac.Model == nil {
		return ""
	}
	return strings.TrimSpace(ac.Model.Primary)
}

// ResolveAgentModelFallbacksOverride 获取 Agent 的 fallbacks 覆盖。
func ResolveAgentModelFallbacksOverride(cfg *types.OpenAcosmiConfig, agentId string) []string {
	ac := ResolveAgentConfig(cfg, agentId)
	if ac == nil || ac.Model == nil {
		return nil
	}
	if ac.Model.Fallbacks == nil {
		return nil
	}
	return *ac.Model.Fallbacks
}

// ResolveAgentSkillsFilter 获取 Agent 的技能过滤列表。
func ResolveAgentSkillsFilter(cfg *types.OpenAcosmiConfig, agentId string) []string {
	ac := ResolveAgentConfig(cfg, agentId)
	if ac == nil {
		return nil
	}
	var normalized []string
	for _, s := range ac.Skills {
		s = strings.TrimSpace(s)
		if s != "" {
			normalized = append(normalized, s)
		}
	}
	return normalized
}

// ---------- 工作目录 ----------

// ResolveAgentWorkspaceDir 解析 Agent 的工作目录。
func ResolveAgentWorkspaceDir(cfg *types.OpenAcosmiConfig, agentId string) string {
	id := NormalizeAgentId(agentId)
	ac := ResolveAgentConfig(cfg, id)

	// Agent 级 workspace
	if ac != nil {
		ws := strings.TrimSpace(ac.Workspace)
		if ws != "" {
			return resolveUserPath(ws)
		}
	}

	// 默认 agent 使用全局 defaults
	defaultID := ResolveDefaultAgentId(cfg)
	if id == defaultID && cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil {
		ws := strings.TrimSpace(cfg.Agents.Defaults.Workspace)
		if ws != "" {
			return resolveUserPath(ws)
		}
	}

	// 默认 agent 回退到默认工作目录
	if id == defaultID {
		return resolveDefaultAgentWorkspaceDir()
	}

	// 非默认 agent 使用独立的 workspace 目录
	// TS 参考: agent-scope.ts → path.join(stateDir, `workspace-${id}`)
	stateDir := resolveStateDir()
	return filepath.Join(stateDir, "workspace-"+id)
}

// ResolveAgentDir 解析 Agent 的状态目录。
func ResolveAgentDir(cfg *types.OpenAcosmiConfig, agentId string) string {
	id := NormalizeAgentId(agentId)
	ac := ResolveAgentConfig(cfg, id)
	if ac != nil {
		dir := strings.TrimSpace(ac.AgentDir)
		if dir != "" {
			return resolveUserPath(dir)
		}
	}
	stateDir := resolveStateDir()
	return filepath.Join(stateDir, "agents", id, "agent")
}

// resolveUserPath 展开 ~ 为用户主目录。
func resolveUserPath(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// resolveDefaultAgentWorkspaceDir 解析默认 Agent 的工作目录。
// TS 参考: workspace.ts L10-19 (resolveDefaultAgentWorkspaceDir)
func resolveDefaultAgentWorkspaceDir() string {
	home, _ := os.UserHomeDir()
	profile := strings.TrimSpace(os.Getenv("OPENACOSMI_PROFILE"))
	if profile != "" && strings.ToLower(profile) != "default" {
		return filepath.Join(home, ".openacosmi", "workspace-"+profile)
	}
	return filepath.Join(home, ".openacosmi", "workspace")
}

// resolveStateDir 解析状态目录。
func resolveStateDir() string {
	if dir := os.Getenv("OPENACOSMI_STATE_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "openacosmi")
}
