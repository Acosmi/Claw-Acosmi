package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// 对应 TS src/cli/cli-utils.ts — 通用 CLI 工具函数

// FormatErrorMessage 将 error 格式化为用户友好消息。
func FormatErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// IsTruthyEnv 判断环境变量是否为 truthy 值。
// 对应 TS infra/env.ts isTruthyEnvValue()。
func IsTruthyEnv(key string) bool {
	val := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch val {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func envValueCompat(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func IsTruthyAnyEnv(keys ...string) bool {
	val := strings.TrimSpace(strings.ToLower(envValueCompat(keys...)))
	switch val {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// profileNameRe 合法的 profile 名称：仅字母、数字、下划线、连字符，长度 1-64。
var profileNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsValidProfileName 校验 profile 名称是否合法。
// 合法名称：仅 [a-zA-Z0-9_-]，长度 1-64。
func IsValidProfileName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	return profileNameRe.MatchString(name)
}

// ResolveProfile 解析 CLI profile 名称（--dev 或 --profile）。
// profile 影响 state/config 目录隔离。
// 非法名称返回 error。
func ResolveProfile(args []string) (string, error) {
	if HasFlag(args, "--dev") {
		return "dev", nil
	}
	name, found := GetFlagValue(args, "--profile")
	if found && name != "" {
		if !IsValidProfileName(name) {
			return "", fmt.Errorf("invalid profile name %q: must match [a-zA-Z0-9_-]{1,64}", name)
		}
		return name, nil
	}
	return "", nil
}

// ResolveStateDir 根据 profile 解析 state 目录。
func ResolveStateDir(profile string) string {
	if envDir := envValueCompat("CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR"); envDir != "" {
		return envDir
	}
	home := config.ResolveHomeDir()
	if strings.TrimSpace(home) == "" {
		home = "."
	}
	trimmed := strings.TrimSpace(profile)
	if trimmed != "" && !strings.EqualFold(trimmed, "default") {
		return filepath.Join(home, config.NewStateDirname+"-"+trimmed)
	}
	return filepath.Join(home, config.NewStateDirname)
}

// coreChannelOrder 核心频道名称列表（与 TS CHAT_CHANNEL_ORDER 对应）
var coreChannelOrder = []string{
	"whatsapp",
	"telegram",
	"discord",
	"slack",
	"signal",
	"imessage",
}

// ChannelOptions 已知频道名称列表（兼容旧代码引用）。
// 注意：优先使用 ResolveCliChannelOptions() 获取包含插件频道的完整列表。
var ChannelOptions = coreChannelOrder

// ResolveCliChannelOptions 解析 CLI 可用频道列表。
// 当 CRABCLAW_EAGER_CHANNEL_OPTIONS=1 或 OPENACOSMI_EAGER_CHANNEL_OPTIONS=1 时，加载插件注册表并合并插件频道。
// 对应 TS channel-options.ts resolveCliChannelOptions()。
func ResolveCliChannelOptions() []string {
	base := make([]string, len(coreChannelOrder))
	copy(base, coreChannelOrder)

	if !IsTruthyAnyEnv("CRABCLAW_EAGER_CHANNEL_OPTIONS", "OPENACOSMI_EAGER_CHANNEL_OPTIONS") {
		return base
	}

	// 加载插件注册表以获取动态频道
	EnsurePluginRegistryLoaded()
	reg := GetGlobalPluginRegistry()
	if reg == nil {
		return base
	}

	// 合并插件频道（去重）
	seen := make(map[string]bool, len(base))
	for _, ch := range base {
		seen[ch] = true
	}
	for _, ch := range reg.GetChannelNames() {
		if ch != "" && !seen[ch] {
			seen[ch] = true
			base = append(base, ch)
		}
	}
	return base
}

// FormatChannelOptions 格式化频道选项字符串（用 | 分隔）。
func FormatChannelOptions(extra ...string) string {
	channels := ResolveCliChannelOptions()
	all := make([]string, 0, len(extra)+len(channels))
	seen := make(map[string]bool)
	for _, ch := range append(extra, channels...) {
		if ch == "" || seen[ch] {
			continue
		}
		seen[ch] = true
		all = append(all, ch)
	}
	return strings.Join(all, "|")
}
