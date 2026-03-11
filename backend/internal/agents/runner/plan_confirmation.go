package runner

// plan_confirmation.go — 三级指挥体系 Phase 1: 方案确认门控
//
// task_write / task_delete / task_multimodal 意图下，
// 主智能体先生成方案 → 用户批准 → 才执行。
//
// 复用 CoderConfirmationManager 的阻塞 channel 模式。
// 行业对标:
//   - LangGraph: interrupt() + checkpoint（R4 TTL 清理借鉴 checkpoint 机制）
//   - Anthropic: Oversight Paradox（R5 GateMode 预留 smart/monitor 模式）
//   - OpenAI Agents SDK: Guardrails tripwire（紧急中止能力）

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/google/uuid"
)

// ---------- 请求 / 决策类型 ----------

// PlanConfirmationRequest 方案确认请求。
type PlanConfirmationRequest struct {
	ID                  string                `json:"id"`
	TaskBrief           string                `json:"taskBrief"`
	PlanSteps           []string              `json:"planSteps"`
	EstimatedScope      []ScopeEntry          `json:"estimatedScope,omitempty"`
	PrimaryApproval     ApprovalRequirement   `json:"primaryApproval,omitempty"`
	AdditionalApprovals []ApprovalRequirement `json:"additionalApprovals,omitempty"`
	ApprovalSummary     []string              `json:"approvalSummary,omitempty"`
	IntentTier          string                `json:"intentTier"`
	CreatedAtMs         int64                 `json:"createdAtMs"`
	ExpiresAtMs         int64                 `json:"expiresAtMs"`
	Workflow            ApprovalWorkflow      `json:"workflow,omitempty"`
}

// PlanDecision 用户对方案的决策。
type PlanDecision struct {
	Action     string `json:"action"`               // "approve" | "reject" | "edit"
	EditedPlan string `json:"editedPlan,omitempty"` // action=edit 时的修改方案
	Feedback   string `json:"feedback,omitempty"`   // action=reject 时的拒绝原因
}

// PlanDecisionRecord 决策记录（用于 VFS 持久化 [R9]）。
type PlanDecisionRecord struct {
	RequestID   string       `json:"requestId"`
	TaskBrief   string       `json:"taskBrief"`
	PlanSteps   []string     `json:"planSteps"`
	IntentTier  string       `json:"intentTier"`
	Decision    PlanDecision `json:"decision"`
	DecidedAtMs int64        `json:"decidedAtMs"`
}

// PlanDecisionLogger 决策持久化接口（[R9] 由 gateway 注入 VFS 实现）。
type PlanDecisionLogger interface {
	LogPlanDecision(record PlanDecisionRecord) error
}

// PlanConfirmRemoteNotifyFunc 方案确认远程通知回调（飞书/钉钉等非 Web 渠道）。
// sessionKey 用于确定目标渠道（如 "feishu:<chatID>"），空字符串表示广播到所有已配置渠道。
type PlanConfirmRemoteNotifyFunc func(req PlanConfirmationRequest, sessionKey string)

// ---------- GateMode [R5] ----------

const (
	// GateModeFull 全量门控：所有 task_write+ 意图都弹窗确认。
	GateModeFull = "full"
	// GateModeSmart 智能门控：低风险自动通过，高风险才弹窗（未来实现）。
	GateModeSmart = "smart"
	// GateModeMonitor 监控门控：全部自动通过，用户可随时干预（Anthropic 推荐终态）。
	GateModeMonitor = "monitor"
)

// ---------- Manager ----------

// planConfirmationEntry 内部 pending 条目（含过期时间和决策 channel）。
type planConfirmationEntry struct {
	ch        chan PlanDecision
	expiresAt time.Time
	req       PlanConfirmationRequest
}

// PlanConfirmationManager 方案确认管理器。
// 当主智能体生成方案后:
//  1. 广播 "plan.confirm.requested" 给前端（WebSocket）
//  2. 阻塞等待用户决策（approve/reject/edit）或超时
//  3. 前端通过 "plan.confirm.resolve" RPC 回调
//
// 为 nil 时完全跳过方案确认（兼容现有行为）。
type PlanConfirmationManager struct {
	mu      sync.Mutex
	pending map[string]*planConfirmationEntry // id → pending entry (含过期时间)

	broadcast      CoderConfirmBroadcastFunc   // 复用 coder 广播类型（解耦 runner ↔ gateway）
	remoteNotify   PlanConfirmRemoteNotifyFunc // 远程通知回调（飞书卡片等），可为 nil
	decisionLogger PlanDecisionLogger          // [R9] 可选，nil = 不记录决策

	timeout  time.Duration // 默认 5min
	gateMode string        // [R5] "full" | "smart" | "monitor"

	// TTL 清理 [R4]
	closeOnce   sync.Once     // 防止 Close() 重复调用 panic
	cleanupDone chan struct{} // 关闭时停止 TTL 清理 goroutine
}

// NewPlanConfirmationManager 创建方案确认管理器。
func NewPlanConfirmationManager(
	broadcastFn CoderConfirmBroadcastFunc,
	remoteNotifyFn PlanConfirmRemoteNotifyFunc,
	timeout time.Duration,
) *PlanConfirmationManager {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	m := &PlanConfirmationManager{
		pending:      make(map[string]*planConfirmationEntry),
		broadcast:    broadcastFn,
		remoteNotify: remoteNotifyFn,
		timeout:      timeout,
		gateMode:     GateModeFull, // Phase 1 硬编码 full
		cleanupDone:  make(chan struct{}),
	}
	// [R4] 启动 TTL 清理 goroutine
	go m.ttlCleanupLoop()
	return m
}

// SetDecisionLogger 设置决策持久化日志器 [R9]。
func (m *PlanConfirmationManager) SetDecisionLogger(logger PlanDecisionLogger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisionLogger = logger
}

// SetGateMode 设置门控模式 [R5]。
func (m *PlanConfirmationManager) SetGateMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch mode {
	case GateModeFull, GateModeSmart, GateModeMonitor:
		m.gateMode = mode
	default:
		slog.Warn("invalid gate mode, keeping current", "mode", mode, "current", m.gateMode)
	}
}

// GateMode 返回当前门控模式。
func (m *PlanConfirmationManager) GateMode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gateMode
}

// ShouldGate 判断当前模式是否需要门控（full 需要，monitor 不需要，smart 未来按风险判断）。
func (m *PlanConfirmationManager) ShouldGate() bool {
	mode := m.GateMode()
	switch mode {
	case GateModeMonitor:
		return false
	case GateModeSmart:
		// TODO: Phase L3 — 按 intentTier + 任务风险动态判断
		return true // 暂时等同 full
	default:
		return true
	}
}

// RequestPlanConfirmation 请求用户确认执行方案。
// 阻塞直到用户决策、超时或 ctx 取消。
// [R1] ctx 应为独立 context（不复用 RunAttempt timeout）。
func (m *PlanConfirmationManager) RequestPlanConfirmation(ctx context.Context, req PlanConfirmationRequest) (PlanDecision, error) {
	return m.RequestPlanConfirmationWithSessionKey(ctx, req, "")
}

// RequestPlanConfirmationWithSessionKey 请求用户确认执行方案，传递真实 sessionKey。
// P5-10: 修复 sessionKey 丢失 — 远程通知需要 sessionKey 才能路由到正确的频道。
func (m *PlanConfirmationManager) RequestPlanConfirmationWithSessionKey(ctx context.Context, req PlanConfirmationRequest, sessionKey string) (PlanDecision, error) {
	// 填充默认字段
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	now := time.Now()
	if req.CreatedAtMs == 0 {
		req.CreatedAtMs = now.UnixMilli()
	}
	if req.ExpiresAtMs == 0 {
		req.ExpiresAtMs = now.Add(m.timeout).UnixMilli()
	}

	ch := make(chan PlanDecision, 1)

	m.mu.Lock()
	m.pending[req.ID] = &planConfirmationEntry{
		ch:        ch,
		expiresAt: time.UnixMilli(req.ExpiresAtMs),
		req:       req,
	}
	m.mu.Unlock()

	// 广播方案确认请求到前端（WebSocket）
	if m.broadcast != nil {
		m.broadcast("plan.confirm.requested", req)
	}
	broadcastApprovalWorkflow(m.broadcast, req.Workflow, "plan.confirm.requested", req.ID)

	// P5-10: 推送远程通知到非 Web 渠道（飞书卡片等），传递真实 sessionKey
	if m.remoteNotify != nil {
		m.remoteNotify(req, sessionKey)
	}

	slog.Debug("plan confirmation requested",
		"id", req.ID,
		"tier", req.IntentTier,
		"steps", len(req.PlanSteps),
	)

	// 等待用户决策、超时或 ctx 取消
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()

	var decision PlanDecision
	select {
	case decision = <-ch:
		// 用户已决策
	case <-timer.C:
		decision = PlanDecision{Action: "reject", Feedback: "timeout"}
		slog.Info("plan confirmation timed out, auto-rejecting",
			"id", req.ID,
		)
	case <-ctx.Done():
		decision = PlanDecision{Action: "reject", Feedback: "context cancelled"}
		slog.Debug("plan confirmation cancelled by context",
			"id", req.ID,
		)
	}

	// 清理 pending
	m.mu.Lock()
	delete(m.pending, req.ID)
	logger := m.decisionLogger
	m.mu.Unlock()

	resolvedWorkflow := req.Workflow
	if resolvedWorkflow.ID != "" {
		resolvedWorkflow = resolvedWorkflow.MarkStageResolved(ApprovalTypePlanConfirmRunner, req.ID, decision.Action)
	}

	// 广播决策结果
	if m.broadcast != nil {
		m.broadcast("plan.confirm.resolved", map[string]interface{}{
			"id":       req.ID,
			"decision": decision,
			"ts":       time.Now().UnixMilli(),
			"workflow": resolvedWorkflow,
		})
	}
	broadcastApprovalWorkflow(m.broadcast, resolvedWorkflow, "plan.confirm.resolved", req.ID)

	// [R9] 决策记录
	if logger != nil {
		record := PlanDecisionRecord{
			RequestID:   req.ID,
			TaskBrief:   req.TaskBrief,
			PlanSteps:   req.PlanSteps,
			IntentTier:  req.IntentTier,
			Decision:    decision,
			DecidedAtMs: time.Now().UnixMilli(),
		}
		if logErr := logger.LogPlanDecision(record); logErr != nil {
			slog.Warn("failed to log plan decision", "error", logErr, "id", req.ID)
		}
	}

	return decision, nil
}

// ResolvePlanConfirmation 处理前端的方案确认决策回调。
// 由 WebSocket RPC "plan.confirm.resolve" 调用。
func (m *PlanConfirmationManager) ResolvePlanConfirmation(id string, decision PlanDecision) error {
	if decision.Action != "approve" && decision.Action != "reject" && decision.Action != "edit" {
		return fmt.Errorf("invalid decision action: %q (expected approve/reject/edit)", decision.Action)
	}

	m.mu.Lock()
	entry, ok := m.pending[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("no pending plan confirmation with id: %s", id)
	}

	// 非阻塞写入（channel 有 1 缓冲）
	select {
	case entry.ch <- decision:
		slog.Debug("plan confirmation resolved",
			"id", id,
			"action", decision.Action,
		)
	default:
		// channel 已被写入（超时或重复调用），忽略
	}

	return nil
}

func (m *PlanConfirmationManager) PendingRequest(id string) (PlanConfirmationRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.pending[id]
	if !ok || entry == nil {
		return PlanConfirmationRequest{}, false
	}
	return entry.req, true
}

// Close 关闭管理器，停止 TTL 清理 goroutine。安全支持重复调用。
func (m *PlanConfirmationManager) Close() {
	m.closeOnce.Do(func() {
		close(m.cleanupDone)
	})
}

// ---------- TTL 清理 [R4] ----------

// ttlCleanupLoop 后台 goroutine 每 1min 扫描 pending map，
// 删除超过 TTL 的条目并关闭 channel（防止 goroutine 泄漏）。
func (m *PlanConfirmationManager) ttlCleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.cleanupDone:
			return
		case <-ticker.C:
			m.cleanupExpired()
		}
	}
}

// cleanupExpired 清理超过 TTL 的 pending 条目。
// 仅清理已过期条目，未过期条目保留（修复: 之前未检查过期时间导致提前清理）。
func (m *PlanConfirmationManager) cleanupExpired() {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, entry := range m.pending {
		if now.Before(entry.expiresAt) {
			continue // 未过期，跳过
		}
		// 过期 — auto-reject
		select {
		case entry.ch <- PlanDecision{Action: "reject", Feedback: "ttl_expired"}:
			slog.Debug("plan confirmation TTL expired, auto-rejected", "id", id)
		default:
			// channel 已有决策（timer 先触发），只清理 map
		}
		delete(m.pending, id)
	}
}

// Timeout 返回确认超时时间。
func (m *PlanConfirmationManager) Timeout() time.Duration {
	return m.timeout
}

// PendingCount 返回当前等待确认的请求数（用于监控）。
func (m *PlanConfirmationManager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}

// ---------- Phase 6: 审批语义拆分 — Mutating/Validating 两阶段 ----------
// 借鉴: K8s Admission Webhook (Mutating → Validating 两阶段模式)

// ApprovalRequirement 表示一个明确的审批需求。
// 用于表达 send_media 这类可能同时需要 data_export 与 mount_access 的双审批结构。
type ApprovalRequirement struct {
	Type            string   `json:"type"`                      // plan_confirm/exec_escalation/mount_access/data_export
	RequestedLevel  string   `json:"requestedLevel,omitempty"`  // exec_escalation
	TTLMinutes      int      `json:"ttlMinutes"`                // exec/data_export 使用 TTL；mount_access 固定为 0（本任务）
	MountMode       string   `json:"mountMode,omitempty"`       // mount_access
	MountPath       string   `json:"mountPath,omitempty"`       // mount_access
	NeedsOriginator bool     `json:"needsOriginator,omitempty"` // data_export
	NeedsRunSession bool     `json:"needsRunSession,omitempty"` // exec_escalation
	ExportFiles     []string `json:"exportFiles,omitempty"`     // data_export
	TargetChannel   string   `json:"targetChannel,omitempty"`   // data_export
}

// ApprovalScope 是 Mutating Phase 的输出。
// 携带从 IntentAnalysis + 树推导的审批作用域和 EscalationHints 默认值。
// 为兼容现有调用，保留旧的一组扁平字段，同时补出 PrimaryApproval/AdditionalApprovals。
type ApprovalScope struct {
	PrimaryApproval     ApprovalRequirement   `json:"primaryApproval"`
	AdditionalApprovals []ApprovalRequirement `json:"additionalApprovals,omitempty"`
	Type                string                `json:"type"`            // 向后兼容：PrimaryApproval.Type
	RequestedLevel      string                `json:"requestedLevel"`  // From EscalationHints.DefaultRequestedLevel
	TTLMinutes          int                   `json:"ttlMinutes"`      // From EscalationHints.DefaultTTLMinutes; mount_access 固定为 0（本任务）
	MountMode           string                `json:"mountMode"`       // From EscalationHints.DefaultMountMode
	MountPath           string                `json:"mountPath"`       // 从 targets 推导的挂载路径
	NeedsOriginator     bool                  `json:"needsOriginator"` // From EscalationHints.NeedsOriginator
	NeedsRunSession     bool                  `json:"needsRunSession"` // From EscalationHints.NeedsRunSession
	Scope               []ScopeEntry          `json:"scope"`           // 从 targets 推导的操作范围
	Risks               []string              `json:"risks"`           // From IntentAnalysis.RiskHints
	ExportFiles         []string              `json:"exportFiles"`     // 数据导出涉及的文件
	TargetChannel       string                `json:"targetChannel"`   // 数据导出目标频道
}

// DeriveApprovalScope (Mutating Phase, P6-7) 从 IntentAnalysis 和树自动推导审批作用域。
//
// 推导规则:
//  1. 遍历 RequiredActions 中每个动作对应的树节点
//  2. 选择最高优先级的 ApprovalType 作为主审批类型
//  3. 从 EscalationHints 填充 RequestedLevel/TTL/MountMode 等默认值 (P6-9)
//  4. 从 Targets 推导文件路径和频道信息
//  5. 上下文感知升级: send_file + 文件目标 → data_export
func DeriveApprovalScope(analysis IntentAnalysis, tree *capabilities.CapabilityTree) ApprovalScope {
	scope := ApprovalScope{
		Scope: EstimatedScopeFromAnalysis(analysis, tree),
		Risks: analysis.RiskHints,
	}

	// 1. 遍历 actions，选择最高优先级 ApprovalType + 填充 EscalationHints 默认值 (P6-9)
	for _, action := range analysis.RequiredActions {
		node := tree.LookupByToolHint(action.ToolHint)
		if node == nil || node.Perms == nil {
			continue
		}

		approvalType := node.Perms.ApprovalType
		if approvalType == "" || approvalType == "none" {
			continue
		}

		if approvalTypePriority(approvalType) > approvalTypePriority(scope.Type) {
			scope.Type = approvalType
		}

		// P6-9: EscalationHints 驱动默认值填充 (most-permissive-wins)
		// 多个 action 可能具有不同 hints，选择最宽松的值确保审批范围覆盖全部需求。
		if node.Perms.EscalationHints != nil {
			hints := node.Perms.EscalationHints
			if hints.DefaultRequestedLevel != "" &&
				requestedLevelPriority(hints.DefaultRequestedLevel) > requestedLevelPriority(scope.RequestedLevel) {
				scope.RequestedLevel = hints.DefaultRequestedLevel
			}
			if hints.DefaultTTLMinutes > scope.TTLMinutes {
				scope.TTLMinutes = hints.DefaultTTLMinutes
			}
			if hints.DefaultMountMode != "" &&
				mountModePriority(hints.DefaultMountMode) > mountModePriority(scope.MountMode) {
				scope.MountMode = hints.DefaultMountMode
			}
			if hints.NeedsOriginator {
				scope.NeedsOriginator = true
			}
			if hints.NeedsRunSession {
				scope.NeedsRunSession = true
			}
		}
	}

	// 2. 从 Targets 提取文件和路径信息
	for _, t := range analysis.Targets {
		switch t.Kind {
		case "file":
			scope.ExportFiles = append(scope.ExportFiles, t.Value)
			if t.Known && strings.HasPrefix(t.Value, "/") {
				if scope.MountPath == "" {
					scope.MountPath = filepath.Dir(t.Value)
				}
			}
		}
	}

	// 3. 上下文感知审批类型升级: send_file + 文件目标 → data_export
	for _, action := range analysis.RequiredActions {
		if action.Action == "send_file" && len(scope.ExportFiles) > 0 {
			if approvalTypePriority(scope.Type) <= approvalTypePriority("plan_confirm") {
				scope.Type = "data_export"
			}
		}
	}

	// 4. 安全默认值
	if scope.Type == "" {
		scope.Type = "plan_confirm"
	}
	if scope.Type == "mount_access" {
		scope.TTLMinutes = 0
	} else if scope.Type == "exec_escalation" && scope.RequestedLevel == "full" {
		scope.TTLMinutes = 0
	} else if scope.TTLMinutes == 0 {
		scope.TTLMinutes = 30
	}

	scope.PrimaryApproval = scope.primaryRequirement()

	if hasSendFileApprovalRequirement(analysis) {
		exportApproval := ApprovalRequirement{
			Type:            "data_export",
			TTLMinutes:      scope.TTLMinutes,
			NeedsOriginator: scope.NeedsOriginator,
			ExportFiles:     append([]string(nil), scope.ExportFiles...),
			TargetChannel:   scope.TargetChannel,
		}
		if scope.Type != exportApproval.Type {
			scope.AdditionalApprovals = appendAdditionalApproval(scope.AdditionalApprovals, exportApproval)
		}
	}
	if hasMountAccessApprovalRequirement(analysis, tree) && scope.MountPath != "" {
		mountApproval := ApprovalRequirement{
			Type:       "mount_access",
			TTLMinutes: 0,
			MountMode:  scope.MountMode,
			MountPath:  scope.MountPath,
		}
		if scope.Type != mountApproval.Type {
			scope.AdditionalApprovals = appendAdditionalApproval(scope.AdditionalApprovals, mountApproval)
		}
	}

	return scope
}

// ValidateApprovalScope (Validating Phase, P6-8) 校验权限并最终确定审批请求。
//
// 校验规则:
//  1. 审批类型必须是四种有效类型之一
//  2. exec_escalation 必须有 RequestedLevel
//  3. mount_access 必须有 MountMode
//  4. exec/data_export 的 TTL 在合理范围 (1-480 分钟)，mount_access 固定为当前任务
func ValidateApprovalScope(scope ApprovalScope, tree *capabilities.CapabilityTree) (ApprovalScope, error) {
	switch scope.Type {
	case "plan_confirm", "exec_escalation", "mount_access", "data_export":
		// 有效类型
	default:
		return scope, fmt.Errorf("invalid approval type: %q", scope.Type)
	}

	if scope.Type == "exec_escalation" && scope.RequestedLevel == "" {
		scope.RequestedLevel = "sandboxed"
	}

	if scope.Type == "mount_access" && scope.MountMode == "" {
		scope.MountMode = "ro"
	}

	if scope.Type == "mount_access" {
		scope.TTLMinutes = 0
	} else if scope.Type == "exec_escalation" && scope.RequestedLevel == "full" {
		scope.TTLMinutes = 0
	} else if scope.TTLMinutes <= 0 {
		scope.TTLMinutes = 30
	}
	if scope.Type != "mount_access" && scope.TTLMinutes > 480 {
		scope.TTLMinutes = 480
	}

	scope.PrimaryApproval = scope.primaryRequirement()
	for i := range scope.AdditionalApprovals {
		normalized, err := normalizeApprovalRequirement(scope.AdditionalApprovals[i])
		if err != nil {
			return scope, err
		}
		scope.AdditionalApprovals[i] = normalized
	}

	return scope, nil
}

func (s ApprovalScope) primaryRequirement() ApprovalRequirement {
	return ApprovalRequirement{
		Type:            s.Type,
		RequestedLevel:  s.RequestedLevel,
		TTLMinutes:      s.TTLMinutes,
		MountMode:       s.MountMode,
		MountPath:       s.MountPath,
		NeedsOriginator: s.NeedsOriginator,
		NeedsRunSession: s.NeedsRunSession,
		ExportFiles:     append([]string(nil), s.ExportFiles...),
		TargetChannel:   s.TargetChannel,
	}
}

func hasSendFileApprovalRequirement(analysis IntentAnalysis) bool {
	if len(analysis.RequiredActions) == 0 {
		return false
	}
	for _, action := range analysis.RequiredActions {
		if action.Action == "send_file" {
			return true
		}
	}
	return false
}

func hasMountAccessApprovalRequirement(analysis IntentAnalysis, tree *capabilities.CapabilityTree) bool {
	if len(analysis.RequiredActions) == 0 {
		return false
	}
	for _, action := range analysis.RequiredActions {
		node := tree.LookupByToolHint(action.ToolHint)
		if node != nil && node.Perms != nil && node.Perms.ScopeCheck == "mount_required" {
			return true
		}
	}
	return false
}

func appendAdditionalApproval(existing []ApprovalRequirement, req ApprovalRequirement) []ApprovalRequirement {
	if req.Type == "" {
		return existing
	}
	for i := range existing {
		if existing[i].Type == req.Type {
			existing[i] = mergeApprovalRequirement(existing[i], req)
			return existing
		}
	}
	return append(existing, req)
}

func mergeApprovalRequirement(base, extra ApprovalRequirement) ApprovalRequirement {
	if requestedLevelPriority(extra.RequestedLevel) > requestedLevelPriority(base.RequestedLevel) {
		base.RequestedLevel = extra.RequestedLevel
	}
	if base.Type == "mount_access" || extra.Type == "mount_access" {
		base.TTLMinutes = 0
	} else if extra.TTLMinutes > base.TTLMinutes {
		base.TTLMinutes = extra.TTLMinutes
	}
	if mountModePriority(extra.MountMode) > mountModePriority(base.MountMode) {
		base.MountMode = extra.MountMode
	}
	if base.MountPath == "" {
		base.MountPath = extra.MountPath
	}
	base.NeedsOriginator = base.NeedsOriginator || extra.NeedsOriginator
	base.NeedsRunSession = base.NeedsRunSession || extra.NeedsRunSession
	if len(base.ExportFiles) == 0 {
		base.ExportFiles = append([]string(nil), extra.ExportFiles...)
	}
	if base.TargetChannel == "" {
		base.TargetChannel = extra.TargetChannel
	}
	return base
}

func normalizeApprovalRequirement(req ApprovalRequirement) (ApprovalRequirement, error) {
	switch req.Type {
	case "plan_confirm", "exec_escalation", "mount_access", "data_export":
	default:
		return req, fmt.Errorf("invalid approval type: %q", req.Type)
	}

	if req.Type == "exec_escalation" && req.RequestedLevel == "" {
		req.RequestedLevel = "sandboxed"
	}
	if req.Type == "mount_access" && req.MountMode == "" {
		req.MountMode = "ro"
	}
	if req.Type == "mount_access" {
		req.TTLMinutes = 0
		return req, nil
	}
	if req.Type == "exec_escalation" && req.RequestedLevel == "full" {
		req.TTLMinutes = 0
		return req, nil
	}
	if req.TTLMinutes <= 0 {
		req.TTLMinutes = 30
	}
	if req.TTLMinutes > 480 {
		req.TTLMinutes = 480
	}
	return req, nil
}

// ApprovalSummaryFromScope 将审批结构压缩为人类可读摘要，用于方案确认卡片。
func ApprovalSummaryFromScope(scope ApprovalScope) []string {
	lines := make([]string, 0, 1+len(scope.AdditionalApprovals))
	if scope.PrimaryApproval.Type != "" {
		lines = append(lines, "主审批: "+approvalRequirementSummary(scope.PrimaryApproval, false))
	}
	for _, extra := range scope.AdditionalApprovals {
		lines = append(lines, "附加审批: "+approvalRequirementSummary(extra, true))
	}
	return lines
}

func approvalRequirementSummary(req ApprovalRequirement, conditional bool) string {
	switch req.Type {
	case "data_export":
		if len(req.ExportFiles) > 0 {
			return fmt.Sprintf("data_export（对外发送 %s）", strings.Join(req.ExportFiles, ", "))
		}
		return "data_export（对外发送文件或媒体）"
	case "mount_access":
		mode := req.MountMode
		if mode == "" {
			mode = "ro"
		}
		if req.MountPath != "" {
			if conditional {
				return fmt.Sprintf("mount_access（如超出当前作用域，本任务 %s 挂载 %s）", mode, req.MountPath)
			}
			return fmt.Sprintf("mount_access（本任务 %s 挂载 %s）", mode, req.MountPath)
		}
		return fmt.Sprintf("mount_access（本任务 %s）", mode)
	case "exec_escalation":
		level := req.RequestedLevel
		if level == "" {
			level = "sandboxed"
		}
		if req.TTLMinutes == 0 && level == "full" {
			return fmt.Sprintf("exec_escalation（%s, permanent）", level)
		}
		if req.TTLMinutes > 0 {
			return fmt.Sprintf("exec_escalation（%s, %d 分钟）", level, req.TTLMinutes)
		}
		return fmt.Sprintf("exec_escalation（%s）", level)
	case "plan_confirm":
		return "plan_confirm（执行前确认方案）"
	case ApprovalTypeResultReview:
		return "result_review（交付前最终签收）"
	default:
		return req.Type
	}
}

// approvalTypePriority 返回审批类型的优先级（数值越大越严格）。
func approvalTypePriority(t string) int {
	switch t {
	case "plan_confirm":
		return 1
	case "data_export":
		return 2
	case "mount_access":
		return 3
	case "exec_escalation":
		return 4
	default:
		return 0
	}
}

// requestedLevelPriority 返回请求权限级别的优先级（数值越大权限越高）。
// most-permissive-wins: 多个 action 的 hints 中取最高权限级别。
func requestedLevelPriority(level string) int {
	switch level {
	case "allowlist":
		return 1
	case "sandboxed":
		return 2
	case "full":
		return 3
	default:
		return 0
	}
}

// mountModePriority 返回挂载模式的优先级（数值越大权限越宽）。
// most-permissive-wins: rw > ro。
func mountModePriority(mode string) int {
	switch mode {
	case "ro":
		return 1
	case "rw":
		return 2
	default:
		return 0
	}
}
