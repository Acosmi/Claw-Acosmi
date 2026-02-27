package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/anthropic/open-acosmi/internal/agents/runner"
	"github.com/anthropic/open-acosmi/internal/argus"
	"github.com/anthropic/open-acosmi/internal/channels"
	"github.com/anthropic/open-acosmi/internal/memory/uhms"
	"github.com/anthropic/open-acosmi/internal/sandbox"
	"github.com/anthropic/open-acosmi/pkg/mcpremote"
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

	// 原生沙箱 Worker Bridge（可选 — 仅 CLI 二进制可用时初始化）
	nativeSandboxBridge *sandbox.NativeSandboxBridge

	// UHMS 记忆系统（可选 — 仅配置启用时初始化）
	uhmsManager *uhms.DefaultManager

	// Coder 确认管理器（可选 — 仅 coder bridge 可用时初始化）
	coderConfirmMgr *runner.CoderConfirmationManager
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
	argusPath := resolveArgusBinaryPath()
	if argus.IsAvailable(argusPath) {
		// 方案 B 兜底：裸二进制自动签名，确保 macOS TCC 授权持久化
		if err := argus.EnsureCodeSigned(argusPath); err != nil {
			slog.Debug("argus: code signing skipped (non-fatal)", "error", err)
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
		if err := bridge.Start(); err != nil {
			slog.Warn("gateway: argus bridge start failed (non-fatal)", "error", err)
		} else {
			s.argusBridge = bridge
		}
	} else {
		slog.Info("gateway: Argus binary not available, visual agent disabled")
	}

	// (Phase 2A: Coder Bridge MCP 启动已删除 — oa-coder 升级为独立 LLM Agent Session)

	// ── 命令审批门控（Ask 规则 + Coder 确认） ──────────────────
	// 无条件初始化：bash Ask 规则需要真正阻塞审批（fail-closed 安全策略），
	// 不依赖 Coder Bridge 是否存在。前端通过 coder.confirm.* 事件处理。
	s.coderConfirmMgr = runner.NewCoderConfirmationManager(func(event string, payload interface{}) {
		bc.Broadcast(event, payload, nil)
	}, 60*time.Second)
	slog.Info("gateway: command approval gate initialized")

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
		slog.Info("gateway: UHMS disabled (enable via config memory.uhms.enabled)")
	}

	return s
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

// ArgusBridge 返回 Argus 视觉子智能体 Bridge（可能为 nil）。
func (s *GatewayState) ArgusBridge() *argus.Bridge { return s.argusBridge }

// StopArgus 优雅关闭 Argus 子智能体。
func (s *GatewayState) StopArgus() {
	if s.argusBridge != nil {
		s.argusBridge.Stop()
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

// UHMSManager 返回 UHMS 记忆管理器（可能为 nil）。
func (s *GatewayState) UHMSManager() *uhms.DefaultManager { return s.uhmsManager }

// SetUHMSManager 设置 UHMS 记忆管理器（由 server.go 启动时注入）。
func (s *GatewayState) SetUHMSManager(m *uhms.DefaultManager) { s.uhmsManager = m }

// UHMSVFS 返回 UHMS VFS 实例（可能为 nil）。
// 用于技能分级状态检查等场景。
func (s *GatewayState) UHMSVFS() *uhms.LocalVFS {
	if s.uhmsManager == nil {
		return nil
	}
	return s.uhmsManager.VFS()
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

// resolveArgusBinaryPath 解析 Argus 二进制路径。
//
// 优先级:
//  1. $ARGUS_BINARY_PATH（显式覆盖）
//  2. .app bundle 内已签名二进制（方案 A — 授权持久化最佳路径）
//  3. ~/.openacosmi/bin/argus-sensory（裸二进制，macOS 自动签名兜底）
//  4. argus-sensory（PATH 查找，macOS 自动签名兜底）
func resolveArgusBinaryPath() string {
	// 1. 环境变量显式指定
	if v := os.Getenv("ARGUS_BINARY_PATH"); v != "" {
		return v
	}

	// 2. 方案 A：优先使用 .app bundle 内二进制（已持久化签名，TCC 授权不丢失）
	if bundleBin := argus.FindAppBundleBinary(); bundleBin != "" {
		slog.Info("argus: using .app bundle binary (persistent authorization)",
			"path", bundleBin)
		return bundleBin
	}

	// 3. 用户级安装
	home, err := os.UserHomeDir()
	if err == nil {
		candidate := home + "/.openacosmi/bin/argus-sensory"
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 4. PATH 查找
	return "argus-sensory"
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
	home, err := os.UserHomeDir()
	if err == nil {
		candidate := home + "/.openacosmi/bin/openacosmi"
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
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
