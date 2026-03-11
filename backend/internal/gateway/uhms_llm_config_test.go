package gateway

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveEffectiveUHMSLLMConfig_InheritsPrimaryModelProvider(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "openai/gpt-4o-mini"},
			},
		},
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"openai": {
					BaseURL: "https://api.openai.com/v1",
					APIKey:  "sk-main",
				},
			},
		},
	}

	got := resolveEffectiveUHMSLLMConfig(nil, cfg)

	if got.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", got.Provider)
	}
	if got.Model != "gpt-4o-mini" {
		t.Fatalf("Model = %q, want gpt-4o-mini", got.Model)
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want https://api.openai.com/v1", got.BaseURL)
	}
	if got.APIKey != "sk-main" {
		t.Fatalf("APIKey = %q, want sk-main", got.APIKey)
	}
	if !got.Inherited {
		t.Fatal("Inherited = false, want true")
	}
}

func TestResolveEffectiveUHMSLLMConfig_PrefersExplicitUHMSOverrides(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "anthropic/claude-sonnet-4-5-20250514"},
			},
		},
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"anthropic": {
					BaseURL: "https://api.anthropic.com",
					APIKey:  "sk-anthropic",
				},
			},
		},
	}

	got := resolveEffectiveUHMSLLMConfig(&types.MemoryUHMSConfig{
		LLMProvider: "deepseek",
		LLMModel:    "deepseek-chat",
		LLMBaseURL:  "https://api.deepseek.com",
		LLMApiKey:   "sk-deepseek",
	}, cfg)

	if got.Provider != "deepseek" {
		t.Fatalf("Provider = %q, want deepseek", got.Provider)
	}
	if got.Model != "deepseek-chat" {
		t.Fatalf("Model = %q, want deepseek-chat", got.Model)
	}
	if got.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL = %q, want https://api.deepseek.com", got.BaseURL)
	}
	if got.APIKey != "sk-deepseek" {
		t.Fatalf("APIKey = %q, want sk-deepseek", got.APIKey)
	}
	if got.Inherited {
		t.Fatal("Inherited = true, want false")
	}
}

func TestResolveEffectiveUHMSLLMConfig_FallsBackToProviderDefaultBaseURL(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}

	got := resolveEffectiveUHMSLLMConfig(&types.MemoryUHMSConfig{
		LLMProvider: "openai",
		LLMModel:    "gpt-4o-mini",
	}, cfg)

	if got.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", got.Provider)
	}
	if got.Model != "gpt-4o-mini" {
		t.Fatalf("Model = %q, want gpt-4o-mini", got.Model)
	}
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want https://api.openai.com/v1", got.BaseURL)
	}
	if got.Inherited {
		t.Fatal("Inherited = true, want false")
	}
}
