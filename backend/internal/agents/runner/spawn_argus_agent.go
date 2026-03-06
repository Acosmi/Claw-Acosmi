package runner

// ============================================================================
// spawn_argus_agent — 委托合约驱动的灵瞳视觉子智能体生成工具
//
// 将灵瞳从 MCP 直接工具模式升级为与 Open Coder 同级的完整子智能体，
// 拥有独立 LLM session、DelegationContract、ThoughtResult。
//
// Phase 5: 三级指挥体系 — 灵瞳完全子智能体化
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// ---------- 工具输入 ----------

// spawnArgusAgentInput spawn_argus_agent 工具输入 JSON schema。
type spawnArgusAgentInput struct {
	TaskBrief      string          `json:"task_brief"`
	Scope          []ScopeEntry    `json:"scope,omitempty"`
	Constraints    json.RawMessage `json:"constraints,omitempty"`
	ParentContract string          `json:"parent_contract,omitempty"`
	TimeoutMs      *uint32         `json:"timeout_ms,omitempty"`
}

// ---------- 工具定义 ----------

// SpawnArgusAgentToolDef 返回 spawn_argus_agent 的 LLM 工具定义。
func SpawnArgusAgentToolDef() llmclient.ToolDef {
	return llmclient.ToolDef{
		Name: "spawn_argus_agent",
		Description: "Spawn a 灵瞳 (Argus) visual sub-agent with a delegation contract. " +
			"The sub-agent runs as an independent LLM session with access to visual perception and interaction tools " +
			"(screen capture, click, type, scroll, etc.). Use this for tasks that require visual understanding or " +
			"desktop/browser interaction via screen coordinates.",
		InputSchema: json.RawMessage(`{
	"type": "object",
	"properties": {
		"task_brief": {
			"type": "string",
			"description": "Human-readable visual task description (≤500 chars). Be specific about what the sub-agent should observe or interact with."
		},
		"scope": {
			"type": "array",
			"description": "Allowed visual scope. Use 'screen://' paths for screen regions. Optional — defaults to full screen read and write access.",
			"items": {
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Scope path. 'screen://' for screen, or file path for screenshots." },
					"permissions": {
						"type": "array",
						"items": { "type": "string", "enum": ["read", "write", "execute"] }
					}
				},
				"required": ["path", "permissions"]
			}
		},
		"constraints": {
			"type": "object",
			"description": "Execution constraints for the visual sub-agent.",
			"properties": {
				"no_network": { "type": "boolean", "description": "Deny network access (blocks open_url)" },
				"no_spawn": { "type": "boolean", "description": "Deny process spawning (blocks run_shell)" }
			}
		},
		"parent_contract": {
			"type": "string",
			"description": "Parent contract ID for resuming a previously suspended visual task (optional)."
		},
		"timeout_ms": {
			"type": "integer",
			"description": "Sub-agent timeout in milliseconds (default: 90000, visual tasks need more time)."
		}
	},
	"required": ["task_brief"]
}`),
	}
}

// ---------- 工具执行 ----------

// executeSpawnArgusAgent 处理 spawn_argus_agent 工具调用。
// 创建委托合约 → 构建灵瞳系统提示词 → 通过 SpawnSubagent 回调启动子 session。
func executeSpawnArgusAgent(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	// 1. 解析输入
	var input spawnArgusAgentInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Sprintf("[spawn_argus_agent] Invalid input: %s", err), nil
	}

	// 2. 默认值处理
	if len(input.Scope) == 0 {
		// 灵瞳默认 scope: 全屏读写（视觉操作需要）
		input.Scope = []ScopeEntry{
			{Path: "screen://", Permissions: []ScopePermission{PermRead, PermWrite}},
		}
	}

	// 3. 解析 constraints
	var constraints ContractConstraints
	if len(input.Constraints) > 0 {
		if err := json.Unmarshal(input.Constraints, &constraints); err != nil {
			return fmt.Sprintf("[spawn_argus_agent] Invalid constraints: %s", err), nil
		}
	}

	// 4. 创建委托合约
	issuedBy := params.SessionID
	if issuedBy == "" {
		issuedBy = "main-agent"
	}
	contract, err := NewDelegationContract(issuedBy, input.TaskBrief, "", input.Scope, constraints)
	if err != nil {
		return fmt.Sprintf("[spawn_argus_agent] Contract validation failed: %s", err), nil
	}

	// 默认超时: 90s（视觉任务需要更多时间进行截屏和交互）
	if input.TimeoutMs != nil && *input.TimeoutMs > 0 {
		contract.TimeoutMs = *input.TimeoutMs
	} else if contract.TimeoutMs == 0 || contract.TimeoutMs == 60000 {
		contract.TimeoutMs = 90000
	}

	// 父合约引用（断点恢复）
	if input.ParentContract != "" {
		contract.ParentContract = input.ParentContract
	}

	// Phase 6: 加载父合约恢复上下文
	var resumeContext string
	var iterationIndex int
	if input.ParentContract != "" && params.ContractStore != nil {
		_, parentThought, loadErr := params.ContractStore.LoadContract(input.ParentContract)
		if loadErr != nil {
			slog.Warn("spawn_argus_agent: failed to load parent contract",
				"parentContract", input.ParentContract, "error", loadErr)
		} else if parentThought != nil {
			resumeContext = parentThought.ResumeHint
			iterationIndex = int(parentThought.IterationCount) + 1
		}
	}

	// 迭代上限
	if iterationIndex > 3 {
		return fmt.Sprintf("[spawn_argus_agent] Negotiation round limit exceeded (%d > 3). "+
			"Please handle this task directly or escalate to the user.", iterationIndex), nil
	}

	slog.Info("spawn_argus_agent: contract created",
		"contractID", contract.ContractID,
		"taskBrief", contract.TaskBrief,
		"scopeCount", len(contract.Scope),
	)

	// 5. 单调衰减校验
	parentCaps := CapabilitySetFromToolExecParams(&params)
	contractCaps := CapabilitySetFromContract(contract)
	if err := parentCaps.ValidateMonotonicDecay(contractCaps); err != nil {
		return fmt.Sprintf("[spawn_argus_agent] Permission monotonic decay violation: %s", err), nil
	}

	// 6. 构建灵瞳专用系统提示词
	systemPrompt := BuildArgusSubagentSystemPrompt(ArgusSubagentPromptParams{
		Task:                input.TaskBrief,
		Contract:            contract,
		RequesterSessionKey: params.SessionKey,
		ResumeContext:       resumeContext,
		IterationIndex:      iterationIndex,
	})

	// 7. 检查 SpawnSubagent 回调
	if params.SpawnSubagent == nil {
		contractJSON, _ := json.MarshalIndent(contract, "", "  ")
		return fmt.Sprintf("[spawn_argus_agent] Contract created but spawn callback not configured.\n\nContract:\n%s", contractJSON), nil
	}

	// 8. 启动灵瞳子智能体 session
	contract.Status = ContractActive

	// 绑定活动合约到审批路由器（灵瞳也需要 CoderConfirmation 审批）
	if params.CoderConfirmation != nil {
		params.CoderConfirmation.SetActiveContract(contract)
		defer params.CoderConfirmation.ClearActiveContract()
	}

	outcome, err := params.SpawnSubagent(ctx, SpawnSubagentParams{
		Contract:     contract,
		Task:         input.TaskBrief,
		SystemPrompt: systemPrompt,
		TimeoutMs:    int64(contract.TimeoutMs),
		Label:        fmt.Sprintf("argus-%s", contract.ContractID[:8]),
		Channel:      params.AgentChannel,
		AgentType:    "argus", // Phase 5: 标识灵瞳子智能体
	})
	if err != nil {
		contract.Status = ContractFailed
		return fmt.Sprintf("[spawn_argus_agent] Sub-agent spawn failed: %s", err), nil
	}

	// 9. 质量审核（Phase 2）
	var qualityReviewSummary string
	if outcome != nil && outcome.ThoughtResult != nil && outcome.ThoughtResult.Status == ThoughtCompleted {
		reviewParams := QualityReviewParams{
			Contract:  contract,
			Outcome:   outcome,
			TaskBrief: input.TaskBrief,
		}
		reviewResult := ReviewSubagentResult(reviewParams, params.QualityReviewFn)
		if reviewResult != nil && !reviewResult.Approved {
			contract.Status = ContractFailed
			slog.Info("spawn_argus_agent: quality review failed",
				"contractID", contract.ContractID,
				"issues", reviewResult.Issues,
			)
			return FormatReviewFailedResult(contract, outcome, reviewResult), nil
		}
		if reviewResult != nil && reviewResult.ReviewSummary != "" {
			qualityReviewSummary = reviewResult.ReviewSummary
		}
	}

	// 10. 最终交付门控（Phase 3）
	if params.ResultApprovalMgr != nil && outcome != nil && outcome.ThoughtResult != nil &&
		outcome.ThoughtResult.Status == ThoughtCompleted {

		tr := outcome.ThoughtResult
		approvalReq := ResultApprovalRequest{
			OriginalTask:  input.TaskBrief,
			ContractID:    contract.ContractID,
			Result:        truncate(tr.Result, 500),
			Artifacts:     tr.Artifacts,
			ReviewSummary: qualityReviewSummary,
		}

		approvalCtx, approvalCancel := context.WithTimeout(context.Background(), params.ResultApprovalMgr.Timeout())
		defer approvalCancel()

		approvalDecision, approvalErr := params.ResultApprovalMgr.RequestResultApproval(approvalCtx, approvalReq)
		if approvalErr != nil {
			slog.Warn("spawn_argus_agent: result approval error, proceeding with result",
				"contractID", contract.ContractID, "error", approvalErr)
		} else if approvalDecision.Action == "reject" {
			slog.Info("spawn_argus_agent: result rejected by user",
				"contractID", contract.ContractID, "feedback", approvalDecision.Feedback)
			return fmt.Sprintf("[Argus Agent Result - User Rejected]\nContract: %s\n\n"+
				"The user rejected the visual sub-agent's result.\nFeedback: %s\n\n"+
				"ACTION: Review the user's feedback and either:\n"+
				"  1. Re-delegate with adjusted parameters\n"+
				"  2. Handle the task differently\n"+
				"  3. Ask the user for clarification\n",
				contract.ContractID, approvalDecision.Feedback), nil
		}
	}

	// 11. 合约状态转换到终态
	contract.Status = ContractCompleted

	// 12. 格式化返回结果
	return formatArgusSpawnResult(contract, outcome), nil
}

// formatArgusSpawnResult 格式化灵瞳子智能体执行结果。
func formatArgusSpawnResult(contract *DelegationContract, outcome *SubagentRunOutcome) string {
	if outcome == nil {
		return fmt.Sprintf("[spawn_argus_agent] Contract %s: no outcome returned", contract.ContractID)
	}

	if tr := outcome.ThoughtResult; tr != nil {
		if tr.Status == ThoughtNeedsAuth {
			return formatNeedsAuthResult(contract, tr)
		}

		result := fmt.Sprintf("[Argus Agent Result]\nContract: %s\nStatus: %s\n",
			contract.ContractID, tr.Status)
		if tr.Result != "" {
			result += fmt.Sprintf("\n%s\n", tr.Result)
		}
		if tr.ReasoningSummary != "" {
			result += fmt.Sprintf("\nReasoning: %s\n", tr.ReasoningSummary)
		}
		if tr.ResumeHint != "" {
			result += fmt.Sprintf("\nResume hint: %s\n", tr.ResumeHint)
		}
		if len(tr.ScopeViolations) > 0 {
			result += fmt.Sprintf("\nScope violations: %v\n", tr.ScopeViolations)
		}
		return result
	}

	result := fmt.Sprintf("[Argus Agent Result]\nContract: %s\nStatus: %s\n",
		contract.ContractID, outcome.Status)
	if outcome.Error != "" {
		result += fmt.Sprintf("Error: %s\n", outcome.Error)
	}
	return result
}
