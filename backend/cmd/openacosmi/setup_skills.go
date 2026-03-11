package main

// setup_skills.go — Onboarding Skills 安装向导
// TS 对照: src/commands/onboard-skills.ts (206L)
//
// 提供 SetupSkills — 技能发现、依赖安装、API key 配置。

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/tui"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- 辅助函数 ----------

// SummarizeInstallFailure 提取安装失败消息中的关键信息。
// 对应 TS: summarizeInstallFailure (onboard-skills.ts L9-16)。
func SummarizeInstallFailure(message string) string {
	// 移除 "Install failed (xxx):" 前缀
	cleaned := message
	if idx := strings.Index(strings.ToLower(cleaned), "install failed"); idx >= 0 {
		rest := cleaned[idx:]
		// 跳过 "Install failed" 及可选的 (exit N): 部分
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			cleaned = strings.TrimSpace(rest[colonIdx+1:])
		} else {
			cleaned = strings.TrimSpace(rest[len("install failed"):])
		}
	}
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	const maxLen = 140
	if len(cleaned) > maxLen {
		return cleaned[:maxLen-1] + "…"
	}
	return cleaned
}

// FormatSkillHint 生成技能选项的提示文本。
// 对应 TS: formatSkillHint (onboard-skills.ts L18-30)。
func FormatSkillHint(description string, installLabel string) string {
	desc := strings.TrimSpace(description)
	label := strings.TrimSpace(installLabel)

	var combined string
	switch {
	case desc != "" && label != "":
		combined = desc + " — " + label
	case desc != "":
		combined = desc
	case label != "":
		combined = label
	default:
		return "install"
	}

	const maxLen = 90
	if len(combined) > maxLen {
		return combined[:maxLen-1] + "…"
	}
	return combined
}

// UpsertSkillEntry 更新/插入技能配置条目。
// 对应 TS: upsertSkillEntry (onboard-skills.ts L32-47)。
func UpsertSkillEntry(cfg *types.OpenAcosmiConfig, skillKey string, apiKey string) *types.OpenAcosmiConfig {
	next := shallowCopyConfig(cfg)
	if next.Skills == nil {
		next.Skills = &types.SkillsConfig{}
	}
	if next.Skills.Entries == nil {
		next.Skills.Entries = make(map[string]*types.SkillConfig)
	}
	existing := next.Skills.Entries[skillKey]
	if existing == nil {
		existing = &types.SkillConfig{}
	}
	if apiKey != "" {
		existing.APIKey = apiKey
	}
	next.Skills.Entries[skillKey] = existing
	return next
}

// ---------- 核心流程 ----------

// SkillStatusSummary 技能状态汇总 (简化的 workspace skill status)。
type SkillStatusSummary struct {
	Name        string
	Description string
	Emoji       string
	Eligible    bool
	Disabled    bool
	BlockedByAL bool
	PrimaryEnv  string
	SkillKey    string
	MissingBins []string
	MissingEnv  []string
	InstallOpts []SkillInstallOpt
}

// SkillInstallOpt 技能安装选项。
type SkillInstallOpt struct {
	ID    string
	Kind  string
	Label string
}

// BuildSkillStatusSummaries 从 workspace 构建技能状态列表。
// 概念对应 TS: buildWorkspaceSkillStatus。
func BuildSkillStatusSummaries(workspaceDir string, cfg *types.OpenAcosmiConfig) []SkillStatusSummary {
	entries := skills.LoadSkillEntries(workspaceDir, "", skills.ResolveBundledSkillsDir(""), cfg)
	var results []SkillStatusSummary

	for _, entry := range entries {
		eligible := skills.ShouldIncludeSkill(entry, cfg, nil)
		disabled := false
		if sc := skills.ResolveSkillConfig(cfg, skills.ResolveSkillKey(entry.Skill.Name, entry.Metadata)); sc != nil {
			if sc.Enabled != nil && !*sc.Enabled {
				disabled = true
			}
		}

		var blockedByAL bool
		if cfg != nil && cfg.Skills != nil && len(cfg.Skills.AllowBundled) > 0 {
			blockedByAL = !skills.IsBundledSkillAllowed(entry, cfg.Skills.AllowBundled)
		}

		var missingBins, missingEnv []string
		var primaryEnv string
		if entry.Metadata != nil {
			primaryEnv = entry.Metadata.PrimaryEnv
			if entry.Metadata.Requires != nil {
				for _, bin := range entry.Metadata.Requires.Bins {
					if !skills.HasBinary(bin) {
						missingBins = append(missingBins, bin)
					}
				}
				for _, envName := range entry.Metadata.Requires.Env {
					if os.Getenv(envName) == "" {
						missingEnv = append(missingEnv, envName)
					}
				}
			}
		}

		var installOpts []SkillInstallOpt
		if entry.Metadata != nil {
			for i, spec := range entry.Metadata.Install {
				id := spec.ID
				if id == "" {
					id = fmt.Sprintf("%s-%d", spec.Kind, i)
				}
				installOpts = append(installOpts, SkillInstallOpt{
					ID:    id,
					Kind:  spec.Kind,
					Label: spec.Label,
				})
			}
		}

		emoji := "🧩"
		if entry.Metadata != nil && entry.Metadata.Emoji != "" {
			emoji = entry.Metadata.Emoji
		}

		results = append(results, SkillStatusSummary{
			Name:        entry.Skill.Name,
			Description: entry.Skill.Description,
			Emoji:       emoji,
			Eligible:    eligible,
			Disabled:    disabled,
			BlockedByAL: blockedByAL,
			PrimaryEnv:  primaryEnv,
			SkillKey:    skills.ResolveSkillKey(entry.Skill.Name, entry.Metadata),
			MissingBins: missingBins,
			MissingEnv:  missingEnv,
			InstallOpts: installOpts,
		})
	}
	return results
}

// SetupSkills 引导用户配置技能。
// 对应 TS: setupSkills (onboard-skills.ts L49-205)。
func SetupSkills(
	cfg *types.OpenAcosmiConfig,
	workspaceDir string,
	prompter tui.WizardPrompter,
) (*types.OpenAcosmiConfig, error) {
	summaries := BuildSkillStatusSummaries(workspaceDir, cfg)

	var eligible, missing, blocked []SkillStatusSummary
	for _, s := range summaries {
		switch {
		case s.Eligible:
			eligible = append(eligible, s)
		case s.BlockedByAL:
			blocked = append(blocked, s)
		case !s.Disabled:
			missing = append(missing, s)
		}
	}

	// Homebrew 检测（仅非 Windows）
	needsBrewPrompt := false
	if runtime.GOOS != "windows" {
		for _, s := range summaries {
			for _, opt := range s.InstallOpts {
				if opt.Kind == "brew" {
					needsBrewPrompt = true
					break
				}
			}
			if needsBrewPrompt {
				break
			}
		}
		if needsBrewPrompt && DetectBinary("brew") {
			needsBrewPrompt = false
		}
	}

	// Windows 包管理器检测（winget / Scoop）
	needsWinPkgPrompt := false
	if runtime.GOOS == "windows" {
		for _, s := range summaries {
			if len(s.InstallOpts) > 0 && len(s.MissingBins) > 0 {
				needsWinPkgPrompt = true
				break
			}
		}
		if needsWinPkgPrompt {
			if DetectBinary("winget") || DetectBinary("scoop") {
				needsWinPkgPrompt = false
			}
		}
	}

	// 展示技能状态概览
	prompter.Note(strings.Join([]string{
		fmt.Sprintf("Eligible: %d", len(eligible)),
		fmt.Sprintf("Missing requirements: %d", len(missing)),
		fmt.Sprintf("Blocked by allowlist: %d", len(blocked)),
	}, "\n"), i18n.Tp("onboard.skill.title"))

	// 确认是否配置
	shouldConfigure, err := prompter.Confirm(i18n.Tp("onboard.skill.configure"), true)
	if err != nil {
		return cfg, fmt.Errorf("confirm skills: %w", err)
	}
	if !shouldConfigure {
		return cfg, nil
	}

	// Homebrew 提醒
	if needsBrewPrompt {
		prompter.Note(i18n.Tp("onboard.skill.node_missing"), i18n.Tp("onboard.skill.title"))

		showBrewInstall, err := prompter.Confirm(i18n.Tp("onboard.skill.brew_confirm"), true)
		if err != nil {
			return cfg, fmt.Errorf("confirm brew: %w", err)
		}
		if showBrewInstall {
			prompter.Note(i18n.Tp("onboard.skill.brew_hint"), i18n.Tp("onboard.skill.title"))
		}
	}

	// Windows 包管理器提醒
	if needsWinPkgPrompt {
		prompter.Note(i18n.Tp("onboard.skill.win_pkg_missing"), i18n.Tp("onboard.skill.title"))

		showWinPkgInstall, err := prompter.Confirm(i18n.Tp("onboard.skill.win_pkg_confirm"), true)
		if err != nil {
			return cfg, fmt.Errorf("confirm win pkg: %w", err)
		}
		if showWinPkgInstall {
			prompter.Note(i18n.Tp("onboard.skill.win_pkg_hint"), i18n.Tp("onboard.skill.title"))
		}
	}

	// 节点管理器选择
	nodeManagerOpts := []tui.PromptOption{
		{Value: "npm", Label: "npm"},
		{Value: "pnpm", Label: "pnpm"},
		{Value: "bun", Label: "bun"},
	}
	nodeManager, err := prompter.Select(i18n.Tp("onboard.skill.node_manager"), nodeManagerOpts, "npm")
	if err != nil {
		return cfg, fmt.Errorf("node manager: %w", err)
	}

	next := shallowCopyConfig(cfg)
	if next.Skills == nil {
		next.Skills = &types.SkillsConfig{}
	}
	if next.Skills.Install == nil {
		next.Skills.Install = &types.SkillsInstallConfig{}
	}
	next.Skills.Install.NodeManager = nodeManager

	// 可安装技能选择
	var installable []SkillStatusSummary
	for _, s := range missing {
		if len(s.InstallOpts) > 0 && len(s.MissingBins) > 0 {
			installable = append(installable, s)
		}
	}

	if len(installable) > 0 {
		installOptions := []tui.PromptOption{
			{Value: "__skip__", Label: "Skip for now", Hint: "Continue without installing dependencies"},
		}
		for _, s := range installable {
			label := fmt.Sprintf("%s %s", s.Emoji, s.Name)
			hint := FormatSkillHint(s.Description, "")
			if len(s.InstallOpts) > 0 {
				hint = FormatSkillHint(s.Description, s.InstallOpts[0].Label)
			}
			installOptions = append(installOptions, tui.PromptOption{
				Value: s.Name,
				Label: label,
				Hint:  hint,
			})
		}

		toInstall, err := prompter.MultiSelect("Install missing skill dependencies", installOptions, nil)
		if err != nil {
			return next, fmt.Errorf("install selection: %w", err)
		}

		for _, name := range toInstall {
			if name == "__skip__" {
				continue
			}
			var target *SkillStatusSummary
			for i := range installable {
				if installable[i].Name == name {
					target = &installable[i]
					break
				}
			}
			if target == nil || len(target.InstallOpts) == 0 {
				continue
			}
			installID := target.InstallOpts[0].ID

			result := skills.InstallSkillFromSpec(skills.SkillInstallRequest{
				WorkspaceDir: workspaceDir,
				SkillName:    target.Name,
				InstallID:    installID,
				Config:       next,
			})

			if result.OK {
				msg := fmt.Sprintf("Installed %s", name)
				if len(result.Warnings) > 0 {
					msg = fmt.Sprintf("Installed %s (with warnings)", name)
				}
				slog.Info(msg)
				for _, w := range result.Warnings {
					slog.Warn(w)
				}
			} else {
				detail := SummarizeInstallFailure(result.Message)
				codeStr := ""
				if result.Code != nil {
					codeStr = fmt.Sprintf(" (exit %d)", *result.Code)
				}
				msg := fmt.Sprintf("Install failed: %s%s", name, codeStr)
				if detail != "" {
					msg += " — " + detail
				}
				slog.Error(msg)
				if result.Stderr != "" {
					slog.Info(strings.TrimSpace(result.Stderr))
				} else if result.Stdout != "" {
					slog.Info(strings.TrimSpace(result.Stdout))
				}
				slog.Info("Tip: run `crabclaw doctor` to review skills + requirements.")
				slog.Info("Docs: docs/skills/")
			}
		}
	}

	// API key 配置
	for _, s := range missing {
		if s.PrimaryEnv == "" || len(s.MissingEnv) == 0 {
			continue
		}
		wantsKey, err := prompter.Confirm(
			i18n.Tp("onboard.skill.api_key_q"),
			false,
		)
		if err != nil {
			continue
		}
		if !wantsKey {
			continue
		}

		apiKey, err := prompter.TextInput(
			i18n.Tf("onboard.skill.api_key_input", s.PrimaryEnv),
			"",
			"",
			func(v string) string {
				if strings.TrimSpace(v) == "" {
					return "Required"
				}
				return ""
			},
		)
		if err != nil {
			continue
		}
		next = UpsertSkillEntry(next, s.SkillKey, strings.TrimSpace(apiKey))
	}

	return next, nil
}
