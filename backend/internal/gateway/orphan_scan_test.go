package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScanOrphanTranscripts_NoOrphans(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0o755)

	store := NewSessionStoreWithSQLite(storeDir)
	defer store.closeDB()

	// 创建一个 session
	store.Save(&SessionEntry{
		SessionKey: "user:sess-1",
		SessionId:  "sess-1",
		Label:      "test",
		CreatedAt:  100,
	})

	// 创建对应的 transcript 文件
	writeMinimalTranscript(t, tmpDir, "sess-1")

	result := ScanOrphanTranscripts(storeDir, store.db)
	if result.ScannedFiles != 1 {
		t.Fatalf("scannedFiles = %d, want 1", result.ScannedFiles)
	}
	if result.IndexedFiles != 1 {
		t.Fatalf("indexedFiles = %d, want 1", result.IndexedFiles)
	}
	if len(result.Orphans) != 0 {
		t.Fatalf("orphans = %d, want 0", len(result.Orphans))
	}
}

func TestScanOrphanTranscripts_FindsOrphans(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0o755)

	store := NewSessionStoreWithSQLite(storeDir)
	defer store.closeDB()

	// 创建一个有索引的 session
	store.Save(&SessionEntry{
		SessionKey: "user:indexed-1",
		SessionId:  "indexed-1",
		CreatedAt:  100,
	})
	writeMinimalTranscript(t, tmpDir, "indexed-1")

	// 创建两个无索引的 transcript（孤儿）
	writeMinimalTranscript(t, tmpDir, "orphan-1")
	writeMinimalTranscript(t, tmpDir, "orphan-2")

	result := ScanOrphanTranscripts(storeDir, store.db)
	if result.ScannedFiles != 3 {
		t.Fatalf("scannedFiles = %d, want 3", result.ScannedFiles)
	}
	if result.IndexedFiles != 1 {
		t.Fatalf("indexedFiles = %d, want 1", result.IndexedFiles)
	}
	if len(result.Orphans) != 2 {
		t.Fatalf("orphans = %d, want 2", len(result.Orphans))
	}

	// 验证孤儿文件信息
	orphanIDs := map[string]bool{}
	for _, o := range result.Orphans {
		orphanIDs[o.SessionID] = true
		if o.SizeBytes == 0 {
			t.Errorf("orphan %s has 0 size", o.SessionID)
		}
		if o.ModTime == 0 {
			t.Errorf("orphan %s has 0 modTime", o.SessionID)
		}
	}
	if !orphanIDs["orphan-1"] || !orphanIDs["orphan-2"] {
		t.Fatalf("expected orphan-1 and orphan-2 in results, got %v", orphanIDs)
	}
}

func TestScanOrphanTranscripts_HeaderFallback(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0o755)

	store := NewSessionStoreWithSQLite(storeDir)
	defer store.closeDB()

	// session 的 sessionId 与文件名不同（通过 header 匹配）
	store.Save(&SessionEntry{
		SessionKey: "user:real-id",
		SessionId:  "real-id",
		CreatedAt:  100,
	})

	// 文件名是 alias-name.jsonl，但 header 中的 sessionId 是 real-id
	writeTranscriptWithHeader(t, tmpDir, "alias-name", "real-id")

	result := ScanOrphanTranscripts(storeDir, store.db)
	if result.ScannedFiles != 1 {
		t.Fatalf("scannedFiles = %d, want 1", result.ScannedFiles)
	}
	// 通过 header fallback 匹配到索引
	if result.IndexedFiles != 1 {
		t.Fatalf("indexedFiles = %d, want 1", result.IndexedFiles)
	}
	if len(result.Orphans) != 0 {
		t.Fatalf("orphans = %d, want 0", len(result.Orphans))
	}
}

func TestScanOrphanTranscripts_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0o755)

	store := NewSessionStoreWithSQLite(storeDir)
	defer store.closeDB()

	result := ScanOrphanTranscripts(storeDir, store.db)
	if result.ScannedFiles != 0 {
		t.Fatalf("scannedFiles = %d, want 0", result.ScannedFiles)
	}
}

func TestScanOrphanTranscripts_NilDB(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0o755)

	writeMinimalTranscript(t, tmpDir, "some-session")

	// db=nil → 所有文件都是孤儿
	result := ScanOrphanTranscripts(storeDir, nil)
	if result.ScannedFiles != 1 {
		t.Fatalf("scannedFiles = %d, want 1", result.ScannedFiles)
	}
	if len(result.Orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(result.Orphans))
	}
}

func TestRebuildOrphanIndex(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0o755)

	store := NewSessionStoreWithSQLite(storeDir)
	defer store.closeDB()

	orphans := []OrphanTranscript{
		{FilePath: filepath.Join(tmpDir, "orphan-a.jsonl"), SessionID: "orphan-a", ModTime: 12345},
		{FilePath: filepath.Join(tmpDir, "orphan-b.jsonl"), SessionID: "orphan-b", ModTime: 67890},
	}

	rebuilt := RebuildOrphanIndex(orphans, store)
	if rebuilt != 2 {
		t.Fatalf("rebuilt = %d, want 2", rebuilt)
	}

	// 验证索引已创建
	entry := store.LoadSessionEntry("user:orphan-a")
	if entry == nil {
		t.Fatal("expected entry for orphan-a")
	}
	if entry.Label != "recovered (orphan scan)" {
		t.Fatalf("label = %q", entry.Label)
	}
	if entry.CreatedAt != 12345 {
		t.Fatalf("createdAt = %d, want 12345", entry.CreatedAt)
	}

	// 再次 rebuild 应跳过已存在的
	rebuilt2 := RebuildOrphanIndex(orphans, store)
	if rebuilt2 != 0 {
		t.Fatalf("second rebuild = %d, want 0 (idempotent)", rebuilt2)
	}
}

// ---------- helpers ----------

func writeMinimalTranscript(t *testing.T, dir, sessionID string) {
	t.Helper()
	header := map[string]interface{}{
		"type":      "session_start",
		"sessionId": sessionID,
		"version":   "1.0",
	}
	data, _ := json.Marshal(header)
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write transcript %s: %v", path, err)
	}
}

func writeTranscriptWithHeader(t *testing.T, dir, fileName, headerSessionID string) {
	t.Helper()
	header := map[string]interface{}{
		"type":      "session_start",
		"sessionId": headerSessionID,
		"version":   "1.0",
	}
	data, _ := json.Marshal(header)
	path := filepath.Join(dir, fileName+".jsonl")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write transcript %s: %v", path, err)
	}
}
