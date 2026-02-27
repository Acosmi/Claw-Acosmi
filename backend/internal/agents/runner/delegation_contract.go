package runner

// ============================================================================
// Delegation Contract — 委托合约系统核心类型
// 设计文档: docs/claude/tracking/design-delegation-contract-system-2026-02-27.md §4
// ============================================================================

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropic/open-acosmi/internal/infra"
	"github.com/google/uuid"
)

// ---------- VFS 路径常量 ----------

const (
	// ContractVFSBase VFS 根路径（_system 级别，不暴露给 agent）。
	ContractVFSBase = "_system/contracts"
	// ContractDirActive 运行中的合约。
	ContractDirActive = ContractVFSBase + "/active"
	// ContractDirSuspended 等待主 agent 授权的合约。
	ContractDirSuspended = ContractVFSBase + "/suspended"
	// ContractDirCompleted 已完成的合约（TTL 后可清理）。
	ContractDirCompleted = ContractVFSBase + "/completed"
	// ContractDirFailed 失败/超时的合约（保留供审计）。
	ContractDirFailed = ContractVFSBase + "/failed"
)

// ---------- 字段大小约束 ----------

const (
	MaxTaskBriefLen       = 500
	MaxSuccessCriteriaLen = 300
	MaxScopeEntries       = 20
	MaxAllowedCommands    = 50
	MaxResultLen          = 10_000
	MaxResumeHintLen      = 300
	MaxReasoningSummary   = 500
	MaxScopeViolations    = 20
)

// ---------- DelegationContract ----------

// ContractStatus 合约生命周期状态。
type ContractStatus string

const (
	ContractPending   ContractStatus = "pending"
	ContractActive    ContractStatus = "active"
	ContractSuspended ContractStatus = "suspended"
	ContractCompleted ContractStatus = "completed"
	ContractFailed    ContractStatus = "failed"
	ContractCancelled ContractStatus = "cancelled"
)

// ScopePermission 路径权限维度。
type ScopePermission string

const (
	PermRead    ScopePermission = "read"
	PermWrite   ScopePermission = "write"
	PermExecute ScopePermission = "execute"
)

// ScopeEntry 描述合约允许的路径及权限。
type ScopeEntry struct {
	Path        string            `json:"path"`
	Permissions []ScopePermission `json:"permissions"`
}

// ContractConstraints 合约约束集。
type ContractConstraints struct {
	NoNetwork       bool     `json:"no_network"`
	NoSpawn         bool     `json:"no_spawn"`
	SandboxRequired bool     `json:"sandbox_required"`
	MaxBashCalls    *uint32  `json:"max_bash_calls,omitempty"`
	AllowedCommands []string `json:"allowed_commands,omitempty"`
}

// DelegationContract 委托合约——主 agent 授权子 agent 执行任务的结构化契约。
type DelegationContract struct {
	ContractID     string              `json:"contract_id"`
	SchemaVersion  string              `json:"schema_version"`
	ParentContract string              `json:"parent_contract,omitempty"`
	TaskBrief      string              `json:"task_brief"`
	SuccessCriteria string             `json:"success_criteria"`
	Scope          []ScopeEntry        `json:"scope"`
	Constraints    ContractConstraints `json:"constraints"`
	IssuedBy       string              `json:"issued_by"`
	IssuedAt       time.Time           `json:"issued_at"`
	TimeoutMs      uint32              `json:"timeout_ms"`
	Status         ContractStatus      `json:"status"`
}

// NewDelegationContract 创建一份新合约（pending 状态）。
func NewDelegationContract(issuedBy, taskBrief, successCriteria string, scope []ScopeEntry, constraints ContractConstraints) (*DelegationContract, error) {
	c := &DelegationContract{
		ContractID:      uuid.New().String(),
		SchemaVersion:   "1.0",
		TaskBrief:       taskBrief,
		SuccessCriteria: successCriteria,
		Scope:           scope,
		Constraints:     constraints,
		IssuedBy:        issuedBy,
		IssuedAt:        time.Now(),
		TimeoutMs:       60_000,
		Status:          ContractPending,
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid contract: %w", err)
	}
	return c, nil
}

// Validate 校验合约字段大小约束。
func (c *DelegationContract) Validate() error {
	if len([]rune(c.TaskBrief)) > MaxTaskBriefLen {
		return fmt.Errorf("task_brief exceeds %d chars", MaxTaskBriefLen)
	}
	if len([]rune(c.SuccessCriteria)) > MaxSuccessCriteriaLen {
		return fmt.Errorf("success_criteria exceeds %d chars", MaxSuccessCriteriaLen)
	}
	if len(c.Scope) > MaxScopeEntries {
		return fmt.Errorf("scope entries (%d) exceeds max %d", len(c.Scope), MaxScopeEntries)
	}
	if len(c.Constraints.AllowedCommands) > MaxAllowedCommands {
		return fmt.Errorf("allowed_commands (%d) exceeds max %d", len(c.Constraints.AllowedCommands), MaxAllowedCommands)
	}
	if c.TaskBrief == "" {
		return fmt.Errorf("task_brief is required")
	}
	return nil
}

// ---------- ThoughtResult ----------

// ThoughtStatus 子 agent 返回状态。
type ThoughtStatus string

const (
	ThoughtCompleted ThoughtStatus = "completed"
	ThoughtPartial   ThoughtStatus = "partial"
	ThoughtBlocked   ThoughtStatus = "blocked"
	ThoughtNeedsAuth ThoughtStatus = "needs_auth"
	ThoughtFailed    ThoughtStatus = "failed"
	ThoughtTimeout   ThoughtStatus = "timeout"
)

// ThoughtArtifacts 子 agent 执行产物。
type ThoughtArtifacts struct {
	FilesModified []string `json:"files_modified,omitempty"`
	FilesCreated  []string `json:"files_created,omitempty"`
	CommandsRun   []string `json:"commands_run,omitempty"`
}

// AuthRequest 授权请求（status=needs_auth 时填写）。
type AuthRequest struct {
	Reason                        string       `json:"reason"`
	RequestedScopeExtension       []ScopeEntry `json:"requested_scope_extension,omitempty"`
	RequestedConstraintRelaxation []string     `json:"requested_constraint_relaxation,omitempty"`
	RiskLevel                     string       `json:"risk_level"` // "low" | "medium" | "high"
}

// ThoughtResult 子 agent 结构化返回——包含执行结果、状态、授权请求等。
type ThoughtResult struct {
	Result           string            `json:"result"`
	ContractID       string            `json:"contract_id"`
	Status           ThoughtStatus     `json:"status"`
	Artifacts        *ThoughtArtifacts `json:"artifacts,omitempty"`
	AuthRequest      *AuthRequest      `json:"auth_request,omitempty"`
	ResumeHint       string            `json:"resume_hint,omitempty"`
	PartialArtifacts *ThoughtArtifacts `json:"partial_artifacts,omitempty"`
	ReasoningSummary string            `json:"reasoning_summary,omitempty"`
	IterationCount   uint32            `json:"iteration_count,omitempty"`
	ScopeViolations  []string          `json:"scope_violations,omitempty"`
}

// ValidateResult 校验 ThoughtResult 字段大小约束。
func (t *ThoughtResult) ValidateResult() error {
	if len([]rune(t.Result)) > MaxResultLen {
		return fmt.Errorf("result exceeds %d chars", MaxResultLen)
	}
	if len([]rune(t.ResumeHint)) > MaxResumeHintLen {
		return fmt.Errorf("resume_hint exceeds %d chars", MaxResumeHintLen)
	}
	if len([]rune(t.ReasoningSummary)) > MaxReasoningSummary {
		return fmt.Errorf("reasoning_summary exceeds %d chars", MaxReasoningSummary)
	}
	if len(t.ScopeViolations) > MaxScopeViolations {
		return fmt.Errorf("scope_violations (%d) exceeds max %d", len(t.ScopeViolations), MaxScopeViolations)
	}
	return nil
}

// ---------- ThoughtResult 解析 ----------

// ParseThoughtResult 尝试从子 agent 回复文本中解析 ThoughtResult JSON。
// 返回 nil 表示非 JSON 或解析失败——向后兼容纯文本模式。
func ParseThoughtResult(reply string) *ThoughtResult {
	trimmed := strings.TrimSpace(reply)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var tr ThoughtResult
	if err := json.Unmarshal([]byte(trimmed), &tr); err != nil {
		return nil
	}
	// 基本校验：status 是 ThoughtResult 必填字段
	if tr.Status == "" {
		return nil
	}
	return &tr
}

// ---------- 合约 → 系统提示词序列化 ----------

// FormatForSystemPrompt 将合约序列化为子 agent 系统提示词中的 Delegation Contract 段。
func (c *DelegationContract) FormatForSystemPrompt() string {
	var b strings.Builder
	b.WriteString("## Delegation Contract\n\n")
	b.WriteString(fmt.Sprintf("- **Contract ID**: %s\n", c.ContractID))
	b.WriteString(fmt.Sprintf("- **Task**: %s\n", c.TaskBrief))
	if c.SuccessCriteria != "" {
		b.WriteString(fmt.Sprintf("- **Success Criteria**: %s\n", c.SuccessCriteria))
	}
	if c.ParentContract != "" {
		b.WriteString(fmt.Sprintf("- **Parent Contract**: %s (resumed task)\n", c.ParentContract))
	}
	b.WriteString(fmt.Sprintf("- **Timeout**: %dms\n", c.TimeoutMs))

	// Scope
	b.WriteString("\n### Allowed Scope\n\n")
	for _, s := range c.Scope {
		perms := make([]string, len(s.Permissions))
		for i, p := range s.Permissions {
			perms[i] = string(p)
		}
		b.WriteString(fmt.Sprintf("- `%s` [%s]\n", s.Path, strings.Join(perms, ", ")))
	}

	// Constraints
	b.WriteString("\n### Constraints\n\n")
	if c.Constraints.NoNetwork {
		b.WriteString("- **No network access**\n")
	}
	if c.Constraints.NoSpawn {
		b.WriteString("- **No process spawning**\n")
	}
	if c.Constraints.SandboxRequired {
		b.WriteString("- **Sandbox execution required**\n")
	}
	if c.Constraints.MaxBashCalls != nil {
		b.WriteString(fmt.Sprintf("- **Max bash calls**: %d\n", *c.Constraints.MaxBashCalls))
	}
	if len(c.Constraints.AllowedCommands) > 0 {
		b.WriteString(fmt.Sprintf("- **Allowed commands**: %s\n", strings.Join(c.Constraints.AllowedCommands, ", ")))
	}

	// Rules
	b.WriteString("\n### Rules\n\n")
	b.WriteString("1. **Stay within scope** — accessing paths outside the allowed scope will terminate your session\n")
	b.WriteString("2. **Respect constraints** — violating constraints will terminate your session\n")
	b.WriteString("3. **Report blockers** — if you cannot complete the task within scope, return a ThoughtResult with status `needs_auth`\n")
	b.WriteString("4. **Return structured result** — your final message MUST be a valid ThoughtResult JSON\n")

	return b.String()
}

// ---------- 合约 → ToolExecParams 约束注入 ----------

// ApplyConstraints 将合约约束映射到 ToolExecParams 权限守卫。
// 只收窄权限，不扩展——符合权限单调衰减原则（DeepMind arXiv:2602.11865）。
func (c *DelegationContract) ApplyConstraints(params *ToolExecParams) {
	if c == nil {
		return
	}
	params.DelegationContract = c

	// read_only: scope 不含 write 权限 → AllowWrite = false
	hasWrite := false
	for _, s := range c.Scope {
		for _, p := range s.Permissions {
			if p == PermWrite {
				hasWrite = true
				break
			}
		}
		if hasWrite {
			break
		}
	}
	if !hasWrite {
		params.AllowWrite = false
	}

	// no_exec / scope 不含 execute 权限 → AllowExec = false
	if c.Constraints.NoSpawn {
		params.AllowExec = false
	} else {
		hasExec := false
		for _, s := range c.Scope {
			for _, p := range s.Permissions {
				if p == PermExecute {
					hasExec = true
					break
				}
			}
			if hasExec {
				break
			}
		}
		if !hasExec {
			params.AllowExec = false
		}
	}

	// sandbox_required → 强制 SandboxMode + 固定 SecurityLevel
	if c.Constraints.SandboxRequired {
		params.SandboxMode = true
		params.SecurityLevel = "allowlist"
	}

	// allowed_commands → deny-all + 白名单规则集
	if len(c.Constraints.AllowedCommands) > 0 {
		var contractRules []infra.CommandRule
		// deny-all 基础规则（最低优先级）
		contractRules = append(contractRules, infra.CommandRule{
			ID:       "contract-deny-all",
			Pattern:  "*",
			Action:   infra.RuleActionDeny,
			Priority: 999,
		})
		// 白名单规则（高优先级）
		for i, cmd := range c.Constraints.AllowedCommands {
			contractRules = append(contractRules, infra.CommandRule{
				ID:       fmt.Sprintf("contract-allow-%d", i),
				Pattern:  cmd,
				Action:   infra.RuleActionAllow,
				Priority: i,
			})
		}
		// 合约规则追加到现有规则前（优先匹配）
		params.Rules = append(contractRules, params.Rules...)
	}

	// scope_paths 提取（供 validateToolPathWithScope 使用）
	scopePaths := make([]string, 0, len(c.Scope))
	for _, s := range c.Scope {
		scopePaths = append(scopePaths, s.Path)
	}
	if len(scopePaths) > 0 {
		params.ScopePaths = scopePaths
	}
}
