package gateway

// server_methods_chat.go — chat.send, chat.abort, chat.history, chat.inject
// 对应 TS src/gateway/server-methods/chat.ts
//
// chat.send 是核心聊天管线入口：
//   消息 → 附件解析 → session 解析 → agent command 分发
//
// 当前实现策略:
//   - chat.history → 从 SessionStore 读取 transcript
//   - chat.abort   → 通过 ChatRunState 标记中断
//   - chat.send    → 参数解析 + session resolve + agent command 分发
//   - chat.inject  → transcript 追加 assistant 消息

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/internal/media"
	sessiontypes "github.com/Acosmi/ClawAcosmi/internal/session"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ChatHandlers 返回 chat.* 方法处理器映射。
func ChatHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"chat.history":     handleChatHistory,
		"chat.abort":       handleChatAbort,
		"chat.send":        handleChatSend,
		"chat.inject":      handleChatInject,
		"chat.active_runs": handleChatActiveRuns,
	}
}

// ---------- chat.active_runs ----------
// F4: 查询指定 session 是否有活跃的聊天运行。
// 供前端 WS 重连时判断是否应保留 chatRunId（避免误清正在执行的运行）。

func handleChatActiveRuns(ctx *MethodHandlerContext) {
	sessionKey, _ := ctx.Params["sessionKey"].(string)
	if sessionKey == "" {
		sessionKey, _ = ctx.Params["sessionId"].(string)
	}

	chatState := ctx.Context.ChatState
	if chatState == nil || chatState.Registry == nil {
		ctx.Respond(true, map[string]interface{}{
			"hasActiveRun": false,
		}, nil)
		return
	}

	entry := chatState.Registry.Peek(sessionKey)
	if entry != nil {
		ctx.Respond(true, map[string]interface{}{
			"hasActiveRun": true,
			"runId":        entry.ClientRunID,
			"sessionKey":   entry.SessionKey,
		}, nil)
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"hasActiveRun": false,
	}, nil)
}

// ---------- chat.history ----------
// 对应 TS chat.ts L30-L100
// 返回指定 session 的消息历史。

func handleChatHistory(ctx *MethodHandlerContext) {
	sessionKey, _ := ctx.Params["sessionId"].(string)
	if sessionKey == "" {
		sessionKey, _ = ctx.Params["sessionKey"].(string)
	}

	// 解析 limit (默认 50)
	limit := 50
	if v, ok := ctx.Params["limit"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			limit = int(f)
		}
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	// 获取 session
	if sessionKey == "" {
		// 使用主 session
		cfg := resolveConfigFromContext(ctx)
		if cfg != nil {
			sessionKey = scope.ResolveDefaultAgentId(cfg) + ":main"
		} else {
			sessionKey = "default:main"
		}
	}

	session := store.LoadSessionEntry(sessionKey)
	if session == nil {
		ctx.Respond(true, map[string]interface{}{
			"sessionKey": sessionKey,
			"messages":   []interface{}{},
			"total":      0,
		}, nil)
		return
	}

	// 从 transcript JSONL 文件读取消息
	storePath := ctx.Context.StorePath
	var messages []map[string]interface{}
	if session.SessionId != "" {
		rawMessages := ReadTranscriptMessages(session.SessionId, storePath, session.SessionFile)
		sanitized := StripEnvelopeFromMessages(rawMessages)

		// 按 limit 和字节限制裁剪
		hardMax := 1000
		defaultLimit := 200
		requested := limit
		if requested <= 0 {
			requested = defaultLimit
		}
		max := requested
		if max > hardMax {
			max = hardMax
		}
		if len(sanitized) > max {
			sanitized = sanitized[len(sanitized)-max:]
		}

		// 按 JSON 大小上限裁剪 (5MB)
		const maxChatHistoryBytes = 5 * 1024 * 1024
		messages = CapArrayByJSONBytes(sanitized, maxChatHistoryBytes)
	}
	if messages == nil {
		messages = []map[string]interface{}{}
	}

	ctx.Respond(true, map[string]interface{}{
		"sessionKey": sessionKey,
		"sessionId":  session.SessionId,
		"messages":   messages,
		"total":      len(messages),
		"title":      session.Label,
		"limit":      limit,
	}, nil)
}

// ---------- chat.abort ----------
// 对应 TS chat.ts L102-L133
// 中断指定 session 的运行中聊天。

func handleChatAbort(ctx *MethodHandlerContext) {
	sessionKey, _ := ctx.Params["sessionId"].(string)
	if sessionKey == "" {
		sessionKey, _ = ctx.Params["sessionKey"].(string)
	}
	runId, _ := ctx.Params["runId"].(string)

	chatState := ctx.Context.ChatState
	if chatState == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "chat state not available"))
		return
	}

	// 标记为 aborted
	if runId != "" {
		chatState.AbortedRuns.Store(runId, time.Now().UnixMilli())
	}
	if sessionKey != "" && chatState.Registry != nil {
		entry := chatState.Registry.Shift(sessionKey)
		if entry != nil {
			if runId == "" {
				runId = entry.ClientRunID
			}
			slog.Info("chat.abort: aborted run", "sessionKey", sessionKey, "runId", runId)
		}
	}
	if runId != "" && ctx.Context.EscalationMgr != nil {
		ctx.Context.EscalationMgr.TaskComplete(runId)
	}

	// 广播 abort 事件
	if bc := ctx.Context.Broadcaster; bc != nil {
		bc.Broadcast("chat.abort", map[string]interface{}{
			"sessionKey": sessionKey,
			"runId":      runId,
			"ts":         time.Now().UnixMilli(),
		}, nil)
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":      true,
		"aborted": true,
	}, nil)
}

// ---------- chat.send ----------
// 对应 TS chat.ts L135-L695
// 核心聊天发送管线。
//
// 完整实现需依赖:
//   - dispatchInboundMessage (autoreply/reply/)
//   - session transcript read/write
//   - agent execution pipeline
// 当前为框架实现: 参数解析 + session resolve + broadcast 骨架。

func handleChatSend(ctx *MethodHandlerContext) {
	text, _ := ctx.Params["text"].(string)
	if text == "" {
		text, _ = ctx.Params["message"].(string) // 兼容前端 chat.ts 发送的 "message" 字段
	}
	sessionKey, _ := ctx.Params["sessionId"].(string)
	if sessionKey == "" {
		sessionKey, _ = ctx.Params["sessionKey"].(string)
	}
	agentId, _ := ctx.Params["agentId"].(string)
	idempotencyKey, _ := ctx.Params["idempotencyKey"].(string)

	// 解析 async 参数（Phase 1: 仅解析，Phase 2 实现异步路由）
	asyncMode, _ := ctx.Params["async"].(bool)

	// 自动异步检测：消息内容暗示复杂多步操作时自动启用
	if !asyncMode && text != "" {
		if shouldAutoAsync(text) {
			asyncMode = true
			slog.Info("chat.send: auto-async activated",
				"textLen", len([]rune(text)),
				"textPreview", truncateStr(text, 60),
			)
		}
	}

	// 解析 attachments
	var attachments []map[string]interface{}
	if v, ok := ctx.Params["attachments"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					attachments = append(attachments, m)
				}
			}
		}
	}

	// 解析 session / agent
	cfg := resolveConfigFromContext(ctx)
	if agentId == "" && cfg != nil {
		agentId = scope.ResolveDefaultAgentId(cfg)
	}
	if sessionKey == "" {
		sessionKey = agentId + ":main"
	}

	// 幂等检查
	if idempotencyKey != "" && ctx.Context.IdempotencyCache != nil {
		check := ctx.Context.IdempotencyCache.CheckOrRegister(idempotencyKey)
		if check.IsDuplicate {
			if check.State == IdempotencyCompleted {
				ctx.Respond(true, check.CachedResult, nil)
				return
			}
			// InFlight — 正在处理中
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "duplicate request in flight"))
			return
		}
	}

	chatState := ctx.Context.ChatState
	if chatState == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "chat state not available"))
		return
	}

	// Phase 2: async=true 并发守卫 — 在生成 runId 之前拦截
	if asyncMode {
		if !chatState.TryAcquireAsync() {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeTooManyRequests,
				fmt.Sprintf("too many async tasks running (max %d)", MaxAsyncTasks)))
			return
		}
		// 注意: Release 在 goroutine 的 defer 中执行
	}

	// 生成 runId — 优先使用客户端的 idempotencyKey，确保事件 runId 与 UI 的 chatRunId 匹配
	runId := idempotencyKey
	if runId == "" {
		runId = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}

	// 注册运行条目
	chatState.Registry.Add(sessionKey, ChatRunEntry{
		SessionKey:  sessionKey,
		ClientRunID: runId,
	})

	slog.Info("chat.send: dispatching",
		"sessionKey", sessionKey,
		"agentId", agentId,
		"text", truncateStr(text, 80),
		"attachments", len(attachments),
		"runId", runId,
		"async", asyncMode,
	)

	// Phase 2: async=true 时广播 task.queued（在 ACK 之前广播，确保前端先收到排队事件）
	if asyncMode {
		if bc := ctx.Context.Broadcaster; bc != nil {
			bc.Broadcast(EventTaskQueued, TaskQueuedEvent{
				TaskID:     runId,
				SessionKey: sessionKey,
				Text:       truncateStr(text, 120),
				Ts:         time.Now().UnixMilli(),
				Async:      true,
			}, &BroadcastOptions{DropIfSlow: true})
		}
		// 持久化 task 到独立 TaskStore（P1 域隔离）
		if ts := ctx.Context.TaskStore; ts != nil {
			now := time.Now().UnixMilli()
			ts.Save(&TaskEntry{
				TaskID:     runId,
				SessionKey: sessionKey,
				Label:      truncateStr(text, 60),
				Status:     "queued",
				Async:      true,
				CreatedAt:  now,
				UpdatedAt:  now,
				TaskMeta: &sessiontypes.TaskMeta{
					Status: "queued",
					Async:  true,
				},
			})
		}
	}

	// 广播 chat 开始事件
	if bc := ctx.Context.Broadcaster; bc != nil {
		bc.Broadcast("chat.delta", map[string]interface{}{
			"sessionKey": sessionKey,
			"runId":      runId,
			"agentId":    agentId,
			"type":       "start",
			"ts":         time.Now().UnixMilli(),
		}, nil)
	}

	// 立即返回 ack（非阻塞）
	ackStatus := "started"
	if asyncMode {
		ackStatus = "queued"
	}
	ctx.Respond(true, map[string]interface{}{
		"runId":  runId,
		"status": ackStatus,
		"async":  asyncMode,
		"ts":     time.Now().UnixMilli(), // F5: ACK 时间戳
	}, nil)

	// 在 goroutine 中异步运行 autoreply 管线
	// TS 对照: chat.ts L520-614 dispatchInboundMessage 异步流
	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	broadcaster := ctx.Context.Broadcaster
	storePath := ctx.Context.StorePath
	dispatcher := ctx.Context.PipelineDispatcher

	go func() {
		defer pipelineCancel()
		defer func() {
			// 清理运行条目
			chatState.Registry.Remove(sessionKey, runId, sessionKey)
			if ctx.Context.EscalationMgr != nil {
				ctx.Context.EscalationMgr.TaskComplete(runId)
			}
		}()
		// Phase 2: async=true 时释放并发槽位
		if asyncMode {
			defer chatState.ReleaseAsync()
		}

		// 广播 task.started 事件（结构化看板事件）
		if broadcaster != nil {
			broadcaster.Broadcast(EventTaskStarted, TaskStartedEvent{
				TaskID:     runId,
				SessionKey: sessionKey,
				Ts:         time.Now().UnixMilli(),
			}, &BroadcastOptions{DropIfSlow: true})
		}
		// 更新 TaskStore status = "started"（P1 域隔离）
		if ts := ctx.Context.TaskStore; ts != nil {
			ts.UpdateStatus(runId, "started", func(e *TaskEntry) {
				if e.TaskMeta == nil {
					e.TaskMeta = &sessiontypes.TaskMeta{}
				}
				e.TaskMeta.Status = "started"
				e.TaskMeta.StartedAt = time.Now().UnixMilli()
			})
		}

		// 订阅全局事件总线 → 广播 agent 工具事件到 WebSocket
		if broadcaster != nil {
			unsubAgentEvents := infra.OnAgentEvent(func(evt infra.AgentEventPayload) {
				if evt.RunID != runId {
					return
				}
				broadcaster.Broadcast("agent", evt, &BroadcastOptions{DropIfSlow: true})
			})
			defer unsubAgentEvents()
		}

		// ---- 确保 session 存在 & 解析 sessionId ----
		var resolvedSessionId string
		{
			store := ctx.Context.SessionStore
			if store != nil {
				entry := store.LoadSessionEntry(sessionKey)
				if entry == nil {
					// P2 身份收敛: 前端应通过 sessions.create API 创建 session。
					// 过渡期保留 auto-create 兼容，但记录 warning 以便追踪遗漏。
					slog.Warn("chat.send: auto-creating session for unregistered key (should use sessions.create)",
						"sessionKey", sessionKey)
					newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
					entry = &SessionEntry{
						SessionKey: sessionKey,
						SessionId:  newId,
						Label:      sessionKey,
						CreatedAt:  time.Now().UnixMilli(),
					}
					store.Save(entry)
				}
				resolvedSessionId = entry.SessionId
			}
		}
		if resolvedSessionId == "" {
			resolvedSessionId = runId // 最后兜底
		}

		// 处理附件：音频→STT 转录，文档→DocConv 转换
		enhancedText, attachmentBlocks := processAttachmentsForChat(pipelineCtx, text, attachments, ctx.Context.ConfigLoader)

		// 附件-only 场景（无文字但有附件）：构建占位 prompt 防止管线空文本早退
		effectiveText := enhancedText
		if effectiveText == "" && len(attachmentBlocks) > 0 {
			effectiveText = "[用户发送了附件]"
		}

		// F1: 立即写入用户消息到 transcript，确保 chat.history 在 Agent 执行期间就能返回用户问话。
		// 使用 effectiveText（经 STT/DocConv 增强后的文本），与 persistToTranscript 的 params.Prompt 一致，
		// 保证 dedup 文本匹配（attempt_runner.go:2237-2248 读 transcript 尾部比对）。
		// ensureUserTranscriptOnError (L872) 也有同样的 dedup，三者兼容。
		if resolvedSessionId != "" && storePath != "" && strings.TrimSpace(effectiveText) != "" {
			result := AppendUserTranscriptMessage(AppendTranscriptParams{
				Message:         effectiveText,
				SessionID:       resolvedSessionId,
				StorePath:       storePath,
				CreateIfMissing: true,
			})
			if result != nil && !result.OK {
				slog.Warn("chat.send: early transcript write failed (non-blocking)",
					"error", result.Error, "sessionId", resolvedSessionId)
			}
		}

		// 构建 MsgContext
		msgCtx := &autoreply.MsgContext{
			Body:               effectiveText,
			BodyForAgent:       effectiveText,
			BodyForCommands:    effectiveText,
			RawBody:            text,
			CommandBody:        effectiveText,
			SessionID:          resolvedSessionId,
			SessionKey:         sessionKey,
			Provider:           "webchat",
			Surface:            "webchat",
			OriginatingChannel: "webchat",
			ChatType:           "direct",
			CommandAuthorized:  true,
			MessageSid:         runId,
			Attachments:        attachmentBlocks,
		}

		// 任务频道：懒创建 task 条目，仅在有工具调用时才创建（P1 域隔离：写入 TaskStore）
		taskCreated := false // 单 goroutine 内闭包访问，无需 sync
		// 注意: onToolResult 是历史死代码（autoreply pipeline 从未调用），
		// 已被下方 onToolEvent 替代。保留以维持 DispatchInboundParams 接口兼容。
		var onToolResult func(payload autoreply.ReplyPayload)

		// 任务频道结构化工具事件回调（看板持久化：仅广播 task.* WS 事件 + 更新 TaskMeta）
		var onToolEvent any
		if broadcaster != nil {
			onToolEvent = func(event runner.ToolEvent) {
				// 首次工具调用 → 懒创建任务条目（非 async 任务在 queued 时未创建）
				if !taskCreated {
					taskCreated = true
					if ts := ctx.Context.TaskStore; ts != nil {
						ts.SaveOrCreate(runId, func() *TaskEntry {
							now := time.Now().UnixMilli()
							return &TaskEntry{
								TaskID:     runId,
								SessionKey: sessionKey,
								Label:      truncateStr(text, 60),
								Status:     "started",
								CreatedAt:  now,
								UpdatedAt:  now,
								TaskMeta: &sessiontypes.TaskMeta{
									Status:    "started",
									StartedAt: now,
								},
							}
						}, func(e *TaskEntry) {
							// 已存在（async 路径），仅更新状态
							if e.TaskMeta == nil {
								e.TaskMeta = &sessiontypes.TaskMeta{}
							}
							if e.TaskMeta.Status == "queued" {
								e.TaskMeta.Status = "started"
								e.TaskMeta.StartedAt = time.Now().UnixMilli()
							}
							e.UpdatedAt = time.Now().UnixMilli()
						})
					}
				}
				// 广播结构化工具事件
				var toolText string
				switch event.Phase {
				case "start":
					toolText = fmt.Sprintf("[工具] %s: %s", event.ToolName, event.Args)
				case "end":
					if event.IsError {
						toolText = fmt.Sprintf("[错误] %s (%dms)", event.Result, event.Duration)
					} else {
						toolText = fmt.Sprintf("[结果] %s (%dms)", event.Result, event.Duration)
					}
				}

				progressText := truncateForLog(toolText, 300)
				now := time.Now().UnixMilli()

				// 广播 task.progress 结构化看板事件
				broadcaster.Broadcast(EventTaskProgress, TaskProgressEvent{
					TaskID:     runId,
					SessionKey: sessionKey,
					ToolName:   event.ToolName,
					ToolID:     event.ToolID,
					Phase:      event.Phase,
					Text:       progressText,
					IsError:    event.IsError,
					Duration:   event.Duration,
					Ts:         now,
				}, &BroadcastOptions{DropIfSlow: true})
				// 持久化最近一个工具步骤到 TaskStore
				if ts := ctx.Context.TaskStore; ts != nil {
					ts.UpdateStatus(runId, "progress", func(e *TaskEntry) {
						if e.TaskMeta == nil {
							e.TaskMeta = &sessiontypes.TaskMeta{}
						}
						e.TaskMeta.Status = "progress"
						e.TaskMeta.ToolName = event.ToolName
						e.TaskMeta.ProgressPhase = event.Phase
						e.TaskMeta.ProgressText = progressText
						e.TaskMeta.ProgressIsError = event.IsError
						e.TaskMeta.ProgressDuration = event.Duration
						e.TaskMeta.ProgressAt = now
						if e.TaskMeta.StartedAt == 0 {
							e.TaskMeta.StartedAt = now
						}
					})
				}
			}
		}

		// 调用管线
		result := DispatchInboundMessage(pipelineCtx, DispatchInboundParams{
			MsgCtx:       msgCtx,
			SessionKey:   sessionKey,
			AgentID:      agentId,
			StorePath:    storePath,
			RunID:        runId,
			Ctx:          pipelineCtx,
			Dispatcher:   dispatcher,
			OnToolResult: onToolResult,
			OnToolEvent:  onToolEvent,
			OnProgress:   buildChatProgressCallback(broadcaster, sessionKey),
		})

		if result.Error != nil {
			slog.Error("chat.send: pipeline error",
				"error", result.Error,
				"runId", runId,
				"sessionKey", sessionKey,
			)
			// 兜底: 如果管线在 RunAttempt 之前就失败，defer 不会执行，用户消息未持久化。
			// 此处确保用户消息至少写入 transcript，刷新后不会完全消失。
			ensureUserTranscriptOnError(resolvedSessionId, storePath, effectiveText, attachmentBlocks)
			// 广播 task.failed 结构化看板事件
			if broadcaster != nil {
				broadcaster.Broadcast(EventTaskFailed, TaskFailedEvent{
					TaskID:     runId,
					SessionKey: sessionKey,
					Error:      truncateForLog(result.Error.Error(), 200),
					Ts:         time.Now().UnixMilli(),
				}, &BroadcastOptions{DropIfSlow: true})
			}
			// 更新 TaskStore status = "failed"（P1 域隔离）
			if ts := ctx.Context.TaskStore; ts != nil {
				now := time.Now().UnixMilli()
				errMsg := truncateForLog(result.Error.Error(), 200)
				ts.SaveOrCreate(runId, func() *TaskEntry {
					return &TaskEntry{
						TaskID:     runId,
						SessionKey: sessionKey,
						Label:      truncateStr(text, 60),
						Status:     "failed",
						CreatedAt:  now,
						UpdatedAt:  now,
						TaskMeta: &sessiontypes.TaskMeta{
							Status:      "failed",
							Error:       errMsg,
							CompletedAt: now,
						},
					}
				}, func(e *TaskEntry) {
					e.Status = "failed"
					e.UpdatedAt = now
					if e.TaskMeta == nil {
						e.TaskMeta = &sessiontypes.TaskMeta{}
					}
					e.TaskMeta.Status = "failed"
					e.TaskMeta.Error = errMsg
					e.TaskMeta.CompletedAt = now
				})
			}
			// Phase 2: async=true 时注入错误通知到 webchat
			if asyncMode && broadcaster != nil {
				broadcaster.Broadcast("channel.message.incoming", map[string]interface{}{
					"sessionKey": sessionKey,
					"channel":    "webchat",
					"text":       fmt.Sprintf("[异步任务失败] %s: %s", truncateStr(text, 60), truncateForLog(result.Error.Error(), 100)),
					"from":       "system",
					"ts":         time.Now().UnixMilli(),
					"async":      true,
				}, nil)
			}
			// 广播错误
			if broadcaster != nil {
				broadcaster.Broadcast("chat", map[string]interface{}{
					"runId":        runId,
					"sessionKey":   sessionKey,
					"state":        "error",
					"errorMessage": result.Error.Error(),
				}, nil)
			}
			return
		}

		// 合并回复
		combinedReply := CombineReplyPayloads(result.Replies)
		mediaItems := ExtractMediaListFromReplies(result.Replies)

		// AI 回复 transcript 由 attempt_runner.persistToTranscript 写入，此处优先回读 canonical assistant 消息，
		// 保证 final/chat.history 使用同一条 transcript 锚点，避免 workflow 回绑错位。
		message := loadLatestSessionAssistantMessageMatching(
			ctx.Context.SessionStore,
			sessionKey,
			ctx.Context.StorePath,
			combinedReply,
			mediaItems,
		)
		if message == nil {
			if combinedReply != "" {
				now := time.Now().UnixMilli()
				// P5 Usage 收敛: fallback 消息不写虚假 usage 零值
				message = map[string]interface{}{
					"role": "assistant",
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": combinedReply},
					},
					"timestamp":  now,
					"stopReason": "stop",
				}
			} else if len(mediaItems) > 0 {
				// P5 Usage 收敛: fallback 消息不写虚假 usage 零值
				message = map[string]interface{}{
					"role":       "assistant",
					"content":    []interface{}{},
					"timestamp":  time.Now().UnixMilli(),
					"stopReason": "stop",
				}
			}
			if message != nil && len(mediaItems) > 0 {
				if content, ok := message["content"].([]interface{}); ok {
					for _, item := range mediaItems {
						if item.Base64Data == "" {
							continue
						}
						mime := item.MimeType
						if mime == "" {
							mime = "image/png"
						}
						content = append(content, map[string]interface{}{
							"type": "image",
							"source": map[string]interface{}{
								"type":       "base64",
								"data":       item.Base64Data,
								"media_type": mime,
							},
						})
					}
					message["content"] = content
				}
			}
		}

		// 广播 task.completed 结构化看板事件
		if broadcaster != nil {
			summary := truncateForLog(combinedReply, 200)
			broadcaster.Broadcast(EventTaskCompleted, TaskCompletedEvent{
				TaskID:     runId,
				SessionKey: sessionKey,
				Summary:    summary,
				Ts:         time.Now().UnixMilli(),
			}, &BroadcastOptions{DropIfSlow: true})
		}
		// 更新 TaskStore status = "completed"（P1 域隔离）
		if ts := ctx.Context.TaskStore; ts != nil {
			now := time.Now().UnixMilli()
			summary := truncateForLog(combinedReply, 200)
			ts.SaveOrCreate(runId, func() *TaskEntry {
				return &TaskEntry{
					TaskID:     runId,
					SessionKey: sessionKey,
					Label:      truncateStr(text, 60),
					Status:     "completed",
					Summary:    summary,
					CreatedAt:  now,
					UpdatedAt:  now,
					TaskMeta: &sessiontypes.TaskMeta{
						Status:      "completed",
						Summary:     summary,
						CompletedAt: now,
					},
				}
			}, func(e *TaskEntry) {
				e.Status = "completed"
				e.Summary = summary
				e.UpdatedAt = now
				if e.TaskMeta == nil {
					e.TaskMeta = &sessiontypes.TaskMeta{}
				}
				e.TaskMeta.Status = "completed"
				e.TaskMeta.Summary = summary
				e.TaskMeta.CompletedAt = now
			})
		}

		// Phase 2: async=true 时注入 webchat 通知（跨 session 通知用户异步任务已完成）
		if asyncMode && broadcaster != nil {
			notifyText := fmt.Sprintf("[异步任务完成] %s", truncateStr(text, 60))
			if combinedReply != "" {
				notifyText = fmt.Sprintf("[异步任务完成] %s\n结果: %s",
					truncateStr(text, 60), truncateForLog(combinedReply, 150))
			}
			// 1. 保留原有通知（向后兼容 — 看板、通知铃铛）
			broadcaster.Broadcast("channel.message.incoming", map[string]interface{}{
				"sessionKey": sessionKey,
				"channel":    "webchat",
				"text":       notifyText,
				"from":       "system",
				"ts":         time.Now().UnixMilli(),
				"async":      true,
			}, nil)

			// 2. 以 chat.message 形式回填到聊天区域，用户可看到完整异步结果
			if combinedReply != "" {
				broadcaster.Broadcast("chat.message", map[string]interface{}{
					"sessionKey":  sessionKey,
					"role":        "assistant",
					"text":        combinedReply,
					"channel":     "webchat",
					"ts":          time.Now().UnixMilli(),
					"asyncResult": true,
				}, nil)
			}
		}

		// 广播 final
		if broadcaster != nil {
			broadcaster.Broadcast("chat", map[string]interface{}{
				"runId":      runId,
				"sessionKey": sessionKey,
				"state":      "final",
				"message":    message,
			}, nil)
		}

		slog.Info("chat.send: complete",
			"runId", runId,
			"sessionKey", sessionKey,
			"replyLength", len(combinedReply),
			"async", asyncMode,
		)
	}()
}

// ---------- chat.inject ----------
// 对应 TS chat.ts (最后部分)
// 将 assistant 消息注入 transcript，不触发 agent。

func handleChatInject(ctx *MethodHandlerContext) {
	sessionKey, _ := ctx.Params["sessionId"].(string)
	if sessionKey == "" {
		sessionKey, _ = ctx.Params["sessionKey"].(string)
	}
	text, _ := ctx.Params["text"].(string)
	role, _ := ctx.Params["role"].(string)
	if role == "" {
		role = "assistant"
	}

	if text == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "text required"))
		return
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	if sessionKey == "" {
		cfg := resolveConfigFromContext(ctx)
		if cfg != nil {
			sessionKey = scope.ResolveDefaultAgentId(cfg) + ":main"
		} else {
			sessionKey = "default:main"
		}
	}

	slog.Info("chat.inject", "sessionKey", sessionKey, "role", role, "textLen", len(text))

	// 解析 label
	label, _ := ctx.Params["label"].(string)

	// 加载 session 获取 transcript 路径
	session := store.LoadSessionEntry(sessionKey)
	var sessionId, storePath, sessionFile string
	if session != nil {
		sessionId = session.SessionId
		sessionFile = session.SessionFile
	}
	storePath = ctx.Context.StorePath

	if sessionId == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "session not found"))
		return
	}

	// 追加到 session transcript
	appended := AppendAssistantTranscriptMessage(AppendTranscriptParams{
		Message:         text,
		Label:           label,
		SessionID:       sessionId,
		StorePath:       storePath,
		SessionFile:     sessionFile,
		CreateIfMissing: true,
	})

	if !appended.OK || appended.MessageID == "" {
		errMsg := "unknown error"
		if appended.Error != "" {
			errMsg = appended.Error
		}
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "failed to write transcript: "+errMsg))
		return
	}

	// 广播到 webchat 实现即时 UI 更新
	if bc := ctx.Context.Broadcaster; bc != nil {
		chatPayload := map[string]interface{}{
			"runId":      fmt.Sprintf("inject-%s", appended.MessageID),
			"sessionKey": sessionKey,
			"seq":        0,
			"state":      "final",
			"message":    appended.Message,
		}
		bc.Broadcast("chat", chatPayload, nil)
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":        true,
		"messageId": appended.MessageID,
	}, nil)
}

// ---------- 辅助函数 ----------

// ensureUserTranscriptOnError 在管线失败时兜底持久化用户消息。
// 调用场景: 管线在 RunAttempt 之前就出错（如指令解析失败），
// RunAttempt 内的 defer persistToTranscript 不会执行，用户消息未被持久化。
// 此函数检查 transcript 是否已包含该消息（避免与 RunAttempt 的 defer 双写），
// 若 transcript 尾部已有同文本 user 消息则跳过。
func ensureUserTranscriptOnError(sessionId, storePath, text string, attachments []session.ContentBlock) {
	if sessionId == "" || storePath == "" {
		return
	}
	if strings.TrimSpace(text) == "" && len(attachments) == 0 {
		return
	}
	mgr := session.NewSessionManager("")
	sessionFile := filepath.Join(filepath.Dir(storePath), sessionId+".jsonl")

	// 检查 transcript 尾部是否已有相同 user 消息（RunAttempt defer 可能已写入）
	existing, _ := mgr.LoadSessionMessages(sessionId, sessionFile)
	if len(existing) > 0 {
		last := existing[len(existing)-1]
		if role, _ := last["role"].(string); role == "user" {
			// 仅当尾部 user 消息文本与当前消息一致时才跳过，
			// 避免前一轮对话的 user 消息导致当前消息被错误跳过
			lastText := extractTranscriptMessageText(last)
			if lastText == text {
				return
			}
		}
	}

	if _, err := mgr.EnsureSessionFile(sessionId, sessionFile); err != nil {
		slog.Debug("ensureUserTranscriptOnError: ensure file failed", "error", err)
		return
	}
	content := []session.ContentBlock{}
	if strings.TrimSpace(text) != "" {
		content = append(content, session.TextBlock(text))
	}
	if len(attachments) > 0 {
		content = append(content, attachments...)
	}
	entry := session.TranscriptEntry{
		Role:      "user",
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}
	if err := mgr.AppendMessage(sessionId, sessionFile, entry); err != nil {
		slog.Warn("ensureUserTranscriptOnError: append failed", "error", err)
	}
}

// extractTranscriptMessageText 从 transcript 消息条目中提取纯文本。
// 支持两种格式: 顶层 "text" 字段，或 content 数组中的 text block。
func extractTranscriptMessageText(msg map[string]interface{}) string {
	if t, ok := msg["text"].(string); ok && t != "" {
		return t
	}
	if content, ok := msg["content"].([]interface{}); ok {
		for _, block := range content {
			if m, ok := block.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						return t
					}
				}
			}
		}
	}
	return ""
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

const maxInlineTextDocumentChars = 12000

func isInlineTextDocument(fileName, mimeType string) bool {
	switch media.FormatCategory(fileName) {
	case "text", "code", "web":
		return true
	}
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	switch mime {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml":
		return true
	}
	return strings.HasPrefix(mime, "text/")
}

func inlineTextDocumentLanguage(fileName, mimeType string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".csv":
		return "csv"
	case ".txt":
		return "text"
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp":
		return "cpp"
	case ".h":
		return "c"
	case ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".sh":
		return "bash"
	case ".sql":
		return "sql"
	case ".css":
		return "css"
	case ".html", ".htm":
		return "html"
	}
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	switch mime {
	case "application/json":
		return "json"
	case "application/xml":
		return "xml"
	case "application/yaml", "application/x-yaml":
		return "yaml"
	}
	if strings.HasPrefix(mime, "text/") {
		return "text"
	}
	return ""
}

func buildInlineTextDocumentPrompt(fileName, mimeType string, data []byte) string {
	content := strings.TrimSpace(strings.ToValidUTF8(string(data), "?"))
	if content == "" {
		return fmt.Sprintf("[文件: %s, 内容为空]", fileName)
	}

	truncated := false
	if len([]rune(content)) > maxInlineTextDocumentChars {
		content = truncateStr(content, maxInlineTextDocumentChars)
		truncated = true
	}

	lang := inlineTextDocumentLanguage(fileName, mimeType)
	if lang == "" {
		lang = "text"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("[文件: %s]\n````%s\n%s\n````", fileName, lang, content))
	if truncated {
		b.WriteString("\n[文件内容已截断]")
	}
	return b.String()
}

type chatAttachmentProviderSnapshot struct {
	sttProvider media.STTProvider
	sttInitErr  error

	docConverter media.DocConverter
	docInitErr   error
}

type chatAttachmentProviderCache struct {
	mu sync.Mutex

	ttl       time.Duration
	expiresAt time.Time

	sttConfigSig string
	docConfigSig string

	sttProvider media.STTProvider
	sttInitErr  error

	docConverter media.DocConverter
	docInitErr   error

	newSTTProvider  func(cfg *types.STTConfig) (media.STTProvider, error)
	newDocConverter func(cfg *types.DocConvConfig) (media.DocConverter, error)
}

func newChatAttachmentProviderCache(ttl time.Duration) *chatAttachmentProviderCache {
	if ttl <= 0 {
		ttl = 20 * time.Second
	}
	return &chatAttachmentProviderCache{
		ttl:             ttl,
		newSTTProvider:  media.NewSTTProvider,
		newDocConverter: media.NewDocConverter,
	}
}

var defaultChatAttachmentProviderCache = newChatAttachmentProviderCache(20 * time.Second)

func (c *chatAttachmentProviderCache) Resolve(cfg *types.OpenAcosmiConfig) chatAttachmentProviderSnapshot {
	if c == nil || cfg == nil {
		return chatAttachmentProviderSnapshot{}
	}

	sttSig := chatAttachmentConfigSignature(cfg.STT)
	docSig := chatAttachmentConfigSignature(cfg.DocConv)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if now.Before(c.expiresAt) && sttSig == c.sttConfigSig && docSig == c.docConfigSig {
		return chatAttachmentProviderSnapshot{
			sttProvider:  c.sttProvider,
			sttInitErr:   c.sttInitErr,
			docConverter: c.docConverter,
			docInitErr:   c.docInitErr,
		}
	}

	c.sttConfigSig = sttSig
	c.docConfigSig = docSig
	c.expiresAt = now.Add(c.ttl)

	c.sttProvider = nil
	c.sttInitErr = nil
	if cfg.STT != nil && strings.TrimSpace(cfg.STT.Provider) != "" {
		provider, err := c.newSTTProvider(cfg.STT)
		if err != nil {
			c.sttInitErr = err
			slog.Warn("chat.send: STT provider init failed (cached)", "error", err)
		} else {
			c.sttProvider = provider
		}
	}

	c.docConverter = nil
	c.docInitErr = nil
	if cfg.DocConv != nil && strings.TrimSpace(cfg.DocConv.Provider) != "" {
		converter, err := c.newDocConverter(cfg.DocConv)
		if err != nil {
			c.docInitErr = err
			slog.Warn("chat.send: DocConv provider init failed (cached)", "error", err)
		} else {
			c.docConverter = converter
		}
	}

	return chatAttachmentProviderSnapshot{
		sttProvider:  c.sttProvider,
		sttInitErr:   c.sttInitErr,
		docConverter: c.docConverter,
		docInitErr:   c.docInitErr,
	}
}

func chatAttachmentConfigSignature(v interface{}) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%T", v)
	}
	return string(data)
}

// processAttachmentsForChat 处理 chat.send 附件：音频→STT，文档→DocConv。
// 返回增强文本（用于 LLM prompt）和附件 content blocks（用于 transcript 持久化）。
func processAttachmentsForChat(ctx context.Context, text string, attachments []map[string]interface{}, cfgLoader interface {
	LoadConfig() (*types.OpenAcosmiConfig, error)
}) (string, []session.ContentBlock) {
	return processAttachmentsForChatWithCache(ctx, text, attachments, cfgLoader, defaultChatAttachmentProviderCache)
}

func processAttachmentsForChatWithCache(
	ctx context.Context,
	text string,
	attachments []map[string]interface{},
	cfgLoader interface {
		LoadConfig() (*types.OpenAcosmiConfig, error)
	},
	providerCache *chatAttachmentProviderCache,
) (string, []session.ContentBlock) {
	if len(attachments) == 0 {
		return text, nil
	}
	if providerCache == nil {
		providerCache = defaultChatAttachmentProviderCache
	}

	var cfg *types.OpenAcosmiConfig
	if cfgLoader != nil {
		loadedCfg, err := cfgLoader.LoadConfig()
		if err != nil {
			slog.Warn("chat.send: attachment config unavailable", "error", err)
		} else {
			cfg = loadedCfg
		}
	}
	var providerSnapshot chatAttachmentProviderSnapshot
	if cfg != nil {
		providerSnapshot = providerCache.Resolve(cfg)
	}

	var parts []string
	if text != "" {
		parts = append(parts, text)
	}

	var blocks []session.ContentBlock

	for _, att := range attachments {
		attType, _ := att["type"].(string)
		contentB64, _ := att["content"].(string)
		mimeType, _ := att["mimeType"].(string)
		fileName, _ := att["fileName"].(string)
		fileSize, _ := att["fileSize"].(float64) // JSON numbers are float64

		if contentB64 == "" {
			continue
		}

		maxAttachmentBytes := channels.MaxChatAttachmentBytesForType(attType)
		maxBase64Len := maxAttachmentBytes*4/3 + 4
		if len(contentB64) > maxBase64Len {
			parts = append(parts, "[附件: 数据过大]")
			continue
		}

		switch attType {
		case "image":
			blocks = append(blocks, session.ContentBlock{
				Type:     "image",
				FileName: fileName,
				FileSize: int64(fileSize),
				MimeType: mimeType,
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: mimeType,
					Data:      contentB64,
				},
			})

		case "video":
			blocks = append(blocks, session.ContentBlock{
				Type:     "video",
				FileName: fileName,
				FileSize: int64(fileSize),
				MimeType: mimeType,
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: mimeType,
					Data:      contentB64,
				},
			})

		case "audio":
			if mimeType == "" {
				mimeType = "audio/webm"
			}
			// Always build the audio content block for transcript persistence
			blocks = append(blocks, session.ContentBlock{
				Type:     "audio",
				FileName: fileName,
				FileSize: int64(fileSize),
				MimeType: mimeType,
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: mimeType,
					Data:      contentB64,
				},
			})
			// STT text enhancement for LLM prompt
			if cfg == nil {
				parts = append(parts, "[语音附件: 配置不可用，未执行语音转录]")
				continue
			}
			if cfg.STT == nil || cfg.STT.Provider == "" {
				parts = append(parts, "[语音附件: 语音转文字(STT)未配置，请前往 设置→Speech to Text 配置语音识别服务]")
				continue
			}
			if providerSnapshot.sttProvider == nil {
				parts = append(parts, "[语音附件: STT 初始化失败]")
				if providerSnapshot.sttInitErr != nil {
					slog.Warn("chat.send: STT provider unavailable",
						"provider", cfg.STT.Provider, "error", providerSnapshot.sttInitErr)
				}
				continue
			}
			data, decErr := base64.StdEncoding.DecodeString(contentB64)
			if decErr != nil {
				parts = append(parts, "[语音附件: 解码失败]")
				continue
			}
			if len(data) > maxAttachmentBytes {
				parts = append(parts, "[语音附件: 数据过大]")
				continue
			}
			sttCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			transcript, sttErr := providerSnapshot.sttProvider.Transcribe(sttCtx, data, mimeType)
			cancel()
			if sttErr != nil {
				slog.Error("chat.send: STT failed", "error", sttErr)
				parts = append(parts, "[语音转录失败]")
			} else if strings.TrimSpace(transcript) == "" {
				parts = append(parts, "[语音附件: 转录结果为空]")
			} else {
				parts = append(parts, fmt.Sprintf("[语音转录]: %s", transcript))
			}

		case "document":
			if fileName == "" {
				fileName = "untitled"
			}
			// Always build the document content block for transcript persistence (metadata only, no raw data)
			blocks = append(blocks, session.ContentBlock{
				Type:     "document",
				FileName: fileName,
				FileSize: int64(fileSize),
				MimeType: mimeType,
			})
			data, decErr := base64.StdEncoding.DecodeString(contentB64)
			if decErr != nil {
				parts = append(parts, fmt.Sprintf("[文件: %s, 解码失败]", fileName))
				continue
			}
			if len(data) > maxAttachmentBytes {
				parts = append(parts, fmt.Sprintf("[文件: %s, 数据过大]", fileName))
				continue
			}
			if isInlineTextDocument(fileName, mimeType) {
				parts = append(parts, buildInlineTextDocumentPrompt(fileName, mimeType, data))
				continue
			}
			// DocConv text enhancement for non-text documents
			if cfg == nil {
				parts = append(parts, fmt.Sprintf("[文件: %s]", fileName))
				continue
			}
			if cfg.DocConv == nil || cfg.DocConv.Provider == "" {
				parts = append(parts, fmt.Sprintf("[文件: %s]", fileName))
				continue
			}
			if !media.IsSupportedFormat(fileName) {
				parts = append(parts, fmt.Sprintf("[文件: %s, 格式不支持转换]", fileName))
				continue
			}
			if providerSnapshot.docConverter == nil {
				parts = append(parts, fmt.Sprintf("[文件: %s, 转换器初始化失败]", fileName))
				if providerSnapshot.docInitErr != nil {
					slog.Warn("chat.send: DocConv provider unavailable",
						"provider", cfg.DocConv.Provider, "file", fileName, "error", providerSnapshot.docInitErr)
				}
				continue
			}
			convCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			markdown, convErr2 := providerSnapshot.docConverter.Convert(convCtx, data, mimeType, fileName)
			cancel()
			if convErr2 != nil {
				slog.Error("chat.send: DocConv failed", "file", fileName, "error", convErr2)
				parts = append(parts, fmt.Sprintf("[文件: %s, 转换失败]", fileName))
			} else {
				parts = append(parts, fmt.Sprintf("[文件: %s]\n%s", fileName, markdown))
			}
		}
	}

	enhancedText := text
	if len(parts) > 0 {
		enhancedText = strings.Join(parts, "\n")
	}
	return enhancedText, blocks
}
