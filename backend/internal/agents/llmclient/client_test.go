package llmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- Anthropic SSE 解析测试 ----------

func TestAnthropicStreamChat_TextOnly(t *testing.T) {
	// 构造 Anthropic SSE 响应
	sseData := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":42}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}

event: message_stop
data: {"type":"message_stop"}

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Error("missing anthropic-version header")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	var events []StreamEvent
	result, err := StreamChat(context.Background(), ChatRequest{
		Provider:     "anthropic",
		Model:        "claude-3-sonnet",
		SystemPrompt: "be helpful",
		Messages:     []ChatMessage{TextMessage("user", "hi")},
		APIKey:       "test-key",
		BaseURL:      server.URL,
	}, func(evt StreamEvent) {
		events = append(events, evt)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop_reason=end_turn, got %q", result.StopReason)
	}
	if result.Usage.InputTokens != 42 {
		t.Errorf("expected input_tokens=42, got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 10 {
		t.Errorf("expected output_tokens=10, got %d", result.Usage.OutputTokens)
	}

	// 验证 assistant message
	msg := result.AssistantMessage
	if msg.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "Hello world!" {
		t.Errorf("unexpected content: %+v", msg.Content)
	}

	// 验证流事件
	textEvents := 0
	for _, evt := range events {
		if evt.Type == EventText {
			textEvents++
		}
	}
	if textEvents != 2 {
		t.Errorf("expected 2 text events, got %d", textEvents)
	}
}

func TestAnthropicStreamChat_ToolUse(t *testing.T) {
	sseData := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":100}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me run that."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_123","name":"bash"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"ls -la\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":50}}

event: message_stop
data: {"type":"message_stop"}

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	var toolStartCount int
	result, err := StreamChat(context.Background(), ChatRequest{
		Provider: "anthropic",
		Model:    "claude-3-sonnet",
		Messages: []ChatMessage{TextMessage("user", "list files")},
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}, func(evt StreamEvent) {
		if evt.Type == EventToolUseStart {
			toolStartCount++
		}
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != "tool_use" {
		t.Errorf("expected stop_reason=tool_use, got %q", result.StopReason)
	}
	if toolStartCount != 1 {
		t.Errorf("expected 1 tool_use_start event, got %d", toolStartCount)
	}

	// 验证 content blocks
	msg := result.AssistantMessage
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "text" || msg.Content[0].Text != "Let me run that." {
		t.Errorf("unexpected first block: %+v", msg.Content[0])
	}
	if msg.Content[1].Type != "tool_use" || msg.Content[1].Name != "bash" || msg.Content[1].ID != "toolu_123" {
		t.Errorf("unexpected tool_use block: %+v", msg.Content[1])
	}

	// 验证 tool input
	var input map[string]string
	if err := json.Unmarshal(msg.Content[1].Input, &input); err != nil {
		t.Fatalf("failed to unmarshal tool input: %v", err)
	}
	if input["command"] != "ls -la" {
		t.Errorf("expected command='ls -la', got %q", input["command"])
	}
}

func TestAnthropicStreamChat_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"too many requests"}}`)
	}))
	defer server.Close()

	_, err := StreamChat(context.Background(), ChatRequest{
		Provider: "anthropic",
		Model:    "claude-3",
		Messages: []ChatMessage{TextMessage("user", "hi")},
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}, nil)

	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsRateLimit() {
		t.Error("expected rate limit error")
	}
	if !apiErr.Retryable {
		t.Error("expected retryable")
	}
}

// ---------- OpenAI SSE 解析测试 ----------

func TestOpenAIStreamChat_TextOnly(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":" there"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}

data: [DONE]

`

	var requestBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing Bearer auth")
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	result, err := StreamChat(context.Background(), ChatRequest{
		Provider: "openai",
		Model:    "gpt-4",
		Messages: []ChatMessage{TextMessage("user", "hello")},
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected end_turn, got %q", result.StopReason)
	}
	if len(result.AssistantMessage.Content) != 1 {
		t.Fatal("expected 1 content block")
	}
	if result.AssistantMessage.Content[0].Text != "Hello there" {
		t.Errorf("expected 'Hello there', got %q", result.AssistantMessage.Content[0].Text)
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 || result.Usage.TotalTokens != 15 {
		t.Errorf("unexpected usage: %+v", result.Usage)
	}
	streamOptions, ok := requestBody["stream_options"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected stream_options in request, got %T", requestBody["stream_options"])
	}
	if includeUsage, _ := streamOptions["include_usage"].(bool); !includeUsage {
		t.Fatalf("expected stream_options.include_usage=true, got %+v", streamOptions)
	}
}

func TestOpenAICompatibleStreamChat_ParsesCacheUsage(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":45,"total_tokens":195,"prompt_tokens_details":{"cached_tokens":30}}}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	result, err := StreamChat(context.Background(), ChatRequest{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Messages: []ChatMessage{TextMessage("user", "hello")},
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Usage.InputTokens != 120 {
		t.Fatalf("expected input_tokens=120, got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 45 {
		t.Fatalf("expected output_tokens=45, got %d", result.Usage.OutputTokens)
	}
	if result.Usage.CacheReadTokens != 30 {
		t.Fatalf("expected cache_read_input_tokens=30, got %d", result.Usage.CacheReadTokens)
	}
	if result.Usage.TotalTokens != 195 {
		t.Fatalf("expected total_tokens=195, got %d", result.Usage.TotalTokens)
	}
}

func TestOpenAIStreamChat_ToolCalls(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\":"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	result, err := StreamChat(context.Background(), ChatRequest{
		Provider: "openai",
		Model:    "gpt-4",
		Messages: []ChatMessage{TextMessage("user", "list files")},
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != "tool_use" {
		t.Errorf("expected tool_use, got %q", result.StopReason)
	}
	if len(result.AssistantMessage.Content) != 1 {
		t.Fatalf("expected 1 content block (tool_use), got %d", len(result.AssistantMessage.Content))
	}
	block := result.AssistantMessage.Content[0]
	if block.Type != "tool_use" || block.Name != "bash" {
		t.Errorf("unexpected block: %+v", block)
	}
}

// ---------- Ollama NDJSON 解析测试 ----------

func TestOllamaStreamChat_TextOnly(t *testing.T) {
	ndjson := `{"message":{"role":"assistant","content":"Hello"},"done":false}
{"message":{"role":"assistant","content":" world"},"done":false}
{"message":{"role":"assistant","content":"!"},"done":true,"prompt_eval_count":15,"eval_count":3}
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ndjson)
	}))
	defer server.Close()

	var textParts []string
	result, err := StreamChat(context.Background(), ChatRequest{
		Provider: "ollama",
		Model:    "llama3",
		Messages: []ChatMessage{TextMessage("user", "hi")},
		BaseURL:  server.URL,
	}, func(evt StreamEvent) {
		if evt.Type == EventText {
			textParts = append(textParts, evt.Text)
		}
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected end_turn, got %q", result.StopReason)
	}
	if result.AssistantMessage.Content[0].Text != "Hello world!" {
		t.Errorf("unexpected text: %q", result.AssistantMessage.Content[0].Text)
	}
	if len(textParts) != 3 {
		t.Errorf("expected 3 text events, got %d", len(textParts))
	}
	if result.Usage.InputTokens != 15 || result.Usage.OutputTokens != 3 {
		t.Errorf("unexpected usage: %+v", result.Usage)
	}
}

// ---------- 消息转换测试 ----------

func TestToAnthropicMessages_SystemSkipped(t *testing.T) {
	msgs := []ChatMessage{
		TextMessage("system", "ignored"),
		TextMessage("user", "hello"),
	}
	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message (system skipped), got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user role, got %q", result[0].Role)
	}
}

func TestToAnthropicMessages_ToolResultConversion(t *testing.T) {
	msgs := []ChatMessage{
		TextMessage("user", "run ls"),
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "text", Text: "Sure"},
				{Type: "tool_use", ID: "toolu_1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
			},
		},
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "toolu_1", Name: "bash", ResultText: "file1.txt"},
			},
		},
	}
	result := toAnthropicMessages(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// Verify assistant message has 2 blocks (text + tool_use)
	var assistantContent []map[string]interface{}
	if err := json.Unmarshal(result[1].Content, &assistantContent); err != nil {
		t.Fatalf("failed to unmarshal assistant content: %v", err)
	}
	if len(assistantContent) != 2 {
		t.Errorf("expected 2 content blocks in assistant, got %d", len(assistantContent))
	}
	if assistantContent[1]["type"] != "tool_use" {
		t.Errorf("expected second block type=tool_use, got %v", assistantContent[1]["type"])
	}

	// Verify tool_result message
	var toolContent []map[string]interface{}
	if err := json.Unmarshal(result[2].Content, &toolContent); err != nil {
		t.Fatalf("failed to unmarshal tool_result content: %v", err)
	}
	if len(toolContent) != 1 {
		t.Fatalf("expected 1 tool_result block, got %d", len(toolContent))
	}
	if toolContent[0]["tool_use_id"] != "toolu_1" {
		t.Errorf("expected tool_use_id=toolu_1, got %v", toolContent[0]["tool_use_id"])
	}
	if toolContent[0]["content"] != "file1.txt" {
		t.Errorf("expected content=file1.txt, got %v", toolContent[0]["content"])
	}
}

func TestToAnthropicMessages_ToolResultEmptyContent(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "toolu_1", ResultText: ""},
			},
		},
	}
	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	var blocks []map[string]interface{}
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal content: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	// GW-LLM-D1: content field MUST be present even when ResultText is empty
	contentVal, exists := blocks[0]["content"]
	if !exists {
		t.Fatal("expected 'content' key to be present in tool_result block, but it was missing")
	}
	if contentVal != "" {
		t.Errorf("expected content to be empty string, got %v", contentVal)
	}
}

func TestToAnthropicMessages_ToolResultWithContent(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "toolu_2", ResultText: "hello world"},
			},
		},
	}
	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	var blocks []map[string]interface{}
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal content: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0]["content"] != "hello world" {
		t.Errorf("expected content='hello world', got %v", blocks[0]["content"])
	}
}

func TestToAnthropicMessages_UserImageConversion(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "text", Text: "describe"},
				{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "ZmFrZQ==",
					},
				},
			},
		},
	}

	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	var blocks []map[string]interface{}
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("failed to unmarshal content: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0]["type"] != "text" {
		t.Fatalf("unexpected first block: %+v", blocks[0])
	}
	source, ok := blocks[1]["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected image source map, got %+v", blocks[1])
	}
	if source["media_type"] != "image/png" || source["data"] != "ZmFrZQ==" {
		t.Fatalf("unexpected image source: %+v", source)
	}
}

func TestToOpenAIMessages_ToolResultConversion(t *testing.T) {
	msgs := []ChatMessage{
		TextMessage("user", "run ls"),
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "text", Text: "Sure"},
				{Type: "tool_use", ID: "call_1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
			},
		},
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "call_1", ResultText: "file1.txt\nfile2.txt"},
			},
		},
	}
	result := toOpenAIMessages("system prompt", msgs)

	// system + user + assistant (with tool_calls) + tool (result) = 4
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected system message first")
	}
	if result[2].Role != "assistant" || len(result[2].ToolCalls) != 1 {
		t.Errorf("expected assistant with 1 tool_call: %+v", result[2])
	}
	if result[3].Role != "tool" || result[3].ToolCallID != "call_1" {
		t.Errorf("expected tool message: %+v", result[3])
	}
}

func TestToOpenAIMessages_UserImageConversion(t *testing.T) {
	msgs := []ChatMessage{
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "text", Text: "describe"},
				{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "ZmFrZQ==",
					},
				},
			},
		},
	}

	result := toOpenAIMessages("system prompt", msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	parts, ok := result[1].Content.([]openAIContentPart)
	if !ok {
		t.Fatalf("expected multimodal content parts, got %T", result[1].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "describe" {
		t.Fatalf("unexpected text part: %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("unexpected image part: %+v", parts[1])
	}
	if parts[1].ImageURL.URL != "data:image/png;base64,ZmFrZQ==" {
		t.Fatalf("unexpected image url: %q", parts[1].ImageURL.URL)
	}
}

// ---------- 分发路由测试 ----------

func TestStreamChat_UnsupportedProvider(t *testing.T) {
	_, err := StreamChat(context.Background(), ChatRequest{
		Provider: "unknown-provider",
		Model:    "model",
	}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------- APIError 测试 ----------

func TestAPIError_Methods(t *testing.T) {
	err := &APIError{StatusCode: 429, Type: "rate_limit_error", Message: "slow down", Retryable: true}
	if !err.IsRateLimit() {
		t.Error("expected IsRateLimit=true")
	}
	if err.IsOverloaded() {
		t.Error("expected IsOverloaded=false")
	}
	if err.Error() != "rate_limit_error: slow down" {
		t.Errorf("unexpected Error(): %q", err.Error())
	}

	err2 := &APIError{StatusCode: 529, Type: "overloaded_error"}
	if !err2.IsOverloaded() {
		t.Error("expected IsOverloaded=true")
	}
	if err2.Error() != "overloaded_error" {
		t.Errorf("unexpected Error(): %q", err2.Error())
	}
}
