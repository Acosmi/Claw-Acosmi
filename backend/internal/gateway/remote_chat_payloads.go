package gateway

import (
	"encoding/json"
	"strings"
)

func buildRemoteAssistantMessage(
	replyText string,
	ts int64,
	mediaItems []ReplyMediaItem,
	mediaBase64 string,
	mediaMime string,
) map[string]interface{} {
	if replyText == "" && mediaBase64 == "" && len(mediaItems) == 0 {
		return nil
	}

	content := make([]interface{}, 0, 1+len(mediaItems))
	if replyText != "" {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": replyText,
		})
	}

	if len(mediaItems) > 0 {
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
	} else if mediaBase64 != "" {
		mime := mediaMime
		if mime == "" {
			mime = "image/png"
		}
		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"data":       mediaBase64,
				"media_type": mime,
			},
		})
	}

	// P5 Usage 收敛: 非 LLM API 响应不写虚假 usage 零值。
	// 省略 usage 字段 → parseUsageFromEntry 返回 hasUsage=false → estimateUsageFromEntry 提供合理估算。
	return map[string]interface{}{
		"role":       "assistant",
		"content":    content,
		"timestamp":  ts,
		"stopReason": "stop",
	}
}

func buildRemoteAssistantChatPayload(
	sessionKey string,
	channel string,
	chatID string,
	replyText string,
	ts int64,
	mediaItems []ReplyMediaItem,
	mediaBase64 string,
	mediaMime string,
) map[string]interface{} {
	if replyText == "" && mediaBase64 == "" && len(mediaItems) == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"sessionKey": sessionKey,
		"channel":    channel,
		"role":       "assistant",
		"text":       replyText,
		"chatId":     chatID,
		"ts":         ts,
	}

	if len(mediaItems) > 0 {
		items := make([]map[string]string, 0, len(mediaItems))
		for _, item := range mediaItems {
			if item.Base64Data == "" {
				continue
			}
			items = append(items, map[string]string{
				"mediaBase64":   item.Base64Data,
				"mediaMimeType": item.MimeType,
			})
		}
		if len(items) > 0 {
			payload["mediaItems"] = items
			payload["mediaBase64"] = items[0]["mediaBase64"]
			payload["mediaMimeType"] = items[0]["mediaMimeType"]
		}
	} else if mediaBase64 != "" {
		payload["mediaBase64"] = mediaBase64
		payload["mediaMimeType"] = mediaMime
	}

	return payload
}

func loadLatestAssistantTranscriptMessage(sessionID, storePath, sessionFile string) map[string]interface{} {
	if strings.TrimSpace(sessionID) == "" && strings.TrimSpace(sessionFile) == "" {
		return nil
	}
	messages := ReadTranscriptMessages(sessionID, storePath, sessionFile)
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		role, _ := message["role"].(string)
		if role == "assistant" {
			return message
		}
	}
	return nil
}

func loadLatestSessionAssistantMessageMatching(
	store *SessionStore,
	sessionKey string,
	storePath string,
	replyText string,
	mediaItems []ReplyMediaItem,
) map[string]interface{} {
	if store == nil || strings.TrimSpace(sessionKey) == "" {
		return nil
	}
	entry := store.LoadSessionEntry(sessionKey)
	if entry == nil {
		return nil
	}
	message := loadLatestAssistantTranscriptMessage(entry.SessionId, storePath, entry.SessionFile)
	if !assistantMessageMatchesReplyResult(message, replyText, mediaItems) {
		return nil
	}
	return message
}

func loadLatestSessionAssistantBroadcastMatching(
	store *SessionStore,
	sessionKey string,
	channel string,
	chatID string,
	storePath string,
	replyText string,
	mediaItems []ReplyMediaItem,
) (map[string]interface{}, map[string]interface{}) {
	message := loadLatestSessionAssistantMessageMatching(store, sessionKey, storePath, replyText, mediaItems)
	if message == nil {
		return nil, nil
	}
	return message, buildRemoteAssistantChatPayloadFromMessage(sessionKey, channel, chatID, message)
}

func buildRemoteAssistantChatPayloadFromMessage(
	sessionKey string,
	channel string,
	chatID string,
	message map[string]interface{},
) map[string]interface{} {
	if message == nil {
		return nil
	}
	role, _ := message["role"].(string)
	if role != "" && role != "assistant" {
		return nil
	}
	replyText, mediaItems := extractRemoteAssistantTextAndMedia(message)
	if replyText == "" && len(mediaItems) == 0 {
		return nil
	}
	return buildRemoteAssistantChatPayload(
		sessionKey,
		channel,
		chatID,
		replyText,
		normalizeRemoteAssistantTimestamp(message["timestamp"]),
		mediaItems,
		"",
		"",
	)
}

func extractRemoteAssistantTextAndMedia(message map[string]interface{}) (string, []ReplyMediaItem) {
	switch content := message["content"].(type) {
	case string:
		return strings.TrimSpace(content), nil
	case []interface{}:
		texts := make([]string, 0, len(content))
		mediaItems := make([]ReplyMediaItem, 0, len(content))
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				text, _ := block["text"].(string)
				if strings.TrimSpace(text) != "" {
					texts = append(texts, text)
				}
			case "image":
				source, _ := block["source"].(map[string]interface{})
				if source == nil {
					continue
				}
				data, _ := source["data"].(string)
				if strings.TrimSpace(data) == "" {
					continue
				}
				mimeType, _ := source["media_type"].(string)
				mediaItems = append(mediaItems, ReplyMediaItem{
					Base64Data: data,
					MimeType:   mimeType,
				})
			}
		}
		return strings.Join(texts, "\n\n"), mediaItems
	default:
		return "", nil
	}
}

func assistantMessageMatchesReplyResult(message map[string]interface{}, replyText string, mediaItems []ReplyMediaItem) bool {
	if message == nil {
		return false
	}
	actualText, actualMedia := extractRemoteAssistantTextAndMedia(message)
	expectedText := strings.TrimSpace(replyText)
	actualText = strings.TrimSpace(actualText)

	if expectedText == "" && actualText != "" {
		return false
	}
	if expectedText != "" && actualText != expectedText {
		return false
	}
	if len(mediaItems) == 0 && len(actualMedia) > 0 {
		return false
	}
	if len(mediaItems) > len(actualMedia) {
		return false
	}
	for i, item := range mediaItems {
		if actualMedia[i].Base64Data != item.Base64Data {
			return false
		}
		if normalizeRemoteAssistantMime(actualMedia[i].MimeType) != normalizeRemoteAssistantMime(item.MimeType) {
			return false
		}
	}
	return expectedText != "" || len(mediaItems) > 0
}

func normalizeRemoteAssistantMime(mimeType string) string {
	mimeType = strings.TrimSpace(strings.ToLower(mimeType))
	if mimeType == "" {
		return "image/png"
	}
	return mimeType
}

func normalizeRemoteAssistantTimestamp(raw interface{}) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		n, err := value.Int64()
		if err == nil {
			return n
		}
	}
	return 0
}
