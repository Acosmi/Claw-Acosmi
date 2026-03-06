package gateway

// openresponses_http.go — OpenResponses /v1/responses 完整实现
// 对应 TS: src/gateway/openresponses-http.ts (915L)
// 文档: https://www.open-responses.com/
//
// 替代旧的 HandleOpenAIResponses (openai_http.go L129-220) 简单代理,
// 实现完整的 OpenResponses SSE 事件协议.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/google/uuid"
)

// HandleOpenAIResponses 处理 POST /v1/responses — 完整 OpenResponses 实现。
// 替代旧的简单代理转发，支持:
//   - 结构化请求体验证
//   - tool_choice 处理
//   - 非流式: ResponseResource JSON 响应 (含 function_call)
//   - 流式: OpenResponses SSE 事件协议
func HandleOpenAIResponses(w http.ResponseWriter, r *http.Request, cfg OpenAIChatHandlerConfig) {
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
		maxBytes = 20 * 1024 * 1024 // 20MB (Responses 允许更大 body)
	}
	rawBody, err := ReadJSONBody(r, maxBytes)
	if err != nil {
		SendInvalidRequest(w, err.Error())
		return
	}

	// 解析为结构体
	rawBytes, _ := json.Marshal(rawBody)
	var body CreateResponseBody
	if err := json.Unmarshal(rawBytes, &body); err != nil {
		sendORError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error")
		return
	}

	if body.Model == "" {
		sendORError(w, http.StatusBadRequest, "model is required", "invalid_request_error")
		return
	}

	// 解析 stream
	stream := false
	if body.Stream != nil {
		stream = *body.Stream
	}
	model := body.Model
	user := body.User

	// 解析 input → agent prompt
	prompt, err := buildResponsesAgentPrompt(body.Input)
	if err != nil {
		sendORError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	// 处理 tool_choice
	toolChoicePrompt := applyORToolChoice(body.Tools, body.ToolChoice)

	agentID := ResolveAgentIDForRequest(r, model)
	sessionKey := resolveOpenResponsesSessionKey(agentID, user)

	// 组合 instructions + extraSystemPrompt + toolChoicePrompt
	extraParts := []string{}
	if body.Instructions != "" {
		extraParts = append(extraParts, body.Instructions)
	}
	if prompt.extraSystemPrompt != "" {
		extraParts = append(extraParts, prompt.extraSystemPrompt)
	}
	if toolChoicePrompt != "" {
		extraParts = append(extraParts, toolChoicePrompt)
	}
	fullExtraSystem := strings.Join(extraParts, "\n\n")

	if prompt.message == "" {
		sendORError(w, http.StatusBadRequest, "Missing user message in `input`.", "invalid_request_error")
		return
	}

	responseID := "resp_" + uuid.New().String()
	outputItemID := "msg_" + uuid.New().String()

	if !stream {
		handleORNonStreaming(w, r, cfg, responseID, outputItemID, model, agentID, sessionKey,
			agentPrompt{message: prompt.message, extraSystemPrompt: fullExtraSystem})
	} else {
		handleORStreaming(w, r, cfg, responseID, outputItemID, model, agentID, sessionKey,
			agentPrompt{message: prompt.message, extraSystemPrompt: fullExtraSystem})
	}
}

// ---------- 非流式处理 ----------

func handleORNonStreaming(
	w http.ResponseWriter, r *http.Request,
	cfg OpenAIChatHandlerConfig,
	responseID, outputItemID, model, agentID, sessionKey string,
	prompt agentPrompt,
) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// 在 dispatch 前注册 usage 监听，确保捕获管线运行期间的事件
	var collectedUsage ORUsage
	var usageMu sync.Mutex
	unsubUsage := infra.OnAgentEvent(func(evt infra.AgentEventPayload) {
		if evt.RunID != responseID {
			return
		}
		if evt.Stream == infra.StreamLifecycle {
			if u := extractUsageFromAgentEvent(evt); u != nil {
				usageMu.Lock()
				collectedUsage = *u
				usageMu.Unlock()
			}
		}
	})
	defer unsubUsage()

	msgCtx := buildORMsgContext(prompt, sessionKey, responseID)
	result := DispatchInboundMessage(ctx, DispatchInboundParams{
		MsgCtx:     msgCtx,
		SessionKey: sessionKey,
		AgentID:    agentID,
		RunID:      responseID,
		Ctx:        ctx,
		Dispatcher: cfg.Dispatcher,
	})

	if result.Error != nil {
		resp := createResponseResource(responseID, model, "failed", []OutputItem{}, emptyUsage(),
			&ORError{Code: "api_error", Message: result.Error.Error()})
		SendJSON(w, http.StatusInternalServerError, resp)
		return
	}

	content := CombineReplyPayloads(result.Replies)
	if content == "" {
		content = "No response from Claw Acosmi."
	}

	usageMu.Lock()
	usage := collectedUsage
	usageMu.Unlock()

	output := []OutputItem{
		createAssistantOutputItem(outputItemID, content, "completed"),
	}
	resp := createResponseResource(responseID, model, "completed", output, usage, nil)
	SendJSON(w, http.StatusOK, resp)
}

// ---------- 流式处理 ----------

func handleORStreaming(
	w http.ResponseWriter, r *http.Request,
	cfg OpenAIChatHandlerConfig,
	responseID, outputItemID, model, agentID, sessionKey string,
	prompt agentPrompt,
) {
	SetSSEHeaders(w)

	var (
		accText        strings.Builder
		sawDelta       int32 // atomic
		closed         int32 // atomic
		collectedUsage ORUsage
		// writeMu 保护 ResponseWriter + accText + collectedUsage 的并发访问。
		// http.ResponseWriter 非 goroutine 安全，strings.Builder 非线程安全。
		writeMu sync.Mutex
	)

	// 发射初始事件
	initialResp := createResponseResource(responseID, model, "in_progress", []OutputItem{}, emptyUsage(), nil)
	writeORSSE(w, "response.created", map[string]interface{}{"response": initialResp})
	writeORSSE(w, "response.in_progress", map[string]interface{}{"response": initialResp})

	// 添加 output item
	outputItem := createAssistantOutputItem(outputItemID, "", "in_progress")
	writeORSSE(w, "response.output_item.added", map[string]interface{}{
		"output_index": 0,
		"item":         outputItem,
	})

	// 添加 content part
	writeORSSE(w, "response.content_part.added", map[string]interface{}{
		"item_id":       outputItemID,
		"output_index":  0,
		"content_index": 0,
		"part":          OutputTextPart{Type: "output_text", Text: ""},
	})

	// 监听 agent 事件
	unsubscribe := infra.OnAgentEvent(func(evt infra.AgentEventPayload) {
		if evt.RunID != responseID {
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

			atomic.StoreInt32(&sawDelta, 1)
			writeMu.Lock()
			accText.WriteString(delta)

			writeORSSE(w, "response.output_text.delta", map[string]interface{}{
				"item_id":       outputItemID,
				"output_index":  0,
				"content_index": 0,
				"delta":         delta,
			})
			writeMu.Unlock()
			return
		}

		if evt.Stream == infra.StreamLifecycle {
			// 收集 usage 数据
			if u := extractUsageFromAgentEvent(evt); u != nil {
				writeMu.Lock()
				collectedUsage = *u
				writeMu.Unlock()
			}

			phase, _ := evt.Data["phase"].(string)
			if phase == "end" || phase == "error" {
				finalStatus := "completed"
				if phase == "error" {
					finalStatus = "failed"
				}
				writeMu.Lock()
				finalText := accText.String()
				if finalText == "" {
					finalText = "No response from Claw Acosmi."
				}
				usage := collectedUsage
				writeMu.Unlock()
				finalizeORStream(w, responseID, outputItemID, model, finalStatus, finalText, usage, &closed)
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
				// 管线结束但未通过事件流 finalize
				writeMu.Lock()
				finalText := accText.String()
				if finalText == "" {
					finalText = "No response from Claw Acosmi."
				}
				usage := collectedUsage
				writeMu.Unlock()
				finalizeORStream(w, responseID, outputItemID, model, "completed", finalText, usage, &closed)
			}
		}()

		msgCtx := buildORMsgContext(prompt, sessionKey, responseID)
		result := DispatchInboundMessage(ctx, DispatchInboundParams{
			MsgCtx:     msgCtx,
			SessionKey: sessionKey,
			AgentID:    agentID,
			RunID:      responseID,
			Ctx:        ctx,
			Dispatcher: cfg.Dispatcher,
		})

		if atomic.LoadInt32(&closed) != 0 {
			return
		}

		// Fallback: 如果没有收到流式 delta，使用管线结果
		if atomic.LoadInt32(&sawDelta) == 0 {
			content := ""
			if result.Error != nil {
				content = "Error: " + result.Error.Error()
			} else {
				content = CombineReplyPayloads(result.Replies)
				if content == "" {
					content = "No response from Claw Acosmi."
				}
			}
			writeMu.Lock()
			accText.WriteString(content)
			writeORSSE(w, "response.output_text.delta", map[string]interface{}{
				"item_id":       outputItemID,
				"output_index":  0,
				"content_index": 0,
				"delta":         content,
			})
			writeMu.Unlock()
		}
	}()
}

// ---------- SSE 辅助 ----------

// writeORSSE 写入 OpenResponses SSE 事件。
// 格式: event: {type}\ndata: {json}\n\n
func writeORSSE(w http.ResponseWriter, eventType string, payload map[string]interface{}) {
	payload["type"] = eventType
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", string(data))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// finalizeORStream 发射流式终止事件序列。
func finalizeORStream(
	w http.ResponseWriter,
	responseID, outputItemID, model, status, text string,
	usage ORUsage,
	closed *int32,
) {
	// output_text.done
	writeORSSE(w, "response.output_text.done", map[string]interface{}{
		"item_id":       outputItemID,
		"output_index":  0,
		"content_index": 0,
		"text":          text,
	})

	// content_part.done
	writeORSSE(w, "response.content_part.done", map[string]interface{}{
		"item_id":       outputItemID,
		"output_index":  0,
		"content_index": 0,
		"part":          OutputTextPart{Type: "output_text", Text: text},
	})

	// output_item.done
	completedItem := createAssistantOutputItem(outputItemID, text, "completed")
	writeORSSE(w, "response.output_item.done", map[string]interface{}{
		"output_index": 0,
		"item":         completedItem,
	})

	// response.completed
	finalResp := createResponseResource(responseID, model, status, []OutputItem{completedItem}, usage, nil)
	writeORSSE(w, "response.completed", map[string]interface{}{
		"response": finalResp,
	})

	WriteSSEDone(w)
}

// ---------- Prompt 构建 ----------

// buildResponsesAgentPrompt 从 OpenResponses input 构建 agent prompt。
// 对应 TS openresponses-http.ts buildAgentPrompt()
func buildResponsesAgentPrompt(inputRaw json.RawMessage) (agentPrompt, error) {
	if len(inputRaw) == 0 {
		return agentPrompt{}, fmt.Errorf("missing `input` field")
	}

	// 尝试作为字符串解析
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		if strings.TrimSpace(inputStr) == "" {
			return agentPrompt{}, fmt.Errorf("empty input string")
		}
		return agentPrompt{message: inputStr}, nil
	}

	// 作为 []ItemParam 解析
	var items []ItemParam
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return agentPrompt{}, fmt.Errorf("input must be a string or array of items")
	}

	var systemParts []string
	type convEntry struct {
		role   string
		sender string
		body   string
	}
	var entries []convEntry

	for _, item := range items {
		switch item.Type {
		case "message":
			content := extractORTextContent(item.Content)
			if content == "" {
				continue
			}

			if item.Role == "system" || item.Role == "developer" {
				systemParts = append(systemParts, content)
				continue
			}

			normalizedRole := item.Role
			if normalizedRole == "assistant" {
				normalizedRole = "assistant"
			} else {
				normalizedRole = "user"
			}
			sender := "User"
			if normalizedRole == "assistant" {
				sender = "Assistant"
			}

			entries = append(entries, convEntry{role: normalizedRole, sender: sender, body: content})

		case "function_call_output":
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			entries = append(entries, convEntry{
				role:   "tool",
				sender: "Tool:" + callID,
				body:   item.Output,
			})
		}
		// Skip reasoning and item_reference
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
	}, nil
}

// extractORTextContent 提取文本内容（string 或 ContentPart 数组）。
// 同时处理 input_image 和 input_file 类型，将其转为文本描述。
func extractORTextContent(contentRaw json.RawMessage) string {
	if len(contentRaw) == 0 {
		return ""
	}

	// 尝试作为字符串
	var s string
	if err := json.Unmarshal(contentRaw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	// 尝试作为 []ContentPart
	var parts []ContentPart
	if err := json.Unmarshal(contentRaw, &parts); err != nil {
		return ""
	}

	var texts []string
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text":
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		case "input_image":
			desc := extractORImageDescription(part)
			if desc != "" {
				texts = append(texts, desc)
			}
		case "input_file":
			desc := extractORFileDescription(part)
			if desc != "" {
				texts = append(texts, desc)
			}
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

// extractORImageDescription 从 input_image ContentPart 提取图像描述。
// 对应 TS: openresponses-http.ts L392-413 extractImageContentFromSource
func extractORImageDescription(part ContentPart) string {
	if part.Source != nil {
		switch part.Source.Type {
		case "url":
			if part.Source.URL != "" {
				return fmt.Sprintf("[Image: %s]", part.Source.URL)
			}
		case "base64":
			media := part.Source.MediaType
			if media == "" {
				media = "image/png"
			}
			dataLen := len(part.Source.Data)
			if dataLen > 0 {
				// 估算原始字节大小（base64 约 4/3 膨胀）
				origSize := dataLen * 3 / 4
				return fmt.Sprintf("[Image: base64 %s, ~%d bytes]", media, origSize)
			}
		}
	}
	// image_url shorthand
	if part.ImageURL != "" {
		return fmt.Sprintf("[Image: %s]", part.ImageURL)
	}
	return "[Image: embedded]"
}

// extractORFileDescription 从 input_file ContentPart 提取文件内容描述。
// 对应 TS: openresponses-http.ts L415-448 extractFileContentFromSource
func extractORFileDescription(part ContentPart) string {
	filename := part.Filename
	if filename == "" && part.Source != nil {
		filename = part.Source.Filename
	}
	if filename == "" {
		filename = "unnamed"
	}

	if part.Source != nil && part.Source.Type == "base64" && part.Source.Data != "" {
		// 尝试 base64 解码
		data, err := base64.StdEncoding.DecodeString(part.Source.Data)
		if err != nil {
			// 尝试 RawStdEncoding（无 padding）
			data, err = base64.RawStdEncoding.DecodeString(part.Source.Data)
		}
		if err == nil && len(data) > 0 {
			media := part.Source.MediaType
			if media == "" {
				media = "application/octet-stream"
			}
			// 文本文件直接提取内容
			if isTextMime(media) {
				content := string(data)
				if len(content) > 50000 {
					content = content[:50000] + "\n... [truncated]"
				}
				return fmt.Sprintf("<file name=%q>\n%s\n</file>", filename, content)
			}
			// PDF / 二进制文件
			return fmt.Sprintf("[File: %s, %s, %d bytes]", filename, media, len(data))
		}
	}

	if part.Source != nil && part.Source.Type == "url" && part.Source.URL != "" {
		// 远程 URL 抓取（对齐 TS extractFileContentFromSource）
		content, fetchMime, err := fetchRemoteFileContent(part.Source.URL)
		if err == nil && content != "" {
			if isTextMime(fetchMime) {
				if len(content) > 50000 {
					content = content[:50000] + "\n... [truncated]"
				}
				return fmt.Sprintf("<file name=%q>\n%s\n</file>", filename, content)
			}
			return fmt.Sprintf("[File: %s, %s, %d bytes]", filename, fetchMime, len(content))
		}
		// fetch 失败时回退为 URL 描述
		return fmt.Sprintf("[File: %s, URL: %s]", filename, part.Source.URL)
	}

	return fmt.Sprintf("[File: %s]", filename)
}

// isTextMime 判断 MIME 类型是否为纯文本。
func isTextMime(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	textTypes := []string{
		"application/json",
		"application/xml",
		"application/javascript",
		"application/typescript",
		"application/x-yaml",
		"application/yaml",
		"application/toml",
		"application/x-sh",
	}
	for _, t := range textTypes {
		if mime == t {
			return true
		}
	}
	return false
}

// extractUsageFromAgentEvent 从 agent lifecycle 事件中提取 usage 统计。
// 对应 TS: openresponses-http.ts L260-294 toUsage + extractUsageFromResult
func extractUsageFromAgentEvent(evt infra.AgentEventPayload) *ORUsage {
	// usage 数据可能在 data.usage 或 data.agentMeta.usage
	usageRaw := evt.Data["usage"]
	if usageRaw == nil {
		if meta, ok := evt.Data["agentMeta"].(map[string]interface{}); ok {
			usageRaw = meta["usage"]
		}
	}
	if usageRaw == nil {
		return nil
	}

	usageMap, ok := usageRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	toInt := func(key string) int {
		v, _ := usageMap[key]
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
		return 0
	}

	input := toInt("input")
	output := toInt("output")
	cacheRead := toInt("cacheRead")
	cacheWrite := toInt("cacheWrite")
	total := toInt("total")
	// 对齐 TS toUsage(): total = value.total ?? input + output + cacheRead + cacheWrite
	if total == 0 {
		total = input + output + cacheRead + cacheWrite
	}

	return &ORUsage{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  total,
	}
}

// fetchRemoteFileContent 从远程 URL 抓取文件内容。
// 对应 TS: input-files.ts extractFileContentFromSource URL 分支
// 限制: 20MB 最大，30s 超时
func fetchRemoteFileContent(url string) (content string, mimeType string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// 限制读取 20MB
	const maxBytes = 20 * 1024 * 1024
	limitReader := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}
	if len(data) > maxBytes {
		return "", "", fmt.Errorf("file exceeds 20MB limit")
	}

	mimeType = resp.Header.Get("Content-Type")
	if idx := strings.Index(mimeType, ";"); idx > 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return string(data), mimeType, nil
}

// ---------- Tool Choice ----------

// applyORToolChoice 处理 tool_choice 参数。返回额外的系统提示。
func applyORToolChoice(tools []ToolDefinition, toolChoiceRaw json.RawMessage) string {
	if len(toolChoiceRaw) == 0 {
		return ""
	}

	// 尝试作为字符串
	var choiceStr string
	if json.Unmarshal(toolChoiceRaw, &choiceStr) == nil {
		switch choiceStr {
		case "none":
			return "" // 工具将被忽略
		case "required":
			if len(tools) > 0 {
				return "You must call one of the available tools before responding."
			}
		case "auto":
			return ""
		}
		return ""
	}

	// 尝试作为 {type: "function", function: {name: "xxx"}}
	var choiceObj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(toolChoiceRaw, &choiceObj) == nil && choiceObj.Type == "function" {
		name := strings.TrimSpace(choiceObj.Function.Name)
		if name != "" {
			return fmt.Sprintf("You must call the %s tool before responding.", name)
		}
	}

	return ""
}

// ---------- Session Key ----------

// resolveOpenResponsesSessionKey 构建 OpenResponses session key。
func resolveOpenResponsesSessionKey(agentID, user string) string {
	if agentID == "" {
		agentID = "main"
	}
	if user != "" {
		return fmt.Sprintf("openresponses:%s:%s", agentID, user)
	}
	return fmt.Sprintf("openresponses:%s", agentID)
}

// ---------- MsgContext 构建 ----------

func buildORMsgContext(prompt agentPrompt, sessionKey, runID string) *autoreply.MsgContext {
	msgCtx := &autoreply.MsgContext{
		Body:               prompt.message,
		BodyForAgent:       prompt.message,
		BodyForCommands:    prompt.message,
		RawBody:            prompt.message,
		CommandBody:        prompt.message,
		SessionKey:         sessionKey,
		Provider:           "openai-compat",
		Surface:            "api",
		OriginatingChannel: "openresponses",
		ChatType:           "direct",
		CommandAuthorized:  true,
		MessageSid:         runID,
	}
	if prompt.extraSystemPrompt != "" {
		msgCtx.GroupSystemPrompt = prompt.extraSystemPrompt
	}
	return msgCtx
}

// ---------- 错误辅助 ----------

func sendORError(w http.ResponseWriter, status int, message, errType string) {
	SendJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"type":    errType,
		},
	})
}

// nowUnix 返回当前 Unix 时间戳。
func nowUnix() int64 {
	return time.Now().Unix()
}
