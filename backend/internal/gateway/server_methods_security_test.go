package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------- security.get ----------

func TestSecurityHandlers_GetDefault(t *testing.T) {
	// 使用临时目录作为 HOME
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	r := NewMethodRegistry()
	r.RegisterAll(SecurityHandlers())

	req := &RequestFrame{Method: "security.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Fatal("security.get should succeed")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}

	// 默认安全级别应为 "deny"
	if result["currentLevel"] != "deny" {
		t.Errorf("expected default level 'deny', got %v", result["currentLevel"])
	}

	// 默认不应该是永久完全授权
	if result["isPermanentFull"] != false {
		t.Errorf("expected isPermanentFull=false, got %v", result["isPermanentFull"])
	}

	// 应该返回 4 个级别描述（L0 deny, L1 allowlist, L2 sandboxed, L3 full）
	levels, ok := result["levels"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected levels to be []map, got %T", result["levels"])
	}
	if len(levels) != 4 {
		t.Errorf("expected 4 levels, got %d", len(levels))
	}

	// hash 不为空
	if result["hash"] == "" {
		t.Error("expected non-empty hash")
	}

	if _, err := os.Stat(filepath.Join(tmpHome, ".openacosmi", "exec-approvals.json")); !os.IsNotExist(err) {
		t.Fatalf("security.get should not create exec-approvals.json, err=%v", err)
	}
}

func TestExecApprovalsHandlers_GetDoesNotCreateFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	r := NewMethodRegistry()
	r.RegisterAll(ExecApprovalsHandlers())

	req := &RequestFrame{Method: "exec.approvals.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Fatal("exec.approvals.get should succeed")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if result["exists"] != false {
		t.Fatalf("expected exists=false, got %v", result["exists"])
	}
	if _, err := os.Stat(filepath.Join(tmpHome, ".openacosmi", "exec-approvals.json")); !os.IsNotExist(err) {
		t.Fatalf("exec.approvals.get should not create exec-approvals.json, err=%v", err)
	}
}

func TestSecurityHandlers_GetWithFullSecurity(t *testing.T) {
	// 使用临时目录作为 HOME，预设 exec-approvals.json
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// 创建带 full 安全级别的配置文件
	ocDir := filepath.Join(tmpHome, ".openacosmi")
	os.MkdirAll(ocDir, 0o755)
	os.WriteFile(filepath.Join(ocDir, "exec-approvals.json"), []byte(`{
		"version": 1,
		"defaults": {
			"security": "full"
		}
	}`), 0o600)

	r := NewMethodRegistry()
	r.RegisterAll(SecurityHandlers())

	req := &RequestFrame{Method: "security.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Fatal("security.get should succeed")
	}
	result := gotPayload.(map[string]interface{})

	if result["currentLevel"] != "full" {
		t.Errorf("expected level 'full', got %v", result["currentLevel"])
	}
	if result["isPermanentFull"] != true {
		t.Errorf("expected isPermanentFull=true, got %v", result["isPermanentFull"])
	}
}
