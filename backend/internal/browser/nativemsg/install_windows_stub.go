//go:build !windows

package nativemsg

// Stub implementations for non-Windows platforms.
// The real implementations are in install_windows.go (build tag: windows).

func installWindows(_ InstallConfig, _ HostManifest) (int, error) {
	panic("installWindows called on non-Windows platform")
}

func uninstallWindows() {
	panic("uninstallWindows called on non-Windows platform")
}

func isInstalledWindows() bool {
	panic("isInstalledWindows called on non-Windows platform")
}

func manifestPathsWindows() []string {
	panic("manifestPathsWindows called on non-Windows platform")
}
