package skills

import (
	"testing"
)

func TestResolveConfigDir_UsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	if got := resolveConfigDir(); got != stateDir {
		t.Fatalf("resolveConfigDir() = %q, want %q", got, stateDir)
	}
}
