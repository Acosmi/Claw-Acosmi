package session

import (
	"encoding/json"
)

// SessionEntry 会话条目 (与 TS SessionEntry 1:1 对齐，types.ts L25-96)。
// 从 gateway.SessionEntry 迁移至独立包以避免循环导入。
type SessionEntry struct {
	SessionKey string `json:"sessionKey"`
	MainKey    string `json:"mainKey,omitempty"` // 主会话 key（子会话指向父级）
	Label      string `json:"label,omitempty"`
	CreatedAt  int64  `json:"createdAt,omitempty"`
	UpdatedAt  int64  `json:"updatedAt,omitempty"`

	// 标识
	SessionId    string `json:"sessionId,omitempty"`   // UUID 会话标识
	SessionFile  string `json:"sessionFile,omitempty"` // 自定义 transcript 文件路径
	DisplayName  string `json:"displayName,omitempty"`
	Subject      string `json:"subject,omitempty"`
	ChatType     string `json:"chatType,omitempty"`     // "group" | "channel" | ""
	GroupChannel string `json:"groupChannel,omitempty"` // 群组频道名
	GroupId      string `json:"groupId,omitempty"`      // 群组 ID
	Space        string `json:"space,omitempty"`        // 工作空间标识
	SpawnedBy    string `json:"spawnedBy,omitempty"`    // 由谁创建
	Channel      string `json:"channel,omitempty"`      // 频道标识 (telegram, slack 等)

	// 来源
	Origin *SessionOrigin `json:"origin,omitempty"`

	// 状态标记
	SystemSent     bool `json:"systemSent,omitempty"`
	AbortedLastRun bool `json:"abortedLastRun,omitempty"`

	// Heartbeat 去重
	LastHeartbeatText   string `json:"lastHeartbeatText,omitempty"`
	LastHeartbeatSentAt int64  `json:"lastHeartbeatSentAt,omitempty"`

	// 思考/推理等级
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
	VerboseLevel   string `json:"verboseLevel,omitempty"`
	ReasoningLevel string `json:"reasoningLevel,omitempty"`
	ElevatedLevel  string `json:"elevatedLevel,omitempty"`

	// TTS
	TtsAuto string `json:"ttsAuto,omitempty"` // TtsAutoMode

	// 执行环境
	ExecHost     string `json:"execHost,omitempty"`
	ExecSecurity string `json:"execSecurity,omitempty"`
	ExecAsk      string `json:"execAsk,omitempty"`
	ExecNode     string `json:"execNode,omitempty"`

	// 发送策略: "allow" | "deny"
	SendPolicy string `json:"sendPolicy,omitempty"`

	// 响应用量模式: "on" | "off" | "tokens" | "full"
	ResponseUsage string `json:"responseUsage,omitempty"`

	// 模型覆盖
	ModelOverride    string `json:"modelOverride,omitempty"`
	ProviderOverride string `json:"providerOverride,omitempty"`
	ContextTokens    *int   `json:"contextTokens,omitempty"`

	// 认证覆盖
	AuthProfileOverride                string `json:"authProfileOverride,omitempty"`
	AuthProfileOverrideSource          string `json:"authProfileOverrideSource,omitempty"` // "auto" | "user"
	AuthProfileOverrideCompactionCount int    `json:"authProfileOverrideCompactionCount,omitempty"`

	// 群组激活
	GroupActivation                 string `json:"groupActivation,omitempty"` // "mention" | "always"
	GroupActivationNeedsSystemIntro bool   `json:"groupActivationNeedsSystemIntro,omitempty"`

	// 队列
	QueueMode       string `json:"queueMode,omitempty"` // steer|followup|collect|steer-backlog|steer+backlog|queue|interrupt
	QueueDebounceMs int    `json:"queueDebounceMs,omitempty"`
	QueueCap        int    `json:"queueCap,omitempty"`
	QueueDrop       string `json:"queueDrop,omitempty"` // "old" | "new" | "summarize"

	// Token 用量
	InputTokens  int64 `json:"inputTokens,omitempty"`
	OutputTokens int64 `json:"outputTokens,omitempty"`
	TotalTokens  int64 `json:"totalTokens,omitempty"`

	// 压缩 & 内存
	CompactionCount            int   `json:"compactionCount,omitempty"`
	MemoryFlushAt              int64 `json:"memoryFlushAt,omitempty"`
	MemoryFlushCompactionCount int   `json:"memoryFlushCompactionCount,omitempty"`

	// CLI 会话
	CliSessionIds      map[string]string `json:"cliSessionIds,omitempty"`
	ClaudeCliSessionId string            `json:"claudeCliSessionId,omitempty"`

	// 投递上下文
	DeliveryContext *DeliveryContext    `json:"deliveryContext,omitempty"`
	LastChannel     *SessionLastChannel `json:"lastChannel,omitempty"`
	LastTo          string              `json:"lastTo,omitempty"`
	LastAccountId   string              `json:"lastAccountId,omitempty"`
	LastThreadId    interface{}         `json:"lastThreadId,omitempty"` // string | number

	// 技能快照
	SkillsSnapshot *SessionSkillSnapshot `json:"skillsSnapshot,omitempty"`

	// 系统提示报告
	SystemPromptReport json.RawMessage `json:"systemPromptReport,omitempty"` // 复杂嵌套，保持 raw

	// Model provider (新增字段)
	ModelProvider string `json:"modelProvider,omitempty"`

	// 任务看板元数据（仅 task: session 使用）
	TaskMeta *TaskMeta `json:"taskMeta,omitempty"`
}

// TaskMeta 任务看板元数据（仅 task: session 使用）。
type TaskMeta struct {
	Status           string `json:"status"`                     // "queued" | "started" | "progress" | "completed" | "failed"
	Async            bool   `json:"async,omitempty"`            // 是否异步执行
	Summary          string `json:"summary,omitempty"`          // 完成摘要
	Error            string `json:"error,omitempty"`            // 终态错误信息
	ToolName         string `json:"toolName,omitempty"`         // 当前/最后工具名
	ProgressPhase    string `json:"progressPhase,omitempty"`    // 最近一个工具阶段
	ProgressText     string `json:"progressText,omitempty"`     // 最近一个工具步骤摘要
	ProgressIsError  bool   `json:"progressIsError,omitempty"`  // 最近一个工具步骤是否报错
	ProgressDuration int64  `json:"progressDuration,omitempty"` // 最近一个工具步骤耗时 ms
	ProgressAt       int64  `json:"progressAt,omitempty"`       // 最近一个工具步骤时间 (UnixMilli)
	StartedAt        int64  `json:"startedAt,omitempty"`        // 开始时间 (UnixMilli)
	CompletedAt      int64  `json:"completedAt,omitempty"`      // 完成时间 (UnixMilli)
}

// SessionOrigin 会话来源信息（与 TS SessionOrigin 完全对齐）。
type SessionOrigin struct {
	Label     string      `json:"label,omitempty"`
	Provider  string      `json:"provider,omitempty"`
	Surface   string      `json:"surface,omitempty"`
	ChatType  string      `json:"chatType,omitempty"`
	From      string      `json:"from,omitempty"`
	To        string      `json:"to,omitempty"`
	AccountId string      `json:"accountId,omitempty"`
	ThreadId  interface{} `json:"threadId,omitempty"` // string | number
}

// SessionLastChannel 最后频道信息。
type SessionLastChannel struct {
	Channel   string `json:"channel,omitempty"`
	AccountId string `json:"accountId,omitempty"`
	To        string `json:"to,omitempty"`
}

// SessionSkillSnapshot 技能快照（与 TS SessionSkillSnapshot 对齐）。
type SessionSkillSnapshot struct {
	Prompt         string                     `json:"prompt"`
	Skills         []SessionSkillSnapshotItem `json:"skills"`
	ResolvedSkills json.RawMessage            `json:"resolvedSkills,omitempty"` // 复杂类型，保持 raw
	Version        int                        `json:"version,omitempty"`
}

// SessionSkillSnapshotItem 快照中的单个技能。
type SessionSkillSnapshotItem struct {
	Name       string `json:"name"`
	PrimaryEnv string `json:"primaryEnv,omitempty"`
}

// DeliveryContext 投递上下文。
type DeliveryContext struct {
	Channel   string      `json:"channel,omitempty"`
	AccountId string      `json:"accountId,omitempty"`
	To        string      `json:"to,omitempty"`
	ThreadId  interface{} `json:"threadId,omitempty"` // string | number
}
