package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/goproviders/common"
)

func setWizardPathEnv(t *testing.T, key, value string) {
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

func TestPersistOAuthTokenUsesResolvedRuntimeAgentDir(t *testing.T) {
	stateDir := t.TempDir()
	setWizardPathEnv(t, "CRABCLAW_STATE_DIR", stateDir)
	setWizardPathEnv(t, "OPENACOSMI_STATE_DIR", "")
	setWizardPathEnv(t, "OPENCLAW_STATE_DIR", "")
	setWizardPathEnv(t, "CRABCLAW_AGENT_DIR", "")
	setWizardPathEnv(t, "OPENACOSMI_AGENT_DIR", "")
	setWizardPathEnv(t, "OPENCLAW_AGENT_DIR", "")
	setWizardPathEnv(t, "PI_CODING_AGENT_DIR", "")

	persistOAuthToken("google", "access-token", "refresh-token", 12345)

	authPath := common.ResolveAuthStorePath("")
	wantPath := filepath.Join(stateDir, "state", "agents", "main", "agent", "auth-profiles.json")
	if authPath != wantPath {
		t.Fatalf("ResolveAuthStorePath(\"\") = %q, want %q", authPath, wantPath)
	}

	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth store: %v", err)
	}

	var store struct {
		Profiles map[string]map[string]interface{} `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatalf("unmarshal auth store: %v", err)
	}

	profile := store.Profiles["google:default"]
	if profile == nil {
		t.Fatalf("google:default profile missing: %#v", store.Profiles)
	}
	if profile["provider"] != "google" {
		t.Fatalf("provider = %#v", profile["provider"])
	}
	if profile["access"] != "access-token" {
		t.Fatalf("access = %#v", profile["access"])
	}
	if profile["refresh"] != "refresh-token" {
		t.Fatalf("refresh = %#v", profile["refresh"])
	}
	if profile["expires"] != float64(12345) {
		t.Fatalf("expires = %#v", profile["expires"])
	}
}
