package runner

// ============================================================================
// spawn_coder_agent — 委托合约驱动的编程子智能体生成工具
// 替代 Phase 2A 删除的 coder_* MCP 工具桥接。
//
// 设计文档: docs/claude/tracking/design-delegation-contract-system-2026-02-27.md §5/§9
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// ---------- 子智能体生成回调类型 ----------

// SpawnSubagentParams spawn_coder_agent 子智能体生成参数。
type SpawnSubagentParams struct {
	// Contract 委托合约（已验证）。
	Contract *DelegationContract
	// Task 任务描述（直接来自合约 TaskBrief）。
	Task string
	// SystemPrompt 子智能体系统提示词（含合约段）。
	SystemPrompt string
	// TimeoutMs 子智能体超时（来自合约 TimeoutMs）。
	TimeoutMs int64
	// Label 子智能体标签（前端显示用）。
	Label string
	// Channel 异步消息通道（可选，nil = 不支持求助通道）。
	// Phase 4: 三级指挥体系 — 子智能体执行中异步向主智能体求助。
	Channel *AgentChannel
	// AgentType 子智能体类型: "coder" | "argus"。
	// Phase 5: 灵瞳完全子智能体化 — gateway 根据此字段差异化注入工具后端。
	AgentType string
}

// SpawnSubagentFunc 子智能体生成回调。
// 由 gateway/server.go 注入实现，通过 RunEmbeddedPiAgent 启动独立 LLM session。
// 返回子智能体执行结果（SubagentRunOutcome 含 ThoughtResult）。
type SpawnSubagentFunc func(ctx context.Context, params SpawnSubagentParams) (*SubagentRunOutcome, error)

// ---------- 工具输入 ----------

// spawnCoderAgentInput spawn_coder_agent 工具输入 JSON schema。
type spawnCoderAgentInput struct {
	TaskBrief       string          `json:"task_brief"`
	SuccessCriteria string          `json:"success_criteria,omitempty"`
	Scope           []ScopeEntry    `json:"scope"`
	Constraints     json.RawMessage `json:"constraints,omitempty"`
	ParentContract  string          `json:"parent_contract,omitempty"`
	TimeoutMs       *uint32         `json:"timeout_ms,omitempty"`
}

// ---------- 工具定义 ----------

// SpawnCoderAgentToolDef 返回 spawn_coder_agent 的 LLM 工具定义。
func SpawnCoderAgentToolDef() llmclient.ToolDef {
	return llmclient.ToolDef{
		Name:        "spawn_coder_agent",
		Description: "Spawn an Open Coder sub-agent with a delegation contract. The sub-agent runs as an independent LLM session with scoped permissions. Use this for complex coding tasks that benefit from delegation.",
		InputSchema: json.RawMessage(`{
	"type": "object",
	"properties": {
		"task_brief": {
			"type": "string",
			"description": "Human-readable task description (≤500 chars). Be specific about what the sub-agent should accomplish."
		},
		"success_criteria": {
			"type": "string",
			"description": "Acceptance criteria for the task (≤300 chars, optional)."
		},
		"scope": {
			"type": "array",
			"description": "Allowed file paths and permissions for the sub-agent.",
			"items": {
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "File or directory path (relative to workspace)" },
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
			"description": "Execution constraints for the sub-agent.",
			"properties": {
				"no_network": { "type": "boolean", "description": "Deny network access" },
				"no_spawn": { "type": "boolean", "description": "Deny process spawning" },
				"sandbox_required": { "type": "boolean", "description": "Force sandbox execution" },
				"max_bash_calls": { "type": "integer", "description": "Maximum bash calls allowed" },
				"allowed_commands": {
					"type": "array",
					"items": { "type": "string" },
					"description": "Whitelist of allowed bash commands (empty = unrestricted)"
				}
			}
		},
		"parent_contract": {
			"type": "string",
			"description": "Parent contract ID for resuming a previously suspended task (optional)."
		},
		"timeout_ms": {
			"type": "integer",
			"description": "Sub-agent timeout in milliseconds (default: 60000)."
		}
	},
	"required": ["task_brief", "scope"]
}`),
	}
}

// ---------- 工具执行 ----------

// executeSpawnCoderAgent 处理 spawn_coder_agent 工具调用。
// 创建委托合约 → 构建子智能体系统提示词 → 通过 SpawnSubagent 回调启动子 session。
func executeSpawnCoderAgent(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	// 1. 解析输入
	var input spawnCoderAgentInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Sprintf("[spawn_coder_agent] Invalid input: %s", err), nil
	}

	// 2. 解析 constraints（可选字段，默认空）
	var constraints ContractConstraints
	if len(input.Constraints) > 0 {
		if err := json.Unmarshal(input.Constraints, &constraints); err != nil {
			return fmt.Sprintf("[spawn_coder_agent] Invalid constraints: %s", err), nil
		}
	}

	// 3. 创建委托合约
	issuedBy := params.SessionID
	if issuedBy == "" {
		issuedBy = "main-agent"
	}
	contract, err := NewDelegationContract(issuedBy, input.TaskBrief, input.SuccessCriteria, input.Scope, constraints)
	if err != nil {
		return fmt.Sprintf("[spawn_coder_agent] Contract validation failed: %s", err), nil
	}

	// 可选: 设置父合约引用（断点恢复）
	if input.ParentContract != "" {
		contract.ParentContract = input.ParentContract
	}
	// 可选: 覆盖超时
	if input.TimeoutMs != nil && *input.TimeoutMs > 0 {
		contract.TimeoutMs = *input.TimeoutMs
	}

	// Phase 6: 加载父合约恢复上下文（如果是续接）
	var resumeContext string
	var iterationIndex int
	if input.ParentContract != "" && params.ContractStore != nil {
		_, parentThought, err := params.ContractStore.LoadContract(input.ParentContract)
		if err != nil {
			slog.Warn("spawn_coder_agent: failed to load parent contract (continuing without resume context)",
				"parentContract", input.ParentContract, "error", err)
		} else if parentThought != nil {
			resumeContext = parentThought.ResumeHint
			iterationIndex = int(parentThought.IterationCount) + 1
		}
	}

	// Phase 6: 迭代上限检查（硬上限 3 轮，防止 LLM 失控循环）
	if iterationIndex > 3 {
		return fmt.Sprintf("[spawn_coder_agent] Negotiation round limit exceeded (%d > 3). "+
			"Please handle this task directly or escalate to the user.", iterationIndex), nil
	}

	slog.Info("spawn_coder_agent: contract created",
		"contractID", contract.ContractID,
		"taskBrief", contract.TaskBrief,
		"scopeCount", len(contract.Scope),
		"parentContract", contract.ParentContract,
		"iterationIndex", iterationIndex,
	)

	// 3.5 单调衰减校验: 合约不能超出当前 agent 的权限
	parentCaps := CapabilitySetFromToolExecParams(&params)
	contractCaps := CapabilitySetFromContract(contract)
	if err := parentCaps.ValidateMonotonicDecay(contractCaps); err != nil {
		return fmt.Sprintf("[spawn_coder_agent] Permission monotonic decay violation: %s", err), nil
	}

	// 4. 构建 oa-coder 专用系统提示词（含合约段 + 编码行为准则 + 恢复上下文）
	systemPrompt := BuildCoderSubagentSystemPrompt(CoderSubagentPromptParams{
		Task:                input.TaskBrief,
		SuccessCriteria:     input.SuccessCriteria,
		Contract:            contract,
		RequesterSessionKey: params.SessionKey,
		ResumeContext:       resumeContext,
		IterationIndex:      iterationIndex,
	})

	// 5. 检查 SpawnSubagent 回调
	if params.SpawnSubagent == nil {
		// 回调未注入（Phase 2 尚未完成 gateway 接线）—— 返回合约信息供调试
		contractJSON, _ := json.MarshalIndent(contract, "", "  ")
		return fmt.Sprintf("[spawn_coder_agent] Contract created but spawn callback not configured.\n\nContract:\n%s", contractJSON), nil
	}

	// 6. 启动子智能体 session
	contract.Status = ContractActive

	// Phase 7: 绑定活动合约到审批路由器
	if params.CoderConfirmation != nil {
		params.CoderConfirmation.SetActiveContract(contract)
		defer params.CoderConfirmation.ClearActiveContract()
	}

	outcome, err := params.SpawnSubagent(ctx, SpawnSubagentParams{
		Contract:     contract,
		Task:         input.TaskBrief,
		SystemPrompt: systemPrompt,
		TimeoutMs:    int64(contract.TimeoutMs),
		Label:        fmt.Sprintf("coder-%s", contract.ContractID[:8]),
		Channel:      params.AgentChannel, // Phase 4: 注入异步消息通道
	})
	if err != nil {
		contract.Status = ContractFailed
		return fmt.Sprintf("[spawn_coder_agent] Sub-agent spawn failed: %s", err), nil
	}

	// 7. Phase 2 + Phase 3: 质量审核 + 最终交付门控（三级指挥体系）
	var qualityReviewSummary string

	// Phase 2: 质量审核门控
	// 子智能体返回结果后，主智能体审核质量 → 通过后才交付。
	// R3 混合模式: 规则预检 + 可选 LLM 语义审核。
	if outcome != nil && outcome.ThoughtResult != nil && outcome.ThoughtResult.Status == ThoughtCompleted {
		reviewParams := QualityReviewParams{
			Contract:        contract,
			Outcome:         outcome,
			TaskBrief:       input.TaskBrief,
			SuccessCriteria: input.SuccessCriteria,
		}

		reviewResult := ReviewSubagentResult(reviewParams, params.QualityReviewFn)
		if reviewResult != nil && !reviewResult.Approved {
			contract.Status = ContractFailed
			slog.Info("spawn_coder_agent: quality review failed",
				"contractID", contract.ContractID,
				"issues", reviewResult.Issues,
			)
			return FormatReviewFailedResult(contract, outcome, reviewResult), nil
		}

		// 审核通过 — 保存 review summary 供 Phase 3 展示
		if reviewResult != nil && reviewResult.ReviewSummary != "" {
			qualityReviewSummary = reviewResult.ReviewSummary
			slog.Debug("spawn_coder_agent: quality review passed",
				"contractID", contract.ContractID,
				"summary", qualityReviewSummary,
			)
		}
	}

	// Phase 3: 最终交付门控
	// 质量审核通过后，将结果呈现给用户做最终签收。
	// 用户 approve → 返回正常结果；reject → 返回 "用户要求修改" 给主智能体。
	if params.ResultApprovalMgr != nil && normalizeSecurityLevelValue(deriveMaxSecurityLevel(contract)) != "full" &&
		outcome != nil && outcome.ThoughtResult != nil &&
		outcome.ThoughtResult.Status == ThoughtCompleted {

		tr := outcome.ThoughtResult

		resultWorkflow := NewSingleStageApprovalWorkflow(
			input.TaskBrief,
			ApprovalTypeResultReview,
			"result_review（交付前最终签收）",
		)
		approvalReq := ResultApprovalRequest{
			OriginalTask:  input.TaskBrief,
			ContractID:    contract.ContractID,
			Result:        truncate(tr.Result, 500),
			Artifacts:     tr.Artifacts,
			ReviewSummary: qualityReviewSummary,
			Workflow:      resultWorkflow,
		}

		// 使用独立 context（同 Phase 1 R1 策略）
		approvalCtx, approvalCancel := context.WithTimeout(context.Background(), params.ResultApprovalMgr.Timeout())
		defer approvalCancel()

		approvalDecision, approvalErr := params.ResultApprovalMgr.RequestResultApprovalWithSessionKey(approvalCtx, approvalReq, params.SessionKey)
		if approvalErr != nil {
			slog.Warn("spawn_coder_agent: result approval error, proceeding with result",
				"contractID", contract.ContractID,
				"error", approvalErr,
			)
		} else if approvalDecision.Action == "reject" {
			slog.Info("spawn_coder_agent: result rejected by user",
				"contractID", contract.ContractID,
				"feedback", approvalDecision.Feedback,
			)
			return fmt.Sprintf("[Coder Agent Result - User Rejected]\nContract: %s\n\n"+
				"The user rejected the sub-agent's result.\nFeedback: %s\n\n"+
				"ACTION: Review the user's feedback and either:\n"+
				"  1. Re-delegate with adjusted parameters\n"+
				"  2. Handle the task differently\n"+
				"  3. Ask the user for clarification\n",
				contract.ContractID, approvalDecision.Feedback), nil
		}
		// approve — 继续返回正常结果
	}

	// 8. 合约状态转换到终态
	contract.Status = ContractCompleted

	// 9. 格式化返回结果
	return formatSpawnResult(contract, outcome), nil
}

// formatSpawnResult 格式化子智能体执行结果为主智能体可读文本。
// Phase 6: needs_auth 状态输出可操作指引（scope 扩展、约束放宽、parent_contract 提示）。
func formatSpawnResult(contract *DelegationContract, outcome *SubagentRunOutcome) string {
	if outcome == nil {
		return fmt.Sprintf("[spawn_coder_agent] Contract %s: no outcome returned", contract.ContractID)
	}

	// 有 ThoughtResult 时返回结构化摘要
	if tr := outcome.ThoughtResult; tr != nil {
		// Phase 6: needs_auth 专用可操作指引
		if tr.Status == ThoughtNeedsAuth {
			return formatNeedsAuthResult(contract, tr)
		}

		result := fmt.Sprintf("[Coder Agent Result]\nContract: %s\nStatus: %s\n",
			contract.ContractID, tr.Status)

		if tr.Result != "" {
			result += fmt.Sprintf("\n%s\n", tr.Result)
		}
		if tr.ReasoningSummary != "" {
			result += fmt.Sprintf("\nReasoning: %s\n", tr.ReasoningSummary)
		}
		if tr.Artifacts != nil {
			if len(tr.Artifacts.FilesModified) > 0 {
				result += fmt.Sprintf("\nFiles modified: %v\n", tr.Artifacts.FilesModified)
			}
			if len(tr.Artifacts.FilesCreated) > 0 {
				result += fmt.Sprintf("\nFiles created: %v\n", tr.Artifacts.FilesCreated)
			}
		}
		if tr.ResumeHint != "" {
			result += fmt.Sprintf("\nResume hint: %s\n", tr.ResumeHint)
		}
		if len(tr.ScopeViolations) > 0 {
			result += fmt.Sprintf("\nScope violations: %v\n", tr.ScopeViolations)
		}
		return result
	}

	// 无 ThoughtResult —— 返回原始状态
	result := fmt.Sprintf("[Coder Agent Result]\nContract: %s\nStatus: %s\n",
		contract.ContractID, outcome.Status)
	if outcome.Error != "" {
		result += fmt.Sprintf("Error: %s\n", outcome.Error)
	}
	return result
}

// formatNeedsAuthResult 格式化 needs_auth 状态的可操作指引。
// 主 Agent 收到此信息后，可根据 prompt 引导决策是否重新委托。
func formatNeedsAuthResult(contract *DelegationContract, tr *ThoughtResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[Coder Agent - Needs Authorization]\nContract: %s\nStatus: needs_auth\n", contract.ContractID))

	if tr.AuthRequest != nil {
		b.WriteString(fmt.Sprintf("\nReason: %s\n", tr.AuthRequest.Reason))
		b.WriteString(fmt.Sprintf("Risk level: %s\n", tr.AuthRequest.RiskLevel))

		if len(tr.AuthRequest.RequestedScopeExtension) > 0 {
			b.WriteString("\nRequested scope extensions:\n")
			for _, s := range tr.AuthRequest.RequestedScopeExtension {
				perms := make([]string, len(s.Permissions))
				for i, p := range s.Permissions {
					perms[i] = string(p)
				}
				b.WriteString(fmt.Sprintf("  - %s [%s]\n", s.Path, strings.Join(perms, ", ")))
			}
		}

		if len(tr.AuthRequest.RequestedConstraintRelaxation) > 0 {
			b.WriteString(fmt.Sprintf("\nRequested constraint relaxations: %v\n", tr.AuthRequest.RequestedConstraintRelaxation))
		}
	}

	if tr.ResumeHint != "" {
		b.WriteString(fmt.Sprintf("\nResume hint: %s\n", tr.ResumeHint))
	}

	if tr.Result != "" {
		b.WriteString(fmt.Sprintf("\nPartial result: %s\n", tr.Result))
	}

	// 可操作指引
	b.WriteString("\n---\n")
	b.WriteString(fmt.Sprintf("ACTION: To resume, use spawn_coder_agent with parent_contract=\"%s\"\n", contract.ContractID))
	b.WriteString("- Expand the scope to include the requested paths\n")
	b.WriteString("- Relax the constraints as requested (if risk is acceptable)\n")
	b.WriteString("- Include the resume_hint in the new task_brief for continuity\n")

	return b.String()
}
