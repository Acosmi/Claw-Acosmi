package models

import (
	"context"
	"fmt"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestNormalizeProviderId(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"anthropic", "anthropic"},
		{"Anthropic", "anthropic"},
		{"z.ai", "zai"},
		{"z-ai", "zai"},
		{"openacosmi-zen", "openacosmi"},
		{"qwen", "qwen"},
		{"kimi-code", "kimi-coding"},
		{"OpenAI", "openai"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeProviderId(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeProviderId(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		raw      string
		provider string
		wantP    string
		wantM    string
		wantNil  bool
	}{
		{"claude-3", "anthropic", "anthropic", "claude-3", false},
		{"anthropic/claude-3", "openai", "anthropic", "claude-3", false},
		{"openai/gpt-4", "anthropic", "openai", "gpt-4", false},
		{"qwen/qwen3.5-plus", "anthropic", "qwen", "qwen3.5-plus", false},
		{"", "anthropic", "", "", true},
		{"  /model", "anthropic", "", "", true},                          // empty provider
		{"opus-4.6", "anthropic", "anthropic", "claude-opus-4-6", false}, // alias
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := ParseModelRef(tt.raw, tt.provider)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil")
			}
			if got.Provider != tt.wantP || got.Model != tt.wantM {
				t.Errorf("got (%q, %q), want (%q, %q)", got.Provider, got.Model, tt.wantP, tt.wantM)
			}
		})
	}
}

func TestModelKey(t *testing.T) {
	if ModelKey("anthropic", "claude-3") != "anthropic/claude-3" {
		t.Error("unexpected key")
	}
}

func TestResolveThinkingDefault(t *testing.T) {
	// No config → off
	if got := ResolveThinkingDefault(nil, "anthropic", "claude-3", nil); got != ThinkOff {
		t.Errorf("nil config → %q, want off", got)
	}

	// Configured
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				ThinkingDefault: "high",
			},
		},
	}
	if got := ResolveThinkingDefault(cfg, "anthropic", "claude-3", nil); got != ThinkHigh {
		t.Errorf("configured → %q, want high", got)
	}

	// Reasoning model
	boolTrue := true
	catalog := []ModelCatalogEntry{
		{ID: "o1", Provider: "openai", Reasoning: &boolTrue},
	}
	if got := ResolveThinkingDefault(nil, "openai", "o1", catalog); got != ThinkLow {
		t.Errorf("reasoning model → %q, want low", got)
	}
}

func TestResolveFallbackCandidates(t *testing.T) {
	// No fallbacksOverride → BUG-8: primary tail is added (DefaultProvider/DefaultModel)
	// "anthropic/claude-3" + "anthropic/claude-opus-4-6" = 2 candidates
	candidates := ResolveFallbackCandidates(nil, "anthropic", "claude-3", nil)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (primary + default tail), got %d: %+v", len(candidates), candidates)
	}

	// Same model as default → dedup → only 1
	candidates = ResolveFallbackCandidates(nil, DefaultProvider, DefaultModel, nil)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate (dedup with default), got %d: %+v", len(candidates), candidates)
	}

	// With override fallbacks → no tail added (BUG-8 only applies when override is nil)
	candidates = ResolveFallbackCandidates(nil, "anthropic", "claude-3", []string{
		"openai/gpt-4",
		"anthropic/claude-3", // duplicate → dedup
		"google/gemini",
	})
	if len(candidates) != 3 {
		t.Fatalf("expected 3 unique candidates, got %d: %+v", len(candidates), candidates)
	}
}

func TestRunWithModelFallback(t *testing.T) {
	callCount := 0
	run := func(ctx context.Context, provider, model string) (string, error) {
		callCount++
		if callCount == 1 {
			return "", fmt.Errorf("rate limit exceeded (429)")
		}
		return "success from " + provider + "/" + model, nil
	}

	result, err := RunWithModelFallback(
		context.Background(),
		nil,
		"anthropic", "claude-3",
		[]string{"openai/gpt-4"},
		nil, // authStore
		run,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "openai" || result.Model != "gpt-4" {
		t.Errorf("expected openai/gpt-4, got %s/%s", result.Provider, result.Model)
	}
	if len(result.Attempts) != 1 {
		t.Errorf("expected 1 failed attempt, got %d", len(result.Attempts))
	}
}

func TestShouldFailover(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("rate limit exceeded"), true},
		{fmt.Errorf("HTTP 429 Too Many Requests"), true},
		{fmt.Errorf("overloaded_error"), true},         // TS: overloaded → rate_limit
		{fmt.Errorf("service unavailable 503"), false}, // TS: no overload/server bucket
		{fmt.Errorf("connection refused"), false},
		{fmt.Errorf("invalid API key"), true},       // classified as auth
		{fmt.Errorf("authentication failed"), true}, // classified as auth
		{fmt.Errorf("random unknown error"), false},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			if got := ShouldFailover(tt.err); got != tt.want {
				t.Errorf("ShouldFailover(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestResolveDefaultModelForAgent(t *testing.T) {
	// 基础 config：全局 model.primary = "claude-sonnet-4-5"
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{
					Primary: "claude-sonnet-4-5",
				},
			},
			List: []types.AgentListItemConfig{
				{
					ID: "mybot",
					Model: &types.AgentModelConfig{
						Primary: "openai/gpt-4-turbo",
					},
				},
				{
					ID:   "nomodel",
					Name: "Agent Without Model",
				},
			},
		},
	}

	t.Run("no_agentID_returns_global", func(t *testing.T) {
		ref := ResolveDefaultModelForAgent(cfg, "")
		if ref.Provider != "anthropic" || ref.Model != "claude-sonnet-4-5" {
			t.Errorf("expected anthropic/claude-sonnet-4-5, got %s/%s", ref.Provider, ref.Model)
		}
	})

	t.Run("agent_without_model_returns_global", func(t *testing.T) {
		ref := ResolveDefaultModelForAgent(cfg, "nomodel")
		if ref.Provider != "anthropic" || ref.Model != "claude-sonnet-4-5" {
			t.Errorf("expected anthropic/claude-sonnet-4-5, got %s/%s", ref.Provider, ref.Model)
		}
	})

	t.Run("agent_with_model_override", func(t *testing.T) {
		ref := ResolveDefaultModelForAgent(cfg, "mybot")
		if ref.Provider != "openai" || ref.Model != "gpt-4-turbo" {
			t.Errorf("expected openai/gpt-4-turbo, got %s/%s", ref.Provider, ref.Model)
		}
	})

	t.Run("nil_config_returns_default", func(t *testing.T) {
		ref := ResolveDefaultModelForAgent(nil, "mybot")
		if ref.Provider != DefaultProvider || ref.Model != DefaultModel {
			t.Errorf("expected %s/%s, got %s/%s", DefaultProvider, DefaultModel, ref.Provider, ref.Model)
		}
	})

	t.Run("nonexistent_agent_returns_global", func(t *testing.T) {
		ref := ResolveDefaultModelForAgent(cfg, "ghost")
		if ref.Provider != "anthropic" || ref.Model != "claude-sonnet-4-5" {
			t.Errorf("expected anthropic/claude-sonnet-4-5, got %s/%s", ref.Provider, ref.Model)
		}
	})

	t.Run("does_not_mutate_original_config", func(t *testing.T) {
		originalPrimary := cfg.Agents.Defaults.Model.Primary
		_ = ResolveDefaultModelForAgent(cfg, "mybot")
		if cfg.Agents.Defaults.Model.Primary != originalPrimary {
			t.Errorf("original config mutated: %q → %q", originalPrimary, cfg.Agents.Defaults.Model.Primary)
		}
	})
}
