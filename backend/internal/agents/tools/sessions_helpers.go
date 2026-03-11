// tools/sessions_helpers.go — 会话辅助函数。
// TS 参考：src/agents/tools/sessions-helpers.ts (394L)
// 全量移植：类型定义 + session key 解析 + A2A 策略 + classify/derive + 文本清理
package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ---------- 本地辅助（避免跨包依赖） ----------

// isACPSessionKeyLocal 检查是否为 ACP session key（本地版本）。
func isACPSessionKeyLocal(key string) bool {
	raw := strings.TrimSpace(key)
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "acp:") {
		return true
	}
	if strings.HasPrefix(lower, "agent:") {
		parts := strings.SplitN(raw, ":", 3)
		if len(parts) >= 3 {
			return strings.HasPrefix(strings.ToLower(parts[2]), "acp:")
		}
	}
	return false
}

// normalizeMainKeyLocal 标准化主 session key（本地版本）。
func normalizeMainKeyLocal(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "main"
	}
	return strings.ToLower(trimmed)
}

// ---------- 类型定义 ----------

// SessionKind 会话类型分类。
type SessionKind string

const (
	SessionKindMain  SessionKind = "main"
	SessionKindGroup SessionKind = "group"
	SessionKindCron  SessionKind = "cron"
	SessionKindHook  SessionKind = "hook"
	SessionKindNode  SessionKind = "node"
	SessionKindOther SessionKind = "other"
)

// SessionListDeliveryContext 会话列表投递上下文。
type SessionListDeliveryContext struct {
	Channel   string `json:"channel,omitempty"`
	To        string `json:"to,omitempty"`
	AccountID string `json:"accountId,omitempty"`
}

// SessionListRow 会话列表行。
type SessionListRow struct {
	Key             string                      `json:"key"`
	Kind            SessionKind                 `json:"kind"`
	Channel         string                      `json:"channel"`
	Label           string                      `json:"label,omitempty"`
	DisplayName     string                      `json:"displayName,omitempty"`
	DeliveryContext *SessionListDeliveryContext `json:"deliveryContext,omitempty"`
	UpdatedAt       *float64                    `json:"updatedAt,omitempty"`
	SessionID       string                      `json:"sessionId,omitempty"`
	Model           string                      `json:"model,omitempty"`
	ContextTokens   *int                        `json:"contextTokens,omitempty"`
	TotalTokens     *int                        `json:"totalTokens,omitempty"`
	ThinkingLevel   string                      `json:"thinkingLevel,omitempty"`
	VerboseLevel    string                      `json:"verboseLevel,omitempty"`
	SystemSent      bool                        `json:"systemSent,omitempty"`
	AbortedLastRun  bool                        `json:"abortedLastRun,omitempty"`
	SendPolicy      string                      `json:"sendPolicy,omitempty"`
	LastChannel     string                      `json:"lastChannel,omitempty"`
	LastTo          string                      `json:"lastTo,omitempty"`
	LastAccountID   string                      `json:"lastAccountId,omitempty"`
	TranscriptPath  string                      `json:"transcriptPath,omitempty"`
	Messages        []any                       `json:"messages,omitempty"`
}

// ---------- Key 标准化 ----------

func normalizeKeyStr(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

// SessionAliasConfig 会话别名配置参数。
type SessionAliasConfig struct {
	MainKey string
	Scope   string // "global" | "per-sender"
}

// ResolveMainSessionAlias 解析主会话别名。
// 对齐 TS: resolveMainSessionAlias()
func ResolveMainSessionAlias(mainKey, scope string) (resolvedMainKey, alias, resolvedScope string) {
	resolvedMainKey = normalizeMainKeyLocal(mainKey)
	resolvedScope = scope
	if resolvedScope == "" {
		resolvedScope = "per-sender"
	}
	if resolvedScope == "global" {
		alias = "global"
	} else {
		alias = resolvedMainKey
	}
	return
}

// ResolveDisplaySessionKey 解析展示用 session key（将别名还原为 "main"）。
// 对齐 TS: resolveDisplaySessionKey()
func ResolveDisplaySessionKey(key, alias, mainKey string) string {
	if key == alias {
		return "main"
	}
	if key == mainKey {
		return "main"
	}
	return key
}

// ResolveInternalSessionKey 将展示用 key 转换为内部 key。
// 对齐 TS: resolveInternalSessionKey()
func ResolveInternalSessionKey(key, alias, mainKey string) string {
	if key == "main" {
		return alias
	}
	return key
}

// ---------- A2A 策略 ----------

// AgentToAgentPolicy Agent 间通信策略。
type AgentToAgentPolicy struct {
	Enabled      bool
	MatchesAllow func(agentID string) bool
	IsAllowed    func(requesterAgentID, targetAgentID string) bool
}

// AgentToAgentConfig A2A 配置。
type AgentToAgentConfig struct {
	Enabled bool     `json:"enabled,omitempty"`
	Allow   []string `json:"allow,omitempty"`
}

// CreateAgentToAgentPolicy 创建 A2A 策略。
// 对齐 TS: createAgentToAgentPolicy()
func CreateAgentToAgentPolicy(a2aCfg *AgentToAgentConfig) *AgentToAgentPolicy {
	enabled := false
	var allowPatterns []string
	if a2aCfg != nil {
		enabled = a2aCfg.Enabled
		allowPatterns = a2aCfg.Allow
	}

	matchesAllow := func(agentID string) bool {
		if len(allowPatterns) == 0 {
			return true
		}
		for _, pattern := range allowPatterns {
			raw := strings.TrimSpace(fmt.Sprintf("%v", pattern))
			if raw == "" {
				continue
			}
			if raw == "*" {
				return true
			}
			if !strings.Contains(raw, "*") {
				if raw == agentID {
					return true
				}
				continue
			}
			// 将 glob 转为正则
			escaped := regexp.QuoteMeta(raw)
			reStr := "^" + strings.ReplaceAll(escaped, `\*`, ".*") + "$"
			re, err := regexp.Compile("(?i)" + reStr)
			if err != nil {
				continue
			}
			if re.MatchString(agentID) {
				return true
			}
		}
		return false
	}

	isAllowed := func(requesterAgentID, targetAgentID string) bool {
		if requesterAgentID == targetAgentID {
			return true
		}
		if !enabled {
			return false
		}
		return matchesAllow(requesterAgentID) && matchesAllow(targetAgentID)
	}

	return &AgentToAgentPolicy{
		Enabled:      enabled,
		MatchesAllow: matchesAllow,
		IsAllowed:    isAllowed,
	}
}

// ---------- Session ID / Key 判定 ----------

var sessionIDRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// LooksLikeSessionID 检查是否看起来像 UUID 格式的 session ID。
// 对齐 TS: looksLikeSessionId()
func LooksLikeSessionID(value string) bool {
	return sessionIDRE.MatchString(strings.TrimSpace(value))
}

// LooksLikeSessionKey 检查是否看起来像规范的 session key。
// 对齐 TS: looksLikeSessionKey()
func LooksLikeSessionKey(value string) bool {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return false
	}
	// 规范 key 模式
	if raw == "main" || raw == "global" || raw == "unknown" {
		return true
	}
	if isACPSessionKeyLocal(raw) {
		return true
	}
	if strings.HasPrefix(raw, "agent:") {
		return true
	}
	if strings.HasPrefix(raw, "cron:") || strings.HasPrefix(raw, "hook:") {
		return true
	}
	if strings.HasPrefix(raw, "node-") || strings.HasPrefix(raw, "node:") {
		return true
	}
	if strings.Contains(raw, ":group:") || strings.Contains(raw, ":channel:") {
		return true
	}
	return false
}

// ShouldResolveSessionIDInput 是否需要将输入当作 sessionId 来解析。
// 对齐 TS: shouldResolveSessionIdInput()
func ShouldResolveSessionIDInput(value string) bool {
	return LooksLikeSessionID(value) || !LooksLikeSessionKey(value)
}

// ---------- Session Reference Resolution ----------

// SessionReferenceResolution session 引用解析结果。
type SessionReferenceResolution struct {
	OK                   bool   `json:"ok"`
	Key                  string `json:"key,omitempty"`
	DisplayKey           string `json:"displayKey,omitempty"`
	ResolvedViaSessionID bool   `json:"resolvedViaSessionId,omitempty"`
	Status               string `json:"status,omitempty"` // "error" | "forbidden"（当 OK=false 时）
	Error                string `json:"error,omitempty"`
}

// SessionReferenceResolver gateway 会话解析接口。
type SessionReferenceResolver interface {
	ResolveBySessionID(ctx context.Context, sessionID string, spawnedBy string, includeGlobal, includeUnknown bool) (string, error)
	ResolveByKey(ctx context.Context, key string, spawnedBy string) (string, error)
}

// resolveSessionKeyFromSessionID 通过 sessionId 解析 session key。
// 对齐 TS: resolveSessionKeyFromSessionId()
func resolveSessionKeyFromSessionID(ctx context.Context, resolver SessionReferenceResolver, sessionID, alias, mainKey, requesterInternalKey string, restrictToSpawned bool) *SessionReferenceResolution {
	spawnedBy := ""
	if restrictToSpawned && requesterInternalKey != "" {
		spawnedBy = requesterInternalKey
	}
	includeGlobal := !restrictToSpawned
	includeUnknown := !restrictToSpawned

	key, err := resolver.ResolveBySessionID(ctx, sessionID, spawnedBy, includeGlobal, includeUnknown)
	if err != nil {
		if restrictToSpawned {
			return &SessionReferenceResolution{
				OK:     false,
				Status: "forbidden",
				Error:  fmt.Sprintf("Session not visible from this sandboxed agent session: %s", sessionID),
			}
		}
		errMsg := err.Error()
		if errMsg == "" {
			errMsg = fmt.Sprintf("Session not found: %s (use the full sessionKey from sessions_list)", sessionID)
		}
		return &SessionReferenceResolution{
			OK:     false,
			Status: "error",
			Error:  errMsg,
		}
	}

	key = strings.TrimSpace(key)
	if key == "" {
		msg := fmt.Sprintf("Session not found: %s (use the full sessionKey from sessions_list)", sessionID)
		if restrictToSpawned {
			return &SessionReferenceResolution{OK: false, Status: "forbidden", Error: msg}
		}
		return &SessionReferenceResolution{OK: false, Status: "error", Error: msg}
	}

	return &SessionReferenceResolution{
		OK:                   true,
		Key:                  key,
		DisplayKey:           ResolveDisplaySessionKey(key, alias, mainKey),
		ResolvedViaSessionID: true,
	}
}

// resolveSessionKeyFromKey 通过 key 解析 session key。
// 对齐 TS: resolveSessionKeyFromKey()
func resolveSessionKeyFromKey(ctx context.Context, resolver SessionReferenceResolver, key, alias, mainKey, requesterInternalKey string, restrictToSpawned bool) *SessionReferenceResolution {
	spawnedBy := ""
	if restrictToSpawned && requesterInternalKey != "" {
		spawnedBy = requesterInternalKey
	}

	resolved, err := resolver.ResolveByKey(ctx, key, spawnedBy)
	if err != nil {
		return nil
	}
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return nil
	}

	return &SessionReferenceResolution{
		OK:                   true,
		Key:                  resolved,
		DisplayKey:           ResolveDisplaySessionKey(resolved, alias, mainKey),
		ResolvedViaSessionID: false,
	}
}

// ResolveSessionReference 解析会话引用（支持 sessionId 和 sessionKey）。
// 对齐 TS: resolveSessionReference()
func ResolveSessionReference(ctx context.Context, resolver SessionReferenceResolver, sessionKey, alias, mainKey, requesterInternalKey string, restrictToSpawned bool) *SessionReferenceResolution {
	raw := strings.TrimSpace(sessionKey)
	if ShouldResolveSessionIDInput(raw) {
		// 先尝试 key 解析，避免误判
		if resolver != nil {
			resolvedByKey := resolveSessionKeyFromKey(ctx, resolver, raw, alias, mainKey, requesterInternalKey, restrictToSpawned)
			if resolvedByKey != nil {
				return resolvedByKey
			}
			return resolveSessionKeyFromSessionID(ctx, resolver, raw, alias, mainKey, requesterInternalKey, restrictToSpawned)
		}
		return &SessionReferenceResolution{OK: false, Status: "error", Error: "session resolver not configured"}
	}

	resolvedKey := ResolveInternalSessionKey(raw, alias, mainKey)
	displayKey := ResolveDisplaySessionKey(resolvedKey, alias, mainKey)
	return &SessionReferenceResolution{
		OK:                   true,
		Key:                  resolvedKey,
		DisplayKey:           displayKey,
		ResolvedViaSessionID: false,
	}
}

// ---------- 分类 ----------

// ClassifySessionKind 分类会话类型。
// 对齐 TS: classifySessionKind()
func ClassifySessionKind(key, gatewayKind, alias, mainKey string) SessionKind {
	if key == alias || key == mainKey {
		return SessionKindMain
	}
	if strings.HasPrefix(key, "cron:") {
		return SessionKindCron
	}
	if strings.HasPrefix(key, "hook:") {
		return SessionKindHook
	}
	if strings.HasPrefix(key, "node-") || strings.HasPrefix(key, "node:") {
		return SessionKindNode
	}
	if gatewayKind == "group" {
		return SessionKindGroup
	}
	if strings.Contains(key, ":group:") || strings.Contains(key, ":channel:") {
		return SessionKindGroup
	}
	return SessionKindOther
}

// DeriveChannel 从会话信息推导频道名称。
// 对齐 TS: deriveChannel()
func DeriveChannel(key string, kind SessionKind, channel, lastChannel string) string {
	if kind == SessionKindCron || kind == SessionKindHook || kind == SessionKindNode {
		return "internal"
	}
	if ch := normalizeKeyStr(channel); ch != "" {
		return ch
	}
	if lch := normalizeKeyStr(lastChannel); lch != "" {
		return lch
	}
	// 从 key 中推导频道
	parts := strings.FieldsFunc(key, func(r rune) bool { return r == ':' })
	if len(parts) >= 3 && (parts[1] == "group" || parts[1] == "channel") {
		return parts[0]
	}
	return "unknown"
}

// ---------- 消息过滤与文本清理 ----------

// StripToolMessages 过滤掉 toolResult 类型的消息。
// 对齐 TS: stripToolMessages()
func StripToolMessages(messages []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			result = append(result, msg)
			continue
		}
		role, _ := msg["role"].(string)
		if role == "toolResult" {
			continue
		}
		result = append(result, msg)
	}
	return result
}

// SanitizeTextContent 清理工具调用标记和思维标签。
// 对齐 TS: sanitizeTextContent()
// 依赖: StripThinkingTags, StripDowngradedToolCallText, StripMinimaxToolCallXml
func SanitizeTextContent(text string) string {
	if text == "" {
		return text
	}
	// 链式清理：minimax xml → downgraded tool call → thinking tags
	result := stripMinimaxToolCallXml(text)
	result = stripDowngradedToolCallText(result)
	result = stripBracketToolDirectiveText(result)
	result = stripThinkingTagsFromText(result)
	return result
}

// ExtractAssistantText 从 assistant 消息中提取纯文本。
// 对齐 TS: extractAssistantText()
func ExtractAssistantText(message map[string]any) string {
	if message == nil {
		return ""
	}
	role, _ := message["role"].(string)
	if role != "assistant" {
		return ""
	}
	content, ok := message["content"].([]any)
	if !ok {
		return ""
	}
	var chunks []string
	for _, block := range content {
		b, ok := block.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := b["type"].(string)
		if typ != "text" {
			continue
		}
		text, _ := b["text"].(string)
		if text != "" {
			sanitized := SanitizeTextContent(text)
			if strings.TrimSpace(sanitized) != "" {
				chunks = append(chunks, sanitized)
			}
		}
	}
	joined := strings.TrimSpace(strings.Join(chunks, ""))
	if joined == "" {
		return ""
	}
	return sanitizeUserFacingTextFunc(joined)
}

// ---------- 内部文本清理函数 ----------

// sanitizeUserFacingTextFunc 对齐 TS sanitizeUserFacingText。
// 由外部注入或使用默认实现。
var sanitizeUserFacingTextFunc = defaultSanitizeUserFacingText

func defaultSanitizeUserFacingText(text string) string {
	return strings.TrimSpace(text)
}

// 清理正则
var (
	thinkingTagsRE       = regexp.MustCompile(`(?s)<thinking>.*?</thinking>`)
	minimaxToolCallXmlRE = regexp.MustCompile(`(?s)<tool_call>.*?</tool_call>`)
	downgradedToolCallRE = regexp.MustCompile(`(?m)^ToolCall\(.*?\)$`)
	bracketToolCallRE    = regexp.MustCompile(`(?ms)^\s*\[\[[a-z_][a-z0-9_]*\]\]\s*(?:\n[a-zA-Z0-9_.-]+\s*:\s*.*)*\s*$`)
)

// stripThinkingTagsFromText 移除思维标签。
func stripThinkingTagsFromText(text string) string {
	return strings.TrimSpace(thinkingTagsRE.ReplaceAllString(text, ""))
}

// stripMinimaxToolCallXml 移除 Minimax 风格的 XML 工具调用。
func stripMinimaxToolCallXml(text string) string {
	return strings.TrimSpace(minimaxToolCallXmlRE.ReplaceAllString(text, ""))
}

// stripDowngradedToolCallText 移除降级的工具调用文本。
func stripDowngradedToolCallText(text string) string {
	return strings.TrimSpace(downgradedToolCallRE.ReplaceAllString(text, ""))
}

func stripBracketToolDirectiveText(text string) string {
	return strings.TrimSpace(bracketToolCallRE.ReplaceAllString(text, ""))
}
