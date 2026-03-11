package models

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// mockManagedProvider implements ManagedModelProvider for testing.
type mockManagedProvider struct {
	entries      []types.ManagedModelEntry
	defaultModel *types.ManagedModelEntry
}

func (m *mockManagedProvider) List() ([]types.ManagedModelEntry, error) {
	return m.entries, nil
}

func (m *mockManagedProvider) DefaultModel() *types.ManagedModelEntry {
	return m.defaultModel
}

func TestResolveManagedModelRef_UserExplicitCustomOverrides(t *testing.T) {
	// User explicitly configured a custom model → managed should NOT be used
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "anthropic/claude-opus-4-6"},
			},
		},
		Models: &types.ModelsConfig{
			ManagedModels: &types.ManagedModelsConfig{Enabled: true},
		},
	}
	managed := &mockManagedProvider{
		defaultModel: &types.ManagedModelEntry{
			ID: "m1", Provider: "openai", ModelID: "gpt-4o",
		},
	}

	ref := ResolveManagedModelRef(cfg, managed)
	if ref != nil {
		t.Errorf("expected nil (user explicit custom), got %+v", ref)
	}
}

func TestResolveManagedModelRef_ManagedPriority(t *testing.T) {
	// No user model configured, managed enabled → use managed
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			ManagedModels: &types.ManagedModelsConfig{Enabled: true},
		},
	}
	managed := &mockManagedProvider{
		defaultModel: &types.ManagedModelEntry{
			ID: "m1", Provider: "openai", ModelID: "gpt-4o",
		},
	}

	ref := ResolveManagedModelRef(cfg, managed)
	if ref == nil {
		t.Fatal("expected managed model ref, got nil")
	}
	if ref.Provider != "openai" || ref.Model != "gpt-4o" {
		t.Errorf("expected openai/gpt-4o, got %s/%s", ref.Provider, ref.Model)
	}
}

func TestResolveManagedModelRef_ManagedDisabled(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			ManagedModels: &types.ManagedModelsConfig{Enabled: false},
		},
	}
	managed := &mockManagedProvider{
		defaultModel: &types.ManagedModelEntry{
			ID: "m1", Provider: "openai", ModelID: "gpt-4o",
		},
	}

	ref := ResolveManagedModelRef(cfg, managed)
	if ref != nil {
		t.Errorf("expected nil (managed disabled), got %+v", ref)
	}
}

func TestResolveManagedModelRef_NoManagedConfig(t *testing.T) {
	// No ManagedModels config at all
	cfg := &types.OpenAcosmiConfig{}

	ref := ResolveManagedModelRef(cfg, nil)
	if ref != nil {
		t.Errorf("expected nil (no managed config), got %+v", ref)
	}
}

func TestResolveManagedModelRef_NilProvider(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			ManagedModels: &types.ManagedModelsConfig{Enabled: true},
		},
	}

	ref := ResolveManagedModelRef(cfg, nil)
	if ref != nil {
		t.Errorf("expected nil (nil provider), got %+v", ref)
	}
}

func TestResolveManagedModelRef_NoDefaultModel(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Models: &types.ModelsConfig{
			ManagedModels: &types.ManagedModelsConfig{Enabled: true},
		},
	}
	managed := &mockManagedProvider{
		defaultModel: nil,
	}

	ref := ResolveManagedModelRef(cfg, managed)
	if ref != nil {
		t.Errorf("expected nil (no default model), got %+v", ref)
	}
}

func TestResolveManagedModelRef_NilConfig(t *testing.T) {
	ref := ResolveManagedModelRef(nil, nil)
	if ref != nil {
		t.Errorf("expected nil (nil config), got %+v", ref)
	}
}

// Regression test: existing custom model selection is NOT affected
func TestResolveConfiguredModelRef_CustomUnchanged(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "deepseek/deepseek-chat"},
			},
		},
	}

	ref := ResolveConfiguredModelRef(cfg, DefaultProvider, DefaultModel)
	if ref.Provider != "deepseek" || ref.Model != "deepseek-chat" {
		t.Errorf("expected deepseek/deepseek-chat, got %s/%s", ref.Provider, ref.Model)
	}
}

func TestResolveConfiguredModelRef_DefaultUnchanged(t *testing.T) {
	ref := ResolveConfiguredModelRef(nil, DefaultProvider, DefaultModel)
	if ref.Provider != "anthropic" || ref.Model != "claude-opus-4-6" {
		t.Errorf("expected anthropic/claude-opus-4-6, got %s/%s", ref.Provider, ref.Model)
	}
}

func TestResolveConfiguredModelRef_QwenRuntimeProviderUnchanged(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "qwen/qwen3.5-plus"},
			},
		},
	}

	ref := ResolveConfiguredModelRef(cfg, DefaultProvider, DefaultModel)
	if ref.Provider != "qwen" || ref.Model != "qwen3.5-plus" {
		t.Errorf("expected qwen/qwen3.5-plus, got %s/%s", ref.Provider, ref.Model)
	}
}
