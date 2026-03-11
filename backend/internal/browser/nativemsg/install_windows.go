//go:build windows

package nativemsg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// registryKeys lists the HKCU registry paths Chrome-family browsers check for
// native messaging host manifests on Windows.
//
// Chrome docs: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
var registryKeys = []struct {
	browser string
	path    string
}{
	{"Chrome", `Software\Google\Chrome\NativeMessagingHosts\` + HostName},
	{"Chromium", `Software\Chromium\NativeMessagingHosts\` + HostName},
	{"Brave", `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + HostName},
	{"Edge", `Software\Microsoft\Edge\NativeMessagingHosts\` + HostName},
}

// windowsManifestDir returns the directory where the manifest JSON file is stored on Windows.
// Uses %LOCALAPPDATA%\CrabClaw\ as a stable, user-writable location.
func windowsManifestDir() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "CrabClaw", "NativeMessagingHosts")
}

// installWindows writes the manifest JSON file and creates registry entries for all browsers.
func installWindows(cfg InstallConfig, manifest HostManifest) (int, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshal manifest: %w", err)
	}

	// Write manifest file to a stable location.
	dir := windowsManifestDir()
	if dir == "" {
		return 0, fmt.Errorf("cannot determine manifest directory on Windows")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create manifest dir: %w", err)
	}

	manifestPath := filepath.Join(dir, HostName+".json")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return 0, fmt.Errorf("write manifest file: %w", err)
	}
	cfg.Logger.Info("native messaging host manifest written", "path", manifestPath)

	// Create registry entries pointing to the manifest file.
	installed := 0
	for _, rk := range registryKeys {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, rk.path, registry.SET_VALUE)
		if err != nil {
			cfg.Logger.Warn("cannot create registry key",
				"browser", rk.browser, "key", rk.path, "err", err)
			continue
		}
		if err := key.SetStringValue("", manifestPath); err != nil {
			cfg.Logger.Warn("cannot set registry value",
				"browser", rk.browser, "key", rk.path, "err", err)
			key.Close()
			continue
		}
		key.Close()
		cfg.Logger.Info("native messaging host registry entry created",
			"browser", rk.browser, "key", `HKCU\`+rk.path)
		installed++
	}

	if installed == 0 {
		return 0, fmt.Errorf("manifest file written but no registry entries created")
	}
	return installed, nil
}

// uninstallWindows removes registry entries and the manifest file.
func uninstallWindows() {
	for _, rk := range registryKeys {
		_ = registry.DeleteKey(registry.CURRENT_USER, rk.path)
	}

	dir := windowsManifestDir()
	if dir != "" {
		os.Remove(filepath.Join(dir, HostName+".json"))
	}
}

// isInstalledWindows checks if a registry entry exists for at least one browser.
func isInstalledWindows() bool {
	for _, rk := range registryKeys {
		key, err := registry.OpenKey(registry.CURRENT_USER, rk.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		val, _, err := key.GetStringValue("")
		key.Close()
		if err == nil && val != "" {
			return true
		}
	}
	return false
}

// manifestPathsWindows returns registry key paths and the manifest file path.
func manifestPathsWindows() []string {
	paths := make([]string, 0, len(registryKeys)+1)
	dir := windowsManifestDir()
	if dir != "" {
		paths = append(paths, filepath.Join(dir, HostName+".json"))
	}
	for _, rk := range registryKeys {
		paths = append(paths, `HKCU\`+rk.path)
	}
	return paths
}
