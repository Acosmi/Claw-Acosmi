package gateway

// transcript.go — transcript JSONL 读写辅助函数
// 对应 TS src/gateway/server-methods/chat.ts (resolveTranscriptPath, ensureTranscriptFile,
// readSessionMessages, appendAssistantTranscriptMessage, stripEnvelopeFromMessages)
// 以及 src/gateway/session-utils.ts 中的 readSessionMessages。

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CURRENT_SESSION_VERSION transcript 文件版本。
// TS 对照: @mariozechner/pi-coding-agent CURRENT_SESSION_VERSION
const CURRENT_SESSION_VERSION = "1.0"

var downgradedToolDirectiveTextRE = regexp.MustCompile(`(?ms)^\s*\[\[[a-z_][a-z0-9_]*\]\]\s*(?:\n[a-zA-Z0-9_.-]+\s*:\s*.*)*\s*$`)

// TranscriptAppendResult transcript 追加结果。
type TranscriptAppendResult struct {
	OK        bool
	MessageID string
	Message   map[string]interface{}
	Error     string
}

// ResolveTranscriptPath 解析 transcript 文件路径。
// TS 对照: chat.ts resolveTranscriptPath (L51-64)
func ResolveTranscriptPath(sessionId, storePath, sessionFile string) string {
	if sessionFile != "" {
		return sessionFile
	}
	if storePath == "" {
		return ""
	}
	dir := filepath.Dir(storePath)
	return filepath.Join(dir, sessionId+".jsonl")
}

// EnsureTranscriptFile 确保 transcript 文件存在（含 header 行）。
// TS 对照: chat.ts ensureTranscriptFile (L66-87)
func EnsureTranscriptFile(transcriptPath, sessionId string) error {
	if _, err := os.Stat(transcriptPath); err == nil {
		return nil // 已存在
	}

	// 创建目录
	dir := filepath.Dir(transcriptPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create transcript dir: %w", err)
	}

	// 写入 header
	header := map[string]interface{}{
		"type":      "session",
		"version":   CURRENT_SESSION_VERSION,
		"id":        sessionId,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal transcript header: %w", err)
	}

	if err := os.WriteFile(transcriptPath, append(headerJSON, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write transcript file: %w", err)
	}
	return nil
}

// ReadTranscriptMessages 从 JSONL 文件读取消息。
// TS 对照: session-utils.ts readSessionMessages
// 跳过第一行 header 和非 message 类型的行。
func ReadTranscriptMessages(sessionId, storePath, sessionFile string) []map[string]interface{} {
	transcriptPath := ResolveTranscriptPath(sessionId, storePath, sessionFile)
	if transcriptPath == "" {
		return nil
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		slog.Debug("transcript file not found", "path", transcriptPath, "error", err)
		return nil
	}
	defer f.Close()

	var messages []map[string]interface{}
	scanner := bufio.NewScanner(f)

	// 增加 buffer 大小以处理大消息
	const maxScanTokenSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			slog.Debug("transcript line parse error", "line", lineNum, "error", err)
			continue
		}

		// 跳过 session header 行
		if entryType, _ := entry["type"].(string); entryType == "session" {
			continue
		}

		// 只保留有 role 字段的消息
		if _, hasRole := entry["role"]; hasRole {
			messages = append(messages, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("transcript scan error", "path", transcriptPath, "error", err)
	}

	return messages
}

// AppendAssistantTranscriptMessage 追加 assistant 消息到 transcript JSONL。
// TS 对照: chat.ts appendAssistantTranscriptMessage (L89-158)
func AppendAssistantTranscriptMessage(params AppendTranscriptParams) *TranscriptAppendResult {
	transcriptPath := ResolveTranscriptPath(params.SessionID, params.StorePath, params.SessionFile)
	if transcriptPath == "" {
		return &TranscriptAppendResult{OK: false, Error: "transcript path not resolved"}
	}

	// 检查文件是否存在
	if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
		if !params.CreateIfMissing {
			return &TranscriptAppendResult{OK: false, Error: "transcript file not found"}
		}
		if err := EnsureTranscriptFile(transcriptPath, params.SessionID); err != nil {
			return &TranscriptAppendResult{OK: false, Error: err.Error()}
		}
	}

	now := time.Now().UnixMilli()
	messageID := fmt.Sprintf("msg_%d", now)

	// 构建标签前缀
	labelPrefix := ""
	if params.Label != "" {
		labelPrefix = fmt.Sprintf("[%s]\n\n", params.Label)
	}

	// 构建消息体
	message := map[string]interface{}{
		"role": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": labelPrefix + params.Message,
			},
		},
		"timestamp":  now,
		"stopReason": "stop",
		// P5 Usage 收敛: 注入消息用 "source":"injected" 标记，不伪造 0 值。
		// parseUsageFromEntry 遇到全零值时 hasUsage=false，source 字段帮助统计层准确归类。
		"usage": map[string]interface{}{
			"source": "injected",
		},
		"api":      "openai-responses",
		"provider": "openacosmi",
		"model":    "gateway-injected",
		"id":       messageID,
	}

	// 追加到文件
	msgJSON, err := json.Marshal(message)
	if err != nil {
		return &TranscriptAppendResult{OK: false, Error: fmt.Sprintf("marshal failed: %v", err)}
	}

	f, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return &TranscriptAppendResult{OK: false, Error: fmt.Sprintf("open failed: %v", err)}
	}
	defer f.Close()

	if _, err := f.Write(append(msgJSON, '\n')); err != nil {
		return &TranscriptAppendResult{OK: false, Error: fmt.Sprintf("write failed: %v", err)}
	}

	return &TranscriptAppendResult{OK: true, MessageID: messageID, Message: message}
}

// AppendTranscriptParams 追加 transcript 参数。
type AppendTranscriptParams struct {
	Message         string
	Label           string
	SessionID       string
	StorePath       string
	SessionFile     string
	CreateIfMissing bool
}

// AppendUserTranscriptMessage 追加 user 消息到 transcript JSONL。
// 对应 TS: chat.ts 中用户消息持久化逻辑。
func AppendUserTranscriptMessage(params AppendTranscriptParams) *TranscriptAppendResult {
	transcriptPath := ResolveTranscriptPath(params.SessionID, params.StorePath, params.SessionFile)
	if transcriptPath == "" {
		return &TranscriptAppendResult{OK: false, Error: "transcript path not resolved"}
	}

	// 检查文件是否存在
	if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
		if !params.CreateIfMissing {
			return &TranscriptAppendResult{OK: false, Error: "transcript file not found"}
		}
		if err := EnsureTranscriptFile(transcriptPath, params.SessionID); err != nil {
			return &TranscriptAppendResult{OK: false, Error: err.Error()}
		}
	}

	now := time.Now().UnixMilli()
	messageID := fmt.Sprintf("msg_%d", now)

	message := map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": params.Message,
			},
		},
		"timestamp": now,
		"id":        messageID,
	}

	msgJSON, err := json.Marshal(message)
	if err != nil {
		return &TranscriptAppendResult{OK: false, Error: fmt.Sprintf("marshal failed: %v", err)}
	}

	f, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return &TranscriptAppendResult{OK: false, Error: fmt.Sprintf("open failed: %v", err)}
	}
	defer f.Close()

	if _, err := f.Write(append(msgJSON, '\n')); err != nil {
		return &TranscriptAppendResult{OK: false, Error: fmt.Sprintf("write failed: %v", err)}
	}

	return &TranscriptAppendResult{OK: true, MessageID: messageID, Message: message}
}

// StripEnvelopeFromMessages 清理消息 envelope 元数据。
// TS 对照: chat-sanitize.ts stripEnvelopeFromMessages
// 移除内部字段（parentId, uuid 等），只保留对前端有用的字段。
func StripEnvelopeFromMessages(msgs []map[string]interface{}) []map[string]interface{} {
	if len(msgs) == 0 {
		return msgs
	}

	// 需要移除的内部字段
	removeKeys := []string{
		"parentId", "uuid", "cwd", "version",
	}

	result := make([]map[string]interface{}, 0, len(msgs))
	for _, msg := range msgs {
		cleaned := make(map[string]interface{}, len(msg))
		for k, v := range msg {
			cleaned[k] = v
		}
		for _, key := range removeKeys {
			delete(cleaned, key)
		}

		// 安全兜底：清理 assistant 消息中的 reply tag（如 [[reply_to_current]]）
		if role, _ := cleaned["role"].(string); role == "assistant" {
			stripReplyTagsFromContent(cleaned)
		}

		result = append(result, cleaned)
	}
	return result
}

// stripReplyTagsFromContent 清理 transcript 消息 content 中的 reply 标签。
// 处理两种格式：content 为字符串（旧格式）或 content block 数组（新格式）。
func stripReplyTagsFromContent(msg map[string]interface{}) {
	switch c := msg["content"].(type) {
	case string:
		if stripped := stripReplyTagsText(c); stripped != c {
			msg["content"] = stripped
		}
	case []interface{}:
		for _, block := range c {
			if b, ok := block.(map[string]interface{}); ok {
				if b["type"] == "text" {
					if t, ok := b["text"].(string); ok {
						b["text"] = stripReplyTagsText(t)
					}
				}
			}
		}
	}
}

// stripReplyTagsText 剥离文本中的 [[reply_to_current]] / [[reply_to:...]] / [[reply:...]] 标签。
// 与 dispatch_inbound.go 中 stripReplyTags 逻辑一致。
func stripReplyTagsText(text string) string {
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
		} else {
			searchFrom = closeIdx + 2
		}
	}
	result = strings.TrimSpace(result)
	if downgradedToolDirectiveTextRE.MatchString(result) {
		return ""
	}
	return result
}

// CapArrayByJSONBytes 按 JSON 字节限制裁剪数组（从尾部保留）。
// TS 对照: session-utils.ts capArrayByJsonBytes
func CapArrayByJSONBytes(items []map[string]interface{}, maxBytes int) []map[string]interface{} {
	if maxBytes <= 0 || len(items) == 0 {
		return items
	}

	totalBytes := 0
	startIdx := len(items)
	for i := len(items) - 1; i >= 0; i-- {
		b, err := json.Marshal(items[i])
		if err != nil {
			continue
		}
		if totalBytes+len(b) > maxBytes {
			if startIdx == len(items) {
				startIdx = i
			}
			break
		}
		totalBytes += len(b)
		startIdx = i
	}

	return items[startIdx:]
}
