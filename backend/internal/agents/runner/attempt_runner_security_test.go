package runner

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveEffectiveSecurityLevel_PrefersRuntimeGrant(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "allowlist"},
		},
	}
	params := AttemptParams{
		SecurityLevelFunc: func() string { return "full" },
	}

	if got := resolveEffectiveSecurityLevel(params, cfg); got != "full" {
		t.Fatalf("resolveEffectiveSecurityLevel() = %q, want full", got)
	}
}

func TestResolveEffectiveSecurityLevel_FallsBackToConfig(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "full"},
		},
	}

	if got := resolveEffectiveSecurityLevel(AttemptParams{}, cfg); got != "full" {
		t.Fatalf("resolveEffectiveSecurityLevel() = %q, want full", got)
	}
}

func TestResolveEffectiveSecurityLevel_NormalizesAliases(t *testing.T) {
	params := AttemptParams{
		SecurityLevelFunc: func() string { return " sandbox " },
	}

	if got := resolveEffectiveSecurityLevel(params, nil); got != "allowlist" {
		t.Fatalf("resolveEffectiveSecurityLevel() = %q, want allowlist", got)
	}
}
