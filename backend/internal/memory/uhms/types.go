// Package uhms — Unified Hierarchical Memory System (embedded local edition).
//
// Default storage: local file system (VFS) + SQLite metadata.
// Vector search is optional, controlled by VectorMode config.
package uhms

import (
	"time"
)

// ============================================================================
// Memory Types — 5 semantic memory types
// ============================================================================

// MemoryType classifies how a memory was formed and its cognitive role.
type MemoryType string

const (
	MemTypeEpisodic    MemoryType = "episodic"    // 情景记忆: 对话/事件
	MemTypeSemantic    MemoryType = "semantic"    // 语义记忆: 事实/知识
	MemTypeProcedural  MemoryType = "procedural"  // 程序记忆: 技能/流程
	MemTypePermanent   MemoryType = "permanent"   // 永久记忆: 核心偏好/身份
	MemTypeImagination MemoryType = "imagination" // 想象记忆: 推理/预测
)

// AllMemoryTypes lists all valid memory types.
var AllMemoryTypes = []MemoryType{
	MemTypeEpisodic, MemTypeSemantic, MemTypeProcedural,
	MemTypePermanent, MemTypeImagination,
}

// ============================================================================
// Memory Categories — 13 semantic classification categories
// ============================================================================

// MemoryCategory classifies the semantic content of a memory.
type MemoryCategory string

const (
	CatPreference   MemoryCategory = "preference"   // 偏好
	CatHabit        MemoryCategory = "habit"        // 习惯
	CatProfile      MemoryCategory = "profile"      // 个人信息
	CatSkill        MemoryCategory = "skill"        // 技能/知识
	CatRelationship MemoryCategory = "relationship" // 关系
	CatEvent        MemoryCategory = "event"        // 事件
	CatOpinion      MemoryCategory = "opinion"      // 观点
	CatFact         MemoryCategory = "fact"         // 事实
	CatGoal         MemoryCategory = "goal"         // 目标
	CatTask         MemoryCategory = "task"         // 任务
	CatReminder     MemoryCategory = "reminder"     // 提醒
	CatInsight      MemoryCategory = "insight"      // 洞察
	CatSummary      MemoryCategory = "summary"      // 总结
)

// AllCategories lists all valid memory categories.
var AllCategories = []MemoryCategory{
	CatPreference, CatHabit, CatProfile, CatSkill,
	CatRelationship, CatEvent, CatOpinion, CatFact,
	CatGoal, CatTask, CatReminder, CatInsight, CatSummary,
}

// ============================================================================
// Retention Policies
// ============================================================================

// RetentionPolicy controls how long a memory is kept before decay.
type RetentionPolicy string

const (
	RetentionStandard  RetentionPolicy = "standard"  // 正常衰减
	RetentionImportant RetentionPolicy = "important" // 缓慢衰减
	RetentionPermanent RetentionPolicy = "permanent" // 永不衰减
	RetentionSession   RetentionPolicy = "session"   // 会话结束清除
)

// ============================================================================
// Core Models — SQLite metadata + VFS file content
// ============================================================================

// Memory represents a single memory entry.
// Metadata stored in SQLite; content stored in VFS as L0/L1/L2 files.
type Memory struct {
	ID              string          `gorm:"primaryKey;size:36" json:"id"`
	UserID          string          `gorm:"index;not null" json:"user_id"`
	Content         string          `gorm:"type:text" json:"content"` // 简短描述 (SQLite, 用于 FTS5 搜索)
	MemoryType      MemoryType      `gorm:"size:32;default:episodic;index" json:"memory_type"`
	Category        MemoryCategory  `gorm:"size:32;default:fact;index" json:"category"`
	ImportanceScore float64         `gorm:"default:0.5" json:"importance_score"`
	DecayFactor     float64         `gorm:"default:1.0" json:"decay_factor"`
	RetentionPolicy RetentionPolicy `gorm:"size:30;default:standard" json:"retention_policy"`
	AccessCount     int             `gorm:"default:0" json:"access_count"`
	LastAccessedAt  *time.Time      `json:"last_accessed_at,omitempty"`
	ArchivedAt      *time.Time      `json:"archived_at,omitempty"`

	// 双时态 (bi-temporal)
	EventTime  *time.Time `gorm:"index" json:"event_time,omitempty"` // 事件实际发生时间
	IngestedAt time.Time  `gorm:"autoCreateTime;index" json:"ingested_at"`

	// VFS 路径 (指向文件系统中的 L0/L1/L2 目录)
	VFSPath string `gorm:"size:512" json:"vfs_path,omitempty"`

	// 可选向量索引引用 (仅 VectorMode != off 时使用)
	EmbeddingRef string `gorm:"size:36" json:"embedding_ref,omitempty"`

	// 元数据 JSON
	Metadata string `gorm:"type:text" json:"metadata,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// Entity represents a knowledge graph entity.
type Entity struct {
	ID          string    `gorm:"primaryKey;size:36" json:"id"`
	UserID      string    `gorm:"index;not null" json:"user_id"`
	Name        string    `gorm:"not null;index" json:"name"`
	EntityType  string    `gorm:"size:32" json:"entity_type"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// Relation represents a knowledge graph edge between two entities.
type Relation struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	UserID       string    `gorm:"index;not null" json:"user_id"`
	SourceID     string    `gorm:"size:36;not null;index" json:"source_id"`
	TargetID     string    `gorm:"size:36;not null;index" json:"target_id"`
	RelationType string    `gorm:"size:64;not null" json:"relation_type"`
	Weight       float64   `gorm:"default:1.0" json:"weight"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// CoreMemory stores the user's persistent core memory (persona, preferences).
type CoreMemory struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	UserID       string    `gorm:"uniqueIndex;not null" json:"user_id"`
	Persona      string    `gorm:"type:text" json:"persona"`
	Preferences  string    `gorm:"type:text" json:"preferences"`
	Instructions string    `gorm:"type:text" json:"instructions"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TreeNode represents a hierarchical memory tree node.
type TreeNode struct {
	ID            string    `gorm:"primaryKey;size:36" json:"id"`
	UserID        string    `gorm:"index;not null" json:"user_id"`
	ParentID      string    `gorm:"size:36;index" json:"parent_id,omitempty"`
	Content       string    `gorm:"type:text;not null" json:"content"`
	NodeType      string    `gorm:"size:32;default:leaf" json:"node_type"` // leaf, summary, root
	Category      string    `gorm:"size:32" json:"category"`
	Depth         int       `gorm:"default:0" json:"depth"`
	ChildrenCount int       `gorm:"default:0" json:"children_count"`
	IsLeaf        bool      `gorm:"default:true" json:"is_leaf"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// DecayProfile stores per-(user, memoryType) decay parameters.
type DecayProfile struct {
	ID         string     `gorm:"primaryKey;size:36" json:"id"`
	UserID     string     `gorm:"not null;uniqueIndex:idx_dp_user_type" json:"user_id"`
	MemoryType MemoryType `gorm:"size:32;not null;uniqueIndex:idx_dp_user_type" json:"memory_type"`
	HalfLife   float64    `gorm:"default:168" json:"half_life"` // hours (default 7 days)
	MinDecay   float64    `gorm:"default:0.01" json:"min_decay"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// ============================================================================
// L0/L1/L2 Tiered Loading Entries
// ============================================================================

// L0Entry is a minimal abstract (~100 tokens) for filtering/listing.
type L0Entry struct {
	MemoryID   string         `json:"memory_id"`
	MemoryType MemoryType     `json:"memory_type"`
	Category   MemoryCategory `json:"category"`
	Abstract   string         `json:"abstract"` // 1-2 句摘要
	Score      float64        `json:"score,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// L1Entry is a mid-level overview (~2K tokens) with enough detail.
type L1Entry struct {
	L0Entry
	Overview string `json:"overview"` // 段落级概述
}

// L2Entry is the full content (unlimited).
type L2Entry struct {
	L1Entry
	Detail   string            `json:"detail"` // 完整内容
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ============================================================================
// System Entry Types — for _system/ namespace (skills, plugins, sessions)
// ============================================================================

// SystemEntryRef identifies a system entry by namespace path components.
type SystemEntryRef struct {
	Category string `json:"category"`
	ID       string `json:"id"`
}

// SystemL0Entry is a minimal abstract for a system entry.
type SystemL0Entry struct {
	ID       string                 `json:"id"`
	Category string                 `json:"category"`
	Abstract string                 `json:"abstract"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
}

// ============================================================================
// List Options
// ============================================================================

// ListOptions configures memory listing queries.
type ListOptions struct {
	MemoryType      MemoryType
	Category        MemoryCategory
	MinImportance   float64
	MinDecayFactor  float64
	IncludeArchived bool
	Limit           int
	Offset          int
}

// ============================================================================
// Search Result
// ============================================================================

// SearchResult represents a memory search hit with score.
type SearchResult struct {
	Memory Memory  `json:"memory"`
	Score  float64 `json:"score"`
	Source string  `json:"source"` // "fts5", "vector", "hybrid"
}

// ============================================================================
// Session Commit Result
// ============================================================================

// CommitResult summarizes the output of committing a session to memory.
type CommitResult struct {
	SessionKey      string   `json:"session_key"`
	MemoriesCreated int      `json:"memories_created"`
	MemoriesUpdated int      `json:"memories_updated"`
	MemoryIDs       []string `json:"memory_ids"`
	ArchivePath     string   `json:"archive_path,omitempty"`
	TokensSaved     int      `json:"tokens_saved,omitempty"`
}
