package gateway

// server_methods_subagent.go — subagent.list / subagent.ctl
// 子智能体状态查询与控制 RPC。

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/argus"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

// argusBridgeMu 保护 Argus bridge 的并发创建（防止多个 WS 连接同时创建 bridge）。
var argusBridgeMu sync.Mutex

// SubagentHandlers 返回子智能体方法映射。
func SubagentHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"subagent.list": handleSubagentList,
		"subagent.ctl":  handleSubagentCtl,
	}
}

// ---------- subagent.list ----------

type subagentEntry struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Status     string `json:"status"` // "running" | "stopped" | "error" | "degraded" | "starting"
	Error      string `json:"error,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Configured bool   `json:"configured,omitempty"`
}

func handleSubagentList(ctx *MethodHandlerContext) {
	var entries []subagentEntry

	// 热加载最新配置（wizard 保存后 ctx.Context.Config 仍为启动快照）
	liveCfg := ctx.Context.Config
	if ctx.Context.ConfigLoader != nil {
		if fresh, err := ctx.Context.ConfigLoader.LoadConfig(); err == nil {
			liveCfg = fresh
		}
	}

	// 1. Argus 视觉子智能体 — 从 GatewayState 动态读取（而非连接快照）
	var bridge *argus.Bridge
	if ctx.Context.State != nil {
		bridge = ctx.Context.State.ArgusBridge()
	}
	entries = append(entries, buildArgusEntry(bridge))

	// 2. oa-coder 编程子智能体
	entries = append(entries, buildCoderEntry(ctx.Context.CoderConfirmMgr != nil, liveCfg))

	// 3. oa-media 媒体运营子智能体
	entries = append(entries, buildMediaEntry(ctx.Context.MediaSubsystem, liveCfg))

	ctx.Respond(true, map[string]interface{}{
		"agents": entries,
	}, nil)
}

func buildArgusEntry(bridge *argus.Bridge) subagentEntry {
	entry := subagentEntry{
		ID:    "argus-screen",
		Label: "灵瞳 Vision",
	}
	if bridge == nil {
		entry.Status = "stopped"
		entry.Error = "Argus binary not available"
		return entry
	}
	state := bridge.State()
	switch state {
	case argus.BridgeStateReady:
		entry.Status = "running"
	case argus.BridgeStateDegraded:
		entry.Status = "degraded"
	case argus.BridgeStateStarting:
		entry.Status = "starting"
	default:
		entry.Status = "stopped"
	}
	return entry
}

func persistScreenObserverEnabled(ctx *MethodHandlerContext, enabled bool) error {
	if ctx == nil || ctx.Context == nil || ctx.Context.ConfigLoader == nil {
		return nil
	}
	cfg, err := ctx.Context.ConfigLoader.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.ScreenObserver == nil {
		cfg.SubAgents.ScreenObserver = &types.ScreenObserverSettings{}
	}
	cfg.SubAgents.ScreenObserver.Enabled = &enabled
	if err := ctx.Context.ConfigLoader.WriteConfigFile(cfg); err != nil {
		return err
	}
	ctx.Context.ConfigLoader.ClearCache()
	return nil
}

func buildCoderEntry(confirmMgrAvailable bool, cfg *types.OpenAcosmiConfig) subagentEntry {
	entry := subagentEntry{
		ID:    "oa-coder",
		Label: "Open Coder",
	}
	// oa-coder 是按需 spawn 的 LLM session，不是持久进程。
	// 只要 CoderConfirmMgr 已初始化，说明 coder 子系统可用。
	if confirmMgrAvailable {
		entry.Status = "available"
	} else {
		entry.Status = "stopped"
	}

	// 填充 provider/model/configured 状态
	entry.Configured = cfg != nil && cfg.SubAgents != nil && cfg.SubAgents.OpenCoder != nil
	if entry.Configured {
		// 仅已显式配置时返回 provider/model，避免暴露 fallback 供应商品牌
		provider, model, _, _ := resolveOpenCoderConfig(cfg)
		entry.Provider = provider
		entry.Model = model
	}
	return entry
}

// ---------- subagent.ctl ----------

func handleSubagentCtl(ctx *MethodHandlerContext) {
	agentID, _ := ctx.Params["agent_id"].(string)
	action, _ := ctx.Params["action"].(string)
	if agentID == "" || action == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "agent_id and action required"))
		return
	}

	switch agentID {
	case "argus-screen":
		handleArgusCtl(ctx, action)
	case "oa-coder":
		handleCoderCtl(ctx, action)
	case "oa-media":
		handleMediaCtl(ctx, action)
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown agent: %s", agentID)))
	}
}

// sanitizePath 将绝对路径中的用户主目录替换为 ~，避免泄漏。
func sanitizePath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return strings.Replace(path, home, "~", 1)
	}
	return path
}

// argusStartErrorResponse 构造 Argus 启动失败的结构化错误响应。
func argusStartErrorResponse(err error) *ErrorShape {
	phase := "handshake"
	recovery := "Check argus-sensory binary and permissions. Try enabling again to retry."
	if strings.Contains(err.Error(), "start process") {
		phase = "crash"
	}
	reason := sanitizePath(err.Error())
	return NewErrorShape(ErrCodeServiceUnavailable, "argus start failed: "+reason).
		WithDetails(map[string]string{"phase": phase, "reason": reason, "recovery": recovery})
}

func handleArgusCtl(ctx *MethodHandlerContext, action string) {
	// 从 GatewayState 动态读取 bridge（而非连接级快照），确保跨连接可见性
	var bridge *argus.Bridge
	if ctx.Context.State != nil {
		bridge = ctx.Context.State.ArgusBridge()
	}
	switch action {
	case "set_enabled":
		enabled, _ := ctx.Params["value"].(bool)
		if enabled {
			if err := persistScreenObserverEnabled(ctx, true); err != nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to persist argus enabled state: "+err.Error()))
				return
			}
			// 加锁防止多个 WS 连接并发创建 bridge
			argusBridgeMu.Lock()
			defer argusBridgeMu.Unlock()

			// 重新读取（锁内）防止 check-then-act 竞态
			bridge = ctx.Context.State.ArgusBridge()

			// 路径 1: bridge 非 nil — 检查状态并尝试启动/重试
			if bridge != nil {
				state := bridge.State()
				if state == argus.BridgeStateReady || state == argus.BridgeStateStarting || state == argus.BridgeStateDegraded {
					ctx.Respond(true, map[string]interface{}{"ok": true, "state": string(state)}, nil)
					return
				}
				// 状态为 init/stopped — 尝试 (重新) 启动
				if err := bridge.Start(); err != nil {
					ctx.Respond(false, nil, argusStartErrorResponse(err))
					return
				}
				slog.Info("subagent.ctl: argus started via UI (retry)")
				ctx.Respond(true, map[string]interface{}{"ok": true, "state": string(bridge.State())}, nil)
				return
			}
			// 路径 2: bridge nil — 尝试发现二进制并创建新 bridge
			// ARGUS-002: 从 live config 读取显式配置的 binaryPath
			var configBinaryPath string
			liveCfg := ctx.Context.Config
			if ctx.Context.ConfigLoader != nil {
				if fresh, err := ctx.Context.ConfigLoader.LoadConfig(); err == nil {
					liveCfg = fresh
				}
			}
			if liveCfg != nil && liveCfg.SubAgents != nil && liveCfg.SubAgents.ScreenObserver != nil {
				configBinaryPath = liveCfg.SubAgents.ScreenObserver.BinaryPath
			}
			argusPath, resolveErr := resolveArgusBinaryPathWithError(configBinaryPath)
			if resolveErr != nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, resolveErr.Error()).
					WithDetails(map[string]string{"phase": resolveErr.Phase, "reason": resolveErr.Reason, "recovery": resolveErr.Recovery}))
				return
			}
			check := argus.CheckBinary(argusPath)
			if check.Status != "available" {
				recovery := sanitizePath(check.Recovery)
				ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "Argus binary not available: "+check.Status).
					WithDetails(map[string]string{"phase": "permission", "reason": check.Status, "recovery": recovery}))
				return
			}
			cfg := argus.DefaultBridgeConfig()
			cfg.BinaryPath = argusPath
			newBridge := argus.NewBridge(cfg)
			if err := newBridge.Start(); err != nil {
				// 启动失败也保留 bridge 以允许 UI 重试
				ctx.Context.State.SetArgusBridge(newBridge)
				ctx.Respond(false, nil, argusStartErrorResponse(err))
				return
			}
			// 持久化到 GatewayState（所有连接可见）
			ctx.Context.State.SetArgusBridge(newBridge)
			slog.Info("subagent.ctl: argus started via UI (new bridge)")
			ctx.Respond(true, map[string]interface{}{"ok": true, "state": string(newBridge.State())}, nil)
		} else {
			if err := persistScreenObserverEnabled(ctx, false); err != nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to persist argus enabled state: "+err.Error()))
				return
			}
			if bridge != nil {
				bridge.Stop()
				slog.Info("subagent.ctl: argus stopped via UI")
			}
			ctx.Respond(true, map[string]interface{}{"ok": true, "state": "stopped"}, nil)
		}

	case "set_interval_ms":
		val, ok := ctx.Params["value"].(float64)
		if !ok || val < 100 || val > 60000 {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "set_interval_ms: value must be 100-60000"))
			return
		}
		if bridge == nil || (bridge.State() != argus.BridgeStateReady && bridge.State() != argus.BridgeStateDegraded) {
			ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action, "applied": false, "reason": "argus_not_running"}, nil)
			return
		}
		// MCP 协议不支持直接设置 interval，需通过工具调用或重启时配置
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action, "applied": false, "reason": "not_yet_implemented"}, nil)

	case "set_goal":
		val, ok := ctx.Params["value"].(string)
		if !ok || val == "" || len(val) > 1000 {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "set_goal: value must be non-empty string (max 1000 chars)"))
			return
		}
		if bridge == nil || (bridge.State() != argus.BridgeStateReady && bridge.State() != argus.BridgeStateDegraded) {
			ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action, "applied": false, "reason": "argus_not_running"}, nil)
			return
		}
		// MCP 协议不支持直接设置 goal，需通过工具调用或重启时配置
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action, "applied": false, "reason": "not_yet_implemented"}, nil)

	case "set_vla_model":
		val, ok := ctx.Params["value"].(string)
		// 白名单与 NewVLAClient 工厂一致: anthropic（已实现）+ none（禁用）
		// 其他模型在工厂中未实现，不在白名单中暴露
		validModels := map[string]bool{"none": true, "anthropic": true}
		if !ok || !validModels[val] {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "set_vla_model: value must be one of: none, anthropic"))
			return
		}
		if bridge == nil || (bridge.State() != argus.BridgeStateReady && bridge.State() != argus.BridgeStateDegraded) {
			ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action, "applied": false, "reason": "argus_not_running"}, nil)
			return
		}
		// VLA 模型切换需要重建 VLAClient，当前 MCP bridge 不支持运行时切换
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": action, "applied": false, "reason": "not_yet_implemented"}, nil)

	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown action for argus: %s", action)))
	}
}

func handleCoderCtl(ctx *MethodHandlerContext, action string) {
	switch action {
	case "set_enabled":
		// oa-coder 是按需 spawn 的子智能体，enable/disable 仅影响前端 UI 状态。
		// 实际的 coder 启停由 agent run 时自动控制。
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": "set_enabled"}, nil)

	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown action for oa-coder: %s", action)))
	}
}

// ---------- oa-media ----------

func buildMediaEntry(sub interface{}, cfg *types.OpenAcosmiConfig) subagentEntry {
	entry := subagentEntry{
		ID:    "oa-media",
		Label: "媒体运营 Media Ops",
	}
	if sub == nil {
		entry.Status = "stopped"
		entry.Error = "media subsystem not initialized"
		return entry
	}
	entry.Status = "available"
	// 从配置中读取 LLM 信息
	if cfg != nil && cfg.SubAgents != nil && cfg.SubAgents.MediaAgent != nil {
		ma := cfg.SubAgents.MediaAgent
		entry.Provider = ma.Provider
		entry.Model = ma.Model
		entry.Configured = ma.Provider != "" || ma.Model != ""
	}
	return entry
}

func handleMediaCtl(ctx *MethodHandlerContext, action string) {
	switch action {
	case "set_enabled":
		// oa-media 是进程内子系统，始终随 gateway 运行。
		// set_enabled 仅控制 UI 显示状态。
		ctx.Respond(true, map[string]interface{}{"ok": true, "ack": "set_enabled"}, nil)
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, fmt.Sprintf("unknown action for oa-media: %s", action)))
	}
}
