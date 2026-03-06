package media

// ============================================================================
// media/state_store.go — 媒体子智能体持久状态
// 跨会话记住已处理热点、发布统计等信息。
// 存储模式: 单个 JSON 文件 (`_media/state.json`)。
//
// Tracking doc: docs/claude/tracking/tracking-media-subagent-upgrade.md §P2-1
// ============================================================================

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ---------- 接口 ----------

// MediaStateStore 持久状态存储接口。
type MediaStateStore interface {
	// Load 加载状态。文件不存在时返回空状态（不报错）。
	Load() (*MediaState, error)
	// Save 保存状态。
	Save(state *MediaState) error
	// MarkTopicProcessed 标记热点为已处理。
	MarkTopicProcessed(topicTitle string) error
	// IsTopicProcessed 检查热点是否已处理。
	IsTopicProcessed(topicTitle string) bool
	// GetPublishStats 获取发布统计。
	GetPublishStats() PublishStats
	// RecordPublish 记录一次发布事件。
	RecordPublish(platform, title string) error
	// CanAutoSpawn 检查今日是否可自动 spawn（计数未超限）。
	CanAutoSpawn(maxPerDay int) bool
	// RecordAutoSpawn 记录一次自动 spawn。
	RecordAutoSpawn() error
	// GetAutoSpawnCount 获取今日自动 spawn 计数。
	GetAutoSpawnCount() int
}

// ---------- 数据结构 ----------

// MediaState 跨会话持久化状态。
type MediaState struct {
	// ProcessedTopics 已处理热点标题集合（key=title, value=处理时间）。
	ProcessedTopics map[string]time.Time `json:"processed_topics,omitempty"`
	// PublishCounts 各平台发布计数。
	PublishCounts map[string]int `json:"publish_counts,omitempty"`
	// LastPublishedAt 最后一次发布时间。
	LastPublishedAt *time.Time `json:"last_published_at,omitempty"`
	// LastPublishedTitle 最后一次发布的标题。
	LastPublishedTitle string `json:"last_published_title,omitempty"`
	// UpdatedAt 状态更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// AutoSpawnCount 今日自动 spawn 计数。
	AutoSpawnCount int `json:"auto_spawn_count,omitempty"`
	// AutoSpawnResetDay 自动 spawn 计数重置日期（"2006-01-02" 格式）。
	AutoSpawnResetDay string `json:"auto_spawn_reset_day,omitempty"`
}

// PublishStats 发布统计摘要。
type PublishStats struct {
	TotalPublished      int            `json:"total_published"`
	PlatformCounts      map[string]int `json:"platform_counts,omitempty"`
	LastPublishedAt     *time.Time     `json:"last_published_at,omitempty"`
	LastPublishedTitle  string         `json:"last_published_title,omitempty"`
	ProcessedTopicCount int            `json:"processed_topic_count"`
}

// ---------- FileMediaStateStore ----------

// FileMediaStateStore JSON 文件驱动的状态存储。
type FileMediaStateStore struct {
	mu       sync.Mutex
	filePath string
	state    *MediaState // 内存缓存
}

// NewFileMediaStateStore 创建文件状态存储。
// 父目录不存在时自动创建。
func NewFileMediaStateStore(dir string) (*FileMediaStateStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	s := &FileMediaStateStore{
		filePath: filepath.Join(dir, "state.json"),
	}
	// 预加载状态到内存缓存
	state, err := s.loadFromDisk()
	if err != nil {
		return nil, err
	}
	s.state = state
	return s, nil
}

// Load 返回当前状态的副本。
func (s *FileMediaStateStore) Load() (*MediaState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMediaState(s.state), nil
}

// Save 保存状态到磁盘并更新缓存。
// 内部会克隆传入的 state，调用方后续修改不会影响 store。
func (s *FileMediaStateStore) Save(state *MediaState) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state.UpdatedAt = time.Now().UTC()
	if err := s.writeToDisk(state); err != nil {
		return err
	}
	// 防御性拷贝：存储克隆而非调用方指针
	s.state = cloneMediaState(state)
	return nil
}

// MarkTopicProcessed 标记热点为已处理。
func (s *FileMediaStateStore) MarkTopicProcessed(topicTitle string) error {
	if topicTitle == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.ProcessedTopics == nil {
		s.state.ProcessedTopics = make(map[string]time.Time)
	}
	s.state.ProcessedTopics[topicTitle] = time.Now().UTC()
	s.state.UpdatedAt = time.Now().UTC()
	return s.writeToDisk(s.state)
}

// IsTopicProcessed 检查热点是否已处理。
func (s *FileMediaStateStore) IsTopicProcessed(topicTitle string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.ProcessedTopics == nil {
		return false
	}
	_, ok := s.state.ProcessedTopics[topicTitle]
	return ok
}

// GetPublishStats 获取发布统计。
// 返回值中的 PlatformCounts 是独立副本，修改不影响 store。
func (s *FileMediaStateStore) GetPublishStats() PublishStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, c := range s.state.PublishCounts {
		total += c
	}
	// 克隆 map 防止调用方修改泄漏到内部状态
	var countsCopy map[string]int
	if s.state.PublishCounts != nil {
		countsCopy = make(map[string]int, len(s.state.PublishCounts))
		for k, v := range s.state.PublishCounts {
			countsCopy[k] = v
		}
	}
	return PublishStats{
		TotalPublished:      total,
		PlatformCounts:      countsCopy,
		LastPublishedAt:     s.state.LastPublishedAt,
		LastPublishedTitle:  s.state.LastPublishedTitle,
		ProcessedTopicCount: len(s.state.ProcessedTopics),
	}
}

// RecordPublish 记录一次发布事件。
func (s *FileMediaStateStore) RecordPublish(platform, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.PublishCounts == nil {
		s.state.PublishCounts = make(map[string]int)
	}
	s.state.PublishCounts[platform]++
	now := time.Now().UTC()
	s.state.LastPublishedAt = &now
	s.state.LastPublishedTitle = title
	s.state.UpdatedAt = now
	return s.writeToDisk(s.state)
}

// CanAutoSpawn 检查今日是否可自动 spawn（计数未超限）。
// 跨日自动重置计数。
func (s *FileMediaStateStore) CanAutoSpawn(maxPerDay int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	if s.state.AutoSpawnResetDay != today {
		// 跨日：视为可用
		return true
	}
	return s.state.AutoSpawnCount < maxPerDay
}

// RecordAutoSpawn 记录一次自动 spawn，跨日自动重置计数。
func (s *FileMediaStateStore) RecordAutoSpawn() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	if s.state.AutoSpawnResetDay != today {
		s.state.AutoSpawnCount = 0
		s.state.AutoSpawnResetDay = today
	}
	s.state.AutoSpawnCount++
	s.state.UpdatedAt = time.Now().UTC()
	return s.writeToDisk(s.state)
}

// GetAutoSpawnCount 获取今日自动 spawn 计数。跨日返回 0。
func (s *FileMediaStateStore) GetAutoSpawnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	if s.state.AutoSpawnResetDay != today {
		return 0
	}
	return s.state.AutoSpawnCount
}

// ---------- 内部 I/O ----------

func (s *FileMediaStateStore) loadFromDisk() (*MediaState, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &MediaState{}, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}
	var state MediaState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state file: %w", err)
	}
	return &state, nil
}

func (s *FileMediaStateStore) writeToDisk(state *MediaState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0o600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

// cloneMediaState 返回 MediaState 的深拷贝。
func cloneMediaState(src *MediaState) *MediaState {
	clone := &MediaState{
		UpdatedAt:          src.UpdatedAt,
		LastPublishedTitle: src.LastPublishedTitle,
		LastPublishedAt:    src.LastPublishedAt,
		AutoSpawnCount:     src.AutoSpawnCount,
		AutoSpawnResetDay:  src.AutoSpawnResetDay,
	}
	if src.ProcessedTopics != nil {
		clone.ProcessedTopics = make(map[string]time.Time, len(src.ProcessedTopics))
		for k, v := range src.ProcessedTopics {
			clone.ProcessedTopics[k] = v
		}
	}
	if src.PublishCounts != nil {
		clone.PublishCounts = make(map[string]int, len(src.PublishCounts))
		for k, v := range src.PublishCounts {
			clone.PublishCounts[k] = v
		}
	}
	return clone
}
