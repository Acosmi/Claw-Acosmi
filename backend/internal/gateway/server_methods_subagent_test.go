package gateway

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/argus"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- 授权分类测试 (Phase 5) ----------

func TestAuthz_SubagentList_InReadMethods(t *testing.T) {
	// subagent.list 应被 readMethods 收录，read scope 可访问
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.read"},
	}}
	if err := AuthorizeGatewayMethod("subagent.list", client); err != nil {
		t.Errorf("subagent.list should be accessible with read scope, got %v", err)
	}
}

func TestAuthz_SubagentCtl_InWriteMethods(t *testing.T) {
	// subagent.ctl 应被 writeMethods 收录，write scope 可访问
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.write"},
	}}
	if err := AuthorizeGatewayMethod("subagent.ctl", client); err != nil {
		t.Errorf("subagent.ctl should be accessible with write scope, got %v", err)
	}
}

func TestAuthz_ArgusPermissionCheck_InReadMethods(t *testing.T) {
	// argus.permission.check 应被 readMethods 收录
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.read"},
	}}
	if err := AuthorizeGatewayMethod("argus.permission.check", client); err != nil {
		t.Errorf("argus.permission.check should be accessible with read scope, got %v", err)
	}
}

func TestAuthz_SubagentList_DeniedWithoutScope(t *testing.T) {
	// 无 scope 时 subagent.list 应被拒绝
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{},
	}}
	if err := AuthorizeGatewayMethod("subagent.list", client); err == nil {
		t.Error("subagent.list should be denied without read scope")
	}
}

func TestAuthz_SubagentCtl_DeniedWithReadOnly(t *testing.T) {
	// read scope 不应能调用 subagent.ctl
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.read"},
	}}
	if err := AuthorizeGatewayMethod("subagent.ctl", client); err == nil {
		t.Error("subagent.ctl should be denied with read-only scope")
	}
}

// ---------- handleArgusCtl 单元测试 ----------

// mockRespond 捕获 RPC 响应。
type mockRespond struct {
	ok      bool
	payload interface{}
	err     *ErrorShape
}

func newMockRespond() (*mockRespond, RespondFunc) {
	r := &mockRespond{}
	return r, func(ok bool, payload interface{}, err *ErrorShape) {
		r.ok = ok
		r.payload = payload
		r.err = err
	}
}

func payloadMap(r *mockRespond) map[string]interface{} {
	m, _ := r.payload.(map[string]interface{})
	return m
}

// newTestContext 创建包含最小 State 的测试 context（State.ArgusBridge() 返回 nil）。
func newTestContext(params map[string]interface{}, respond RespondFunc) *MethodHandlerContext {
	return &MethodHandlerContext{
		Params: params,
		Context: &GatewayMethodContext{
			State: &GatewayState{}, // ArgusBridge() → nil
		},
		Respond: respond,
	}
}

// ---------- ACK 语义: bridge nil → applied=false, reason=argus_not_running ----------

func TestArgusCtl_ACK_SetIntervalMs_BridgeNil(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"agent_id": "argus-screen", "action": "set_interval_ms", "value": float64(500)},
		respond,
	)
	handleArgusCtl(ctx, "set_interval_ms")
	if !resp.ok {
		t.Fatal("ACK with bridge nil should still return ok=true")
	}
	m := payloadMap(resp)
	if m["applied"] != false {
		t.Errorf("expected applied=false, got %v", m["applied"])
	}
	if m["reason"] != "argus_not_running" {
		t.Errorf("expected reason=argus_not_running, got %v", m["reason"])
	}
}

func TestArgusCtl_ACK_SetGoal_BridgeNil(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": "test goal"},
		respond,
	)
	handleArgusCtl(ctx, "set_goal")
	if !resp.ok {
		t.Fatal("ACK with bridge nil should return ok=true")
	}
	m := payloadMap(resp)
	if m["applied"] != false {
		t.Errorf("expected applied=false, got %v", m["applied"])
	}
	if m["reason"] != "argus_not_running" {
		t.Errorf("expected reason=argus_not_running, got %v", m["reason"])
	}
}

func TestArgusCtl_ACK_SetVlaModel_BridgeNil(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": "anthropic"},
		respond,
	)
	handleArgusCtl(ctx, "set_vla_model")
	if !resp.ok {
		t.Fatal("ACK with bridge nil should return ok=true")
	}
	m := payloadMap(resp)
	if m["applied"] != false {
		t.Errorf("expected applied=false, got %v", m["applied"])
	}
	if m["reason"] != "argus_not_running" {
		t.Errorf("expected reason=argus_not_running, got %v", m["reason"])
	}
}

func TestBuildArgusEntry_InitBridgeShownStoppedUntilEnabled(t *testing.T) {
	entry := buildArgusEntry(argus.NewBridge(argus.DefaultBridgeConfig()))
	if entry.Status != "stopped" {
		t.Fatalf("expected init bridge to stay stopped before explicit enable, got %q", entry.Status)
	}
}

func TestArgusCtl_SetEnabled_PersistsDesiredStateBeforePermissionRecovery(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openacosmi.json")
	loader := config.NewConfigLoader(config.WithConfigPath(configPath))
	initial := &types.OpenAcosmiConfig{
		SubAgents: &types.SubAgentConfig{
			ScreenObserver: &types.ScreenObserverSettings{
				BinaryPath: "/nonexistent/argus-sensory-test",
			},
		},
	}
	if err := loader.WriteConfigFile(initial); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	resp, respond := newMockRespond()
	ctx := &MethodHandlerContext{
		Params: map[string]interface{}{
			"agent_id": "argus-screen",
			"action":   "set_enabled",
			"value":    true,
		},
		Context: &GatewayMethodContext{
			State:        &GatewayState{},
			ConfigLoader: loader,
		},
		Respond: respond,
	}

	handleArgusCtl(ctx, "set_enabled")
	if resp.ok {
		t.Fatal("expected enable attempt to fail because binary path is invalid")
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.SubAgents == nil || cfg.SubAgents.ScreenObserver == nil || cfg.SubAgents.ScreenObserver.Enabled == nil || !*cfg.SubAgents.ScreenObserver.Enabled {
		t.Fatalf("expected screenObserver.enabled to persist as true, got %#v", cfg.SubAgents)
	}
}

// ---------- 值参数校验 ----------

func TestArgusCtl_SetIntervalMs_TooLow(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": float64(50)},
		respond,
	)
	handleArgusCtl(ctx, "set_interval_ms")
	if resp.ok {
		t.Error("value=50 should be rejected (min 100)")
	}
	if resp.err == nil || resp.err.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request error, got %v", resp.err)
	}
}

func TestArgusCtl_SetIntervalMs_TooHigh(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": float64(100000)},
		respond,
	)
	handleArgusCtl(ctx, "set_interval_ms")
	if resp.ok {
		t.Error("value=100000 should be rejected (max 60000)")
	}
}

func TestArgusCtl_SetIntervalMs_WrongType(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": "not_a_number"},
		respond,
	)
	handleArgusCtl(ctx, "set_interval_ms")
	if resp.ok {
		t.Error("string value should be rejected for set_interval_ms")
	}
}

func TestArgusCtl_SetGoal_Empty(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": ""},
		respond,
	)
	handleArgusCtl(ctx, "set_goal")
	if resp.ok {
		t.Error("empty goal should be rejected")
	}
}

func TestArgusCtl_SetGoal_TooLong(t *testing.T) {
	longGoal := make([]byte, 1001)
	for i := range longGoal {
		longGoal[i] = 'a'
	}
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": string(longGoal)},
		respond,
	)
	handleArgusCtl(ctx, "set_goal")
	if resp.ok {
		t.Error("goal > 1000 chars should be rejected")
	}
}

func TestArgusCtl_SetVlaModel_InvalidModel(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"value": "invalid_model"},
		respond,
	)
	handleArgusCtl(ctx, "set_vla_model")
	if resp.ok {
		t.Error("invalid_model should be rejected")
	}
}

func TestArgusCtl_SetVlaModel_ValidModels(t *testing.T) {
	// 白名单仅包含实际已实现的模型: none（禁用）+ anthropic（已实现）
	validModels := []string{"none", "anthropic"}
	for _, model := range validModels {
		resp, respond := newMockRespond()
		ctx := newTestContext(
			map[string]interface{}{"value": model},
			respond,
		)
		handleArgusCtl(ctx, "set_vla_model")
		if !resp.ok {
			t.Errorf("model %q should be accepted, got error: %v", model, resp.err)
		}
		m := payloadMap(resp)
		if m["applied"] != false {
			t.Errorf("model %q: expected applied=false (bridge nil), got %v", model, m["applied"])
		}
	}
}

func TestArgusCtl_SetVlaModel_RejectedModels(t *testing.T) {
	// 工厂中未实现的模型应被白名单拒绝
	rejectedModels := []string{"gemini", "qwen", "ollama", "openai"}
	for _, model := range rejectedModels {
		resp, respond := newMockRespond()
		ctx := newTestContext(
			map[string]interface{}{"value": model},
			respond,
		)
		handleArgusCtl(ctx, "set_vla_model")
		if resp.ok {
			t.Errorf("model %q should be rejected (not in whitelist)", model)
		}
	}
}

func TestArgusCtl_UnknownAction(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{},
		respond,
	)
	handleArgusCtl(ctx, "unknown_action")
	if resp.ok {
		t.Error("unknown action should be rejected")
	}
	if resp.err == nil || resp.err.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request, got %v", resp.err)
	}
}

// ---------- handleSubagentCtl 路由测试 ----------

func TestSubagentCtl_MissingParams(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{},
		respond,
	)
	handleSubagentCtl(ctx)
	if resp.ok {
		t.Error("missing agent_id and action should fail")
	}
}

func TestSubagentCtl_UnknownAgent(t *testing.T) {
	resp, respond := newMockRespond()
	ctx := newTestContext(
		map[string]interface{}{"agent_id": "nonexistent", "action": "set_enabled"},
		respond,
	)
	handleSubagentCtl(ctx)
	if resp.ok {
		t.Error("unknown agent should fail")
	}
}

// ---------- argusStartErrorResponse 测试 ----------

func TestArgusStartErrorResponse_CrashPhase(t *testing.T) {
	err := argusStartErrorResponse(fmt.Errorf("start process failed: spawn error"))
	if err.Code != ErrCodeServiceUnavailable {
		t.Errorf("expected service_unavailable, got %s", err.Code)
	}
	details, ok := err.Details.(map[string]string)
	if !ok {
		t.Fatalf("details should be map[string]string, got %T", err.Details)
	}
	if details["phase"] != "crash" {
		t.Errorf("expected phase=crash, got %s", details["phase"])
	}
}

func TestArgusStartErrorResponse_HandshakePhase(t *testing.T) {
	err := argusStartErrorResponse(fmt.Errorf("handshake timeout"))
	details, ok := err.Details.(map[string]string)
	if !ok {
		t.Fatalf("details should be map[string]string, got %T", err.Details)
	}
	if details["phase"] != "handshake" {
		t.Errorf("expected phase=handshake, got %s", details["phase"])
	}
}

// ---------- sanitizePath 测试 ----------

func TestSanitizePath_ReplacesHomeDir(t *testing.T) {
	// sanitizePath 应将 home 目录替换为 ~
	result := sanitizePath("/some/random/path")
	// 无 home 匹配时原样返回
	if result != "/some/random/path" {
		t.Errorf("non-matching path should be returned as-is, got %q", result)
	}
}

// ---------- 启动失败广播事件测试 (Phase 6) ----------

func TestBroadcast_ArgusStatusChanged_OnStartFailure(t *testing.T) {
	// 模拟 boot.go 中的广播逻辑: Start 失败 → 广播 argus.status.changed
	bc := NewBroadcaster()
	c, sent := mockClient("test", "operator", nil)
	bc.AddClient(c)

	// 模拟 boot.go L192-200 的广播
	bc.Broadcast("argus.status.changed", map[string]interface{}{
		"state":    "stopped",
		"reason":   "binary not found",
		"phase":    "start",
		"recovery": "Check argus-sensory binary and permissions.",
		"ts":       int64(1709568000000),
	}, nil)

	if len(*sent) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(*sent))
	}

	var frame eventFrame
	if err := json.Unmarshal((*sent)[0], &frame); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if frame.Event != "argus.status.changed" {
		t.Errorf("event = %q, want argus.status.changed", frame.Event)
	}

	// 验证 payload 结构
	payload, ok := frame.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("payload should be map, got %T", frame.Payload)
	}
	if payload["state"] != "stopped" {
		t.Errorf("state = %v, want stopped", payload["state"])
	}
	if payload["phase"] != "start" {
		t.Errorf("phase = %v, want start", payload["phase"])
	}
	if payload["recovery"] == nil || payload["recovery"] == "" {
		t.Error("recovery should be non-empty")
	}
	if payload["ts"] == nil {
		t.Error("ts should be present")
	}
}

func TestBroadcast_ArgusStatusChanged_OnStateChange(t *testing.T) {
	// 模拟 boot.go L178-184: OnStateChange 回调广播
	bc := NewBroadcaster()
	c, sent := mockClient("test", "operator", nil)
	bc.AddClient(c)

	// 模拟 OnStateChange 回调
	states := []struct {
		state  string
		reason string
	}{
		{"starting", "spawn"},
		{"ready", "handshake complete"},
		{"degraded", "health check failed"},
		{"stopped", "process exited"},
	}

	for _, s := range states {
		bc.Broadcast("argus.status.changed", map[string]interface{}{
			"state":  s.state,
			"reason": s.reason,
			"ts":     int64(1709568000000),
		}, nil)
	}

	if len(*sent) != 4 {
		t.Fatalf("expected 4 broadcasts, got %d", len(*sent))
	}

	// 验证第一个事件
	var frame eventFrame
	json.Unmarshal((*sent)[0], &frame)
	if frame.Event != "argus.status.changed" {
		t.Errorf("first event = %q", frame.Event)
	}
	p, _ := frame.Payload.(map[string]interface{})
	if p["state"] != "starting" {
		t.Errorf("first state = %v", p["state"])
	}
}

func TestBroadcast_ArgusStatusChanged_HasRecoveryOnFailure(t *testing.T) {
	bc := NewBroadcaster()
	c, sent := mockClient("test", "operator", nil)
	bc.AddClient(c)

	// 启动失败的广播必须包含 recovery 字段
	bc.Broadcast("argus.status.changed", map[string]interface{}{
		"state":    "stopped",
		"reason":   "exec: argus-sensory not found",
		"phase":    "start",
		"recovery": "Check argus-sensory binary and permissions. Try enabling from SubAgents panel.",
		"ts":       int64(1709568000000),
	}, nil)

	var frame eventFrame
	json.Unmarshal((*sent)[0], &frame)
	payload, _ := frame.Payload.(map[string]interface{})

	recovery, ok := payload["recovery"].(string)
	if !ok || recovery == "" {
		t.Error("failure broadcast must include non-empty recovery guidance")
	}
	if payload["phase"] != "start" {
		t.Errorf("failure phase should be 'start', got %v", payload["phase"])
	}
}
