package runner

import (
	"context"

	"github.com/anthropic/open-acosmi/pkg/types"
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
	Text      string   `json:"text,omitempty"`
	MediaURL  string   `json:"mediaUrl,omitempty"`
	MediaURLs []string `json:"mediaUrls,omitempty"`
	ReplyToID string   `json:"replyToId,omitempty"`
	IsError   bool     `json:"isError,omitempty"`
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
	Config              *types.OpenAcosmiConfig `json:"-"`
	ThinkLevel          string                  `json:"thinkLevel,omitempty"`
	AuthProfileID       string                  `json:"authProfileId,omitempty"`
	AuthProfileIDSource string                  `json:"authProfileIdSource,omitempty"`
	FallbackModels      []string                `json:"fallbackModels,omitempty"`
	// 权限拒绝事件回调 — 通知网关广播 WebSocket 事件
	OnPermissionDenied func(tool, detail string) `json:"-"`
	// WaitForApproval 阻塞等待提权审批结果（由 server.go 注入）
	WaitForApproval func(ctx context.Context) bool `json:"-"`
	// SecurityLevelFunc 动态获取当前有效安全级别（由 server.go 注入）
	SecurityLevelFunc func() string `json:"-"`
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
