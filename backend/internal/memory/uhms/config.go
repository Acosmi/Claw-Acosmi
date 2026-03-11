package uhms

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ============================================================================
// Vector Modes — 6 optional vector search modes (default: off)
// ============================================================================

// VectorMode controls which vector search backend is used (if any).
type VectorMode string

const (
	// VectorOff disables vector search entirely. Default mode.
	// Memory retrieval uses SQLite FTS5 full-text search + file system traversal.
	VectorOff VectorMode = "off"

	// VectorBuiltin uses pure Go cosine similarity. Zero external dependencies.
	// Embeddings stored as binary blobs in SQLite. Good for < 10K memories.
	VectorBuiltin VectorMode = "builtin"

	// VectorFFI uses Rust FFI (nexus-core) for SIMD-accelerated cosine similarity.
	// Requires compiled Rust library (libnexus_unified.a).
	VectorFFI VectorMode = "ffi"

	// VectorSegment uses Qdrant segment library in-process via Rust FFI.
	// Memory-mapped disk storage. Best for 10K-1M memories.
	// Requires compiled Rust library.
	VectorSegment VectorMode = "segment"

	// VectorQdrant uses a Qdrant Docker container as external vector DB.
	// Best for large-scale (1M+ memories) or multi-instance deployment.
	// Requires Docker; user will be guided to pull qdrant/qdrant image.
	VectorQdrant VectorMode = "qdrant"

	// VectorHybrid combines Qdrant segment with BM25 (FTS5) reranking.
	// Best retrieval quality but requires Rust FFI.
	VectorHybrid VectorMode = "hybrid"
)

// AllVectorModes lists all valid vector modes.
var AllVectorModes = []VectorMode{
	VectorOff, VectorBuiltin, VectorFFI,
	VectorSegment, VectorQdrant, VectorHybrid,
}

// NeedsRustFFI returns true if the mode requires compiled Rust libraries.
func (m VectorMode) NeedsRustFFI() bool {
	return m == VectorFFI || m == VectorSegment || m == VectorHybrid
}

// NeedsDocker returns true if the mode requires a Docker container.
func (m VectorMode) NeedsDocker() bool {
	return m == VectorQdrant
}

// ============================================================================
// UHMS Configuration
// ============================================================================

// UHMSConfig is the top-level configuration for the embedded UHMS system.
type UHMSConfig struct {
	// Enabled activates the UHMS memory subsystem.
	Enabled bool `json:"enabled,omitempty"`

	// DBPath is the SQLite database path for metadata.
	// Default: ~/.openacosmi/memory/uhms.db
	DBPath string `json:"dbPath,omitempty"`

	// VFSPath is the root directory for L0/L1/L2 memory files.
	// Default: ~/.openacosmi/memory/vfs/
	VFSPath string `json:"vfsPath,omitempty"`

	// VectorMode selects the vector search backend. Default: "off" (file system only).
	VectorMode VectorMode `json:"vectorMode,omitempty"`

	// CompressionThreshold is the token count that triggers context compression.
	// Default: 200000
	CompressionThreshold int `json:"compressionThreshold,omitempty"`

	// DecayEnabled activates FSRS-6 memory decay. Default: true when Enabled.
	DecayEnabled *bool `json:"decayEnabled,omitempty"`

	// DecayIntervalHours is the interval between decay cycles. Default: 6.
	DecayIntervalHours int `json:"decayIntervalHours,omitempty"`

	// MaxMemories limits total memories per user. Default: 100000. 0 = unlimited.
	MaxMemories int `json:"maxMemories,omitempty"`

	// TieredLoadingEnabled enables L0/L1/L2 progressive loading. Default: true.
	TieredLoadingEnabled *bool `json:"tieredLoadingEnabled,omitempty"`

	// EmbeddingProvider configures which embedding provider to use when vector mode is active.
	// Values: "ollama", "openai", "anthropic", "cohere", "local", "auto" (default).
	// "auto" = try Ollama → fall back to cloud provider configured in LLM settings.
	EmbeddingProvider string `json:"embeddingProvider,omitempty"`

	// EmbeddingModel overrides the default embedding model for the selected provider.
	// Empty = provider default (e.g. "text-embedding-3-small" for OpenAI).
	EmbeddingModel string `json:"embeddingModel,omitempty"`

	// EmbeddingBaseURL overrides the default API base URL for the embedding provider.
	// Empty = provider default (e.g. "https://api.openai.com" for OpenAI).
	EmbeddingBaseURL string `json:"embeddingBaseUrl,omitempty"`

	// QdrantEndpoint is the Qdrant server URL when VectorMode == "qdrant".
	// Default: http://localhost:6334
	QdrantEndpoint string `json:"qdrantEndpoint,omitempty"`

	// CompressionTriggerPercent sets the context utilization percentage that triggers compression.
	// E.g. 80 means compress when totalTokens > budget*80/100.
	// 0 = legacy behavior (compress when totalTokens >= budget).
	CompressionTriggerPercent int `json:"compressionTriggerPercent,omitempty"`

	// ObservationMaskTurns controls observation masking: tool/system outputs older than
	// the most recent N user turns are replaced with placeholders.
	// 0 = disabled (no masking).
	ObservationMaskTurns int `json:"observationMaskTurns,omitempty"`

	// KeepRecentMessages is the number of recent messages to preserve during compression.
	// 0 = default (5).
	KeepRecentMessages int `json:"keepRecentMessages,omitempty"`

	// BootFilePath is the path to the boot.json file.
	// Default: ~/.openacosmi/memory/boot.json
	BootFilePath string `json:"bootFilePath,omitempty"`

	// SkillsVFSDistribution enables VFS tiered distribution for skills.
	// When enabled, skills.distribute RPC indexes skills into _system/skills/ VFS.
	// Default: false (backward compatible).
	SkillsVFSDistribution bool `json:"skillsVFSDistribution,omitempty"`
}

// DefaultUHMSConfig returns sensible defaults for local-first deployment.
func DefaultUHMSConfig() UHMSConfig {
	t := true
	return UHMSConfig{
		Enabled:              false,
		DBPath:               defaultDBPath(),
		VFSPath:              defaultVFSPath(),
		VectorMode:           VectorOff,
		CompressionThreshold: 200000,
		DecayEnabled:         &t,
		DecayIntervalHours:   6,
		MaxMemories:          100000,
		TieredLoadingEnabled: &t,
		EmbeddingProvider:    "auto",
		QdrantEndpoint:       "http://localhost:6334",
		// 压缩优化默认值 (参考 ACON NeurIPS 2025: 70% 触发 + observation masking)
		CompressionTriggerPercent: 75, // 75% budget 时触发压缩
		ObservationMaskTurns:      3,  // 保留最近 3 轮 user turn 完整输出
		BootFilePath:              defaultBootFilePath(),
	}
}

// IsDecayEnabled returns whether decay is active.
func (c *UHMSConfig) IsDecayEnabled() bool {
	return c.DecayEnabled == nil || *c.DecayEnabled
}

// IsTieredLoadingEnabled returns whether L0/L1/L2 progressive loading is active.
func (c *UHMSConfig) IsTieredLoadingEnabled() bool {
	return c.TieredLoadingEnabled == nil || *c.TieredLoadingEnabled
}

// ResolvedDBPath returns the absolute DB path, expanding ~ if needed.
func (c *UHMSConfig) ResolvedDBPath() string {
	if c.DBPath != "" {
		return expandHome(c.DBPath)
	}
	return defaultDBPath()
}

// ResolvedVFSPath returns the absolute VFS root path, expanding ~ if needed.
func (c *UHMSConfig) ResolvedVFSPath() string {
	if c.VFSPath != "" {
		return expandHome(c.VFSPath)
	}
	return defaultVFSPath()
}

// ResolvedKeepRecent returns the number of recent messages to keep during compression.
// Returns 5 if not configured (zero value).
func (c *UHMSConfig) ResolvedKeepRecent() int {
	if c.KeepRecentMessages > 0 {
		return c.KeepRecentMessages
	}
	return 5
}

// ResolvedTriggerPercent returns the compression trigger percentage.
// Returns 0 if not configured (legacy: compress at budget boundary).
func (c *UHMSConfig) ResolvedTriggerPercent() int {
	if c.CompressionTriggerPercent > 0 && c.CompressionTriggerPercent <= 100 {
		return c.CompressionTriggerPercent
	}
	return 0
}

// ResolvedBootFilePath returns the absolute boot file path, expanding ~ if needed.
func (c *UHMSConfig) ResolvedBootFilePath() string {
	if c.BootFilePath != "" {
		return expandHome(c.BootFilePath)
	}
	return defaultBootFilePath()
}

// ---------- helpers ----------

func defaultDBPath() string {
	return filepath.Join(defaultMemoryRoot(), "uhms.db")
}

func defaultVFSPath() string {
	return filepath.Join(defaultMemoryRoot(), "vfs")
}

func defaultBootFilePath() string {
	return filepath.Join(defaultMemoryRoot(), "boot.json")
}

func defaultMemoryRoot() string {
	stateDir := strings.TrimSpace(config.ResolveStateDir())
	if stateDir == "" || stateDir == "." {
		return filepath.Join(os.TempDir(), "openacosmi-memory")
	}
	return filepath.Join(stateDir, "memory")
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
