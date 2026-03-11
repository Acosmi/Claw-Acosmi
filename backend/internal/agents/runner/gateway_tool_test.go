package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/configtools"
	"github.com/Acosmi/ClawAcosmi/internal/agents/gatewayclient"
)

func TestExecuteToolCall_Gateway(t *testing.T) {
	var gotMethod string

	result, err := ExecuteToolCall(context.Background(), "gateway",
		json.RawMessage(`{"action":"config.schema"}`),
		ToolExecParams{
			GatewayOpts: gatewayclient.GatewayOptions{
				URL: "ws://127.0.0.1:26222",
				Caller: func(_ context.Context, _ gatewayclient.GatewayOptions, method string, _ interface{}) (map[string]interface{}, error) {
					gotMethod = method
					return map[string]interface{}{"version": "1.2.3"}, nil
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("ExecuteToolCall gateway: %v", err)
	}
	if gotMethod != "config.schema" {
		t.Fatalf("method=%q, want config.schema", gotMethod)
	}
	if !strings.Contains(result, `"version": "1.2.3"`) {
		t.Fatalf("result should include gateway payload, got %q", result)
	}
}

func TestExecuteToolCall_GatewayBrowserSet(t *testing.T) {
	var gotMethod string
	var gotBaseHash string

	result, err := ExecuteToolCall(context.Background(), "gateway",
		json.RawMessage(`{"action":"tools.browser.set","baseHash":"hash-789","enabled":true}`),
		ToolExecParams{
			GatewayOpts: gatewayclient.GatewayOptions{
				URL: "ws://127.0.0.1:26222",
				Caller: func(_ context.Context, _ gatewayclient.GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
					gotMethod = method
					if typed, ok := params.(map[string]interface{}); ok {
						if baseHash, ok := typed["baseHash"].(string); ok {
							gotBaseHash = baseHash
						}
					}
					return map[string]interface{}{"ok": true}, nil
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("ExecuteToolCall gateway: %v", err)
	}
	if gotMethod != "tools.browser.set" {
		t.Fatalf("method=%q, want tools.browser.set", gotMethod)
	}
	if gotBaseHash != "hash-789" {
		t.Fatalf("baseHash=%q, want hash-789", gotBaseHash)
	}
	if !strings.Contains(result, `"action": "tools.browser.set"`) {
		t.Fatalf("result should include gateway action envelope, got %q", result)
	}
}

func TestExecuteToolCall_GatewayUnavailable(t *testing.T) {
	_, err := ExecuteToolCall(context.Background(), "gateway",
		json.RawMessage(`{"action":"config.get"}`),
		ToolExecParams{},
	)
	if err == nil {
		t.Fatal("expected error when gateway tool is unavailable")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteToolCall_BrowserConfigTopLevel(t *testing.T) {
	var gotMethod string
	var gotBaseHash string

	result, err := ExecuteToolCall(context.Background(), "browser_config",
		json.RawMessage(`{"action":"set","baseHash":"hash-321","enabled":true}`),
		ToolExecParams{
			GatewayOpts: gatewayclient.GatewayOptions{
				URL: "ws://127.0.0.1:26222",
				Caller: func(_ context.Context, _ gatewayclient.GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
					gotMethod = method
					if typed, ok := params.(map[string]interface{}); ok {
						if baseHash, ok := typed["baseHash"].(string); ok {
							gotBaseHash = baseHash
						}
					}
					return map[string]interface{}{"ok": true}, nil
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("ExecuteToolCall browser_config: %v", err)
	}
	if gotMethod != "tools.browser.set" {
		t.Fatalf("method=%q, want tools.browser.set", gotMethod)
	}
	if gotBaseHash != "hash-321" {
		t.Fatalf("baseHash=%q, want hash-321", gotBaseHash)
	}
	if !strings.Contains(result, `"tool": "browser_config"`) {
		t.Fatalf("result should include top-level tool envelope, got %q", result)
	}
	if !strings.Contains(result, `"action": "set"`) {
		t.Fatalf("result should include action envelope, got %q", result)
	}
}

func TestExecuteToolCall_SpecializedConfigRequiresBaseHash(t *testing.T) {
	spec, ok := configtools.ToolSpecByName("browser_config")
	if !ok {
		t.Fatal("browser_config spec missing")
	}
	_, err := ExecuteToolCall(context.Background(), spec.ToolName,
		json.RawMessage(`{"action":"set","enabled":true}`),
		ToolExecParams{
			GatewayOpts: gatewayclient.GatewayOptions{URL: "ws://127.0.0.1:26222"},
		},
	)
	if err == nil {
		t.Fatal("expected missing baseHash error")
	}
	if !strings.Contains(err.Error(), "baseHash required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
