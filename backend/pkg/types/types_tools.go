package types

// 工具配置类型 — 继承自 src/config/types.tools.ts (462行)

// ============================================================
// 媒体理解 (Media Understanding) 配置
// ============================================================

// MediaUnderstandingScopeMatch 媒体理解范围匹配条件
type MediaUnderstandingScopeMatch struct {
	Channel   string   `json:"channel,omitempty"`
	ChatType  ChatType `json:"chatType,omitempty"`
	KeyPrefix string   `json:"keyPrefix,omitempty"`
}

// MediaUnderstandingScopeRule 媒体理解范围规则
type MediaUnderstandingScopeRule struct {
	Action SessionSendPolicyAction       `json:"action"`
	Match  *MediaUnderstandingScopeMatch `json:"match,omitempty"`
}

// MediaUnderstandingScopeConfig 媒体理解范围配置
type MediaUnderstandingScopeConfig struct {
	Default SessionSendPolicyAction       `json:"default,omitempty"`
	Rules   []MediaUnderstandingScopeRule `json:"rules,omitempty"`
}

// MediaUnderstandingCapability 媒体理解能力
type MediaUnderstandingCapability string

const (
	MediaCapImage MediaUnderstandingCapability = "image"
	MediaCapAudio MediaUnderstandingCapability = "audio"
	MediaCapVideo MediaUnderstandingCapability = "video"
)

// MediaUnderstandingAttachmentsConfig 附件选择策略
type MediaUnderstandingAttachmentsConfig struct {
	Mode           string `json:"mode,omitempty"` // "first"|"all"
	MaxAttachments *int   `json:"maxAttachments,omitempty"`
	Prefer         string `json:"prefer,omitempty"` // "first"|"last"|"path"|"url"
}

// DeepgramConfig Deepgram 配置 (@deprecated)
type DeepgramConfig struct {
	DetectLanguage *bool `json:"detectLanguage,omitempty"`
	Punctuate      *bool `json:"punctuate,omitempty"`
	SmartFormat    *bool `json:"smartFormat,omitempty"`
}

// MediaUnderstandingModelConfig 媒体理解模型配置
type MediaUnderstandingModelConfig struct {
	Provider         string                            `json:"provider,omitempty"`
	Model            string                            `json:"model,omitempty"`
	Capabilities     []MediaUnderstandingCapability    `json:"capabilities,omitempty"`
	Type             string                            `json:"type,omitempty"` // "provider"|"cli"
	Command          string                            `json:"command,omitempty"`
	Args             []string                          `json:"args,omitempty"`
	Prompt           string                            `json:"prompt,omitempty"`
	MaxChars         *int                              `json:"maxChars,omitempty"`
	MaxBytes         *int                              `json:"maxBytes,omitempty"`
	TimeoutSeconds   *int                              `json:"timeoutSeconds,omitempty"`
	Language         string                            `json:"language,omitempty"`
	ProviderOptions  map[string]map[string]interface{} `json:"providerOptions,omitempty"`
	Deepgram         *DeepgramConfig                   `json:"deepgram,omitempty"` // @deprecated
	BaseURL          string                            `json:"baseUrl,omitempty"`
	Headers          map[string]string                 `json:"headers,omitempty"`
	Profile          string                            `json:"profile,omitempty"`
	PreferredProfile string                            `json:"preferredProfile,omitempty"`
}

// MediaUnderstandingConfig 媒体理解总配置
type MediaUnderstandingConfig struct {
	Enabled         *bool                                `json:"enabled,omitempty"`
	Scope           *MediaUnderstandingScopeConfig       `json:"scope,omitempty"`
	MaxBytes        *int                                 `json:"maxBytes,omitempty"`
	MaxChars        *int                                 `json:"maxChars,omitempty"`
	Prompt          string                               `json:"prompt,omitempty"`
	TimeoutSeconds  *int                                 `json:"timeoutSeconds,omitempty"`
	Language        string                               `json:"language,omitempty"`
	ProviderOptions map[string]map[string]interface{}    `json:"providerOptions,omitempty"`
	Deepgram        *DeepgramConfig                      `json:"deepgram,omitempty"` // @deprecated
	BaseURL         string                               `json:"baseUrl,omitempty"`
	Headers         map[string]string                    `json:"headers,omitempty"`
	Attachments     *MediaUnderstandingAttachmentsConfig `json:"attachments,omitempty"`
	Models          []MediaUnderstandingModelConfig      `json:"models,omitempty"`
}

// ============================================================
// 链接工具 (Link Tools) 配置
// ============================================================

// LinkModelConfig 链接处理模型配置
type LinkModelConfig struct {
	Type           string   `json:"type,omitempty"` // "cli"
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	TimeoutSeconds *int     `json:"timeoutSeconds,omitempty"`
}

// LinkToolsConfig 链接工具配置
type LinkToolsConfig struct {
	Enabled        *bool                          `json:"enabled,omitempty"`
	Scope          *MediaUnderstandingScopeConfig `json:"scope,omitempty"`
	MaxLinks       *int                           `json:"maxLinks,omitempty"`
	TimeoutSeconds *int                           `json:"timeoutSeconds,omitempty"`
	Models         []LinkModelConfig              `json:"models,omitempty"`
}

// MediaToolsConfig 媒体工具总配置
type MediaToolsConfig struct {
	Models      []MediaUnderstandingModelConfig `json:"models,omitempty"`
	Concurrency *int                            `json:"concurrency,omitempty"`
	Image       *MediaUnderstandingConfig       `json:"image,omitempty"`
	Audio       *MediaUnderstandingConfig       `json:"audio,omitempty"`
	Video       *MediaUnderstandingConfig       `json:"video,omitempty"`
}

// ============================================================
// 工具策略 (Tool Policy) 配置
// ============================================================

// ToolProfileId 工具配置文件 ID
type ToolProfileId string

const (
	ToolProfileMinimal   ToolProfileId = "minimal"
	ToolProfileCoding    ToolProfileId = "coding"
	ToolProfileMessaging ToolProfileId = "messaging"
	ToolProfileFull      ToolProfileId = "full"
)

// ToolPolicyConfig 工具策略配置
type ToolPolicyConfig struct {
	Allow     []string      `json:"allow,omitempty"`
	AlsoAllow []string      `json:"alsoAllow,omitempty"`
	Deny      []string      `json:"deny,omitempty"`
	Profile   ToolProfileId `json:"profile,omitempty"`
}

// GroupToolPolicyConfig 群组工具策略
type GroupToolPolicyConfig struct {
	Allow     []string `json:"allow,omitempty"`
	AlsoAllow []string `json:"alsoAllow,omitempty"`
	Deny      []string `json:"deny,omitempty"`
}

// GroupToolPolicyBySenderConfig 按发送者的群组工具策略
type GroupToolPolicyBySenderConfig map[string]*GroupToolPolicyConfig

// ============================================================
// 执行工具 (Exec Tool) 配置
// ============================================================

// ExecApplyPatchConfig apply_patch 子工具配置
type ExecApplyPatchConfig struct {
	Enabled     *bool    `json:"enabled,omitempty"`
	AllowModels []string `json:"allowModels,omitempty"`
}

// ExecToolConfig 执行工具配置
// 原版: export type ExecToolConfig
type ExecToolConfig struct {
	Host                    string                `json:"host,omitempty"`     // "sandbox"|"gateway"|"node"
	Security                string                `json:"security,omitempty"` // "deny"|"allowlist"|"full"
	Ask                     string                `json:"ask,omitempty"`      // "off"|"on-miss"|"always"
	Node                    string                `json:"node,omitempty"`
	PathPrepend             []string              `json:"pathPrepend,omitempty"`
	SafeBins                []string              `json:"safeBins,omitempty"`
	BackgroundMs            *int                  `json:"backgroundMs,omitempty"`
	TimeoutSec              *int                  `json:"timeoutSec,omitempty"`
	ApprovalRunningNoticeMs *int                  `json:"approvalRunningNoticeMs,omitempty"`
	CleanupMs               *int                  `json:"cleanupMs,omitempty"`
	NotifyOnExit            *bool                 `json:"notifyOnExit,omitempty"`
	ApplyPatch              *ExecApplyPatchConfig `json:"applyPatch,omitempty"`
}

// AgentToolsConfig Agent 工具配置
// 原版: export type AgentToolsConfig
type AgentToolsConfig struct {
	Profile    ToolProfileId                `json:"profile,omitempty"`
	Allow      []string                     `json:"allow,omitempty"`
	AlsoAllow  []string                     `json:"alsoAllow,omitempty"`
	Deny       []string                     `json:"deny,omitempty"`
	ByProvider map[string]*ToolPolicyConfig `json:"byProvider,omitempty"`
	Elevated   *AgentToolsElevatedConfig    `json:"elevated,omitempty"`
	Exec       *ExecToolConfig              `json:"exec,omitempty"`
	Sandbox    *AgentToolsSandboxConfig     `json:"sandbox,omitempty"`
}

// AgentToolsElevatedConfig Agent 提权工具配置
type AgentToolsElevatedConfig struct {
	Enabled   *bool                        `json:"enabled,omitempty"`
	AllowFrom AgentElevatedAllowFromConfig `json:"allowFrom,omitempty"`
}

// AgentToolsSandboxConfig Agent 沙箱工具配置
type AgentToolsSandboxConfig struct {
	Tools *ToolAllowDenyConfig `json:"tools,omitempty"`
}

// ToolAllowDenyConfig 工具允许/拒绝列表
type ToolAllowDenyConfig struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ============================================================
// 内存搜索 (Memory Search) 配置
// ============================================================

// MemorySearchSource 内存搜索来源
type MemorySearchSource string

const (
	MemorySourceMemory   MemorySearchSource = "memory"
	MemorySourceSessions MemorySearchSource = "sessions"
)

// MemorySearchRemoteBatchConfig 远程嵌入批处理配置
type MemorySearchRemoteBatchConfig struct {
	Enabled        *bool `json:"enabled,omitempty"`
	Wait           *bool `json:"wait,omitempty"`
	Concurrency    *int  `json:"concurrency,omitempty"`
	PollIntervalMs *int  `json:"pollIntervalMs,omitempty"`
	TimeoutMinutes *int  `json:"timeoutMinutes,omitempty"`
}

// MemorySearchRemoteConfig 远程嵌入配置
type MemorySearchRemoteConfig struct {
	BaseURL string                         `json:"baseUrl,omitempty"`
	APIKey  string                         `json:"apiKey,omitempty"`
	Headers map[string]string              `json:"headers,omitempty"`
	Batch   *MemorySearchRemoteBatchConfig `json:"batch,omitempty"`
}

// MemorySearchLocalConfig 本地嵌入配置
type MemorySearchLocalConfig struct {
	ModelPath     string `json:"modelPath,omitempty"`
	ModelCacheDir string `json:"modelCacheDir,omitempty"`
}

// MemorySearchStoreVectorConfig 向量存储配置
type MemorySearchStoreVectorConfig struct {
	Enabled       *bool  `json:"enabled,omitempty"`
	ExtensionPath string `json:"extensionPath,omitempty"`
}

// MemorySearchStoreCacheConfig 存储缓存配置
type MemorySearchStoreCacheConfig struct {
	Enabled    *bool `json:"enabled,omitempty"`
	MaxEntries *int  `json:"maxEntries,omitempty"`
}

// MemorySearchStoreConfig 索引存储配置
type MemorySearchStoreConfig struct {
	Driver string                         `json:"driver,omitempty"` // "sqlite"
	Path   string                         `json:"path,omitempty"`
	Vector *MemorySearchStoreVectorConfig `json:"vector,omitempty"`
	Cache  *MemorySearchStoreCacheConfig  `json:"cache,omitempty"`
}

// MemorySearchChunkingConfig 分块配置
type MemorySearchChunkingConfig struct {
	Tokens  *int `json:"tokens,omitempty"`
	Overlap *int `json:"overlap,omitempty"`
}

// MemorySearchSyncSessionsConfig 会话同步配置
type MemorySearchSyncSessionsConfig struct {
	DeltaBytes    *int `json:"deltaBytes,omitempty"`
	DeltaMessages *int `json:"deltaMessages,omitempty"`
}

// MemorySearchSyncConfig 同步配置
type MemorySearchSyncConfig struct {
	OnSessionStart  *bool                           `json:"onSessionStart,omitempty"`
	OnSearch        *bool                           `json:"onSearch,omitempty"`
	Watch           *bool                           `json:"watch,omitempty"`
	WatchDebounceMs *int                            `json:"watchDebounceMs,omitempty"`
	IntervalMinutes *int                            `json:"intervalMinutes,omitempty"`
	Sessions        *MemorySearchSyncSessionsConfig `json:"sessions,omitempty"`
}

// MemorySearchHybridConfig 混合搜索配置 (BM25 + Vector)
type MemorySearchHybridConfig struct {
	Enabled             *bool    `json:"enabled,omitempty"`
	VectorWeight        *float64 `json:"vectorWeight,omitempty"`
	TextWeight          *float64 `json:"textWeight,omitempty"`
	CandidateMultiplier *int     `json:"candidateMultiplier,omitempty"`
}

// MemorySearchQueryConfig 查询配置
type MemorySearchQueryConfig struct {
	MaxResults *int                      `json:"maxResults,omitempty"`
	MinScore   *float64                  `json:"minScore,omitempty"`
	Hybrid     *MemorySearchHybridConfig `json:"hybrid,omitempty"`
}

// MemorySearchCacheConfig 查询缓存配置
type MemorySearchCacheConfig struct {
	Enabled    *bool `json:"enabled,omitempty"`
	MaxEntries *int  `json:"maxEntries,omitempty"`
}

// MemorySearchConfig 向量内存搜索配置
// 原版: export type MemorySearchConfig (100 行)
type MemorySearchConfig struct {
	Enabled      *bool                           `json:"enabled,omitempty"`
	Sources      []MemorySearchSource            `json:"sources,omitempty"`
	ExtraPaths   []string                        `json:"extraPaths,omitempty"`
	Experimental *MemorySearchExperimentalConfig `json:"experimental,omitempty"`
	Provider     string                          `json:"provider,omitempty"` // "openai"|"gemini"|"local"|"voyage"
	Remote       *MemorySearchRemoteConfig       `json:"remote,omitempty"`
	Fallback     string                          `json:"fallback,omitempty"` // "openai"|"gemini"|"local"|"voyage"|"none"
	Model        string                          `json:"model,omitempty"`
	Local        *MemorySearchLocalConfig        `json:"local,omitempty"`
	Store        *MemorySearchStoreConfig        `json:"store,omitempty"`
	Chunking     *MemorySearchChunkingConfig     `json:"chunking,omitempty"`
	Sync         *MemorySearchSyncConfig         `json:"sync,omitempty"`
	Query        *MemorySearchQueryConfig        `json:"query,omitempty"`
	Cache        *MemorySearchCacheConfig        `json:"cache,omitempty"`
}

// MemorySearchExperimentalConfig 实验性搜索配置
type MemorySearchExperimentalConfig struct {
	SessionMemory *bool `json:"sessionMemory,omitempty"`
}

// ============================================================
// Web 工具配置
// ============================================================

// WebSearchBochaConfig 博查搜索配置
type WebSearchBochaConfig struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// WebSearchGoogleConfig Google 搜索配置
type WebSearchGoogleConfig struct {
	APIKey         string `json:"apiKey,omitempty"`
	SearchEngineID string `json:"searchEngineId,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

// WebSearchConfig Web 搜索配置
type WebSearchConfig struct {
	Enabled         *bool                      `json:"enabled,omitempty"`
	Provider        string                   `json:"provider,omitempty"` // "bocha"|"google"
	APIKey          string                   `json:"apiKey,omitempty"`
	MaxResults      *int                     `json:"maxResults,omitempty"`
	TimeoutSeconds  *int                     `json:"timeoutSeconds,omitempty"`
	CacheTTLMinutes *int                     `json:"cacheTtlMinutes,omitempty"`
	Bocha           *WebSearchBochaConfig    `json:"bocha,omitempty"`
	Google          *WebSearchGoogleConfig   `json:"google,omitempty"`
}

// WebFetchFirecrawlConfig Firecrawl 配置
type WebFetchFirecrawlConfig struct {
	Enabled         *bool  `json:"enabled,omitempty"`
	APIKey          string `json:"apiKey,omitempty"`
	BaseURL         string `json:"baseUrl,omitempty"`
	OnlyMainContent *bool  `json:"onlyMainContent,omitempty"`
	MaxAgeMs        *int   `json:"maxAgeMs,omitempty"`
	TimeoutSeconds  *int   `json:"timeoutSeconds,omitempty"`
}

// WebFetchConfig Web 抓取配置
type WebFetchConfig struct {
	Enabled         *bool                    `json:"enabled,omitempty"`
	MaxChars        *int                     `json:"maxChars,omitempty"`
	MaxCharsCap     *int                     `json:"maxCharsCap,omitempty"`
	TimeoutSeconds  *int                     `json:"timeoutSeconds,omitempty"`
	CacheTTLMinutes *int                     `json:"cacheTtlMinutes,omitempty"`
	MaxRedirects    *int                     `json:"maxRedirects,omitempty"`
	UserAgent       string                   `json:"userAgent,omitempty"`
	Readability     *bool                    `json:"readability,omitempty"`
	Firecrawl       *WebFetchFirecrawlConfig `json:"firecrawl,omitempty"`
}

// WebToolsConfig Web 工具总配置
type WebToolsConfig struct {
	Search *WebSearchConfig `json:"search,omitempty"`
	Fetch  *WebFetchConfig  `json:"fetch,omitempty"`
}

// ============================================================
// 消息工具 (Message Tool) 配置
// ============================================================

// MessageCrossContextMarkerConfig 跨上下文标记配置
type MessageCrossContextMarkerConfig struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
	Suffix  string `json:"suffix,omitempty"`
}

// MessageCrossContextConfig 跨上下文发送配置
type MessageCrossContextConfig struct {
	AllowWithinProvider  *bool                            `json:"allowWithinProvider,omitempty"`
	AllowAcrossProviders *bool                            `json:"allowAcrossProviders,omitempty"`
	Marker               *MessageCrossContextMarkerConfig `json:"marker,omitempty"`
}

// MessageBroadcastConfig 消息广播配置
type MessageBroadcastConfig struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// MessageToolConfig 消息工具配置
type MessageToolConfig struct {
	AllowCrossContextSend *bool                      `json:"allowCrossContextSend,omitempty"` // @deprecated
	CrossContext          *MessageCrossContextConfig `json:"crossContext,omitempty"`
	Broadcast             *MessageBroadcastConfig    `json:"broadcast,omitempty"`
}

// AgentToAgentToolConfig Agent 间通信工具配置
type AgentToAgentToolConfig struct {
	Enabled *bool    `json:"enabled,omitempty"`
	Allow   []string `json:"allow,omitempty"`
}

// ToolsElevatedConfig 提权工具配置
type ToolsElevatedConfig struct {
	Enabled   *bool                        `json:"enabled,omitempty"`
	AllowFrom AgentElevatedAllowFromConfig `json:"allowFrom,omitempty"`
}

// SubagentToolModelConfig 子代理工具模型配置
type SubagentToolModelConfig struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// ToolsSubagentConfig 子代理工具配置
type ToolsSubagentConfig struct {
	Model interface{}          `json:"model,omitempty"` // string | SubagentToolModelConfig
	Tools *ToolAllowDenyConfig `json:"tools,omitempty"`
}

// ToolsSandboxConfig 工具沙箱配置
type ToolsSandboxConfig struct {
	Tools *ToolAllowDenyConfig `json:"tools,omitempty"`
}

// ToolsConfig 工具总配置
// 原版: export type ToolsConfig (136 行)
type ToolsConfig struct {
	Profile      ToolProfileId                `json:"profile,omitempty"`
	Allow        []string                     `json:"allow,omitempty"`
	AlsoAllow    []string                     `json:"alsoAllow,omitempty"`
	Deny         []string                     `json:"deny,omitempty"`
	ByProvider   map[string]*ToolPolicyConfig `json:"byProvider,omitempty"`
	Web          *WebToolsConfig              `json:"web,omitempty"`
	Media        *MediaToolsConfig            `json:"media,omitempty"`
	Links        *LinkToolsConfig             `json:"links,omitempty"`
	Message      *MessageToolConfig           `json:"message,omitempty"`
	AgentToAgent *AgentToAgentToolConfig      `json:"agentToAgent,omitempty"`
	Elevated     *ToolsElevatedConfig         `json:"elevated,omitempty"`
	Exec         *ExecToolConfig              `json:"exec,omitempty"`
	Subagents    *ToolsSubagentConfig         `json:"subagents,omitempty"`
	Sandbox      *ToolsSandboxConfig          `json:"sandbox,omitempty"`
}
