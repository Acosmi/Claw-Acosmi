package prompt

import (
	"fmt"
	"strings"
)

// ---------- 段落构建器（续） ----------

func buildSandboxSection(info *SandboxInfo) string {
	if info == nil || !info.Enabled {
		return ""
	}
	lines := []string{
		"## Sandbox",
		"You are running in a sandboxed runtime (tools execute in Docker).",
		"Sub-agents stay sandboxed (no elevated/host access).",
	}
	if info.WorkspaceDir != "" {
		lines = append(lines, fmt.Sprintf("Sandbox workspace: %s", info.WorkspaceDir))
	}
	if info.WorkspaceAccess != "" {
		s := fmt.Sprintf("Agent workspace access: %s", info.WorkspaceAccess)
		if info.AgentWorkspaceMount != "" {
			s += fmt.Sprintf(" (mounted at %s)", info.AgentWorkspaceMount)
		}
		lines = append(lines, s)
	}
	if info.BrowserBridgeURL != "" {
		lines = append(lines, "Sandbox browser: enabled.")
	}
	if info.BrowserNoVncURL != "" {
		lines = append(lines, fmt.Sprintf("Sandbox browser observer (noVNC): %s", info.BrowserNoVncURL))
	}
	if info.HostBrowserAllowed != nil {
		if *info.HostBrowserAllowed {
			lines = append(lines, "Host browser control: allowed.")
		} else {
			lines = append(lines, "Host browser control: blocked.")
		}
	}
	if info.Elevated != nil && info.Elevated.Allowed {
		lines = append(lines, "Elevated exec is available for this session.")
		lines = append(lines, "User can toggle with /elevated on|off|ask|full.")
		lines = append(lines, fmt.Sprintf("Current elevated level: %s", info.Elevated.DefaultLevel))
	}
	return strings.Join(lines, "\n")
}

func buildReplyTagsSection(isMinimal bool) string {
	if isMinimal {
		return ""
	}
	return "## Reply Tags\n" +
		"To request a native reply/quote on supported surfaces, include one tag in your reply:\n" +
		"- [[reply_to_current]] replies to the triggering message.\n" +
		"- [[reply_to:<id>]] replies to a specific message id when you have it.\n" +
		"Whitespace inside the tag is allowed (e.g. [[ reply_to_current ]] / [[ reply_to: 123 ]]).\n" +
		"Tags are stripped before sending; support depends on the current channel config."
}

func buildMessagingSection(isMinimal bool, available map[string]bool, messageToolHints []string) string {
	if isMinimal {
		return ""
	}
	lines := []string{
		"## Messaging",
		"- Reply in current session → automatically routes to the source channel.",
		"- Cross-session messaging → use sessions_send(sessionKey, message)",
		"- Never use exec/curl for provider messaging; Claw Acosmi handles all routing internally.",
	}
	if available["message"] {
		lines = append(lines, "### message tool")
		lines = append(lines, "- Use `message` for proactive sends + channel actions (polls, reactions, etc.).")
		lines = append(lines, "- For `action=send`, include `to` and `message`.")
		lines = append(lines, fmt.Sprintf("- If you use `message` (`action=send`) to deliver your user-visible reply, respond with ONLY: %s", SilentReplyToken))
		for _, h := range messageToolHints {
			if h = strings.TrimSpace(h); h != "" {
				lines = append(lines, h)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func buildVoiceSection(isMinimal bool, ttsHint string) string {
	if isMinimal {
		return ""
	}
	hint := strings.TrimSpace(ttsHint)
	if hint == "" {
		return ""
	}
	return fmt.Sprintf("## Voice (TTS)\n%s", hint)
}

func buildDocsSection(docsPath string, isMinimal bool) string {
	dp := strings.TrimSpace(docsPath)
	if dp == "" || isMinimal {
		return ""
	}
	return fmt.Sprintf("## Documentation\n"+
		"Claw Acosmi docs: %s\n"+
		"Source: https://github.com/Acosmi/ClawAcosmi\n"+
		"For Claw Acosmi behavior, commands, config, or architecture: consult local docs first.\n"+
		"When diagnosing issues, run `openacosmi status` yourself when possible; only ask the user if you lack access (e.g., sandboxed).", dp)
}

func buildSilentRepliesSection() string {
	return fmt.Sprintf("## Silent Replies\n"+
		"When you have nothing to say, respond with ONLY: %s\n"+
		"\n"+
		"⚠️ Rules:\n"+
		"- It must be your ENTIRE message — nothing else\n"+
		"- Never append it to an actual response (never include \"%s\" in real replies)\n"+
		"- Never wrap it in markdown or code blocks\n"+
		"\n"+
		"❌ Wrong: \"Here's help... %s\"\n"+
		"❌ Wrong: \"%s\"\n"+
		"✅ Right: %s", SilentReplyToken, SilentReplyToken, SilentReplyToken, SilentReplyToken, SilentReplyToken)
}

func buildHeartbeatsSection(heartbeatPrompt string) string {
	line := "Heartbeat prompt: (configured)"
	if hp := strings.TrimSpace(heartbeatPrompt); hp != "" {
		line = fmt.Sprintf("Heartbeat prompt: %s", hp)
	}
	return "## Heartbeats\n" + line + "\n" +
		"If you receive a heartbeat poll (a user message matching the heartbeat prompt above), and there is nothing that needs attention, reply exactly:\n" +
		"HEARTBEAT_OK\n" +
		"Claw Acosmi treats a leading/trailing \"HEARTBEAT_OK\" as a heartbeat ack (and may discard it).\n" +
		"If something needs attention, do NOT include \"HEARTBEAT_OK\"; reply with the alert text instead."
}

func buildReactionsSection(rg *ReactionGuidance) string {
	if rg == nil {
		return ""
	}
	var text string
	if rg.Level == "minimal" {
		text = fmt.Sprintf("Reactions are enabled for %s in MINIMAL mode.\n"+
			"React ONLY when truly relevant:\n"+
			"- Acknowledge important user requests or confirmations\n"+
			"- Express genuine sentiment (humor, appreciation) sparingly\n"+
			"- Avoid reacting to routine messages or your own replies\n"+
			"Guideline: at most 1 reaction per 5-10 exchanges.", rg.Channel)
	} else {
		text = fmt.Sprintf("Reactions are enabled for %s in EXTENSIVE mode.\n"+
			"Feel free to react liberally:\n"+
			"- Acknowledge messages with appropriate emojis\n"+
			"- Express sentiment and personality through reactions\n"+
			"- React to interesting content, humor, or notable events\n"+
			"- Use reactions to confirm understanding or agreement\n"+
			"Guideline: react whenever it feels natural.", rg.Channel)
	}
	return "## Reactions\n" + text
}

func buildContextFilesSection(files []ContextFile) string {
	if len(files) == 0 {
		return ""
	}
	lines := []string{
		"# Project Context",
		"",
		"The following project context files have been loaded:",
	}
	hasSoul := false
	for _, f := range files {
		base := f.Path
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		if strings.EqualFold(base, "soul.md") {
			hasSoul = true
		}
	}
	if hasSoul {
		lines = append(lines, "If SOUL.md is present, embody its persona and tone.")
	}
	lines = append(lines, "")
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("## %s", f.Path), "", f.Content, "")
	}
	return strings.Join(lines, "\n")
}

func buildReasoningFormatSection(tagHint bool) string {
	if !tagHint {
		return ""
	}
	return "## Reasoning Format\n" +
		"ALL internal reasoning MUST be inside <think>...</think>. " +
		"Do not output any analysis outside <think>. " +
		"Format every reply as <think>...</think> then <final>...</final>, with no other text. " +
		"Only the final user-visible reply may appear inside <final>. " +
		"Only text inside <final> is shown to the user; everything else is discarded and never seen by the user. " +
		"Example: " +
		"<think>Short internal reasoning.</think> " +
		"<final>Hey there! What would you like to do next?</final>"
}
