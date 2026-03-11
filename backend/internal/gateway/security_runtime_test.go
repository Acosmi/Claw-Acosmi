package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveBaseSecurityLevelWithConfigPrefersExecApprovals(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	ocDir := filepath.Join(tmpHome, ".openacosmi")
	if err := os.MkdirAll(ocDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ocDir, "exec-approvals.json"), []byte(`{
  "version": 1,
  "defaults": {
    "security": "full"
  }
}`), 0o600); err != nil {
		t.Fatalf("write exec-approvals: %v", err)
	}

	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "deny"},
		},
	}

	if got := resolveBaseSecurityLevelWithConfig(cfg); got != "full" {
		t.Fatalf("expected persisted full security, got %q", got)
	}
}

func TestResolveApprovalWaitOutcomeTreatsUpgradedBaseLevelAsApproved(t *testing.T) {
	done, approved := resolveApprovalWaitOutcome("deny", "", EscalationStatus{
		HasPending:  false,
		HasActive:   false,
		ActiveLevel: "full",
	})
	if !done || !approved {
		t.Fatalf("expected permanent base upgrade to satisfy wait, got done=%v approved=%v", done, approved)
	}
}

func TestResolveApprovalWaitOutcomeRejectsUnchangedLevelWithoutGrant(t *testing.T) {
	done, approved := resolveApprovalWaitOutcome("allowlist", "", EscalationStatus{
		HasPending:  false,
		HasActive:   false,
		ActiveLevel: "allowlist",
	})
	if !done || approved {
		t.Fatalf("expected unchanged level to resolve as not approved, got done=%v approved=%v", done, approved)
	}
}

func TestResolveApprovalWaitOutcomeRequiresMatchingGrantForSpecificRequest(t *testing.T) {
	done, approved := resolveApprovalWaitOutcome("sandboxed", "esc_mount_2", EscalationStatus{
		HasPending: false,
		HasActive:  true,
		ActiveGrants: []*ActiveEscalationGrant{
			{ID: "esc_mount_1", Level: "sandboxed"},
		},
		ActiveLevel: "sandboxed",
	})
	if !done || approved {
		t.Fatalf("expected unrelated active grant to not satisfy request, got done=%v approved=%v", done, approved)
	}
}
