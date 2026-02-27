package uhms

import (
	"context"
)

// ============================================================================
// Core Interfaces — abstractions for pluggable backends
// ============================================================================

// MemoryStore defines CRUD + search operations on memories.
// Default implementation: Store (SQLite) + LocalVFS (file system).
type MemoryStore interface {
	CreateMemory(m *Memory) error
	GetMemory(id string) (*Memory, error)
	UpdateMemory(m *Memory) error
	DeleteMemory(id string) error
	ListMemories(userID string, opts ListOptions) ([]Memory, error)
	SearchByFTS5(userID, query string, limit int) ([]SearchResult, error)
	IncrementAccess(id string) error
	CountMemories(userID string) (int64, error)
	Close() error
}

// VFS defines the file system layer for L0/L1/L2 content.
// Default implementation: LocalVFS (local os files).
type VFS interface {
	// --- User Memory Operations ---
	WriteMemory(userID string, m *Memory, l0, l1, l2 string) error
	WriteArchive(userID, sessionKey, l0, l1 string) (string, error)
	DeleteMemory(userID, memoryType, category, memoryID string) error

	ReadL0(userID, memoryType, category, memoryID string) (string, error)
	ReadL1(userID, memoryType, category, memoryID string) (string, error)
	ReadL2(userID, memoryType, category, memoryID string) (string, error)
	ReadByVFSPath(vfsPath string, level int) (string, error)

	BatchReadL0(userID string, memories []Memory) []L0Entry
	BatchReadL1(userID string, memories []Memory) []L1Entry

	ListCategories(userID, memoryType string) ([]string, error)
	ListMemoryIDs(userID, memoryType, category string) ([]VFSDirEntry, error)
	ListArchives(userID string) ([]ArchiveEntry, error)
	DiskUsage(userID string) (int64, error)

	// --- System Entry Operations (_system/ namespace) ---
	// Used for skills, plugins, sessions — shared system-level data.
	WriteSystemEntry(namespace, category, id, l0, l1, l2 string, meta map[string]interface{}) error
	ReadSystemL0(namespace, category, id string) (string, error)
	ReadSystemL1(namespace, category, id string) (string, error)
	ReadSystemL2(namespace, category, id string) (string, error)
	ReadSystemMeta(namespace, category, id string) (map[string]interface{}, error)
	BatchReadSystemL0(namespace string, refs []SystemEntryRef) []SystemL0Entry
	ListSystemEntries(namespace, category string) ([]SystemEntryRef, error)
	ListSystemCategories(namespace string) ([]string, error)
	DeleteSystemEntry(namespace, category, id string) error
	SystemEntryExists(namespace, category, id string) bool
}

// VectorIndex abstracts optional vector search backends.
// Only used when VectorMode != VectorOff.
type VectorIndex interface {
	// Upsert adds or updates a vector for a memory.
	Upsert(ctx context.Context, collection, id string, vector []float32, payload map[string]interface{}) error

	// Search finds the top-k nearest vectors by cosine similarity.
	Search(ctx context.Context, collection string, query []float32, topK int) ([]VectorHit, error)

	// Delete removes a vector.
	Delete(ctx context.Context, collection, id string) error

	// Close releases resources.
	Close() error
}

// VectorHit represents a vector search result.
type VectorHit struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// PayloadHit represents a payload-based search result with metadata.
// Used for system collections (skills, plugins, sessions) where retrieval is by payload filter.
type PayloadHit struct {
	ID      string                 `json:"id"`
	Payload map[string]interface{} `json:"payload"`
	Score   float64                `json:"score"`
	VFSPath string                 `json:"vfs_path,omitempty"`
}

// EmbeddingProvider generates embedding vectors from text.
// Only used when VectorMode != VectorOff.
type EmbeddingProvider interface {
	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimension returns the embedding vector dimension.
	Dimension() int
}

// LLMProvider generates text completions for memory operations
// (classification, summarization, entity extraction, deduplication).
type LLMProvider interface {
	// Complete sends a prompt and returns the completion text.
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)

	// EstimateTokens returns an approximate token count for a string.
	EstimateTokens(text string) int
}

// ============================================================================
// Manager — top-level orchestrator interface
// ============================================================================

// Manager orchestrates all UHMS subsystems.
type Manager interface {
	// AddMemory classifies, deduplicates, and stores a new memory.
	AddMemory(ctx context.Context, userID, content string, memType MemoryType, category MemoryCategory) (*Memory, error)

	// SearchMemories finds relevant memories using FTS5 (+ optional vector).
	SearchMemories(ctx context.Context, userID, query string, opts SearchOptions) ([]SearchResult, error)

	// CommitSession extracts memories from a conversation transcript.
	CommitSession(ctx context.Context, userID, sessionKey string, transcript []Message) (*CommitResult, error)

	// BuildContextBlock builds a context injection block within a token budget.
	BuildContextBlock(ctx context.Context, userID, query string, tokenBudget int) (string, error)

	// CompressIfNeeded compresses messages if total tokens exceed threshold.
	CompressIfNeeded(ctx context.Context, messages []Message, tokenBudget int) ([]Message, error)

	// RunDecayCycle applies FSRS-6 decay to all non-permanent memories.
	RunDecayCycle(ctx context.Context, userID string) error

	// Status returns current UHMS subsystem status.
	Status() ManagerStatus

	// Close releases all resources.
	Close() error
}

// SearchOptions configures memory search behavior.
type SearchOptions struct {
	MemoryType    MemoryType
	Category      MemoryCategory
	TopK          int
	MinScore      float64
	IncludeVector bool // 是否使用向量搜索 (仅 VectorMode != off 时有效)
	TieredLevel   int  // 返回级别: 0=L0, 1=L1, 2=L2
}

// Message is a minimal chat message representation for compression/commit.
type Message struct {
	Role    string `json:"role"` // "user", "assistant", "system", "tool"
	Content string `json:"content"`
}

// ManagerStatus reports the UHMS subsystem health.
type ManagerStatus struct {
	Enabled     bool       `json:"enabled"`
	VectorMode  VectorMode `json:"vector_mode"`
	DBPath      string     `json:"db_path"`
	VFSPath     string     `json:"vfs_path"`
	VectorReady bool       `json:"vector_ready"` // false when VectorMode==off or backend unavailable
	MemoryCount int64      `json:"memory_count"`
	DiskUsage   int64      `json:"disk_usage_bytes"`
}
