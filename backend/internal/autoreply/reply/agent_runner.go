package reply

import (
	"context"
	"fmt"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
)

// TS 对照: auto-reply/reply/agent-runner.ts (526L)

// RunReplyAgentParams 运行回复 agent 参数。
type RunReplyAgentParams struct {
	CommandBody           string
	FollowupRun           FollowupRun
	QueueKey              string
	Opts                  *autoreply.GetReplyOptions
	Typing                *TypingController
	SessionEntry          *SessionEntry
	SessionKey            string
	StorePath             string
	DefaultModel          string
	AgentCfgContextTokens int
	ResolvedVerboseLevel  autoreply.VerboseLevel
	IsNewSession          bool
	IsHeartbeat           bool
	BlockStreamingEnabled bool
	BlockReplyChunking    *BlockReplyChunking
	BlockStreamingBreak   string
	TypingMode            TypingMode

	// DI 接口
	MemoryFlusher MemoryFlusher
	AgentExecutor AgentExecutor
}

// RunReplyAgent 主 agent 回复入口。
// TS 对照: agent-runner.ts runReplyAgent (L29-526)
// 编排：内存冲刷 → agent 执行 → 载荷构建 → followup 调度。
func RunReplyAgent(ctx context.Context, params RunReplyAgentParams) ([]autoreply.ReplyPayload, error) {
	typing := params.Typing
	if typing == nil {
		return nil, fmt.Errorf("typing controller is required")
	}

	signaler := NewTypingSignaler(typing, params.TypingMode, params.IsHeartbeat)

	// 1. typing 信号
	if err := signaler.SignalRunStart(); err != nil {
		return nil, fmt.Errorf("typing signal failed: %w", err)
	}

	// 2. 内存冲刷
	activeEntry, err := RunMemoryFlushIfNeeded(params.MemoryFlusher, MemoryFlushParams{
		FollowupRun:           params.FollowupRun,
		SessionEntry:          params.SessionEntry,
		SessionKey:            params.SessionKey,
		StorePath:             params.StorePath,
		DefaultModel:          params.DefaultModel,
		AgentCfgContextTokens: params.AgentCfgContextTokens,
		ResolvedVerboseLevel:  params.ResolvedVerboseLevel,
		IsHeartbeat:           params.IsHeartbeat,
	})
	if err != nil {
		// 内存冲刷失败不应阻止 agent 运行
		activeEntry = params.SessionEntry
	}

	// 3. agent 执行
	turnResult, err := RunAgentTurnWithFallback(ctx, params.AgentExecutor, AgentTurnParams{
		FollowupRun:           params.FollowupRun,
		CommandBody:           params.CommandBody,
		SessionEntry:          activeEntry,
		SessionKey:            params.SessionKey,
		StorePath:             params.StorePath,
		DefaultModel:          params.DefaultModel,
		ResolvedVerboseLevel:  params.ResolvedVerboseLevel,
		IsHeartbeat:           params.IsHeartbeat,
		IsNewSession:          params.IsNewSession,
		BlockStreamingEnabled: params.BlockStreamingEnabled,
		BlockReplyChunking:    params.BlockReplyChunking,
		BlockStreamingBreak:   params.BlockStreamingBreak,
		ExtraSystemPrompt:     params.FollowupRun.Run.ExtraSystemPrompt,
		OnPartialReply:        resolvePartialReplyCallback(params.Opts),
		OnToolResult:          resolveToolResultCallback(params.Opts),
		OnReasoningStream:     resolveReasoningCallback(params.Opts),
		OnModelSelected:       resolveModelSelectedCallback(params.Opts),
		OnBlockReply:          resolveBlockReplyCallback(params.Opts),
		OnProgress:            resolveProgressCallback(params.Opts),
	})
	if err != nil {
		typing.Cleanup()
		return nil, fmt.Errorf("agent turn failed: %w", err)
	}

	if turnResult == nil {
		typing.Cleanup()
		return nil, nil
	}

	// 4. 构建最终载荷
	built := BuildReplyPayloads(BuildReplyPayloadsParams{
		Payloads:                 turnResult.Payloads,
		IsHeartbeat:              params.IsHeartbeat,
		BlockStreamingEnabled:    params.BlockStreamingEnabled,
		MessagingToolSentTargets: turnResult.MessagingToolSentTargets,
	})

	// 5. 用量行追加
	if params.ResolvedVerboseLevel != "" && params.ResolvedVerboseLevel != autoreply.VerboseOff {
		usageLine := FormatResponseUsageLine(UsageLineParams{
			Usage: turnResult.Usage,
		})
		if usageLine != "" {
			built.ReplyPayloads = AppendUsageLine(built.ReplyPayloads, usageLine)
		}
	}

	// 6. 触发 typing（如果有可渲染载荷）
	if len(built.ReplyPayloads) > 0 {
		_ = SignalTypingIfNeeded(built.ReplyPayloads, signaler)
	}

	typing.Cleanup()
	return built.ReplyPayloads, nil
}

// ---------- 回调解析 ----------

func resolvePartialReplyCallback(opts *autoreply.GetReplyOptions) func(autoreply.ReplyPayload) {
	if opts != nil && opts.OnPartialReply != nil {
		return opts.OnPartialReply
	}
	return nil
}

func resolveToolResultCallback(opts *autoreply.GetReplyOptions) func(autoreply.ReplyPayload) {
	if opts != nil && opts.OnToolResult != nil {
		return opts.OnToolResult
	}
	return nil
}

func resolveReasoningCallback(opts *autoreply.GetReplyOptions) func(autoreply.ReplyPayload) {
	if opts != nil && opts.OnReasoningStream != nil {
		return opts.OnReasoningStream
	}
	return nil
}

func resolveModelSelectedCallback(opts *autoreply.GetReplyOptions) func(autoreply.ModelSelectedContext) {
	if opts != nil && opts.OnModelSelected != nil {
		return opts.OnModelSelected
	}
	return nil
}

func resolveBlockReplyCallback(opts *autoreply.GetReplyOptions) func(autoreply.ReplyPayload, *autoreply.BlockReplyContext) {
	if opts != nil && opts.OnBlockReply != nil {
		return opts.OnBlockReply
	}
	return nil
}

func resolveProgressCallback(opts *autoreply.GetReplyOptions) func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus {
	if opts == nil || opts.OnProgress == nil {
		return nil
	}
	cb, ok := opts.OnProgress.(func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus)
	if !ok {
		return nil
	}
	return cb
}
