package gateway

// server_methods_argus.go — argus.* RPC 方法
//
// 静态方法:
//   argus.status — Bridge 状态 + 工具清单 + PID + 健康信息
//   argus.approval.resolve — 审批决策中继（Phase 2 预留）
//
// 动态方法:
//   RegisterArgusDynamicMethods() 遍历工具列表，为每个工具注册 argus.<tool_name>

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/argus"
)

// ArgusHandlers 返回 argus.* 静态方法映射。
func ArgusHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"argus.status":           handleArgusStatus,
		"argus.restart":          handleArgusRestart,
		"argus.approval.resolve": handleArgusApprovalResolve,
		"argus.permission.check": handleArgusPermissionCheck,
		"argus.diagnose":         handleArgusDiagnose, // ARGUS-004: 一键诊断
	}
}

// ---------- argus.permission.check ----------

func handleArgusPermissionCheck(ctx *MethodHandlerContext) {
	result := resolveArgusBinaryFull(resolveLiveArgusBinaryPath(ctx))
	tcc := argus.CheckTCCPermissionsForBinary(result.Path)

	// ARGUS-006: 构建结构化恢复动作（供前端 UI 渲染按钮）
	var actions []map[string]string
	if tcc.ScreenRecording != argus.PermGranted {
		actions = append(actions, map[string]string{
			"action": "open_settings",
			"target": "screen_recording",
			"label":  "Open Screen Recording Settings",
			"hint":   "System Settings > Privacy & Security > Screen Recording",
		})
	}
	if tcc.Accessibility != argus.PermGranted {
		actions = append(actions, map[string]string{
			"action": "open_settings",
			"target": "accessibility",
			"label":  "Open Accessibility Settings",
			"hint":   "System Settings > Privacy & Security > Accessibility",
		})
	}
	if tcc.HasRequiredPermissions() {
		// 权限已获得 → 提供一键重试启动
		bridge := ctx.Context.State.ArgusBridge()
		if bridge == nil || bridge.State() == argus.BridgeStateStopped {
			actions = append(actions, map[string]string{
				"action": "retry_start",
				"label":  "Retry Start Argus",
				"hint":   "Permissions granted. Click to start Argus.",
			})
		}
	}

	// Bridge 状态（供前端判断是否显示重试按钮）
	bridge := ctx.Context.State.ArgusBridge()
	bridgeState := "not_configured"
	if bridge != nil {
		bridgeState = string(bridge.State())
	}

	ctx.Respond(true, map[string]interface{}{
		"screen_recording":           string(tcc.ScreenRecording),
		"accessibility":              string(tcc.Accessibility),
		"all_granted":                tcc.HasRequiredPermissions(),
		"recovery":                   tcc.Recovery(),
		"screen_recording_expiring":  tcc.ScreenRecordingExpiring,
		"screen_recording_days_left": tcc.ScreenRecordingDaysLeft,
		"permission_subject":         tcc.PermissionSubject,
		"permission_path":            tcc.PermissionPath,
		"detection_mode":             tcc.DetectionMode,
		"bridge_state":               bridgeState,
		"actions":                    actions,
	}, nil)
}

// ---------- argus.status ----------

func handleArgusStatus(ctx *MethodHandlerContext) {
	bridge := ctx.Context.ArgusBridge
	if bridge == nil {
		ctx.Respond(true, map[string]interface{}{
			"available": false,
			"state":     "not_configured",
			"message":   "Argus binary not available or not configured",
		}, nil)
		return
	}

	tools := bridge.Tools()
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}

	lastPing, lastRTT := bridge.LastPing()

	ctx.Respond(true, map[string]interface{}{
		"available": true,
		"state":     string(bridge.State()),
		"pid":       bridge.PID(),
		"tools":     toolNames,
		"toolCount": len(tools),
		"lastPing":  lastPing.UnixMilli(),
		"lastRTTMs": lastRTT.Milliseconds(),
	}, nil)
}

// ---------- argus.restart ----------
// 在熔断或 stopped 状态后手动重启 Argus 子进程。

func handleArgusRestart(ctx *MethodHandlerContext) {
	bridge := ctx.Context.ArgusBridge
	if bridge == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "Argus binary not available or not configured"))
		return
	}

	state := bridge.State()
	if state == argus.BridgeStateReady {
		ctx.Respond(true, map[string]interface{}{
			"ok":      true,
			"message": "Argus is already running",
			"state":   string(state),
		}, nil)
		return
	}

	// 先停止残留进程
	if state != argus.BridgeStateStopped && state != argus.BridgeStateInit {
		bridge.Stop()
	}

	// 重新启动
	if err := bridge.Start(); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "argus restart failed: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":      true,
		"message": "Argus restarted successfully",
		"state":   string(bridge.State()),
		"pid":     bridge.PID(),
	}, nil)
}

// ---------- argus.approval.resolve ----------
// Phase 2 预留：审批决策中继。

func handleArgusApprovalResolve(ctx *MethodHandlerContext) {
	// Phase 2: 实现审批决策中继到 Argus ApprovalGateway
	ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "argus.approval.resolve: not yet implemented (Phase 2)"))
}

// ---------- argus.diagnose ----------
// ARGUS-004: 一键诊断 — 输出 resolvedPath/exists/executable/codesign/TCC/recovery/trace。

func handleArgusDiagnose(ctx *MethodHandlerContext) {
	// 运行完整 resolver 获取路径和 trace
	result := resolveArgusBinaryFull(resolveLiveArgusBinaryPath(ctx))

	// 二进制检查
	binaryCheck := argus.CheckBinary(result.Path)

	// TCC 权限检查
	tcc := argus.CheckTCCPermissionsForBinary(result.Path)

	// 签名状态（仅路径有效时检查）
	codesignStatus := "unknown"
	codesignIdentifier := ""
	if binaryCheck.Status == "available" {
		if argus.IsValidlySigned(result.Path) {
			codesignStatus = "valid"
		} else {
			codesignStatus = "unsigned_or_invalid"
		}
		codesignIdentifier = argus.CodeSignIdentifier(result.Path)
	}

	// Bridge 运行状态
	bridge := ctx.Context.State.ArgusBridge()
	bridgeState := "not_configured"
	var bridgePID int
	if bridge != nil {
		bridgeState = string(bridge.State())
		bridgePID = bridge.PID()
	}

	// 恢复建议
	recovery := ""
	if result.Error != nil {
		recovery = result.Error.Recovery
	} else if binaryCheck.Recovery != "" {
		recovery = binaryCheck.Recovery
	} else if !tcc.HasRequiredPermissions() {
		recovery = tcc.Recovery()
	}

	ctx.Respond(true, map[string]interface{}{
		"resolvedPath": result.Path,
		"exists":       binaryCheck.Status != "not_found",
		"executable":   binaryCheck.Status == "available",
		"codesign":     codesignStatus,
		"identifier":   codesignIdentifier,
		"tcc": map[string]interface{}{
			"screen_recording": string(tcc.ScreenRecording),
			"accessibility":    string(tcc.Accessibility),
			"all_granted":      tcc.HasRequiredPermissions(),
			"subject":          tcc.PermissionSubject,
			"path":             tcc.PermissionPath,
			"detection_mode":   tcc.DetectionMode,
		},
		"bridge": map[string]interface{}{
			"state": bridgeState,
			"pid":   bridgePID,
		},
		"trace":    result.Trace,
		"recovery": recovery,
	}, nil)
}

func resolveLiveArgusBinaryPath(ctx *MethodHandlerContext) string {
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
	return configBinaryPath
}

// ---------- 动态方法注册 ----------

// RegisterArgusDynamicMethods 遍历 Argus 工具列表，为每个工具注册 argus.<tool_name> 方法。
func RegisterArgusDynamicMethods(registry *MethodRegistry, bridge *argus.Bridge) {
	tools := bridge.Tools()
	for _, tool := range tools {
		toolName := tool.Name
		methodName := "argus." + toolName

		registry.Register(methodName, func(ctx *MethodHandlerContext) {
			b := ctx.Context.ArgusBridge
			if b == nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "argus bridge not available"))
				return
			}

			// 提取参数并序列化为 JSON
			var args json.RawMessage
			if params := ctx.Params; len(params) > 0 {
				data, err := json.Marshal(params)
				if err != nil {
					ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid params: "+err.Error()))
					return
				}
				args = data
			}

			// 解析超时
			timeout := 30 * time.Second
			if raw, ok := ctx.Params["_timeout"].(float64); ok && raw > 0 {
				timeout = time.Duration(raw) * time.Millisecond
			}

			// 调用 MCP 工具
			result, err := b.CallTool(context.Background(), toolName, args, timeout)
			if err != nil {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "argus tool call failed: "+err.Error()))
				return
			}

			// 解析 MCP content → 网关响应
			if result.IsError {
				errMsg := "argus tool error"
				if len(result.Content) > 0 && result.Content[0].Text != "" {
					errMsg = result.Content[0].Text
				}
				ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, errMsg))
				return
			}

			// 构建响应
			response := map[string]interface{}{
				"tool":    toolName,
				"content": result.Content,
			}
			ctx.Respond(true, response, nil)
		})
	}
}
