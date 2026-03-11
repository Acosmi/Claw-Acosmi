package main

import (
	"path/filepath"
	"testing"
)

func TestResolveSetupPathsUseResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")
	t.Setenv("OPENACOSMI_CONFIG", "")

	if got := resolveConfigPath(); got != filepath.Join(stateDir, "config.json") {
		t.Fatalf("resolveConfigPath() = %q", got)
	}
	if got := resolveWorkspacePath("agents"); got != filepath.Join(stateDir, "agents") {
		t.Fatalf("resolveWorkspacePath() = %q", got)
	}
	if got := resolveSessionsDir(); got != filepath.Join(stateDir, "sessions") {
		t.Fatalf("resolveSessionsDir() = %q", got)
	}
	if got := resolveAuthStorePath(); got != filepath.Join(stateDir, "auth.json") {
		t.Fatalf("resolveAuthStorePath() = %q", got)
	}
}

func TestResolveSetupPathsUseProfileAwareStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENACOSMI_HOME", home)
	t.Setenv("OPENACOSMI_PROFILE", "staging")
	t.Setenv("CRABCLAW_STATE_DIR", "")
	t.Setenv("OPENACOSMI_STATE_DIR", "")
	t.Setenv("OPENACOSMI_CONFIG", "")

	wantStateDir := filepath.Join(home, ".openacosmi-staging")
	if got := resolveWorkspacePath("agents"); got != filepath.Join(wantStateDir, "agents") {
		t.Fatalf("resolveWorkspacePath() = %q", got)
	}
	if got := resolveSessionsDir(); got != filepath.Join(wantStateDir, "sessions") {
		t.Fatalf("resolveSessionsDir() = %q", got)
	}
}
