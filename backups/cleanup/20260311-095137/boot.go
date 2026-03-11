package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/argus"
	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/memory/uhms"
	"github.com/Acosmi/ClawAcosmi/internal/packages"
	"github.com/Acosmi/ClawAcosmi/internal/sandbox"
	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
	"github.com/Acosmi/ClawAcosmi/pkg/mcpinstall"
	"github.com/Acosmi/ClawAcosmi/pkg/mcpremote"
)

// ---------- 服务引导 ----------

// BootConfig 网关启动配置。
type BootConfig struct {
	Server         ServerConfig
	Auth           ResolvedGatewayAuth
	Reload         ReloadSettings
	TrustedProxies []string
}

// GatewayState 网关运行时状态（集中管理所有子系统）。
type GatewayState struct {
	mu                     sync.RWMutex
	phase                  BootPhase
	broadcaster            *Broadcaster
	chatState              *ChatRunState
	toolReg                *ToolRegistry
	eventDisp              *NodeEventDispatcher
	escalationMgr          *EscalationManager
	remoteApprovalNotifier *RemoteApprovalNotifier // P4: 远程审批通知
	taskPresetMgr          *TaskPresetManager      // P5: 任务级预设权限
	channelMgr             *channels.Manager       // Phase 5: 频道插件管理

	// 沙箱子系统（可选 — 仅 Docker 可用时初始化）
	sandboxPool   *sandbox.ContainerPool // 容器池
	sandboxWorker *sandbox.Worker        // 异步任务工作池
	sandboxHub    *sandbox.ProgressHub   // WebSocket 进度推送
	sandboxStore  *sandbox.TaskStore     // 任务存储

	// Argus 视觉子智能体（可选 — 仅二进制可用时初始化）
	argusBridge *argus.Bridge

	// (Phase 2A 已删除: Coder Bridge MCP 桥接 → spawn_coder_agent 替代)

	// MCP 远程工具 Bridge（可选 — 仅配置启用时初始化）
	remoteMCPBridge *mcpremote.RemoteBridge

	// MCP 本地安装服务器管理器（可选 — 仅注册表非空时初始化）
	mcpLocalManager *mcpinstall.McpLocalManager

	// 原生沙箱 Worker Bridge（可选 — 仅 CLI 二进制可用时初始化）
	nativeSandboxBridge *sandbox.NativeSandboxBridge

	// UHMS 记忆系统（可选 — 仅配置启用时初始化）
	uhmsManager *uhms.DefaultManager
	uhmsBootMgr *uhms.BootManager // Boot 文件管理器（技能分级状态等）

	// Coder 确认管理器（可选 — 仅 coder bridge 可用时初始化）
	coderConfirmMgr *runner.CoderConfirmationManager

	// 方案确认管理器（Phase 1: 三级指挥体系 — task_write+ 意图需用户确认方案）
	planConfirmMgr *runner.PlanConfirmationManager

	// 结果签收管理器（Phase 3: 三级指挥体系 — 质量审核通过后用户签收）
	resultApprovalMgr *runner.ResultApprovalManager

	// Phase 5: 合约 VFS 持久化（可选 — 仅 UHMS VFS 可用时初始化）
	contractStore       *VFSContractPersistence
	contractCleanupDone chan struct{} // 关闭时取消 TTL 清理 goroutine

	// Phase 4: 异步消息通道注册表（help request ID → AgentChannel）
	// 用于 subagent.help.resolve RPC 将用户回复路由到正确的子智能体。
	agentChannelsMu sync.RWMutex
	agentChannels   map[string]*agentChannelRef // help request msgID → channel ref

	// Phase 4.1: 自动启动的 Chrome 实例（可选 — 仅在自动启动时非 nil）
	// 由 EnsureChrome() 创建，关闭时需 Stop() 释放。
	managedChrome managedChromeInstance

	// Phase 2: Extension Relay 服务器（可选 — 浏览器启用时创建）
	// 桥接 Chrome 扩展和 Agent 工具层，关闭时需 Close() 释放。
	extensionRelay extensionRelayInstance

	// Monitor 频道热更新管理器（由 server.go 在 startMonitorChannels 替代时注入）
	channelMonitorMgr *ChannelMonitorManager

	// Phase 2A: OAuth Token Manager（可选 — 仅 OAuth 配置存在时初始化）
	authManager *mcpremote.OAuthTokenManager

	// Phase 4: 托管模型目录（可选 — 仅 ManagedModels.Enabled 且已登录时初始化）
	managedCatalog *models.ManagedModelCatalog

	// Phase 3: 统一应用中心（可选 — 初始化失败不阻塞启动）
	packageCatalog   *packages.PackageCatalog
	packageLedger    *packages.PackageLedger
	packageInstaller *packages.PackageInstaller
}

// managedChromeInstance is an interface for an auto-launched Chrome process.
// Implemented by browser.ChromeInstance. Defined here to avoid importing browser in boot.go.
type managedChromeInstance interface {
	Stop() error
}

// extensionRelayInstance is an interface for the Chrome extension relay server.
// Implemented by browser.ExtensionRelay. Defined here to avoid importing browser in boot.go.
type extensionRelayInstance interface {
	Close() error
	Port() int
	AuthToken() string
	ExtensionConnected() bool
}

// BootPhase 网关启动阶段。
type BootPhase string

const (
	BootPhaseInit     BootPhase = "init"
	BootPhaseStarting BootPhase = "starting"
	BootPhaseReady    BootPhase = "ready"
	BootPhaseStopping BootPhase = "stopping"
	BootPhaseStopped  BootPhase = "stopped"
)

// NewGatewayState 创建网关运行时状态。
func NewGatewayState() *GatewayState {
	return newGatewayStateWithPlan(resolveGatewayBootstrapPlan(GatewayServerOptions{}))
}

func newGatewayStateWithPlan(plan gatewayBootstrapPlan) *GatewayState {
	bc := NewBroadcaster()
	auditLogger := NewEscalationAuditLogger()
	remoteNotifier := NewRemoteApprovalNotifier(bc)
	s := &GatewayState{
		phase:                  BootPhaseInit,
		broadcaster:            bc,
		chatState:              NewChatRunState(),
		toolReg:                NewToolRegistry(),
		eventDisp:              NewNodeEventDispatcher(),
		escalationMgr:          NewEscalationManager(bc, auditLogger, remoteNotifier),
		remoteApprovalNotifier: remoteNotifier, // Phase 4.1: RestoreFromDisk 在 NewGatewayState 末尾调用
		taskPresetMgr:          NewTaskPresetManager(),
		channelMgr:             channels.NewManager(),
	}

	// 沙箱初始化策略：优先 Rust 原生沙箱，仅在原生不可用时回退到 Docker 容器池。
	// Docker 容器池代码保留作为备份，但不在原生沙箱可用时初始化，避免不必要的资源消耗。

	// 第一步：尝试初始化原生沙箱 Worker Bridge
	nativeBinaryPath := resolveNativeSandboxBinaryPath()
	if sandbox.IsNativeSandboxAvailable(nativeBinaryPath) {
		cfg := sandbox.DefaultNativeSandboxConfig()
		cfg.BinaryPath = nativeBinaryPath
		// Workspace 在 AttemptRunner 层动态设置，此处使用 /tmp 作为默认
		cfg.Workspace = os.TempDir()
		bridge := sandbox.NewNativeSandboxBridge(cfg)
		if err := bridge.Start(); err != nil {
			slog.Warn("gateway: native sandbox bridge start failed, will try Docker fallback", "error", err)
		} else {
			s.nativeSandboxBridge = bridge
			slog.Info("gateway: native sandbox bridge started", "pid", bridge.PID())
		}
	} else {
		slog.Info("gateway: native sandbox CLI binary not available, will try Docker fallback")
	}

	// 第二步：仅在原生沙箱不可用时，初始化 Docker 容器池作为兜底
	if s.nativeSandboxBridge == nil {
		if sandbox.IsDockerAvailable() {
			store := sandbox.NewTaskStore()
			hub := sandbox.NewProgressHub(nil) // nil = allow all origins
			pool := sandbox.NewContainerPool(sandbox.DefaultContainerPoolConfig())
			worker := sandbox.NewWorker(store, sandbox.NewDockerTaskExecutor(pool), hub, sandbox.DefaultWorkerConfig())

			s.sandboxStore = store
			s.sandboxHub = hub
			s.sandboxPool = pool
			s.sandboxWorker = worker

			// 启动容器池和工作池
			ctx := context.Background()
			pool.Start(ctx)
			worker.Start(ctx)
			slog.Info("gateway: Docker sandbox fallback started (native sandbox unavailable)")
		} else {
			slog.Warn("gateway: no sandbox available (neither native nor Docker)")
		}
	} else {
		slog.Info("gateway: Docker sandbox skipped (native sandbox active)")
	}

	// 可选：初始化 Argus 视觉子智能体（仅二进制可用时）
	if plan.InitializeArgus {
		argusPath := resolveArgusBinaryPath("")
		if argus.IsAvailable(argusPath) {
			// ARGUS-007: 安装位标准化 — 在 ~/.openacosmi/bin/ 创建符号链接，
			// 确保下次重启 resolver 的 "user_bin" 层可直接命中。
			if err := argus.EnsureUserBinLink(argusPath); err != nil {
				slog.Warn("argus: user bin link creation failed (non-fatal)",
					"error", err, "binary", argusPath)
			}

			// 方案 B 兜底：裸二进制自动签名，确保 macOS TCC 授权持久化
			if err := argus.EnsureCodeSigned(argusPath); err != nil {
				slog.Warn("argus: code signing failed (non-fatal, TCC authorization may not persist)",
					"error", err, "binary", argusPath, "phase", "codesign")
			}

			// TCC 权限预检（仅 macOS）
			tccStatus := argus.CheckTCCPermissionsForBinary(argusPath)
			if !tccStatus.HasRequiredPermissions() {
				slog.Warn("argus: TCC permissions missing (Argus may fail to capture screen)",
					"screen_recording", string(tccStatus.ScreenRecording),
					"accessibility", string(tccStatus.Accessibility),
					"permission_subject", tccStatus.PermissionSubject,
					"permission_path", tccStatus.PermissionPath,
					"detection_mode", tccStatus.DetectionMode,
					"recovery", tccStatus.Recovery())
			}

			cfg := argus.DefaultBridgeConfig()
			cfg.BinaryPath = argusPath
			// 注入状态变更回调 → broadcast 通知前端
			cfg.OnStateChange = func(state argus.BridgeState, reason string) {
				if bc := s.broadcaster; bc != nil {
					bc.Broadcast("argus.status.changed", map[string]interface{}{
						"state":  string(state),
						"reason": reason,
						"ts":     time.Now().UnixMilli(),
					}, nil)
				}
			}
			bridge := argus.NewBridge(cfg)
			slog.Info("gateway: argus bridge initialized for on-demand start",
				"binary", argusPath, "phase", "init")
			// 保留 bridge 实例，但不在首次打开时自动启动。
			// 由 UI 在智能体面板显式启用，避免开机即拉起 macOS 权限弹窗。
			s.SetArgusBridge(bridge)
		} else {
			slog.Info("gateway: Argus binary not available, visual agent disabled")
		}
	} else {
		slog.Info("gateway: bootstrap profile disables Argus startup hooks")
	}

	// (Phase 2A: Coder Bridge MCP 启动已删除 — oa-coder 升级为独立 LLM Agent Session)

	// ── 命令审批门控（Ask 规则 + Coder 确认） ──────────────────
	// 无条件初始化：bash Ask 规则需要真正阻塞审批（fail-closed 安全策略），
	// 不依赖 Coder Bridge 是否存在。前端通过 coder.confirm.* 事件处理。
	s.coderConfirmMgr = runner.NewCoderConfirmationManager(
		func(event string, payload interface{}) {
			bc.Broadcast(event, payload, nil)
		},
		func(req runner.CoderConfirmationRequest, sessionKey string) {
			// 远程通知：将操作确认卡片推送到非 Web 渠道（飞书等）
			if s.remoteApprovalNotifier == nil {
				return
			}
			// 从 sessionKey 提取 chatID（格式: "feishu:<chatID>"）
			var chatID string
			if strings.HasPrefix(sessionKey, "feishu:") {
				chatID = strings.TrimPrefix(sessionKey, "feishu:")
			}
			preview := ""
			if req.Preview != nil {
				if req.Preview.Command != "" {
					preview = req.Preview.Command
				} else if req.Preview.FilePath != "" {
					preview = req.Preview.FilePath
				}
			}
			// D5-F2: 从 RemoteApprovalNotifier 获取 LastKnownUserID，
			// 与 SendApprovalRequest（提权审批）对齐，确保私聊卡片可送达。
			var userID string
			cfg := s.remoteApprovalNotifier.GetConfig()
			if cfg.Feishu != nil {
				userID = cfg.Feishu.LastKnownUserID
			}
			fallbackReq := CoderConfirmCardRequest{
				ConfirmID:        req.ID,
				ToolName:         req.ToolName,
				Preview:          preview,
				SessionKey:       sessionKey,
				OriginatorChatID: chatID,
				OriginatorUserID: userID,
				TTLMinutes:       5,
				Workflow:         req.Workflow,
			}
			if req.ToolName == "send_media" {
				typedReq := buildSendMediaExportApproval(req, sessionKey, chatID, userID)
				s.remoteApprovalNotifier.NotifyTypedOrCoderConfirm(&typedReq, fallbackReq)
				return
			}
			s.remoteApprovalNotifier.NotifyCoderConfirm(fallbackReq)
		},
		5*time.Minute,
	)
	slog.Info("gateway: command approval gate initialized")

	// ── 方案确认门控（Phase 1: 三级指挥体系） ──────────────────
	// 无条件初始化：task_write/task_delete/task_multimodal 意图需用户确认方案。
	// 前端通过 plan.confirm.* 事件处理。
	s.planConfirmMgr = runner.NewPlanConfirmationManager(
		func(event string, payload interface{}) {
			bc.Broadcast(event, payload, nil)
		},
		func(req runner.PlanConfirmationRequest, sessionKey string) {
			// 远程通知：将方案确认卡片推送到非 Web 渠道（飞书等）
			if s.remoteApprovalNotifier == nil {
				return
			}
			// 从 sessionKey 提取 chatID（格式: "feishu:<chatID>"）
			var chatID string
			if strings.HasPrefix(sessionKey, "feishu:") {
				chatID = strings.TrimPrefix(sessionKey, "feishu:")
			}
			// 获取 LastKnownUserID 用于私聊 fallback
			var userID string
			cfg := s.remoteApprovalNotifier.GetConfig()
			if cfg.Feishu != nil {
				userID = cfg.Feishu.LastKnownUserID
			}
			typedReq := TypedApprovalRequest{
				Type:             ApprovalTypePlanConfirm,
				ID:               req.ID,
				Reason:           "执行前方案确认",
				TTLMinutes:       5,
				RequestedAt:      time.UnixMilli(req.CreatedAtMs),
				SessionKey:       sessionKey,
				OriginatorChatID: chatID,
				OriginatorUserID: userID,
				TaskBrief:        req.TaskBrief,
				PlanSteps:        req.PlanSteps,
				IntentTier:       req.IntentTier,
				Workflow:         req.Workflow,
			}
			s.remoteApprovalNotifier.NotifyTypedOrPlanConfirm(&typedReq, PlanConfirmCardRequest{
				ConfirmID:        req.ID,
				TaskBrief:        req.TaskBrief,
				PlanSteps:        req.PlanSteps,
				ApprovalSummary:  req.ApprovalSummary,
				IntentTier:       req.IntentTier,
				SessionKey:       sessionKey,
				OriginatorChatID: chatID,
				OriginatorUserID: userID,
				TTLMinutes:       5,
				Workflow:         req.Workflow,
			})
		},
		5*time.Minute,
	)
	slog.Info("gateway: plan confirmation gate initialized")

	// ── 结果签收门控（Phase 3: 三级指挥体系） ──────────────────
	// 质量审核通过后，结果呈现给用户做最终签收。
	// 前端通过 result.approve.* 事件处理。
	s.resultApprovalMgr = runner.NewResultApprovalManager(
		func(event string, payload interface{}) {
			bc.Broadcast(event, payload, nil)
		},
		func(req runner.ResultApprovalRequest, sessionKey string) {
			if s.remoteApprovalNotifier == nil {
				return
			}
			var chatID string
			if strings.HasPrefix(sessionKey, "feishu:") {
				chatID = strings.TrimPrefix(sessionKey, "feishu:")
			}
			var userID string
			cfg := s.remoteApprovalNotifier.GetConfig()
			if cfg.Feishu != nil {
				userID = cfg.Feishu.LastKnownUserID
			}
			typedReq := buildResultReviewApproval(req, sessionKey, chatID, userID)
			s.remoteApprovalNotifier.NotifyTypedApproval(typedReq)
		},
		3*time.Minute,
	)
	slog.Info("gateway: result approval gate initialized")

	// Phase 4.1: 从磁盘恢复未过期的 pending 审批请求
	s.escalationMgr.RestoreFromDisk()

	// 可选：初始化 UHMS 记忆系统（仅配置启用时）
	// 注意：此处不传 LLMProvider（需要由 server.go 在运行时注入），
	// 所以 UHMS 在无 LLM 时仍可工作（仅 FTS5 搜索 + VFS 存储）。
	uhmsCfg := uhms.DefaultUHMSConfig()
	// 实际配置由 server.go 从 OpenAcosmiConfig.Memory.UHMS 读取并覆盖
	if uhmsCfg.Enabled {
		mgr, err := uhms.NewManager(uhmsCfg, nil)
		if err != nil {
			slog.Warn("gateway: UHMS init failed (non-fatal)", "error", err)
		} else {
			s.uhmsManager = mgr
			slog.Info("gateway: UHMS memory system initialized",
				"vectorMode", uhmsCfg.VectorMode,
				"vfsPath", uhmsCfg.ResolvedVFSPath(),
			)
		}
	} else {
		slog.Debug("gateway: UHMS not enabled in boot defaults (may be initialized from config)")
	}

	return s
}

func buildSendMediaExportApproval(
	req runner.CoderConfirmationRequest,
	sessionKey, originatorChatID, originatorUserID string,
) TypedApprovalRequest {
	var input struct {
		Target   string `json:"target"`
		FilePath string `json:"file_path"`
		FileName string `json:"file_name"`
		Message  string `json:"message"`
	}
	_ = json.Unmarshal(req.Args, &input)

	targetChannel := strings.TrimSpace(input.Target)
	if targetChannel == "" {
		targetChannel = strings.TrimSpace(sessionKey)
	}

	exportFile := strings.TrimSpace(input.FileName)
	if exportFile == "" && strings.TrimSpace(input.FilePath) != "" {
		exportFile = filepath.Base(strings.TrimSpace(input.FilePath))
	}
	if exportFile == "" && req.Preview != nil && strings.TrimSpace(req.Preview.FilePath) != "" {
		exportFile = filepath.Base(strings.TrimSpace(req.Preview.FilePath))
	}
	if exportFile == "" {
		exportFile = "(inline media)"
	}

	reason := "send_media 请求向外部频道发送文件或媒体"
	if strings.TrimSpace(input.Message) != "" {
		reason = "send_media 请求向外部频道发送文件或媒体，并附带消息"
	}

	requestedAt := time.Now()
	if req.CreatedAtMs > 0 {
		requestedAt = time.UnixMilli(req.CreatedAtMs)
	}

	return TypedApprovalRequest{
		Type:             ApprovalTypeDataExport,
		ID:               req.ID,
		Reason:           reason,
		TTLMinutes:       5,
		RequestedAt:      requestedAt,
		SessionKey:       sessionKey,
		OriginatorChatID: originatorChatID,
		OriginatorUserID: originatorUserID,
		TargetChannel:    targetChannel,
		ExportFiles:      []string{exportFile},
		Sanitized:        false,
		Workflow:         req.Workflow,
	}
}

func buildResultReviewApproval(
	req runner.ResultApprovalRequest,
	sessionKey, originatorChatID, originatorUserID string,
) TypedApprovalRequest {
	requestedAt := time.Now()
	if req.CreatedAtMs > 0 {
		requestedAt = time.UnixMilli(req.CreatedAtMs)
	}
	return TypedApprovalRequest{
		Type:             ApprovalTypeResultReview,
		ID:               req.ID,
		Reason:           "子智能体结果需要最终签收",
		TTLMinutes:       3,
		RequestedAt:      requestedAt,
		SessionKey:       sessionKey,
		OriginatorChatID: originatorChatID,
		OriginatorUserID: originatorUserID,
		ResultSummary:    req.Result,
		ReviewSummary:    req.ReviewSummary,
		Workflow:         req.Workflow,
	}
}

// Phase 返回当前阶段。
func (s *GatewayState) Phase() BootPhase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phase
}

// SetPhase 设置当前阶段。
func (s *GatewayState) SetPhase(phase BootPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
}

// Broadcaster 返回广播器。
func (s *GatewayState) Broadcaster() *Broadcaster { return s.broadcaster }

// ChatState 返回聊天状态。
func (s *GatewayState) ChatState() *ChatRunState { return s.chatState }

// ToolRegistry 返回工具注册表。
func (s *GatewayState) ToolRegistry() *ToolRegistry { return s.toolReg }

// EventDispatcher 返回事件分发器。
func (s *GatewayState) EventDispatcher() *NodeEventDispatcher { return s.eventDisp }

// EscalationMgr 返回权限提升管理器。
func (s *GatewayState) EscalationMgr() *EscalationManager { return s.escalationMgr }

// RemoteApprovalNotifier 返回远程审批通知管理器。
func (s *GatewayState) RemoteApprovalNotifier() *RemoteApprovalNotifier {
	return s.remoteApprovalNotifier
}

// TaskPresetMgr 返回任务预设权限管理器。
func (s *GatewayState) TaskPresetMgr() *TaskPresetManager { return s.taskPresetMgr }

// ChannelMgr 返回频道插件管理器。
func (s *GatewayState) ChannelMgr() *channels.Manager { return s.channelMgr }

// SandboxPool 返回沙箱容器池（可能为 nil）。
func (s *GatewayState) SandboxPool() *sandbox.ContainerPool { return s.sandboxPool }

// SandboxWorker 返回沙箱工作池（可能为 nil）。
func (s *GatewayState) SandboxWorker() *sandbox.Worker { return s.sandboxWorker }

// SandboxHub 返回沙箱进度推送 Hub（可能为 nil）。
func (s *GatewayState) SandboxHub() *sandbox.ProgressHub { return s.sandboxHub }

// SandboxStore 返回沙箱任务存储（可能为 nil）。
func (s *GatewayState) SandboxStore() *sandbox.TaskStore { return s.sandboxStore }

// StopSandbox 优雅关闭沙箱子系统。
func (s *GatewayState) StopSandbox() {
	if s.sandboxWorker != nil {
		s.sandboxWorker.Stop()
	}
	if s.sandboxPool != nil {
		s.sandboxPool.Stop()
	}
}

// ArgusBridge 返回 Argus 视觉子智能体 Bridge（可能为 nil，并发安全）。
func (s *GatewayState) ArgusBridge() *argus.Bridge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.argusBridge
}

// SetArgusBridge 设置 Argus Bridge（并发安全，用于运行时 UI 重试创建）。
func (s *GatewayState) SetArgusBridge(b *argus.Bridge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.argusBridge = b
}

// StopArgus 优雅关闭 Argus 子智能体。
func (s *GatewayState) StopArgus() {
	b := s.ArgusBridge()
	if b != nil {
		b.Stop()
	}
}

// SetManagedChrome 设置自动拉起的 Chrome 实例。
// 如已有旧实例则先关闭，避免重复遗留后台进程。
func (s *GatewayState) SetManagedChrome(c managedChromeInstance) {
	s.mu.Lock()
	old := s.managedChrome
	s.managedChrome = c
	s.mu.Unlock()

	if old != nil && old != c {
		_ = old.Stop()
	}
}

// StopManagedChrome 优雅关闭自动拉起的 Chrome 实例。
func (s *GatewayState) StopManagedChrome() {
	s.mu.Lock()
	c := s.managedChrome
	s.managedChrome = nil
	s.mu.Unlock()

	if c != nil {
		_ = c.Stop()
	}
}

// (Phase 2A: CoderBridge/StopCoder 已删除 — oa-coder 升级为 spawn_coder_agent)

// RemoteMCPBridge 返回 MCP 远程工具 Bridge（可能为 nil）。
func (s *GatewayState) RemoteMCPBridge() *mcpremote.RemoteBridge { return s.remoteMCPBridge }

// SetRemoteMCPBridge 设置 MCP 远程工具 Bridge（由 server.go 启动时注入）。
func (s *GatewayState) SetRemoteMCPBridge(b *mcpremote.RemoteBridge) { s.remoteMCPBridge = b }

// StopRemoteMCP 优雅关闭 MCP 远程工具 Bridge。
func (s *GatewayState) StopRemoteMCP() {
	if s.remoteMCPBridge != nil {
		s.remoteMCPBridge.Stop()
	}
}

// McpLocalManager 返回 MCP 本地安装服务器管理器（可能为 nil）。
func (s *GatewayState) McpLocalManager() *mcpinstall.McpLocalManager { return s.mcpLocalManager }

// SetMcpLocalManager 设置 MCP 本地安装服务器管理器（由 server.go 启动时注入）。
func (s *GatewayState) SetMcpLocalManager(m *mcpinstall.McpLocalManager) { s.mcpLocalManager = m }

// StopLocalMCP 优雅关闭所有本地安装的 MCP 服务器。
func (s *GatewayState) StopLocalMCP() {
	if s.mcpLocalManager != nil {
		s.mcpLocalManager.StopAll()
	}
}

// NativeSandboxBridge 返回原生沙箱 Worker Bridge（可能为 nil）。
func (s *GatewayState) NativeSandboxBridge() *sandbox.NativeSandboxBridge {
	return s.nativeSandboxBridge
}

// StopNativeSandbox 优雅关闭原生沙箱 Worker Bridge。
func (s *GatewayState) StopNativeSandbox() {
	if s.nativeSandboxBridge != nil {
		s.nativeSandboxBridge.Stop()
	}
}

// SetChannelMonitorMgr 设置 Monitor 频道管理器（由 server.go 启动时注入）。
func (s *GatewayState) SetChannelMonitorMgr(mgr *ChannelMonitorManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelMonitorMgr = mgr
}

// GetChannelMonitorMgr 返回 Monitor 频道管理器（可能为 nil）。
func (s *GatewayState) GetChannelMonitorMgr() *ChannelMonitorManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.channelMonitorMgr
}

// UHMSManager 返回 UHMS 记忆管理器（可能为 nil）。
func (s *GatewayState) UHMSManager() *uhms.DefaultManager { return s.uhmsManager }

// SetUHMSManager 设置 UHMS 记忆管理器（由 server.go 启动时注入）。
func (s *GatewayState) SetUHMSManager(m *uhms.DefaultManager) { s.uhmsManager = m }

// UHMSBootMgr 返回 UHMS Boot 文件管理器（可能为 nil）。
func (s *GatewayState) UHMSBootMgr() *uhms.BootManager { return s.uhmsBootMgr }

// SetUHMSBootMgr 设置 UHMS Boot 文件管理器（由 server.go 启动时注入）。
func (s *GatewayState) SetUHMSBootMgr(bm *uhms.BootManager) { s.uhmsBootMgr = bm }

// UHMSVFS 返回 UHMS VFS 实例（可能为 nil）。
// 用于技能分级状态检查等场景。
func (s *GatewayState) UHMSVFS() *uhms.LocalVFS {
	if s.uhmsManager == nil {
		return nil
	}
	return s.uhmsManager.VFS()
}

// ContractStore 返回合约 VFS 持久化实例（可能为 nil）。
func (s *GatewayState) ContractStore() *VFSContractPersistence { return s.contractStore }

// AuthManager 返回 OAuth Token Manager（可能为 nil，并发安全）。
func (s *GatewayState) AuthManager() *mcpremote.OAuthTokenManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authManager
}

// SetAuthManager 设置 OAuth Token Manager。
func (s *GatewayState) SetAuthManager(m *mcpremote.OAuthTokenManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authManager = m
}

// ManagedCatalog 返回托管模型目录（可能为 nil，并发安全）。
func (s *GatewayState) ManagedCatalog() *models.ManagedModelCatalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.managedCatalog
}

// SetManagedCatalog 设置托管模型目录。
func (s *GatewayState) SetManagedCatalog(c *models.ManagedModelCatalog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedCatalog = c
}

// PackageCatalog 返回统一应用中心目录（可能为 nil，并发安全）。
func (s *GatewayState) PackageCatalog() *packages.PackageCatalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.packageCatalog
}

// SetPackageCatalog 设置统一应用中心目录（并发安全）。
func (s *GatewayState) SetPackageCatalog(c *packages.PackageCatalog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packageCatalog = c
}

// PackageLedger 返回安装账本（可能为 nil，并发安全）。
func (s *GatewayState) PackageLedger() *packages.PackageLedger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.packageLedger
}

// SetPackageLedger 设置安装账本（并发安全）。
func (s *GatewayState) SetPackageLedger(l *packages.PackageLedger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packageLedger = l
}

// PackageInstaller 返回安装编排器（可能为 nil，并发安全）。
func (s *GatewayState) PackageInstaller() *packages.PackageInstaller {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.packageInstaller
}

// SetPackageInstaller 设置安装编排器（并发安全）。
func (s *GatewayState) SetPackageInstaller(i *packages.PackageInstaller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packageInstaller = i
}

// StopUHMS 优雅关闭 UHMS 记忆系统。
func (s *GatewayState) StopUHMS() {
	if s.uhmsManager != nil {
		s.uhmsManager.Close()
	}
}

// CoderConfirmMgr 返回 Coder 确认管理器（可能为 nil）。
func (s *GatewayState) CoderConfirmMgr() *runner.CoderConfirmationManager {
	return s.coderConfirmMgr
}

// PlanConfirmMgr 返回方案确认管理器（可能为 nil）。
func (s *GatewayState) PlanConfirmMgr() *runner.PlanConfirmationManager {
	return s.planConfirmMgr
}

// ResultApprovalMgr 返回结果签收管理器（可能为 nil）。
func (s *GatewayState) ResultApprovalMgr() *runner.ResultApprovalManager {
	return s.resultApprovalMgr
}

// resolveArgusBinaryPath 解析 Argus 二进制路径。
//
// 优先级 (ARGUS-003 统一顺序):
//  1. $ARGUS_BINARY_PATH（显式覆盖）
//  2. subAgents.screenObserver.binaryPath 配置字段（ARGUS-002: 持久化配置）
//  3. .app bundle 内已签名二进制（方案 A — 授权持久化最佳路径）
//  4. ~/.openacosmi/bin/argus-sensory（裸二进制，macOS 自动签名兜底）
//  5. argus-sensory（PATH 查找，macOS 自动签名兜底）
func resolveArgusBinaryPath(configBinaryPath string) string {
	path, _ := resolveArgusBinaryPathWithError(configBinaryPath)
	return path
}

// ResolveTrace 路径解析追踪条目（ARGUS-004: 一键诊断用）。
type ResolveTrace struct {
	Layer string `json:"layer"` // "env" | "config" | "app_bundle" | "user_bin" | "path"
	Path  string `json:"path"`
	Found bool   `json:"found"`
}

// ResolveResult 统一的二进制解析结果（ARGUS-003/004）。
type ResolveResult struct {
	Path  string                 `json:"path"`
	Trace []ResolveTrace         `json:"trace"`
	Error *argus.ArgusStartError `json:"error,omitempty"`
}

// resolveArgusBinaryPathWithError 解析 Argus 二进制路径，失败时返回结构化错误。
func resolveArgusBinaryPathWithError(configBinaryPath string) (string, *argus.ArgusStartError) {
	result := resolveArgusBinaryFull(configBinaryPath)
	return result.Path, result.Error
}

// resolveArgusBinaryFull 完整解析 Argus 二进制路径，包含每层 trace（供 argus.diagnose 使用）。
func resolveArgusBinaryFull(configBinaryPath string) ResolveResult {
	var trace []ResolveTrace

	// 1. 环境变量显式指定
	if v := os.Getenv("ARGUS_BINARY_PATH"); v != "" {
		if _, err := os.Stat(v); err != nil {
			slog.Warn("argus: $ARGUS_BINARY_PATH points to invalid path",
				"path", v, "error", err)
			trace = append(trace, ResolveTrace{Layer: "env", Path: v, Found: false})
			return ResolveResult{
				Trace: trace,
				Error: &argus.ArgusStartError{
					Phase:    "resolve",
					Reason:   "env_path_invalid",
					Recovery: fmt.Sprintf("$ARGUS_BINARY_PATH=%s does not exist. Fix the path or unset the variable.", v),
					Err:      err,
				},
			}
		}
		trace = append(trace, ResolveTrace{Layer: "env", Path: v, Found: true})
		return ResolveResult{Path: v, Trace: trace}
	}

	// 2. 配置层 binaryPath（ARGUS-002: subAgents.screenObserver.binaryPath）
	if configBinaryPath != "" {
		if _, err := os.Stat(configBinaryPath); err == nil {
			trace = append(trace, ResolveTrace{Layer: "config", Path: configBinaryPath, Found: true})
			slog.Info("argus: using configured binaryPath (persistent)",
				"path", configBinaryPath)
			return ResolveResult{Path: configBinaryPath, Trace: trace}
		}
		slog.Warn("argus: configured binaryPath invalid, trying next layer",
			"path", configBinaryPath)
		trace = append(trace, ResolveTrace{Layer: "config", Path: configBinaryPath, Found: false})
	}

	// 3. 方案 A：优先使用 .app bundle 内二进制（已持久化签名，TCC 授权不丢失）
	if bundleBin := argus.FindAppBundleBinary(); bundleBin != "" {
		slog.Info("argus: using .app bundle binary (persistent authorization)",
			"path", bundleBin)
		trace = append(trace, ResolveTrace{Layer: "app_bundle", Path: bundleBin, Found: true})
		return ResolveResult{Path: bundleBin, Trace: trace}
	}
	trace = append(trace, ResolveTrace{Layer: "app_bundle", Path: "(candidates searched)", Found: false})

	// 4. 用户级安装
	userBinDir := filepath.Join(statepaths.ResolveStateDir(), "bin")
	if strings.TrimSpace(userBinDir) != "" {
		candidate := filepath.Join(userBinDir, "argus-sensory")
		if _, err := os.Stat(candidate); err == nil {
			trace = append(trace, ResolveTrace{Layer: "user_bin", Path: candidate, Found: true})
			return ResolveResult{Path: candidate, Trace: trace}
		}
		trace = append(trace, ResolveTrace{Layer: "user_bin", Path: candidate, Found: false})
	}

	// 5. PATH 查找（ARGUS-005: 使用 exec.LookPath 搜索并返回绝对路径）
	if resolved, err := argus.ResolveBinary("argus-sensory"); err == nil {
		trace = append(trace, ResolveTrace{Layer: "path", Path: resolved, Found: true})
		return ResolveResult{Path: resolved, Trace: trace}
	}
	trace = append(trace, ResolveTrace{Layer: "path", Path: "argus-sensory", Found: false})

	// 未找到任何可用二进制
	return ResolveResult{
		Trace: trace,
		Error: &argus.ArgusStartError{
			Phase:    "resolve",
			Reason:   "binary_not_found",
			Recovery: fmt.Sprintf("Install argus-sensory: place it in %s or add to PATH, or set $ARGUS_BINARY_PATH, or configure subAgents.screenObserver.binaryPath.", userBinDir),
		},
	}
}

// resolveNativeSandboxBinaryPath 解析原生沙箱 CLI 二进制路径。
//
// 优先级:
//  1. $OA_CLI_BINARY（测试/开发覆盖）
//  2. ~/.openacosmi/bin/openacosmi（用户级安装）
//  3. openacosmi（PATH 查找）
func resolveNativeSandboxBinaryPath() string {
	if v := os.Getenv("OA_CLI_BINARY"); v != "" {
		return v
	}
	userBinDir := filepath.Join(statepaths.ResolveStateDir(), "bin")
	candidate := filepath.Join(userBinDir, "openacosmi")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "openacosmi"
}

// (Phase 2A: resolveCoderBinaryPath 已删除 — Coder MCP 进程不再由 Go 启动)

// ---------- 健康检查 ----------

// HealthStatus 健康检查响应。
type HealthStatus struct {
	Status  string `json:"status"` // "ok" | "starting" | "stopping"
	Phase   string `json:"phase"`
	Version string `json:"version,omitempty"`
}

// GetHealthStatus 返回健康检查状态。
func GetHealthStatus(state *GatewayState, version string) HealthStatus {
	phase := state.Phase()
	status := "ok"
	switch phase {
	case BootPhaseInit, BootPhaseStarting:
		status = "starting"
	case BootPhaseStopping, BootPhaseStopped:
		status = "stopping"
	}
	return HealthStatus{Status: status, Phase: string(phase), Version: version}
}

// ---------- 启动验证 ----------

// ValidateBootConfig 校验启动配置。
func ValidateBootConfig(cfg BootConfig) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", cfg.Server.Port)
	}
	if err := AssertGatewayAuthConfigured(cfg.Auth); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}
	return nil
}
