// Package session — Agent session 文件 I/O 管理。
//
// 职责：session transcript 文件的读写操作 + 进程内文件锁。
// 桥接 internal/sessions（元数据层）和 internal/gateway（transcript 层），
// 为 Runner 提供统一的 session 文件管理接口。
//
// TS 参考: src/gateway/session-utils.fs.ts, src/gateway/session-utils.ts
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------- 常量 ----------

// CurrentTranscriptVersion 当前 transcript 文件格式版本。
const CurrentTranscriptVersion = 3

// ---------- TranscriptEntry ----------

// TranscriptEntry 写入 transcript 的消息条目。
type TranscriptEntry struct {
	Role       string                 `json:"role"`
	Content    []ContentBlock         `json:"content"`
	API        string                 `json:"api,omitempty"`
	Provider   string                 `json:"provider,omitempty"`
	Model      string                 `json:"model,omitempty"`
	Usage      map[string]interface{} `json:"usage,omitempty"`
	ToolCalls  map[string]int         `json:"toolCalls,omitempty"`
	StopReason string                 `json:"stopReason,omitempty"`
	Timestamp  int64                  `json:"timestamp"`
}

// ContentBlock 消息内容块。
// Type: "text" | "image" | "document" | "audio" | "video"
type ContentBlock struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	Source   *MediaSource `json:"source,omitempty"`
	FileName string       `json:"fileName,omitempty"`
	FileSize int64        `json:"fileSize,omitempty"`
	MimeType string       `json:"mimeType,omitempty"`
}

// MediaSource 媒体数据来源（base64 编码，适用于 image/video/audio）。
type MediaSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", etc.
	Data      string `json:"data"`       // base64 编码数据
}

// TranscriptHeader JSONL 文件的 header 行。
type TranscriptHeader struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd,omitempty"`
}

// ---------- SessionManager ----------

// SessionManager session 文件 I/O 管理器（进程内锁保护）。
// TS 参考: session-utils.fs.ts → readSessionMessages + archiveFileOnDisk
type SessionManager struct {
	mu        sync.Mutex
	storePath string
}

// NewSessionManager 创建 session 文件管理器。
func NewSessionManager(storePath string) *SessionManager {
	return &SessionManager{
		storePath: storePath,
	}
}

// LoadSessionMessages 从 transcript JSONL 文件读取消息列表。
// 跳过 header 行和无 role 字段的条目。
// TS 参考: session-utils.fs.ts → readSessionMessages()
func (m *SessionManager) LoadSessionMessages(sessionID, sessionFile string) ([]map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.resolveFilePath(sessionID, sessionFile)
	if filePath == "" {
		return nil, fmt.Errorf("无法解析 session 文件路径 (sessionID=%s)", sessionID)
	}

	return readTranscriptMessagesFromFile(filePath)
}

// AppendMessage 追加消息到 transcript 文件（带进程内锁）。
// TS 参考: session-utils.fs.ts + transcript.ts → appendAssistantMessageToSessionTranscript
func (m *SessionManager) AppendMessage(sessionID, sessionFile string, entry TranscriptEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.resolveFilePath(sessionID, sessionFile)
	if filePath == "" {
		return fmt.Errorf("无法解析 session 文件路径 (sessionID=%s)", sessionID)
	}

	// 确保文件存在
	if err := ensureTranscriptFile(filePath, sessionID); err != nil {
		return fmt.Errorf("确保 transcript 文件失败: %w", err)
	}

	// 设置时间戳
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UnixMilli()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("打开 transcript 文件失败: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("写入 transcript 失败: %w", err)
	}

	return nil
}

// EnsureSessionFile 确保 session 文件存在（创建目录 + header），返回最终文件路径。
// TS 参考: transcript.ts → ensureSessionHeader()
func (m *SessionManager) EnsureSessionFile(sessionID, sessionFile string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.resolveFilePath(sessionID, sessionFile)
	if filePath == "" {
		return "", fmt.Errorf("无法解析 session 文件路径 (sessionID=%s)", sessionID)
	}

	if err := ensureTranscriptFile(filePath, sessionID); err != nil {
		return "", err
	}

	return filePath, nil
}

// ResolveFilePath 解析 session 文件路径（公开接口）。
// 优先使用 sessionFile，其次根据 storePath + sessionID 推导。
// TS 参考: session-utils.fs.ts → resolveSessionTranscriptCandidates
func (m *SessionManager) ResolveFilePath(sessionID, sessionFile, agentID string) string {
	if sessionFile != "" {
		return sessionFile
	}
	return m.resolveFilePath(sessionID, "")
}

// ---------- Custom Entry API ----------

// CustomEntry 自定义条目（非消息），用于 session marker、model snapshot 等元数据持久化。
// TS 对照: SessionManager.appendCustomEntry() / getEntries()
type CustomEntry struct {
	Type       string      `json:"type"`           // 固定为 "custom"
	CustomType string      `json:"customType"`     // 自定义类型标识
	Data       interface{} `json:"data,omitempty"` // 自定义数据
	Timestamp  int64       `json:"timestamp"`      // Unix 毫秒时间戳
}

// AppendCustomEntry 追加自定义条目到 transcript 文件。
// TS 对照: google.ts → sessionManager.appendCustomEntry(customType, data)
func (m *SessionManager) AppendCustomEntry(sessionID, sessionFile, customType string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.resolveFilePath(sessionID, sessionFile)
	if filePath == "" {
		return fmt.Errorf("无法解析 session 文件路径 (sessionID=%s)", sessionID)
	}

	if err := ensureTranscriptFile(filePath, sessionID); err != nil {
		return fmt.Errorf("确保 transcript 文件失败: %w", err)
	}

	entry := CustomEntry{
		Type:       "custom",
		CustomType: customType,
		Data:       data,
		Timestamp:  time.Now().UnixMilli(),
	}

	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化自定义条目失败: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("打开 transcript 文件失败: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(entryData, '\n')); err != nil {
		return fmt.Errorf("写入自定义条目失败: %w", err)
	}

	return nil
}

// LoadAllEntries 读取 transcript 文件中的全部条目（含消息和自定义条目）。
// 跳过 header 行（type="session"），其余全部返回。
// TS 对照: SessionManager.getEntries()
func (m *SessionManager) LoadAllEntries(sessionID, sessionFile string) ([]map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := m.resolveFilePath(sessionID, sessionFile)
	if filePath == "" {
		return nil, fmt.Errorf("无法解析 session 文件路径 (sessionID=%s)", sessionID)
	}

	return readAllEntriesFromFile(filePath)
}

// ---------- 内部方法 ----------

// resolveFilePath 内部路径解析。
func (m *SessionManager) resolveFilePath(sessionID, sessionFile string) string {
	if sessionFile != "" {
		return sessionFile
	}
	if m.storePath == "" || sessionID == "" {
		return ""
	}
	dir := filepath.Dir(m.storePath)
	return filepath.Join(dir, sessionID+".jsonl")
}

// ---------- 包级辅助函数 ----------

// readTranscriptMessagesFromFile 从文件读取 transcript 消息。
func readTranscriptMessagesFromFile(filePath string) ([]map[string]interface{}, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在返回空列表
		}
		return nil, fmt.Errorf("打开 transcript 文件失败: %w", err)
	}
	defer f.Close()

	var messages []map[string]interface{}
	scanner := bufio.NewScanner(f)

	// 增大 buffer 处理大消息（最大 10MB）
	const maxScanTokenSize = 10 * 1024 * 1024
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
			slog.Debug("transcript 行解析错误", "line", lineNum, "error", err)
			continue
		}

		// 跳过 header 行
		if entryType, _ := entry["type"].(string); entryType == "session" {
			continue
		}

		// 只保留有 role 字段的消息
		if _, hasRole := entry["role"]; hasRole {
			messages = append(messages, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return messages, fmt.Errorf("transcript 扫描错误: %w", err)
	}

	return messages, nil
}

// readAllEntriesFromFile 从文件读取全部条目（含 custom entries），跳过 header。
func readAllEntriesFromFile(filePath string) ([]map[string]interface{}, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("打开 transcript 文件失败: %w", err)
	}
	defer f.Close()

	var entries []map[string]interface{}
	scanner := bufio.NewScanner(f)

	const maxScanTokenSize = 10 * 1024 * 1024
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
			slog.Debug("transcript 行解析错误", "line", lineNum, "error", err)
			continue
		}

		// 跳过 header 行
		if entryType, _ := entry["type"].(string); entryType == "session" {
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("transcript 扫描错误: %w", err)
	}

	return entries, nil
}

// ensureTranscriptFile 确保 transcript 文件存在并包含 header。
func ensureTranscriptFile(filePath, sessionID string) error {
	if _, err := os.Stat(filePath); err == nil {
		return nil // 文件已存在
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 transcript 目录失败: %w", err)
	}

	cwd, _ := os.Getwd()
	header := TranscriptHeader{
		Type:      "session",
		Version:   CurrentTranscriptVersion,
		ID:        sessionID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Cwd:       cwd,
	}
	data, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("序列化 header 失败: %w", err)
	}

	return os.WriteFile(filePath, append(data, '\n'), 0o644)
}
