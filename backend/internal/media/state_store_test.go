package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileMediaStateStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 初始状态应该是空的
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if len(state.ProcessedTopics) != 0 {
		t.Errorf("ProcessedTopics: got %d, want 0", len(state.ProcessedTopics))
	}

	// 修改并保存
	state.PublishCounts = map[string]int{"wechat": 3, "xiaohongshu": 1}
	state.LastPublishedTitle = "Test Article"
	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 重新加载验证
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.PublishCounts["wechat"] != 3 {
		t.Errorf("wechat count: got %d, want 3", loaded.PublishCounts["wechat"])
	}
	if loaded.LastPublishedTitle != "Test Article" {
		t.Errorf("title: got %q, want %q", loaded.LastPublishedTitle, "Test Article")
	}
}

func TestFileMediaStateStore_MarkTopicProcessed(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	if store.IsTopicProcessed("topic1") {
		t.Error("topic1 should not be processed initially")
	}

	if err := store.MarkTopicProcessed("topic1"); err != nil {
		t.Fatalf("MarkTopicProcessed: %v", err)
	}

	if !store.IsTopicProcessed("topic1") {
		t.Error("topic1 should be processed after marking")
	}
	if store.IsTopicProcessed("topic2") {
		t.Error("topic2 should not be processed")
	}

	// 验证持久化 — 重新创建 store
	store2, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore (reload): %v", err)
	}
	if !store2.IsTopicProcessed("topic1") {
		t.Error("topic1 should survive reload")
	}
}

func TestFileMediaStateStore_MarkTopicProcessed_Empty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 空标题应该无操作
	if err := store.MarkTopicProcessed(""); err != nil {
		t.Fatalf("MarkTopicProcessed empty: %v", err)
	}
	state, _ := store.Load()
	if len(state.ProcessedTopics) != 0 {
		t.Errorf("ProcessedTopics: got %d, want 0", len(state.ProcessedTopics))
	}
}

func TestFileMediaStateStore_GetPublishStats(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	stats := store.GetPublishStats()
	if stats.TotalPublished != 0 {
		t.Errorf("TotalPublished: got %d, want 0", stats.TotalPublished)
	}

	if err := store.RecordPublish("wechat", "Article 1"); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}
	if err := store.RecordPublish("wechat", "Article 2"); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}
	if err := store.RecordPublish("xiaohongshu", "Note 1"); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}

	stats = store.GetPublishStats()
	if stats.TotalPublished != 3 {
		t.Errorf("TotalPublished: got %d, want 3", stats.TotalPublished)
	}
	if stats.PlatformCounts["wechat"] != 2 {
		t.Errorf("wechat: got %d, want 2", stats.PlatformCounts["wechat"])
	}
	if stats.LastPublishedTitle != "Note 1" {
		t.Errorf("LastPublishedTitle: got %q, want %q", stats.LastPublishedTitle, "Note 1")
	}
	if stats.LastPublishedAt == nil {
		t.Error("LastPublishedAt should not be nil")
	}

	// Mark some topics
	_ = store.MarkTopicProcessed("t1")
	_ = store.MarkTopicProcessed("t2")
	stats = store.GetPublishStats()
	if stats.ProcessedTopicCount != 2 {
		t.Errorf("ProcessedTopicCount: got %d, want 2", stats.ProcessedTopicCount)
	}
}

func TestFileMediaStateStore_RecordPublish(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	if err := store.RecordPublish("website", "Blog Post"); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}

	state, _ := store.Load()
	if state.PublishCounts["website"] != 1 {
		t.Errorf("website count: got %d, want 1", state.PublishCounts["website"])
	}
	if state.LastPublishedTitle != "Blog Post" {
		t.Errorf("title: got %q, want %q", state.LastPublishedTitle, "Blog Post")
	}
}

func TestFileMediaStateStore_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	// 直接创建 store — loadFromDisk 对不存在的文件返回空状态
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state == nil {
		t.Fatal("state should not be nil")
	}
}

func TestFileMediaStateStore_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	// 写入损坏的 JSON
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := NewFileMediaStateStore(dir)
	if err == nil {
		t.Fatal("should error on corrupt file")
	}
}

func TestFileMediaStateStore_SaveNil(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	if err := store.Save(nil); err == nil {
		t.Fatal("Save(nil) should error")
	}
}

func TestFileMediaStateStore_LoadClone(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	_ = store.MarkTopicProcessed("topic_a")

	state1, _ := store.Load()
	state2, _ := store.Load()

	// 修改 state1 不应影响 state2
	state1.ProcessedTopics["topic_b"] = state1.ProcessedTopics["topic_a"]

	if _, ok := state2.ProcessedTopics["topic_b"]; ok {
		t.Error("Load should return independent clones")
	}
}

func TestFileMediaStateStore_GetPublishStatsClone(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	if err := store.RecordPublish("wechat", "Article 1"); err != nil {
		t.Fatalf("RecordPublish: %v", err)
	}

	stats := store.GetPublishStats()
	// 修改返回的 PlatformCounts 不应影响 store 内部状态
	stats.PlatformCounts["wechat"] = 999

	stats2 := store.GetPublishStats()
	if stats2.PlatformCounts["wechat"] != 1 {
		t.Errorf("GetPublishStats should return independent clone: got %d, want 1", stats2.PlatformCounts["wechat"])
	}
}

func TestFileMediaStateStore_SaveClone(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	state := &MediaState{
		PublishCounts: map[string]int{"wechat": 2},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 修改外部 state 不应影响 store 内部
	state.PublishCounts["wechat"] = 999

	loaded, _ := store.Load()
	if loaded.PublishCounts["wechat"] != 2 {
		t.Errorf("Save should clone: got %d, want 2", loaded.PublishCounts["wechat"])
	}
}

func TestCanAutoSpawn_UnderLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 初始状态：今日零计数，应可自动 spawn
	if !store.CanAutoSpawn(5) {
		t.Error("should be able to auto spawn initially")
	}

	// 记录 2 次
	if err := store.RecordAutoSpawn(); err != nil {
		t.Fatalf("RecordAutoSpawn: %v", err)
	}
	if err := store.RecordAutoSpawn(); err != nil {
		t.Fatalf("RecordAutoSpawn: %v", err)
	}

	if !store.CanAutoSpawn(5) {
		t.Error("should still be able to auto spawn (2 < 5)")
	}
	if store.GetAutoSpawnCount() != 2 {
		t.Errorf("GetAutoSpawnCount: got %d, want 2", store.GetAutoSpawnCount())
	}
}

func TestCanAutoSpawn_OverLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 记录到达上限
	for i := 0; i < 3; i++ {
		if err := store.RecordAutoSpawn(); err != nil {
			t.Fatalf("RecordAutoSpawn: %v", err)
		}
	}

	if store.CanAutoSpawn(3) {
		t.Error("should NOT be able to auto spawn (3 >= 3)")
	}
	if store.CanAutoSpawn(4) {
		t.Log("should be able with higher limit")
	}
}

func TestCanAutoSpawn_DayReset(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 模拟昨天的记录
	store.mu.Lock()
	store.state.AutoSpawnCount = 10
	store.state.AutoSpawnResetDay = "2020-01-01" // 过去的日期
	store.mu.Unlock()

	// 跨日应该允许
	if !store.CanAutoSpawn(5) {
		t.Error("should be able to auto spawn after day reset")
	}

	// 记录一次后计数重置
	if err := store.RecordAutoSpawn(); err != nil {
		t.Fatalf("RecordAutoSpawn: %v", err)
	}

	if store.GetAutoSpawnCount() != 1 {
		t.Errorf("GetAutoSpawnCount after reset: got %d, want 1", store.GetAutoSpawnCount())
	}
}

func TestFileMediaStateStore_RecordPublishViaInterface(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileMediaStateStore(dir)
	if err != nil {
		t.Fatalf("NewFileMediaStateStore: %v", err)
	}

	// 验证 RecordPublish 可通过 MediaStateStore 接口调用
	var iface MediaStateStore = store
	if err := iface.RecordPublish("website", "Blog Post"); err != nil {
		t.Fatalf("RecordPublish via interface: %v", err)
	}

	stats := iface.GetPublishStats()
	if stats.TotalPublished != 1 {
		t.Errorf("TotalPublished: got %d, want 1", stats.TotalPublished)
	}
	if stats.PlatformCounts["website"] != 1 {
		t.Errorf("website count: got %d, want 1", stats.PlatformCounts["website"])
	}
}
