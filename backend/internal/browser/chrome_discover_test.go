package browser

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestDiscoverRunningChrome_NoPorts(t *testing.T) {
	// Probe ports that are very unlikely to have Chrome.
	result := DiscoverRunningChrome([]int{19999, 19998}, slog.Default())
	if result != nil {
		t.Errorf("expected nil for unused ports, got %+v", result)
	}
}

func TestDiscoverOrDefault_Empty(t *testing.T) {
	// Save and restore default ports to avoid probing real Chrome.
	orig := DefaultDiscoveryPorts
	DefaultDiscoveryPorts = []int{19997, 19996}
	defer func() { DefaultDiscoveryPorts = orig }()

	url := DiscoverOrDefault(slog.Default())
	if url != "" {
		t.Errorf("expected empty string for unused ports, got %q", url)
	}
}

func TestEnsureChromeResult_Fields(t *testing.T) {
	r := &EnsureChromeResult{
		WSURL:    "ws://127.0.0.1:18800/devtools/browser/abc",
		CDPURL:   "http://127.0.0.1:18800",
		Instance: nil,
		Launched: false,
	}
	if r.WSURL == "" {
		t.Error("WSURL should not be empty")
	}
	if r.Launched {
		t.Error("Launched should be false for discovered Chrome")
	}
	if r.Instance != nil {
		t.Error("Instance should be nil for discovered Chrome")
	}
}

func TestEnsureChrome_NoExecutable(t *testing.T) {
	// Override PATH to ensure no Chrome is found.
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	// Override discovery ports to avoid finding real Chrome.
	orig := DefaultDiscoveryPorts
	DefaultDiscoveryPorts = []int{19995}
	defer func() { DefaultDiscoveryPorts = orig }()

	// Also temporarily replace macAppPaths to avoid finding system Chrome.
	origMac := macAppPaths
	macAppPaths = []struct {
		path string
		kind BrowserExecutableKind
	}{{"/nonexistent/chrome", KindChrome}}
	defer func() { macAppPaths = origMac }()

	_, err := EnsureChrome(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("expected error when no Chrome executable exists")
	}
	if testing.Verbose() {
		t.Logf("expected error: %v", err)
	}
}
