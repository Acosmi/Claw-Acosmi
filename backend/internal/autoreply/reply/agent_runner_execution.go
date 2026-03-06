package reply

import (
	"context"
	"fmt"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
)

// TS 对照: auto-reply/reply/agent-runner-execution.ts (605L)

// ---------- DI 接口 ----------

// AgentExecutor agent 执行器接口（DI）。
// 封装 runWithModelFallback、runEmbeddedPiAgent、runCliAgent。
// 完整实现依赖 agents/ 包，延迟到集成阶段。
type AgentExecutor interface {
	// RunTurn 执行一次 agent 回合。
	RunTurn(ctx context.Context, params AgentTurnParams) (*AgentRunLoopResult, error)
}

// AgentTurnParams agent 回合参数。
type AgentTurnParams struct {
	FollowupRun           FollowupRun
	CommandBody           string
	SessionEntry          *SessionEntry
	SessionKey            string
	StorePath             string
	DefaultModel          string
	ResolvedVerboseLevel  autoreply.VerboseLevel
	IsHeartbeat           bool
	IsNewSession          bool
	BlockStreamingEnabled bool
	BlockReplyChunking    *BlockReplyChunking
	BlockStreamingBreak   string
	ExtraSystemPrompt     string

	// 回调
	OnPartialReply    func(payload autoreply.ReplyPayload)
	OnToolResult      func(payload autoreply.ReplyPayload)
	OnReasoningStream func(payload autoreply.ReplyPayload)
	OnModelSelected   func(ctx autoreply.ModelSelectedContext)
	OnBlockReply      func(payload autoreply.ReplyPayload, ctx *autoreply.BlockReplyContext)
	OnProgress        func(ctx context.Context, update runner.ProgressUpdate) runner.ProgressReportStatus
}

// BlockReplyChunking 块回复分块配置。
type BlockReplyChunking struct {
	MinChars         int
	MaxChars         int
	BreakPreference  string // "paragraph" | "newline" | "sentence"
	FlushOnParagraph bool
}

// AgentRunLoopResult agent 运行循环结果。
// TS 对照: agent-runner-execution.ts AgentRunLoopResult
type AgentRunLoopResult struct {
	Payloads                 []autoreply.ReplyPayload
	Usage                    *NormalizedUsage
	SessionResetRequired     bool
	Error                    error
	ToolResultsSent          int
	MessagingToolSentTargets []runner.MessagingToolSend
}

// ---------- Stub 实现 ----------

// StubAgentExecutor agent 执行器 stub。
type StubAgentExecutor struct{}

func (s StubAgentExecutor) RunTurn(_ context.Context, params AgentTurnParams) (*AgentRunLoopResult, error) {
	return &AgentRunLoopResult{
		Payloads: []autoreply.ReplyPayload{
			{Text: fmt.Sprintf("[stub] echo: %s", truncate(params.CommandBody, 100))},
		},
	}, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ---------- 运行逻辑 ----------

// RunAgentTurnWithFallback 执行 agent 回合（含降级）。
// TS 对照: agent-runner-execution.ts L40-605
// 当前通过 AgentExecutor DI 接口委托，核心降级/重试逻辑在集成阶段填充。
func RunAgentTurnWithFallback(ctx context.Context, executor AgentExecutor, params AgentTurnParams) (*AgentRunLoopResult, error) {
	if executor == nil {
		return &AgentRunLoopResult{}, nil
	}
	return executor.RunTurn(ctx, params)
}
