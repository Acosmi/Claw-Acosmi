package argus

// install.go — ARGUS-007: 安装位标准化
//
// EnsureUserBinLink 在 ~/.openacosmi/bin/ 下创建 argus-sensory 软链接，
// 指向已解析的二进制路径。确保 resolver 的 "user_bin" 层始终命中。

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// EnsureUserBinLink 在当前状态目录的 bin/argus-sensory 创建指向 resolvedPath 的符号链接。
//
// 行为:
//   - 如果目标已存在且指向同一路径 → 跳过（幂等）
//   - 如果目标已存在但指向不同路径 → 更新链接
//   - 如果目标不存在 → 创建目录并建立链接
//
// 返回 nil 表示链接已就绪，非 nil 表示链接创建失败（non-fatal）。
func EnsureUserBinLink(resolvedPath string) error {
	if resolvedPath == "" {
		return fmt.Errorf("argus: cannot create user bin link: empty resolved path")
	}

	// 确保 resolvedPath 是绝对路径
	absResolved, err := filepath.Abs(resolvedPath)
	if err != nil {
		return fmt.Errorf("argus: abs path: %w", err)
	}

	// 验证源文件存在
	if _, err := os.Stat(absResolved); err != nil {
		return fmt.Errorf("argus: source binary not found: %w", err)
	}

	binDir := filepath.Join(config.ResolveStateDir(), "bin")
	linkPath := filepath.Join(binDir, "argus-sensory")

	// 确保 bin 目录存在
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("argus: create bin dir: %w", err)
	}

	// 检查是否已存在相同链接（幂等）
	existing, err := os.Readlink(linkPath)
	if err == nil {
		if existing == absResolved {
			slog.Debug("argus: user bin link already correct", "link", linkPath, "target", absResolved)
			return nil
		}
		// 指向不同路径 → 删除旧链接
		slog.Info("argus: updating user bin link", "link", linkPath, "old", existing, "new", absResolved)
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("argus: remove old link: %w", err)
		}
	} else if _, statErr := os.Stat(linkPath); statErr == nil {
		// 不是链接但文件存在（可能是拷贝的二进制）→ 不覆盖
		slog.Warn("argus: user bin path exists but is not a symlink, skipping",
			"path", linkPath)
		return nil
	}

	// 创建新符号链接
	if err := os.Symlink(absResolved, linkPath); err != nil {
		return fmt.Errorf("argus: create symlink: %w", err)
	}

	slog.Info("argus: user bin link created",
		"link", linkPath,
		"target", absResolved)
	return nil
}

// StandardInstallPaths 返回 Argus 标准安装路径列表（供文档和 diagnose 参考）。
func StandardInstallPaths() []string {
	var paths []string

	// 1. 系统级
	paths = append(paths, "/Applications/Argus.app/Contents/MacOS/argus-sensory")

	// 2. 用户级
	if stateDir := config.ResolveStateDir(); stateDir != "" {
		paths = append(paths,
			filepath.Join(stateDir, "Argus.app", "Contents", "MacOS", "argus-sensory"),
			filepath.Join(stateDir, "bin", "argus-sensory"),
		)
	}

	return paths
}
