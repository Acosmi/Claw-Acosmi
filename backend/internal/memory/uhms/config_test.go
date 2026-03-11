package uhms

import (
	"path/filepath"
	"testing"
)

func TestDefaultPathsUseResolvedStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	if got := defaultDBPath(); got != filepath.Join(stateDir, "memory", "uhms.db") {
		t.Fatalf("defaultDBPath() = %q", got)
	}
	if got := defaultVFSPath(); got != filepath.Join(stateDir, "memory", "vfs") {
		t.Fatalf("defaultVFSPath() = %q", got)
	}
	if got := defaultBootFilePath(); got != filepath.Join(stateDir, "memory", "boot.json") {
		t.Fatalf("defaultBootFilePath() = %q", got)
	}
}
