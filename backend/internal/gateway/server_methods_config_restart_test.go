package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

type captureRestartPlanRestarter struct {
	plan GatewayRestartPlan
}

func (c *captureRestartPlanRestarter) ScheduleRestart(plan GatewayRestartPlan) *GatewayRestartResult {
	c.plan = plan
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

func TestConfigApply_SchedulesTransactionalRestart(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	oldRaw := "{}\n"
	if err := os.WriteFile(cfgPath, []byte(oldRaw), 0o600); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	loader := config.NewConfigLoader(config.WithConfigPath(cfgPath))
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	registry := NewMethodRegistry()
	registry.RegisterAll(ConfigHandlers())

	restarter := &captureRestartPlanRestarter{}
	req := &RequestFrame{
		Method: "config.apply",
		Params: map[string]interface{}{
			"raw":      `{"gateway":{"port":26223}}`,
			"baseHash": snapshot.Hash,
		},
	}

	var gotOK bool
	var gotErr *ErrorShape
	var gotPayload interface{}
	HandleGatewayRequest(registry, req, nil, &GatewayMethodContext{
		ConfigLoader:      loader,
		GatewayRestarter:  restarter,
		ChannelMonitorMgr: nil,
	}, func(ok bool, payload interface{}, err *ErrorShape) {
		gotOK = ok
		gotPayload = payload
		gotErr = err
	})

	if !gotOK {
		t.Fatalf("config.apply failed: %+v", gotErr)
	}
	if restarter.plan.Rollback == nil {
		t.Fatal("config.apply did not attach rollback snapshot to restart plan")
	}
	if restarter.plan.Reason != "config.apply" {
		t.Fatalf("unexpected restart reason: %q", restarter.plan.Reason)
	}
	if string(restarter.plan.Rollback.PreviousRaw) != oldRaw {
		t.Fatalf("unexpected rollback snapshot: %q", string(restarter.plan.Rollback.PreviousRaw))
	}
	if restarter.plan.Rollback.ConfigPath != cfgPath {
		t.Fatalf("unexpected rollback config path: %q", restarter.plan.Rollback.ConfigPath)
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected payload map, got %T", gotPayload)
	}
	verification, ok := result["verification"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected verification map, got %T", result["verification"])
	}
	if verification["runtimeEffect"] != "restart_scheduled" {
		t.Fatalf("runtimeEffect=%v, want restart_scheduled", verification["runtimeEffect"])
	}
	if verification["restartScheduled"] != true {
		t.Fatalf("restartScheduled=%v, want true", verification["restartScheduled"])
	}
	if _, ok := result["hash"].(string); !ok {
		t.Fatalf("expected result hash, got %T", result["hash"])
	}
}
