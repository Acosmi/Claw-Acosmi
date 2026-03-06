package wecom

// plugin.go — 企业微信频道插件
// 实现 channels.Plugin 接口

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// WeComPlugin 企业微信频道插件
type WeComPlugin struct {
	config *types.OpenAcosmiConfig
	mu     sync.Mutex

	clients  map[string]*WeComClient
	senders  map[string]*WeComSender
	handlers map[string]*CallbackHandler

	// DispatchFunc 消息分发回调 — 由 gateway 注入，路由到 autoreply 管线
	DispatchFunc func(ctx context.Context, channel, accountID, chatID, userID, text string) string

	// DispatchMultimodalFunc 多模态消息分发回调（Phase A 新增）
	// 优先使用：如未设置则回退 DispatchFunc(text)
	DispatchMultimodalFunc channels.DispatchMultimodalFunc
}

// NewWeComPlugin 创建企业微信插件
func NewWeComPlugin(cfg *types.OpenAcosmiConfig) *WeComPlugin {
	return &WeComPlugin{
		config:   cfg,
		clients:  make(map[string]*WeComClient),
		senders:  make(map[string]*WeComSender),
		handlers: make(map[string]*CallbackHandler),
	}
}

// ID 返回频道标识
func (p *WeComPlugin) ID() channels.ChannelID {
	return channels.ChannelWeCom
}

// UpdateConfig 实现 channels.ConfigUpdater 接口。
// 热重载时注入新配置，后续 Start() 将使用新凭证（CorpID/Secret/AgentID）重建客户端。
func (p *WeComPlugin) UpdateConfig(cfg interface{}) {
	if c, ok := cfg.(*types.OpenAcosmiConfig); ok {
		p.mu.Lock()
		p.config = c
		p.mu.Unlock()
	}
}

// Start 启动企业微信频道
func (p *WeComPlugin) Start(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	acct := ResolveWeComAccount(p.config, accountID)
	if acct == nil {
		return fmt.Errorf("wecom account %q not found in config", accountID)
	}

	if err := ValidateWeComConfig(acct.Config); err != nil {
		return fmt.Errorf("wecom config validation: %w", err)
	}

	if !channels.IsAccountEnabled(acct.Config.Enabled) {
		slog.Info("wecom account disabled, skipping", "account", accountID)
		return nil
	}

	// HTTP 客户端
	client := NewWeComClient(acct)
	p.clients[accountID] = client

	// 消息发送器
	p.senders[accountID] = NewWeComSender(client, client.AgentID)

	// 回调处理器
	capturedAccountID := accountID
	handler := NewCallbackHandler(CallbackHandlerConfig{
		Client: client,
		OnMessage: func(msg *channels.ChannelMessage, fromUser string) {
			msgType := ""
			content := ""
			if msg != nil {
				msgType = msg.MessageType
				content = msg.Text
			}
			slog.Info("wecom message received (plugin)",
				"account", capturedAccountID,
				"msg_type", msgType,
				"from_user", fromUser,
				"content", content,
			)

			// 路由到 Agent 管线获取回复（优先多模态）
			var dispatchReply *channels.DispatchReply
			if p.DispatchMultimodalFunc != nil {
				dispatchReply = p.DispatchMultimodalFunc(
					"wecom", capturedAccountID, fromUser, fromUser, msg)
			} else if p.DispatchFunc != nil {
				replyText := p.DispatchFunc(context.Background(),
					"wecom", capturedAccountID, fromUser, fromUser, content)
				if replyText != "" {
					dispatchReply = &channels.DispatchReply{Text: replyText}
				}
			} else {
				slog.Warn("wecom: DispatchFunc not set, message not routed to agent",
					"account", capturedAccountID)
			}

			if dispatchReply != nil {
				sender := p.GetSender(capturedAccountID)
				if sender != nil {
					if err := p.sendDispatchReplyMedia(context.Background(), sender, dispatchReply, fromUser); err != nil {
						slog.Warn("wecom: failed to send media reply",
							"account", capturedAccountID, "to", fromUser, "error", err)
					}
					if dispatchReply.Text != "" {
						if err := sender.SendText(context.Background(), fromUser, dispatchReply.Text); err != nil {
							slog.Error("wecom: failed to send reply",
								"account", capturedAccountID, "to", fromUser, "error", err)
						}
					}
				}
			}
		},
	})
	p.handlers[accountID] = handler

	slog.Info("wecom channel started",
		"account", accountID,
		"corp_id", acct.Config.CorpID,
		"agent_id", client.AgentID,
	)
	return nil
}

// Stop 停止企业微信频道
func (p *WeComPlugin) Stop(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	delete(p.clients, accountID)
	delete(p.senders, accountID)
	delete(p.handlers, accountID)

	slog.Info("wecom channel stopped", "account", accountID)
	return nil
}

// GetSender 获取消息发送器
func (p *WeComPlugin) GetSender(accountID string) *WeComSender {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.senders[accountID]
}

// GetClient 获取 API 客户端（用于资源下载等场景）。
func (p *WeComPlugin) GetClient(accountID string) *WeComClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.clients[accountID]
}

// GetCallbackHandler 获取回调处理器
func (p *WeComPlugin) GetCallbackHandler(accountID string) *CallbackHandler {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.handlers[accountID]
}

// SendMessage 实现 channels.MessageSender 接口。
func (p *WeComPlugin) SendMessage(params channels.OutboundSendParams) (*channels.OutboundSendResult, error) {
	accountID := params.AccountID
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	sender := p.GetSender(accountID)
	if sender == nil {
		return nil, channels.NewSendError(channels.ChannelWeCom, channels.SendErrUnavailable,
			fmt.Sprintf("wecom sender not available for account %s", accountID)).
			WithOperation("send.init").
			WithRetryable(true).
			WithDetails(map[string]interface{}{"accountId": accountID})
	}

	ctx := params.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	outboundText := strings.TrimSpace(params.Text)
	if mediaURLFallback := wecomMediaURLFallbackText(params.MediaURL, len(params.MediaData) > 0); mediaURLFallback != "" {
		if outboundText == "" {
			outboundText = mediaURLFallback
		} else {
			outboundText += "\n" + mediaURLFallback
		}
	}

	mediaSent := false
	if len(params.MediaData) > 0 {
		if err := sender.SendBinaryMedia(ctx, params.To, params.MediaData, params.MediaMimeType, params.MediaFileName); err != nil {
			slog.Warn("wecom: binary media send failed, fallback to text",
				"to", params.To, "mimeType", params.MediaMimeType, "error", err)
			if outboundText == "" {
				return nil, channels.WrapSendError(channels.ChannelWeCom, channels.SendErrUpstream,
					"send.media", "wecom binary media send failed", err).
					WithRetryable(true).
					WithDetails(map[string]interface{}{
						"to":       params.To,
						"mimeType": params.MediaMimeType,
					})
			}
		} else {
			mediaSent = true
		}
	}

	textSent := false
	if outboundText != "" {
		if err := sender.SendText(ctx, params.To, outboundText); err != nil {
			return nil, channels.WrapSendError(channels.ChannelWeCom, channels.SendErrUpstream,
				"send.text", "wecom text send failed", err).
				WithRetryable(true).
				WithDetails(map[string]interface{}{"to": params.To})
		}
		textSent = true
	}

	if !textSent && !mediaSent {
		return nil, channels.NewSendError(channels.ChannelWeCom, channels.SendErrInvalidRequest,
			"wecom: empty outbound message").
			WithOperation("send.validate").
			WithDetails(map[string]interface{}{
				"to":           params.To,
				"hasMediaData": len(params.MediaData) > 0,
				"hasMediaURL":  strings.TrimSpace(params.MediaURL) != "",
			})
	}

	return &channels.OutboundSendResult{
		Channel: string(channels.ChannelWeCom),
		ChatID:  params.To,
	}, nil
}

func (p *WeComPlugin) sendDispatchReplyMedia(ctx context.Context, sender *WeComSender, reply *channels.DispatchReply, toUser string) error {
	if reply == nil || sender == nil {
		return nil
	}

	if len(reply.MediaItems) > 0 {
		for _, item := range reply.MediaItems {
			if len(item.Data) == 0 {
				continue
			}
			if err := sender.SendBinaryMedia(ctx, toUser, item.Data, item.MimeType, ""); err != nil {
				return err
			}
		}
		return nil
	}

	if len(reply.MediaData) > 0 {
		return sender.SendBinaryMedia(ctx, toUser, reply.MediaData, reply.MediaMimeType, "")
	}
	return nil
}

func wecomMediaURLFallbackText(mediaURL string, hasMediaData bool) string {
	if hasMediaData {
		return ""
	}
	urlText := strings.TrimSpace(mediaURL)
	if urlText == "" {
		return ""
	}
	return "[该消息包含媒体链接，当前企微通道暂不支持自动抓取外部媒体]\n媒体链接：" + urlText
}
