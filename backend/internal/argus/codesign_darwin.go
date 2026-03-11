//go:build darwin

package argus

// codesign_darwin.go — macOS 代码签名与 .app bundle 发现
//
// 解决问题：每次 go build 产生新哈希，macOS TCC 按哈希追踪裸二进制的
// 辅助功能/屏幕录制授权，导致授权失效。
//
// 方案 A（优先）：发现 .app bundle 内的已签名二进制
// 方案 B（兜底）：对裸二进制用 "Argus Dev" 持久化证书签名

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
)

const (
	// devSignIdentity 开发者自签名证书名称（与 sign-console.sh 一致）
	devSignIdentity = "Argus Dev"

	// mcpBinaryIdentifier 裸二进制签名时使用的 identifier
	// 匹配 .app bundle 的 CFBundleIdentifier 前缀，保持 TCC 识别一致性
	mcpBinaryIdentifier = "com.argus.sensory.mcp"

	// appBundleExecutable .app bundle 内的二进制名（匹配 Info.plist CFBundleExecutable）
	appBundleExecutable = "argus-sensory"

	// altBundleExecutable go-sensory 独立 bundle 内的二进制名
	altBundleExecutable = "sensory-server"
)

// findAppBundleBinary 搜索 .app bundle 内的已签名二进制。
// 按优先级返回第一个找到的路径，或空字符串。
func FindAppBundleBinary() string {
	candidates := buildAppBundleCandidates()
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			slog.Debug("argus: found .app bundle binary", "path", path)
			return path
		}
	}
	return ""
}

// buildAppBundleCandidates 构造所有可能的 .app bundle 二进制路径。
func buildAppBundleCandidates() []string {
	var candidates []string

	// 0. 当前主 App bundle 内置的 Argus 资源
	if execPath, err := os.Executable(); err == nil {
		candidates = append(candidates, buildEmbeddedAppCandidates(execPath)...)
	}

	// 1. 项目构建产物目录（make app 产出）
	// 相对于 backend 的 Argus 构建目录
	if wd, err := os.Getwd(); err == nil {
		// 从 backend/ 向上推一级到 monorepo 根
		monoRoot := filepath.Dir(wd)
		candidates = append(candidates,
			filepath.Join(monoRoot, "Argus", "build", "Argus.app", "Contents", "MacOS", appBundleExecutable),
		)
		// go-sensory 独立 bundle
		candidates = append(candidates,
			filepath.Join(monoRoot, "Argus", "go-sensory", "Argus Sensory.app", "Contents", "MacOS", altBundleExecutable),
		)
	}

	// 2. 系统安装路径（pkg 安装后）
	candidates = append(candidates,
		filepath.Join("/Applications", "Argus.app", "Contents", "MacOS", appBundleExecutable),
	)

	// 3. 用户级安装路径
	if home, err := os.UserHomeDir(); err == nil {
		stateDir := statepaths.ResolveStateDir()
		candidates = append(candidates,
			filepath.Join(home, "Applications", "Argus.app", "Contents", "MacOS", appBundleExecutable),
			filepath.Join(stateDir, "Argus.app", "Contents", "MacOS", appBundleExecutable),
		)
	}

	return candidates
}

func buildEmbeddedAppCandidates(execPath string) []string {
	appBundle := enclosingAppBundle(execPath)
	if appBundle == "" {
		return nil
	}
	return []string{
		filepath.Join(appBundle, "Contents", "Helpers", "Argus.app", "Contents", "MacOS", appBundleExecutable),
		filepath.Join(appBundle, "Contents", "Resources", "Argus.app", "Contents", "MacOS", appBundleExecutable),
		filepath.Join(appBundle, "Contents", "Resources", "Argus", appBundleExecutable),
		filepath.Join(appBundle, "Contents", "Helpers", appBundleExecutable),
	}
}

func enclosingAppBundle(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	current := absPath
	for {
		if strings.HasSuffix(filepath.Base(current), ".app") {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// ensureCodeSigned 检查裸二进制的签名状态，必要时用 "Argus Dev" 证书签名。
// 返回 nil 表示签名有效（已有或新签），非 nil 表示签名失败（仍可运行，但授权不持久）。
func EnsureCodeSigned(binaryPath string) error {
	// 跳过 .app bundle 内的二进制（已由 make app / sign-console.sh 签名）
	if isInsideAppBundle(binaryPath) {
		slog.Debug("argus: binary is inside .app bundle, skipping re-sign", "path", binaryPath)
		return nil
	}

	// 检查当前签名状态
	if IsValidlySigned(binaryPath) {
		slog.Debug("argus: binary already signed", "path", binaryPath)
		return nil
	}

	// 检查 "Argus Dev" 证书是否在 Keychain
	if !hasSigningIdentity(devSignIdentity) {
		slog.Warn("argus: signing identity not found, authorization may not persist across rebuilds",
			"identity", devSignIdentity,
			"hint", "run Argus/scripts/package/create-dev-cert.sh to create it",
		)
		return fmt.Errorf("signing identity %q not found in keychain", devSignIdentity)
	}

	// 签名
	slog.Info("argus: signing bare binary for persistent authorization",
		"path", binaryPath, "identity", devSignIdentity)

	entitlements := findEntitlementsPlist()
	args := []string{
		"--force",
		"--options", "runtime",
		"-s", devSignIdentity,
		"--identifier", mcpBinaryIdentifier,
	}
	if entitlements != "" {
		args = append(args, "--entitlements", entitlements)
	}
	args = append(args, binaryPath)

	cmd := exec.Command("codesign", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("argus: codesign failed", "error", err, "output", string(output))
		return fmt.Errorf("codesign failed: %w", err)
	}

	slog.Info("argus: binary signed successfully — TCC authorization will persist",
		"identity", devSignIdentity, "identifier", mcpBinaryIdentifier)
	return nil
}

// isInsideAppBundle 判断路径是否在 .app bundle 内。
func isInsideAppBundle(path string) bool {
	return enclosingAppBundle(path) != ""
}

// IsValidlySigned 检查二进制是否有有效签名。
func IsValidlySigned(path string) bool {
	cmd := exec.Command("codesign", "--verify", "--verbose=0", path)
	return cmd.Run() == nil
}

// CodeSignIdentifier 返回 codesign 标识符；读取失败时返回空字符串。
func CodeSignIdentifier(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	cmd := exec.Command("codesign", "-dvv", path)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Identifier=") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "Identifier="))
	}
	return ""
}

// hasSigningIdentity 检查 Keychain 中是否存在指定签名身份。
func hasSigningIdentity(identity string) bool {
	cmd := exec.Command("security", "find-identity", "-v", "-p", "codesigning")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), identity)
}

// findEntitlementsPlist 查找 entitlements.plist 文件。
func findEntitlementsPlist() string {
	candidates := []string{}

	if wd, err := os.Getwd(); err == nil {
		monoRoot := filepath.Dir(wd)
		candidates = append(candidates,
			filepath.Join(monoRoot, "Argus", "scripts", "package", "entitlements.plist"),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
