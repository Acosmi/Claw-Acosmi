package llmclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- Anthropic Messages API 流式客户端 ----------
// 对齐 TS: @mariozechner/pi-ai streamSimple (Anthropic provider)
// API 文档: https://docs.anthropic.com/en/api/messages-streaming

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicAPIVersion     = "2023-06-01"
	defaultMaxTokens        = 8192
)

// ---------- OPENACOSMI_ANTHROPIC_PAYLOAD_LOG 调试支持 ----------
// 对应 TS: src/agents/anthropic-payload-log.ts

// payloadLogWriter 懒初始化的日志文件写入器（进程级单例）。
type payloadLogWriter struct {
	once sync.Once
	mu   sync.Mutex
	f    *os.File
	path string
}

var globalPayloadLogWriter payloadLogWriter

// isPayloadLogEnabled 判断 payload 日志是否启用。
func isPayloadLogEnabled() bool {
	v := os.Getenv("OPENACOSMI_ANTHROPIC_PAYLOAD_LOG")
	return v == "1" || strings.ToLower(v) == "true"
}

// resolvePayloadLogFilePath 解析 payload 日志文件路径。
func resolvePayloadLogFilePath() string {
	if v := strings.TrimSpace(os.Getenv("OPENACOSMI_ANTHROPIC_PAYLOAD_LOG_FILE")); v != "" {
		return v
	}
	return filepath.Join(config.ResolveStateDir(), "logs", "anthropic-payload.jsonl")
}

// writePayloadLogLine 将 JSON 行写入 payload 日志文件。
// 按需懒初始化写入器；写入失败时静默记录 warn。
func writePayloadLogLine(line string) {
	globalPayloadLogWriter.once.Do(func() {
		globalPayloadLogWriter.path = resolvePayloadLogFilePath()
	})
	globalPayloadLogWriter.mu.Lock()
	defer globalPayloadLogWriter.mu.Unlock()
	if globalPayloadLogWriter.f == nil {
		dir := filepath.Dir(globalPayloadLogWriter.path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Warn("llmclient: payload log dir create failed", "err", err)
			return
		}
		f, err := os.OpenFile(globalPayloadLogWriter.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			slog.Warn("llmclient: payload log open failed", "err", err)
			return
		}
		globalPayloadLogWriter.f = f
	}
	_, _ = fmt.Fprintln(globalPayloadLogWriter.f, line)
}

// logAnthropicRequestPayload 记录 Anthropic 请求 payload（当日志启用时）。
// stage 为 "request"；数据含时间戳、stage、payload 摘要及原始字节。
func logAnthropicRequestPayload(bodyBytes []byte) {
	if !isPayloadLogEnabled() {
		return
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(bodyBytes))
	entry := map[string]interface{}{
		"ts":            time.Now().UTC().Format(time.RFC3339Nano),
		"stage":         "request",
		"payloadDigest": digest,
		"payload":       json.RawMessage(bodyBytes),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	writePayloadLogLine(string(data))
}

// logAnthropicUsagePayload 记录 Anthropic 响应 usage（当日志启用时）。
func logAnthropicUsagePayload(usage UsageInfo) {
	if !isPayloadLogEnabled() {
		return
	}
	entry := map[string]interface{}{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"stage": "usage",
		"usage": map[string]interface{}{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	writePayloadLogLine(string(data))
}

// anthropicStreamChat 调用 Anthropic Messages API (流式)。
func anthropicStreamChat(ctx context.Context, req ChatRequest, onEvent func(StreamEvent)) (*ChatResult, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/messages"

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	// 构建请求 body
	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    true,
		Messages:  toAnthropicMessages(req.Messages),
	}
	if req.SystemPrompt != "" {
		body.System = req.SystemPrompt
	}
	if len(req.Tools) > 0 {
		body.Tools = toAnthropicTools(req.Tools)
	}
	if req.Temperature != nil {
		body.Temperature = req.Temperature
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal anthropic request: %w", err)
	}

	// L-3: OPENACOSMI_ANTHROPIC_PAYLOAD_LOG — 记录请求 payload
	logAnthropicRequestPayload(bodyBytes)

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	// E-6: extended thinking 需要额外的 beta 请求头
	// 对应 TS: anthropic-beta: interleaved-thinking-2025-05-14
	if req.ThinkLevel != "" && req.ThinkLevel != "off" {
		httpReq.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient: anthropic HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAnthropicError(resp)
	}

	result, err := parseAnthropicSSE(resp.Body, onEvent)
	// L-3: OPENACOSMI_ANTHROPIC_PAYLOAD_LOG — 记录响应 usage
	if result != nil {
		logAnthropicUsagePayload(result.Usage)
	}
	return result, err
}

// ---------- Anthropic 请求结构 ----------

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string 或 []content_block
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ---------- 消息转换 ----------

func toAnthropicMessages(msgs []ChatMessage) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue // system 走顶层 system 字段
		}
		am := anthropicMessage{Role: m.Role}

		// 优化: 纯文本消息直接用 string
		if len(m.Content) == 1 && m.Content[0].Type == "text" {
			data, _ := json.Marshal(m.Content[0].Text)
			am.Content = data
		} else {
			blocks := make([]map[string]interface{}, 0, len(m.Content))
			for _, b := range m.Content {
				block := map[string]interface{}{"type": b.Type}
				switch b.Type {
				case "text":
					block["text"] = b.Text
				case "image":
					if b.Source != nil {
						block["source"] = map[string]interface{}{
							"type":       b.Source.Type,
							"media_type": b.Source.MediaType,
							"data":       b.Source.Data,
						}
					}
				case "tool_use":
					block["id"] = b.ID
					block["name"] = b.Name
					if b.Input != nil {
						block["input"] = json.RawMessage(b.Input)
					}
				case "tool_result":
					block["tool_use_id"] = b.ToolUseID
					if b.IsError {
						block["is_error"] = true
					}
					// 多模态 tool_result: ResultBlocks 包含 image + text blocks
					if len(b.ResultBlocks) > 0 {
						var contentBlocks []map[string]interface{}
						for _, rb := range b.ResultBlocks {
							switch rb.Type {
							case "text":
								contentBlocks = append(contentBlocks, map[string]interface{}{
									"type": "text",
									"text": rb.Text,
								})
							case "image":
								if rb.Source != nil {
									contentBlocks = append(contentBlocks, map[string]interface{}{
										"type": "image",
										"source": map[string]interface{}{
											"type":       rb.Source.Type,
											"media_type": rb.Source.MediaType,
											"data":       rb.Source.Data,
										},
									})
								}
							}
						}
						block["content"] = contentBlocks
					} else {
						block["content"] = b.ResultText
					}
				}
				blocks = append(blocks, block)
			}
			data, _ := json.Marshal(blocks)
			am.Content = data
		}
		out = append(out, am)
	}
	return out
}

func toAnthropicTools(tools []ToolDef) []anthropicTool {
	out := make([]anthropicTool, len(tools))
	for i, t := range tools {
		out[i] = anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return out
}

// ---------- SSE 解析 ----------

func parseAnthropicSSE(r io.Reader, onEvent func(StreamEvent)) (*ChatResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	var (
		result      ChatResult
		textBuf     strings.Builder
		toolUses    []ContentBlock
		curToolIdx  = -1
		curToolID   string
		curToolName string
		inputBuf    strings.Builder
		eventType   string
	)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		switch eventType {
		case "message_start":
			// 提取 usage
			var msg struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(data), &msg) == nil {
				result.Usage.InputTokens = msg.Message.Usage.InputTokens
			}

		case "content_block_start":
			var block struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if json.Unmarshal([]byte(data), &block) == nil {
				if block.ContentBlock.Type == "tool_use" {
					curToolIdx = block.Index
					curToolID = block.ContentBlock.ID
					curToolName = block.ContentBlock.Name
					inputBuf.Reset()
					onEvent(StreamEvent{
						Type: EventToolUseStart,
						ToolUse: &ToolUseEvent{
							ID:   block.ContentBlock.ID,
							Name: block.ContentBlock.Name,
						},
					})
				}
			}

		case "content_block_delta":
			var delta struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &delta) == nil {
				switch delta.Delta.Type {
				case "text_delta":
					textBuf.WriteString(delta.Delta.Text)
					onEvent(StreamEvent{Type: EventText, Text: delta.Delta.Text})
				case "input_json_delta":
					inputBuf.WriteString(delta.Delta.PartialJSON)
					onEvent(StreamEvent{
						Type: EventToolUseInput,
						ToolUse: &ToolUseEvent{
							InputDelta: delta.Delta.PartialJSON,
						},
					})
				}
			}

		case "content_block_stop":
			if curToolIdx >= 0 {
				inputJSON := json.RawMessage(inputBuf.String())
				toolUses = append(toolUses, ContentBlock{
					Type:  "tool_use",
					ID:    curToolID,
					Name:  curToolName,
					Input: inputJSON,
				})
				curToolIdx = -1
				curToolID = ""
				curToolName = ""
			}

		case "message_delta":
			var md struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &md) == nil {
				result.StopReason = md.Delta.StopReason
				result.Usage.OutputTokens = md.Usage.OutputTokens
				onEvent(StreamEvent{
					Type:       EventStop,
					StopReason: md.Delta.StopReason,
				})
			}

		case "message_stop":
			onEvent(StreamEvent{Type: EventUsage, Usage: &result.Usage})

		case "ping":
			onEvent(StreamEvent{Type: EventPing})

		case "error":
			var errData struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(data), &errData) == nil {
				onEvent(StreamEvent{
					Type:  EventError,
					Error: errData.Error.Type + ": " + errData.Error.Message,
				})
			}
		}

		eventType = "" // reset for next event
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("llmclient: anthropic SSE scan: %w", err)
	}

	// 构建最终 assistant message
	blocks := make([]ContentBlock, 0)
	if textBuf.Len() > 0 {
		blocks = append(blocks, ContentBlock{Type: "text", Text: textBuf.String()})
	}
	blocks = append(blocks, toolUses...)
	result.AssistantMessage = ChatMessage{Role: "assistant", Content: blocks}

	return &result, nil
}

// ---------- 错误解析 ----------

func parseAnthropicError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	apiErr := &APIError{StatusCode: resp.StatusCode}

	var errResp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		apiErr.Type = errResp.Error.Type
		apiErr.Message = errResp.Error.Message
	} else {
		apiErr.Type = "http_error"
		apiErr.Message = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	apiErr.Retryable = apiErr.IsRateLimit() || apiErr.IsOverloaded() || resp.StatusCode >= 500
	return apiErr
}
