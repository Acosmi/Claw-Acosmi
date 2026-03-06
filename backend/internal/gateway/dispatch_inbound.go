package gateway

// dispatch_inbound.go — gateway → autoreply 管线桥接
// 对应 TS src/auto-reply/dispatch.ts dispatchInboundMessage
//
// 在 chat.send 中调用，将用户消息路由到 autoreply 管线。
// 使用 DI 回调模式（PipelineDispatcher）避免 gateway ↔ autoreply/reply 的循环导入。
// 回调在 server.go 启动时由外层注入。

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
)

// PipelineDispatcher 管线分发接口。
// 由外层（如 server.go）注入，打破 gateway ↔ autoreply/reply 循环导入。
type PipelineDispatcher func(ctx context.Context, msgCtx *autoreply.MsgContext, opts *autoreply.GetReplyOptions) ([]autoreply.ReplyPayload, error)

// DispatchInboundParams 入站消息分发参数。
type DispatchInboundParams struct {
	// MsgContext 消息上下文
	MsgCtx *autoreply.MsgContext

	// 会话信息
	SessionKey string
	SessionID  string
	StorePath  string
	AgentID    string

	// abort 信号
	Ctx context.Context

	// 事件处理
	RunID string

	// 管线分发器（DI 注入）
	Dispatcher PipelineDispatcher

	// 任务频道回调（可选，由 chat.send 注入用于广播执行步骤）
	OnToolResult func(payload autoreply.ReplyPayload)
	// OnToolEvent 结构化工具事件回调（可选，由 chat.send 注入用于广播工具详情）。
	// 实际类型: func(runner.ToolEvent)，通过 any 传递避免循环导入。
	OnToolEvent any
	// OnProgress 中间进度回调（可选）。
	// 仅在显式接入远程阶段性投递时设置；nil = 仅本地实时事件。
	OnProgress func(ctx context.Context, update runner.ProgressUpdate) runner.ProgressReportStatus
}

// DispatchInboundResult 分发结果。
type DispatchInboundResult struct {
	Replies []autoreply.ReplyPayload
	Error   error
}

// DispatchInboundMessage 将入站消息路由到 autoreply 管线。
// TS 对照: auto-reply/dispatch.ts dispatchInboundMessage
//
// 流程：
// 1. 构建 GetReplyOptions
// 2. 通过 PipelineDispatcher 分发到 autoreply 管线
// 3. 返回回复结果
func DispatchInboundMessage(ctx context.Context, params DispatchInboundParams) *DispatchInboundResult {
	if params.MsgCtx == nil {
		return &DispatchInboundResult{Error: fmt.Errorf("MsgCtx is required")}
	}

	// 使用 stub 分发器（如果未注入管线分发器）
	if params.Dispatcher == nil {
		slog.Info("dispatch_inbound: no pipeline dispatcher injected, using stub",
			"sessionKey", params.SessionKey,
			"runId", params.RunID,
		)
		return &DispatchInboundResult{
			Replies: []autoreply.ReplyPayload{},
		}
	}

	// 构建 GetReplyOptions
	getReplyOpts := &autoreply.GetReplyOptions{
		RunID:        params.RunID,
		IsHeartbeat:  false,
		OnToolResult: params.OnToolResult,
		OnToolEvent:  params.OnToolEvent,
		OnProgress:   params.OnProgress,
	}

	slog.Info("dispatch_inbound: starting pipeline",
		"sessionKey", params.SessionKey,
		"agentId", params.AgentID,
		"runId", params.RunID,
		"body", truncateForLog(params.MsgCtx.Body, 80),
	)

	// 调用管线分发器
	replies, err := params.Dispatcher(ctx, params.MsgCtx, getReplyOpts)
	if err != nil {
		slog.Error("dispatch_inbound: pipeline error",
			"error", err,
			"sessionKey", params.SessionKey,
			"runId", params.RunID,
		)
		return &DispatchInboundResult{Error: err}
	}

	slog.Info("dispatch_inbound: pipeline complete",
		"sessionKey", params.SessionKey,
		"runId", params.RunID,
		"replyCount", len(replies),
	)

	return &DispatchInboundResult{Replies: replies}
}

// CombineReplyPayloads 合并回复载荷文本。
// TS 对照: chat.ts .then 块中的 combinedReply 逻辑
// 同时剥离 [[reply_to_current]] / [[reply_to:<id>]] / [[reply:<id>]] 等 reply 标签，
// 避免标签原样暴露给用户。
func CombineReplyPayloads(replies []autoreply.ReplyPayload) string {
	parts := make([]string, 0, len(replies))
	for _, r := range replies {
		text := strings.TrimSpace(r.Text)
		text = stripReplyTags(text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// stripReplyTags 剥离 [[reply_to_current]] / [[reply_to:<id>]] / [[reply:<val>]] 标签。
// 这是轻量级实现，避免 gateway 包 import reply 包导致循环依赖。
func stripReplyTags(text string) string {
	result := text
	searchFrom := 0
	for {
		idx := strings.Index(result[searchFrom:], "[[")
		if idx < 0 {
			break
		}
		openIdx := searchFrom + idx
		closeIdx := strings.Index(result[openIdx+2:], "]]")
		if closeIdx < 0 {
			break
		}
		closeIdx += openIdx + 2
		inner := strings.TrimSpace(strings.ToLower(result[openIdx+2 : closeIdx]))
		if inner == "reply_to_current" ||
			strings.HasPrefix(inner, "reply_to:") ||
			strings.HasPrefix(inner, "reply:") {
			result = result[:openIdx] + result[closeIdx+2:]
			// 不递增 searchFrom，因为剥离后后续内容前移
		} else {
			// 非 reply 标签，跳过继续扫描
			searchFrom = closeIdx + 2
		}
	}
	return strings.TrimSpace(result)
}

// ReplyMediaItem 回复媒体项（base64 + MIME）。
type ReplyMediaItem struct {
	Base64Data string
	MimeType   string
}

// ExtractMediaListFromReplies 从回复载荷中提取所有 base64 媒体数据。
func ExtractMediaListFromReplies(replies []autoreply.ReplyPayload) []ReplyMediaItem {
	if len(replies) == 0 {
		return nil
	}
	items := make([]ReplyMediaItem, 0, len(replies))
	for _, r := range replies {
		if len(r.MediaItems) > 0 {
			appended := false
			for _, media := range r.MediaItems {
				if media.MediaBase64 == "" {
					continue
				}
				mimeType := media.MediaMimeType
				if mimeType == "" {
					mimeType = r.MediaMimeType
				}
				items = append(items, ReplyMediaItem{
					Base64Data: media.MediaBase64,
					MimeType:   mimeType,
				})
				appended = true
			}
			// 某些异常 payload 可能携带空 mediaItems，但 legacy 字段仍有数据。
			if !appended && r.MediaBase64 != "" {
				items = append(items, ReplyMediaItem{
					Base64Data: r.MediaBase64,
					MimeType:   r.MediaMimeType,
				})
			}
			continue
		}
		if r.MediaBase64 != "" {
			items = append(items, ReplyMediaItem{
				Base64Data: r.MediaBase64,
				MimeType:   r.MediaMimeType,
			})
		}
	}
	return items
}

// ExtractMediaFromReplies 从回复载荷中提取第一个 base64 媒体数据。
// 兼容旧调用：新逻辑请优先使用 ExtractMediaListFromReplies。
func ExtractMediaFromReplies(replies []autoreply.ReplyPayload) (base64Data, mimeType string) {
	items := ExtractMediaListFromReplies(replies)
	if len(items) > 0 {
		return items[0].Base64Data, items[0].MimeType
	}
	return "", ""
}

// truncateForLog 截断字符串用于日志。
func truncateForLog(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
