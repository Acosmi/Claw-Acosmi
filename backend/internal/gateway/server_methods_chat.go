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
		"chat.history": handleChatHistory,
		"chat.abort":   handleChatAbort,
		"chat.send":    handleChatSend,
		"chat.inject":  handleChatInject,
	}
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
			slog.Info("chat.abort: aborted run", "sessionKey", sessionKey, "runId", runId)
		}
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
		// 持久化 task session + TaskMeta（看板持久化）
		if store := ctx.Context.SessionStore; store != nil {
			now := time.Now().UnixMilli()
			store.Save(&SessionEntry{
				SessionKey: fmt.Sprintf("task:%s", runId),
				SessionId:  runId,
				Label:      truncateStr(text, 60),
				Channel:    "task",
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
		// 更新 TaskMeta.Status = "started"（看板持久化）
		if store := ctx.Context.SessionStore; store != nil {
			taskKey := fmt.Sprintf("task:%s", runId)
			if entry := store.LoadSessionEntry(taskKey); entry != nil {
				if entry.TaskMeta == nil {
					entry.TaskMeta = &sessiontypes.TaskMeta{}
				}
				entry.TaskMeta.Status = "started"
				entry.TaskMeta.StartedAt = time.Now().UnixMilli()
				entry.UpdatedAt = time.Now().UnixMilli()
				store.Save(entry)
			}
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
					// 首次对话 — 自动创建 session
					newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
					entry = &SessionEntry{
						SessionKey: sessionKey,
						SessionId:  newId,
						Label:      sessionKey,
					}
					store.Save(entry)
					slog.Info("chat.send: auto-created session", "sessionKey", sessionKey, "sessionId", newId)
				}
				resolvedSessionId = entry.SessionId
			}
		}
		if resolvedSessionId == "" {
			resolvedSessionId = runId // 最后兜底
		}

		// 用户消息 transcript 由 attempt_runner.persistToTranscript 写入（正常路径 + defer 失败路径）。
		// 但如果管线在 RunAttempt 之前就失败（指令解析错误等），defer 不会执行。
		// 下方 result.Error 分支用 ensureUserTranscriptOnError 做兜底，确保用户消息不丢失。

		// 处理附件：音频→STT 转录，文档→DocConv 转换
		enhancedText := processAttachmentsForChat(pipelineCtx, text, attachments, ctx.Context.ConfigLoader)

		// 构建 MsgContext
		msgCtx := &autoreply.MsgContext{
			Body:               enhancedText,
			BodyForAgent:       enhancedText,
			BodyForCommands:    enhancedText,
			RawBody:            text,
			CommandBody:        enhancedText,
			SessionID:          resolvedSessionId,
			SessionKey:         sessionKey,
			Provider:           "webchat",
			Surface:            "webchat",
			OriginatingChannel: "webchat",
			ChatType:           "direct",
			CommandAuthorized:  true,
			MessageSid:         runId,
		}

		// 任务频道：懒创建 task:<runID> session，仅在有工具调用时才创建
		taskSessionKey := fmt.Sprintf("task:%s", runId)
		taskSessionCreated := false // 单 goroutine 内闭包访问，无需 sync
		// 注意: onToolResult 是历史死代码（autoreply pipeline 从未调用），
		// 已被下方 onToolEvent 替代。保留以维持 DispatchInboundParams 接口兼容。
		var onToolResult func(payload autoreply.ReplyPayload)

		// 任务频道结构化工具事件回调（看板持久化：仅广播 task.* WS 事件 + 更新 TaskMeta）
		var onToolEvent any
		if broadcaster != nil {
			onToolEvent = func(event runner.ToolEvent) {
				// 首次工具调用 → 懒创建任务 session（非 async 任务在 queued 时未创建）
				if !taskSessionCreated {
					taskSessionCreated = true
					store := ctx.Context.SessionStore
					if store != nil {
						// M-01 修复: 先检查 session 是否已存在（async 任务在 queued 时已创建）
						existing := store.LoadSessionEntry(taskSessionKey)
						if existing != nil {
							// session 已存在（async 路径），仅更新 TaskMeta
							if existing.TaskMeta == nil {
								existing.TaskMeta = &sessiontypes.TaskMeta{}
							}
							if existing.TaskMeta.Status == "queued" {
								existing.TaskMeta.Status = "started"
								existing.TaskMeta.StartedAt = time.Now().UnixMilli()
							}
							existing.UpdatedAt = time.Now().UnixMilli()
							store.Save(existing)
						} else {
							// session 不存在（sync 路径），创建新的
							taskLabel := truncateStr(text, 60)
							now := time.Now().UnixMilli()
							store.Save(&SessionEntry{
								SessionKey: taskSessionKey,
								SessionId:  runId,
								Label:      taskLabel,
								Channel:    "task",
								CreatedAt:  now,
								UpdatedAt:  now,
								TaskMeta: &sessiontypes.TaskMeta{
									Status:    "started",
									StartedAt: now,
								},
							})
						}
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
				// 持久化最近一个工具步骤，供 tasks.list 在刷新后稳定读取。
				if store := ctx.Context.SessionStore; store != nil {
					if entry := store.LoadSessionEntry(taskSessionKey); entry != nil {
						if entry.TaskMeta == nil {
							entry.TaskMeta = &sessiontypes.TaskMeta{}
						}
						entry.TaskMeta.Status = "progress"
						entry.TaskMeta.ToolName = event.ToolName
						entry.TaskMeta.ProgressPhase = event.Phase
						entry.TaskMeta.ProgressText = progressText
						entry.TaskMeta.ProgressIsError = event.IsError
						entry.TaskMeta.ProgressDuration = event.Duration
						entry.TaskMeta.ProgressAt = now
						if entry.TaskMeta.StartedAt == 0 {
							entry.TaskMeta.StartedAt = now
						}
						entry.UpdatedAt = now
						store.Save(entry)
					}
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
		})

		if result.Error != nil {
			slog.Error("chat.send: pipeline error",
				"error", result.Error,
				"runId", runId,
				"sessionKey", sessionKey,
			)
			// 兜底: 如果管线在 RunAttempt 之前就失败，defer 不会执行，用户消息未持久化。
			// 此处确保用户消息至少写入 transcript，刷新后不会完全消失。
			ensureUserTranscriptOnError(resolvedSessionId, storePath, enhancedText)
			// 广播 task.failed 结构化看板事件
			if broadcaster != nil {
				broadcaster.Broadcast(EventTaskFailed, TaskFailedEvent{
					TaskID:     runId,
					SessionKey: sessionKey,
					Error:      truncateForLog(result.Error.Error(), 200),
					Ts:         time.Now().UnixMilli(),
				}, &BroadcastOptions{DropIfSlow: true})
			}
			// 更新 TaskMeta.Status = "failed"（看板持久化）
			if store := ctx.Context.SessionStore; store != nil {
				taskKey := fmt.Sprintf("task:%s", runId)
				now := time.Now().UnixMilli()
				entry := store.LoadSessionEntry(taskKey)
				if entry == nil {
					// D-01 兜底：sync 任务无工具调用时 session 未创建
					entry = &SessionEntry{
						SessionKey: taskKey,
						SessionId:  runId,
						Label:      truncateStr(text, 60),
						Channel:    "task",
						CreatedAt:  now,
					}
				}
				if entry.TaskMeta == nil {
					entry.TaskMeta = &sessiontypes.TaskMeta{}
				}
				entry.TaskMeta.Status = "failed"
				entry.TaskMeta.Error = truncateForLog(result.Error.Error(), 200)
				entry.TaskMeta.CompletedAt = now
				entry.UpdatedAt = now
				store.Save(entry)
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

		// AI 回复 transcript 由 attempt_runner.persistToTranscript 写入，此处仅构造广播消息
		var message map[string]interface{}
		if combinedReply != "" {
			now := time.Now().UnixMilli()
			message = map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": combinedReply},
				},
				"timestamp":  now,
				"stopReason": "stop",
				"usage":      map[string]interface{}{"input": 0, "output": 0, "totalTokens": 0},
			}
		} else if len(mediaItems) > 0 {
			// 纯媒体（无文本）：构建仅含图片的消息
			message = map[string]interface{}{
				"role":       "assistant",
				"content":    []interface{}{},
				"timestamp":  time.Now().UnixMilli(),
				"stopReason": "stop",
				"usage":      map[string]interface{}{"input": 0, "output": 0, "totalTokens": 0},
			}
		}

		// 将媒体数据注入 message.content（前端 extractImages 自动识别）
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
		// 更新 TaskMeta.Status = "completed"（看板持久化）
		if store := ctx.Context.SessionStore; store != nil {
			taskKey := fmt.Sprintf("task:%s", runId)
			now := time.Now().UnixMilli()
			entry := store.LoadSessionEntry(taskKey)
			if entry == nil {
				// D-01 兜底：sync 任务无工具调用时 session 未创建
				entry = &SessionEntry{
					SessionKey: taskKey,
					SessionId:  runId,
					Label:      truncateStr(text, 60),
					Channel:    "task",
					CreatedAt:  now,
				}
			}
			if entry.TaskMeta == nil {
				entry.TaskMeta = &sessiontypes.TaskMeta{}
			}
			entry.TaskMeta.Status = "completed"
			entry.TaskMeta.Summary = truncateForLog(combinedReply, 200)
			entry.TaskMeta.CompletedAt = now
			entry.UpdatedAt = now
			store.Save(entry)
		}

		// Phase 2: async=true 时注入 webchat 通知（跨 session 通知用户异步任务已完成）
		if asyncMode && broadcaster != nil {
			notifyText := fmt.Sprintf("[异步任务完成] %s", truncateStr(text, 60))
			if combinedReply != "" {
				notifyText = fmt.Sprintf("[异步任务完成] %s\n结果: %s",
					truncateStr(text, 60), truncateForLog(combinedReply, 150))
			}
			broadcaster.Broadcast("channel.message.incoming", map[string]interface{}{
				"sessionKey": sessionKey,
				"channel":    "webchat",
				"text":       notifyText,
				"from":       "system",
				"ts":         time.Now().UnixMilli(),
				"async":      true,
			}, nil)
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
func ensureUserTranscriptOnError(sessionId, storePath, text string) {
	if sessionId == "" || storePath == "" || strings.TrimSpace(text) == "" {
		return
	}
	mgr := session.NewSessionManager("")
	sessionFile := filepath.Join(filepath.Dir(storePath), sessionId+".jsonl")

	// 检查 transcript 尾部是否已有相同 user 消息（RunAttempt defer 可能已写入）
	existing, _ := mgr.LoadSessionMessages(sessionId, sessionFile)
	if len(existing) > 0 {
		last := existing[len(existing)-1]
		if role, _ := last["role"].(string); role == "user" {
			// 已有 user 消息在末尾，RunAttempt 的 defer 已经处理，跳过
			return
		}
	}

	if _, err := mgr.EnsureSessionFile(sessionId, sessionFile); err != nil {
		slog.Debug("ensureUserTranscriptOnError: ensure file failed", "error", err)
		return
	}
	entry := session.TranscriptEntry{
		Role:      "user",
		Content:   []session.ContentBlock{{Type: "text", Text: text}},
		Timestamp: time.Now().UnixMilli(),
	}
	if err := mgr.AppendMessage(sessionId, sessionFile, entry); err != nil {
		slog.Warn("ensureUserTranscriptOnError: append failed", "error", err)
	}
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
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
// 将转录/转换结果追加到消息文本中返回增强后的文本。
func processAttachmentsForChat(ctx context.Context, text string, attachments []map[string]interface{}, cfgLoader interface {
	LoadConfig() (*types.OpenAcosmiConfig, error)
}) string {
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
) string {
	if len(attachments) == 0 || cfgLoader == nil {
		return text
	}
	if providerCache == nil {
		providerCache = defaultChatAttachmentProviderCache
	}

	cfg, err := cfgLoader.LoadConfig()
	if err != nil || cfg == nil {
		return text
	}
	providerSnapshot := providerCache.Resolve(cfg)

	var parts []string
	if text != "" {
		parts = append(parts, text)
	}

	for _, att := range attachments {
		attType, _ := att["type"].(string)
		contentB64, _ := att["content"].(string)
		mimeType, _ := att["mimeType"].(string)
		fileName, _ := att["fileName"].(string)

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
		case "audio":
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
			if mimeType == "" {
				mimeType = "audio/webm"
			}
			sttCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			transcript, sttErr := providerSnapshot.sttProvider.Transcribe(sttCtx, data, mimeType)
			cancel()
			if sttErr != nil {
				slog.Error("chat.send: STT failed", "error", sttErr)
				parts = append(parts, "[语音转录失败]")
			} else {
				parts = append(parts, fmt.Sprintf("[语音转录]: %s", transcript))
			}

		case "document":
			if cfg.DocConv == nil || cfg.DocConv.Provider == "" {
				if fileName != "" {
					parts = append(parts, fmt.Sprintf("[文件: %s]", fileName))
				}
				continue
			}
			if fileName == "" {
				fileName = "untitled"
			}
			if !media.IsSupportedFormat(fileName) {
				parts = append(parts, fmt.Sprintf("[文件: %s, 格式不支持转换]", fileName))
				continue
			}
			data, decErr := base64.StdEncoding.DecodeString(contentB64)
			if decErr != nil {
				parts = append(parts, fmt.Sprintf("[文件: %s, 解码失败]", fileName))
				continue
			}
			if len(data) > maxAttachmentBytes {
				parts = append(parts, fmt.Sprintf("[文件: %s, 数据过大]", fileName))
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

	if len(parts) <= 1 {
		return text
	}
	return strings.Join(parts, "\n")
}
