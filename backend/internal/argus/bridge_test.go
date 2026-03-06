package argus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/mcpclient"
)

// ---------- IsAvailable 测试 ----------

func TestIsAvailable_EmptyPath(t *testing.T) {
	if IsAvailable("") {
		t.Error("expected false for empty path")
	}
}

func TestIsAvailable_NonExistent(t *testing.T) {
	if IsAvailable("/nonexistent/argus-sensory-xyz-12345") {
		t.Error("expected false for non-existent path")
	}
}

func TestIsAvailable_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "argus-sensory")
	if err := os.WriteFile(tmpFile, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsAvailable(tmpFile) {
		t.Error("expected true for existing file")
	}
}

func TestIsAvailable_NotExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "argus-sensory")
	// 0o644: 可读写但不可执行
	if err := os.WriteFile(tmpFile, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsAvailable(tmpFile) {
		t.Error("expected false for non-executable file")
	}
}

func TestIsAvailable_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	if IsAvailable(tmpDir) {
		t.Error("expected false for directory")
	}
}

// ---------- ResolveBinary 测试 ----------

func TestResolveBinary_EmptyName(t *testing.T) {
	_, err := ResolveBinary("")
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestResolveBinary_NonExistent(t *testing.T) {
	_, err := ResolveBinary("argus-sensory-nonexistent-xyz-12345")
	if err == nil {
		t.Error("expected error for non-existent binary")
	}
}

func TestResolveBinary_InPATH(t *testing.T) {
	// 创建临时目录并放入可执行文件，加入 PATH
	tmpDir := t.TempDir()
	tmpBin := filepath.Join(tmpDir, "argus-test-bin")
	if err := os.WriteFile(tmpBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 临时修改 PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)

	resolved, err := ResolveBinary("argus-test-bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != tmpBin {
		t.Errorf("expected %q, got %q", tmpBin, resolved)
	}
}

// ---------- NewBridge + 状态 测试 ----------

func TestNewBridge_InitialState(t *testing.T) {
	cfg := DefaultBridgeConfig()
	b := NewBridge(cfg)

	if b.State() != BridgeStateInit {
		t.Errorf("expected init state, got %s", b.State())
	}
	if b.PID() != 0 {
		t.Errorf("expected PID 0, got %d", b.PID())
	}
	if len(b.Tools()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(b.Tools()))
	}
}

func TestDefaultBridgeConfig(t *testing.T) {
	cfg := DefaultBridgeConfig()
	if cfg.BinaryPath != "argus-sensory" {
		t.Errorf("expected binary path 'argus-sensory', got %q", cfg.BinaryPath)
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "-mcp" {
		t.Errorf("expected args [-mcp], got %v", cfg.Args)
	}
	if cfg.HealthInterval != defaultHealthInterval {
		t.Errorf("expected health interval %v, got %v", defaultHealthInterval, cfg.HealthInterval)
	}
}

func TestBridge_StartWithInvalidBinary(t *testing.T) {
	cfg := DefaultBridgeConfig()
	cfg.BinaryPath = "/nonexistent/argus-sensory-xyz-12345"
	b := NewBridge(cfg)

	err := b.Start()
	if err == nil {
		t.Fatal("expected error for non-existent binary")
	}

	// 启动失败后应回到 stopped 状态
	if b.State() != BridgeStateStopped {
		t.Errorf("expected stopped state after failed start, got %s", b.State())
	}
}

func TestBridge_RetainedAfterFailure(t *testing.T) {
	// 验证: Start 失败后 bridge 仍可被引用，且允许重试 Start
	cfg := DefaultBridgeConfig()
	cfg.BinaryPath = "/nonexistent/argus-sensory-xyz-12345"
	b := NewBridge(cfg)

	err := b.Start()
	if err == nil {
		t.Fatal("expected error")
	}

	// bridge 仍可被引用（非 nil 指针）
	if b == nil {
		t.Fatal("bridge should not be nil after failed start")
	}

	// 状态为 stopped，允许重新 Start
	if b.State() != BridgeStateStopped {
		t.Errorf("expected stopped, got %s", b.State())
	}

	// 重试 Start（仍然会失败，但不应 panic）
	err2 := b.Start()
	if err2 == nil {
		t.Fatal("expected error on retry")
	}
	if b.State() != BridgeStateStopped {
		t.Errorf("expected stopped after retry, got %s", b.State())
	}
}

func TestBridge_StopAfterFailedStart(t *testing.T) {
	// 验证: Start 失败后 Stop 不 panic 且正确清理
	cfg := DefaultBridgeConfig()
	cfg.BinaryPath = "/nonexistent/argus-sensory-xyz-12345"
	b := NewBridge(cfg)

	_ = b.Start()
	b.Stop() // 不应 panic
	b.Stop() // 幂等
}

func TestBridge_StopIdempotent(t *testing.T) {
	cfg := DefaultBridgeConfig()
	b := NewBridge(cfg)

	// 从未启动的 bridge 调用 Stop 不应 panic
	b.mu.Lock()
	b.state = BridgeStateStopped
	b.mu.Unlock()

	b.Stop() // 第一次
	b.Stop() // 第二次 — 幂等
}

func TestBridge_CallToolNotAvailable(t *testing.T) {
	cfg := DefaultBridgeConfig()
	b := NewBridge(cfg)

	// 未启动时调用 CallTool 应报错
	_, err := b.CallTool(nil, "capture_screen", nil, 0)
	if err == nil {
		t.Fatal("expected error when bridge not started")
	}
}

// ---------- ArgusStartError / CheckBinary 测试 ----------

func TestArgusStartError_Error(t *testing.T) {
	e := &ArgusStartError{Phase: "resolve", Reason: "binary_not_found", Recovery: "Install argus-sensory"}
	msg := e.Error()
	if !strings.Contains(msg, "resolve") || !strings.Contains(msg, "binary_not_found") {
		t.Errorf("unexpected error message: %s", msg)
	}
	if e.Recovery == "" {
		t.Error("expected non-empty recovery")
	}
}

func TestArgusStartError_WithUnderlying(t *testing.T) {
	inner := fmt.Errorf("file not found")
	e := &ArgusStartError{Phase: "crash", Reason: "start_failed", Err: inner}
	if e.Unwrap() != inner {
		t.Error("Unwrap should return inner error")
	}
	if !strings.Contains(e.Error(), "file not found") {
		t.Errorf("error message should contain inner error: %s", e.Error())
	}
}

func TestArgusStartError_JSON(t *testing.T) {
	e := &ArgusStartError{Phase: "permission", Reason: "not_executable", Recovery: "chmod +x"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if m["phase"] != "permission" || m["reason"] != "not_executable" || m["recovery"] != "chmod +x" {
		t.Errorf("unexpected JSON: %s", string(data))
	}
}

func TestCheckBinary_Empty(t *testing.T) {
	r := CheckBinary("")
	if r.Status != "not_found" || r.Recovery == "" {
		t.Errorf("expected not_found with recovery, got: %+v", r)
	}
}

func TestCheckBinary_NonExistent(t *testing.T) {
	r := CheckBinary("/nonexistent/argus-xyz-12345")
	if r.Status != "not_found" || r.Recovery == "" {
		t.Errorf("expected not_found with recovery, got: %+v", r)
	}
}

func TestCheckBinary_NotExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "argus-sensory")
	os.WriteFile(tmpFile, []byte("#!/bin/sh\n"), 0o644)

	r := CheckBinary(tmpFile)
	if r.Status != "not_executable" || r.Recovery == "" {
		t.Errorf("expected not_executable with recovery, got: %+v", r)
	}
}

func TestCheckBinary_Available(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "argus-sensory")
	os.WriteFile(tmpFile, []byte("#!/bin/sh\n"), 0o755)

	r := CheckBinary(tmpFile)
	if r.Status != "available" {
		t.Errorf("expected available, got: %+v", r)
	}
}

func TestCheckBinary_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	r := CheckBinary(tmpDir)
	if r.Status != "not_executable" {
		t.Errorf("expected not_executable for directory, got: %+v", r)
	}
}

// ---------- BuildArgusSkillEntries 测试 ----------

func TestBuildArgusSkillEntries_AllKnownTools(t *testing.T) {
	tools := []mcpclient.MCPToolDef{
		{Name: "capture_screen", Description: "Capture screenshot"},
		{Name: "click", Description: "Click at position"},
		{Name: "run_shell", Description: "Run shell command"},
		{Name: "macos_shortcut", Description: "macOS shortcut"},
	}

	entries := BuildArgusSkillEntries(tools)

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	tests := []struct {
		idx      int
		name     string
		category string
		risk     string
		emoji    string
	}{
		{0, "argus.capture_screen", "perception", "low", "eye"},
		{1, "argus.click", "action", "medium", "pointer"},
		{2, "argus.run_shell", "shell", "high", "terminal"},
		{3, "argus.macos_shortcut", "macos", "medium", "apple"},
	}

	for _, tc := range tests {
		e := entries[tc.idx]
		if e.Name != tc.name {
			t.Errorf("[%d] name: expected %q, got %q", tc.idx, tc.name, e.Name)
		}
		if e.Category != tc.category {
			t.Errorf("[%d] category: expected %q, got %q", tc.idx, tc.category, e.Category)
		}
		if e.Risk != tc.risk {
			t.Errorf("[%d] risk: expected %q, got %q", tc.idx, tc.risk, e.Risk)
		}
		if e.Emoji != tc.emoji {
			t.Errorf("[%d] emoji: expected %q, got %q", tc.idx, tc.emoji, e.Emoji)
		}
		if e.Source != "argus" {
			t.Errorf("[%d] source: expected 'argus', got %q", tc.idx, e.Source)
		}
		if !e.Eligible {
			t.Errorf("[%d] expected eligible=true", tc.idx)
		}
		if e.SkillKey != tc.name {
			t.Errorf("[%d] skillKey: expected %q, got %q", tc.idx, tc.name, e.SkillKey)
		}
	}
}

func TestBuildArgusSkillEntries_UnknownTool(t *testing.T) {
	tools := []mcpclient.MCPToolDef{
		{Name: "future_tool", Description: "Some new tool"},
	}

	entries := BuildArgusSkillEntries(tools)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Category != "unknown" {
		t.Errorf("expected category 'unknown' for unrecognized tool, got %q", e.Category)
	}
	if e.Risk != "medium" {
		t.Errorf("expected risk 'medium' for unrecognized tool, got %q", e.Risk)
	}
	if e.Emoji != "tool" {
		t.Errorf("expected emoji 'tool' for unknown category, got %q", e.Emoji)
	}
}

func TestBuildArgusSkillEntries_Empty(t *testing.T) {
	entries := BuildArgusSkillEntries(nil)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nil tools, got %d", len(entries))
	}
}

func TestBuildArgusSkillEntries_UsesDocsSkillBindingWhenAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "tools", "argus-screen-reading")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	content := "---\nname: argus-screen-reading\ndescription: Argus 读屏：先 read_text，再 describe_scene；capture_screen 只在状态不明时使用\ntools: argus_read_text, argus_capture_screen\n---\n"
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := os.Getenv("OPENACOSMI_DOCS_SKILLS_DIR")
	if err := os.Setenv("OPENACOSMI_DOCS_SKILLS_DIR", tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if prev == "" {
			_ = os.Unsetenv("OPENACOSMI_DOCS_SKILLS_DIR")
			return
		}
		_ = os.Setenv("OPENACOSMI_DOCS_SKILLS_DIR", prev)
	}()

	tools := []mcpclient.MCPToolDef{
		{Name: "read_text", Description: "OCR text"},
		{Name: "click", Description: "Click at position"},
	}

	entries := BuildArgusSkillEntries(tools)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Description != "Argus 读屏：先 read_text，再 describe_scene；capture_screen 只在状态不明时使用" {
		t.Fatalf("expected docs-backed description, got %q", entries[0].Description)
	}
	if entries[0].FilePath != skillFile {
		t.Fatalf("expected filePath %q, got %q", skillFile, entries[0].FilePath)
	}
	if entries[0].BaseDir != skillDir {
		t.Fatalf("expected baseDir %q, got %q", skillDir, entries[0].BaseDir)
	}

	if entries[1].Description != "Click at position" {
		t.Fatalf("expected MCP fallback description for unbound tool, got %q", entries[1].Description)
	}
	if entries[1].FilePath != "" {
		t.Fatalf("expected empty filePath for unbound tool, got %q", entries[1].FilePath)
	}
}

func TestBuildArgusSkillEntries_JSONSerialization(t *testing.T) {
	tools := []mcpclient.MCPToolDef{
		{Name: "capture_screen", Description: "Capture screenshot"},
	}

	entries := BuildArgusSkillEntries(tools)
	data, err := json.Marshal(entries[0])
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// 验证关键字段存在
	for _, key := range []string{"name", "description", "source", "skillKey", "category", "risk", "eligible", "requirements", "missing"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON field %q", key)
		}
	}
}

// ---------- emojiForCategory 测试 ----------

func TestEmojiForCategory(t *testing.T) {
	tests := []struct {
		category string
		expected string
	}{
		{"perception", "eye"},
		{"Perception", "eye"},
		{"action", "pointer"},
		{"shell", "terminal"},
		{"macos", "apple"},
		{"unknown", "tool"},
		{"", "tool"},
	}

	for _, tc := range tests {
		got := emojiForCategory(tc.category)
		if got != tc.expected {
			t.Errorf("emojiForCategory(%q): expected %q, got %q", tc.category, tc.expected, got)
		}
	}
}

// ---------- slogWriter 测试 ----------

func TestSlogWriter(t *testing.T) {
	w := &slogWriter{prefix: "test"}

	// 不应 panic
	n, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 12 {
		t.Errorf("expected 12 bytes written, got %d", n)
	}

	// 空行
	n, err = w.Write([]byte("\n"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 byte written, got %d", n)
	}
}
