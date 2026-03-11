package gateway

// orphan_scan.go — P3-6 孤儿 transcript 扫描工具
//
// 扫描 transcript 目录中的 .jsonl 文件，对比 SQLite sessions 表，
// 识别没有对应 session 索引的孤儿 transcript 文件。
//
// 用途:
// - 诊断 session 索引损坏后的数据恢复
// - 验证 JSON→SQLite 迁移完整性
// - 日常维护：清理或重建无主 transcript 的索引

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OrphanTranscript 表示没有对应 session 索引的 transcript 文件。
type OrphanTranscript struct {
	FilePath  string `json:"filePath"`
	SessionID string `json:"sessionId"` // 从文件名或 header 提取的 sessionId
	SizeBytes int64  `json:"sizeBytes"`
	ModTime   int64  `json:"modTime"` // UnixMilli
}

// OrphanScanResult 扫描结果。
type OrphanScanResult struct {
	ScannedFiles int                `json:"scannedFiles"`
	IndexedFiles int                `json:"indexedFiles"`
	Orphans      []OrphanTranscript `json:"orphans"`
	ScanDuration int64              `json:"scanDurationMs"`
}

// ScanOrphanTranscripts 扫描 transcript 目录，找出没有 session 索引的孤儿文件。
// storePath 是 store 目录路径（如 ~/.openacosmi/store）。
// transcript 文件存储在 storePath 的父目录。
func ScanOrphanTranscripts(storePath string, db *sql.DB) *OrphanScanResult {
	start := time.Now()
	result := &OrphanScanResult{}

	if storePath == "" {
		return result
	}

	transcriptDir := filepath.Dir(storePath)
	if transcriptDir == "" || transcriptDir == "." {
		return result
	}

	// 从 SQLite 加载所有已索引的 sessionId 集合
	indexedIDs := loadIndexedSessionIDs(db)

	// 扫描 .jsonl 文件
	entries, err := os.ReadDir(transcriptDir)
	if err != nil {
		slog.Warn("orphan_scan: read dir failed", "dir", transcriptDir, "error", err)
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		result.ScannedFiles++

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if sessionID == "" {
			continue
		}

		if indexedIDs[sessionID] {
			result.IndexedFiles++
			continue
		}

		// 尝试从文件 header 确认 sessionId
		fullPath := filepath.Join(transcriptDir, entry.Name())
		headerID := readTranscriptHeaderSessionID(fullPath)
		if headerID != "" && indexedIDs[headerID] {
			result.IndexedFiles++
			continue
		}

		// 确认为孤儿
		info, _ := entry.Info()
		orphan := OrphanTranscript{
			FilePath:  fullPath,
			SessionID: sessionID,
		}
		if info != nil {
			orphan.SizeBytes = info.Size()
			orphan.ModTime = info.ModTime().UnixMilli()
		}
		result.Orphans = append(result.Orphans, orphan)
	}

	result.ScanDuration = time.Since(start).Milliseconds()
	return result
}

// loadIndexedSessionIDs 从 SQLite 读取所有已索引的 sessionId。
// 如果 db 为 nil，返回空 map。
func loadIndexedSessionIDs(db *sql.DB) map[string]bool {
	ids := make(map[string]bool)
	if db == nil {
		return ids
	}

	rows, err := db.Query(`SELECT session_id FROM sessions WHERE session_id != ''`)
	if err != nil {
		slog.Warn("orphan_scan: query sessions failed", "error", err)
		return ids
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			ids[id] = true
		}
	}
	return ids
}

// readTranscriptHeaderSessionID 从 transcript 文件的 header 行提取 sessionId。
// 格式: {"type":"session_start","sessionId":"..."}
func readTranscriptHeaderSessionID(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return ""
	}
	line := scanner.Text()

	var header struct {
		Type      string `json:"type"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(line), &header); err != nil {
		return ""
	}
	return header.SessionID
}

// RebuildOrphanIndex 为孤儿 transcript 文件重建 session 索引。
// 仅创建最小化的 SessionEntry（sessionKey=user:{sessionId}, channel=unknown）。
// 返回成功重建的数量。
func RebuildOrphanIndex(orphans []OrphanTranscript, store *SessionStore) int {
	if store == nil || len(orphans) == 0 {
		return 0
	}

	rebuilt := 0
	for _, o := range orphans {
		if o.SessionID == "" {
			continue
		}

		sessionKey := "user:" + o.SessionID
		if existing := store.LoadSessionEntry(sessionKey); existing != nil {
			continue // 已存在
		}

		now := time.Now().UnixMilli()
		createdAt := o.ModTime
		if createdAt == 0 {
			createdAt = now
		}

		entry := &SessionEntry{
			SessionKey: sessionKey,
			SessionId:  o.SessionID,
			Label:      "recovered (orphan scan)",
			Channel:    "unknown",
			CreatedAt:  createdAt,
			UpdatedAt:  now,
		}
		store.Save(entry)
		rebuilt++
		slog.Info("orphan_scan: rebuilt index", "sessionKey", sessionKey, "file", o.FilePath)
	}
	return rebuilt
}
