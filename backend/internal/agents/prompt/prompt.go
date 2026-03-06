package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ---------- 系统提示词构建器 ----------

// TS 参考: src/agents/system-prompt.ts (649 行) + system-prompt-params.ts (116 行)

// PromptMode 控制提示词中包含的硬编码段落级别。
type PromptMode string

const (
	PromptModeFull    PromptMode = "full"    // 所有段落（主 agent）
	PromptModeMinimal PromptMode = "minimal" // 精简段落（子 agent）
	PromptModeNone    PromptMode = "none"    // 仅基础身份行
)

// SilentReplyToken 静默回复标记。TS 对应: auto-reply/tokens.ts → SILENT_REPLY_TOKEN
const SilentReplyToken = "NO_REPLY"

// SessionState 会话状态（状态机路由核心）。
type SessionState string

const (
	SessionColdStart SessionState = "COLD_START" // 全新用户初次见面（无历史、无 boot brief）
	SessionWarmStart SessionState = "WARM_START" // 系统重启/二次唤醒（无 assistant 历史，有 boot brief）
	SessionNormal    SessionState = "NORMAL"     // 常规连续对话
)

// ---------- 新增类型 ----------

// SandboxInfo 沙箱环境信息。
type SandboxInfo struct {
	Enabled             bool
	WorkspaceDir        string
	WorkspaceAccess     string // "none"|"ro"|"rw"
	AgentWorkspaceMount string
	BrowserBridgeURL    string
	BrowserNoVncURL     string
	HostBrowserAllowed  *bool // nil=unknown
	Elevated            *SandboxElevated
}

// SandboxElevated 沙箱提权配置。
type SandboxElevated struct {
	Allowed      bool
	DefaultLevel string // "on"|"off"|"ask"|"full"
}

// ContextFile 注入的上下文文件。
type ContextFile struct {
	Path    string
	Content string
}

// ReactionGuidance 反应指导配置。
type ReactionGuidance struct {
	Level   string // "minimal"|"extensive"
	Channel string
}

// ---------- Runtime 参数 ----------

// RuntimeInfo 运行时信息。
// TS 参考: system-prompt-params.ts → RuntimeInfoInput
type RuntimeInfo struct {
	AgentID      string   `json:"agentId,omitempty"`
	Host         string   `json:"host"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	GoVersion    string   `json:"goVersion"`
	Model        string   `json:"model"`
	DefaultModel string   `json:"defaultModel,omitempty"`
	Shell        string   `json:"shell,omitempty"`
	Channel      string   `json:"channel,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	RepoRoot     string   `json:"repoRoot,omitempty"`
}

// SystemPromptParams 系统提示词构建参数。
type SystemPromptParams struct {
	RuntimeInfo  RuntimeInfo
	UserTimezone string
	UserTime     string
}

// BuildSystemPromptParams 构建系统提示词运行时参数。
// TS 参考: system-prompt-params.ts → buildSystemPromptParams()
func BuildSystemPromptParams(agentID string, rt RuntimeInfo, workspaceDir, cwd, userTimezone string) SystemPromptParams {
	// 解析 repo root
	repoRoot := resolveRepoRoot(workspaceDir, cwd)
	if repoRoot != "" {
		rt.RepoRoot = repoRoot
	}
	rt.AgentID = agentID

	// 时区
	tz := userTimezone
	if tz == "" {
		tz = resolveLocalTimezone()
	}

	// 当前时间
	userTime := formatUserTime(time.Now(), tz)

	return SystemPromptParams{
		RuntimeInfo:  rt,
		UserTimezone: tz,
		UserTime:     userTime,
	}
}

// ---------- 提示词构建 ----------

// BuildParams 构建系统提示词的完整参数集。
type BuildParams struct {
	Mode                    PromptMode
	WorkspaceDir            string
	ExtraSystemPrompt       string            // 用户自定义追加
	SkillsPrompt            string            // 技能注入段
	OwnerLine               string            // 用户身份行
	ToolNames               []string          // 可用工具名称
	ToolSummaries           map[string]string // 工具名→描述映射
	ModelAliasLines         []string          // 模型别名行
	HeartbeatPrompt         string            // 心跳提示词
	DocsPath                string            // 文档路径
	WorkspaceNotes          []string          // 工作区备注
	TTSHint                 string            // TTS 提示
	SandboxInfo             *SandboxInfo      // 沙箱信息
	ContextFiles            []ContextFile     // 注入的上下文文件
	ReasoningTagHint        bool              // 是否启用 <think>/<final> 标签
	ReasoningLevel          string            // off|on|stream
	MessageToolHints        []string          // 消息工具附加提示
	ReactionGuidance        *ReactionGuidance // 反应指导
	MemoryCitations         string            // off|on
	RuntimeInfo             *RuntimeInfo
	UserTimezone            string
	ThinkLevel              string       // "off"|"low"|"medium"|"high"
	BootContextBrief        string       // Agent 启动上下文简报（上次工作摘要 ~200 tokens）
	SessionState            SessionState // COLD_START | WARM_START | NORMAL（状态机路由）
	IntentGuidance          string       // 意图行为指引（五维联动：按意图层级动态注入）
	PlanConfirmationEnabled bool         // Phase 1: 方案确认门控是否启用（三级指挥体系）
}

// BuildAgentSystemPrompt 构建 Agent 系统提示词。
// TS 参考: system-prompt.ts → buildAgentSystemPrompt() (649L)
func BuildAgentSystemPrompt(params BuildParams) string {
	mode := params.Mode
	if mode == "" {
		mode = PromptModeFull
	}
	isMinimal := mode == PromptModeMinimal || mode == PromptModeNone

	// "none" 模式: 仅返回身份行
	if mode == PromptModeNone {
		return "You are a personal assistant running inside Claw Acosmi."
	}

	// 构建可用工具集
	available := make(map[string]bool)
	for _, t := range params.ToolNames {
		available[strings.ToLower(strings.TrimSpace(t))] = true
	}
	hasGateway := available["gateway"]
	readToolName := "read"

	var sections []string
	add := func(s string) {
		if s != "" {
			sections = append(sections, s)
		}
	}

	// 0. System Context（最顶部，最高位置权重）
	add(buildSystemContextBlock(params.SessionState, params.UserTimezone, params.BootContextBrief))

	// 1. 身份行
	add("You are **创宇太虚（Claw Acosmi）**, an AI agent running on the Claw Acosmi platform.\n" +
		"You assist with software engineering, knowledge management, and enterprise workflows using the tools below.")

	// 1a. 核心交互路由（状态机，替代旧 Cold Start + Response Style）
	add(buildInteractionRouting())

	// 1b. 运行准则
	add("## Operating Principles\n" +
		"- **Language**: 用中文回复，技术术语保留英文。用户用英文则用英文回复。\n" +
		"- **Action**: 可逆操作（编辑、搜索、分析）直接执行；不可逆操作（删除文件、部署、修改权限）先告知再执行。不确定意图时推断最可能的有用动作。\n" +
		"- **Honesty**: 绝不编造工具调用结果、文件内容或代码输出。不确定时明确说明。缺少工具时直接告知。\n" +
		"- **Objectivity**: 技术准确优先于迎合。有分歧直说，有不确定先查证再回应。不附加不必要的夸赞或情感认同。\n" +
		"- **Scope**: 优先编辑已有文件，不创建不必要的新文件。只做用户要求的改动，不做未要求的「改进」。\n" +
		"- **Emoji**: 不使用，除非用户明确要求。")

	// 1c. 任务执行
	add("## Task Execution\n" +
		"- 简短指令 = 充分方向。通过查阅代码和现有约定推断缺失细节。\n" +
		"- 只在真正阻塞时提问（检查上下文后仍无法安全选择默认值）。阻塞条件：\n" +
		"  * 歧义会实质性改变结果，且无法通过读代码消歧\n" +
		"  * 操作不可逆、涉及生产环境、或改变安全态势\n" +
		"  * 需要无法推断的密钥/凭据\n" +
		"- 必须提问时：先完成所有非阻塞工作，问一个精准问题，附推荐默认值。\n" +
		"- 不问许可式问题——直接执行最合理方案，告知结果。\n" +
		"- Context window 接近上限时系统会自动压缩历史消息。不要因 token 预算担忧而提前终止任务。")

	// 2. Tooling
	add(buildToolingSection(params.ToolNames, params.ToolSummaries))
	// 3. Tool Call Style
	add(buildToolCallStyleSection())
	// 3.5 Delegation Guidance (Phase 6: 仅在 spawn_coder_agent 可用时注入)
	if !isMinimal && available["spawn_coder_agent"] {
		add(buildDelegationGuidanceSection())
	}
	// 3.6 三级指挥体系（Phase 1: 仅在方案确认门控可用时注入，非 minimal 模式）
	if !isMinimal && params.PlanConfirmationEnabled {
		add(buildPlanGenerationSection())
	}
	// 4. Safety
	add(buildSafetySection())
	// 5. CLI Quick Reference
	add(buildCLISection())
	// 6. Skills
	add(buildSkillsSectionFull(params.SkillsPrompt, isMinimal, readToolName))
	// 6b. Boot Context Brief — 已整合到 System_Context 的 [Last_Summary] 中
	// 7. Memory Recall
	add(buildMemorySectionFull(available, params.MemoryCitations))
	// 8. Self-Update
	add(buildSelfUpdateSection(hasGateway, isMinimal))
	// 9. Model Aliases
	add(buildModelAliasesSection(params.ModelAliasLines, isMinimal))
	// 10. Workspace
	if params.WorkspaceDir != "" {
		ws := fmt.Sprintf("## Workspace\nYour working directory is: %s\nTreat this directory as the single global workspace.", params.WorkspaceDir)
		for _, note := range params.WorkspaceNotes {
			if n := strings.TrimSpace(note); n != "" {
				ws += "\n" + n
			}
		}
		add(ws)
	}
	// 11. Docs
	add(buildDocsSection(params.DocsPath, isMinimal))
	// 12. Sandbox
	add(buildSandboxSection(params.SandboxInfo))
	// 13. User Identity
	if params.OwnerLine != "" && !isMinimal {
		add(buildUserIdentitySection(params.OwnerLine))
	}
	// 14. Time
	if params.UserTimezone != "" {
		add(buildTimeSection(params.UserTimezone))
	}
	// 14b. Workspace Files (injected) — TS L504-506
	if len(params.ContextFiles) > 0 {
		add("## Workspace Files (injected)\n" +
			"These user-editable files are loaded by Claw Acosmi and included below in Project Context.")
	}
	// 15. Reply Tags
	add(buildReplyTagsSection(isMinimal))
	// 16. Messaging
	add(buildMessagingSection(isMinimal, available, params.MessageToolHints))
	// 17. Voice/TTS
	add(buildVoiceSection(isMinimal, params.TTSHint))

	// Intent Guidance (五维联动: 意图→行为指引动态注入)
	if params.IntentGuidance != "" {
		add(params.IntentGuidance)
	}

	// Extra system prompt
	if params.ExtraSystemPrompt != "" {
		header := "## Group Chat Context"
		if mode == PromptModeMinimal {
			header = "## Subagent Context"
		}
		add(header + "\n" + params.ExtraSystemPrompt)
	}

	// Reactions
	add(buildReactionsSection(params.ReactionGuidance))
	// Reasoning Format
	add(buildReasoningFormatSection(params.ReasoningTagHint))
	// Context Files
	add(buildContextFilesSection(params.ContextFiles))

	// Silent Replies (full mode only)
	if !isMinimal {
		add(buildSilentRepliesSection())
	}
	// Heartbeats (full mode only)
	if !isMinimal {
		add(buildHeartbeatsSection(params.HeartbeatPrompt))
	}

	// Runtime (always last)
	if params.RuntimeInfo != nil {
		add(buildRuntimeLine(params.RuntimeInfo, params.ThinkLevel))
	}
	reasoningLevel := params.ReasoningLevel
	if reasoningLevel == "" {
		reasoningLevel = "off"
	}
	add(fmt.Sprintf("Reasoning: %s (hidden unless on/stream).", reasoningLevel))

	return joinSections(sections)
}

// ---------- 状态机路由构建器 ----------

// buildSystemContextBlock 构建顶部结构化上下文块（最高位置权重）。
func buildSystemContextBlock(state SessionState, timezone string, bootBrief string) string {
	if state == "" {
		state = SessionNormal
	}

	// Environment: 时间 + 时区
	env := "未配置"
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err == nil {
			t := time.Now().In(loc)
			env = t.Format("2006-01-02 15:04 (Monday)") + " " + timezone
		}
	}

	// Last Summary
	summary := "无"
	if brief := strings.TrimSpace(bootBrief); brief != "" {
		summary = brief
	}

	return fmt.Sprintf("<System_Context>\n"+
		"  [Session_State]: %s\n"+
		"  [Environment]: %s\n"+
		"  [Last_Summary]: %s\n"+
		"</System_Context>", state, env, summary)
}

// buildInteractionRouting 构建核心交互路由段落（状态机协议 A/B/C）。
func buildInteractionRouting() string {
	return "## Core Interaction Routing\n" +
		"读取顶部 <System_Context> 中的 [Session_State]，严格执行唯一对应的协议：\n\n" +

		"### 协议 A：COLD_START（全新用户初次见面）\n" +
		"使用 `lookup_skill` 查找 `acosmi-intro` 技能获取介绍内容，做一次专业且有温度的系统推介。\n" +
		"约束：总字数 ≤ 300。\n" +
		"结构：1. 破冰问候 2. 核心定位（1 句） 3. 高光优势（3-4 个列表项） 4. 交互引导（开放式问句）\n" +
		"非问候消息直接处理任务。\n\n" +

		"### 协议 B：WARM_START（重启唤醒/二次回归）\n" +
		"执行老友重逢与进度唤醒协议。约束：语气干练贴心，总字数 ≤ 250。\n" +
		"结构（缺失变量自然跳过）：\n" +
		"1. 环境问候：结合 [Environment] 向用户问好\n" +
		"2. 进度回顾：读取 [Last_Summary]，1 句话唤醒记忆\n" +
		"3. 身份就绪：极简一句宣布就绪\n" +
		"注意：仅询问是否继续，**禁止擅自执行**代码或任务。\n\n" +

		"### 协议 C：NORMAL（常规连续对话）\n" +
		"日常极客模式：\n" +
		"- 回复长度与请求复杂度**严格成正比**。\n" +
		"- 问候/闲聊 → 1-3 句自然回复。\n" +
		"- 简单问题 → 直答，≤ 5 句。\n" +
		"- 代码任务 → 改了什么 → 在哪 → 为什么。\n" +
		"- 复杂分析 → 分段标题组织。\n" +
		"- 禁止：重复自我介绍、主动列举能力、添加收尾套话、主动执行历史未完成任务。\n" +
		"- Markdown 格式。文件引用: `path/to/file.go:42`。不 dump 大段文件/代码。"
}

// ---------- 段落构建器 ----------

func buildUserIdentitySection(ownerLine string) string {
	return fmt.Sprintf("## User Identity\n%s", ownerLine)
}

func buildTimeSection(timezone string) string {
	t := time.Now()
	loc, err := time.LoadLocation(timezone)
	if err == nil {
		t = t.In(loc)
	}
	return fmt.Sprintf("## Current Time\n%s (%s)", t.Format("2006-01-02 15:04:05"), timezone)
}

func buildRuntimeLine(rt *RuntimeInfo, thinkLevel string) string {
	parts := []string{}
	if rt.Host != "" {
		parts = append(parts, fmt.Sprintf("Host: %s", rt.Host))
	}
	if rt.OS != "" {
		parts = append(parts, fmt.Sprintf("OS: %s/%s", rt.OS, rt.Arch))
	}
	if rt.Model != "" {
		parts = append(parts, fmt.Sprintf("Model: %s", rt.Model))
	}
	if rt.Shell != "" {
		parts = append(parts, fmt.Sprintf("Shell: %s", rt.Shell))
	}
	if rt.RepoRoot != "" {
		parts = append(parts, fmt.Sprintf("Repo: %s", rt.RepoRoot))
	}
	if thinkLevel != "" && thinkLevel != "off" {
		parts = append(parts, fmt.Sprintf("Thinking: %s", thinkLevel))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("## Runtime\n%s", strings.Join(parts, " | "))
}

// ---------- RepoRoot 解析 ----------

// ResolveRepoRoot 从配置和工作路径中解析 Git 仓库根目录。
// TS 参考: system-prompt-params.ts → resolveRepoRoot()
func resolveRepoRoot(workspaceDir, cwd string) string {
	candidates := []string{workspaceDir, cwd}
	seen := make(map[string]bool)

	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		root := FindGitRoot(abs)
		if root != "" {
			return root
		}
	}
	return ""
}

// FindGitRoot 向上查找 .git 目录。
// TS 参考: system-prompt-params.ts → findGitRoot()
func FindGitRoot(startDir string) string {
	current, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for i := 0; i < 12; i++ {
		gitPath := filepath.Join(current, ".git")
		info, err := os.Stat(gitPath)
		if err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func resolveLocalTimezone() string {
	return time.Now().Location().String()
}

func formatUserTime(t time.Time, timezone string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return t.Format("2006-01-02 15:04:05")
	}
	return t.In(loc).Format("2006-01-02 15:04:05")
}

func joinSections(sections []string) string {
	return strings.Join(sections, "\n\n")
}

// DefaultRuntimeInfo 创建带默认值的运行时信息。
func DefaultRuntimeInfo() RuntimeInfo {
	hostname, _ := os.Hostname()
	shell := os.Getenv("SHELL")
	return RuntimeInfo{
		Host:      hostname,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		Shell:     shell,
	}
}
