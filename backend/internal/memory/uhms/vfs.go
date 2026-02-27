package uhms

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LocalVFS manages memory content as L0/L1/L2 files on the local filesystem.
//
// Directory layout:
//
//	{vfsRoot}/{userID}/{memoryType}/{category}/{memoryID}/
//	  ├── l0.txt       ← ~100 tokens abstract (1-2 sentences)
//	  ├── l1.txt       ← ~2K tokens overview (paragraph)
//	  ├── l2.txt       ← full content (unlimited)
//	  └── meta.json    ← { memory_id, memory_type, category, created_at }
//
//	{vfsRoot}/{userID}/archives/{sessionKey}/
//	  ├── l0.txt       ← session summary
//	  ├── l1.txt       ← session overview
//	  └── l2.txt       ← full conversation transcript
type LocalVFS struct {
	root string
	mu   sync.RWMutex // 保护并发文件操作
}

// NewLocalVFS creates a local VFS rooted at the given path.
func NewLocalVFS(vfsRoot string) (*LocalVFS, error) {
	vfsRoot = expandHome(vfsRoot)
	if err := os.MkdirAll(vfsRoot, 0700); err != nil {
		return nil, fmt.Errorf("uhms/vfs: create root %s: %w", vfsRoot, err)
	}
	slog.Info("uhms/vfs: initialized", "root", vfsRoot)
	return &LocalVFS{root: vfsRoot}, nil
}

// Root returns the VFS root path.
func (v *LocalVFS) Root() string { return v.root }

// ============================================================================
// Write Operations
// ============================================================================

// WriteMemory writes a memory's L0/L1/L2 content to the file system.
func (v *LocalVFS) WriteMemory(userID string, m *Memory, l0Abstract, l1Overview, l2Detail string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := v.memoryDir(userID, string(m.MemoryType), string(m.Category), m.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("uhms/vfs: mkdir %s: %w", dir, err)
	}

	// 写入 L0/L1/L2 文件
	if err := writeFile(filepath.Join(dir, "l0.txt"), l0Abstract); err != nil {
		return err
	}
	if l1Overview != "" {
		if err := writeFile(filepath.Join(dir, "l1.txt"), l1Overview); err != nil {
			return err
		}
	}
	if l2Detail != "" {
		if err := writeFile(filepath.Join(dir, "l2.txt"), l2Detail); err != nil {
			return err
		}
	}

	// 写入 meta.json
	meta := map[string]interface{}{
		"memory_id":   m.ID,
		"memory_type": m.MemoryType,
		"category":    m.Category,
		"created_at":  m.CreatedAt.Unix(),
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("uhms/vfs: marshal meta: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "meta.json"), string(metaBytes)); err != nil {
		return err
	}

	// 更新 Memory 的 VFSPath
	m.VFSPath = v.relativeMemoryPath(userID, string(m.MemoryType), string(m.Category), m.ID)
	return nil
}

// WriteArchive writes a session archive's L0/L1/L2 to the file system.
// l2Transcript is the full conversation text; truncated to maxArchiveL2Bytes if too large.
func (v *LocalVFS) WriteArchive(userID, sessionKey, l0Summary, l1Overview, l2Transcript string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := v.archiveDir(userID, sessionKey)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("uhms/vfs: mkdir archive %s: %w", dir, err)
	}

	if err := writeFile(filepath.Join(dir, "l0.txt"), l0Summary); err != nil {
		return "", err
	}
	if l1Overview != "" {
		if err := writeFile(filepath.Join(dir, "l1.txt"), l1Overview); err != nil {
			return "", err
		}
	}
	if l2Transcript != "" {
		// Truncate L2 to 200KB (~50K tokens) to prevent unbounded disk usage.
		// Use rune-aware truncation to avoid splitting multi-byte UTF-8 characters.
		if len(l2Transcript) > maxArchiveL2Bytes {
			runes := []rune(l2Transcript)
			// Find rune count that fits within byte limit
			byteCount := 0
			cutIdx := 0
			for i, r := range runes {
				byteCount += len(string(r))
				if byteCount > maxArchiveL2Bytes {
					cutIdx = i
					break
				}
			}
			if cutIdx > 0 {
				l2Transcript = string(runes[:cutIdx]) + "\n[Transcript truncated at 200KB]"
			}
		}
		if err := writeFile(filepath.Join(dir, "l2.txt"), l2Transcript); err != nil {
			return "", err
		}
	}

	return v.relativeArchivePath(userID, sessionKey), nil
}

// WriteL0L1 overwrites only the L0 abstract and L1 overview files for a memory.
// L2 full content and meta.json are left untouched.
// Used by async LLM summary upgrade to replace truncated placeholders with real summaries.
func (v *LocalVFS) WriteL0L1(userID string, m *Memory, l0Abstract, l1Overview string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := v.memoryDir(userID, string(m.MemoryType), string(m.Category), m.ID)

	if err := writeFile(filepath.Join(dir, "l0.txt"), l0Abstract); err != nil {
		return fmt.Errorf("write l0: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "l1.txt"), l1Overview); err != nil {
		return fmt.Errorf("write l1: %w", err)
	}
	return nil
}

// DeleteMemory removes a memory's VFS directory.
func (v *LocalVFS) DeleteMemory(userID, memoryType, category, memoryID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := v.memoryDir(userID, memoryType, category, memoryID)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("uhms/vfs: remove %s: %w", dir, err)
	}
	return nil
}

// ============================================================================
// Read Operations — Tiered Loading
// ============================================================================

// ReadL0 reads the L0 abstract (~100 tokens) for a memory.
func (v *LocalVFS) ReadL0(userID, memoryType, category, memoryID string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return readFile(filepath.Join(v.memoryDir(userID, memoryType, category, memoryID), "l0.txt"))
}

// ReadL1 reads the L1 overview (~2K tokens) for a memory.
func (v *LocalVFS) ReadL1(userID, memoryType, category, memoryID string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return readFile(filepath.Join(v.memoryDir(userID, memoryType, category, memoryID), "l1.txt"))
}

// ReadL2 reads the L2 full detail for a memory.
func (v *LocalVFS) ReadL2(userID, memoryType, category, memoryID string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return readFile(filepath.Join(v.memoryDir(userID, memoryType, category, memoryID), "l2.txt"))
}

// ReadByVFSPath reads L0/L1/L2 by a relative VFS path (stored in Memory.VFSPath).
func (v *LocalVFS) ReadByVFSPath(vfsPath string, level int) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	dir := filepath.Join(v.root, vfsPath)
	fileName := "l0.txt"
	switch level {
	case 1:
		fileName = "l1.txt"
	case 2:
		fileName = "l2.txt"
	}
	return readFile(filepath.Join(dir, fileName))
}

// BatchReadL0 reads L0 abstracts for multiple memories, returning entries sorted by created_at desc.
func (v *LocalVFS) BatchReadL0(userID string, memories []Memory) []L0Entry {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entries := make([]L0Entry, 0, len(memories))
	for _, m := range memories {
		dir := v.memoryDir(userID, string(m.MemoryType), string(m.Category), m.ID)
		abstract, err := readFile(filepath.Join(dir, "l0.txt"))
		if err != nil {
			// 如果 L0 不存在，用 Content 字段截取前 200 字符作兜底
			abstract = truncate(m.Content, 200)
		}
		entries = append(entries, L0Entry{
			MemoryID:   m.ID,
			MemoryType: m.MemoryType,
			Category:   m.Category,
			Abstract:   abstract,
			CreatedAt:  m.CreatedAt,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	return entries
}

// BatchReadL1 reads L1 overviews for a set of memory IDs.
func (v *LocalVFS) BatchReadL1(userID string, memories []Memory) []L1Entry {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entries := make([]L1Entry, 0, len(memories))
	for _, m := range memories {
		dir := v.memoryDir(userID, string(m.MemoryType), string(m.Category), m.ID)
		abstract, _ := readFile(filepath.Join(dir, "l0.txt"))
		if abstract == "" {
			abstract = truncate(m.Content, 200)
		}
		overview, _ := readFile(filepath.Join(dir, "l1.txt"))
		if overview == "" {
			overview = truncate(m.Content, 2000)
		}
		entries = append(entries, L1Entry{
			L0Entry: L0Entry{
				MemoryID:   m.ID,
				MemoryType: m.MemoryType,
				Category:   m.Category,
				Abstract:   abstract,
				CreatedAt:  m.CreatedAt,
			},
			Overview: overview,
		})
	}
	return entries
}

// ============================================================================
// Browse / List Operations
// ============================================================================

// VFSDirEntry represents a directory entry in the VFS hierarchy.
type VFSDirEntry struct {
	Name       string `json:"name"`
	IsDir      bool   `json:"is_dir"`
	L0Abstract string `json:"l0_abstract,omitempty"`
	CreatedAt  int64  `json:"created_at,omitempty"` // Unix timestamp
}

// ListCategories lists all memory categories under a memory type for a user.
func (v *LocalVFS) ListCategories(userID, memoryType string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	typeDir := filepath.Join(v.root, userID, memoryType)
	entries, err := os.ReadDir(typeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("uhms/vfs: list categories %s: %w", typeDir, err)
	}

	categories := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			categories = append(categories, e.Name())
		}
	}
	return categories, nil
}

// ListMemoryIDs lists all memory IDs under a (user, type, category).
func (v *LocalVFS) ListMemoryIDs(userID, memoryType, category string) ([]VFSDirEntry, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	catDir := filepath.Join(v.root, userID, memoryType, category)
	entries, err := os.ReadDir(catDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("uhms/vfs: list memories %s: %w", catDir, err)
	}

	results := make([]VFSDirEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(catDir, e.Name())
		abstract, _ := readFile(filepath.Join(memDir, "l0.txt"))
		var createdAt int64
		if meta := readMeta(filepath.Join(memDir, "meta.json")); meta != nil {
			if ts, ok := meta["created_at"].(float64); ok {
				createdAt = int64(ts)
			}
		}
		results = append(results, VFSDirEntry{
			Name:       e.Name(),
			IsDir:      true,
			L0Abstract: abstract,
			CreatedAt:  createdAt,
		})
	}
	return results, nil
}

// DiskUsage returns the total VFS disk usage in bytes for a user.
func (v *LocalVFS) DiskUsage(userID string) (int64, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var totalSize int64
	userDir := filepath.Join(v.root, userID)
	err := filepath.Walk(userDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return totalSize, nil
}

// ============================================================================
// System Entry Operations — _system/ namespace
// ============================================================================

// WriteSystemEntry writes a system entry's L0/L1/L2 + meta.json to _system/{namespace}/{category}/{id}/.
// Used for skills, plugins, sessions — shared system-level data isolated from user memories.
func (v *LocalVFS) WriteSystemEntry(namespace, category, id, l0, l1, l2 string, meta map[string]interface{}) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := v.systemDir(namespace, category, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("uhms/vfs: mkdir system %s: %w", dir, err)
	}

	if err := writeFile(filepath.Join(dir, "l0.txt"), l0); err != nil {
		return err
	}
	if l1 != "" {
		if err := writeFile(filepath.Join(dir, "l1.txt"), l1); err != nil {
			return err
		}
	}
	if l2 != "" {
		if err := writeFile(filepath.Join(dir, "l2.txt"), l2); err != nil {
			return err
		}
	}

	if len(meta) > 0 {
		metaBytes, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return fmt.Errorf("uhms/vfs: marshal system meta: %w", err)
		}
		if err := writeFile(filepath.Join(dir, "meta.json"), string(metaBytes)); err != nil {
			return err
		}
	}
	return nil
}

// ReadSystemL0 reads the L0 abstract for a system entry.
func (v *LocalVFS) ReadSystemL0(namespace, category, id string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return readFile(filepath.Join(v.systemDir(namespace, category, id), "l0.txt"))
}

// ReadSystemL1 reads the L1 overview for a system entry.
func (v *LocalVFS) ReadSystemL1(namespace, category, id string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return readFile(filepath.Join(v.systemDir(namespace, category, id), "l1.txt"))
}

// ReadSystemL2 reads the L2 full content for a system entry.
func (v *LocalVFS) ReadSystemL2(namespace, category, id string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return readFile(filepath.Join(v.systemDir(namespace, category, id), "l2.txt"))
}

// ReadSystemMeta reads the meta.json for a system entry.
func (v *LocalVFS) ReadSystemMeta(namespace, category, id string) (map[string]interface{}, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	meta := readMeta(filepath.Join(v.systemDir(namespace, category, id), "meta.json"))
	if meta == nil {
		return nil, fmt.Errorf("uhms/vfs: system meta not found: %s/%s/%s", namespace, category, id)
	}
	return meta, nil
}

// BatchReadSystemL0 reads L0 abstracts for multiple system entries.
func (v *LocalVFS) BatchReadSystemL0(namespace string, refs []SystemEntryRef) []SystemL0Entry {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entries := make([]SystemL0Entry, 0, len(refs))
	for _, ref := range refs {
		dir := v.systemDir(namespace, ref.Category, ref.ID)
		abstract, err := readFile(filepath.Join(dir, "l0.txt"))
		if err != nil {
			continue // skip entries without L0
		}
		meta := readMeta(filepath.Join(dir, "meta.json"))
		entries = append(entries, SystemL0Entry{
			ID:       ref.ID,
			Category: ref.Category,
			Abstract: abstract,
			Meta:     meta,
		})
	}
	return entries
}

// ListSystemEntries lists all entries under _system/{namespace}/{category}/.
func (v *LocalVFS) ListSystemEntries(namespace, category string) ([]SystemEntryRef, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	dir := filepath.Join(v.root, "_system", namespace, category)
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("uhms/vfs: list system entries %s: %w", dir, err)
	}

	refs := make([]SystemEntryRef, 0, len(dirEntries))
	for _, e := range dirEntries {
		if e.IsDir() {
			refs = append(refs, SystemEntryRef{Category: category, ID: e.Name()})
		}
	}
	return refs, nil
}

// ListSystemCategories lists all categories under _system/{namespace}/.
func (v *LocalVFS) ListSystemCategories(namespace string) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	dir := filepath.Join(v.root, "_system", namespace)
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("uhms/vfs: list system categories %s: %w", dir, err)
	}

	cats := make([]string, 0, len(dirEntries))
	for _, e := range dirEntries {
		if e.IsDir() {
			cats = append(cats, e.Name())
		}
	}
	return cats, nil
}

// DeleteSystemEntry removes a system entry's directory.
func (v *LocalVFS) DeleteSystemEntry(namespace, category, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := v.systemDir(namespace, category, id)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("uhms/vfs: remove system entry %s: %w", dir, err)
	}
	return nil
}

// SystemEntryExists checks if a system entry exists.
func (v *LocalVFS) SystemEntryExists(namespace, category, id string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()

	dir := v.systemDir(namespace, category, id)
	_, err := os.Stat(filepath.Join(dir, "meta.json"))
	return err == nil
}

// ============================================================================
// Path Helpers
// ============================================================================

func (v *LocalVFS) systemDir(namespace, category, id string) string {
	return filepath.Join(v.root, "_system", namespace, category, id)
}

func (v *LocalVFS) memoryDir(userID, memoryType, category, memoryID string) string {
	return filepath.Join(v.root, userID, memoryType, category, memoryID)
}

func (v *LocalVFS) archiveDir(userID, sessionKey string) string {
	return filepath.Join(v.root, userID, "archives", sessionKey)
}

func (v *LocalVFS) relativeMemoryPath(userID, memoryType, category, memoryID string) string {
	return filepath.Join(userID, memoryType, category, memoryID)
}

func (v *LocalVFS) relativeArchivePath(userID, sessionKey string) string {
	return filepath.Join(userID, "archives", sessionKey)
}

// ============================================================================
// File I/O Helpers
// ============================================================================

// maxArchiveL2Bytes is the maximum size for L2 transcript files (200KB ≈ 50K tokens).
const maxArchiveL2Bytes = 200 * 1024

func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("uhms/vfs: write %s: %w", path, err)
	}
	return nil
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readMeta(path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return meta
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// ============================================================================
// Archive Search (Session Archive Listing)
// ============================================================================

// ArchiveEntry represents a session archive.
type ArchiveEntry struct {
	SessionKey string `json:"session_key"`
	L0Summary  string `json:"l0_summary"`
	CreatedAt  int64  `json:"created_at"` // Unix timestamp
}

// ListArchives lists all session archives for a user.
func (v *LocalVFS) ListArchives(userID string) ([]ArchiveEntry, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	archDir := filepath.Join(v.root, userID, "archives")
	entries, err := os.ReadDir(archDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("uhms/vfs: list archives: %w", err)
	}

	archives := make([]ArchiveEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(archDir, e.Name())
		summary, _ := readFile(filepath.Join(dir, "l0.txt"))
		info, _ := e.Info()
		var ts int64
		if info != nil {
			ts = info.ModTime().Unix()
		} else {
			ts = time.Now().Unix()
		}
		archives = append(archives, ArchiveEntry{
			SessionKey: e.Name(),
			L0Summary:  summary,
			CreatedAt:  ts,
		})
	}

	// 最新在前
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].CreatedAt > archives[j].CreatedAt
	})
	return archives, nil
}
