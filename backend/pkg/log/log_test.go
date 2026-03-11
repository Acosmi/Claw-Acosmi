package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ==================== levels_test ====================

func TestNormalizeLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		fallback types.LogLevel
		want     types.LogLevel
	}{
		{"info", types.LogInfo, types.LogInfo},
		{"debug", types.LogInfo, types.LogDebug},
		{"trace", types.LogInfo, types.LogTrace},
		{"silent", types.LogInfo, types.LogSilent},
		{"", types.LogInfo, types.LogInfo},
		{"  info  ", types.LogInfo, types.LogInfo},
		{"INVALID", types.LogWarn, types.LogWarn},
		{"INFO", types.LogInfo, types.LogInfo}, // 不匹配：大写非法
	}

	for _, tt := range tests {
		got := NormalizeLogLevel(tt.input, tt.fallback)
		if got != tt.want {
			t.Errorf("NormalizeLogLevel(%q, %q) = %q, want %q", tt.input, tt.fallback, got, tt.want)
		}
	}
}

func TestLevelPriority(t *testing.T) {
	// fatal < error < warn < info < debug < trace
	if LevelPriority(types.LogFatal) >= LevelPriority(types.LogError) {
		t.Error("fatal should have lower priority number than error")
	}
	if LevelPriority(types.LogError) >= LevelPriority(types.LogWarn) {
		t.Error("error should have lower priority number than warn")
	}
	if LevelPriority(types.LogInfo) >= LevelPriority(types.LogDebug) {
		t.Error("info should have lower priority number than debug")
	}
	if LevelPriority(types.LogDebug) >= LevelPriority(types.LogTrace) {
		t.Error("debug should have lower priority number than trace")
	}
	// silent 最高
	if LevelPriority(types.LogSilent) <= LevelPriority(types.LogTrace) {
		t.Error("silent should have highest priority number")
	}
}

func TestIsLevelEnabled(t *testing.T) {
	tests := []struct {
		msg, min types.LogLevel
		want     bool
	}{
		{types.LogError, types.LogInfo, true},   // error 在 info 级别下可输出
		{types.LogDebug, types.LogInfo, false},  // debug 在 info 级别下不可输出
		{types.LogInfo, types.LogInfo, true},    // 同级
		{types.LogTrace, types.LogTrace, true},  // trace 在 trace 下可输出
		{types.LogInfo, types.LogSilent, false}, // silent 关闭所有
	}

	for _, tt := range tests {
		got := IsLevelEnabled(tt.msg, tt.min)
		if got != tt.want {
			t.Errorf("IsLevelEnabled(%q, %q) = %v, want %v", tt.msg, tt.min, got, tt.want)
		}
	}
}

// ==================== file_test ====================

func TestFileWriter_WriteAndRoll(t *testing.T) {
	dir := t.TempDir()
	fw := NewFileWriter(dir)
	defer fw.Close()

	entry := map[string]interface{}{
		"level": "info",
		"msg":   "test message",
	}
	if err := fw.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	// 文件应该被创建
	path := fw.CurrentPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("log file not created: %s", path)
	}

	// 文件内容应包含 JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "test message") {
		t.Error("log file should contain test message")
	}
	if !strings.Contains(content, "time") {
		t.Error("log file should contain time field")
	}
}

func TestFileWriter_CurrentPath(t *testing.T) {
	dir := t.TempDir()
	fw := NewFileWriter(dir)
	defer fw.Close()

	path := fw.CurrentPath()
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(path, today) {
		t.Errorf("path %q should contain today's date %q", path, today)
	}
	if !strings.HasPrefix(filepath.Base(path), "openacosmi-") {
		t.Errorf("path %q should start with openacosmi-", path)
	}
}

func TestDefaultRollingPath_UsesSystemTempDir(t *testing.T) {
	path := DefaultRollingPath()
	if !strings.HasPrefix(path, filepath.Join(os.TempDir(), "openacosmi")) {
		t.Fatalf("DefaultRollingPath() = %q", path)
	}
}

func TestIsRollingLogName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"openacosmi-2024-01-15.log", true},
		{"openacosmi-2026-02-12.log", true},
		{"openacosmi.log", false},
		{"other-2024-01-15.log", false},
		{"openacosmi-2024-01-15.txt", false},
		{"openacosmi-20240115.log", false}, // 无连字符
	}

	for _, tt := range tests {
		got := isRollingLogName(tt.name)
		if got != tt.want {
			t.Errorf("isRollingLogName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneOldLogs(t *testing.T) {
	dir := t.TempDir()

	// 创建一个旧日志
	oldName := filepath.Join(dir, "openacosmi-2020-01-01.log")
	if err := os.WriteFile(oldName, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	// 设置修改时间为 48h 前
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldName, oldTime, oldTime)

	// 创建一个新日志
	newName := filepath.Join(dir, "openacosmi-2026-02-12.log")
	if err := os.WriteFile(newName, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	pruneOldLogs(dir)

	// 旧日志应被删除
	if _, err := os.Stat(oldName); !os.IsNotExist(err) {
		t.Error("old log should be pruned")
	}
	// 新日志应保留
	if _, err := os.Stat(newName); os.IsNotExist(err) {
		t.Error("new log should be kept")
	}
}

// ==================== redact_test ====================

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "***"},
		{"sk-abcdefghijklmnopqrstuvwxyz", "sk-abc…wxyz"},
	}
	for _, tt := range tests {
		got := maskToken(tt.input)
		if got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRedactSensitiveText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "SK token",
			input: "Using key sk-abcdefghijklmnopqrstuvwxyz for API",
			check: func(s string) bool {
				return !strings.Contains(s, "sk-abcdefghijklmnopqrstuvwxyz") && strings.Contains(s, "sk-abc")
			},
		},
		{
			name:  "GitHub PAT",
			input: "token ghp_abcdefghijklmnopqrstuvwxyz1234",
			check: func(s string) bool {
				return !strings.Contains(s, "ghp_abcdefghijklmnopqrstuvwxyz1234")
			},
		},
		{
			name:  "Bearer token",
			input: "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz",
			check: func(s string) bool {
				return !strings.Contains(s, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
			},
		},
		{
			name:  "no sensitive data",
			input: "Hello world, this is normal text",
			check: func(s string) bool {
				return s == "Hello world, this is normal text"
			},
		},
		{
			name:  "empty string",
			input: "",
			check: func(s string) bool { return s == "" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactSensitiveText(tt.input, nil)
			if !tt.check(got) {
				t.Errorf("RedactSensitiveText(%q) = %q, check failed", tt.input, got)
			}
		})
	}
}

func TestRedactToolDetail(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	input := "API key is " + secret

	// tools 模式下应脱敏
	got := RedactToolDetail(input, RedactTools)
	if strings.Contains(got, secret) {
		t.Error("tools mode should redact the secret")
	}

	// off 模式下不脱敏
	got = RedactToolDetail(input, RedactOff)
	if got != input {
		t.Errorf("off mode should not modify text, got %q", got)
	}
}

// ==================== log_test ====================

func TestNewLogger(t *testing.T) {
	l := New("test")
	if l.Subsystem() != "test" {
		t.Errorf("Subsystem() = %q, want %q", l.Subsystem(), "test")
	}
}

func TestLoggerChild(t *testing.T) {
	l := New("parent")
	child := l.Child("child")
	if child.Subsystem() != "parent/child" {
		t.Errorf("child Subsystem() = %q, want %q", child.Subsystem(), "parent/child")
	}
}

func TestGlobalLevel(t *testing.T) {
	// 保存原始值
	orig := GetGlobalLevel()
	defer SetGlobalLevel(orig)

	SetGlobalLevel(types.LogDebug)
	if got := GetGlobalLevel(); got != types.LogDebug {
		t.Errorf("GetGlobalLevel() = %q after set debug, want %q", got, types.LogDebug)
	}

	SetGlobalLevel(types.LogSilent)
	if got := GetGlobalLevel(); got != types.LogSilent {
		t.Errorf("GetGlobalLevel() = %q after set silent, want %q", got, types.LogSilent)
	}
}

func TestEnableFileLogging(t *testing.T) {
	dir := t.TempDir()
	EnableFileLogging(dir)

	l := New("test-file")
	l.Info("hello from file logging test")

	// 检查文件被创建
	fw := getFileWriter()
	if fw == nil {
		t.Fatal("file writer should be set")
	}
	path := fw.CurrentPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("log file not created at %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello from file logging test") {
		t.Error("log file should contain the test message")
	}

	// 清理
	_ = fw.Close()
	globalMu.Lock()
	globalFileWriter = nil
	globalMu.Unlock()
}
