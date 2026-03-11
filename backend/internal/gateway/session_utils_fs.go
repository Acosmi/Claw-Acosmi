package gateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- 常量 ----------

const maxLinesToScan = 10
const lastMsgMaxBytes = 16384
const lastMsgMaxLines = 20

// ---------- Transcript 候选路径 ----------

// ResolveSessionTranscriptCandidates 生成 transcript 文件候选路径列表。
func ResolveSessionTranscriptCandidates(sessionId, storePath, sessionFile, agentId string) []string {
	var candidates []string
	if sessionFile != "" {
		candidates = append(candidates, sessionFile)
	}
	if storePath != "" {
		dir := filepath.Dir(storePath)
		candidates = append(candidates, filepath.Join(dir, sessionId+".jsonl"))
	}
	if agentId != "" {
		// 简化版：使用状态目录下的 agent transcript 路径
		stateDir := config.ResolveStateDir()
		if stateDir != "" {
			candidates = append(candidates,
				filepath.Join(stateDir, "agents", agentId, "sessions", sessionId+".jsonl"))
		}
	}
	// 全局回退路径
	stateDir := config.ResolveStateDir()
	if stateDir != "" {
		candidates = append(candidates,
			filepath.Join(stateDir, "sessions", sessionId+".jsonl"))
	}
	return candidates
}

// ---------- 读取消息 ----------

// transcriptLine 单行 JSONL 记录。
type transcriptLine struct {
	Message json.RawMessage `json:"message,omitempty"`
	Type    string          `json:"type,omitempty"`
	ID      string          `json:"id,omitempty"`
	TS      string          `json:"timestamp,omitempty"`
}

// transcriptMessage 消息结构。
type transcriptMessage struct {
	Role    string      `json:"role,omitempty"`
	Content interface{} `json:"content,omitempty"` // string 或 []contentPart
	Text    string      `json:"text,omitempty"`
}

// contentPart content 数组元素。
type contentPart struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

// ReadSessionMessages 读取会话的全部消息。
func ReadSessionMessages(sessionId, storePath, sessionFile string) []map[string]interface{} {
	candidates := ResolveSessionTranscriptCandidates(sessionId, storePath, sessionFile, "")
	filePath := findExistingFile(candidates)
	if filePath == "" {
		return nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var messages []map[string]interface{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tl transcriptLine
		if err := json.Unmarshal([]byte(line), &tl); err != nil {
			continue
		}
		if len(tl.Message) > 0 {
			var msg map[string]interface{}
			if err := json.Unmarshal(tl.Message, &msg); err == nil {
				messages = append(messages, msg)
			}
			continue
		}
		// Compaction 合成消息
		if tl.Type == "compaction" {
			messages = append(messages, map[string]interface{}{
				"role":    "system",
				"content": []map[string]string{{"type": "text", "text": "Compaction"}},
				"__openacosmi": map[string]interface{}{
					"kind": "compaction",
					"id":   tl.ID,
				},
			})
		}
	}
	return messages
}

// ---------- 提取文本 ----------

// extractTextFromContent 从 content 字段提取文本。
func extractTextFromContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	case []interface{}:
		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			text, _ := m["text"].(string)
			typ, _ := m["type"].(string)
			if typ == "text" || typ == "output_text" || typ == "input_text" {
				if t := strings.TrimSpace(text); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

// ---------- 首条用户消息 ----------

// ReadFirstUserMessageFromTranscript 读取 transcript 中第一条用户消息文本。
func ReadFirstUserMessageFromTranscript(sessionId, storePath, sessionFile, agentId string) string {
	candidates := ResolveSessionTranscriptCandidates(sessionId, storePath, sessionFile, agentId)
	filePath := findExistingFile(candidates)
	if filePath == "" {
		return ""
	}

	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	// 只读取前 8KB
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	if n == 0 {
		return ""
	}

	lines := strings.SplitN(string(buf[:n]), "\n", maxLinesToScan+1)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tl transcriptLine
		if err := json.Unmarshal([]byte(line), &tl); err != nil {
			continue
		}
		if len(tl.Message) == 0 {
			continue
		}
		var msg transcriptMessage
		if err := json.Unmarshal(tl.Message, &msg); err != nil {
			continue
		}
		if msg.Role == "user" {
			if text := extractTextFromContent(msg.Content); text != "" {
				return text
			}
		}
	}
	return ""
}

// ---------- 最后消息预览 ----------

// ReadLastMessagePreviewFromTranscript 读取 transcript 尾部的最后一条用户/助手消息。
func ReadLastMessagePreviewFromTranscript(sessionId, storePath, sessionFile, agentId string) string {
	candidates := ResolveSessionTranscriptCandidates(sessionId, storePath, sessionFile, agentId)
	filePath := findExistingFile(candidates)
	if filePath == "" {
		return ""
	}

	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return ""
	}

	size := stat.Size()
	readStart := size - int64(lastMsgMaxBytes)
	if readStart < 0 {
		readStart = 0
	}
	readLen := int(size - readStart)

	buf := make([]byte, readLen)
	_, err = f.ReadAt(buf, readStart)
	if err != nil && err != io.EOF {
		return ""
	}

	lines := strings.Split(string(buf), "\n")
	// 过滤空行
	var nonEmpty []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}
	// 取尾部
	if len(nonEmpty) > lastMsgMaxLines {
		nonEmpty = nonEmpty[len(nonEmpty)-lastMsgMaxLines:]
	}

	// 从尾部往前找
	for i := len(nonEmpty) - 1; i >= 0; i-- {
		var tl transcriptLine
		if err := json.Unmarshal([]byte(nonEmpty[i]), &tl); err != nil {
			continue
		}
		if len(tl.Message) == 0 {
			continue
		}
		var msg transcriptMessage
		if err := json.Unmarshal(tl.Message, &msg); err != nil {
			continue
		}
		if msg.Role == "user" || msg.Role == "assistant" {
			if text := extractTextFromContent(msg.Content); text != "" {
				return text
			}
		}
	}
	return ""
}

// ---------- 文件归档 ----------

// ArchiveFileOnDisk 将文件重命名为归档文件（添加时间戳和原因后缀）。
func ArchiveFileOnDisk(filePath, reason string) (string, error) {
	ts := time.Now().Format("2006-01-02T15-04-05.000Z07-00")
	archived := fmt.Sprintf("%s.%s.%s", filePath, reason, ts)
	err := os.Rename(filePath, archived)
	return archived, err
}

// ---------- JSON 字节限制 ----------

// CapArrayByJsonBytes 按 JSON 字节上限从前端截断数组。
func CapArrayByJsonBytes(items []json.RawMessage, maxBytes int) ([]json.RawMessage, int) {
	if len(items) == 0 {
		return items, 2 // "[]"
	}
	sizes := make([]int, len(items))
	total := 2 // [ ]
	for i, item := range items {
		sizes[i] = len(item)
		total += sizes[i]
	}
	total += len(items) - 1 // 逗号

	start := 0
	for total > maxBytes && start < len(items)-1 {
		total -= sizes[start] + 1
		start++
	}
	if start > 0 {
		return items[start:], total
	}
	return items, total
}

// ---------- Preview 相关 ----------

// ReadSessionPreviewItemsFromTranscript 读取会话消息预览。
func ReadSessionPreviewItemsFromTranscript(
	sessionId, storePath, sessionFile, agentId string,
	maxItems, maxChars int,
) []SessionPreviewItem {
	candidates := ResolveSessionTranscriptCandidates(sessionId, storePath, sessionFile, agentId)
	filePath := findExistingFile(candidates)
	if filePath == "" {
		return nil
	}

	boundedItems := clampInt(maxItems, 1, 50)
	boundedChars := clampInt(maxChars, 20, 2000)

	readSizes := []int{64 * 1024, 256 * 1024, 1024 * 1024}
	for i, readSize := range readSizes {
		messages := readRecentMessagesFromFile(filePath, boundedItems, readSize)
		if len(messages) > 0 || i == len(readSizes)-1 {
			return buildPreviewItems(messages, boundedItems, boundedChars)
		}
	}
	return nil
}

// readRecentMessagesFromFile 从文件尾部读取最近的消息。
func readRecentMessagesFromFile(filePath string, maxMessages, readBytes int) []transcriptMessage {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return nil
	}

	size := stat.Size()
	readStart := size - int64(readBytes)
	if readStart < 0 {
		readStart = 0
	}
	readLen := int(size - readStart)

	buf := make([]byte, readLen)
	_, err = f.ReadAt(buf, readStart)
	if err != nil && err != io.EOF {
		return nil
	}

	lines := strings.Split(string(buf), "\n")
	var nonEmpty []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}
	const previewMaxLines = 200
	if len(nonEmpty) > previewMaxLines {
		nonEmpty = nonEmpty[len(nonEmpty)-previewMaxLines:]
	}

	var collected []transcriptMessage
	for i := len(nonEmpty) - 1; i >= 0; i-- {
		var tl transcriptLine
		if err := json.Unmarshal([]byte(nonEmpty[i]), &tl); err != nil {
			continue
		}
		if len(tl.Message) == 0 {
			continue
		}
		var msg transcriptMessage
		if err := json.Unmarshal(tl.Message, &msg); err != nil {
			continue
		}
		collected = append(collected, msg)
		if len(collected) >= maxMessages {
			break
		}
	}
	// 翻转为正序
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected
}

// buildPreviewItems 构建预览条目列表。
func buildPreviewItems(messages []transcriptMessage, maxItems, maxChars int) []SessionPreviewItem {
	var items []SessionPreviewItem
	for _, msg := range messages {
		isTool := isToolCallMessage(msg)
		role := normalizePreviewRole(msg.Role, isTool)
		text := extractPreviewText(msg)
		if text == "" {
			continue
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		trimmed = truncatePreviewText(trimmed, maxChars)
		items = append(items, SessionPreviewItem{Role: role, Text: trimmed})
	}
	if len(items) > maxItems {
		items = items[len(items)-maxItems:]
	}
	return items
}

// toolCallTypes content 块中表示 tool call 的类型集合。
// 对齐 TS: src/utils/transcript-tools.ts TOOL_CALL_TYPES
var toolCallTypes = map[string]bool{
	"tool_use":  true,
	"toolcall":  true,
	"tool_call": true,
}

// isToolCallMessage 检测消息是否包含 tool call 块。
// 对齐 TS: src/utils/transcript-tools.ts hasToolCall() — 遍历 content 数组
// 查找 type 为 tool_use/toolcall/tool_call 的块。
func isToolCallMessage(msg transcriptMessage) bool {
	arr, ok := msg.Content.([]interface{})
	if !ok {
		return false
	}
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		if toolCallTypes[strings.TrimSpace(strings.ToLower(typ))] {
			return true
		}
	}
	return false
}

// normalizePreviewRole 标准化预览角色。
// 对齐 TS: session-utils.fs.ts normalizeRole(role, isTool) — 当 isTool 为 true 时优先返回 "tool"。
func normalizePreviewRole(role string, isTool bool) string {
	if isTool {
		return "tool"
	}
	switch strings.ToLower(role) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "tool":
		return "tool"
	default:
		return "other"
	}
}

// extractPreviewText 从消息中提取预览文本。
func extractPreviewText(msg transcriptMessage) string {
	text := extractTextFromContent(msg.Content)
	if text != "" {
		return text
	}
	if t := strings.TrimSpace(msg.Text); t != "" {
		return t
	}
	return ""
}

// truncatePreviewText 截断预览文本。
func truncatePreviewText(text string, maxChars int) string {
	if maxChars <= 0 || len([]rune(text)) <= maxChars {
		return text
	}
	runes := []rune(text)
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
}

// ---------- 内部辅助 ----------

// findExistingFile 找到第一个存在的文件。
func findExistingFile(candidates []string) string {
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// clampInt 将值限制在 [min, max] 范围内。
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// 确保 bufio.Scanner 用于未来扩展。
var _ = bufio.NewScanner
