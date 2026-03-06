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
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/prompt"
	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	goproviders_common "github.com/Acosmi/ClawAcosmi/internal/goproviders/common"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/internal/routing"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
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
	// AgentCallToolMultimodal 返回多模态工具结果（含 image blocks）。
	// 如未实现可返回 nil, 调用方降级到 AgentCallTool。
	AgentCallToolMultimodal(ctx context.Context, name string, args json.RawMessage, timeout time.Duration) ([]llmclient.ContentBlock, error)
}

// (Phase 2A: CoderBridgeForAgent 已删除 — oa-coder 升级为 spawn_coder_agent)

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
	// Bug#11: 从 30 降到 15 — 审计中 6 轮已过多，30 完全不合理。
	maxToolLoopIterations = 15
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

	// --- Boot 模式扩展 ---

	// IsSkillsIndexed 检查技能是否已分级到 VFS（Boot 模式前置条件）。
	IsSkillsIndexed() bool
	// IsSkillsDistributing 返回 true 表示技能正在索引中（部分结果可能不完整）。
	IsSkillsDistributing() bool
	// SearchSkillsVFS 在 VFS 中搜索技能（Qdrant 主路径 → VFS meta.json 降级）。
	SearchSkillsVFS(ctx context.Context, query string, topK int) ([]SkillSearchHit, error)
	// ReadSkillVFS 从 VFS 读取技能 L2 完整内容。
	ReadSkillVFS(ctx context.Context, category, name string) (string, error)

	// --- Bug#11: 记忆搜索/获取工具接口 ---

	// SearchMemories 在记忆系统中搜索与 query 相关的记忆条目。
	SearchMemories(ctx context.Context, query string, limit int) ([]MemorySearchHit, error)
	// GetMemory 根据 ID 获取单条记忆的完整内容。
	GetMemory(ctx context.Context, id string) (*MemoryHit, error)
}

// MemorySearchHit 记忆搜索命中结果。
type MemorySearchHit struct {
	ID       string  `json:"id"`
	Content  string  `json:"content"`
	Category string  `json:"category,omitempty"`
	Type     string  `json:"type,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// MemoryHit 单条记忆完整内容。
type MemoryHit struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Category  string `json:"category,omitempty"`
	Type      string `json:"type,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

// SkillSearchHit 技能搜索命中结果。
type SkillSearchHit struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Abstract string `json:"abstract"` // L0 摘要
	VFSPath  string `json:"vfs_path"`
}

// EmbeddedAttemptRunner 真实的 AttemptRunner 实现。
// 负责构建消息、调用 LLM API、执行工具循环、返回结果。
type EmbeddedAttemptRunner struct {
	Config      *types.OpenAcosmiConfig
	AuthStore   AuthProfileStore
	ArgusBridge ArgusBridgeForAgent // 可选，nil = Argus 不可用
	// (Phase 2A: CoderBridge 已删除 — oa-coder 升级为 spawn_coder_agent)
	RemoteMCPBridge   RemoteMCPBridgeForAgent   // 可选，nil = 远程 MCP 工具不可用
	NativeSandbox     NativeSandboxForAgent     // 可选，nil = 使用 Docker fallback
	UHMSBridge        UHMSBridgeForAgent        // 可选，nil = UHMS 记忆系统不可用
	CoderConfirmation *CoderConfirmationManager // 可选，nil = coder 工具不需要确认
	// PlanConfirmation 方案确认管理器（可选，nil = 不需要方案确认门控）
	// Phase 1: 三级指挥体系 — task_write/task_delete/task_multimodal 意图下先确认方案再执行
	PlanConfirmation *PlanConfirmationManager
	// SpawnSubagent 子智能体生成回调（可选，nil = spawn_coder_agent 返回合约但不启动 session）
	SpawnSubagent SpawnSubagentFunc
	// BrowserEvaluateEnabled 是否允许 browser evaluate（JS 执行），默认 true。
	BrowserEvaluateEnabled bool
	// BrowserController 浏览器自动化控制器（可选，nil = browser 工具不可用）
	BrowserController interface {
		Navigate(ctx context.Context, url string) error
		GetContent(ctx context.Context) (string, error)
		Click(ctx context.Context, selector string) error
		Type(ctx context.Context, selector, text string) error
		Screenshot(ctx context.Context) ([]byte, string, error)
		Evaluate(ctx context.Context, script string) (any, error)
		WaitForSelector(ctx context.Context, selector string) error
		GoBack(ctx context.Context) error
		GoForward(ctx context.Context) error
		GetURL(ctx context.Context) (string, error)
		// Phase 1: ARIA 快照 + ref 元素交互
		SnapshotAI(ctx context.Context) (map[string]any, error)
		ClickRef(ctx context.Context, ref string) error
		FillRef(ctx context.Context, ref, text string) error
		// Phase 4: Mariner AI 循环
		AIBrowse(ctx context.Context, goal string) (string, error)
	}
	// WebSearchProvider 网页搜索 provider（可选，nil = web_search 工具不可用）
	WebSearchProvider interface {
		Search(ctx context.Context, query string, maxResults int) ([]WebSearchResult, error)
	}
	// MediaSender 媒体文件发送器（可选，nil = send_media 工具不可用）
	MediaSender interface {
		SendMedia(ctx context.Context, channelID, to string, data []byte, fileName, mimeType, message string) error
	}
	// MediaSubsystem 媒体子系统（可选，nil = spawn_media_agent 工具不注册）。
	// 提供 trending_topics / content_compose / media_publish / social_interact 工具。
	MediaSubsystem MediaSubsystemForAgent

	// QualityReviewFn 质量审核回调（可选，nil = 跳过 LLM 语义审核，只做规则预检）
	// Phase 2: 三级指挥体系 — 子智能体结果质量审核
	QualityReviewFn QualityReviewFunc
	// ResultApprovalMgr 结果签收管理器（可选，nil = 跳过最终交付门控）
	// Phase 3: 三级指挥体系 — 质量审核通过后用户签收
	ResultApprovalMgr *ResultApprovalManager

	// ArgusApprovalMode Argus 工具审批模式（"none"/"medium_and_above"/"all"），
	// 从配置 SubAgents.ScreenObserver.ApprovalMode 读取。
	ArgusApprovalMode string

	// Phase 6: 合约持久化（可选，nil = 恢复上下文不可用）
	ContractStore ContractPersistence

	// skillsCache 按需加载缓存: skill name → full SKILL.md content
	// 在 buildSystemPrompt 中填充，在 lookup_skill 工具调用时读取
	skillsCache map[string]string
	// toolBindings 工具绑定: tool name → skill description
	// 在 buildSystemPrompt 中从技能 entries 构建，在 buildToolDefinitions 中注入到工具 Description
	toolBindings map[string]string
}

// WebSearchResult 搜索结果（与 tools.WebSearchResult 对齐，避免循环依赖）。
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
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

	// 2. 加载对话历史 + 会话状态检测
	priorMessages := r.loadPriorMessages(params, log)
	// 会话状态三态判定:
	//   COLD_START: 无 assistant 历史 + 无 boot brief → 全新用户
	//   WARM_START: 无 assistant 历史 + 有 boot brief → 重启唤醒
	//   NORMAL: 有 assistant 历史 → 常规对话
	hasAssistant := hasAnyAssistantMessage(priorMessages)
	var hasBootBrief bool
	if r.UHMSBridge != nil {
		hasBootBrief = r.UHMSBridge.BuildContextBrief(context.Background()) != ""
	}
	sessionState := prompt.SessionNormal
	if !hasAssistant {
		if hasBootBrief {
			sessionState = prompt.SessionWarmStart
		} else {
			sessionState = prompt.SessionColdStart
		}
	}

	// 3. 意图分级（六级分类，影响工具过滤 + 历史裁剪 + 系统提示词）
	// 提前到 buildSystemPrompt 之前，使 intent guidance 能注入提示词
	tier := classifyIntent(params.Prompt)

	// 4. 构建工具定义（必须在构建系统提示词之前，以便传入真实工具名列表）
	tools := r.buildToolDefinitions()
	// Phase 4: 如果有异步消息通道，动态添加 request_help 工具
	if params.AgentChannel != nil {
		tools = append(tools, RequestHelpToolDef())
	}
	// Fix D5-4: report_progress 工具 — 允许智能体主动汇报中间进度
	tools = append(tools, ReportProgressToolDef())
	// 媒体子智能体: 注入媒体专属工具（主智能体只见 spawn_media_agent 入口）
	if params.AgentType == "media" && r.MediaSubsystem != nil {
		for _, name := range r.MediaSubsystem.ToolNames() {
			if schema, desc, ok := r.MediaSubsystem.GetToolDef(name); ok {
				tools = append(tools, llmclient.ToolDef{
					Name:        name,
					Description: "[媒体] " + desc,
					InputSchema: schema,
				})
			}
		}
	}

	// 4b. 提取真实工具名列表（未经意图过滤），用于构建 ## Tooling 段落
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}
	toolSummaries := capabilities.ToolSummaries()

	// 5. 构建系统提示词（含会话状态路由 + 意图行为指引 + 真实工具名）
	systemPrompt := r.buildSystemPrompt(params, sessionState, tier, toolNames, toolSummaries)

	// 5a. 按意图裁剪历史（greeting 靠 boot brief 感知上下文，不需要历史）
	priorMessages = trimHistoryByIntent(priorMessages, tier)

	// 5b. 组装消息列表
	messages := append(priorMessages, llmclient.TextMessage("user", params.Prompt))

	// 5b.1 Transcript 持久化保障: 使用 defer 确保即使 LLM 失败/超时也能持久化用户消息。
	transcriptPersisted := params.SuppressTranscript
	defer func() {
		if !transcriptPersisted {
			r.persistToTranscript(params, messages, log)
		}
	}()

	// 5c. 工具过滤（六级分级暴露 3-12 个工具，约束 LLM 输出收敛性）
	tools = filterToolsByIntent(tools, tier)
	log.Debug("intent filter", "tier", tier, "toolCount", len(tools), "historyCount", len(priorMessages))

	effectiveSecurityLevel := resolveEffectiveSecurityLevel(params, r.Config)

	// 5c. Phase 1: 方案确认门控（三级指挥体系）
	// [R1] 使用独立 context，不受 RunAttempt timeout 约束。
	// 用户确认等待（最长 5min）不消耗 RunAttempt 的执行时间预算。
	if r.PlanConfirmation != nil && needsPlanConfirmation(tier) && r.PlanConfirmation.ShouldGate() {
		if effectiveSecurityLevel == "full" {
			log.Debug("plan confirmation bypassed: full security level", "tier", tier)
		} else {
			planCtx, planCancel := context.WithTimeout(context.Background(), r.PlanConfirmation.Timeout())
			defer planCancel()

			planReq := PlanConfirmationRequest{
				TaskBrief:  params.Prompt,
				IntentTier: string(tier),
			}

			log.Debug("plan confirmation gate triggered", "tier", tier)

			decision, planErr := r.PlanConfirmation.RequestPlanConfirmation(planCtx, planReq)
			if planErr != nil {
				log.Warn("plan confirmation error, aborting", "error", planErr)
				return &AttemptResult{
					Aborted:        true,
					AssistantTexts: []string{fmt.Sprintf("方案确认出错: %v", planErr)},
					SessionIDUsed:  params.SessionID,
				}, nil
			}

			switch decision.Action {
			case "reject":
				log.Info("plan rejected by user", "feedback", decision.Feedback)
				return &AttemptResult{
					Aborted:        true,
					AssistantTexts: []string{fmt.Sprintf("方案已被拒绝。%s", decision.Feedback)},
					SessionIDUsed:  params.SessionID,
				}, nil
			case "edit":
				// 用修改后方案增强 prompt 上下文
				if decision.EditedPlan != "" {
					messages = append(messages, llmclient.TextMessage("user",
						fmt.Sprintf("[用户修改方案] %s", decision.EditedPlan)))
					log.Debug("plan edited by user, augmenting prompt")
				}
			case "approve":
				log.Debug("plan approved by user")
			}
		}
	}

	// 6. 设置超时
	timeout := time.Duration(params.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 6. Tool Loop: 调用 LLM → 执行工具 → 回传结果 → 再调用 LLM
	var (
		assistantTexts []string
		mediaBlocks    []MediaBlock
		toolMetas      []interface{}
		lastResult     *llmclient.ChatResult
		totalUsage     llmclient.UsageInfo
		lastToolError  string
	)

	baseURL := r.resolveBaseURL(params.Provider)
	var permDeniedRounds int // 连续全部权限拒绝的轮次计数

	// 同工具死循环检测: 连续多轮仅调用同一个工具则断路
	var (
		consecutiveSameToolCount int
		lastSingleToolName       string
	)
	const maxConsecutiveSameTool = 5 // 同一工具连续调用超过此值触发断路

	// 工具失败计数: 跟踪每个工具的累计失败/错误次数（含交替调用场景）
	toolFailureCounts := make(map[string]int)
	const maxToolFailures = 3 // 单个工具累计失败超过此值注入指导消息
	const maxToolHardStop = 5 // Bug#11: 单个工具累计失败超过此值强制终止循环
	totalToolFailures := 0
	// 全局工具失败预算: 净失败数（失败+1、成功-1、下限0）超过此值终止循环。
	// 使用净值而非单调累加，避免长会话中偶发失败的无害积累误触发断路。
	const maxTotalToolFailures = 8

	for iteration := 0; iteration < maxToolLoopIterations; iteration++ {
		log.Debug("llm call",
			"iteration", iteration,
			"messageCount", len(messages),
		)

		// Phase 4: 非阻塞检查主智能体指令通道
		// 如果主智能体通过 AgentChannel 发送了指令/回复，注入到 messages 作为上下文。
		if params.AgentChannel != nil {
			for {
				directive := params.AgentChannel.ReceiveFromParent()
				if directive == nil {
					break
				}
				log.Debug("received directive from parent",
					"msgID", directive.ID,
					"type", directive.Type,
				)
				// 注入为 user 角色消息，让 LLM 在下一轮看到
				directiveText := formatDirectiveAsContext(directive)
				messages = append(messages, llmclient.TextMessage("user", directiveText))
			}
		}

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
			AuthMode:     r.resolveAuthMode(params.Provider),
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
		// --- 同工具死循环检测 ---
		if len(toolCalls) == 1 && toolCalls[0].Name == lastSingleToolName {
			consecutiveSameToolCount++
		} else {
			consecutiveSameToolCount = 0
		}
		if len(toolCalls) == 1 {
			lastSingleToolName = toolCalls[0].Name
		} else {
			lastSingleToolName = ""
		}

		if consecutiveSameToolCount >= maxConsecutiveSameTool {
			log.Warn("circuit breaker: same tool called consecutively, breaking loop",
				"tool", lastSingleToolName,
				"count", consecutiveSameToolCount+1,
				"iteration", iteration,
			)
			// D2-F1: 熔断时移除本轮 LLM 输出文本（通常包含无用的自我介绍），
			// 只保留熔断警告，避免用户先看到"我是创宇太虚..."再看到警告。
			if len(assistantTexts) > 0 {
				assistantTexts = assistantTexts[:len(assistantTexts)-1]
			}
			assistantTexts = append(assistantTexts,
				fmt.Sprintf("⚠️ 工具 %s 已连续调用 %d 次未取得进展，自动终止循环。请换一种方式尝试或直接咨询用户。\n"+
					"Tool %s was called %d times consecutively without progress. Loop terminated. Try a different approach or ask the user.",
					lastSingleToolName, consecutiveSameToolCount+1,
					lastSingleToolName, consecutiveSameToolCount+1))
			break
		}

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

			// 频道工具事件: start
			if params.OnToolEvent != nil {
				params.OnToolEvent(ToolEvent{
					Phase:    "start",
					ToolName: tc.Name,
					ToolID:   tc.ID,
					Args:     extractToolArgsSummary(tc.Name, toolArgs),
				})
			}

			// 动态获取安全级别（含临时提权）
			secLvl := resolveSecurityLevel(r.Config)
			if params.SecurityLevelFunc != nil {
				secLvl = params.SecurityLevelFunc()
			}

			toolExecParams := r.buildToolExecParams(params, secLvl)
			toolStartTime := time.Now()
			output, toolErr := ExecuteToolCall(ctx, tc.Name, tc.Input, toolExecParams)

			isError := false
			if toolErr != nil {
				output = fmt.Sprintf("Error: %s", toolErr.Error())
				isError = true
				lastToolError = fmt.Sprintf("%s: %s", tc.Name, toolErr.Error())
			}

			// 工具失败计数: 错误或含 "[send_media]"/"Error:" 前缀的软错误
			if isError || isToolSoftError(tc.Name, output) {
				toolFailureCounts[tc.Name]++
				totalToolFailures++
				if toolFailureCounts[tc.Name] >= maxToolFailures {
					guidance := toolFailureGuidance(tc.Name, toolFailureCounts[tc.Name])
					if guidance != "" {
						output += "\n\n" + guidance
					}
					log.Warn("tool failure threshold reached",
						"tool", tc.Name,
						"failCount", toolFailureCounts[tc.Name],
						"totalFail", totalToolFailures,
						"iteration", iteration,
					)
				}
			} else {
				// 成功调用重置该工具的失败计数，全局净失败数回退（下限 0）
				toolFailureCounts[tc.Name] = 0
				if totalToolFailures > 0 {
					totalToolFailures--
				}
			}

			// 频道工具事件: end
			if params.OnToolEvent != nil {
				params.OnToolEvent(ToolEvent{
					Phase:    "end",
					ToolName: tc.Name,
					ToolID:   tc.ID,
					Args:     extractToolArgsSummary(tc.Name, toolArgs),
					Result:   truncateRuneSafe(output, 200),
					IsError:  isError,
					Duration: time.Since(toolStartTime).Milliseconds(),
				})
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

			// 检查多模态结果（Argus image blocks）
			if strings.HasPrefix(output, "__MULTIMODAL__") {
				var resultBlocks []llmclient.ContentBlock
				if jsonErr := json.Unmarshal([]byte(output[len("__MULTIMODAL__"):]), &resultBlocks); jsonErr == nil {
					toolResults = append(toolResults, llmclient.ContentBlock{
						Type:         "tool_result",
						ToolUseID:    tc.ID,
						Name:         tc.Name,
						ResultBlocks: resultBlocks,
						IsError:      isError,
					})
					// 提取图片块，用于出站管线传播
					for _, rb := range resultBlocks {
						if rb.Type == "image" && rb.Source != nil && rb.Source.Data != "" {
							mediaBlocks = append(mediaBlocks, MediaBlock{
								MimeType: rb.Source.MediaType,
								Base64:   rb.Source.Data,
							})
							log.Info("media block extracted from tool result",
								"tool", tc.Name,
								"mimeType", rb.Source.MediaType,
								"dataLen", len(rb.Source.Data),
							)
						}
					}
				} else {
					// JSON 解析失败，降级为纯文本
					toolResults = append(toolResults, llmclient.ContentBlock{
						Type:       "tool_result",
						ToolUseID:  tc.ID,
						Name:       tc.Name,
						ResultText: output[len("__MULTIMODAL__"):],
						IsError:    isError,
					})
				}
			} else {
				toolResults = append(toolResults, llmclient.ContentBlock{
					Type:       "tool_result",
					ToolUseID:  tc.ID,
					Name:       tc.Name, // S1-4: Gemini functionResponse 需要 Name 字段
					ResultText: output,
					IsError:    isError,
				})
			}
		}

		// --- Bug#11: L2 硬停 — 单个工具累计失败 >= maxToolHardStop 时强制终止循环 ---
		hardStopTool := ""
		for tName, tCount := range toolFailureCounts {
			if tCount >= maxToolHardStop {
				hardStopTool = tName
				break
			}
		}
		if hardStopTool != "" {
			log.Warn("tool hard stop triggered",
				"tool", hardStopTool,
				"failCount", toolFailureCounts[hardStopTool],
				"iteration", iteration,
			)
			assistantTexts = append(assistantTexts, fmt.Sprintf(
				"⚠️ Tool %s failed %d times. Forced loop termination to avoid resource waste.",
				hardStopTool, toolFailureCounts[hardStopTool]))
			break
		}

		// --- 全局工具失败预算耗尽: 终止循环 ---
		if totalToolFailures >= maxTotalToolFailures {
			log.Warn("global tool failure budget exhausted, terminating loop",
				"totalFail", totalToolFailures,
				"iteration", iteration,
				"failMap", toolFailureCounts,
			)
			// 列出失败最多的工具供诊断
			worstTool, worstCount := "", 0
			for t, c := range toolFailureCounts {
				if c > worstCount {
					worstTool, worstCount = t, c
				}
			}
			warningMsg := fmt.Sprintf("⚠️ 工具调用累计失败 %d 次（主要: %s × %d），自动终止循环以避免浪费资源。\n"+
				"Tool calls failed %d times total (primary: %s × %d). Loop terminated to save resources. Please try a different approach.",
				totalToolFailures, worstTool, worstCount,
				totalToolFailures, worstTool, worstCount)
			// 保留已有的 assistantTexts（可能包含有用的部分回答），将警告追加在末尾
			assistantTexts = append(assistantTexts, warningMsg)
			break
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

						// 频道工具事件: start（重试路径）
						if params.OnToolEvent != nil {
							params.OnToolEvent(ToolEvent{
								Phase:    "start",
								ToolName: tc.Name,
								ToolID:   tc.ID,
								Args:     extractToolArgsSummary(tc.Name, retryArgs),
							})
						}

						secLvl := resolveSecurityLevel(r.Config)
						if params.SecurityLevelFunc != nil {
							secLvl = params.SecurityLevelFunc()
						}

						retryToolExecParams := r.buildToolExecParams(params, secLvl)
						retryStartTime := time.Now()
						output, toolErr := ExecuteToolCall(ctx, tc.Name, tc.Input, retryToolExecParams)
						isError := false
						if toolErr != nil {
							output = fmt.Sprintf("Error: %s", toolErr.Error())
							isError = true
							lastToolError = fmt.Sprintf("%s: %s", tc.Name, toolErr.Error())
						}

						// 频道工具事件: end（重试路径）
						if params.OnToolEvent != nil {
							params.OnToolEvent(ToolEvent{
								Phase:    "end",
								ToolName: tc.Name,
								ToolID:   tc.ID,
								Args:     extractToolArgsSummary(tc.Name, retryArgs),
								Result:   truncateRuneSafe(output, 200),
								IsError:  isError,
								Duration: time.Since(retryStartTime).Milliseconds(),
							})
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
			// 本地单用户场景: 统一使用 "default" 作为 userID，
			// 与 server_methods_memory.go 的查询路径保持一致 (Bug#11 P0 修复)。
			uid := "default"
			commitErr := r.UHMSBridge.CommitChatSession(
				context.Background(), uid, params.SessionID, messages)
			if commitErr != nil {
				slog.Warn("uhms commit session failed (non-fatal)", "error", commitErr)
			}
		}()
	}

	// 6.6 Transcript: 持久化当前轮次消息到 transcript 文件（供下次对话加载历史）
	// 在正常完成路径显式调用（messages 包含完整 user+assistant），标记已完成以避免 defer 重复执行。
	r.persistToTranscript(params, messages, log)
	transcriptPersisted = true

	// 7. 构建最终结果
	if len(mediaBlocks) > 0 {
		log.Info("attempt result: mediaBlocks collected", "count", len(mediaBlocks))
	}
	result := &AttemptResult{
		SessionIDUsed:  params.SessionID,
		AssistantTexts: assistantTexts,
		MediaBlocks:    mediaBlocks,
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
		"provider", params.Provider,
		"model", params.ModelID,
		"inputTokens", totalUsage.InputTokens,
		"outputTokens", totalUsage.OutputTokens,
	)

	return result, nil
}

// ---------- 内部方法 ----------

func (r *EmbeddedAttemptRunner) resolveAPIKey(params AttemptParams) (string, error) {
	if r.AuthStore == nil {
		// 优先从配置文件读取 API key（向导保存的值）
		if r.Config != nil && r.Config.Models != nil && r.Config.Models.Providers != nil {
			pc := r.Config.Models.Providers[strings.ToLower(params.Provider)]
			if pc == nil {
				pc = r.Config.Models.Providers[params.Provider]
			}
			if pc != nil {
				// OAuth 模式: 从 auth-profiles.json 读取 token（API Key 字段为空）
				if pc.Auth == types.ModelAuthOAuth {
					if token := loadOAuthTokenFromAuthProfiles(params.Provider); token != "" {
						return token, nil
					}
				}
				if pc.APIKey != "" {
					return pc.APIKey, nil
				}
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

// resolveAuthMode 从配置中获取 provider 的认证模式。
// 返回 "oauth" | "" (默认 API key)。
func (r *EmbeddedAttemptRunner) resolveAuthMode(provider string) string {
	if r.Config == nil || r.Config.Models == nil || r.Config.Models.Providers == nil {
		return ""
	}
	if pc := r.Config.Models.Providers[strings.ToLower(provider)]; pc != nil {
		return string(pc.Auth)
	}
	if pc := r.Config.Models.Providers[provider]; pc != nil {
		return string(pc.Auth)
	}
	return ""
}

// loadOAuthTokenFromAuthProfiles 从 auth-profiles.json 中读取 OAuth token。
// 当 AuthStore 不可用且 provider 使用 OAuth 认证时作为回退。
func loadOAuthTokenFromAuthProfiles(provider string) string {
	authProfilePath := goproviders_common.ResolveAuthStorePath("")
	data, err := os.ReadFile(authProfilePath)
	if err != nil {
		return ""
	}

	var store struct {
		Profiles map[string]map[string]interface{} `json:"profiles"`
	}
	if json.Unmarshal(data, &store) != nil || store.Profiles == nil {
		return ""
	}

	// 搜索匹配 provider 的 profile（格式: "provider:email" 或 "provider:default"）
	providerLower := strings.ToLower(provider)
	for profileID, creds := range store.Profiles {
		parts := strings.SplitN(profileID, ":", 2)
		if len(parts) < 1 {
			continue
		}
		profileProvider := strings.ToLower(parts[0])
		if profileProvider == providerLower || profileProvider == providerLower+"-portal" {
			if access, ok := creds["access"].(string); ok && access != "" {
				return access
			}
		}
	}

	return ""
}

func (r *EmbeddedAttemptRunner) buildSystemPrompt(params AttemptParams, sessionState prompt.SessionState, tier intentTier, toolNames []string, toolSummaries map[string]string) string {
	// 使用 prompt 包的 BuildAgentSystemPrompt 构建完整系统提示
	rt := prompt.DefaultRuntimeInfo()
	rt.Model = params.Provider + "/" + params.ModelID

	// 构建技能快照 — Boot 模式 vs 文件扫描模式
	skillsPrompt := ""
	isBootMode := r.UHMSBridge != nil && r.UHMSBridge.IsSkillsIndexed()

	if isBootMode {
		// Boot 模式: 技能已分级到 VFS，无需文件扫描。
		// prompt 仅放一行提示，LLM 通过 search_skills + lookup_skill 按需获取。
		r.skillsCache = nil // 不预加载
		skillsPrompt = "Skills are indexed in VFS. For standard operations (bash commands, file access, system status checks), use tools directly without searching skills. Only use `search_skills` when the task requires specialized domain knowledge or unfamiliar workflows, then `lookup_skill` to read full content."
		// 工具绑定即使在 Boot 模式也从文件加载（绑定是静态的，<5ms）
		bundledDir := skills.ResolveBundledSkillsDir("")
		bindingEntries := skills.LoadSkillEntries(params.WorkspaceDir, "", bundledDir, r.Config)
		r.toolBindings = skills.ResolveToolSkillBindings(bindingEntries)
		slog.Debug("buildSystemPrompt: boot mode (VFS skills)", "toolBindings", len(r.toolBindings))
	} else if params.WorkspaceDir != "" {
		// 文件扫描模式（向后兼容）
		bundledDir := skills.ResolveBundledSkillsDir("")
		entries := skills.LoadSkillEntries(params.WorkspaceDir, "", bundledDir, r.Config)
		snap := skills.BuildWorkspaceSkillSnapshot(skills.BuildSnapshotParams{
			WorkspaceDir: params.WorkspaceDir,
			BundledDir:   bundledDir,
			Config:       r.Config,
			Entries:      entries, // 传入预加载 entries，避免重复扫描
		})

		// 填充 skillsCache（lookup_skill 工具使用）
		r.skillsCache = make(map[string]string, len(snap.ResolvedSkills))
		for _, s := range snap.ResolvedSkills {
			if s.Content != "" {
				r.skillsCache[s.Name] = s.Content
			}
		}

		// 复用已加载的 entries 构建工具绑定
		r.toolBindings = skills.ResolveToolSkillBindings(entries)

		// 所有技能统一走按需加载: prompt 只放紧凑索引，LLM 通过 lookup_skill 获取完整内容
		if idx := skills.FormatSkillIndex(snap.ResolvedSkills); idx != "" {
			skillsPrompt = idx
		}
	}

	// Boot context brief: 注入上次工作摘要 (~200 tokens)
	var bootBrief string
	if r.UHMSBridge != nil {
		bootBrief = r.UHMSBridge.BuildContextBrief(context.Background())
	}

	// 使用传入的 PromptMode（子智能体传 "minimal" 跳过无关段落）
	promptMode := prompt.PromptModeFull
	if params.PromptMode != "" {
		promptMode = prompt.PromptMode(params.PromptMode)
	}

	bp := prompt.BuildParams{
		Mode:                    promptMode,
		WorkspaceDir:            params.WorkspaceDir,
		ExtraSystemPrompt:       params.ExtraSystemPrompt,
		SkillsPrompt:            skillsPrompt,
		ToolNames:               toolNames,
		ToolSummaries:           toolSummaries,
		RuntimeInfo:             &rt,
		ThinkLevel:              params.ThinkLevel,
		BootContextBrief:        bootBrief,
		SessionState:            sessionState,
		IntentGuidance:          intentGuidanceText(tier),
		PlanConfirmationEnabled: r.PlanConfirmation != nil,
	}

	// 注入 workspace context files (SOUL.md, MEMORY.md, TOOLS.md 等)
	// Layer 2: session.ResolveContextFiles 按优先级扫描已知文件
	//
	// 双路径扫描：
	//   1. params.WorkspaceDir（agent 管理的工作区，如 ~/.openacosmi/workspace/）
	//   2. 项目源码根目录（CWD 向上查找，如 /path/to/project/，SOUL.md 通常在此）
	// 去重：同名文件以先扫描到的为准（workspace 优先）。
	seen := make(map[string]bool)
	if params.WorkspaceDir != "" {
		for _, cf := range session.ResolveContextFiles(params.WorkspaceDir) {
			seen[cf.Path] = true
			bp.ContextFiles = append(bp.ContextFiles, prompt.ContextFile{
				Path:    cf.Path,
				Content: cf.Content,
			})
		}
	}
	// 项目根补充扫描（避免与 workspace 同目录时重复扫描）
	projectRoot := session.ResolveProjectRootContextDir()
	if projectRoot != "" && projectRoot != params.WorkspaceDir {
		for _, cf := range session.ResolveContextFiles(projectRoot) {
			if !seen[cf.Path] {
				seen[cf.Path] = true
				bp.ContextFiles = append(bp.ContextFiles, prompt.ContextFile{
					Path:    cf.Path,
					Content: cf.Content,
				})
			}
		}
	}

	// 子智能体会话过滤：移除 MEMORY.md / SOUL.md（防止 WARM_START 误判和上下文污染）
	if routing.IsSubagentSessionKey(params.SessionKey) && len(bp.ContextFiles) > 0 {
		filtered := bp.ContextFiles[:0]
		for _, cf := range bp.ContextFiles {
			base := filepath.Base(cf.Path)
			if base == "MEMORY.md" || base == "SOUL.md" {
				slog.Debug("buildSystemPrompt: subagent filtering out context file", "file", cf.Path)
				continue
			}
			filtered = append(filtered, cf)
		}
		bp.ContextFiles = filtered
	}

	// 诊断日志: 确认 prompt 各环节是否生效
	var ctxFileNames []string
	for _, cf := range bp.ContextFiles {
		ctxFileNames = append(ctxFileNames, cf.Path)
	}
	slog.Info("buildSystemPrompt: diagnostics",
		"sessionState", sessionState,
		"workspaceDir", params.WorkspaceDir,
		"projectRoot", projectRoot,
		"contextFiles", ctxFileNames,
		"skillsMode", func() string {
			if skillsPrompt == "" {
				return "none"
			}
			if strings.Contains(skillsPrompt, "VFS") {
				return "boot"
			}
			return "file-scan"
		}(),
		"hasBootBrief", bootBrief != "",
	)

	result := prompt.BuildAgentSystemPrompt(bp)
	slog.Info("buildSystemPrompt: total length", "chars", len(result), "sections", len(bp.ContextFiles))
	return result
}

func (r *EmbeddedAttemptRunner) buildToolDefinitions() []llmclient.ToolDef {
	tools := []llmclient.ToolDef{
		{
			Name:        "bash",
			Description: "Execute a bash command in the workspace. Use for system operations (memory/CPU/disk monitoring via top, vm_stat, df, etc.), process management, package operations, and any shell command. This is the primary tool for executing commands directly.",
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

	// 技能工具注册: Boot 模式 (search_skills + lookup_skill) 或文件扫描模式 (lookup_skill only)
	isBootMode := r.UHMSBridge != nil && r.UHMSBridge.IsSkillsIndexed()
	if isBootMode {
		// Boot 模式: search_skills 搜索 + lookup_skill 读取全文
		tools = append(tools, llmclient.ToolDef{
			Name:        "search_skills",
			Description: "Search for skills by keyword or topic. Returns matching skills with L0 summaries. Use lookup_skill to read full content.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search keywords or topic"},"top_k":{"type":"integer","description":"Max results (default 10)"}},"required":["query"]}`),
		})
		tools = append(tools, llmclient.ToolDef{
			Name:        "lookup_skill",
			Description: "Look up the full content of a skill by name. Use after search_skills to read the complete SKILL.md.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Skill name from search_skills results"}},"required":["name"]}`),
		})
	} else if len(r.skillsCache) > 0 {
		// 文件扫描模式: 仅 lookup_skill（索引在 system prompt 中）
		tools = append(tools, llmclient.ToolDef{
			Name:        "lookup_skill",
			Description: "Look up the full content of a skill by name. Use this when a skill from <available_skills> applies to the current task.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Skill name from <available_skills>"}},"required":["name"]}`),
		})
	}

	// web_search: 联网搜索工具
	if r.WebSearchProvider != nil {
		tools = append(tools, llmclient.ToolDef{
			Name:        "web_search",
			Description: "Search the web using a search engine. Returns relevant results with titles, URLs, and snippets. Use this when you need real-time information from the internet.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query string"},"count":{"type":"integer","description":"Number of results (1-10, default 5)"}},"required":["query"]}`),
		})
	}

	// browser: 浏览器自动化工具（ARIA ref + CSS selector 双模式，比 Argus 屏幕坐标更精准高效）
	if r.BrowserController != nil {
		tools = append(tools, llmclient.ToolDef{
			Name: "browser",
			Description: `Control a browser with structured page understanding. Recommended workflow:
1. Use "observe" to get ARIA accessibility tree + screenshot (understand page structure)
2. Use "click_ref"/"fill_ref" with element refs (e.g. "e1") from observe results
3. Fall back to CSS selectors with "click"/"type" only when refs are unavailable.
For complex multi-step tasks, use "ai_browse" with a goal description to auto-execute.
Also supports: navigate, screenshot, evaluate JS, wait_for, go_back/forward, get_url, get_content.`,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["navigate","get_content","click","type","screenshot","evaluate","wait_for","go_back","go_forward","get_url","observe","click_ref","fill_ref","ai_browse"],"description":"Browser action. Use 'observe' first, then 'click_ref'/'fill_ref' with refs. Use 'ai_browse' for complex multi-step goals."},"url":{"type":"string","description":"URL for navigate action"},"selector":{"type":"string","description":"CSS selector for click/type/wait_for actions"},"text":{"type":"string","description":"Text for type/fill_ref actions"},"script":{"type":"string","description":"JavaScript for evaluate action"},"ref":{"type":"string","description":"Element ref from observe results (e.g. 'e1') for click_ref/fill_ref actions"},"goal":{"type":"string","description":"Goal description for ai_browse action (e.g. 'Search for MacBook Pro and get the price')"}},"required":["action"]}`),
		})
	}

	// send_media: 文件/媒体发送工具
	if r.MediaSender != nil {
		tools = append(tools, llmclient.ToolDef{
			Name: "send_media",
			Description: "Send a file or media to the current conversation channel. Supports images, documents, audio, and video.\n" +
				"IMPORTANT: Do NOT provide 'target' — it automatically defaults to the current conversation channel. " +
				"Only set 'target' when explicitly asked to send to a DIFFERENT channel.\n" +
				"Use 'file_path' with an ABSOLUTE path (e.g. '/tmp/screenshot.png'). " +
				"MIME type is auto-detected from the file extension.\n" +
				"Only use 'media_base64' if data is already in base64 form. For screenshots, save to a file first then use file_path.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string","description":"ABSOLUTE path to a local file (e.g. '/tmp/screenshot.png'). Preferred method. MIME type is auto-detected from extension."},"message":{"type":"string","description":"Optional text message to accompany the file."},"target":{"type":"string","description":"DO NOT SET unless sending to a different channel. Defaults to current conversation channel."},"media_base64":{"type":"string","description":"Base64-encoded media data. Only use when data is already in base64 form; otherwise save to file and use file_path."},"mime_type":{"type":"string","description":"MIME type. Auto-detected from file extension when using file_path, only needed for media_base64."}}}`),
		})
	}

	// 追加 Argus 视觉工具（前缀 argus_ 以区分）
	if r.ArgusBridge != nil {
		for _, t := range r.ArgusBridge.AgentTools() {
			tools = append(tools, llmclient.ToolDef{
				Name:        "argus_" + t.Name,
				Description: "[灵瞳] " + t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	// spawn_coder_agent: 委托合约驱动的编程子智能体生成工具
	tools = append(tools, SpawnCoderAgentToolDef())

	// spawn_argus_agent: 委托合约驱动的灵瞳视觉子智能体生成工具
	// Phase 5: 灵瞳完全子智能体化 — 仅在 ArgusBridge 可用时注册
	if r.ArgusBridge != nil {
		tools = append(tools, SpawnArgusAgentToolDef())
	}

	// spawn_media_agent: 委托合约驱动的媒体运营子智能体生成工具
	// 仅在 MediaSubsystem 可用时注册（主智能体只见入口，子工具在 RunAttempt 按 AgentType 注入）
	if r.MediaSubsystem != nil {
		tools = append(tools, SpawnMediaAgentToolDef())
	}

	// request_help: 异步求助工具（Phase 4 — 子智能体向主智能体求助）
	// 注: 实际注入在 RunAttempt 中根据 params.AgentChannel 动态添加

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

	// Bug#11: memory_search / memory_get 记忆工具注入（仅在 UHMS Bridge 可用时）
	if r.UHMSBridge != nil {
		tools = append(tools,
			llmclient.ToolDef{
				Name:        "memory_search",
				Description: "Search long-term memories by keyword or topic. Returns matching memory entries with relevance scores. Use this to recall past conversations, user preferences, or learned facts.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query (keywords or topic)"},"limit":{"type":"integer","description":"Max results (default 5)"}},"required":["query"]}`),
			},
			llmclient.ToolDef{
				Name:        "memory_get",
				Description: "Get a specific memory entry by ID. Use after memory_search to read full details of a memory.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Memory ID from memory_search results"}},"required":["id"]}`),
			},
		)
	}

	// 注入工具绑定的技能描述
	if len(r.toolBindings) > 0 {
		for i := range tools {
			if guidance, ok := r.toolBindings[tools[i].Name]; ok {
				tools[i].Description += " [Skill: " + guidance + "]"
			}
		}
	}

	return tools
}

// ---------- 意图分级工具注入 ----------
// 六级意图分类系统已迁移到 intent_router.go
// 类型: intentTier (greeting/question/task_light/task_write/task_delete/task_multimodal)
// 函数: classifyIntent(), filterToolsByIntent(), trimHistoryByIntent(), intentGuidanceText()

func (r *EmbeddedAttemptRunner) resolveBaseURL(provider string) string {
	return models.ResolveProviderBaseURL(provider, r.Config)
}

// buildToolExecParams 构造 ToolExecParams 并注入合约约束（如有）。
// 集中处理 SecurityLevel 封顶 + ApplyConstraints 收窄，消除主路径/重试路径的代码重复。
func (r *EmbeddedAttemptRunner) buildToolExecParams(params AttemptParams, secLvl string) ToolExecParams {
	// Phase 3: 合约约束注入前，先封顶安全级别
	if params.DelegationContract != nil {
		maxLevel := deriveMaxSecurityLevel(params.DelegationContract)
		if securityLevelRank(secLvl) > securityLevelRank(maxLevel) {
			secLvl = maxLevel
		}
	}

	tep := ToolExecParams{
		WorkspaceDir:           params.WorkspaceDir,
		SessionID:              params.SessionID,
		RunID:                  params.RunID,
		SessionKey:             params.SessionKey,
		TimeoutMs:              params.TimeoutMs,
		AllowWrite:             secLvl == "full" || secLvl == "sandboxed" || secLvl == "allowlist",
		AllowExec:              secLvl == "full" || secLvl == "sandboxed" || secLvl == "allowlist",
		AllowNetwork:           secLvl == "full" || secLvl == "sandboxed", // L2+ 全网络，L1 由 Rust 端 NetworkPolicy::Restricted 处理
		SandboxMode:            secLvl == "sandboxed" || secLvl == "allowlist",
		Rules:                  resolveCommandRules(),
		SecurityLevel:          secLvl,
		OnPermissionDenied:     params.OnPermissionDenied,
		ArgusBridge:            r.ArgusBridge,
		CoderConfirmation:      r.CoderConfirmation,
		RemoteMCPBridge:        r.RemoteMCPBridge,
		NativeSandbox:          r.NativeSandbox,
		SkillsCache:            r.skillsCache,
		UHMSBridge:             r.UHMSBridge,
		SpawnSubagent:          r.SpawnSubagent,
		WebSearchProvider:      r.WebSearchProvider,
		ArgusApprovalMode:      r.ArgusApprovalMode,
		BrowserEvaluateEnabled: r.BrowserEvaluateEnabled,
		BrowserController:      r.BrowserController,
		ContractStore:          r.ContractStore,
		MediaSender:            r.MediaSender,
		QualityReviewFn:        r.QualityReviewFn,
		ResultApprovalMgr:      r.ResultApprovalMgr,
		OnProgress:             params.OnProgress,
		AgentChannel:           params.AgentChannel, // Phase 4: 从 AttemptParams 获取（每次调用独立）
		MediaSubsystem:         r.MediaSubsystem,    // 媒体工具 dispatch
	}

	// Phase 3.4: 临时挂载请求注入（从 escalation grant）
	if params.MountRequestsFunc != nil {
		tep.MountRequests = params.MountRequestsFunc()
	}

	// Phase 3: 合约约束注入 — 只收窄不扩展
	if params.DelegationContract != nil {
		params.DelegationContract.ApplyConstraints(&tep)
	}

	return tep
}

// ---------- 工具失败循环检测 ----------

// isToolSoftError 检测工具输出是否为"软错误"（工具返回 nil error 但输出内容表示失败）。
// 几乎所有工具将错误信息作为 string 返回而非 error（`return fmt.Sprintf("[prefix] ...", err), nil`），
// 需要此函数识别这些软错误。
func isToolSoftError(toolName, output string) bool {
	// 快速排除：成功结果通常以 JSON、__MULTIMODAL__、或普通文本开头
	if output == "" || output[0] == '{' || output[0] == '"' || strings.HasPrefix(output, "__MULTIMODAL__") {
		return false
	}

	switch toolName {
	case "bash":
		// bash 前置拦截: "[bash] Resource budget exhausted" 或 "[Command blocked..."
		if strings.HasPrefix(output, "[bash]") || strings.HasPrefix(output, "[Command blocked") ||
			strings.HasPrefix(output, "[Command denied") || strings.HasPrefix(output, "[Write operation approval error") {
			return true
		}
		// Bug#11 修复: bash 后执行失败 — 非零 exit code
		// bash 输出格式: "stdout\n[exit code: N]" 或 "stdout\n[sandbox exit code: N]"
		trimmed := strings.TrimSpace(output)
		if strings.Contains(output, "\n[exit code:") && !strings.HasSuffix(trimmed, "[exit code: 0]") {
			return true
		}
		if strings.Contains(output, "\n[sandbox exit code:") && !strings.HasSuffix(trimmed, "[sandbox exit code: 0]") {
			return true
		}
		return false
	case "send_media":
		return strings.HasPrefix(output, "[send_media]")
	case "browser":
		return strings.HasPrefix(output, "[Browser ") || strings.HasPrefix(output, "[Unknown browser action")
	default:
		// 通用模式: "[Prefix error/not found/...]" 或 "Error: ..."
		if strings.HasPrefix(output, "Error:") || strings.HasPrefix(output, "[error]") {
			return true
		}
		// 工具名前缀匹配: "[Tool ..." / "[Argus ..." / "[Remote ..." / "[Skill ..." / "[No ..."
		if len(output) > 2 && output[0] == '[' {
			// 检查是否包含错误相关关键词（避免把 "[{...}]" JSON 数组误判）
			bracket := strings.IndexByte(output, ']')
			if bracket > 0 && bracket < 120 {
				prefix := strings.ToLower(output[1:bracket])
				return strings.Contains(prefix, "error") || strings.Contains(prefix, "not found") ||
					strings.Contains(prefix, "denied") || strings.Contains(prefix, "blocked") ||
					strings.Contains(prefix, "not available") || strings.Contains(prefix, "not yet implemented")
			}
		}
		return false
	}
}

// toolFailureGuidance 根据工具名称和失败次数生成可操作的修复指导。
// 注入到工具返回结果中，帮助 LLM 跳出参数猜测循环。
func toolFailureGuidance(toolName string, failCount int) string {
	switch toolName {
	case "send_media":
		screenshotCmd := screenshotCommand()
		return fmt.Sprintf("⚠️ TOOL FAILURE LOOP DETECTED (%d failures). STOP retrying send_media with different parameters.\n"+
			"REMEDIATION:\n"+
			"1. Do NOT provide 'target' — it defaults to the current conversation channel automatically.\n"+
			"2. Use 'file_path' with an ABSOLUTE path (e.g. '/tmp/screenshot.png').\n"+
			"3. Do NOT use 'media_base64' — save data to a file first, then use 'file_path'.\n"+
			"4. If sending a screenshot: run '%s' via bash, then call send_media with file_path='/tmp/screenshot.png'.\n"+
			"5. If the file does not exist at the path, check the actual path with 'ls' first.\n"+
			"If you cannot send the media after these steps, respond with text instead of retrying.", failCount, screenshotCmd)
	case "browser":
		return fmt.Sprintf("⚠️ TOOL FAILURE LOOP DETECTED (%d failures). STOP retrying browser with the same approach.\n"+
			"REMEDIATION:\n"+
			"1. Use 'observe' first to refresh ARIA refs before click_ref/fill_ref.\n"+
			"2. If refs are stale, re-observe. If the page has changed, re-navigate.\n"+
			"3. For complex multi-step goals, use 'ai_browse' instead of manual step-by-step.\n"+
			"If the page is unresponsive, respond with text to describe the issue.", failCount)
	default:
		return fmt.Sprintf("⚠️ Tool %s has failed %d times. Consider trying a different approach or respond with text to the user.", toolName, failCount)
	}
}

// screenshotCommand 返回当前平台的截图命令示例。
func screenshotCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "screencapture -x /tmp/screenshot.png"
	case "linux":
		return "import -window root /tmp/screenshot.png"
	default:
		return "take a screenshot and save to /tmp/screenshot.png"
	}
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

// formatDirectiveAsContext 将主智能体发来的 AgentMessage 格式化为上下文文本，
// 注入到子智能体的 messages 中让 LLM 看到。
func formatDirectiveAsContext(msg *AgentMessage) string {
	var b strings.Builder
	switch msg.Type {
	case MsgHelpResponse:
		b.WriteString("[Parent Agent Help Response]\n")
		if msg.ReplyTo != "" {
			b.WriteString(fmt.Sprintf("(reply to: %s)\n", msg.ReplyTo))
		}
		b.WriteString(msg.Content)
	case MsgDirective:
		b.WriteString("[Parent Agent Directive]\n")
		b.WriteString(msg.Content)
	default:
		b.WriteString(fmt.Sprintf("[Parent Agent Message (%s)]\n", msg.Type))
		b.WriteString(msg.Content)
	}
	if msg.Context != "" {
		b.WriteString(fmt.Sprintf("\nContext: %s", msg.Context))
	}
	return b.String()
}

// truncateRuneSafe rune-safe 截断，保证中文等多字节字符不被截断。
func truncateRuneSafe(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// extractToolArgsSummary 从工具参数中提取摘要字符串，用于频道广播。
// 不同工具提取不同的关键字段作为摘要。
func extractToolArgsSummary(toolName string, args map[string]interface{}) string {
	var summary string
	switch toolName {
	case "bash":
		summary, _ = args["command"].(string)
	case "read", "read_file", "edit", "write", "write_file":
		summary, _ = args["file_path"].(string)
	case "glob":
		summary, _ = args["pattern"].(string)
	case "grep", "search":
		summary, _ = args["pattern"].(string)
	case "web_search":
		summary, _ = args["query"].(string)
	case "web_fetch":
		summary, _ = args["url"].(string)
	case "spawn_coder_agent":
		summary, _ = args["task_brief"].(string)
	case "memory_search":
		summary, _ = args["query"].(string)
	case "browser":
		if action, ok := args["action"].(string); ok {
			summary = action
			if url, ok := args["url"].(string); ok && url != "" {
				summary += " " + url
			}
		}
	default:
		// 未知工具：不提取参数摘要
	}
	return truncateRuneSafe(summary, 100)
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
// 安全级别映射: "full"/"sandboxed"/"allowlist" → true, "deny" → false
func resolveAllowWrite(cfg *types.OpenAcosmiConfig) bool {
	level := resolveSecurityLevel(cfg)
	return level == "full" || level == "sandboxed" || level == "allowlist"
}

// resolveAllowExec 从配置解析是否允许执行命令。
// 安全级别映射: "full"/"sandboxed"/"allowlist" → true, "deny" → false
func resolveAllowExec(cfg *types.OpenAcosmiConfig) bool {
	level := resolveSecurityLevel(cfg)
	return level == "full" || level == "sandboxed" || level == "allowlist"
}

// resolveSandboxMode 判断是否启用 Docker 沙箱模式。
// L1 (allowlist) 和 L2 (sandboxed) 启用沙箱，L3 (full) 直接在宿主机执行。
func resolveSandboxMode(cfg *types.OpenAcosmiConfig) bool {
	level := resolveSecurityLevel(cfg)
	return level == "sandboxed" || level == "allowlist"
}

// resolveAllowNetwork 从配置解析是否允许网络访问。
// L2 (sandboxed) 和 L3 (full) 允许全网络，L1 (allowlist) 由 Rust 端处理受限网络。
func resolveAllowNetwork(cfg *types.OpenAcosmiConfig) bool {
	level := resolveSecurityLevel(cfg)
	return level == "full" || level == "sandboxed"
}

func normalizeSecurityLevelValue(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "full":
		return "full"
	case "sandboxed":
		return "sandboxed"
	case "allowlist", "sandbox":
		return "allowlist"
	case "deny", "off", "":
		return "deny"
	default:
		return "deny"
	}
}

func resolveEffectiveSecurityLevel(params AttemptParams, cfg *types.OpenAcosmiConfig) string {
	if params.SecurityLevelFunc != nil {
		if level := normalizeSecurityLevelValue(params.SecurityLevelFunc()); level != "" {
			return level
		}
	}
	return resolveSecurityLevel(cfg)
}

// resolveSecurityLevel 从 OpenAcosmiConfig 中提取 tools.exec.security 字段值。
// 返回规范化的安全级别字符串: "deny", "allowlist", "sandboxed", "full"。
// 兼容 "sandbox" 作为 "allowlist" 的别名，"off" 作为 "deny" 的别名。
func resolveSecurityLevel(cfg *types.OpenAcosmiConfig) string {
	if cfg == nil || cfg.Tools == nil || cfg.Tools.Exec == nil {
		return "deny"
	}
	return normalizeSecurityLevelValue(cfg.Tools.Exec.Security)
}

// (Phase 2A: resolveCoderTimeoutSeconds 已删除 — Coder MCP 模式不再使用)

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

// ---------- Transcript 历史加载 / 持久化 ----------

// hasAnyAssistantMessage 检查消息历史中是否包含 assistant 回复。
// 用于 Cold Start 检测: chat.send 预写入用户消息后，transcript 非空但仍应判定为冷启动。
func hasAnyAssistantMessage(messages []llmclient.ChatMessage) bool {
	for _, m := range messages {
		if m.Role == "assistant" {
			return true
		}
	}
	return false
}

// maxHistoryMessages 从 transcript 加载的最大历史消息数。
// 保留最近 N 条消息（user+assistant 交替），防止 context window 溢出。
const maxHistoryMessages = 20

// loadPriorMessages 从 transcript JSONL 文件加载先前对话历史。
// 返回转换后的 ChatMessage 列表（可能为空）。
func (r *EmbeddedAttemptRunner) loadPriorMessages(params AttemptParams, log *slog.Logger) []llmclient.ChatMessage {
	if params.SessionFile == "" && params.SessionID == "" {
		return nil
	}

	mgr := session.NewSessionManager("")
	rawMsgs, err := mgr.LoadSessionMessages(params.SessionID, params.SessionFile)
	if err != nil {
		log.Debug("transcript load failed (non-fatal, starting fresh)", "error", err)
		return nil
	}
	if len(rawMsgs) == 0 {
		return nil
	}

	// 验证 + 截断
	rawMsgs = session.ValidateHistoryMessages(rawMsgs)
	if len(rawMsgs) > maxHistoryMessages {
		rawMsgs = rawMsgs[len(rawMsgs)-maxHistoryMessages:]
	}

	// 转换 map[string]interface{} → llmclient.ChatMessage
	var messages []llmclient.ChatMessage
	for _, raw := range rawMsgs {
		msg := transcriptEntryToChatMessage(raw)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	if len(messages) > 0 {
		log.Info("transcript history loaded", "messageCount", len(messages))
	}
	return messages
}

// transcriptEntryToChatMessage 将 transcript 条目转换为 ChatMessage。
// 只转换 user 和 assistant 角色的纯文本消息（忽略 tool_use/tool_result 等复杂块）。
func transcriptEntryToChatMessage(raw map[string]interface{}) *llmclient.ChatMessage {
	role, _ := raw["role"].(string)
	if role != "user" && role != "assistant" {
		return nil
	}

	// 提取文本内容
	var text string
	switch c := raw["content"].(type) {
	case string:
		text = c
	case []interface{}:
		// content block 数组 → 拼接文本
		var parts []string
		for _, block := range c {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			if t, ok := blockMap["text"].(string); ok && t != "" {
				parts = append(parts, t)
			}
		}
		text = strings.Join(parts, "\n")
	}

	if strings.TrimSpace(text) == "" {
		return nil
	}

	msg := llmclient.TextMessage(role, text)
	return &msg
}

// persistToTranscript 将当前轮次的 user 消息和 assistant 回复写入 transcript 文件。
//
// 设计要点: 用户消息直接从 params.Prompt 提取，而非反向扫描 messages 数组。
// 原因: 工具循环中，tool_result 也以 role:"user" 加入 messages，
// 反向扫描会在 tool_result 处误停，导致原始用户提问永远不被持久化。
func (r *EmbeddedAttemptRunner) persistToTranscript(params AttemptParams, messages []llmclient.ChatMessage, log *slog.Logger) {
	if params.SessionFile == "" && params.SessionID == "" {
		return
	}

	mgr := session.NewSessionManager("")

	// 确保 transcript 文件存在
	if _, err := mgr.EnsureSessionFile(params.SessionID, params.SessionFile); err != nil {
		log.Debug("transcript ensure file failed (non-fatal)", "error", err)
		return
	}

	savedCount := 0

	// 1. 保存用户消息: 直接使用 params.Prompt，确保原始用户提问不被 tool_result 遮蔽。
	// 去重: model fallback 场景下，前一次 RunAttempt 的 defer 可能已写入同一 user 消息。
	if params.Prompt != "" {
		shouldWriteUser := true
		if existing, _ := mgr.LoadSessionMessages(params.SessionID, params.SessionFile); len(existing) > 0 {
			last := existing[len(existing)-1]
			if role, _ := last["role"].(string); role == "user" {
				if content, ok := last["content"].([]interface{}); ok && len(content) > 0 {
					if block, ok := content[0].(map[string]interface{}); ok {
						if text, _ := block["text"].(string); text == params.Prompt {
							shouldWriteUser = false // 已有相同 user 消息，跳过
						}
					}
				}
			}
		}
		if shouldWriteUser {
			userEntry := session.TranscriptEntry{
				Role:      "user",
				Content:   []session.ContentBlock{{Type: "text", Text: params.Prompt}},
				Timestamp: time.Now().UnixMilli(),
			}
			if err := mgr.AppendMessage(params.SessionID, params.SessionFile, userEntry); err != nil {
				log.Warn("transcript append user failed (non-fatal)", "error", err)
			} else {
				savedCount++
			}
		}
	}

	// 2. 保存最后一个有效内容的 assistant 消息（跳过纯 tool_call assistant 消息）
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		var blocks []session.ContentBlock
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					blocks = append(blocks, session.ContentBlock{
						Type: "text",
						Text: block.Text,
					})
				}
			case "image":
				if block.Source != nil && block.Source.Data != "" {
					blocks = append(blocks, session.ContentBlock{
						Type: "image",
						Source: &session.ImageSource{
							Type:      block.Source.Type,
							MediaType: block.Source.MediaType,
							Data:      block.Source.Data,
						},
					})
				}
			}
		}
		if len(blocks) == 0 {
			continue // 跳过纯 tool_call 的 assistant 消息（无文本/图片）
		}
		asstEntry := session.TranscriptEntry{
			Role:      "assistant",
			Content:   blocks,
			Timestamp: time.Now().UnixMilli(),
		}
		if err := mgr.AppendMessage(params.SessionID, params.SessionFile, asstEntry); err != nil {
			log.Warn("transcript append assistant failed (non-fatal)", "error", err)
		} else {
			savedCount++
		}
		break // 只保存最后一个有效 assistant 消息
	}

	log.Debug("transcript persisted", "savedCount", savedCount)
}
