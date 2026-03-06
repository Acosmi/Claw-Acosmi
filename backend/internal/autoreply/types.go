package autoreply

import "context"

// TS 对照: auto-reply/types.ts

// ReplyMediaItem 回复媒体项（base64 + MIME）。
type ReplyMediaItem struct {
	MediaBase64   string `json:"mediaBase64,omitempty"`
	MediaMimeType string `json:"mediaMimeType,omitempty"`
}

// BlockReplyContext 阻塞回复上下文。
// TS 对照: types.ts L4-7
type BlockReplyContext struct {
	Ctx       context.Context
	TimeoutMs int
}

// ModelSelectedContext 模型选定上下文。
// TS 对照: types.ts L10-14
type ModelSelectedContext struct {
	Provider   string
	Model      string
	ThinkLevel string
}

// ReplyPayload 回复载荷。
// TS 对照: types.ts L46-59
type ReplyPayload struct {
	Text           string           `json:"text,omitempty"`
	MediaURL       string           `json:"mediaUrl,omitempty"`
	MediaURLs      []string         `json:"mediaUrls,omitempty"`
	MediaItems     []ReplyMediaItem `json:"mediaItems,omitempty"`
	MediaBase64    string           `json:"mediaBase64,omitempty"`
	MediaMimeType  string           `json:"mediaMimeType,omitempty"`
	ReplyToID      string           `json:"replyToId,omitempty"`
	ReplyToTag     bool             `json:"replyToTag,omitempty"`
	ReplyToCurrent bool             `json:"replyToCurrent,omitempty"`
	AudioAsVoice   bool             `json:"audioAsVoice,omitempty"`
	IsError        bool             `json:"isError,omitempty"`
	ChannelData    map[string]any   `json:"channelData,omitempty"`
}

// GetReplyOptions 获取回复选项。
// TS 对照: types.ts L16-44
type GetReplyOptions struct {
	RunID             string
	Ctx               context.Context
	OnAgentRunStart   func(runID string)
	OnReplyStart      func()
	OnTypingCleanup   func()
	IsHeartbeat       bool
	OnPartialReply    func(payload ReplyPayload)
	OnReasoningStream func(payload ReplyPayload)
	OnBlockReply      func(payload ReplyPayload, ctx *BlockReplyContext)
	OnToolResult      func(payload ReplyPayload)
	OnModelSelected   func(ctx ModelSelectedContext)
	// OnToolEvent 结构化工具事件回调（可选）。
	// 用于频道广播工具名、参数、结果。参数类型: runner.ToolEvent（通过 any 传递避免循环导入）。
	OnToolEvent any
	// OnProgress 中间进度回调（可选）。
	// 参数类型: func(context.Context, runner.ProgressUpdate) runner.ProgressReportStatus（通过 any 传递避免循环导入）。
	OnProgress            any
	DisableBlockStreaming bool
	BlockReplyTimeoutMs   int
	SkillFilter           []string
	HasRepliedRef         *BoolRef
}

// BoolRef 可变布尔引用（用于跨回调跟踪状态）。
type BoolRef struct {
	Value bool
}
