// tools/display.go — 工具显示逻辑。
// TS 参考：src/agents/tool-display.ts (292L) + tool-display.json (309L)
//
// 修复项 #1-5 (W3-T2b):
//
//	#1 meta fallback — TS 在无 detail 时用 params.meta 作 fallback
//	#2 detailKeys 多字段解析 — 支持点路径查找 + 多字段组合
//	#3 read/write/edit/attach 特殊 detail 解析
//	#4 RedactToolDetail — 工具详情脱敏
//	#5 ShortenHomeInString — 路径 ~ 缩写
package tools

import (
	"fmt"
	"math"
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/log"
	"github.com/Acosmi/ClawAcosmi/pkg/utils"
)

// ToolDisplay 工具展示信息。
type ToolDisplay struct {
	Name   string `json:"name"`
	Emoji  string `json:"emoji"`
	Title  string `json:"title"`
	Label  string `json:"label"`
	Verb   string `json:"verb,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ToolDisplayActionSpec action 级展示配置。
type ToolDisplayActionSpec struct {
	Label      string   `json:"label,omitempty"`
	DetailKeys []string `json:"detailKeys,omitempty"`
}

// ToolDisplaySpec 工具展示规格。
type ToolDisplaySpec struct {
	Emoji      string                           `json:"emoji,omitempty"`
	Title      string                           `json:"title,omitempty"`
	Label      string                           `json:"label,omitempty"`
	DetailKeys []string                         `json:"detailKeys,omitempty"`
	Actions    map[string]ToolDisplayActionSpec `json:"actions,omitempty"`
}

// ---------- 常量 ----------

const maxDetailEntries = 8

// detailLabelOverrides 字段名 → 显示标签映射（修复项 #2）。
var detailLabelOverrides = map[string]string{
	"agentId":           "agent",
	"sessionKey":        "session",
	"targetId":          "target",
	"targetUrl":         "url",
	"nodeId":            "node",
	"requestId":         "request",
	"messageId":         "message",
	"threadId":          "thread",
	"channelId":         "channel",
	"guildId":           "guild",
	"userId":            "user",
	"runTimeoutSeconds": "timeout",
	"timeoutSeconds":    "timeout",
	"includeTools":      "tools",
	"pollQuestion":      "poll",
	"maxChars":          "max chars",
}

// fallbackDetailKeys 默认 fallback detailKeys（来自 tool-display.json）。
var fallbackDetailKeys = []string{
	"command", "path", "url", "targetUrl", "targetId",
	"ref", "element", "node", "nodeId", "id", "requestId",
	"to", "channelId", "guildId", "userId", "name", "query",
	"pattern", "messageId",
}

// ---------- 工具注册表（来自 tool-display.json）----------

var toolRegistry = map[string]ToolDisplaySpec{
	// Canonical tool names (from capabilities.Registry)
	"bash":        {Emoji: "🛠️", Title: "Bash", DetailKeys: []string{"command"}},
	"read_file":   {Emoji: "📖", Title: "Read File", DetailKeys: []string{"path"}},
	"write_file":  {Emoji: "✍️", Title: "Write File", DetailKeys: []string{"path"}},
	"list_dir":    {Emoji: "📂", Title: "List Dir", DetailKeys: []string{"path"}},
	"apply_patch": {Emoji: "🩹", Title: "Apply Patch"},
	// Legacy tool names (backward compat for old transcripts)
	"exec":    {Emoji: "🛠️", Title: "Exec", DetailKeys: []string{"command"}},
	"process": {Emoji: "🧰", Title: "Process", DetailKeys: []string{"sessionId"}},
	"read":    {Emoji: "📖", Title: "Read", DetailKeys: []string{"path"}},
	"write":   {Emoji: "✍️", Title: "Write", DetailKeys: []string{"path"}},
	"edit":    {Emoji: "📝", Title: "Edit", DetailKeys: []string{"path"}},
	"attach":  {Emoji: "📎", Title: "Attach", DetailKeys: []string{"path", "url", "fileName"}},
	"browser": {Emoji: "🌐", Title: "Browser", Actions: map[string]ToolDisplayActionSpec{
		"status": {Label: "status"}, "start": {Label: "start"}, "stop": {Label: "stop"},
		"tabs":       {Label: "tabs"},
		"open":       {Label: "open", DetailKeys: []string{"targetUrl"}},
		"focus":      {Label: "focus", DetailKeys: []string{"targetId"}},
		"close":      {Label: "close", DetailKeys: []string{"targetId"}},
		"snapshot":   {Label: "snapshot", DetailKeys: []string{"targetUrl", "targetId", "ref", "element", "format"}},
		"screenshot": {Label: "screenshot", DetailKeys: []string{"targetUrl", "targetId", "ref", "element"}},
		"navigate":   {Label: "navigate", DetailKeys: []string{"targetUrl", "targetId"}},
		"console":    {Label: "console", DetailKeys: []string{"level", "targetId"}},
		"pdf":        {Label: "pdf", DetailKeys: []string{"targetId"}},
		"upload":     {Label: "upload", DetailKeys: []string{"paths", "ref", "inputRef", "element", "targetId"}},
		"dialog":     {Label: "dialog", DetailKeys: []string{"accept", "promptText", "targetId"}},
		"act":        {Label: "act", DetailKeys: []string{"request.kind", "request.ref", "request.selector", "request.text", "request.value"}},
	}},
	"canvas": {Emoji: "🖼️", Title: "Canvas", Actions: map[string]ToolDisplayActionSpec{
		"present":    {Label: "present", DetailKeys: []string{"target", "node", "nodeId"}},
		"hide":       {Label: "hide", DetailKeys: []string{"node", "nodeId"}},
		"navigate":   {Label: "navigate", DetailKeys: []string{"url", "node", "nodeId"}},
		"eval":       {Label: "eval", DetailKeys: []string{"javaScript", "node", "nodeId"}},
		"snapshot":   {Label: "snapshot", DetailKeys: []string{"format", "node", "nodeId"}},
		"a2ui_push":  {Label: "A2UI push", DetailKeys: []string{"jsonlPath", "node", "nodeId"}},
		"a2ui_reset": {Label: "A2UI reset", DetailKeys: []string{"node", "nodeId"}},
	}},
	"nodes": {Emoji: "📱", Title: "Nodes", Actions: map[string]ToolDisplayActionSpec{
		"status": {Label: "status"}, "describe": {Label: "describe", DetailKeys: []string{"node", "nodeId"}},
		"pending":       {Label: "pending"},
		"approve":       {Label: "approve", DetailKeys: []string{"requestId"}},
		"reject":        {Label: "reject", DetailKeys: []string{"requestId"}},
		"notify":        {Label: "notify", DetailKeys: []string{"node", "nodeId", "title", "body"}},
		"camera_snap":   {Label: "camera snap", DetailKeys: []string{"node", "nodeId", "facing", "deviceId"}},
		"camera_list":   {Label: "camera list", DetailKeys: []string{"node", "nodeId"}},
		"camera_clip":   {Label: "camera clip", DetailKeys: []string{"node", "nodeId", "facing", "duration", "durationMs"}},
		"screen_record": {Label: "screen record", DetailKeys: []string{"node", "nodeId", "duration", "durationMs", "fps", "screenIndex"}},
	}},
	"cron": {Emoji: "⏰", Title: "Cron", Actions: map[string]ToolDisplayActionSpec{
		"status": {Label: "status"}, "list": {Label: "list"},
		"add":    {Label: "add", DetailKeys: []string{"job.name", "job.id", "job.schedule", "job.cron"}},
		"update": {Label: "update", DetailKeys: []string{"id"}},
		"remove": {Label: "remove", DetailKeys: []string{"id"}},
		"run":    {Label: "run", DetailKeys: []string{"id"}},
		"runs":   {Label: "runs", DetailKeys: []string{"id"}},
		"wake":   {Label: "wake", DetailKeys: []string{"text", "mode"}},
	}},
	"gateway": {Emoji: "🔌", Title: "Gateway", Actions: map[string]ToolDisplayActionSpec{
		"restart": {Label: "restart", DetailKeys: []string{"reason", "delayMs"}},
	}},
	"message": {Emoji: "✉️", Title: "Message", Actions: map[string]ToolDisplayActionSpec{
		"send":           {Label: "send", DetailKeys: []string{"provider", "to", "media", "replyTo", "threadId"}},
		"poll":           {Label: "poll", DetailKeys: []string{"provider", "to", "pollQuestion"}},
		"react":          {Label: "react", DetailKeys: []string{"provider", "to", "messageId", "emoji", "remove"}},
		"reactions":      {Label: "reactions", DetailKeys: []string{"provider", "to", "messageId", "limit"}},
		"read":           {Label: "read", DetailKeys: []string{"provider", "to", "limit"}},
		"edit":           {Label: "edit", DetailKeys: []string{"provider", "to", "messageId"}},
		"delete":         {Label: "delete", DetailKeys: []string{"provider", "to", "messageId"}},
		"pin":            {Label: "pin", DetailKeys: []string{"provider", "to", "messageId"}},
		"unpin":          {Label: "unpin", DetailKeys: []string{"provider", "to", "messageId"}},
		"list-pins":      {Label: "list pins", DetailKeys: []string{"provider", "to"}},
		"permissions":    {Label: "permissions", DetailKeys: []string{"provider", "channelId", "to"}},
		"thread-create":  {Label: "thread create", DetailKeys: []string{"provider", "channelId", "threadName"}},
		"thread-list":    {Label: "thread list", DetailKeys: []string{"provider", "guildId", "channelId"}},
		"thread-reply":   {Label: "thread reply", DetailKeys: []string{"provider", "channelId", "messageId"}},
		"search":         {Label: "search", DetailKeys: []string{"provider", "guildId", "query"}},
		"sticker":        {Label: "sticker", DetailKeys: []string{"provider", "to", "stickerId"}},
		"member-info":    {Label: "member", DetailKeys: []string{"provider", "guildId", "userId"}},
		"role-info":      {Label: "roles", DetailKeys: []string{"provider", "guildId"}},
		"emoji-list":     {Label: "emoji list", DetailKeys: []string{"provider", "guildId"}},
		"emoji-upload":   {Label: "emoji upload", DetailKeys: []string{"provider", "guildId", "emojiName"}},
		"sticker-upload": {Label: "sticker upload", DetailKeys: []string{"provider", "guildId", "stickerName"}},
		"role-add":       {Label: "role add", DetailKeys: []string{"provider", "guildId", "userId", "roleId"}},
		"role-remove":    {Label: "role remove", DetailKeys: []string{"provider", "guildId", "userId", "roleId"}},
		"channel-info":   {Label: "channel", DetailKeys: []string{"provider", "channelId"}},
		"channel-list":   {Label: "channels", DetailKeys: []string{"provider", "guildId"}},
		"voice-status":   {Label: "voice", DetailKeys: []string{"provider", "guildId", "userId"}},
		"event-list":     {Label: "events", DetailKeys: []string{"provider", "guildId"}},
		"event-create":   {Label: "event create", DetailKeys: []string{"provider", "guildId", "eventName"}},
		"timeout":        {Label: "timeout", DetailKeys: []string{"provider", "guildId", "userId"}},
		"kick":           {Label: "kick", DetailKeys: []string{"provider", "guildId", "userId"}},
		"ban":            {Label: "ban", DetailKeys: []string{"provider", "guildId", "userId"}},
	}},
	"agents_list":      {Emoji: "🧭", Title: "Agents"},
	"sessions_list":    {Emoji: "🗂️", Title: "Sessions", DetailKeys: []string{"kinds", "limit", "activeMinutes", "messageLimit"}},
	"sessions_history": {Emoji: "🧾", Title: "Session History", DetailKeys: []string{"sessionKey", "limit", "includeTools"}},
	"sessions_send":    {Emoji: "📨", Title: "Session Send", DetailKeys: []string{"label", "sessionKey", "agentId", "timeoutSeconds"}},
	"sessions_spawn":   {Emoji: "🧑‍🔧", Title: "Sub-agent", DetailKeys: []string{"label", "task", "agentId", "model", "thinking", "runTimeoutSeconds", "cleanup", "timeoutSeconds"}},
	"session_status":   {Emoji: "📊", Title: "Session Status", DetailKeys: []string{"sessionKey", "model"}},
	"memory_search":    {Emoji: "🧠", Title: "Memory Search", DetailKeys: []string{"query"}},
	"memory_get":       {Emoji: "📓", Title: "Memory Get", DetailKeys: []string{"path", "from", "lines"}},
	"web_search":       {Emoji: "🔎", Title: "Web Search", DetailKeys: []string{"query", "count"}},
	"web_fetch":        {Emoji: "📄", Title: "Web Fetch", DetailKeys: []string{"url", "extractMode", "maxChars"}},
	"whatsapp_login": {Emoji: "🟢", Title: "WhatsApp Login", Actions: map[string]ToolDisplayActionSpec{
		"start": {Label: "start"}, "wait": {Label: "wait"},
	}},
	"tts": {Emoji: "🔊", Title: "Text-to-Speech"},
}

// ---------- 辅助函数 ----------

// formatToolTitle 将 tool_name → Tool Name。
func formatToolTitle(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if len(p) > 0 {
			// 短全大写保留（如 "ID"）
			if len(p) <= 2 && strings.ToUpper(p) == p {
				parts[i] = p
			} else {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
	}
	return strings.Join(parts, " ")
}

// normalizeVerb 规范化 verb 文本。
func normalizeVerb(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ReplaceAll(trimmed, "_", " ")
}

// coerceDisplayValue 将任意值转为可展示字符串（修复项 #2）。
// TS 参考: tool-display.ts coerceDisplayValue
func coerceDisplayValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
		}
		// 取第一行
		firstLine := strings.SplitN(trimmed, "\n", 2)[0]
		firstLine = strings.TrimSpace(firstLine)
		if firstLine == "" {
			return ""
		}
		if len([]rune(firstLine)) > 160 {
			return string([]rune(firstLine)[:157]) + "…"
		}
		return firstLine
	case bool:
		if v {
			return "true"
		}
		return ""
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) || v == 0 {
			return ""
		}
		return fmt.Sprintf("%g", v)
	case int:
		if v == 0 {
			return ""
		}
		return fmt.Sprintf("%d", v)
	case int64:
		if v == 0 {
			return ""
		}
		return fmt.Sprintf("%d", v)
	case []interface{}:
		var values []string
		for _, item := range v {
			d := coerceDisplayValue(item)
			if d != "" {
				values = append(values, d)
			}
		}
		if len(values) == 0 {
			return ""
		}
		if len(values) > 3 {
			return strings.Join(values[:3], ", ") + "…"
		}
		return strings.Join(values, ", ")
	}
	return ""
}

// lookupValueByPath 通过点路径查找嵌套值（修复项 #2）。
// TS 参考: tool-display.ts lookupValueByPath
func lookupValueByPath(args interface{}, path string) interface{} {
	if args == nil || path == "" {
		return nil
	}
	current := args
	for _, segment := range strings.Split(path, ".") {
		if segment == "" {
			return nil
		}
		record, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = record[segment]
	}
	return current
}

// formatDetailKey 格式化 detail key 为显示标签（修复项 #2）。
// TS 参考: tool-display.ts formatDetailKey
func formatDetailKey(raw string) string {
	segments := strings.Split(raw, ".")
	var nonEmpty []string
	for _, s := range segments {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	last := raw
	if len(nonEmpty) > 0 {
		last = nonEmpty[len(nonEmpty)-1]
	}
	if override, ok := detailLabelOverrides[last]; ok {
		return override
	}
	cleaned := strings.ReplaceAll(strings.ReplaceAll(last, "_", " "), "-", " ")
	// camelCase → spaced
	var spaced strings.Builder
	for i, ch := range cleaned {
		if i > 0 && ch >= 'A' && ch <= 'Z' {
			prev := rune(cleaned[i-1])
			if prev >= 'a' && prev <= 'z' || prev >= '0' && prev <= '9' {
				spaced.WriteRune(' ')
			}
		}
		spaced.WriteRune(ch)
	}
	return strings.ToLower(strings.TrimSpace(spaced.String()))
}

// resolveDetailFromKeys 从多个 key 中组合 detail 文本（修复项 #2）。
// TS 参考: tool-display.ts resolveDetailFromKeys
func resolveDetailFromKeys(args interface{}, keys []string) string {
	type entry struct {
		label, value string
	}
	var entries []entry
	for _, key := range keys {
		val := lookupValueByPath(args, key)
		display := coerceDisplayValue(val)
		if display == "" {
			continue
		}
		entries = append(entries, entry{label: formatDetailKey(key), value: display})
	}
	if len(entries) == 0 {
		return ""
	}
	if len(entries) == 1 {
		return entries[0].value
	}
	// 去重
	seen := make(map[string]bool)
	var unique []entry
	for _, e := range entries {
		token := e.label + ":" + e.value
		if seen[token] {
			continue
		}
		seen[token] = true
		unique = append(unique, e)
	}
	if len(unique) == 0 {
		return ""
	}
	if len(unique) > maxDetailEntries {
		unique = unique[:maxDetailEntries]
	}
	parts := make([]string, len(unique))
	for i, e := range unique {
		parts[i] = e.label + " " + e.value
	}
	return strings.Join(parts, " · ")
}

// resolveReadDetail 解析 read 工具的特殊 detail（修复项 #3）。
// TS 参考: tool-display.ts resolveReadDetail
func resolveReadDetail(args interface{}) string {
	record, ok := args.(map[string]interface{})
	if !ok {
		return ""
	}
	path, _ := record["path"].(string)
	if path == "" {
		return ""
	}
	offset, hasOffset := record["offset"].(float64)
	limit, hasLimit := record["limit"].(float64)
	if hasOffset && hasLimit {
		return fmt.Sprintf("%s:%d-%d", path, int(offset), int(offset+limit))
	}
	return path
}

// resolveWriteDetail 解析 write/edit/attach 工具的特殊 detail（修复项 #3）。
func resolveWriteDetail(args interface{}) string {
	record, ok := args.(map[string]interface{})
	if !ok {
		return ""
	}
	path, _ := record["path"].(string)
	return path
}

// ---------- 核心 API ----------

// ResolveToolDisplay 解析工具显示信息。
// TS 参考: tool-display.ts resolveToolDisplay
// 修复项 #1: 增加 meta 参数
// 修复项 #2: detailKeys 多字段解析
// 修复项 #3: read/write/edit/attach 特殊解析
// 修复项 #5: ShortenHomeInString
func ResolveToolDisplay(toolName string, args map[string]any, meta ...string) ToolDisplay {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "tool"
	}
	key := strings.ToLower(name)
	spec, hasSpec := toolRegistry[key]

	emoji := "🧩"
	title := formatToolTitle(name)
	label := title
	if hasSpec {
		if spec.Emoji != "" {
			emoji = spec.Emoji
		}
		if spec.Title != "" {
			title = spec.Title
		}
		if spec.Label != "" {
			label = spec.Label
		} else {
			label = title
		}
	}

	// 解析 action
	var actionStr string
	if args != nil {
		if a, ok := args["action"].(string); ok {
			actionStr = strings.TrimSpace(a)
		}
	}
	var actionSpec *ToolDisplayActionSpec
	if hasSpec && actionStr != "" && spec.Actions != nil {
		if as, ok := spec.Actions[actionStr]; ok {
			actionSpec = &as
		}
	}

	verb := ""
	if actionSpec != nil && actionSpec.Label != "" {
		verb = normalizeVerb(actionSpec.Label)
	} else if actionStr != "" {
		verb = normalizeVerb(actionStr)
	}

	// ----- detail 解析 -----

	var detail string

	// 修复项 #3: read/write/edit/attach 特殊解析
	if key == "read" {
		detail = resolveReadDetail(args)
	}
	if detail == "" && (key == "write" || key == "edit" || key == "attach") {
		detail = resolveWriteDetail(args)
	}

	// 修复项 #2: detailKeys 多字段解析
	var detailKeys []string
	if actionSpec != nil && len(actionSpec.DetailKeys) > 0 {
		detailKeys = actionSpec.DetailKeys
	} else if hasSpec && len(spec.DetailKeys) > 0 {
		detailKeys = spec.DetailKeys
	} else if !hasSpec {
		detailKeys = fallbackDetailKeys
	}
	if detail == "" && len(detailKeys) > 0 && args != nil {
		// 将 map[string]any 转为 interface{} 用于 lookupValueByPath
		var argsIface interface{} = args
		detail = resolveDetailFromKeys(argsIface, detailKeys)
	}

	// 修复项 #1: meta fallback
	if detail == "" && len(meta) > 0 && meta[0] != "" {
		detail = meta[0]
	}

	// 修复项 #5: ShortenHomeInString
	if detail != "" {
		detail = utils.ShortenHomeInString(detail)
	}

	return ToolDisplay{
		Name:   name,
		Emoji:  emoji,
		Title:  title,
		Label:  label,
		Verb:   verb,
		Detail: detail,
	}
}

// FormatToolDetail 格式化工具详情行（修复项 #4: redact）。
// TS 参考: tool-display.ts formatToolDetail
func FormatToolDetail(display ToolDisplay) string {
	var parts []string
	if display.Verb != "" {
		parts = append(parts, display.Verb)
	}
	if display.Detail != "" {
		// 修复项 #4: 调用 RedactToolDetail
		parts = append(parts, log.RedactToolDetail(display.Detail, log.RedactTools))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

// FormatToolSummary 格式化工具使用摘要（用于日志/UI）。
// TS 参考: tool-display.ts formatToolSummary
func FormatToolSummary(toolName string, args map[string]any) string {
	d := ResolveToolDisplay(toolName, args)
	detail := FormatToolDetail(d)
	if detail != "" {
		return fmt.Sprintf("%s %s: %s", d.Emoji, d.Label, detail)
	}
	return fmt.Sprintf("%s %s", d.Emoji, d.Label)
}
