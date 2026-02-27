package runner

// ============================================================================
// 子 Agent 通告辅助函数
// 对应 TS: agents/subagent-announce.ts L29-358
// ============================================================================

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// --- 格式化函数 ---

// FormatTokenCount 格式化 token 数量为人类可读形式。
func FormatTokenCount(value int) string {
	if value <= 0 {
		return "0"
	}
	if value >= 1_000_000 {
		return fmt.Sprintf("%.1fm", float64(value)/1_000_000)
	}
	if value >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(value)/1_000)
	}
	return fmt.Sprintf("%d", value)
}

// FormatUsd 格式化美元金额。
// TS 对照: subagent-announce.ts L46-52
func FormatUsd(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	if value >= 1.0 {
		return fmt.Sprintf("$%.2f", value)
	}
	if value >= 0.01 {
		return fmt.Sprintf("$%.2f", value)
	}
	return fmt.Sprintf("$%.4f", value)
}

// FormatDurationCompact 格式化持续时间为紧凑形式。
func FormatDurationCompact(d time.Duration) string {
	if d <= 0 {
		return "n/a"
	}
	secs := d.Seconds()
	if secs < 60 {
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := int(secs) / 60
	remainSecs := int(secs) % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%ds", mins, remainSecs)
	}
	hours := mins / 60
	remainMins := mins % 60
	return fmt.Sprintf("%dh%dm", hours, remainMins)
}

// --- 统计信息 ---

// SubagentStats 子 Agent 运行统计。
type SubagentStats struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	RuntimeMs    int64
	Provider     string
	Model        string
	SessionKey   string
	SessionID    string
}

// BuildSubagentStatsLine 构建统计信息行。
func BuildSubagentStatsLine(stats SubagentStats) string {
	var parts []string

	// 运行时间
	if stats.RuntimeMs > 0 {
		d := time.Duration(stats.RuntimeMs) * time.Millisecond
		parts = append(parts, "runtime "+FormatDurationCompact(d))
	} else {
		parts = append(parts, "runtime n/a")
	}

	// Token 计数
	total := stats.TotalTokens
	if total == 0 && stats.InputTokens > 0 && stats.OutputTokens > 0 {
		total = stats.InputTokens + stats.OutputTokens
	}
	if total > 0 {
		parts = append(parts, fmt.Sprintf(
			"tokens %s (in %s / out %s)",
			FormatTokenCount(total),
			FormatTokenCount(stats.InputTokens),
			FormatTokenCount(stats.OutputTokens),
		))
	} else {
		parts = append(parts, "tokens n/a")
	}

	// Session
	if stats.SessionKey != "" {
		parts = append(parts, "sessionKey "+stats.SessionKey)
	}
	if stats.SessionID != "" {
		parts = append(parts, "sessionId "+stats.SessionID)
	}

	return "Stats: " + strings.Join(parts, " • ")
}

// --- 系统提示词 ---

// SubagentSystemPromptParams 子 Agent 系统提示词参数。
type SubagentSystemPromptParams struct {
	RequesterSessionKey string
	RequesterChannel    string
	ChildSessionKey     string
	Label               string
	Task                string
	// Contract 委托合约（可选，非 nil 时追加合约段到系统提示词）。
	Contract *DelegationContract
}

// BuildSubagentSystemPrompt 构建子 Agent 系统提示词。
func BuildSubagentSystemPrompt(p SubagentSystemPromptParams) string {
	taskText := strings.TrimSpace(p.Task)
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	}
	taskText = strings.Join(strings.Fields(taskText), " ")

	lines := []string{
		"# Subagent Context",
		"",
		"You are a **subagent** spawned by the main agent for a specific task.",
		"",
		"## Your Role",
		fmt.Sprintf("- You were created to handle: %s", taskText),
		"- Complete this task. That's your entire purpose.",
		"- You are NOT the main agent. Don't try to be.",
		"",
		"## Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		"2. **Complete the task** - Your final message will be automatically reported to the main agent",
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You may be terminated after task completion. That's fine.",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the main agent should know",
		"- Keep it concise but informative",
		"",
		"## What You DON'T Do",
		"- NO user conversations (that's main agent's job)",
		"- NO external messages (email, tweets, etc.) unless explicitly tasked with a specific recipient/channel",
		"- NO cron jobs or persistent state",
		"- NO pretending to be the main agent",
		"- Only use the `message` tool when explicitly instructed to contact a specific external recipient; otherwise return plain text and let the main agent deliver it",
		"",
		"## Session Context",
	}

	if p.Label != "" {
		lines = append(lines, fmt.Sprintf("- Label: %s", p.Label))
	}
	if p.RequesterSessionKey != "" {
		lines = append(lines, fmt.Sprintf("- Requester session: %s.", p.RequesterSessionKey))
	}
	if p.RequesterChannel != "" {
		lines = append(lines, fmt.Sprintf("- Requester channel: %s.", p.RequesterChannel))
	}
	lines = append(lines, fmt.Sprintf("- Your session: %s.", p.ChildSessionKey))
	lines = append(lines, "")

	// 合约段：有委托合约时追加结构化约束到系统提示词
	if p.Contract != nil {
		lines = append(lines, p.Contract.FormatForSystemPrompt())
	}

	return strings.Join(lines, "\n")
}
