package config

// 配置路径解析 — 对应 src/config/paths.ts (275 行)
//
// 解析配置文件路径、状态目录、网关锁目录、OAuth 凭证路径等。
// 支持环境变量覆盖和旧版目录名向后兼容。

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
)

// ----- 常量 -----

const (
	// NewStateDirname 当前默认写入状态目录名
	NewStateDirname = ".openacosmi"
	// CompatibilityStateDirname 新品牌兼容状态目录名（双读阶段）
	CompatibilityStateDirname = ".crabclaw"
	// ConfigFilename 配置文件名
	ConfigFilename = "openacosmi.json"
	// DefaultGatewayPort 默认网关端口
	DefaultGatewayPort = 19001
	// OAuthFilename OAuth 凭证文件名
	OAuthFilename = "oauth.json"
	// ConfigBackupCount 配置备份数量
	ConfigBackupCount = 5
)

// LegacyStateDirnames 旧版状态目录名（按优先级排序）
var LegacyStateDirnames = []string{".openclaw", ".clawdbot", ".moltbot", ".moldbot"}

// LegacyConfigFilenames 旧版配置文件名
var LegacyConfigFilenames = []string{"openclaw.json", "clawdbot.json", "moltbot.json", "moldbot.json"}

// ----- Nix 模式 -----

// IsNixMode 检测当前是否 Nix 模式运行
func IsNixMode() bool {
	if envTrimmedCompat("CRABCLAW_NIX_MODE", "OPENACOSMI_NIX_MODE") == "1" {
		return true
	}
	return os.Getenv("OPENCLAW_NIX_MODE") == "1"
}

// ----- 主目录解析 -----

// ResolveHomeDir 解析用户主目录（支持 OPENACOSMI_HOME 环境变量覆盖，旧名 OPENCLAW_HOME fallback）
func ResolveHomeDir() string {
	return statepaths.ResolveHomeDir()
}

// expandTilde 展开 ~ 前缀
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// resolveUserPath 解析用户输入的路径（处理 ~ 和相对路径）
func resolveUserPath(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "~") {
		return filepath.Clean(expandTilde(trimmed))
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return trimmed
	}
	return abs
}

// ----- 状态目录 -----

// ResolveStateDir 解析状态目录
// 优先级:
// 1. OPENACOSMI_STATE_DIR / OPENCLAW_STATE_DIR / CLAWDBOT_STATE_DIR
// 2. 已存在且包含受管状态内容的 ~/.crabclaw
// 3. 已存在的 ~/.openacosmi
// 4. 已存在的旧版目录
// 5. 默认 ~/.openacosmi（保持旧默认写路径不变）
func ResolveStateDir() string {
	return statepaths.ResolveStateDir()
}

// ----- 配置文件路径 -----

// ResolveCanonicalConfigPath 获取规范配置文件路径
func ResolveCanonicalConfigPath() string {
	if v := envTrimmedCompat("CRABCLAW_CONFIG_PATH", "OPENACOSMI_CONFIG_PATH"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("OPENCLAW_CONFIG_PATH"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("CLAWDBOT_CONFIG_PATH"); v != "" {
		return resolveUserPath(v)
	}
	return filepath.Join(ResolveStateDir(), ConfigFilename)
}

// ResolveConfigPath 解析活跃配置路径（优先选择已存在的文件）
func ResolveConfigPath() string {
	if v := envTrimmedCompat("CRABCLAW_CONFIG_PATH", "OPENACOSMI_CONFIG_PATH"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("OPENCLAW_CONFIG_PATH"); v != "" {
		return resolveUserPath(v)
	}

	// 搜索候选配置文件
	candidates := ResolveConfigCandidates()
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}

	return ResolveCanonicalConfigPath()
}

// ResolveConfigCandidates 生成所有配置文件候选路径
func ResolveConfigCandidates() []string {
	if v := envTrimmedCompat("CRABCLAW_CONFIG_PATH", "OPENACOSMI_CONFIG_PATH"); v != "" {
		return []string{resolveUserPath(v)}
	}
	if v := envTrimmed("OPENCLAW_CONFIG_PATH"); v != "" {
		return []string{resolveUserPath(v)}
	}
	if v := envTrimmed("CLAWDBOT_CONFIG_PATH"); v != "" {
		return []string{resolveUserPath(v)}
	}

	var candidates []string
	home := ResolveHomeDir()
	suffix := statepaths.ResolveProfileSuffix()

	// 状态目录覆盖
	for _, envKey := range []string{"CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR", "OPENCLAW_STATE_DIR", "CLAWDBOT_STATE_DIR"} {
		if v := envTrimmed(envKey); v != "" {
			dir := resolveUserPath(v)
			candidates = append(candidates, filepath.Join(dir, ConfigFilename))
			for _, name := range LegacyConfigFilenames {
				candidates = append(candidates, filepath.Join(dir, name))
			}
		}
	}

	// 默认目录：先尝试新兼容目录，再尝试旧默认目录与更早的历史目录。
	allDirs := []string{
		filepath.Join(home, CompatibilityStateDirname+suffix),
		filepath.Join(home, NewStateDirname+suffix),
	}
	for _, name := range LegacyStateDirnames {
		allDirs = append(allDirs, filepath.Join(home, name+suffix))
	}
	for _, dir := range allDirs {
		candidates = append(candidates, filepath.Join(dir, ConfigFilename))
		for _, name := range LegacyConfigFilenames {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	return candidates
}

// ----- 网关端口 -----

// ResolveGatewayPort 解析网关端口 (env > config > default)
func ResolveGatewayPort(cfgPort *int) int {
	for _, key := range []string{"CRABCLAW_GATEWAY_PORT", "OPENACOSMI_GATEWAY_PORT", "OPENCLAW_GATEWAY_PORT", "CLAWDBOT_GATEWAY_PORT"} {
		if v := envTrimmed(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
	}
	if cfgPort != nil && *cfgPort > 0 {
		return *cfgPort
	}
	return DefaultGatewayPort
}

// ----- 网关锁目录 -----

// ResolveGatewayLockDir 解析网关锁文件目录
func ResolveGatewayLockDir() string {
	base := os.TempDir()
	uid := os.Getuid()
	if uid >= 0 && runtime.GOOS != "windows" {
		return filepath.Join(base, fmt.Sprintf("openacosmi-%d", uid))
	}
	return filepath.Join(base, "openacosmi")
}

// ----- OAuth 路径 -----

// ResolveOAuthDir 解析 OAuth 凭证目录
func ResolveOAuthDir() string {
	return statepaths.ResolveOAuthDir()
}

// ResolveOAuthPath 解析 OAuth 凭证文件路径
func ResolveOAuthPath() string {
	return filepath.Join(ResolveOAuthDir(), OAuthFilename)
}

// ResolveStoreDir 解析运行态 store 目录。
func ResolveStoreDir() string {
	return statepaths.ResolveStoreDir()
}

// ----- 辅助函数 -----

func envTrimmed(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envTrimmedCompat(keys ...string) string {
	for _, key := range keys {
		if v := envTrimmed(key); v != "" {
			return v
		}
	}
	return ""
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func stateDirHasManagedContent(dir string) bool {
	if !dirExists(dir) {
		return false
	}

	markers := []string{
		ConfigFilename,
		"credentials",
		"state",
		"store",
		"sessions",
		"agents",
		"memory",
		"_media",
		"workspace",
		"extensions",
		"logs",
		"exec-approvals.json",
		"gateway-token",
		"relay-token",
		"desktop-update-state.json",
		"oauth.json",
	}
	for _, marker := range markers {
		p := filepath.Join(dir, marker)
		if dirExists(p) || fileExists(p) {
			return true
		}
	}
	return false
}
