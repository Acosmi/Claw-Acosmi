package browser

import (
	"path/filepath"
	"testing"
)

func TestDefaultChromeDataDir_UsesResolvedStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	if got := defaultChromeDataDir("default"); got != filepath.Join(stateDir, "browser-profiles", "default") {
		t.Fatalf("defaultChromeDataDir() = %q", got)
	}
}
