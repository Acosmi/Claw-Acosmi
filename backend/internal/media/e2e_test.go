package media

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// e2e_test.go — Phase 5 端到端集成测试
//
// 验证完整链路：
// 1. 子系统初始化 → 工具注册
// 2. 热点采集 → 去重标记
// 3. 草稿创建 → 状态流转
// 4. 发布 → 历史记录
// 5. 机会评估 → 自动 Spawn 计数
// 6. Cron 心跳注册 → 事件触发
// ============================================================================

// ---------- Mock Publisher (e2e) ----------

type e2eMockPublisher struct {
	published []*ContentDraft
}

func (p *e2eMockPublisher) Publish(_ context.Context, draft *ContentDraft) (*PublishResult, error) {
	p.published = append(p.published, draft)
	return &PublishResult{
		Platform:    draft.Platform,
		PostID:      "mock-post-" + draft.ID,
		URL:         "https://example.com/" + draft.ID,
		Status:      "published",
		PublishedAt: time.Now().UTC(),
	}, nil
}

// ---------- Mock Trending Source (e2e) ----------

type e2eMockTrendingSource struct {
	name   string
	topics []TrendingTopic
}

func (s *e2eMockTrendingSource) Name() string { return s.name }
func (s *e2eMockTrendingSource) Fetch(_ context.Context, _ string, limit int) ([]TrendingTopic, error) {
	if limit > 0 && limit < len(s.topics) {
		return s.topics[:limit], nil
	}
	return s.topics, nil
}

// ---------- E2E Tests ----------

// TestE2E_FullPipeline 验证完整链路：trending → draft → publish → history
func TestE2E_FullPipeline(t *testing.T) {
	dir := t.TempDir()

	// 1. 初始化子系统（含发布）
	sub, err := NewMediaSubsystem(MediaSubsystemConfig{
		Workspace:      dir,
		EnablePublish:  true,
		EnableInteract: true,
	})
	if err != nil {
		t.Fatalf("NewMediaSubsystem: %v", err)
	}

	// 验证工具数量
	names := sub.ToolNames()
	if len(names) < 3 {
		t.Fatalf("expected at least 3 tools, got %d: %v", len(names), names)
	}

	// 2. 注册 mock publisher
	pub := &e2eMockPublisher{}
	sub.RegisterPublisher(PlatformWebsite, pub)
	if sub.Publishers[PlatformWebsite] == nil {
		t.Fatal("publisher not registered")
	}

	// 3. 添加 mock 热点源并验证聚合
	mockSrc := &e2eMockTrendingSource{
		name: "mock",
		topics: []TrendingTopic{
			{Title: "E2E测试热点", Source: "mock", HeatScore: 50000, FetchedAt: time.Now()},
		},
	}
	sub.Aggregator.AddSource(mockSrc)
	topics, results := sub.Aggregator.FetchAll(context.Background(), "", 10)
	if len(topics) == 0 {
		t.Fatal("expected at least 1 topic from mock source")
	}
	// 验证 mock 源返回成功
	foundMock := false
	for _, r := range results {
		if r.Source == "mock" && r.Err == nil {
			foundMock = true
		}
	}
	if !foundMock {
		t.Error("mock source should succeed")
	}

	// 4. 标记热点为已处理并验证去重
	if err := sub.StateStore.MarkTopicProcessed("E2E测试热点"); err != nil {
		t.Fatalf("MarkTopicProcessed: %v", err)
	}
	if !sub.StateStore.IsTopicProcessed("E2E测试热点") {
		t.Error("topic should be marked as processed")
	}

	// 5. 创建草稿
	draft := &ContentDraft{
		Title:    "E2E 测试文章",
		Body:     "这是端到端测试的内容",
		Platform: PlatformWebsite,
		Style:    StyleInformative,
	}
	if err := sub.DraftStore.Save(draft); err != nil {
		t.Fatalf("DraftStore.Save: %v", err)
	}
	if draft.ID == "" {
		t.Fatal("draft ID should be auto-generated")
	}

	// 6. 验证草稿可检索
	loaded, err := sub.DraftStore.Get(draft.ID)
	if err != nil {
		t.Fatalf("DraftStore.Get: %v", err)
	}
	if loaded.Title != "E2E 测试文章" {
		t.Errorf("title: got %q, want %q", loaded.Title, "E2E 测试文章")
	}

	// 7. 更新草稿状态 → approved
	if err := sub.DraftStore.UpdateStatus(draft.ID, DraftStatusApproved); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	// 8. 发布
	result, err := pub.Publish(context.Background(), loaded)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Status != "published" {
		t.Errorf("publish status: got %q, want %q", result.Status, "published")
	}

	// 9. 记录发布历史
	record := &PublishRecord{
		DraftID:  draft.ID,
		Title:    draft.Title,
		Platform: PlatformWebsite,
		PostID:   result.PostID,
		URL:      result.URL,
		Status:   "published",
	}
	if err := sub.PublishHistory.Save(record); err != nil {
		t.Fatalf("PublishHistory.Save: %v", err)
	}

	// 10. 验证发布历史可检索
	records, err := sub.PublishHistory.List(nil)
	if err != nil {
		t.Fatalf("PublishHistory.List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 publish record, got %d", len(records))
	}
	if records[0].Title != "E2E 测试文章" {
		t.Errorf("record title: got %q, want %q", records[0].Title, "E2E 测试文章")
	}

	// 11. 记录到持久状态
	if err := sub.StateStore.RecordPublish("website", draft.Title); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}
	stats := sub.StateStore.GetPublishStats()
	if stats.TotalPublished != 1 {
		t.Errorf("TotalPublished: got %d, want 1", stats.TotalPublished)
	}

	// 12. 更新草稿状态 → published
	if err := sub.DraftStore.UpdateStatus(draft.ID, DraftStatusPublished); err != nil {
		t.Fatalf("UpdateStatus to published: %v", err)
	}
	finalDraft, _ := sub.DraftStore.Get(draft.ID)
	if finalDraft.Status != DraftStatusPublished {
		t.Errorf("final status: got %q, want %q", finalDraft.Status, DraftStatusPublished)
	}
}

// TestE2E_OpportunityEvaluation_WithStateStore 验证评估引擎与状态存储联动
func TestE2E_OpportunityEvaluation_WithStateStore(t *testing.T) {
	dir := t.TempDir()

	sub, err := NewMediaSubsystem(MediaSubsystemConfig{
		Workspace:     dir,
		EnablePublish: true,
	})
	if err != nil {
		t.Fatalf("NewMediaSubsystem: %v", err)
	}

	now := time.Now()

	// 准备话题列表
	topics := []TrendingTopic{
		{Title: "新话题A", Source: "weibo", HeatScore: 60000, FetchedAt: now.Add(-30 * time.Minute)},
		{Title: "旧话题B", Source: "baidu", HeatScore: 45000, FetchedAt: now.Add(-1 * time.Hour)},
		{Title: "冷话题C", Source: "zhihu", HeatScore: 5000, FetchedAt: now.Add(-8 * time.Hour)},
	}

	// 标记 B 和 C 为已处理
	if err := sub.StateStore.MarkTopicProcessed("旧话题B"); err != nil {
		t.Fatalf("MarkTopicProcessed B: %v", err)
	}
	if err := sub.StateStore.MarkTopicProcessed("冷话题C"); err != nil {
		t.Fatalf("MarkTopicProcessed C: %v", err)
	}

	// 评估
	results := EvaluateOpportunities(topics, sub.StateStore, DefaultOpportunityConfig())
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// 新话题A 应该得最高分（高热度 + 新鲜 + 时效）
	top := results[0]
	if top.Topic.Title != "新话题A" {
		t.Errorf("top topic: got %q, want 新话题A", top.Topic.Title)
	}
	if top.Action != ActionCreate {
		t.Errorf("top action: got %q, want create", top.Action)
	}

	// 冷话题C 应该被跳过（低热度 + 超时效）
	for _, r := range results {
		if r.Topic.Title == "冷话题C" {
			if r.Action != ActionSkip {
				t.Errorf("cold topic action: got %q, want skip", r.Action)
			}
		}
	}
}

// TestE2E_AutoSpawn_WithCron 验证 AutoSpawn 计数 + Cron 注册联动
func TestE2E_AutoSpawn_WithCron(t *testing.T) {
	dir := t.TempDir()

	// 1. 初始化状态存储
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 2. 注册 Cron 任务（启用 AutoSpawn）
	adder := &mockCronAdder{}
	cfg := DefaultMediaCronConfig()
	cfg.AutoSpawnEnabled = true
	cfg.MaxAutoSpawnsPerDay = 3

	refs, err := RegisterMediaCronJobs(adder, cfg)
	if err != nil {
		t.Fatalf("RegisterMediaCronJobs: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 job refs, got %d", len(refs))
	}

	// 3. 验证 trending 任务的消息包含自动创作指引
	for _, j := range adder.jobs {
		if j.Name == "media.patrol.trending" {
			if len(j.Payload.Message) <= 100 {
				t.Error("trending message should contain auto-spawn guidance")
			}
		}
	}

	// 4. 验证 JobRef 包含 jobName
	trendingRef := refs[0]
	if trendingRef.JobName != "media.patrol.trending" {
		t.Errorf("first ref: got %q, want media.patrol.trending", trendingRef.JobName)
	}

	// 5. 模拟自动 spawn 消耗配额
	for i := 0; i < 3; i++ {
		if !store.CanAutoSpawn(3) {
			t.Fatalf("should be able to auto spawn (count=%d, max=3)", i)
		}
		if err := store.RecordAutoSpawn(); err != nil {
			t.Fatalf("RecordAutoSpawn: %v", err)
		}
	}

	// 6. 配额耗尽
	if store.CanAutoSpawn(3) {
		t.Error("should NOT be able to auto spawn (quota exhausted)")
	}
	if store.GetAutoSpawnCount() != 3 {
		t.Errorf("auto spawn count: got %d, want 3", store.GetAutoSpawnCount())
	}
}

// TestE2E_EventTriggerLifecycle 验证事件触发器完整生命周期
func TestE2E_EventTriggerLifecycle(t *testing.T) {
	adder := &mockCronAdder{}
	mgr := NewMediaEventManager()

	// 注册 cron 触发器
	mgr.Register(NewCronPollTrigger(CronPollTriggerConfig{
		Name:    "xhs-poll",
		CronSvc: adder,
		JobName: "media.poll.xhs",
		Message: "poll xhs",
		EveryMs: 60_000,
	}))

	// 注册 webhook 触发器
	var received []string
	mgr.Register(NewWebhookBridgeTrigger("wechat-mp", func(eventType string, payload map[string]any) {
		received = append(received, eventType)
	}))

	// 状态检查
	statuses := mgr.Status()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(statuses))
	}

	// 启动所有触发器
	if err := mgr.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	if len(adder.jobs) != 1 {
		t.Errorf("expected 1 cron job registered, got %d", len(adder.jobs))
	}

	// 触发 webhook 事件
	wt := mgr.triggers[1].(*WebhookBridgeTrigger)
	wt.OnEvent("new_comment", nil)
	wt.OnEvent("new_dm", nil)
	if len(received) != 2 {
		t.Errorf("expected 2 events received, got %d", len(received))
	}

	// 停止所有触发器
	mgr.StopAll()

	// 停止后 webhook 不应触发
	wt.OnEvent("should_not_fire", nil)
	if len(received) != 2 {
		t.Errorf("expected still 2 events after stop, got %d", len(received))
	}
}

// TestE2E_SystemPromptWithState 验证系统提示词包含跨会话状态
func TestE2E_SystemPromptWithState(t *testing.T) {
	dir := t.TempDir()

	sub, err := NewMediaSubsystem(MediaSubsystemConfig{
		Workspace:     dir,
		EnablePublish: true,
	})
	if err != nil {
		t.Fatalf("NewMediaSubsystem: %v", err)
	}

	// 记录一些状态
	if err := sub.StateStore.MarkTopicProcessed("已处理话题1"); err != nil {
		t.Fatalf("MarkTopicProcessed: %v", err)
	}
	if err := sub.StateStore.RecordPublish("wechat", "测试文章"); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}

	// 构建系统提示词
	prompt := sub.BuildSystemPrompt("创作热点内容", "", "session-123")
	if prompt == "" {
		t.Fatal("system prompt should not be empty")
	}
	if len(prompt) < 100 {
		t.Errorf("system prompt too short: %d chars", len(prompt))
	}
}

// TestE2E_DraftLifecycle 验证草稿完整生命周期（创建→列表→删除）
func TestE2E_DraftLifecycle(t *testing.T) {
	dir := t.TempDir()

	sub, err := NewMediaSubsystem(MediaSubsystemConfig{
		Workspace: dir,
	})
	if err != nil {
		t.Fatalf("NewMediaSubsystem: %v", err)
	}

	// 创建 3 个草稿
	for i := 0; i < 3; i++ {
		draft := &ContentDraft{
			Title:    "草稿" + itoa(i),
			Body:     "内容",
			Platform: PlatformWeChat,
			Style:    StyleCasual,
		}
		if err := sub.DraftStore.Save(draft); err != nil {
			t.Fatalf("Save draft %d: %v", i, err)
		}
	}

	// 列表
	drafts, err := sub.DraftStore.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(drafts) != 3 {
		t.Fatalf("expected 3 drafts, got %d", len(drafts))
	}

	// 按平台过滤
	filtered, err := sub.DraftStore.List("wechat")
	if err != nil {
		t.Fatalf("List(wechat): %v", err)
	}
	if len(filtered) != 3 {
		t.Errorf("filtered: got %d, want 3", len(filtered))
	}

	// 删除第一个
	if err := sub.DraftStore.Delete(drafts[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 验证删除后只剩 2 个
	remaining, _ := sub.DraftStore.List("")
	if len(remaining) != 2 {
		t.Errorf("after delete: got %d, want 2", len(remaining))
	}
}
