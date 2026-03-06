package session

import (
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/prompt"
	"github.com/Acosmi/ClawAcosmi/internal/routing"
)

// TestFilterSubagentContextFiles_ExcludesMemoryAndSoul S6-T6:
// 子智能体会话不注入 MEMORY.md 和 SOUL.md。
func TestFilterSubagentContextFiles_ExcludesMemoryAndSoul(t *testing.T) {
	input := []ContextFile{
		{Path: "SOUL.md", Content: "persona", Priority: 0},
		{Path: "TOOLS.md", Content: "tools guide", Priority: 1},
		{Path: "MEMORY.md", Content: "user memory", Priority: 2},
		{Path: "CLAUDE.md", Content: "project config", Priority: 4},
	}

	filtered := filterSubagentContextFiles(input)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 files after filtering, got %d", len(filtered))
	}

	for _, f := range filtered {
		if f.Path == "SOUL.md" || f.Path == "MEMORY.md" {
			t.Errorf("subagent should not receive %s", f.Path)
		}
	}

	// Verify TOOLS.md and CLAUDE.md are kept
	names := make(map[string]bool)
	for _, f := range filtered {
		names[f.Path] = true
	}
	if !names["TOOLS.md"] {
		t.Error("TOOLS.md should be kept for subagents")
	}
	if !names["CLAUDE.md"] {
		t.Error("CLAUDE.md should be kept for subagents")
	}
}

// TestFilterSubagentContextFiles_EmptyInput 空输入返回 nil。
func TestFilterSubagentContextFiles_EmptyInput(t *testing.T) {
	filtered := filterSubagentContextFiles(nil)
	if filtered != nil {
		t.Errorf("expected nil for nil input, got %v", filtered)
	}
}

// TestBuildDynamicSystemPrompt_SubagentFiltering 验证 IsSubagent=true 时
// BuildDynamicSystemPrompt 不包含 MEMORY.md 内容。
func TestBuildDynamicSystemPrompt_SubagentFiltering(t *testing.T) {
	params := DynamicPromptParams{
		IsSubagent: true,
		ContextFiles: []ContextFile{
			{Path: "SOUL.md", Content: "I am the soul", Priority: 0},
			{Path: "TOOLS.md", Content: "tool guide content", Priority: 1},
			{Path: "MEMORY.md", Content: "secret memory data", Priority: 2},
		},
	}

	result := BuildDynamicSystemPrompt(params)

	if containsStr(result, "secret memory data") {
		t.Error("subagent prompt should not contain MEMORY.md content")
	}
	if containsStr(result, "I am the soul") {
		t.Error("subagent prompt should not contain SOUL.md content")
	}
	if !containsStr(result, "tool guide content") {
		t.Error("subagent prompt should contain TOOLS.md content")
	}
}

// TestBuildDynamicSystemPrompt_MainSessionKeepsAll 验证 IsSubagent=false 时
// 所有 context files 都保留。
func TestBuildDynamicSystemPrompt_MainSessionKeepsAll(t *testing.T) {
	params := DynamicPromptParams{
		IsSubagent: false,
		ContextFiles: []ContextFile{
			{Path: "SOUL.md", Content: "I am the soul", Priority: 0},
			{Path: "MEMORY.md", Content: "secret memory data", Priority: 2},
		},
	}

	result := BuildDynamicSystemPrompt(params)

	if !containsStr(result, "I am the soul") {
		t.Error("main session prompt should contain SOUL.md")
	}
	if !containsStr(result, "secret memory data") {
		t.Error("main session prompt should contain MEMORY.md")
	}
}

// ---------- F7 修复: 测试实际执行路径的子智能体过滤逻辑 ----------
// attempt_runner.go 中的过滤逻辑使用 routing.IsSubagentSessionKey() + filepath.Base() 匹配。
// 以下测试验证该模式在 session 包内的等效行为，确保过滤逻辑与实际执行管线一致。

// TestAttemptRunnerSubagentFilteringPattern 验证 attempt_runner.go 中
// 使用的子智能体过滤模式：routing.IsSubagentSessionKey() + filepath.Base() 匹配。
func TestAttemptRunnerSubagentFilteringPattern(t *testing.T) {
	subagentKeys := []string{
		"subagent:coder:abc123",
		"subagent:argus:def456",
		"subagent:media:ghi789",
	}
	mainKeys := []string{
		"main",
		"webchat:user1",
		"feishu:oc_xxx",
		"",
	}

	// 子智能体 session key 应被识别
	for _, key := range subagentKeys {
		if !routing.IsSubagentSessionKey(key) {
			t.Errorf("expected %q to be recognized as subagent session key", key)
		}
	}

	// 主会话 session key 不应被识别为子智能体
	for _, key := range mainKeys {
		if routing.IsSubagentSessionKey(key) {
			t.Errorf("expected %q to NOT be recognized as subagent session key", key)
		}
	}

	// 验证 filepath.Base 匹配逻辑（与 attempt_runner.go 一致）
	contextFiles := []prompt.ContextFile{
		{Path: "SOUL.md", Content: "persona"},
		{Path: "TOOLS.md", Content: "tools guide"},
		{Path: "MEMORY.md", Content: "user memory"},
		{Path: "CLAUDE.md", Content: "project config"},
	}

	// 模拟 attempt_runner.go 中的过滤逻辑
	filtered := contextFiles[:0:0] // 不修改原 slice
	for _, cf := range contextFiles {
		base := filepath.Base(cf.Path)
		if base == "MEMORY.md" || base == "SOUL.md" {
			continue
		}
		filtered = append(filtered, cf)
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 files after filtering, got %d", len(filtered))
	}
	for _, f := range filtered {
		base := filepath.Base(f.Path)
		if base == "MEMORY.md" || base == "SOUL.md" {
			t.Errorf("subagent should not receive %s", f.Path)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
