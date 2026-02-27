package feishu

// plugin.go — 飞书频道插件（SDK 模式）
// 实现 channels.Plugin 接口

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/anthropic/open-acosmi/internal/channels"
	"github.com/anthropic/open-acosmi/pkg/types"
)

// FeishuPlugin 飞书频道插件
type FeishuPlugin struct {
	config *types.OpenAcosmiConfig
	mu     sync.Mutex

	clients     map[string]*FeishuClient
	senders     map[string]*FeishuSender
	dispatchers map[string]*dispatcher.EventDispatcher
	wsClients   map[string]*larkws.Client
	wsCancel    map[string]context.CancelFunc

	// seenMessages 去重缓存：message_id → 收到时间
	seenMessages sync.Map

	// DispatchFunc 消息分发回调 — 由 gateway 注入，路由到 autoreply 管线
	DispatchFunc func(ctx context.Context, channel, accountID, chatID, userID, text string) string

	// DispatchMultimodalFunc 多模态消息分发回调（Phase A 新增）
	// 优先使用：如未设置则回退 DispatchFunc(text)
	DispatchMultimodalFunc channels.DispatchMultimodalFunc

	// CardActionFunc 卡片回传交互回调 — 由 gateway 注入，处理审批按钮点击
	// 通过 WebSocket 长连接接收，无需公网地址。
	CardActionFunc CardActionHandler
}

// NewFeishuPlugin 创建飞书插件
func NewFeishuPlugin(cfg *types.OpenAcosmiConfig) *FeishuPlugin {
	return &FeishuPlugin{
		config:      cfg,
		clients:     make(map[string]*FeishuClient),
		senders:     make(map[string]*FeishuSender),
		dispatchers: make(map[string]*dispatcher.EventDispatcher),
		wsClients:   make(map[string]*larkws.Client),
		wsCancel:    make(map[string]context.CancelFunc),
	}
}

// ID 返回频道标识
func (p *FeishuPlugin) ID() channels.ChannelID {
	return channels.ChannelFeishu
}

// GetClient 返回指定账号的飞书客户端（可能为 nil）。
func (p *FeishuPlugin) GetClient(accountID string) *FeishuClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clients[accountID]
}

// Start 启动飞书频道
func (p *FeishuPlugin) Start(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	acct := ResolveFeishuAccount(p.config, accountID)
	if acct == nil {
		return fmt.Errorf("feishu account %q not found in config", accountID)
	}

	if err := ValidateFeishuConfig(acct.Config); err != nil {
		return fmt.Errorf("feishu config validation: %w", err)
	}

	if !channels.IsAccountEnabled(acct.Config.Enabled) {
		slog.Info("feishu account disabled, skipping", "account", accountID)
		return nil
	}

	// SDK 客户端
	client := NewFeishuClient(acct)
	p.clients[accountID] = client

	// 消息发送器
	sender := NewFeishuSender(client)
	p.senders[accountID] = sender

	// SDK 事件分发器（WebSocket 模式不需要 verifyToken/encryptKey）
	capturedAccountID := accountID
	d := NewEventDispatcher("", "",
		func(msg *FeishuMessageEvent) {
			// 去重：检查 message_id 是否已处理
			msgID := msg.Message.MessageID
			if msgID != "" {
				if _, loaded := p.seenMessages.LoadOrStore(msgID, time.Now()); loaded {
					slog.Debug("feishu: duplicate message ignored",
						"account", capturedAccountID,
						"message_id", msgID,
					)
					return
				}
				// 5 分钟后清除缓存条目
				go func() {
					time.Sleep(5 * time.Minute)
					p.seenMessages.Delete(msgID)
				}()
			}

			text := ExtractTextFromMessage(msg)
			chatID := msg.Message.ChatID
			// 提取发送者 open_id（用于审批卡片路由）
			userOpenID := ""
			if msg.Sender != nil && msg.Sender.SenderID != nil {
				userOpenID = msg.Sender.SenderID.OpenID
			}
			slog.Info("feishu message received",
				"account", capturedAccountID,
				"chat_id", chatID,
				"user_open_id", userOpenID,
				"msg_type", msg.Message.MessageType,
				"text", text,
			)

			// 路由到 Agent 管线获取回复（优先多模态）
			var reply string
			if p.DispatchMultimodalFunc != nil {
				cm := ExtractMultimodalMessage(msg)
				reply = p.DispatchMultimodalFunc(
					"feishu", capturedAccountID, chatID, userOpenID, cm)
			} else if p.DispatchFunc != nil {
				reply = p.DispatchFunc(context.Background(),
					"feishu", capturedAccountID, chatID, userOpenID, text)
			} else {
				slog.Warn("feishu: DispatchFunc not set, message not routed to agent",
					"account", capturedAccountID)
			}

			if reply != "" {
				// 发送回复
				idType := ReceiveIDTypeChatID
				if err := sender.SendText(context.Background(), chatID, idType, reply); err != nil {
					slog.Error("feishu: failed to send reply",
						"account", capturedAccountID,
						"chat_id", chatID,
						"error", err,
					)
				} else {
					slog.Info("feishu: reply sent",
						"account", capturedAccountID,
						"chat_id", chatID,
						"reply_len", len(reply),
					)
				}
			}
		})
	// 注册卡片回传交互处理器（审批按钮点击，走 WebSocket 长连接，无需公网）
	if p.CardActionFunc != nil {
		RegisterCardActionHandler(d, p.CardActionFunc)
		slog.Info("feishu: card action handler registered via WebSocket", "account", accountID)
	}

	p.dispatchers[accountID] = d

	// 创建 WebSocket 长连接客户端（接收事件 — SDK 推荐模式）
	wsClient := larkws.NewClient(acct.Config.AppID, acct.Config.AppSecret,
		larkws.WithEventHandler(d),
	)
	p.wsClients[accountID] = wsClient

	// 在后台 goroutine 中启动 WebSocket 连接
	wsCtx, wsCancel := context.WithCancel(context.Background())
	p.wsCancel[accountID] = wsCancel
	go func() {
		slog.Info("feishu: starting WebSocket connection",
			"account", capturedAccountID,
			"app_id", acct.Config.AppID,
		)
		if err := wsClient.Start(wsCtx); err != nil {
			slog.Error("feishu: WebSocket connection error",
				"account", capturedAccountID,
				"error", err,
			)
		}
	}()

	slog.Info("feishu channel started",
		"account", accountID,
		"domain", acct.Config.Domain,
		"app_id", acct.Config.AppID,
	)
	return nil
}

// Stop 停止飞书频道
func (p *FeishuPlugin) Stop(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	delete(p.clients, accountID)
	delete(p.senders, accountID)
	delete(p.dispatchers, accountID)

	slog.Info("feishu channel stopped", "account", accountID)
	return nil
}

// GetSender 获取指定账号的消息发送器
func (p *FeishuPlugin) GetSender(accountID string) *FeishuSender {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.senders[accountID]
}

// GetDispatcher 获取指定账号的事件分发器（供 HTTP 路由使用）
func (p *FeishuPlugin) GetDispatcher(accountID string) *dispatcher.EventDispatcher {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.dispatchers[accountID]
}

// SendMessage 实现 channels.MessageSender 接口。
// 自动检测 receiveID 类型：oc_ 开头为 chat_id，ou_ 开头为 open_id，其余默认 open_id。
// 如果 params.MediaURL 非空，自动下载并上传为飞书多媒体消息。
func (p *FeishuPlugin) SendMessage(params channels.OutboundSendParams) (*channels.OutboundSendResult, error) {
	accountID := params.AccountID
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	sender := p.GetSender(accountID)
	if sender == nil {
		return nil, fmt.Errorf("feishu sender not available for account %s", accountID)
	}
	client := p.GetClient(accountID)

	// 自动检测 receive_id_type
	idType := ReceiveIDTypeOpenID
	if strings.HasPrefix(params.To, "oc_") {
		idType = ReceiveIDTypeChatID
	}

	ctx := params.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 如有 MediaData（base64 解码的二进制），直接上传发送（跳过 HTTP 下载和 SSRF 校验）
	if len(params.MediaData) > 0 && client != nil {
		mediaErr := p.sendMediaData(ctx, client, sender, params.To, idType, params.MediaData, params.MediaMimeType)
		if mediaErr != nil {
			slog.Warn("feishu: binary media send failed, falling back to text",
				"mimeType", params.MediaMimeType, "dataLen", len(params.MediaData), "error", mediaErr)
			// fallthrough 到文字发送
		} else {
			if params.Text != "" {
				_ = sender.SendText(ctx, params.To, idType, params.Text)
			}
			return &channels.OutboundSendResult{
				Channel: string(channels.ChannelFeishu),
				ChatID:  params.To,
			}, nil
		}
	}

	// 如有 MediaURL，尝试下载并发送多媒体消息
	if params.MediaURL != "" && client != nil {
		mediaErr := p.sendMediaMessage(ctx, client, sender, params.To, idType, params.MediaURL)
		if mediaErr != nil {
			slog.Warn("feishu: media send failed, falling back to text",
				"mediaURL", params.MediaURL, "error", mediaErr)
			// fallthrough 到文字发送
		} else {
			// 如果同时有文字，追加发送
			if params.Text != "" {
				_ = sender.SendText(ctx, params.To, idType, params.Text)
			}
			return &channels.OutboundSendResult{
				Channel: string(channels.ChannelFeishu),
				ChatID:  params.To,
			}, nil
		}
	}

	if err := sender.SendText(ctx, params.To, idType, params.Text); err != nil {
		return nil, err
	}
	return &channels.OutboundSendResult{
		Channel: string(channels.ChannelFeishu),
		ChatID:  params.To,
	}, nil
}

// sendMediaMessage 下载媒体 URL → 上传到飞书 → 发送对应类型消息。
func (p *FeishuPlugin) sendMediaMessage(
	ctx context.Context,
	client *FeishuClient,
	sender *FeishuSender,
	receiveID, idType, mediaURL string,
) error {
	// 下载媒体
	resp, err := httpGetWithContext(ctx, mediaURL)
	if err != nil {
		return fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()

	const maxMediaSize = 30 * 1024 * 1024
	data, err := readLimited(resp.Body, maxMediaSize)
	if err != nil {
		return fmt.Errorf("read media body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	mediaCategory := detectMediaCategory(contentType, data)

	switch mediaCategory {
	case "image":
		imageKey, err := client.UploadImage(ctx, data, "message")
		if err != nil {
			return fmt.Errorf("upload image: %w", err)
		}
		return sender.SendImage(ctx, receiveID, idType, imageKey)

	case "audio":
		fileKey, err := client.UploadFile(ctx, data, "audio.opus", "opus", 0)
		if err != nil {
			return fmt.Errorf("upload audio: %w", err)
		}
		return sender.SendAudio(ctx, receiveID, idType, fileKey)

	default:
		// 文件类型
		fileName := "file"
		fileType := FeishuFileType(contentType, fileName)
		fileKey, err := client.UploadFile(ctx, data, fileName, fileType, 0)
		if err != nil {
			return fmt.Errorf("upload file: %w", err)
		}
		return sender.SendFile(ctx, receiveID, idType, fileKey)
	}
}

// sendMediaData 直接上传二进制媒体数据到飞书并发送（无 HTTP 下载，无 SSRF 校验）。
// 用于 Agent 生成的截图/图表等场景。
func (p *FeishuPlugin) sendMediaData(
	ctx context.Context,
	client *FeishuClient,
	sender *FeishuSender,
	receiveID, idType string,
	data []byte,
	mimeType string,
) error {
	mediaCategory := detectMediaCategory(mimeType, data)

	switch mediaCategory {
	case "image":
		imageKey, err := client.UploadImage(ctx, data, "message")
		if err != nil {
			return fmt.Errorf("upload image: %w", err)
		}
		return sender.SendImage(ctx, receiveID, idType, imageKey)

	case "audio":
		fileKey, err := client.UploadFile(ctx, data, "audio.opus", "opus", 0)
		if err != nil {
			return fmt.Errorf("upload audio: %w", err)
		}
		return sender.SendAudio(ctx, receiveID, idType, fileKey)

	default:
		fileName := "file"
		fileType := FeishuFileType(mimeType, fileName)
		fileKey, err := client.UploadFile(ctx, data, fileName, fileType, 0)
		if err != nil {
			return fmt.Errorf("upload file: %w", err)
		}
		return sender.SendFile(ctx, receiveID, idType, fileKey)
	}
}

// detectMediaCategory 从 Content-Type 和 magic bytes 推断媒体类别。
func detectMediaCategory(contentType string, data []byte) string {
	if strings.HasPrefix(contentType, "image/") {
		return "image"
	}
	if strings.HasPrefix(contentType, "audio/") {
		return "audio"
	}
	if strings.HasPrefix(contentType, "video/") {
		return "video"
	}
	// magic bytes fallback
	if len(data) >= 4 {
		// PNG
		if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return "image"
		}
		// JPEG
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return "image"
		}
		// GIF
		if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
			return "image"
		}
	}
	return "file"
}

// httpGetWithContext 带 context 的 HTTP GET（含 SSRF 防护）。
func httpGetWithContext(ctx context.Context, rawURL string) (*http.Response, error) {
	if err := validateMediaURL(rawURL); err != nil {
		return nil, fmt.Errorf("media URL rejected: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// 使用自定义 client 防止重定向到内部地址
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateMediaURL(req.URL.String())
		},
	}
	return client.Do(req)
}

// validateMediaURL 检查 URL 安全性（防 SSRF）。
func validateMediaURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	// 仅允许 HTTP/HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (only http/https allowed)", u.Scheme)
	}
	// 解析主机地址，拒绝私有/回环 IP
	host := u.Hostname()
	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q: %w", host, err)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL resolves to private/loopback address %s", ipStr)
		}
	}
	return nil
}

// readLimited 读取 body 但限制最大字节数。超限则返回错误而非静默截断。
func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("body exceeds %d bytes limit", maxBytes)
	}
	return data, nil
}
