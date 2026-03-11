package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/session"
)

// ---------- P3: SQLite SessionStore ----------

func TestSQLiteSessionStore_SaveAndLoad(t *testing.T) {
	store := NewSessionStoreWithSQLite(t.TempDir())
	defer store.closeDB()

	now := time.Now().UnixMilli()
	entry := &SessionEntry{
		SessionKey:    "user:abc",
		SessionId:     "session_123",
		Label:         "test session",
		Channel:       "webchat",
		CreatedAt:     now,
		UpdatedAt:     now,
		ThinkingLevel: "standard",
		DisplayName:   "Test User",
	}
	store.Save(entry)

	loaded := store.LoadSessionEntry("user:abc")
	if loaded == nil {
		t.Fatal("expected loaded entry")
	}
	if loaded.SessionId != "session_123" {
		t.Fatalf("sessionId = %q, want session_123", loaded.SessionId)
	}
	if loaded.Label != "test session" {
		t.Fatalf("label = %q, want 'test session'", loaded.Label)
	}
	if loaded.ThinkingLevel != "standard" {
		t.Fatalf("thinkingLevel = %q, want standard", loaded.ThinkingLevel)
	}
	if loaded.DisplayName != "Test User" {
		t.Fatalf("displayName = %q, want 'Test User'", loaded.DisplayName)
	}
}

func TestSQLiteSessionStore_List(t *testing.T) {
	store := NewSessionStoreWithSQLite(t.TempDir())
	defer store.closeDB()

	store.Save(&SessionEntry{SessionKey: "a", SessionId: "1", CreatedAt: 100})
	store.Save(&SessionEntry{SessionKey: "b", SessionId: "2", CreatedAt: 200})

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	if store.Count() != 2 {
		t.Fatalf("count = %d, want 2", store.Count())
	}
}

func TestSQLiteSessionStore_Delete(t *testing.T) {
	store := NewSessionStoreWithSQLite(t.TempDir())
	defer store.closeDB()

	store.Save(&SessionEntry{SessionKey: "to-delete", SessionId: "1"})
	if store.Count() != 1 {
		t.Fatal("expected 1 entry before delete")
	}

	store.Delete("to-delete")
	if store.Count() != 0 {
		t.Fatalf("expected 0 entries after delete, got %d", store.Count())
	}
	if store.LoadSessionEntry("to-delete") != nil {
		t.Fatal("should be nil after delete")
	}
}

func TestSQLiteSessionStore_Reset(t *testing.T) {
	store := NewSessionStoreWithSQLite(t.TempDir())
	defer store.closeDB()

	store.Save(&SessionEntry{SessionKey: "a", SessionId: "1"})
	store.Save(&SessionEntry{SessionKey: "b", SessionId: "2"})
	store.Reset()

	if store.Count() != 0 {
		t.Fatalf("expected 0 after reset, got %d", store.Count())
	}
}

func TestSQLiteSessionStore_PersistAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()

	// 第一个实例：写入
	store1 := NewSessionStoreWithSQLite(tmpDir)
	store1.Save(&SessionEntry{
		SessionKey: "user:persist",
		SessionId:  "session_persist",
		Label:      "persistent",
		CreatedAt:  123,
		UpdatedAt:  456,
	})
	store1.closeDB()

	// 第二个实例：读取（模拟进程重启）
	store2 := NewSessionStoreWithSQLite(tmpDir)
	defer store2.closeDB()

	loaded := store2.LoadSessionEntry("user:persist")
	if loaded == nil {
		t.Fatal("expected persisted entry after restart")
	}
	if loaded.Label != "persistent" {
		t.Fatalf("label = %q, want persistent", loaded.Label)
	}
	if loaded.CreatedAt != 123 {
		t.Fatalf("createdAt = %d, want 123", loaded.CreatedAt)
	}
}

func TestSQLiteSessionStore_MetaPreservesExtendedFields(t *testing.T) {
	store := NewSessionStoreWithSQLite(t.TempDir())
	defer store.closeDB()

	entry := &SessionEntry{
		SessionKey:     "user:meta",
		SessionId:      "session_meta",
		Label:          "meta test",
		Channel:        "webchat",
		CreatedAt:      100,
		UpdatedAt:      200,
		ThinkingLevel:  "extended",
		VerboseLevel:   "detailed",
		ReasoningLevel: "high",
		ElevatedLevel:  "max",
		SendPolicy:     "always",
		ResponseUsage:  "full",
		ModelOverride:  "gpt-4",
		InputTokens:    500,
		OutputTokens:   300,
		TotalTokens:    800,
		SystemSent:     true,
		Subject:        "Test Subject",
		GroupChannel:   "general",
	}
	store.Save(entry)

	loaded := store.LoadSessionEntry("user:meta")
	if loaded == nil {
		t.Fatal("expected loaded entry")
	}
	if loaded.ThinkingLevel != "extended" {
		t.Fatalf("thinkingLevel = %q", loaded.ThinkingLevel)
	}
	if loaded.InputTokens != 500 {
		t.Fatalf("inputTokens = %d", loaded.InputTokens)
	}
	if !loaded.SystemSent {
		t.Fatal("systemSent should be true")
	}
	if loaded.Subject != "Test Subject" {
		t.Fatalf("subject = %q", loaded.Subject)
	}
}

// ---------- P3: JSON → SQLite 迁移 ----------

func TestMigrateJSONToSQLite(t *testing.T) {
	tmpDir := t.TempDir()

	// 准备 sessions.json
	sessions := map[string]*SessionEntry{
		"user:old": {
			SessionKey: "user:old",
			SessionId:  "session_old",
			Label:      "old session",
			CreatedAt:  100,
			UpdatedAt:  200,
		},
		"task:run-1": {
			SessionKey: "task:run-1",
			SessionId:  "run-1",
			Label:      "task 1",
			CreatedAt:  300,
			UpdatedAt:  400,
			TaskMeta: &session.TaskMeta{
				Status:  "completed",
				Summary: "done",
				Async:   true,
			},
		},
	}
	data, _ := json.MarshalIndent(sessions, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "sessions.json"), data, 0o644)

	// 创建 SQLite store（触发自动迁移）
	store := NewSessionStoreWithSQLite(tmpDir)
	defer store.closeDB()

	// 验证 session 已迁移
	loaded := store.LoadSessionEntry("user:old")
	if loaded == nil {
		t.Fatal("user:old should be migrated")
	}
	if loaded.Label != "old session" {
		t.Fatalf("label = %q, want 'old session'", loaded.Label)
	}

	// 验证 task: 条目不在 sessions 表
	taskEntry := store.LoadSessionEntry("task:run-1")
	if taskEntry != nil {
		t.Fatal("task: entry should not be in sessions table")
	}

	// 验证 sessions.json 已重命名
	if _, err := os.Stat(filepath.Join(tmpDir, "sessions.json")); !os.IsNotExist(err) {
		t.Fatal("sessions.json should be renamed to sessions.json.migrated")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "sessions.json.migrated")); err != nil {
		t.Fatal("sessions.json.migrated should exist")
	}

	// 验证 run 在 SQLite runs 表
	if store.db != nil {
		run := sqliteLoadRun(store.db, "run-1")
		if run == nil {
			t.Fatal("run-1 should be in runs table")
		}
		if run.Status != "completed" {
			t.Fatalf("run status = %q, want completed", run.Status)
		}
		if !run.Async {
			t.Fatal("run should be async")
		}
	}
}

func TestMigrateJSONToSQLite_NoJSONFile(t *testing.T) {
	tmpDir := t.TempDir()
	// 无 sessions.json — 应正常创建空 SQLite
	store := NewSessionStoreWithSQLite(tmpDir)
	defer store.closeDB()

	if store.Count() != 0 {
		t.Fatalf("expected 0 sessions, got %d", store.Count())
	}
	if store.db == nil {
		t.Fatal("db should be initialized")
	}
}

func TestSQLiteSessionStore_FallbackOnOpenError(t *testing.T) {
	// 传入一个不可写的路径 — 应降级到 JSON 模式
	store := NewSessionStoreWithSQLite("")
	if store.db != nil {
		t.Fatal("empty storePath should not create db")
	}
}
