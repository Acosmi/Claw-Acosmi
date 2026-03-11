package gateway

// server_methods_usage.go — sessions.usage.* 方法处理器
// 对应 TS: src/gateway/server-methods/usage.ts (822L)
//
// 完整实现：session discovery + cost aggregation + 多维度聚合。
// 隐藏依赖 #2: costUsageCache 模块级 Map + TTL 30s

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/internal/agents/configtools"
	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/prompt"
	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
	agentsession "github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/sessions"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// UsageHandlers 返回 sessions.usage.* 方法处理器映射。
func UsageHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"sessions.usage":            handleSessionsUsage,
		"sessions.usage.timeseries": handleSessionsUsageTimeseries,
		"sessions.usage.logs":       handleSessionsUsageLogs,
		"usage.status":              handleUsageStatus,
		"usage.cost":                handleUsageCost,
	}
}

// ---------- 类型定义 ----------

type usageDateRange struct {
	startMs int64
	endMs   int64
}

type usageTotals struct {
	Input              int     `json:"input"`
	Output             int     `json:"output"`
	CacheRead          int     `json:"cacheRead"`
	CacheWrite         int     `json:"cacheWrite"`
	TotalTokens        int     `json:"totalTokens"`
	TotalCost          float64 `json:"totalCost"`
	InputCost          float64 `json:"inputCost"`
	OutputCost         float64 `json:"outputCost"`
	CacheReadCost      float64 `json:"cacheReadCost"`
	CacheWriteCost     float64 `json:"cacheWriteCost"`
	MissingCostEntries int     `json:"missingCostEntries"`
}

type usageCostDailyEntry struct {
	Date string `json:"date"`
	usageTotals
}

type usageCostSummary struct {
	UpdatedAt int64                 `json:"updatedAt"`
	Days      int                   `json:"days"`
	Daily     []usageCostDailyEntry `json:"daily"`
	Totals    *usageTotals          `json:"totals"`
}

type toolUsageEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type toolUsageSummary struct {
	TotalCalls  int              `json:"totalCalls"`
	UniqueTools int              `json:"uniqueTools"`
	Tools       []toolUsageEntry `json:"tools"`
}

type modelUsageEntry struct {
	Provider string       `json:"provider,omitempty"`
	Model    string       `json:"model,omitempty"`
	Count    int          `json:"count"`
	Totals   *usageTotals `json:"totals"`
}

type latencySummary struct {
	Count int     `json:"count"`
	AvgMs float64 `json:"avgMs"`
	P95Ms float64 `json:"p95Ms"`
	MinMs float64 `json:"minMs"`
	MaxMs float64 `json:"maxMs"`
}

type usageContextWeight struct {
	SystemPrompt struct {
		Chars                  int `json:"chars"`
		ProjectContextChars    int `json:"projectContextChars"`
		NonProjectContextChars int `json:"nonProjectContextChars"`
	} `json:"systemPrompt"`
	Skills struct {
		PromptChars int                      `json:"promptChars"`
		Entries     []usageContextSkillEntry `json:"entries"`
	} `json:"skills"`
	Tools struct {
		ListChars   int                     `json:"listChars"`
		SchemaChars int                     `json:"schemaChars"`
		Entries     []usageContextToolEntry `json:"entries"`
	} `json:"tools"`
	InjectedWorkspaceFiles []usageContextFileEntry `json:"injectedWorkspaceFiles"`
}

type usageContextSkillEntry struct {
	Name       string `json:"name"`
	BlockChars int    `json:"blockChars"`
}

type usageContextToolEntry struct {
	Name         string `json:"name"`
	SummaryChars int    `json:"summaryChars"`
	SchemaChars  int    `json:"schemaChars"`
}

type usageContextFileEntry struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	RawChars      int    `json:"rawChars"`
	InjectedChars int    `json:"injectedChars"`
	Truncated     bool   `json:"truncated"`
}

type sessionCostSummary struct {
	usageTotals
	MessageCounts      *messageCounts          `json:"messageCounts,omitempty"`
	ToolUsage          *toolUsageSummary       `json:"toolUsage,omitempty"`
	ModelUsage         []modelUsageEntry       `json:"modelUsage,omitempty"`
	Latency            *latencySummary         `json:"latency,omitempty"`
	UsageSource        string                  `json:"usageSource,omitempty"`   // actual | estimated | mixed
	ByModel            map[string]*usageTotals `json:"-"`                       // provider::model → totals
	ByModelCounts      map[string]int          `json:"-"`                       // provider::model → request count
	ToolNames          map[string]int          `json:"-"`                       // toolName → callCount
	FirstActivity      int64                   `json:"firstActivity,omitempty"` // Unix ms
	LastActivity       int64                   `json:"lastActivity,omitempty"`  // Unix ms
	DurationMs         int64                   `json:"durationMs,omitempty"`
	ActivityDates      []string                `json:"activityDates,omitempty"` // ["2026-02-26"]
	DailyBreakdown     []dailyUsageEntry       `json:"dailyBreakdown,omitempty"`
	DailyTotals        []usageCostDailyEntry   `json:"-"`
	DailyMessageCounts []dailyMsgCountEntry    `json:"dailyMessageCounts,omitempty"`
	DailyModelUsage    []dailyModelEntry       `json:"dailyModelUsage,omitempty"`
	DailyLatency       []dailyLatencyEntry     `json:"dailyLatency,omitempty"`
	rawLatencyData     map[string][]float64    `json:"-"` // 全局聚合用原始数据
	lastProvider       string                  `json:"-"`
	lastModel          string                  `json:"-"`
}

type dailyUsageEntry struct {
	Date   string  `json:"date"` // "2026-02-26"
	Tokens int     `json:"tokens"`
	Cost   float64 `json:"cost"`
	usageTotals
}

type dailyMsgCountEntry struct {
	Date        string `json:"date"`
	Total       int    `json:"total"`
	User        int    `json:"user"`
	Assistant   int    `json:"assistant"`
	ToolCalls   int    `json:"toolCalls"`
	ToolResults int    `json:"toolResults"`
	Errors      int    `json:"errors"`
}

type messageCounts struct {
	Total       int `json:"total"`
	User        int `json:"user"`
	Assistant   int `json:"assistant"`
	ToolCalls   int `json:"toolCalls"`
	ToolResults int `json:"toolResults"`
	Errors      int `json:"errors"`
}

type dailyModelEntry struct {
	Date     string  `json:"date"`
	Provider string  `json:"provider,omitempty"`
	Model    string  `json:"model"`
	Tokens   int     `json:"tokens"`
	Cost     float64 `json:"cost"`
	Count    int     `json:"count"`
}

type dailyLatencyEntry struct {
	Date  string  `json:"date"`
	Count int     `json:"count"`
	AvgMs float64 `json:"avgMs"`
	P95Ms float64 `json:"p95Ms"`
	MinMs float64 `json:"minMs"`
	MaxMs float64 `json:"maxMs"`
}

// ---------- 缓存 (隐依赖 #2: costUsageCache) ----------

const costUsageCacheTTL = 30 * time.Second

type costUsageCacheEntry struct {
	summary   *usageCostSummary
	updatedAt time.Time
}

var (
	costCacheMu sync.RWMutex
	costCache   = map[string]*costUsageCacheEntry{}
)

// ---------- 日期解析 ----------

var dateRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)

func parseDateToMs(raw interface{}) (int64, bool) {
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return 0, false
	}
	m := dateRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	return t.UnixMilli(), true
}

func parseDays(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return int(v), true
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
	}
	return 0, false
}

func parseDateRange(params map[string]interface{}) usageDateRange {
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayEndMs := todayStart.Add(24*time.Hour).UnixMilli() - 1

	startMs, startOK := parseDateToMs(params["startDate"])
	endMs, endOK := parseDateToMs(params["endDate"])
	if startOK && endOK {
		return usageDateRange{startMs: startMs, endMs: endMs + 24*60*60*1000 - 1}
	}

	if days, ok := parseDays(params["days"]); ok {
		if days < 1 {
			days = 1
		}
		start := todayStart.AddDate(0, 0, -(days - 1)).UnixMilli()
		return usageDateRange{startMs: start, endMs: todayEndMs}
	}

	// 默认 30 天
	defaultStart := todayStart.AddDate(0, 0, -29).UnixMilli()
	return usageDateRange{startMs: defaultStart, endMs: todayEndMs}
}

func formatDateStr(ms int64) string {
	t := time.UnixMilli(ms).UTC()
	return t.Format("2006-01-02")
}

// ---------- Session Discovery ----------

type discoveredSession struct {
	sessionID   string
	sessionFile string
	agentID     string
	mtime       int64 // UnixMilli
}

// discoverSessionsForAgent 扫描指定 agent 的 sessions 目录。
func discoverSessionsForAgent(agentID string, startMs, endMs int64) []discoveredSession {
	dir := sessions.ResolveAgentSessionsDir(agentID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []discoveredSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mtimeMs := info.ModTime().UnixMilli()
		// 时间范围过滤
		if mtimeMs < startMs || mtimeMs > endMs {
			continue
		}
		sid := strings.TrimSuffix(e.Name(), ".jsonl")
		// 跳过 topic 文件
		if strings.Contains(sid, "-topic-") {
			continue
		}
		results = append(results, discoveredSession{
			sessionID:   sid,
			sessionFile: filepath.Join(dir, e.Name()),
			agentID:     agentID,
			mtime:       mtimeMs,
		})
	}
	return results
}

// discoverAllSessionsForUsage 遍历所有 agent 发现会话。
// 始终包含 routing.DefaultAgentID ("main")，确保文件系统默认路径不被遗漏。
func discoverAllSessionsForUsage(cfg interface{ GetAgentIds() []string }, startMs, endMs int64, agentIds []string) []discoveredSession {
	// 确保 routing.DefaultAgentID (main) 始终在扫描列表中
	// （scope.DefaultAgentID 是 "default"，但文件实际存在 "main" 目录下）
	seen := make(map[string]bool, len(agentIds)+1)
	var ids []string
	for _, id := range agentIds {
		normalized := strings.ToLower(strings.TrimSpace(id))
		if normalized != "" && !seen[normalized] {
			seen[normalized] = true
			ids = append(ids, id)
		}
	}
	const routingDefault = "main" // routing.DefaultAgentID
	if !seen[routingDefault] {
		ids = append(ids, routingDefault)
	}

	var all []discoveredSession
	for _, id := range ids {
		all = append(all, discoverSessionsForAgent(id, startMs, endMs)...)
	}
	// 按 mtime 降序
	sort.Slice(all, func(i, j int) bool { return all[i].mtime > all[j].mtime })
	return all
}

func resolveUsageTranscriptCandidates(sessionID string, storeEntry *SessionEntry, storePath, agentID string) []string {
	seen := map[string]struct{}{}
	add := func(path string, results *[]string) {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		*results = append(*results, trimmed)
	}

	var candidates []string
	if storeEntry != nil {
		add(storeEntry.SessionFile, &candidates)
	}
	if storePath != "" && sessionID != "" {
		add(filepath.Join(filepath.Dir(storePath), sessionID+".jsonl"), &candidates)
	}
	if sessionID != "" {
		add(sessions.ResolveSessionFilePath(sessionID, nil, agentID), &candidates)
	}
	return candidates
}

func resolveExistingUsageTranscript(sessionID string, storeEntry *SessionEntry, storePath, agentID string) (string, int64) {
	for _, candidate := range resolveUsageTranscriptCandidates(sessionID, storeEntry, storePath, agentID) {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, info.ModTime().UnixMilli()
	}
	return "", 0
}

func discoverStoreSessionsForUsage(
	storeEntries map[string]*SessionEntry,
	storePath string,
	startMs, endMs int64,
) []discoveredSession {
	if len(storeEntries) == 0 {
		return nil
	}
	results := make([]discoveredSession, 0, len(storeEntries))
	for key, entry := range storeEntries {
		if entry == nil || strings.TrimSpace(entry.SessionId) == "" {
			continue
		}
		agentID := ResolveSessionStoreAgentId(nil, key)
		if agentID == "" {
			agentID = "main"
		}
		sessionFile, mtime := resolveExistingUsageTranscript(entry.SessionId, entry, storePath, agentID)
		if sessionFile == "" {
			continue
		}
		updatedAt := mtime
		if entry.UpdatedAt > updatedAt {
			updatedAt = entry.UpdatedAt
		}
		if updatedAt < startMs || updatedAt > endMs {
			continue
		}
		results = append(results, discoveredSession{
			sessionID:   entry.SessionId,
			sessionFile: sessionFile,
			agentID:     agentID,
			mtime:       updatedAt,
		})
	}
	return results
}

func discoverLegacyTopLevelSessions(storePath string, startMs, endMs int64) []discoveredSession {
	if storePath == "" {
		return nil
	}
	root := filepath.Dir(storePath)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	results := make([]discoveredSession, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		mtime := info.ModTime().UnixMilli()
		if mtime < startMs || mtime > endMs {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		results = append(results, discoveredSession{
			sessionID:   sessionID,
			sessionFile: filepath.Join(root, entry.Name()),
			agentID:     "main",
			mtime:       mtime,
		})
	}
	return results
}

func mergeDiscoveredSessions(sources ...[]discoveredSession) []discoveredSession {
	merged := make(map[string]discoveredSession)
	for _, source := range sources {
		for _, session := range source {
			key := strings.TrimSpace(session.sessionFile)
			if key == "" {
				key = strings.TrimSpace(session.sessionID)
			}
			if key == "" {
				continue
			}
			if existing, ok := merged[key]; ok && existing.mtime >= session.mtime {
				continue
			}
			merged[key] = session
		}
	}
	results := make([]discoveredSession, 0, len(merged))
	for _, session := range merged {
		results = append(results, session)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].mtime > results[j].mtime })
	return results
}

// ---------- Session Cost 从 JSONL 聚合 ----------

func readIntFromMap(raw map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func readFloatFromMap(raw map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed
		case int:
			return float64(typed)
		case int64:
			return float64(typed)
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func readToolCallsFromEntry(entry map[string]interface{}) map[string]int {
	raw, ok := entry["toolCalls"]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]int:
		if len(typed) == 0 {
			return nil
		}
		cloned := make(map[string]int, len(typed))
		for name, count := range typed {
			if strings.TrimSpace(name) == "" || count <= 0 {
				continue
			}
			cloned[name] = count
		}
		return cloned
	case map[string]interface{}:
		if len(typed) == 0 {
			return nil
		}
		cloned := make(map[string]int, len(typed))
		for name, value := range typed {
			if strings.TrimSpace(name) == "" {
				continue
			}
			switch count := value.(type) {
			case float64:
				if int(count) > 0 {
					cloned[name] = int(count)
				}
			case int:
				if count > 0 {
					cloned[name] = count
				}
			case int64:
				if count > 0 {
					cloned[name] = int(count)
				}
			}
		}
		if len(cloned) == 0 {
			return nil
		}
		return cloned
	default:
		return nil
	}
}

func flattenTranscriptContent(entry map[string]interface{}) string {
	content, ok := entry["content"]
	if !ok {
		return ""
	}
	if text, ok := content.(string); ok {
		return strings.TrimSpace(text)
	}
	blocks, ok := content.([]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		bm, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := bm["type"].(string)
		switch blockType {
		case "text":
			if text, _ := bm["text"].(string); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		case "tool_use":
			name, _ := bm["name"].(string)
			if name == "" {
				name = "unknown"
			}
			parts = append(parts, fmt.Sprintf("[Tool: %s]", name))
		case "tool_result":
			if text, _ := bm["result_text"].(string); strings.TrimSpace(text) != "" {
				parts = append(parts, "[Tool Result] "+text)
			} else {
				parts = append(parts, "[Tool Result]")
			}
		case "document":
			name, _ := bm["fileName"].(string)
			if strings.TrimSpace(name) == "" {
				name = "document"
			}
			parts = append(parts, "[File: "+name+"]")
		case "audio":
			parts = append(parts, "[Audio]")
		case "video":
			parts = append(parts, "[Video]")
		case "image":
			parts = append(parts, "[Image]")
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func parseUsageFromEntry(entry map[string]interface{}) (usageTotals, bool) {
	raw, ok := entry["usage"].(map[string]interface{})
	if !ok {
		return usageTotals{}, false
	}
	result := usageTotals{
		Input:      readIntFromMap(raw, "input", "inputTokens", "input_tokens", "promptTokens", "prompt_tokens"),
		Output:     readIntFromMap(raw, "output", "outputTokens", "output_tokens", "completionTokens", "completion_tokens"),
		CacheRead:  readIntFromMap(raw, "cacheRead", "cache_read", "cacheReadInputTokens", "cache_read_input_tokens"),
		CacheWrite: readIntFromMap(raw, "cacheWrite", "cache_write", "cacheCreationInputTokens", "cache_creation_input_tokens"),
	}
	if costRaw, ok := raw["cost"].(map[string]interface{}); ok {
		result.InputCost = readFloatFromMap(costRaw, "input")
		result.OutputCost = readFloatFromMap(costRaw, "output")
		result.CacheReadCost = readFloatFromMap(costRaw, "cacheRead", "cache_read")
		result.CacheWriteCost = readFloatFromMap(costRaw, "cacheWrite", "cache_write")
		result.TotalCost = readFloatFromMap(costRaw, "total", "totalCost", "total_cost", "totalCostUsd", "total_cost_usd")
	}
	if result.TotalCost == 0 {
		result.InputCost = readFloatFromMap(raw, "inputCost", "input_cost")
		result.OutputCost = readFloatFromMap(raw, "outputCost", "output_cost")
		result.CacheReadCost = readFloatFromMap(raw, "cacheReadCost", "cache_read_cost")
		result.CacheWriteCost = readFloatFromMap(raw, "cacheWriteCost", "cache_write_cost")
		result.TotalCost = readFloatFromMap(raw, "totalCost", "total_cost", "totalCostUsd", "total_cost_usd")
	}
	result.TotalTokens = result.Input + result.Output + result.CacheRead + result.CacheWrite
	if result.TotalTokens == 0 {
		result.TotalTokens = readIntFromMap(raw, "totalTokens", "total_tokens", "total")
	}
	hasUsage := result.Input > 0 || result.Output > 0 || result.CacheRead > 0 || result.CacheWrite > 0 || result.TotalCost > 0 || result.TotalTokens > 0
	return result, hasUsage
}

func estimateUsageFromEntry(entry map[string]interface{}, role string) (usageTotals, bool) {
	if role != "user" && role != "assistant" {
		return usageTotals{}, false
	}
	content := flattenTranscriptContent(entry)
	if content == "" {
		return usageTotals{}, false
	}
	tokens := agentsession.EstimatePromptTokens(content)
	if tokens <= 0 {
		return usageTotals{}, false
	}
	result := usageTotals{TotalTokens: tokens}
	if role == "user" {
		result.Input = tokens
	} else {
		result.Output = tokens
	}
	return result, true
}

func calculateUsageCost(model string, usage *usageTotals) bool {
	if usage == nil || strings.TrimSpace(model) == "" {
		return false
	}
	if usage.TotalCost > 0 || usage.InputCost > 0 || usage.OutputCost > 0 || usage.CacheReadCost > 0 || usage.CacheWriteCost > 0 {
		return true
	}
	cost := models.LookupZenModelCost(model)
	if cost == nil {
		return false
	}
	usage.InputCost = float64(usage.Input) * cost.Input / 1_000_000
	usage.OutputCost = float64(usage.Output) * cost.Output / 1_000_000
	usage.CacheReadCost = float64(usage.CacheRead) * cost.CacheRead / 1_000_000
	usage.CacheWriteCost = float64(usage.CacheWrite) * cost.CacheWrite / 1_000_000
	usage.TotalCost = usage.InputCost + usage.OutputCost + usage.CacheReadCost + usage.CacheWriteCost
	return usage.TotalCost > 0
}

func buildLatencySummary(latencies map[string][]float64) *latencySummary {
	values := make([]float64, 0)
	for _, dateValues := range latencies {
		values = append(values, dateValues...)
	}
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	index := int(math.Ceil(0.95*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return &latencySummary{
		Count: len(values),
		AvgMs: math.Round(sum/float64(len(values))*100) / 100,
		P95Ms: values[index],
		MinMs: values[0],
		MaxMs: values[len(values)-1],
	}
}

// loadSessionCostFromFile 从 JSONL 转录文件聚合 token 和消息统计。
func loadSessionCostFromFile(sessionFile string, filter *usageDateRange) *sessionCostSummary {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil
	}

	counts := &messageCounts{}
	totals := usageTotals{}
	byModel := map[string]*usageTotals{}
	byModelCounts := map[string]int{}
	toolNames := map[string]int{}
	var firstTs, lastTs int64
	dateSet := map[string]struct{}{}
	dailyMap := map[string]*dailyUsageEntry{}
	dailyTotalsMap := map[string]*usageCostDailyEntry{}
	dailyMsgMap := map[string]*dailyMsgCountEntry{}
	dailyModelMap := map[string]*dailyModelEntry{} // key: date::provider::model
	latencyByDate := map[string][]float64{}        // date → latency values in ms
	var lastUserTs int64                           // 延迟计算: user→assistant 时间差
	var actualUsageSeen bool
	var estimatedUsageSeen bool
	var lastProvider, lastModel string
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		role, _ := entry["role"].(string)

		// 提取 timestamp (提前到 role 计数后、usage 提取前，供后续按日聚合使用)
		var msgTs int64
		if ts, ok := entry["timestamp"].(float64); ok && ts > 0 {
			msgTs = int64(ts)
		} else if ts := readIntFromMap(entry, "timestampMs"); ts > 0 {
			msgTs = int64(ts)
		}
		if filter != nil && (msgTs <= 0 || msgTs < filter.startMs || msgTs > filter.endMs) {
			continue
		}
		if msgTs > 0 {
			if firstTs == 0 || msgTs < firstTs {
				firstTs = msgTs
			}
			if msgTs > lastTs {
				lastTs = msgTs
			}
			dateSet[time.UnixMilli(msgTs).Format("2006-01-02")] = struct{}{}
		}

		switch role {
		case "user":
			counts.Total++
			counts.User++
		case "assistant":
			counts.Total++
			counts.Assistant++
		case "tool":
			counts.Total++
			counts.ToolCalls++
		case "tool_result", "toolResult":
			counts.Total++
			counts.ToolResults++
		}

		// 延迟近似计算 (D-03): user ts → first assistant ts
		if role == "user" && msgTs > 0 {
			lastUserTs = msgTs
		} else if role == "assistant" && msgTs > 0 && lastUserTs > 0 {
			latencyMs := float64(msgTs - lastUserTs)
			if latencyMs > 0 && latencyMs < 600_000 { // <10 分钟为有效延迟
				dateStr := time.UnixMilli(msgTs).Format("2006-01-02")
				latencyByDate[dateStr] = append(latencyByDate[dateStr], latencyMs)
			}
			lastUserTs = 0 // 只取 user 后第一条 assistant
		}

		// 提取 model / provider
		model, _ := entry["model"].(string)
		provider, _ := entry["provider"].(string)
		if strings.TrimSpace(provider) != "" {
			lastProvider = provider
		}
		if strings.TrimSpace(model) != "" {
			lastModel = model
		}
		modelKey := ""
		if model != "" {
			if provider != "" {
				modelKey = provider + "::" + model
			} else {
				modelKey = model
			}
		}

		msgUsage, hasActualUsage := parseUsageFromEntry(entry)
		if hasActualUsage {
			actualUsageSeen = true
			_ = calculateUsageCost(model, &msgUsage)
			if msgUsage.TotalCost == 0 && model != "" && msgUsage.TotalTokens > 0 {
				totals.MissingCostEntries++
			}
		} else if estimated, ok := estimateUsageFromEntry(entry, role); ok {
			msgUsage = estimated
			estimatedUsageSeen = true
		}
		mergeTotalsInto(&totals, &msgUsage)

		// 按 model 聚合 (含 cache + cost)
		if modelKey != "" && msgUsage.TotalTokens > 0 {
			mt := byModel[modelKey]
			if mt == nil {
				mt = &usageTotals{}
				byModel[modelKey] = mt
			}
			mergeTotalsInto(mt, &msgUsage)
			byModelCounts[modelKey]++
		}

		// 按日聚合 token + cost
		if msgTs > 0 && msgUsage.TotalTokens > 0 {
			dateStr := time.UnixMilli(msgTs).Format("2006-01-02")
			dt := dailyMap[dateStr]
			if dt == nil {
				dt = &dailyUsageEntry{Date: dateStr}
				dailyMap[dateStr] = dt
			}
			dt.Tokens += msgUsage.TotalTokens
			dt.Cost += msgUsage.TotalCost
			mergeTotalsInto(&dt.usageTotals, &msgUsage)

			totalsEntry := dailyTotalsMap[dateStr]
			if totalsEntry == nil {
				totalsEntry = &usageCostDailyEntry{Date: dateStr}
				dailyTotalsMap[dateStr] = totalsEntry
			}
			mergeTotalsInto(&totalsEntry.usageTotals, &msgUsage)
		}

		// 按日+模型聚合 (D-01)
		if msgTs > 0 && modelKey != "" && msgUsage.TotalTokens > 0 {
			dateStr := time.UnixMilli(msgTs).Format("2006-01-02")
			dmKey := dateStr + "::" + modelKey
			dm := dailyModelMap[dmKey]
			if dm == nil {
				prov, mod := "", model
				if parts := strings.SplitN(modelKey, "::", 2); len(parts) == 2 {
					prov = parts[0]
					mod = parts[1]
				}
				dm = &dailyModelEntry{Date: dateStr, Provider: prov, Model: mod}
				dailyModelMap[dmKey] = dm
			}
			dm.Tokens += msgUsage.TotalTokens
			dm.Cost += msgUsage.TotalCost
			dm.Count++
		}

		// 工具调用计数
		var msgToolCalls, msgToolResults, msgErrors int
		persistedToolCalls := readToolCallsFromEntry(entry)
		if len(persistedToolCalls) > 0 {
			for name, count := range persistedToolCalls {
				msgToolCalls += count
				toolNames[name] += count
			}
		}
		if content, ok := entry["content"].([]interface{}); ok {
			for _, block := range content {
				if bm, ok := block.(map[string]interface{}); ok {
					if tp, ok := bm["type"].(string); ok {
						if tp == "tool_use" {
							if len(persistedToolCalls) == 0 {
								msgToolCalls++
								if tn, ok := bm["name"].(string); ok && tn != "" {
									toolNames[tn]++
								}
							}
						} else if tp == "tool_result" {
							msgToolResults++
							if isErr, _ := bm["is_error"].(bool); isErr {
								counts.Errors++
								msgErrors++
							}
						}
					}
				}
			}
		}
		counts.ToolCalls += msgToolCalls

		// 按日聚合消息计数
		if msgTs > 0 && role != "" {
			dateStr := time.UnixMilli(msgTs).Format("2006-01-02")
			dm := dailyMsgMap[dateStr]
			if dm == nil {
				dm = &dailyMsgCountEntry{Date: dateStr}
				dailyMsgMap[dateStr] = dm
			}
			switch role {
			case "user":
				dm.Total++
				dm.User++
			case "assistant":
				dm.Total++
				dm.Assistant++
			}
			dm.ToolCalls += msgToolCalls
			dm.ToolResults += msgToolResults
			dm.Errors += msgErrors
		}
	}

	// 构建 activityDates (排序)
	activityDates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		activityDates = append(activityDates, d)
	}
	sort.Strings(activityDates)

	// 构建 dailyBreakdown (排序)
	dailyBreakdown := make([]dailyUsageEntry, 0, len(dailyMap))
	for _, dt := range dailyMap {
		dailyBreakdown = append(dailyBreakdown, *dt)
	}
	sort.Slice(dailyBreakdown, func(i, j int) bool {
		return dailyBreakdown[i].Date < dailyBreakdown[j].Date
	})

	dailyTotals := make([]usageCostDailyEntry, 0, len(dailyTotalsMap))
	for _, dt := range dailyTotalsMap {
		dailyTotals = append(dailyTotals, *dt)
	}
	sort.Slice(dailyTotals, func(i, j int) bool {
		return dailyTotals[i].Date < dailyTotals[j].Date
	})

	// 构建 dailyMessageCounts (排序)
	dailyMsgCounts := make([]dailyMsgCountEntry, 0, len(dailyMsgMap))
	for _, dm := range dailyMsgMap {
		dailyMsgCounts = append(dailyMsgCounts, *dm)
	}
	sort.Slice(dailyMsgCounts, func(i, j int) bool {
		return dailyMsgCounts[i].Date < dailyMsgCounts[j].Date
	})

	// 构建 dailyModelUsage (D-01: 按 date 然后 model 排序)
	dailyModelUsage := make([]dailyModelEntry, 0, len(dailyModelMap))
	for _, dm := range dailyModelMap {
		dailyModelUsage = append(dailyModelUsage, *dm)
	}
	sort.Slice(dailyModelUsage, func(i, j int) bool {
		if dailyModelUsage[i].Date != dailyModelUsage[j].Date {
			return dailyModelUsage[i].Date < dailyModelUsage[j].Date
		}
		return dailyModelUsage[i].Model < dailyModelUsage[j].Model
	})

	// 构建 dailyLatency (D-03)
	dailyLatency := make([]dailyLatencyEntry, 0, len(latencyByDate))
	for dateStr, vals := range latencyByDate {
		if len(vals) == 0 {
			continue
		}
		sort.Float64s(vals)
		n := len(vals)
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		p95Idx := int(math.Ceil(0.95*float64(n))) - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		if p95Idx >= n {
			p95Idx = n - 1
		}
		dailyLatency = append(dailyLatency, dailyLatencyEntry{
			Date:  dateStr,
			Count: n,
			AvgMs: math.Round(sum/float64(n)*100) / 100,
			P95Ms: vals[p95Idx],
			MinMs: vals[0],
			MaxMs: vals[n-1],
		})
	}
	sort.Slice(dailyLatency, func(i, j int) bool {
		return dailyLatency[i].Date < dailyLatency[j].Date
	})

	var durationMs int64
	if lastTs > firstTs {
		durationMs = lastTs - firstTs
	}

	modelUsage := make([]modelUsageEntry, 0, len(byModel))
	for key, totalsForModel := range byModel {
		provider := ""
		model := key
		if parts := strings.SplitN(key, "::", 2); len(parts) == 2 {
			provider = parts[0]
			model = parts[1]
		}
		modelUsage = append(modelUsage, modelUsageEntry{
			Provider: provider,
			Model:    model,
			Count:    byModelCounts[key],
			Totals:   cloneUsageTotals(totalsForModel),
		})
	}
	sort.Slice(modelUsage, func(i, j int) bool {
		return modelUsage[i].Totals.TotalCost > modelUsage[j].Totals.TotalCost
	})

	toolUsage := &toolUsageSummary{
		UniqueTools: len(toolNames),
		Tools:       []toolUsageEntry{},
	}
	for name, count := range toolNames {
		toolUsage.TotalCalls += count
		toolUsage.Tools = append(toolUsage.Tools, toolUsageEntry{Name: name, Count: count})
	}
	sort.Slice(toolUsage.Tools, func(i, j int) bool {
		if toolUsage.Tools[i].Count != toolUsage.Tools[j].Count {
			return toolUsage.Tools[i].Count > toolUsage.Tools[j].Count
		}
		return toolUsage.Tools[i].Name < toolUsage.Tools[j].Name
	})

	usageSource := ""
	switch {
	case actualUsageSeen && estimatedUsageSeen:
		usageSource = "mixed"
	case actualUsageSeen:
		usageSource = "actual"
	case estimatedUsageSeen:
		usageSource = "estimated"
	}

	return &sessionCostSummary{
		usageTotals:        totals,
		MessageCounts:      counts,
		ToolUsage:          toolUsage,
		ModelUsage:         modelUsage,
		Latency:            buildLatencySummary(latencyByDate),
		UsageSource:        usageSource,
		ByModel:            byModel,
		ByModelCounts:      byModelCounts,
		ToolNames:          toolNames,
		FirstActivity:      firstTs,
		LastActivity:       lastTs,
		DurationMs:         durationMs,
		ActivityDates:      activityDates,
		DailyBreakdown:     dailyBreakdown,
		DailyTotals:        dailyTotals,
		DailyMessageCounts: dailyMsgCounts,
		DailyModelUsage:    dailyModelUsage,
		DailyLatency:       dailyLatency,
		rawLatencyData:     latencyByDate,
		lastProvider:       lastProvider,
		lastModel:          lastModel,
	}
}

func cloneUsageTotals(src *usageTotals) *usageTotals {
	if src == nil {
		return &usageTotals{}
	}
	cloned := *src
	return &cloned
}

func collectUsageDiscoveredSessions(
	ctx *MethodHandlerContext,
	cfg *types.OpenAcosmiConfig,
	storeEntries map[string]*SessionEntry,
	dr usageDateRange,
) []discoveredSession {
	agentIds := []string{"main"}
	if cfg != nil {
		agentIds = scope.ListAgentIds(cfg)
	}
	return mergeDiscoveredSessions(
		discoverStoreSessionsForUsage(storeEntries, ctx.Context.StorePath, dr.startMs, dr.endMs),
		discoverLegacyTopLevelSessions(ctx.Context.StorePath, dr.startMs, dr.endMs),
		discoverAllSessionsForUsage(nil, dr.startMs, dr.endMs, agentIds),
	)
}

func resolveUsageSessionTarget(
	key string,
	storeEntries map[string]*SessionEntry,
	storePath string,
	cfg *types.OpenAcosmiConfig,
) (string, string, string, *SessionEntry) {
	rawKey := strings.TrimSpace(key)
	if rawKey == "" {
		return "", "", "", nil
	}
	if storeEntry := storeEntries[rawKey]; storeEntry != nil && strings.TrimSpace(storeEntry.SessionId) != "" {
		agentID := ResolveSessionStoreAgentId(cfg, rawKey)
		sessionFile, _ := resolveExistingUsageTranscript(storeEntry.SessionId, storeEntry, storePath, agentID)
		return storeEntry.SessionId, sessionFile, agentID, storeEntry
	}
	parsed := scope.ParseAgentSessionKey(rawKey)
	agentID := ""
	sessionID := rawKey
	if parsed != nil {
		agentID = parsed.AgentID
		sessionID = parsed.Rest
	}
	sessionFile, _ := resolveExistingUsageTranscript(sessionID, nil, storePath, agentID)
	return sessionID, sessionFile, agentID, nil
}

func buildUsageToolContext() (int, []usageContextToolEntry) {
	summaries := capabilities.TreeToolSummaries()
	for name, summary := range configtools.ToolSummaries() {
		if summary != "" {
			summaries[name] = summary
		}
	}
	names := capabilities.TreeToolOrder()
	seen := map[string]struct{}{}
	entries := make([]usageContextToolEntry, 0, len(names))
	totalChars := 0
	for _, name := range names {
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
		summary := summaries[name]
		lineChars := len(name)
		if summary != "" {
			lineChars += 4 + len(summary)
		}
		entries = append(entries, usageContextToolEntry{
			Name:         name,
			SummaryChars: lineChars,
			SchemaChars:  0,
		})
		totalChars += lineChars
	}
	for name, summary := range summaries {
		if _, ok := seen[name]; ok {
			continue
		}
		lineChars := len(name)
		if summary != "" {
			lineChars += 4 + len(summary)
		}
		entries = append(entries, usageContextToolEntry{
			Name:         name,
			SummaryChars: lineChars,
			SchemaChars:  0,
		})
		totalChars += lineChars
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return totalChars, entries
}

func buildUsageSkillContext(
	cfg *types.OpenAcosmiConfig,
	storeEntry *SessionEntry,
	workspaceDir string,
) (int, []usageContextSkillEntry) {
	if storeEntry == nil || storeEntry.SkillsSnapshot == nil || len(storeEntry.SkillsSnapshot.Skills) == 0 {
		return 0, nil
	}
	snapshotNames := map[string]struct{}{}
	for _, item := range storeEntry.SkillsSnapshot.Skills {
		if name := strings.TrimSpace(item.Name); name != "" {
			snapshotNames[name] = struct{}{}
		}
	}
	if len(snapshotNames) == 0 {
		return 0, nil
	}
	bundledDir := skills.ResolveBundledSkillsDir("")
	loaded := skills.LoadSkillEntries(workspaceDir, "", bundledDir, cfg)
	resolved := make([]skills.Skill, 0, len(snapshotNames))
	entries := make([]usageContextSkillEntry, 0, len(snapshotNames))
	for _, entry := range loaded {
		if _, ok := snapshotNames[entry.Skill.Name]; !ok {
			continue
		}
		resolved = append(resolved, entry.Skill)
		lineChars := len(entry.Skill.Name)
		if entry.Skill.Description != "" {
			desc := entry.Skill.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			lineChars += 4 + len(desc)
		}
		entries = append(entries, usageContextSkillEntry{
			Name:       entry.Skill.Name,
			BlockChars: lineChars,
		})
		delete(snapshotNames, entry.Skill.Name)
	}
	for name := range snapshotNames {
		resolved = append(resolved, skills.Skill{Name: name})
		entries = append(entries, usageContextSkillEntry{Name: name, BlockChars: len(name)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return len(skills.FormatSkillIndex(resolved)), entries
}

func buildUsageContextWeight(
	cfg *types.OpenAcosmiConfig,
	storeEntry *SessionEntry,
	agentID string,
) *usageContextWeight {
	if storeEntry == nil {
		return nil
	}
	workspaceDir := scope.ResolveAgentWorkspaceDir(cfg, agentID)
	if strings.TrimSpace(workspaceDir) == "" {
		workspaceDir = agentsession.ResolveProjectRootContextDir()
	}
	basePromptChars := len(prompt.BuildAgentSystemPrompt(prompt.BuildParams{
		WorkspaceDir: workspaceDir,
	}))
	skillPromptChars, skillEntries := buildUsageSkillContext(cfg, storeEntry, workspaceDir)
	toolChars, toolEntries := buildUsageToolContext()
	contextFiles := agentsession.ResolveContextFiles(workspaceDir)
	fileEntries := make([]usageContextFileEntry, 0, len(contextFiles))
	for _, file := range contextFiles {
		fileEntries = append(fileEntries, usageContextFileEntry{
			Name:          filepath.Base(file.Path),
			Path:          file.Path,
			RawChars:      len(file.Content),
			InjectedChars: len(file.Content),
			Truncated:     false,
		})
	}
	result := &usageContextWeight{}
	result.SystemPrompt.Chars = basePromptChars
	result.SystemPrompt.ProjectContextChars = 0
	result.SystemPrompt.NonProjectContextChars = basePromptChars
	result.Skills.PromptChars = skillPromptChars
	result.Skills.Entries = skillEntries
	result.Tools.ListChars = toolChars
	result.Tools.SchemaChars = 0
	result.Tools.Entries = toolEntries
	result.InjectedWorkspaceFiles = fileEntries
	if result.SystemPrompt.Chars == 0 && result.Skills.PromptChars == 0 && result.Tools.ListChars == 0 && len(result.InjectedWorkspaceFiles) == 0 {
		return nil
	}
	return result
}

// ---------- usage.status ----------

func handleUsageStatus(ctx *MethodHandlerContext) {
	ctx.Respond(true, map[string]interface{}{
		"ok":      true,
		"message": "usage status not yet fully implemented",
	}, nil)
}

// ---------- usage.cost ----------

func handleUsageCost(ctx *MethodHandlerContext) {
	dr := parseDateRange(ctx.Params)
	cacheKey := fmt.Sprintf("%d-%d", dr.startMs, dr.endMs)

	// 检查缓存
	costCacheMu.RLock()
	cached := costCache[cacheKey]
	costCacheMu.RUnlock()

	if cached != nil && time.Since(cached.updatedAt) < costUsageCacheTTL {
		ctx.Respond(true, cached.summary, nil)
		return
	}

	// 重新计算
	cfg := resolveConfigFromContext(ctx)
	var storeEntries map[string]*SessionEntry
	if ctx.Context.SessionStore != nil {
		defaultAgentID := scope.ResolveDefaultAgentId(cfg)
		storeEntries = ctx.Context.SessionStore.LoadCombinedStore(defaultAgentID)
	}

	discovered := collectUsageDiscoveredSessions(ctx, cfg, storeEntries, dr)
	totals := &usageTotals{}
	dailyMap := map[string]*usageCostDailyEntry{}
	for _, d := range discovered {
		cost := loadSessionCostFromFile(d.sessionFile, &dr)
		if cost != nil {
			mergeTotalsInto(totals, &cost.usageTotals)
			for _, day := range cost.DailyTotals {
				existing := dailyMap[day.Date]
				if existing == nil {
					existing = &usageCostDailyEntry{Date: day.Date}
					dailyMap[day.Date] = existing
				}
				mergeTotalsInto(&existing.usageTotals, &day.usageTotals)
			}
		}
	}

	daily := make([]usageCostDailyEntry, 0, len(dailyMap))
	for _, entry := range dailyMap {
		daily = append(daily, *entry)
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })
	summary := &usageCostSummary{
		UpdatedAt: time.Now().UnixMilli(),
		Days:      len(daily),
		Daily:     daily,
		Totals:    totals,
	}

	costCacheMu.Lock()
	costCache[cacheKey] = &costUsageCacheEntry{summary: summary, updatedAt: time.Now()}
	costCacheMu.Unlock()

	ctx.Respond(true, summary, nil)
}

// ---------- sessions.usage ----------

func handleSessionsUsage(ctx *MethodHandlerContext) {
	cfg := resolveConfigFromContext(ctx)
	if cfg == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config not available"))
		return
	}

	dr := parseDateRange(ctx.Params)
	limit := 50
	if v, ok := ctx.Params["limit"].(float64); ok && !math.IsNaN(v) {
		limit = int(v)
	}
	defaultAgentId := scope.ResolveDefaultAgentId(cfg)
	includeContextWeight, _ := ctx.Params["includeContextWeight"].(bool)

	// 加载 session store
	store := ctx.Context.SessionStore
	var storeEntries map[string]*SessionEntry
	if store != nil {
		storeEntries = store.LoadCombinedStore(defaultAgentId)
	}

	// Session discovery + store 合并
	type mergedEntry struct {
		key         string
		sessionID   string
		sessionFile string
		label       string
		updatedAt   int64
		agentID     string
		storeEntry  *SessionEntry
	}

	var merged []mergedEntry

	// 检查是否请求特定 key
	specificKey := ""
	if v, ok := ctx.Params["key"].(string); ok {
		specificKey = strings.TrimSpace(v)
	}

	if specificKey != "" {
		sessionID, sessionFile, agentID, storeEntry := resolveUsageSessionTarget(
			specificKey,
			storeEntries,
			ctx.Context.StorePath,
			cfg,
		)
		if sessionFile != "" {
			updatedAt := time.Now().UnixMilli()
			if info, err := os.Stat(sessionFile); err == nil {
				updatedAt = info.ModTime().UnixMilli()
			}
			if storeEntry != nil && storeEntry.UpdatedAt > updatedAt {
				updatedAt = storeEntry.UpdatedAt
			}
			merged = append(merged, mergedEntry{
				key:         specificKey,
				sessionID:   sessionID,
				sessionFile: sessionFile,
				label: func() string {
					if storeEntry != nil {
						return storeEntry.Label
					}
					return ""
				}(),
				updatedAt:  updatedAt,
				agentID:    agentID,
				storeEntry: storeEntry,
			})
		}
	} else {
		discovered := collectUsageDiscoveredSessions(ctx, cfg, storeEntries, dr)

		// 构建 sessionID → store entry 映射
		storeBySessionID := map[string]struct {
			key   string
			entry *SessionEntry
		}{}
		for k, e := range storeEntries {
			if e != nil && e.SessionId != "" {
				storeBySessionID[e.SessionId] = struct {
					key   string
					entry *SessionEntry
				}{key: k, entry: e}
			}
		}

		for _, d := range discovered {
			if match, ok := storeBySessionID[d.sessionID]; ok {
				updAt := d.mtime
				if match.entry.UpdatedAt > 0 {
					updAt = match.entry.UpdatedAt
				}
				merged = append(merged, mergedEntry{
					key: match.key, sessionID: d.sessionID, sessionFile: d.sessionFile,
					label: match.entry.Label, updatedAt: updAt, agentID: d.agentID,
					storeEntry: match.entry,
				})
			} else {
				merged = append(merged, mergedEntry{
					key:         fmt.Sprintf("agent:%s:%s", d.agentID, d.sessionID),
					sessionID:   d.sessionID,
					sessionFile: d.sessionFile,
					updatedAt:   d.mtime,
					agentID:     d.agentID,
				})
			}
		}
	}

	// 按 updatedAt 降序
	sort.Slice(merged, func(i, j int) bool { return merged[i].updatedAt > merged[j].updatedAt })

	// 限制
	if len(merged) > limit {
		merged = merged[:limit]
	}

	// 聚合
	aggTotals := &usageTotals{}
	aggMsgs := &messageCounts{}
	// byModel: provider::model → *usageTotals
	globalByModel := map[string]*usageTotals{}
	globalByModelCounts := map[string]int{}
	// byAgent: agentID → *usageTotals
	globalByAgent := map[string]*usageTotals{}
	// tools: toolName → callCount
	globalToolNames := map[string]int{}
	// daily: date → aggregated entry
	type globalDailyEntry struct {
		Tokens    int     `json:"tokens"`
		Cost      float64 `json:"cost"`
		Messages  int     `json:"messages"`
		ToolCalls int     `json:"toolCalls"`
		Errors    int     `json:"errors"`
	}
	globalDaily := map[string]*globalDailyEntry{}
	globalDailyModel := map[string]*dailyModelEntry{}
	globalLatencyByDate := map[string][]float64{}
	globalByChannel := map[string]*usageTotals{}
	globalByProvider := map[string]*usageTotals{}
	globalByProviderCounts := map[string]int{}

	sessionsOut := make([]map[string]interface{}, 0, len(merged))

	for _, m := range merged {
		cost := loadSessionCostFromFile(m.sessionFile, &dr)
		resolvedProvider, resolvedModel := ResolveSessionModelRef(cfg, m.storeEntry, m.agentID)
		if cost != nil && len(cost.ModelUsage) == 0 && resolvedModel != "" {
			count := 1
			if cost.MessageCounts != nil && cost.MessageCounts.Assistant > 0 {
				count = cost.MessageCounts.Assistant
			}
			cost.ModelUsage = []modelUsageEntry{{
				Provider: resolvedProvider,
				Model:    resolvedModel,
				Count:    count,
				Totals:   cloneUsageTotals(&cost.usageTotals),
			}}
		}
		displayProvider := resolvedProvider
		displayModel := resolvedModel
		if cost != nil {
			if strings.TrimSpace(cost.lastProvider) != "" {
				displayProvider = cost.lastProvider
			}
			if strings.TrimSpace(cost.lastModel) != "" {
				displayModel = cost.lastModel
			}
		}

		if cost != nil {
			mergeTotalsInto(aggTotals, &cost.usageTotals)
			if cost.MessageCounts != nil {
				aggMsgs.Total += cost.MessageCounts.Total
				aggMsgs.User += cost.MessageCounts.User
				aggMsgs.Assistant += cost.MessageCounts.Assistant
				aggMsgs.ToolCalls += cost.MessageCounts.ToolCalls
				aggMsgs.ToolResults += cost.MessageCounts.ToolResults
				aggMsgs.Errors += cost.MessageCounts.Errors
			}
			// 合并 toolNames
			for tn, cnt := range cost.ToolNames {
				globalToolNames[tn] += cnt
			}
			for _, modelEntry := range cost.ModelUsage {
				modelKey := modelEntry.Model
				if modelEntry.Provider != "" {
					modelKey = modelEntry.Provider + "::" + modelEntry.Model
				}
				if globalByModel[modelKey] == nil {
					globalByModel[modelKey] = &usageTotals{}
				}
				mergeTotalsInto(globalByModel[modelKey], modelEntry.Totals)
				globalByModelCounts[modelKey] += modelEntry.Count

				providerKey := modelEntry.Provider
				if providerKey == "" {
					providerKey = "unknown"
				}
				if globalByProvider[providerKey] == nil {
					globalByProvider[providerKey] = &usageTotals{}
				}
				mergeTotalsInto(globalByProvider[providerKey], modelEntry.Totals)
				globalByProviderCounts[providerKey] += modelEntry.Count
			}
			// 合并 byAgent
			if m.agentID != "" {
				if globalByAgent[m.agentID] == nil {
					globalByAgent[m.agentID] = &usageTotals{}
				}
				mergeTotalsInto(globalByAgent[m.agentID], &cost.usageTotals)
			}
			// 合并 daily tokens
			for _, d := range cost.DailyBreakdown {
				gd := globalDaily[d.Date]
				if gd == nil {
					gd = &globalDailyEntry{}
					globalDaily[d.Date] = gd
				}
				gd.Tokens += d.Tokens
				gd.Cost += d.Cost
			}
			// 合并 daily message counts
			for _, dm := range cost.DailyMessageCounts {
				gd := globalDaily[dm.Date]
				if gd == nil {
					gd = &globalDailyEntry{}
					globalDaily[dm.Date] = gd
				}
				gd.Messages += dm.Total
				gd.ToolCalls += dm.ToolCalls
				gd.Errors += dm.Errors
			}
			// 合并 dailyModelUsage (D-01)
			for _, dm := range cost.DailyModelUsage {
				key := dm.Date + "::" + dm.Provider + "::" + dm.Model
				gm := globalDailyModel[key]
				if gm == nil {
					gm = &dailyModelEntry{Date: dm.Date, Provider: dm.Provider, Model: dm.Model}
					globalDailyModel[key] = gm
				}
				gm.Tokens += dm.Tokens
				gm.Cost += dm.Cost
				gm.Count += dm.Count
			}
			// 合并 rawLatencyData (D-03)
			for dateStr, vals := range cost.rawLatencyData {
				globalLatencyByDate[dateStr] = append(globalLatencyByDate[dateStr], vals...)
			}
		}

		// 按频道聚合 (D-04)
		ch := "direct"
		if m.storeEntry != nil && m.storeEntry.Channel != "" {
			ch = m.storeEntry.Channel
		}
		if cost != nil {
			if globalByChannel[ch] == nil {
				globalByChannel[ch] = &usageTotals{}
			}
			mergeTotalsInto(globalByChannel[ch], &cost.usageTotals)
		}

		entry := map[string]interface{}{
			"key":           m.key,
			"sessionId":     m.sessionID,
			"updatedAt":     m.updatedAt,
			"agentId":       m.agentID,
			"usage":         cost,
			"modelProvider": displayProvider,
			"model":         displayModel,
		}
		if m.label != "" {
			entry["label"] = m.label
		}
		// Session 元数据 (D-05)
		if m.storeEntry != nil {
			if m.storeEntry.Subject != "" {
				entry["subject"] = m.storeEntry.Subject
			}
			if m.storeEntry.GroupChannel != "" {
				entry["room"] = m.storeEntry.GroupChannel
			}
			if m.storeEntry.Space != "" {
				entry["space"] = m.storeEntry.Space
			}
			if m.storeEntry.Channel != "" {
				entry["channel"] = m.storeEntry.Channel
			}
			if m.storeEntry.ChatType != "" {
				entry["chatType"] = m.storeEntry.ChatType
			}
			if m.storeEntry.Origin != nil {
				entry["origin"] = m.storeEntry.Origin
			}
			if m.storeEntry.ModelOverride != "" {
				entry["modelOverride"] = m.storeEntry.ModelOverride
			}
			if m.storeEntry.ProviderOverride != "" {
				entry["providerOverride"] = m.storeEntry.ProviderOverride
			}
			if includeContextWeight {
				if contextWeight := buildUsageContextWeight(cfg, m.storeEntry, m.agentID); contextWeight != nil {
					entry["contextWeight"] = contextWeight
				}
			}
		}
		sessionsOut = append(sessionsOut, entry)
	}

	// 构建 byModel 数组
	byModelArr := make([]map[string]interface{}, 0, len(globalByModel))
	for mk, mt := range globalByModel {
		parts := strings.SplitN(mk, "::", 2)
		provider := ""
		model := mk
		if len(parts) == 2 {
			provider = parts[0]
			model = parts[1]
		}
		byModelArr = append(byModelArr, map[string]interface{}{
			"model":    model,
			"provider": provider,
			"count":    globalByModelCounts[mk],
			"totals":   mt,
		})
	}
	sort.Slice(byModelArr, func(i, j int) bool {
		left, _ := byModelArr[i]["totals"].(*usageTotals)
		right, _ := byModelArr[j]["totals"].(*usageTotals)
		if left == nil || right == nil {
			return i < j
		}
		return left.TotalCost > right.TotalCost
	})

	// 构建 byProvider 数组
	byProviderArr := make([]map[string]interface{}, 0, len(globalByProvider))
	for p, t := range globalByProvider {
		byProviderArr = append(byProviderArr, map[string]interface{}{
			"provider": p,
			"count":    globalByProviderCounts[p],
			"totals":   t,
		})
	}
	sort.Slice(byProviderArr, func(i, j int) bool {
		left, _ := byProviderArr[i]["totals"].(*usageTotals)
		right, _ := byProviderArr[j]["totals"].(*usageTotals)
		if left == nil || right == nil {
			return i < j
		}
		return left.TotalCost > right.TotalCost
	})

	// 构建 byAgent 数组
	byAgentArr := make([]map[string]interface{}, 0, len(globalByAgent))
	for aid, t := range globalByAgent {
		byAgentArr = append(byAgentArr, map[string]interface{}{
			"agentId": aid,
			"totals":  t,
		})
	}

	// 构建 tools 数组
	toolsArr := make([]map[string]interface{}, 0, len(globalToolNames))
	for tn, cnt := range globalToolNames {
		toolsArr = append(toolsArr, map[string]interface{}{
			"name":  tn,
			"count": cnt,
		})
	}
	sort.Slice(toolsArr, func(i, j int) bool {
		ci, _ := toolsArr[i]["count"].(int)
		cj, _ := toolsArr[j]["count"].(int)
		return ci > cj
	})

	// 构建 daily 数组 (排序)
	dailyArr := make([]map[string]interface{}, 0, len(globalDaily))
	for d, gd := range globalDaily {
		dailyArr = append(dailyArr, map[string]interface{}{
			"date":      d,
			"tokens":    gd.Tokens,
			"cost":      gd.Cost,
			"messages":  gd.Messages,
			"toolCalls": gd.ToolCalls,
			"errors":    gd.Errors,
		})
	}
	sort.Slice(dailyArr, func(i, j int) bool {
		di, _ := dailyArr[i]["date"].(string)
		dj, _ := dailyArr[j]["date"].(string)
		return di < dj
	})

	// 构建 modelDaily 数组 (D-01)
	modelDailyArr := make([]map[string]interface{}, 0, len(globalDailyModel))
	for _, gm := range globalDailyModel {
		modelDailyArr = append(modelDailyArr, map[string]interface{}{
			"date":     gm.Date,
			"provider": gm.Provider,
			"model":    gm.Model,
			"tokens":   gm.Tokens,
			"cost":     gm.Cost,
			"count":    gm.Count,
		})
	}
	sort.Slice(modelDailyArr, func(i, j int) bool {
		di, _ := modelDailyArr[i]["date"].(string)
		dj, _ := modelDailyArr[j]["date"].(string)
		if di != dj {
			return di < dj
		}
		mi, _ := modelDailyArr[i]["model"].(string)
		mj, _ := modelDailyArr[j]["model"].(string)
		return mi < mj
	})

	// 构建全局 dailyLatency (D-03)
	aggDailyLatency := make([]map[string]interface{}, 0, len(globalLatencyByDate))
	for dateStr, vals := range globalLatencyByDate {
		if len(vals) == 0 {
			continue
		}
		sort.Float64s(vals)
		n := len(vals)
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		p95Idx := int(math.Ceil(0.95*float64(n))) - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		if p95Idx >= n {
			p95Idx = n - 1
		}
		aggDailyLatency = append(aggDailyLatency, map[string]interface{}{
			"date":  dateStr,
			"count": n,
			"avgMs": math.Round(sum/float64(n)*100) / 100,
			"p95Ms": vals[p95Idx],
			"minMs": vals[0],
			"maxMs": vals[n-1],
		})
	}
	sort.Slice(aggDailyLatency, func(i, j int) bool {
		di, _ := aggDailyLatency[i]["date"].(string)
		dj, _ := aggDailyLatency[j]["date"].(string)
		return di < dj
	})
	aggLatency := buildLatencySummary(globalLatencyByDate)

	// 构建 byChannel 数组 (D-04)
	byChannelArr := make([]map[string]interface{}, 0, len(globalByChannel))
	for ch, t := range globalByChannel {
		byChannelArr = append(byChannelArr, map[string]interface{}{
			"channel": ch,
			"totals":  t,
		})
	}

	now := time.Now()
	ctx.Respond(true, map[string]interface{}{
		"updatedAt": now.UnixMilli(),
		"startDate": formatDateStr(dr.startMs),
		"endDate":   formatDateStr(dr.endMs),
		"sessions":  sessionsOut,
		"totals":    aggTotals,
		"aggregates": map[string]interface{}{
			"messages": aggMsgs,
			"tools": map[string]interface{}{
				"totalCalls":  aggMsgs.ToolCalls,
				"uniqueTools": len(globalToolNames),
				"tools":       toolsArr,
			},
			"byModel":      byModelArr,
			"byProvider":   byProviderArr,
			"byAgent":      byAgentArr,
			"byChannel":    byChannelArr,
			"latency":      aggLatency,
			"dailyLatency": aggDailyLatency,
			"modelDaily":   modelDailyArr,
			"daily":        dailyArr,
		},
	}, nil)
}

// ---------- sessions.usage.timeseries ----------

func handleSessionsUsageTimeseries(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if strings.TrimSpace(key) == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key is required for timeseries"))
		return
	}

	cfg := resolveConfigFromContext(ctx)
	var storeEntries map[string]*SessionEntry
	if ctx.Context.SessionStore != nil {
		storeEntries = ctx.Context.SessionStore.LoadCombinedStore(scope.ResolveDefaultAgentId(cfg))
	}
	sessionID, sessionFile, _, _ := resolveUsageSessionTarget(key, storeEntries, ctx.Context.StorePath, cfg)
	if sessionFile == "" {
		ctx.Respond(true, map[string]interface{}{
			"sessionId": sessionID,
			"points":    []map[string]interface{}{},
		}, nil)
		return
	}

	// 从 JSONL 提取时序数据
	points := loadTimeseriesFromFile(sessionFile)

	ctx.Respond(true, map[string]interface{}{
		"sessionId": sessionID,
		"points":    points,
	}, nil)
}

// loadTimeseriesFromFile 从 JSONL 提取每消息 usage 时序。
func loadTimeseriesFromFile(sessionFile string) []map[string]interface{} {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return []map[string]interface{}{}
	}

	var points []map[string]interface{}
	cumulativeTokens := 0
	cumulativeCost := 0.0
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		role, _ := entry["role"].(string)
		msgUsage, hasActualUsage := parseUsageFromEntry(entry)
		if hasActualUsage {
			model, _ := entry["model"].(string)
			_ = calculateUsageCost(model, &msgUsage)
		} else if estimated, ok := estimateUsageFromEntry(entry, role); ok {
			msgUsage = estimated
		}
		if msgUsage.TotalTokens == 0 && msgUsage.TotalCost == 0 {
			continue
		}

		ts, _ := entry["timestamp"].(float64)
		timestamp := int64(ts)
		if timestamp == 0 {
			timestamp = int64(readIntFromMap(entry, "timestampMs"))
		}
		if timestamp == 0 {
			continue
		}

		cumulativeTokens += msgUsage.TotalTokens
		cumulativeCost += msgUsage.TotalCost

		points = append(points, map[string]interface{}{
			"timestamp":        timestamp,
			"input":            msgUsage.Input,
			"output":           msgUsage.Output,
			"cacheRead":        msgUsage.CacheRead,
			"cacheWrite":       msgUsage.CacheWrite,
			"totalTokens":      msgUsage.TotalTokens,
			"cost":             msgUsage.TotalCost,
			"cumulativeTokens": cumulativeTokens,
			"cumulativeCost":   cumulativeCost,
			// Backward compatibility for older consumers.
			"inputTokens":  msgUsage.Input,
			"outputTokens": msgUsage.Output,
		})
	}

	if points == nil {
		return []map[string]interface{}{}
	}
	// 限制 200 点
	if len(points) > 200 {
		points = points[len(points)-200:]
	}
	return points
}

// ---------- sessions.usage.logs ----------

func handleSessionsUsageLogs(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if strings.TrimSpace(key) == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key is required for logs"))
		return
	}

	limit := 200
	if v, ok := ctx.Params["limit"].(float64); ok && !math.IsNaN(v) {
		l := int(v)
		if l > 0 && l < 1000 {
			limit = l
		}
	}

	cfg := resolveConfigFromContext(ctx)
	var storeEntries map[string]*SessionEntry
	if ctx.Context.SessionStore != nil {
		storeEntries = ctx.Context.SessionStore.LoadCombinedStore(scope.ResolveDefaultAgentId(cfg))
	}
	_, sessionFile, _, _ := resolveUsageSessionTarget(key, storeEntries, ctx.Context.StorePath, cfg)

	logs := loadLogsFromFile(sessionFile, limit)
	ctx.Respond(true, map[string]interface{}{
		"logs": logs,
	}, nil)
}

// loadLogsFromFile 从 JSONL 文件加载日志条目。
func loadLogsFromFile(sessionFile string, limit int) []map[string]interface{} {
	if strings.TrimSpace(sessionFile) == "" {
		return []map[string]interface{}{}
	}
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return []map[string]interface{}{}
	}

	lines := strings.Split(string(data), "\n")
	var logs []map[string]interface{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		role, _ := entry["role"].(string)
		if role == "" {
			continue
		}
		content := flattenTranscriptContent(entry)
		msgUsage, hasActualUsage := parseUsageFromEntry(entry)
		if hasActualUsage {
			model, _ := entry["model"].(string)
			_ = calculateUsageCost(model, &msgUsage)
		} else if estimated, ok := estimateUsageFromEntry(entry, role); ok {
			msgUsage = estimated
		}
		uiRole := role
		switch role {
		case "tool_result":
			uiRole = "toolResult"
		}
		logEntry := map[string]interface{}{
			"role":    uiRole,
			"content": content,
		}
		if ts, ok := entry["timestamp"].(float64); ok {
			logEntry["timestamp"] = int64(ts)
		} else if ts := readIntFromMap(entry, "timestampMs"); ts > 0 {
			logEntry["timestamp"] = int64(ts)
		}
		if msgUsage.TotalTokens > 0 {
			logEntry["tokens"] = msgUsage.TotalTokens
		}
		if msgUsage.TotalCost > 0 {
			logEntry["cost"] = msgUsage.TotalCost
		}
		if model, ok := entry["model"].(string); ok {
			logEntry["model"] = model
		}
		logs = append(logs, logEntry)
	}

	if logs == nil {
		return []map[string]interface{}{}
	}
	if len(logs) > limit {
		logs = logs[len(logs)-limit:]
	}
	return logs
}

// ---------- 辅助函数 ----------

func mergeTotalsInto(dst, src *usageTotals) {
	dst.Input += src.Input
	dst.Output += src.Output
	dst.CacheRead += src.CacheRead
	dst.CacheWrite += src.CacheWrite
	dst.TotalTokens += src.TotalTokens
	dst.TotalCost += src.TotalCost
	dst.InputCost += src.InputCost
	dst.OutputCost += src.OutputCost
	dst.CacheReadCost += src.CacheReadCost
	dst.CacheWriteCost += src.CacheWriteCost
	dst.MissingCostEntries += src.MissingCostEntries
}

func init() {
	// 静默未使用的 slog import（后续会用于 discovery 错误日志）
	_ = slog.Info
}
