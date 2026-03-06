package prompt

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
)

// TestCoreToolSummariesAlignWithRegistry 确保 coreToolSummaries 中的主链路工具名
// 与 capabilities.Registry 一致（不出现 Registry 中不存在的工具名）。
func TestCoreToolSummariesAlignWithRegistry(t *testing.T) {
	registered := make(map[string]bool)
	for _, spec := range capabilities.Registry {
		if spec.ToolName != "" {
			registered[spec.ToolName] = true
		}
	}

	// 主链路工具（在 coreToolSummaries 注释中标记为"主链路"的）必须在 Registry 中。
	mainPathTools := []string{
		"bash", "read_file", "write_file", "list_dir",
		"web_search", "browser", "send_media",
		"spawn_coder_agent", "spawn_argus_agent", "spawn_media_agent",
		"search_skills", "lookup_skill",
		"memory_search", "memory_get",
		"report_progress", "request_help",
	}
	for _, tool := range mainPathTools {
		if _, ok := coreToolSummaries[tool]; !ok {
			t.Errorf("main path tool %q missing from coreToolSummaries", tool)
		}
		if !registered[tool] {
			t.Errorf("main path tool %q in coreToolSummaries but missing from capabilities.Registry", tool)
		}
	}
}

// TestToolOrderContainsAllMainPathTools 确保 toolOrder 包含所有主链路工具。
func TestToolOrderContainsAllMainPathTools(t *testing.T) {
	orderSet := make(map[string]bool)
	for _, name := range toolOrder {
		orderSet[name] = true
	}

	mainPathTools := []string{
		"bash", "read_file", "write_file", "list_dir",
		"spawn_coder_agent", "spawn_argus_agent", "spawn_media_agent",
		"search_skills", "lookup_skill",
		"memory_search", "memory_get",
		"report_progress", "request_help",
	}
	for _, tool := range mainPathTools {
		if !orderSet[tool] {
			t.Errorf("main path tool %q missing from toolOrder", tool)
		}
	}
}

// TestBuildToolingSectionOutputsRegisteredTools 确保 buildToolingSection 用真实工具名
// 输出包含 Registry 中的摘要。
func TestBuildToolingSectionOutputsRegisteredTools(t *testing.T) {
	toolNames := []string{"bash", "read_file", "write_file"}
	summaries := capabilities.ToolSummaries()

	output := buildToolingSection(toolNames, summaries)

	// 每个传入的工具名都应出现在输出中
	for _, name := range toolNames {
		if !contains(output, name) {
			t.Errorf("buildToolingSection output missing tool %q", name)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- S6-T4: WARM_START / COLD_START 判定测试 ----------

// TestBuildSystemContextBlock_ColdStartNoSummary 无 bootBrief 时 [Last_Summary] 为 "无"。
func TestBuildSystemContextBlock_ColdStartNoSummary(t *testing.T) {
	block := buildSystemContextBlock(SessionColdStart, "", "")

	if !contains(block, "COLD_START") {
		t.Error("expected COLD_START in context block")
	}
	if !contains(block, "[Last_Summary]: 无") {
		t.Error("expected [Last_Summary]: 无 when bootBrief is empty")
	}
}

// TestBuildSystemContextBlock_WarmStartWithSummary 有 bootBrief 时应正确填入。
func TestBuildSystemContextBlock_WarmStartWithSummary(t *testing.T) {
	brief := "上次修复了 media 模块的 3 个 bug"
	block := buildSystemContextBlock(SessionWarmStart, "", brief)

	if !contains(block, "WARM_START") {
		t.Error("expected WARM_START in context block")
	}
	if !contains(block, brief) {
		t.Error("expected boot brief content in [Last_Summary]")
	}
}

// TestBuildSystemContextBlock_EmptyBriefNotWarmStart 空 brief + WARM_START 状态
// 仍然输出 [Last_Summary]: 无 — 防止模型产生虚假记忆恢复。
func TestBuildSystemContextBlock_EmptyBriefNotWarmStart(t *testing.T) {
	// 即使上游错误传入 WARM_START + 空 brief，Last_Summary 也应为 "无"
	block := buildSystemContextBlock(SessionWarmStart, "", "")

	if !contains(block, "[Last_Summary]: 无") {
		t.Error("[Last_Summary] should be 无 when bootBrief is empty, even with WARM_START state")
	}
}

// TestBuildSystemContextBlock_WhitespaceBriefTreatedAsEmpty 纯空白 brief 视为空。
func TestBuildSystemContextBlock_WhitespaceBriefTreatedAsEmpty(t *testing.T) {
	block := buildSystemContextBlock(SessionWarmStart, "", "   \n\t  ")

	if !contains(block, "[Last_Summary]: 无") {
		t.Error("[Last_Summary] should be 无 when bootBrief is only whitespace")
	}
}
