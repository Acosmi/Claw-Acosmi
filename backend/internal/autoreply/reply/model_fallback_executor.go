package reply

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// TS 对照: auto-reply/reply/agent-runner-execution.ts L146-465
// 将 RunWithModelFallback → RunEmbeddedPiAgent 管线桥接到 AgentExecutor 接口。

// ---------- ModelFallbackExecutor ----------

// ModelFallbackExecutor 基于 model fallback 的 agent 执行器。
// 实现 AgentExecutor 接口，替换 StubAgentExecutor。
//
// 管线: RunTurn → RunWithModelFallback → RunEmbeddedPiAgent
type ModelFallbackExecutor struct {
	RunnerDeps         runner.EmbeddedRunDeps
	Config             *types.OpenAcosmiConfig
	AgentDir           string
	OnPermissionDenied func(tool, detail string)              // 权限拒绝→WebSocket广播
	WaitForApproval    func(ctx context.Context) bool         // 等待提权审批
	SecurityLevelFunc  func() string                          // 动态安全级别
	MountRequestsFunc  func() []runner.MountRequestForSandbox // 动态临时挂载请求（Phase 3.4）
	OnToolEvent        func(event runner.ToolEvent)           // 结构化工具事件→频道广播
}

// RunTurn 执行一次 agent 回合（含模型失败切换）。
// TS 对照: agent-runner-execution.ts L54-604
func (e *ModelFallbackExecutor) RunTurn(ctx context.Context, params AgentTurnParams) (*AgentRunLoopResult, error) {
	log := slog.Default().With("subsystem", "model-fallback-executor")
	run := params.FollowupRun.Run

	provider := run.Provider
	model := run.Model
	if provider == "" || model == "" {
		// 优先从配置解析默认模型（向导保存的 agents.defaults.model.primary）
		if e.Config != nil {
			ref := models.ResolveDefaultModelForAgent(e.Config, run.AgentID)
			if ref.Provider != "" && ref.Model != "" {
				// 验证配置中的 provider 有可用的 API key
				if hasProviderAPIKey(e.Config, ref.Provider) {
					if provider == "" {
						provider = ref.Provider
					}
					if model == "" {
						model = ref.Model
					}
				}
			}
		}
		// 如果配置中也没有可用的模型，回退到环境变量检测
		if provider == "" {
			provider, model = autoDetectProvider()
		}
	}
	if model == "" {
		model = models.DefaultModel
	}

	// 1. 解析 fallbacksOverride
	fallbacksOverride := resolveFallbacksOverride(e.Config, run.SessionKey, run.AgentID)

	// 2. 准备 authStore 适配器
	var authChecker models.AuthProfileChecker
	if e.RunnerDeps.AuthStore != nil {
		authChecker = &authStoreAdapter{store: e.RunnerDeps.AuthStore, cfg: e.Config}
	}

	// 3. 通过 RunWithModelFallback 执行
	fallbackResult, err := models.RunWithModelFallback(
		ctx,
		e.Config,
		provider, model,
		fallbacksOverride,
		authChecker,
		func(ctx context.Context, fbProvider, fbModel string) (*runner.EmbeddedPiRunResult, error) {
			// 模型选择回调
			if params.OnModelSelected != nil {
				params.OnModelSelected(autoreply.ModelSelectedContext{
					Provider:   fbProvider,
					Model:      fbModel,
					ThinkLevel: string(run.ThinkLevel),
				})
			}

			return runner.RunEmbeddedPiAgent(ctx, runner.RunEmbeddedPiAgentParams{
				SessionID:           run.SessionID,
				SessionKey:          run.SessionKey,
				AgentID:             run.AgentID,
				SessionFile:         run.SessionFile,
				WorkspaceDir:        run.WorkspaceDir,
				AgentDir:            e.AgentDir,
				Config:              e.Config,
				Prompt:              params.CommandBody,
				ExtraSystemPrompt:   params.ExtraSystemPrompt,
				Provider:            fbProvider,
				Model:               fbModel,
				AuthProfileID:       resolveAuthProfileID(run, fbProvider),
				AuthProfileIDSource: resolveAuthProfileIDSource(run, fbProvider),
				ThinkLevel:          string(run.ThinkLevel),
				TimeoutMs:           int64(run.TimeoutMs),
				RunID:               run.RunID, // Bug#11: 从 FollowupRunParams 传递，确保全链路可追踪
				SuppressTranscript:  true,      // Bug#11: fallback 场景跳过 transcript 持久化，避免失败 attempt 污染历史
				FallbackModels:      fallbacksOverride,
				OnPermissionDenied:  e.OnPermissionDenied,
				WaitForApproval:     e.WaitForApproval,
				SecurityLevelFunc:   e.SecurityLevelFunc,
				MountRequestsFunc:   e.MountRequestsFunc,
				OnToolEvent:         e.OnToolEvent,
				OnProgress:          params.OnProgress,
			}, e.RunnerDeps)
		},
		func(fbProvider, fbModel string, fbErr error, attempt, total int) {
			log.Warn("model fallback attempt failed",
				"provider", fbProvider,
				"model", fbModel,
				"attempt", attempt,
				"total", total,
				"error", fbErr,
			)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("agent model fallback failed: %w", err)
	}

	// 4. 转换 EmbeddedPiRunResult → AgentRunLoopResult
	return convertEmbeddedResult(fallbackResult.Result, fallbackResult.Provider, fallbackResult.Model), nil
}

// ---------- 结果转换 ----------

// convertEmbeddedResult 将 runner.EmbeddedPiRunResult 转换为 AgentRunLoopResult。
func convertEmbeddedResult(result *runner.EmbeddedPiRunResult, provider, model string) *AgentRunLoopResult {
	if result == nil {
		return &AgentRunLoopResult{}
	}

	// 转换 payloads
	var payloads []autoreply.ReplyPayload
	for _, p := range result.Payloads {
		mediaItems := make([]autoreply.ReplyMediaItem, 0, len(p.MediaItems))
		for _, item := range p.MediaItems {
			if item.Base64 == "" {
				continue
			}
			mediaItems = append(mediaItems, autoreply.ReplyMediaItem{
				MediaBase64:   item.Base64,
				MediaMimeType: item.MimeType,
			})
		}
		payloads = append(payloads, autoreply.ReplyPayload{
			Text:          p.Text,
			MediaURL:      p.MediaURL,
			MediaURLs:     p.MediaURLs,
			MediaItems:    mediaItems,
			MediaBase64:   p.MediaBase64,
			MediaMimeType: p.MediaMimeType,
			IsError:       p.IsError,
		})
	}

	// 转换 usage
	var usage *NormalizedUsage
	if result.Meta.AgentMeta != nil && result.Meta.AgentMeta.Usage != nil {
		u := result.Meta.AgentMeta.Usage
		usage = &NormalizedUsage{}
		if u.Input > 0 {
			v := u.Input
			usage.Input = &v
		}
		if u.Output > 0 {
			v := u.Output
			usage.Output = &v
		}
		if u.CacheRead > 0 {
			v := u.CacheRead
			usage.CacheRead = &v
		}
		if u.CacheWrite > 0 {
			v := u.CacheWrite
			usage.CacheWrite = &v
		}
	}

	return &AgentRunLoopResult{
		Payloads:                 payloads,
		Usage:                    usage,
		SessionResetRequired:     false,
		Error:                    nil,
		ToolResultsSent:          0,
		MessagingToolSentTargets: result.MessagingToolSentTargets,
	}
}

// ---------- 辅助函数 ----------

// resolveFallbacksOverride 解析 agent 级 fallback 覆盖。
// TS 对照: agent-scope.ts → resolveAgentModelFallbacksOverride()
func resolveFallbacksOverride(cfg *types.OpenAcosmiConfig, sessionKey, agentID string) []string {
	if agentID == "" {
		return nil
	}
	return scope.ResolveAgentModelFallbacksOverride(cfg, agentID)
}

// resolveAuthProfileID 确定是否传递 auth profile ID。
// 仅当 provider 匹配原始 provider 时传递原始 profile。
func resolveAuthProfileID(run FollowupRunParams, candidateProvider string) string {
	if candidateProvider == run.Provider && run.AuthProfileID != "" {
		return run.AuthProfileID
	}
	return ""
}

// resolveAuthProfileIDSource 确定 auth profile ID 来源。
func resolveAuthProfileIDSource(run FollowupRunParams, candidateProvider string) string {
	if candidateProvider == run.Provider && run.AuthProfileID != "" {
		return run.AuthProfileIDSrc
	}
	return ""
}

// ---------- AuthStore 适配器 ----------

// authStoreAdapter 将 runner.AuthProfileStore 适配为 models.AuthProfileChecker。
type authStoreAdapter struct {
	store runner.AuthProfileStore
	cfg   *types.OpenAcosmiConfig
}

func (a *authStoreAdapter) ResolveProfileOrder(cfg *types.OpenAcosmiConfig, provider string, preferred string) []string {
	return a.store.ResolveProfileOrder(cfg, provider, preferred)
}

func (a *authStoreAdapter) IsInCooldown(profileID string) bool {
	return a.store.IsInCooldown(profileID)
}

// autoDetectProvider 自动检测可用的 LLM 供应商。
// 通过检查环境变量中的 API Key 来确定使用哪个供应商。
// 优先级: ANTHROPIC → DEEPSEEK → OPENAI → 默认 anthropic
func autoDetectProvider() (provider, model string) {
	if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_OAUTH_TOKEN") != "" {
		return models.DefaultProvider, models.DefaultModel
	}
	if os.Getenv("DEEPSEEK_API_KEY") != "" {
		defaults := models.GetProviderDefaults("deepseek")
		if defaults != nil && defaults.DefaultModel != "" {
			return "deepseek", defaults.DefaultModel
		}
		return "deepseek", "deepseek-chat"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "openai", "gpt-4o"
	}
	return models.DefaultProvider, models.DefaultModel
}

// hasProviderAPIKey 检查指定供应商是否有可用的 API Key（配置文件或环境变量）。
func hasProviderAPIKey(cfg *types.OpenAcosmiConfig, providerID string) bool {
	// 1. 检查配置中的 API key
	if cfg != nil && cfg.Models != nil && cfg.Models.Providers != nil {
		if pc := cfg.Models.Providers[providerID]; pc != nil && pc.APIKey != "" {
			return true
		}
	}
	// 2. 检查环境变量
	if models.ResolveEnvApiKeyWithFallback(providerID) != "" {
		return true
	}
	// 3. Ollama 等本地 provider 不需要 key
	if providerID == "ollama" {
		return true
	}
	return false
}
