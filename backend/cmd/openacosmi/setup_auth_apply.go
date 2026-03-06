package main

// setup_auth_apply.go — 认证选择应用逻辑
// 对应 TS src/commands/auth-choice.apply.ts + auth-choice.apply.*.ts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/auth"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/bridge"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ApplyAuthChoice 主路由 — 按 authChoice 分发到对应 handler。
// 对应 TS: applyAuthChoice (auth-choice.apply.ts)
func ApplyAuthChoice(params ApplyAuthChoiceParams) (*ApplyAuthChoiceResult, error) {
	applyDefault := &bridge.ApplyOpts{SetDefaultModel: true}

	switch params.AuthChoice {
	// Anthropic
	case AuthChoiceToken:
		result, err := applyAnthropicToken(params)
		if err == nil && result != nil {
			bridge.ApplyProviderByID("anthropic", result.Config, applyDefault)
		}
		return result, err
	case AuthChoiceApiKey:
		result, err := applyAnthropicApiKey(params)
		if err == nil && result != nil {
			bridge.ApplyProviderByID("anthropic", result.Config, applyDefault)
		}
		return result, err

	// OpenAI
	case AuthChoiceOpenAIApiKey:
		result, err := applyGenericApiKey(params, "openai", "OPENAI_API_KEY", "Enter OpenAI API key")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("openai", result.Config, applyDefault)
		}
		return result, err

	// Google
	case AuthChoiceGeminiApiKey:
		result, err := applyGenericApiKey(params, "google", "GEMINI_API_KEY", "Enter Gemini API key")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("google", result.Config, applyDefault)
		}
		return result, err
	case AuthChoiceGoogleAntigravity:
		result, err := applyOAuthPlaceholder(params, "google", "Google Antigravity OAuth")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("google", result.Config, applyDefault)
		}
		return result, err
	case AuthChoiceGoogleGeminiCli:
		result, err := applyOAuthPlaceholder(params, "google", "Google Gemini CLI OAuth")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("google", result.Config, applyDefault)
		}
		return result, err

	// xAI
	case AuthChoiceXAIApiKey:
		result, err := applyGenericApiKey(params, "xai", "XAI_API_KEY", "Enter xAI (Grok) API key")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("xai", result.Config, applyDefault)
		}
		return result, err

	// Moonshot / Kimi
	case AuthChoiceMoonshotApiKey, AuthChoiceMoonshotApiKeyCn:
		result, err := applyGenericApiKey(params, "moonshot", "MOONSHOT_API_KEY", "Enter Moonshot API key")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("moonshot", result.Config, applyDefault)
		}
		return result, err

	// Z.AI (GLM)
	case AuthChoiceZaiApiKey:
		result, err := applyGenericApiKey(params, "zai", "ZAI_API_KEY", "Enter Z.AI API key")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("zai", result.Config, applyDefault)
		}
		return result, err

	// MiniMax
	case AuthChoiceMinimaxApi, AuthChoiceMinimaxApiLightning:
		result, err := applyGenericApiKey(params, "minimax", "MINIMAX_API_KEY", "Enter MiniMax API key")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("minimax", result.Config, applyDefault)
		}
		return result, err
	case AuthChoiceMinimaxPortal:
		result, err := applyOAuthPlaceholder(params, "minimax-portal", "MiniMax OAuth")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("minimax", result.Config, applyDefault)
		}
		return result, err

	// Claw Acosmi Zen
	case AuthChoiceAcosmiZen:
		return applyGenericApiKey(params, "openacosmi", "OPENACOSMI_API_KEY", "Enter Claw Acosmi Zen API key")

	// OAuth-only providers
	case AuthChoiceQwenPortal:
		result, err := applyOAuthPlaceholder(params, "qwen-portal", "Qwen OAuth")
		if err == nil && result != nil {
			bridge.ApplyProviderByID("qwen", result.Config, applyDefault)
		}
		return result, err
	case AuthChoiceGitHubCopilot:
		return applyCopilotDeviceFlow(params)

	case AuthChoiceSkip:
		return &ApplyAuthChoiceResult{Config: params.Config}, nil

	default:
		slog.Warn("setup: unknown auth choice", "choice", params.AuthChoice)
		return &ApplyAuthChoiceResult{Config: params.Config}, nil
	}
}

// ---------- Anthropic handlers ----------

func applyAnthropicToken(params ApplyAuthChoiceParams) (*ApplyAuthChoiceResult, error) {
	params.Prompter.Note(
		"Run `claude setup-token` in your terminal.\nThen paste the generated token below.",
		"Anthropic setup-token",
	)

	token, err := params.Prompter.TextInput("Paste Anthropic setup-token", "", "", func(v string) string {
		if len(v) < 10 {
			return "Token too short"
		}
		return ""
	})
	if err != nil {
		return nil, fmt.Errorf("token input: %w", err)
	}

	profileID := "anthropic:default"
	if err := storeApiKeyCredential(params.AuthStore, profileID, "anthropic", token); err != nil {
		return nil, err
	}

	cfg := ensureAuthConfig(params.Config, profileID, "anthropic")
	return &ApplyAuthChoiceResult{Config: cfg}, nil
}

func applyAnthropicApiKey(params ApplyAuthChoiceParams) (*ApplyAuthChoiceResult, error) {
	return applyGenericApiKey(params, "anthropic", "ANTHROPIC_API_KEY", "Enter Anthropic API key")
}

// ---------- 通用 API Key handler ----------

func applyGenericApiKey(params ApplyAuthChoiceParams, provider, envVar, prompt string) (*ApplyAuthChoiceResult, error) {
	var apiKey string

	// 1. 检测环境变量
	envResult := ResolveEnvApiKey(provider)
	if envResult != nil {
		useExisting, err := params.Prompter.Confirm(
			fmt.Sprintf("Use existing %s (%s, %s)?", envVar, envResult.Source, FormatApiKeyPreview(envResult.ApiKey)),
			true,
		)
		if err != nil {
			return nil, fmt.Errorf("confirm: %w", err)
		}
		if useExisting {
			apiKey = envResult.ApiKey
		}
	}

	// 2. 交互式输入
	if apiKey == "" {
		key, err := params.Prompter.TextInput(prompt, "", "", ValidateApiKeyInput)
		if err != nil {
			return nil, fmt.Errorf("api key input: %w", err)
		}
		apiKey = NormalizeApiKeyInput(key)
	}

	// 3. 存储凭据
	profileID := auth.FormatProfileId(provider, "default")
	if err := storeApiKeyCredential(params.AuthStore, profileID, provider, apiKey); err != nil {
		return nil, err
	}

	// 4. 更新配置
	cfg := ensureAuthConfig(params.Config, profileID, provider)
	return &ApplyAuthChoiceResult{Config: cfg}, nil
}

// ---------- OAuth Web Flow ----------

func applyOAuthPlaceholder(params ApplyAuthChoiceParams, provider, label string) (*ApplyAuthChoiceResult, error) {
	// 查找 provider 配置（使用 auth 包的统一注册表）
	providerConfig := auth.GetOAuthProviderConfig(provider)
	if providerConfig == nil {
		// 对于未注册 OAuth 端点的 provider，回退到提示信息
		params.Prompter.Note(
			fmt.Sprintf("%s requires browser-based OAuth.\nPlease use `openacosmi onboard --provider %s` when the gateway is running.", label, provider),
			label,
		)
		profileID := auth.FormatProfileId(provider, "default")
		cfg := ensureAuthConfig(params.Config, profileID, provider)
		return &ApplyAuthChoiceResult{Config: cfg}, nil
	}

	if providerConfig.ClientID == "" {
		// Client ID 未配置 — 引导用户设置
		envKey := strings.ToUpper(strings.ReplaceAll(provider, "-", "_")) + "_CLIENT_ID"
		clientID, err := params.Prompter.TextInput(
			fmt.Sprintf("Enter %s OAuth Client ID (or set %s)", label, envKey), "", "", func(v string) string {
				if strings.TrimSpace(v) == "" {
					return "Client ID is required"
				}
				return ""
			},
		)
		if err != nil {
			return nil, fmt.Errorf("client id input: %w", err)
		}
		providerConfig.ClientID = strings.TrimSpace(clientID)
	}

	// 运行 OAuth Web Flow（使用 golang.org/x/oauth2）
	params.Prompter.Note(
		fmt.Sprintf("Opening browser for %s authorization...\nA local callback server will listen for the response.", label),
		label,
	)

	ctx := context.Background()
	token, err := auth.RunOAuthWebFlow(ctx, providerConfig, params.AuthStore)
	if err != nil {
		// OAuth 失败 — 明确报错，不写入空 profile
		slog.Warn("OAuth web flow failed", "provider", provider, "error", err)
		params.Prompter.Note(
			fmt.Sprintf("OAuth authorization failed: %v\nYou can retry with `openacosmi onboard --provider %s`", err, provider),
			label,
		)
		return nil, fmt.Errorf("OAuth authorization for %s failed: %w", label, err)
	}

	params.Prompter.Note(
		fmt.Sprintf("✅ %s OAuth authorized successfully!", label),
		label,
	)

	profileID := auth.FormatProfileId(provider, "default")
	cfg := ensureAuthConfig(params.Config, profileID, provider)
	// 设置 mode 为 OAuth
	if cfg.Auth != nil && cfg.Auth.Profiles != nil {
		if profile, ok := cfg.Auth.Profiles[profileID]; ok {
			profile.Mode = types.AuthModeOAuth
		}
	}
	_ = token // 已通过 RunOAuthWebFlow 存入 auth store

	return &ApplyAuthChoiceResult{Config: cfg}, nil
}

// ---------- 凭据存储辅助 ----------

func storeApiKeyCredential(store *auth.AuthStore, profileID, provider, apiKey string) error {
	if store == nil {
		return nil
	}
	_, err := store.Update(func(s *auth.AuthProfileStore) bool {
		s.Profiles[profileID] = &auth.AuthProfileCredential{
			Type:     auth.CredentialAPIKey,
			Provider: provider,
			Key:      apiKey,
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("store credential for %s: %w", provider, err)
	}
	return nil
}

func ensureAuthConfig(cfg *types.OpenAcosmiConfig, profileID, provider string) *types.OpenAcosmiConfig {
	if cfg == nil {
		cfg = &types.OpenAcosmiConfig{}
	}
	if cfg.Auth == nil {
		cfg.Auth = &types.AuthConfig{}
	}
	if cfg.Auth.Profiles == nil {
		cfg.Auth.Profiles = make(map[string]*types.AuthProfileConfig)
	}
	// 设置 profile 条目
	cfg.Auth.Profiles[profileID] = &types.AuthProfileConfig{
		Provider: provider,
		Mode:     types.AuthModeAPIKey,
	}
	// 设置 order（确保新 profile 在首位）
	if cfg.Auth.Order == nil {
		cfg.Auth.Order = make(map[string][]string)
	}
	if _, ok := cfg.Auth.Order[provider]; !ok {
		cfg.Auth.Order[provider] = []string{profileID}
	}
	return cfg
}

// ---------- GitHub Copilot Device Flow ----------

func applyCopilotDeviceFlow(params ApplyAuthChoiceParams) (*ApplyAuthChoiceResult, error) {
	ctx := context.Background()

	profileID := auth.FormatProfileId("github-copilot", "github")

	// 检查已有凭据
	if params.AuthStore != nil {
		existing := params.AuthStore.GetProfile(profileID)
		if existing != nil {
			params.Prompter.Note(
				fmt.Sprintf("Auth profile already exists: %s\nRe-running will overwrite it.", profileID),
				"Existing credentials",
			)
		}
	}

	// 1. 请求设备码
	params.Prompter.Note("Requesting device code from GitHub...", "GitHub Copilot")
	device, err := auth.RequestCopilotDeviceCode(ctx, nil, auth.CopilotDefaultScope)
	if err != nil {
		return nil, fmt.Errorf("GitHub 设备码请求失败: %w", err)
	}

	// 2. 引导用户授权
	params.Prompter.Note(
		fmt.Sprintf("Visit: %s\nCode: %s", device.VerificationURI, device.UserCode),
		"Authorize GitHub Copilot",
	)

	// 3. 轮询获取 access token
	expiresAt := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	intervalMs := device.Interval * 1000
	if intervalMs < 1000 {
		intervalMs = 1000
	}

	params.Prompter.Note("Waiting for GitHub authorization...", "Polling")
	accessToken, err := auth.PollForCopilotAccessToken(ctx, nil, device.DeviceCode, intervalMs, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("GitHub 授权失败: %w", err)
	}

	// 4. 存入 AuthStore
	if err := auth.StoreCopilotAuthProfile(params.AuthStore, profileID, accessToken); err != nil {
		return nil, fmt.Errorf("存储凭据失败: %w", err)
	}

	cfg := ensureAuthConfig(params.Config, profileID, "github-copilot")
	return &ApplyAuthChoiceResult{Config: cfg}, nil
}
