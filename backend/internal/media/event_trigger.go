package media

// ============================================================================
// media/event_trigger.go — 平台事件触发器
//
// 提供统一的事件驱动唤醒接口，支持两种模式：
// 1. CronPollTrigger — 基于 cron 轮询（无 webhook 的平台，如小红书）
// 2. WebhookBridgeTrigger — 桥接现有 webhook 通道（如微信公众号 OnMessage）
//
// MediaEventManager 聚合管理所有触发器的生命周期。
// ============================================================================

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/cron"
)

// ---------- 接口 ----------

// MediaEventTrigger 平台事件触发器接口。
type MediaEventTrigger interface {
	// Name 返回触发器标识名。
	Name() string
	// Start 启动触发器。
	Start(ctx context.Context) error
	// Stop 停止触发器。
	Stop()
}

// ---------- CronPollTrigger ----------

// CronPollTrigger 基于 cron 轮询的事件触发器。
// 用于小红书等无 webhook 的平台，通过 cron 定时轮询检测新事件。
type CronPollTrigger struct {
	name    string
	cronSvc CronServiceAdder
	jobName string
	message string
	everyMs int64

	mu    sync.Mutex
	jobID string // 已注册的 cron job ID
}

// CronPollTriggerConfig 轮询触发器配置。
type CronPollTriggerConfig struct {
	Name    string           // 触发器名称
	CronSvc CronServiceAdder // cron 服务
	JobName string           // cron job 名称
	Message string           // agent 巡检消息
	EveryMs int64            // 轮询间隔 (ms)
}

// NewCronPollTrigger 创建 cron 轮询触发器。
func NewCronPollTrigger(cfg CronPollTriggerConfig) *CronPollTrigger {
	return &CronPollTrigger{
		name:    cfg.Name,
		cronSvc: cfg.CronSvc,
		jobName: cfg.JobName,
		message: cfg.Message,
		everyMs: cfg.EveryMs,
	}
}

func (t *CronPollTrigger) Name() string { return t.name }

func (t *CronPollTrigger) Start(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.jobID != "" {
		return nil // 已启动
	}

	enabled := true
	result, err := t.cronSvc.Add(cron.CronJobCreate{
		Name:    t.jobName,
		Enabled: &enabled,
		Schedule: cron.CronSchedule{
			Kind:    cron.ScheduleKindEvery,
			EveryMs: t.everyMs,
		},
		SessionTarget: cron.SessionTargetIsolated,
		WakeMode:      cron.WakeModeNow,
		Payload: cron.CronPayload{
			Kind:    cron.PayloadKindAgentTurn,
			Message: t.message,
		},
	})
	if err != nil {
		return fmt.Errorf("cron poll trigger %q: %w", t.name, err)
	}
	t.jobID = result.JobID
	slog.Info("cron poll trigger started", "trigger", t.name, "jobID", t.jobID)
	return nil
}

func (t *CronPollTrigger) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.jobID != "" {
		slog.Info("cron poll trigger stopped", "trigger", t.name, "jobID", t.jobID)
		t.jobID = ""
	}
}

// ---------- WebhookBridgeTrigger ----------

// WebhookBridgeCallback 是 webhook 事件到来时的回调函数。
// 由现有 handler（如微信 OnMessage）调用，触发媒体 agent 处理。
type WebhookBridgeCallback func(eventType string, payload map[string]any)

// WebhookBridgeTrigger 桥接现有 webhook 通道的触发器。
// 不创建新监听器，只提供回调函数供现有 handler 调用。
type WebhookBridgeTrigger struct {
	name     string
	callback WebhookBridgeCallback

	mu      sync.Mutex
	started bool
}

// NewWebhookBridgeTrigger 创建 webhook 桥接触发器。
func NewWebhookBridgeTrigger(name string, callback WebhookBridgeCallback) *WebhookBridgeTrigger {
	return &WebhookBridgeTrigger{
		name:     name,
		callback: callback,
	}
}

func (t *WebhookBridgeTrigger) Name() string { return t.name }

func (t *WebhookBridgeTrigger) Start(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = true
	slog.Info("webhook bridge trigger started", "trigger", t.name)
	return nil
}

func (t *WebhookBridgeTrigger) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.started = false
	slog.Info("webhook bridge trigger stopped", "trigger", t.name)
}

// OnEvent 当 webhook 事件到来时调用。仅在 started 状态下执行回调。
func (t *WebhookBridgeTrigger) OnEvent(eventType string, payload map[string]any) {
	t.mu.Lock()
	started := t.started
	t.mu.Unlock()
	if !started || t.callback == nil {
		return
	}
	t.callback(eventType, payload)
}

// ---------- MediaEventManager ----------

// MediaEventManager 统一管理所有事件触发器。
type MediaEventManager struct {
	mu       sync.RWMutex
	triggers []MediaEventTrigger
}

// NewMediaEventManager 创建事件管理器。
func NewMediaEventManager() *MediaEventManager {
	return &MediaEventManager{}
}

// Register 注册触发器。
func (m *MediaEventManager) Register(trigger MediaEventTrigger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triggers = append(m.triggers, trigger)
}

// StartAll 启动所有已注册的触发器。
func (m *MediaEventManager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, t := range m.triggers {
		if err := t.Start(ctx); err != nil {
			return fmt.Errorf("start trigger %q: %w", t.Name(), err)
		}
	}
	return nil
}

// StopAll 停止所有触发器。
func (m *MediaEventManager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, t := range m.triggers {
		t.Stop()
	}
}

// Status 返回所有触发器的状态摘要。
func (m *MediaEventManager) Status() []TriggerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]TriggerStatus, 0, len(m.triggers))
	for _, t := range m.triggers {
		out = append(out, TriggerStatus{Name: t.Name()})
	}
	return out
}

// TriggerStatus 触发器状态。
type TriggerStatus struct {
	Name string `json:"name"`
}
