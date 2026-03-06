package runner

// intent_router.go — 五维联动架构核心：六级意图分类 + 工具-技能绑定 + 行为指引注入
//
// 架构 v2 核心组件。替代旧三级 (greeting/chat/task) 关键词匹配，
// 实现精细化意图路由，联动工具树、技能树、安全层和提示词系统。
//
// 行业对齐:
//   - Anthropic Progressive Disclosure: 按意图逐级暴露工具
//   - Google ADK Dispatcher Pattern: 意图驱动路由
//   - OpenAI Agents SDK Manager Pattern: 任务委托
//   - Microsoft Zero Trust: 操作分级审批

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
)

// ---------- 意图分级 ----------

// intentTier 六级意图分类（决定工具暴露 + 历史裁剪 + 行为指引）。
type intentTier string

const (
	intentGreeting       intentTier = "greeting"        // 问候 → 零工具
	intentQuestion       intentTier = "question"        // 提问/回顾 → 搜索工具
	intentTaskLight      intentTier = "task_light"      // 轻量查看 → 读取 + bash
	intentTaskWrite      intentTier = "task_write"      // 创建/修改 → 写入 + coder 委托
	intentTaskDelete     intentTier = "task_delete"     // 破坏性操作 → bash(带审批)
	intentTaskMultimodal intentTier = "task_multimodal" // 视觉/浏览器 → 全部工具
)

// classifyIntent 根据用户 prompt 快速分类意图（六级）。
// 优先级: greeting > question > task_delete > task_multimodal > task_write > task_light
//
// Stage 1 纯规则路由（零 LLM 成本，<1ms）。
// Stage 2 轻量 LLM 分类（预留接口，当前未启用）。
func classifyIntent(prompt string) intentTier {
	trimmed := strings.TrimSpace(prompt)
	runes := []rune(trimmed)
	lower := strings.ToLower(trimmed)

	// ── 1. Greeting: 短文本 + 匹配问候词 ──
	if len(runes) <= 10 {
		greetings := []string{
			"你好", "hi", "hello", "嗨", "hey",
			"早上好", "下午好", "晚上好", "早安", "晚安",
			"good morning", "good afternoon", "good evening",
			"哈喽", "嘿", "在吗", "在不在", "nihao",
		}
		for _, g := range greetings {
			if lower == g {
				return intentGreeting
			}
		}
	}

	// ── 2. Question: 疑问标记 + 无祈使前缀 + 无动作动词 ──
	// 核心设计: 先检测提问再检测任务，避免 "代码是谁写的？" 被误判为 task_write
	// Bug#11 修复: 动作动词兜底 — 即使句式是疑问且无祈使前缀，包含诊断/执行类动词也应归类为任务
	if isInterrogative(lower) && !hasImperativePrefix(lower) {
		if !containsActionVerb(lower) {
			return intentQuestion
		}
		// 包含动作动词 → 跳过 question，继续走关键词匹配
	}

	// ── 3. Task Delete: 破坏性操作关键词 ──
	// 高安全优先级，确保删除类意图被正确捕获
	if containsAnyKeyword(lower, deleteKeywords) {
		return intentTaskDelete
	}

	// ── 4. Task Multimodal: 视觉/浏览器交互关键词 ──
	if containsAnyKeyword(lower, multimodalKeywords) {
		return intentTaskMultimodal
	}

	// ── 5. Task Write: 创建/修改关键词 ──
	if containsAnyKeyword(lower, writeKeywords) {
		return intentTaskWrite
	}

	// ── 6. Default: Task Light ──
	// 比旧架构的 intentChat 更安全 — 提供 bash + 读取工具，不会陷入死循环
	return intentTaskLight
}

// ---------- 关键词集 ----------

var deleteKeywords = []string{
	"删除", "删掉", "删了", "删",
	"移除", "清理", "清除", "清掉",
	"remove", "delete", "rm ",
}

var multimodalKeywords = []string{
	"截屏", "截图", "截个", "截一", "屏幕截",
	"拍照", "拍个照", "拍一张",
	"screenshot", "capture screen", "capture ",
	"浏览器", "网页", "browser",
	"灵瞳", "argus",
}

var writeKeywords = []string{
	"写", "创建", "编写", "开发", "修改", "改一", "改个",
	"修", "添加", "新增", "生成", "实现", "重构", "优化",
	"部署", "配置", "安装", "升级",
	"发送", "发给", "发消息", "通知",
	"write", "create", "code", "develop", "build", "fix",
	"add", "modify", "implement", "generate", "refactor",
	"deploy", "configure", "install", "upgrade", "send",
}

// ---------- 疑问 / 祈使检测 ----------

// isInterrogative 检测是否为疑问句。
func isInterrogative(lower string) bool {
	// 末尾疑问标记
	if strings.HasSuffix(lower, "？") || strings.HasSuffix(lower, "?") {
		return true
	}

	// 中文疑问助词
	interrogativeParticles := []string{
		"吗", "呢", "么",
		"是不是", "能不能", "能否", "有没有",
		"什么", "怎么", "为什么", "哪个", "哪里", "几个", "多少",
	}
	for _, p := range interrogativeParticles {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// 英文疑问词开头
	englishQuestionStarts := []string{
		"what ", "how ", "why ", "where ", "when ", "which ",
		"is ", "are ", "was ", "were ", "did ", "does ", "do ",
		"can ", "could ", "would ", "should ", "will ",
	}
	for _, qs := range englishQuestionStarts {
		if strings.HasPrefix(lower, qs) {
			return true
		}
	}

	return false
}

// hasImperativePrefix 检测是否有祈使/命令标记。
// 祈使标记表示用户在下达指令（非提问），即使句子包含疑问标记也应归类为任务。
//
// Bug#4 修复: "帮我"等强任务标记不仅检查前缀，还检查嵌入位置。
// 例: "嗨，你帮我看下系统资源？" — "帮我"不在开头但明确表示任务委托。
func hasImperativePrefix(lower string) bool {
	// 1. 前缀检测（"给我"/"来" 太通用，仅做前缀匹配）
	prefixOnly := []string{
		"给我", "来",
	}
	for _, p := range prefixOnly {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}

	// 2. 强任务标记: 无论出现在前缀还是句中都表示任务委托
	// "帮我X" = "替我做X"，"帮忙X" = "请做X"，"请帮" = "please help"，"替我" = "on my behalf"
	// Bug#11 修复: 增加中文礼貌祈使句 + 英文对应模式
	strongMarkers := []string{
		"帮我", "帮忙", "麻烦", "请帮", "替我",
		// 礼貌祈使: "你能X吗"/"能不能X"/"可以帮X"/"可以做X"/"能否X"
		"你能", "能不能", "可以帮", "可以做", "能否",
		// 英文礼貌祈使
		"can you", "could you", "would you",
	}
	for _, m := range strongMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}

	// 3. "请" 前缀特殊处理: "请问" 是疑问，"请完成/请执行/请删除" 是祈使
	if strings.HasPrefix(lower, "请") && !strings.HasPrefix(lower, "请问") {
		return true
	}

	return false
}

// ---------- 工具过滤 ----------

// filterToolsByIntent 按意图分级过滤工具。
// 核心设计: 每个 tier 暴露 3-8 个工具，约束 LLM 输出收敛性。
//
// | Tier            | 工具数 | 策略                    |
// |-----------------|--------|------------------------|
// | greeting        | 0      | 纯文字回复              |
// | question        | 3-5    | 搜索 + 技能查找          |
// | task_light      | 6-9    | 读取 + bash + 搜索       |
// | task_write      | 8-12   | + 写入 + coder 委托 + 截屏 |
// | task_delete     | 4-6    | bash(带审批) + 读取      |
// | task_multimodal | 全部    | 含 argus/browser        |
func filterToolsByIntent(tools []llmclient.ToolDef, tier intentTier) []llmclient.ToolDef {
	if tier == intentGreeting {
		return nil
	}
	if tier == intentTaskMultimodal {
		// Phase 5: task_multimodal 移除直接 argus_* 工具（除 capture_screen），
		// 改由 spawn_argus_agent 子智能体提供完整视觉操作能力。
		filtered := make([]llmclient.ToolDef, 0, len(tools))
		for _, t := range tools {
			if strings.HasPrefix(t.Name, "argus_") && t.Name != "argus_capture_screen" {
				continue // 跳过直接 argus 工具
			}
			filtered = append(filtered, t)
		}
		return filtered
	}

	allowed := tierToolAllowlist[tier]
	if allowed == nil {
		return tools // safety fallback
	}

	filtered := make([]llmclient.ToolDef, 0, len(allowed))
	for _, t := range tools {
		name := t.Name

		// argus_* 前缀工具: 仅 task_write(只允许截屏) 和 task_multimodal(全部)
		if strings.HasPrefix(name, "argus_") {
			if tier == intentTaskWrite && name == "argus_capture_screen" {
				filtered = append(filtered, t)
			}
			// 其他 tier 不暴露 argus 工具
			continue
		}

		// remote_* 前缀工具: task_write 及以上
		if strings.HasPrefix(name, "remote_") {
			if tier == intentTaskWrite {
				filtered = append(filtered, t)
			}
			continue
		}

		// 普通工具: 按 allowlist 过滤
		if allowed[name] {
			filtered = append(filtered, t)
		}
	}

	return filtered
}

// tierToolAllowlist 每个意图层级的工具白名单。
// nil = 全量通过（用于 task_multimodal 和 fallback）。
var tierToolAllowlist = map[intentTier]map[string]bool{
	intentQuestion: {
		"search_skills": true,
		"lookup_skill":  true,
		"memory_search": true,
		"memory_get":    true,
	},
	intentTaskLight: {
		"bash":            true,
		"read_file":       true,
		"list_dir":        true,
		"search":          true,
		"grep":            true,
		"glob":            true,
		"web_search":      true,
		"search_skills":   true,
		"lookup_skill":    true,
		"memory_search":   true,
		"memory_get":      true,
		"report_progress": true,
	},
	intentTaskWrite: {
		"bash":              true,
		"read_file":         true,
		"write_file":        true,
		"list_dir":          true,
		"search":            true,
		"grep":              true,
		"glob":              true,
		"web_search":        true,
		"browser":           true,
		"send_media":        true,
		"spawn_coder_agent": true,
		"spawn_media_agent": true,
		"search_skills":     true,
		"lookup_skill":      true,
		"memory_search":     true,
		"memory_get":        true,
		"report_progress":   true,
	},
	intentTaskDelete: {
		"bash":            true,
		"read_file":       true,
		"list_dir":        true,
		"search_skills":   true,
		"lookup_skill":    true,
		"report_progress": true,
	},
}

// ---------- 历史裁剪 ----------

// trimHistoryByIntent 按意图裁剪历史消息，减少不必要的 token 消耗。
//
//   - greeting: 不加载历史（boot brief 在系统提示中提供上下文感知）
//   - question: 最近 4 条消息（2 轮对话，足够回溯上下文回答问题）
//   - task_delete: 全量（需要完整路径历史做安全确认）
//   - task_*: 不裁剪（保持完整上下文供工具决策）
func trimHistoryByIntent(messages []llmclient.ChatMessage, tier intentTier) []llmclient.ChatMessage {
	switch tier {
	case intentGreeting:
		return nil
	case intentQuestion:
		// 比旧 chat 的 2 条多一些（4 条 = 2 轮对话），帮助回答回顾性问题
		if len(messages) > 4 {
			return messages[len(messages)-4:]
		}
		return messages
	default:
		return messages
	}
}

// ---------- 行为指引 ----------

// intentGuidanceText 返回当前意图层级的行为指引文本。
// 注入到系统提示词中，引导 LLM 在特定意图下采取最优策略。
// 这是提示词系统维度与意图维度的联动点。
func intentGuidanceText(tier intentTier) string {
	switch tier {
	case intentGreeting:
		return "" // 无需指引
	case intentQuestion:
		return `## Intent Guidance (Question Mode)
This is an informational question, NOT a task execution request.
- Answer from conversation history and memory FIRST — do not call tools to verify known information.
- If the answer exists in prior messages, respond directly without tool calls.
- Only use search tools if the history is truly insufficient.
- Keep responses concise (under 200 chars for simple factual questions).`
	case intentTaskLight:
		return `## Intent Guidance (Light Task Mode)
This is a read/check operation.
- For system status checks (memory, CPU, disk, processes): use bash directly with standard commands (top, vm_stat, df, ps, sysctl, etc.). Do NOT search for skills first.
- Use known file paths from history — avoid broad searches like 'find ~'.
- Prefer read_file/list_dir for direct access over bash for file operations.
- If the user is checking status, provide a brief summary.
- When the user's request matches a known skill topic (e.g., system diagnostics, deployment, debugging, monitoring), use search_skills first to leverage specialized knowledge and best practices.
- For common system commands (ls, top, df, cat, grep, etc.), use tools directly without searching skills.
- NEVER execute system diagnostics, service start/stop/restart, or environment repair commands that were NOT explicitly requested by the user.`
	case intentTaskWrite:
		return `## Intent Guidance (Write Task Mode)
This is a creation/modification task.
- Simple edits (single-file, <50 lines, clear target): execute directly with write_file/bash tools.
- Complex/multi-file coding tasks: delegate to spawn_coder_agent (Open Coder).
- Media operations (content creation, publishing, trending analysis): delegate to spawn_media_agent.
- After creating visual artifacts (HTML/web), use browser screenshot for verification.
- Combine related write operations into fewer steps.
- Complex tasks may go through plan confirmation — wait for user approval before executing.`
	case intentTaskDelete:
		return `## Intent Guidance (Delete Task Mode)
This is a destructive operation requiring caution.
- Use file paths from conversation history — NEVER use 'find ~' or broad searches.
  The target path should already be known from prior context.
- Combine deletion steps into one command (e.g., 'rm file && rmdir dir').
- Deletion commands will trigger security approval — this is expected behavior.
- After deletion, a brief confirmation is sufficient (no need for ls verification).
- Destructive operations require plan confirmation — wait for user approval before executing.`
	case intentTaskMultimodal:
		return `## Intent Guidance (Multimodal Task Mode)
This task involves visual or browser interaction.
- For opening web pages / URLs: ALWAYS use 'browser' tool with 'navigate' action. NEVER use bash 'open' command for URLs.
- For web page screenshots: use 'browser' tool with 'screenshot' action (NOT argus_capture_screen).
- For web page interaction (click, type, scroll): use 'browser' tool with CSS selectors or ARIA refs.
- For complex multi-step web tasks: use 'browser' tool with 'ai_browse' action.
- For desktop application interaction (native apps, not web): use 'spawn_argus_agent' to delegate to 灵瞳 sub-agent.
- For full desktop screenshots (not web): use 'argus_capture_screen' directly.
- Rule: if the target is a URL or web page, use 'browser'. Only use argus for native desktop apps.
- Complex multimodal tasks may go through plan confirmation — wait for user approval before executing.`
	default:
		return ""
	}
}

// ---------- 方案确认门控 ----------

// needsPlanConfirmation 判断当前意图层级是否需要方案确认门控。
// task_write / task_delete / task_multimodal 需要用户确认方案后才执行。
// greeting / question / task_light 直接处理，不走门控。
func needsPlanConfirmation(tier intentTier) bool {
	switch tier {
	case intentTaskWrite, intentTaskDelete, intentTaskMultimodal:
		return true
	default:
		return false
	}
}

// ---------- 辅助函数 ----------

// ---------- 动作动词检测（Bug#11 修复） ----------

// actionVerbs 诊断/执行类动作动词 — 表示用户期望 agent 采取行动，
// 即使句式是疑问也不应归类为 question。
var actionVerbs = []string{
	// 诊断类
	"排查", "诊断", "调试", "检查", "分析", "定位", "排错", "修复",
	"troubleshoot", "investigate", "diagnose", "debug",
	// 执行类
	"执行", "运行", "启动", "停止", "重启", "部署", "安装", "卸载",
	"execute", "restart",
}

// containsActionVerb 检测消息是否包含动作动词。
func containsActionVerb(lower string) bool {
	return containsAnyKeyword(lower, actionVerbs)
}

// containsAnyKeyword 检查 lower 是否包含关键词列表中的任何一个。
func containsAnyKeyword(lower string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
