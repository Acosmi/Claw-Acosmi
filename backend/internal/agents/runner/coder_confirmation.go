package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------- Coder 确认流 ----------

// CoderConfirmPreview 包含用于前端预览的工具调用详情。
type CoderConfirmPreview struct {
	FilePath  string `json:"filePath,omitempty"`
	OldString string `json:"oldString,omitempty"` // edit: 待替换文本片段
	NewString string `json:"newString,omitempty"` // edit: 新文本片段
	Content   string `json:"content,omitempty"`   // write: 内容预览 (截断 500 字符)
	Command   string `json:"command,omitempty"`   // bash: 命令文本
	LineCount int    `json:"lineCount,omitempty"` // write: 内容行数
}

// CoderConfirmationRequest 表示一个等待用户确认的 coder 操作。
type CoderConfirmationRequest struct {
	ID          string               `json:"id"`
	ToolName    string               `json:"toolName"` // "edit"|"write"|"bash"
	Args        json.RawMessage      `json:"args"`     // 原始参数
	Preview     *CoderConfirmPreview `json:"preview"`  // 预览数据
	CreatedAtMs int64                `json:"createdAtMs"`
	ExpiresAtMs int64                `json:"expiresAtMs"`
}

// CoderConfirmBroadcastFunc 广播回调（解耦 runner 与 gateway）。
type CoderConfirmBroadcastFunc func(event string, payload interface{})

// CoderConfirmationManager 管理 coder 工具调用确认流。
// 当 coder 触发可确认工具 (edit/write/bash) 时：
//  1. 广播 "coder.confirm.requested" 给前端
//  2. 阻塞等待用户决策（allow/deny）或超时
//  3. 前端通过 "coder.confirm.resolve" RPC 回调
//
// 为 nil 时完全跳过确认（兼容现有行为）。
type CoderConfirmationManager struct {
	mu        sync.Mutex
	pending   map[string]chan string // id → decision ("allow"/"deny")
	broadcast CoderConfirmBroadcastFunc
	timeout   time.Duration // 默认 60s
}

// NewCoderConfirmationManager 创建确认管理器。
func NewCoderConfirmationManager(broadcastFn CoderConfirmBroadcastFunc, timeout time.Duration) *CoderConfirmationManager {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &CoderConfirmationManager{
		pending:   make(map[string]chan string),
		broadcast: broadcastFn,
		timeout:   timeout,
	}
}

// RequestConfirmation 请求用户确认一个 coder 操作。
// 阻塞直到用户决策、超时或 ctx 取消。
// 返回 true 表示用户批准，false 表示拒绝/超时/取消。
func (m *CoderConfirmationManager) RequestConfirmation(ctx context.Context, toolName string, args json.RawMessage) (bool, error) {
	now := time.Now()
	req := CoderConfirmationRequest{
		ID:          uuid.New().String(),
		ToolName:    toolName,
		Args:        args,
		Preview:     extractCoderPreview(toolName, args),
		CreatedAtMs: now.UnixMilli(),
		ExpiresAtMs: now.Add(m.timeout).UnixMilli(),
	}

	ch := make(chan string, 1)

	m.mu.Lock()
	m.pending[req.ID] = ch
	m.mu.Unlock()

	// 广播确认请求到前端
	if m.broadcast != nil {
		m.broadcast("coder.confirm.requested", req)
	}

	slog.Debug("coder confirmation requested",
		"id", req.ID,
		"tool", toolName,
	)

	// 等待用户决策、超时或 ctx 取消
	timer := time.NewTimer(m.timeout)
	defer timer.Stop()

	var decision string
	select {
	case decision = <-ch:
		// 用户已决策
	case <-timer.C:
		decision = "deny"
		slog.Info("coder confirmation timed out, auto-denying",
			"id", req.ID,
			"tool", toolName,
		)
	case <-ctx.Done():
		decision = "deny"
		slog.Debug("coder confirmation cancelled by context",
			"id", req.ID,
		)
	}

	// 清理 pending
	m.mu.Lock()
	delete(m.pending, req.ID)
	m.mu.Unlock()

	// 广播决策结果
	if m.broadcast != nil {
		m.broadcast("coder.confirm.resolved", map[string]interface{}{
			"id":       req.ID,
			"decision": decision,
			"ts":       time.Now().UnixMilli(),
		})
	}

	return decision == "allow", nil
}

// ResolveConfirmation 处理前端的确认决策回调。
// 由 WebSocket RPC "coder.confirm.resolve" 调用。
func (m *CoderConfirmationManager) ResolveConfirmation(id, decision string) error {
	if decision != "allow" && decision != "deny" {
		return fmt.Errorf("invalid decision: %q (expected allow/deny)", decision)
	}

	m.mu.Lock()
	ch, ok := m.pending[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("no pending confirmation with id: %s", id)
	}

	// 非阻塞写入（channel 有 1 缓冲）
	select {
	case ch <- decision:
		slog.Debug("coder confirmation resolved",
			"id", id,
			"decision", decision,
		)
	default:
		// channel 已被写入（超时或重复调用），忽略
	}

	return nil
}

// (Phase 2A: isCoderConfirmable 已删除 — 审批逻辑移至 Ask 规则和 Argus 审批)

// extractCoderPreview 从工具参数中提取预览数据。
func extractCoderPreview(toolName string, args json.RawMessage) *CoderConfirmPreview {
	var parsed map[string]interface{}
	if err := json.Unmarshal(args, &parsed); err != nil {
		return nil
	}

	preview := &CoderConfirmPreview{}

	switch toolName {
	case "edit":
		if v, ok := parsed["filePath"].(string); ok {
			preview.FilePath = v
		}
		if v, ok := parsed["oldString"].(string); ok {
			preview.OldString = truncatePreview(v, 500)
		}
		if v, ok := parsed["newString"].(string); ok {
			preview.NewString = truncatePreview(v, 500)
		}
	case "write":
		if v, ok := parsed["filePath"].(string); ok {
			preview.FilePath = v
		}
		if v, ok := parsed["content"].(string); ok {
			preview.Content = truncatePreview(v, 500)
			preview.LineCount = countLines(v)
		}
	case "bash":
		if v, ok := parsed["command"].(string); ok {
			preview.Command = v
		}
	}

	return preview
}

// truncatePreview 截断预览文本到指定长度。
func truncatePreview(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// countLines 统计文本行数。
func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}
