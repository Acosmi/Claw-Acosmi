package config

// 配置默认值模块 — 对应 src/config/defaults.ts (471 行)
//
// 在配置加载后应用默认值。
// 遵循 TypeScript 版本的 apply* 函数链:
// applyMessageDefaults → applyLoggingDefaults → applySessionDefaults →
// applyAgentDefaults → applyContextPruningDefaults → applyCompactionDefaults →
// applyModelDefaults

import (
	"os"
	"strings"

	"log"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ModelRef 解析后的模型引用
// 对应 TS agents/model-selection.ts:L7-L10
type ModelRef struct {
	Provider string
	Model    string
}

// NormalizeProviderID 规范化 provider ID（含特殊别名映射）
// 对应 TS agents/model-selection.ts:L33-L48
func NormalizeProviderID(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "z.ai", "z-ai":
		return "zai"
	case "openacosmi-zen":
		return "openacosmi"
	case "kimi-code":
		return "kimi-coding"
	}
	return normalized
}

// ParseModelRef 解析 "provider/model" 格式的模型引用
// 对应 TS agents/model-selection.ts:L81-L100
func ParseModelRef(raw string, defaultProvider string) *ModelRef {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	slash := strings.Index(trimmed, "/")
	if slash == -1 {
		provider := NormalizeProviderID(defaultProvider)
		return &ModelRef{Provider: provider, Model: trimmed}
	}
	providerRaw := strings.TrimSpace(trimmed[:slash])
	provider := NormalizeProviderID(providerRaw)
	model := strings.TrimSpace(trimmed[slash+1:])
	if provider == "" || model == "" {
		return nil
	}
	return &ModelRef{Provider: provider, Model: model}
}

// sessionWarnOnce 防止重复警告 session.mainKey
var sessionWarnOnce bool

// 默认模型别名 — 对应 TS defaults.ts:L14-L26
var DefaultModelAliases = map[string]string{
	"opus":         "anthropic/claude-opus-4-6",
	"sonnet":       "anthropic/claude-sonnet-4-5",
	"gpt":          "openai/gpt-5.2",
	"gpt-mini":     "openai/gpt-5-mini",
	"gemini":       "google/gemini-3-pro-preview",
	"gemini-flash": "google/gemini-3-flash-preview",
}

// 默认配置常量 (整数)
const (
	DefaultContextTokens          = 200000
	DefaultAgentMaxConcurrent     = 5
	DefaultSubagentMaxConcurrent  = 3
	DefaultTimeoutSeconds         = 300
	DefaultMediaMaxMb             = 25
	DefaultTypingIntervalSeconds  = 3
	DefaultGatewayPortValue       = 19001
	DefaultCompactionReserveFloor = 4000
	DefaultModelMaxTokens         = 8192
)

// 默认配置常量 (浮点/字符串)
const (
	DefaultLogLevel              types.LogLevel = "info"
	DefaultMaxHistoryShare                      = 0.5
	DefaultSoftTrimRatioDefault                 = 0.3
	DefaultHardClearRatioDefault                = 0.2
)

// resolvePrimaryModelRef 解析主模型引用，支持别名展开
// 对应 TS defaults.ts:L96-L106
func resolvePrimaryModelRef(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	aliasKey := strings.ToLower(trimmed)
	if alias, ok := DefaultModelAliases[aliasKey]; ok {
		return alias
	}
	return trimmed
}

// resolveAnthropicDefaultAuthMode 解析 Anthropic 默认认证模式
// 对应 TS defaults.ts:L56-L94
// 返回 "api_key" / "oauth" / ""(未配置)
func resolveAnthropicDefaultAuthMode(cfg *types.OpenAcosmiConfig) string {
	if cfg.Auth == nil {
		// 回退到环境变量
		if strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_TOKEN")) != "" {
			return "oauth"
		}
		if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
			return "api_key"
		}
		return ""
	}

	profiles := cfg.Auth.Profiles

	// 1) 检查 auth order 中的第一个 anthropic profile
	if order, ok := cfg.Auth.Order["anthropic"]; ok {
		for _, profileID := range order {
			entry := profiles[profileID]
			if entry == nil || entry.Provider != "anthropic" {
				continue
			}
			switch entry.Mode {
			case types.AuthModeAPIKey:
				return "api_key"
			case types.AuthModeOAuth, types.AuthModeToken:
				return "oauth"
			}
		}
	}

	// 2) 检查所有 anthropic profiles 的 mode
	hasAPIKey := false
	hasOAuth := false
	for _, profile := range profiles {
		if profile == nil || profile.Provider != "anthropic" {
			continue
		}
		switch profile.Mode {
		case types.AuthModeAPIKey:
			hasAPIKey = true
		case types.AuthModeOAuth, types.AuthModeToken:
			hasOAuth = true
		}
	}
	if hasAPIKey && !hasOAuth {
		return "api_key"
	}
	if hasOAuth && !hasAPIKey {
		return "oauth"
	}

	// 3) 环境变量回退
	if strings.TrimSpace(os.Getenv("ANTHROPIC_OAUTH_TOKEN")) != "" {
		return "oauth"
	}
	if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
		return "api_key"
	}
	return ""
}

// resolveModelCost 解析模型成本，补全零值
// 对应 TS defaults.ts:L44-L54
func resolveModelCost(raw *types.ModelCostConfig) types.ModelCostConfig {
	if raw == nil {
		return types.ModelCostConfig{}
	}
	return *raw
}

// applyTalkApiKey Talk API key 回退
// 对应 TS defaults.ts:L154-L170
// 从环境变量 OPENACOSMI_TALK_API_KEY 填充 talk.apiKey
func applyTalkApiKey(cfg *types.OpenAcosmiConfig) {
	resolved := strings.TrimSpace(os.Getenv("OPENACOSMI_TALK_API_KEY"))
	if resolved == "" {
		return
	}
	if cfg.Talk != nil && strings.TrimSpace(cfg.Talk.APIKey) != "" {
		return
	}
	if cfg.Talk == nil {
		cfg.Talk = &types.TalkConfig{}
	}
	cfg.Talk.APIKey = resolved
}

// ApplyDefaults 应用完整的默认值链
// 对应 io.ts 中的: applyModelDefaults(applyCompactionDefaults(...applyMessageDefaults(cfg)))
func ApplyDefaults(cfg *types.OpenAcosmiConfig) *types.OpenAcosmiConfig {
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	applyMessageDefaults(cfg)
	applyLoggingDefaults(cfg)
	applySessionDefaults(cfg)
	applyAgentDefaults(cfg)
	applyTalkApiKey(cfg)
	applyContextPruningDefaults(cfg)
	applyCompactionDefaults(cfg)
	applyModelDefaults(cfg)
	return cfg
}

// applyMessageDefaults 消息配置默认值
// 对应 TS defaults.ts:L113-L126
// TS: 即使 messages 为 nil 也创建并设置 ackReactionScope
func applyMessageDefaults(cfg *types.OpenAcosmiConfig) {
	if cfg.Messages == nil {
		cfg.Messages = &types.MessagesConfig{}
	}
	if cfg.Messages.AckReactionScope != "" {
		return
	}
	cfg.Messages.AckReactionScope = types.AckGroupMentions
}

// applyLoggingDefaults 日志配置默认值
// 对应 TS defaults.ts:L335-L350
// 注意: TS 版本在 logging == nil 时直接返回（不创建）
func applyLoggingDefaults(cfg *types.OpenAcosmiConfig) {
	if cfg.Logging == nil {
		return
	}
	if cfg.Logging.RedactSensitive == "" {
		cfg.Logging.RedactSensitive = "tools"
	}
}

// applySessionDefaults 会话配置默认值
// 对应 TS defaults.ts:L128-L152
// TS 强制 mainKey 为 "main"，忽略用户自定义值
func applySessionDefaults(cfg *types.OpenAcosmiConfig) {
	if cfg.Session == nil || cfg.Session.MainKey == "" {
		return
	}
	trimmed := strings.TrimSpace(cfg.Session.MainKey)
	if trimmed != "" && trimmed != "main" && !sessionWarnOnce {
		sessionWarnOnce = true
		log.Println(`[openacosmi] session.mainKey is ignored; main session is always "main".`)
	}
	cfg.Session.MainKey = "main"
}

// applyAgentDefaults Agent 配置默认值
// 对应 TS defaults.ts:L294-L333
// TS 仅设置 maxConcurrent 和 subagents.maxConcurrent
// contextTokens/timeoutSeconds/mediaMaxMB 等由 Agent Engine 运行时处理
func applyAgentDefaults(cfg *types.OpenAcosmiConfig) {
	if cfg.Agents == nil {
		cfg.Agents = &types.AgentsConfig{}
	}
	if cfg.Agents.Defaults == nil {
		cfg.Agents.Defaults = &types.AgentDefaultsConfig{}
	}

	defaults := cfg.Agents.Defaults

	// maxConcurrent 默认值 — TS L308-L310
	if defaults.MaxConcurrent == nil {
		v := DefaultAgentMaxConcurrent
		defaults.MaxConcurrent = &v
	}

	// subagents.maxConcurrent 默认值 — TS L313-L316
	// G1: TS 在 subagents 为 nil 时也创建并设置 maxConcurrent
	if defaults.Subagents == nil {
		defaults.Subagents = &types.SubagentDefaultsConfig{}
	}
	if defaults.Subagents.MaxConcurrent == nil {
		v := DefaultSubagentMaxConcurrent
		defaults.Subagents.MaxConcurrent = &v
	}
}

// applyContextPruningDefaults 上下文裁剪默认值
// 对应 TS defaults.ts:L352-L441
func applyContextPruningDefaults(cfg *types.OpenAcosmiConfig) {
	defaults := cfg.Agents.Defaults
	if defaults == nil {
		return
	}

	authMode := resolveAnthropicDefaultAuthMode(cfg)
	if authMode == "" {
		return
	}

	// contextPruning 默认值 — TS L368-L374
	// TS: 仅在 mode === undefined 时同时设置 mode + ttl
	if defaults.ContextPruning == nil {
		defaults.ContextPruning = &types.AgentContextPruningConfig{}
	}
	if defaults.ContextPruning.Mode == "" {
		defaults.ContextPruning.Mode = "cache-ttl"
		// F13: ttl 仅在 mode 未设置时附带（TS L372）
		if defaults.ContextPruning.TTL == "" {
			defaults.ContextPruning.TTL = "1h"
		}
	}

	// heartbeat 默认值
	if defaults.Heartbeat == nil {
		defaults.Heartbeat = &types.HeartbeatConfig{}
	}
	if defaults.Heartbeat.Every == "" {
		if authMode == "oauth" {
			defaults.Heartbeat.Every = "1h"
		} else {
			defaults.Heartbeat.Every = "30m"
		}
	}

	// —— A1 修复: cacheRetention 注入（仅 api_key 模式）——
	// 对应 TS defaults.ts:L385-L427
	if authMode == "api_key" {
		injectCacheRetention(defaults)
	}
}

// injectCacheRetention 为 Anthropic 模型注入 cacheRetention = "short"
// 对应 TS defaults.ts:L385-L427
func injectCacheRetention(defaults *types.AgentDefaultsConfig) {
	if defaults.Models == nil {
		defaults.Models = make(map[string]*types.AgentModelEntryConfig)
	}
	models := defaults.Models

	// 遍历所有已有模型 key，为 Anthropic provider 的模型注入
	for key, entry := range models {
		parsed := ParseModelRef(key, "anthropic")
		if parsed == nil || parsed.Provider != "anthropic" {
			continue
		}
		if entry == nil {
			entry = &types.AgentModelEntryConfig{}
			models[key] = entry
		}
		if entry.Params == nil {
			entry.Params = make(map[string]interface{})
		}
		if _, ok := entry.Params["cacheRetention"]; ok {
			if _, isStr := entry.Params["cacheRetention"].(string); isStr {
				continue // 已有字符串值，跳过
			}
		}
		entry.Params["cacheRetention"] = "short"
	}

	// 对 primary model 也注入
	primary := resolvePrimaryModelRef("")
	if defaults.Model != nil {
		primary = resolvePrimaryModelRef(defaults.Model.Primary)
	}
	if primary != "" {
		parsedPrimary := ParseModelRef(primary, "anthropic")
		if parsedPrimary != nil && parsedPrimary.Provider == "anthropic" {
			key := parsedPrimary.Provider + "/" + parsedPrimary.Model
			entry := models[key]
			if entry == nil {
				entry = &types.AgentModelEntryConfig{}
				models[key] = entry
			}
			if entry.Params == nil {
				entry.Params = make(map[string]interface{})
			}
			if _, ok := entry.Params["cacheRetention"]; !ok {
				entry.Params["cacheRetention"] = "short"
			}
		}
	}
}

// F3: fillContextPruningFields 已移除
// TS 不在 config defaults 层注入 SoftTrimRatio/HardClearRatio
// 这些由 pruning engine 运行时提供默认值

// applyCompactionDefaults 压缩配置默认值
// 对应 TS defaults.ts:L443-L466
// TS: 仅在 compaction?.mode 为 falsy 时设置 mode = "safeguard"，不填充其他字段
func applyCompactionDefaults(cfg *types.OpenAcosmiConfig) {
	if cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return
	}
	defaults := cfg.Agents.Defaults
	if defaults.Compaction == nil {
		return
	}

	// TS L449: if (compaction?.mode) return cfg;
	if defaults.Compaction.Mode != "" {
		return
	}
	defaults.Compaction.Mode = types.CompactionSafeguard
}

// applyModelDefaults 模型定义默认值
// 对应 TS defaults.ts:L172-L292
func applyModelDefaults(cfg *types.OpenAcosmiConfig) {
	// 1) 补全模型供应商中各模型的默认字段
	if cfg.Models != nil && cfg.Models.Providers != nil {
		for _, provider := range cfg.Models.Providers {
			if provider == nil || len(provider.Models) == 0 {
				continue
			}
			for i := range provider.Models {
				m := &provider.Models[i]

				// reasoning: Go bool 零值 false，与 TS 行为一致

				// input 默认 ["text"]
				if len(m.Input) == 0 {
					m.Input = []types.ModelInputType{types.ModelInputText}
				}

				// cost: Go 零值 {0, 0, 0, 0} 与 TS DEFAULT_MODEL_COST 一致

				// contextWindow 默认值
				if m.ContextWindow <= 0 {
					m.ContextWindow = DefaultContextTokens
				}

				// maxTokens 默认值
				if m.MaxTokens <= 0 {
					defaultMax := DefaultModelMaxTokens
					if defaultMax > m.ContextWindow {
						defaultMax = m.ContextWindow
					}
					m.MaxTokens = defaultMax
				}

				// maxTokens 不能超过 contextWindow
				if m.MaxTokens > m.ContextWindow {
					m.MaxTokens = m.ContextWindow
				}
			}
		}
	}

	// 2) Agent 模型目录中注入别名
	if cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return
	}
	models := cfg.Agents.Defaults.Models
	if len(models) == 0 {
		return
	}
	for alias, target := range DefaultModelAliases {
		entry, ok := models[target]
		if !ok || entry == nil {
			continue
		}
		if entry.Alias != "" {
			continue
		}
		entry.Alias = alias
	}
}
