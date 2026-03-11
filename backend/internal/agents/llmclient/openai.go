package llmclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------- OpenAI Chat Completions API 流式客户端 ----------
// 对齐 TS: @mariozechner/pi-ai streamSimple (OpenAI provider)
// API 文档: https://platform.openai.com/docs/api-reference/chat/create

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// providerDefaultBaseURLs maps OpenAI-compatible providers to their default API endpoints.
// Prevents silent misrouting when BaseURL is empty (e.g. DeepSeek → api.openai.com).
var providerDefaultBaseURLs = map[string]string{
	"deepseek":          "https://api.deepseek.com/v1",
	"deepseek-reasoner": "https://api.deepseek.com/v1",
	"moonshot":          "https://api.moonshot.cn/v1",
	"kimi":              "https://api.moonshot.cn/v1",
	"qwen":              "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"qwen-portal":       "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"minimax":           "https://api.minimax.chat/v1",
	"zai":               "https://open.bigmodel.cn/api/paas/v4",
	"zhipu":             "https://open.bigmodel.cn/api/paas/v4",
	"doubao":            "https://ark.cn-beijing.volces.com/api/v3",
	"xai":               "https://api.x.ai/v1",
}

// openaiStreamChat 调用 OpenAI Chat Completions API (流式)。
func openaiStreamChat(ctx context.Context, req ChatRequest, onEvent func(StreamEvent)) (*ChatResult, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		if providerURL, ok := providerDefaultBaseURLs[strings.ToLower(req.Provider)]; ok {
			baseURL = providerURL
		} else {
			baseURL = defaultOpenAIBaseURL
		}
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	body := openaiRequest{
		Model:         req.Model,
		MaxTokens:     maxTokens,
		Stream:        true,
		StreamOptions: &openaiStreamOptions{IncludeUsage: true},
		Messages:      toOpenAIMessages(req.SystemPrompt, req.Messages),
	}
	if len(req.Tools) > 0 {
		body.Tools = toOpenAITools(req.Tools)
	}
	if req.Temperature != nil {
		body.Temperature = req.Temperature
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal openai request: %w", err)
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient: openai HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseOpenAIError(resp)
	}

	return parseOpenAISSE(resp.Body, onEvent)
}

// ---------- OpenAI 请求结构 ----------

type openaiRequest struct {
	Model         string               `json:"model"`
	MaxTokens     int                  `json:"max_tokens"`
	Stream        bool                 `json:"stream"`
	StreamOptions *openaiStreamOptions `json:"stream_options,omitempty"`
	Messages      []openaiMessage      `json:"messages"`
	Tools         []openaiTool         `json:"tools,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
}

type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"` // string 或多模态 content parts。DeepSeek 要求 assistant 消息必须有 content 字段。
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIContentPart struct {
	Type     string             `json:"type"`
	Text     string             `json:"text,omitempty"`
	ImageURL *openAIImageURLRef `json:"image_url,omitempty"`
}

type openAIImageURLRef struct {
	URL string `json:"url"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiTool struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// ---------- 消息转换 ----------

// openaiToolResultText 提取 tool_result 的文本内容（OpenAI 不支持 image tool results）。
func openaiToolResultText(b ContentBlock) string {
	if b.ResultText != "" {
		return b.ResultText
	}
	if len(b.ResultBlocks) > 0 {
		var parts []string
		for _, rb := range b.ResultBlocks {
			if rb.Type == "text" && rb.Text != "" {
				parts = append(parts, rb.Text)
			} else if rb.Type == "image" {
				parts = append(parts, "[image attached]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func toOpenAIMessages(systemPrompt string, msgs []ChatMessage) []openaiMessage {
	out := make([]openaiMessage, 0, len(msgs)+1)
	if systemPrompt != "" {
		out = append(out, openaiMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		// 检查是否包含 tool_use / tool_result
		hasToolUse := false
		hasToolResult := false
		hasImage := false
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				hasToolUse = true
			}
			if b.Type == "tool_result" {
				hasToolResult = true
			}
			if b.Type == "image" {
				hasImage = true
			}
		}

		if m.Role == "assistant" && hasToolUse {
			// 转换为 OpenAI tool_calls 格式
			om := openaiMessage{Role: "assistant", Content: ""}
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					om.Content = b.Text
				case "tool_use":
					om.ToolCalls = append(om.ToolCalls, openaiToolCall{
						ID:   b.ID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      b.Name,
							Arguments: string(b.Input),
						},
					})
				}
			}
			out = append(out, om)
		} else if hasToolResult {
			// 每个 tool_result 变成独立的 "tool" 消息
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					out = append(out, openaiMessage{
						Role:       "tool",
						Content:    openaiToolResultText(b),
						ToolCallID: b.ToolUseID,
					})
				}
			}
		} else if m.Role == "user" && hasImage {
			parts := make([]openAIContentPart, 0, len(m.Content))
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					if b.Text != "" {
						parts = append(parts, openAIContentPart{
							Type: "text",
							Text: b.Text,
						})
					}
				case "image":
					if b.Source == nil || b.Source.Data == "" || b.Source.MediaType == "" {
						continue
					}
					parts = append(parts, openAIContentPart{
						Type: "image_url",
						ImageURL: &openAIImageURLRef{
							URL: "data:" + b.Source.MediaType + ";base64," + b.Source.Data,
						},
					})
				}
			}
			if len(parts) == 0 {
				out = append(out, openaiMessage{Role: m.Role, Content: ""})
			} else {
				out = append(out, openaiMessage{Role: m.Role, Content: parts})
			}
		} else {
			// 纯文本
			var text strings.Builder
			for _, b := range m.Content {
				if b.Type == "text" {
					text.WriteString(b.Text)
				}
			}
			out = append(out, openaiMessage{Role: m.Role, Content: text.String()})
		}
	}
	return out
}

func toOpenAITools(tools []ToolDef) []openaiTool {
	out := make([]openaiTool, len(tools))
	for i, t := range tools {
		out[i] = openaiTool{Type: "function"}
		out[i].Function.Name = t.Name
		out[i].Function.Description = t.Description
		out[i].Function.Parameters = t.InputSchema
	}
	return out
}

// ---------- SSE 解析 ----------

func parseOpenAISSE(r io.Reader, onEvent func(StreamEvent)) (*ChatResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		result    ChatResult
		textBuf   strings.Builder
		toolCalls []openaiToolCall
		inputBufs = map[int]*strings.Builder{} // tool call index → accumulated args
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage map[string]interface{} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			// 文本增量
			if delta.Content != "" {
				textBuf.WriteString(delta.Content)
				onEvent(StreamEvent{Type: EventText, Text: delta.Content})
			}

			// 工具调用增量
			for _, tc := range delta.ToolCalls {
				if tc.ID != "" {
					// 新工具调用开始
					for len(toolCalls) <= tc.Index {
						toolCalls = append(toolCalls, openaiToolCall{Type: "function"})
					}
					toolCalls[tc.Index].ID = tc.ID
					toolCalls[tc.Index].Function.Name = tc.Function.Name
					inputBufs[tc.Index] = &strings.Builder{}
					onEvent(StreamEvent{
						Type: EventToolUseStart,
						ToolUse: &ToolUseEvent{
							ID:   tc.ID,
							Name: tc.Function.Name,
						},
					})
				}
				if tc.Function.Arguments != "" {
					if buf, ok := inputBufs[tc.Index]; ok {
						buf.WriteString(tc.Function.Arguments)
					}
					onEvent(StreamEvent{
						Type: EventToolUseInput,
						ToolUse: &ToolUseEvent{
							InputDelta: tc.Function.Arguments,
						},
					})
				}
			}

			// 终止
			if chunk.Choices[0].FinishReason != nil {
				reason := *chunk.Choices[0].FinishReason
				var stopReason string
				switch reason {
				case "stop":
					stopReason = "end_turn"
				case "tool_calls":
					stopReason = "tool_use"
				case "length":
					stopReason = "max_tokens"
				default:
					stopReason = reason
				}
				result.StopReason = stopReason
				onEvent(StreamEvent{Type: EventStop, StopReason: stopReason})
			}
		}

		if chunk.Usage != nil {
			mergeOpenAIUsage(&result.Usage, chunk.Usage)
			onEvent(StreamEvent{Type: EventUsage, Usage: &result.Usage})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("llmclient: openai SSE scan: %w", err)
	}

	// 构建最终 assistant message
	blocks := make([]ContentBlock, 0)
	if textBuf.Len() > 0 {
		blocks = append(blocks, ContentBlock{Type: "text", Text: textBuf.String()})
	}
	// 转换 tool_calls → ContentBlocks
	for i, tc := range toolCalls {
		args := ""
		if buf, ok := inputBufs[i]; ok {
			args = buf.String()
		}
		blocks = append(blocks, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}
	result.AssistantMessage = ChatMessage{Role: "assistant", Content: blocks}

	return &result, nil
}

func mergeOpenAIUsage(dst *UsageInfo, raw map[string]interface{}) {
	if dst == nil || raw == nil {
		return
	}

	input := firstPositiveInt(
		readIntFromMap(raw, "input_tokens", "prompt_tokens"),
	)
	output := firstPositiveInt(
		readIntFromMap(raw, "output_tokens", "completion_tokens"),
	)
	cacheRead := firstPositiveInt(
		readIntFromMap(raw, "cache_read_input_tokens", "cache_read"),
		readNestedIntFromMap(raw, "prompt_tokens_details", "cached_tokens"),
		readNestedIntFromMap(raw, "input_tokens_details", "cached_tokens"),
	)
	cacheWrite := firstPositiveInt(
		readIntFromMap(raw, "cache_creation_input_tokens", "cache_write"),
		readNestedIntFromMap(raw, "prompt_tokens_details", "cache_creation_input_tokens"),
		readNestedIntFromMap(raw, "input_tokens_details", "cache_creation_input_tokens"),
	)
	total := firstPositiveInt(
		readIntFromMap(raw, "total_tokens", "total"),
	)
	if total == 0 {
		total = input + output + cacheRead + cacheWrite
	}

	if input > 0 {
		dst.InputTokens = input
	}
	if output > 0 {
		dst.OutputTokens = output
	}
	if cacheRead > 0 {
		dst.CacheReadTokens = cacheRead
	}
	if cacheWrite > 0 {
		dst.CacheWriteTokens = cacheWrite
	}
	if total > 0 {
		dst.TotalTokens = total
	}
}

func readIntFromMap(raw map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		}
	}
	return 0
}

func readNestedIntFromMap(raw map[string]interface{}, parentKey, childKey string) int {
	parent, ok := raw[parentKey].(map[string]interface{})
	if !ok {
		return 0
	}
	return readIntFromMap(parent, childKey)
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// ---------- 错误解析 ----------

func parseOpenAIError(resp *http.Response) *APIError {
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

	apiErr.Retryable = apiErr.IsRateLimit() || resp.StatusCode >= 500
	return apiErr
}
