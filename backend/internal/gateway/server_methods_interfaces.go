package gateway

// server_methods_interfaces.go — DW1 Gateway 方法 DI 接口定义
// 定义 CronService / TTS / NodeRegistry / VoiceWake / ChannelPlugins 等接口，
// 使 server_methods_*.go 处理器可通过 GatewayMethodContext 获取依赖。

import (
	"github.com/Acosmi/ClawAcosmi/internal/cron"
	"github.com/Acosmi/ClawAcosmi/internal/tts"
)

// ---------- Cron ----------

// CronServiceAPI cron.* 网关方法委托接口。
// 实际实现: *cron.CronService
type CronServiceAPI interface {
	Wake(mode string, text string) *cron.CronOpResult
	List(includeDisabled bool) ([]cron.CronJob, error)
	Status() cron.CronStatusResult
	Add(input cron.CronJobCreate) (*cron.CronAddResult, error)
	Update(id string, patch cron.CronJobPatch) (*cron.CronOpResult, error)
	Remove(id string) (*cron.CronOpResult, error)
	Run(id string, mode string) (*cron.CronRunResult, error)
}

// ---------- TTS ----------

// TtsConfigProvider tts.* 网关方法配置提供者。
// 返回解析后的 TTS 配置和偏好路径。
type TtsConfigProvider interface {
	ResolveConfig() tts.ResolvedTtsConfig
	PrefsPath() string
}

// ---------- Node Registry ----------

// NodeRegistryForGateway node.* 网关方法委托接口。
// 提供 node invoke / event 路由 / 连接管理。
// 初始版本为 stub，后续 D-W2 补全。
type NodeRegistryForGateway interface {
	// ConnectedNodes 返回当前已连接节点列表
	ConnectedNodes() []ConnectedNodeInfo
	// Invoke 在指定节点上执行命令
	Invoke(nodeID string, command string, params interface{}) (interface{}, error)
	// HandleInvokeResult 处理节点返回的 invoke 结果
	HandleInvokeResult(nodeID string, requestID string, result interface{}) error
	// HandleNodeEvent 处理节点事件
	HandleNodeEvent(nodeID string, event string, payload interface{}) error
}

// ConnectedNodeInfo 已连接节点信息。
// 对应 TS: NodeSession (node-registry.ts)
type ConnectedNodeInfo struct {
	NodeID          string      `json:"nodeId"`
	DisplayName     string      `json:"displayName,omitempty"`
	Platform        string      `json:"platform,omitempty"`
	Version         string      `json:"version,omitempty"`
	CoreVersion     string      `json:"coreVersion,omitempty"`
	UIVersion       string      `json:"uiVersion,omitempty"`
	DeviceFamily    string      `json:"deviceFamily,omitempty"`
	ModelIdentifier string      `json:"modelIdentifier,omitempty"`
	RemoteIP        string      `json:"remoteIp,omitempty"`
	Caps            []string    `json:"caps,omitempty"`
	Commands        []string    `json:"commands,omitempty"`
	PathEnv         string      `json:"pathEnv,omitempty"`
	Permissions     interface{} `json:"permissions,omitempty"`
	ConnectedAtMs   int64       `json:"connectedAtMs,omitempty"`
}

// ---------- VoiceWake ----------

// VoiceWakeAPI voicewake.* 网关方法委托接口。
type VoiceWakeAPI interface {
	Get() (map[string]interface{}, error)
	Set(triggers interface{}) error
}

// ---------- Channel Plugins ----------

// ChannelPluginsProvider web.login.* 网关方法委托接口。
type ChannelPluginsProvider interface {
	// FindWebLoginProvider 查找支持 web login 的频道插件
	FindWebLoginProvider(accountID string) (WebLoginProvider, error)
}

// WebLoginProvider 支持 web login 的频道插件接口。
type WebLoginProvider interface {
	LoginWithQrStart(accountID string) (map[string]interface{}, error)
	LoginWithQrWait(accountID string) (map[string]interface{}, error)
}

// ---------- Broadcast 辅助 ----------

// BroadcastFunc 广播函数签名。
type BroadcastFunc func(event string, payload interface{}, opts *BroadcastOptions)

// ---------- Restart Sentinel ----------
// 对应 TS: infra/restart-sentinel.ts → writeRestartSentinel()

// RestartSentinelPayload sentinel 文件内容。
// 对应 TS: RestartSentinelPayload (infra/restart-sentinel.ts)
type RestartSentinelPayload struct {
	Kind       string                 `json:"kind"`   // "config-apply"
	Status     string                 `json:"status"` // "ok"
	Ts         int64                  `json:"ts"`     // UnixMilli
	SessionKey string                 `json:"sessionKey,omitempty"`
	Message    *string                `json:"message"` // nullable
	DoctorHint string                 `json:"doctorHint,omitempty"`
	Stats      map[string]interface{} `json:"stats,omitempty"`
}

// RestartSentinelWriter 写入 restart sentinel 文件。
// 对应 TS: writeRestartSentinel() (infra/restart-sentinel.ts)
type RestartSentinelWriter interface {
	WriteRestartSentinel(payload *RestartSentinelPayload) (sentinelPath string, err error)
	FormatDoctorNonInteractiveHint() string
}

// ---------- Gateway Restart ----------
// 对应 TS: infra/restart.ts → scheduleGatewaySigusr1Restart()

// GatewayRestartResult 重启调度结果。
type GatewayRestartResult struct {
	Scheduled     bool   `json:"scheduled"`
	DelayMs       int    `json:"delayMs,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Transactional bool   `json:"transactional,omitempty"`
}

// GatewayConfigRollback 网关重启失败时的配置回滚信息。
type GatewayConfigRollback struct {
	ConfigPath       string `json:"-"`
	PreviousRaw      []byte `json:"-"`
	DeleteOnRollback bool   `json:"-"`
}

// GatewayRestartPlan 网关重启计划。
type GatewayRestartPlan struct {
	DelayMs  *int
	Reason   string
	Rollback *GatewayConfigRollback
}

// GatewayRestarter 网关重启调度器。
// 对应 TS: scheduleGatewaySigusr1Restart() (infra/restart.ts)
type GatewayRestarter interface {
	ScheduleRestart(plan GatewayRestartPlan) *GatewayRestartResult
}

// ---------- Legacy Migration ----------
// 对应 TS: config/legacy.ts → applyLegacyMigrations()

// LegacyMigrationResult 迁移结果。
type LegacyMigrationResult struct {
	Applied bool        // 是否有迁移发生
	Next    interface{} // 迁移后的配置（nil 表示无变化）
}

// LegacyMigrator 配置遗留格式迁移器。
// 对应 TS: applyLegacyMigrations() (config/legacy.ts)
type LegacyMigrator interface {
	ApplyLegacyMigrations(configData interface{}) *LegacyMigrationResult
}

// ---------- Config Schema Provider ----------
// 对应 TS: config/schema.ts → buildConfigSchema()

// ConfigSchemaPluginEntry schema 构建时的插件条目。
type ConfigSchemaPluginEntry struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description,omitempty"`
	ConfigUIHints interface{} `json:"configUiHints,omitempty"`
	ConfigSchema  interface{} `json:"configSchema,omitempty"`
}

// ConfigSchemaChannelEntry schema 构建时的频道条目。
type ConfigSchemaChannelEntry struct {
	ID            string      `json:"id"`
	Label         string      `json:"label"`
	Description   string      `json:"description,omitempty"`
	ConfigSchema  interface{} `json:"configSchema,omitempty"`
	ConfigUIHints interface{} `json:"configUiHints,omitempty"`
}

// ConfigSchemaProvider 加载插件和频道列表用于构建 config schema。
// 对应 TS: config.schema 方法中的 loadOpenAcosmiPlugins() + listChannelPlugins()
type ConfigSchemaProvider interface {
	ListPluginsForSchema() []ConfigSchemaPluginEntry
	ListChannelsForSchema() []ConfigSchemaChannelEntry
}

// ---------- Attachment Parser ----------
// 对应 TS: chat-attachments.ts → parseMessageWithAttachments()

// ParsedAttachmentResult 附件解析结果。
type ParsedAttachmentResult struct {
	Message string
	Images  []AgentImage
}

// AgentImage agent 图片附件。
type AgentImage struct {
	Type     string `json:"type"` // "image"
	Data     string `json:"data"` // base64
	MimeType string `json:"mimeType"`
}

// AttachmentParser 解析消息中的附件（图片等）。
// 对应 TS: parseMessageWithAttachments() (chat-attachments.ts)
type AttachmentParser interface {
	ParseMessageWithAttachments(
		message string,
		attachments []map[string]interface{},
		maxBytes int,
	) (*ParsedAttachmentResult, error)
}

// ---------- Delivery Plan ----------
// 对应 TS: infra/outbound/agent-delivery.ts

// DeliveryPlan agent 投递计划。
type DeliveryPlan struct {
	ResolvedChannel    string `json:"resolvedChannel"`
	DeliveryTargetMode string `json:"deliveryTargetMode"`
	ResolvedAccountID  string `json:"resolvedAccountId,omitempty"`
	ResolvedTo         string `json:"resolvedTo,omitempty"`
	ResolvedThreadID   string `json:"resolvedThreadId,omitempty"`
}

// DeliveryPlanResolver 解析 agent 投递计划。
// 对应 TS: resolveAgentDeliveryPlan() + resolveAgentOutboundTarget()
type DeliveryPlanResolver interface {
	ResolveAgentDeliveryPlan(params *DeliveryPlanParams) *DeliveryPlan
	ResolveAgentOutboundTarget(plan *DeliveryPlan, cfg interface{},
		targetMode string, validateExplicit bool) *OutboundTargetResult
}

// DeliveryPlanParams 投递计划解析参数。
type DeliveryPlanParams struct {
	SessionEntry     interface{} // *SessionEntry
	RequestedChannel string
	ExplicitTo       string
	ExplicitThreadID string
	AccountID        string
	WantsDelivery    bool
}

// OutboundTargetResult outbound 目标解析结果。
type OutboundTargetResult struct {
	ResolvedTo string
	OK         bool
}

// ---------- Outbound Pipeline ----------
// 对应 TS: send.ts 中的 outbound 投递管线

// OutboundPipeline send/poll 方法的完整 outbound 管线。
// 扩展自原有的 ChannelOutboundSender，提供完整的目标解析 + 会话路由 + 投递。
type OutboundPipeline interface {
	// ResolveTarget 解析 outbound 目标。
	ResolveTarget(channelID string, to string, accountID string) (*OutboundResolvedTarget, error)
	// EnsureSessionRoute 确保 outbound session 路由存在。
	EnsureSessionRoute(target *OutboundResolvedTarget, sessionKey string) error
	// Deliver 投递 outbound 消息。
	Deliver(target *OutboundResolvedTarget, message string, mediaURLs []string, opts *OutboundDeliverOpts) (*OutboundDeliverResult, error)
	// SendPoll 通过频道插件发送 poll。
	SendPoll(channelID string, question string, options []string, to string) (*OutboundPollResult, error)
}

// OutboundResolvedTarget 解析后的 outbound 目标。
type OutboundResolvedTarget struct {
	ChannelID      string `json:"channelId"`
	To             string `json:"to"`
	AccountID      string `json:"accountId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
}

// OutboundDeliverOpts 投递选项。
type OutboundDeliverOpts struct {
	SessionKey     string
	IdempotencyKey string
	GifPlayback    bool
	Mirror         bool
}

// OutboundDeliverResult 投递结果。
type OutboundDeliverResult struct {
	MessageID string `json:"messageId"`
	ChatID    string `json:"chatId,omitempty"`
	OK        bool   `json:"ok"`
}

// OutboundPollResult poll 投递结果。
type OutboundPollResult struct {
	PollID string `json:"pollId,omitempty"`
	OK     bool   `json:"ok"`
}

// ---------- Timestamp Injection ----------
// 对应 TS: agent-timestamp.ts → injectTimestamp()

// TimestampInjector 消息时间戳注入器。
type TimestampInjector interface {
	InjectTimestamp(message string, cfg interface{}) string
}

// ---------- Channel Validator ----------
// 对应 TS: utils/message-channel.ts

// ChannelValidator 频道验证器。
type ChannelValidator interface {
	IsGatewayMessageChannel(channel string) bool
	NormalizeMessageChannel(channel string) string
	IsDeliverableMessageChannel(channel string) bool
}
