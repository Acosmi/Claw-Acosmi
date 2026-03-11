// nativemsg/install.go — Cross-platform Chrome Native Messaging Host manifest installer.
//
// Installs the JSON manifest that tells Chrome where to find the native host binary.
// Supports macOS, Linux, and Windows. Handles Chrome, Chromium, Brave, and Edge.
package nativemsg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// HostName is the native messaging host identifier.
// Must match the name used in chrome.runtime.connectNative() calls.
const HostName = "com.acosmi.crabclaw"

// HostManifest is the JSON schema Chrome expects for a native messaging host.
type HostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// InstallConfig configures host manifest installation.
type InstallConfig struct {
	HostBinaryPath string   // Absolute path to the native host binary.
	ExtensionIDs   []string // Chrome extension IDs allowed to connect.
	Logger         *slog.Logger
}

// Install writes the native messaging host manifest to all detected browser directories.
// On Windows, it also creates HKCU registry entries pointing to the manifest file.
// Returns the number of manifests successfully written.
func Install(cfg InstallConfig) (int, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	if cfg.HostBinaryPath == "" {
		return 0, fmt.Errorf("host binary path is required")
	}

	// Resolve to absolute path.
	absPath, err := filepath.Abs(cfg.HostBinaryPath)
	if err != nil {
		return 0, fmt.Errorf("resolve binary path: %w", err)
	}

	// Build allowed_origins from extension IDs.
	origins := make([]string, len(cfg.ExtensionIDs))
	for i, id := range cfg.ExtensionIDs {
		origins[i] = "chrome-extension://" + id + "/"
	}

	manifest := HostManifest{
		Name:           HostName,
		Description:    "CrabClaw Native Messaging Host — bridges Chrome extension to CrabClaw gateway",
		Path:           absPath,
		Type:           "stdio",
		AllowedOrigins: origins,
	}

	// Windows: manifest file + registry entries.
	if runtime.GOOS == "windows" {
		return installWindows(cfg, manifest)
	}

	// macOS / Linux: manifest files in browser directories.
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshal manifest: %w", err)
	}

	dirs := manifestDirs()
	if len(dirs) == 0 {
		return 0, fmt.Errorf("no browser native messaging directories found for %s", runtime.GOOS)
	}

	filename := HostName + ".json"
	installed := 0
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			cfg.Logger.Warn("cannot create manifest dir", "dir", dir, "err", err)
			continue
		}
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			cfg.Logger.Warn("cannot write manifest", "path", path, "err", err)
			continue
		}
		cfg.Logger.Info("native messaging host manifest installed", "path", path)
		installed++
	}

	if installed == 0 {
		return 0, fmt.Errorf("failed to install manifest to any browser directory")
	}
	return installed, nil
}

// Uninstall removes the native messaging host manifest from all browser directories.
// On Windows, it also removes the registry entries.
func Uninstall() {
	if runtime.GOOS == "windows" {
		uninstallWindows()
		return
	}
	filename := HostName + ".json"
	for _, dir := range manifestDirs() {
		path := filepath.Join(dir, filename)
		os.Remove(path)
	}
}

// IsInstalled checks if the manifest exists in at least one browser directory.
// On Windows, checks registry entries.
func IsInstalled() bool {
	if runtime.GOOS == "windows" {
		return isInstalledWindows()
	}
	filename := HostName + ".json"
	for _, dir := range manifestDirs() {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// ManifestPaths returns all paths where the manifest would be/is installed.
// On Windows, includes registry key paths and the manifest file path.
func ManifestPaths() []string {
	if runtime.GOOS == "windows" {
		return manifestPathsWindows()
	}
	filename := HostName + ".json"
	dirs := manifestDirs()
	paths := make([]string, len(dirs))
	for i, dir := range dirs {
		paths[i] = filepath.Join(dir, filename)
	}
	return paths
}

// manifestDirsOverride allows tests to inject custom directories.
var manifestDirsOverride []string

// manifestDirs returns platform-specific native messaging host manifest directories.
func manifestDirs() []string {
	if manifestDirsOverride != nil {
		return manifestDirsOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	switch runtime.GOOS {
	case "darwin":
		return []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"),
			filepath.Join(home, "Library", "Application Support", "Chromium", "NativeMessagingHosts"),
			filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
			filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "NativeMessagingHosts"),
		}
	case "linux":
		return []string{
			filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"),
			filepath.Join(home, ".config", "chromium", "NativeMessagingHosts"),
			filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
			filepath.Join(home, ".config", "microsoft-edge", "NativeMessagingHosts"),
		}
	default:
		// Windows uses registry — handled separately if needed.
		return nil
	}
}
