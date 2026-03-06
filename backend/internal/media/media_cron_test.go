package media

import (
	"fmt"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/cron"
)

// ---------- Mock ----------

type mockCronAdder struct {
	jobs []cron.CronJobCreate
	seq  int
	fail bool // 模拟 Add 失败
}

func (m *mockCronAdder) Add(input cron.CronJobCreate) (*cron.CronAddResult, error) {
	if m.fail {
		return nil, fmt.Errorf("mock add error")
	}
	m.seq++
	m.jobs = append(m.jobs, input)
	return &cron.CronAddResult{
		CronOpResult: cron.CronOpResult{OK: true},
		JobID:        fmt.Sprintf("mock-job-%d", m.seq),
	}, nil
}

// ---------- Tests ----------

func TestRegisterMediaCronJobs_Default(t *testing.T) {
	adder := &mockCronAdder{}
	cfg := DefaultMediaCronConfig()

	refs, err := RegisterMediaCronJobs(adder, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 job refs, got %d", len(refs))
	}
	if len(adder.jobs) != 3 {
		t.Fatalf("expected 3 jobs registered, got %d", len(adder.jobs))
	}

	// 验证返回的 ref 包含正确的 JobName
	refNames := map[string]bool{}
	for _, ref := range refs {
		refNames[ref.JobName] = true
		if ref.JobID == "" {
			t.Errorf("ref for %q has empty JobID", ref.JobName)
		}
	}
	for _, want := range []string{
		"media.patrol.trending",
		"media.patrol.publish",
		"media.patrol.interact",
	} {
		if !refNames[want] {
			t.Errorf("missing job ref %q", want)
		}
	}

	// 验证公共字段
	for _, j := range adder.jobs {
		if j.SessionTarget != cron.SessionTargetIsolated {
			t.Errorf("job %q: sessionTarget=%q, want isolated", j.Name, j.SessionTarget)
		}
		if j.WakeMode != cron.WakeModeNow {
			t.Errorf("job %q: wakeMode=%q, want now", j.Name, j.WakeMode)
		}
		if j.Payload.Kind != cron.PayloadKindAgentTurn {
			t.Errorf("job %q: payload.kind=%q, want agentTurn", j.Name, j.Payload.Kind)
		}
		if j.Schedule.Kind != cron.ScheduleKindEvery {
			t.Errorf("job %q: schedule.kind=%q, want every", j.Name, j.Schedule.Kind)
		}
		if j.Enabled == nil || !*j.Enabled {
			t.Errorf("job %q: expected enabled=true", j.Name)
		}
	}
}

func TestRegisterMediaCronJobs_Disabled(t *testing.T) {
	adder := &mockCronAdder{}
	cfg := DefaultMediaCronConfig()
	cfg.Enabled = false

	refs, err := RegisterMediaCronJobs(adder, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 job refs when disabled, got %d", len(refs))
	}
	if len(adder.jobs) != 0 {
		t.Errorf("expected 0 jobs registered when disabled, got %d", len(adder.jobs))
	}
}

func TestRegisterMediaCronJobs_CustomIntervals(t *testing.T) {
	adder := &mockCronAdder{}
	cfg := MediaCronConfig{
		TrendingPatrolIntervalMs: 60_000,  // 1 min
		PublishCheckIntervalMs:   120_000, // 2 min
		InteractPatrolIntervalMs: 30_000,  // 30 sec
		Enabled:                  true,
	}

	refs, err := RegisterMediaCronJobs(adder, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 job refs, got %d", len(refs))
	}

	// 验证间隔
	expectedIntervals := map[string]int64{
		"media.patrol.trending": 60_000,
		"media.patrol.publish":  120_000,
		"media.patrol.interact": 30_000,
	}
	for _, j := range adder.jobs {
		want, ok := expectedIntervals[j.Name]
		if !ok {
			t.Errorf("unexpected job %q", j.Name)
			continue
		}
		if j.Schedule.EveryMs != want {
			t.Errorf("job %q: everyMs=%d, want %d", j.Name, j.Schedule.EveryMs, want)
		}
	}
}

func TestDefaultMediaCronConfig(t *testing.T) {
	cfg := DefaultMediaCronConfig()
	if !cfg.Enabled {
		t.Error("default config should be enabled")
	}
	if cfg.TrendingPatrolIntervalMs != 14_400_000 {
		t.Errorf("trending interval: got %d, want 14400000", cfg.TrendingPatrolIntervalMs)
	}
	if cfg.PublishCheckIntervalMs != 7_200_000 {
		t.Errorf("publish interval: got %d, want 7200000", cfg.PublishCheckIntervalMs)
	}
	if cfg.InteractPatrolIntervalMs != 3_600_000 {
		t.Errorf("interact interval: got %d, want 3600000", cfg.InteractPatrolIntervalMs)
	}
}
