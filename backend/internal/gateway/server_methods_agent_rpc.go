package gateway

// server_methods_agent_rpc.go — agent 主 RPC 处理器
// 对应 TS src/gateway/server-methods/agent.ts L46-L433
//
// 这是前端 Agent 面板对话入口。与 chat.send 不同，agent 方法：
//   - 绑定特定 agent
//   - 支持多频道路由 (channel/accountId)
//   - 自动管理 session 创建和 key 解析
//
// 管线流程：
//   params → agent 解析 → session 创建/解析 → ack 返回 → 异步 dispatchInboundMessage → 广播结果

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// AgentRPCHandlers 返回 agent 主 RPC 方法处理器。
// 注意：与 AgentHandlers() (agent.identity.get / agent.wait) 分开注册，
// 因为 "agent" 方法是独立的顶层 RPC 而非 "agent.*" 子方法。
func AgentRPCHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"agent": handleAgentRPC,
	}
}

// handleAgentRPC 处理 agent 主 RPC 请求。
// TS 对照: agent.ts L46-L433
//
// 参数:
//   - text/message: 用户消息文本
//   - sessionKey/sessionId: 会话标识（可选，缺省自动创建）
//   - agentId: Agent 标识（可选，缺省使用默认 agent）
//   - channel: 消息来源频道（可选，默认 "webchat"）
//   - accountId: 来源账号 ID（可选）
//   - images: 附带图片列表（可选）
//   - idempotencyKey: 幂等键（可选）
func handleAgentRPC(ctx *MethodHandlerContext) {
	// ---------- 1. 参数解析 ----------
	text, _ := ctx.Params["text"].(string)
	if text == "" {
		text, _ = ctx.Params["message"].(string)
	}
	sessionKey, _ := ctx.Params["sessionKey"].(string)
	if sessionKey == "" {
		sessionKey, _ = ctx.Params["sessionId"].(string)
	}
	agentId, _ := ctx.Params["agentId"].(string)
	channel, _ := ctx.Params["channel"].(string)
	accountId, _ := ctx.Params["accountId"].(string)
	idempotencyKey, _ := ctx.Params["idempotencyKey"].(string)

	// TS L68-L75: group 字段
	groupId, _ := ctx.Params["groupId"].(string)
	groupChannel, _ := ctx.Params["groupChannel"].(string)
	groupSpace, _ := ctx.Params["groupSpace"].(string)
	spawnedBy, _ := ctx.Params["spawnedBy"].(string)

	// TS L78-L82: 客户端能力
	clientCaps, _ := ctx.Params["clientCapabilities"].(map[string]interface{})

	// 解析 images
	var images []string
	if v, ok := ctx.Params["images"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					images = append(images, s)
				}
			}
		}
	}

	// 解析 attachments（对应 TS L85-L100 parseMessageWithAttachments）
	var parsedImages []AgentImage
	if ap := ctx.Context.AttachmentParser; ap != nil {
		var attachments []map[string]interface{}
		if v, ok := ctx.Params["attachments"].([]interface{}); ok {
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					attachments = append(attachments, m)
				}
			}
		}
		if len(attachments) > 0 {
			parsed, err := ap.ParseMessageWithAttachments(text, attachments, 10*1024*1024)
			if err != nil {
				slog.Warn("agent: attachment parse failed", "error", err)
			} else {
				text = parsed.Message
				parsedImages = parsed.Images
			}
		}
	}

	// 时间戳注入（对应 TS L102-L106 injectTimestamp）
	if ti := ctx.Context.TimestampInject; ti != nil {
		text = ti.InjectTimestamp(text, ctx.Context.Config)
	}

	// ---------- 2. Agent 解析 ----------
	cfg := resolveConfigFromContext(ctx)
	if agentId == "" && cfg != nil {
		agentId = scope.ResolveDefaultAgentId(cfg)
	}
	if agentId != "" {
		agentId = scope.NormalizeAgentId(agentId)
	}

	// 频道验证和规范化（对应 TS L108-L122 isGatewayMessageChannel / normalizeMessageChannel）
	if channel != "" {
		if cv := ctx.Context.ChannelValidator; cv != nil {
			if !cv.IsGatewayMessageChannel(channel) {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
					"unsupported message channel: "+channel))
				return
			}
			channel = cv.NormalizeMessageChannel(channel)
		}
	}
	if channel == "" {
		channel = "webchat"
	}

	// ---------- 3. Session key 解析 ----------
	// TS 对照: agent.ts L120-L160 resolveSessionStoreKey
	if sessionKey == "" {
		// 自动生成: agent:<agentId>:main
		if agentId != "" {
			sessionKey = "agent:" + agentId + ":main"
		} else {
			sessionKey = "default:main"
		}
	}

	// ---------- 4. 幂等检查 ----------
	if idempotencyKey != "" && ctx.Context.IdempotencyCache != nil {
		check := ctx.Context.IdempotencyCache.CheckOrRegister(idempotencyKey)
		if check.IsDuplicate {
			if check.State == IdempotencyCompleted {
				ctx.Respond(true, check.CachedResult, nil)
				return
			}
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "duplicate request in flight"))
			return
		}
	}

	// ---------- 5. ChatState 检查 ----------
	chatState := ctx.Context.ChatState
	if chatState == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "chat state not available"))
		return
	}

	// 生成 runId（对应 TS L155: idempotencyKey 优先作为 runId）
	runId := idempotencyKey
	if runId == "" {
		runId = fmt.Sprintf("agent_run_%d", time.Now().UnixNano())
	}

	// 注册运行条目
	chatState.Registry.Add(sessionKey, ChatRunEntry{
		SessionKey:  sessionKey,
		ClientRunID: runId,
	})

	slog.Info("agent: dispatching",
		"sessionKey", sessionKey,
		"agentId", agentId,
		"channel", channel,
		"text", truncateForLog(text, 80),
		"images", len(images),
		"runId", runId,
	)

	// ---------- 6. 广播 chat.stream.start ----------
	if bc := ctx.Context.Broadcaster; bc != nil {
		bc.Broadcast("chat.stream.start", map[string]interface{}{
			"sessionKey": sessionKey,
			"runId":      runId,
			"agentId":    agentId,
			"channel":    channel,
			"ts":         time.Now().UnixMilli(),
		}, nil)
	}

	// ---------- 7. 立即返回 ack ----------
	ctx.Respond(true, map[string]interface{}{
		"runId":  runId,
		"status": "started",
	}, nil)

	// ---------- 8. 异步管线 ----------
	pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
	broadcaster := ctx.Context.Broadcaster
	storePath := ctx.Context.StorePath
	dispatcher := ctx.Context.PipelineDispatcher

	go func() {
		defer pipelineCancel()
		defer func() {
			chatState.Registry.Remove(sessionKey, runId, sessionKey)
		}()

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
					slog.Info("agent: auto-created session",
						"sessionKey", sessionKey,
						"sessionId", newId,
						"agentId", agentId,
					)
				}
				resolvedSessionId = entry.SessionId
			}
		}
		if resolvedSessionId == "" {
			resolvedSessionId = runId
		}

		// ---- 持久化用户消息到 transcript ----
		AppendUserTranscriptMessage(AppendTranscriptParams{
			Message:         text,
			SessionID:       resolvedSessionId,
			StorePath:       storePath,
			CreateIfMissing: true,
		})

		// ---- 构建 MsgContext ----
		// TS 对照: agent.ts L200-L280
		surface := channel
		if surface == "" {
			surface = "webchat"
		}
		chatType := "direct"
		// 群组键检测
		if ParseGroupKey(sessionKey) != nil {
			chatType = "group"
		}

		msgCtx := &autoreply.MsgContext{
			Body:               text,
			BodyForAgent:       text,
			BodyForCommands:    text,
			RawBody:            text,
			CommandBody:        text,
			SessionID:          resolvedSessionId,
			SessionKey:         sessionKey,
			Provider:           channel,
			Surface:            surface,
			OriginatingChannel: channel,
			AccountID:          accountId,
			ChatType:           chatType,
			CommandAuthorized:  true,
			MessageSid:         runId,
			// TS L235-L240: group 字段
			GroupID:      groupId,
			GroupChannel: groupChannel,
			GroupSpace:   groupSpace,
			SpawnedBy:    spawnedBy,
		}

		// 客户端能力注入（对应 TS L242-L245）
		if clientCaps != nil {
			msgCtx.ClientCapabilities = clientCaps
		}

		// 图片注入（合并 images 和 parsedImages）
		if len(parsedImages) > 0 {
			for _, img := range parsedImages {
				msgCtx.Images = append(msgCtx.Images, autoreply.MessageImage{
					Type:     img.Type,
					Data:     img.Data,
					MimeType: img.MimeType,
				})
			}
		}
		if len(images) > 0 {
			for _, url := range images {
				msgCtx.Images = append(msgCtx.Images, autoreply.MessageImage{
					Type: "url",
					Data: url,
				})
			}
		}

		// ---- 调用管线 ----
		result := DispatchInboundMessage(pipelineCtx, DispatchInboundParams{
			MsgCtx:     msgCtx,
			SessionKey: sessionKey,
			AgentID:    agentId,
			StorePath:  storePath,
			RunID:      runId,
			Ctx:        pipelineCtx,
			Dispatcher: dispatcher,
			OnProgress: buildChatProgressCallback(broadcaster, sessionKey),
		})

		if result.Error != nil {
			slog.Error("agent: pipeline error",
				"error", result.Error,
				"runId", runId,
				"sessionKey", sessionKey,
				"agentId", agentId,
			)
			// 广播错误
			if broadcaster != nil {
				broadcaster.Broadcast("chat", map[string]interface{}{
					"runId":        runId,
					"sessionKey":   sessionKey,
					"agentId":      agentId,
					"state":        "error",
					"errorMessage": result.Error.Error(),
				}, nil)
			}
			return
		}

		// 合并回复
		combinedReply := CombineReplyPayloads(result.Replies)

		// 写入 transcript 并广播 final
		var message map[string]interface{}
		if combinedReply != "" {
			var sessionId string
			store := ctx.Context.SessionStore
			if store != nil {
				if entry := store.LoadSessionEntry(sessionKey); entry != nil {
					sessionId = entry.SessionId
				}
			}
			if sessionId == "" {
				sessionId = runId
			}

			appended := AppendAssistantTranscriptMessage(AppendTranscriptParams{
				Message:         combinedReply,
				SessionID:       sessionId,
				StorePath:       storePath,
				CreateIfMissing: true,
			})
			if appended.OK {
				message = appended.Message
			} else {
				slog.Warn("agent: transcript append failed", "error", appended.Error)
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
			}
		}

		// 广播 chat final
		if broadcaster != nil {
			broadcaster.Broadcast("chat", map[string]interface{}{
				"runId":      runId,
				"sessionKey": sessionKey,
				"agentId":    agentId,
				"state":      "final",
				"message":    message,
			}, nil)
		}

		slog.Info("agent: complete",
			"runId", runId,
			"sessionKey", sessionKey,
			"agentId", agentId,
			"replyLength", len(combinedReply),
		)
	}()
}
