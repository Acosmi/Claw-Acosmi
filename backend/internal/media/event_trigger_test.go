package media

import (
	"context"
	"testing"
)

func TestCronPollTrigger_StartStop(t *testing.T) {
	adder := &mockCronAdder{}
	trigger := NewCronPollTrigger(CronPollTriggerConfig{
		Name:    "xhs-poll",
		CronSvc: adder,
		JobName: "media.poll.xhs",
		Message: "poll xhs events",
		EveryMs: 60_000,
	})

	if trigger.Name() != "xhs-poll" {
		t.Errorf("Name() = %q, want %q", trigger.Name(), "xhs-poll")
	}

	// Start 应注册一个 cron job
	err := trigger.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if len(adder.jobs) != 1 {
		t.Fatalf("expected 1 job registered, got %d", len(adder.jobs))
	}
	if adder.jobs[0].Name != "media.poll.xhs" {
		t.Errorf("job name = %q, want %q", adder.jobs[0].Name, "media.poll.xhs")
	}

	// 重复 Start 不应注册新 job
	err = trigger.Start(context.Background())
	if err != nil {
		t.Fatalf("double Start failed: %v", err)
	}
	if len(adder.jobs) != 1 {
		t.Errorf("double Start registered extra job: got %d", len(adder.jobs))
	}

	// Stop
	trigger.Stop()
	// Stop 后再 Start 应重新注册
	err = trigger.Start(context.Background())
	if err != nil {
		t.Fatalf("Start after Stop failed: %v", err)
	}
	if len(adder.jobs) != 2 {
		t.Errorf("expected 2 jobs after restart, got %d", len(adder.jobs))
	}
}

func TestMediaEventManager_StartAll(t *testing.T) {
	adder := &mockCronAdder{}

	mgr := NewMediaEventManager()
	mgr.Register(NewCronPollTrigger(CronPollTriggerConfig{
		Name:    "trigger-a",
		CronSvc: adder,
		JobName: "media.poll.a",
		Message: "poll a",
		EveryMs: 30_000,
	}))
	mgr.Register(NewCronPollTrigger(CronPollTriggerConfig{
		Name:    "trigger-b",
		CronSvc: adder,
		JobName: "media.poll.b",
		Message: "poll b",
		EveryMs: 60_000,
	}))

	err := mgr.StartAll(context.Background())
	if err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	if len(adder.jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(adder.jobs))
	}

	mgr.StopAll()
}

func TestMediaEventManager_Status(t *testing.T) {
	mgr := NewMediaEventManager()
	mgr.Register(NewCronPollTrigger(CronPollTriggerConfig{
		Name:    "t1",
		CronSvc: &mockCronAdder{},
		JobName: "j1",
		Message: "m1",
		EveryMs: 10_000,
	}))

	received := false
	mgr.Register(NewWebhookBridgeTrigger("t2", func(eventType string, payload map[string]any) {
		received = true
	}))

	statuses := mgr.Status()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Name != "t1" {
		t.Errorf("status[0].Name = %q, want t1", statuses[0].Name)
	}
	if statuses[1].Name != "t2" {
		t.Errorf("status[1].Name = %q, want t2", statuses[1].Name)
	}

	// WebhookBridgeTrigger 回调测试 — 未启动时不触发
	wt := mgr.triggers[1].(*WebhookBridgeTrigger)
	wt.OnEvent("test", nil)
	if received {
		t.Error("callback should not fire before Start()")
	}

	// 启动后回调应触发
	if err := wt.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	wt.OnEvent("test", nil)
	if !received {
		t.Error("callback should fire after Start()")
	}
}
