package reply

import (
	"context"
	"time"

	agentsession "github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/internal/session"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// TS 对照: auto-reply/reply/followup-runner.ts (286L) — 骨架版
// 完整实现延迟到 Window 2（依赖 agent-runner 和 model-fallback）。

// FollowupRun 后续运行参数。
// TS 对照: queue/types.ts
type FollowupRun struct {
	Prompt               string
	MessageID            string // Provider 消息 ID，用于去重
	SummaryLine          string // 摘要行，用于 summarize drop policy
	EnqueuedAt           int64  // 入队时间戳 (Unix ms)
	Run                  FollowupRunParams
	OriginatingChannel   string
	OriginatingTo        string
	OriginatingAccountID string
	OriginatingThreadID  string
	OriginatingChatType  string
	Attachments          []agentsession.ContentBlock // 用户附件 content blocks（用于 transcript 持久化）
}

// FollowupRunParams 后续运行核心参数。
type FollowupRunParams struct {
	SessionID         string
	SessionKey        string
	AgentID           string
	AgentDir          string // TS: agentDir
	MessageProvider   string
	AgentAccountID    string
	GroupID           string
	GroupChannel      string
	GroupSpace        string
	SenderID          string
	SenderName        string
	SenderUsername    string
	SenderE164        string
	SessionFile       string
	WorkspaceDir      string
	Config            *types.OpenAcosmiConfig       // 系统配置
	SkillsSnapshot    *session.SessionSkillSnapshot // 技能快照
	ExtraSystemPrompt string
	OwnerNumbers      []string
	EnforceFinalTag   bool
	Provider          string
	Model             string
	SkillFilter       []string
	AuthProfileID     string
	AuthProfileIDSrc  string
	ThinkLevel        autoreply.ThinkLevel
	VerboseLevel      autoreply.VerboseLevel
	ReasoningLevel    autoreply.ReasoningLevel
	ElevatedLevel     autoreply.ElevatedLevel // TS: elevatedLevel
	ExecOverrides     *ExecOverrides          // exec 覆盖配置
	BashElevated      *BashElevatedConfig     // TS: bashElevated 完整结构
	TimeoutMs         int
	BlockReplyBreak   string
	RunID             string // Bug#11: 从 dispatch_inbound 传入，确保全链路可追踪
}

// BashElevatedConfig bash 提权配置。
// TS 对照: FollowupRun.run.bashElevated
type BashElevatedConfig struct {
	Enabled      bool
	Allowed      bool
	DefaultLevel autoreply.ElevatedLevel
}

// FollowupRunnerParams 后续运行器创建参数。
type FollowupRunnerParams struct {
	Typing                *TypingController
	TypingMode            TypingMode
	SessionKey            string
	StorePath             string
	DefaultModel          string
	AgentCfgContextTokens int
	IsHeartbeat           bool
	Ctx                   context.Context // 父 context（支持取消/超时）
	TimeoutMs             int             // 单次运行超时毫秒（默认 300000 = 5min）
}

// FollowupRunner 后续运行器类型。
type FollowupRunner func(queued FollowupRun) error

// NewFollowupRunner 创建后续运行器。
// TS 对照: followup-runner.ts L29-285
// HEALTH-D5: 使用带超时的 context 替代 context.TODO()。
func NewFollowupRunner(params FollowupRunnerParams) FollowupRunner {
	signaler := NewTypingSignaler(params.Typing, params.TypingMode, params.IsHeartbeat)
	_ = signaler // Window 2 使用

	// 解析父 context 和超时
	parentCtx := params.Ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	timeoutMs := params.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 300_000 // 默认 5 分钟
	}

	return func(queued FollowupRun) error {
		defer params.Typing.MarkRunComplete()

		// 使用 queued 中的超时覆盖默认值
		effectiveTimeout := timeoutMs
		if queued.Run.TimeoutMs > 0 {
			effectiveTimeout = queued.Run.TimeoutMs
		}

		// 派生带超时的子 context
		runCtx, cancel := context.WithTimeout(parentCtx, time.Duration(effectiveTimeout)*time.Millisecond)
		defer cancel()

		_, err := RunReplyAgent(runCtx, RunReplyAgentParams{
			CommandBody:   queued.Prompt,
			FollowupRun:   queued,
			QueueKey:      params.SessionKey,
			Typing:        params.Typing,
			SessionKey:    params.SessionKey,
			StorePath:     params.StorePath,
			DefaultModel:  params.DefaultModel,
			IsHeartbeat:   params.IsHeartbeat,
			TypingMode:    params.TypingMode,
			MemoryFlusher: StubMemoryFlusher{},
			AgentExecutor: StubAgentExecutor{},
		})
		return err
	}
}
