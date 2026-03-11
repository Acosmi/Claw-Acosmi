package reply

import (
	"context"
	"math/rand"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
)

// TS 对照: auto-reply/reply/get-reply-run.ts (435L)

// BareSessionResetPrompt 空重置会话提示。
const BareSessionResetPrompt = "A new session was started via /new or /reset. Greet the user in your configured persona, if one is provided. Be yourself - use your defined voice, mannerisms, and mood. Keep it to 1-3 sentences and ask what they want to do. If the runtime model differs from default_model in the system prompt, mention the default model. Do not mention internal steps, files, tools, or reasoning."

// PreparedReplyParams 准备好的回复参数。
type PreparedReplyParams struct {
	Ctx                    *autoreply.MsgContext
	AgentID                string
	AgentDir               string
	CommandAuthorized      bool
	Command                CommandContext
	CommandSource          string
	AllowTextCommands      bool
	Directives             InlineDirectives
	DefaultActivation      GroupActivation
	ResolvedThinkLevel     autoreply.ThinkLevel
	ResolvedVerboseLevel   autoreply.VerboseLevel
	ResolvedReasoningLevel autoreply.ReasoningLevel
	ResolvedElevatedLevel  autoreply.ElevatedLevel
	ExecOverrides          *ExecOverrides
	ElevatedEnabled        bool
	ElevatedAllowed        bool
	BlockStreamingEnabled  bool
	BlockReplyChunking     *BlockReplyChunking
	BlockStreamingBreak    string
	Provider               string
	Model                  string
	PerMessageQueueMode    QueueMode
	PerMessageQueueOptions *QueueOptions
	Typing                 *TypingController
	Opts                   *autoreply.GetReplyOptions
	DefaultProvider        string
	DefaultModel           string
	TimeoutMs              int
	IsNewSession           bool
	ResetTriggered         bool
	SystemSent             bool
	SessionEntry           *SessionEntry
	SessionKey             string
	SessionID              string
	StorePath              string
	WorkspaceDir           string
	AbortedLastRun         bool

	// DI 接口
	MemoryFlusher MemoryFlusher
	AgentExecutor AgentExecutor
	TypingMode    TypingMode
}

// RunPreparedReply 运行准备好的回复。
// TS 对照: get-reply-run.ts runPreparedReply (L109-434)
// 编排：消息体准备 → 会话提示 → 队列配置 → RunReplyAgent。
func RunPreparedReply(ctx context.Context, params PreparedReplyParams) ([]autoreply.ReplyPayload, error) {
	msgCtx := params.Ctx
	if msgCtx == nil {
		return nil, nil
	}

	isHeartbeat := params.Opts != nil && params.Opts.IsHeartbeat

	// 1. 解析消息体
	baseBody := msgCtx.BodyStripped
	if baseBody == "" {
		baseBody = msgCtx.Body
	}
	baseBodyTrimmed := strings.TrimSpace(baseBody)
	if baseBodyTrimmed == "" && len(msgCtx.Attachments) == 0 {
		if params.Typing != nil {
			_ = params.Typing.OnReplyStart()
			params.Typing.Cleanup()
		}
		return []autoreply.ReplyPayload{
			{Text: "I didn't receive any text in your message. Please resend or add a caption."},
		}, nil
	}
	// 附件-only 场景：确保有占位 prompt 供 LLM 处理
	if baseBodyTrimmed == "" && len(msgCtx.Attachments) > 0 {
		baseBody = "[用户发送了附件]"
		baseBodyTrimmed = baseBody
	}

	// 2. 空重置检测
	rawBody := strings.TrimSpace(msgCtx.CommandBody)
	if rawBody == "" {
		rawBody = strings.TrimSpace(msgCtx.RawBody)
	}
	if rawBody == "" {
		rawBody = strings.TrimSpace(msgCtx.Body)
	}
	isBareReset := rawBody == "/new" || rawBody == "/reset"
	isBareSessionReset := params.IsNewSession && (baseBodyTrimmed == "" || isBareReset)
	commandBody := baseBody
	if isBareSessionReset {
		commandBody = BareSessionResetPrompt
	}

	// 3. 构建 followup run
	sessionID := params.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// 解析 transcript 文件路径（storePath 目录 + sessionID.jsonl）
	var sessionFile string
	if params.StorePath != "" {
		sessionFile = filepath.Join(filepath.Dir(params.StorePath), sessionID+".jsonl")
	}

	followupRun := FollowupRun{
		Prompt:               commandBody,
		OriginatingChannel:   msgCtx.OriginatingChannel,
		OriginatingTo:        msgCtx.OriginatingTo,
		OriginatingAccountID: msgCtx.AccountID,
		OriginatingThreadID:  msgCtx.MessageThreadID,
		OriginatingChatType:  msgCtx.ChatType,
		Attachments:          msgCtx.Attachments,
		Run: FollowupRunParams{
			SessionID:         sessionID,
			SessionKey:        params.SessionKey,
			AgentID:           params.AgentID,
			SessionFile:       sessionFile,
			WorkspaceDir:      params.WorkspaceDir,
			MessageProvider:   strings.ToLower(strings.TrimSpace(msgCtx.Provider)),
			AgentAccountID:    msgCtx.AccountID,
			Provider:          params.Provider,
			Model:             params.Model,
			SkillFilter:       cloneSkillFilter(params.Opts),
			ThinkLevel:        params.ResolvedThinkLevel,
			VerboseLevel:      params.ResolvedVerboseLevel,
			ReasoningLevel:    params.ResolvedReasoningLevel,
			TimeoutMs:         params.TimeoutMs,
			BlockReplyBreak:   params.BlockStreamingBreak,
			ExtraSystemPrompt: resolveExtraSystemPrompt(msgCtx),
			RunID:             resolveRunID(params.Opts),
		},
	}

	// 4. 调用 RunReplyAgent
	return RunReplyAgent(ctx, RunReplyAgentParams{
		CommandBody:           commandBody,
		FollowupRun:           followupRun,
		QueueKey:              params.SessionKey,
		Opts:                  params.Opts,
		Typing:                params.Typing,
		SessionEntry:          params.SessionEntry,
		SessionKey:            params.SessionKey,
		StorePath:             params.StorePath,
		DefaultModel:          params.DefaultModel,
		ResolvedVerboseLevel:  params.ResolvedVerboseLevel,
		IsNewSession:          params.IsNewSession,
		IsHeartbeat:           isHeartbeat,
		BlockStreamingEnabled: params.BlockStreamingEnabled,
		BlockReplyChunking:    params.BlockReplyChunking,
		BlockStreamingBreak:   params.BlockStreamingBreak,
		TypingMode:            params.TypingMode,
		MemoryFlusher:         params.MemoryFlusher,
		AgentExecutor:         params.AgentExecutor,
	})
}

// ---------- 辅助函数 ----------

func resolveExtraSystemPrompt(ctx *autoreply.MsgContext) string {
	parts := []string{}
	if ctx.GroupSystemPrompt != "" {
		parts = append(parts, strings.TrimSpace(ctx.GroupSystemPrompt))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// resolveRunID 安全提取 RunID（nil-safe）。
func resolveRunID(opts *autoreply.GetReplyOptions) string {
	if opts != nil {
		return opts.RunID
	}
	return ""
}

func cloneSkillFilter(opts *autoreply.GetReplyOptions) []string {
	if opts == nil || len(opts.SkillFilter) == 0 {
		return nil
	}
	return append([]string(nil), opts.SkillFilter...)
}

func generateSessionID() string {
	// 简化版 UUID 生成
	return "session-" + randomHex(8)
}

func randomHex(n int) string {
	const chars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
