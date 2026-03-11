package feishu

// handler.go — 飞书事件处理（SDK 模式）
// 使用 oapi-sdk-go/v3 SDK 内置的事件解析和验签

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

// FeishuMessageEvent im.message.receive_v1 事件（简化结构）
type FeishuMessageEvent struct {
	Sender  *FeishuSenderInfo  `json:"sender,omitempty"`
	Message *FeishuMessageInfo `json:"message,omitempty"`
}

// FeishuSenderInfo 发送者信息
type FeishuSenderInfo struct {
	SenderID   *FeishuSenderID `json:"sender_id,omitempty"`
	SenderType string          `json:"sender_type,omitempty"`
	TenantKey  string          `json:"tenant_key,omitempty"`
}

// FeishuSenderID 发送者 ID
type FeishuSenderID struct {
	UnionID string `json:"union_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	OpenID  string `json:"open_id,omitempty"`
}

// FeishuMessageInfo 消息信息
type FeishuMessageInfo struct {
	MessageID   string `json:"message_id,omitempty"`
	RootID      string `json:"root_id,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	CreateTime  string `json:"create_time,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	ChatType    string `json:"chat_type,omitempty"` // "p2p" | "group"
	MessageType string `json:"message_type,omitempty"`
	Content     string `json:"content,omitempty"` // JSON 字符串
}

// ParseMessageContent 解析文本消息内容
func ParseMessageContent(content string) (string, error) {
	var c struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &c); err != nil {
		return "", fmt.Errorf("parse message content: %w", err)
	}
	return c.Text, nil
}

// ExtractTextFromMessage 从消息事件中提取纯文本
func ExtractTextFromMessage(msg *FeishuMessageEvent) string {
	if msg == nil || msg.Message == nil || msg.Message.Content == "" {
		return ""
	}
	text, err := ParseMessageContent(msg.Message.Content)
	if err != nil {
		slog.Warn("failed to parse feishu message content", "error", err)
		return ""
	}
	return strings.TrimSpace(text)
}

// ---------- 多模态消息解析（Phase A 新增） ----------

// ExtractMultimodalMessage 从飞书消息事件中解析多模态消息。
// 根据 message_type 提取文本、图片 key、文件 key 等。
func ExtractMultimodalMessage(msg *FeishuMessageEvent) *channels.ChannelMessage {
	if msg == nil || msg.Message == nil {
		return &channels.ChannelMessage{}
	}

	cm := &channels.ChannelMessage{
		MessageID:   msg.Message.MessageID,
		MessageType: msg.Message.MessageType,
		RawContent:  msg.Message.Content,
	}

	switch msg.Message.MessageType {
	case "text":
		text, err := ParseMessageContent(msg.Message.Content)
		if err != nil {
			slog.Warn("feishu: failed to parse text message", "error", err)
		}
		cm.Text = strings.TrimSpace(text)

	case "image":
		imageKey := parseImageKey(msg.Message.Content)
		if imageKey != "" {
			cm.Attachments = append(cm.Attachments, channels.ChannelAttachment{
				Category: "image",
				FileKey:  imageKey,
				MimeType: "image/png", // 飞书图片默认 PNG，实际格式在下载时确定
			})
		}

	case "audio":
		fileKey, duration := parseAudioContent(msg.Message.Content)
		if fileKey != "" {
			att := channels.ChannelAttachment{
				Category: "audio",
				FileKey:  fileKey,
				MimeType: "audio/opus", // 飞书语音格式
			}
			_ = duration // 预留，后续可用于 UI 展示
			cm.Attachments = append(cm.Attachments, att)
		}

	case "file":
		fileKey, fileName, fileSize := parseFileContent(msg.Message.Content)
		if fileKey != "" {
			cm.Attachments = append(cm.Attachments, channels.ChannelAttachment{
				Category: inferCategoryFromFileName(fileName),
				FileKey:  fileKey,
				FileName: fileName,
				FileSize: fileSize,
			})
		}

	case "post":
		// 富文本：提取纯文本摘要，并保留内嵌图片为附件。
		text, attachments := parsePostContentWithAttachments(msg.Message.Content)
		cm.Text = strings.TrimSpace(text)
		cm.Attachments = append(cm.Attachments, attachments...)
		cm.MessageType = "text" // 归一化为文本

	default:
		// 未识别类型：尝试提取文本
		text, _ := ParseMessageContent(msg.Message.Content)
		cm.Text = strings.TrimSpace(text)
	}

	return cm
}

// parseImageKey 从图片消息 content 中提取 image_key
func parseImageKey(content string) string {
	var c struct {
		ImageKey string `json:"image_key"`
	}
	if err := json.Unmarshal([]byte(content), &c); err != nil {
		return ""
	}
	return c.ImageKey
}

// parseAudioContent 从语音消息 content 中提取 file_key 和 duration
func parseAudioContent(content string) (fileKey string, duration int) {
	var c struct {
		FileKey  string `json:"file_key"`
		Duration int    `json:"duration"`
	}
	if err := json.Unmarshal([]byte(content), &c); err != nil {
		return "", 0
	}
	return c.FileKey, c.Duration
}

// parseFileContent 从文件消息 content 中提取 file_key、file_name、file_size
func parseFileContent(content string) (fileKey, fileName string, fileSize int64) {
	var c struct {
		FileKey  string `json:"file_key"`
		FileName string `json:"file_name"`
		FileSize int64  `json:"file_size"`
	}
	if err := json.Unmarshal([]byte(content), &c); err != nil {
		return "", "", 0
	}
	return c.FileKey, c.FileName, c.FileSize
}

// parsePostContent 从富文本消息中提取纯文本
func parsePostContent(content string) string {
	text, _ := parsePostContentWithAttachments(content)
	return text
}

func parsePostContentWithAttachments(content string) (string, []channels.ChannelAttachment) {
	var c struct {
		ZhCN *postBody `json:"zh_cn"`
		EnUS *postBody `json:"en_us"`
	}
	if err := json.Unmarshal([]byte(content), &c); err != nil {
		return "", nil
	}
	body := c.ZhCN
	if body == nil {
		body = c.EnUS
	}
	if body == nil {
		return "", nil
	}
	var sb strings.Builder
	attachments := make([]channels.ChannelAttachment, 0)
	if body.Title != "" {
		sb.WriteString(body.Title)
		sb.WriteString("\n")
	}
	for _, line := range body.Content {
		lineHasText := false
		for _, elem := range line {
			if elem.Tag == "text" {
				sb.WriteString(elem.Text)
				lineHasText = true
			} else if elem.Tag == "a" {
				sb.WriteString(elem.Text)
				if elem.Href != "" {
					sb.WriteString("(")
					sb.WriteString(elem.Href)
					sb.WriteString(")")
				}
				lineHasText = true
			} else if elem.Tag == "img" {
				imageKey := strings.TrimSpace(elem.ImageKey)
				if imageKey == "" {
					imageKey = strings.TrimSpace(elem.ImageKeyCamel)
				}
				if imageKey != "" {
					attachments = append(attachments, channels.ChannelAttachment{
						Category: "image",
						FileKey:  imageKey,
						MimeType: "image/png",
					})
				}
			}
		}
		if lineHasText {
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), attachments
}

type postBody struct {
	Title   string       `json:"title"`
	Content [][]postElem `json:"content"`
}

type postElem struct {
	Tag           string `json:"tag"`
	Text          string `json:"text,omitempty"`
	Href          string `json:"href,omitempty"`
	ImageKey      string `json:"image_key,omitempty"`
	ImageKeyCamel string `json:"imageKey,omitempty"`
}

// inferCategoryFromFileName 根据文件名推断附件类别
func inferCategoryFromFileName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"),
		strings.HasSuffix(lower, ".png"), strings.HasSuffix(lower, ".gif"),
		strings.HasSuffix(lower, ".webp"), strings.HasSuffix(lower, ".bmp"):
		return "image"
	case strings.HasSuffix(lower, ".mp3"), strings.HasSuffix(lower, ".wav"),
		strings.HasSuffix(lower, ".ogg"), strings.HasSuffix(lower, ".opus"),
		strings.HasSuffix(lower, ".m4a"), strings.HasSuffix(lower, ".flac"):
		return "audio"
	case strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".avi"),
		strings.HasSuffix(lower, ".mov"), strings.HasSuffix(lower, ".webm"):
		return "video"
	default:
		return "document"
	}
}
