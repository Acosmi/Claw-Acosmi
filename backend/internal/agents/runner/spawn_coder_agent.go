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

	"github.com/anthropic/open-acosmi/internal/agents/llmclient"
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
		Description: "Spawn a coding sub-agent with a delegation contract. The sub-agent runs as an independent LLM session with scoped permissions. Use this instead of directly editing/running code when the task is complex enough to delegate.",
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
	issuedBy := "main-agent" // TODO: Phase 3 从 session context 获取
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

	slog.Info("spawn_coder_agent: contract created",
		"contractID", contract.ContractID,
		"taskBrief", contract.TaskBrief,
		"scopeCount", len(contract.Scope),
		"parentContract", contract.ParentContract,
	)

	// 4. 构建子智能体系统提示词（含合约段）
	systemPrompt := BuildSubagentSystemPrompt(SubagentSystemPromptParams{
		Task:     input.TaskBrief,
		Contract: contract,
		Label:    "coder",
	})

	// 5. 检查 SpawnSubagent 回调
	if params.SpawnSubagent == nil {
		// 回调未注入（Phase 2 尚未完成 gateway 接线）—— 返回合约信息供调试
		contractJSON, _ := json.MarshalIndent(contract, "", "  ")
		return fmt.Sprintf("[spawn_coder_agent] Contract created but spawn callback not configured.\n\nContract:\n%s", contractJSON), nil
	}

	// 6. 启动子智能体 session
	contract.Status = ContractActive
	outcome, err := params.SpawnSubagent(ctx, SpawnSubagentParams{
		Contract:     contract,
		Task:         input.TaskBrief,
		SystemPrompt: systemPrompt,
		TimeoutMs:    int64(contract.TimeoutMs),
		Label:        fmt.Sprintf("coder-%s", contract.ContractID[:8]),
	})
	if err != nil {
		contract.Status = ContractFailed
		return fmt.Sprintf("[spawn_coder_agent] Sub-agent spawn failed: %s", err), nil
	}

	// 7. 格式化返回结果
	return formatSpawnResult(contract, outcome), nil
}

// formatSpawnResult 格式化子智能体执行结果为主智能体可读文本。
func formatSpawnResult(contract *DelegationContract, outcome *SubagentRunOutcome) string {
	if outcome == nil {
		return fmt.Sprintf("[spawn_coder_agent] Contract %s: no outcome returned", contract.ContractID)
	}

	// 有 ThoughtResult 时返回结构化摘要
	if tr := outcome.ThoughtResult; tr != nil {
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
		if tr.AuthRequest != nil {
			result += fmt.Sprintf("\n⚠️ Auth Request: %s (risk: %s)\n", tr.AuthRequest.Reason, tr.AuthRequest.RiskLevel)
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
