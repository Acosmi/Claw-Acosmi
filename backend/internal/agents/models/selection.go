package models

import (
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- 模型选择核心 ----------

// TS 参考: src/agents/model-selection.ts (448 行)

// ModelRef 供应商 + 模型 ID 的引用。
type ModelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// ThinkLevel 思考级别。
type ThinkLevel string

const (
	ThinkOff     ThinkLevel = "off"
	ThinkMinimal ThinkLevel = "minimal"
	ThinkLow     ThinkLevel = "low"
	ThinkMedium  ThinkLevel = "medium"
	ThinkHigh    ThinkLevel = "high"
	ThinkXHigh   ThinkLevel = "xhigh"
)

// 默认值
const (
	DefaultProvider = "anthropic"
	DefaultModel    = "claude-opus-4-6"
)

// AnthropicModelAliases Anthropic 模型别名映射。
var AnthropicModelAliases = map[string]string{
	"opus-4.6":   "claude-opus-4-6",
	"opus-4.5":   "claude-opus-4-5",
	"sonnet-4.5": "claude-sonnet-4-5",
}

// ModelKey 生成 provider/model 形式的唯一键。
func ModelKey(provider, model string) string {
	return provider + "/" + model
}

// NormalizeProviderId 规范化供应商 ID。
func NormalizeProviderId(provider string) string {
	n := strings.ToLower(strings.TrimSpace(provider))
	switch n {
	case "z.ai", "z-ai":
		return "zai"
	case "openacosmi-zen":
		return "openacosmi"
	case "kimi-code":
		return "kimi-coding"
	default:
		return n
	}
}

// IsCliProvider 判断是否是 CLI 供应商。
func IsCliProvider(provider string, cfg *types.OpenAcosmiConfig) bool {
	n := NormalizeProviderId(provider)
	if n == "claude-cli" || n == "codex-cli" {
		return true
	}
	if cfg == nil || cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return false
	}
	for key := range cfg.Agents.Defaults.CliBackends {
		if NormalizeProviderId(key) == n {
			return true
		}
	}
	return false
}

func normalizeAnthropicModelId(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return trimmed
	}
	lower := strings.ToLower(trimmed)
	if alias, ok := AnthropicModelAliases[lower]; ok {
		return alias
	}
	return trimmed
}

func normalizeProviderModelId(provider, model string) string {
	switch provider {
	case "anthropic":
		return normalizeAnthropicModelId(model)
	case "google":
		return NormalizeGoogleModelId(model)
	default:
		return model
	}
}

// ParseModelRef 解析 "provider/model" 或 "model" 字符串为 ModelRef。
func ParseModelRef(raw, defaultProvider string) *ModelRef {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	slash := strings.Index(trimmed, "/")
	if slash == -1 {
		provider := NormalizeProviderId(defaultProvider)
		model := normalizeProviderModelId(provider, trimmed)
		return &ModelRef{Provider: provider, Model: model}
	}
	providerRaw := strings.TrimSpace(trimmed[:slash])
	provider := NormalizeProviderId(providerRaw)
	model := strings.TrimSpace(trimmed[slash+1:])
	if provider == "" || model == "" {
		return nil
	}
	normalizedModel := normalizeProviderModelId(provider, model)
	return &ModelRef{Provider: provider, Model: normalizedModel}
}

// ---------- 别名索引 ----------

// ModelAliasEntry 别名条目。
type ModelAliasEntry struct {
	Alias string
	Ref   ModelRef
}

// ModelAliasIndex 模型别名索引。
type ModelAliasIndex struct {
	ByAlias map[string]ModelAliasEntry // normalized alias → entry
	ByKey   map[string][]string        // model key → alias list
}

// BuildModelAliasIndex 从配置构建别名索引。
func BuildModelAliasIndex(cfg *types.OpenAcosmiConfig, defaultProvider string) ModelAliasIndex {
	idx := ModelAliasIndex{
		ByAlias: make(map[string]ModelAliasEntry),
		ByKey:   make(map[string][]string),
	}
	if cfg == nil || cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return idx
	}
	rawModels := cfg.Agents.Defaults.Models
	if rawModels == nil {
		return idx
	}
	for keyRaw, entryRaw := range rawModels {
		parsed := ParseModelRef(keyRaw, defaultProvider)
		if parsed == nil {
			continue
		}
		if entryRaw == nil {
			continue
		}
		alias := ""
		if entryRaw.Alias != "" {
			alias = strings.TrimSpace(entryRaw.Alias)
		}
		if alias == "" {
			continue
		}
		aliasKey := strings.ToLower(strings.TrimSpace(alias))
		idx.ByAlias[aliasKey] = ModelAliasEntry{Alias: alias, Ref: *parsed}
		key := ModelKey(parsed.Provider, parsed.Model)
		idx.ByKey[key] = append(idx.ByKey[key], alias)
	}
	return idx
}

// ResolveModelRefFromString 从字符串解析 ModelRef，优先检查别名。
func ResolveModelRefFromString(raw, defaultProvider string, aliasIndex *ModelAliasIndex) *ModelRef {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if !strings.Contains(trimmed, "/") && aliasIndex != nil {
		aliasKey := strings.ToLower(strings.TrimSpace(trimmed))
		if entry, ok := aliasIndex.ByAlias[aliasKey]; ok {
			ref := entry.Ref
			return &ref
		}
	}
	return ParseModelRef(trimmed, defaultProvider)
}

// ResolveConfiguredModelRef 解析配置中的默认模型。
func ResolveConfiguredModelRef(cfg *types.OpenAcosmiConfig, defaultProvider, defaultModel string) ModelRef {
	if cfg == nil || cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return ModelRef{Provider: defaultProvider, Model: defaultModel}
	}

	model := cfg.Agents.Defaults.Model
	rawModel := ""
	if model != nil {
		if model.Primary != "" {
			rawModel = strings.TrimSpace(model.Primary)
		}
	}

	if rawModel != "" {
		aliasIndex := BuildModelAliasIndex(cfg, defaultProvider)
		if !strings.Contains(rawModel, "/") {
			aliasKey := strings.ToLower(strings.TrimSpace(rawModel))
			if entry, ok := aliasIndex.ByAlias[aliasKey]; ok {
				return entry.Ref
			}
			return ModelRef{Provider: "anthropic", Model: rawModel}
		}
		ref := ResolveModelRefFromString(rawModel, defaultProvider, &aliasIndex)
		if ref != nil {
			return *ref
		}
	}
	return ModelRef{Provider: defaultProvider, Model: defaultModel}
}

// ---------- 允许列表 ----------

// AllowedModelSet 允许的模型集合结果。
type AllowedModelSet struct {
	AllowAny       bool
	AllowedCatalog []ModelCatalogEntry
	AllowedKeys    map[string]bool
}

// BuildAllowedModelSet 构建允许的模型集合。
func BuildAllowedModelSet(cfg *types.OpenAcosmiConfig, catalog []ModelCatalogEntry, defaultProvider, defaultModel string) AllowedModelSet {
	rawModels := map[string]*types.AgentModelEntryConfig{}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Models != nil {
		rawModels = cfg.Agents.Defaults.Models
	}

	allowAny := len(rawModels) == 0
	defaultKey := ""
	if defaultModel != "" && defaultProvider != "" {
		defaultKey = ModelKey(defaultProvider, strings.TrimSpace(defaultModel))
	}

	catalogKeys := make(map[string]bool)
	for _, entry := range catalog {
		catalogKeys[ModelKey(entry.Provider, entry.ID)] = true
	}

	if allowAny {
		if defaultKey != "" {
			catalogKeys[defaultKey] = true
		}
		return AllowedModelSet{
			AllowAny:       true,
			AllowedCatalog: catalog,
			AllowedKeys:    catalogKeys,
		}
	}

	// TS 对照: model-selection.ts L290 — configuredProviders
	configuredProviders := make(map[string]bool)
	if cfg != nil && cfg.Models != nil {
		for k := range cfg.Models.Providers {
			configuredProviders[NormalizeProviderId(k)] = true
		}
	}

	allowedKeys := make(map[string]bool)
	for raw := range rawModels {
		parsed := ParseModelRef(raw, defaultProvider)
		if parsed == nil {
			continue
		}
		key := ModelKey(parsed.Provider, parsed.Model)
		providerKey := NormalizeProviderId(parsed.Provider)
		if IsCliProvider(parsed.Provider, cfg) || catalogKeys[key] || configuredProviders[providerKey] {
			allowedKeys[key] = true
		}
	}
	if defaultKey != "" {
		allowedKeys[defaultKey] = true
	}

	var allowedCatalog []ModelCatalogEntry
	for _, entry := range catalog {
		if allowedKeys[ModelKey(entry.Provider, entry.ID)] {
			allowedCatalog = append(allowedCatalog, entry)
		}
	}

	if len(allowedCatalog) == 0 && len(allowedKeys) == 0 {
		if defaultKey != "" {
			catalogKeys[defaultKey] = true
		}
		return AllowedModelSet{
			AllowAny:       true,
			AllowedCatalog: catalog,
			AllowedKeys:    catalogKeys,
		}
	}

	return AllowedModelSet{AllowAny: false, AllowedCatalog: allowedCatalog, AllowedKeys: allowedKeys}
}

// ModelRefStatus 模型引用状态。
type ModelRefStatus struct {
	Key       string
	InCatalog bool
	AllowAny  bool
	Allowed   bool
}

// GetModelRefStatus 获取模型引用状态。
func GetModelRefStatus(cfg *types.OpenAcosmiConfig, catalog []ModelCatalogEntry, ref ModelRef, defaultProvider, defaultModel string) ModelRefStatus {
	allowed := BuildAllowedModelSet(cfg, catalog, defaultProvider, defaultModel)
	key := ModelKey(ref.Provider, ref.Model)
	inCatalog := false
	for _, entry := range catalog {
		if ModelKey(entry.Provider, entry.ID) == key {
			inCatalog = true
			break
		}
	}
	return ModelRefStatus{
		Key:       key,
		InCatalog: inCatalog,
		AllowAny:  allowed.AllowAny,
		Allowed:   allowed.AllowAny || allowed.AllowedKeys[key],
	}
}

// ResolveAllowedModelRef 解析并验证模型是否在允许列表中。
func ResolveAllowedModelRef(cfg *types.OpenAcosmiConfig, catalog []ModelCatalogEntry, raw, defaultProvider, defaultModel string) (*ModelRef, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, "", fmt.Errorf("invalid model: empty")
	}

	aliasIndex := BuildModelAliasIndex(cfg, defaultProvider)
	ref := ResolveModelRefFromString(trimmed, defaultProvider, &aliasIndex)
	if ref == nil {
		return nil, "", fmt.Errorf("invalid model: %s", trimmed)
	}

	status := GetModelRefStatus(cfg, catalog, *ref, defaultProvider, defaultModel)
	if !status.Allowed {
		return nil, "", fmt.Errorf("model not allowed: %s", status.Key)
	}

	return ref, status.Key, nil
}

// ResolveThinkingDefault 解析默认思考级别。
func ResolveThinkingDefault(cfg *types.OpenAcosmiConfig, provider, model string, catalog []ModelCatalogEntry) ThinkLevel {
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil {
		configured := cfg.Agents.Defaults.ThinkingDefault
		if configured != "" {
			return ThinkLevel(configured)
		}
	}
	for _, entry := range catalog {
		if entry.Provider == provider && entry.ID == model {
			if entry.Reasoning != nil && *entry.Reasoning {
				return ThinkLow
			}
			break
		}
	}
	return ThinkOff
}

// BuildConfiguredAllowlistKeys 构建配置的模型白名单键集合。
// TS 参考: model-selection.ts → buildConfiguredAllowlistKeys()
// 返回 nil 表示允许任何模型 (allowAny)。
func BuildConfiguredAllowlistKeys(cfg *types.OpenAcosmiConfig, defaultProvider string) map[string]bool {
	if cfg == nil || cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return nil
	}
	rawModels := cfg.Agents.Defaults.Models
	if len(rawModels) == 0 {
		return nil // allowAny
	}
	keys := make(map[string]bool)
	for raw := range rawModels {
		parsed := ParseModelRef(raw, defaultProvider)
		if parsed != nil {
			keys[ModelKey(parsed.Provider, parsed.Model)] = true
		}
	}
	return keys
}

// ResolveDefaultModelForAgent 解析 Agent 的默认模型。
// TS 参考: model-selection.ts L224-254 → resolveDefaultModelForAgent()
// 优先级链: agent.model.primary → global agents.defaults.model.primary → DefaultModel
func ResolveDefaultModelForAgent(cfg *types.OpenAcosmiConfig, agentID string) ModelRef {
	override := resolveAgentModelPrimary(cfg, agentID)
	if override != "" {
		overrideCfg := shallowOverrideModelPrimary(cfg, override)
		return ResolveConfiguredModelRef(overrideCfg, DefaultProvider, DefaultModel)
	}
	return ResolveConfiguredModelRef(cfg, DefaultProvider, DefaultModel)
}

// resolveAgentModelPrimary 获取 agent 级别的 model.primary 覆盖值。
// TS 参考: agent-scope.ts L139-149 → resolveAgentModelPrimary()
func resolveAgentModelPrimary(cfg *types.OpenAcosmiConfig, agentID string) string {
	if agentID == "" {
		return ""
	}
	ac := scope.ResolveAgentConfig(cfg, agentID)
	if ac == nil || ac.Model == nil {
		return ""
	}
	primary := strings.TrimSpace(ac.Model.Primary)
	return primary
}

// shallowOverrideModelPrimary 浅克隆 cfg 并覆盖 agents.defaults.model.primary。
// 不修改原始 cfg，避免副作用。
func shallowOverrideModelPrimary(cfg *types.OpenAcosmiConfig, primary string) *types.OpenAcosmiConfig {
	if cfg == nil {
		return &types.OpenAcosmiConfig{
			Agents: &types.AgentsConfig{
				Defaults: &types.AgentDefaultsConfig{
					Model: &types.AgentModelListConfig{Primary: primary},
				},
			},
		}
	}

	// 浅克隆 config 链
	cloned := *cfg
	if cloned.Agents == nil {
		cloned.Agents = &types.AgentsConfig{}
	} else {
		agentsCopy := *cloned.Agents
		cloned.Agents = &agentsCopy
	}
	if cloned.Agents.Defaults == nil {
		cloned.Agents.Defaults = &types.AgentDefaultsConfig{}
	} else {
		defaultsCopy := *cloned.Agents.Defaults
		cloned.Agents.Defaults = &defaultsCopy
	}
	if cloned.Agents.Defaults.Model == nil {
		cloned.Agents.Defaults.Model = &types.AgentModelListConfig{Primary: primary}
	} else {
		modelCopy := *cloned.Agents.Defaults.Model
		modelCopy.Primary = primary
		cloned.Agents.Defaults.Model = &modelCopy
	}
	return &cloned
}

// ResolveHooksGmailModel 解析 Gmail hook 配置的模型引用。
// 对应 TS: model-selection.ts resolveHooksGmailModel()
// 返回 nil 表示未配置 gmail model。
func ResolveHooksGmailModel(cfg *types.OpenAcosmiConfig, defaultProvider string) *ModelRef {
	if cfg == nil || cfg.Hooks == nil || cfg.Hooks.Gmail == nil {
		return nil
	}
	hooksModel := strings.TrimSpace(cfg.Hooks.Gmail.Model)
	if hooksModel == "" {
		return nil
	}
	aliasIndex := BuildModelAliasIndex(cfg, defaultProvider)
	return ResolveModelRefFromString(hooksModel, defaultProvider, &aliasIndex)
}

// ---------- Managed Model 双轨优先判断 (Phase 4) ----------

// ManagedModelProvider 托管模型提供者接口。
type ManagedModelProvider interface {
	List() ([]types.ManagedModelEntry, error)
	DefaultModel() *types.ManagedModelEntry
}

// ResolveManagedModelRef 解析托管模型为 ModelRef。
// 优先规则（前置判断，不修改现有 custom 逻辑）:
//  1. 用户显式指定了 custom model.primary → 返回 nil（尊重用户选择）
//  2. ManagedModels.Enabled && managed 有可用模型 → 返回 managed 默认模型
//  3. 否则 → 返回 nil（调用方走现有 custom 逻辑）
func ResolveManagedModelRef(cfg *types.OpenAcosmiConfig, managed ManagedModelProvider) *ModelRef {
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil &&
		cfg.Agents.Defaults.Model != nil && cfg.Agents.Defaults.Model.Primary != "" {
		return nil
	}
	if cfg == nil || cfg.Models == nil || cfg.Models.ManagedModels == nil || !cfg.Models.ManagedModels.Enabled {
		return nil
	}
	if managed == nil {
		return nil
	}
	entry := managed.DefaultModel()
	if entry == nil {
		return nil
	}
	return &ModelRef{
		Provider: entry.Provider,
		Model:    entry.ModelID,
	}
}
