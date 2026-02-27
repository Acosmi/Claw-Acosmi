package runner

// ============================================================================
// EmbeddedAttemptRunner — AttemptRunner 接口的真实实现
// 对齐 TS: pi-embedded-runner/run/attempt.ts → runEmbeddedAttempt()
//
// 职责: 构建 prompt → 调用 LLM (流式) → 执行 tool loop → 返回 AttemptResult
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/anthropic/open-acosmi/internal/agents/llmclient"
	"github.com/anthropic/open-acosmi/internal/agents/models"
	"github.com/anthropic/open-acosmi/internal/agents/prompt"
	"github.com/anthropic/open-acosmi/internal/agents/skills"
	"github.com/anthropic/open-acosmi/internal/infra"
	"github.com/anthropic/open-acosmi/pkg/types"
)

// ---------- Argus 视觉子智能体接口（agent 侧） ----------

// ArgusToolDef Argus 工具定义（agent 侧使用的本地类型）。
type ArgusToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ArgusBridgeForAgent 视觉子智能体接口。
// 使用 runner 本地类型，避免引入 mcpclient 包依赖。
type ArgusBridgeForAgent interface {
	AgentTools() []ArgusToolDef
	AgentCallTool(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) (string, error)
}

// CoderBridgeForAgent 编程子智能体接口。
// 复用 ArgusToolDef 类型（结构相同：Name + Description + InputSchema）。
type CoderBridgeForAgent interface {
	AgentTools() []ArgusToolDef
	AgentCallTool(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) (string, error)
}

// ---------- MCP 远程工具接口（agent 侧） ----------

// RemoteToolDef 远程 MCP 工具定义（agent 侧使用的本地类型）。
type RemoteToolDef struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// RemoteMCPBridgeForAgent 远程 MCP 工具接口。
// 使用 runner 本地类型，避免引入 mcpremote 包依赖。
type RemoteMCPBridgeForAgent interface {
	AgentRemoteTools() []RemoteToolDef
	AgentCallRemoteTool(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) (string, error)
}

const (
	// maxToolLoopIterations 防止无限工具调用循环。
	maxToolLoopIterations = 30
	// maxConsecutivePermDeniedRounds 连续全部权限拒绝的最大轮次。
	// 超过此数则提前退出工具循环，避免浪费 LLM 调用并防止用户等待过久。
	maxConsecutivePermDeniedRounds = 3
)

// UHMSBridgeForAgent UHMS 记忆系统接口（agent 侧）。
// 使用 runner 本地类型避免 uhms 包直接依赖。
type UHMSBridgeForAgent interface {
	// CompressChatMessages 在 token 超阈值时压缩消息列表。
	// 返回压缩后的消息（可能添加摘要/记忆注入），未超阈值则原样返回。
	CompressChatMessages(ctx context.Context, messages []llmclient.ChatMessage, tokenBudget int) ([]llmclient.ChatMessage, error)
	// CommitChatSession 在工具循环结束后提交对话到长期记忆。
	CommitChatSession(ctx context.Context, userID, sessionKey string, messages []llmclient.ChatMessage) error
	// BuildContextBrief 生成 L0 级别上下文简报 (~200 tokens)，
	// 注入子智能体 (coder/argus) 的工具调用，减少 inter-agent misalignment。
	BuildContextBrief(ctx context.Context) string
}

// EmbeddedAttemptRunner 真实的 AttemptRunner 实现。
// 负责构建消息、调用 LLM API、执行工具循环、返回结果。
type EmbeddedAttemptRunner struct {
	Config            *types.OpenAcosmiConfig
	AuthStore         AuthProfileStore
	ArgusBridge       ArgusBridgeForAgent       // 可选，nil = Argus 不可用
	CoderBridge       CoderBridgeForAgent       // 可选，nil = Coder 不可用
	RemoteMCPBridge   RemoteMCPBridgeForAgent   // 可选，nil = 远程 MCP 工具不可用
	NativeSandbox     NativeSandboxForAgent     // 可选，nil = 使用 Docker fallback
	UHMSBridge        UHMSBridgeForAgent        // 可选，nil = UHMS 记忆系统不可用
	CoderConfirmation *CoderConfirmationManager // 可选，nil = coder 工具不需要确认

	// skillsCache 按需加载缓存: skill name → full SKILL.md content
	// 在 buildSystemPrompt 中填充，在 lookup_skill 工具调用时读取
	skillsCache map[string]string
}

// RunAttempt 实现 AttemptRunner 接口。
func (r *EmbeddedAttemptRunner) RunAttempt(ctx context.Context, params AttemptParams) (*AttemptResult, error) {
	log := slog.Default().With("subsystem", "attempt-runner", "runId", params.RunID)

	log.Debug("attempt start",
		"sessionId", params.SessionID,
		"provider", params.Provider,
		"model", params.ModelID,
	)

	// 注册运行上下文到全局事件总线（供 WebSocket/SSE 路径过滤事件）
	infra.RegisterAgentRunContext(params.RunID, infra.AgentRunContext{
		SessionKey: params.SessionKey,
	})
	defer infra.ClearAgentRunContext(params.RunID)

	// 发射 lifecycle start 事件
	infra.EmitAgentEvent(params.RunID, infra.StreamLifecycle,
		map[string]interface{}{"phase": "start"}, "")

	started := time.Now()

	// 1. 解析 API Key
	apiKey, err := r.resolveAPIKey(params)
	if err != nil {
		return nil, fmt.Errorf("attempt-runner: resolve api key: %w", err)
	}

	// 2. 构建系统提示词
	systemPrompt := r.buildSystemPrompt(params)

	// 3. 构建初始消息（用户 prompt）
	messages := []llmclient.ChatMessage{
		llmclient.TextMessage("user", params.Prompt),
	}

	// 4. 构建工具定义
	tools := r.buildToolDefinitions()

	// 5. 设置超时
	timeout := time.Duration(params.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 6. Tool Loop: 调用 LLM → 执行工具 → 回传结果 → 再调用 LLM
	var (
		assistantTexts []string
		toolMetas      []interface{}
		lastResult     *llmclient.ChatResult
		totalUsage     llmclient.UsageInfo
		lastToolError  string
	)

	baseURL := r.resolveBaseURL(params.Provider)
	var permDeniedRounds int // 连续全部权限拒绝的轮次计数

	for iteration := 0; iteration < maxToolLoopIterations; iteration++ {
		log.Debug("llm call",
			"iteration", iteration,
			"messageCount", len(messages),
		)

		// UHMS: 在 LLM 调用前压缩上下文（可选，nil = 跳过）
		if r.UHMSBridge != nil {
			compressed, compErr := r.UHMSBridge.CompressChatMessages(ctx, messages, 0)
			if compErr != nil {
				log.Warn("uhms compress failed (non-fatal, using original messages)", "error", compErr)
			} else {
				messages = compressed
			}
		}

		// 调用 LLM
		var streamedText strings.Builder
		result, err := llmclient.StreamChat(ctx, llmclient.ChatRequest{
			Provider:     params.Provider,
			Model:        params.ModelID,
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        tools,
			MaxTokens:    getMaxTokens(params),
			ThinkLevel:   params.ThinkLevel,
			TimeoutMs:    params.TimeoutMs,
			APIKey:       apiKey,
			BaseURL:      baseURL,
		}, func(evt llmclient.StreamEvent) {
			switch evt.Type {
			case llmclient.EventText:
				streamedText.WriteString(evt.Text)
			case llmclient.EventError:
				log.Warn("llm stream error", "error", evt.Error)
			}
		})

		if err != nil {
			// 检查超时
			if ctx.Err() == context.DeadlineExceeded {
				return &AttemptResult{
					TimedOut:       true,
					Aborted:        true,
					AssistantTexts: assistantTexts,
					SessionIDUsed:  params.SessionID,
					AttemptUsage:   usageToNormalized(totalUsage),
				}, nil
			}
			// 返回 prompt error
			return &AttemptResult{
				PromptError:    err,
				AssistantTexts: assistantTexts,
				SessionIDUsed:  params.SessionID,
				AttemptUsage:   usageToNormalized(totalUsage),
			}, nil
		}

		lastResult = result
		totalUsage.InputTokens += result.Usage.InputTokens
		totalUsage.OutputTokens += result.Usage.OutputTokens

		// 收集 assistant 文本
		for _, block := range result.AssistantMessage.Content {
			if block.Type == "text" && block.Text != "" {
				assistantTexts = append(assistantTexts, block.Text)
			}
		}

		// 将 assistant 消息加入历史
		messages = append(messages, result.AssistantMessage)

		// 提取工具调用
		toolCalls := extractToolCalls(result.AssistantMessage)

		// 检查是否需要执行工具
		// Anthropic: stopReason="tool_use"; Gemini: stopReason="end_turn" 但消息中仍有 functionCall
		if result.StopReason != "tool_use" && len(toolCalls) == 0 {
			// 非工具调用停止且无工具调用，结束循环
			break
		}
		if len(toolCalls) == 0 {
			break
		}

		// 执行工具并收集结果
		toolResults := make([]llmclient.ContentBlock, 0, len(toolCalls))
		for _, tc := range toolCalls {
			log.Debug("tool call",
				"tool", tc.Name,
				"id", tc.ID,
				"iteration", iteration,
			)

			toolMetas = append(toolMetas, map[string]string{
				"toolName": tc.Name,
			})

			// 发射 tool.start 事件（含完整 args 供前端卡片预览）
			toolArgs := make(map[string]interface{})
			_ = json.Unmarshal(tc.Input, &toolArgs)
			infra.EmitAgentEvent(params.RunID, infra.StreamTool, map[string]interface{}{
				"phase":      "start",
				"name":       tc.Name,
				"toolCallId": tc.ID,
				"args":       toolArgs,
			}, "")

			// 动态获取安全级别（含临时提权）
			secLvl := resolveSecurityLevel(r.Config)
			if params.SecurityLevelFunc != nil {
				secLvl = params.SecurityLevelFunc()
			}

			output, toolErr := ExecuteToolCall(ctx, tc.Name, tc.Input, ToolExecParams{
				WorkspaceDir:        params.WorkspaceDir,
				TimeoutMs:           params.TimeoutMs,
				AllowWrite:          secLvl == "full" || secLvl == "allowlist", // L1 沙箱内允许写入
				AllowExec:           secLvl == "full" || secLvl == "allowlist",
				SandboxMode:         secLvl == "allowlist",
				Rules:               resolveCommandRules(),
				SecurityLevel:       secLvl,
				OnPermissionDenied:  params.OnPermissionDenied,
				ArgusBridge:         r.ArgusBridge,
				CoderBridge:         r.CoderBridge,
				CoderTimeoutSeconds: resolveCoderTimeoutSeconds(r.Config),
				CoderConfirmation:   r.CoderConfirmation,
				RemoteMCPBridge:     r.RemoteMCPBridge,
				NativeSandbox:       r.NativeSandbox,
				SkillsCache:         r.skillsCache,
				UHMSBridge:          r.UHMSBridge,
			})

			isError := false
			if toolErr != nil {
				output = fmt.Sprintf("Error: %s", toolErr.Error())
				isError = true
				lastToolError = fmt.Sprintf("%s: %s", tc.Name, toolErr.Error())
			}

			// 发射 tool.result 事件
			resultData := map[string]interface{}{
				"phase":      "result",
				"name":       tc.Name,
				"toolCallId": tc.ID,
				"isError":    isError,
			}
			if output != "" {
				resultData["result"] = truncateForEvent(output, 2048)
			}
			infra.EmitAgentEvent(params.RunID, infra.StreamTool, resultData, "")

			toolResults = append(toolResults, llmclient.ContentBlock{
				Type:       "tool_result",
				ToolUseID:  tc.ID,
				Name:       tc.Name, // S1-4: Gemini functionResponse 需要 Name 字段
				ResultText: output,
				IsError:    isError,
			})
		}

		// --- 权限拒绝: 等待审批或断路 ---
		allDenied := len(toolResults) > 0
		for _, tr := range toolResults {
			if !IsPermissionDeniedOutput(tr.ResultText) {
				allDenied = false
				break
			}
		}
		if allDenied {
			permDeniedRounds++

			// 首次全部拒绝时等待审批（如果有回调）
			if params.WaitForApproval != nil && permDeniedRounds == 1 {
				log.Info("permission denied, waiting for approval...",
					"iteration", iteration,
				)
				approved := params.WaitForApproval(ctx)
				if approved {
					log.Info("approval granted, retrying tools with new security level",
						"iteration", iteration,
					)
					// 审批通过 — 清空本轮拒绝的 tool_results，用新权限重试
					permDeniedRounds = 0
					toolResults = toolResults[:0]
					for _, tc := range toolCalls {
						// F-02: 重试路径也需发射 tool 事件（前端按 toolCallId 覆盖更新）
						retryArgs := make(map[string]interface{})
						_ = json.Unmarshal(tc.Input, &retryArgs)
						infra.EmitAgentEvent(params.RunID, infra.StreamTool, map[string]interface{}{
							"phase":      "start",
							"name":       tc.Name,
							"toolCallId": tc.ID,
							"args":       retryArgs,
						}, "")

						secLvl := resolveSecurityLevel(r.Config)
						if params.SecurityLevelFunc != nil {
							secLvl = params.SecurityLevelFunc()
						}
						output, toolErr := ExecuteToolCall(ctx, tc.Name, tc.Input, ToolExecParams{
							WorkspaceDir:        params.WorkspaceDir,
							TimeoutMs:           params.TimeoutMs,
							AllowWrite:          secLvl == "full" || secLvl == "allowlist", // L1 沙箱内允许写入
							AllowExec:           secLvl == "full" || secLvl == "allowlist",
							SandboxMode:         secLvl == "allowlist",
							Rules:               resolveCommandRules(),
							SecurityLevel:       secLvl,
							OnPermissionDenied:  params.OnPermissionDenied,
							ArgusBridge:         r.ArgusBridge,
							CoderBridge:         r.CoderBridge,
							CoderTimeoutSeconds: resolveCoderTimeoutSeconds(r.Config),
							CoderConfirmation:   r.CoderConfirmation,
							RemoteMCPBridge:     r.RemoteMCPBridge,
							NativeSandbox:       r.NativeSandbox,
							SkillsCache:         r.skillsCache,
							UHMSBridge:          r.UHMSBridge,
						})
						isError := false
						if toolErr != nil {
							output = fmt.Sprintf("Error: %s", toolErr.Error())
							isError = true
							lastToolError = fmt.Sprintf("%s: %s", tc.Name, toolErr.Error())
						}

						// F-02: 重试路径 tool.result 事件
						retryResult := map[string]interface{}{
							"phase":      "result",
							"name":       tc.Name,
							"toolCallId": tc.ID,
							"isError":    isError,
						}
						if output != "" {
							retryResult["result"] = truncateForEvent(output, 2048)
						}
						infra.EmitAgentEvent(params.RunID, infra.StreamTool, retryResult, "")

						toolResults = append(toolResults, llmclient.ContentBlock{
							Type:       "tool_result",
							ToolUseID:  tc.ID,
							Name:       tc.Name,
							ResultText: output,
							IsError:    isError,
						})
					}
				} else {
					// 审批被拒绝或超时 — 直接退出
					log.Warn("approval denied/timed out, stopping",
						"iteration", iteration,
					)
					assistantTexts = append(assistantTexts,
						"⚠️ 权限审批被拒绝或超时。如需执行此操作，请调整安全设置后重试。\n"+
							"Permission approval denied or timed out. Adjust security settings and try again.")
					break
				}
			} else if permDeniedRounds >= maxConsecutivePermDeniedRounds {
				// 无等待回调或多轮连续拒绝 — 硬断路
				log.Warn("circuit breaker: all tool calls permission-denied, stopping",
					"rounds", permDeniedRounds,
					"iteration", iteration,
				)
				assistantTexts = append(assistantTexts,
					"⚠️ 权限不足，无法执行请求的操作。请在聊天窗口的权限弹窗中点击「临时授权」放行，"+
						"或前往 安全设置 → 执行安全级别 调整权限。\n"+
						"Permission denied. Please authorize via the permission popup or adjust security settings.")
				break
			}
		} else {
			permDeniedRounds = 0
		}

		// 将工具结果作为 user 消息加入历史
		messages = append(messages, llmclient.ChatMessage{
			Role:    "user",
			Content: toolResults,
		})
	}

	// 发射 lifecycle end 事件
	infra.EmitAgentEvent(params.RunID, infra.StreamLifecycle,
		map[string]interface{}{"phase": "end"}, "")

	// 6.5 UHMS: 工具循环结束后异步提交对话到长期记忆
	// 使用 panic recovery 包装 (参考 CockroachDB Stopper / LaunchDarkly GoSafely)
	if r.UHMSBridge != nil {
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					stack := make([]byte, 8192)
					stack = stack[:runtime.Stack(stack, false)]
					slog.Error("uhms commit session panicked",
						slog.Any("panic", rec),
						slog.String("stack", string(stack)),
					)
				}
			}()
			// 本地单用户场景: 用 sessionKey 作为 userID 代理
			uid := params.SessionKey
			if uid == "" {
				uid = "default"
			}
			commitErr := r.UHMSBridge.CommitChatSession(
				context.Background(), uid, params.SessionID, messages)
			if commitErr != nil {
				slog.Warn("uhms commit session failed (non-fatal)", "error", commitErr)
			}
		}()
	}

	// 7. 构建最终结果
	result := &AttemptResult{
		SessionIDUsed:  params.SessionID,
		AssistantTexts: assistantTexts,
		ToolMetas:      toolMetas,
		AttemptUsage:   usageToNormalized(totalUsage),
		LastToolError:  lastToolError,
	}

	if lastResult != nil {
		result.LastAssistant = &AssistantMessage{
			Provider:   params.Provider,
			Model:      params.ModelID,
			StopReason: lastResult.StopReason,
		}
	}

	durationMs := time.Since(started).Milliseconds()
	log.Debug("attempt end",
		"sessionId", params.SessionID,
		"durationMs", durationMs,
		"assistantCount", len(assistantTexts),
		"toolCount", len(toolMetas),
	)

	return result, nil
}

// ---------- 内部方法 ----------

func (r *EmbeddedAttemptRunner) resolveAPIKey(params AttemptParams) (string, error) {
	if r.AuthStore == nil {
		// 优先从配置文件读取 API key（向导保存的值）
		if r.Config != nil && r.Config.Models != nil && r.Config.Models.Providers != nil {
			if pc := r.Config.Models.Providers[strings.ToLower(params.Provider)]; pc != nil && pc.APIKey != "" {
				return pc.APIKey, nil
			}
			// 也尝试原始 provider 名称（大小写可能不同）
			if pc := r.Config.Models.Providers[params.Provider]; pc != nil && pc.APIKey != "" {
				return pc.APIKey, nil
			}
		}
		// 尝试从环境变量读取
		switch strings.ToLower(params.Provider) {
		case "anthropic":
			if key := lookupEnvKey("ANTHROPIC_API_KEY"); key != "" {
				return key, nil
			}
		case "openai":
			if key := lookupEnvKey("OPENAI_API_KEY"); key != "" {
				return key, nil
			}
		case "ollama":
			return "", nil // Ollama 不需要 API key
		default:
			// 通用 provider 环境变量解析（支持 deepseek 等）
			if key := models.ResolveEnvApiKeyWithFallback(params.Provider); key != "" {
				return key, nil
			}
		}
		return "", fmt.Errorf("no API key found for provider %q", params.Provider)
	}

	info, err := r.AuthStore.GetApiKeyForModel(params.Model, params.Config, "", params.AgentDir)
	if err != nil {
		return "", err
	}
	if info == nil || info.ApiKey == "" {
		// Fallback to env
		switch strings.ToLower(params.Provider) {
		case "anthropic":
			if key := lookupEnvKey("ANTHROPIC_API_KEY"); key != "" {
				return key, nil
			}
		case "openai":
			if key := lookupEnvKey("OPENAI_API_KEY"); key != "" {
				return key, nil
			}
		case "ollama":
			return "", nil
		default:
			if key := models.ResolveEnvApiKeyWithFallback(params.Provider); key != "" {
				return key, nil
			}
		}
		return "", fmt.Errorf("no API key resolved for %s", params.Provider)
	}
	return info.ApiKey, nil
}

func (r *EmbeddedAttemptRunner) buildSystemPrompt(params AttemptParams) string {
	// 使用 prompt 包的 BuildAgentSystemPrompt 构建完整系统提示
	rt := prompt.DefaultRuntimeInfo()
	rt.Model = params.Provider + "/" + params.ModelID

	// 构建技能快照 → 按需加载模式: prompt 只放索引，LLM 通过 lookup_skill 获取完整内容
	skillsPrompt := ""
	if params.WorkspaceDir != "" {
		bundledDir := skills.ResolveBundledSkillsDir("")
		snap := skills.BuildWorkspaceSkillSnapshot(skills.BuildSnapshotParams{
			WorkspaceDir: params.WorkspaceDir,
			BundledDir:   bundledDir,
			Config:       r.Config,
		})

		// 填充 skillsCache（lookup_skill 工具使用）
		r.skillsCache = make(map[string]string, len(snap.ResolvedSkills))
		for _, s := range snap.ResolvedSkills {
			if s.Content != "" {
				r.skillsCache[s.Name] = s.Content
			}
		}

		// 所有技能统一走按需加载: prompt 只放紧凑索引，LLM 通过 lookup_skill 获取完整内容
		if idx := skills.FormatSkillIndex(snap.ResolvedSkills); idx != "" {
			skillsPrompt = idx
		}
	}

	bp := prompt.BuildParams{
		Mode:              prompt.PromptModeFull,
		WorkspaceDir:      params.WorkspaceDir,
		ExtraSystemPrompt: params.ExtraSystemPrompt,
		SkillsPrompt:      skillsPrompt,
		RuntimeInfo:       &rt,
		ThinkLevel:        params.ThinkLevel,
	}

	return prompt.BuildAgentSystemPrompt(bp)
}

func (r *EmbeddedAttemptRunner) buildToolDefinitions() []llmclient.ToolDef {
	tools := []llmclient.ToolDef{
		{
			Name:        "bash",
			Description: "Execute a bash command in the workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The bash command to execute"}},"required":["command"]}`),
		},
		{
			Name:        "read_file",
			Description: "Read file contents.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file"}},"required":["path"]}`),
		},
		{
			Name:        "write_file",
			Description: "Write content to a file.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file"},"content":{"type":"string","description":"Content to write"}},"required":["path","content"]}`),
		},
		{
			Name:        "list_dir",
			Description: "List directory contents.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the directory"}},"required":["path"]}`),
		},
	}

	// 技能按需加载工具
	if len(r.skillsCache) > 0 {
		tools = append(tools, llmclient.ToolDef{
			Name:        "lookup_skill",
			Description: "Look up the full content of a skill by name. Use this when a skill from <available_skills> applies to the current task.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Skill name from <available_skills>"}},"required":["name"]}`),
		})
	}

	// 追加 Argus 视觉工具（前缀 argus_ 以区分）
	if r.ArgusBridge != nil {
		for _, t := range r.ArgusBridge.AgentTools() {
			tools = append(tools, llmclient.ToolDef{
				Name:        "argus_" + t.Name,
				Description: "[Argus 视觉] " + t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	// 追加 Coder 编程工具（前缀 coder_ 以区分）
	if r.CoderBridge != nil {
		for _, t := range r.CoderBridge.AgentTools() {
			tools = append(tools, llmclient.ToolDef{
				Name:        "coder_" + t.Name,
				Description: "[Coder 编程] " + t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	// 追加远程 MCP 工具（前缀 remote_ 以区分）
	if r.RemoteMCPBridge != nil {
		for _, t := range r.RemoteMCPBridge.AgentRemoteTools() {
			tools = append(tools, llmclient.ToolDef{
				Name:        "remote_" + t.Name,
				Description: "[远程] " + t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	return tools
}

func (r *EmbeddedAttemptRunner) resolveBaseURL(provider string) string {
	return models.ResolveProviderBaseURL(provider, r.Config)
}

// ---------- 辅助函数 ----------

// truncateForEvent 截断输出到 maxLen bytes，用于事件负载防膨胀。
// 使用 rune-aware 截断，保证输出始终是有效 UTF-8。
func truncateForEvent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// 逐 rune 累计字节数，在超过 maxLen 前停止
	byteCount := 0
	for _, r := range s {
		runeLen := len(string(r))
		if byteCount+runeLen > maxLen {
			break
		}
		byteCount += runeLen
	}
	return s[:byteCount] + "…[truncated]"
}

func extractToolCalls(msg llmclient.ChatMessage) []llmclient.ContentBlock {
	var calls []llmclient.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			calls = append(calls, block)
		}
	}
	return calls
}

func usageToNormalized(usage llmclient.UsageInfo) *NormalizedUsage {
	n := &NormalizedUsage{}
	if usage.InputTokens > 0 {
		input := usage.InputTokens
		n.Input = &input
	}
	if usage.OutputTokens > 0 {
		output := usage.OutputTokens
		n.Output = &output
	}
	return n
}

func getMaxTokens(params AttemptParams) int {
	// 供应商级最大 token 限制
	providerMax := 0
	if defaults := models.GetProviderDefaults(params.Provider); defaults != nil && defaults.MaxTokens > 0 {
		providerMax = defaults.MaxTokens
	}

	if params.Model != nil && params.Model.ContextWindow > 0 {
		// 使用上下文窗口的 1/4 作为最大输出
		max := params.Model.ContextWindow / 4
		if max < 4096 {
			max = 4096
		}
		if max > 16384 {
			max = 16384
		}
		// 尊重供应商级限制
		if providerMax > 0 && max > providerMax {
			max = providerMax
		}
		return max
	}

	if providerMax > 0 {
		return providerMax
	}
	return 8192 // 默认
}

func lookupEnvKey(name string) string {
	return strings.TrimSpace(envLookup(name))
}

// envLookup 读取环境变量 — 提取为函数便于测试。
var envLookup = os.Getenv

// ---------- 权限解析 (P0) ----------

// resolveAllowWrite 从配置解析是否允许写文件。
// 安全级别映射: "full" → true, 其他 → false
func resolveAllowWrite(cfg *types.OpenAcosmiConfig) bool {
	return resolveSecurityLevel(cfg) == "full"
}

// resolveAllowExec 从配置解析是否允许执行命令。
// 安全级别映射: "full" 或 "allowlist" → true, 其他 → false
func resolveAllowExec(cfg *types.OpenAcosmiConfig) bool {
	level := resolveSecurityLevel(cfg)
	return level == "full" || level == "allowlist"
}

// resolveSandboxMode 判断是否启用 Docker 沙箱模式。
// 仅 L1 (sandbox/allowlist) 级别时启用，L2 (full) 直接在宿主机执行。
func resolveSandboxMode(cfg *types.OpenAcosmiConfig) bool {
	return resolveSecurityLevel(cfg) == "allowlist"
}

// resolveSecurityLevel 从 OpenAcosmiConfig 中提取 tools.exec.security 字段值。
// 返回规范化的安全级别字符串: "deny", "allowlist", "full"。
// 兼容 "sandbox" 作为 "allowlist" 的别名，"off" 作为 "deny" 的别名。
func resolveSecurityLevel(cfg *types.OpenAcosmiConfig) string {
	if cfg == nil || cfg.Tools == nil || cfg.Tools.Exec == nil {
		return "deny"
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Tools.Exec.Security)) {
	case "full":
		return "full"
	case "allowlist", "sandbox":
		return "allowlist"
	case "deny", "off", "":
		return "deny"
	default:
		return "deny"
	}
}

// resolveCoderTimeoutSeconds 从 config 安全提取 coder 超时秒数。
// 返回 0 表示使用默认值（executeCoderTool 内默认 120s）。
func resolveCoderTimeoutSeconds(cfg *types.OpenAcosmiConfig) int {
	if cfg == nil || cfg.Agents == nil || cfg.Agents.Defaults == nil ||
		cfg.Agents.Defaults.Coder == nil || cfg.Agents.Defaults.Coder.TimeoutSeconds == nil {
		return 0
	}
	v := *cfg.Agents.Defaults.Coder.TimeoutSeconds
	if v < 10 {
		return 10 // 最低 10s 防止过短
	}
	if v > 600 {
		return 600 // 最高 600s (10 分钟)
	}
	return v
}

// ---------- 命令规则解析 (P3) ----------

// resolveCommandRules 从 exec-approvals.json 读取用户自定义规则并与预设规则合并。
func resolveCommandRules() []infra.CommandRule {
	snapshot := infra.ReadExecApprovalsSnapshot()
	var userRules []infra.CommandRule
	if snapshot.File != nil && snapshot.File.Defaults != nil {
		userRules = snapshot.File.Defaults.Rules
	}
	return MergeRulesWithPresets(userRules)
}
