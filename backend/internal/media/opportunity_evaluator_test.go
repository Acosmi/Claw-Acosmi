package media

import (
	"testing"
	"time"
)

func TestEvaluateOpportunities_HighHeat(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	topics := []TrendingTopic{
		{
			Title:     "AI大模型突破",
			Source:    "weibo",
			HeatScore: 60000,
			FetchedAt: time.Now().Add(-30 * time.Minute), // 30min 前，时效满分
		},
	}

	results := EvaluateOpportunities(topics, store, DefaultOpportunityConfig())
	if len(results) != 1 {
		t.Fatalf("results: got %d, want 1", len(results))
	}

	r := results[0]
	// 热度 40 + 新鲜度 30 + 时效 10 = 80 (无多源)
	if r.Score < 60 {
		t.Errorf("score: got %.1f, want >= 60", r.Score)
	}
	if r.Action != ActionCreate {
		t.Errorf("action: got %q, want %q", r.Action, ActionCreate)
	}
	if len(r.Reasons) == 0 {
		t.Error("reasons should not be empty")
	}
}

func TestEvaluateOpportunities_AlreadyProcessed(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 标记话题为已处理
	if err := store.MarkTopicProcessed("旧话题复热"); err != nil {
		t.Fatalf("MarkTopicProcessed: %v", err)
	}

	topics := []TrendingTopic{
		{
			Title:     "旧话题复热",
			Source:    "baidu",
			HeatScore: 8000,                           // 低于默认阈值 10000
			FetchedAt: time.Now().Add(-8 * time.Hour), // 超过 6h
		},
	}

	results := EvaluateOpportunities(topics, store, DefaultOpportunityConfig())
	if len(results) != 1 {
		t.Fatalf("results: got %d, want 1", len(results))
	}

	r := results[0]
	// 热度 0 + 已处理 0 + 无多源 0 + 时效 0 = 0
	if r.Score >= 30 {
		t.Errorf("score: got %.1f, want < 30 (skip)", r.Score)
	}
	if r.Action != ActionSkip {
		t.Errorf("action: got %q, want %q", r.Action, ActionSkip)
	}
}

func TestEvaluateOpportunities_MaxTopics(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	now := time.Now()
	topics := make([]TrendingTopic, 5)
	for i := range topics {
		topics[i] = TrendingTopic{
			Title:     "热点话题" + itoa(i),
			Source:    "weibo",
			HeatScore: 60000,
			FetchedAt: now.Add(-time.Duration(i) * 30 * time.Minute),
		}
	}

	cfg := DefaultOpportunityConfig()
	cfg.MaxTopicsPerRun = 2

	results := EvaluateOpportunities(topics, store, cfg)
	if len(results) != 5 {
		t.Fatalf("results: got %d, want 5", len(results))
	}

	createCount := 0
	for _, r := range results {
		if r.Action == ActionCreate {
			createCount++
		}
	}
	if createCount != 2 {
		t.Errorf("create count: got %d, want 2 (max capped)", createCount)
	}
}

func TestEvaluateOpportunities_Empty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	results := EvaluateOpportunities(nil, store, DefaultOpportunityConfig())
	if results != nil {
		t.Errorf("results: got %v, want nil", results)
	}

	results = EvaluateOpportunities([]TrendingTopic{}, store, DefaultOpportunityConfig())
	if results != nil {
		t.Errorf("results for empty slice: got %v, want nil", results)
	}
}

func TestEvaluateOpportunities_MultiSource(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	now := time.Now()
	topics := []TrendingTopic{
		{
			Title:     "ChatGPT重大更新",
			Source:    "weibo",
			HeatScore: 30000,
			FetchedAt: now.Add(-1 * time.Hour),
		},
		{
			Title:     "chatgpt重大更新", // 同一话题不同大小写
			Source:    "zhihu",
			HeatScore: 25000,
			FetchedAt: now.Add(-2 * time.Hour),
		},
		{
			Title:     "独立话题",
			Source:    "baidu",
			HeatScore: 30000,
			FetchedAt: now.Add(-1 * time.Hour),
		},
	}

	results := EvaluateOpportunities(topics, store, DefaultOpportunityConfig())
	if len(results) != 3 {
		t.Fatalf("results: got %d, want 3", len(results))
	}

	// 找到多源话题（weibo 的 ChatGPT）和独立话题
	var multiSourceScore, singleSourceScore float64
	for _, r := range results {
		if r.Topic.Title == "ChatGPT重大更新" {
			multiSourceScore = r.Score
		}
		if r.Topic.Title == "独立话题" {
			singleSourceScore = r.Score
		}
	}

	// 多源话题应该比同等热度的单源话题分数高 20 分
	if multiSourceScore <= singleSourceScore {
		t.Errorf("multi-source (%.1f) should score higher than single-source (%.1f)",
			multiSourceScore, singleSourceScore)
	}
}

func TestDefaultOpportunityConfig(t *testing.T) {
	cfg := DefaultOpportunityConfig()
	if cfg.HeatThreshold != 10000 {
		t.Errorf("HeatThreshold: got %.0f, want 10000", cfg.HeatThreshold)
	}
	if cfg.MaxTopicsPerRun != 3 {
		t.Errorf("MaxTopicsPerRun: got %d, want 3", cfg.MaxTopicsPerRun)
	}
	if cfg.CooldownHours != 24 {
		t.Errorf("CooldownHours: got %d, want 24", cfg.CooldownHours)
	}
}

func TestNormalizeHeat(t *testing.T) {
	cases := []struct {
		heat, threshold, want float64
	}{
		{5000, 10000, 0},    // 低于阈值
		{10000, 10000, 0},   // 等于阈值
		{30000, 10000, 20},  // 中间值
		{50000, 10000, 40},  // 满分
		{100000, 10000, 40}, // 超过上限仍为满分
	}
	for _, tc := range cases {
		got := normalizeHeat(tc.heat, tc.threshold)
		if got != tc.want {
			t.Errorf("normalizeHeat(%.0f, %.0f): got %.1f, want %.1f", tc.heat, tc.threshold, got, tc.want)
		}
	}
}

func TestCalcFreshness(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		fetchedAt time.Time
		wantMin   float64
		wantMax   float64
	}{
		{"just now", now, 10, 10},
		{"1h ago", now.Add(-1 * time.Hour), 10, 10},
		{"2h ago", now.Add(-2 * time.Hour), 9, 10},
		{"4h ago", now.Add(-4 * time.Hour), 4, 6},
		{"6h ago", now.Add(-6 * time.Hour), 0, 0.1},
		{"12h ago", now.Add(-12 * time.Hour), 0, 0},
		{"zero time", time.Time{}, 0, 0},
	}
	for _, tc := range cases {
		got := calcFreshness(tc.fetchedAt, now)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("%s: got %.1f, want [%.1f, %.1f]", tc.name, got, tc.wantMin, tc.wantMax)
		}
	}
}

func TestMapAction(t *testing.T) {
	cases := []struct {
		score float64
		want  OpportunityAction
	}{
		{80, ActionCreate},
		{60, ActionCreate},
		{59.9, ActionWatch},
		{30, ActionWatch},
		{29.9, ActionSkip},
		{0, ActionSkip},
	}
	for _, tc := range cases {
		got := mapAction(tc.score)
		if got != tc.want {
			t.Errorf("mapAction(%.1f): got %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"ChatGPT重大更新", "chatgpt重大更新"},
		{"  ChatGPT 重大更新 ", "chatgpt重大更新"},
		{"#ChatGPT重大更新#", "chatgpt重大更新"},
		{"【热搜】ChatGPT更新", "热搜chatgpt更新"},
	}
	for _, tc := range cases {
		got := normalizeTitle(tc.input)
		if got != tc.want {
			t.Errorf("normalizeTitle(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}
