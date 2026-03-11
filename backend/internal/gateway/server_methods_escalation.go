package gateway

// server_methods_escalation.go — security.escalation.* 方法处理器
// P2: 智能体即时授权 + UI 弹窗 + 自动降权
//
// 方法:
//   - security.escalation.request  — 智能体请求临时提权
//   - security.escalation.resolve  — 用户审批/拒绝
//   - security.escalation.status   — 查询提权状态
//   - security.escalation.audit    — 查询审计日志
//   - security.escalation.revoke   — 手动撤销活跃提权

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// EscalationHandlers 返回 security.escalation.* 方法处理器映射。
func EscalationHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"security.escalation.request": handleEscalationRequest,
		"security.escalation.resolve": handleEscalationResolve,
		"security.escalation.status":  handleEscalationStatus,
		"security.escalation.audit":   handleEscalationAudit,
		"security.escalation.revoke":  handleEscalationRevoke,
	}
}

// ---------- security.escalation.request ----------
// 智能体请求临时提权。

func handleEscalationRequest(ctx *MethodHandlerContext) {
	mgr := ctx.Context.EscalationMgr
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "escalation manager not initialized"))
		return
	}

	level, _ := ctx.Params["level"].(string)
	level = strings.TrimSpace(level)
	if level == "" {
		level = "full"
	}

	reason, _ := ctx.Params["reason"].(string)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Agent requests elevated permissions"
	}

	runID, _ := ctx.Params["runId"].(string)
	sessionID, _ := ctx.Params["sessionId"].(string)
	originatorChatID, _ := ctx.Params["originatorChatId"].(string)
	originatorUserID, _ := ctx.Params["originatorUserId"].(string)

	ttlMinutes := 30
	if ttlRaw, ok := ctx.Params["ttlMinutes"].(float64); ok && ttlRaw > 0 {
		ttlMinutes = int(ttlRaw)
	}
	mountRequests := parseMountRequests(ctx.Params["mountRequests"])

	// 生成请求 ID
	id := generateEscalationID()

	if err := mgr.RequestEscalation(id, level, reason, runID, sessionID, originatorChatID, originatorUserID, ttlMinutes, mountRequests...); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, err.Error()))
		return
	}

	status := mgr.GetStatus()
	resolvedTTL := ttlMinutes
	taskScoped := false
	if status.Pending != nil && status.Pending.ID == id {
		resolvedTTL = status.Pending.TTLMinutes
		taskScoped = status.Pending.TaskScoped
	}

	ctx.Respond(true, map[string]interface{}{
		"id":             id,
		"requestedLevel": level,
		"ttlMinutes":     resolvedTTL,
		"taskScoped":     taskScoped,
		"mountRequests":  mountRequests,
		"status":         "pending",
	}, nil)
}

// ---------- security.escalation.resolve ----------
// 用户审批或拒绝提权请求。

func handleEscalationResolve(ctx *MethodHandlerContext) {
	mgr := ctx.Context.EscalationMgr
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "escalation manager not initialized"))
		return
	}

	approve, _ := ctx.Params["approve"].(bool)

	ttlMinutes := 0
	if ttlRaw, ok := ctx.Params["ttlMinutes"].(float64); ok && ttlRaw > 0 {
		ttlMinutes = int(ttlRaw)
	}

	if err := mgr.ResolveEscalation(approve, ttlMinutes); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, err.Error()))
		return
	}

	status := "denied"
	if approve {
		status = "approved"
	}

	ctx.Respond(true, map[string]interface{}{
		"status":  status,
		"approve": approve,
	}, nil)
}

// ---------- security.escalation.status ----------
// 查询当前提权状态。

func handleEscalationStatus(ctx *MethodHandlerContext) {
	mgr := ctx.Context.EscalationMgr
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "escalation manager not initialized"))
		return
	}

	status := mgr.GetStatus()
	ctx.Respond(true, status, nil)
}

// ---------- security.escalation.audit ----------
// 查询审计日志。

func handleEscalationAudit(ctx *MethodHandlerContext) {
	mgr := ctx.Context.EscalationMgr
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "escalation manager not initialized"))
		return
	}

	limit := 50
	if limitRaw, ok := ctx.Params["limit"].(float64); ok && limitRaw > 0 {
		limit = int(limitRaw)
	}
	if limit > 200 {
		limit = 200
	}

	if mgr.auditLogger == nil {
		ctx.Respond(true, map[string]interface{}{
			"entries": []interface{}{},
			"total":   0,
		}, nil)
		return
	}

	entries, err := mgr.auditLogger.ReadRecent(limit)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read audit log: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"entries": entries,
		"total":   len(entries),
	}, nil)
}

// ---------- security.escalation.revoke ----------
// 手动撤销活跃提权。

func handleEscalationRevoke(ctx *MethodHandlerContext) {
	mgr := ctx.Context.EscalationMgr
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "escalation manager not initialized"))
		return
	}

	mgr.ManualRevoke()

	ctx.Respond(true, map[string]interface{}{
		"status": "revoked",
	}, nil)
}

// ---------- 辅助 ----------

func generateEscalationID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "esc_" + hex.EncodeToString(buf)
}

func parseMountRequests(raw interface{}) []MountRequest {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]MountRequest, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		hostPath, _ := m["hostPath"].(string)
		hostPath = strings.TrimSpace(hostPath)
		if hostPath == "" || !filepath.IsAbs(hostPath) {
			continue
		}
		mode, _ := m["mountMode"].(string)
		mode = strings.ToLower(strings.TrimSpace(mode))
		if mode != "rw" {
			mode = "ro"
		}
		out = append(out, MountRequest{
			HostPath:  filepath.Clean(hostPath),
			MountMode: mode,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
