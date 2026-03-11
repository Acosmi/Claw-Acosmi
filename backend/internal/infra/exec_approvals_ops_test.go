package infra

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBaseExecSecurityPrefersExecApprovals(t *testing.T) {
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

	if got := ResolveBaseExecSecurity("deny"); got != ExecSecurityFull {
		t.Fatalf("expected exec-approvals security to win, got %q", got)
	}
}

func TestResolveBaseExecSecurityFallsBackToConfigAndAliases(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	if got := ResolveBaseExecSecurity("sandbox"); got != ExecSecuritySandboxed {
		t.Fatalf("expected sandbox alias to normalize to sandboxed (L2), got %q", got)
	}
	if got := ResolveBaseExecSecurity("sandboxed"); got != ExecSecuritySandboxed {
		t.Fatalf("expected sandboxed fallback to stay sandboxed, got %q", got)
	}
}

func TestResolveExecApprovalsPath_UsesResolvedStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	if got := ResolveExecApprovalsPath(); got != filepath.Join(stateDir, "exec-approvals.json") {
		t.Fatalf("ResolveExecApprovalsPath() = %q", got)
	}
	if got := ResolveExecApprovalsSocketPath(); got != filepath.Join(stateDir, "exec-approvals.sock") {
		t.Fatalf("ResolveExecApprovalsSocketPath() = %q", got)
	}
}
