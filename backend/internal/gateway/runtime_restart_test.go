package gateway

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGatewayRuntimeController_RestartPreservesStableHandle(t *testing.T) {
	controller := newGatewayRuntimeController(0, GatewayServerOptions{})

	original := &GatewayRuntime{State: NewGatewayState()}
	controller.bind(original)

	handle := &GatewayRuntime{controller: controller}
	controller.attachHandle(handle)
	if handle.State != original.State {
		t.Fatal("stable handle did not adopt initial runtime state")
	}

	restarted := make(chan *GatewayRuntime, 1)
	controller.startFn = func(int, GatewayServerOptions, *gatewayRuntimeController) (runtimeRestartTarget, error) {
		next := &GatewayRuntime{State: NewGatewayState()}
		restarted <- next
		return next, nil
	}

	delay := 0
	result := controller.ScheduleRestart(GatewayRestartPlan{DelayMs: &delay, Reason: "test restart"})
	if result == nil || !result.Scheduled {
		t.Fatal("expected restart to be scheduled")
	}

	var next *GatewayRuntime
	select {
	case next = <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restart")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if handle.State == next.State {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if handle.State != next.State {
		t.Fatal("stable handle did not switch to restarted runtime")
	}

	original.mu.Lock()
	originalClosed := original.closed
	original.mu.Unlock()
	if !originalClosed {
		t.Fatal("original runtime was not closed during restart")
	}

	if err := handle.Close("test shutdown"); err != nil {
		t.Fatalf("stable handle close failed: %v", err)
	}

	next.mu.Lock()
	nextClosed := next.closed
	next.mu.Unlock()
	if !nextClosed {
		t.Fatal("stable handle did not close restarted runtime")
	}
}

func TestGatewayRuntimeController_ScheduleRestartReturnsFalseAfterClose(t *testing.T) {
	controller := newGatewayRuntimeController(0, GatewayServerOptions{})
	controller.bind(&GatewayRuntime{State: NewGatewayState()})

	handle := &GatewayRuntime{controller: controller}
	controller.attachHandle(handle)

	if err := handle.Close("test shutdown"); err != nil {
		t.Fatalf("stable handle close failed: %v", err)
	}

	delay := 0
	result := controller.ScheduleRestart(GatewayRestartPlan{DelayMs: &delay, Reason: "after close"})
	if result == nil {
		t.Fatal("expected restart result")
	}
	if result.Scheduled {
		t.Fatal("restart should not be scheduled after controller close")
	}
}

type stubRestartSentinelWriter struct{}

func (stubRestartSentinelWriter) WriteRestartSentinel(*RestartSentinelPayload) (string, error) {
	return "", nil
}

func (stubRestartSentinelWriter) FormatDoctorNonInteractiveHint() string {
	return "doctor"
}

type stubGatewayRestarter struct {
	mu      sync.Mutex
	reasons []string
}

func (s *stubGatewayRestarter) ScheduleRestart(plan GatewayRestartPlan) *GatewayRestartResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reasons = append(s.reasons, plan.Reason)
	delay := 0
	if plan.DelayMs != nil {
		delay = *plan.DelayMs
	}
	return &GatewayRestartResult{
		Scheduled:     true,
		DelayMs:       delay,
		Reason:        plan.Reason,
		Transactional: plan.Rollback != nil,
	}
}

func TestGatewayRuntimeController_RollbackRestoresPreviousConfigOnRestartFailure(t *testing.T) {
	controller := newGatewayRuntimeController(0, GatewayServerOptions{})
	original := &GatewayRuntime{State: NewGatewayState()}
	controller.bind(original)

	handle := &GatewayRuntime{controller: controller}
	controller.attachHandle(handle)

	configPath := filepath.Join(t.TempDir(), "openacosmi.json")
	if err := os.WriteFile(configPath, []byte(`{"version":"old"}`), 0o600); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"version":"new"}`), 0o600); err != nil {
		t.Fatalf("write new config: %v", err)
	}

	recovered := make(chan *GatewayRuntime, 1)
	var attempts int
	controller.startFn = func(int, GatewayServerOptions, *gatewayRuntimeController) (runtimeRestartTarget, error) {
		attempts++
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		if string(raw) == `{"version":"old"}` {
			next := &GatewayRuntime{State: NewGatewayState()}
			recovered <- next
			return next, nil
		}
		return nil, os.ErrInvalid
	}

	delay := 0
	result := controller.ScheduleRestart(GatewayRestartPlan{
		DelayMs: &delay,
		Reason:  "restart with rollback",
		Rollback: &GatewayConfigRollback{
			ConfigPath:  configPath,
			PreviousRaw: []byte(`{"version":"old"}`),
		},
	})
	if result == nil || !result.Scheduled || !result.Transactional {
		t.Fatal("expected transactional restart to be scheduled")
	}

	var next *GatewayRuntime
	select {
	case next = <-recovered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for rollback recovery")
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if string(raw) != `{"version":"old"}` {
		t.Fatalf("expected config rollback to restore old file, got %q", string(raw))
	}
	if attempts <= 1 {
		t.Fatal("expected at least one failed restart attempt before rollback recovery")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if handle.State == next.State {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if handle.State != next.State {
		t.Fatal("stable handle did not adopt rollback recovery runtime")
	}
}
