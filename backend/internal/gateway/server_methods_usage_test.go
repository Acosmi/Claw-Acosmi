package gateway

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentsession "github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

type usageHandlersFixture struct {
	registry              *MethodRegistry
	context               *GatewayMethodContext
	sessionKey            string
	sessionID             string
	startDate             string
	endDate               string
	estimatedUserTokens   int
	expectedActualTokens  int
	expectedToolCallTotal int
}

func TestUsageHandlers_SessionsUsage(t *testing.T) {
	fixture := newUsageHandlersFixture(t)

	ok, payload, err := fixture.call("sessions.usage", map[string]interface{}{
		"startDate":            fixture.startDate,
		"endDate":              fixture.endDate,
		"includeContextWeight": true,
	})
	if !ok {
		t.Fatalf("sessions.usage should succeed: %+v", err)
	}

	result, ok := payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", payload)
	}
	if result["startDate"] != fixture.startDate {
		t.Fatalf("expected startDate=%s, got %v", fixture.startDate, result["startDate"])
	}
	if result["endDate"] != fixture.endDate {
		t.Fatalf("expected endDate=%s, got %v", fixture.endDate, result["endDate"])
	}

	sessions, ok := result["sessions"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected sessions array, got %T", result["sessions"])
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	sessionEntry := sessions[0]
	if sessionEntry["key"] != fixture.sessionKey {
		t.Fatalf("expected session key=%s, got %v", fixture.sessionKey, sessionEntry["key"])
	}
	if sessionEntry["modelProvider"] != "openai" {
		t.Fatalf("expected modelProvider=openai, got %v", sessionEntry["modelProvider"])
	}
	if sessionEntry["model"] != "gpt-5.1" {
		t.Fatalf("expected model=gpt-5.1, got %v", sessionEntry["model"])
	}

	usage, ok := sessionEntry["usage"].(*sessionCostSummary)
	if !ok {
		t.Fatalf("expected usage summary, got %T", sessionEntry["usage"])
	}
	expectedTotalTokens := fixture.expectedActualTokens + fixture.estimatedUserTokens
	if usage.TotalTokens != expectedTotalTokens {
		t.Fatalf("expected totalTokens=%d, got %d", expectedTotalTokens, usage.TotalTokens)
	}
	if usage.CacheRead != 20 {
		t.Fatalf("expected cacheRead=20, got %d", usage.CacheRead)
	}
	if usage.TotalCost <= 0 {
		t.Fatalf("expected actual totalCost > 0, got %f", usage.TotalCost)
	}
	if usage.UsageSource != "mixed" {
		t.Fatalf("expected usageSource=mixed, got %q", usage.UsageSource)
	}
	if usage.ToolUsage == nil {
		t.Fatal("expected tool usage summary")
	}
	if usage.ToolUsage.TotalCalls != fixture.expectedToolCallTotal {
		t.Fatalf("expected tool call total=%d, got %d", fixture.expectedToolCallTotal, usage.ToolUsage.TotalCalls)
	}
	if usage.ToolUsage.UniqueTools != 2 {
		t.Fatalf("expected unique tools=2, got %d", usage.ToolUsage.UniqueTools)
	}
	if usage.Latency == nil || usage.Latency.Count != 1 {
		t.Fatalf("expected latency summary with one sample, got %+v", usage.Latency)
	}
	if usage.Latency.AvgMs != 45000 {
		t.Fatalf("expected avg latency=45000ms, got %f", usage.Latency.AvgMs)
	}

	contextWeight, ok := sessionEntry["contextWeight"].(*usageContextWeight)
	if !ok || contextWeight == nil {
		t.Fatalf("expected contextWeight payload, got %T", sessionEntry["contextWeight"])
	}
	if contextWeight.SystemPrompt.Chars <= 0 {
		t.Fatal("expected system prompt chars > 0")
	}
	if len(contextWeight.InjectedWorkspaceFiles) == 0 {
		t.Fatal("expected injected workspace files")
	}
	if contextWeight.InjectedWorkspaceFiles[0].Name != "SOUL.md" {
		t.Fatalf("expected first injected file SOUL.md, got %q", contextWeight.InjectedWorkspaceFiles[0].Name)
	}

	totals, ok := result["totals"].(*usageTotals)
	if !ok {
		t.Fatalf("expected totals payload, got %T", result["totals"])
	}
	if totals.TotalTokens != expectedTotalTokens {
		t.Fatalf("expected aggregate totalTokens=%d, got %d", expectedTotalTokens, totals.TotalTokens)
	}

	aggregates, ok := result["aggregates"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected aggregates map, got %T", result["aggregates"])
	}
	toolAgg, ok := aggregates["tools"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected aggregate tools map, got %T", aggregates["tools"])
	}
	if toolAgg["totalCalls"] != fixture.expectedToolCallTotal {
		t.Fatalf("expected aggregate tool totalCalls=%d, got %v", fixture.expectedToolCallTotal, toolAgg["totalCalls"])
	}
	tools, ok := toolAgg["tools"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected aggregate tools array, got %T", toolAgg["tools"])
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 aggregate tools, got %d", len(tools))
	}
	if tools[0]["name"] != "search_query" || tools[0]["count"] != 2 {
		t.Fatalf("expected top tool search_query x2, got %+v", tools[0])
	}
	byProvider, ok := aggregates["byProvider"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected byProvider array, got %T", aggregates["byProvider"])
	}
	if len(byProvider) != 1 || byProvider[0]["provider"] != "openai" || byProvider[0]["count"] != 1 {
		t.Fatalf("expected openai provider aggregate, got %+v", byProvider)
	}
	if latency, ok := aggregates["latency"].(*latencySummary); !ok || latency == nil || latency.Count != 1 {
		t.Fatalf("expected aggregate latency summary, got %T %+v", aggregates["latency"], aggregates["latency"])
	}
}

func TestUsageHandlers_SessionsUsageDefaultDates(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(UsageHandlers())

	req := &RequestFrame{Method: "sessions.usage", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}

	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{Config: &types.OpenAcosmiConfig{}}, respond)
	if !gotOK {
		t.Fatal("sessions.usage should succeed with default dates")
	}
	result := gotPayload.(map[string]interface{})
	today := time.Now().UTC().Format("2006-01-02")
	if result["endDate"] != today {
		t.Fatalf("expected endDate=%s, got %v", today, result["endDate"])
	}
}

func TestUsageHandlers_Timeseries(t *testing.T) {
	fixture := newUsageHandlersFixture(t)

	ok, payload, err := fixture.call("sessions.usage.timeseries", map[string]interface{}{
		"key": fixture.sessionKey,
	})
	if !ok {
		t.Fatalf("sessions.usage.timeseries should succeed: %+v", err)
	}

	result := payload.(map[string]interface{})
	points, ok := result["points"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected points array, got %T", result["points"])
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 timeseries points, got %d", len(points))
	}

	userPoint := points[0]
	if userPoint["input"] != fixture.estimatedUserTokens {
		t.Fatalf("expected estimated user input=%d, got %v", fixture.estimatedUserTokens, userPoint["input"])
	}
	if userPoint["cost"] != 0.0 {
		t.Fatalf("expected estimated point cost=0, got %v", userPoint["cost"])
	}

	assistantPoint := points[1]
	if assistantPoint["cacheRead"] != 20 {
		t.Fatalf("expected cacheRead=20, got %v", assistantPoint["cacheRead"])
	}
	if assistantPoint["totalTokens"] != fixture.expectedActualTokens {
		t.Fatalf("expected assistant totalTokens=%d, got %v", fixture.expectedActualTokens, assistantPoint["totalTokens"])
	}
	if assistantPoint["cumulativeTokens"] != fixture.expectedActualTokens+fixture.estimatedUserTokens {
		t.Fatalf("unexpected cumulativeTokens=%v", assistantPoint["cumulativeTokens"])
	}
	if cost, ok := assistantPoint["cost"].(float64); !ok || cost <= 0 {
		t.Fatalf("expected assistant cost > 0, got %v", assistantPoint["cost"])
	}
}

func TestUsageHandlers_TimeseriesMissingKey(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(UsageHandlers())

	req := &RequestFrame{Method: "sessions.usage.timeseries", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}

	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Fatal("should fail without key")
	}
	if gotErr == nil || !strings.Contains(gotErr.Message, "key is required") {
		t.Fatalf("expected key required error, got %+v", gotErr)
	}
}

func TestUsageHandlers_Logs(t *testing.T) {
	fixture := newUsageHandlersFixture(t)

	ok, payload, err := fixture.call("sessions.usage.logs", map[string]interface{}{
		"key":   fixture.sessionKey,
		"limit": float64(50),
	})
	if !ok {
		t.Fatalf("sessions.usage.logs should succeed: %+v", err)
	}

	result := payload.(map[string]interface{})
	logs, ok := result["logs"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected logs array, got %T", result["logs"])
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0]["role"] != "user" || logs[0]["content"] == "" {
		t.Fatalf("expected first log to be user with content, got %+v", logs[0])
	}
	if logs[1]["role"] != "assistant" {
		t.Fatalf("expected second log assistant, got %+v", logs[1])
	}
	if logs[1]["model"] != "gpt-5.1" {
		t.Fatalf("expected assistant model=gpt-5.1, got %+v", logs[1])
	}
	if logs[1]["tokens"] != fixture.expectedActualTokens {
		t.Fatalf("expected assistant tokens=%d, got %v", fixture.expectedActualTokens, logs[1]["tokens"])
	}
	if cost, ok := logs[1]["cost"].(float64); !ok || cost <= 0 {
		t.Fatalf("expected assistant cost > 0, got %v", logs[1]["cost"])
	}
}

func TestUsageHandlers_UsageCost(t *testing.T) {
	fixture := newUsageHandlersFixture(t)

	ok, payload, err := fixture.call("usage.cost", map[string]interface{}{
		"startDate": fixture.startDate,
		"endDate":   fixture.endDate,
	})
	if !ok {
		t.Fatalf("usage.cost should succeed: %+v", err)
	}

	summary, ok := payload.(*usageCostSummary)
	if !ok {
		t.Fatalf("expected usageCostSummary payload, got %T", payload)
	}
	if summary.Days != 1 {
		t.Fatalf("expected 1 daily bucket, got %d", summary.Days)
	}
	if len(summary.Daily) != 1 {
		t.Fatalf("expected 1 daily row, got %d", len(summary.Daily))
	}
	expectedTotalTokens := fixture.expectedActualTokens + fixture.estimatedUserTokens
	if summary.Totals == nil || summary.Totals.TotalTokens != expectedTotalTokens {
		t.Fatalf("expected totals totalTokens=%d, got %+v", expectedTotalTokens, summary.Totals)
	}
	if summary.Daily[0].TotalTokens != expectedTotalTokens {
		t.Fatalf("expected daily totalTokens=%d, got %d", expectedTotalTokens, summary.Daily[0].TotalTokens)
	}
	if summary.Daily[0].TotalCost <= 0 {
		t.Fatalf("expected daily totalCost > 0, got %f", summary.Daily[0].TotalCost)
	}
}

func TestLoadSessionCostFromFile_EmptyToolUsageSerializesAsArray(t *testing.T) {
	t.Helper()

	transcriptPath := filepath.Join(t.TempDir(), "session-empty-tools.jsonl")
	baseTs := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC).UnixMilli()
	writeUsageTranscriptFixture(
		t,
		transcriptPath,
		"session-empty-tools",
		map[string]interface{}{
			"role": "user",
			"content": []map[string]interface{}{
				{"type": "text", "text": "统计一下这个会话。"},
			},
			"timestamp": baseTs,
		},
		map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "这里没有工具调用。"},
			},
			"provider": "qwen",
			"model":    "qwen3.5-plus",
			"usage": map[string]interface{}{
				"input_tokens":  32,
				"output_tokens": 12,
			},
			"timestamp": baseTs + 1000,
		},
	)

	summary := loadSessionCostFromFile(transcriptPath, nil)
	if summary == nil || summary.ToolUsage == nil {
		t.Fatalf("expected tool usage summary, got %+v", summary)
	}
	if summary.ToolUsage.TotalCalls != 0 {
		t.Fatalf("expected zero tool calls, got %d", summary.ToolUsage.TotalCalls)
	}
	if summary.ToolUsage.Tools == nil {
		t.Fatal("expected empty tools slice, got nil")
	}

	encoded, err := json.Marshal(summary.ToolUsage)
	if err != nil {
		t.Fatalf("marshal tool usage: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"tools":[]`)) {
		t.Fatalf("expected tools to serialize as empty array, got %s", string(encoded))
	}
}

func newUsageHandlersFixture(t *testing.T) *usageHandlersFixture {
	t.Helper()
	resetUsageCostCacheForTest()

	rootDir := t.TempDir()
	storeDir := filepath.Join(rootDir, "store")
	workspaceDir := filepath.Join(rootDir, "workspace")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("mkdir store dir: %v", err)
	}
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "SOUL.md"), []byte("usage audit context"), 0o600); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}

	sessionID := "usage-session-1"
	sessionKey := "agent:main:main"
	transcriptPath := filepath.Join(rootDir, sessionID+".jsonl")
	userText := "请帮我审计今天的模型用量"
	userTs := time.Date(2025, time.January, 15, 10, 0, 0, 0, time.UTC).UnixMilli()
	assistantTs := userTs + 45_000

	writeUsageTranscriptFixture(t, transcriptPath, sessionID,
		map[string]interface{}{
			"role": "user",
			"content": []map[string]interface{}{
				{"type": "text", "text": userText},
			},
			"timestamp": userTs,
		},
		map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "已统计完成。"},
			},
			"provider": "openai",
			"model":    "gpt-5.1",
			"usage": map[string]interface{}{
				"input_tokens":  120,
				"output_tokens": 30,
				"cache_read":    20,
			},
			"toolCalls": map[string]int{
				"search_query": 2,
				"exec_command": 1,
			},
			"timestamp": assistantTs,
		},
	)
	if err := os.Chtimes(transcriptPath, time.UnixMilli(assistantTs), time.UnixMilli(assistantTs)); err != nil {
		t.Fatalf("chtimes transcript fixture: %v", err)
	}

	store := NewSessionStore(storeDir)
	store.Save(&SessionEntry{
		SessionKey:       sessionKey,
		SessionId:        sessionID,
		SessionFile:      transcriptPath,
		Label:            "Usage Fixture",
		UpdatedAt:        assistantTs,
		Channel:          "feishu",
		ChatType:         "group",
		ModelOverride:    "gpt-5.1",
		ProviderOverride: "openai",
		SkillsSnapshot: &SessionSkillSnapshot{
			Skills: []SessionSkillSnapshotItem{{Name: "usage-audit"}},
		},
	})

	defaultAgent := true
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{
					ID:        "main",
					Default:   &defaultAgent,
					Workspace: workspaceDir,
				},
			},
		},
	}

	registry := NewMethodRegistry()
	registry.RegisterAll(UsageHandlers())

	return &usageHandlersFixture{
		registry:              registry,
		context:               &GatewayMethodContext{SessionStore: store, StorePath: storeDir, Config: cfg},
		sessionKey:            sessionKey,
		sessionID:             sessionID,
		startDate:             "2025-01-01",
		endDate:               "2025-01-31",
		estimatedUserTokens:   agentsession.EstimatePromptTokens(userText),
		expectedActualTokens:  170,
		expectedToolCallTotal: 3,
	}
}

func (f *usageHandlersFixture) call(method string, params map[string]interface{}) (bool, interface{}, *ErrorShape) {
	req := &RequestFrame{Method: method, Params: params}
	var gotOK bool
	var gotPayload interface{}
	var gotErr *ErrorShape
	respond := func(ok bool, payload interface{}, err *ErrorShape) {
		gotOK = ok
		gotPayload = payload
		gotErr = err
	}
	HandleGatewayRequest(f.registry, req, nil, f.context, respond)
	return gotOK, gotPayload, gotErr
}

func writeUsageTranscriptFixture(t *testing.T, path, sessionID string, entries ...map[string]interface{}) {
	t.Helper()

	lines := make([][]byte, 0, len(entries)+1)
	header, err := json.Marshal(map[string]interface{}{
		"type":      "session",
		"version":   agentsession.CurrentTranscriptVersion,
		"id":        sessionID,
		"timestamp": time.UnixMilli(time.Now().UnixMilli()).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("marshal transcript header: %v", err)
	}
	lines = append(lines, header)

	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal transcript entry: %v", err)
		}
		lines = append(lines, line)
	}

	if err := os.WriteFile(path, append(bytes.Join(lines, []byte("\n")), '\n'), 0o600); err != nil {
		t.Fatalf("write transcript fixture: %v", err)
	}
}

func resetUsageCostCacheForTest() {
	costCacheMu.Lock()
	defer costCacheMu.Unlock()
	costCache = map[string]*costUsageCacheEntry{}
}
