package gateway

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestConvertWizardV2PayloadToConfig_ProviderMapping(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"anthropic": "sk-ant-xxx",
			"openai":    "sk-openai-yyy",
		},
		FallbackConfig: map[string]string{
			"deepseek": "sk-ds-zzz",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"anthropic": {Model: "claude-4-sonnet", AuthMode: "apiKey"},
			"openai":    {Model: "gpt-5", AuthMode: "apiKey"},
			"deepseek":  {Model: "deepseek-chat", AuthMode: "apiKey"},
		},
		CustomBaseUrls: map[string]string{},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	// 验证 ModelsConfig 创建
	if cfg.Models == nil {
		t.Fatal("Models should not be nil")
	}
	if cfg.Models.Providers == nil {
		t.Fatal("Providers should not be nil")
	}

	// 验证 provider 数量
	if len(cfg.Models.Providers) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(cfg.Models.Providers))
	}

	// 验证 anthropic
	ant := cfg.Models.Providers["anthropic"]
	if ant == nil {
		t.Fatal("anthropic provider missing")
	}
	if ant.APIKey != "sk-ant-xxx" {
		t.Errorf("anthropic APIKey: got %q, want %q", ant.APIKey, "sk-ant-xxx")
	}

	// 验证 deepseek (fallback)
	ds := cfg.Models.Providers["deepseek"]
	if ds == nil {
		t.Fatal("deepseek provider missing")
	}
	if ds.APIKey != "sk-ds-zzz" {
		t.Errorf("deepseek APIKey: got %q, want %q", ds.APIKey, "sk-ds-zzz")
	}
}

func TestConvertWizardV2PayloadToConfig_QwenAPIKeyUsesRuntimeProviderConfig(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"google": "google-key",
		},
		FallbackConfig: map[string]string{
			"qwen": "qwen-key",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"google": {Model: "gemini-3.1-pro-preview", AuthMode: "apiKey"},
			"qwen":   {Model: "qwen3.5-plus", AuthMode: "apiKey"},
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	qwen := cfg.Models.Providers["qwen"]
	if qwen == nil {
		t.Fatal("qwen runtime provider should be created")
	}
	if _, ok := cfg.Models.Providers["qwen-portal"]; ok {
		t.Fatal("qwen-portal config should not be materialized for API key mode")
	}
	if qwen.APIKey != "qwen-key" {
		t.Fatalf("qwen API key = %q", qwen.APIKey)
	}
	if qwen.BaseURL == "" {
		t.Fatal("qwen baseUrl should be populated from bridge config")
	}
	if len(qwen.Models) == 0 {
		t.Fatal("qwen models should be populated from bridge config")
	}
	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil || cfg.Agents.Defaults.Model.Fallbacks == nil {
		t.Fatal("fallbacks should be created")
	}
	gotFallbacks := *cfg.Agents.Defaults.Model.Fallbacks
	if len(gotFallbacks) != 1 || gotFallbacks[0] != "qwen/qwen3.5-plus" {
		t.Fatalf("fallbacks = %#v, want []string{\"qwen/qwen3.5-plus\"}", gotFallbacks)
	}
}

func TestConvertWizardV2PayloadToConfig_DeviceCodeProvidersKeepRuntimeConfigKeys(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		marker   string
		wantRef  string
		authMode string
	}{
		{
			name:     "qwen",
			provider: "qwen",
			model:    "qwen3.5-plus",
			marker:   "device-code-qwen",
			wantRef:  "qwen/qwen3.5-plus",
			authMode: "deviceCode",
		},
		{
			name:     "minimax",
			provider: "minimax",
			model:    "MiniMax-M2.5",
			marker:   "device-code-minimax",
			wantRef:  "minimax/MiniMax-M2.5",
			authMode: "deviceCode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := &WizardV2Payload{
				PrimaryConfig: map[string]string{
					tt.provider: tt.marker,
				},
				ProviderSelections: map[string]WizardV2ProviderSelection{
					tt.provider: {Model: tt.model, AuthMode: tt.authMode},
				},
			}

			cfg := &types.OpenAcosmiConfig{}
			convertWizardV2PayloadToConfig(payload, cfg)

			providerCfg := cfg.Models.Providers[tt.provider]
			if providerCfg == nil {
				t.Fatalf("%s runtime provider should be created", tt.provider)
			}
			if providerCfg.Auth != types.ModelAuthOAuth {
				t.Fatalf("%s auth mode = %q, want oauth", tt.provider, providerCfg.Auth)
			}
			if providerCfg.APIKey != "" {
				t.Fatalf("%s API key should stay empty in device flow mode", tt.provider)
			}
			if providerCfg.BaseURL == "" {
				t.Fatalf("%s baseUrl should be populated from bridge config", tt.provider)
			}
			if len(providerCfg.Models) == 0 {
				t.Fatalf("%s models should be populated from bridge config", tt.provider)
			}
			portalKey := tt.provider + "-portal"
			if _, ok := cfg.Models.Providers[portalKey]; ok {
				t.Fatalf("%s config should not be materialized; runtime key must remain %s", portalKey, tt.provider)
			}
			if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
				t.Fatal("agents.defaults.model should be created")
			}
			if cfg.Agents.Defaults.Model.Primary != tt.wantRef {
				t.Fatalf("primary = %q, want %q", cfg.Agents.Defaults.Model.Primary, tt.wantRef)
			}
		})
	}
}

func TestConvertWizardV2PayloadToConfig_PrimaryExtrasBecomeFallbacks(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"google": "google-key",
			"openai": "openai-key",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"google": {Model: "gemini-2.5-pro", AuthMode: "apiKey"},
			"openai": {Model: "gpt-4o", AuthMode: "apiKey"},
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("agents.defaults.model should be created")
	}
	if got := cfg.Agents.Defaults.Model.Primary; got != "google/gemini-2.5-pro" {
		t.Fatalf("primary model: got %q, want %q", got, "google/gemini-2.5-pro")
	}
	if cfg.Agents.Defaults.Model.Fallbacks == nil {
		t.Fatal("fallbacks should not be nil when multiple primary providers are configured")
	}
	wantFallbacks := []string{"openai/gpt-4o"}
	if got := *cfg.Agents.Defaults.Model.Fallbacks; len(got) != len(wantFallbacks) || got[0] != wantFallbacks[0] {
		t.Fatalf("fallbacks: got %#v, want %#v", got, wantFallbacks)
	}
}

func TestConvertWizardV2PayloadToConfig_FallbacksAppendSecondaryPageProviders(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"openai": "openai-key",
		},
		FallbackConfig: map[string]string{
			"deepseek": "deepseek-key",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"openai":   {Model: "gpt-4o", AuthMode: "apiKey"},
			"deepseek": {Model: "deepseek-chat", AuthMode: "apiKey"},
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("agents.defaults.model should be created")
	}
	if got := cfg.Agents.Defaults.Model.Primary; got != "openai/gpt-4o" {
		t.Fatalf("primary model: got %q, want %q", got, "openai/gpt-4o")
	}
	if cfg.Agents.Defaults.Model.Fallbacks == nil {
		t.Fatal("fallbacks should not be nil")
	}
	wantFallbacks := []string{"deepseek/deepseek-chat"}
	if got := *cfg.Agents.Defaults.Model.Fallbacks; len(got) != len(wantFallbacks) || got[0] != wantFallbacks[0] {
		t.Fatalf("fallbacks: got %#v, want %#v", got, wantFallbacks)
	}
}

func TestConvertWizardV2PayloadToConfig_EmptyValuesSkipped(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"anthropic": "sk-ant-xxx",
			"openai":    "",    // 空值不应写入
			"google":    "   ", // 纯空白不应写入
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	if len(cfg.Models.Providers) != 1 {
		t.Errorf("Expected 1 provider (only anthropic), got %d", len(cfg.Models.Providers))
	}
	if _, ok := cfg.Models.Providers["openai"]; ok {
		t.Error("openai should not be in providers (empty API key)")
	}
	if _, ok := cfg.Models.Providers["google"]; ok {
		t.Error("google should not be in providers (whitespace-only API key)")
	}
}

func TestConvertWizardV2PayloadToConfig_ChannelMapping(t *testing.T) {
	payload := &WizardV2Payload{
		ChannelConfig: WizardV2ChannelConfig{
			Feishu: WizardV2ChannelFeishu{
				AppID:     "cli_xxx",
				AppSecret: "secret-xxx",
			},
			Telegram: WizardV2ChannelTelegram{
				BotToken: "123456:ABC",
			},
			// WeCom 和 DingTalk 留空，不应写入
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	// 飞书
	if cfg.Channels == nil || cfg.Channels.Feishu == nil {
		t.Fatal("Feishu channel should be created")
	}
	if cfg.Channels.Feishu.AppID != "cli_xxx" {
		t.Errorf("Feishu AppID: got %q, want %q", cfg.Channels.Feishu.AppID, "cli_xxx")
	}
	if cfg.Channels.Feishu.Enabled == nil || !*cfg.Channels.Feishu.Enabled {
		t.Error("Feishu should be enabled")
	}

	// Telegram
	if cfg.Channels.Telegram == nil {
		t.Fatal("Telegram channel should be created")
	}
	if cfg.Channels.Telegram.BotToken != "123456:ABC" {
		t.Errorf("Telegram BotToken: got %q, want %q", cfg.Channels.Telegram.BotToken, "123456:ABC")
	}

	// WeCom 应为空
	if cfg.Channels.WeCom != nil {
		t.Error("WeCom should be nil (no input)")
	}

	// DingTalk 应为空
	if cfg.Channels.DingTalk != nil {
		t.Error("DingTalk should be nil (no input)")
	}
}

func TestConvertWizardV2PayloadToConfig_MemoryConfig(t *testing.T) {
	payload := &WizardV2Payload{
		MemoryConfig: WizardV2MemoryConfig{
			EnableVector: true,
			HostingType:  "cloud",
			APIEndpoint:  "https://my-qdrant.cloud.io",
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	if cfg.Memory == nil || cfg.Memory.UHMS == nil {
		t.Fatal("UHMS should be created")
	}
	if !cfg.Memory.UHMS.Enabled {
		t.Error("UHMS should be enabled")
	}
	if cfg.Memory.UHMS.VectorMode != "qdrant" {
		t.Errorf("VectorMode: got %q, want %q", cfg.Memory.UHMS.VectorMode, "qdrant")
	}
	if cfg.Memory.UHMS.QdrantEndpoint != "https://my-qdrant.cloud.io" {
		t.Errorf("QdrantEndpoint: got %q, want %q", cfg.Memory.UHMS.QdrantEndpoint, "https://my-qdrant.cloud.io")
	}
}

func TestConvertWizardV2PayloadToConfig_SecurityLevel(t *testing.T) {
	tests := []struct {
		level       string
		expectDeny  int // 对 DenyCommands 的预期长度: -1 = 不检查
		expectAllow int // 对 AllowCommands 的预期长度: -1 = 不检查
	}{
		{"deny", 1, -1},       // DenyCommands = ["*"]
		{"full", 0, 1},        // DenyCommands = [], AllowCommands = ["*"]
		{"allowlist", -1, -1}, // 默认拒绝列表
		{"sandboxed", -1, -1}, // 沙箱模式拒绝列表
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			payload := &WizardV2Payload{
				SecurityLevelConfig: tt.level,
			}
			cfg := &types.OpenAcosmiConfig{}
			convertWizardV2PayloadToConfig(payload, cfg)

			if cfg.Gateway == nil || cfg.Gateway.Nodes == nil {
				t.Fatal("Gateway.Nodes should be set")
			}

			if tt.expectDeny >= 0 && len(cfg.Gateway.Nodes.DenyCommands) != tt.expectDeny {
				t.Errorf("DenyCommands len: got %d, want %d", len(cfg.Gateway.Nodes.DenyCommands), tt.expectDeny)
			}
			if tt.expectAllow >= 0 && len(cfg.Gateway.Nodes.AllowCommands) != tt.expectAllow {
				t.Errorf("AllowCommands len: got %d, want %d", len(cfg.Gateway.Nodes.AllowCommands), tt.expectAllow)
			}
		})
	}
}

func TestConvertWizardV2PayloadToConfig_PreservesExistingConfig(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"openai": "new-key",
		},
	}

	// 创建已有配置
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"anthropic": {
					APIKey:  "existing-key",
					BaseURL: "https://api.anthropic.com",
				},
			},
		},
	}

	convertWizardV2PayloadToConfig(payload, cfg)

	// 新 provider 应被添加
	if cfg.Models.Providers["openai"] == nil {
		t.Fatal("openai should be added")
	}

	// 已有 provider 应保留
	ant := cfg.Models.Providers["anthropic"]
	if ant == nil {
		t.Fatal("existing anthropic should be preserved")
	}
	if ant.APIKey != "existing-key" {
		t.Errorf("existing anthropic APIKey should be preserved, got %q", ant.APIKey)
	}
}

func TestSyncWizardV2ExecSecurity_UpdatesExecApprovalsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	file, err := infra.EnsureExecApprovals()
	if err != nil {
		t.Fatalf("ensure exec approvals: %v", err)
	}
	file.Defaults = &infra.ExecApprovalsDefaults{
		Security: infra.ExecSecurityDeny,
		Ask:      infra.ExecAskOnMiss,
	}
	if err := infra.SaveExecApprovals(file); err != nil {
		t.Fatalf("seed exec approvals: %v", err)
	}
	socketPath := file.Socket.Path
	socketToken := file.Socket.Token

	if err := syncWizardV2ExecSecurity("sandboxed"); err != nil {
		t.Fatalf("sync wizard exec security: %v", err)
	}

	snapshot := infra.ReadExecApprovalsSnapshot()
	if snapshot.File == nil || snapshot.File.Defaults == nil {
		t.Fatal("expected exec approvals defaults to exist")
	}
	if got := snapshot.File.Defaults.Security; got != infra.ExecSecuritySandboxed {
		t.Fatalf("expected security %q, got %q", infra.ExecSecuritySandboxed, got)
	}
	if got := snapshot.File.Defaults.EscalationFallback; got != infra.ExecEscalationFallbackBase {
		t.Fatalf("expected escalation fallback %q, got %q", infra.ExecEscalationFallbackBase, got)
	}
	if got := snapshot.File.Defaults.Ask; got != infra.ExecAskOnMiss {
		t.Fatalf("expected ask policy to be preserved, got %q", got)
	}
	if snapshot.File.Socket == nil {
		t.Fatal("expected socket config to be preserved")
	}
	if snapshot.File.Socket.Path != socketPath || snapshot.File.Socket.Token != socketToken {
		t.Fatal("expected socket path/token to remain unchanged")
	}
}

func TestConvertWizardV2PayloadToConfig_CustomBaseURL(t *testing.T) {
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"custom-openai": "sk-custom-xxx",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"custom-openai": {Model: "qwen-max", AuthMode: "apiKey"},
		},
		CustomBaseUrls: map[string]string{
			"custom-openai": "https://api.openrouter.ai/v1",
		},
	}

	cfg := &types.OpenAcosmiConfig{}
	convertWizardV2PayloadToConfig(payload, cfg)

	custom := cfg.Models.Providers["custom-openai"]
	if custom == nil {
		t.Fatal("custom-openai provider should exist")
	}
	if custom.BaseURL != "https://api.openrouter.ai/v1" {
		t.Errorf("BaseURL: got %q, want %q", custom.BaseURL, "https://api.openrouter.ai/v1")
	}
	if custom.APIKey != "sk-custom-xxx" {
		t.Errorf("APIKey: got %q, want %q", custom.APIKey, "sk-custom-xxx")
	}
}

func TestParseWizardV2Payload(t *testing.T) {
	params := map[string]interface{}{
		"primaryConfig": map[string]interface{}{
			"anthropic": "sk-ant-xxx",
		},
		"securityLevelConfig": "allowlist",
		"securityAck":         true,
		"selectedSkills": map[string]interface{}{
			"bash":    true,
			"fs":      true,
			"browser": false,
			"mcp":     false,
		},
	}

	payload, err := parseWizardV2Payload(params)
	if err != nil {
		t.Fatalf("parseWizardV2Payload error: %v", err)
	}

	if payload.PrimaryConfig["anthropic"] != "sk-ant-xxx" {
		t.Errorf("PrimaryConfig[anthropic]: got %q", payload.PrimaryConfig["anthropic"])
	}
	if payload.SecurityLevelConfig != "allowlist" {
		t.Errorf("SecurityLevelConfig: got %q", payload.SecurityLevelConfig)
	}
	if !payload.SecurityAck {
		t.Error("SecurityAck should be true")
	}
	if !payload.SelectedSkills["bash"] {
		t.Error("SelectedSkills[bash] should be true")
	}
	if payload.SelectedSkills["browser"] {
		t.Error("SelectedSkills[browser] should be false")
	}
}

// ---------- Bug#11: Memory LLM 配置测试 ----------

// ---------- resetWizardManagedSections 测试 ----------

func TestResetWizardManagedSections_ClearAndPreserve(t *testing.T) {
	enabled := true
	port := 19001

	feishuCfg := &types.FeishuConfig{}
	feishuCfg.AppID = "cli_old"
	feishuCfg.Enabled = &enabled

	telegramCfg := &types.TelegramConfig{}
	telegramCfg.BotToken = "old-token"
	telegramCfg.Enabled = &enabled

	slackCfg := &types.SlackConfig{}
	slackCfg.BotToken = "xoxb-slack-keep"

	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Mode: "auto",
			Providers: map[string]*types.ModelProviderConfig{
				"anthropic": {APIKey: "sk-ant-old", BaseURL: "https://api.anthropic.com"},
				"deepseek":  {APIKey: "sk-ds-old"},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model:     &types.AgentModelListConfig{Primary: "anthropic/claude-sonnet-4-6"},
				Workspace: "/home/user/workspace",
			},
			List: []types.AgentListItemConfig{{ID: "main"}},
		},
		Tools: &types.ToolsConfig{
			Allow: []string{"group:fs", "group:runtime"},
			Deny:  []string{"group:dangerous"},
		},
		Channels: &types.ChannelsConfig{
			Feishu:   feishuCfg,
			Telegram: telegramCfg,
			Slack:    slackCfg,
		},
		Memory: &types.MemoryConfig{
			Backend: "vfs",
			UHMS: &types.MemoryUHMSConfig{
				Enabled:    true,
				VectorMode: "qdrant",
			},
		},
		SubAgents: &types.SubAgentConfig{
			OpenCoder:      &types.OpenCoderSettings{Provider: "deepseek", Model: "deepseek-chat"},
			ScreenObserver: &types.ScreenObserverSettings{Provider: "openai"},
		},
		Gateway: &types.GatewayConfig{
			Port:  &port,
			Nodes: &types.GatewayNodesConfig{DenyCommands: []string{"rm -rf /"}},
		},
	}

	resetWizardManagedSections(cfg)

	// Wizard 管理的段落应被清空
	if len(cfg.Models.Providers) != 0 {
		t.Errorf("Providers should be empty map, got %d entries", len(cfg.Models.Providers))
	}
	if cfg.Models.Mode != "auto" {
		t.Errorf("Models.Mode should be preserved, got %q", cfg.Models.Mode)
	}
	if cfg.Agents.Defaults.Model != nil {
		t.Error("Agents.Defaults.Model should be nil")
	}
	if cfg.Agents.Defaults.Workspace != "/home/user/workspace" {
		t.Errorf("Agents.Defaults.Workspace should be preserved, got %q", cfg.Agents.Defaults.Workspace)
	}
	if len(cfg.Agents.List) != 1 {
		t.Errorf("Agents.List should be preserved, got %d", len(cfg.Agents.List))
	}
	if cfg.Tools.Allow != nil {
		t.Error("Tools.Allow should be nil")
	}
	if len(cfg.Tools.Deny) != 1 || cfg.Tools.Deny[0] != "group:dangerous" {
		t.Errorf("Tools.Deny should be preserved, got %v", cfg.Tools.Deny)
	}
	if cfg.Channels.Feishu != nil {
		t.Error("Channels.Feishu should be nil")
	}
	if cfg.Channels.Telegram != nil {
		t.Error("Channels.Telegram should be nil")
	}
	if cfg.Channels.Slack == nil {
		t.Error("Channels.Slack should be preserved")
	} else if cfg.Channels.Slack.BotToken != "xoxb-slack-keep" {
		t.Errorf("Channels.Slack.BotToken should be preserved, got %q", cfg.Channels.Slack.BotToken)
	}
	if cfg.Memory.UHMS != nil {
		t.Error("Memory.UHMS should be nil")
	}
	if cfg.Memory.Backend != "vfs" {
		t.Errorf("Memory.Backend should be preserved, got %q", cfg.Memory.Backend)
	}
	if cfg.SubAgents.OpenCoder != nil {
		t.Error("SubAgents.OpenCoder should be nil")
	}
	if cfg.SubAgents.ScreenObserver != nil {
		t.Error("SubAgents.ScreenObserver should be nil")
	}
	if cfg.Gateway.Nodes != nil {
		t.Error("Gateway.Nodes should be nil")
	}
	if cfg.Gateway.Port == nil || *cfg.Gateway.Port != 19001 {
		t.Error("Gateway.Port should be preserved")
	}
}

func TestResetThenApply_NoStaleProviders(t *testing.T) {
	// 旧配置有 anthropic + deepseek
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"anthropic": {APIKey: "sk-ant-old", BaseURL: "https://api.anthropic.com"},
				"deepseek":  {APIKey: "sk-ds-old"},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "anthropic/claude-sonnet-4-6"},
			},
		},
	}

	// 重置
	resetWizardManagedSections(cfg)

	// 用只含 openai 的 payload 重新填充
	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"openai": "sk-openai-new",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"openai": {Model: "gpt-4.1", AuthMode: "apiKey"},
		},
	}
	convertWizardV2PayloadToConfig(payload, cfg)

	// 验证只有 openai，旧 provider 已消失
	if _, ok := cfg.Models.Providers["anthropic"]; ok {
		t.Error("anthropic should have been cleared by reset")
	}
	if _, ok := cfg.Models.Providers["deepseek"]; ok {
		t.Error("deepseek should have been cleared by reset")
	}
	if cfg.Models.Providers["openai"] == nil {
		t.Fatal("openai should be present after apply")
	}
	if cfg.Models.Providers["openai"].APIKey != "sk-openai-new" {
		t.Errorf("openai APIKey: got %q, want %q", cfg.Models.Providers["openai"].APIKey, "sk-openai-new")
	}

	// 验证主模型已更新
	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("Agents.Defaults.Model should be set after apply")
	}
	if cfg.Agents.Defaults.Model.Primary != "openai/gpt-4.1" {
		t.Errorf("Primary model: got %q, want %q", cfg.Agents.Defaults.Model.Primary, "openai/gpt-4.1")
	}
}

func TestResetThenApply_MemoryLLMFallsBackToPrimaryModel(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"deepseek": {
					APIKey:  "sk-ds-old",
					BaseURL: "https://api.deepseek.com",
				},
			},
		},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "deepseek/deepseek-chat"},
			},
		},
		Memory: &types.MemoryConfig{
			UHMS: &types.MemoryUHMSConfig{
				Enabled:     true,
				LLMProvider: "deepseek",
				LLMModel:    "deepseek-chat",
				LLMApiKey:   "sk-ds-old",
				LLMBaseURL:  "https://api.deepseek.com",
			},
		},
	}

	resetWizardManagedSections(cfg)

	payload := &WizardV2Payload{
		PrimaryConfig: map[string]string{
			"openai": "sk-openai-new",
		},
		ProviderSelections: map[string]WizardV2ProviderSelection{
			"openai": {Model: "gpt-4.1", AuthMode: "apiKey"},
		},
		MemoryConfig: WizardV2MemoryConfig{},
	}
	convertWizardV2PayloadToConfig(payload, cfg)

	if cfg.Memory == nil || cfg.Memory.UHMS == nil {
		t.Fatal("Memory.UHMS should be initialized")
	}
	if cfg.Memory.UHMS.LLMProvider != "" {
		t.Fatalf("Memory.UHMS.LLMProvider = %q, want empty to inherit primary model", cfg.Memory.UHMS.LLMProvider)
	}
	if cfg.Memory.UHMS.LLMModel != "" {
		t.Fatalf("Memory.UHMS.LLMModel = %q, want empty to inherit primary model", cfg.Memory.UHMS.LLMModel)
	}

	got := resolveEffectiveUHMSLLMConfig(cfg.Memory.UHMS, cfg)
	if got.Provider != "openai" {
		t.Fatalf("resolved provider = %q, want openai", got.Provider)
	}
	if got.Model != "gpt-4.1" {
		t.Fatalf("resolved model = %q, want gpt-4.1", got.Model)
	}
	if got.APIKey != "sk-openai-new" {
		t.Fatalf("resolved API key = %q, want sk-openai-new", got.APIKey)
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("resolved BaseURL = %q, want https://api.openai.com/v1", got.BaseURL)
	}
	if !got.Inherited {
		t.Fatal("resolved config should be marked as inherited")
	}
}

func TestApplyMemoryConfig_LLMFields(t *testing.T) {
	payload := &WizardV2Payload{
		MemoryConfig: WizardV2MemoryConfig{
			LLMProvider: "deepseek",
			LLMModel:    "deepseek-chat",
			LLMApiKey:   "sk-ds-test",
			LLMBaseURL:  "https://api.deepseek.com",
		},
	}
	cfg := &types.OpenAcosmiConfig{}
	applyMemoryConfig(payload, cfg)

	if cfg.Memory == nil || cfg.Memory.UHMS == nil {
		t.Fatal("Memory.UHMS should be initialized")
	}
	if cfg.Memory.UHMS.LLMProvider != "deepseek" {
		t.Errorf("LLMProvider: got %q, want %q", cfg.Memory.UHMS.LLMProvider, "deepseek")
	}
	if cfg.Memory.UHMS.LLMModel != "deepseek-chat" {
		t.Errorf("LLMModel: got %q, want %q", cfg.Memory.UHMS.LLMModel, "deepseek-chat")
	}
	if cfg.Memory.UHMS.LLMApiKey != "sk-ds-test" {
		t.Errorf("LLMApiKey: got %q, want %q", cfg.Memory.UHMS.LLMApiKey, "sk-ds-test")
	}
	if cfg.Memory.UHMS.LLMBaseURL != "https://api.deepseek.com" {
		t.Errorf("LLMBaseURL: got %q, want %q", cfg.Memory.UHMS.LLMBaseURL, "https://api.deepseek.com")
	}
}

func TestApplyMemoryConfig_EmptyLLMNotOverwrite(t *testing.T) {
	payload := &WizardV2Payload{
		MemoryConfig: WizardV2MemoryConfig{
			LLMProvider: "",
			LLMModel:    "",
		},
	}
	cfg := &types.OpenAcosmiConfig{
		Memory: &types.MemoryConfig{
			UHMS: &types.MemoryUHMSConfig{
				LLMProvider: "anthropic",
				LLMModel:    "claude-haiku-4-5-20251001",
			},
		},
	}
	applyMemoryConfig(payload, cfg)

	if cfg.Memory.UHMS.LLMProvider != "anthropic" {
		t.Errorf("LLMProvider should not be overwritten by empty, got %q", cfg.Memory.UHMS.LLMProvider)
	}
	if cfg.Memory.UHMS.LLMModel != "claude-haiku-4-5-20251001" {
		t.Errorf("LLMModel should not be overwritten by empty, got %q", cfg.Memory.UHMS.LLMModel)
	}
}

func TestApplyMemoryConfig_VectorPlusLLM(t *testing.T) {
	payload := &WizardV2Payload{
		MemoryConfig: WizardV2MemoryConfig{
			EnableVector: true,
			APIEndpoint:  "http://localhost:6334",
			LLMProvider:  "openai",
			LLMModel:     "gpt-4o-mini",
			LLMApiKey:    "sk-openai-test",
		},
	}
	cfg := &types.OpenAcosmiConfig{}
	applyMemoryConfig(payload, cfg)

	// 验证向量配置
	if cfg.Memory.UHMS.VectorMode != "qdrant" {
		t.Errorf("VectorMode: got %q, want %q", cfg.Memory.UHMS.VectorMode, "qdrant")
	}
	if cfg.Memory.UHMS.QdrantEndpoint != "http://localhost:6334" {
		t.Errorf("QdrantEndpoint: got %q, want %q", cfg.Memory.UHMS.QdrantEndpoint, "http://localhost:6334")
	}
	// 验证 LLM 配置
	if cfg.Memory.UHMS.LLMProvider != "openai" {
		t.Errorf("LLMProvider: got %q, want %q", cfg.Memory.UHMS.LLMProvider, "openai")
	}
	if cfg.Memory.UHMS.LLMModel != "gpt-4o-mini" {
		t.Errorf("LLMModel: got %q, want %q", cfg.Memory.UHMS.LLMModel, "gpt-4o-mini")
	}
}
