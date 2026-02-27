package gateway

// server_methods_send.go — send (channel message send) + poll
// 对应 TS src/gateway/server-methods/send.ts (364L)
//
// send 方法处理频道消息发送（非 chat），路由到 outbound 管线。
// poll 方法处理频道消息轮询。
// 隐藏依赖: inflightByContext 去重 (TS: WeakMap, Go: sync.Map)

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/anthropic/open-acosmi/internal/channels"
)

// ChannelOutboundSender DI 接口 — 频道消息发送。
// 对应 TS 中 channel plugin 的 outbound 发送能力。
type ChannelOutboundSender interface {
	SendOutbound(channel, accountId, target, text string, media []map[string]interface{}) error
}

// SendHandlers 返回 send + poll 方法处理器映射。
func SendHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"send": handleSend,
		"poll": handlePoll,
	}
}

// ---------- inflight 去重 (隐依赖 #3) ----------
// TS 用 WeakMap<GatewayRequestContext, Map<string, Promise>>
// Go 用 sync.Map (key = context pointer + idempotency key)

var inflightSends sync.Map // key: string → value: time.Time

func checkInflight(key string) bool {
	if key == "" {
		return false
	}
	_, loaded := inflightSends.LoadOrStore(key, time.Now())
	return loaded // true = 重复
}

func clearInflight(key string) {
	if key != "" {
		inflightSends.Delete(key)
	}
}

// ---------- send ----------
// 对应 TS send.ts L46-L242

func handleSend(ctx *MethodHandlerContext) {
	// 参数 schema 与 TS 协议对齐
	to := readString(ctx.Params, "to")
	message := readString(ctx.Params, "message")
	channel := readString(ctx.Params, "channel")
	accountId := readString(ctx.Params, "accountId")
	idempotencyKey := readString(ctx.Params, "idempotencyKey")
	sessionKey := readString(ctx.Params, "sessionKey")

	// TS L62-L68: gifPlayback / mirror 选项
	gifPlayback, _ := ctx.Params["gifPlayback"].(bool)
	mirror, _ := ctx.Params["mirror"].(bool)

	if message == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid send params: message is required"))
		return
	}

	if to == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid send params: to is required"))
		return
	}

	// 规范化频道 ID — 可选，默认 chat
	channelID := channel
	if channel != "" {
		channelID = normalizeChannelID(channel)
		if channelID == "" {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unsupported channel: "+channel))
			return
		}
	}
	if channelID == "" {
		channelID = "chat"
	}

	// 幂等去重 (对应 TS context.dedupe + inflightByContext)
	dedupeKey := "send:" + idempotencyKey
	if idempotencyKey != "" {
		if ctx.Context.IdempotencyCache != nil {
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
		if checkInflight(dedupeKey) {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "duplicate send in flight"))
			return
		}
		defer clearInflight(dedupeKey)
	}

	// 解析 media
	var mediaUrls []string
	if v, ok := ctx.Params["mediaUrls"].([]interface{}); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				mediaUrls = append(mediaUrls, s)
			}
		}
	}
	if mu := readString(ctx.Params, "mediaUrl"); mu != "" && len(mediaUrls) == 0 {
		mediaUrls = []string{mu}
	}

	// 解析 base64 媒体（Agent 生成的截图/图表，无公网 URL）
	mediaBase64 := readString(ctx.Params, "mediaBase64")
	mediaMimeType := readString(ctx.Params, "mediaMimeType")

	// 生成消息 ID
	msgId := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	slog.Info("send: dispatching",
		"channel", channelID,
		"to", truncateStr(to, 40),
		"text", truncateStr(message, 80),
		"mediaUrls", len(mediaUrls),
		"hasBase64Media", mediaBase64 != "",
		"sessionKey", sessionKey,
		"msgId", msgId,
	)

	// base64 媒体路径：解码后通过 ChannelMgr 直接发送到频道插件
	if mediaBase64 != "" && ctx.Context.ChannelMgr != nil && channelID != "chat" {
		const maxMediaBase64Size = 10 * 1024 * 1024 // 10 MB（匹配飞书 image API 限制）
		mediaData, err := base64.StdEncoding.DecodeString(mediaBase64)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid mediaBase64: "+err.Error()))
			return
		}
		if len(mediaData) > maxMediaBase64Size {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
				fmt.Sprintf("mediaBase64 decoded size %d exceeds %d byte limit", len(mediaData), maxMediaBase64Size)))
			return
		}
		result, err := ctx.Context.ChannelMgr.SendMessage(channels.ChannelID(channelID), channels.OutboundSendParams{
			Ctx:           context.Background(),
			To:            to,
			Text:          message,
			AccountID:     accountId,
			MediaData:     mediaData,
			MediaMimeType: mediaMimeType,
		})
		if err != nil {
			slog.Warn("send: base64 media delivery failed", "error", err)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
			return
		}
		payload := map[string]interface{}{
			"runId":     idempotencyKey,
			"messageId": msgId,
			"channel":   channelID,
		}
		if result != nil && result.ChatID != "" {
			payload["chatId"] = result.ChatID
		}
		ctx.Respond(true, payload, nil)
		return
	}

	// 优先使用 OutboundPipeline（完整管线）
	if pipe := ctx.Context.OutboundPipe; pipe != nil {
		// 1) 解析 outbound 目标
		target, err := pipe.ResolveTarget(channelID, to, accountId)
		if err != nil {
			slog.Warn("send: target resolution failed", "error", err)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "target resolution failed: "+err.Error()))
			return
		}

		// 2) 确保 session route
		if sessionKey != "" {
			if err := pipe.EnsureSessionRoute(target, sessionKey); err != nil {
				slog.Warn("send: session route failed", "error", err,
					"sessionKey", sessionKey)
			}
		}

		// 3) 投递
		result, err := pipe.Deliver(target, message, mediaUrls, &OutboundDeliverOpts{
			SessionKey:     sessionKey,
			IdempotencyKey: idempotencyKey,
			GifPlayback:    gifPlayback,
			Mirror:         mirror,
		})
		if err != nil {
			slog.Warn("send: delivery failed", "error", err)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
			return
		}

		payload := map[string]interface{}{
			"runId":     idempotencyKey,
			"messageId": result.MessageID,
			"channel":   channelID,
		}
		if result.ChatID != "" {
			payload["chatId"] = result.ChatID
		}
		ctx.Respond(true, payload, nil)
		return
	}

	// Fallback: 旧 ChannelOutboundSender DI
	if ctx.Context.ChannelSender != nil {
		var media []map[string]interface{}
		for _, u := range mediaUrls {
			media = append(media, map[string]interface{}{"url": u})
		}
		if err := ctx.Context.ChannelSender.SendOutbound(channelID, accountId, to, message, media); err != nil {
			slog.Warn("send: outbound failed", "error", err)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
			return
		}
		payload := map[string]interface{}{
			"runId":     idempotencyKey,
			"messageId": msgId,
			"channel":   channelID,
		}
		ctx.Respond(true, payload, nil)
		return
	}

	// Fallback: DI 未注入 — stub 响应
	ctx.Respond(true, map[string]interface{}{
		"runId":     idempotencyKey,
		"messageId": msgId,
		"channel":   channelID,
		"stub":      true,
		"note":      "outbound channel pipeline not connected",
	}, nil)
}

// ---------- poll ----------
// 对应 TS send.ts L243-L363

func handlePoll(ctx *MethodHandlerContext) {
	to := readString(ctx.Params, "to")
	question := readString(ctx.Params, "question")
	channel := readString(ctx.Params, "channel")
	idempotencyKey := readString(ctx.Params, "idempotencyKey")

	if to == "" || question == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid poll params: to and question are required"))
		return
	}

	// 解析 options
	var options []string
	if v, ok := ctx.Params["options"].([]interface{}); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				options = append(options, s)
			}
		}
	}
	if len(options) == 0 {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid poll params: options are required"))
		return
	}

	// 频道规范化
	channelID := channel
	if channel != "" {
		channelID = normalizeChannelID(channel)
		if channelID == "" {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unsupported poll channel: "+channel))
			return
		}
	}
	if channelID == "" {
		channelID = "chat"
	}

	// 幂等去重
	if idempotencyKey != "" {
		dedupeKey := "poll:" + idempotencyKey
		if ctx.Context.IdempotencyCache != nil {
			check := ctx.Context.IdempotencyCache.CheckOrRegister(idempotencyKey)
			if check.IsDuplicate && check.State == IdempotencyCompleted {
				ctx.Respond(true, check.CachedResult, nil)
				return
			}
		}
		if checkInflight(dedupeKey) {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "duplicate poll in flight"))
			return
		}
		defer clearInflight(dedupeKey)
	}

	// Poll 需要 channel plugin 系统支持
	if pipe := ctx.Context.OutboundPipe; pipe != nil {
		result, err := pipe.SendPoll(channelID, question, options, to)
		if err != nil {
			slog.Warn("poll: delivery failed", "error", err)
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
			return
		}
		payload := map[string]interface{}{
			"runId":   idempotencyKey,
			"channel": channelID,
			"ok":      result.OK,
		}
		if result.PollID != "" {
			payload["pollId"] = result.PollID
		}
		ctx.Respond(true, payload, nil)
		return
	}

	// Fallback: DI 未注入 — stub 响应
	ctx.Respond(true, map[string]interface{}{
		"runId":   idempotencyKey,
		"channel": channelID,
		"stub":    true,
		"note":    "poll requires channel plugin sendPoll support",
	}, nil)
}
