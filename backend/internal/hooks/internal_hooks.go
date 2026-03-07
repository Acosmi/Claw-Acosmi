package hooks

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ============================================================================
// 内部钩子注册表 — 事件驱动的 handler 注册/触发系统
// 对应 TS: internal-hooks.ts
// ============================================================================

var (
	internalMu       sync.RWMutex
	internalHandlers = make(map[string][]InternalHookHandler)
)

// RegisterInternalHook 注册内部钩子处理函数
// 对应 TS: internal-hooks.ts registerInternalHook
//
// eventKey 可以是事件类型（如 "command"）或具体动作（如 "command:new"）。
func RegisterInternalHook(eventKey string, handler InternalHookHandler) {
	internalMu.Lock()
	defer internalMu.Unlock()
	internalHandlers[eventKey] = append(internalHandlers[eventKey], handler)
}

// UnregisterInternalHook 取消注册指定 handler
// 对应 TS: internal-hooks.ts unregisterInternalHook
//
// 注意：Go 中函数不可比较，此处按指针比较。调用方需保存注册时的函数引用。
func UnregisterInternalHook(eventKey string, handler InternalHookHandler) {
	internalMu.Lock()
	defer internalMu.Unlock()

	handlers := internalHandlers[eventKey]
	if len(handlers) == 0 {
		return
	}

	// 按指针比较
	targetPtr := fmt.Sprintf("%p", handler)
	filtered := make([]InternalHookHandler, 0, len(handlers))
	for _, h := range handlers {
		if fmt.Sprintf("%p", h) != targetPtr {
			filtered = append(filtered, h)
		}
	}

	if len(filtered) == 0 {
		delete(internalHandlers, eventKey)
	} else {
		internalHandlers[eventKey] = filtered
	}
}

// ClearInternalHooks 清除所有注册的钩子（测试用）
// 对应 TS: internal-hooks.ts clearInternalHooks
func ClearInternalHooks() {
	internalMu.Lock()
	defer internalMu.Unlock()
	internalHandlers = make(map[string][]InternalHookHandler)
}

// GetRegisteredEventKeys 获取所有已注册的事件 key（调试用）
// 对应 TS: internal-hooks.ts getRegisteredEventKeys
func GetRegisteredEventKeys() []string {
	internalMu.RLock()
	defer internalMu.RUnlock()
	keys := make([]string, 0, len(internalHandlers))
	for k := range internalHandlers {
		keys = append(keys, k)
	}
	return keys
}

// TriggerInternalHook 触发内部钩子事件
// 对应 TS: internal-hooks.ts triggerInternalHook
//
// 依次调用：
// 1. 匹配事件类型的 handler（如 "command"）
// 2. 匹配 type:action 组合的 handler（如 "command:new"）
//
// 单个 handler 报错不影响后续 handler 执行。
func TriggerInternalHook(event *InternalHookEvent) {
	internalMu.RLock()
	typeKey := string(event.Type)
	specificKey := typeKey + ":" + event.Action

	var allHandlers []InternalHookHandler
	allHandlers = append(allHandlers, internalHandlers[typeKey]...)
	allHandlers = append(allHandlers, internalHandlers[specificKey]...)
	internalMu.RUnlock()

	if len(allHandlers) == 0 {
		return
	}

	for _, handler := range allHandlers {
		if err := handler(event); err != nil {
			slog.Error("Hook error",
				"event", typeKey+":"+event.Action,
				"error", err.Error(),
			)
		}
	}
}

// CreateInternalHookEvent 创建内部钩子事件
// 对应 TS: internal-hooks.ts createInternalHookEvent
func CreateInternalHookEvent(
	eventType InternalHookEventType,
	action string,
	sessionKey string,
	context map[string]interface{},
) *InternalHookEvent {
	if context == nil {
		context = make(map[string]interface{})
	}
	return &InternalHookEvent{
		Type:       eventType,
		Action:     action,
		SessionKey: sessionKey,
		Context:    context,
		Timestamp:  time.Now().UnixMilli(),
		Messages:   make([]string, 0),
	}
}

// IsAgentBootstrapEvent 检查事件是否为 agent bootstrap 事件
// 对应 TS: internal-hooks.ts isAgentBootstrapEvent
func IsAgentBootstrapEvent(event *InternalHookEvent) bool {
	if event.Type != HookEventAgent || event.Action != "bootstrap" {
		return false
	}
	if event.Context == nil {
		return false
	}
	if _, ok := event.Context["workspaceDir"].(string); !ok {
		return false
	}
	if _, ok := event.Context["workspaceBootstrapFiles"]; !ok {
		return false
	}
	return true
}
