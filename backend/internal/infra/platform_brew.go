//go:build darwin

// platform_brew.go — macOS/Linux Homebrew 路径检测 (S2-7: HIDDEN-8)

package infra

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveBrewPathDirs 返回 Homebrew 的候选 PATH 目录列表。
// 优先使用 HOMEBREW_PREFIX 环境变量，然后追加 Linuxbrew 和 macOS 默认路径。
func ResolveBrewPathDirs() []string {
	homeDir, _ := os.UserHomeDir()
	var dirs []string

	if prefix := strings.TrimSpace(os.Getenv("HOMEBREW_PREFIX")); prefix != "" {
		dirs = append(dirs, filepath.Join(prefix, "bin"), filepath.Join(prefix, "sbin"))
	}

	// Linuxbrew defaults.
	if homeDir != "" {
		dirs = append(dirs,
			filepath.Join(homeDir, ".linuxbrew", "bin"),
			filepath.Join(homeDir, ".linuxbrew", "sbin"),
		)
	}
	dirs = append(dirs, "/home/linuxbrew/.linuxbrew/bin", "/home/linuxbrew/.linuxbrew/sbin")

	// macOS defaults (also used by some Linux setups).
	dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin")

	return dirs
}

// ResolveBrewExecutable 返回首个可执行的 brew 二进制路径。
// 按优先级检查：HOMEBREW_BREW_FILE env → HOMEBREW_PREFIX/bin/brew → 静态候选路径。
func ResolveBrewExecutable() string {
	homeDir, _ := os.UserHomeDir()
	var candidates []string

	if brewFile := strings.TrimSpace(os.Getenv("HOMEBREW_BREW_FILE")); brewFile != "" {
		candidates = append(candidates, brewFile)
	}

	if prefix := strings.TrimSpace(os.Getenv("HOMEBREW_PREFIX")); prefix != "" {
		candidates = append(candidates, filepath.Join(prefix, "bin", "brew"))
	}

	// Linuxbrew defaults.
	if homeDir != "" {
		candidates = append(candidates, filepath.Join(homeDir, ".linuxbrew", "bin", "brew"))
	}
	candidates = append(candidates, "/home/linuxbrew/.linuxbrew/bin/brew")

	// macOS defaults.
	candidates = append(candidates, "/opt/homebrew/bin/brew", "/usr/local/bin/brew")

	for _, c := range candidates {
		if isExecutableFile(c) {
			return c
		}
	}

	return ""
}

// isExecutableFile 检查路径是否为可执行文件。
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

// ResolveBrewPrefix 返回 Homebrew 的安装前缀路径。
// 通过执行 `brew --prefix` 获取，失败时返回空字符串和错误。
func ResolveBrewPrefix() (string, error) {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		return "", err
	}
	out, err := exec.Command(brewPath, "--prefix").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsBrewInstalled 检查 Homebrew 是否已安装。
func IsBrewInstalled() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}
