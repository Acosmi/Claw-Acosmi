package gateway

// task_store.go — 独立的任务运行存储
// 将 task/run 从 session 域剥离，解决 session 索引污染问题。
// P3 (SQLite) 完成后本文件将退役为 SQLite 迁移输入源。

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/session"
)

// TaskEntry 任务运行条目。
// 从 SessionEntry 中剥离出的 task/run 专用结构。
type TaskEntry struct {
	TaskID      string            `json:"taskId"`
	SessionKey  string            `json:"sessionKey"`  // 关联的聊天 session key
	Label       string            `json:"label"`       // 任务摘要（截断文本）
	Status      string            `json:"status"`      // queued | started | progress | completed | failed
	Async       bool              `json:"async,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Error       string            `json:"error,omitempty"`
	CreatedAt   int64             `json:"createdAt"`
	UpdatedAt   int64             `json:"updatedAt"`
	TaskMeta    *session.TaskMeta `json:"taskMeta,omitempty"`
}

// TaskStore 线程安全的任务运行存储。
// 磁盘持久化到 tasks.json，与 sessions.json 完全隔离。
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*TaskEntry // key = taskId

	storePath string // 存储根路径
	filePath  string // tasks.json 完整路径
}

// NewTaskStore 创建任务存储。
func NewTaskStore(storePath string) *TaskStore {
	ts := &TaskStore{
		tasks:     make(map[string]*TaskEntry),
		storePath: storePath,
	}
	if storePath != "" {
		ts.filePath = filepath.Join(storePath, "tasks.json")
		ts.loadFromDisk()
	}
	return ts
}

// ---------- 磁盘 I/O ----------

func (ts *TaskStore) loadFromDisk() {
	if ts.filePath == "" {
		return
	}
	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		slog.Warn("task_store: failed to read tasks.json", "error", err, "path", ts.filePath)
		return
	}
	var loaded map[string]*TaskEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		slog.Warn("task_store: failed to parse tasks.json, starting fresh", "error", err)
		return
	}
	ts.tasks = loaded
	slog.Info("task_store: loaded from disk", "path", ts.filePath, "count", len(loaded))
}

func (ts *TaskStore) saveToDisk() {
	if ts.filePath == "" {
		return
	}
	dir := filepath.Dir(ts.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.Error("task_store: failed to create dir", "error", err, "dir", dir)
		return
	}
	data, err := json.MarshalIndent(ts.tasks, "", "  ")
	if err != nil {
		slog.Error("task_store: failed to marshal tasks", "error", err)
		return
	}
	tmpPath := ts.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		slog.Error("task_store: failed to write tmp", "error", err, "path", tmpPath)
		return
	}
	if err := os.Rename(tmpPath, ts.filePath); err != nil {
		slog.Error("task_store: failed to rename tmp", "error", err)
		os.Remove(tmpPath)
		return
	}
}

// ---------- 公开 API ----------

// Save 保存或更新任务条目。
func (ts *TaskStore) Save(entry *TaskEntry) {
	if entry == nil || entry.TaskID == "" {
		return
	}
	ts.mu.Lock()
	ts.tasks[entry.TaskID] = entry
	ts.saveToDisk()
	ts.mu.Unlock()
}

// Load 加载指定 taskId 的条目。
func (ts *TaskStore) Load(taskId string) *TaskEntry {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tasks[taskId]
}

// List 列出所有任务条目。
func (ts *TaskStore) List() []*TaskEntry {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	result := make([]*TaskEntry, 0, len(ts.tasks))
	for _, e := range ts.tasks {
		result = append(result, e)
	}
	return result
}

// Delete 删除任务条目。
func (ts *TaskStore) Delete(taskId string) {
	ts.mu.Lock()
	delete(ts.tasks, taskId)
	ts.saveToDisk()
	ts.mu.Unlock()
}

// Count 返回任务数量。
func (ts *TaskStore) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.tasks)
}

// UpdateStatus 更新任务状态（便捷方法，内部 load→merge→save）。
func (ts *TaskStore) UpdateStatus(taskId string, status string, merge func(entry *TaskEntry)) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	entry := ts.tasks[taskId]
	if entry == nil {
		return
	}
	entry.Status = status
	entry.UpdatedAt = time.Now().UnixMilli()
	if merge != nil {
		merge(entry)
	}
	ts.saveToDisk()
}

// SaveOrCreate 如果 taskId 不存在则创建，存在则更新。
func (ts *TaskStore) SaveOrCreate(taskId string, create func() *TaskEntry, update func(entry *TaskEntry)) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	entry := ts.tasks[taskId]
	if entry == nil {
		entry = create()
		if entry == nil {
			return
		}
		ts.tasks[taskId] = entry
	} else if update != nil {
		update(entry)
	}
	ts.saveToDisk()
}
