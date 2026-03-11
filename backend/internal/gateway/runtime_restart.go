package gateway

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

const (
	defaultRuntimeRestartDelay = 150
	maxRuntimeRestartAttempts  = 5
	runtimeRestartBackoff      = 200 * time.Millisecond
)

type runtimeRestartTarget interface {
	Close(reason string) error
}

type gatewayRuntimeStartFunc func(
	port int,
	opts GatewayServerOptions,
	controller *gatewayRuntimeController,
) (runtimeRestartTarget, error)

type gatewayRuntimeController struct {
	mu       sync.Mutex
	port     int
	opts     GatewayServerOptions
	stateDir string
	current  runtimeRestartTarget
	handle   *GatewayRuntime
	startFn  gatewayRuntimeStartFunc

	restarting bool
	closed     bool
	pending    GatewayRestartPlan
}

func newGatewayRuntimeController(port int, opts GatewayServerOptions) *gatewayRuntimeController {
	if port <= 0 {
		port = config.DefaultGatewayPort
		slog.Warn("gateway: runtime controller received port=0, falling back to default",
			"defaultPort", port)
	}
	controller := &gatewayRuntimeController{
		port:     port,
		opts:     opts,
		stateDir: config.ResolveStateDir(),
	}
	controller.startFn = defaultGatewayRuntimeStarter
	return controller
}

func defaultGatewayRuntimeStarter(
	port int,
	opts GatewayServerOptions,
	controller *gatewayRuntimeController,
) (runtimeRestartTarget, error) {
	return startGatewayServerInstance(port, opts, controller)
}

func (c *gatewayRuntimeController) bind(target runtimeRestartTarget) {
	c.mu.Lock()
	c.current = target
	handle := c.handle
	c.mu.Unlock()

	if handle != nil {
		if runtime, ok := target.(*GatewayRuntime); ok {
			handle.adopt(runtime)
		}
	}
}

func (c *gatewayRuntimeController) attachHandle(handle *GatewayRuntime) {
	c.mu.Lock()
	c.handle = handle
	current := c.current
	c.mu.Unlock()

	if handle != nil {
		if runtime, ok := current.(*GatewayRuntime); ok {
			handle.adopt(runtime)
		}
	}
}

func (c *gatewayRuntimeController) close(reason string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	current := c.current
	c.current = nil
	c.mu.Unlock()

	if current == nil {
		return nil
	}
	return current.Close(reason)
}

func (c *gatewayRuntimeController) shouldAbortRestart() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		c.restarting = false
		return true
	}
	return false
}

func (c *gatewayRuntimeController) sentinelWriter() RestartSentinelWriter {
	if c == nil {
		return nil
	}
	return &gatewayRestartSentinelWriter{stateDir: c.stateDir}
}

func (c *gatewayRuntimeController) ScheduleRestart(plan GatewayRestartPlan) *GatewayRestartResult {
	if c == nil {
		return &GatewayRestartResult{Scheduled: false, Reason: plan.Reason}
	}

	delay := defaultRuntimeRestartDelay
	if plan.DelayMs != nil && *plan.DelayMs >= 0 {
		delay = *plan.DelayMs
	}
	normalizedPlan := GatewayRestartPlan{
		DelayMs:  &delay,
		Reason:   plan.Reason,
		Rollback: cloneGatewayConfigRollback(plan.Rollback),
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return &GatewayRestartResult{Scheduled: false, Reason: plan.Reason}
	}
	if c.restarting {
		if c.pending.Rollback == nil && normalizedPlan.Rollback != nil {
			c.pending.Rollback = normalizedPlan.Rollback
		}
		transactional := c.pending.Rollback != nil
		c.mu.Unlock()
		slog.Info("gateway: restart already in progress, coalescing request", "reason", plan.Reason)
		return &GatewayRestartResult{
			Scheduled:     true,
			DelayMs:       delay,
			Reason:        plan.Reason,
			Transactional: transactional,
		}
	}
	c.restarting = true
	c.pending = normalizedPlan
	c.mu.Unlock()

	go c.performRestart()

	return &GatewayRestartResult{
		Scheduled:     true,
		DelayMs:       delay,
		Reason:        plan.Reason,
		Transactional: normalizedPlan.Rollback != nil,
	}
}

func (c *gatewayRuntimeController) performRestart() {
	c.mu.Lock()
	plan := c.pending
	c.pending = GatewayRestartPlan{}
	c.mu.Unlock()

	delayMs := defaultRuntimeRestartDelay
	if plan.DelayMs != nil && *plan.DelayMs >= 0 {
		delayMs = *plan.DelayMs
	}
	reason := plan.Reason

	if delayMs > 0 {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}
	if c.shouldAbortRestart() {
		return
	}

	c.mu.Lock()
	current := c.current
	c.mu.Unlock()

	if current != nil {
		if err := current.Close("gateway runtime restart: " + reason); err != nil {
			slog.Warn("gateway: runtime close failed during restart", "reason", reason, "error", err)
		}
	}

	var (
		target runtimeRestartTarget
		err    error
	)
	for attempt := 1; attempt <= maxRuntimeRestartAttempts; attempt++ {
		if c.shouldAbortRestart() {
			return
		}
		target, err = c.startFn(c.port, c.opts, c)
		if err == nil {
			break
		}
		slog.Warn("gateway: restart attempt failed",
			"attempt", attempt,
			"maxAttempts", maxRuntimeRestartAttempts,
			"reason", reason,
			"error", err,
		)
		if attempt < maxRuntimeRestartAttempts {
			time.Sleep(runtimeRestartBackoff)
		}
	}

	if err != nil {
		if recoveredTarget, recoveryErr := c.recoverFromRollback(plan, reason, err); recoveryErr == nil {
			c.bind(recoveredTarget)
			if sentinel := infra.ConsumeRestartSentinel(c.stateDir); sentinel != nil {
				slog.Info("gateway: restart sentinel consumed after rollback recovery",
					"summary", infra.SummarizeRestartSentinel(sentinel.Payload))
			}
			slog.Warn("gateway: restart recovered by rolling back config", "reason", reason, "port", c.port)
			c.mu.Lock()
			c.restarting = false
			c.mu.Unlock()
			return
		}
		c.mu.Lock()
		c.restarting = false
		c.mu.Unlock()
		slog.Error("gateway: restart failed", "reason", reason, "error", err)
		return
	}

	c.mu.Lock()
	if c.closed {
		c.restarting = false
		c.mu.Unlock()
		if target != nil {
			_ = target.Close("gateway runtime controller closed during restart")
		}
		return
	}
	c.current = target
	handle := c.handle
	c.restarting = false
	c.mu.Unlock()

	if handle != nil {
		if runtime, ok := target.(*GatewayRuntime); ok {
			handle.adopt(runtime)
		}
	}
	if sentinel := infra.ConsumeRestartSentinel(c.stateDir); sentinel != nil {
		slog.Info("gateway: restart sentinel consumed after in-process restart",
			"summary", infra.SummarizeRestartSentinel(sentinel.Payload))
	}
	slog.Info("gateway: in-process restart complete", "reason", reason, "port", c.port)
}

func (c *gatewayRuntimeController) recoverFromRollback(
	plan GatewayRestartPlan,
	reason string,
	restartErr error,
) (runtimeRestartTarget, error) {
	rollback := cloneGatewayConfigRollback(plan.Rollback)
	if rollback == nil {
		return nil, restartErr
	}
	if err := restoreGatewayConfigRollback(rollback); err != nil {
		return nil, fmt.Errorf("restart failed: %w; rollback restore failed: %w", restartErr, err)
	}

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("restart failed: %w; config restored during shutdown", restartErr)
	}

	var (
		target runtimeRestartTarget
		err    error
	)
	for attempt := 1; attempt <= maxRuntimeRestartAttempts; attempt++ {
		target, err = c.startFn(c.port, c.opts, c)
		if err == nil {
			return target, nil
		}
		slog.Warn("gateway: rollback recovery attempt failed",
			"attempt", attempt,
			"maxAttempts", maxRuntimeRestartAttempts,
			"reason", reason,
			"error", err,
		)
		if attempt < maxRuntimeRestartAttempts {
			time.Sleep(runtimeRestartBackoff)
		}
	}
	return nil, fmt.Errorf("restart failed: %w; rollback recovery failed: %w", restartErr, err)
}

type gatewayRestartSentinelWriter struct {
	stateDir string
}

func (w *gatewayRestartSentinelWriter) WriteRestartSentinel(payload *RestartSentinelPayload) (string, error) {
	if w == nil || payload == nil {
		return "", nil
	}

	message := ""
	if payload.Message != nil {
		message = *payload.Message
	}

	infraPayload := infra.RestartSentinelPayload{
		Kind:       infra.RestartSentinelKind(payload.Kind),
		Status:     infra.RestartSentinelStatus(payload.Status),
		Ts:         payload.Ts,
		SessionKey: payload.SessionKey,
		Message:    message,
		DoctorHint: payload.DoctorHint,
		Stats:      mapGatewayRestartStats(payload.Stats),
	}
	return infra.WriteRestartSentinel(w.stateDir, infraPayload)
}

func (w *gatewayRestartSentinelWriter) FormatDoctorNonInteractiveHint() string {
	if w == nil || w.stateDir == "" {
		return "Run `crabclaw doctor` to inspect runtime state after restart."
	}
	return fmt.Sprintf(
		"Run `crabclaw doctor` and inspect `%s` if the runtime does not recover automatically.",
		w.stateDir,
	)
}

func mapGatewayRestartStats(stats map[string]interface{}) *infra.RestartSentinelStats {
	if len(stats) == 0 {
		return nil
	}

	mapped := &infra.RestartSentinelStats{}
	if mode, ok := stats["mode"].(string); ok {
		mapped.Mode = mode
	}
	if root, ok := stats["root"].(string); ok {
		mapped.Root = root
	}
	if reason, ok := stats["reason"].(string); ok {
		mapped.Reason = reason
	}
	if mapped.Mode == "" && mapped.Root == "" && mapped.Reason == "" {
		return nil
	}
	return mapped
}
