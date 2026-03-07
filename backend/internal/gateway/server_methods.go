package gateway

import (
	"context"
	"strings"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/argus"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/internal/memory/uhms"
	"github.com/Acosmi/ClawAcosmi/pkg/mcpremote"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- Handler 类型定义 (移植自 server-methods/types.ts) ----------

// GatewayMethodHandler 网关方法处理函数。
type GatewayMethodHandler func(ctx *MethodHandlerContext)

// MethodHandlerContext 方法处理上下文。
type MethodHandlerContext struct {
	Method  string
	Params  map[string]interface{}
	Client  *GatewayClient
	Context *GatewayMethodContext
	Respond RespondFunc
	Ctx     context.Context // 请求级 context，支持取消传播
}

// GatewayClient 已连接客户端信息。
type GatewayClient struct {
	Connect *ConnectParamsFull `json:"connect,omitempty"`
	ConnID  string             `json:"connId,omitempty"`
}

// RespondFunc 响应回调。
type RespondFunc func(ok bool, payload interface{}, err *ErrorShape)

// GatewayMethodContext 方法执行上下文。
type GatewayMethodContext struct {
	// 会话
	SessionStore *SessionStore
	StorePath    string                  // 会话存储路径（用于 sessions.list Path 字段）
	Config       *types.OpenAcosmiConfig // 配置引用（用于 defaults/主 session key 保护）

	// Batch A
	ConfigLoader *config.ConfigLoader // 配置加载器（用于 config.* 方法）
	ModelCatalog *models.ModelCatalog // 模型目录（用于 models.list）

	// Batch C — 系统 & 频道
	PresenceStore   *SystemPresenceStore // 系统在线状态（system-presence/event）
	HeartbeatState  *HeartbeatState      // 心跳状态（last-heartbeat/set-heartbeats）
	EventQueue      *SystemEventQueue    // 系统事件队列（system-event）
	Broadcaster     *Broadcaster         // 广播器（system-event presence 广播）
	LogFilePath     string               // 日志文件路径（logs.tail）
	ChannelLogoutFn ChannelLogoutFunc    // 频道 logout DI 回调

	// Batch B — Chat & Agent
	ChatState          *ChatRunState      // 聊天运行状态（chat.send/abort）
	PipelineDispatcher PipelineDispatcher // 管线分发器 DI（chat.send 管线入口）

	// Batch D — 频道发送 + Agent 等待
	ChannelSender ChannelOutboundSender // 频道消息发送 DI 回调
	AgentWaiter   AgentCommandWaiter    // Agent 命令等待 DI 回调

	// Batch FB — 幂等去重
	IdempotencyCache *IdempotencyCache // 请求幂等去重缓存

	// Batch D-W1 — Cron / TTS / Node / VoiceWake / Update / Web
	CronService    CronServiceAPI         // cron.* 方法委托
	CronStorePath  string                 // cron.runs 日志存储路径
	TtsConfig      TtsConfigProvider      // tts.* 方法配置
	NodeRegistryGW NodeRegistryForGateway // node.* 方法委托
	VoiceWake      VoiceWakeAPI           // voicewake.* 方法委托
	BroadcastFn    BroadcastFunc          // 广播回调
	ChannelPlugins ChannelPluginsProvider // web.login.* 委托
	ConfigWriterFn func() error           // 配置持久化回调
	PairingBaseDir string                 // device/node pairing 基目录

	// Batch D-W1b — Config / Send / Agent RPC 补全
	RestartSentinel  RestartSentinelWriter // restart sentinel 写入
	GatewayRestarter GatewayRestarter      // 网关重启调度
	LegacyMigrator   LegacyMigrator        // 配置遗留格式迁移
	ConfigSchemaProv ConfigSchemaProvider  // config.schema 插件/频道提供
	OutboundPipe     OutboundPipeline      // send/poll outbound 管线
	AttachmentParser AttachmentParser      // 消息附件解析
	TimestampInject  TimestampInjector     // 消息时间戳注入
	ChannelValidator ChannelValidator      // 频道验证
	DeliveryResolver DeliveryPlanResolver  // agent 投递计划解析

	// Batch P2 — 权限提升
	EscalationMgr *EscalationManager // P2: 临时提权管理器

	// Batch P4 — 远程审批
	RemoteApprovalNotifier *RemoteApprovalNotifier // P4: 远程审批通知

	// Batch P5 — 任务级预设权限
	TaskPresetMgr *TaskPresetManager // P5: 任务预设权限

	// Phase 5 — 频道管理器
	ChannelMgr *channels.Manager // 频道插件管理器

	// Argus 视觉子智能体
	ArgusBridge *argus.Bridge

	// 技能商店客户端（nexus-v4 REST API）
	SkillStoreClient *skills.SkillStoreClient

	// MCP 远程工具桥接 (P2: nexus-v4 MCP Server)
	RemoteMCPBridge *mcpremote.RemoteBridge

	// UHMS 记忆系统管理器（可选）
	UHMSManager *uhms.DefaultManager
	// UHMS Boot 文件管理器（可选，用于 skills.distribute 标记 Boot 状态）
	UHMSBootMgr *uhms.BootManager

	// Coder 确认管理器（可选，nil = 不需要确认）
	CoderConfirmMgr *runner.CoderConfirmationManager
	// 方案确认管理器（可选，nil = 不需要方案确认）
	PlanConfirmMgr *runner.PlanConfirmationManager
	// 结果签收管理器（可选，nil = 不需要结果签收）
	ResultApprovalMgr *runner.ResultApprovalManager

	// Phase 8: 合约持久化（可选，nil = contract.* RPC 返回空）
	ContractStore *VFSContractPersistence

	// Phase 4: 网关状态（用于 subagent.help.resolve 查找活跃通道）
	State *GatewayState

	// Phase 5+6: 媒体子系统（可选，nil = media.* RPC 不可用）
	MediaSubsystem *media.MediaSubsystem
}

// ---------- 权限常量 ----------
// scopeAdmin, scopeApprovals, scopePairing 已在 broadcast.go 中声明。

const (
	scopeRead  = "operator.read"
	scopeWrite = "operator.write"
)

// 方法分类集。
var (
	readMethods = newStringSet(
		"health", "logs.tail", "channels.status", "status",
		"usage.status", "usage.cost", "tts.status", "tts.providers",
		"models.list", "agents.list", "agent.identity.get",
		"skills.status", "skills.store.browse", "skills.store.refresh", "skills.store.link",
		"voicewake.get",
		"sessions.list", "sessions.preview",
		"sessions.usage", "sessions.usage.timeseries", "sessions.usage.logs",
		"cron.list", "cron.status", "cron.runs",
		"system-presence", "last-heartbeat",
		"node.list", "node.describe", "chat.history",
		"agents.files.list", "agents.files.get",
		"security.escalation.status", "security.escalation.audit",
		"security.rules.list", "security.rules.test", // P3: 规则查询
		"security.remoteApproval.config.get",                      // P4: 远程审批配置查询
		"security.taskPresets.list", "security.taskPresets.match", // P5: 任务预设查询
		"sandbox.config.get", "sandbox.status", "sandbox.test", // 沙箱配置/状态/测试
		"mcp.remote.status", "mcp.remote.tools", // P2: MCP 远程工具查询
		"memory.uhms.status", "memory.uhms.search", // P3: UHMS 状态/搜索 (修复授权缺失)
		"memory.uhms.llm.get",                       // UHMS 独立 LLM 配置查询
		"memory.list", "memory.get", "memory.stats", // memory.* 直接操作 (读)
		"contract.list", "contract.get", "contract.audit", // Phase 8: 合约生命周期
		"media.trending.fetch", "media.trending.sources", // Phase 5: 媒体热点
		"media.drafts.list", "media.drafts.get", // Phase 5: 媒体草稿 (读)
		"wizard.v2.providers.list", // Wizard V2 provider 目录 (只读)
		"subagent.list",            // 子智能体状态查询
		"argus.permission.check",   // Argus TCC 权限检查
	)

	writeMethods = newStringSet(
		"send", "poll", "agent", "agent.wait", "wake",
		"talk.mode", "tts.enable", "tts.disable",
		"tts.convert", "tts.setProvider",
		"voicewake.set", "node.invoke",
		"chat.send", "chat.abort", "browser.request",
		"web.login.start", "web.login.wait",
		"security.escalation.request",
		"memory.uhms.add",                                                                               // P3: UHMS 添加 (修复授权缺失)
		"memory.uhms.llm.set",                                                                           // UHMS 独立 LLM 配置设置
		"memory.delete", "memory.compress", "memory.commit", "memory.decay.run", "memory.import.skills", // memory.* 直接操作 (写)
		"media.drafts.delete", // Phase 5: 媒体草稿 (写)
		"subagent.ctl",        // 子智能体控制
	)

	approvalMethods = newStringSet(
		"exec.approval.request", "exec.approval.resolve",
		"security.escalation.resolve", "security.escalation.revoke",
		"coder.confirm.resolve",
	)

	nodeRoleMethods = newStringSet(
		"node.invoke.result", "node.event", "skills.bins",
	)

	pairingMethods = newStringSet(
		"node.pair.request", "node.pair.list",
		"node.pair.approve", "node.pair.reject", "node.pair.verify",
		"device.pair.list", "device.pair.approve", "device.pair.reject",
		"device.token.rotate", "device.token.revoke", "node.rename",
	)

	adminMethodPrefixes = []string{"exec.approvals."}

	adminExactMethods = newStringSet(
		"channels.logout", "channels.save", "agents.create", "agents.update", "agents.delete",
		"skills.install", "skills.update", "skills.distribute", "skills.store.pull",
		"cron.add", "cron.update", "cron.remove", "cron.run",
		"sessions.patch", "sessions.reset", "sessions.delete", "sessions.compact",
		"agents.files.set",
		"security.rules.add", "security.rules.remove", // P3: 规则管理
		"security.remoteApproval.config.set", "security.remoteApproval.test", // P4: 远程审批管理
		"security.remoteApproval.callback",                                                       // P4: 远程审批回调
		"security.taskPresets.add", "security.taskPresets.update", "security.taskPresets.remove", // P5: 任务预设管理
		"mcp.remote.connect", // P2: MCP 远程工具连接/重连
	)
)

// ---------- 权限检查 ----------

// AuthorizeGatewayMethod 检查客户端是否有权调用指定方法。
// 返回 nil 表示允许，非 nil 返回错误。
func AuthorizeGatewayMethod(method string, client *GatewayClient) *ErrorShape {
	if client == nil || client.Connect == nil {
		return nil
	}
	role := client.Connect.Role
	if role == "" {
		role = "operator"
	}
	scopes := client.Connect.Scopes

	// 节点角色方法
	if nodeRoleMethods.has(method) {
		if role == "node" {
			return nil
		}
		return NewErrorShape(ErrCodeForbidden, "unauthorized role: "+role)
	}
	if role == "node" {
		return NewErrorShape(ErrCodeForbidden, "unauthorized role: "+role)
	}
	if role != "operator" {
		return NewErrorShape(ErrCodeForbidden, "unauthorized role: "+role)
	}

	// admin scope 拥有全部权限
	if hasScope(scopes, scopeAdmin) {
		return nil
	}

	// 审批方法
	if approvalMethods.has(method) {
		if !hasScope(scopes, scopeApprovals) {
			return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeApprovals)
		}
		return nil
	}
	// 配对方法
	if pairingMethods.has(method) {
		if !hasScope(scopes, scopePairing) {
			return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopePairing)
		}
		return nil
	}
	// 只读方法
	if readMethods.has(method) {
		if !hasScope(scopes, scopeRead) && !hasScope(scopes, scopeWrite) {
			return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeRead)
		}
		return nil
	}
	// 写方法
	if writeMethods.has(method) {
		if !hasScope(scopes, scopeWrite) {
			return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeWrite)
		}
		return nil
	}
	// admin 前缀
	for _, prefix := range adminMethodPrefixes {
		if strings.HasPrefix(method, prefix) {
			return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeAdmin)
		}
	}
	// config/wizard/update 等管理方法
	if strings.HasPrefix(method, "config.") ||
		strings.HasPrefix(method, "wizard.") ||
		strings.HasPrefix(method, "update.") ||
		adminExactMethods.has(method) {
		return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeAdmin)
	}

	// argus.* 视觉子智能体方法
	if strings.HasPrefix(method, "argus.") {
		// argus.status 允许只读访问
		if method == "argus.status" {
			if hasScope(scopes, scopeRead) || hasScope(scopes, scopeWrite) {
				return nil
			}
			return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeRead)
		}
		// 其他 argus 方法需要写权限
		if hasScope(scopes, scopeWrite) {
			return nil
		}
		return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeWrite)
	}

	// 默认拒绝
	return NewErrorShape(ErrCodeForbidden, "missing scope: "+scopeAdmin)
}

// ---------- Handler 注册表 ----------

// MethodRegistry 方法处理器注册表。
type MethodRegistry struct {
	mu       sync.RWMutex
	handlers map[string]GatewayMethodHandler
}

// NewMethodRegistry 创建方法注册表。
func NewMethodRegistry() *MethodRegistry {
	return &MethodRegistry{handlers: make(map[string]GatewayMethodHandler)}
}

// Register 注册方法处理器。
func (r *MethodRegistry) Register(method string, handler GatewayMethodHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[method] = handler
}

// RegisterAll 批量注册。
func (r *MethodRegistry) RegisterAll(handlers map[string]GatewayMethodHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for method, handler := range handlers {
		r.handlers[method] = handler
	}
}

// Get 获取处理器。
func (r *MethodRegistry) Get(method string) GatewayMethodHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[method]
}

// Methods 列出所有注册方法。
func (r *MethodRegistry) Methods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.handlers))
	for m := range r.handlers {
		result = append(result, m)
	}
	return result
}

// HandleGatewayRequest 分发网关请求。
func HandleGatewayRequest(
	registry *MethodRegistry,
	req *RequestFrame,
	client *GatewayClient,
	ctx *GatewayMethodContext,
	respond RespondFunc,
) {
	// 权限检查
	if authErr := AuthorizeGatewayMethod(req.Method, client); authErr != nil {
		respond(false, nil, authErr)
		return
	}

	handler := registry.Get(req.Method)
	if handler == nil {
		respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unknown method: "+req.Method))
		return
	}

	params, _ := req.Params.(map[string]interface{})
	if params == nil {
		params = make(map[string]interface{})
	}

	handler(&MethodHandlerContext{
		Method:  req.Method,
		Params:  params,
		Client:  client,
		Context: ctx,
		Respond: respond,
		Ctx:     context.Background(),
	})
}

// ---------- 辅助工具 ----------

type stringSet map[string]struct{}

func newStringSet(items ...string) stringSet {
	s := make(stringSet, len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

func (s stringSet) has(item string) bool {
	_, ok := s[item]
	return ok
}

func hasScope(scopes []string, scope string) bool {
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}
