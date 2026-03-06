package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// ---------- exec.approvals.get ----------

func TestExecApprovalsHandlers_GetEmpty(t *testing.T) {
	// 使用临时目录作为 HOME，确保不存在 exec-approvals.json
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
	if result["path"] == "" {
		t.Error("expected non-empty path")
	}
	if result["hash"] == "" {
		t.Error("expected non-empty hash")
	}
}

// ---------- exec.approvals.set + get round-trip ----------

func TestExecApprovalsHandlers_SetAndGet(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	r := NewMethodRegistry()
	r.RegisterAll(ExecApprovalsHandlers())

	// 1. First GET to establish baseline hash
	req := &RequestFrame{Method: "exec.approvals.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Fatal("initial GET should succeed")
	}
	result := gotPayload.(map[string]interface{})
	hash1 := result["hash"].(string)

	// 2. SET with baseHash
	req = &RequestFrame{Method: "exec.approvals.set", Params: map[string]interface{}{
		"baseHash": hash1,
		"file": map[string]interface{}{
			"version": float64(1),
			"defaults": map[string]interface{}{
				"security": "allowlist",
			},
		},
	}}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Fatal("SET should succeed")
	}
	result2 := gotPayload.(map[string]interface{})
	hash2 := result2["hash"].(string)

	// Hash should change after SET
	if hash2 == hash1 {
		t.Error("hash should change after SET")
	}

	// 3. Second GET should return new hash
	req = &RequestFrame{Method: "exec.approvals.get", Params: map[string]interface{}{}}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Fatal("second GET should succeed")
	}
	result3 := gotPayload.(map[string]interface{})
	if result3["hash"] != hash2 {
		t.Error("GET hash should match SET response hash")
	}
}

// ---------- exec.approvals.set OCC conflict ----------

func TestExecApprovalsHandlers_SetConflict(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Pre-seed the file so it exists
	ocDir := filepath.Join(tmpHome, ".openacosmi")
	os.MkdirAll(ocDir, 0o755)
	os.WriteFile(filepath.Join(ocDir, "exec-approvals.json"), []byte(`{"version":1}`), 0o600)

	r := NewMethodRegistry()
	r.RegisterAll(ExecApprovalsHandlers())

	// Try SET with a wrong baseHash
	req := &RequestFrame{Method: "exec.approvals.set", Params: map[string]interface{}{
		"baseHash": "wrong-hash-value",
		"file": map[string]interface{}{
			"version": float64(1),
		},
	}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should fail with stale baseHash")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request for conflict, got %v", gotErr)
	}
}

// ---------- exec.approvals.node.get / node.set ----------

func TestExecApprovalsHandlers_NodeGetUnsupported(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ExecApprovalsHandlers())

	req := &RequestFrame{Method: "exec.approvals.node.get", Params: map[string]interface{}{
		"nodeId": "test-node",
	}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Fatal("exec.approvals.node.get should fail with unsupported_feature")
	}
	if gotErr == nil || gotErr.Code != ErrCodeUnsupportedFeature {
		t.Fatalf("expected unsupported_feature, got %v", gotErr)
	}
	if gotErr.Details == nil {
		t.Fatal("expected details on unsupported node approvals error")
	}
}

func TestExecApprovalsHandlers_NodeGetMissingNodeId(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ExecApprovalsHandlers())

	req := &RequestFrame{Method: "exec.approvals.node.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should fail without nodeId")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request, got %v", gotErr)
	}
}

func TestExecApprovalsHandlers_NodeSetUnsupported(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ExecApprovalsHandlers())

	req := &RequestFrame{Method: "exec.approvals.node.set", Params: map[string]interface{}{
		"nodeId": "test-node",
		"file":   map[string]interface{}{"version": float64(1)},
	}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Fatal("exec.approvals.node.set should fail with unsupported_feature")
	}
	if gotErr == nil || gotErr.Code != ErrCodeUnsupportedFeature {
		t.Fatalf("expected unsupported_feature, got %v", gotErr)
	}
}

// ---------- infra: RedactExecApprovals ----------

func TestRedactExecApprovals(t *testing.T) {
	file := &infra.ExecApprovalsFile{
		Version: 1,
		Socket: &infra.ExecApprovalsSocket{
			Path:  "/tmp/test.sock",
			Token: "secret-token-123",
		},
	}
	redacted := infra.RedactExecApprovals(file)
	if redacted.Socket == nil {
		t.Fatal("socket should not be nil after redact")
	}
	if redacted.Socket.Path != "/tmp/test.sock" {
		t.Error("socket path should be preserved")
	}
	if redacted.Socket.Token != "" {
		t.Errorf("socket token should be redacted, got %q", redacted.Socket.Token)
	}
}
