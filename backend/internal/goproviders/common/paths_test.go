package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	authstoretypes "github.com/Acosmi/ClawAcosmi/internal/goproviders/types"
	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
)

func setCommonPathEnv(t *testing.T, key, value string) {
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

func TestResolveDefaultAgentDirUsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	setCommonPathEnv(t, "CRABCLAW_STATE_DIR", stateDir)
	setCommonPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setCommonPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setCommonPathEnv(t, "CRABCLAW_AGENT_DIR", "")
	setCommonPathEnv(t, "OPENACOSMI_AGENT_DIR", "")
	setCommonPathEnv(t, "OPENCLAW_AGENT_DIR", "")
	setCommonPathEnv(t, "PI_CODING_AGENT_DIR", "")

	want := filepath.Join(stateDir, "state", "agents", "main", "agent")
	if got := ResolveDefaultAgentDir(); got != want {
		t.Fatalf("ResolveDefaultAgentDir() = %q, want %q", got, want)
	}
}

func TestResolveDefaultAgentDirPrefersExplicitAgentOverride(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "custom-agent")
	setCommonPathEnv(t, "CRABCLAW_AGENT_DIR", agentDir)
	setCommonPathEnv(t, "OPENACOSMI_AGENT_DIR", "")
	setCommonPathEnv(t, "OPENCLAW_AGENT_DIR", "")
	setCommonPathEnv(t, "PI_CODING_AGENT_DIR", "")

	if got := ResolveDefaultAgentDir(); got != agentDir {
		t.Fatalf("ResolveDefaultAgentDir() = %q, want %q", got, agentDir)
	}
}

func TestEnsureRuntimeScaffoldCreatesManagedRuntimeFiles(t *testing.T) {
	stateDir := t.TempDir()
	setCommonPathEnv(t, "CRABCLAW_STATE_DIR", stateDir)
	setCommonPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setCommonPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setCommonPathEnv(t, "CRABCLAW_AGENT_DIR", "")
	setCommonPathEnv(t, "OPENACOSMI_AGENT_DIR", "")
	setCommonPathEnv(t, "OPENCLAW_AGENT_DIR", "")
	setCommonPathEnv(t, "PI_CODING_AGENT_DIR", "")

	if err := EnsureRuntimeScaffold(""); err != nil {
		t.Fatalf("EnsureRuntimeScaffold: %v", err)
	}

	authPath := ResolveAuthStorePath("")
	if _, err := os.Stat(authPath); err != nil {
		t.Fatalf("auth store stat: %v", err)
	}
	if _, err := os.Stat(statepaths.ResolveOAuthDir()); err != nil {
		t.Fatalf("oauth dir stat: %v", err)
	}
	if _, err := os.Stat(statepaths.ResolveStoreDir()); err != nil {
		t.Fatalf("store dir stat: %v", err)
	}

	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth store: %v", err)
	}
	var store authstoretypes.AuthProfileStore
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatalf("unmarshal auth store: %v", err)
	}
	if store.Version != AuthStoreVersion {
		t.Fatalf("store version = %d, want %d", store.Version, AuthStoreVersion)
	}
	if store.Profiles == nil {
		t.Fatal("profiles should be initialized")
	}
}
