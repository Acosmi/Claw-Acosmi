package bridge

import (
	"testing"

	pkgtypes "github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestApplyProviderByID_Zai(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("zai", cfg, &ApplyOpts{APIKey: "test-key", SetDefaultModel: true})

	if cfg.Models == nil || cfg.Models.Providers == nil {
		t.Fatal("Models.Providers not initialized")
	}
	provCfg := cfg.Models.Providers["zai"]
	if provCfg == nil {
		t.Fatal("zai provider not found")
	}
	if provCfg.API != "openai-completions" {
		t.Errorf("expected API=openai-completions, got %s", provCfg.API)
	}
	if provCfg.APIKey != "test-key" {
		t.Errorf("expected APIKey=test-key, got %s", provCfg.APIKey)
	}
	if len(provCfg.Models) == 0 {
		t.Fatal("no models for zai provider")
	}
	// 检查第一个模型的 ContextWindow
	foundGLM5 := false
	for _, m := range provCfg.Models {
		if m.ID == "glm-5" {
			foundGLM5 = true
			if m.ContextWindow == 0 {
				t.Errorf("glm-5 ContextWindow should be >0, got 0")
			}
			break
		}
	}
	if !foundGLM5 {
		t.Error("glm-5 model not found in zai provider")
	}

	// 检查默认模型
	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("agents.defaults.model not set")
	}
	if cfg.Agents.Defaults.Model.Primary == "" {
		t.Error("primary model not set")
	}
}

func TestApplyProviderByID_Anthropic(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("anthropic", cfg, &ApplyOpts{APIKey: "sk-ant-xxx", SetDefaultModel: true})

	provCfg := cfg.Models.Providers["anthropic"]
	if provCfg == nil {
		t.Fatal("anthropic provider not found")
	}
	if provCfg.API != "anthropic-messages" {
		t.Errorf("expected API=anthropic-messages, got %s", provCfg.API)
	}
	if provCfg.APIKey != "sk-ant-xxx" {
		t.Errorf("expected APIKey=sk-ant-xxx, got %s", provCfg.APIKey)
	}
	if len(provCfg.Models) == 0 {
		t.Fatal("no models for anthropic")
	}
	// 验证 claude-sonnet-4-6 模型参数
	for _, m := range provCfg.Models {
		if m.ID == "claude-sonnet-4-6" {
			if m.ContextWindow != 200000 {
				t.Errorf("expected ContextWindow=200000, got %d", m.ContextWindow)
			}
			return
		}
	}
	t.Error("claude-sonnet-4-6 model not found")
}

func TestApplyProviderByID_DeepSeek(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("deepseek", cfg, &ApplyOpts{APIKey: "sk-deep-xxx"})

	provCfg := cfg.Models.Providers["deepseek"]
	if provCfg == nil {
		t.Fatal("deepseek provider not found")
	}
	if provCfg.API != "openai-completions" {
		t.Errorf("expected API=openai-completions, got %s", provCfg.API)
	}
	if provCfg.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("unexpected BaseURL: %s", provCfg.BaseURL)
	}
	if len(provCfg.Models) < 2 {
		t.Errorf("expected >=2 models, got %d", len(provCfg.Models))
	}
	for _, m := range provCfg.Models {
		if m.ID == "deepseek-chat" && m.ContextWindow != 131072 {
			t.Errorf("deepseek-chat ContextWindow expected 131072, got %d", m.ContextWindow)
		}
	}
}

func TestNormalizeProviderID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"zhipu", "zai"},
		{"doubao", "volcengine"},
		{"qwen", "qwen"},
		{"minimax", "minimax"},
		{"anthropic", "anthropic"},
		{"  OpenAI  ", "openai"},
	}
	for _, tt := range tests {
		got := NormalizeProviderID(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeProviderID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveAuthProviderID(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		authMode string
		want     string
	}{
		{name: "qwen api key", provider: "qwen", authMode: "apiKey", want: "qwen"},
		{name: "qwen device", provider: "qwen", authMode: "deviceCode", want: "qwen-portal"},
		{name: "minimax api key", provider: "minimax", authMode: "apiKey", want: "minimax"},
		{name: "minimax device", provider: "minimax", authMode: "deviceCode", want: "minimax-portal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveAuthProviderID(tt.provider, tt.authMode); got != tt.want {
				t.Fatalf("ResolveAuthProviderID(%q, %q) = %q, want %q", tt.provider, tt.authMode, got, tt.want)
			}
		})
	}
}

func TestGetDefaultModelRef(t *testing.T) {
	tests := []struct {
		providerID string
		wantPrefix string
	}{
		{"anthropic", "anthropic/"},
		{"zai", "zai/"},
		{"deepseek", "deepseek/"},
		{"moonshot", "moonshot/"},
	}
	for _, tt := range tests {
		got := GetDefaultModelRef(tt.providerID)
		if got == "" {
			t.Errorf("GetDefaultModelRef(%q) returned empty", tt.providerID)
		}
		if len(got) <= len(tt.wantPrefix) {
			t.Errorf("GetDefaultModelRef(%q) = %q, too short", tt.providerID, got)
		}
	}
}

func TestApplyProviderByID_MergeExisting(t *testing.T) {
	// 测试合并已有配置（不覆盖已有模型）
	cfg := &pkgtypes.OpenAcosmiConfig{
		Models: &pkgtypes.ModelsConfig{
			Providers: map[string]*pkgtypes.ModelProviderConfig{
				"zai": {
					BaseURL: "https://custom.example.com",
					APIKey:  "old-key",
					Models: []pkgtypes.ModelDefinitionConfig{
						{ID: "glm-5", Name: "My Custom GLM-5"},
					},
				},
			},
		},
	}

	ApplyProviderByID("zai", cfg, &ApplyOpts{APIKey: "new-key"})

	provCfg := cfg.Models.Providers["zai"]
	// API key 应该被更新
	if provCfg.APIKey != "new-key" {
		t.Errorf("APIKey not updated: got %s", provCfg.APIKey)
	}
	// 已有模型 glm-5 的自定义 Name 应该保留
	for _, m := range provCfg.Models {
		if m.ID == "glm-5" && m.Name != "My Custom GLM-5" {
			t.Errorf("existing model name overwritten: got %s", m.Name)
		}
	}
	// 应新增其他模型
	if len(provCfg.Models) < 2 {
		t.Errorf("expected new models to be merged, got %d models", len(provCfg.Models))
	}
}

func TestApplyProviderByID_CustomBaseURL(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("openai", cfg, &ApplyOpts{
		APIKey:  "sk-xxx",
		BaseURL: "https://my-proxy.example.com/v1",
	})

	provCfg := cfg.Models.Providers["openai"]
	if provCfg.BaseURL != "https://my-proxy.example.com/v1" {
		t.Errorf("custom BaseURL not applied: got %s", provCfg.BaseURL)
	}
}

func TestApplyProviderByID_QwenUsesRuntimeProviderConfig(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("qwen", cfg, &ApplyOpts{APIKey: "qwen-key", SetDefaultModel: true})

	provCfg := cfg.Models.Providers["qwen"]
	if provCfg == nil {
		t.Fatal("qwen provider not found")
	}
	if _, ok := cfg.Models.Providers["qwen-portal"]; ok {
		t.Fatal("qwen-portal provider should not be materialized for runtime config")
	}
	if provCfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("unexpected qwen BaseURL: %s", provCfg.BaseURL)
	}
	if provCfg.APIKey != "qwen-key" {
		t.Fatalf("unexpected qwen API key: %s", provCfg.APIKey)
	}
	if len(provCfg.Models) == 0 {
		t.Fatal("expected qwen models to be populated")
	}
	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("agents.defaults.model not set")
	}
	if cfg.Agents.Defaults.Model.Primary != "qwen/qwen3.5-plus" {
		t.Fatalf("primary model = %q, want %q", cfg.Agents.Defaults.Model.Primary, "qwen/qwen3.5-plus")
	}
}

func TestApplyProviderByID_MinimaxUsesHostedProviderConfig(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("minimax", cfg, &ApplyOpts{APIKey: "minimax-key", SetDefaultModel: true})

	provCfg := cfg.Models.Providers["minimax"]
	if provCfg == nil {
		t.Fatal("minimax provider not found")
	}
	if provCfg.API != "openai-completions" {
		t.Fatalf("unexpected minimax API: %s", provCfg.API)
	}
	if provCfg.BaseURL != "https://api.minimax.io/v1" {
		t.Fatalf("unexpected minimax BaseURL: %s", provCfg.BaseURL)
	}
	if provCfg.APIKey != "minimax-key" {
		t.Fatalf("unexpected minimax API key: %s", provCfg.APIKey)
	}
	if len(provCfg.Models) == 0 {
		t.Fatal("expected minimax models to be populated")
	}
	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("agents.defaults.model not set")
	}
	if cfg.Agents.Defaults.Model.Primary != "minimax/MiniMax-M2.5" {
		t.Fatalf("primary model = %q, want %q", cfg.Agents.Defaults.Model.Primary, "minimax/MiniMax-M2.5")
	}
}

func TestGetDefaultModelRef_Google_Gemini3(t *testing.T) {
	ref := GetDefaultModelRef("google")
	want := "google/gemini-3.1-pro-preview"
	if ref != want {
		t.Errorf("GetDefaultModelRef(google) = %q, want %q", ref, want)
	}
}

func TestApplyProviderByID_Google_Gemini3Models(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("google", cfg, &ApplyOpts{APIKey: "test-google-key"})

	provCfg := cfg.Models.Providers["google"]
	if provCfg == nil {
		t.Fatal("google provider not found")
	}
	if provCfg.API != "google-generative-ai" {
		t.Errorf("expected API=google-generative-ai, got %s", provCfg.API)
	}
	if len(provCfg.Models) < 5 {
		t.Errorf("expected >=5 models (3 new + 2 legacy), got %d", len(provCfg.Models))
	}

	// 验证 gemini-3.1-pro-preview 存在且元数据正确
	found := false
	for _, m := range provCfg.Models {
		if m.ID == "gemini-3.1-pro-preview" {
			found = true
			if m.ContextWindow != 1048576 {
				t.Errorf("gemini-3.1-pro ContextWindow: got %d, want 1048576", m.ContextWindow)
			}
			if !m.Reasoning {
				t.Error("gemini-3.1-pro should have Reasoning=true")
			}
			if m.MaxTokens != 65536 {
				t.Errorf("gemini-3.1-pro MaxTokens: got %d, want 65536", m.MaxTokens)
			}
			break
		}
	}
	if !found {
		t.Error("gemini-3.1-pro-preview model not found in google provider")
	}

	// 验证 gemini-2.5-pro 仍然存在（向后兼容）
	foundLegacy := false
	for _, m := range provCfg.Models {
		if m.ID == "gemini-2.5-pro" {
			foundLegacy = true
			break
		}
	}
	if !foundLegacy {
		t.Error("gemini-2.5-pro should still be present for backward compatibility")
	}
}

func TestHasProvider(t *testing.T) {
	if !HasProvider("anthropic") {
		t.Error("HasProvider(anthropic) should be true")
	}
	if !HasProvider("zai") {
		t.Error("HasProvider(zai) should be true")
	}
	if !HasProvider("google-gemini-cli") {
		t.Error("HasProvider(google-gemini-cli) should be true")
	}
	if HasProvider("nonexistent-provider-xyz") {
		t.Error("HasProvider(nonexistent-provider-xyz) should be false")
	}
}

func TestGetDefaultModelRef_GeminiCli(t *testing.T) {
	ref := GetDefaultModelRef("google-gemini-cli")
	want := "google-gemini-cli/gemini-3.1-pro-preview"
	if ref != want {
		t.Errorf("GetDefaultModelRef(google-gemini-cli) = %q, want %q", ref, want)
	}
}

func TestNormalizeProviderID_GeminiCli(t *testing.T) {
	got := NormalizeProviderID("gemini-cli")
	if got != "google-gemini-cli" {
		t.Errorf("NormalizeProviderID(gemini-cli) = %q, want %q", got, "google-gemini-cli")
	}
}

func TestApplyProviderByID_GeminiCli(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("google-gemini-cli", cfg, &ApplyOpts{SetDefaultModel: true})

	provCfg := cfg.Models.Providers["google-gemini-cli"]
	if provCfg == nil {
		t.Fatal("google-gemini-cli provider not found")
	}
	if provCfg.API != "google-generative-ai" {
		t.Errorf("expected API=google-generative-ai, got %s", provCfg.API)
	}
	// OAuth provider 不设 API key
	if provCfg.APIKey != "" {
		t.Errorf("expected empty APIKey for OAuth provider, got %s", provCfg.APIKey)
	}
	if len(provCfg.Models) < 5 {
		t.Errorf("expected >=5 models, got %d", len(provCfg.Models))
	}

	// 验证 gemini-3.1-pro-preview 存在
	found := false
	for _, m := range provCfg.Models {
		if m.ID == "gemini-3.1-pro-preview" {
			found = true
			if m.ContextWindow != 1048576 {
				t.Errorf("gemini-3.1-pro ContextWindow: got %d, want 1048576", m.ContextWindow)
			}
			break
		}
	}
	if !found {
		t.Error("gemini-3.1-pro-preview model not found")
	}

	// 验证默认模型已设置
	if cfg.Agents == nil || cfg.Agents.Defaults == nil || cfg.Agents.Defaults.Model == nil {
		t.Fatal("agents.defaults.model not set")
	}
	wantPrimary := "google-gemini-cli/gemini-3.1-pro-preview"
	if cfg.Agents.Defaults.Model.Primary != wantPrimary {
		t.Errorf("primary model = %q, want %q", cfg.Agents.Defaults.Model.Primary, wantPrimary)
	}
}

func TestApplyProviderByID_GeminiCli_ViaAlias(t *testing.T) {
	cfg := &pkgtypes.OpenAcosmiConfig{}
	ApplyProviderByID("gemini-cli", cfg, &ApplyOpts{})

	// 别名应该 normalize 到 google-gemini-cli
	provCfg := cfg.Models.Providers["google-gemini-cli"]
	if provCfg == nil {
		t.Fatal("google-gemini-cli provider not found (via gemini-cli alias)")
	}
	if provCfg.API != "google-generative-ai" {
		t.Errorf("expected API=google-generative-ai, got %s", provCfg.API)
	}
}
