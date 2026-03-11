// Package llmclient 提供统一的 LLM API 流式调用接口。
//
// 对齐 TS: @mariozechner/pi-ai 的 streamSimple / streamChat
// Go 策略: 不引入第三方 SDK, 用标准库 net/http + SSE 解析实现。
package llmclient

import (
	"encoding/json"
)

// ---------- 消息类型 ----------

// ChatMessage 统一消息格式（兼容 Anthropic / OpenAI / Ollama）。
type ChatMessage struct {
	Role    string         `json:"role"`    // "user" | "assistant" | "system"
	Content []ContentBlock `json:"content"` // 文本 / tool_use / tool_result
}

// TextMessage 创建纯文本消息的快捷方法。
func TextMessage(role, text string) ChatMessage {
	return ChatMessage{
		Role:    role,
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

// ImageSource 图片数据来源（Anthropic API 格式）。
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", "image/gif", "image/webp"
	Data      string `json:"data"`       // base64 编码数据
}

// ContentBlock 消息内容块。
type ContentBlock struct {
	Type string `json:"type"` // "text" | "tool_use" | "tool_result" | "thinking" | "image"

	// text
	Text string `json:"text,omitempty"`

	// thinking block (promoted from <thinking> tags)
	Thinking string `json:"thinking,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`    // tool call ID
	Name  string          `json:"name,omitempty"`  // tool name
	Input json.RawMessage `json:"input,omitempty"` // tool input JSON

	// tool_result
	ToolUseID    string         `json:"tool_use_id,omitempty"`
	IsError      bool           `json:"is_error,omitempty"`
	ResultText   string         `json:"result_text,omitempty"`   // tool result 纯文本
	ResultBlocks []ContentBlock `json:"result_blocks,omitempty"` // tool result 多模态内容（含 image blocks）

	// image (Anthropic API: type="image" + source={type, media_type, data})
	Source *ImageSource `json:"source,omitempty"`

	// thinking block 签名 (Anthropic extended thinking)
	// 规范化为统一字段名，序列化时输出 thinkingSignature。
	// 反序列化时识别多种字段名：
	//   "thinkingSignature" (Anthropic)
	//   "signature"         (Anthropic 旧版)
	//   "thought_signature" (Google/Gemini)
	//   "thoughtSignature"  (OpenAI 风格)
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
}

// contentBlockAlias 用于自定义 UnmarshalJSON 时避免无限递归。
type contentBlockAlias ContentBlock

// rawContentBlock 用于捕获多种 thinking 签名字段名。
type rawContentBlock struct {
	contentBlockAlias
	// 多格式 thinking 签名字段
	Signature         string `json:"signature,omitempty"`
	ThoughtSignature  string `json:"thought_signature,omitempty"`
	ThoughtSignature2 string `json:"thoughtSignature,omitempty"`
}

// UnmarshalJSON 实现自定义反序列化，识别多种 thinking 签名字段名。
// 对应 TS google.ts sanitizeAntigravityThinkingBlocks 的映射逻辑：
//
//	thinkingSignature ?? signature ?? thought_signature ?? thoughtSignature
func (c *ContentBlock) UnmarshalJSON(data []byte) error {
	var raw rawContentBlock
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = ContentBlock(raw.contentBlockAlias)

	// 若 ThinkingSignature 未被填充，依次检查其他字段名
	if c.ThinkingSignature == "" {
		switch {
		case raw.Signature != "":
			c.ThinkingSignature = raw.Signature
		case raw.ThoughtSignature != "":
			c.ThinkingSignature = raw.ThoughtSignature
		case raw.ThoughtSignature2 != "":
			c.ThinkingSignature = raw.ThoughtSignature2
		}
	}
	return nil
}

// ---------- 流式事件 ----------

// StreamEventType 流式事件类型枚举。
type StreamEventType string

const (
	EventText         StreamEventType = "text"
	EventToolUseStart StreamEventType = "tool_use_start"
	EventToolUseInput StreamEventType = "tool_use_input"
	EventStop         StreamEventType = "stop"
	EventError        StreamEventType = "error"
	EventUsage        StreamEventType = "usage"
	EventPing         StreamEventType = "ping"
)

// StreamEvent 从 LLM API 收到的单个流式事件。
type StreamEvent struct {
	Type       StreamEventType
	Text       string        // EventText: 增量文本
	ToolUse    *ToolUseEvent // EventToolUseStart / EventToolUseInput
	Usage      *UsageInfo    // EventUsage
	Error      string        // EventError
	StopReason string        // EventStop: "end_turn" | "tool_use" | "max_tokens"
}

// ToolUseEvent 工具调用流式事件。
type ToolUseEvent struct {
	ID         string          // tool call ID
	Name       string          // tool name
	InputDelta string          // 增量 JSON 输入片段
	InputFull  json.RawMessage // 完成后的完整 JSON（仅 stop 时填充）
}

// UsageInfo token 使用统计。
type UsageInfo struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_input_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_creation_input_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// ---------- 请求 / 响应 ----------

// ChatRequest 统一 LLM 调用请求。
type ChatRequest struct {
	Provider     string        // "anthropic" | "openai" | "ollama" | custom
	Model        string        // 模型 ID
	SystemPrompt string        // 系统提示词
	Messages     []ChatMessage // 对话历史
	Tools        []ToolDef     // 可用工具定义
	MaxTokens    int           // 最大输出 token
	ThinkLevel   string        // "off" | "low" | "medium" | "high"
	TimeoutMs    int64         // 超时毫秒
	APIKey       string        // API Key
	AuthMode     string        // "oauth" = Bearer token; "" | "key" = API key header
	BaseURL      string        // 自定义 API Base URL（可选）
	Temperature  *float64      // 可选温度
}

// ToolDef 工具定义（Anthropic tool schema 格式）。
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

// ChatResult 非流式最终结果。
type ChatResult struct {
	AssistantMessage ChatMessage // 完整的 assistant 回复
	StopReason       string      // "end_turn" | "tool_use" | "max_tokens"
	Usage            UsageInfo
}

// ---------- 错误类型 ----------

// APIError LLM API 返回的错误。
type APIError struct {
	StatusCode int
	Type       string // "rate_limit_error" | "overloaded_error" | ...
	Message    string
	Retryable  bool
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Type + ": " + e.Message
	}
	return e.Type
}

// IsRateLimit 检查是否为限流错误。
func (e *APIError) IsRateLimit() bool {
	return e.StatusCode == 429 || e.Type == "rate_limit_error"
}

// IsOverloaded 检查是否为过载错误。
func (e *APIError) IsOverloaded() bool {
	return e.StatusCode == 529 || e.Type == "overloaded_error"
}
