package gateway

// store_sqlite.go — P3 存储收敛: sessions.json → SQLite
//
// SQLite 后端为 SessionStore 提供行级事务存储，替代 JSON 整表写回。
// 使用 WAL 模式支持并发读写，无需文件锁或 TTL 缓存。
//
// 架构模式: SessionStore 持有 *sql.DB，非 nil 时所有 CRUD 走 SQLite 路径。
// 纯内存模式（storePath=""，用于测试）不使用 SQLite。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/session"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// ---------- Schema ----------

const sessionStoreSchemaVersion = 1

const sessionStoreSchema = `
CREATE TABLE IF NOT EXISTS sessions (
	session_key  TEXT PRIMARY KEY,
	session_id   TEXT NOT NULL DEFAULT '',
	label        TEXT NOT NULL DEFAULT '',
	channel      TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL DEFAULT 0,
	updated_at   INTEGER NOT NULL DEFAULT 0,
	meta_json    TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS runs (
	run_id       TEXT PRIMARY KEY,
	session_key  TEXT NOT NULL DEFAULT '',
	label        TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'queued',
	summary      TEXT NOT NULL DEFAULT '',
	error        TEXT NOT NULL DEFAULT '',
	async        INTEGER NOT NULL DEFAULT 0,
	created_at   INTEGER NOT NULL DEFAULT 0,
	updated_at   INTEGER NOT NULL DEFAULT 0,
	meta_json    TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS schema_version (
	id      INTEGER PRIMARY KEY CHECK (id = 1),
	version INTEGER NOT NULL
);
`

// ---------- Open / Init ----------

// openSessionDB opens (or creates) the SQLite database at the given directory.
// Returns nil if storePath is empty.
func openSessionDB(storePath string) (*sql.DB, error) {
	if storePath == "" {
		return nil, nil
	}

	if err := os.MkdirAll(storePath, 0o700); err != nil {
		return nil, fmt.Errorf("session_sqlite: mkdir %s: %w", storePath, err)
	}

	dbPath := filepath.Join(storePath, "sessions.db")
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("session_sqlite: open %s: %w", dbPath, err)
	}

	// SQLite 单写多读
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("session_sqlite: ping: %w", err)
	}

	if err := ensureSessionSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("session_sqlite: schema: %w", err)
	}

	return db, nil
}

func ensureSessionSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 执行 schema DDL（幂等）
	for _, stmt := range strings.Split(sessionStoreSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", truncateStr(stmt, 80), err)
		}
	}

	// 记录 schema version
	_, err = tx.Exec(`INSERT OR REPLACE INTO schema_version (id, version) VALUES (1, ?)`, sessionStoreSchemaVersion)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ---------- SessionStore SQLite CRUD ----------

// sqliteSaveSession 写入单个 session 到 SQLite。
func (s *SessionStore) sqliteSaveSession(entry *session.SessionEntry) {
	if s.db == nil || entry == nil {
		return
	}

	metaJSON := marshalSessionMeta(entry)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO sessions (session_key, session_id, label, channel, created_at, updated_at, meta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.SessionKey,
		entry.SessionId,
		entry.Label,
		entry.Channel,
		entry.CreatedAt,
		entry.UpdatedAt,
		metaJSON,
	)
	if err != nil {
		slog.Error("session_sqlite: save failed", "key", entry.SessionKey, "error", err)
	}
}

// sqliteLoadSession 从 SQLite 读取单个 session。
func (s *SessionStore) sqliteLoadSession(key string) *session.SessionEntry {
	if s.db == nil {
		return nil
	}

	var entry session.SessionEntry
	var metaJSON string
	err := s.db.QueryRow(
		`SELECT session_key, session_id, label, channel, created_at, updated_at, meta_json FROM sessions WHERE session_key = ?`,
		key,
	).Scan(&entry.SessionKey, &entry.SessionId, &entry.Label, &entry.Channel, &entry.CreatedAt, &entry.UpdatedAt, &metaJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		slog.Error("session_sqlite: load failed", "key", key, "error", err)
		return nil
	}

	unmarshalSessionMeta(&entry, metaJSON)
	return &entry
}

// sqliteListSessions 列出所有 session。
func (s *SessionStore) sqliteListSessions() []*session.SessionEntry {
	if s.db == nil {
		return nil
	}

	rows, err := s.db.Query(`SELECT session_key, session_id, label, channel, created_at, updated_at, meta_json FROM sessions`)
	if err != nil {
		slog.Error("session_sqlite: list failed", "error", err)
		return nil
	}
	defer rows.Close()

	var result []*session.SessionEntry
	for rows.Next() {
		var entry session.SessionEntry
		var metaJSON string
		if err := rows.Scan(&entry.SessionKey, &entry.SessionId, &entry.Label, &entry.Channel, &entry.CreatedAt, &entry.UpdatedAt, &metaJSON); err != nil {
			slog.Error("session_sqlite: scan failed", "error", err)
			continue
		}
		unmarshalSessionMeta(&entry, metaJSON)
		result = append(result, &entry)
	}
	return result
}

// sqliteDeleteSession 从 SQLite 删除单个 session。
func (s *SessionStore) sqliteDeleteSession(key string) {
	if s.db == nil {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE session_key = ?`, key); err != nil {
		slog.Error("session_sqlite: delete failed", "key", key, "error", err)
	}
}

// sqliteCount 返回 session 数量。
func (s *SessionStore) sqliteCount() int {
	if s.db == nil {
		return 0
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count); err != nil {
		slog.Error("session_sqlite: count failed", "error", err)
		return 0
	}
	return count
}

// sqliteReset 清空所有 session（用于测试）。
func (s *SessionStore) sqliteReset() {
	if s.db == nil {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM sessions`); err != nil {
		slog.Error("session_sqlite: reset failed", "error", err)
	}
}

// ---------- Meta JSON 序列化 ----------

// marshalSessionMeta 将 SessionEntry 的扩展字段序列化为 JSON。
// 核心字段 (session_key, session_id, label, channel, created_at, updated_at) 存储在列中，
// 其他字段存储在 meta_json 中。
func marshalSessionMeta(entry *session.SessionEntry) string {
	// 创建一个临时副本，清除列存储的字段以减小 meta_json 体积
	type metaOnly session.SessionEntry
	meta := metaOnly(*entry)
	meta.SessionKey = ""
	meta.SessionId = ""
	meta.Label = ""
	meta.Channel = ""
	meta.CreatedAt = 0
	meta.UpdatedAt = 0

	data, err := json.Marshal(meta)
	if err != nil {
		slog.Error("session_sqlite: marshal meta failed", "key", entry.SessionKey, "error", err)
		return "{}"
	}
	return string(data)
}

// unmarshalSessionMeta 从 meta_json 恢复 SessionEntry 的扩展字段。
func unmarshalSessionMeta(entry *session.SessionEntry, metaJSON string) {
	if metaJSON == "" || metaJSON == "{}" {
		return
	}
	// 保留列字段
	key := entry.SessionKey
	id := entry.SessionId
	label := entry.Label
	channel := entry.Channel
	createdAt := entry.CreatedAt
	updatedAt := entry.UpdatedAt

	if err := json.Unmarshal([]byte(metaJSON), entry); err != nil {
		slog.Warn("session_sqlite: unmarshal meta failed", "key", key, "error", err)
		// 恢复列字段
		entry.SessionKey = key
		entry.SessionId = id
		entry.Label = label
		entry.Channel = channel
		entry.CreatedAt = createdAt
		entry.UpdatedAt = updatedAt
		return
	}

	// 恢复列字段（列字段优先于 meta_json）
	entry.SessionKey = key
	entry.SessionId = id
	if label != "" {
		entry.Label = label
	}
	if channel != "" {
		entry.Channel = channel
	}
	if createdAt > 0 {
		entry.CreatedAt = createdAt
	}
	if updatedAt > 0 {
		entry.UpdatedAt = updatedAt
	}
}

// ---------- Migration ----------

// migrateJSONToSQLite 从 sessions.json 导入数据到 SQLite。
// 成功后将 sessions.json 重命名为 sessions.json.migrated。
func migrateJSONToSQLite(db *sql.DB, storePath string) error {
	jsonPath := filepath.Join(storePath, "sessions.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 无 JSON 文件，跳过迁移
		}
		return fmt.Errorf("read sessions.json: %w", err)
	}

	var loaded map[string]*session.SessionEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parse sessions.json: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO sessions (session_key, session_id, label, channel, created_at, updated_at, meta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	runStmt, err := tx.Prepare(`INSERT OR IGNORE INTO runs (run_id, session_key, label, status, summary, error, async, created_at, updated_at, meta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer runStmt.Close()

	var sessionCount, runCount int
	for key, entry := range loaded {
		if entry == nil {
			continue
		}
		entry.SessionKey = key

		// task: 前缀条目 → runs 表
		if IsTaskSessionKey(key) {
			taskId := strings.TrimPrefix(key, "task:")
			status := ""
			summary := ""
			errStr := ""
			async := false
			var metaJSON string
			if entry.TaskMeta != nil {
				status = entry.TaskMeta.Status
				summary = entry.TaskMeta.Summary
				errStr = entry.TaskMeta.Error
				async = entry.TaskMeta.Async
				if metaBytes, err := json.Marshal(entry.TaskMeta); err == nil {
					metaJSON = string(metaBytes)
				}
			}
			if status == "" {
				status = "unknown"
			}
			asyncInt := 0
			if async {
				asyncInt = 1
			}
			_, err := runStmt.Exec(taskId, key, entry.Label, status, summary, errStr, asyncInt, entry.CreatedAt, entry.UpdatedAt, metaJSON)
			if err != nil {
				slog.Warn("session_sqlite: migrate run failed", "taskId", taskId, "error", err)
			} else {
				runCount++
			}
			continue
		}

		// 普通 session → sessions 表
		metaJSON := marshalSessionMeta(entry)
		_, err := stmt.Exec(entry.SessionKey, entry.SessionId, entry.Label, entry.Channel, entry.CreatedAt, entry.UpdatedAt, metaJSON)
		if err != nil {
			slog.Warn("session_sqlite: migrate session failed", "key", key, "error", err)
		} else {
			sessionCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	// 重命名 sessions.json → sessions.json.migrated
	migratedPath := jsonPath + ".migrated"
	if err := os.Rename(jsonPath, migratedPath); err != nil {
		slog.Warn("session_sqlite: rename sessions.json failed", "error", err)
		// 不影响迁移成功
	}

	slog.Info("session_sqlite: migration complete",
		"sessions", sessionCount,
		"runs", runCount,
		"backup", migratedPath,
	)
	return nil
}

// ---------- Close ----------

// closeDB closes the SQLite database connection.
func (s *SessionStore) closeDB() {
	if s.db != nil {
		s.db.Close()
		s.db = nil
	}
}

// ---------- TaskStore SQLite (P3: runs 表统一) ----------
// 预留: 未来 TaskStore 也将使用同一 SQLite 数据库。
// 当前 TaskStore 仍使用独立 tasks.json，P3 完成后可合并。

// sqliteSaveRun 保存 run 到 SQLite runs 表。
func sqliteSaveRun(db *sql.DB, entry *TaskEntry) {
	if db == nil || entry == nil {
		return
	}
	var metaJSON string
	if entry.TaskMeta != nil {
		if data, err := json.Marshal(entry.TaskMeta); err == nil {
			metaJSON = string(data)
		}
	}
	asyncInt := 0
	if entry.Async {
		asyncInt = 1
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO runs (run_id, session_key, label, status, summary, error, async, created_at, updated_at, meta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.TaskID, entry.SessionKey, entry.Label, entry.Status, entry.Summary, entry.Error, asyncInt,
		entry.CreatedAt, entry.UpdatedAt, metaJSON,
	)
	if err != nil {
		slog.Error("session_sqlite: save run failed", "taskId", entry.TaskID, "error", err)
	}
}

// sqliteLoadRun 从 SQLite runs 表读取单个 run。
func sqliteLoadRun(db *sql.DB, taskId string) *TaskEntry {
	if db == nil {
		return nil
	}
	var entry TaskEntry
	var metaJSON string
	var asyncInt int
	err := db.QueryRow(
		`SELECT run_id, session_key, label, status, summary, error, async, created_at, updated_at, meta_json FROM runs WHERE run_id = ?`,
		taskId,
	).Scan(&entry.TaskID, &entry.SessionKey, &entry.Label, &entry.Status, &entry.Summary, &entry.Error, &asyncInt, &entry.CreatedAt, &entry.UpdatedAt, &metaJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		slog.Error("session_sqlite: load run failed", "taskId", taskId, "error", err)
		return nil
	}
	entry.Async = asyncInt != 0
	if metaJSON != "" {
		var tm session.TaskMeta
		if err := json.Unmarshal([]byte(metaJSON), &tm); err == nil {
			entry.TaskMeta = &tm
		}
	}
	return &entry
}

// NewSessionStoreWithSQLite 创建使用 SQLite 后端的会话存储。
// 如果 sessions.json 存在，自动执行迁移。
func NewSessionStoreWithSQLite(storePath string) *SessionStore {
	s := &SessionStore{
		sessions:  make(map[string]*SessionEntry),
		storePath: storePath,
	}
	if storePath == "" {
		return s
	}

	db, err := openSessionDB(storePath)
	if err != nil {
		slog.Error("session_sqlite: failed to open, falling back to JSON", "error", err)
		// 降级到 JSON 模式（lockPath 不再赋值 — lockFile/unlockFile 已在 P3-5 删除）
		s.filePath = filepath.Join(storePath, "sessions.json")
		s.loadFromDisk()
		return s
	}

	s.db = db

	// 自动迁移 sessions.json → SQLite
	jsonPath := filepath.Join(storePath, "sessions.json")
	if _, err := os.Stat(jsonPath); err == nil {
		if err := migrateJSONToSQLite(db, storePath); err != nil {
			slog.Error("session_sqlite: migration failed", "error", err)
		}
	}

	// 预加载到内存缓存（保持与 JSON 模式相同的读取行为）
	s.loadFromSQLite()

	slog.Info("session_sqlite: initialized", "path", filepath.Join(storePath, "sessions.db"), "count", len(s.sessions))
	return s
}

// loadFromSQLite 从 SQLite 加载所有 session 到内存缓存。
func (s *SessionStore) loadFromSQLite() {
	entries := s.sqliteListSessions()
	s.sessions = make(map[string]*SessionEntry, len(entries))
	for _, entry := range entries {
		s.sessions[entry.SessionKey] = entry
	}
	s.loadedAt = time.Now().UnixMilli()
}
