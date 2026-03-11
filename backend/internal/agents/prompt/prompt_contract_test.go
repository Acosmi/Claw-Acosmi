package prompt

import (
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/internal/agents/configtools"
)

// TestTreeToolSummariesAlignWithRegistry 确保 TreeToolSummaries 中的主链路工具名
// 与 capabilities.Registry 一致（不出现 Registry 中不存在的工具名）。
// P1-2: 迁移自 coreToolSummaries → capabilities.TreeToolSummaries()
func TestTreeToolSummariesAlignWithRegistry(t *testing.T) {
	registered := make(map[string]bool)
	for _, spec := range capabilities.Registry {
		if spec.ToolName != "" {
			registered[spec.ToolName] = true
		}
	}

	treeSummaries := capabilities.TreeToolSummaries()

	// 主链路工具必须在 tree summaries 和 Registry 中。
	mainPathTools := []string{
		"bash", "read_file", "write_file", "list_dir",
		"web_search", "browser", "send_media",
		"spawn_coder_agent", "spawn_argus_agent", "spawn_media_agent",
		"search_skills", "lookup_skill",
		"memory_search", "memory_get",
		"report_progress", "request_help",
	}
	for _, tool := range mainPathTools {
		if _, ok := treeSummaries[tool]; !ok {
			t.Errorf("main path tool %q missing from TreeToolSummaries", tool)
		}
		if !registered[tool] {
			t.Errorf("main path tool %q in TreeToolSummaries but missing from capabilities.Registry", tool)
		}
	}
}

// TestTreeToolOrderContainsAllMainPathTools 确保 TreeToolOrder 包含所有主链路工具。
// P1-3: 迁移自 toolOrder → capabilities.TreeToolOrder()
func TestTreeToolOrderContainsAllMainPathTools(t *testing.T) {
	orderSet := make(map[string]bool)
	for _, name := range capabilities.TreeToolOrder() {
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

func TestBuildToolingSectionIncludesSendMediaDelegationHint(t *testing.T) {
	output := buildToolingSection([]string{"send_media", "spawn_argus_agent"}, capabilities.TreeToolSummaries())
	if !contains(output, "existing local file send/forward → send_media") {
		t.Fatalf("tooling section should mention send_media for existing local files, got: %q", output)
	}
	if !contains(output, "desktop/GUI discovery or native app interaction → spawn_argus_agent") {
		t.Fatalf("tooling section should narrow argus delegation scope, got: %q", output)
	}
}

func TestBuildToolingSectionIncludesSpecializedRuntimeExtras(t *testing.T) {
	summaries := capabilities.TreeToolSummaries()
	for name, summary := range configtools.ToolSummaries() {
		summaries[name] = summary
	}
	output := buildToolingSection([]string{"gateway", "browser_config", "media_config"}, summaries)
	if !contains(output, "- browser_config: Read or update browser configuration via dedicated gateway actions") {
		t.Fatalf("tooling section should include browser_config summary, got: %q", output)
	}
	if !contains(output, "- media_config: Read or update media-agent configuration via dedicated gateway actions") {
		t.Fatalf("tooling section should include media_config summary, got: %q", output)
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

// ---------- P1-13: Prompt ## Tooling output contract test ----------

// TestBuildToolingSectionTreeDerived 确保 tree-derived buildToolingSection 输出
// 包含所有预期的结构元素和主链路工具。
func TestBuildToolingSectionTreeDerived(t *testing.T) {
	// Use tree-derived summaries and order
	treeSummaries := capabilities.TreeToolSummaries()
	treeOrder := capabilities.TreeToolOrder()

	output := buildToolingSection(treeOrder, treeSummaries)

	// Must contain header
	if !contains(output, "## Tooling") {
		t.Error("output missing '## Tooling' header")
	}
	if !contains(output, "Tool availability (filtered by policy):") {
		t.Error("output missing availability description")
	}

	// Main path tools must appear
	mainPathTools := []string{
		"bash", "read_file", "write_file", "list_dir",
		"web_search", "browser", "send_media",
		"spawn_coder_agent", "spawn_argus_agent", "spawn_media_agent",
		"memory_search", "memory_get",
		"report_progress", "request_help",
	}
	for _, tool := range mainPathTools {
		if !contains(output, tool) {
			t.Errorf("output missing main path tool %q", tool)
		}
	}

	// Each tool with a summary should appear as "- toolname: summary"
	for _, tool := range mainPathTools {
		if summary, ok := treeSummaries[tool]; ok && summary != "" {
			expected := "- " + tool + ": "
			if !contains(output, expected) {
				t.Errorf("output missing formatted entry for %q", tool)
			}
		}
	}
}

// TestBuildToolingSectionOrderMatchesTree 确保输出中工具的顺序与 TreeToolOrder 一致。
func TestBuildToolingSectionOrderMatchesTree(t *testing.T) {
	treeOrder := capabilities.TreeToolOrder()
	treeSummaries := capabilities.TreeToolSummaries()
	output := buildToolingSection(treeOrder, treeSummaries)

	positions := make(map[string]int)
	index := 0
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		name := strings.TrimPrefix(line, "- ")
		if cut := strings.Index(name, ": "); cut >= 0 {
			name = name[:cut]
		}
		positions[name] = index
		index++
	}

	lastPos := -1
	for _, tool := range treeOrder {
		pos, ok := positions[tool]
		if !ok {
			continue
		}
		if pos < lastPos {
			t.Errorf("tool %q appears before previous tool in output (order violation)", tool)
		}
		lastPos = pos
	}
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
