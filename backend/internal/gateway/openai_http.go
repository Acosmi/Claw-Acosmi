package gateway

// openai_http.go — OpenAI Chat Completions API 兼容处理器
// 对应 TS src/gateway/openai-http.ts (427L)
//
// 支持两种模式:
//   - 非流式 (stream=false): 调用管线 → 200 JSON chat.completion
//   - 流式 (stream=true): SSE chat.completion.chunk + [DONE]
//
// 也处理 /v1/responses 代理转发。

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/google/uuid"
)

// OpenAIChatHandlerConfig OpenAI 兼容 API 处理器配置。
type OpenAIChatHandlerConfig struct {
	// GetAuth 获取认证配置。
	GetAuth func() ResolvedGatewayAuth
	// Dispatcher 管线分发器（DI 注入）。
	Dispatcher PipelineDispatcher
	// Broadcaster WebSocket 广播器（可选，用于进度推送）。
	Broadcaster *Broadcaster
	// Logger
	Logger *slog.Logger
	// MaxBodyBytes body 上限（默认 1MB）。
	MaxBodyBytes int64
	// TrustedProxies
	TrustedProxies []string
}

// ---------- 请求/响应类型 ----------

// openAIChatMessage OpenAI 消息格式。
type openAIChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []part
	Name    string      `json:"name,omitempty"`
}

// openAIChatRequest OpenAI 请求体。
type openAIChatRequest struct {
	Model    string      `json:"model"`
	Stream   interface{} `json:"stream"` // bool 或缺失
	Messages interface{} `json:"messages"`
	User     string      `json:"user,omitempty"`
}

// ---------- 处理器 ----------

// HandleOpenAIChatCompletions 处理 POST /v1/chat/completions。
func HandleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request, cfg OpenAIChatHandlerConfig) {
	if r.Method != http.MethodPost {
		SendMethodNotAllowed(w, "POST")
		return
	}

	// 认证
	auth := cfg.GetAuth()
	token := GetBearerToken(r)
	if token == "" {
		token = GetHeader(r, "X-OpenAcosmi-Token")
	}
	if !authorizeOpenAI(auth, token) {
		SendUnauthorized(w)
		return
	}

	// 读取 body
	maxBytes := cfg.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	body, err := ReadJSONBody(r, maxBytes)
	if err != nil {
		SendInvalidRequest(w, err.Error())
		return
	}

	// 解析请求
	bodyBytes, _ := json.Marshal(body)
	var req openAIChatRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		SendInvalidRequest(w, "invalid JSON body")
		return
	}

	stream := coerceBool(req.Stream)
	model := req.Model
	if model == "" {
		model = "openacosmi"
	}
	user := req.User

	// 解析 agent ID 和 session key
	agentID := ResolveAgentIDForRequest(r, model)
	sessionKey := resolveOpenAISessionKey(agentID, user)

	// 提取 prompt
	prompt := buildAgentPrompt(req.Messages)
	if prompt.message == "" {
		SendJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "Missing user message in `messages`.",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	runID := "chatcmpl_" + uuid.New().String()

	if !stream {
		handleNonStreaming(w, r, cfg, runID, model, agentID, sessionKey, prompt)
	} else {
		handleStreaming(w, r, cfg, runID, model, agentID, sessionKey, prompt)
	}
}

// HandleOpenAIResponses 已移至 openresponses_http.go，实现完整 OpenResponses 协议。

// ---------- 非流式处理 ----------

func handleNonStreaming(
	w http.ResponseWriter, r *http.Request,
	cfg OpenAIChatHandlerConfig,
	runID, model, agentID, sessionKey string,
	prompt agentPrompt,
) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	msgCtx := &autoreply.MsgContext{
		Body:               prompt.message,
		BodyForAgent:       prompt.message,
		BodyForCommands:    prompt.message,
		RawBody:            prompt.message,
		CommandBody:        prompt.message,
		SessionKey:         sessionKey,
		Provider:           "openai-compat",
		Surface:            "api",
		OriginatingChannel: "openai",
		ChatType:           "direct",
		CommandAuthorized:  true,
		MessageSid:         runID,
	}
	if prompt.extraSystemPrompt != "" {
		msgCtx.GroupSystemPrompt = prompt.extraSystemPrompt
	}

	result := DispatchInboundMessage(ctx, DispatchInboundParams{
		MsgCtx:     msgCtx,
		SessionKey: sessionKey,
		AgentID:    agentID,
		RunID:      runID,
		Ctx:        ctx,
		Dispatcher: cfg.Dispatcher,
		OnProgress: buildChatProgressCallback(cfg.Broadcaster, sessionKey),
	})

	if result.Error != nil {
		slog.Error("openai: pipeline error", "error", result.Error, "runId", runID)
		SendJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]string{
				"message": result.Error.Error(),
				"type":    "api_error",
			},
		})
		return
	}

	content := CombineReplyPayloads(result.Replies)
	if content == "" {
		content = "No response from Claw Acosmi."
	}

	SendJSON(w, http.StatusOK, map[string]interface{}{
		"id":      runID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	})
}

// ---------- 流式处理 ----------

func handleStreaming(
	w http.ResponseWriter, r *http.Request,
	cfg OpenAIChatHandlerConfig,
	runID, model, agentID, sessionKey string,
	prompt agentPrompt,
) {
	SetSSEHeaders(w)

	var wroteRole int32    // atomic
	var sawDelta int32     // atomic
	var closed int32       // atomic
	var writeMu sync.Mutex // 保护 ResponseWriter 并发写 (http.ResponseWriter 非 goroutine 安全)

	// 监听 agent 事件 → 转发为 SSE
	unsubscribe := infra.OnAgentEvent(func(evt infra.AgentEventPayload) {
		if evt.RunID != runID {
			return
		}
		if atomic.LoadInt32(&closed) != 0 {
			return
		}

		if evt.Stream == infra.StreamAssistant {
			delta, _ := evt.Data["delta"].(string)
			if delta == "" {
				delta, _ = evt.Data["text"].(string)
			}
			if delta == "" {
				return
			}

			writeMu.Lock()
			// 首次写入 role chunk
			if atomic.CompareAndSwapInt32(&wroteRole, 0, 1) {
				writeSSEChunk(w, runID, model, map[string]string{"role": "assistant"}, "")
			}

			atomic.StoreInt32(&sawDelta, 1)
			writeSSEChunk(w, runID, model, nil, delta)
			writeMu.Unlock()
			return
		}

		if evt.Stream == infra.StreamLifecycle {
			phase, _ := evt.Data["phase"].(string)
			if phase == "end" || phase == "error" {
				atomic.StoreInt32(&closed, 1)
				writeMu.Lock()
				// finish chunk
				writeSSEFinishChunk(w, runID, model)
				WriteSSEDone(w)
				writeMu.Unlock()
			}
		}
	})

	// 先创建可取消 context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// 客户端断开时清理
	notify := r.Context().Done()
	go func() {
		<-notify
		cancel() // 取消推理
		if atomic.CompareAndSwapInt32(&closed, 0, 1) {
			unsubscribe()
		}
	}()

	// 异步运行管线
	go func() {
		defer cancel()
		defer func() {
			if atomic.CompareAndSwapInt32(&closed, 0, 1) {
				unsubscribe()
				writeMu.Lock()
				writeSSEFinishChunk(w, runID, model)
				WriteSSEDone(w)
				writeMu.Unlock()
			}
		}()

		msgCtx := &autoreply.MsgContext{
			Body:               prompt.message,
			BodyForAgent:       prompt.message,
			BodyForCommands:    prompt.message,
			RawBody:            prompt.message,
			CommandBody:        prompt.message,
			SessionKey:         sessionKey,
			Provider:           "openai-compat",
			Surface:            "api",
			OriginatingChannel: "openai",
			ChatType:           "direct",
			CommandAuthorized:  true,
			MessageSid:         runID,
		}
		if prompt.extraSystemPrompt != "" {
			msgCtx.GroupSystemPrompt = prompt.extraSystemPrompt
		}

		result := DispatchInboundMessage(ctx, DispatchInboundParams{
			MsgCtx:     msgCtx,
			SessionKey: sessionKey,
			AgentID:    agentID,
			RunID:      runID,
			Ctx:        ctx,
			Dispatcher: cfg.Dispatcher,
			OnProgress: buildChatProgressCallback(cfg.Broadcaster, sessionKey),
		})

		if atomic.LoadInt32(&closed) != 0 {
			return
		}

		// 如果管线没有通过事件流返回 delta，使用结果做 fallback
		if atomic.LoadInt32(&sawDelta) == 0 {
			writeMu.Lock()
			if atomic.CompareAndSwapInt32(&wroteRole, 0, 1) {
				writeSSEChunk(w, runID, model, map[string]string{"role": "assistant"}, "")
			}

			content := ""
			if result.Error != nil {
				content = "Error: " + result.Error.Error()
			} else {
				content = CombineReplyPayloads(result.Replies)
				if content == "" {
					content = "No response from Claw Acosmi."
				}
			}
			writeSSEChunk(w, runID, model, nil, content)
			writeMu.Unlock()
		}
	}()
}

// ---------- SSE 辅助 ----------

func writeSSEChunk(w http.ResponseWriter, id, model string, delta map[string]string, content string) {
	choice := map[string]interface{}{
		"index": 0,
	}
	if delta != nil {
		choice["delta"] = delta
	} else {
		choice["delta"] = map[string]interface{}{"content": content}
		choice["finish_reason"] = nil
	}

	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{choice},
	}

	data, _ := json.Marshal(chunk)
	WriteSSEData(w, string(data))
}

func writeSSEFinishChunk(w http.ResponseWriter, id, model string) {
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": "stop",
			},
		},
	}
	data, _ := json.Marshal(chunk)
	WriteSSEData(w, string(data))
}

// ---------- Prompt 构建 ----------

type agentPrompt struct {
	message           string
	extraSystemPrompt string
}

// buildAgentPrompt 从 OpenAI messages 构建 agent prompt。
// 对应 TS openai-http.ts buildAgentPrompt()
func buildAgentPrompt(messagesRaw interface{}) agentPrompt {
	arr, ok := messagesRaw.([]interface{})
	if !ok {
		return agentPrompt{}
	}

	var systemParts []string
	type convEntry struct {
		role   string // user / assistant / tool
		sender string
		body   string
	}
	var entries []convEntry

	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role := strings.TrimSpace(fmt.Sprintf("%v", m["role"]))
		content := extractTextContent(m["content"])
		if role == "" || content == "" {
			continue
		}

		if role == "system" || role == "developer" {
			systemParts = append(systemParts, content)
			continue
		}

		normalized := role
		if normalized == "function" {
			normalized = "tool"
		}
		if normalized != "user" && normalized != "assistant" && normalized != "tool" {
			continue
		}

		name, _ := m["name"].(string)
		var sender string
		switch normalized {
		case "assistant":
			sender = "Assistant"
		case "user":
			sender = "User"
		case "tool":
			if name != "" {
				sender = "Tool:" + name
			} else {
				sender = "Tool"
			}
		}

		entries = append(entries, convEntry{role: normalized, sender: sender, body: content})
	}

	message := ""
	if len(entries) > 0 {
		// 找最后一个 user/tool 消息
		currentIdx := len(entries) - 1
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].role == "user" || entries[i].role == "tool" {
				currentIdx = i
				break
			}
		}

		if currentIdx == 0 && len(entries) == 1 {
			message = entries[0].body
		} else {
			// 构建历史上下文
			var parts []string
			for i := 0; i <= currentIdx; i++ {
				parts = append(parts, entries[i].sender+": "+entries[i].body)
			}
			message = strings.Join(parts, "\n\n")
		}
	}

	return agentPrompt{
		message:           message,
		extraSystemPrompt: strings.Join(systemParts, "\n\n"),
	}
}

// extractTextContent 提取文本内容（string 或 part 数组）。
// 对应 TS openai-http.ts extractTextContent()
func extractTextContent(content interface{}) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}
	arr, ok := content.([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, part := range arr {
		pm, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := pm["type"].(string)
		text, _ := pm["text"].(string)
		inputText, _ := pm["input_text"].(string)

		switch {
		case typ == "text" && text != "":
			parts = append(parts, text)
		case typ == "input_text" && text != "":
			parts = append(parts, text)
		case inputText != "":
			parts = append(parts, inputText)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// ---------- Session Key ----------

// resolveOpenAISessionKey 构建 OpenAI 兼容 session key。
func resolveOpenAISessionKey(agentID, user string) string {
	if agentID == "" {
		agentID = "main"
	}
	if user != "" {
		return fmt.Sprintf("openai:%s:%s", agentID, user)
	}
	return fmt.Sprintf("openai:%s", agentID)
}

// ---------- 认证辅助 ----------

func authorizeOpenAI(auth ResolvedGatewayAuth, token string) bool {
	if auth.Token == "" {
		return true // 无认证配置
	}
	return SafeEqual(token, auth.Token)
}

// coerceBool 将 interface{} 转换为 bool。
func coerceBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val == "true" || val == "1"
	default:
		return false
	}
}
