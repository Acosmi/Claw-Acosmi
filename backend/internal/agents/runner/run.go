package runner

// ============================================================================
// runEmbeddedPiAgent — 完整实现
// 对应 TS: pi-embedded-runner/run.ts (867L)
// 隐藏依赖审计: docs/renwu/phase4-embedded-runner-audit.md
// ============================================================================

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/helpers"
	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/workspace"
)

const (
	DefaultProvider        = "anthropic"
	DefaultModel           = "claude-sonnet-4-20250514"
	MaxOverflowCompactions = 3
)

// RunEmbeddedPiAgent 执行嵌入式 PI Agent。
// TS 对应: pi-embedded-runner/run.ts → runEmbeddedPiAgent()
//
// ctx 控制整个 run 的生命周期；调用方可传入带取消/超时的 context。
// 传 context.Background() 等效于旧行为。
func RunEmbeddedPiAgent(ctx context.Context, params RunEmbeddedPiAgentParams, deps EmbeddedRunDeps) (*EmbeddedPiRunResult, error) {
	// activeRuns 追踪: 注册当前 run，结束时自动去注册
	stubHandle := &stubRunHandle{}
	DefaultActiveRuns.RegisterRun(params.SessionID, stubHandle)
	defer DefaultActiveRuns.DeregisterRun(params.SessionID, stubHandle)

	if deps.ModelResolver == nil || deps.AttemptRunner == nil {
		return nil, errors.New("agents/runner: RunEmbeddedPiAgent deps not configured")
	}
	log := slog.Default().With("subsystem", "embedded-pi-runner")
	started := time.Now()
	// ctx is now received from the caller; derive a cancellable child so we
	// can cancel all downstream operations when the run ends.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. 工作区解析
	wsResult := workspace.ResolveRunWorkspaceDir(
		params.WorkspaceDir, params.SessionKey, params.AgentID, params.Config,
	)
	resolvedWorkspace := wsResult.WorkspaceDir
	if wsResult.UsedFallback {
		log.Warn("workspace-fallback",
			"caller", "runEmbeddedPiAgent",
			"runId", params.RunID,
		)
	}

	// 2. 模型解析
	provider := params.Provider
	if provider == "" {
		provider = DefaultProvider
	}
	modelID := params.Model
	if modelID == "" {
		modelID = DefaultModel
	}
	agentDir := params.AgentDir

	resolved := deps.ModelResolver.ResolveModel(provider, modelID, agentDir, params.Config)
	if resolved.Model == nil {
		errMsg := resolved.Error
		if errMsg == "" {
			errMsg = fmt.Sprintf("Unknown model: %s/%s", provider, modelID)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	model := resolved.Model

	// 3. 上下文窗口检查
	ctxInfo := deps.ModelResolver.ResolveContextWindowInfo(params.Config, provider, modelID, model.ContextWindow)
	ctxGuard := EvaluateContextWindowGuard(ctxInfo)
	if ctxGuard.ShouldWarn {
		log.Warn("low context window", "provider", provider, "model", modelID, "tokens", ctxGuard.Tokens)
	}
	if ctxGuard.ShouldBlock {
		return nil, &models.FailoverError{
			Message:  fmt.Sprintf("Model context window too small (%d tokens). Minimum is %d.", ctxGuard.Tokens, ContextWindowHardMinTokens),
			Reason:   "unknown",
			Provider: provider,
			Model:    modelID,
		}
	}

	// 4. Auth profile 解析
	fallbackConfigured := len(params.FallbackModels) > 0
	profileCandidates, lockedProfileID := resolveProfileCandidates(deps.AuthStore, provider, params)
	initialThinkLevel := params.ThinkLevel
	if initialThinkLevel == "" {
		initialThinkLevel = "off"
	}
	thinkLevel := initialThinkLevel
	attemptedThinking := map[string]bool{}
	var lastProfileID string

	// 5. 初始 auth profile 应用
	profileIndex := 0
	if err := applyInitialProfile(deps.AuthStore, model, params, profileCandidates, lockedProfileID, &profileIndex, &lastProfileID); err != nil {
		if _, ok := models.IsFailoverError(err); ok {
			return nil, err
		}
		if profileCandidates[profileIndex] == lockedProfileID {
			return nil, buildAuthFailoverError(provider, modelID, fallbackConfigured, false, err)
		}
		advanced := advanceProfile(deps.AuthStore, model, params, profileCandidates, lockedProfileID, &profileIndex, &lastProfileID, initialThinkLevel, &thinkLevel, attemptedThinking)
		if !advanced {
			return nil, buildAuthFailoverError(provider, modelID, fallbackConfigured, false, err)
		}
	}

	// 5b. 通知子智能体频道：任务已接收
	if params.OnCoderEvent != nil {
		params.OnCoderEvent("task_received", map[string]interface{}{
			"prompt": params.Prompt,
			"model":  modelID,
		})
	}

	// 6. 主重试循环
	usageAcc := NewUsageAccumulator()
	autoCompactionCount := 0
	overflowAttempts := 0

	for {
		attemptedThinking[thinkLevel] = true
		_ = os.MkdirAll(resolvedWorkspace, 0o755)

		prompt := params.Prompt
		if provider == "anthropic" {
			prompt = ScrubAnthropicRefusalMagic(prompt)
		}

		attempt, err := deps.AttemptRunner.RunAttempt(ctx, AttemptParams{
			SessionID:          params.SessionID,
			SessionKey:         params.SessionKey,
			SessionFile:        params.SessionFile,
			WorkspaceDir:       resolvedWorkspace,
			AgentDir:           agentDir,
			Config:             params.Config,
			Prompt:             prompt,
			Provider:           provider,
			ModelID:            modelID,
			Model:              model,
			ThinkLevel:         thinkLevel,
			TimeoutMs:          params.TimeoutMs,
			RunID:              params.RunID,
			ExtraSystemPrompt:  params.ExtraSystemPrompt,
			OnPermissionDenied: params.OnPermissionDenied,
			WaitForApproval:    params.WaitForApproval,
			SecurityLevelFunc:  params.SecurityLevelFunc,
			MountRequestsFunc:  params.MountRequestsFunc,
			DelegationContract: params.DelegationContract,
			PromptMode:         params.PromptMode,
			OnToolEvent:        params.OnToolEvent,
			OnProgress:         params.OnProgress,
			AgentChannel:       params.AgentChannel,
			AgentType:          params.AgentType,
			SuppressTranscript: params.SuppressTranscript,
		})
		if err != nil {
			return nil, err
		}

		durationMs := time.Since(started).Milliseconds()
		if attempt.AttemptUsage != nil {
			usageAcc.MergeUsage(attempt.AttemptUsage)
		}
		autoCompactionCount += intMax(0, attempt.CompactionCount)

		// 6a. 上下文溢出处理
		overflowResult, shouldRetry := handleContextOverflow(
			attempt, deps, params, provider, modelID, resolvedWorkspace, agentDir,
			lastProfileID, thinkLevel, ctxInfo, &overflowAttempts, &autoCompactionCount, started,
		)
		if overflowResult != nil {
			return overflowResult, nil
		}
		if shouldRetry {
			continue // auto-compaction succeeded, retry
		}

		// 6b. Prompt 错误处理
		if attempt.PromptError != nil && !attempt.Aborted {
			errText := DescribeUnknownError(attempt.PromptError)

			// 角色排序错误
			if isRoleOrderingError(errText) {
				return buildErrorResult(durationMs, attempt, provider, model.ID, "role_ordering", errText), nil
			}
			// 图片大小错误
			if ParseImageSizeError(errText) != nil {
				return buildErrorResult(durationMs, attempt, provider, model.ID, "image_size", errText), nil
			}
			// Failover 处理
			failoverReason := models.ClassifyFailoverReason(errText)
			if failoverReason != "" && failoverReason != "timeout" && lastProfileID != "" && deps.AuthStore != nil {
				_ = deps.AuthStore.MarkFailure(lastProfileID, string(failoverReason), params.Config, agentDir)
			}
			if helpers.IsFailoverErrorMessage(errText) && failoverReason != "timeout" {
				if advanceProfile(deps.AuthStore, model, params, profileCandidates, lockedProfileID, &profileIndex, &lastProfileID, initialThinkLevel, &thinkLevel, attemptedThinking) {
					continue
				}
			}
			// 思考级别降级
			fallback := PickFallbackThinkingLevel(errText, attemptedThinking)
			if fallback != "" {
				log.Warn("thinking level fallback", "level", fallback)
				thinkLevel = fallback
				continue
			}
			if fallbackConfigured && helpers.IsFailoverErrorMessage(errText) {
				reason := failoverReason
				if reason == "" {
					reason = "unknown"
				}
				return nil, &models.FailoverError{
					Message:  errText,
					Reason:   reason,
					Provider: provider,
					Model:    modelID,
					Status:   models.ResolveFailoverStatus(reason),
				}
			}
			return nil, attempt.PromptError
		}

		// 6c. 思考级别降级（assistantError）
		if msg := attempt.LastAssistant; msg != nil && !attempt.Aborted {
			fallback := PickFallbackThinkingLevel(msg.ErrorMessage, attemptedThinking)
			if fallback != "" {
				thinkLevel = fallback
				continue
			}
		}

		// 6d. Auth profile 轮换
		shouldRotate := (!attempt.Aborted && isFailoverAssistant(attempt.LastAssistant)) || attempt.TimedOut
		if shouldRotate {
			if lastProfileID != "" && deps.AuthStore != nil {
				reason := "unknown"
				if attempt.TimedOut {
					reason = "timeout"
				}
				_ = deps.AuthStore.MarkFailure(lastProfileID, reason, params.Config, agentDir)
			}
			if advanceProfile(deps.AuthStore, model, params, profileCandidates, lockedProfileID, &profileIndex, &lastProfileID, initialThinkLevel, &thinkLevel, attemptedThinking) {
				continue
			}
			if fallbackConfigured {
				msg := "LLM request failed."
				if attempt.TimedOut {
					msg = "LLM request timed out."
				}
				return nil, &models.FailoverError{
					Message:  msg,
					Reason:   "unknown",
					Provider: provider,
					Model:    modelID,
				}
			}
		}

		// 6e. 成功 — 标记 profile good
		if lastProfileID != "" && deps.AuthStore != nil {
			_ = deps.AuthStore.MarkGood(provider, lastProfileID, agentDir)
			_ = deps.AuthStore.MarkUsed(lastProfileID, agentDir)
		}

		// 6f. 通知子智能体频道：轮次完成
		if params.OnCoderEvent != nil {
			var replyText string
			for i := len(attempt.AssistantTexts) - 1; i >= 0; i-- {
				if attempt.AssistantTexts[i] != "" {
					replyText = attempt.AssistantTexts[i]
					break
				}
			}
			if replyText != "" {
				params.OnCoderEvent("turn_complete", map[string]interface{}{
					"text": replyText,
				})
			}
		}

		// 6g. 构建结果
		normUsage := usageAcc.ToNormalizedUsage()
		var agentUsage *EmbeddedPiAgentUsage
		if normUsage != nil {
			agentUsage = normalizedToAgentUsage(normUsage)
		}
		agentMeta := &EmbeddedPiAgentMeta{
			SessionID: attempt.SessionIDUsed,
			Provider:  provider,
			Model:     model.ID,
			Usage:     agentUsage,
		}
		if autoCompactionCount > 0 {
			agentMeta.CompactionCount = autoCompactionCount
		}

		return &EmbeddedPiRunResult{
			Payloads: buildPayloads(attempt),
			Meta: EmbeddedPiRunMeta{
				DurationMs: durationMs,
				AgentMeta:  agentMeta,
				Aborted:    attempt.Aborted,
			},
			DidSendViaMessagingTool:  attempt.DidSendViaMessaging,
			MessagingToolSentTargets: attempt.MessagingSentTargets,
		}, nil
	}
}

// --- 内部辅助 ---

func resolveProfileCandidates(store AuthProfileStore, provider string, params RunEmbeddedPiAgentParams) ([]string, string) {
	if store == nil {
		return []string{""}, ""
	}
	lockedID := ""
	if params.AuthProfileIDSource == "user" && params.AuthProfileID != "" {
		lockedID = params.AuthProfileID
	}
	order := store.ResolveProfileOrder(params.Config, provider, params.AuthProfileID)
	if lockedID != "" {
		return []string{lockedID}, lockedID
	}
	if len(order) > 0 {
		return order, ""
	}
	return []string{""}, ""
}

func applyInitialProfile(store AuthProfileStore, model *ResolvedModel, params RunEmbeddedPiAgentParams, candidates []string, lockedID string, idx *int, lastID *string) error {
	if store == nil {
		return nil
	}
	for *idx < len(candidates) {
		c := candidates[*idx]
		if c != "" && c != lockedID && store.IsInCooldown(c) {
			*idx++
			continue
		}
		info, err := store.GetApiKeyForModel(model, params.Config, c, params.AgentDir)
		if err != nil {
			return err
		}
		*lastID = info.ProfileID
		return nil
	}
	return fmt.Errorf("all auth profiles in cooldown for %s", model.Provider)
}

func advanceProfile(store AuthProfileStore, model *ResolvedModel, params RunEmbeddedPiAgentParams, candidates []string, lockedID string, idx *int, lastID *string, initialThink string, thinkLevel *string, attempted map[string]bool) bool {
	if store == nil || lockedID != "" {
		return false
	}
	next := *idx + 1
	for next < len(candidates) {
		c := candidates[next]
		if c != "" && store.IsInCooldown(c) {
			next++
			continue
		}
		info, err := store.GetApiKeyForModel(model, params.Config, c, params.AgentDir)
		if err != nil {
			next++
			continue
		}
		*idx = next
		*lastID = info.ProfileID
		*thinkLevel = initialThink
		for k := range attempted {
			delete(attempted, k)
		}
		return true
	}
	return false
}

func buildAuthFailoverError(provider, modelID string, fallbackConfigured, allInCooldown bool, cause error) error {
	msg := "No available auth profile."
	if cause != nil {
		msg = cause.Error()
	}
	reason := models.ClassifyFailoverReason(msg)
	if allInCooldown {
		reason = models.FailoverRateLimit
	}
	if reason == "" {
		reason = models.FailoverAuth
	}
	if fallbackConfigured {
		return &models.FailoverError{
			Message:  msg,
			Reason:   reason,
			Provider: provider,
			Model:    modelID,
			Status:   models.ResolveFailoverStatus(reason),
			Cause:    cause,
		}
	}
	if cause != nil {
		return cause
	}
	return fmt.Errorf("%s", msg)
}

func isContextOverflow(attempt *AttemptResult) bool {
	if attempt == nil || attempt.Aborted {
		return false
	}
	if attempt.PromptError != nil {
		return helpers.IsContextOverflowError(DescribeUnknownError(attempt.PromptError))
	}
	if attempt.LastAssistant != nil && attempt.LastAssistant.StopReason == "error" {
		return helpers.IsContextOverflowError(attempt.LastAssistant.ErrorMessage)
	}
	return false
}

func handleContextOverflow(
	attempt *AttemptResult, deps EmbeddedRunDeps, params RunEmbeddedPiAgentParams,
	provider, modelID, workspace, agentDir, lastProfileID, thinkLevel string,
	ctxInfo ContextWindowInfo, overflowAttempts *int, autoCompactionCount *int,
	started time.Time,
) (result *EmbeddedPiRunResult, shouldRetry bool) {
	if !isContextOverflow(attempt) {
		return nil, false
	}
	errText := ""
	if attempt.PromptError != nil {
		errText = DescribeUnknownError(attempt.PromptError)
	} else if attempt.LastAssistant != nil {
		errText = attempt.LastAssistant.ErrorMessage
	}

	if *overflowAttempts < MaxOverflowCompactions && deps.CompactionRunner != nil {
		*overflowAttempts++
		compactResult, err := deps.CompactionRunner.CompactSession(context.Background(), CompactionParams{
			SessionID:     params.SessionID,
			SessionKey:    params.SessionKey,
			SessionFile:   params.SessionFile,
			WorkspaceDir:  workspace,
			AgentDir:      agentDir,
			Provider:      provider,
			Model:         modelID,
			AuthProfileID: lastProfileID,
		})
		if err == nil && compactResult.Compacted {
			*autoCompactionCount++
			return nil, true // signal to continue
		}
	}

	return &EmbeddedPiRunResult{
		Payloads: []RunPayload{{
			Text:    "Context overflow: prompt too large for the model. Try again with less input or a larger-context model.",
			IsError: true,
		}},
		Meta: EmbeddedPiRunMeta{
			DurationMs: time.Since(started).Milliseconds(),
			AgentMeta: &EmbeddedPiAgentMeta{
				SessionID: attempt.SessionIDUsed,
				Provider:  provider,
				Model:     modelID,
			},
			Error: &EmbeddedPiRunError{Kind: "context_overflow", Message: errText},
		},
	}, false
}

func isRoleOrderingError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "incorrect role") ||
		strings.Contains(lower, "roles must alternate")
}

func normalizedToAgentUsage(n *NormalizedUsage) *EmbeddedPiAgentUsage {
	if n == nil {
		return nil
	}
	u := &EmbeddedPiAgentUsage{}
	if n.Input != nil {
		u.Input = *n.Input
	}
	if n.Output != nil {
		u.Output = *n.Output
	}
	if n.CacheRead != nil {
		u.CacheRead = *n.CacheRead
	}
	if n.CacheWrite != nil {
		u.CacheWrite = *n.CacheWrite
	}
	if n.Total != nil {
		u.Total = *n.Total
	}
	return u
}

func isFailoverAssistant(msg *AssistantMessage) bool {
	if msg == nil {
		return false
	}
	return helpers.IsFailoverErrorMessage(msg.ErrorMessage)
}

func buildErrorResult(durationMs int64, attempt *AttemptResult, provider, modelID, kind, errText string) *EmbeddedPiRunResult {
	return &EmbeddedPiRunResult{
		Payloads: []RunPayload{{Text: errText, IsError: true}},
		Meta: EmbeddedPiRunMeta{
			DurationMs: durationMs,
			AgentMeta: &EmbeddedPiAgentMeta{
				SessionID: attempt.SessionIDUsed,
				Provider:  provider,
				Model:     modelID,
			},
			Error: &EmbeddedPiRunError{Kind: kind, Message: errText},
		},
	}
}

func buildPayloads(attempt *AttemptResult) []RunPayload {
	if attempt == nil {
		return nil
	}
	var payloads []RunPayload
	for _, t := range attempt.AssistantTexts {
		if t != "" {
			payloads = append(payloads, RunPayload{Text: t})
		}
	}

	// 媒体-only 响应：没有文本时仍保留一个 payload，避免媒体被直接丢弃。
	if len(payloads) == 0 && len(attempt.MediaBlocks) > 0 {
		payloads = append(payloads, RunPayload{})
	}

	// 将全部媒体附加到第一个 payload，并保留旧字段兼容（仍使用最后一项）。
	if len(attempt.MediaBlocks) > 0 && len(payloads) > 0 {
		items := make([]MediaBlock, 0, len(attempt.MediaBlocks))
		for _, block := range attempt.MediaBlocks {
			if block.Base64 == "" {
				continue
			}
			items = append(items, MediaBlock{
				MimeType: block.MimeType,
				Base64:   block.Base64,
			})
		}
		if len(items) > 0 {
			payloads[0].MediaItems = items
			last := items[len(items)-1]
			payloads[0].MediaBase64 = last.Base64
			payloads[0].MediaMimeType = last.MimeType
			slog.Info("buildPayloads: media attached to payload",
				"count", len(items),
				"lastMimeType", last.MimeType,
				"lastBase64Len", len(last.Base64),
			)
		}
	}

	if len(payloads) == 0 {
		return nil
	}
	return payloads
}
