package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeBrowserExtensionStatusDefaultsToOff(t *testing.T) {
	req := httptest.NewRequest("GET", "/browser-extension/status", nil)
	rec := httptest.NewRecorder()

	serveBrowserExtensionStatus(rec, req, BrowserExtensionHandlerConfig{})

	if rec.Code != 200 {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}

	var info RelayStatusInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if info.Port != 0 {
		t.Fatalf("port = %d, want 0", info.Port)
	}
	if info.RelayURL != "" {
		t.Fatalf("relayUrl = %q, want empty", info.RelayURL)
	}
	if info.Connected {
		t.Fatal("connected = true, want false")
	}
	if info.Token != "" {
		t.Fatalf("token = %q, want empty", info.Token)
	}
}

func TestServeBrowserExtensionStatusUsesLiveRelayInfo(t *testing.T) {
	req := httptest.NewRequest("GET", "/browser-extension/status", nil)
	rec := httptest.NewRecorder()

	serveBrowserExtensionStatus(rec, req, BrowserExtensionHandlerConfig{
		GetRelayInfo: func() *RelayStatusInfo {
			return &RelayStatusInfo{
				Port:      19004,
				Token:     "secret",
				Connected: true,
				RelayURL:  "ws://127.0.0.1:19004/ws",
			}
		},
	})

	if rec.Code != 200 {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}

	var info RelayStatusInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if info.Port != 19004 {
		t.Fatalf("port = %d, want 19004", info.Port)
	}
	if info.Token != "secret" {
		t.Fatalf("token = %q, want %q", info.Token, "secret")
	}
	if !info.Connected {
		t.Fatal("connected = false, want true")
	}
	if info.RelayURL != "ws://127.0.0.1:19004/ws" {
		t.Fatalf("relayUrl = %q, want ws://127.0.0.1:19004/ws", info.RelayURL)
	}
}

func TestResolveExtensionDirFromPaths_FindsAppBundleResources(t *testing.T) {
	tmpDir := t.TempDir()
	extDir := filepath.Join(tmpDir, "Crab Claw.app", "Contents", "Resources", "browser-extension")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(`{"manifest_version":3}`), 0o644); err != nil {
		t.Fatal(err)
	}

	execPath := filepath.Join(tmpDir, "Crab Claw.app", "Contents", "MacOS", "CrabClaw")
	got := resolveExtensionDirFromPaths("", execPath, "")
	if got != extDir {
		t.Fatalf("resolveExtensionDirFromPaths() = %q, want %q", got, extDir)
	}
}

func TestResolveExtensionDirFromPaths_PrefersConfiguredDir(t *testing.T) {
	tmpDir := t.TempDir()
	configuredDir := filepath.Join(tmpDir, "configured-extension")
	if err := os.MkdirAll(configuredDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configuredDir, "manifest.json"), []byte(`{"manifest_version":3}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := resolveExtensionDirFromPaths(configuredDir, "", "")
	if got != configuredDir {
		t.Fatalf("resolveExtensionDirFromPaths() = %q, want %q", got, configuredDir)
	}
}
