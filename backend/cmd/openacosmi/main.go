// Package main 是 Open Acosmi CLI 的入口程序。
// 基于 Cobra 框架构建，对应 TS 端 src/cli/program/ 的完整 CLI 框架。
//
// DEPRECATED: Go CLI (openacosmi) 已弃用。请使用 Rust CLI 替代。
// Go 端仅保留 Gateway 服务 (cmd/acosmi)。
// 参见 docs/adr/001-rust-cli-go-gateway.md
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Acosmi/ClawAcosmi/internal/cli"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
)

// rootCmd 根命令（对应 TS buildProgram() + configureProgramHelp()）
var rootCmd = &cobra.Command{
	Use:   cli.CLIName,
	Short: "🦜 Claw Acosmi — AI Agent 管理平台",
	Long: `🦜 Claw Acosmi — 你的 AI Agent 管理平台

管理 AI Agent、消息频道、插件和服务。
支持 WhatsApp、Telegram、Discord、Slack、Signal、iMessage 等频道。`,
	Version: cli.Version,
	// PersistentPreRunE 对应 TS registerPreActionHooks()
	// 执行链: deprecation → profile → banner → verbose/yes → config guard → plugin registry
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 步骤 -1: 弃用警告
		// Go CLI 已弃用，引导用户使用 Rust CLI
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "⚠️  DEPRECATED: Go CLI (openacosmi) 已弃用。")
		fmt.Fprintln(os.Stderr, "   请使用 Rust 版本的 openacosmi CLI 替代。")
		fmt.Fprintln(os.Stderr, "   Gateway 服务请使用: acosmi (cmd/acosmi)")
		fmt.Fprintln(os.Stderr, "   参见: docs/adr/001-rust-cli-go-gateway.md")
		fmt.Fprintln(os.Stderr, "")

		// 步骤 0: i18n 初始化（从环境变量自动检测语言）
		i18n.InitFromEnv()
		if langOverride, _ := cmd.Flags().GetString("lang"); langOverride != "" {
			switch langOverride {
			case "zh-CN", "zh", "cn":
				i18n.SetLang(i18n.LangZhCN)
			case "en-US", "en":
				i18n.SetLang(i18n.LangEnUS)
			}
		}

		// 步骤 1: 解析 profile（--dev / --profile）
		profile, profErr := cli.ResolveProfile(os.Args)
		if profErr != nil {
			return profErr
		}
		if profile != "" {
			_ = os.Setenv("OPENACOSMI_PROFILE", profile)
		}

		// 步骤 2: 输出 banner
		// 对应 TS preaction.ts L35-39: update/completion/plugins-update 跳过
		cmdPath := cli.GetCommandPath(os.Args[1:], 2)
		hideBanner := len(cmdPath) > 0 && (cmdPath[0] == "doctor" ||
			cmdPath[0] == "completion" ||
			cmdPath[0] == "update" ||
			(cmdPath[0] == "plugins" && len(cmdPath) > 1 && cmdPath[1] == "update"))
		if !hideBanner && cmd.Name() != "version" {
			cli.EmitBanner()
		}

		// 步骤 3: 设置全局状态 (H2-1 修复)
		verbose, _ := cmd.Flags().GetBool("verbose")
		cli.SetVerbose(verbose)
		if verbose {
			_ = os.Setenv("OPENACOSMI_VERBOSE", "1")
		}
		// --yes flag（如果注册了的话）
		if yes, err := cmd.Flags().GetBool("yes"); err == nil && yes {
			cli.SetYes(true)
		}

		// 步骤 4: config guard (H2-3, H7-1 修复)
		if len(cmdPath) > 0 && cmdPath[0] != "doctor" && cmdPath[0] != "completion" {
			if err := cli.EnsureConfigReady(cmdPath); err != nil {
				return err
			}
		}

		// 步骤 5: plugin registry
		// 对应 TS PLUGIN_REQUIRED_COMMANDS = {message, channels, directory}
		pluginRequiredCommands := map[string]bool{
			"message":   true,
			"channels":  true,
			"directory": true,
		}
		if len(cmdPath) > 0 && pluginRequiredCommands[cmdPath[0]] {
			cli.EnsurePluginRegistryLoaded()
		}

		return nil
	},
	// 使用 SilenceErrors + SilenceUsage 由我们自己控制错误输出
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	// 全局 persistent flags（对应 TS program.option() 全局选项）
	rootCmd.PersistentFlags().Bool("dev", false,
		"Dev profile: isolate state under ~/.openacosmi-dev, default gateway port 19001")
	rootCmd.PersistentFlags().String("profile", "",
		"Use a named profile (isolates state/config under ~/.openacosmi-<name>)")
	rootCmd.PersistentFlags().Bool("verbose", false,
		"Enable verbose output")
	rootCmd.PersistentFlags().Bool("json", false,
		"Output in JSON format")
	rootCmd.PersistentFlags().Bool("no-color", false,
		"Disable ANSI colors")
	rootCmd.PersistentFlags().String("lang", "",
		"UI language override (zh-CN, en-US)")

	// 注册所有子命令
	registerAllCommands()
}

// registerAllCommands 注册所有 CLI 子命令。
// 对应 TS commandRegistry + entries（register.subclis.ts）。
func registerAllCommands() {
	rootCmd.AddCommand(
		newGatewayCmd(),
		newAgentCmd(),
		newSandboxCmd(),
		newStatusCmd(),
		newSetupCmd(),
		newModelsCmd(),
		newChannelsCmd(),
		newDaemonCmd(),
		newCronCmd(),
		newDoctorCmd(),
		newSkillsCmd(),
		newHooksCmd(),
		newPluginsCmd(),
		newBrowserCmd(),
		newNodesCmd(),
		newInfraCmd(),
		newSecurityCmd(),
		newMiscCmd(),
	)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
