package configtools

import (
	"context"
	"testing"
)

func TestToolParametersExposeDomainActionsAndFields(t *testing.T) {
	spec, ok := ToolSpecByName("remote_approval_config")
	if !ok {
		t.Fatal("remote_approval_config spec not found")
	}

	params := ToolParameters(spec)
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type=%T", params["properties"])
	}
	actionProp, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatalf("action type=%T", props["action"])
	}
	enumVals, ok := actionProp["enum"].([]any)
	if !ok {
		t.Fatalf("action enum type=%T", actionProp["enum"])
	}
	got := make(map[string]bool, len(enumVals))
	for _, value := range enumVals {
		got[value.(string)] = true
	}
	for _, want := range []string{"get", "set", "test"} {
		if !got[want] {
			t.Fatalf("missing action %q in schema", want)
		}
	}
	for _, key := range []string{"baseHash", "provider", "feishu", "dingtalk", "wecom"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("schema missing %q", key)
		}
	}
}

func TestBuildActionParamsCoercesTypedFields(t *testing.T) {
	spec, ok := ToolSpecByName("media_config")
	if !ok {
		t.Fatal("media_config spec not found")
	}
	action, ok := FindActionSpec(spec, "update")
	if !ok {
		t.Fatal("media_config update action not found")
	}

	params, err := BuildActionParams(map[string]any{
		"baseHash":           "hash-123",
		"monitorIntervalMin": "15",
		"hotKeywords":        []any{"go", "gateway"},
		"autoDraftEnabled":   "true",
		"trendingThreshold":  "0.75",
	}, action.RequiredParams, action.AllowedParams)
	if err != nil {
		t.Fatalf("BuildActionParams: %v", err)
	}

	if params["baseHash"] != "hash-123" {
		t.Fatalf("baseHash=%v", params["baseHash"])
	}
	if params["monitorIntervalMin"] != 15 {
		t.Fatalf("monitorIntervalMin=%v", params["monitorIntervalMin"])
	}
	if params["autoDraftEnabled"] != true {
		t.Fatalf("autoDraftEnabled=%v", params["autoDraftEnabled"])
	}
	if params["trendingThreshold"] != 0.75 {
		t.Fatalf("trendingThreshold=%v", params["trendingThreshold"])
	}
	hotKeywords, ok := params["hotKeywords"].([]string)
	if !ok {
		t.Fatalf("hotKeywords type=%T", params["hotKeywords"])
	}
	if len(hotKeywords) != 2 || hotKeywords[0] != "go" || hotKeywords[1] != "gateway" {
		t.Fatalf("hotKeywords=%v", hotKeywords)
	}
}

func TestExecuteToolActionRequiresBaseHashForWrite(t *testing.T) {
	spec, ok := ToolSpecByName("browser_config")
	if !ok {
		t.Fatal("browser_config spec not found")
	}

	_, err := ExecuteToolAction(context.Background(), spec, map[string]any{
		"action":  "set",
		"enabled": true,
	}, func(context.Context, string, interface{}) (map[string]interface{}, error) {
		t.Fatal("call should not be reached when baseHash is missing")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected missing baseHash error")
	}
	if err.Error() != "baseHash required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteToolActionMapsMethodAndEnvelope(t *testing.T) {
	spec, ok := ToolSpecByName("remote_approval_config")
	if !ok {
		t.Fatal("remote_approval_config spec not found")
	}

	var gotMethod string
	var gotParams interface{}
	result, err := ExecuteToolAction(context.Background(), spec, map[string]any{
		"action":   "test",
		"provider": "feishu",
	}, func(_ context.Context, method string, params interface{}) (map[string]interface{}, error) {
		gotMethod = method
		gotParams = params
		return map[string]interface{}{"ok": true}, nil
	})
	if err != nil {
		t.Fatalf("ExecuteToolAction: %v", err)
	}
	if gotMethod != "security.remoteApproval.test" {
		t.Fatalf("method=%q", gotMethod)
	}
	typedParams, ok := gotParams.(map[string]interface{})
	if !ok {
		t.Fatalf("params type=%T", gotParams)
	}
	if typedParams["provider"] != "feishu" {
		t.Fatalf("provider=%v", typedParams["provider"])
	}

	envelope, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type=%T", result)
	}
	if envelope["tool"] != "remote_approval_config" {
		t.Fatalf("tool=%v", envelope["tool"])
	}
	if envelope["action"] != "test" {
		t.Fatalf("action=%v", envelope["action"])
	}
	if envelope["status"] != "verified" {
		t.Fatalf("status=%v", envelope["status"])
	}
}
