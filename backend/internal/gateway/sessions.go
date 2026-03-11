package gateway

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/session"
)

// ---------- Session 存储 (移植自 sessions.ts / store.ts) ----------

// DefaultSessionStoreTTLMs 默认 session store 缓存 TTL（毫秒）。
// 对齐 TS config/sessions/store.ts: DEFAULT_SESSION_STORE_TTL_MS = 45_000
const DefaultSessionStoreTTLMs = 45_000

// SessionEntry 是 session.SessionEntry 的类型别名。
// Phase 10 Window 4: SessionEntry 已迁移至 internal/session 包以避免循环导入。
type SessionEntry = session.SessionEntry

// SessionOrigin 是 session.SessionOrigin 的类型别名。
type SessionOrigin = session.SessionOrigin

// SessionLastChannel 是 session.SessionLastChannel 的类型别名。
type SessionLastChannel = session.SessionLastChannel

// SessionSkillSnapshot 是 session.SessionSkillSnapshot 的类型别名。
type SessionSkillSnapshot = session.SessionSkillSnapshot

// SessionSkillSnapshotItem 是 session.SessionSkillSnapshotItem 的类型别名。
type SessionSkillSnapshotItem = session.SessionSkillSnapshotItem

// SessionStore 线程安全的会话存储。
// 支持可选的磁盘持久化（storePath 非空时启用）。
// TS 对照: config/sessions/store.ts
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SessionEntry

	// 磁盘持久化（storePath 为空时退化为纯内存模式）
	storePath string // 存储根路径（如 ~/.openacosmi/store）
	filePath  string // sessions.json 完整路径

	// P3 SQLite 后端（非 nil 时所有 CRUD 走 SQLite 路径）
	db *sql.DB

	// TTL 缓存 (对齐 TS SESSION_STORE_CACHE — 45s)
	loadedAt int64 // 上次从磁盘加载的时间 (UnixMilli)
	mtimeMs  int64 // 上次加载时文件的 mtime (UnixMilli)
}

// NewSessionStore 创建会话存储。
// storePath 为空时使用纯内存模式（向后兼容）。
// storePath 非空时启用磁盘持久化，构造时自动 loadFromDisk。
func NewSessionStore(storePath string) *SessionStore {
	s := &SessionStore{
		sessions:  make(map[string]*SessionEntry),
		storePath: storePath,
	}
	if storePath != "" {
		s.filePath = filepath.Join(storePath, "sessions.json")
		s.loadFromDisk()
	}
	return s
}

// ---------- 磁盘 I/O ----------

// loadFromDisk 启动时从磁盘加载 session store。
// TS 对照: store.ts loadSessionStore()
func (s *SessionStore) loadFromDisk() {
	if s.filePath == "" {
		return
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("session_store: no sessions.json found, starting fresh", "path", s.filePath)
			return
		}
		slog.Warn("session_store: failed to read sessions.json", "error", err, "path", s.filePath)
		return
	}

	var loaded map[string]*SessionEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		slog.Warn("session_store: failed to parse sessions.json, starting fresh", "error", err, "path", s.filePath)
		return
	}

	// 遗留字段迁移
	normalizeSessionStore(loaded)

	s.sessions = loaded
	s.loadedAt = time.Now().UnixMilli()
	if info, err := os.Stat(s.filePath); err == nil {
		s.mtimeMs = info.ModTime().UnixMilli()
	}
	slog.Info("session_store: loaded from disk", "path", s.filePath, "count", len(loaded))
}

// saveToDisk 原子写入 session store 到磁盘。
// TS 对照: store.ts saveSessionStore() — 原子写入 (tmp+rename) + 0o600 权限
func (s *SessionStore) saveToDisk() {
	if s.filePath == "" {
		return
	}

	// 确保目录存在
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("session_store: failed to create dir", "error", err, "dir", dir)
		return
	}

	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		slog.Error("session_store: failed to marshal sessions", "error", err)
		return
	}

	// 原子写入: 写 tmp → rename
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		slog.Error("session_store: failed to write tmp", "error", err, "path", tmpPath)
		return
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		slog.Error("session_store: failed to rename tmp", "error", err, "from", tmpPath, "to", s.filePath)
		// 清理 tmp 文件
		os.Remove(tmpPath)
		return
	}
}

// P3: lockFile/unlockFile/lockPath 已删除 — 零调用方，SQLite WAL 模式天然支持并发。

// normalizeSessionStore 加载后迁移遗留字段。
// TS 对照: store.ts normalizeSessionStore() — provider→channel, room→groupChannel
func normalizeSessionStore(sessions map[string]*SessionEntry) {
	for _, entry := range sessions {
		if entry == nil {
			continue
		}
		// 遗留字段迁移: provider → channel (TS store.ts L340-360)
		// 如果 Channel 为空但 LastChannel 有值，填充 Channel
		if entry.Channel == "" && entry.LastChannel != nil && entry.LastChannel.Channel != "" {
			entry.Channel = entry.LastChannel.Channel
		}
	}
}

// ---------- 公开 API ----------

// Save 保存或更新会话条目。
func (s *SessionStore) Save(entry *SessionEntry) {
	if entry == nil || entry.SessionKey == "" {
		return
	}
	s.mu.Lock()
	s.sessions[entry.SessionKey] = entry
	if s.db != nil {
		s.sqliteSaveSession(entry)
	} else {
		s.saveToDisk()
	}
	s.mu.Unlock()
}

// LoadSessionEntry 加载指定 key 的会话条目。
func (s *SessionStore) LoadSessionEntry(sessionKey string) *SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionKey]
}

// ResolveMainSessionKey 解析会话 key 对应的主会话 key。
// 如果条目有 mainKey 则返回 mainKey，否则返回自身。
func (s *SessionStore) ResolveMainSessionKey(sessionKey string) string {
	key := strings.TrimSpace(sessionKey)
	if key == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, exists := s.sessions[key]
	if !exists || entry.MainKey == "" {
		return key
	}
	return entry.MainKey
}

// List 列出所有会话条目。
func (s *SessionStore) List() []*SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*SessionEntry, 0, len(s.sessions))
	for _, e := range s.sessions {
		result = append(result, e)
	}
	return result
}

// Delete 删除会话条目。
func (s *SessionStore) Delete(sessionKey string) {
	s.mu.Lock()
	delete(s.sessions, sessionKey)
	if s.db != nil {
		s.sqliteDeleteSession(sessionKey)
	} else {
		s.saveToDisk()
	}
	s.mu.Unlock()
}

// Count 返回会话数量。
func (s *SessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Reset 清空所有会话（用于测试）。
func (s *SessionStore) Reset() {
	s.mu.Lock()
	s.sessions = make(map[string]*SessionEntry)
	s.loadedAt = 0
	s.mtimeMs = 0
	if s.db != nil {
		s.sqliteReset()
	} else {
		s.saveToDisk()
	}
	s.mu.Unlock()
}

// ---------- TTL 缓存 (对齐 TS session store cache) ----------

// isCacheStale 检查缓存是否过期。
// 对齐 TS isSessionStoreCacheValid(): now - entry.loadedAt <= ttl && mtime 未变。
func (s *SessionStore) isCacheStale() bool {
	if s.filePath == "" {
		return false // 纯内存模式，永不过期
	}
	if s.loadedAt == 0 {
		return true // 从未加载过
	}
	now := time.Now().UnixMilli()
	if now-s.loadedAt > DefaultSessionStoreTTLMs {
		// TTL 过期，检查文件 mtime 是否变化
		info, err := os.Stat(s.filePath)
		if err != nil {
			return true // 文件不存在或无法访问，强制重新加载
		}
		currentMtime := info.ModTime().UnixMilli()
		if currentMtime != s.mtimeMs {
			return true // 文件被外部修改
		}
		// 文件未变，延长 loadedAt
		s.loadedAt = now
		return false
	}
	return false
}

// reloadIfStale 如果缓存过期则从磁盘重新加载。
// 调用方必须持有写锁。
// P3: SQLite 模式下跳过 — SQLite 事务天然保证一致性，无需 TTL 检测。
func (s *SessionStore) reloadIfStale() {
	if s.db != nil {
		return // SQLite 模式: 内存缓存由 write-through 维护，无 stale 概念
	}
	if !s.isCacheStale() {
		return
	}
	s.loadFromDiskLocked()
}

// loadFromDiskLocked 从磁盘重新加载（调用方已持有写锁）。
func (s *SessionStore) loadFromDiskLocked() {
	if s.filePath == "" {
		return
	}
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("session_store: reload failed", "error", err)
		}
		return
	}
	var loaded map[string]*SessionEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		slog.Warn("session_store: reload parse failed", "error", err)
		return
	}
	normalizeSessionStore(loaded)
	s.sessions = loaded
	s.loadedAt = time.Now().UnixMilli()
	if info, err := os.Stat(s.filePath); err == nil {
		s.mtimeMs = info.ModTime().UnixMilli()
	}
}

// ---------- UpdateLastRoute (对齐 TS store.ts updateLastRoute) ----------

// UpdateLastRouteParams UpdateLastRoute 的参数。
// 对齐 TS updateLastRoute params。
type UpdateLastRouteParams struct {
	Channel         string
	To              string
	AccountId       string
	ThreadId        interface{} // string | number | nil
	DeliveryContext *session.DeliveryContext
}

// UpdateLastRoute 更新会话的最后路由信息（3 层合并管线）。
// 对齐 TS config/sessions/store.ts L418-494。
// 层次: explicitContext ← inlineContext → mergedInput ← sessionFallback → merged → normalized
func (s *SessionStore) UpdateLastRoute(sessionKey string, params UpdateLastRouteParams) {
	if sessionKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reloadIfStale()

	entry := s.sessions[sessionKey]
	if entry == nil {
		entry = &SessionEntry{
			SessionKey: sessionKey,
		}
	}

	now := time.Now().UnixMilli()
	if entry.UpdatedAt < now {
		entry.UpdatedAt = now
	}

	// Layer 1: 规范化显式 delivery context
	explicitContext := NormalizeDeliveryContext(params.DeliveryContext)

	// Layer 2: 从内联参数构建上下文
	inlineContext := NormalizeDeliveryContext(&session.DeliveryContext{
		Channel:   params.Channel,
		To:        params.To,
		AccountId: params.AccountId,
		ThreadId:  params.ThreadId,
	})

	// 合并 explicit + inline
	mergedInput := MergeDeliveryContext(explicitContext, inlineContext)

	// 确定是否提供了显式路由（有 channel 或 to）
	explicitRouteProvided := false
	if explicitContext != nil && (explicitContext.Channel != "" || explicitContext.To != "") {
		explicitRouteProvided = true
	}
	if inlineContext != nil && (inlineContext.Channel != "" || inlineContext.To != "") {
		explicitRouteProvided = true
	}

	// 确定显式 threadId 值
	var explicitThreadValue interface{}
	if params.DeliveryContext != nil && params.DeliveryContext.ThreadId != nil {
		explicitThreadValue = params.DeliveryContext.ThreadId
	}
	if explicitThreadValue == nil && params.ThreadId != nil {
		s := fmt.Sprintf("%v", params.ThreadId)
		if s != "" {
			explicitThreadValue = params.ThreadId
		}
	}

	// Layer 3: 从 session 回退（条件移除 thread）
	clearThreadFromFallback := explicitRouteProvided && explicitThreadValue == nil
	var fallbackContext *session.DeliveryContext
	if clearThreadFromFallback {
		fallbackContext = RemoveThreadFromDeliveryContext(DeliveryContextFromSession(entry))
	} else {
		fallbackContext = DeliveryContextFromSession(entry)
	}

	// 最终合并
	merged := MergeDeliveryContext(mergedInput, fallbackContext)

	// 规范化字段
	normalized := ComputeDeliveryFields(merged)

	// 应用
	entry.DeliveryContext = normalized.DeliveryContext
	if normalized.LastChannel != nil {
		entry.LastChannel = normalized.LastChannel
	}
	entry.LastTo = normalized.LastTo
	entry.LastAccountId = normalized.LastAccountId
	entry.LastThreadId = normalized.LastThreadId

	s.sessions[sessionKey] = entry
	if s.db != nil {
		s.sqliteSaveSession(entry)
	} else {
		s.saveToDisk()
	}
}

// ---------- RecordSessionMeta (对齐 TS store.ts recordSessionMetaFromInbound) ----------

// InboundMeta 入站消息元数据。
type InboundMeta struct {
	DisplayName  string // 发送者显示名
	Subject      string // 消息主题
	Channel      string // 消息渠道
	GroupChannel string // 群组频道名
	UserID       string // 用户 ID
}

// RecordSessionMeta 从入站消息记录会话元数据。
// 对齐 TS config/sessions/store.ts recordSessionMetaFromInbound()。
func (s *SessionStore) RecordSessionMeta(sessionKey string, meta InboundMeta) {
	if sessionKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reloadIfStale()

	entry := s.sessions[sessionKey]
	if entry == nil {
		entry = &SessionEntry{
			SessionKey: sessionKey,
			CreatedAt:  time.Now().UnixMilli(),
		}
	}

	now := time.Now().UnixMilli()
	if entry.UpdatedAt < now {
		entry.UpdatedAt = now
	}

	// 合并元数据（非空值覆盖）
	if meta.DisplayName != "" {
		entry.DisplayName = meta.DisplayName
	}
	if meta.Subject != "" {
		entry.Subject = meta.Subject
	}
	if meta.Channel != "" {
		entry.Channel = meta.Channel
	}
	if meta.GroupChannel != "" {
		entry.GroupChannel = meta.GroupChannel
	}

	s.sessions[sessionKey] = entry
	if s.db != nil {
		s.sqliteSaveSession(entry)
	} else {
		s.saveToDisk()
	}
}

// ParseSessionEntry 从 JSON payload 解析会话条目。
func ParseSessionEntry(payloadJSON string) (*SessionEntry, error) {
	if payloadJSON == "" {
		return nil, fmt.Errorf("empty payload")
	}
	var e SessionEntry
	if err := json.Unmarshal([]byte(payloadJSON), &e); err != nil {
		return nil, err
	}
	e.SessionKey = strings.TrimSpace(e.SessionKey)
	if e.SessionKey == "" {
		return nil, fmt.Errorf("empty sessionKey")
	}
	return &e, nil
}

// ---------- Combined Store (对齐 TS session-utils.ts L471-512) ----------

// LoadCombinedStore 合并所有条目为统一的 canonical key 视图。
// Go 是单一扁平 SessionStore，此方法将裸键规范化为 agent: 前缀格式。
// 对齐 TS: session-utils.ts loadCombinedSessionStoreForGateway()
func (s *SessionStore) LoadCombinedStore(defaultAgentId string) map[string]*SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agentId := NormalizeAgentId(defaultAgentId)
	combined := make(map[string]*SessionEntry, len(s.sessions))
	for key, entry := range s.sessions {
		canonicalKey := CanonicalizeSessionKeyForAgent(agentId, key)
		mergeSessionEntryIntoCombined(combined, canonicalKey, entry, agentId)
	}
	return combined
}

// mergeSessionEntryIntoCombined 将条目合并到 combined map，基于 updatedAt 解决冲突。
// 对齐 TS: session-utils.ts mergeSessionEntryIntoCombined()
func mergeSessionEntryIntoCombined(combined map[string]*SessionEntry, canonicalKey string, entry *SessionEntry, agentId string) {
	existing := combined[canonicalKey]
	if existing == nil {
		clone := *entry
		clone.SessionKey = canonicalKey
		clone.SpawnedBy = CanonicalizeSpawnedByForAgent(agentId, entry.SpawnedBy)
		combined[canonicalKey] = &clone
		return
	}

	// 冲突解决: updatedAt 更大的条目优先
	if existing.UpdatedAt > entry.UpdatedAt {
		// existing 更新，保留 existing 的大部分字段
		existing.SpawnedBy = CanonicalizeSpawnedByForAgent(agentId, coalesceStr(existing.SpawnedBy, entry.SpawnedBy))
	} else {
		// entry 更新，用 entry 覆盖
		clone := *entry
		clone.SessionKey = canonicalKey
		clone.SpawnedBy = CanonicalizeSpawnedByForAgent(agentId, coalesceStr(entry.SpawnedBy, existing.SpawnedBy))
		combined[canonicalKey] = &clone
	}
}

// coalesceStr 返回第一个非空字符串。
func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// GatewaySessionStoreTarget 网关 session store 结果。
// 对齐 TS: session-utils.ts resolveGatewaySessionStoreTarget() 返回值
type GatewaySessionStoreTarget struct {
	AgentId      string
	StorePath    string
	CanonicalKey string
	StoreKeys    []string
}
