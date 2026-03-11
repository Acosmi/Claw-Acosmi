package gateway

// server_methods_system.go — system-presence, system-event, last-heartbeat, set-heartbeats
// 对应 TS src/gateway/server-methods/system.ts

import (
	"strings"
)

// SystemHandlers 返回系统类方法处理器映射。
func SystemHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"system-presence":     handleSystemPresence,
		"system-event":        handleSystemEvent,
		"system.restart":      handleSystemRestart,
		"last-heartbeat":      handleLastHeartbeat,
		"set-heartbeats":      handleSetHeartbeats,
		"system.openExternal": handleSystemOpenExternal,
	}
}

// ---------- system-presence ----------
// 对应 TS system.ts L29-32
// 返回所有在线设备/节点的 presence 列表。

func handleSystemPresence(ctx *MethodHandlerContext) {
	store := ctx.Context.PresenceStore
	if store == nil {
		ctx.Respond(true, []*PresenceEntry{}, nil)
		return
	}
	ctx.Respond(true, store.List(), nil)
}

// ---------- system-event ----------
// 对应 TS system.ts L33-139
// 接收设备事件，更新 presence，检测变更，广播。

func handleSystemEvent(ctx *MethodHandlerContext) {
	text := readString(ctx.Params, "text")
	if text == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "text required"))
		return
	}

	store := ctx.Context.PresenceStore
	if store == nil {
		ctx.Respond(true, map[string]interface{}{"ok": true}, nil)
		return
	}

	entry := &PresenceEntry{
		Text:            text,
		DeviceID:        readString(ctx.Params, "deviceId"),
		InstanceID:      readString(ctx.Params, "instanceId"),
		Host:            readString(ctx.Params, "host"),
		IP:              readString(ctx.Params, "ip"),
		Mode:            readString(ctx.Params, "mode"),
		Version:         readString(ctx.Params, "version"),
		Platform:        readString(ctx.Params, "platform"),
		DeviceFamily:    readString(ctx.Params, "deviceFamily"),
		ModelIdentifier: readString(ctx.Params, "modelIdentifier"),
		Reason:          readString(ctx.Params, "reason"),
	}

	if v, ok := ctx.Params["lastInputSeconds"]; ok {
		if f, ok := v.(float64); ok {
			entry.LastInputSeconds = int(f)
		}
	}

	if v, ok := ctx.Params["roles"]; ok {
		entry.Roles = readStringSlice(v)
	}
	if v, ok := ctx.Params["scopes"]; ok {
		entry.Scopes = readStringSlice(v)
	}
	if v, ok := ctx.Params["tags"]; ok {
		entry.Tags = readStringSlice(v)
	}

	result := store.Update(entry)

	// Node: 前缀 → delta 检测 → enqueue 系统事件
	isNodePresenceLine := strings.HasPrefix(text, "Node:")
	eventQueue := ctx.Context.EventQueue
	if isNodePresenceLine && len(result.ChangedKeys) > 0 {
		changedSet := make(map[string]bool)
		for _, k := range result.ChangedKeys {
			changedSet[k] = true
		}

		reason := entry.Reason
		normalizedReason := strings.ToLower(reason)
		ignoreReason := strings.HasPrefix(normalizedReason, "periodic") || normalizedReason == "heartbeat"

		hostChanged := changedSet["host"]
		ipChanged := changedSet["ip"]
		versionChanged := changedSet["version"]
		modeChanged := changedSet["mode"]
		reasonChanged := changedSet["reason"] && !ignoreReason

		hasChanges := hostChanged || ipChanged || versionChanged || modeChanged || reasonChanged
		if hasChanges && eventQueue != nil {
			contextChanged := eventQueue.IsContextChanged("", result.Key)

			var parts []string
			if contextChanged || hostChanged || ipChanged {
				hostLabel := strings.TrimSpace(entry.Host)
				if hostLabel == "" {
					hostLabel = "Unknown"
				}
				ipLabel := strings.TrimSpace(entry.IP)
				if ipLabel != "" {
					parts = append(parts, "Node: "+hostLabel+" ("+ipLabel+")")
				} else {
					parts = append(parts, "Node: "+hostLabel)
				}
			}
			if versionChanged {
				v := strings.TrimSpace(entry.Version)
				if v == "" {
					v = "unknown"
				}
				parts = append(parts, "app "+v)
			}
			if modeChanged {
				m := strings.TrimSpace(entry.Mode)
				if m == "" {
					m = "unknown"
				}
				parts = append(parts, "mode "+m)
			}
			if reasonChanged {
				r := strings.TrimSpace(reason)
				if r == "" {
					r = "event"
				}
				parts = append(parts, "reason "+r)
			}

			deltaText := strings.Join(parts, " · ")
			if deltaText != "" {
				eventQueue.Enqueue(deltaText, "", result.Key)
			}
		}
	} else if !isNodePresenceLine && eventQueue != nil {
		eventQueue.Enqueue(text, "", "")
	}

	// 广播 presence 更新
	if bc := ctx.Context.Broadcaster; bc != nil {
		bc.Broadcast("presence", map[string]interface{}{
			"presence": store.List(),
		}, nil)
	}

	ctx.Respond(true, map[string]interface{}{"ok": true}, nil)
}

func handleSystemRestart(ctx *MethodHandlerContext) {
	restarter := ctx.Context.GatewayRestarter
	if restarter == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "gateway restarter not available"))
		return
	}

	reason := strings.TrimSpace(readString(ctx.Params, "reason"))
	if reason == "" {
		reason = "system.restart"
	}

	var delayPtr *int
	if raw, ok := ctx.Params["delayMs"]; ok {
		switch v := raw.(type) {
		case float64:
			delay := int(v)
			if delay < 0 {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "delayMs must be >= 0"))
				return
			}
			delayPtr = &delay
		case int:
			delay := v
			if delay < 0 {
				ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "delayMs must be >= 0"))
				return
			}
			delayPtr = &delay
		}
	}

	result := restarter.ScheduleRestart(GatewayRestartPlan{
		DelayMs: delayPtr,
		Reason:  reason,
	})
	if result == nil {
		result = &GatewayRestartResult{Scheduled: false, Reason: reason}
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":      true,
		"restart": result,
	}, nil)
}

// ---------- last-heartbeat ----------
// 对应 TS system.ts L10-12

func handleLastHeartbeat(ctx *MethodHandlerContext) {
	hb := ctx.Context.HeartbeatState
	if hb == nil {
		ctx.Respond(true, nil, nil)
		return
	}
	last := hb.GetLast()
	ctx.Respond(true, last, nil)
}

// ---------- set-heartbeats ----------
// 对应 TS system.ts L13-28

func handleSetHeartbeats(ctx *MethodHandlerContext) {
	enabledRaw, ok := ctx.Params["enabled"]
	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid set-heartbeats params: enabled (boolean) required"))
		return
	}
	enabled, ok := enabledRaw.(bool)
	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid set-heartbeats params: enabled (boolean) required"))
		return
	}

	hb := ctx.Context.HeartbeatState
	if hb != nil {
		hb.SetEnabled(enabled)
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":      true,
		"enabled": enabled,
	}, nil)
}

func handleSystemOpenExternal(ctx *MethodHandlerContext) {
	url := readString(ctx.Params, "url")
	if url == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "url required"))
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "url must start with http:// or https://"))
		return
	}
	if !OpenURL(url) {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to open external url"))
		return
	}
	ctx.Respond(true, map[string]interface{}{"ok": true}, nil)
}

// ---------- 辅助函数 ----------

func readString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func readStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if ok {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
