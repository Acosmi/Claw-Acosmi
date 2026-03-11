package telegram

import (
	"path/filepath"
	"testing"
)

func TestResolveUpdateOffsetPath_UsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	got := resolveUpdateOffsetPath("account-1")
	want := filepath.Join(stateDir, "telegram", "update-offset-account-1.json")
	if got != want {
		t.Fatalf("resolveUpdateOffsetPath() = %q, want %q", got, want)
	}
}
