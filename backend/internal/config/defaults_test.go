package config

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ============================================================
// F4a: DefaultModelAliases 别名数量
// ============================================================

func TestDefaultModelAliases_Count(t *testing.T) {
	// TS defaults.ts:L14-L26 定义 6 个别名
	if got := len(DefaultModelAliases); got != 6 {
		t.Errorf("DefaultModelAliases should have 6 entries, got %d", got)
	}
	// 确认关键别名存在
	for _, key := range []string{"opus", "sonnet", "gpt", "gpt-mini", "gemini", "gemini-flash"} {
		if _, ok := DefaultModelAliases[key]; !ok {
			t.Errorf("DefaultModelAliases missing key %q", key)
		}
	}
}

func TestParseModelRef_KeepsRuntimeProviderIDs(t *testing.T) {
	ref := ParseModelRef("qwen/qwen3.5-plus", "anthropic")
	if ref == nil {
		t.Fatal("expected parsed model ref")
	}
	if ref.Provider != "qwen" || ref.Model != "qwen3.5-plus" {
		t.Fatalf("expected qwen/qwen3.5-plus, got %s/%s", ref.Provider, ref.Model)
	}
}

// ============================================================
// F4b: applyMessageDefaults
// ============================================================

func TestApplyMessageDefaults_SetsAckReactionScope(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Messages: &types.MessagesConfig{},
	}
	ApplyDefaults(cfg)
	if cfg.Messages.AckReactionScope != types.AckGroupMentions {
		t.Errorf("expected ackReactionScope=%q, got %q", types.AckGroupMentions, cfg.Messages.AckReactionScope)
	}
}

func TestApplyMessageDefaults_NoOverwrite(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Messages: &types.MessagesConfig{
			AckReactionScope: types.AckAll,
		},
	}
	ApplyDefaults(cfg)
	if cfg.Messages.AckReactionScope != types.AckAll {
		t.Errorf("should not overwrite existing ackReactionScope, got %q", cfg.Messages.AckReactionScope)
	}
}

func TestApplyMessageDefaults_NilMessages_CreatesDefault(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	ApplyDefaults(cfg)
	if cfg.Messages == nil {
		t.Fatal("should create Messages when nil (TS-aligned)")
	}
	if cfg.Messages.AckReactionScope != types.AckGroupMentions {
		t.Errorf("ackReactionScope = %q, want %q", cfg.Messages.AckReactionScope, types.AckGroupMentions)
	}
}

// ============================================================
// F4c: applySessionDefaults
// ============================================================

func TestApplySessionDefaults_ForcesMainKey(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Session: &types.SessionConfig{MainKey: "custom"},
	}
	ApplyDefaults(cfg)
	if cfg.Session.MainKey != "main" {
		t.Errorf("expected mainKey=%q, got %q", "main", cfg.Session.MainKey)
	}
}

func TestApplySessionDefaults_NilSession_NoOp(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	ApplyDefaults(cfg)
	if cfg.Session != nil {
		t.Error("should not create Session when nil")
	}
}

// ============================================================
// F4d: applyLoggingDefaults
// ============================================================

func TestApplyLoggingDefaults_SetsRedactSensitive(t *testing.T) {
	// A2 修复后: applyLoggingDefaults 不再在 logging == nil 时创建。
	// 测试已有 logging 场景下的默认值填充。
	cfg := &types.OpenAcosmiConfig{
		Logging: &types.LoggingConfig{},
	}
	ApplyDefaults(cfg)
	if cfg.Logging == nil {
		t.Fatal("Logging should still exist")
	}
	if cfg.Logging.RedactSensitive != "tools" {
		t.Errorf("expected redactSensitive=%q, got %q", "tools", cfg.Logging.RedactSensitive)
	}
}

func TestApplyLoggingDefaults_NoOverwrite(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Logging: &types.LoggingConfig{RedactSensitive: "off"},
	}
	ApplyDefaults(cfg)
	if cfg.Logging.RedactSensitive != "off" {
		t.Errorf("should not overwrite existing redactSensitive, got %q", cfg.Logging.RedactSensitive)
	}
}

// ============================================================
// F4e: applyCompactionDefaults
// ============================================================

func TestApplyCompactionDefaults_SetsSafeguardMode(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Compaction: &types.AgentCompactionConfig{},
			},
		},
	}
	ApplyDefaults(cfg)
	if cfg.Agents.Defaults.Compaction.Mode != types.CompactionSafeguard {
		t.Errorf("expected mode=%q, got %q", types.CompactionSafeguard, cfg.Agents.Defaults.Compaction.Mode)
	}
	// F2: maxHistoryShare 不再由 config defaults 注入
	if cfg.Agents.Defaults.Compaction.MaxHistoryShare != nil {
		t.Errorf("maxHistoryShare should remain nil (not injected at config layer)")
	}
}

func TestApplyCompactionDefaults_NoOverwriteMode(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Compaction: &types.AgentCompactionConfig{
					Mode: types.CompactionDefault,
				},
			},
		},
	}
	ApplyDefaults(cfg)
	if cfg.Agents.Defaults.Compaction.Mode != types.CompactionDefault {
		t.Errorf("should not overwrite existing mode, got %q", cfg.Agents.Defaults.Compaction.Mode)
	}
}

// ============================================================
// F4f: applyContextPruningDefaults
// ============================================================

func TestApplyContextPruningDefaults_SetsCacheTTLAndHeartbeat(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Auth: &types.AuthConfig{
			Profiles: map[string]*types.AuthProfileConfig{
				"main": {Provider: "anthropic", Mode: types.AuthModeAPIKey},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				ContextPruning: &types.AgentContextPruningConfig{},
			},
		},
	}
	ApplyDefaults(cfg)
	cp := cfg.Agents.Defaults.ContextPruning
	if cp.Mode != "cache-ttl" {
		t.Errorf("expected mode=%q, got %q", "cache-ttl", cp.Mode)
	}
	if cp.TTL != "1h" {
		t.Errorf("expected ttl=%q, got %q", "1h", cp.TTL)
	}
	// heartbeat 默认值
	hb := cfg.Agents.Defaults.Heartbeat
	if hb == nil {
		t.Fatal("heartbeat should be created")
	}
	if hb.Every != "30m" {
		t.Errorf("expected heartbeat.every=%q, got %q", "30m", hb.Every)
	}
}

func TestApplyContextPruningDefaults_SoftTrimHardClear(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Auth: &types.AuthConfig{
			Profiles: map[string]*types.AuthProfileConfig{
				"main": {Provider: "anthropic", Mode: types.AuthModeAPIKey},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				ContextPruning: &types.AgentContextPruningConfig{},
			},
		},
	}
	ApplyDefaults(cfg)
	cp := cfg.Agents.Defaults.ContextPruning
	// F3: softTrimRatio/hardClearRatio 不再由 config defaults 注入
	if cp.SoftTrimRatio != nil {
		t.Errorf("expected softTrimRatio to remain nil (not injected at config layer)")
	}
	if cp.HardClearRatio != nil {
		t.Errorf("expected hardClearRatio to remain nil (not injected at config layer)")
	}
}

// ============================================================
// F4g: applyModelDefaults
// ============================================================

func TestApplyModelDefaults_SetsContextWindowAndMaxTokens(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"test": {
					Models: []types.ModelDefinitionConfig{
						{ID: "test/model-1", Name: "Model 1"},
					},
				},
			},
		},
	}
	ApplyDefaults(cfg)
	m := cfg.Models.Providers["test"].Models[0]
	if m.ContextWindow != DefaultContextTokens {
		t.Errorf("expected contextWindow=%d, got %d", DefaultContextTokens, m.ContextWindow)
	}
	if m.MaxTokens != DefaultModelMaxTokens {
		t.Errorf("expected maxTokens=%d, got %d", DefaultModelMaxTokens, m.MaxTokens)
	}
	if len(m.Input) != 1 || m.Input[0] != types.ModelInputText {
		t.Errorf("expected input=[text], got %v", m.Input)
	}
}

func TestApplyModelDefaults_MaxTokensCappedByContextWindow(t *testing.T) {
	// contextWindow == 1000，小于 DefaultModelMaxTokens (8192)
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"test": {
					Models: []types.ModelDefinitionConfig{
						{ID: "test/tiny", Name: "Tiny", ContextWindow: 1000},
					},
				},
			},
		},
	}
	ApplyDefaults(cfg)
	m := cfg.Models.Providers["test"].Models[0]
	if m.MaxTokens != 1000 {
		t.Errorf("maxTokens should be capped to contextWindow=1000, got %d", m.MaxTokens)
	}
}

func TestApplyModelDefaults_NoOverwriteExisting(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"test": {
					Models: []types.ModelDefinitionConfig{
						{
							ID:            "test/custom",
							Name:          "Custom",
							ContextWindow: 50000,
							MaxTokens:     4096,
							Input:         []types.ModelInputType{types.ModelInputImage},
						},
					},
				},
			},
		},
	}
	ApplyDefaults(cfg)
	m := cfg.Models.Providers["test"].Models[0]
	if m.ContextWindow != 50000 {
		t.Errorf("should not overwrite existing contextWindow, got %d", m.ContextWindow)
	}
	if m.MaxTokens != 4096 {
		t.Errorf("should not overwrite existing maxTokens, got %d", m.MaxTokens)
	}
	if len(m.Input) != 1 || m.Input[0] != types.ModelInputImage {
		t.Errorf("should not overwrite existing input, got %v", m.Input)
	}
}

func TestApplyModelDefaults_InjectsAlias(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Models: map[string]*types.AgentModelEntryConfig{
					"anthropic/claude-sonnet-4-5": {},
				},
			},
		},
	}
	ApplyDefaults(cfg)
	entry := cfg.Agents.Defaults.Models["anthropic/claude-sonnet-4-5"]
	if entry.Alias != "sonnet" {
		t.Errorf("expected alias=%q, got %q", "sonnet", entry.Alias)
	}
}

func TestApplyModelDefaults_NoOverwriteExistingAlias(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Models: map[string]*types.AgentModelEntryConfig{
					"anthropic/claude-sonnet-4-5": {Alias: "my-sonnet"},
				},
			},
		},
	}
	ApplyDefaults(cfg)
	entry := cfg.Agents.Defaults.Models["anthropic/claude-sonnet-4-5"]
	if entry.Alias != "my-sonnet" {
		t.Errorf("should not overwrite existing alias, got %q", entry.Alias)
	}
}

// ============================================================
// ApplyDefaults 集成测试
// ============================================================

func TestApplyDefaults_NilConfig(t *testing.T) {
	cfg := ApplyDefaults(nil)
	if cfg == nil {
		t.Fatal("should return non-nil config")
	}
	// A2 修复后: logging 不再自动创建
	if cfg.Logging != nil {
		t.Error("should not create logging when not present (TS parity)")
	}
}

func TestApplyDefaults_FullChain(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Messages: &types.MessagesConfig{},
		Session:  &types.SessionConfig{MainKey: "old"},
		Auth: &types.AuthConfig{
			Profiles: map[string]*types.AuthProfileConfig{
				"main": {Provider: "anthropic", Mode: types.AuthModeAPIKey},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				ContextPruning: &types.AgentContextPruningConfig{},
				Compaction:     &types.AgentCompactionConfig{},
			},
		},
	}
	result := ApplyDefaults(cfg)

	// Messages
	if result.Messages.AckReactionScope != types.AckGroupMentions {
		t.Error("ackReactionScope not set")
	}
	// Session
	if result.Session.MainKey != "main" {
		t.Error("mainKey not forced to main")
	}
	// Logging — 此测试未设置 logging，TS 对齐后不再创建
	if result.Logging != nil {
		t.Error("logging should be nil when not in config (TS parity)")
	}
	// Context pruning
	if result.Agents.Defaults.ContextPruning.Mode != "cache-ttl" {
		t.Error("contextPruning.mode not set")
	}
	// Compaction
	if result.Agents.Defaults.Compaction.Mode != types.CompactionSafeguard {
		t.Error("compaction.mode not set")
	}
	// Heartbeat
	if result.Agents.Defaults.Heartbeat == nil || result.Agents.Defaults.Heartbeat.Every != "30m" {
		t.Error("heartbeat.every not set")
	}
}

// ============================================================
// Bug #2: applyContextPruningDefaults nil ContextPruning
// ============================================================

func TestBug2_NilContextPruningCreatesDefaults(t *testing.T) {
	// Bug #2: 当 Anthropic auth 存在但 ContextPruning == nil 时，
	// 应创建默认 ContextPruning 对象
	cfg := &types.OpenAcosmiConfig{
		Auth: &types.AuthConfig{
			Profiles: map[string]*types.AuthProfileConfig{
				"main": {Provider: "anthropic", Mode: types.AuthModeAPIKey},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				// ContextPruning 故意为 nil
			},
		},
	}
	ApplyDefaults(cfg)

	cp := cfg.Agents.Defaults.ContextPruning
	if cp == nil {
		t.Fatal("Bug #2: ContextPruning should be created when Anthropic auth exists")
	}
	if cp.Mode != "cache-ttl" {
		t.Errorf("expected mode=%q, got %q", "cache-ttl", cp.Mode)
	}
	if cp.TTL != "1h" {
		t.Errorf("expected ttl=%q, got %q", "1h", cp.TTL)
	}
}

func TestBug2_OAuthSetsHeartbeat1h(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Auth: &types.AuthConfig{
			Profiles: map[string]*types.AuthProfileConfig{
				"main": {Provider: "anthropic", Mode: types.AuthModeOAuth},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{},
		},
	}
	ApplyDefaults(cfg)

	hb := cfg.Agents.Defaults.Heartbeat
	if hb == nil {
		t.Fatal("heartbeat should be created")
	}
	if hb.Every != "1h" {
		t.Errorf("oauth should set heartbeat=1h, got %q", hb.Every)
	}
}

func TestBug2_NoAnthropicAuth_SkipsDefaults(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				// No ContextPruning, no Anthropic auth
			},
		},
	}
	ApplyDefaults(cfg)

	if cfg.Agents.Defaults.ContextPruning != nil {
		t.Error("should not create ContextPruning without Anthropic auth")
	}
	if cfg.Agents.Defaults.Heartbeat != nil {
		t.Error("should not create Heartbeat without Anthropic auth")
	}
}
