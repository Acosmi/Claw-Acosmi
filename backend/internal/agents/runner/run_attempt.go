package runner

// ============================================================================
// Embedded PI Runner — DI 接口定义
// 对应 TS: pi-embedded-runner/run.ts 中的外部依赖
// ============================================================================

import (
	"context"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// --- 模型解析 ---

// ResolvedModel 已解析的模型。
type ResolvedModel struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"contextWindow,omitempty"`
}

// ModelResolveResult 模型解析结果。
type ModelResolveResult struct {
	Model         *ResolvedModel
	Error         string
	AuthStorage   AuthKeyStorage
	ModelRegistry interface{}
}

// ModelResolver 模型解析接口。
type ModelResolver interface {
	ResolveModel(provider, modelID, agentDir string, cfg *types.OpenAcosmiConfig) ModelResolveResult
	ResolveContextWindowInfo(cfg *types.OpenAcosmiConfig, provider, modelID string, contextWindow int) ContextWindowInfo
}

// AuthKeyStorage 运行时 API key 存储。
type AuthKeyStorage interface {
	SetRuntimeApiKey(provider, key string)
}

// --- Auth Profile ---

// AuthProfileStore auth profile 存储。
type AuthProfileStore interface {
	GetProfiles() map[string]AuthProfile
	GetApiKeyForModel(model *ResolvedModel, cfg *types.OpenAcosmiConfig, profileID string, agentDir string) (*ApiKeyInfo, error)
	ResolveProfileOrder(cfg *types.OpenAcosmiConfig, provider string, preferred string) []string
	MarkFailure(profileID string, reason string, cfg *types.OpenAcosmiConfig, agentDir string) error
	MarkGood(provider, profileID, agentDir string) error
	MarkUsed(profileID, agentDir string) error
	IsInCooldown(profileID string) bool
}

// AuthProfile 认证配置文件。
type AuthProfile struct {
	Provider string `json:"provider"`
	Key      string `json:"key,omitempty"`
}

// ApiKeyInfo API key 解析结果。
type ApiKeyInfo struct {
	ApiKey    string `json:"apiKey,omitempty"`
	Mode      string `json:"mode,omitempty"` // "key"|"aws-sdk"|...
	ProfileID string `json:"profileId,omitempty"`
}

// --- Attempt Runner ---

// AttemptResult 单次尝试结果。
type AttemptResult struct {
	Aborted               bool                `json:"aborted,omitempty"`
	PromptError           error               `json:"-"`
	TimedOut              bool                `json:"timedOut,omitempty"`
	SessionIDUsed         string              `json:"sessionIdUsed,omitempty"`
	LastAssistant         *AssistantMessage   `json:"lastAssistant,omitempty"`
	AssistantTexts        []string            `json:"assistantTexts,omitempty"`
	MediaBlocks           []MediaBlock        `json:"mediaBlocks,omitempty"`
	ToolMetas             []interface{}       `json:"toolMetas,omitempty"`
	LastToolError         string              `json:"lastToolError,omitempty"`
	CompactionCount       int                 `json:"compactionCount,omitempty"`
	AttemptUsage          *NormalizedUsage    `json:"attemptUsage,omitempty"`
	MessagesSnapshot      []interface{}       `json:"messagesSnapshot,omitempty"`
	SystemPromptReport    interface{}         `json:"systemPromptReport,omitempty"`
	ClientToolCall        *ClientToolCall     `json:"clientToolCall,omitempty"`
	CloudCodeAssistFmtErr bool                `json:"cloudCodeAssistFormatError,omitempty"`
	DidSendViaMessaging   bool                `json:"didSendViaMessagingTool,omitempty"`
	MessagingSentTexts    []string            `json:"messagingToolSentTexts,omitempty"`
	MessagingSentTargets  []MessagingToolSend `json:"messagingToolSentTargets,omitempty"`
}

// AssistantMessage 助手消息。
type AssistantMessage struct {
	Provider     string      `json:"provider,omitempty"`
	Model        string      `json:"model,omitempty"`
	StopReason   string      `json:"stopReason,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
	Usage        interface{} `json:"usage,omitempty"`
}

// ClientToolCall 客户端工具调用。
type ClientToolCall struct {
	Name   string      `json:"name"`
	Params interface{} `json:"params"`
}

// AttemptRunner 嵌入式 PI attempt 执行器。
type AttemptRunner interface {
	RunAttempt(ctx context.Context, params AttemptParams) (*AttemptResult, error)
}

// AttemptParams attempt 执行参数（简化版）。
type AttemptParams struct {
	SessionID          string                    `json:"sessionId"`
	SessionKey         string                    `json:"sessionKey"`
	SessionFile        string                    `json:"sessionFile"`
	WorkspaceDir       string                    `json:"workspaceDir"`
	AgentDir           string                    `json:"agentDir"`
	Config             *types.OpenAcosmiConfig   `json:"-"`
	Prompt             string                    `json:"prompt"`
	Provider           string                    `json:"provider"`
	ModelID            string                    `json:"modelId"`
	Model              *ResolvedModel            `json:"model"`
	ThinkLevel         string                    `json:"thinkLevel"`
	TimeoutMs          int64                     `json:"timeoutMs"`
	RunID              string                    `json:"runId"`
	ExtraSystemPrompt  string                    `json:"extraSystemPrompt,omitempty"`
	OnPermissionDenied func(tool, detail string) // 权限拒绝事件回调
	// WaitForApproval 阻塞等待提权审批结果。返回 true=审批通过，false=拒绝/超时。
	// 由 server.go 注入，内部监听 EscalationManager 状态变化。
	WaitForApproval func(ctx context.Context) bool
	// SecurityLevelFunc 动态获取当前有效安全级别（含临时提权）。
	// 由 server.go 注入，内部调用 EscalationManager.GetEffectiveLevel()。
	SecurityLevelFunc func() string
	// MountRequestsFunc 动态获取临时挂载请求（Phase 3.4）。
	MountRequestsFunc func() []MountRequestForSandbox
	// DelegationContract 委托合约约束（可选，nil = 主 agent 无合约限制）。
	DelegationContract *DelegationContract
	// PromptMode 提示词模式（"full"|"minimal"|"none"，空 = "full"）。
	// 子智能体使用 "minimal" 跳过 Self-Update/Messaging/Voice 等无关段落。
	PromptMode string `json:"promptMode,omitempty"`
	// OnToolEvent 结构化工具事件回调（可选，nil = 不广播）。
	// 工具执行前后调用，用于频道广播工具名称、参数摘要和结果摘要。
	OnToolEvent func(event ToolEvent)
	// OnProgress 中间进度回调（可选，nil = 仅本地实时事件）。
	OnProgress func(ctx context.Context, update ProgressUpdate) ProgressReportStatus
	// AgentChannel 异步消息通道（可选，nil = 不支持求助通道）。
	// Phase 4: 三级指挥体系 — 子智能体执行中异步向主智能体求助。
	AgentChannel *AgentChannel
	// AgentType 子智能体类型（可选，空字符串 = 主智能体）。
	// 值: "coder" / "argus" / "media"
	// 用于在 RunAttempt 中按类型注入子智能体专属工具。
	AgentType string `json:"agentType,omitempty"`
	// SuppressTranscript Bug#11: 在 model fallback 场景下跳过 transcript 持久化，
	// 避免失败 attempt 的数据污染后续 attempt 的历史。
	SuppressTranscript bool `json:"-"`
}

// --- Compaction ---

// CompactionResult 压缩结果。
type CompactionResult struct {
	Compacted bool   `json:"compacted"`
	Reason    string `json:"reason,omitempty"`
}

// CompactionRunner session 压缩执行器。
type CompactionRunner interface {
	CompactSession(ctx context.Context, params CompactionParams) (*CompactionResult, error)
}

// CompactionParams 压缩参数（简化版）。
type CompactionParams struct {
	SessionID     string `json:"sessionId"`
	SessionKey    string `json:"sessionKey"`
	SessionFile   string `json:"sessionFile"`
	WorkspaceDir  string `json:"workspaceDir"`
	AgentDir      string `json:"agentDir"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	AuthProfileID string `json:"authProfileId,omitempty"`
}

// --- Embedded Run Dependencies ---

// EmbeddedRunDeps 嵌入式运行的所有依赖。
type EmbeddedRunDeps struct {
	ModelResolver    ModelResolver
	AuthStore        AuthProfileStore
	AttemptRunner    AttemptRunner
	CompactionRunner CompactionRunner
}
