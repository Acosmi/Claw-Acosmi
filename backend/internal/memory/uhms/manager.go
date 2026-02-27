package uhms

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// compressSeq is an atomic counter to disambiguate archive keys within the same millisecond.
var compressSeq atomic.Int64

// safeGo 在后台 goroutine 中运行 fn，带 panic recovery。
// 参考: CockroachDB Stopper (禁止裸 go) + LaunchDarkly GoSafely 模式。
// panic → Error 级别 + stack trace; 正常错误由 fn 内部用 Warn 记录。
func safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := make([]byte, 8192)
				stack = stack[:runtime.Stack(stack, false)]
				slog.Error("uhms: background task panicked",
					slog.String("task", name),
					slog.Any("panic", r),
					slog.String("stack", string(stack)),
				)
			}
		}()
		fn()
	}()
}

// DefaultManager implements the Manager interface.
// Orchestrates Store (SQLite) + LocalVFS (file system) + optional VectorIndex.
type DefaultManager struct {
	cfg   UHMSConfig
	store *Store
	vfs   *LocalVFS
	cache *LRUCache
	llm   LLMProvider

	// Optional (nil when VectorMode == off)
	vectorIndex VectorIndex
	embedder    EmbeddingProvider

	// Optional: Anthropic Compaction API client (set via SetCompactionClient)
	compactionClient *AnthropicCompactionClient

	// lastSummary stores the most recent structured summary for anchored iterative compression.
	// Protected by mu. Persisted to VFS root as .last_summary; restored on gateway restart.
	lastSummary string

	mu     sync.RWMutex
	closed bool
}

// NewManager creates a new UHMS Manager with the given config and dependencies.
func NewManager(cfg UHMSConfig, llm LLMProvider) (*DefaultManager, error) {
	store, err := NewStore(cfg.ResolvedDBPath())
	if err != nil {
		return nil, fmt.Errorf("uhms: init store: %w", err)
	}

	vfs, err := NewLocalVFS(cfg.ResolvedVFSPath())
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("uhms: init vfs: %w", err)
	}

	cache := NewLRUCache(1000, 30*time.Minute)

	m := &DefaultManager{
		cfg:   cfg,
		store: store,
		vfs:   vfs,
		cache: cache,
		llm:   llm,
	}

	// Restore lastSummary from VFS if persisted from a previous session.
	m.loadLastSummary()

	slog.Info("uhms: manager initialized",
		"vectorMode", cfg.VectorMode,
		"dbPath", cfg.ResolvedDBPath(),
		"vfsPath", cfg.ResolvedVFSPath(),
	)
	return m, nil
}

// SetVectorBackend injects optional vector search components.
// Call after NewManager when VectorMode != off.
func (m *DefaultManager) SetVectorBackend(idx VectorIndex, emb EmbeddingProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vectorIndex = idx
	m.embedder = emb
}

// SetCompactionClient injects the optional Anthropic Compaction API client.
// When set, CompressIfNeeded will prefer server-side compaction over local summarization.
func (m *DefaultManager) SetCompactionClient(c *AnthropicCompactionClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compactionClient = c
}

// SetLLMProvider hot-swaps the LLM provider used for memory operations.
// Safe to call while manager is running — new safeGo tasks will pick up the new provider.
func (m *DefaultManager) SetLLMProvider(llm LLMProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llm = llm
}

// LLMInfo returns the current LLM provider and model identifiers.
// Returns empty strings if no LLMClientAdapter is set (e.g. nil or custom impl).
func (m *DefaultManager) LLMInfo() (provider, model string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if adapter, ok := m.llm.(*LLMClientAdapter); ok {
		return adapter.Provider, adapter.Model
	}
	return "", ""
}

// ============================================================================
// AddMemory
// ============================================================================

func (m *DefaultManager) AddMemory(ctx context.Context, userID, content string, memType MemoryType, category MemoryCategory) (*Memory, error) {
	if content == "" {
		return nil, fmt.Errorf("uhms: content is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("uhms: user_id is required")
	}

	// 默认值
	if memType == "" {
		memType = MemTypeEpisodic
	}
	if category == "" {
		category = m.classifyCategory(ctx, content, memType)
	}

	// 检查容量限制
	if m.cfg.MaxMemories > 0 {
		count, _ := m.store.CountMemories(userID)
		if count >= int64(m.cfg.MaxMemories) {
			return nil, fmt.Errorf("uhms: memory limit reached (%d)", m.cfg.MaxMemories)
		}
	}

	// 去重检查
	action, existingID := m.checkDuplicate(ctx, userID, content)
	switch action {
	case dedupNoop:
		slog.Debug("uhms: duplicate detected, skipping", "existingID", existingID)
		existing, err := m.store.GetMemory(existingID)
		if err == nil {
			m.store.IncrementAccess(existingID)
			return existing, nil
		}
	case dedupUpdate:
		slog.Debug("uhms: updating existing memory", "existingID", existingID)
		existing, err := m.store.GetMemory(existingID)
		if err == nil {
			existing.Content = content
			existing.Category = category
			m.store.UpdateMemory(existing)
			m.writeVFS(userID, existing, content)
			return existing, nil
		}
	}

	// 创建新记忆
	mem := &Memory{
		ID:              newID(),
		UserID:          userID,
		Content:         truncate(content, 500), // SQLite 存简短描述
		MemoryType:      memType,
		Category:        category,
		ImportanceScore: m.estimateImportance(content, memType),
		DecayFactor:     1.0,
		RetentionPolicy: retentionForType(memType),
		IngestedAt:      time.Now().UTC(),
		CreatedAt:       time.Now().UTC(),
	}

	// 1. SQLite 元数据 (同步, 必须成功)
	if err := m.store.CreateMemory(mem); err != nil {
		return nil, fmt.Errorf("uhms: create memory: %w", err)
	}

	// 2. VFS 文件写入 (同步)
	m.writeVFS(userID, mem, content)

	// 3. 可选: 向量索引 (异步, safeGo 包装防 panic 崩溃进程)
	if m.vectorIndex != nil && m.embedder != nil {
		safeGo("vector_index", func() { m.indexVector(context.Background(), mem, content) })
	}

	slog.Debug("uhms: memory added", "id", mem.ID, "type", memType, "category", category)
	return mem, nil
}

// ============================================================================
// SearchMemories
// ============================================================================

func (m *DefaultManager) SearchMemories(ctx context.Context, userID, query string, opts SearchOptions) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("uhms: query is required")
	}
	if opts.TopK <= 0 {
		opts.TopK = 20
	}

	// 缓存检查
	cacheKey := m.searchCacheKey(userID, query, opts)
	if cached, ok := m.cache.Get(cacheKey); ok {
		if results, ok := cached.([]SearchResult); ok {
			return results, nil
		}
	}

	var results []SearchResult

	// 向量搜索 (如果启用且请求)
	if opts.IncludeVector && m.vectorIndex != nil && m.embedder != nil {
		vectorResults, err := m.searchByVector(ctx, userID, query, opts.TopK)
		if err != nil {
			slog.Warn("uhms: vector search failed, falling back to FTS5", "error", err)
		} else {
			results = append(results, vectorResults...)
		}
	}

	// FTS5 全文搜索 (始终执行)
	ftsResults, err := m.store.SearchByFTS5(userID, query, opts.TopK)
	if err != nil {
		return nil, fmt.Errorf("uhms: search: %w", err)
	}

	// 合并去重
	results = mergeSearchResults(results, ftsResults, opts.TopK)

	// 按过滤条件筛选
	results = filterResults(results, opts)

	// 增加访问计数
	for _, r := range results {
		m.store.IncrementAccess(r.Memory.ID)
	}

	// 缓存结果
	m.cache.Set(cacheKey, results)

	return results, nil
}

// ============================================================================
// BuildContextBlock
// ============================================================================

func (m *DefaultManager) BuildContextBlock(ctx context.Context, userID, query string, tokenBudget int) (string, error) {
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}

	// 搜索相关记忆
	results, err := m.SearchMemories(ctx, userID, query, SearchOptions{
		TopK:          50,
		IncludeVector: m.vectorIndex != nil,
		TieredLevel:   0, // 先拿 L0
	})
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	// 渐进加载: L0 摘要 → 按 budget 决定加载 L1/L2
	var block strings.Builder
	block.WriteString("## Relevant Memories\n\n")

	usedTokens := m.estimateTokens(block.String())
	for _, r := range results {
		mem := r.Memory

		// 尝试读 L0
		l0, err := m.vfs.ReadL0(userID, string(mem.MemoryType), string(mem.Category), mem.ID)
		if err != nil || l0 == "" {
			l0 = truncate(mem.Content, 200)
		}

		entry := fmt.Sprintf("- [%s/%s] %s\n", mem.MemoryType, mem.Category, l0)
		entryTokens := m.estimateTokens(entry)

		if usedTokens+entryTokens > tokenBudget {
			break
		}

		// 如果 budget 充裕, 尝试加载 L1 替代 L0 (参考 LlamaIndex AutoMergingRetriever: 加载后实测 token)
		// 预留至少 20% budget 给后续条目 (参考 Progressive Context Disclosure 最佳实践)
		if usedTokens+entryTokens < tokenBudget*4/5 {
			l1, err := m.vfs.ReadL1(userID, string(mem.MemoryType), string(mem.Category), mem.ID)
			if err == nil && l1 != "" {
				l1Entry := fmt.Sprintf("- [%s/%s] %s\n", mem.MemoryType, mem.Category, l1)
				l1Tokens := m.estimateTokens(l1Entry)
				// 实测 L1 token 后决定: 仅当 L1 不会超 budget 时才升级
				if usedTokens+l1Tokens <= tokenBudget {
					entry = l1Entry
					entryTokens = l1Tokens
				}
			}
		}

		block.WriteString(entry)
		usedTokens += entryTokens
	}

	return block.String(), nil
}

// maskObservations replaces tool/system message content with placeholders for messages
// older than the most recent N user turns. This reduces token count significantly
// (NeurIPS 2025: tool outputs account for ~84% of trajectory tokens).
//
// Returns a new slice; does not modify the original messages.
// If ObservationMaskTurns == 0, returns the original slice unchanged.
func (m *DefaultManager) maskObservations(messages []Message) []Message {
	maskTurns := m.cfg.ObservationMaskTurns
	if maskTurns <= 0 || len(messages) == 0 {
		return messages
	}

	// Count user turns from the end to find the cutoff index.
	// If we find maskTurns user messages, everything before that point gets masked.
	// If there aren't enough user turns, don't mask anything.
	userTurnCount := 0
	cutoffIdx := 0 // default: mask nothing
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userTurnCount++
			if userTurnCount >= maskTurns {
				cutoffIdx = i
				break
			}
		}
	}

	if cutoffIdx <= 0 {
		return messages // nothing to mask
	}

	// Build new slice with masked old tool/system outputs
	result := make([]Message, len(messages))
	copy(result, messages)
	for i := 0; i < cutoffIdx; i++ {
		role := result[i].Role
		if role == "tool" || role == "system" {
			content := result[i].Content
			preview := content
			runes := []rune(content)
			if len(runes) > 100 {
				preview = string(runes[:100])
			}
			result[i] = Message{
				Role:    role,
				Content: fmt.Sprintf(ObservationMaskPlaceholderFmt, preview),
			}
		}
	}
	return result
}

// ============================================================================
// CompressIfNeeded — P3 核心压缩中间件
// ============================================================================

func (m *DefaultManager) CompressIfNeeded(ctx context.Context, messages []Message, tokenBudget int) ([]Message, error) {
	if tokenBudget <= 0 {
		tokenBudget = m.cfg.CompressionThreshold
	}
	if tokenBudget <= 0 {
		tokenBudget = 200000
	}

	// 估算当前 token 总量
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += m.estimateTokens(msg.Content)
	}

	// 触发判定: 百分比模式 vs legacy 模式
	triggerPercent := m.cfg.ResolvedTriggerPercent()
	shouldCompress := false
	if triggerPercent > 0 {
		threshold := tokenBudget * triggerPercent / 100
		shouldCompress = totalTokens > threshold
	} else {
		shouldCompress = totalTokens >= tokenBudget
	}

	if !shouldCompress {
		return messages, nil
	}

	slog.Info("uhms: compressing context",
		"totalTokens", totalTokens,
		"budget", tokenBudget,
		"triggerPercent", triggerPercent,
	)

	// Observation Masking: 遮蔽旧 tool/system 输出
	masked := m.maskObservations(messages)

	// 保留最近 N 条消息
	keepRecent := m.cfg.ResolvedKeepRecent()
	if keepRecent > len(masked) {
		keepRecent = len(masked)
	}
	recentMessages := masked[len(masked)-keepRecent:]
	oldMessages := masked[:len(masked)-keepRecent]

	// 1. 摘要旧消息 (anchored iteration: 读取 lastSummary)
	m.mu.RLock()
	prevSummary := m.lastSummary
	m.mu.RUnlock()

	summary, err := m.summarizeMessages(ctx, oldMessages, prevSummary)
	if err != nil {
		slog.Warn("uhms: summarize failed, returning original messages", "error", err)
		return messages, nil
	}

	// 存储 lastSummary (加锁写入) + 异步持久化到 VFS
	m.mu.Lock()
	m.lastSummary = summary
	m.mu.Unlock()
	m.persistLastSummary(summary)

	// 提取 userID 一次，供后续异步闭包共享 (避免 triple extractUserID 冗余)
	asyncUserID := extractUserID(messages)
	if asyncUserID == "" {
		asyncUserID = "default"
	}

	// 2a. 压缩存档写入 VFS L0/L1/L2 (异步, non-blocking)
	//     使 CompressIfNeeded 产物可被 BuildContextBlock 在跨会话恢复时检索到。
	safeGo("compress_archive", func() {
		l0 := truncate(summary, 200)
		l1 := summary
		l2 := buildTranscriptText(oldMessages)
		archiveKey := fmt.Sprintf("compress_%d_%d", time.Now().UnixMilli(), compressSeq.Add(1))
		if _, err := m.vfs.WriteArchive(asyncUserID, archiveKey, l0, l1, l2); err != nil {
			slog.Warn("uhms: compress archive write failed (non-fatal)", "error", err)
		}
	})

	// 2b. 压缩摘要作为 semantic/summary 记忆入库 (异步)
	safeGo("compress_memory", func() {
		if _, err := m.AddMemory(context.Background(), asyncUserID, summary, MemTypeSemantic, CatSummary); err != nil {
			slog.Warn("uhms: compress summary memory failed (non-fatal)", "error", err)
		}
	})

	// 2c. 提取记忆 → 持久化到 VFS (异步, safeGo 包装, 使用原始消息)
	safeGo("memory_extraction", func() {
		m.extractAndStoreMemories(context.Background(), asyncUserID, messages[:len(messages)-keepRecent])
	})

	// 3. 构建压缩后消息
	compressed := make([]Message, 0, keepRecent+2)

	// 搜索相关记忆注入
	query := extractQueryFromRecent(recentMessages)
	if query != "" {
		userID := extractUserID(messages)
		if userID == "" {
			userID = "default"
		}
		memBlock, err := m.BuildContextBlock(ctx, userID, query, tokenBudget/5)
		if err == nil && memBlock != "" {
			compressed = append(compressed, Message{
				Role:    "system",
				Content: memBlock,
			})
		}
	}

	// 摘要消息
	compressed = append(compressed, Message{
		Role:    "system",
		Content: fmt.Sprintf("[Conversation Summary]\n%s", summary),
	})

	// 最近消息原样保留
	compressed = append(compressed, recentMessages...)

	newTokens := 0
	for _, msg := range compressed {
		newTokens += m.estimateTokens(msg.Content)
	}

	slog.Info("uhms: compression complete",
		"before", totalTokens,
		"after", newTokens,
		"ratio", fmt.Sprintf("%.1f%%", float64(newTokens)/float64(totalTokens)*100),
	)

	return compressed, nil
}

// ============================================================================
// CommitSession — 会话 → 记忆提取 (P2.3 实现在 session_committer.go)
// ============================================================================

func (m *DefaultManager) CommitSession(ctx context.Context, userID, sessionKey string, transcript []Message) (*CommitResult, error) {
	return commitSession(ctx, m, userID, sessionKey, transcript)
}

// ============================================================================
// RunDecayCycle — FSRS-6 衰减 (P2.2 实现在 decay.go)
// ============================================================================

func (m *DefaultManager) RunDecayCycle(ctx context.Context, userID string) error {
	return runDecayCycle(ctx, m.store, userID)
}

// ============================================================================
// Status
// ============================================================================

func (m *DefaultManager) Status() ManagerStatus {
	userID := "default" // 状态报告用默认用户
	count, _ := m.store.CountMemories(userID)
	diskUsage, _ := m.vfs.DiskUsage(userID)

	return ManagerStatus{
		Enabled:     m.cfg.Enabled,
		VectorMode:  m.cfg.VectorMode,
		DBPath:      m.cfg.ResolvedDBPath(),
		VFSPath:     m.cfg.ResolvedVFSPath(),
		VectorReady: m.vectorIndex != nil,
		MemoryCount: count,
		DiskUsage:   diskUsage,
	}
}

// CompressThreshold returns the configured compression token threshold.
// Used by adapter layer for fast-path gating to skip unnecessary conversions.
func (m *DefaultManager) CompressThreshold() int {
	if m.cfg.CompressionThreshold > 0 {
		return m.cfg.CompressionThreshold
	}
	return 200000 // 默认 200K tokens
}

// ============================================================================
// GetMemory — 获取单条记忆 + 更新访问计数
// ============================================================================

func (m *DefaultManager) GetMemory(ctx context.Context, id string) (*Memory, error) {
	if id == "" {
		return nil, fmt.Errorf("uhms: memory id is required")
	}

	mem, err := m.store.GetMemory(id)
	if err != nil {
		return nil, fmt.Errorf("uhms: get memory: %w", err)
	}

	// 更新访问计数 (best-effort)
	m.store.IncrementAccess(id)

	return mem, nil
}

// ============================================================================
// DeleteMemory — 删除记忆 (含所有权校验)
// ============================================================================

func (m *DefaultManager) DeleteMemory(ctx context.Context, userID, id string) error {
	if id == "" {
		return fmt.Errorf("uhms: memory id is required")
	}
	if userID == "" {
		return fmt.Errorf("uhms: user_id is required")
	}

	// 获取记忆元数据 (需要 memType/category 用于 VFS 路径)
	mem, err := m.store.GetMemory(id)
	if err != nil {
		return fmt.Errorf("uhms: delete: memory not found: %w", err)
	}

	// 数据层所有权验证
	if mem.UserID != userID {
		return fmt.Errorf("uhms: delete: permission denied (owner mismatch)")
	}

	// 1. SQLite 删除 (同步, 必须成功)
	if err := m.store.DeleteMemory(id); err != nil {
		return fmt.Errorf("uhms: delete memory: %w", err)
	}

	// 2. VFS 文件删除 (best-effort, 失败仅 Warn)
	if vfsErr := m.vfs.DeleteMemory(userID, string(mem.MemoryType), string(mem.Category), id); vfsErr != nil {
		slog.Warn("uhms: VFS delete failed (non-fatal)", "id", id, "error", vfsErr)
	}

	// 3. 向量索引删除 (异步, nil-safe)
	if m.vectorIndex != nil {
		collection := "mem_" + string(mem.MemoryType)
		memID := id
		safeGo("vector_delete", func() {
			if delErr := m.vectorIndex.Delete(context.Background(), collection, memID); delErr != nil {
				slog.Warn("uhms: vector delete failed (non-fatal)", "id", memID, "error", delErr)
			}
		})
	}

	// 4. 清除搜索缓存
	m.cache.Clear()

	slog.Debug("uhms: memory deleted", "id", id, "userID", userID)
	return nil
}

// ============================================================================
// ListMemories — 分页列表
// ============================================================================

func (m *DefaultManager) ListMemories(ctx context.Context, userID string, opts ListOptions) ([]Memory, int64, error) {
	if userID == "" {
		return nil, 0, fmt.Errorf("uhms: user_id is required")
	}

	memories, err := m.store.ListMemories(userID, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("uhms: list memories: %w", err)
	}

	total, err := m.store.CountMemories(userID)
	if err != nil {
		return nil, 0, fmt.Errorf("uhms: count memories: %w", err)
	}

	return memories, total, nil
}

// ============================================================================
// AggregateStats — 聚合统计
// ============================================================================

// AggregateStats returns aggregated memory statistics for a user.
func (m *DefaultManager) AggregateStats(userID string) (*AggregateStats, error) {
	return m.store.AggregateStats(userID)
}

// ============================================================================
// ReadVFSContent — 按 level 读取 VFS 内容
// ============================================================================

func (m *DefaultManager) ReadVFSContent(userID, memType, category, memID string, level int) (string, error) {
	switch level {
	case 0:
		return m.vfs.ReadL0(userID, memType, category, memID)
	case 1:
		return m.vfs.ReadL1(userID, memType, category, memID)
	case 2:
		return m.vfs.ReadL2(userID, memType, category, memID)
	default:
		return "", fmt.Errorf("uhms: invalid VFS level %d (expected 0-2)", level)
	}
}

// ============================================================================
// ImportSkill — 导入技能文档到 UHMS，自动生成 L0/L1/L2
// ============================================================================

// ImportSkillResult 描述单个技能导入的结果。
type ImportSkillResult struct {
	Memory  *Memory
	IsNew   bool   // true=新增, false=跳过或更新
	Updated bool   // true=内容已更新
	Status  string // "imported" / "skipped" / "updated"
}

func (m *DefaultManager) ImportSkill(ctx context.Context, userID, skillName, fullContent string) (*ImportSkillResult, error) {
	if skillName == "" {
		return nil, fmt.Errorf("uhms: skill name is required")
	}
	if fullContent == "" {
		return nil, fmt.Errorf("uhms: skill content is required")
	}
	if userID == "" {
		userID = "default"
	}

	// 用 FTS5 搜索同名技能（精确匹配 skillName 前缀）
	existing, err := m.store.SearchByFTS5(userID, skillName, 10)
	if err == nil && len(existing) > 0 {
		// 查找精确匹配: content 以 skillName 开头或包含 skillName 的 procedural/skill 记忆
		for i := range existing {
			mem := &existing[i].Memory
			if mem.MemoryType != MemTypeProcedural || mem.Category != CatSkill {
				continue
			}
			// 读取 L2 全文比较
			l2, l2Err := m.vfs.ReadL2(userID, string(mem.MemoryType), string(mem.Category), mem.ID)
			if l2Err != nil {
				continue
			}

			// 内容相同 → 跳过
			if contentHash(l2) == contentHash(fullContent) {
				return &ImportSkillResult{Memory: mem, IsNew: false, Updated: false, Status: "skipped"}, nil
			}

			// 内容变化 → 更新
			mem.Content = truncate(fullContent, 500)
			mem.UpdatedAt = time.Now().UTC()
			if updateErr := m.store.UpdateMemory(mem); updateErr != nil {
				return nil, fmt.Errorf("uhms: update skill memory: %w", updateErr)
			}
			m.writeVFS(userID, mem, fullContent)
			m.cache.Clear()
			return &ImportSkillResult{Memory: mem, IsNew: false, Updated: true, Status: "updated"}, nil
		}
	}

	// 不存在 → 新增
	mem, err := m.AddMemory(ctx, userID, fullContent, MemTypeProcedural, CatSkill)
	if err != nil {
		return nil, fmt.Errorf("uhms: import skill %q: %w", skillName, err)
	}

	return &ImportSkillResult{Memory: mem, IsNew: true, Updated: false, Status: "imported"}, nil
}

// ============================================================================
// BuildContextBrief — L0 级别上下文简报，用于子智能体注入
// ============================================================================

// BuildContextBrief generates an L0-level context brief (~200 tokens) from
// the current lastSummary. Used to inject into sub-agent (coder/argus)
// tool calls, reducing inter-agent misalignment.
//
// Returns empty string if no summary is available yet.
func (m *DefaultManager) BuildContextBrief(ctx context.Context, userID string) string {
	m.mu.RLock()
	summary := m.lastSummary
	m.mu.RUnlock()

	if summary == "" {
		return ""
	}

	// Extract Session Intent + Files Modified + Decisions from structured summary.
	// The 7-section template has: Session Intent / Files Modified / Decisions Made /
	// Errors Encountered / Current State / Next Steps / Breadcrumbs.
	// For sub-agent brief, keep Session Intent + Files Modified + Current State.
	brief := extractBriefSections(summary)
	if brief == "" {
		// Fallback: truncate the full summary
		brief = truncate(summary, 500)
	}

	return fmt.Sprintf("[Context Brief]\n%s", brief)
}

// extractBriefSections extracts key sections from the structured 7-section summary.
func extractBriefSections(summary string) string {
	lines := strings.Split(summary, "\n")
	var sb strings.Builder
	capturing := false
	capturedSections := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Match section headers:
		//   "## Session Intent" (markdown H2)
		//   "**Session Intent**:" or "**Session Intent**" (bold standalone line)
		// Avoid matching inline bold text like "some **bold** word" by requiring
		// the line to start with "## " or start+end with "**".
		isHeader := strings.HasPrefix(trimmed, "## ")
		if !isHeader && strings.HasPrefix(trimmed, "**") {
			// Only treat as header if the line is a standalone bold label
			// (ends with "**" or "**:" optionally followed by whitespace)
			stripped := strings.TrimRight(trimmed, " \t")
			isHeader = strings.HasSuffix(stripped, "**") || strings.HasSuffix(stripped, "**:")
		}
		if isHeader {
			lower := strings.ToLower(trimmed)
			// Keep: Session Intent, Files Modified, Current State
			if strings.Contains(lower, "session intent") ||
				strings.Contains(lower, "files modified") ||
				strings.Contains(lower, "current state") {
				capturing = true
				capturedSections++
				sb.WriteString(line + "\n")
				continue
			}
			// Stop capturing on other headers
			capturing = false
			continue
		}

		if capturing {
			sb.WriteString(line + "\n")
		}
	}

	if capturedSections == 0 {
		return ""
	}

	result := strings.TrimSpace(sb.String())
	// Limit to ~200 tokens (~500 runes)
	return truncate(result, 500)
}

// VFS returns the underlying LocalVFS instance.
func (m *DefaultManager) VFS() *LocalVFS { return m.vfs }

// ============================================================================
// Close
// ============================================================================

func (m *DefaultManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true

	if m.vectorIndex != nil {
		m.vectorIndex.Close()
	}
	m.cache.Clear()
	return m.store.Close()
}

// ============================================================================
// System Entry Operations — unified index/search for sys_* collections
// ============================================================================

// IndexSystemEntry upserts a payload-only entry into a system collection.
// Used for skills, plugins, sessions — data types that don't need embedding vectors.
func (m *DefaultManager) IndexSystemEntry(ctx context.Context, collection, id string, payload map[string]interface{}) error {
	m.mu.RLock()
	vi := m.vectorIndex
	m.mu.RUnlock()

	if vi == nil {
		return fmt.Errorf("uhms: vector index not initialized (required for system entry indexing)")
	}

	// Type assert to access payload-only upsert (bypasses dimension check)
	type payloadUpserter interface {
		UpsertPayload(ctx context.Context, collection, id string, payload map[string]interface{}) error
	}
	if pu, ok := vi.(payloadUpserter); ok {
		return pu.UpsertPayload(ctx, collection, id, payload)
	}

	// Fallback: use zero vector via standard Upsert (requires knowing dimension)
	return fmt.Errorf("uhms: vector index does not support payload-only upsert")
}

// SearchSystemEntries searches a system collection by payload fields.
// Returns hits with payload data including vfs_path for tiered content loading.
func (m *DefaultManager) SearchSystemEntries(ctx context.Context, collection, query string, topK int) ([]PayloadHit, error) {
	m.mu.RLock()
	vi := m.vectorIndex
	m.mu.RUnlock()

	if vi == nil {
		return nil, fmt.Errorf("uhms: vector index not initialized")
	}

	// Type assert to access payload search
	type payloadSearcher interface {
		SearchByPayload(ctx context.Context, collection, query string, topK int) ([]PayloadHit, error)
	}
	if ps, ok := vi.(payloadSearcher); ok {
		return ps.SearchByPayload(ctx, collection, query, topK)
	}

	return nil, fmt.Errorf("uhms: vector index does not support payload search")
}

// DeleteSystemEntry removes an entry from a system collection.
func (m *DefaultManager) DeleteSystemEntry(ctx context.Context, collection, id string) error {
	m.mu.RLock()
	vi := m.vectorIndex
	m.mu.RUnlock()

	if vi == nil {
		return fmt.Errorf("uhms: vector index not initialized")
	}
	return vi.Delete(ctx, collection, id)
}

// ============================================================================
// Internal Helpers
// ============================================================================

func (m *DefaultManager) classifyCategory(ctx context.Context, content string, memType MemoryType) MemoryCategory {
	if m.llm == nil {
		return CatFact // 无 LLM 时默认 fact
	}

	prompt := fmt.Sprintf(`Classify the following text into exactly ONE category.
Categories: preference, habit, profile, skill, relationship, event, opinion, fact, goal, task, reminder, insight, summary

Text: %s

Reply with ONLY the category name, nothing else.`, truncate(content, 500))

	result, err := m.llm.Complete(ctx, ClassifyCategorySystemPrompt, prompt)
	if err != nil {
		return CatFact
	}

	cat := MemoryCategory(strings.TrimSpace(strings.ToLower(result)))
	for _, c := range AllCategories {
		if c == cat {
			return cat
		}
	}
	return CatFact
}

func (m *DefaultManager) estimateImportance(content string, memType MemoryType) float64 {
	base := 0.5
	switch memType {
	case MemTypePermanent:
		base = 0.9
	case MemTypeSemantic:
		base = 0.7
	case MemTypeProcedural:
		base = 0.7
	case MemTypeImagination:
		base = 0.3
	}

	// 内容长度加分 (较长的内容通常更重要)
	runes := []rune(content)
	if len(runes) > 500 {
		base += 0.1
	}
	if base > 1.0 {
		base = 1.0
	}
	return base
}

func (m *DefaultManager) estimateTokens(text string) int {
	if m.llm != nil {
		return m.llm.EstimateTokens(text)
	}
	// Rust FFI (精确 BPE cl100k_base) 或 纯 Go fallback (rune 估算)
	return countTokensBPE(text)
}

func (m *DefaultManager) writeVFS(userID string, mem *Memory, fullContent string) {
	// Phase 1: 同步截断写入 — 记忆立刻可读
	l0 := truncate(fullContent, 200)
	l1 := truncate(fullContent, 2000)
	if err := m.vfs.WriteMemory(userID, mem, l0, l1, fullContent); err != nil {
		slog.Warn("uhms: VFS write failed (non-fatal)", "id", mem.ID, "error", err)
		return
	}

	// Phase 2: 异步 LLM 摘要生成 — 后台覆写 L0/L1 为真正语义摘要
	if m.llm != nil {
		memCopy := *mem // 捕获值副本，避免异步访问竞态
		safeGo("vfs_summarize_"+mem.ID, func() {
			m.upgradeVFSSummary(context.Background(), userID, &memCopy, fullContent)
		})
	}
}

// upgradeVFSSummary 用 LLM 生成真正的 L0/L1 语义摘要，覆写 VFS 中的截断版本。
// 失败时静默降级，保留截断版本。
func (m *DefaultManager) upgradeVFSSummary(ctx context.Context, userID string, mem *Memory, fullContent string) {
	l0, l1 := generateMemorySummary(ctx, m, fullContent, mem.MemoryType, mem.Category)
	if l0 == "" {
		return // LLM 失败，保留截断版本
	}
	if err := m.vfs.WriteL0L1(userID, mem, l0, l1); err != nil {
		slog.Warn("uhms: VFS L0/L1 upgrade failed (non-fatal)", "id", mem.ID, "error", err)
	}
}

// generateMemorySummary 用 LLM 独立生成 L0 abstract 和 L1 overview。
// 参考 OpenViking SemanticProcessor 的 prompt 风格 + 现有 generateArchiveSummary 模式。
func generateMemorySummary(ctx context.Context, m *DefaultManager, content string, memType MemoryType, category MemoryCategory) (l0, l1 string) {
	if m.llm == nil {
		return "", ""
	}

	// L0: ~100 tokens 极短摘要
	l0Prompt := fmt.Sprintf(`Generate an abstract for this %s/%s memory entry.

Requirements:
- Length: 1-2 sentences (under 50 words)
- Explain what this memory is about and its key point
- Include core keywords for searching
- Output plain text directly, no markdown

Content:
%s

Abstract:`, memType, category, truncate(content, 4000))

	l0Result, err := m.llm.Complete(ctx,
		MemorySummaryL0SystemPrompt,
		l0Prompt)
	if err != nil {
		slog.Warn("uhms: L0 summary LLM failed", "error", err)
		return "", ""
	}
	l0 = strings.TrimSpace(l0Result)

	// L1: ~2K tokens 段落概述
	l1Prompt := fmt.Sprintf(`Generate an overview for this %s/%s memory entry.

Requirements:
- Length: one paragraph (100-300 words)
- Include key details, entities, decisions, and context
- Preserve technical terms, file paths, code identifiers
- Explain what it covers and why it matters
- Output plain text directly, no markdown headers

Content:
%s

Overview:`, memType, category, truncate(content, 8000))

	l1Result, err := m.llm.Complete(ctx,
		MemorySummaryL1SystemPrompt,
		l1Prompt)
	if err != nil {
		slog.Warn("uhms: L1 summary LLM failed", "error", err)
		// L0 成功但 L1 失败：仍返回 L0，L1 用截断
		return l0, truncate(content, 2000)
	}
	l1 = strings.TrimSpace(l1Result)

	return l0, l1
}

func (m *DefaultManager) indexVector(ctx context.Context, mem *Memory, content string) {
	if m.embedder == nil || m.vectorIndex == nil {
		return
	}

	vec, err := m.embedder.Embed(ctx, content)
	if err != nil {
		slog.Warn("uhms: embedding failed", "id", mem.ID, "error", err)
		return
	}

	collection := "mem_" + string(mem.MemoryType)
	payload := map[string]interface{}{
		"user_id":  mem.UserID,
		"category": string(mem.Category),
	}
	if err := m.vectorIndex.Upsert(ctx, collection, mem.ID, vec, payload); err != nil {
		slog.Warn("uhms: vector upsert failed", "id", mem.ID, "error", err)
	}
}

func (m *DefaultManager) searchByVector(ctx context.Context, userID, query string, topK int) ([]SearchResult, error) {
	if m.embedder == nil || m.vectorIndex == nil {
		return nil, nil
	}

	vec, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	// 搜索所有记忆类型集合
	var results []SearchResult
	for _, mt := range AllMemoryTypes {
		collection := "mem_" + string(mt)
		hits, err := m.vectorIndex.Search(ctx, collection, vec, topK)
		if err != nil {
			continue
		}
		for _, hit := range hits {
			mem, err := m.store.GetMemory(hit.ID)
			if err != nil || mem.UserID != userID {
				continue
			}
			results = append(results, SearchResult{
				Memory: *mem,
				Score:  hit.Score,
				Source: "vector",
			})
		}
	}
	return results, nil
}

func (m *DefaultManager) summarizeMessages(ctx context.Context, messages []Message, prevSummary string) (string, error) {
	// 优先尝试 Anthropic Compaction API (服务端压缩, 无额外推理成本)
	if m.compactionClient != nil {
		summary, err := m.compactionClient.Compact(ctx, "", messages, m.cfg.CompressionThreshold/2)
		if err != nil {
			slog.Warn("uhms: Anthropic compaction failed, falling back to local summarization", "error", err)
			// Fall through to existing logic below
		} else {
			slog.Debug("uhms: used Anthropic compaction API for summarization")
			return summary, nil
		}
	}

	if m.llm == nil {
		// 无 LLM 时简单截取
		var sb strings.Builder
		for _, msg := range messages {
			sb.WriteString(fmt.Sprintf("[%s] %s\n", msg.Role, truncate(msg.Content, 200)))
		}
		return sb.String(), nil
	}

	// 构建对话文本（限制总量防止超出 LLM 上下文窗口）
	const maxConversationChars = 60000 // ~15K tokens, 留余量给 system prompt + 输出
	var sb strings.Builder
	for _, msg := range messages {
		entry := fmt.Sprintf("[%s]: %s\n\n", msg.Role, truncate(msg.Content, 2000))
		if sb.Len()+len(entry) > maxConversationChars {
			sb.WriteString(fmt.Sprintf("... (%d older messages truncated)\n", len(messages)-countEntries(sb.String())))
			break
		}
		sb.WriteString(entry)
	}

	var prompt string
	if prevSummary != "" {
		// Anchored Iterative: 增量合并到已有摘要
		prompt = fmt.Sprintf(SummarizeAnchoredPromptFmt, prevSummary, sb.String())
	} else {
		// 首次压缩: 使用结构化模板
		prompt = fmt.Sprintf(SummarizeNewPromptFmt, sb.String(), StructuredSummaryTemplate)
	}

	return m.llm.Complete(ctx, SummarizeSystemPrompt, prompt)
}

func (m *DefaultManager) extractAndStoreMemories(ctx context.Context, userID string, messages []Message) {
	if m.llm == nil {
		return
	}

	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	prompt := fmt.Sprintf(`Extract important facts, preferences, and decisions from this conversation.
Return as a JSON array of objects with "content", "type" (episodic/semantic/procedural/permanent), and "category" (preference/habit/profile/skill/fact/event/goal/task/insight).

Conversation:
%s

JSON array:`, truncate(sb.String(), 8000))

	result, err := m.llm.Complete(ctx, ExtractMemoriesSystemPrompt, prompt)
	if err != nil {
		// Warn 而非 Debug: 非关键但生产环境应可见 (参考 Dave Cheney 日志哲学 + slog 级别定义)
		slog.Warn("uhms: memory extraction failed", "error", err, "userID", userID)
		return
	}

	// 解析并存储
	memories := parseExtractedMemories(result)
	for _, em := range memories {
		m.AddMemory(ctx, userID, em.Content, em.MemType, em.Category)
	}
}

func (m *DefaultManager) searchCacheKey(userID, query string, opts SearchOptions) string {
	raw := fmt.Sprintf("uhms|%s|%s|%d|%s|%s|%.2f",
		userID, query, opts.TopK, opts.MemoryType, opts.Category, opts.MinScore)
	h := md5.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ============================================================================
// Utility Functions
// ============================================================================

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func retentionForType(mt MemoryType) RetentionPolicy {
	switch mt {
	case MemTypePermanent:
		return RetentionPermanent
	case MemTypeImagination:
		return RetentionSession
	default:
		return RetentionStandard
	}
}

func mergeSearchResults(a, b []SearchResult, topK int) []SearchResult {
	seen := make(map[string]bool)
	merged := make([]SearchResult, 0, len(a)+len(b))
	for _, r := range a {
		if !seen[r.Memory.ID] {
			seen[r.Memory.ID] = true
			merged = append(merged, r)
		}
	}
	for _, r := range b {
		if !seen[r.Memory.ID] {
			seen[r.Memory.ID] = true
			merged = append(merged, r)
		}
	}
	if len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}

func filterResults(results []SearchResult, opts SearchOptions) []SearchResult {
	if opts.MemoryType == "" && opts.Category == "" && opts.MinScore <= 0 {
		return results
	}
	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if opts.MemoryType != "" && r.Memory.MemoryType != opts.MemoryType {
			continue
		}
		if opts.Category != "" && r.Memory.Category != opts.Category {
			continue
		}
		if opts.MinScore > 0 && r.Score < opts.MinScore {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func extractUserID(messages []Message) string {
	// 从消息元数据中提取 userID (简化版: 返回空)
	return ""
}

func extractQueryFromRecent(messages []Message) string {
	// 从最近消息中提取搜索 query
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && len(messages[i].Content) > 10 {
			return truncate(messages[i].Content, 200)
		}
	}
	return ""
}

type extractedMemory struct {
	Content  string         `json:"content"`
	MemType  MemoryType     `json:"type"`
	Category MemoryCategory `json:"category"`
}

func parseExtractedMemories(jsonStr string) []extractedMemory {
	// 简单解析 JSON 数组 — 容错处理
	jsonStr = strings.TrimSpace(jsonStr)

	// 找到 JSON 数组边界
	start := strings.Index(jsonStr, "[")
	end := strings.LastIndex(jsonStr, "]")
	if start < 0 || end < 0 || end <= start {
		return nil
	}
	jsonStr = jsonStr[start : end+1]

	var memories []extractedMemory
	// 使用 encoding/json 解析
	if err := parseJSONArray(jsonStr, &memories); err != nil {
		return nil
	}
	return memories
}

func countEntries(s string) int {
	return strings.Count(s, "\n\n")
}

func parseJSONArray(jsonStr string, out *[]extractedMemory) error {
	type raw struct {
		Content  string `json:"content"`
		Type     string `json:"type"`
		Category string `json:"category"`
	}
	var raws []raw
	if err := json.Unmarshal([]byte(jsonStr), &raws); err != nil {
		return err
	}
	for _, r := range raws {
		*out = append(*out, extractedMemory{
			Content:  r.Content,
			MemType:  MemoryType(r.Type),
			Category: MemoryCategory(r.Category),
		})
	}
	return nil
}

// ============================================================================
// lastSummary Persistence
// ============================================================================

const lastSummaryFileName = ".last_summary"

// persistLastSummary writes lastSummary to the VFS root (async, non-blocking).
// Failure is non-fatal — next session just starts without anchored context.
func (m *DefaultManager) persistLastSummary(summary string) {
	safeGo("persist_last_summary", func() {
		path := filepath.Join(m.vfs.Root(), lastSummaryFileName)
		if err := os.WriteFile(path, []byte(summary), 0600); err != nil {
			slog.Warn("uhms: persist lastSummary failed (non-fatal)", "error", err)
		}
	})
}

// loadLastSummary restores lastSummary from VFS root at startup.
func (m *DefaultManager) loadLastSummary() {
	path := filepath.Join(m.vfs.Root(), lastSummaryFileName)
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	m.lastSummary = string(data)
	slog.Info("uhms: restored lastSummary from VFS", "length", len(data))
}
