package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/gatewayclient"
)

func TestGatewayToolParametersExposeConfigWorkflow(t *testing.T) {
	tool := CreateGatewayTool(GatewayOptions{URL: "ws://127.0.0.1:26222"})

	params, ok := tool.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected map parameters, got %T", tool.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", params["properties"])
	}

	actionProp, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatalf("expected action property map, got %T", props["action"])
	}
	enumVals, ok := actionProp["enum"].([]any)
	if !ok {
		t.Fatalf("expected enum slice, got %T", actionProp["enum"])
	}

	actions := make(map[string]bool, len(enumVals))
	for _, raw := range enumVals {
		actions[raw.(string)] = true
	}
	expectedActions := make(map[string]bool, len(gatewayclient.GatewayToolActions))
	for _, want := range gatewayclient.GatewayToolActions {
		expectedActions[want] = true
	}
	if len(actions) != len(expectedActions) {
		t.Fatalf("gateway action count=%d, want %d", len(actions), len(expectedActions))
	}
	for want := range expectedActions {
		if !actions[want] {
			t.Fatalf("gateway schema missing action %q", want)
		}
	}

	if _, ok := props["raw"]; !ok {
		t.Fatal("gateway schema should expose raw")
	}
	if _, ok := props["baseHash"]; !ok {
		t.Fatal("gateway schema should expose baseHash")
	}
	for _, key := range []string{"enabled", "provider", "feishu", "wechat", "delayMs"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("gateway schema should expose %q", key)
		}
	}
	if _, ok := props["config"]; ok {
		t.Fatal("gateway schema should not expose legacy config object")
	}
	if _, ok := props["path"]; ok {
		t.Fatal("gateway schema should not expose unsupported config.get path")
	}
}

func TestGatewayToolExecutesConfigPatchWithRawAndBaseHash(t *testing.T) {
	var gotMethod string
	var gotParams interface{}

	tool := CreateGatewayTool(GatewayOptions{
		URL: "ws://127.0.0.1:26222",
		Caller: func(_ context.Context, _ GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
			gotMethod = method
			gotParams = params
			return map[string]interface{}{"ok": true, "hash": "next-hash"}, nil
		},
	})

	result, err := tool.Execute(context.Background(), "call-1", map[string]any{
		"action":         "config.patch",
		"raw":            `{gateway:{port:26223}}`,
		"baseHash":       "hash-123",
		"sessionKey":     "session-1",
		"note":           "switch port",
		"restartDelayMs": 250,
	})
	if err != nil {
		t.Fatalf("execute gateway tool: %v", err)
	}
	if gotMethod != "config.patch" {
		t.Fatalf("method=%q, want config.patch", gotMethod)
	}

	params, ok := gotParams.(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map, got %T", gotParams)
	}
	if params["raw"] != `{gateway:{port:26223}}` {
		t.Fatalf("raw=%v", params["raw"])
	}
	if params["baseHash"] != "hash-123" {
		t.Fatalf("baseHash=%v", params["baseHash"])
	}
	if params["sessionKey"] != "session-1" {
		t.Fatalf("sessionKey=%v", params["sessionKey"])
	}
	if params["note"] != "switch port" {
		t.Fatalf("note=%v", params["note"])
	}
	if params["restartDelayMs"] != 250 {
		t.Fatalf("restartDelayMs=%v", params["restartDelayMs"])
	}

	details, ok := result.Details.(map[string]any)
	if !ok {
		t.Fatalf("expected result details map, got %T", result.Details)
	}
	if details["action"] != "config.patch" {
		t.Fatalf("action=%v", details["action"])
	}
	if details["status"] != "patched" {
		t.Fatalf("status=%v", details["status"])
	}
}

func TestGatewayToolExecutesBrowserSetWithBaseHash(t *testing.T) {
	var gotMethod string
	var gotParams interface{}

	tool := CreateGatewayTool(GatewayOptions{
		URL: "ws://127.0.0.1:26222",
		Caller: func(_ context.Context, _ GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
			gotMethod = method
			gotParams = params
			return map[string]interface{}{"ok": true}, nil
		},
	})

	result, err := tool.Execute(context.Background(), "call-browser", map[string]any{
		"action":   "tools.browser.set",
		"baseHash": "hash-456",
		"enabled":  true,
		"cdpUrl":   "ws://127.0.0.1:9222/devtools/browser/test",
	})
	if err != nil {
		t.Fatalf("execute gateway tool: %v", err)
	}
	if gotMethod != "tools.browser.set" {
		t.Fatalf("method=%q, want tools.browser.set", gotMethod)
	}

	params, ok := gotParams.(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map, got %T", gotParams)
	}
	if params["baseHash"] != "hash-456" {
		t.Fatalf("baseHash=%v", params["baseHash"])
	}
	if params["enabled"] != true {
		t.Fatalf("enabled=%v", params["enabled"])
	}
	if params["cdpUrl"] != "ws://127.0.0.1:9222/devtools/browser/test" {
		t.Fatalf("cdpUrl=%v", params["cdpUrl"])
	}

	details, ok := result.Details.(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %T", result.Details)
	}
	if details["action"] != "tools.browser.set" {
		t.Fatalf("action=%v", details["action"])
	}
	if details["status"] != "saved" {
		t.Fatalf("status=%v", details["status"])
	}
}

func TestGatewayToolExecutesConfigSchema(t *testing.T) {
	var gotMethod string

	tool := CreateGatewayTool(GatewayOptions{
		URL: "ws://127.0.0.1:26222",
		Caller: func(_ context.Context, _ GatewayOptions, method string, _ interface{}) (map[string]interface{}, error) {
			gotMethod = method
			return map[string]interface{}{"version": "1.0.0"}, nil
		},
	})

	result, err := tool.Execute(context.Background(), "call-2", map[string]any{
		"action": "config.schema",
	})
	if err != nil {
		t.Fatalf("execute gateway tool: %v", err)
	}
	if gotMethod != "config.schema" {
		t.Fatalf("method=%q, want config.schema", gotMethod)
	}
	details, ok := result.Details.(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %T", result.Details)
	}
	if details["version"] != "1.0.0" {
		t.Fatalf("version=%v", details["version"])
	}
}

func TestGatewayToolExecutesRestartWithSystemMethod(t *testing.T) {
	var gotMethod string
	var gotParams interface{}

	tool := CreateGatewayTool(GatewayOptions{
		URL: "ws://127.0.0.1:26222",
		Caller: func(_ context.Context, _ GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
			gotMethod = method
			gotParams = params
			return map[string]interface{}{"ok": true, "restart": map[string]any{"scheduled": true}}, nil
		},
	})

	result, err := tool.Execute(context.Background(), "call-restart", map[string]any{
		"action":  "restart",
		"reason":  "apply browser config",
		"delayMs": 500,
	})
	if err != nil {
		t.Fatalf("execute gateway tool: %v", err)
	}
	if gotMethod != "system.restart" {
		t.Fatalf("method=%q, want system.restart", gotMethod)
	}

	params, ok := gotParams.(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map, got %T", gotParams)
	}
	if params["reason"] != "apply browser config" {
		t.Fatalf("reason=%v", params["reason"])
	}
	if params["delayMs"] != 500 {
		t.Fatalf("delayMs=%v", params["delayMs"])
	}

	details, ok := result.Details.(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %T", result.Details)
	}
	if details["action"] != "restart" {
		t.Fatalf("action=%v", details["action"])
	}
	if details["status"] != "initiated" {
		t.Fatalf("status=%v", details["status"])
	}
}

func TestGatewayToolErrorIncludesStructuredDetails(t *testing.T) {
	tool := CreateGatewayTool(GatewayOptions{
		URL: "ws://127.0.0.1:26222",
		Caller: func(_ context.Context, _ GatewayOptions, _ string, _ interface{}) (map[string]interface{}, error) {
			return nil, &GatewayCallError{
				Method:  "config.apply",
				Code:    "config_invalid",
				Message: "invalid config",
				Details: map[string]any{"issueCount": 1, "summary": "gateway.port: invalid"},
			}
		},
	})

	_, err := tool.Execute(context.Background(), "call-3", map[string]any{
		"action":   "config.apply",
		"raw":      `{"gateway":{"port":-1}}`,
		"baseHash": "hash-123",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "config_invalid") {
		t.Fatalf("error should include code, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "\"issueCount\":1") {
		t.Fatalf("error should include details JSON, got %q", err.Error())
	}
}

func TestGatewaySkillDocsMatchExecutableContract(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../"))

	systemSkillPath := filepath.Join(repoRoot, "docs", "skills", "operations", "system-config", "SKILL.md")
	gatewaySkillPath := filepath.Join(repoRoot, "docs", "skills", "tools", "system", "gateway", "SKILL.md")

	systemSkill, err := os.ReadFile(systemSkillPath)
	if err != nil {
		t.Fatalf("read %s: %v", systemSkillPath, err)
	}
	gatewaySkill, err := os.ReadFile(gatewaySkillPath)
	if err != nil {
		t.Fatalf("read %s: %v", gatewaySkillPath, err)
	}

	systemContent := string(systemSkill)
	gatewayContent := string(gatewaySkill)

	for _, content := range []string{systemContent, gatewayContent} {
		for _, required := range []string{"config.get", "config.schema", "config.set", "config.patch", "config.apply", "baseHash", "raw"} {
			if !strings.Contains(content, required) {
				t.Fatalf("skill docs should mention %q, content=%q", required, content)
			}
		}
		if strings.Contains(content, "config.patch → config.apply") {
			t.Fatalf("skill docs should not describe patch-then-apply flow, content=%q", content)
		}
	}

	for _, required := range []string{
		"browser_config",
		"remote_approval_config",
		"image_config",
		"stt_config",
		"docconv_config",
		"media_config",
		"tools.browser.get",
		"tools.browser.set",
		"security.remoteApproval.config.get",
		"security.remoteApproval.config.set",
		"security.remoteApproval.test",
		"image.config.get",
		"image.config.set",
		"stt.config.get",
		"stt.config.set",
		"docconv.config.get",
		"docconv.config.set",
		"media.config.get",
		"media.config.update",
		"restart",
	} {
		if !strings.Contains(gatewayContent, required) {
			t.Fatalf("gateway skill docs should mention %q, content=%q", required, gatewayContent)
		}
	}

	for _, required := range []string{
		"browser_config",
		"remote_approval_config",
		"image_config",
		"stt_config",
		"docconv_config",
		"media_config",
		"tools.browser.get",
		"security.remoteApproval.config.get",
		"image.config.get",
		"stt.config.get",
		"docconv.config.get",
		"media.config.get",
	} {
		if !strings.Contains(systemContent, required) {
			t.Fatalf("system-config skill docs should mention %q, content=%q", required, systemContent)
		}
	}
}
