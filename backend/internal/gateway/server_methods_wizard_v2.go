package gateway

// server_methods_wizard_v2.go — Wizard V2 配置应用 RPC 处理器
//
// 方法: wizard.v2.apply
// 接收前端 Wizard V2 生成的 WizardV2Payload，转换为 OpenAcosmiConfig，
// 通过 Keyring 安全存储敏感密钥，写入配置文件，并触发引擎热重载。

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/auth"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/bridge"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/common"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/oauth"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/oauth/gemini"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/oauth/minimax"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/oauth/qwen"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/onboard"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/providers/copilot"
	"github.com/Acosmi/ClawAcosmi/pkg/log"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

var wizardV2Logger = log.New("wizard-v2")

// OpenURL 打开指定 URL（跨平台浏览器调用）。
// 从 wizard_helpers.go 迁移至此，供 V2 OAuth 流程使用。
func OpenURL(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return false
	}
	return cmd.Start() == nil
}

// DefaultDangerousNodeDenyCommands 默认危险命令拒绝列表。
// 从 wizard_gateway_config.go 迁移至此，供 V2 安全级别配置使用。
var DefaultDangerousNodeDenyCommands = []string{
	"rm -rf /",
	"mkfs",
	"dd if=/dev/zero",
	":(){ :|:& };:",
	"shutdown",
	"reboot",
	"halt",
	"poweroff",
	"init 0",
	"init 6",
}

// ---------- Payload 类型定义 ----------

// WizardV2ProviderSelection 单个 Provider 的选择信息。
type WizardV2ProviderSelection struct {
	Model    string `json:"model"`
	AuthMode string `json:"authMode"` // "apiKey" | "oauth" | "none"
}

// WizardV2ChannelFeishu 飞书频道配置。
type WizardV2ChannelFeishu struct {
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
}

// WizardV2ChannelWeCom 企微频道配置。
type WizardV2ChannelWeCom struct {
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
}

// WizardV2ChannelDingTalk 钉钉频道配置。
type WizardV2ChannelDingTalk struct {
	AppKey    string `json:"appKey"`
	AppSecret string `json:"appSecret"`
}

// WizardV2ChannelTelegram Telegram 频道配置。
type WizardV2ChannelTelegram struct {
	BotToken string `json:"botToken"`
}

// WizardV2ChannelConfig 所有频道配置。
type WizardV2ChannelConfig struct {
	Feishu   WizardV2ChannelFeishu   `json:"feishu"`
	WeCom    WizardV2ChannelWeCom    `json:"wecom"`
	DingTalk WizardV2ChannelDingTalk `json:"dingtalk"`
	Telegram WizardV2ChannelTelegram `json:"telegram"`
}

// WizardV2MemoryConfig 记忆系统配置。
type WizardV2MemoryConfig struct {
	EnableVector bool   `json:"enableVector"`
	HostingType  string `json:"hostingType"` // "local" | "cloud"
	APIEndpoint  string `json:"apiEndpoint"`
	LLMProvider  string `json:"llmProvider,omitempty"` // 记忆提取 LLM: "anthropic"|"openai"|"deepseek"|"ollama"|...
	LLMModel     string `json:"llmModel,omitempty"`    // 空=按 provider 默认
	LLMApiKey    string `json:"llmApiKey,omitempty"`   // 独立 API key
	LLMBaseURL   string `json:"llmBaseUrl,omitempty"`  // 空=使用 provider 默认 URL
}

// WizardV2SelectedSkills 技能选择（key 匹配 scope.ToolGroups: fs, runtime, ui, web, memory 等）。
type WizardV2SelectedSkills = map[string]bool

// WizardV2SubAgentEntry 子智能体配置项。
type WizardV2SubAgentEntry struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey"`
}

// WizardV2Payload 前端 Wizard V2 提交的完整配置负载。
type WizardV2Payload struct {
	PrimaryConfig       map[string]string                    `json:"primaryConfig"`
	FallbackConfig      map[string]string                    `json:"fallbackConfig"`
	ProviderSelections  map[string]WizardV2ProviderSelection `json:"providerSelections"`
	CustomBaseUrls      map[string]string                    `json:"customBaseUrls"`
	SecurityAck         bool                                 `json:"securityAck"`
	SelectedSkills      WizardV2SelectedSkills               `json:"selectedSkills"`
	ChannelConfig       WizardV2ChannelConfig                `json:"channelConfig"`
	SubAgentsConfig     map[string]WizardV2SubAgentEntry     `json:"subAgentsConfig"`
	MemoryConfig        WizardV2MemoryConfig                 `json:"memoryConfig"`
	SecurityLevelConfig string                               `json:"securityLevelConfig"` // "deny" | "sandboxed" | "allowlist" | "full"
}

// ---------- Device Code Session 管理 ----------

// deviceCodeSession 存储活跃的设备码轮询会话。
type deviceCodeSession struct {
	Provider  string
	ResultCh  chan deviceCodeResult
	Cancel    context.CancelFunc
	ExpiresAt time.Time
}

type deviceCodeResult struct {
	AccessToken string
	Err         error
}

var (
	deviceCodeSessions   = make(map[string]*deviceCodeSession)
	deviceCodeSessionsMu sync.Mutex
)

// noopProgress 用于满足 oauth.ProgressReporter 接口。
type noopProgress struct{}

func (noopProgress) Update(_ string) {}
func (noopProgress) Stop(_ string)   {}

// ---------- Handlers ----------

// WizardV2ProvidersListHandler 返回 wizard.v2.providers.list 方法处理器。
// 从后端 bridge 层构建完整 provider 目录，供前端动态渲染。
func WizardV2ProvidersListHandler() GatewayMethodHandler {
	return handleWizardV2ProvidersList
}

func handleWizardV2ProvidersList(ctx *MethodHandlerContext) {
	catalog := bridge.BuildWizardProviderCatalog()
	ctx.Respond(true, map[string]interface{}{
		"providers": catalog,
	}, nil)
}

// WizardV2ApplyHandler 返回 wizard.v2.apply 方法处理器。
func WizardV2ApplyHandler() GatewayMethodHandler {
	return handleWizardV2Apply
}

// WizardV2OAuthHandler 返回 wizard.v2.oauth 方法处理器。
// 调用 auth.RunOAuthWebFlow 执行真实 OAuth 授权流程（打开浏览器）。
// 支持 Google（PKCE）。设备码流程使用 wizard.v2.oauth.device.start/poll。
func WizardV2OAuthHandler() GatewayMethodHandler {
	return handleWizardV2OAuth
}

// WizardV2OAuthDeviceStartHandler 返回 wizard.v2.oauth.device.start 处理器。
// 启动设备码授权流程，返回 userCode + verificationURI。
func WizardV2OAuthDeviceStartHandler() GatewayMethodHandler {
	return handleWizardV2OAuthDeviceStart
}

// WizardV2OAuthDevicePollHandler 返回 wizard.v2.oauth.device.poll 处理器。
// 轮询设备码授权结果。
func WizardV2OAuthDevicePollHandler() GatewayMethodHandler {
	return handleWizardV2OAuthDevicePoll
}

func handleWizardV2OAuth(ctx *MethodHandlerContext) {
	providerID, _ := ctx.Params["provider"].(string)
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "missing 'provider' parameter"))
		return
	}

	// 设备码流程 provider 不通过此入口
	switch providerID {
	case "minimax", "qwen", "github-copilot":
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams,
			"provider '"+providerID+"' 使用 Device Flow，请使用 wizard.v2.oauth.device.start/poll"))
		return
	}

	// Google Gemini CLI (Antigravity) OAuth — 独立流程，含项目发现
	if providerID == "google-gemini-cli" || providerID == "gemini-cli" {
		handleWizardV2OAuthGeminiCli(ctx, "google-gemini-cli")
		return
	}

	// 前端 provider ID 到 OAuth 注册表 key 的映射
	oauthProviderMapping := map[string]string{
		"google": "google",
	}

	oauthKey := providerID
	if mapped, ok := oauthProviderMapping[providerID]; ok {
		oauthKey = mapped
	}

	// 从 OAuth 注册表获取 provider 配置
	oauthCfg := auth.GetOAuthProviderConfig(oauthKey)
	if oauthCfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams,
			"provider '"+providerID+"' does not support OAuth, please use API Key mode"))
		return
	}

	wizardV2Logger.Info("Starting OAuth flow", "provider", providerID)

	// 创建 AuthStore 用于持久化 OAuth 凭据（refresh token + expires）
	home, _ := os.UserHomeDir()
	authStorePath := filepath.Join(home, ".openacosmi", "auth.json")
	authStore := auth.NewAuthStore(authStorePath)
	if _, loadErr := authStore.Load(); loadErr != nil {
		wizardV2Logger.Warn("Failed to load auth store (will create new)", "error", loadErr)
	}

	// 执行真实 OAuth Web Flow（打开浏览器 → 用户授权 → 回调 → 换 token）
	oauthCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	token, err := auth.RunOAuthWebFlow(oauthCtx, oauthCfg, authStore)
	if err != nil {
		wizardV2Logger.Warn("OAuth flow failed", "provider", providerID, "error", err)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError,
			"OAuth authorization failed: "+err.Error()))
		return
	}

	wizardV2Logger.Info("OAuth flow succeeded", "provider", providerID)

	// 同时持久化到 auth-profiles.json（确保 gateway 重启后 token 可用）
	var refreshToken string
	var expiresMs int64
	if token.RefreshToken != "" {
		refreshToken = token.RefreshToken
	}
	if !token.Expiry.IsZero() {
		expiresMs = token.Expiry.UnixMilli()
	}
	persistOAuthToken(providerID, token.AccessToken, refreshToken, expiresMs)

	ctx.Respond(true, map[string]interface{}{
		"ok":          true,
		"provider":    providerID,
		"accessToken": token.AccessToken,
	}, nil)
}

// handleWizardV2OAuthGeminiCli 处理 Google Gemini CLI (Antigravity) OAuth 流程。
// 调用 gemini.LoginGeminiCliOAuth() 执行完整流程：
//   - PKCE OAuth 授权
//   - 用户邮箱获取
//   - Google Cloud 项目发现/创建（Code Assist API, ideType: ANTIGRAVITY）
func handleWizardV2OAuthGeminiCli(ctx *MethodHandlerContext, providerID string) {
	wizardV2Logger.Info("Starting Gemini CLI (Antigravity) OAuth flow", "provider", providerID)

	// 构建 GeminiCliOAuthContext
	oauthCtx := &gemini.GeminiCliOAuthContext{
		IsRemote: false, // Wizard V2 通过 Web UI 运行，始终是本地
		OpenURL: func(url string) error {
			if !OpenURL(url) {
				return fmt.Errorf("failed to open browser")
			}
			return nil
		},
		Log: func(msg string) {
			wizardV2Logger.Info(msg)
		},
		Note: func(message string, title string) error {
			wizardV2Logger.Info(title+": "+message, "provider", providerID)
			return nil
		},
		Prompt: func(message string) (string, error) {
			// Wizard V2 是 Web UI，不支持终端交互式输入
			// 本地模式下不会触发此回调（浏览器自动回调）
			return "", fmt.Errorf("interactive prompt not supported in Wizard V2 Web UI")
		},
		Progress: &wizardV2ProgressReporter{logger: wizardV2Logger},
	}

	creds, err := gemini.LoginGeminiCliOAuth(oauthCtx)
	if err != nil {
		wizardV2Logger.Warn("Gemini CLI OAuth flow failed", "provider", providerID, "error", err)
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError,
			"Gemini CLI OAuth authorization failed: "+err.Error()))
		return
	}

	wizardV2Logger.Info("Gemini CLI OAuth flow succeeded",
		"provider", providerID,
		"email", creds.Email,
		"projectID", creds.ProjectID,
	)

	// 持久化 OAuth 凭据（含 projectId）
	persistGeminiCliOAuthToken(providerID, creds)

	ctx.Respond(true, map[string]interface{}{
		"ok":          true,
		"provider":    providerID,
		"accessToken": creds.Access,
		"email":       creds.Email,
		"projectId":   creds.ProjectID,
	}, nil)
}

// wizardV2ProgressReporter 实现 oauth.ProgressReporter 接口。
type wizardV2ProgressReporter struct {
	logger *log.Logger
}

func (p *wizardV2ProgressReporter) Update(msg string) {
	p.logger.Info("OAuth progress", "message", msg)
}

func (p *wizardV2ProgressReporter) Stop(msg string) {
	p.logger.Info("OAuth progress done", "message", msg)
}

// handleWizardV2OAuthDeviceStart 启动设备码授权流程。
func handleWizardV2OAuthDeviceStart(ctx *MethodHandlerContext) {
	providerID, _ := ctx.Params["provider"].(string)
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "missing 'provider' parameter"))
		return
	}

	switch providerID {
	case "github-copilot":
		handleDeviceStartGitHubCopilot(ctx, providerID)
	case "minimax":
		handleDeviceStartMiniMax(ctx, providerID)
	case "qwen":
		handleDeviceStartQwen(ctx, providerID)
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams,
			"provider '"+providerID+"' does not support device code flow"))
	}
}

// handleDeviceStartGitHubCopilot 启动 GitHub Copilot 设备码流程。
func handleDeviceStartGitHubCopilot(ctx *MethodHandlerContext, providerID string) {
	deviceResp, err := copilot.RequestDeviceCode("read:user", http.DefaultClient)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError,
			"failed to request device code: "+err.Error()))
		return
	}

	// 创建后台轮询会话
	sessionID := fmt.Sprintf("device-%s-%d", providerID, time.Now().UnixNano())
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Duration(deviceResp.ExpiresIn)*time.Second)

	session := &deviceCodeSession{
		Provider:  providerID,
		ResultCh:  make(chan deviceCodeResult, 1),
		Cancel:    cancel,
		ExpiresAt: time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second),
	}

	deviceCodeSessionsMu.Lock()
	deviceCodeSessions[sessionID] = session
	deviceCodeSessionsMu.Unlock()

	// 后台轮询
	go func() {
		defer cancel()
		intervalMs := int64(deviceResp.Interval) * 1000
		if intervalMs < 5000 {
			intervalMs = 5000
		}
		expiresAt := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second).UnixMilli()

		token, pollErr := copilot.PollForAccessToken(deviceResp.DeviceCode, intervalMs, expiresAt, http.DefaultClient)
		session.ResultCh <- deviceCodeResult{AccessToken: token, Err: pollErr}

		// 如果成功，持久化到 auth-profiles
		if pollErr == nil && token != "" {
			persistOAuthToken(providerID, token, "", 0)
		}

		// 清理过期会话
		time.AfterFunc(5*time.Minute, func() {
			deviceCodeSessionsMu.Lock()
			delete(deviceCodeSessions, sessionID)
			deviceCodeSessionsMu.Unlock()
		})
		_ = pollCtx
	}()

	wizardV2Logger.Info("Device code flow started", "provider", providerID, "sessionID", sessionID)

	ctx.Respond(true, map[string]interface{}{
		"ok":              true,
		"provider":        providerID,
		"userCode":        deviceResp.UserCode,
		"verificationUri": deviceResp.VerificationURI, // 前端使用 camelCase verificationUri
		"expiresIn":       deviceResp.ExpiresIn,
		"sessionId":       sessionID, // 前端使用 camelCase sessionId
	}, nil)
}

// handleDeviceStartMiniMax 启动 MiniMax 设备码流程。
// 仿照 GitHub Copilot 模式：先同步请求设备码返回 userCode，后台 goroutine 轮询 token。
func handleDeviceStartMiniMax(ctx *MethodHandlerContext, providerID string) {
	// 提取 region 参数（在 goroutine 启动前读取 ctx.Params）
	region := minimax.RegionGlobal
	if r, ok := ctx.Params["region"].(string); ok && r == "cn" {
		region = minimax.RegionCN
	}

	// 1. 同步生成 PKCE + state
	pkce, err := common.GeneratePKCEVerifierChallenge()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "PKCE generation failed: "+err.Error()))
		return
	}
	state, err := oauth.GenerateRandomState()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "state generation failed: "+err.Error()))
		return
	}

	// 2. 同步请求设备码（立即返回 userCode）
	device, err := minimax.RequestOAuthCode(pkce.Challenge, state, region)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "device code request failed: "+err.Error()))
		return
	}

	// 3. 计算过期时间（MiniMax ExpiredIn 是绝对毫秒时间戳）
	nowMs := time.Now().UnixMilli()
	expiresInSec := int((device.ExpiredIn - nowMs) / 1000)
	if expiresInSec <= 0 {
		expiresInSec = 600 // 默认 10 分钟
	}

	sessionID := fmt.Sprintf("device-%s-%d", providerID, time.Now().UnixNano())
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Duration(expiresInSec)*time.Second)

	session := &deviceCodeSession{
		Provider:  providerID,
		ResultCh:  make(chan deviceCodeResult, 1),
		Cancel:    cancel,
		ExpiresAt: time.Now().Add(time.Duration(expiresInSec) * time.Second),
	}
	deviceCodeSessionsMu.Lock()
	deviceCodeSessions[sessionID] = session
	deviceCodeSessionsMu.Unlock()

	// 4. 后台 goroutine 轮询 token
	go func() {
		defer cancel()
		pollIntervalMs := 2000
		if device.Interval > 0 {
			pollIntervalMs = device.Interval // MiniMax Interval 已经是毫秒
		}

		for time.Now().UnixMilli() < device.ExpiredIn {
			result := minimax.PollOAuthToken(device.UserCode, pkce.Verifier, region)
			if result.Status == "success" && result.Token != nil {
				session.ResultCh <- deviceCodeResult{AccessToken: result.Token.Access}
				persistOAuthToken("minimax-portal", result.Token.Access, result.Token.Refresh, result.Token.Expires)
				break
			}
			if result.Status == "error" {
				session.ResultCh <- deviceCodeResult{Err: fmt.Errorf("%s", result.Message)}
				break
			}
			// pending 状态，逐步退避
			pollIntervalMs = int(math.Min(float64(pollIntervalMs)*1.5, 10000))
			select {
			case <-pollCtx.Done():
				session.ResultCh <- deviceCodeResult{Err: fmt.Errorf("polling cancelled")}
				return
			case <-time.After(time.Duration(pollIntervalMs) * time.Millisecond):
			}
		}

		time.AfterFunc(5*time.Minute, func() {
			deviceCodeSessionsMu.Lock()
			delete(deviceCodeSessions, sessionID)
			deviceCodeSessionsMu.Unlock()
		})
	}()

	wizardV2Logger.Info("MiniMax device code flow started", "sessionID", sessionID, "userCode", device.UserCode)

	ctx.Respond(true, map[string]interface{}{
		"ok":              true,
		"provider":        providerID,
		"userCode":        device.UserCode,
		"verificationUri": device.VerificationURI,
		"expiresIn":       expiresInSec,
		"sessionId":       sessionID, // 前端使用 camelCase sessionId
	}, nil)
}

// handleDeviceStartQwen 启动 Qwen 设备码流程。
// 仿照 GitHub Copilot 模式：先同步请求设备码返回 userCode，后台 goroutine 轮询 token。
func handleDeviceStartQwen(ctx *MethodHandlerContext, providerID string) {
	// 1. 同步生成 PKCE
	pkce, err := common.GeneratePKCEVerifierChallenge()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "PKCE generation failed: "+err.Error()))
		return
	}

	// 2. 同步请求设备码（立即返回 userCode）
	device, err := qwen.RequestDeviceCode(pkce.Challenge)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "device code request failed: "+err.Error()))
		return
	}

	// 3. 创建后台轮询 session
	sessionID := fmt.Sprintf("device-%s-%d", providerID, time.Now().UnixNano())
	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600 // 默认 10 分钟
	}
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Duration(expiresIn)*time.Second)

	session := &deviceCodeSession{
		Provider:  providerID,
		ResultCh:  make(chan deviceCodeResult, 1),
		Cancel:    cancel,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	deviceCodeSessionsMu.Lock()
	deviceCodeSessions[sessionID] = session
	deviceCodeSessionsMu.Unlock()

	// 4. 后台 goroutine 轮询 token
	go func() {
		defer cancel()
		pollIntervalMs := 2000
		if device.Interval > 0 {
			pollIntervalMs = device.Interval * 1000
		}
		timeoutMs := int64(expiresIn) * 1000
		start := time.Now()

		for time.Since(start).Milliseconds() < timeoutMs {
			result := qwen.PollDeviceToken(device.DeviceCode, pkce.Verifier)
			if result.Status == "success" && result.Token != nil {
				session.ResultCh <- deviceCodeResult{AccessToken: result.Token.Access}
				persistOAuthToken("qwen-portal", result.Token.Access, result.Token.Refresh, result.Token.Expires)
				break
			}
			if result.Status == "error" {
				session.ResultCh <- deviceCodeResult{Err: fmt.Errorf("%s", result.Message)}
				break
			}
			if result.SlowDown {
				pollIntervalMs = int(math.Min(float64(pollIntervalMs)*1.5, 10000))
			}
			select {
			case <-pollCtx.Done():
				session.ResultCh <- deviceCodeResult{Err: fmt.Errorf("polling cancelled")}
				return
			case <-time.After(time.Duration(pollIntervalMs) * time.Millisecond):
			}
		}

		time.AfterFunc(5*time.Minute, func() {
			deviceCodeSessionsMu.Lock()
			delete(deviceCodeSessions, sessionID)
			deviceCodeSessionsMu.Unlock()
		})
	}()

	// 5. 立即返回 userCode + verificationUri（前端可以显示 UI）
	verificationUri := device.VerificationURIComplete
	if verificationUri == "" {
		verificationUri = device.VerificationURI
	}

	wizardV2Logger.Info("Qwen device code flow started", "sessionID", sessionID, "userCode", device.UserCode)

	ctx.Respond(true, map[string]interface{}{
		"ok":              true,
		"provider":        providerID,
		"userCode":        device.UserCode,
		"verificationUri": verificationUri,
		"expiresIn":       expiresIn,
		"sessionId":       sessionID, // 前端使用 camelCase sessionId
		"note":            "Qwen 将自动打开浏览器进行授权，请在浏览器中完成操作",
	}, nil)
}

// handleWizardV2OAuthDevicePoll 轮询设备码授权结果。
func handleWizardV2OAuthDevicePoll(ctx *MethodHandlerContext) {
	sessionID, _ := ctx.Params["sessionId"].(string)
	if sessionID == "" {
		// 兼容旧版前端使用 sessionID（大写 D）
		sessionID, _ = ctx.Params["sessionID"].(string)
	}
	if sessionID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams, "missing 'sessionId' parameter"))
		return
	}

	deviceCodeSessionsMu.Lock()
	session, ok := deviceCodeSessions[sessionID]
	deviceCodeSessionsMu.Unlock()

	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInvalidParams,
			"session '"+sessionID+"' not found or expired"))
		return
	}

	// 检查是否已过期
	if time.Now().After(session.ExpiresAt) {
		deviceCodeSessionsMu.Lock()
		delete(deviceCodeSessions, sessionID)
		deviceCodeSessionsMu.Unlock()
		ctx.Respond(true, map[string]interface{}{
			"ok":     false,
			"status": "expired",
			"error":  "device code expired",
		}, nil)
		return
	}

	// 非阻塞检查结果
	select {
	case result := <-session.ResultCh:
		// 放回通道以便后续 poll 也能获取
		session.ResultCh <- result

		if result.Err != nil {
			ctx.Respond(true, map[string]interface{}{
				"ok":     false,
				"status": "error",
				"error":  result.Err.Error(),
			}, nil)
		} else {
			ctx.Respond(true, map[string]interface{}{
				"ok":          true,
				"status":      "completed",
				"provider":    session.Provider,
				"accessToken": result.AccessToken,
			}, nil)
		}
	default:
		ctx.Respond(true, map[string]interface{}{
			"ok":     true,
			"status": "pending",
		}, nil)
	}
}

// persistOAuthToken 持久化 OAuth token 到 auth-profiles.json。
func persistOAuthToken(providerID, accessToken, refreshToken string, expiresMs int64) {
	home, err := os.UserHomeDir()
	if err != nil {
		wizardV2Logger.Warn("Failed to get home dir for OAuth persistence", "error", err)
		return
	}

	// 使用 go-providers 的 authprofile 写入（而非 auth.AuthStore）
	agentDir := filepath.Join(home, ".openacosmi", "state", "agents", "main", "agent")
	creds := map[string]interface{}{
		"access": accessToken,
	}
	if refreshToken != "" {
		creds["refresh"] = refreshToken
	}
	if expiresMs > 0 {
		creds["expires"] = expiresMs
	}

	// 使用 onboard.WriteOAuthCredentials 写入 auth-profiles.json
	profileID, writeErr := onboard.WriteOAuthCredentials(providerID, creds, agentDir, &onboard.WriteOAuthCredentialsOptions{
		SyncSiblingAgents: true,
	})
	if writeErr != nil {
		wizardV2Logger.Warn("Failed to persist OAuth token", "provider", providerID, "error", writeErr)
	} else {
		wizardV2Logger.Info("OAuth token persisted", "provider", providerID, "profileID", profileID)
	}
}

// persistGeminiCliOAuthToken 持久化 Gemini CLI OAuth 凭据（含 projectId）。
func persistGeminiCliOAuthToken(providerID string, creds *gemini.GeminiCliOAuthCredentials) {
	home, err := os.UserHomeDir()
	if err != nil {
		wizardV2Logger.Warn("Failed to get home dir for Gemini CLI OAuth persistence", "error", err)
		return
	}

	agentDir := filepath.Join(home, ".openacosmi", "state", "agents", "main", "agent")
	credsMap := map[string]interface{}{
		"access": creds.Access,
	}
	if creds.Refresh != "" {
		credsMap["refresh"] = creds.Refresh
	}
	if creds.Expires > 0 {
		credsMap["expires"] = creds.Expires
	}
	if creds.Email != "" {
		credsMap["email"] = creds.Email
	}
	if creds.ProjectID != "" {
		credsMap["projectId"] = creds.ProjectID
	}

	profileID, writeErr := onboard.WriteOAuthCredentials(providerID, credsMap, agentDir, &onboard.WriteOAuthCredentialsOptions{
		SyncSiblingAgents: true,
	})
	if writeErr != nil {
		wizardV2Logger.Warn("Failed to persist Gemini CLI OAuth token", "provider", providerID, "error", writeErr)
	} else {
		wizardV2Logger.Info("Gemini CLI OAuth token persisted",
			"provider", providerID,
			"profileID", profileID,
			"email", creds.Email,
			"projectID", creds.ProjectID,
		)
	}
}

func handleWizardV2Apply(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	// 1. 解析 WizardV2Payload
	payload, err := parseWizardV2Payload(ctx.Params)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid wizard v2 payload: "+err.Error()))
		return
	}

	// 2. 读取当前配置作为基础（保留已有配置）
	currentCfg, loadErr := loader.LoadConfig()
	if loadErr != nil || currentCfg == nil {
		currentCfg = &types.OpenAcosmiConfig{}
	}

	// 2.5 清除 wizard 管理的段落（防止旧配置残留）
	resetWizardManagedSections(currentCfg)

	// 3. 将 Payload 转换为配置并合并
	convertWizardV2PayloadToConfig(payload, currentCfg)

	// 4. 写入 wizard 元数据
	currentCfg.Wizard = &types.OpenAcosmiWizardConfig{
		LastRunAt:      time.Now().Format(time.RFC3339),
		LastRunVersion: config.BuildVersion,
		LastRunMode:    "local",
		LastRunCommand: "wizard.v2.apply",
	}

	// 5. 写入配置文件（WriteConfigFile 内部自动处理 Keyring 脱敏）
	if err := loader.WriteConfigFile(currentCfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write config: "+err.Error()))
		return
	}
	loader.ClearCache()

	wizardV2Logger.Info("Wizard V2 config applied successfully", "path", loader.ConfigPath())

	// 7. 写入 restart sentinel
	var sentinelPath string
	if sw := ctx.Context.RestartSentinel; sw != nil {
		sentinelPayload := &RestartSentinelPayload{
			Kind:       "wizard-v2-apply",
			Status:     "ok",
			Ts:         time.Now().UnixMilli(),
			DoctorHint: sw.FormatDoctorNonInteractiveHint(),
			Stats: map[string]interface{}{
				"mode":          "wizard.v2.apply",
				"root":          loader.ConfigPath(),
				"securityLevel": payload.SecurityLevelConfig,
			},
		}
		sentinelPath, _ = sw.WriteRestartSentinel(sentinelPayload)
	}

	// 8. 调度网关重启（热重载）
	var restartResult *GatewayRestartResult
	if gr := ctx.Context.GatewayRestarter; gr != nil {
		restartResult = gr.ScheduleRestart(nil, "wizard.v2.apply")
	}

	// 9. 构建响应
	result := map[string]interface{}{
		"ok":   true,
		"path": loader.ConfigPath(),
	}
	if restartResult != nil {
		result["restart"] = restartResult
	}
	if sentinelPath != "" {
		result["sentinelPath"] = sentinelPath
	}

	ctx.Respond(true, result, nil)
}

// ---------- Payload 解析 ----------

func parseWizardV2Payload(params map[string]interface{}) (*WizardV2Payload, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var payload WizardV2Payload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// ---------- Payload → Config 转换 ----------

// convertWizardV2PayloadToConfig 将 WizardV2Payload 的字段合并到现有的 OpenAcosmiConfig 中。
// 仅写入用户在向导中实际配置的字段，空值不覆盖已有配置。
func convertWizardV2PayloadToConfig(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	// --- 模型 Provider 配置 ---
	applyProviderConfigs(p, cfg)

	// --- 设置 agents.defaults.model.primary（决定系统使用哪个模型） ---
	applyPrimaryModelSelection(p, cfg)

	// --- 子智能体配置 ---
	applySubAgentsConfig(p, cfg)

	// --- 技能配置 ---
	applySkillsConfig(p, cfg)

	// --- 频道配置 ---
	applyChannelConfigs(p, cfg)

	// --- 记忆系统配置 ---
	applyMemoryConfig(p, cfg)

	// --- 安全级别配置 ---
	applySecurityLevelConfig(p, cfg)
}

// applyProviderConfigs 将 primaryConfig + fallbackConfig 转换为 ModelsConfig.Providers。
// 使用 bridge.ApplyProviderByID 注入完整的 provider 配置（API 类型、BaseURL、
// 完整 ModelDefinitionConfig 含 ContextWindow/MaxTokens/Reasoning）。
func applyProviderConfigs(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	if cfg.Models == nil {
		cfg.Models = &types.ModelsConfig{}
	}
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]*types.ModelProviderConfig)
	}

	// 合并 primary 和 fallback 到同一个 Providers map
	allConfigs := make(map[string]string)
	for k, v := range p.FallbackConfig {
		if strings.TrimSpace(v) != "" {
			allConfigs[k] = v
		}
	}
	// primary 覆盖 fallback（如果同一 provider 同时配置了 primary 和 fallback）
	for k, v := range p.PrimaryConfig {
		if strings.TrimSpace(v) != "" {
			allConfigs[k] = v
		}
	}

	for rawProviderID, apiKey := range allConfigs {
		providerID := normalizeProviderID(strings.TrimSpace(rawProviderID))
		apiKey = strings.TrimSpace(apiKey)
		if providerID == "" || apiKey == "" {
			continue
		}

		// 检查 providerSelections 的 authMode
		sel, hasSel := p.ProviderSelections[providerID]
		if !hasSel {
			sel, hasSel = p.ProviderSelections[rawProviderID]
		}

		// OAuth/DeviceCode 标记不是真实 API Key，跳过 APIKey 注入
		actualAPIKey := apiKey
		isOAuthMarker := apiKey == "oauth-authorized" ||
			strings.HasPrefix(apiKey, "device-code-") ||
			(hasSel && (sel.AuthMode == "oauth" || sel.AuthMode == "deviceCode"))
		if isOAuthMarker {
			actualAPIKey = "" // 真实 token 在 auth-profiles.json 中，不写入 config
		}

		// 自定义 BaseURL
		customURL := ""
		if u, ok := p.CustomBaseUrls[providerID]; ok && strings.TrimSpace(u) != "" {
			customURL = strings.TrimSpace(u)
		}
		if customURL == "" {
			if u, ok := p.CustomBaseUrls[rawProviderID]; ok && strings.TrimSpace(u) != "" {
				customURL = strings.TrimSpace(u)
			}
		}

		// 调用 bridge 注入完整 provider 配置（API 类型、BaseURL、模型列表）
		bridge.ApplyProviderByID(providerID, cfg, &bridge.ApplyOpts{
			APIKey:  actualAPIKey,
			BaseURL: customURL,
		})

		// 从 providerSelections 设置认证模式（OAuth 或 DeviceCode 均标记为 oauth auth）
		if hasSel && (sel.AuthMode == "oauth" || sel.AuthMode == "deviceCode") {
			if provCfg := cfg.Models.Providers[providerID]; provCfg != nil {
				provCfg.Auth = types.ModelAuthOAuth
			}
		}
	}
}

// applyChannelConfigs 将频道配置写入 ChannelsConfig。
func applyChannelConfigs(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	ch := p.ChannelConfig

	// 飞书
	if strings.TrimSpace(ch.Feishu.AppID) != "" || strings.TrimSpace(ch.Feishu.AppSecret) != "" {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.Feishu == nil {
			cfg.Channels.Feishu = &types.FeishuConfig{}
		}
		enabled := true
		if strings.TrimSpace(ch.Feishu.AppID) != "" {
			cfg.Channels.Feishu.AppID = ch.Feishu.AppID
		}
		if strings.TrimSpace(ch.Feishu.AppSecret) != "" {
			cfg.Channels.Feishu.AppSecret = ch.Feishu.AppSecret
		}
		cfg.Channels.Feishu.Enabled = &enabled
	}

	// 企微
	if strings.TrimSpace(ch.WeCom.AppID) != "" || strings.TrimSpace(ch.WeCom.AppSecret) != "" {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.WeCom == nil {
			cfg.Channels.WeCom = &types.WeComConfig{}
		}
		enabled := true
		if strings.TrimSpace(ch.WeCom.AppID) != "" {
			cfg.Channels.WeCom.CorpID = ch.WeCom.AppID
		}
		if strings.TrimSpace(ch.WeCom.AppSecret) != "" {
			cfg.Channels.WeCom.Secret = ch.WeCom.AppSecret
		}
		cfg.Channels.WeCom.Enabled = &enabled
	}

	// 钉钉
	if strings.TrimSpace(ch.DingTalk.AppKey) != "" || strings.TrimSpace(ch.DingTalk.AppSecret) != "" {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.DingTalk == nil {
			cfg.Channels.DingTalk = &types.DingTalkConfig{}
		}
		enabled := true
		if strings.TrimSpace(ch.DingTalk.AppKey) != "" {
			cfg.Channels.DingTalk.AppKey = ch.DingTalk.AppKey
		}
		if strings.TrimSpace(ch.DingTalk.AppSecret) != "" {
			cfg.Channels.DingTalk.AppSecret = ch.DingTalk.AppSecret
		}
		cfg.Channels.DingTalk.Enabled = &enabled
	}

	// Telegram
	if strings.TrimSpace(ch.Telegram.BotToken) != "" {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.Telegram == nil {
			cfg.Channels.Telegram = &types.TelegramConfig{}
		}
		enabled := true
		cfg.Channels.Telegram.BotToken = ch.Telegram.BotToken
		cfg.Channels.Telegram.Enabled = &enabled
	}
}

// applyMemoryConfig 将记忆系统配置写入 MemoryConfig.UHMS。
func applyMemoryConfig(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	mc := p.MemoryConfig

	if cfg.Memory == nil {
		cfg.Memory = &types.MemoryConfig{}
	}
	if cfg.Memory.UHMS == nil {
		cfg.Memory.UHMS = &types.MemoryUHMSConfig{}
	}

	// 始终启用 UHMS（VFS 默认可用）
	cfg.Memory.UHMS.Enabled = true

	if mc.EnableVector {
		cfg.Memory.UHMS.VectorMode = "qdrant"
		if strings.TrimSpace(mc.APIEndpoint) != "" {
			cfg.Memory.UHMS.QdrantEndpoint = strings.TrimSpace(mc.APIEndpoint)
		}
	}

	// Bug#11: 记忆提取 LLM 配置
	if strings.TrimSpace(mc.LLMProvider) != "" {
		cfg.Memory.UHMS.LLMProvider = strings.TrimSpace(mc.LLMProvider)
	}
	if strings.TrimSpace(mc.LLMModel) != "" {
		cfg.Memory.UHMS.LLMModel = strings.TrimSpace(mc.LLMModel)
	}
	if strings.TrimSpace(mc.LLMApiKey) != "" {
		cfg.Memory.UHMS.LLMApiKey = strings.TrimSpace(mc.LLMApiKey)
	}
	if strings.TrimSpace(mc.LLMBaseURL) != "" {
		cfg.Memory.UHMS.LLMBaseURL = strings.TrimSpace(mc.LLMBaseURL)
	}
}

// applySecurityLevelConfig 将全局安全级别映射到 Gateway 节点命令策略。
func applySecurityLevelConfig(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	level := p.SecurityLevelConfig
	if level == "" {
		return
	}

	if cfg.Gateway == nil {
		cfg.Gateway = &types.GatewayConfig{}
	}

	switch level {
	case "deny":
		// 完全禁止 — 拒绝所有命令执行
		cfg.Gateway.Nodes = &types.GatewayNodesConfig{
			DenyCommands: []string{"*"},
		}
	case "sandboxed":
		// 沙箱模式 — 使用默认危险命令拒绝列表，强制沙箱
		cfg.Gateway.Nodes = &types.GatewayNodesConfig{
			DenyCommands: append([]string{}, DefaultDangerousNodeDenyCommands...),
		}
	case "allowlist":
		// 标准白名单模式 — 使用默认危险命令拒绝列表
		cfg.Gateway.Nodes = &types.GatewayNodesConfig{
			DenyCommands: append([]string{}, DefaultDangerousNodeDenyCommands...),
		}
	case "full":
		// 全权放行 — 清空拒绝列表
		cfg.Gateway.Nodes = &types.GatewayNodesConfig{
			DenyCommands:  []string{},
			AllowCommands: []string{"*"},
		}
	}
}

// applySkillsConfig 将技能选择映射到 ToolsConfig.Allow。
// key 匹配 scope.ToolGroups 的分组名（fs, runtime, ui, web, memory, sessions, automation, messaging, nodes）。
func applySkillsConfig(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	if len(p.SelectedSkills) == 0 {
		return
	}

	// 收集启用的技能组
	var allowGroups []string
	for groupName, enabled := range p.SelectedSkills {
		if enabled {
			// 转换为 scope.ToolGroups 格式
			if !strings.HasPrefix(groupName, "group:") {
				groupName = "group:" + groupName
			}
			allowGroups = append(allowGroups, groupName)
		}
	}

	// 写入 ToolsConfig.Allow（工具策略，控制 AI 可使用哪些工具组）
	if len(allowGroups) > 0 {
		if cfg.Tools == nil {
			cfg.Tools = &types.ToolsConfig{}
		}
		cfg.Tools.Allow = allowGroups
	}
}

// ---------- Primary Model Selection ----------

// normalizeProviderID 规范化前端 provider ID 到后端存储 ID。
// 委托到 bridge.NormalizeProviderID（处理 zhipu→zai, doubao→volcengine 等映射）。
func normalizeProviderID(frontendID string) string {
	return bridge.NormalizeProviderID(frontendID)
}

// applyPrimaryModelSelection 从 Wizard payload 推导主模型并写入 agents.defaults.model.primary。
// 优先级: primaryConfig 中第一个有 API key + 有模型选择的 provider。
// 使用 bridge.GetDefaultModelRef 获取默认模型（替代 models.GetProviderDefaults）。
func applyPrimaryModelSelection(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	var primaryProvider, primaryModel string

	// 从 primaryConfig 中找第一个有 API key 的 provider
	for rawProviderID, apiKey := range p.PrimaryConfig {
		if strings.TrimSpace(apiKey) == "" {
			continue
		}
		providerID := normalizeProviderID(strings.TrimSpace(rawProviderID))

		// 从 providerSelections 获取模型（尝试规范化 ID 和原始 ID）
		model := ""
		if sel, ok := p.ProviderSelections[providerID]; ok && sel.Model != "" {
			model = sel.Model
		}
		if model == "" {
			if sel, ok := p.ProviderSelections[rawProviderID]; ok && sel.Model != "" {
				model = sel.Model
			}
		}

		// 如果没有指定模型，从 bridge 获取默认模型
		if model == "" {
			defaultRef := bridge.GetDefaultModelRef(providerID)
			if defaultRef != "" && defaultRef != providerID+"/" {
				model = defaultRef
			}
		}

		if model != "" {
			primaryProvider = providerID
			primaryModel = model
			break
		}
		// 即使没有 model 也记录第一个有 key 的 provider
		if primaryProvider == "" {
			primaryProvider = providerID
		}
	}

	if primaryProvider == "" {
		return // 没有配置任何 provider
	}

	// 确保 agents.defaults 链路完整
	if cfg.Agents == nil {
		cfg.Agents = &types.AgentsConfig{}
	}
	if cfg.Agents.Defaults == nil {
		cfg.Agents.Defaults = &types.AgentDefaultsConfig{}
	}
	if cfg.Agents.Defaults.Model == nil {
		cfg.Agents.Defaults.Model = &types.AgentModelListConfig{}
	}

	// 写入 "provider/model" 格式
	if primaryModel != "" {
		if strings.Contains(primaryModel, "/") {
			// 已经是 provider/model 格式（来自 bridge.GetDefaultModelRef）
			cfg.Agents.Defaults.Model.Primary = primaryModel
		} else {
			cfg.Agents.Defaults.Model.Primary = primaryProvider + "/" + primaryModel
		}
	} else {
		// 没有模型但有 provider — 至少写入 provider，让系统回退到 DefaultModel
		cfg.Agents.Defaults.Model.Primary = primaryProvider + "/"
	}

	// 构建 fallback 列表（fallbackConfig 中其他 provider 的模型）
	var fallbacks []string
	for rawProviderID, apiKey := range p.FallbackConfig {
		if strings.TrimSpace(apiKey) == "" {
			continue
		}
		providerID := normalizeProviderID(strings.TrimSpace(rawProviderID))
		if providerID == primaryProvider {
			continue
		}
		model := ""
		if sel, ok := p.ProviderSelections[providerID]; ok && sel.Model != "" {
			model = sel.Model
		}
		if model == "" {
			if sel, ok := p.ProviderSelections[rawProviderID]; ok && sel.Model != "" {
				model = sel.Model
			}
		}
		if model == "" {
			defaultRef := bridge.GetDefaultModelRef(providerID)
			if defaultRef != "" && defaultRef != providerID+"/" {
				model = defaultRef
			}
		}
		if model != "" {
			if strings.Contains(model, "/") {
				fallbacks = append(fallbacks, model)
			} else {
				fallbacks = append(fallbacks, providerID+"/"+model)
			}
		}
	}
	if len(fallbacks) > 0 {
		cfg.Agents.Defaults.Model.Fallbacks = &fallbacks
	}

	wizardV2Logger.Info("Primary model set", "model", cfg.Agents.Defaults.Model.Primary)
}

// ---------- Wizard 管理段落重置 ----------

// resetWizardManagedSections 清除 wizard 管理的配置段落，防止重新配置时旧数据残留。
// 仅清除 wizard 会重新填充的字段，保留非 wizard 管理的配置不变。
func resetWizardManagedSections(cfg *types.OpenAcosmiConfig) {
	// Models.Providers — 清空（保留 Mode、BedrockDiscovery）
	if cfg.Models != nil {
		cfg.Models.Providers = make(map[string]*types.ModelProviderConfig)
	}

	// Agents.Defaults.Model — 清空主模型选择（保留 Defaults 其他字段、List）
	if cfg.Agents != nil && cfg.Agents.Defaults != nil {
		cfg.Agents.Defaults.Model = nil
	}

	// Tools.Allow — 清空技能白名单（保留 Deny 和其他 Tools 字段）
	if cfg.Tools != nil {
		cfg.Tools.Allow = nil
	}

	// Channels — 仅清除 wizard 管理的频道（飞书/企微/钉钉/Telegram）
	// 保留 Slack、WhatsApp、Discord、iMessage、Signal 等非 wizard 频道
	if cfg.Channels != nil {
		cfg.Channels.Feishu = nil
		cfg.Channels.WeCom = nil
		cfg.Channels.DingTalk = nil
		cfg.Channels.Telegram = nil
	}

	// Memory.UHMS — 清空（保留 Backend、Citations、Qmd）
	if cfg.Memory != nil {
		cfg.Memory.UHMS = nil
	}

	// SubAgents — 清空 OpenCoder 和 ScreenObserver
	if cfg.SubAgents != nil {
		cfg.SubAgents.OpenCoder = nil
		cfg.SubAgents.ScreenObserver = nil
	}

	// Gateway.Nodes — 清空安全策略（保留 Port、Mode、TLS 等）
	if cfg.Gateway != nil {
		cfg.Gateway.Nodes = nil
	}
}

// ---------- 子智能体配置 ----------

// applySubAgentsConfig 将子智能体配置写入 SubAgentConfig。
func applySubAgentsConfig(p *WizardV2Payload, cfg *types.OpenAcosmiConfig) {
	if len(p.SubAgentsConfig) == 0 {
		return
	}

	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}

	for name, entry := range p.SubAgentsConfig {
		switch strings.ToLower(name) {
		case "open-coder", "opencoder", "coder":
			if cfg.SubAgents.OpenCoder == nil {
				cfg.SubAgents.OpenCoder = &types.OpenCoderSettings{}
			}
			if entry.Provider != "" {
				cfg.SubAgents.OpenCoder.Provider = entry.Provider
			}
			if entry.Model != "" {
				cfg.SubAgents.OpenCoder.Model = entry.Model
			}
			if entry.APIKey != "" {
				cfg.SubAgents.OpenCoder.APIKey = entry.APIKey
			}
		case "argus", "screen-observer":
			if cfg.SubAgents.ScreenObserver == nil {
				cfg.SubAgents.ScreenObserver = &types.ScreenObserverSettings{}
			}
			if entry.Provider != "" {
				cfg.SubAgents.ScreenObserver.Provider = entry.Provider
			}
			if entry.Model != "" {
				cfg.SubAgents.ScreenObserver.Model = entry.Model
			}
			if entry.APIKey != "" {
				cfg.SubAgents.ScreenObserver.APIKey = entry.APIKey
			}
		case "media-agent", "media", "oa-media":
			if cfg.SubAgents.MediaAgent == nil {
				cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
			}
			if entry.Provider != "" {
				cfg.SubAgents.MediaAgent.Provider = entry.Provider
			}
			if entry.Model != "" {
				cfg.SubAgents.MediaAgent.Model = entry.Model
			}
			if entry.APIKey != "" {
				cfg.SubAgents.MediaAgent.APIKey = entry.APIKey
			}
		}
	}
}
