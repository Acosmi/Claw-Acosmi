package main

// setup_helpers.go — Onboarding 辅助函数
// TS 对照: src/commands/onboard-helpers.ts (478L)
//
// cmd 层新增函数 + gateway 包 shim。
// gateway 包已实现: ProbeGatewayReachable, WaitForGatewayReachable,
//   ResolveControlUiLinks, RandomToken, NormalizeGatewayTokenInput,
//   DetectBrowserOpenSupport, OpenURL, FormatControlUiSshHint, IsValidIPv4

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/Acosmi/ClawAcosmi/pkg/utils"
)

// DefaultWorkspace 默认 workspace 目录名。
// 对应 TS: export const DEFAULT_WORKSPACE = DEFAULT_AGENT_WORKSPACE_DIR
const DefaultWorkspace = "agents"

// ---------- ResetScope ----------

// ResetScope 重置范围。
type ResetScope string

const (
	ResetConfig      ResetScope = "config"
	ResetConfigCreds ResetScope = "config+creds+sessions"
	ResetFull        ResetScope = "full"
)

// HandleReset 按 scope 重置配置/凭证/workspace。
// 对应 TS handleReset (onboard-helpers.ts L310-320)。
func HandleReset(scope ResetScope, workspaceDir string) error {
	configPath := resolveConfigPath()

	// 删除配置文件
	if err := removeIfExists(configPath); err != nil {
		return fmt.Errorf("remove config: %w", err)
	}
	if scope == ResetConfig {
		return nil
	}

	// 删除凭证和会话
	configDir := config.ResolveStateDir()
	credentialsPath := filepath.Join(configDir, "credentials")
	sessionsDir := resolveSessionsDir()

	if err := removeIfExists(credentialsPath); err != nil {
		return fmt.Errorf("remove credentials: %w", err)
	}
	if err := removeAllIfExists(sessionsDir); err != nil {
		return fmt.Errorf("remove sessions: %w", err)
	}

	// full scope: 删除 workspace
	if scope == ResetFull {
		if err := removeAllIfExists(workspaceDir); err != nil {
			return fmt.Errorf("remove workspace: %w", err)
		}
	}
	return nil
}

// removeIfExists 删除单个文件（如存在）。
func removeIfExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

// removeAllIfExists 递归删除目录（如存在）。
func removeAllIfExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(path)
}

// ---------- Config Summary ----------

// SummarizeExistingConfig 汇总现有配置到可读字符串。
// 对应 TS summarizeExistingConfig (onboard-helpers.ts L38-66)。
func SummarizeExistingConfig(cfg *types.OpenAcosmiConfig) string {
	if cfg == nil {
		return "No key settings detected."
	}

	var rows []string

	if cfg.Agents != nil && cfg.Agents.Defaults != nil {
		d := cfg.Agents.Defaults
		if d.Workspace != "" {
			rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("workspace: %s", d.Workspace)))
		}
		if d.Model != nil && d.Model.Primary != "" {
			rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("model: %s", d.Model.Primary)))
		}
	}

	if cfg.Gateway != nil {
		gw := cfg.Gateway
		if gw.Mode != "" {
			rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("gateway.mode: %s", gw.Mode)))
		}
		if gw.Port != nil {
			rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("gateway.port: %d", *gw.Port)))
		}
		if gw.Bind != "" {
			rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("gateway.bind: %s", gw.Bind)))
		}
		if gw.Remote != nil && gw.Remote.URL != "" {
			rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("gateway.remote.url: %s", gw.Remote.URL)))
		}
	}

	if cfg.Skills != nil && cfg.Skills.Install != nil && cfg.Skills.Install.NodeManager != "" {
		rows = append(rows, utils.ShortenHomeInString(fmt.Sprintf("skills.nodeManager: %s", cfg.Skills.Install.NodeManager)))
	}

	if len(rows) == 0 {
		return "No key settings detected."
	}
	return strings.Join(rows, "\n")
}

// ---------- Wizard Metadata ----------

// WizardMetadata 向导运行元数据。
type WizardMetadata struct {
	Command string
	Mode    string
}

// ApplyWizardMetadata 写入 wizard 运行元数据到配置。
// 对应 TS applyWizardMetadata (onboard-helpers.ts L92-108)。
func ApplyWizardMetadata(cfg *types.OpenAcosmiConfig, meta WizardMetadata) {
	if cfg == nil {
		return
	}
	if cfg.Wizard == nil {
		cfg.Wizard = &types.OpenAcosmiWizardConfig{}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	cfg.Wizard.LastRunAt = now
	cfg.Wizard.LastRunCommand = meta.Command
	cfg.Wizard.LastRunMode = meta.Mode

	// Git commit from env
	commit := strings.TrimSpace(os.Getenv("GIT_COMMIT"))
	if commit == "" {
		commit = strings.TrimSpace(os.Getenv("GIT_SHA"))
	}
	if commit != "" {
		cfg.Wizard.LastRunCommit = commit
	}
}

// ---------- Workspace ----------

// EnsureWorkspaceAndSessions 确保 workspace 和 sessions 目录存在。
// 对应 TS ensureWorkspaceAndSessions (onboard-helpers.ts L267-280)。
func EnsureWorkspaceAndSessions(workspaceDir string, skipBootstrap bool) error {
	wsDir := resolveWorkspacePath(workspaceDir)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	sessionsDir := resolveSessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return fmt.Errorf("create sessions: %w", err)
	}
	return nil
}

// ---------- Node Manager ----------

// NodeManagerOption 节点管理器选项。
type NodeManagerOption struct {
	Value string
	Label string
}

// ResolveNodeManagerOptions 返回可用的节点管理器选项列表。
func ResolveNodeManagerOptions() []NodeManagerOption {
	return []NodeManagerOption{
		{Value: "npm", Label: "npm"},
		{Value: "pnpm", Label: "pnpm"},
		{Value: "bun", Label: "bun"},
	}
}

// ---------- Wizard Header ----------

// PrintWizardHeader 打印向导 ASCII art 头部。
func PrintWizardHeader() string {
	return strings.Join([]string{
		"▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
		"██▄▀▀▀▄██░▀▀▀▄██░▀▀▀▀██░▄██░██▄▀▀▀▄██▄▀▀▀▀██▄▀▀▀▄██▄▀▀▀▀██░▄█▄░██▀▀░▀▀██",
		"██░███░██░▀▀▀███░▀▀▀███░█▀▄░██░▀▀▀░██░██████░███░███▀▀▀▄██░█▀█░████░████",
		"██▀▄▄▄▀██░██████░▄▄▄▄██░███░██░███░██▀▄▄▄▄██▀▄▄▄▀██▄▄▄▄▀██░███░██▄▄░▄▄██",
		"▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
		"                      🦜 OPENACOSMI 🦜                      ",
		" ",
	}, "\n")
}

// ---------- Error 辅助 ----------

// SummarizeError 将 error 缩略为单行，最长 120 字符。
func SummarizeError(err error) string {
	if err == nil {
		return "unknown error"
	}
	raw := err.Error()
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if len(trimmed) > 120 {
				return trimmed[:119] + "…"
			}
			return trimmed
		}
	}
	return raw
}

// ---------- Binary Detection ----------

// DetectBinary 检测指定的命令是否存在于 PATH 中。
// 对应 TS detectBinary (onboard-helpers.ts L322-351)。
func DetectBinary(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	// 绝对路径或相对路径 → 检查文件存在性
	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") ||
		strings.Contains(name, "/") || strings.Contains(name, string(filepath.Separator)) {
		_, err := os.Stat(name)
		return err == nil
	}
	// PATH 查找
	_, err := exec.LookPath(name)
	return err == nil
}

// ---------- Trash ----------

// MoveToTrash 尝试使用 trash 命令安全删除文件/目录。
// 如果 trash 命令不可用则回退为直接删除。
// 对应 TS moveToTrash (onboard-helpers.ts L293-308)。
func MoveToTrash(pathname string) error {
	if pathname == "" {
		return nil
	}
	if _, err := os.Stat(pathname); os.IsNotExist(err) {
		return nil
	}

	// 优先尝试 trash 命令
	if DetectBinary("trash") {
		cmd := exec.Command("trash", pathname)
		if err := cmd.Run(); err == nil {
			slog.Info("moved to trash", "path", pathname)
			return nil
		}
	}

	// fallback: 直接删除
	if err := os.RemoveAll(pathname); err != nil {
		return fmt.Errorf("remove %s: %w", pathname, err)
	}
	slog.Info("removed (no trash)", "path", pathname)
	return nil
}

// ---------- Guard Cancel ----------

// GuardCancel 类型参数辅助 — 运行时环境中取消 abort。
// 对应 TS guardCancel (onboard-helpers.ts L30-36)。
// Go 实现中通过返回 error 替代 process.exit。
func GuardCancel(cancelled bool) error {
	if cancelled {
		return fmt.Errorf("setup cancelled")
	}
	return nil
}
