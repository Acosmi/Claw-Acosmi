// bash/cache_trace.go — 缓存追踪诊断。
// TS 参考：src/agents/cache-trace.ts (295L)
//
// 记录会话生命周期各阶段事件到 JSONL 文件，
// 用于调试 LLM 缓存命中和提示工程。
package bash

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- 类型 ----------

// CacheTraceStage 追踪阶段。
type CacheTraceStage string

const (
	StageSessionLoaded    CacheTraceStage = "session:loaded"
	StageSessionSanitized CacheTraceStage = "session:sanitized"
	StageSessionLimited   CacheTraceStage = "session:limited"
	StagePromptBefore     CacheTraceStage = "prompt:before"
	StagePromptImages     CacheTraceStage = "prompt:images"
	StageStreamContext    CacheTraceStage = "stream:context"
	StageSessionAfter     CacheTraceStage = "session:after"
)

// CacheTraceEvent 追踪事件。
// TS 参考: cache-trace.ts L19-42
type CacheTraceEvent struct {
	Timestamp           string          `json:"ts"`
	Seq                 int             `json:"seq"`
	Stage               CacheTraceStage `json:"stage"`
	RunID               string          `json:"runId,omitempty"`
	SessionID           string          `json:"sessionId,omitempty"`
	SessionKey          string          `json:"sessionKey,omitempty"`
	Provider            string          `json:"provider,omitempty"`
	ModelID             string          `json:"modelId,omitempty"`
	ModelAPI            string          `json:"modelApi,omitempty"`
	WorkspaceDir        string          `json:"workspaceDir,omitempty"`
	Prompt              string          `json:"prompt,omitempty"`
	System              any             `json:"system,omitempty"`
	Options             map[string]any  `json:"options,omitempty"`
	Model               map[string]any  `json:"model,omitempty"`
	Messages            []any           `json:"messages,omitempty"`
	MessageCount        int             `json:"messageCount,omitempty"`
	MessageRoles        []string        `json:"messageRoles,omitempty"`
	MessageFingerprints []string        `json:"messageFingerprints,omitempty"`
	MessagesDigest      string          `json:"messagesDigest,omitempty"`
	SystemDigest        string          `json:"systemDigest,omitempty"`
	Note                string          `json:"note,omitempty"`
	Error               string          `json:"error,omitempty"`
}

// CacheTraceConfig 追踪配置。
type CacheTraceConfig struct {
	Enabled         bool   `json:"enabled"`
	FilePath        string `json:"filePath"`
	IncludeMessages bool   `json:"includeMessages"`
	IncludePrompt   bool   `json:"includePrompt"`
	IncludeSystem   bool   `json:"includeSystem"`
}

// CacheTraceInit 追踪初始化参数。
type CacheTraceInit struct {
	Enabled         bool
	FilePath        string
	RunID           string
	SessionID       string
	SessionKey      string
	Provider        string
	ModelID         string
	ModelAPI        string
	WorkspaceDir    string
	IncludeMessages bool
	IncludePrompt   bool
	IncludeSystem   bool
}

// ---------- CacheTrace ----------

// CacheTrace 缓存追踪器。
type CacheTrace struct {
	mu       sync.Mutex
	cfg      CacheTraceConfig
	base     CacheTraceEvent // 基本字段（不含 ts/seq/stage）
	seq      int
	writer   *traceWriter
	Enabled  bool
	FilePath string
}

// CreateCacheTrace 创建缓存追踪器。
// TS 参考: cache-trace.ts L204-294
func CreateCacheTrace(params CacheTraceInit) *CacheTrace {
	cfg := resolveCacheTraceConfig(params)
	if !cfg.Enabled {
		return nil
	}

	return &CacheTrace{
		cfg: cfg,
		base: CacheTraceEvent{
			RunID:        params.RunID,
			SessionID:    params.SessionID,
			SessionKey:   params.SessionKey,
			Provider:     params.Provider,
			ModelID:      params.ModelID,
			ModelAPI:     params.ModelAPI,
			WorkspaceDir: params.WorkspaceDir,
		},
		writer:   getOrCreateWriter(cfg.FilePath),
		Enabled:  true,
		FilePath: cfg.FilePath,
	}
}

// RecordStage 记录阶段事件。
// TS 参考: cache-trace.ts L223-269
func (ct *CacheTrace) RecordStage(stage CacheTraceStage, payload *CacheTraceEvent) {
	if ct == nil || !ct.Enabled {
		return
	}

	ct.mu.Lock()
	ct.seq++
	seq := ct.seq
	ct.mu.Unlock()

	event := ct.base
	event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	event.Seq = seq
	event.Stage = stage

	if payload != nil {
		if payload.Prompt != "" && ct.cfg.IncludePrompt {
			event.Prompt = payload.Prompt
		}
		if payload.System != nil && ct.cfg.IncludeSystem {
			event.System = payload.System
			event.SystemDigest = digest(payload.System)
		}
		if payload.Options != nil {
			event.Options = payload.Options
		}
		if payload.Model != nil {
			event.Model = payload.Model
		}
		if payload.Messages != nil {
			summary := summarizeMessages(payload.Messages)
			event.MessageCount = summary.messageCount
			event.MessageRoles = summary.messageRoles
			event.MessageFingerprints = summary.messageFingerprints
			event.MessagesDigest = summary.messagesDigest
			if ct.cfg.IncludeMessages {
				event.Messages = payload.Messages
			}
		}
		if payload.Note != "" {
			event.Note = payload.Note
		}
		if payload.Error != "" {
			event.Error = payload.Error
		}
	}

	line, err := safeJSONStringify(event)
	if err != nil || line == "" {
		return
	}
	ct.writer.write(line + "\n")
}

// ---------- 配置解析 ----------

func resolveCacheTraceConfig(params CacheTraceInit) CacheTraceConfig {
	enabled := params.Enabled
	if envVal := os.Getenv("OPENACOSMI_CACHE_TRACE"); envVal != "" {
		enabled = parseBoolEnv(envVal)
	}

	filePath := params.FilePath
	if filePath == "" {
		if envPath := os.Getenv("OPENACOSMI_CACHE_TRACE_FILE"); envPath != "" {
			filePath = strings.TrimSpace(envPath)
		}
	}
	if filePath == "" {
		stateDir := os.Getenv("OPENACOSMI_STATE_DIR")
		if stateDir == "" {
			home, _ := os.UserHomeDir()
			stateDir = filepath.Join(home, ".openacosmi")
		}
		filePath = filepath.Join(stateDir, "logs", "cache-trace.jsonl")
	}

	includeMessages := params.IncludeMessages
	if envVal := os.Getenv("OPENACOSMI_CACHE_TRACE_MESSAGES"); envVal != "" {
		includeMessages = parseBoolEnv(envVal)
	}
	includePrompt := params.IncludePrompt
	if envVal := os.Getenv("OPENACOSMI_CACHE_TRACE_PROMPT"); envVal != "" {
		includePrompt = parseBoolEnv(envVal)
	}
	includeSystem := params.IncludeSystem
	if envVal := os.Getenv("OPENACOSMI_CACHE_TRACE_SYSTEM"); envVal != "" {
		includeSystem = parseBoolEnv(envVal)
	}

	return CacheTraceConfig{
		Enabled:         enabled,
		FilePath:        filePath,
		IncludeMessages: includeMessages,
		IncludePrompt:   includePrompt,
		IncludeSystem:   includeSystem,
	}
}

func parseBoolEnv(val string) bool {
	v := strings.TrimSpace(strings.ToLower(val))
	return v == "1" || v == "true" || v == "yes"
}

// ---------- Writer ----------

var (
	writersMu sync.Mutex
	writers   = make(map[string]*traceWriter)
)

type traceWriter struct {
	filePath string
	mu       sync.Mutex
	ready    bool
}

func getOrCreateWriter(filePath string) *traceWriter {
	writersMu.Lock()
	defer writersMu.Unlock()
	if w, ok := writers[filePath]; ok {
		return w
	}
	w := &traceWriter{filePath: filePath}
	writers[filePath] = w

	// 异步创建目录
	go func() {
		dir := filepath.Dir(filePath)
		_ = os.MkdirAll(dir, 0755)
		w.mu.Lock()
		w.ready = true
		w.mu.Unlock()
	}()

	return w
}

func (w *traceWriter) write(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.ready {
		dir := filepath.Dir(w.filePath)
		_ = os.MkdirAll(dir, 0755)
		w.ready = true
	}
	f, err := os.OpenFile(w.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Debug("cache trace write failed", "path", w.filePath, "err", err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// ---------- 摘要工具 ----------

type messageSummary struct {
	messageCount        int
	messageRoles        []string
	messageFingerprints []string
	messagesDigest      string
}

func summarizeMessages(messages []any) messageSummary {
	fingerprints := make([]string, len(messages))
	roles := make([]string, len(messages))

	for i, msg := range messages {
		fingerprints[i] = digest(msg)
		if m, ok := msg.(map[string]any); ok {
			if role, ok := m["role"].(string); ok {
				roles[i] = role
			}
		}
	}

	return messageSummary{
		messageCount:        len(messages),
		messageRoles:        roles,
		messageFingerprints: fingerprints,
		messagesDigest:      digest(strings.Join(fingerprints, "|")),
	}
}

// digest 计算 SHA-256 摘要。
// TS 参考: cache-trace.ts L162-165
func digest(value any) string {
	serialized := stableStringify(value)
	h := sha256.Sum256([]byte(serialized))
	return fmt.Sprintf("%x", h)
}

// stableStringify 稳定序列化（键排序）。
// TS 参考: cache-trace.ts L127-160
func stableStringify(value any) string {
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case string:
		b, _ := json.Marshal(v)
		return string(b)
	case float64, float32, int, int64, int32, bool:
		b, _ := json.Marshal(v)
		return string(b)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = stableStringify(item)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			keyJSON, _ := json.Marshal(k)
			parts[i] = string(keyJSON) + ":" + stableStringify(v[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "null"
		}
		return string(b)
	}
}

func safeJSONStringify(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
