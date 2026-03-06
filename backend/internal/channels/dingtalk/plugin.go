package dingtalk

// plugin.go — 钉钉频道插件（Stream SDK 模式）
// 实现 channels.Plugin 接口
// 使用 dingtalk-stream-sdk-go 建立长连接接收消息

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	dingtalkstream "github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

type dingTalkMessageSender interface {
	SendGroupMessage(ctx context.Context, openConversationID, text string) error
	SendOToMessage(ctx context.Context, userIDs []string, text string) error
	SendGroupImage(ctx context.Context, openConversationID, photoURL string) error
	SendOToImage(ctx context.Context, userIDs []string, photoURL string) error
	SendGroupFile(ctx context.Context, openConversationID, mediaID, fileName, fileType string) error
	SendOToFile(ctx context.Context, userIDs []string, mediaID, fileName, fileType string) error
	UploadMedia(ctx context.Context, mediaType, fileName string, data []byte) (string, error)
}

// DingTalkPlugin 钉钉频道插件
type DingTalkPlugin struct {
	config *types.OpenAcosmiConfig
	mu     sync.Mutex

	streams map[string]*DingTalkStreamClient
	senders map[string]dingTalkMessageSender

	// DispatchFunc 消息分发回调 — 由 gateway 注入，路由到 autoreply 管线
	DispatchFunc func(ctx context.Context, channel, accountID, chatID, userID, text string) string

	// DispatchMultimodalFunc 多模态消息分发回调（Phase A 新增）
	// 优先使用：如未设置则回退 DispatchFunc(text)
	DispatchMultimodalFunc channels.DispatchMultimodalFunc
}

// NewDingTalkPlugin 创建钉钉插件
func NewDingTalkPlugin(cfg *types.OpenAcosmiConfig) *DingTalkPlugin {
	return &DingTalkPlugin{
		config:  cfg,
		streams: make(map[string]*DingTalkStreamClient),
		senders: make(map[string]dingTalkMessageSender),
	}
}

// ID 返回频道标识
func (p *DingTalkPlugin) ID() channels.ChannelID {
	return channels.ChannelDingTalk
}

// UpdateConfig 实现 channels.ConfigUpdater 接口。
// 热重载时注入新配置，后续 Start() 将使用新凭证（AppKey/AppSecret）建立 Stream 连接。
func (p *DingTalkPlugin) UpdateConfig(cfg interface{}) {
	if c, ok := cfg.(*types.OpenAcosmiConfig); ok {
		p.mu.Lock()
		p.config = c
		p.mu.Unlock()
	}
}

// Start 启动钉钉频道
func (p *DingTalkPlugin) Start(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	acct := ResolveDingTalkAccount(p.config, accountID)
	if acct == nil {
		return fmt.Errorf("dingtalk account %q not found in config", accountID)
	}

	if err := ValidateDingTalkConfig(acct.Config); err != nil {
		return fmt.Errorf("dingtalk config validation: %w", err)
	}

	if !channels.IsAccountEnabled(acct.Config.Enabled) {
		slog.Info("dingtalk account disabled, skipping", "account", accountID)
		return nil
	}

	// Stream 客户端（接收消息 — 长连接）
	stream := NewDingTalkStreamClient(acct)
	capturedAccountID := accountID
	stream.SetMessageHandler(func(data *dingtalkstream.BotCallbackDataModel) {
		LogCallbackInfo(data)
		cm := ExtractMultimodalMessageFromCallback(data)
		text := ""
		if cm != nil {
			text = cm.Text
		}
		chatID := data.ConversationId
		userID := data.SenderId
		slog.Info("dingtalk message received (plugin)",
			"account", capturedAccountID,
			"chat_id", chatID,
			"user_id", userID,
			"text", text,
		)

		// 路由到 Agent 管线获取回复（优先多模态）
		var dispatchReply *channels.DispatchReply
		if p.DispatchMultimodalFunc != nil {
			dispatchReply = p.DispatchMultimodalFunc(
				"dingtalk", capturedAccountID, chatID, userID, cm)
		} else if p.DispatchFunc != nil {
			replyText := p.DispatchFunc(context.Background(),
				"dingtalk", capturedAccountID, chatID, userID, text)
			if replyText != "" {
				dispatchReply = &channels.DispatchReply{Text: replyText}
			}
		} else {
			slog.Warn("dingtalk: DispatchFunc not set, message not routed to agent",
				"account", capturedAccountID)
		}

		if dispatchReply != nil {
			replyText := strings.TrimSpace(dispatchReply.Text)
			sender := p.GetSender(capturedAccountID)
			if sender != nil {
				if err := p.sendDispatchReplyMedia(
					context.Background(),
					sender,
					dispatchReply,
					chatID,
					userID,
					strings.HasPrefix(chatID, "cid"),
				); err != nil {
					slog.Warn("dingtalk: failed to send media reply",
						"account", capturedAccountID,
						"chat_id", chatID,
						"user_id", userID,
						"error", err,
					)
					if fallback := dingtalkMediaFallbackFromDispatchReply(dispatchReply); fallback != "" {
						if replyText == "" {
							replyText = fallback
						} else {
							replyText += "\n" + fallback
						}
					}
				}
			}
			if replyText == "" {
				return
			}
			// 发送回复（根据会话类型选择群消息或单聊消息）
			if sender != nil {
				if strings.HasPrefix(chatID, "cid") {
					if err := sender.SendGroupMessage(context.Background(), chatID, replyText); err != nil {
						slog.Error("dingtalk: failed to send group reply",
							"account", capturedAccountID, "chat_id", chatID, "error", err)
					}
				} else {
					if err := sender.SendOToMessage(context.Background(), []string{userID}, replyText); err != nil {
						slog.Error("dingtalk: failed to send reply",
							"account", capturedAccountID, "user_id", userID, "error", err)
					}
				}
			}
		}
	})

	if err := stream.Start(context.Background()); err != nil {
		return fmt.Errorf("start dingtalk stream: %w", err)
	}
	p.streams[accountID] = stream

	// 消息发送器（发送消息 — HTTP API）
	sender := NewDingTalkSender(acct.Config.AppKey, acct.Config.AppSecret, acct.Config.RobotCode)
	p.senders[accountID] = sender

	slog.Info("dingtalk channel started",
		"account", accountID,
		"app_key", acct.Config.AppKey,
		"robot_code", acct.Config.RobotCode,
	)
	return nil
}

// Stop 停止钉钉频道
func (p *DingTalkPlugin) Stop(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	if stream, ok := p.streams[accountID]; ok {
		stream.Stop()
		delete(p.streams, accountID)
	}
	delete(p.senders, accountID)

	slog.Info("dingtalk channel stopped", "account", accountID)
	return nil
}

// GetSender 获取消息发送器
func (p *DingTalkPlugin) GetSender(accountID string) dingTalkMessageSender {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.senders[accountID]
}

// SendMessage 实现 channels.MessageSender 接口。
// 根据 To 格式自动路由：包含 "cid" 前缀时使用群消息，否则使用单聊消息。
func (p *DingTalkPlugin) SendMessage(params channels.OutboundSendParams) (*channels.OutboundSendResult, error) {
	accountID := params.AccountID
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	sender := p.GetSender(accountID)
	if sender == nil {
		return nil, channels.NewSendError(channels.ChannelDingTalk, channels.SendErrUnavailable,
			fmt.Sprintf("dingtalk sender not available for account %s", accountID)).
			WithOperation("send.init").
			WithRetryable(true).
			WithDetails(map[string]interface{}{"accountId": accountID})
	}

	ctx := params.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	isGroup := strings.HasPrefix(params.To, "cid")
	outboundText := strings.TrimSpace(params.Text)
	mediaURLText := strings.TrimSpace(params.MediaURL)
	canSendImageURL := canSendDingTalkImageURL(mediaURLText, params.MediaMimeType)
	unsentMediaURL := mediaURLText != "" && !canSendImageURL
	unsentBinaryMedia := len(params.MediaData) > 0
	textSent := false
	mediaSent := false

	sendText := func(text, operation string) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		if isGroup {
			if err := sender.SendGroupMessage(ctx, params.To, text); err != nil {
				return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
					operation, "dingtalk group text send failed", err).
					WithRetryable(true).
					WithDetails(map[string]interface{}{"to": params.To})
			}
			return nil
		}
		if err := sender.SendOToMessage(ctx, []string{params.To}, text); err != nil {
			return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
				operation, "dingtalk o2o text send failed", err).
				WithRetryable(true).
				WithDetails(map[string]interface{}{"to": params.To})
		}
		return nil
	}

	if outboundText != "" {
		operation := "send.o2o.text"
		if isGroup {
			operation = "send.group.text"
		}
		if err := sendText(outboundText, operation); err != nil {
			return nil, err
		}
		textSent = true
	}

	if canSendImageURL {
		var imageErr error
		if isGroup {
			imageErr = sender.SendGroupImage(ctx, params.To, mediaURLText)
		} else {
			imageErr = sender.SendOToImage(ctx, []string{params.To}, mediaURLText)
		}
		if imageErr != nil {
			slog.Warn("dingtalk: send image by URL failed",
				"to", params.To, "is_group", isGroup, "error", imageErr)
			unsentMediaURL = true
		} else {
			unsentMediaURL = false
			mediaSent = true
		}
	}

	if unsentBinaryMedia {
		if err := p.sendBinaryMediaMessage(ctx, sender, params.To, isGroup, params.MediaData, params.MediaFileName, params.MediaMimeType); err != nil {
			slog.Warn("dingtalk: send binary media failed",
				"to", params.To, "is_group", isGroup, "mimeType", params.MediaMimeType, "error", err)
		} else {
			unsentBinaryMedia = false
			mediaSent = true
		}
	}

	if !textSent && (unsentBinaryMedia || unsentMediaURL) {
		fallbackParams := params
		if !unsentBinaryMedia {
			fallbackParams.MediaData = nil
			fallbackParams.MediaMimeType = ""
		}
		if !unsentMediaURL {
			fallbackParams.MediaURL = ""
		}
		if fallbackText := dingtalkMediaFallbackFromOutbound(fallbackParams, unsentMediaURL); strings.TrimSpace(fallbackText) != "" {
			operation := "send.o2o.fallback_text"
			if isGroup {
				operation = "send.group.fallback_text"
			}
			if err := sendText(fallbackText, operation); err != nil {
				return nil, err
			}
			textSent = true
		}
	}

	if !textSent && !mediaSent {
		return nil, channels.NewSendError(channels.ChannelDingTalk, channels.SendErrInvalidRequest,
			"dingtalk: nothing sent").
			WithOperation("send.validate").
			WithDetails(map[string]interface{}{
				"to":                 params.To,
				"hasMediaData":       len(params.MediaData) > 0,
				"hasMediaURL":        mediaURLText != "",
				"canSendImageURL":    canSendImageURL,
				"unsentBinaryMedia":  unsentBinaryMedia,
				"unsentMediaURLText": unsentMediaURL,
			})
	}

	return &channels.OutboundSendResult{
		Channel: string(channels.ChannelDingTalk),
		ChatID:  params.To,
	}, nil
}

func (p *DingTalkPlugin) sendBinaryMediaMessage(
	ctx context.Context,
	sender dingTalkMessageSender,
	to string,
	isGroup bool,
	data []byte,
	fileName string,
	mimeType string,
) error {
	if len(data) == 0 {
		return channels.NewSendError(channels.ChannelDingTalk, channels.SendErrInvalidRequest,
			"dingtalk: empty media payload").
			WithOperation("send.media.validate").
			WithDetails(map[string]interface{}{"to": to})
	}

	effectiveMimeType := strings.TrimSpace(mimeType)
	if effectiveMimeType == "" {
		effectiveMimeType = strings.ToLower(http.DetectContentType(data))
	}
	uploadType := dingtalkUploadTypeForMime(effectiveMimeType)
	if strings.TrimSpace(fileName) == "" {
		fileName = dingtalkDefaultUploadFileName(uploadType, effectiveMimeType)
	}

	mediaID, err := sender.UploadMedia(ctx, uploadType, fileName, data)
	if err != nil {
		return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
			"send.media.upload", "dingtalk media upload failed", err).
			WithRetryable(true).
			WithDetails(map[string]interface{}{
				"to":         to,
				"uploadType": uploadType,
				"mimeType":   effectiveMimeType,
			})
	}

	if uploadType == "image" {
		if isGroup {
			if err := sender.SendGroupImage(ctx, to, mediaID); err != nil {
				return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
					"send.group.image_media", "dingtalk group image send failed", err).
					WithRetryable(true).
					WithDetails(map[string]interface{}{
						"to":       to,
						"mediaId":  mediaID,
						"mimeType": effectiveMimeType,
					})
			}
			return nil
		}
		if err := sender.SendOToImage(ctx, []string{to}, mediaID); err != nil {
			return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
				"send.o2o.image_media", "dingtalk o2o image send failed", err).
				WithRetryable(true).
				WithDetails(map[string]interface{}{
					"to":       to,
					"mediaId":  mediaID,
					"mimeType": effectiveMimeType,
				})
		}
		return nil
	}

	fileType := dingtalkFileTypeFromName(fileName)
	if isGroup {
		if err := sender.SendGroupFile(ctx, to, mediaID, fileName, fileType); err != nil {
			return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
				"send.group.file", "dingtalk group file send failed", err).
				WithRetryable(true).
				WithDetails(map[string]interface{}{
					"to":       to,
					"mediaId":  mediaID,
					"fileName": fileName,
					"fileType": fileType,
				})
		}
		return nil
	}
	if err := sender.SendOToFile(ctx, []string{to}, mediaID, fileName, fileType); err != nil {
		return channels.WrapSendError(channels.ChannelDingTalk, channels.SendErrUpstream,
			"send.o2o.file", "dingtalk o2o file send failed", err).
			WithRetryable(true).
			WithDetails(map[string]interface{}{
				"to":       to,
				"mediaId":  mediaID,
				"fileName": fileName,
				"fileType": fileType,
			})
	}
	return nil
}

func (p *DingTalkPlugin) sendDispatchReplyMedia(
	ctx context.Context,
	sender dingTalkMessageSender,
	reply *channels.DispatchReply,
	chatID string,
	userID string,
	isGroup bool,
) error {
	if sender == nil || reply == nil {
		return nil
	}
	to := strings.TrimSpace(userID)
	if isGroup {
		to = strings.TrimSpace(chatID)
	}
	if to == "" {
		return channels.NewSendError(channels.ChannelDingTalk, channels.SendErrInvalidRequest,
			"dingtalk: empty dispatch reply target").
			WithOperation("send.reply_media.validate")
	}

	if len(reply.MediaItems) > 0 {
		for _, item := range reply.MediaItems {
			if len(item.Data) == 0 {
				continue
			}
			if err := p.sendBinaryMediaMessage(ctx, sender, to, isGroup, item.Data, "", item.MimeType); err != nil {
				return err
			}
		}
		return nil
	}

	if len(reply.MediaData) > 0 {
		return p.sendBinaryMediaMessage(ctx, sender, to, isGroup, reply.MediaData, "", reply.MediaMimeType)
	}
	return nil
}

func dingtalkUploadTypeForMime(mimeType string) string {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(normalized, "image/") {
		return "image"
	}
	return "file"
}

func dingtalkDefaultUploadFileName(uploadType, mimeType string) string {
	if exts, err := mime.ExtensionsByType(strings.TrimSpace(mimeType)); err == nil && len(exts) > 0 {
		return "upload" + exts[0]
	}
	if uploadType == "image" {
		return "upload.png"
	}
	return "upload.bin"
}

func dingtalkFileTypeFromName(fileName string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if ext == "" {
		return "bin"
	}
	return ext
}

func canSendDingTalkImageURL(mediaURL, mimeType string) bool {
	urlText := strings.TrimSpace(mediaURL)
	if urlText == "" {
		return false
	}
	lowerURL := strings.ToLower(urlText)
	if !strings.HasPrefix(lowerURL, "https://") && !strings.HasPrefix(lowerURL, "http://") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return true
	}
	parsed, err := url.Parse(urlText)
	pathText := lowerURL
	if err == nil {
		pathText = strings.ToLower(parsed.Path)
	}
	return strings.HasSuffix(pathText, ".png") ||
		strings.HasSuffix(pathText, ".jpg") ||
		strings.HasSuffix(pathText, ".jpeg") ||
		strings.HasSuffix(pathText, ".gif") ||
		strings.HasSuffix(pathText, ".webp") ||
		strings.HasSuffix(pathText, ".bmp")
}

func dingtalkMediaFallbackFromDispatchReply(reply *channels.DispatchReply) string {
	if reply == nil {
		return ""
	}
	items := append([]channels.ChannelMediaItem{}, reply.MediaItems...)
	if len(items) == 0 && len(reply.MediaData) > 0 {
		items = append(items, channels.ChannelMediaItem{
			Data:     reply.MediaData,
			MimeType: reply.MediaMimeType,
		})
	}
	if len(items) == 0 {
		return ""
	}

	mimes := make([]string, 0, len(items))
	for _, item := range items {
		mime := strings.TrimSpace(item.MimeType)
		if mime == "" {
			mime = "application/octet-stream"
		}
		mimes = append(mimes, mime)
	}
	return fmt.Sprintf("[检测到 %d 个媒体附件（%s），媒体回传失败，已降级为文本提示]",
		len(items), summarizeMimeTypes(mimes))
}

func dingtalkMediaFallbackFromOutbound(params channels.OutboundSendParams, includeMediaURL bool) string {
	mediaCount := 0
	mimes := make([]string, 0, 1)

	if len(params.MediaData) > 0 {
		mediaCount++
		mime := strings.TrimSpace(params.MediaMimeType)
		if mime == "" {
			mime = "application/octet-stream"
		}
		mimes = append(mimes, mime)
	}
	if includeMediaURL && strings.TrimSpace(params.MediaURL) != "" {
		mediaCount++
	}
	if mediaCount == 0 {
		return ""
	}

	base := "[该消息包含媒体附件，媒体发送失败或类型暂不支持，已降级为文本提示"
	if len(mimes) > 0 {
		base += "（" + summarizeMimeTypes(mimes) + "）"
	}
	base += "]"
	if includeMediaURL {
		if urlText := strings.TrimSpace(params.MediaURL); urlText != "" {
			base += "\n" + "媒体链接：" + urlText
		}
	}
	return base
}

func summarizeMimeTypes(mimes []string) string {
	if len(mimes) == 0 {
		return "unknown"
	}
	counts := map[string]int{}
	for _, mime := range mimes {
		trimmed := strings.TrimSpace(mime)
		if trimmed == "" {
			trimmed = "unknown"
		}
		counts[trimmed]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if counts[key] <= 1 {
			parts = append(parts, key)
		} else {
			parts = append(parts, fmt.Sprintf("%s×%d", key, counts[key]))
		}
	}
	return strings.Join(parts, ", ")
}
