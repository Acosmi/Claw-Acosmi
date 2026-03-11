package main

// cmd_setup.go — setup 命令实现
// 对应 TS src/commands/setup.ts (76L) + onboard-interactive.ts (26L)

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/agents/auth"
	"github.com/Acosmi/ClawAcosmi/internal/tui"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

const defaultAgentWorkspaceDir = "agents"

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Initial setup wizard",
		Long:  "Run the interactive Crab Claw（蟹爪） setup wizard to configure your AI agents and channels.",
		RunE:  runSetupCommand,
	}
	cmd.Flags().String("workspace", "", "Agent workspace directory")
	cmd.AddCommand(newOnboardCmd())
	return cmd
}

func runSetupCommand(cmd *cobra.Command, args []string) error {
	workspace, _ := cmd.Flags().GetString("workspace")

	// 1. 解析配置路径
	configPath := resolveConfigPath()
	cfg, exists := readConfigFileJSON(configPath)

	// 2. 确保 workspace
	desiredWorkspace := strings.TrimSpace(workspace)
	if desiredWorkspace == "" {
		if cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Workspace != "" {
			desiredWorkspace = cfg.Agents.Defaults.Workspace
		} else {
			desiredWorkspace = defaultAgentWorkspaceDir
		}
	}

	// 3. 更新配置
	if cfg.Agents == nil {
		cfg.Agents = &types.AgentsConfig{}
	}
	if cfg.Agents.Defaults == nil {
		cfg.Agents.Defaults = &types.AgentDefaultsConfig{}
	}

	needsWrite := !exists || cfg.Agents.Defaults.Workspace != desiredWorkspace
	cfg.Agents.Defaults.Workspace = desiredWorkspace

	if needsWrite {
		if err := writeConfigFileJSON(configPath, cfg); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		if !exists {
			cmd.Printf("✅ Wrote %s\n", shortenHome(configPath))
		} else {
			cmd.Printf("✅ Updated %s (set agents.defaults.workspace)\n", shortenHome(configPath))
		}
	} else {
		cmd.Printf("✅ Config OK: %s\n", shortenHome(configPath))
	}

	// 4. 确保工作空间目录
	wsDir := resolveWorkspacePath(desiredWorkspace)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	cmd.Printf("✅ Workspace OK: %s\n", shortenHome(wsDir))

	// 5. 确保会话目录
	sessionsDir := resolveSessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}
	cmd.Printf("✅ Sessions OK: %s\n", shortenHome(sessionsDir))

	return nil
}

// ---------- 配置文件操作 ----------

func resolveConfigPath() string {
	if env := os.Getenv("OPENACOSMI_CONFIG"); env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openacosmi", "config.json")
}

func resolveWorkspacePath(workspace string) string {
	if filepath.IsAbs(workspace) {
		return workspace
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openacosmi", workspace)
}

func resolveSessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openacosmi", "sessions")
}

func resolveAuthStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openacosmi", "auth.json")
}

func readConfigFileJSON(configPath string) (*types.OpenAcosmiConfig, bool) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return &types.OpenAcosmiConfig{}, false
	}
	var cfg types.OpenAcosmiConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("setup: invalid config file", "path", configPath, "error", err)
		return &types.OpenAcosmiConfig{}, true
	}
	return &cfg, true
}

func writeConfigFileJSON(configPath string, cfg *types.OpenAcosmiConfig) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}

func shortenHome(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// ---------- Onboard 命令 ----------

func newOnboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Guided onboarding",
		Long:  "Step-by-step onboarding for new users — configure auth, providers, channels.",
		RunE:  runOnboardCommand,
	}

	// General
	cmd.Flags().Bool("yes", false, "Non-interactive mode (accept defaults)")
	cmd.Flags().String("provider", "", "AI provider (openai|anthropic|google|...)")
	cmd.Flags().String("mode", "local", "Onboarding mode (local|remote)")
	cmd.Flags().String("workspace", "", "Agent workspace directory")
	cmd.Flags().Bool("accept-risk", false, "Accept risk for non-interactive mode")
	cmd.Flags().String("auth-choice", "", "Auth choice override")

	// Provider API keys
	cmd.Flags().String("anthropic-api-key", "", "Anthropic API key")
	cmd.Flags().String("openai-api-key", "", "OpenAI API key")
	cmd.Flags().String("gemini-api-key", "", "Gemini API key")
	cmd.Flags().String("openrouter-api-key", "", "OpenRouter API key")
	cmd.Flags().String("ai-gateway-api-key", "", "AI Gateway API key")
	cmd.Flags().String("cloudflare-ai-gw-account-id", "", "Cloudflare AI Gateway account ID")
	cmd.Flags().String("cloudflare-ai-gw-gateway-id", "", "Cloudflare AI Gateway gateway ID")
	cmd.Flags().String("cloudflare-ai-gw-api-key", "", "Cloudflare AI Gateway API key")
	cmd.Flags().String("moonshot-api-key", "", "Moonshot API key")
	cmd.Flags().String("kimi-code-api-key", "", "Kimi Coding API key")
	cmd.Flags().String("synthetic-api-key", "", "Synthetic API key")
	cmd.Flags().String("venice-api-key", "", "Venice API key")
	cmd.Flags().String("zai-api-key", "", "ZAI API key")
	cmd.Flags().String("xiaomi-api-key", "", "Xiaomi API key")
	cmd.Flags().String("minimax-api-key", "", "MiniMax API key")
	cmd.Flags().String("openacosmi-zen-api-key", "", "Crab Claw Zen API key")
	cmd.Flags().String("xai-api-key", "", "xAI API key")
	cmd.Flags().String("qianfan-api-key", "", "Qianfan API key")

	// Gateway
	cmd.Flags().Int("gateway-port", 0, "Gateway port")
	cmd.Flags().String("gateway-bind", "", "Gateway bind (loopback|lan|auto|custom|tailnet)")
	cmd.Flags().String("gateway-auth", "", "Gateway auth (token|password)")
	cmd.Flags().String("gateway-token", "", "Gateway token")
	cmd.Flags().String("gateway-password", "", "Gateway password")

	// Tailscale
	cmd.Flags().String("tailscale", "", "Tailscale mode (off|serve|funnel)")
	cmd.Flags().Bool("tailscale-reset-on-exit", false, "Reset Tailscale on exit")

	// Misc
	cmd.Flags().Bool("install-daemon", false, "Install gateway as daemon")
	cmd.Flags().Bool("skip-skills", false, "Skip skills configuration")
	cmd.Flags().Bool("skip-health", false, "Skip health checks")
	cmd.Flags().Bool("skip-channels", false, "Skip channel configuration")
	cmd.Flags().String("node-manager", "", "Node manager (npm|pnpm|bun)")
	cmd.Flags().Bool("json", false, "Output as JSON")

	// Remote
	cmd.Flags().String("remote-url", "", "Remote gateway URL")
	cmd.Flags().String("remote-token", "", "Remote gateway token")

	return cmd
}

func runOnboardCommand(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	provider, _ := cmd.Flags().GetString("provider")

	// 1. 运行 setup 初始化
	if err := runSetupCommand(cmd, nil); err != nil {
		return err
	}

	cmd.Println()
	cmd.Println(tui.HeadingStyle.Render("🧙 Crab Claw（蟹爪） Onboarding"))
	cmd.Println()

	// 2. 准备 auth store
	storePath := resolveAuthStorePath()
	store := auth.NewAuthStore(storePath)
	if _, err := store.Load(); err != nil {
		slog.Warn("auth store load", "error", err)
	}

	// 3. 读取当前配置
	configPath := resolveConfigPath()
	cfg, _ := readConfigFileJSON(configPath)

	// 4. 选择认证提供商
	var authChoice AuthChoice

	if provider != "" {
		authChoice = provider
	} else if yes {
		cmd.Println(tui.MutedStyle.Render("Non-interactive mode: skipping auth setup"))
		authChoice = AuthChoiceSkip
	} else {
		// 创建 TUI prompter
		prompter := &cliPrompter{cmd: cmd}
		choice, err := PromptAuthChoiceGrouped(prompter, true)
		if err != nil {
			return fmt.Errorf("auth choice: %w", err)
		}
		authChoice = choice
	}

	if authChoice == AuthChoiceSkip {
		cmd.Println(tui.MutedStyle.Render("Auth setup skipped. Run `crabclaw setup onboard` later."))
	} else {
		// 5. 应用认证选择
		prompter := &cliPrompter{cmd: cmd}
		result, err := ApplyAuthChoice(ApplyAuthChoiceParams{
			AuthChoice:      authChoice,
			Config:          cfg,
			Prompter:        prompter,
			AuthStore:       store,
			SetDefaultModel: true,
		})
		if err != nil {
			return fmt.Errorf("apply auth: %w", err)
		}

		// 6. 写回配置
		if err := writeConfigFileJSON(configPath, result.Config); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		cmd.Println(tui.SuccessStyle.Render("✓ Auth configured"))
	}

	cmd.Println()
	cmd.Println(tui.SuccessStyle.Render("✓ Onboarding complete!"))
	cmd.Println(tui.MutedStyle.Render("  Run `crabclaw doctor` to verify your setup."))
	return nil
}

// ---------- CLI Prompter（简化版，委托 tui 包）----------

type cliPrompter struct {
	cmd *cobra.Command
}

func (p *cliPrompter) Intro(title string) {
	p.cmd.Println(tui.HeadingStyle.Render(title))
}

func (p *cliPrompter) Outro(message string) {
	p.cmd.Println(tui.SuccessStyle.Render(message))
}

func (p *cliPrompter) Note(message, title string) {
	if title != "" {
		p.cmd.Println(tui.AccentStyle.Render("ℹ " + title))
	}
	p.cmd.Println("  " + message)
	p.cmd.Println()
}

func (p *cliPrompter) Select(message string, options []tui.PromptOption, initialValue string) (string, error) {
	return tui.RunSelect(message, options, initialValue)
}

func (p *cliPrompter) MultiSelect(message string, options []tui.PromptOption, initialValues []string) ([]string, error) {
	return tui.RunMultiSelect(message, options, initialValues)
}

func (p *cliPrompter) TextInput(message, placeholder, initial string, validate func(string) string) (string, error) {
	return tui.RunTextInput(message, placeholder, initial, validate)
}

func (p *cliPrompter) Confirm(message string, initial bool) (bool, error) {
	return tui.RunConfirm(message, initial)
}
