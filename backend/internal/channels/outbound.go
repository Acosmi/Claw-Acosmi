package channels

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// 出站适配器 — 继承自 src/channels/plugins/outbound/ (6 个频道文件 + load.ts)
// 定义各频道的出站发送配置和目标解析

// OutboundDeliveryMode 交付模式
type OutboundDeliveryMode string

const (
	DeliveryModeDirect  OutboundDeliveryMode = "direct"
	DeliveryModeGateway OutboundDeliveryMode = "gateway"
)

// OutboundSendParams 出站发送参数
type OutboundSendParams struct {
	Ctx       context.Context
	To        string
	Text      string
	MediaURL  string
	AccountID string
	ReplyToID string
	ThreadID  string

	// MediaData 二进制媒体内容（来自 base64 解码），优先于 MediaURL。
	// 用于 Agent 生成的截图/图表等无公网 URL 的媒体。
	MediaData []byte
	// MediaMimeType 媒体 MIME 类型（如 "image/png"），配合 MediaData 使用。
	MediaMimeType string
}

// OutboundSendResult 出站发送结果
type OutboundSendResult struct {
	Channel   string `json:"channel"`
	MessageID string `json:"messageId,omitempty"`
	ChatID    string `json:"chatId,omitempty"`
}

// ChannelOutboundConfig 频道出站配置
type ChannelOutboundConfig struct {
	ChannelID      ChannelID
	DeliveryMode   OutboundDeliveryMode
	ChunkerMode    string // "text" | "markdown" | ""
	TextChunkLimit int
	PollMaxOptions int
}

// 核心频道出站配置表
var outboundConfigs = map[ChannelID]*ChannelOutboundConfig{
	ChannelTelegram: {
		ChannelID:      ChannelTelegram,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "markdown",
		TextChunkLimit: 4000,
	},
	ChannelDiscord: {
		ChannelID:      ChannelDiscord,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "",
		TextChunkLimit: 2000,
		PollMaxOptions: 10,
	},
	ChannelSlack: {
		ChannelID:      ChannelSlack,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "",
		TextChunkLimit: 4000,
	},
	ChannelWhatsApp: {
		ChannelID:      ChannelWhatsApp,
		DeliveryMode:   DeliveryModeGateway,
		ChunkerMode:    "text",
		TextChunkLimit: 4000,
		PollMaxOptions: 12,
	},
	ChannelSignal: {
		ChannelID:      ChannelSignal,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "text",
		TextChunkLimit: 4000,
	},
	ChannelIMessage: {
		ChannelID:      ChannelIMessage,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "text",
		TextChunkLimit: 4000,
	},
	// Phase 7: 中国频道出站配置
	ChannelFeishu: {
		ChannelID:      ChannelFeishu,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "text",
		TextChunkLimit: 4000,
	},
	ChannelDingTalk: {
		ChannelID:      ChannelDingTalk,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "text",
		TextChunkLimit: 2000,
	},
	ChannelWeCom: {
		ChannelID:      ChannelWeCom,
		DeliveryMode:   DeliveryModeDirect,
		ChunkerMode:    "text",
		TextChunkLimit: 2000,
	},
}

// GetOutboundConfig 获取频道出站配置
func GetOutboundConfig(channelID ChannelID) *ChannelOutboundConfig {
	return outboundConfigs[channelID]
}

// ── Telegram 辅助 ──

// ParseReplyToMessageID 解析回复消息 ID（整数）
func ParseReplyToMessageID(replyToID string) int {
	if replyToID == "" {
		return 0
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(replyToID))
	if err != nil {
		return 0
	}
	return parsed
}

// ParseThreadID 解析线程 ID
func ParseThreadID(threadID string) int {
	trimmed := strings.TrimSpace(threadID)
	if trimmed == "" {
		return 0
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return parsed
}

// ── WhatsApp 目标解析 ──

// WhatsAppResolveTargetResult WhatsApp 目标解析结果
type WhatsAppResolveTargetResult struct {
	OK    bool
	To    string
	Error string
}

// ResolveWhatsAppOutboundTarget WhatsApp 出站目标解析
func ResolveWhatsAppOutboundTarget(to string, allowFrom []string, mode string) WhatsAppResolveTargetResult {
	trimmed := strings.TrimSpace(to)

	// 构建 allowList
	var allowList []string
	hasWildcard := false
	for _, entry := range allowFrom {
		s := strings.TrimSpace(entry)
		if s == "" {
			continue
		}
		if s == "*" {
			hasWildcard = true
			continue
		}
		allowList = append(allowList, s)
	}

	if trimmed != "" {
		normalizedTo := NormalizeWhatsAppMessagingTarget(trimmed)
		if normalizedTo == "" {
			if (mode == "implicit" || mode == "heartbeat") && len(allowList) > 0 {
				return WhatsAppResolveTargetResult{OK: true, To: allowList[0]}
			}
			return WhatsAppResolveTargetResult{
				OK:    false,
				Error: fmt.Sprintf("WhatsApp target could not be resolved: provide <E.164|group JID> or channels.whatsapp.allowFrom[0]"),
			}
		}
		// Group JID 直接允许
		if strings.Contains(normalizedTo, "@g.us") {
			return WhatsAppResolveTargetResult{OK: true, To: normalizedTo}
		}
		if mode == "implicit" || mode == "heartbeat" {
			if hasWildcard || len(allowList) == 0 {
				return WhatsAppResolveTargetResult{OK: true, To: normalizedTo}
			}
			for _, a := range allowList {
				if a == normalizedTo {
					return WhatsAppResolveTargetResult{OK: true, To: normalizedTo}
				}
			}
			return WhatsAppResolveTargetResult{OK: true, To: allowList[0]}
		}
		return WhatsAppResolveTargetResult{OK: true, To: normalizedTo}
	}

	if len(allowList) > 0 {
		return WhatsAppResolveTargetResult{OK: true, To: allowList[0]}
	}
	return WhatsAppResolveTargetResult{
		OK:    false,
		Error: "WhatsApp target could not be resolved: provide <E.164|group JID> or channels.whatsapp.allowFrom[0]",
	}
}
