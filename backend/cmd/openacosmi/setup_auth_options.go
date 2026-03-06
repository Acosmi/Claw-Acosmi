package main

// setup_auth_options.go — 认证提供商分组定义 + 交互式选择
// 对应 TS src/commands/auth-choice-options.ts (272L) + auth-choice-prompt.ts (57L)

import (
	"fmt"

	"github.com/Acosmi/ClawAcosmi/internal/tui"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
)

// ---------- 分组定义 ----------

// authChoiceGroupDef 分组定义（内部）。
type authChoiceGroupDef struct {
	Value   AuthChoiceGroupID
	Label   string
	Hint    string
	Choices []AuthChoice
}

// AUTH_CHOICE_GROUP_DEFS 所有认证分组（对应 TS AUTH_CHOICE_GROUP_DEFS）。
var authChoiceGroupDefs = []authChoiceGroupDef{
	{GroupOpenAI, "OpenAI", "API key", []AuthChoice{AuthChoiceOpenAIApiKey}},
	{GroupAnthropic, "Anthropic", "setup-token + API key", []AuthChoice{AuthChoiceToken, AuthChoiceApiKey}},
	{GroupMinimax, "MiniMax", "M2.1 (recommended)", []AuthChoice{AuthChoiceMinimaxPortal, AuthChoiceMinimaxApi, AuthChoiceMinimaxApiLightning}},
	{GroupMoonshot, "Moonshot AI (Kimi)", "Kimi K2.5 API key", []AuthChoice{AuthChoiceMoonshotApiKey, AuthChoiceMoonshotApiKeyCn}},
	{GroupGoogle, "Google", "Gemini API key + OAuth", []AuthChoice{AuthChoiceGeminiApiKey, AuthChoiceGoogleAntigravity, AuthChoiceGoogleGeminiCli}},
	{GroupXAI, "xAI (Grok)", "API key", []AuthChoice{AuthChoiceXAIApiKey}},
	{GroupQwen, "Qwen", "OAuth", []AuthChoice{AuthChoiceQwenPortal}},
	{GroupZAI, "Z.AI (GLM)", "API key", []AuthChoice{AuthChoiceZaiApiKey}},
	{GroupCopilot, "GitHub Copilot", "GitHub device login", []AuthChoice{AuthChoiceGitHubCopilot}},
	{GroupAcosmiZen, "Claw Acosmi Zen", "API key", []AuthChoice{AuthChoiceAcosmiZen}},
}

// ---------- 选项构建 ----------

// allAuthChoiceOptionDefs 完整选项列表（对应 TS buildAuthChoiceOptions）。
var allAuthChoiceOptionDefs = []AuthChoiceOption{
	{AuthChoiceToken, "Anthropic token (paste setup-token)", "run `claude setup-token` elsewhere, then paste the token here"},
	{AuthChoiceApiKey, "Anthropic API key", ""},
	{AuthChoiceOpenAIApiKey, "OpenAI API key", ""},
	{AuthChoiceXAIApiKey, "xAI (Grok) API key", ""},
	{AuthChoiceMoonshotApiKey, "Kimi API key (.ai)", ""},
	{AuthChoiceMoonshotApiKeyCn, "Kimi API key (.cn)", ""},
	{AuthChoiceGitHubCopilot, "GitHub Copilot (GitHub device login)", "Uses GitHub device flow"},
	{AuthChoiceGeminiApiKey, "Google Gemini API key", ""},
	{AuthChoiceGoogleAntigravity, "Google Antigravity OAuth", "Uses the bundled Antigravity auth plugin"},
	{AuthChoiceGoogleGeminiCli, "Google Gemini CLI OAuth", "Uses the bundled Gemini CLI auth plugin"},
	{AuthChoiceZaiApiKey, "Z.AI (GLM) API key", ""},
	{AuthChoiceMinimaxPortal, "MiniMax OAuth", "OAuth plugin for MiniMax"},
	{AuthChoiceMinimaxApi, "MiniMax M2.1", ""},
	{AuthChoiceMinimaxApiLightning, "MiniMax M2.1 Lightning", "Faster, higher output cost"},
	{AuthChoiceQwenPortal, "Qwen OAuth", ""},
	{AuthChoiceAcosmiZen, "Claw Acosmi Zen (multi-model proxy)", "Claude, GPT, Gemini via openacosmi.com/zen"},
}

// BuildAuthChoiceOptions 构建完整选项列表。
func BuildAuthChoiceOptions(includeSkip bool) []AuthChoiceOption {
	options := make([]AuthChoiceOption, len(allAuthChoiceOptionDefs))
	copy(options, allAuthChoiceOptionDefs)
	if includeSkip {
		options = append(options, AuthChoiceOption{AuthChoiceSkip, "Skip for now", ""})
	}
	return options
}

// BuildAuthChoiceGroups 按组聚合选项。
func BuildAuthChoiceGroups(includeSkip bool) ([]AuthChoiceGroup, *AuthChoiceOption) {
	optionByValue := make(map[AuthChoice]AuthChoiceOption)
	for _, opt := range allAuthChoiceOptionDefs {
		optionByValue[opt.Value] = opt
	}

	var groups []AuthChoiceGroup
	for _, def := range authChoiceGroupDefs {
		group := AuthChoiceGroup{
			Value: def.Value,
			Label: def.Label,
			Hint:  def.Hint,
		}
		for _, choice := range def.Choices {
			if opt, ok := optionByValue[choice]; ok {
				group.Options = append(group.Options, opt)
			}
		}
		if len(group.Options) > 0 {
			groups = append(groups, group)
		}
	}

	var skipOpt *AuthChoiceOption
	if includeSkip {
		s := AuthChoiceOption{AuthChoiceSkip, "Skip for now", ""}
		skipOpt = &s
	}
	return groups, skipOpt
}

// ---------- 交互式选择 ----------

// PromptAuthChoiceGrouped 两级交互式选择（先选组→再选方法）。
// 对应 TS promptAuthChoiceGrouped (auth-choice-prompt.ts)
func PromptAuthChoiceGrouped(prompter tui.WizardPrompter, includeSkip bool) (AuthChoice, error) {
	groups, skipOpt := BuildAuthChoiceGroups(includeSkip)
	backValue := "__back"

	for {
		// 第一级：选择提供商组
		providerOptions := make([]tui.PromptOption, 0, len(groups)+1)
		for _, g := range groups {
			providerOptions = append(providerOptions, tui.PromptOption{
				Value: g.Value,
				Label: g.Label,
				Hint:  g.Hint,
			})
		}
		if skipOpt != nil {
			providerOptions = append(providerOptions, tui.PromptOption{
				Value: skipOpt.Value,
				Label: skipOpt.Label,
			})
		}

		providerSelection, err := prompter.Select(i18n.Tp("onboard.auth.provider_select"), providerOptions, "")
		if err != nil {
			return "", fmt.Errorf("provider selection: %w", err)
		}

		if providerSelection == AuthChoiceSkip {
			return AuthChoiceSkip, nil
		}

		// 查找选中的组
		var selectedGroup *AuthChoiceGroup
		for i := range groups {
			if groups[i].Value == providerSelection {
				selectedGroup = &groups[i]
				break
			}
		}
		if selectedGroup == nil || len(selectedGroup.Options) == 0 {
			prompter.Note(i18n.Tp("onboard.auth.no_methods"), i18n.Tp("onboard.auth.title"))
			continue
		}

		// 第二级：选择认证方式
		methodOptions := make([]tui.PromptOption, 0, len(selectedGroup.Options)+1)
		for _, opt := range selectedGroup.Options {
			methodOptions = append(methodOptions, tui.PromptOption{
				Value: opt.Value,
				Label: opt.Label,
				Hint:  opt.Hint,
			})
		}
		methodOptions = append(methodOptions, tui.PromptOption{Value: backValue, Label: "Back"})

		methodSelection, err := prompter.Select(i18n.Tp("onboard.auth.method_select"), methodOptions, "")
		if err != nil {
			return "", fmt.Errorf("method selection: %w", err)
		}

		if methodSelection == backValue {
			continue
		}

		return methodSelection, nil
	}
}
