package gateway

import "github.com/Acosmi/ClawAcosmi/internal/session"

// ---------- Session Utils DTO 类型 (移植自 session-utils.types.ts) ----------

// GatewaySessionsDefaults 会话默认值。
type GatewaySessionsDefaults struct {
	ModelProvider *string `json:"modelProvider"` // null → nil
	Model         *string `json:"model"`
	ContextTokens *int    `json:"contextTokens"`
}

// GatewaySessionRow 网关会话行 (用于 sessions.list 响应)。
type GatewaySessionRow struct {
	Key                string              `json:"key"`
	Kind               string              `json:"kind"` // "direct" | "group" | "global" | "unknown"
	Label              string              `json:"label,omitempty"`
	DisplayName        string              `json:"displayName,omitempty"`
	DerivedTitle       string              `json:"derivedTitle,omitempty"`
	LastMessagePreview string              `json:"lastMessagePreview,omitempty"`
	Channel            string              `json:"channel,omitempty"`
	Subject            string              `json:"subject,omitempty"`
	GroupChannel       string              `json:"groupChannel,omitempty"`
	Space              string              `json:"space,omitempty"`
	ChatType           string              `json:"chatType,omitempty"`
	Origin             *SessionOrigin      `json:"origin,omitempty"`
	UpdatedAt          *int64              `json:"updatedAt"` // null → nil
	SessionId          string              `json:"sessionId,omitempty"`
	SystemSent         bool                `json:"systemSent,omitempty"`
	AbortedLastRun     bool                `json:"abortedLastRun,omitempty"`
	ThinkingLevel      string              `json:"thinkingLevel,omitempty"`
	VerboseLevel       string              `json:"verboseLevel,omitempty"`
	ReasoningLevel     string              `json:"reasoningLevel,omitempty"`
	ElevatedLevel      string              `json:"elevatedLevel,omitempty"`
	SendPolicy         string              `json:"sendPolicy,omitempty"`
	InputTokens        int64               `json:"inputTokens,omitempty"`
	OutputTokens       int64               `json:"outputTokens,omitempty"`
	TotalTokens        int64               `json:"totalTokens,omitempty"`
	ResponseUsage      string              `json:"responseUsage,omitempty"`
	ModelProvider      string              `json:"modelProvider,omitempty"`
	Model              string              `json:"model,omitempty"`
	ContextTokens      *int                `json:"contextTokens,omitempty"`
	DeliveryContext    *DeliveryContext    `json:"deliveryContext,omitempty"`
	LastChannel        *SessionLastChannel `json:"lastChannel,omitempty"`
	LastTo             string              `json:"lastTo,omitempty"`
	LastAccountId      string              `json:"lastAccountId,omitempty"`
}

// GatewayAgentRow 网关 Agent 行 (用于 agents.list 响应)。
type GatewayAgentRow struct {
	ID       string            `json:"id"`
	Name     string            `json:"name,omitempty"`
	Identity *AgentIdentityRow `json:"identity,omitempty"`
	// 统一发现字段 — 子智能体通过 agents.list 一并返回
	Type       string `json:"type,omitempty"`       // "agent" | "subagent"
	Status     string `json:"status,omitempty"`     // 子智能体运行时状态
	Error      string `json:"error,omitempty"`      // 子智能体错误信息
	Provider   string `json:"provider,omitempty"`   // LLM provider
	Model      string `json:"model,omitempty"`      // LLM model
	Configured *bool  `json:"configured,omitempty"` // 是否已显式配置（指针: nil=不适用, false=未配置, true=已配置）
	Builtin    *bool  `json:"builtin,omitempty"`    // 内置子智能体标记（指针: nil=不适用）
}

// AgentIdentityRow Agent 身份信息。
type AgentIdentityRow struct {
	Name      string `json:"name,omitempty"`
	Theme     string `json:"theme,omitempty"`
	Emoji     string `json:"emoji,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

// DeliveryContext 是 session.DeliveryContext 的类型别名。
type DeliveryContext = session.DeliveryContext

// SessionPreviewItem 会话预览条目。
type SessionPreviewItem struct {
	Role string `json:"role"` // "user" | "assistant" | "tool" | "system" | "other"
	Text string `json:"text"`
}

// SessionsPreviewEntry 会话预览结果项。
type SessionsPreviewEntry struct {
	Key    string               `json:"key"`
	Status string               `json:"status"` // "ok" | "empty" | "missing" | "error"
	Items  []SessionPreviewItem `json:"items"`
}

// SessionsPreviewResult 会话预览响应。
type SessionsPreviewResult struct {
	Ts       int64                  `json:"ts"`
	Previews []SessionsPreviewEntry `json:"previews"`
}

// SessionsListResult 会话列表响应。
type SessionsListResult struct {
	Ts       int64                   `json:"ts"`
	Path     string                  `json:"path"`
	Count    int                     `json:"count"`
	Defaults GatewaySessionsDefaults `json:"defaults"`
	Sessions []GatewaySessionRow     `json:"sessions"`
}

// SessionsPatchResult 会话修改响应。
type SessionsPatchResult struct {
	OK       bool              `json:"ok"`
	Path     string            `json:"path"`
	Key      string            `json:"key"`
	Entry    *SessionEntry     `json:"entry"`
	Resolved *ResolvedModelRef `json:"resolved,omitempty"`
}

// ResolvedModelRef 解析后的模型引用。
type ResolvedModelRef struct {
	ModelProvider string `json:"modelProvider,omitempty"`
	Model         string `json:"model,omitempty"`
}

// SessionsListParams sessions.list 请求参数。
type SessionsListParams struct {
	IncludeGlobal        bool   `json:"includeGlobal,omitempty"`
	IncludeUnknown       bool   `json:"includeUnknown,omitempty"`
	IncludeDerivedTitles bool   `json:"includeDerivedTitles,omitempty"`
	IncludeLastMessage   bool   `json:"includeLastMessage,omitempty"`
	SpawnedBy            string `json:"spawnedBy,omitempty"`
	Label                string `json:"label,omitempty"`
	AgentId              string `json:"agentId,omitempty"`
	Search               string `json:"search,omitempty"`
	ActiveMinutes        *int   `json:"activeMinutes,omitempty"`
	Limit                *int   `json:"limit,omitempty"`
}

// GroupKeyParts parseGroupKey 解析结果。
type GroupKeyParts struct {
	Channel string `json:"channel,omitempty"`
	Kind    string `json:"kind,omitempty"` // "group" | "channel"
	ID      string `json:"id,omitempty"`
}

// StoreTarget resolveGatewaySessionStoreTarget 结果。
type StoreTarget struct {
	AgentId      string   `json:"agentId"`
	StorePath    string   `json:"storePath"`
	CanonicalKey string   `json:"canonicalKey"`
	StoreKeys    []string `json:"storeKeys"`
}
