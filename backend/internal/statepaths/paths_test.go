package statepaths

import (
	"os"
	"path/filepath"
	"testing"
)

func setPathEnv(t *testing.T, key, value string) {
	t.Helper()
	oldValue, hadValue := os.LookupEnv(key)
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv(key, oldValue)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestResolveRuntimePathsUseSelectedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	setPathEnv(t, "CRABCLAW_STATE_DIR", stateDir)
	setPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setPathEnv(t, "CLAWDBOT_STATE_DIR", "")

	if got := ResolveRuntimeStateDir(); got != filepath.Join(stateDir, "state") {
		t.Fatalf("ResolveRuntimeStateDir() = %q", got)
	}
	if got := ResolveRuntimeAgentsRoot(); got != filepath.Join(stateDir, "state", "agents") {
		t.Fatalf("ResolveRuntimeAgentsRoot() = %q", got)
	}
	if got := ResolveDefaultRuntimeAgentDir(); got != filepath.Join(stateDir, "state", "agents", "main", "agent") {
		t.Fatalf("ResolveDefaultRuntimeAgentDir() = %q", got)
	}
	if got := ResolveStoreDir(); got != filepath.Join(stateDir, "store") {
		t.Fatalf("ResolveStoreDir() = %q", got)
	}
}

func TestResolveStoreDirPrefersExplicitOverride(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "custom-store")
	setPathEnv(t, "CRABCLAW_STORE_PATH", storeDir)
	setPathEnv(t, "OPENACOSMI_STORE_PATH", "")
	setPathEnv(t, "OPENCLAW_STORE_PATH", "")
	setPathEnv(t, "CLAWDBOT_STORE_PATH", "")

	if got := ResolveStoreDir(); got != storeDir {
		t.Fatalf("ResolveStoreDir() = %q, want %q", got, storeDir)
	}
}

func TestResolveStateDirPrefersCrabClawWhenItContainsRuntimeState(t *testing.T) {
	tmpHome := t.TempDir()
	setPathEnv(t, "OPENACOSMI_HOME", tmpHome)
	setPathEnv(t, "CRABCLAW_HOME", "")
	setPathEnv(t, "CRABCLAW_STATE_DIR", "")
	setPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setPathEnv(t, "CLAWDBOT_STATE_DIR", "")

	crabDir := filepath.Join(tmpHome, compatibilityStateDirname)
	openDir := filepath.Join(tmpHome, newStateDirname)
	if err := os.MkdirAll(filepath.Join(crabDir, "state", "agents", "main", "agent"), 0o755); err != nil {
		t.Fatalf("mkdir crab runtime state: %v", err)
	}
	if err := os.MkdirAll(openDir, 0o755); err != nil {
		t.Fatalf("mkdir open dir: %v", err)
	}

	if got := ResolveStateDir(); got != crabDir {
		t.Fatalf("ResolveStateDir() = %q, want %q", got, crabDir)
	}
}

func TestResolveStateDirPrefersCrabClawWhenItContainsStore(t *testing.T) {
	tmpHome := t.TempDir()
	setPathEnv(t, "OPENACOSMI_HOME", tmpHome)
	setPathEnv(t, "CRABCLAW_HOME", "")
	setPathEnv(t, "CRABCLAW_STATE_DIR", "")
	setPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setPathEnv(t, "CLAWDBOT_STATE_DIR", "")

	crabDir := filepath.Join(tmpHome, compatibilityStateDirname)
	openDir := filepath.Join(tmpHome, newStateDirname)
	if err := os.MkdirAll(filepath.Join(crabDir, "store"), 0o755); err != nil {
		t.Fatalf("mkdir crab store: %v", err)
	}
	if err := os.MkdirAll(openDir, 0o755); err != nil {
		t.Fatalf("mkdir open dir: %v", err)
	}

	if got := ResolveStateDir(); got != crabDir {
		t.Fatalf("ResolveStateDir() = %q, want %q", got, crabDir)
	}
}

func TestResolveStateDirUsesProfileSuffix(t *testing.T) {
	tmpHome := t.TempDir()
	setPathEnv(t, "OPENACOSMI_HOME", tmpHome)
	setPathEnv(t, "CRABCLAW_HOME", "")
	setPathEnv(t, "CRABCLAW_STATE_DIR", "")
	setPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setPathEnv(t, "CLAWDBOT_STATE_DIR", "")
	setPathEnv(t, "CRABCLAW_PROFILE", "")
	setPathEnv(t, "OPENACOSMI_PROFILE", "staging")

	want := filepath.Join(tmpHome, ".openacosmi-staging")
	if got := ResolveStateDir(); got != want {
		t.Fatalf("ResolveStateDir() = %q, want %q", got, want)
	}
}
