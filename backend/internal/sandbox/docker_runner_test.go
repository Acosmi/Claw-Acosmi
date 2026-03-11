package sandbox

import (
	"path/filepath"
	"testing"
)

func TestResolveSandboxMounts_UsesResolvedStateDirDefaults(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	binds, _, _ := ResolveSandboxMounts(SandboxMountConfig{
		SecurityLevel: "deny",
		ProjectDir:    "/tmp/project",
		HomeDir:       "/Users/tester",
	})

	wantSkills := filepath.Join(stateDir, "skills") + ":/skills:ro"
	wantConfig := filepath.Join(stateDir, "openacosmi.json") + ":/etc/acosmi/config.json:ro"
	foundSkills := false
	foundConfig := false
	for _, bind := range binds {
		if bind == wantSkills {
			foundSkills = true
		}
		if bind == wantConfig {
			foundConfig = true
		}
	}
	if !foundSkills || !foundConfig {
		t.Fatalf("binds = %v, want %q and %q", binds, wantSkills, wantConfig)
	}
}
