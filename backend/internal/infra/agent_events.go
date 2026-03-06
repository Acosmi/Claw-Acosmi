package infra

// ============================================================================
// Agent 运行事件注册 — 全局 runId→AgentRunContext 映射
// 对应 TS: infra/agent-events.ts (84L)
// ============================================================================

import (
	"sync"
	"time"
)

// AgentEventStream 事件流类型。
type AgentEventStream string

const (
	StreamLifecycle AgentEventStream = "lifecycle"
	StreamTool      AgentEventStream = "tool"
	StreamAssistant AgentEventStream = "assistant"
	StreamError     AgentEventStream = "error"
	StreamProgress  AgentEventStream = "agent.progress"
)

// AgentEventPayload 事件负载。
type AgentEventPayload struct {
	RunID      string                 `json:"runId"`
	Seq        int64                  `json:"seq"`
	Stream     AgentEventStream       `json:"stream"`
	Ts         int64                  `json:"ts"`
	Data       map[string]interface{} `json:"data"`
	SessionKey string                 `json:"sessionKey,omitempty"`
}

// AgentRunContext 每次运行的上下文。
type AgentRunContext struct {
	SessionKey   string `json:"sessionKey,omitempty"`
	VerboseLevel string `json:"verboseLevel,omitempty"`
	IsHeartbeat  bool   `json:"isHeartbeat,omitempty"`
}

// AgentEventListener 事件监听回调。
type AgentEventListener func(evt AgentEventPayload)

// ---------- 全局注册表 ----------

var (
	agentMu        sync.RWMutex
	runContextById = make(map[string]*AgentRunContext)
	seqByRun       = make(map[string]int64)
	listenerMu     sync.RWMutex
	eventListeners []AgentEventListener
)

// RegisterAgentRunContext 注册或更新运行上下文。
// TS 对应: registerAgentRunContext()
func RegisterAgentRunContext(runID string, ctx AgentRunContext) {
	if runID == "" {
		return
	}
	agentMu.Lock()
	defer agentMu.Unlock()
	existing, ok := runContextById[runID]
	if !ok {
		copied := ctx
		runContextById[runID] = &copied
		return
	}
	if ctx.SessionKey != "" && existing.SessionKey != ctx.SessionKey {
		existing.SessionKey = ctx.SessionKey
	}
	if ctx.VerboseLevel != "" && existing.VerboseLevel != ctx.VerboseLevel {
		existing.VerboseLevel = ctx.VerboseLevel
	}
	if ctx.IsHeartbeat != existing.IsHeartbeat {
		existing.IsHeartbeat = ctx.IsHeartbeat
	}
}

// GetAgentRunContext 获取运行上下文。
func GetAgentRunContext(runID string) *AgentRunContext {
	agentMu.RLock()
	defer agentMu.RUnlock()
	ctx, ok := runContextById[runID]
	if !ok {
		return nil
	}
	copied := *ctx
	return &copied
}

// ClearAgentRunContext 清除运行上下文。
func ClearAgentRunContext(runID string) {
	agentMu.Lock()
	defer agentMu.Unlock()
	delete(runContextById, runID)
	delete(seqByRun, runID)
}

// ResetAgentRunContextForTest 测试用：清空所有上下文。
func ResetAgentRunContextForTest() {
	agentMu.Lock()
	defer agentMu.Unlock()
	runContextById = make(map[string]*AgentRunContext)
	seqByRun = make(map[string]int64)
}

// EmitAgentEvent 发射事件到所有监听器。
// TS 对应: emitAgentEvent()
func EmitAgentEvent(runID string, stream AgentEventStream, data map[string]interface{}, sessionKey string) {
	agentMu.Lock()
	seq := seqByRun[runID] + 1
	seqByRun[runID] = seq

	// 从注册上下文补全 sessionKey
	if sessionKey == "" {
		if ctx, ok := runContextById[runID]; ok {
			sessionKey = ctx.SessionKey
		}
	}
	agentMu.Unlock()

	evt := AgentEventPayload{
		RunID:      runID,
		Seq:        seq,
		Stream:     stream,
		Ts:         time.Now().UnixMilli(),
		Data:       data,
		SessionKey: sessionKey,
	}

	listenerMu.RLock()
	defer listenerMu.RUnlock()
	for _, listener := range eventListeners {
		func() {
			defer func() { recover() }() // ignore panics (TS: catch {})
			listener(evt)
		}()
	}
}

// OnAgentEvent 注册事件监听器。返回取消函数。
// TS 对应: onAgentEvent()
func OnAgentEvent(listener AgentEventListener) func() {
	listenerMu.Lock()
	idx := len(eventListeners)
	eventListeners = append(eventListeners, listener)
	listenerMu.Unlock()
	return func() {
		listenerMu.Lock()
		defer listenerMu.Unlock()
		if idx < len(eventListeners) {
			eventListeners = append(eventListeners[:idx], eventListeners[idx+1:]...)
		}
	}
}
