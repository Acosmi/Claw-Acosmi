package gateway

// channel_deps_adapter.go — MonitorDeps 适配层
// 将网关已有子系统（routing, session, pairing, media, events）适配为各渠道的 DI 回调。
// 避免各渠道 Monitor 启动代码中重复构建 deps。

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/channels/discord"
	"github.com/Acosmi/ClawAcosmi/internal/channels/slack"
	"github.com/Acosmi/ClawAcosmi/internal/channels/telegram"
	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/internal/routing"
)

// ChannelDepsContext 网关子系统集合，由 server.go 构造后传入。
type ChannelDepsContext struct {
	StorePath    string
	State        *GatewayState
	EventQueue   *SystemEventQueue
	Dispatcher   PipelineDispatcher
	SessionStore *SessionStore
}

// ---------- Discord ----------

// BuildDiscordDeps 构建 Discord MonitorDeps（全部真实实现）。
func BuildDiscordDeps(dctx *ChannelDepsContext) *discord.DiscordMonitorDeps {
	return &discord.DiscordMonitorDeps{
		ResolveAgentRoute:      buildDiscordRouteResolver(dctx),
		DispatchInboundMessage: buildDiscordDispatcher(dctx),
		RecordInboundSession:   buildDiscordSessionRecorder(dctx),
		UpsertPairingRequest:   buildDiscordPairingUpsert(dctx),
		ReadAllowFromStore:     buildReadAllowFromStore(dctx),
		ResolveStorePath:       buildResolveStorePath(dctx),
		ReadSessionUpdatedAt:   buildReadSessionUpdatedAt(dctx),
		ResolveMedia:           buildResolveMedia(dctx),
		EnqueueSystemEvent:     buildDiscordEnqueueEvent(dctx),
		RecordChannelActivity:  buildRecordChannelActivity(dctx),
		ResetSession:           buildResetSession(dctx),
		SwitchModel:            buildSwitchModel(dctx),
		AddReaction:            nil, // discordgo session 在 MonitorProvider 内部设置
		RemoveReaction:         nil, // discordgo session 在 MonitorProvider 内部设置
	}
}

func buildDiscordRouteResolver(dctx *ChannelDepsContext) func(params discord.DiscordAgentRouteParams) (*discord.DiscordAgentRoute, error) {
	return func(params discord.DiscordAgentRouteParams) (*discord.DiscordAgentRoute, error) {
		agentID := routing.DefaultAgentID
		sessionKey := routing.BuildAgentPeerSessionKey(routing.PeerSessionKeyParams{
			AgentID:   agentID,
			MainKey:   routing.DefaultMainKey,
			Channel:   params.Channel,
			AccountID: params.AccountID,
			PeerKind:  params.PeerKind,
			PeerID:    params.PeerID,
		})
		mainSessionKey := routing.BuildAgentMainSessionKey(agentID, routing.DefaultMainKey)
		return &discord.DiscordAgentRoute{
			AgentID:        agentID,
			AccountID:      params.AccountID,
			SessionKey:     sessionKey,
			MainSessionKey: mainSessionKey,
		}, nil
	}
}

func buildDiscordDispatcher(dctx *ChannelDepsContext) func(ctx context.Context, params discord.DiscordDispatchParams) (*discord.DiscordDispatchResult, error) {
	return func(ctx context.Context, params discord.DiscordDispatchParams) (*discord.DiscordDispatchResult, error) {
		if params.Ctx == nil {
			return nil, fmt.Errorf("MsgContext is required")
		}

		// 确保 session 注册 + 获取 sessionId
		sessionKey := params.Ctx.SessionKey
		var resolvedSessionId string
		if dctx.SessionStore != nil && sessionKey != "" {
			entry := dctx.SessionStore.LoadSessionEntry(sessionKey)
			if entry == nil {
				newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
				entry = &SessionEntry{
					SessionKey: sessionKey,
					SessionId:  newId,
					Label:      sessionKey,
					Channel:    params.Ctx.ChannelType,
					CreatedAt:  time.Now().UnixMilli(),
				}
				dctx.SessionStore.Save(entry)
			}
			resolvedSessionId = entry.SessionId
		}
		// 回写 SessionID 到 MsgContext（供 pipelineDispatcher 传递到 reply 层）
		params.Ctx.SessionID = resolvedSessionId

		// 持久化用户消息到 transcript
		if resolvedSessionId != "" {
			AppendUserTranscriptMessage(AppendTranscriptParams{
				Message:         params.Ctx.Body,
				SessionID:       resolvedSessionId,
				StorePath:       dctx.StorePath,
				CreateIfMissing: true,
			})
		}

		// 广播用户消息到前端
		broadcastChatMessage(dctx.State, params.Ctx, "user")

		result := DispatchInboundMessage(ctx, DispatchInboundParams{
			MsgCtx:     params.Ctx,
			SessionKey: params.Ctx.SessionKey,
			Dispatcher: dctx.Dispatcher,
			OnProgress: buildMsgContextProgressCallback(dctx.State, params.Ctx),
		})
		if result.Error != nil {
			return nil, result.Error
		}

		// 广播 AI 回复到前端
		replyText := CombineReplyPayloads(result.Replies)
		mb64, mmime := ExtractMediaFromReplies(result.Replies)

		// AI 回复 transcript 由 attempt_runner.persistToTranscript 写入，此处不再双写

		if replyText != "" || mb64 != "" {
			broadcastChatReply(dctx.State, params.Ctx, replyText, mb64, mmime)
		}

		return &discord.DiscordDispatchResult{QueuedFinal: true}, nil
	}
}

func buildDiscordSessionRecorder(dctx *ChannelDepsContext) func(params discord.DiscordRecordSessionParams) error {
	return func(params discord.DiscordRecordSessionParams) error {
		if params.Ctx != nil && params.SessionKey != "" {
			dctx.SessionStore.RecordSessionMeta(params.SessionKey, InboundMeta{
				Channel:     params.Ctx.ChannelType,
				DisplayName: params.Ctx.From,
			})
		}
		if params.UpdateLastRoute != nil {
			dctx.SessionStore.UpdateLastRoute(params.UpdateLastRoute.SessionKey, UpdateLastRouteParams{
				Channel:   params.UpdateLastRoute.Channel,
				To:        params.UpdateLastRoute.To,
				AccountId: params.UpdateLastRoute.AccountID,
			})
		}
		return nil
	}
}

func buildDiscordPairingUpsert(dctx *ChannelDepsContext) func(params discord.DiscordPairingRequestParams) (*discord.DiscordPairingResult, error) {
	return func(params discord.DiscordPairingRequestParams) (*discord.DiscordPairingResult, error) {
		code, created, err := UpsertChannelPairingRequest(dctx.StorePath, params.Channel, params.ID, params.Meta)
		if err != nil {
			return nil, err
		}
		return &discord.DiscordPairingResult{Code: code, Created: created}, nil
	}
}

func buildDiscordEnqueueEvent(dctx *ChannelDepsContext) func(text, sessionKey, contextKey string) error {
	return func(text, sessionKey, contextKey string) error {
		dctx.EventQueue.Enqueue(text, sessionKey, contextKey)
		return nil
	}
}

// ---------- Telegram ----------

// BuildTelegramDeps 构建 Telegram MonitorDeps（全部真实实现）。
func BuildTelegramDeps(dctx *ChannelDepsContext) *telegram.TelegramMonitorDeps {
	return &telegram.TelegramMonitorDeps{
		ResolveAgentRoute:      buildTelegramRouteResolver(dctx),
		DispatchInboundMessage: buildTelegramDispatcher(dctx),
		RecordInboundSession:   buildTelegramSessionRecorder(dctx),
		UpsertPairingRequest:   buildTelegramPairingUpsert(dctx),
		ReadAllowFromStore:     buildReadAllowFromStore(dctx),
		ResolveStorePath:       buildResolveStorePath(dctx),
		ResetSession:           buildTelegramResetSession(dctx),
		SwitchModel:            buildTelegramSwitchModel(dctx),
		DescribeImage:          nil, // 需要视觉模型，暂留 nil（渠道内部降级处理）
		EnqueueSystemEvent:     buildTelegramEnqueueEvent(dctx),
		LoadSessionEntry:       buildTelegramLoadSessionEntry(dctx),
	}
}

func buildTelegramRouteResolver(dctx *ChannelDepsContext) func(params telegram.TelegramAgentRouteParams) (*telegram.TelegramAgentRoute, error) {
	return func(params telegram.TelegramAgentRouteParams) (*telegram.TelegramAgentRoute, error) {
		agentID := routing.DefaultAgentID
		sessionKey := routing.BuildAgentPeerSessionKey(routing.PeerSessionKeyParams{
			AgentID:   agentID,
			MainKey:   routing.DefaultMainKey,
			Channel:   params.Channel,
			AccountID: params.AccountID,
			PeerKind:  params.PeerKind,
			PeerID:    params.PeerID,
		})
		mainSessionKey := routing.BuildAgentMainSessionKey(agentID, routing.DefaultMainKey)
		return &telegram.TelegramAgentRoute{
			AgentID:        agentID,
			AccountID:      params.AccountID,
			SessionKey:     sessionKey,
			MainSessionKey: mainSessionKey,
		}, nil
	}
}

func buildTelegramDispatcher(dctx *ChannelDepsContext) func(ctx context.Context, params telegram.TelegramDispatchParams) (*telegram.TelegramDispatchResult, error) {
	return func(ctx context.Context, params telegram.TelegramDispatchParams) (*telegram.TelegramDispatchResult, error) {
		if params.Ctx == nil {
			return nil, fmt.Errorf("MsgContext is required")
		}

		// 确保 session 注册 + 获取 sessionId
		sessionKey := params.Ctx.SessionKey
		var resolvedSessionId string
		if dctx.SessionStore != nil && sessionKey != "" {
			entry := dctx.SessionStore.LoadSessionEntry(sessionKey)
			if entry == nil {
				newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
				entry = &SessionEntry{
					SessionKey: sessionKey,
					SessionId:  newId,
					Label:      sessionKey,
					Channel:    params.Ctx.ChannelType,
					CreatedAt:  time.Now().UnixMilli(),
				}
				dctx.SessionStore.Save(entry)
			}
			resolvedSessionId = entry.SessionId
		}
		params.Ctx.SessionID = resolvedSessionId

		// 持久化用户消息到 transcript
		if resolvedSessionId != "" {
			AppendUserTranscriptMessage(AppendTranscriptParams{
				Message:         params.Ctx.Body,
				SessionID:       resolvedSessionId,
				StorePath:       dctx.StorePath,
				CreateIfMissing: true,
			})
		}

		broadcastChatMessage(dctx.State, params.Ctx, "user")

		result := DispatchInboundMessage(ctx, DispatchInboundParams{
			MsgCtx:     params.Ctx,
			SessionKey: params.Ctx.SessionKey,
			Dispatcher: dctx.Dispatcher,
			OnProgress: buildMsgContextProgressCallback(dctx.State, params.Ctx),
		})
		if result.Error != nil {
			return nil, result.Error
		}

		replyText := CombineReplyPayloads(result.Replies)
		tgMb64, tgMmime := ExtractMediaFromReplies(result.Replies)

		// AI 回复 transcript 由 attempt_runner.persistToTranscript 写入，此处不再双写

		if replyText != "" || tgMb64 != "" {
			broadcastChatReply(dctx.State, params.Ctx, replyText, tgMb64, tgMmime)
		}

		return &telegram.TelegramDispatchResult{
			Replies:     result.Replies,
			QueuedFinal: true,
		}, nil
	}
}

func buildTelegramSessionRecorder(dctx *ChannelDepsContext) func(params telegram.TelegramRecordSessionParams) error {
	return func(params telegram.TelegramRecordSessionParams) error {
		if params.Ctx != nil && params.SessionKey != "" {
			dctx.SessionStore.RecordSessionMeta(params.SessionKey, InboundMeta{
				Channel:     params.Ctx.ChannelType,
				DisplayName: params.Ctx.From,
			})
		}
		if params.UpdateLastRoute != nil {
			dctx.SessionStore.UpdateLastRoute(params.UpdateLastRoute.SessionKey, UpdateLastRouteParams{
				Channel:   params.UpdateLastRoute.Channel,
				To:        params.UpdateLastRoute.To,
				AccountId: params.UpdateLastRoute.AccountID,
			})
		}
		return nil
	}
}

func buildTelegramPairingUpsert(dctx *ChannelDepsContext) func(params telegram.TelegramPairingParams) (*telegram.TelegramPairingResult, error) {
	return func(params telegram.TelegramPairingParams) (*telegram.TelegramPairingResult, error) {
		code, created, err := UpsertChannelPairingRequest(dctx.StorePath, params.Channel, params.ID, params.Meta)
		if err != nil {
			return nil, err
		}
		return &telegram.TelegramPairingResult{Code: code, Created: created}, nil
	}
}

func buildTelegramResetSession(dctx *ChannelDepsContext) func(ctx context.Context, sessionKey, storePath string) error {
	return func(ctx context.Context, sessionKey, storePath string) error {
		dctx.SessionStore.Delete(sessionKey)
		return nil
	}
}

func buildTelegramSwitchModel(dctx *ChannelDepsContext) func(ctx context.Context, sessionKey, storePath, modelRef string) error {
	return func(ctx context.Context, sessionKey, storePath, modelRef string) error {
		slog.Info("channel_deps: model switch requested (not yet implemented)",
			"sessionKey", sessionKey, "model", modelRef)
		return nil
	}
}

func buildTelegramEnqueueEvent(dctx *ChannelDepsContext) func(eventType string, data map[string]interface{}) {
	return func(eventType string, data map[string]interface{}) {
		text := fmt.Sprintf("[%s] %v", eventType, data)
		sessionKey := "main"
		if sk, ok := data["sessionKey"].(string); ok && sk != "" {
			sessionKey = sk
		}
		dctx.EventQueue.Enqueue(text, sessionKey, eventType)
	}
}

func buildTelegramLoadSessionEntry(dctx *ChannelDepsContext) func(sessionKey string) (map[string]string, error) {
	return func(sessionKey string) (map[string]string, error) {
		entry := dctx.SessionStore.LoadSessionEntry(sessionKey)
		if entry == nil {
			return nil, nil
		}
		// 返回基本的 key-value 对
		result := make(map[string]string)
		if entry.Channel != "" {
			result["channel"] = entry.Channel
		}
		if entry.GroupChannel != "" {
			result["groupChannel"] = entry.GroupChannel
		}
		return result, nil
	}
}

// ---------- Slack ----------

// BuildSlackDeps 构建 Slack MonitorDeps（全部真实实现）。
func BuildSlackDeps(dctx *ChannelDepsContext) *slack.SlackMonitorDeps {
	return &slack.SlackMonitorDeps{
		ResolveAgentRoute:      buildSlackRouteResolver(dctx),
		DispatchInboundMessage: buildSlackDispatcher(dctx),
		RecordInboundSession:   buildSlackSessionRecorder(dctx),
		UpsertPairingRequest:   buildSlackPairingUpsert(dctx),
		ReadAllowFromStore:     buildReadAllowFromStore(dctx),
		ResolveStorePath:       buildResolveStorePath(dctx),
		ReadSessionUpdatedAt:   buildReadSessionUpdatedAt(dctx),
		ResolveMedia:           buildResolveMedia(dctx),
		EnqueueSystemEvent:     buildSlackEnqueueEvent(dctx),
	}
}

func buildSlackRouteResolver(dctx *ChannelDepsContext) func(params slack.SlackAgentRouteParams) (*slack.SlackAgentRoute, error) {
	return func(params slack.SlackAgentRouteParams) (*slack.SlackAgentRoute, error) {
		agentID := routing.DefaultAgentID
		sessionKey := routing.BuildAgentPeerSessionKey(routing.PeerSessionKeyParams{
			AgentID:   agentID,
			MainKey:   routing.DefaultMainKey,
			Channel:   params.Channel,
			AccountID: params.AccountID,
			PeerKind:  params.PeerKind,
			PeerID:    params.PeerID,
		})
		mainSessionKey := routing.BuildAgentMainSessionKey(agentID, routing.DefaultMainKey)
		return &slack.SlackAgentRoute{
			AgentID:        agentID,
			AccountID:      params.AccountID,
			SessionKey:     sessionKey,
			MainSessionKey: mainSessionKey,
		}, nil
	}
}

func buildSlackDispatcher(dctx *ChannelDepsContext) func(ctx context.Context, params slack.SlackDispatchParams) (*slack.SlackDispatchResult, error) {
	return func(ctx context.Context, params slack.SlackDispatchParams) (*slack.SlackDispatchResult, error) {
		if params.Ctx == nil {
			return nil, fmt.Errorf("MsgContext is required")
		}

		// 确保 session 注册 + 获取 sessionId
		sessionKey := params.Ctx.SessionKey
		var resolvedSessionId string
		if dctx.SessionStore != nil && sessionKey != "" {
			entry := dctx.SessionStore.LoadSessionEntry(sessionKey)
			if entry == nil {
				newId := fmt.Sprintf("session_%d", time.Now().UnixNano())
				entry = &SessionEntry{
					SessionKey: sessionKey,
					SessionId:  newId,
					Label:      sessionKey,
					Channel:    params.Ctx.ChannelType,
					CreatedAt:  time.Now().UnixMilli(),
				}
				dctx.SessionStore.Save(entry)
			}
			resolvedSessionId = entry.SessionId
		}
		params.Ctx.SessionID = resolvedSessionId

		// 持久化用户消息到 transcript
		if resolvedSessionId != "" {
			AppendUserTranscriptMessage(AppendTranscriptParams{
				Message:         params.Ctx.Body,
				SessionID:       resolvedSessionId,
				StorePath:       dctx.StorePath,
				CreateIfMissing: true,
			})
		}

		broadcastChatMessage(dctx.State, params.Ctx, "user")

		result := DispatchInboundMessage(ctx, DispatchInboundParams{
			MsgCtx:     params.Ctx,
			SessionKey: params.Ctx.SessionKey,
			Dispatcher: dctx.Dispatcher,
			OnProgress: buildMsgContextProgressCallback(dctx.State, params.Ctx),
		})
		if result.Error != nil {
			return nil, result.Error
		}

		replyText := CombineReplyPayloads(result.Replies)
		slMb64, slMmime := ExtractMediaFromReplies(result.Replies)

		// AI 回复 transcript 由 attempt_runner.persistToTranscript 写入，此处不再双写

		if replyText != "" || slMb64 != "" {
			broadcastChatReply(dctx.State, params.Ctx, replyText, slMb64, slMmime)
		}

		return &slack.SlackDispatchResult{QueuedFinal: true}, nil
	}
}

func buildSlackSessionRecorder(dctx *ChannelDepsContext) func(params slack.SlackRecordSessionParams) error {
	return func(params slack.SlackRecordSessionParams) error {
		if params.Ctx != nil && params.SessionKey != "" {
			dctx.SessionStore.RecordSessionMeta(params.SessionKey, InboundMeta{
				Channel:     params.Ctx.ChannelType,
				DisplayName: params.Ctx.From,
			})
		}
		if params.UpdateLastRoute != nil {
			dctx.SessionStore.UpdateLastRoute(params.UpdateLastRoute.SessionKey, UpdateLastRouteParams{
				Channel:   params.UpdateLastRoute.Channel,
				To:        params.UpdateLastRoute.To,
				AccountId: params.UpdateLastRoute.AccountID,
			})
		}
		return nil
	}
}

func buildSlackPairingUpsert(dctx *ChannelDepsContext) func(params slack.SlackPairingRequestParams) (*slack.SlackPairingResult, error) {
	return func(params slack.SlackPairingRequestParams) (*slack.SlackPairingResult, error) {
		code, created, err := UpsertChannelPairingRequest(dctx.StorePath, params.Channel, params.ID, params.Meta)
		if err != nil {
			return nil, err
		}
		return &slack.SlackPairingResult{Code: code, Created: created}, nil
	}
}

func buildSlackEnqueueEvent(dctx *ChannelDepsContext) func(text, sessionKey, contextKey string) error {
	return func(text, sessionKey, contextKey string) error {
		dctx.EventQueue.Enqueue(text, sessionKey, contextKey)
		return nil
	}
}

// ---------- 共享回调 ----------

func buildReadAllowFromStore(dctx *ChannelDepsContext) func(channel string) ([]string, error) {
	return func(channel string) ([]string, error) {
		return ReadChannelPairingAllowlist(dctx.StorePath, channel)
	}
}

func buildResolveStorePath(dctx *ChannelDepsContext) func(agentID string) string {
	return func(agentID string) string {
		return dctx.StorePath
	}
}

func buildReadSessionUpdatedAt(dctx *ChannelDepsContext) func(storePath, sessionKey string) *int64 {
	return func(storePath, sessionKey string) *int64 {
		entry := dctx.SessionStore.LoadSessionEntry(sessionKey)
		if entry == nil {
			return nil
		}
		ts := entry.UpdatedAt
		if ts <= 0 {
			return nil
		}
		return &ts
	}
}

func buildResolveMedia(dctx *ChannelDepsContext) func(mediaURL string, maxBytes int) (string, string, error) {
	return func(mediaURL string, maxBytes int) (string, string, error) {
		if maxBytes <= 0 {
			maxBytes = 8 * 1024 * 1024
		}
		result, err := media.FetchRemoteMedia(media.FetchMediaOptions{
			URL:      mediaURL,
			MaxBytes: int64(maxBytes),
		})
		if err != nil {
			return "", "", err
		}
		saved, err := media.SaveMediaBuffer(result.Buffer, result.ContentType, "channel-media", int64(maxBytes), result.FileName)
		if err != nil {
			return "", "", err
		}
		return saved.Path, result.ContentType, nil
	}
}

func buildRecordChannelActivity(dctx *ChannelDepsContext) func(channel, accountID, direction string) {
	return func(channel, accountID, direction string) {
		slog.Debug("channel_activity", "channel", channel, "account", accountID, "direction", direction)
	}
}

func buildResetSession(dctx *ChannelDepsContext) func(ctx context.Context, accountID, channelID, senderID string) error {
	return func(ctx context.Context, accountID, channelID, senderID string) error {
		// 构建 session key 并删除
		sessionKey := routing.BuildAgentPeerSessionKey(routing.PeerSessionKeyParams{
			AgentID:   routing.DefaultAgentID,
			MainKey:   routing.DefaultMainKey,
			Channel:   "discord",
			AccountID: accountID,
			PeerKind:  "direct",
			PeerID:    senderID,
		})
		dctx.SessionStore.Delete(sessionKey)
		return nil
	}
}

func buildSwitchModel(dctx *ChannelDepsContext) func(ctx context.Context, accountID, modelName string) error {
	return func(ctx context.Context, accountID, modelName string) error {
		slog.Info("channel_deps: model switch requested",
			"account", accountID, "model", modelName)
		return nil
	}
}

// ---------- 前端广播辅助 ----------

func broadcastChatMessage(state *GatewayState, msgCtx *autoreply.MsgContext, role string) {
	bc := state.Broadcaster()
	if bc == nil || msgCtx == nil {
		return
	}
	channel := strings.ToLower(msgCtx.ChannelType)
	if channel == "" {
		channel = "unknown"
	}
	bc.Broadcast("chat.message", map[string]interface{}{
		"sessionKey": msgCtx.SessionKey,
		"channel":    channel,
		"role":       role,
		"text":       msgCtx.Body,
		"from":       msgCtx.SenderID,
		"ts":         time.Now().UnixMilli(),
	}, nil)
}

func broadcastChatReply(state *GatewayState, msgCtx *autoreply.MsgContext, replyText, mediaB64, mediaMime string) {
	bc := state.Broadcaster()
	if bc == nil || msgCtx == nil {
		return
	}
	channel := strings.ToLower(msgCtx.ChannelType)
	if channel == "" {
		channel = "unknown"
	}
	payload := map[string]interface{}{
		"sessionKey": msgCtx.SessionKey,
		"channel":    channel,
		"role":       "assistant",
		"text":       replyText,
		"ts":         time.Now().UnixMilli(),
	}
	if mediaB64 != "" {
		payload["mediaBase64"] = mediaB64
		payload["mediaMimeType"] = mediaMime
	}
	bc.Broadcast("chat.message", payload, nil)
}
