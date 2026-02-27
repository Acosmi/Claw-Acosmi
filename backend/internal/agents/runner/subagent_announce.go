package runner

// ============================================================================
// 子 Agent 通告流程 — 完整实现
// 对应 TS: agents/subagent-announce.ts → runSubagentAnnounceFlow() (L367-572)
// 隐藏依赖审计: docs/renwu/phase4-subagent-announce-audit.md
// ============================================================================

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- 依赖注入接口 (隐藏依赖 #6: 协议约定) ---

// GatewayRPC 网关远程调用接口。
// Go 端尚无 callGateway，通过接口注入解耦。
type GatewayRPC interface {
	// CallAgent 向指定 session 发送 agent 消息。
	CallAgent(ctx context.Context, params GatewayAgentParams) error
	// WaitAgentRun 等待 agent 运行完成。
	WaitAgentRun(ctx context.Context, runID string, timeoutMs int64) (*AgentWaitResult, error)
	// PatchSession 更新 session 标签。
	PatchSession(ctx context.Context, key string, label string) error
	// DeleteSession 删除 session。
	DeleteSession(ctx context.Context, key string, deleteTranscript bool) error
}

// GatewayAgentParams 网关 agent 调用参数。
type GatewayAgentParams struct {
	SessionKey     string `json:"sessionKey"`
	Message        string `json:"message"`
	Channel        string `json:"channel,omitempty"`
	AccountID      string `json:"accountId,omitempty"`
	To             string `json:"to,omitempty"`
	ThreadID       string `json:"threadId,omitempty"`
	Deliver        bool   `json:"deliver"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// AgentWaitResult agent.wait 返回结果。
type AgentWaitResult struct {
	Status    string `json:"status"` // "ok"|"error"|"timeout"
	StartedAt int64  `json:"startedAt,omitempty"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

// SessionReader 会话存储读取接口。
type SessionReader interface {
	// ReadSessionEntry 读取 session 条目。
	ReadSessionEntry(sessionKey string) *SessionEntry
	// ReadLatestAssistantReply 读取最新助手回复。
	ReadLatestAssistantReply(sessionKey string) string
}

// SessionEntry session 存储条目（部分字段）。
type SessionEntry struct {
	SessionID     string `json:"sessionId,omitempty"`
	InputTokens   int    `json:"inputTokens,omitempty"`
	OutputTokens  int    `json:"outputTokens,omitempty"`
	TotalTokens   int    `json:"totalTokens,omitempty"`
	ModelProvider string `json:"modelProvider,omitempty"`
	Model         string `json:"model,omitempty"`
	Channel       string `json:"channel,omitempty"`
	LastChannel   string `json:"lastChannel,omitempty"`
	AccountID     string `json:"accountId,omitempty"`
	To            string `json:"to,omitempty"`
	ThreadID      string `json:"threadId,omitempty"`
}

// EmbeddedRunTracker 嵌入式运行状态追踪。
type EmbeddedRunTracker interface {
	IsActive(sessionID string) bool
	WaitForEnd(sessionID string, timeoutMs int64) bool
}

// --- 主流程 ---

// RunSubagentAnnounceDeps 依赖注入。
type RunSubagentAnnounceDeps struct {
	Gateway       GatewayRPC
	Sessions      SessionReader
	RunTracker    EmbeddedRunTracker
	AnnounceQueue AnnounceQueueHandler // 可选：steer/queue 机制 (P4-GA-ANN1)
}

// AnnounceQueueHandler steer/queue 通告处理器。
// TS 对应: subagent-announce.ts → maybeQueueSubagentAnnounce()
type AnnounceQueueHandler interface {
	// MaybeQueueAnnounce 尝试通过 steer 或 queue 方式发送通告。
	// 返回 "steered"/"queued"/"none"。
	MaybeQueueAnnounce(params QueueAnnounceParams) (QueueAnnounceResult, error)
}

// QueueAnnounceParams 队列通告参数。
type QueueAnnounceParams struct {
	RequesterSessionKey string
	TriggerMessage      string
	SummaryLine         string
	RequesterOrigin     *DeliveryContext
}

// QueueAnnounceResult 队列通告结果。
type QueueAnnounceResult string

const (
	QueueResultSteered QueueAnnounceResult = "steered"
	QueueResultQueued  QueueAnnounceResult = "queued"
	QueueResultNone    QueueAnnounceResult = "none"
)

// RunSubagentAnnounceFlow 执行子 Agent 结果通告流程。
// TS 对应: subagent-announce.ts → runSubagentAnnounceFlow() (L367-572)
func RunSubagentAnnounceFlow(params RunSubagentAnnounceParams, deps RunSubagentAnnounceDeps) (didAnnounce bool, err error) {
	log := slog.Default().With("subsystem", "subagent-announce")
	shouldDeleteChild := params.Cleanup == "delete"
	ctx := context.Background()

	defer func() {
		// finally: patch label + delete session (best-effort)
		if params.Label != "" && deps.Gateway != nil {
			_ = deps.Gateway.PatchSession(ctx, params.ChildSessionKey, params.Label)
		}
		if shouldDeleteChild && deps.Gateway != nil {
			_ = deps.Gateway.DeleteSession(ctx, params.ChildSessionKey, true)
		}
	}()

	// 1. 获取子 session ID
	childSessionID := ""
	if deps.Sessions != nil {
		if entry := deps.Sessions.ReadSessionEntry(params.ChildSessionKey); entry != nil {
			childSessionID = strings.TrimSpace(entry.SessionID)
		}
	}

	// 2. 等待嵌入式运行结束 (隐藏依赖 #3: 事件等待)
	settleTimeoutMs := params.TimeoutMs
	if settleTimeoutMs < 1 {
		settleTimeoutMs = 1
	}
	if settleTimeoutMs > 120_000 {
		settleTimeoutMs = 120_000
	}

	if childSessionID != "" && deps.RunTracker != nil && deps.RunTracker.IsActive(childSessionID) {
		settled := deps.RunTracker.WaitForEnd(childSessionID, settleTimeoutMs)
		if !settled && deps.RunTracker.IsActive(childSessionID) {
			shouldDeleteChild = false
			return false, nil
		}
	}

	reply := params.RoundOneReply
	outcome := params.Outcome

	// 3. 等待运行完成（通过 gateway RPC）
	if reply == "" && params.WaitForCompletion && deps.Gateway != nil {
		result, waitErr := deps.Gateway.WaitAgentRun(ctx, params.ChildRunID, settleTimeoutMs)
		if waitErr != nil {
			log.Warn("agent.wait 失败", "err", waitErr)
		} else if result != nil {
			switch result.Status {
			case "timeout":
				outcome = &SubagentRunOutcome{Status: "timeout"}
			case "error":
				outcome = &SubagentRunOutcome{Status: "error", Error: result.Error}
			case "ok":
				outcome = &SubagentRunOutcome{Status: "ok"}
			}
			if result.StartedAt > 0 && params.StartedAt == 0 {
				params.StartedAt = result.StartedAt
			}
			if result.EndedAt > 0 && params.EndedAt == 0 {
				params.EndedAt = result.EndedAt
			}
		}
		if deps.Sessions != nil {
			reply = deps.Sessions.ReadLatestAssistantReply(params.ChildSessionKey)
		}
	}

	// 4. 读取最新回复
	if reply == "" && deps.Sessions != nil {
		reply = deps.Sessions.ReadLatestAssistantReply(params.ChildSessionKey)
	}
	if strings.TrimSpace(reply) == "" && deps.Sessions != nil {
		// 重试读取（基于截止时间，间隔 300ms，上限 min(timeoutMs, 15000)）
		maxWaitMs := params.TimeoutMs
		if maxWaitMs > 15000 {
			maxWaitMs = 15000
		}
		if maxWaitMs < 0 {
			maxWaitMs = 0
		}
		deadline := time.Now().Add(time.Duration(maxWaitMs) * time.Millisecond)
		for time.Now().Before(deadline) && strings.TrimSpace(reply) == "" {
			time.Sleep(300 * time.Millisecond)
			reply = deps.Sessions.ReadLatestAssistantReply(params.ChildSessionKey)
		}
	}

	// 4.5 尝试解析 ThoughtResult JSON（向后兼容：失败保持纯文本行为）
	var thought *ThoughtResult
	if trimmedReply := strings.TrimSpace(reply); trimmedReply != "" {
		thought = ParseThoughtResult(trimmedReply)
		if thought != nil {
			log.Info("子 agent 返回结构化 ThoughtResult",
				"status", thought.Status, "contractID", thought.ContractID)
		}
	}

	// 5. 如果仍无回复且子运行活跃，跳过
	if strings.TrimSpace(reply) == "" && childSessionID != "" &&
		deps.RunTracker != nil && deps.RunTracker.IsActive(childSessionID) {
		shouldDeleteChild = false
		return false, nil
	}

	if outcome == nil {
		outcome = &SubagentRunOutcome{Status: "unknown"}
	}

	// 5.5 将 ThoughtResult 附加到 outcome 并映射状态
	if thought != nil {
		outcome.ThoughtResult = thought
		switch thought.Status {
		case ThoughtCompleted:
			outcome.Status = "ok"
		case ThoughtFailed:
			outcome.Status = "error"
		case ThoughtTimeout:
			outcome.Status = "timeout"
		case ThoughtNeedsAuth, ThoughtBlocked, ThoughtPartial:
			outcome.Status = "ok" // 正常终止，需后续动作
		}
		// 用 ThoughtResult.Result（人类可读）替代原始 JSON blob
		if thought.Result != "" {
			reply = thought.Result
		}
	}

	// 6. 构建统计行
	stats := SubagentStats{SessionKey: params.ChildSessionKey, SessionID: childSessionID}
	if deps.Sessions != nil {
		if entry := deps.Sessions.ReadSessionEntry(params.ChildSessionKey); entry != nil {
			stats.InputTokens = entry.InputTokens
			stats.OutputTokens = entry.OutputTokens
			stats.TotalTokens = entry.TotalTokens
		}
	}
	if params.StartedAt > 0 && params.EndedAt > 0 {
		stats.RuntimeMs = params.EndedAt - params.StartedAt
	}
	statsLine := BuildSubagentStatsLine(stats)

	// 7. 构建状态标签
	statusLabel := "finished with unknown status"
	switch outcome.Status {
	case "ok":
		statusLabel = "completed successfully"
	case "timeout":
		statusLabel = "timed out"
	case "error":
		errMsg := outcome.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		statusLabel = fmt.Sprintf("failed: %s", errMsg)
	}

	// 8. 构建触发消息
	announceType := params.AnnounceType
	if announceType == "" {
		announceType = "subagent task"
	}
	taskLabel := params.Label
	if taskLabel == "" {
		taskLabel = params.Task
	}
	if taskLabel == "" {
		taskLabel = "task"
	}
	replyText := reply
	if strings.TrimSpace(replyText) == "" {
		replyText = "(no output)"
	}

	triggerMessage := strings.Join([]string{
		fmt.Sprintf(`A %s "%s" just %s.`, announceType, taskLabel, statusLabel),
		"",
		"Findings:",
		replyText,
		"",
		statsLine,
		"",
		"Summarize this naturally for the user. Keep it brief (1-2 sentences). Flow it into the conversation naturally.",
		fmt.Sprintf("Do not mention technical details like tokens, stats, or that this was a %s.", announceType),
		"You can respond with NO_REPLY if no announcement is needed (e.g., internal task with no user-facing result).",
	}, "\n")

	// 9. 尝试 steer/queue 通告 (P4-GA-ANN1)
	if deps.AnnounceQueue != nil {
		qResult, qErr := deps.AnnounceQueue.MaybeQueueAnnounce(QueueAnnounceParams{
			RequesterSessionKey: params.RequesterSessionKey,
			TriggerMessage:      triggerMessage,
			SummaryLine:         taskLabel,
			RequesterOrigin:     params.RequesterOrigin,
		})
		if qErr != nil {
			log.Warn("announce queue 失败, 回退直接发送", "err", qErr)
		} else if qResult == QueueResultSteered || qResult == QueueResultQueued {
			return true, nil
		}
	}

	// 10. 直接发送到主 Agent
	if deps.Gateway != nil {
		gwParams := GatewayAgentParams{
			SessionKey:     params.RequesterSessionKey,
			Message:        triggerMessage,
			Deliver:        true,
			IdempotencyKey: uuid.New().String(),
		}
		// 从 RequesterOrigin 填充频道路由信息（F-ANN2）
		if origin := params.RequesterOrigin; origin != nil {
			gwParams.Channel = origin.Channel
			gwParams.AccountID = origin.AccountID
			gwParams.To = origin.To
			if origin.ThreadID != "" {
				gwParams.ThreadID = origin.ThreadID
			}
		}
		sendErr := deps.Gateway.CallAgent(ctx, gwParams)
		if sendErr != nil {
			log.Error("子 Agent 通告发送失败", "err", sendErr)
			return false, nil // best-effort (隐藏依赖 #7)
		}
	}

	return true, nil
}
