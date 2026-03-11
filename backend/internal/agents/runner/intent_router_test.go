package runner

import (
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// ---------- classifyIntent 测试 ----------

func TestClassifyIntent_Greeting(t *testing.T) {
	cases := []string{"你好", "hi", "hello", "嗨", "早上好", "晚安"}
	for _, c := range cases {
		if tier := classifyIntent(c); tier != intentGreeting {
			t.Errorf("classifyIntent(%q) = %q, want greeting", c, tier)
		}
	}
}

func TestClassifyIntent_Question(t *testing.T) {
	cases := []string{
		"这个API怎么用？",
		"什么是Docker？",
		"系统有几个CPU核心？",
	}
	for _, c := range cases {
		if tier := classifyIntent(c); tier != intentQuestion {
			t.Errorf("classifyIntent(%q) = %q, want question", c, tier)
		}
	}
}

func TestClassifyIntent_ImperativeOverridesQuestion(t *testing.T) {
	// "帮我" 即使在疑问句中也应被识别为任务，不是提问
	cases := []struct {
		input string
		want  intentTier
	}{
		// 前缀型（已有逻辑）
		{"帮我看下系统资源？", intentTaskLight},
		{"帮忙查一下日志？", intentTaskLight},
		// 嵌入型（Bug#4 修复点）
		{"嗨，你帮我看下，我们系统目前占用的资源是多少？内存", intentTaskLight},
		{"你帮我查查这个API怎么用？", intentTaskLight},
		{"能帮我看看为什么报错？", intentTaskLight},
		{"请帮我分析一下这段代码？", intentTaskLight},
	}
	for _, c := range cases {
		if tier := classifyIntent(c.input); tier != c.want {
			t.Errorf("classifyIntent(%q) = %q, want %q", c.input, tier, c.want)
		}
	}
}

func TestClassifyIntent_TaskLight(t *testing.T) {
	cases := []string{
		"看下系统状态",
		"查一下内存",
		"运行测试",
	}
	for _, c := range cases {
		if tier := classifyIntent(c); tier != intentTaskLight {
			t.Errorf("classifyIntent(%q) = %q, want task_light", c, tier)
		}
	}
}

func TestClassifyIntent_TaskWrite(t *testing.T) {
	cases := []string{
		"创建一个新文件",
		"帮我写一个函数",
		"修改这个配置",
		"看下浏览器配置",
		"远程审批配置怎么改",
		"帮我调整 stt 配置",
	}
	for _, c := range cases {
		tier := classifyIntent(c)
		if tier != intentTaskWrite {
			t.Errorf("classifyIntent(%q) = %q, want task_write", c, tier)
		}
	}
}

func TestClassifyIntent_TaskDelete(t *testing.T) {
	cases := []string{
		"删除这个文件",
		"清理临时目录",
		"rm 旧的日志",
	}
	for _, c := range cases {
		if tier := classifyIntent(c); tier != intentTaskDelete {
			t.Errorf("classifyIntent(%q) = %q, want task_delete", c, tier)
		}
	}
}

// ---------- hasImperativePrefix 测试 ----------

func TestHasImperativePrefix_Prefix(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"帮我看下", true},
		{"帮忙查一下", true},
		{"麻烦帮我", true},
		{"请帮我看看", true},
		{"给我结果", true},
		{"替我做一下", true},
	}
	for _, c := range cases {
		if got := hasImperativePrefix(c.input); got != c.want {
			t.Errorf("hasImperativePrefix(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestHasImperativePrefix_Embedded(t *testing.T) {
	// Bug#4 核心: "帮我" 嵌入句中应被检测到
	cases := []struct {
		input string
		want  bool
	}{
		{"嗨，你帮我看下", true},
		{"你帮我查一下", true},
		{"能帮我看看", true},
		{"可以帮忙查一下吗", true},
		{"你能替我做吗", true},
		// 不应匹配的
		{"这是什么", false},
		{"系统状态如何", false},
	}
	for _, c := range cases {
		if got := hasImperativePrefix(c.input); got != c.want {
			t.Errorf("hasImperativePrefix(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

// ---------- Bug#11: 中文 NLP 修复测试 ----------

func TestClassifyIntent_ChinesePoliteImperative(t *testing.T) {
	// Bug#11 核心: "你能X吗"/"能不能X"/"可以X吗" 形式的排查任务不应被分类为 question
	cases := []struct {
		input string
		want  intentTier
	}{
		// 礼貌祈使 + 诊断动词 → task_light（非 question）
		{"远程飞书不好用了你能排查吗？", intentTaskLight},
		{"能否检查一下系统日志？", intentTaskLight},
		// 帮我 已覆盖，确认不退化
		{"你能帮我排查吗？", intentTaskLight},
		// 纯疑问句不受影响
		{"这是什么意思？", intentQuestion},
		// 英文对应模式
		{"Can you debug this error?", intentTaskLight},
		{"What is the meaning of life?", intentQuestion},
	}
	for _, c := range cases {
		if tier := classifyIntent(c.input); tier != c.want {
			t.Errorf("classifyIntent(%q) = %q, want %q", c.input, tier, c.want)
		}
	}
}

func TestHasImperativePrefix_PoliteRequest(t *testing.T) {
	// Bug#11: 礼貌祈使句模式检测
	cases := []struct {
		input string
		want  bool
	}{
		{"你能排查一下吗", true},
		{"能不能查一下", true},
		{"可以帮忙看看吗", true},
		{"能否检查一下", true},
		{"can you help me", true},
		{"could you check this", true},
		{"would you please look", true},
		// 不应匹配的
		{"这个好用吗", false},
		{"what is this", false},
	}
	for _, c := range cases {
		if got := hasImperativePrefix(c.input); got != c.want {
			t.Errorf("hasImperativePrefix(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestContainsActionVerb(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"帮我排查一下", true},
		{"需要诊断问题", true},
		{"可以调试吗", true},
		{"请重启服务", true},
		{"can you debug this", true},
		// 不含动作动词
		{"你好吗", false},
		{"这是什么", false},
		{"how are you", false},
	}
	for _, c := range cases {
		if got := containsActionVerb(c.input); got != c.want {
			t.Errorf("containsActionVerb(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

// ---------- filterToolsByIntent 测试 ----------

func TestFilterToolsByIntent_QuestionOnlySearchTools(t *testing.T) {
	tools := mockToolDefs("bash", "read_file", "search_skills", "lookup_skill", "write_file")
	filtered := filterToolsByIntent(tools, intentQuestion)
	names := toolNames(filtered)

	if contains(names, "bash") {
		t.Error("intentQuestion should NOT include bash")
	}
	if !contains(names, "search_skills") {
		t.Error("intentQuestion should include search_skills")
	}
	if !contains(names, "lookup_skill") {
		t.Error("intentQuestion should include lookup_skill")
	}
}

func TestFilterToolsByIntent_TaskLightIncludesBash(t *testing.T) {
	tools := mockToolDefs("bash", "read_file", "search_skills", "lookup_skill", "write_file")
	filtered := filterToolsByIntent(tools, intentTaskLight)
	names := toolNames(filtered)

	if !contains(names, "bash") {
		t.Error("intentTaskLight MUST include bash")
	}
	if !contains(names, "read_file") {
		t.Error("intentTaskLight should include read_file")
	}
}

func TestFilterToolsByIntent_GreetingNoTools(t *testing.T) {
	tools := mockToolDefs("bash", "read_file", "search_skills")
	filtered := filterToolsByIntent(tools, intentGreeting)
	if len(filtered) != 0 {
		t.Errorf("intentGreeting should have 0 tools, got %d", len(filtered))
	}
}

func TestPromoteBrowserContinuation_BrowserQuestionKeepsMultimodal(t *testing.T) {
	prior := []llmclient.ChatMessage{{
		Role: "assistant",
		Content: []llmclient.ContentBlock{{
			Type: "tool_use",
			Name: "browser",
			ID:   "tool-1",
		}},
	}}

	analysis := analyzeIntent("为什么没有 browser 工具")
	if analysis.Tier != intentQuestion {
		t.Fatalf("expected base tier question, got %q", analysis.Tier)
	}

	promoted := promoteBrowserContinuation(analysis, "为什么没有 browser 工具", prior)
	if promoted.Tier != intentTaskMultimodal {
		t.Fatalf("expected multimodal promotion, got %q", promoted.Tier)
	}
}

func TestPromoteBrowserContinuation_ShortFollowUpKeepsMultimodal(t *testing.T) {
	prior := []llmclient.ChatMessage{{
		Role: "assistant",
		Content: []llmclient.ContentBlock{{
			Type: "tool_use",
			Name: "browser",
			ID:   "tool-2",
		}},
	}}

	analysis := analyzeIntent("继续")
	promoted := promoteBrowserContinuation(analysis, "继续", prior)
	if promoted.Tier != intentTaskMultimodal {
		t.Fatalf("expected short browser continuation to stay multimodal, got %q", promoted.Tier)
	}
}

func TestPromoteBrowserContinuation_DoesNotPromoteUnrelatedQuestion(t *testing.T) {
	prior := []llmclient.ChatMessage{{
		Role: "assistant",
		Content: []llmclient.ContentBlock{{
			Type: "tool_use",
			Name: "browser",
			ID:   "tool-3",
		}},
	}}

	analysis := analyzeIntent("为什么 gateway 配置有问题")
	promoted := promoteBrowserContinuation(analysis, "为什么 gateway 配置有问题", prior)
	if promoted.Tier != analysis.Tier {
		t.Fatalf("unrelated question should not be promoted, got %q from %q", promoted.Tier, analysis.Tier)
	}
}

// ---------- P3-9: send_media 路由验证 ----------

// TestSendMedia_RoutedToTierWithTool verifies that "桌面上的 logo.png 发给我"
// is classified to a tier where send_media is available.
// P3-4 changed send_media MinTier from task_write to task_light.
// P3-6 moved "发给" from writeKeywords (send_media's IntentKeywords are not
// used for tier classification — IntentPriority=0). The prompt falls through
// to task_light default, where send_media IS available.
func TestSendMedia_RoutedToTierWithTool(t *testing.T) {
	prompts := []string{
		"桌面上的 logo.png 发给我",
		"把这个文件发给我",
		"发送 report.pdf 到飞书群",
	}

	for _, p := range prompts {
		tier := classifyIntent(p)

		// Build a tool set that includes send_media
		tools := mockToolDefs("bash", "read_file", "write_file", "send_media", "search_skills", "lookup_skill")
		filtered := filterToolsByIntent(tools, tier)
		names := toolNames(filtered)

		if !contains(names, "send_media") {
			t.Errorf("classifyIntent(%q) = %q, but send_media is NOT in filtered tools %v",
				p, tier, names)
		}
	}
}

// TestSendMedia_AvailableAtTaskLight verifies send_media is in the tree's
// task_light allowlist after P3-4 MinTier correction.
func TestSendMedia_AvailableAtTaskLight(t *testing.T) {
	tools := mockToolDefs("send_media", "bash", "read_file")
	filtered := filterToolsByIntent(tools, intentTaskLight)
	names := toolNames(filtered)

	if !contains(names, "send_media") {
		t.Errorf("send_media should be available at task_light after P3-4, got tools: %v", names)
	}
}

func TestGateway_NotAvailableAtTaskWrite(t *testing.T) {
	tools := mockToolDefs("gateway", "bash", "read_file")
	filtered := filterToolsByIntent(tools, intentTaskWrite)
	names := toolNames(filtered)

	if contains(names, "gateway") {
		t.Errorf("gateway should not be available at task_write, got tools: %v", names)
	}
}

func TestGateway_AvailableAtTaskMultimodal(t *testing.T) {
	tools := mockToolDefs("gateway", "bash", "read_file")
	filtered := filterToolsByIntent(tools, intentTaskMultimodal)
	names := toolNames(filtered)

	if !contains(names, "gateway") {
		t.Errorf("gateway should be available at task_multimodal, got tools: %v", names)
	}
}

func TestSpecializedConfigTools_AvailableAtTaskWrite(t *testing.T) {
	tools := mockToolDefs("browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config", "bash")
	filtered := filterToolsByIntent(tools, intentTaskWrite)
	names := toolNames(filtered)

	for _, expected := range []string{"browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config"} {
		if !contains(names, expected) {
			t.Errorf("%s should be available at task_write, got tools: %v", expected, names)
		}
	}
}

func TestSpecializedConfigTools_NotAvailableAtTaskLight(t *testing.T) {
	tools := mockToolDefs("browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config", "bash")
	filtered := filterToolsByIntent(tools, intentTaskLight)
	names := toolNames(filtered)

	for _, excluded := range []string{"browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config"} {
		if contains(names, excluded) {
			t.Errorf("%s should not be available at task_light, got tools: %v", excluded, names)
		}
	}
}

func TestIntentGuidance_TaskLightPrefersSendMediaForKnownFiles(t *testing.T) {
	guidance := intentGuidanceText(intentTaskLight)
	if !strings.Contains(guidance, "use 'send_media' directly") {
		t.Fatalf("task_light guidance should prefer send_media for known files, got: %q", guidance)
	}
	if !strings.Contains(guidance, "Do NOT delegate to 'spawn_argus_agent'") {
		t.Fatalf("task_light guidance should forbid argus detours for known files, got: %q", guidance)
	}
	if !strings.Contains(guidance, "data_export approval") {
		t.Fatalf("task_light guidance should mention send_media export approval, got: %q", guidance)
	}
}

func TestIntentGuidance_MultimodalKeepsKnownFileSendOnSendMedia(t *testing.T) {
	guidance := intentGuidanceText(intentTaskMultimodal)
	if !strings.Contains(guidance, "use 'send_media'") {
		t.Fatalf("multimodal guidance should reference send_media for existing files, got: %q", guidance)
	}
	if !strings.Contains(guidance, "Only use 'spawn_argus_agent' if the file must first be discovered") {
		t.Fatalf("multimodal guidance should scope argus to discovery/native interaction, got: %q", guidance)
	}
	if !strings.Contains(guidance, "data_export approval") {
		t.Fatalf("multimodal guidance should mention send_media export approval, got: %q", guidance)
	}
}

// TestSendEmail_RoutedToTierWithTool verifies that email-related prompts
// route to a tier where send_email is available (task_write).
// F-01 regression fix: send_email IntentPriority=10 ensures "邮件" routes to task_write.
func TestSendEmail_RoutedToTierWithTool(t *testing.T) {
	prompts := []struct {
		input string
		want  intentTier
	}{
		{"发送邮件给小明", intentTaskWrite},             // "邮件" → task_write
		{"邮件", intentTaskWrite},                  // "邮件" → task_write
		{"发邮件", intentTaskWrite},                 // "邮件" substring → task_write
		{"帮我写封邮件", intentTaskWrite},              // "写" + "邮件" → task_write
		{"reply email to John", intentTaskWrite}, // "reply email" → task_write
	}

	for _, c := range prompts {
		tier := classifyIntent(c.input)
		if tier != c.want {
			t.Errorf("classifyIntent(%q) = %q, want %q", c.input, tier, c.want)
		}
	}
}

// ---------- S0: 多信号分类器验证矩阵 (2026-03-10) ----------

func TestClassifyIntent_MultiSignal_QuestionShapedTasks(t *testing.T) {
	// 这些 prompt 在旧硬级联架构下被误分流到 question。
	// 多信号评分后应正确路由到 task_light。
	cases := []struct {
		input string
		want  intentTier
		note  string
	}{
		// 动作动词 + 疑问助词 → 动词信号对冲疑问信号
		{"查询下桌面有什么文件", intentTaskLight, "动词(查询)+目标(桌面) 压过 疑问(什么)"},
		{"查看一下日志文件", intentTaskLight, "动词(查看)+目标(日志)"},
		{"看看这个目录下有什么", intentTaskLight, "动词(看看)+目标(目录)"},
		{"列出当前目录的文件", intentTaskLight, "动词(列出)+目标(目录)"},
		{"打开这个文件", intentTaskLight, "动词(打开)"},
		{"可以列一下当前目录吗", intentTaskLight, "动词(列一下)+目标(目录)"},

		// 纯目标信号 + 疑问助词 → 目标信号对冲疑问信号
		{"桌面里有什么文件", intentTaskLight, "目标(桌面) 压过 疑问(什么)"},
		{"能看下日志吗", intentTaskLight, "祈使(能)+目标(日志) 压过 疑问(吗)"},
		{"下载目录里有什么", intentTaskLight, "目标(下载+目录) 压过 疑问(什么)"},

		// 配置目标信号
		{"stt 配置有什么问题", intentTaskWrite, "配置目标+stt writeKeyword → task_write"},

		// 纯疑问句 — 确认不退化
		{"代码是谁写的？", intentQuestion, "纯疑问, 无目标信号"},
		{"什么是 REST API？", intentQuestion, "纯概念查询"},
		{"什么是文件描述符？", intentQuestion, "概念查询, 文件描述符不触发 FS 目标"},
		{"这个API怎么用？", intentQuestion, "纯疑问, 无动作动词/目标"},

		// F-02 根因修复: "文档" 歧义词移除后的回归验证
		{"什么是技术文档？", intentQuestion, "概念查询, 文档不再触发 FS 目标"},
		{"文档目录里有什么", intentTaskLight, "文档目录 — 由 '目录' alias 捕获"},
		{"文档文件夹下的内容", intentTaskLight, "文档文件夹 — 由 '文件夹' alias 捕获"},
	}
	for _, c := range cases {
		if tier := classifyIntent(c.input); tier != c.want {
			t.Errorf("classifyIntent(%q) = %q, want %q (%s)", c.input, tier, c.want, c.note)
		}
	}
}

func TestComputeIntentScore_SignalCombinations(t *testing.T) {
	cases := []struct {
		input    string
		wantSign string // "positive", "negative", "zero"
		note     string
	}{
		// 纯疑问, 无任何正向信号 → 负分
		{"什么是Docker？", "negative", "只有疑问信号"},
		// 疑问 + FS 目标 → 正分
		{"桌面里有什么文件", "positive", "目标(+0.6)压过疑问(-0.3)"},
		// 疑问 + 动作动词 → 正分
		{"查询下有什么问题", "positive", "动词(+0.4)压过疑问(-0.3)"},
		// 疑问 + 祈使 → 正分
		{"帮我看下这是什么", "positive", "祈使(+0.5)压过疑问(-0.3)"},
		// 无信号 → 零分
		{"写个脚本", "zero", "无疑问无目标无祈使无动词"},
	}
	for _, c := range cases {
		lower := strings.ToLower(strings.TrimSpace(c.input))
		score := computeIntentScore(lower, c.input)
		switch c.wantSign {
		case "positive":
			if score <= 0 {
				t.Errorf("computeIntentScore(%q) = %.1f, want positive (%s)", c.input, score, c.note)
			}
		case "negative":
			if score >= 0 {
				t.Errorf("computeIntentScore(%q) = %.1f, want negative (%s)", c.input, score, c.note)
			}
		case "zero":
			if score != 0 {
				t.Errorf("computeIntentScore(%q) = %.1f, want zero (%s)", c.input, score, c.note)
			}
		}
	}
}

// ---------- S2: capability_manage 曝光策略 ----------

func TestCapabilityManage_NotVisibleAtQuestion(t *testing.T) {
	tools := mockToolDefs("capability_manage", "search_skills", "lookup_skill", "bash", "read_file")
	filtered := filterToolsByIntent(tools, intentQuestion)
	names := toolNames(filtered)

	if contains(names, "capability_manage") {
		t.Error("capability_manage should NOT be visible at question tier after S2 fix")
	}
}

func TestCapabilityManage_VisibleAtTaskLight(t *testing.T) {
	tools := mockToolDefs("capability_manage", "search_skills", "lookup_skill", "bash", "read_file")
	filtered := filterToolsByIntent(tools, intentTaskLight)
	names := toolNames(filtered)

	if !contains(names, "capability_manage") {
		t.Error("capability_manage should be visible at task_light tier")
	}
}

// ---------- S3: 端到端路由 + 工具可见性 ----------

func TestE2E_QueryRoutingWithToolVisibility(t *testing.T) {
	allTools := mockToolDefs(
		"bash", "read_file", "write_file", "list_dir", "send_media",
		"search_skills", "lookup_skill", "memory_search", "memory_get",
		"capability_manage", "browser", "gateway",
	)

	cases := []struct {
		input       string
		wantTier    intentTier
		mustInclude []string
		mustExclude []string
	}{
		{
			"查询下桌面有什么文件", intentTaskLight,
			[]string{"list_dir", "bash", "read_file"},
			[]string{},
		},
		{
			"桌面里有什么文件", intentTaskLight,
			[]string{"list_dir", "bash"},
			[]string{},
		},
		{
			"代码是谁写的？", intentQuestion,
			[]string{"search_skills"},
			[]string{"bash", "list_dir", "capability_manage"},
		},
		{
			"什么是 REST API？", intentQuestion,
			[]string{"search_skills"},
			[]string{"bash", "capability_manage"},
		},
		{
			"删除 test.txt", intentTaskDelete,
			[]string{"bash"},
			[]string{},
		},
	}

	for _, c := range cases {
		tier := classifyIntent(c.input)
		if tier != c.wantTier {
			t.Errorf("classifyIntent(%q) = %q, want %q", c.input, tier, c.wantTier)
			continue
		}

		filtered := filterToolsByIntent(allTools, tier)
		names := toolNames(filtered)

		for _, must := range c.mustInclude {
			if !contains(names, must) {
				t.Errorf("%q at tier %q: expected tool %q in %v", c.input, tier, must, names)
			}
		}
		for _, excluded := range c.mustExclude {
			if contains(names, excluded) {
				t.Errorf("%q at tier %q: unexpected tool %q in %v", c.input, tier, excluded, names)
			}
		}
	}
}

func TestIntentGuidance_QuestionNeverClaimLackCapabilities(t *testing.T) {
	guidance := intentGuidanceText(intentQuestion)
	if !strings.Contains(guidance, "NEVER claim you lack capabilities") {
		t.Fatal("question guidance must include NEVER-claim-lack guardrail")
	}
}

// ---------- helpers ----------

func mockToolDefs(names ...string) []llmclient.ToolDef {
	var defs []llmclient.ToolDef
	for _, n := range names {
		defs = append(defs, llmclient.ToolDef{Name: n})
	}
	return defs
}

func toolNames(defs []llmclient.ToolDef) []string {
	var names []string
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
