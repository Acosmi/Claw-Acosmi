package runner

import (
	"context"

	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ============================================================================
// Agent Runner 类型定义
// 对应 TS: agents/pi-embedded-runner/types.ts (81L)
// ============================================================================

// ---------- Embedded PI Agent 元数据 ----------

// EmbeddedPiAgentMeta 嵌入式 PI Agent 运行元数据。
type EmbeddedPiAgentMeta struct {
	SessionID       string                `json:"sessionId"`
	Provider        string                `json:"provider"`
	Model           string                `json:"model"`
	CompactionCount int                   `json:"compactionCount,omitempty"`
	Usage           *EmbeddedPiAgentUsage `json:"usage,omitempty"`
}

// EmbeddedPiAgentUsage Token 使用量统计。
type EmbeddedPiAgentUsage struct {
	Input      int `json:"input,omitempty"`
	Output     int `json:"output,omitempty"`
	CacheRead  int `json:"cacheRead,omitempty"`
	CacheWrite int `json:"cacheWrite,omitempty"`
	Total      int `json:"total,omitempty"`
}

// ---------- 运行元数据 ----------

// EmbeddedPiRunMeta 运行元数据。
type EmbeddedPiRunMeta struct {
	DurationMs         int64                `json:"durationMs"`
	AgentMeta          *EmbeddedPiAgentMeta `json:"agentMeta,omitempty"`
	Aborted            bool                 `json:"aborted,omitempty"`
	SystemPromptReport interface{}          `json:"systemPromptReport,omitempty"`
	Error              *EmbeddedPiRunError  `json:"error,omitempty"`
	StopReason         string               `json:"stopReason,omitempty"`
	PendingToolCalls   []PendingToolCall    `json:"pendingToolCalls,omitempty"`
}

// EmbeddedPiRunError 运行错误详情。
type EmbeddedPiRunError struct {
	Kind    string `json:"kind"` // "context_overflow" | "compaction_failure" | "role_ordering" | "image_size"
	Message string `json:"message"`
}

// PendingToolCall 待处理的工具调用。
type PendingToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ---------- 媒体块 ----------

// MediaBlock 工具产出的媒体块（图片等）。
type MediaBlock struct {
	MimeType string `json:"mimeType"`
	Base64   string `json:"base64"`
}

// ---------- 进度汇报 ----------

// ProgressUpdate report_progress 的结构化进度负载。
type ProgressUpdate struct {
	Summary string `json:"summary"`
	Percent int    `json:"percent,omitempty"`
	Phase   string `json:"phase,omitempty"`
}

// ProgressReportStatus 远程进度投递结果。
// 默认零值表示仅更新本地实时界面。
type ProgressReportStatus struct {
	RemoteDelivered bool   `json:"remoteDelivered,omitempty"`
	Throttled       bool   `json:"throttled,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ---------- 运行结果 ----------

// EmbeddedPiRunResult 嵌入式 PI 运行结果。
type EmbeddedPiRunResult struct {
	Payloads                 []RunPayload        `json:"payloads,omitempty"`
	Meta                     EmbeddedPiRunMeta   `json:"meta"`
	DidSendViaMessagingTool  bool                `json:"didSendViaMessagingTool,omitempty"`
	MessagingToolSentTargets []MessagingToolSend `json:"messagingToolSentTargets,omitempty"`
}

// RunPayload 运行输出负载。
type RunPayload struct {
	Text          string       `json:"text,omitempty"`
	MediaURL      string       `json:"mediaUrl,omitempty"`
	MediaURLs     []string     `json:"mediaUrls,omitempty"`
	MediaItems    []MediaBlock `json:"mediaItems,omitempty"`
	MediaBase64   string       `json:"mediaBase64,omitempty"`
	MediaMimeType string       `json:"mediaMimeType,omitempty"`
	ReplyToID     string       `json:"replyToId,omitempty"`
	IsError       bool         `json:"isError,omitempty"`
}

// MessagingToolSend 消息工具发送目标。
// TS 对照: pi-embedded-messaging.ts L3-8 MessagingToolSend
type MessagingToolSend struct {
	Tool      string `json:"tool"`
	Provider  string `json:"provider"`
	AccountID string `json:"accountId,omitempty"`
	To        string `json:"to,omitempty"`
}

// ---------- Compaction 结果 ----------

// EmbeddedPiCompactResult 压缩结果。
type EmbeddedPiCompactResult struct {
	OK        bool                     `json:"ok"`
	Compacted bool                     `json:"compacted"`
	Reason    string                   `json:"reason,omitempty"`
	Result    *EmbeddedPiCompactDetail `json:"result,omitempty"`
}

// EmbeddedPiCompactDetail 压缩详情。
type EmbeddedPiCompactDetail struct {
	Summary          string      `json:"summary"`
	FirstKeptEntryID string      `json:"firstKeptEntryId"`
	TokensBefore     int         `json:"tokensBefore"`
	TokensAfter      int         `json:"tokensAfter,omitempty"`
	Details          interface{} `json:"details,omitempty"`
}

// ---------- Sandbox 信息 ----------

// EmbeddedSandboxInfo 沙箱环境信息。
type EmbeddedSandboxInfo struct {
	Enabled             bool                   `json:"enabled"`
	WorkspaceDir        string                 `json:"workspaceDir,omitempty"`
	WorkspaceAccess     string                 `json:"workspaceAccess,omitempty"` // "none" | "ro" | "rw"
	AgentWorkspaceMount string                 `json:"agentWorkspaceMount,omitempty"`
	BrowserBridgeURL    string                 `json:"browserBridgeUrl,omitempty"`
	BrowserNoVncURL     string                 `json:"browserNoVncUrl,omitempty"`
	HostBrowserAllowed  bool                   `json:"hostBrowserAllowed,omitempty"`
	Elevated            *SandboxElevatedConfig `json:"elevated,omitempty"`
}

// SandboxElevatedConfig 沙箱提权配置。
type SandboxElevatedConfig struct {
	Allowed      bool   `json:"allowed"`
	DefaultLevel string `json:"defaultLevel"` // "on" | "off" | "ask" | "full"
}

// ---------- 运行参数 ----------

// RunEmbeddedPiAgentParams RunEmbeddedPiAgent 的调用参数。
// 完整参数列表对应 TS run.ts 顶部的 params 类型。
type RunEmbeddedPiAgentParams struct {
	SessionID           string                  `json:"sessionId"`
	SessionKey          string                  `json:"sessionKey,omitempty"`
	AgentID             string                  `json:"agentId,omitempty"`
	SessionFile         string                  `json:"sessionFile"`
	WorkspaceDir        string                  `json:"workspaceDir"`
	AgentDir            string                  `json:"agentDir,omitempty"`
	Prompt              string                  `json:"prompt"`
	Provider            string                  `json:"provider"`
	Model               string                  `json:"model,omitempty"`
	TimeoutMs           int64                   `json:"timeoutMs"`
	RunID               string                  `json:"runId"`
	ExtraSystemPrompt   string                  `json:"extraSystemPrompt,omitempty"`
	SkillFilter         []string                `json:"skillFilter,omitempty"`
	Config              *types.OpenAcosmiConfig `json:"-"`
	ThinkLevel          string                  `json:"thinkLevel,omitempty"`
	AuthProfileID       string                  `json:"authProfileId,omitempty"`
	AuthProfileIDSource string                  `json:"authProfileIdSource,omitempty"`
	FallbackModels      []string                `json:"fallbackModels,omitempty"`
	// 权限拒绝事件回调 — 通知网关广播 WebSocket 事件
	OnPermissionDenied func(tool, detail string) `json:"-"`
	// OnPermissionDeniedWithContext 在权限拒绝时回传更完整的审批上下文。
	OnPermissionDeniedWithContext func(notice PermissionDeniedNotice) `json:"-"`
	// WaitForApproval 阻塞等待提权审批结果（由 server.go 注入）
	WaitForApproval func(ctx context.Context) bool `json:"-"`
	// SecurityLevelFunc 动态获取当前有效安全级别（由 server.go 注入）
	SecurityLevelFunc func() string `json:"-"`
	// MountRequestsFunc 动态获取活跃 grant 的临时挂载请求（由 server.go 注入）。
	// Phase 3.4: escalation grant → ToolExecParams.MountRequests → CLI --mount。
	MountRequestsFunc func(runID string) []MountRequestForSandbox `json:"-"`
	// DelegationContract 子 agent 委托合约（可选）——传递到 AttemptParams → ToolExecParams。
	// nil = 主 agent 无合约限制。
	DelegationContract *DelegationContract `json:"-"`
	// PromptMode 提示词模式（"full"|"minimal"|"none"，空 = "full"）。
	// 子智能体使用 "minimal" 跳过 Self-Update/Messaging/Voice 等无关段落。
	PromptMode string `json:"promptMode,omitempty"`
	// OnCoderEvent 子智能体对话频道事件回调（由 server.go SpawnSubagent 注入）。
	// event: "task_received" | "turn_complete" | "tool_use" | "complete"
	// data: 事件相关数据（text, toolName, status 等）
	OnCoderEvent func(event string, data map[string]interface{}) `json:"-"`
	// OnToolEvent 结构化工具事件回调（由 server.go 注入）。
	// 工具执行前后调用，用于频道广播工具名称、参数摘要和结果摘要。
	// nil = 不广播工具事件（默认，向后兼容）。
	OnToolEvent func(event ToolEvent) `json:"-"`
	// OnProgress 中间进度回调（可选）。
	// 用于将 report_progress 同步到显式开启的远程渠道；nil = 仅本地实时事件。
	OnProgress func(ctx context.Context, update ProgressUpdate) ProgressReportStatus `json:"-"`
	// AgentChannel 异步消息通道（可选，nil = 不支持求助通道）。
	// Phase 4: 三级指挥体系 — 子智能体执行中异步向主智能体求助。
	AgentChannel *AgentChannel `json:"-"`
	// AgentType 子智能体类型（可选，空字符串 = 主智能体）。
	// 值: "coder" / "argus" / "media"
	// 传递到 AttemptParams.AgentType，用于按类型注入子智能体专属工具。
	AgentType string `json:"agentType,omitempty"`
	// SuppressTranscript Bug#11: 在 model fallback 场景下跳过 transcript 持久化。
	SuppressTranscript bool `json:"-"`
	// Attachments 用户附件 content blocks（用于 transcript 持久化 + LLM 多模态注入）。
	Attachments []session.ContentBlock `json:"-"`
}

// ---------- ToolEvent 工具事件 ----------

// ToolEvent 结构化工具事件，用于频道广播。
type ToolEvent struct {
	Phase    string `json:"phase"`    // "start" | "end"
	ToolName string `json:"toolName"` // bash, edit, read, glob, ...
	ToolID   string `json:"toolId"`   // tool call ID
	Args     string `json:"args"`     // 参数摘要（截断后，rune-safe）
	Result   string `json:"result"`   // 结果摘要（仅 end 阶段，截断后）
	IsError  bool   `json:"isError"`  // 是否失败
	Duration int64  `json:"duration"` // 执行耗时 ms（仅 end 阶段）
}

// ---------- Subagent Announce ----------

// DeliveryContext 投递上下文（频道/账户/线程信息）。
// TS 对应: utils/delivery-context.ts → DeliveryContext
type DeliveryContext struct {
	Channel   string `json:"channel,omitempty"`
	AccountID string `json:"accountId,omitempty"`
	To        string `json:"to,omitempty"`
	ThreadID  string `json:"threadId,omitempty"`
}

// SubagentRunOutcome 子 Agent 运行结果状态。
type SubagentRunOutcome struct {
	Status        string         `json:"status"` // "ok" | "error" | "timeout" | "unknown"
	Error         string         `json:"error,omitempty"`
	ThoughtResult *ThoughtResult `json:"thoughtResult,omitempty"` // 结构化子 agent 返回（可选）
}

// SubagentAnnounceType 子 Agent 通告类型。
type SubagentAnnounceType string

const (
	SubagentAnnounceTask SubagentAnnounceType = "subagent task"
	SubagentAnnounceCron SubagentAnnounceType = "cron job"
)

// RunSubagentAnnounceParams runSubagentAnnounceFlow 参数。
type RunSubagentAnnounceParams struct {
	ChildSessionKey     string               `json:"childSessionKey"`
	ChildRunID          string               `json:"childRunId"`
	RequesterSessionKey string               `json:"requesterSessionKey"`
	RequesterOrigin     *DeliveryContext     `json:"requesterOrigin,omitempty"`
	RequesterDisplayKey string               `json:"requesterDisplayKey"`
	Task                string               `json:"task"`
	TimeoutMs           int64                `json:"timeoutMs"`
	Cleanup             string               `json:"cleanup"` // "delete" | "keep"
	RoundOneReply       string               `json:"roundOneReply,omitempty"`
	WaitForCompletion   bool                 `json:"waitForCompletion,omitempty"`
	StartedAt           int64                `json:"startedAt,omitempty"`
	EndedAt             int64                `json:"endedAt,omitempty"`
	Label               string               `json:"label,omitempty"`
	Outcome             *SubagentRunOutcome  `json:"outcome,omitempty"`
	AnnounceType        SubagentAnnounceType `json:"announceType,omitempty"`
}
