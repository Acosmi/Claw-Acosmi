package runner

// result_approval.go — 三级指挥体系 Phase 3: 最终交付门控
//
// 质量审核通过后，将结果呈现给用户做最终签收。
// 用户批准 → 返回正常结果；拒绝 → 返回 "用户要求修改" 给主智能体。
//
// 复用 PlanConfirmationManager 的阻塞 channel 模式。
// WebSocket 事件: result.approve.requested / result.approve.resolved

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// resultApprovalEntry 内部 pending 条目（含过期时间和决策 channel）。
type resultApprovalEntry struct {
	ch        chan ResultApprovalDecision
	expiresAt time.Time
	req       ResultApprovalRequest
}

// ---------- 请求 / 决策类型 ----------

// ResultApprovalRequest 结果签收请求。
type ResultApprovalRequest struct {
	ID            string            `json:"id"`
	OriginalTask  string            `json:"originalTask"`
	ContractID    string            `json:"contractId"`
	Result        string            `json:"result"`
	Artifacts     *ThoughtArtifacts `json:"artifacts,omitempty"`
	ReviewSummary string            `json:"reviewSummary,omitempty"`
	HighRisk      bool              `json:"highRisk,omitempty"` // 高风险任务（涉及删除/系统配置/提权），超时时自动拒绝而非批准
	CreatedAtMs   int64             `json:"createdAtMs"`
	ExpiresAtMs   int64             `json:"expiresAtMs"`
	Workflow      ApprovalWorkflow  `json:"workflow,omitempty"`
}

// ResultApprovalDecision 用户对结果的签收决策。
type ResultApprovalDecision struct {
	Action   string `json:"action"`             // "approve" | "reject"
	Feedback string `json:"feedback,omitempty"` // action=reject 时的拒绝理由 / 修改要求
}

// ---------- Manager ----------

// ResultApprovalManager 结果签收管理器。
// 当子智能体结果通过质量审核后:
//  1. 广播 "result.approve.requested" 给前端（WebSocket）
//  2. 阻塞等待用户签收决策（approve/reject）或超时
//  3. 前端通过 "result.approve.resolve" RPC 回调
//
// 为 nil 时完全跳过结果签收（兼容现有行为）。
type ResultApprovalManager struct {
	mu      sync.Mutex
	pending map[string]*resultApprovalEntry // id → pending entry (含过期时间)

	broadcast    CoderConfirmBroadcastFunc // 复用广播类型（解耦 runner ↔ gateway）
	remoteNotify ResultApprovalRemoteNotifyFunc
	timeout      time.Duration // 默认 3min（签收比方案确认更快）
	closeOnce    sync.Once     // 防止 Close() 重复调用 panic
	cleanupDone  chan struct{} // 关闭时停止 TTL 清理 goroutine
}

type ResultApprovalRemoteNotifyFunc func(req ResultApprovalRequest, sessionKey string)

// NewResultApprovalManager 创建结果签收管理器。
func NewResultApprovalManager(
	broadcastFn CoderConfirmBroadcastFunc,
	remoteNotifyFn ResultApprovalRemoteNotifyFunc,
	timeout time.Duration,
) *ResultApprovalManager {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	m := &ResultApprovalManager{
		pending:      make(map[string]*resultApprovalEntry),
		broadcast:    broadcastFn,
		remoteNotify: remoteNotifyFn,
		timeout:      timeout,
		cleanupDone:  make(chan struct{}),
	}
	go m.ttlCleanupLoop()
	return m
}

// RequestResultApproval 请求用户签收子智能体执行结果。
// 阻塞直到用户决策、超时或 ctx 取消。
func (m *ResultApprovalManager) RequestResultApproval(ctx context.Context, req ResultApprovalRequest) (ResultApprovalDecision, error) {
	return m.RequestResultApprovalWithSessionKey(ctx, req, "")
}

func (m *ResultApprovalManager) RequestResultApprovalWithSessionKey(ctx context.Context, req ResultApprovalRequest, sessionKey string) (ResultApprovalDecision, error) {
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

	ch := make(chan ResultApprovalDecision, 1)

	m.mu.Lock()
	m.pending[req.ID] = &resultApprovalEntry{
		ch:        ch,
		expiresAt: time.UnixMilli(req.ExpiresAtMs),
		req:       req,
	}
	m.mu.Unlock()

	// 广播结果签收请求到前端
	if m.broadcast != nil {
		m.broadcast("result.approve.requested", req)
	}
	broadcastApprovalWorkflow(m.broadcast, req.Workflow, "result.approve.requested", req.ID)

	if m.remoteNotify != nil {
		m.remoteNotify(req, sessionKey)
	}

	slog.Debug("result approval requested",
		"id", req.ID,
		"contractId", req.ContractID,
		"task", truncate(req.OriginalTask, 80),
	)

	// 等待用户决策、超时或 ctx 取消
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()

	var decision ResultApprovalDecision
	select {
	case decision = <-ch:
		// 用户已决策
	case <-timer.C:
		// 超时行为按风险等级区分：
		// - 高风险（涉及删除/系统配置）：超时自动拒绝，防止未审查的危险操作被应用
		// - 低风险：超时自动批准（用户未响应，默认接受结果更合理）
		if req.HighRisk {
			decision = ResultApprovalDecision{Action: "reject", Feedback: "timeout_auto_rejected_high_risk"}
			slog.Warn("result approval timed out for high-risk task, auto-rejecting",
				"id", req.ID,
			)
		} else {
			decision = ResultApprovalDecision{Action: "approve", Feedback: "timeout_auto_approved"}
			slog.Info("result approval timed out, auto-approving",
				"id", req.ID,
			)
		}
	case <-ctx.Done():
		if req.HighRisk {
			decision = ResultApprovalDecision{Action: "reject", Feedback: "context_cancelled_high_risk"}
		} else {
			decision = ResultApprovalDecision{Action: "approve", Feedback: "context_cancelled"}
		}
		slog.Debug("result approval cancelled by context",
			"id", req.ID,
			"highRisk", req.HighRisk,
		)
	}

	// 清理 pending
	m.mu.Lock()
	delete(m.pending, req.ID)
	m.mu.Unlock()

	resolvedWorkflow := req.Workflow
	if resolvedWorkflow.ID != "" {
		resolvedWorkflow = resolvedWorkflow.MarkStageResolved(ApprovalTypeResultReview, req.ID, decision.Action)
	}

	// 广播决策结果
	if m.broadcast != nil {
		m.broadcast("result.approve.resolved", map[string]interface{}{
			"id":       req.ID,
			"decision": decision,
			"ts":       time.Now().UnixMilli(),
			"workflow": resolvedWorkflow,
		})
	}
	broadcastApprovalWorkflow(m.broadcast, resolvedWorkflow, "result.approve.resolved", req.ID)

	return decision, nil
}

// ResolveResultApproval 处理前端的结果签收决策回调。
// 由 WebSocket RPC "result.approve.resolve" 调用。
func (m *ResultApprovalManager) ResolveResultApproval(id string, decision ResultApprovalDecision) error {
	if decision.Action != "approve" && decision.Action != "reject" {
		return fmt.Errorf("invalid decision action: %q (expected approve/reject)", decision.Action)
	}

	m.mu.Lock()
	entry, ok := m.pending[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("no pending result approval with id: %s", id)
	}

	// 非阻塞写入（channel 有 1 缓冲）
	select {
	case entry.ch <- decision:
		slog.Debug("result approval resolved",
			"id", id,
			"action", decision.Action,
		)
	default:
		// channel 已被写入（超时或重复调用），忽略
	}

	return nil
}

func (m *ResultApprovalManager) PendingRequest(id string) (ResultApprovalRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.pending[id]
	if !ok || entry == nil {
		return ResultApprovalRequest{}, false
	}
	return entry.req, true
}

// Close 关闭管理器，停止 TTL 清理 goroutine。安全支持重复调用。
func (m *ResultApprovalManager) Close() {
	m.closeOnce.Do(func() {
		close(m.cleanupDone)
	})
}

// Timeout 返回签收超时时间。
func (m *ResultApprovalManager) Timeout() time.Duration {
	return m.timeout
}

// PendingCount 返回当前等待签收的请求数（用于监控）。
func (m *ResultApprovalManager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}

// ---------- TTL 清理 ----------

func (m *ResultApprovalManager) ttlCleanupLoop() {
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

func (m *ResultApprovalManager) cleanupExpired() {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, entry := range m.pending {
		if now.Before(entry.expiresAt) {
			continue // 未过期，跳过
		}
		// 过期 — 按风险等级区分
		var ttlDecision ResultApprovalDecision
		if entry.req.HighRisk {
			ttlDecision = ResultApprovalDecision{Action: "reject", Feedback: "ttl_expired_high_risk"}
		} else {
			ttlDecision = ResultApprovalDecision{Action: "approve", Feedback: "ttl_expired"}
		}
		select {
		case entry.ch <- ttlDecision:
			slog.Debug("result approval TTL expired", "id", id, "action", ttlDecision.Action)
		default:
			// channel 已有决策（timer 先触发），只清理 map
		}
		delete(m.pending, id)
	}
}
