package nodehost

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// ---------- config_test ----------

func TestNormalizeConfig_EmptyNodeID(t *testing.T) {
	cfg := normalizeConfig(nil)
	if cfg.NodeID == "" {
		t.Fatal("expected auto-generated nodeId")
	}
	if cfg.Version != 1 {
		t.Fatalf("expected version 1, got %d", cfg.Version)
	}
}

func TestNormalizeConfig_PreservesExisting(t *testing.T) {
	input := &Config{Version: 1, NodeID: "  test-id  ", Token: "tok", DisplayName: "My Node"}
	cfg := normalizeConfig(input)
	if cfg.NodeID != "test-id" {
		t.Fatalf("expected trimmed nodeId, got %q", cfg.NodeID)
	}
	if cfg.Token != "tok" {
		t.Fatal("expected token preserved")
	}
}

func TestLoadSaveConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("OPENACOSMI_STATE_DIR", tmp)

	cfg := &Config{Version: 1, NodeID: "n1", DisplayName: "Test"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded := LoadConfig()
	if loaded == nil {
		t.Fatal("expected config to load")
	}
	if loaded.NodeID != "n1" {
		t.Fatalf("got nodeId=%q", loaded.NodeID)
	}
}

// ---------- sanitize_test ----------

func TestSanitizeEnv_BlockedKeys(t *testing.T) {
	env := SanitizeEnv(map[string]string{
		"NODE_OPTIONS": "malicious",
		"PYTHONHOME":   "/bad",
		"SAFE_VAR":     "ok",
	})
	if env == nil {
		t.Fatal("expected non-nil env")
	}
	if _, ok := env["NODE_OPTIONS"]; ok {
		t.Error("NODE_OPTIONS should be blocked")
	}
	if _, ok := env["PYTHONHOME"]; ok {
		t.Error("PYTHONHOME should be blocked")
	}
	if env["SAFE_VAR"] != "ok" {
		t.Error("SAFE_VAR should pass through")
	}
}

func TestSanitizeEnv_BlockedPrefixes(t *testing.T) {
	env := SanitizeEnv(map[string]string{
		"DYLD_INSERT_LIBRARIES": "/lib",
		"LD_PRELOAD":            "/lib",
		"NORMAL":                "ok",
	})
	if _, ok := env["DYLD_INSERT_LIBRARIES"]; ok {
		t.Error("DYLD_ prefix should be blocked")
	}
	if _, ok := env["LD_PRELOAD"]; ok {
		t.Error("LD_ prefix should be blocked")
	}
}

func TestSanitizeEnv_NilInput(t *testing.T) {
	if env := SanitizeEnv(nil); env != nil {
		t.Error("expected nil for nil input")
	}
}

func TestFormatCommand(t *testing.T) {
	tests := []struct {
		argv     []string
		expected string
	}{
		{[]string{"ls", "-la"}, "ls -la"},
		{[]string{"echo", "hello world"}, `echo "hello world"`},
		{[]string{""}, `""`},
	}
	for _, tt := range tests {
		got := FormatCommand(tt.argv)
		if got != tt.expected {
			t.Errorf("FormatCommand(%v) = %q, want %q", tt.argv, got, tt.expected)
		}
	}
}

func TestTruncateOutput(t *testing.T) {
	text, truncated := TruncateOutput("hello", 10)
	if truncated || text != "hello" {
		t.Error("short string should not truncate")
	}

	text, truncated = TruncateOutput("abcdefghij", 5)
	if !truncated {
		t.Error("should truncate")
	}
	if !strings.HasSuffix(text, "fghij") {
		t.Errorf("got %q", text)
	}
}

// ---------- exec_test ----------

func TestRunCommand_Echo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	result := RunCommand([]string{"echo", "hello"}, "", nil, 0)
	if !result.Success {
		t.Fatalf("expected success, error=%s", result.Error)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("got stdout=%q", result.Stdout)
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	result := RunCommand([]string{"sleep", "10"}, "", nil, 200)
	if !result.TimedOut {
		t.Fatal("expected timeout")
	}
}

func TestRunCommand_BadCommand(t *testing.T) {
	result := RunCommand([]string{"/nonexistent/binary"}, "", nil, 0)
	if result.Success {
		t.Fatal("expected failure")
	}
}

func TestResolveExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	// "sh" should be findable on any unix
	path := ResolveExecutable("sh", nil)
	if path == "" {
		t.Fatal("expected to find sh")
	}

	// path traversal should return empty
	path = ResolveExecutable("/bin/sh", nil)
	if path != "" {
		t.Error("should reject paths with /")
	}
}

func TestHandleSystemWhich(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	found := HandleSystemWhich([]string{"sh", "nonexistent_binary_xyz", ""}, nil)
	if _, ok := found["sh"]; !ok {
		t.Error("should find sh")
	}
	if _, ok := found["nonexistent_binary_xyz"]; ok {
		t.Error("should not find nonexistent binary")
	}
}

// ---------- invoke_test ----------

func TestCoerceNodeInvokePayload(t *testing.T) {
	// Valid
	payload := map[string]interface{}{
		"id": "inv-1", "nodeId": "n-1", "command": "system.run",
		"paramsJSON": `{"command":["echo"]}`,
	}
	req := CoerceNodeInvokePayload(payload)
	if req == nil {
		t.Fatal("expected non-nil")
	}
	if req.ID != "inv-1" || req.Command != "system.run" {
		t.Errorf("got %+v", req)
	}

	// Missing command
	if CoerceNodeInvokePayload(map[string]interface{}{"id": "x", "nodeId": "y"}) != nil {
		t.Error("should reject missing command")
	}

	// params → paramsJSON
	payload2 := map[string]interface{}{
		"id": "inv-2", "nodeId": "n-2", "command": "test",
		"params": map[string]interface{}{"key": "val"},
	}
	req2 := CoerceNodeInvokePayload(payload2)
	if req2 == nil || req2.ParamsJSON == "" {
		t.Fatal("should serialize params to paramsJSON")
	}
}

func TestBuildInvokeResult(t *testing.T) {
	frame := &NodeInvokeRequest{ID: "inv-1", NodeID: "n-1", Command: "system.run"}

	// OK result
	r := BuildInvokeResult(frame, true, `{"ok":true}`, nil)
	if !r.OK || r.PayloadJSON != `{"ok":true}` || r.Error != nil {
		t.Errorf("unexpected: %+v", r)
	}

	// Error, no payload
	r2 := BuildInvokeResult(frame, false, "", &InvokeErrorShape{Code: "BAD"})
	if r2.OK || r2.PayloadJSON != "" || r2.Error == nil {
		t.Errorf("unexpected: %+v", r2)
	}
}

func TestIsCmdExeInvocation(t *testing.T) {
	if IsCmdExeInvocation(nil) {
		t.Error("nil should return false")
	}
	if !IsCmdExeInvocation([]string{"cmd.exe", "/c", "dir"}) {
		t.Error("should detect cmd.exe")
	}
	if !IsCmdExeInvocation([]string{`C:\Windows\System32\cmd.exe`}) {
		t.Error("should detect full path cmd.exe")
	}
	if IsCmdExeInvocation([]string{"powershell"}) {
		t.Error("should not detect powershell")
	}
}

// ---------- skill_bins_test ----------

func TestSkillBinsCache(t *testing.T) {
	calls := 0
	cache := NewSkillBinsCache(func() ([]string, error) {
		calls++
		return []string{"git", "curl"}, nil
	})

	bins := cache.Current(false)
	if len(bins) != 2 {
		t.Fatalf("expected 2 bins, got %d", len(bins))
	}
	if _, ok := bins["git"]; !ok {
		t.Error("should contain git")
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}

	// 第二次调用不应触发 refresh（TTL 内）
	bins = cache.Current(false)
	if calls != 1 {
		t.Error("should use cache")
	}

	// 强制刷新
	bins = cache.Current(true)
	if calls != 2 {
		t.Error("force should trigger refresh")
	}
}

// ---------- runner_test (HandleInvoke) ----------

func TestNodeHostService_HandleWhich(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}

	var sentMethod string
	var sentParams interface{}
	svc := NewNodeHostService(
		&Config{Version: 1, NodeID: "test"},
		nil,
		func(method string, params interface{}) error {
			sentMethod = method
			sentParams = params
			return nil
		},
		nil, // requestFunc not needed for this test
	)
	// 用 slog.Default 替代 nil
	svc.logger = newTestLogger()

	payload := map[string]interface{}{
		"id": "test-1", "nodeId": "n1", "command": "system.which",
		"paramsJSON": `{"bins":["sh"]}`,
	}
	svc.HandleInvoke(payload)

	if sentMethod != "node.invoke.result" {
		t.Fatalf("expected node.invoke.result, got %s", sentMethod)
	}
	result, ok := sentParams.(*InvokeResult)
	if !ok {
		t.Fatalf("expected *InvokeResult, got %T", sentParams)
	}
	if !result.OK {
		t.Fatal("expected ok=true")
	}

	// 验证 payloadJSON 包含 sh
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result.PayloadJSON), &parsed); err != nil {
		t.Fatal(err)
	}
	bins, _ := parsed["bins"].(map[string]interface{})
	if _, ok := bins["sh"]; !ok {
		t.Error("should find sh in bins")
	}
}

func TestNodeHostService_HandleSystemRun_DeniesAllowlistMiss(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := infra.SaveExecApprovals(&infra.ExecApprovalsFile{
		Version: 1,
		Defaults: &infra.ExecApprovalsDefaults{
			Security: infra.ExecSecurityAllowlist,
			Ask:      infra.ExecAskOff,
		},
		Agents: map[string]*infra.ExecApprovalsAgent{},
	}); err != nil {
		t.Fatalf("save exec approvals: %v", err)
	}

	var sent []capturedNodeRequest
	svc := NewNodeHostService(
		&Config{Version: 1, NodeID: "test"},
		nil,
		func(method string, params interface{}) error {
			sent = append(sent, capturedNodeRequest{method: method, params: params})
			return nil
		},
		nil,
	)
	svc.logger = newTestLogger()

	frame := &NodeInvokeRequest{
		ID:      "inv-allowlist-miss",
		NodeID:  "n-1",
		Command: "system.run",
		ParamsJSON: `{
			"command":["/bin/echo","hello"],
			"rawCommand":"/bin/echo hello",
			"agentId":"main",
			"sessionKey":"sess-1",
			"runId":"run-1"
		}`,
	}

	svc.handleSystemRun(frame)

	result := findInvokeResult(t, sent)
	if result.OK {
		t.Fatal("expected system.run allowlist miss to be denied")
	}
	if result.Error == nil || result.Error.Code != "FORBIDDEN" {
		t.Fatalf("expected FORBIDDEN error, got %+v", result.Error)
	}
	if result.Error == nil || !strings.Contains(result.Error.Message, "allowlist miss") {
		t.Fatalf("expected allowlist miss error message, got %+v", result.Error)
	}
	if event := findNodeEvent(t, sent); event != "exec.denied" {
		t.Fatalf("expected exec.denied event, got %q", event)
	}
}

func TestNodeHostService_HandleSystemRun_AllowsApprovedAllowOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := infra.SaveExecApprovals(&infra.ExecApprovalsFile{
		Version: 1,
		Defaults: &infra.ExecApprovalsDefaults{
			Security: infra.ExecSecurityAllowlist,
			Ask:      infra.ExecAskOnMiss,
		},
		Agents: map[string]*infra.ExecApprovalsAgent{},
	}); err != nil {
		t.Fatalf("save exec approvals: %v", err)
	}

	var sent []capturedNodeRequest
	svc := NewNodeHostService(
		&Config{Version: 1, NodeID: "test"},
		nil,
		func(method string, params interface{}) error {
			sent = append(sent, capturedNodeRequest{method: method, params: params})
			return nil
		},
		nil,
	)
	svc.logger = newTestLogger()

	frame := &NodeInvokeRequest{
		ID:      "inv-approved-once",
		NodeID:  "n-1",
		Command: "system.run",
		ParamsJSON: `{
			"command":["/bin/echo","hello"],
			"rawCommand":"/bin/echo hello",
			"agentId":"main",
			"sessionKey":"sess-1",
			"runId":"run-2",
			"approved":true,
			"approvalDecision":"allow-once"
		}`,
	}

	svc.handleSystemRun(frame)

	result := findInvokeResult(t, sent)
	if !result.OK {
		t.Fatalf("expected approved system.run to succeed, got %+v", result.Error)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result.PayloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal invoke payload: %v", err)
	}
	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected success payload, got %+v", payload)
	}
	if stdout, _ := payload["stdout"].(string); strings.TrimSpace(stdout) != "hello" {
		t.Fatalf("expected stdout=hello, got %+v", payload)
	}
	if event := findNodeEvent(t, sent); event != "exec.finished" {
		t.Fatalf("expected exec.finished event, got %q", event)
	}
}

func TestNodeHostService_HandleSystemRun_PersistsAllowAlways(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := infra.SaveExecApprovals(&infra.ExecApprovalsFile{
		Version: 1,
		Defaults: &infra.ExecApprovalsDefaults{
			Security: infra.ExecSecurityAllowlist,
			Ask:      infra.ExecAskOnMiss,
		},
		Agents: map[string]*infra.ExecApprovalsAgent{},
	}); err != nil {
		t.Fatalf("save exec approvals: %v", err)
	}

	var sent []capturedNodeRequest
	svc := NewNodeHostService(
		&Config{Version: 1, NodeID: "test"},
		nil,
		func(method string, params interface{}) error {
			sent = append(sent, capturedNodeRequest{method: method, params: params})
			return nil
		},
		nil,
	)
	svc.logger = newTestLogger()

	frame := &NodeInvokeRequest{
		ID:      "inv-approved-always",
		NodeID:  "n-1",
		Command: "system.run",
		ParamsJSON: `{
			"command":["/bin/echo","hello"],
			"rawCommand":"/bin/echo hello",
			"agentId":"main",
			"sessionKey":"sess-1",
			"runId":"run-3",
			"approved":true,
			"approvalDecision":"allow-always"
		}`,
	}

	svc.handleSystemRun(frame)

	result := findInvokeResult(t, sent)
	if !result.OK {
		t.Fatalf("expected allow-always system.run to succeed, got %+v", result.Error)
	}

	snapshot := infra.ReadExecApprovalsSnapshot()
	agent := snapshot.File.Agents["main"]
	if agent == nil {
		t.Fatal("expected main agent approvals after allow-always")
	}
	found := false
	for _, entry := range agent.Allowlist {
		if entry.Pattern == "/bin/echo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /bin/echo in allowlist after allow-always, got %+v", agent.Allowlist)
	}
}

// ---------- config file permission test ----------

func TestConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	tmp := t.TempDir()
	t.Setenv("OPENACOSMI_STATE_DIR", tmp)

	cfg := &Config{Version: 1, NodeID: "perm-test"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(tmp, nodeHostFile))
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected 0600, got %o", perm)
	}
}

// ---------- helpers ----------

func newTestLogger() *slog.Logger {
	return slog.Default()
}

type capturedNodeRequest struct {
	method string
	params interface{}
}

func findInvokeResult(t *testing.T, sent []capturedNodeRequest) *InvokeResult {
	t.Helper()
	for _, entry := range sent {
		if entry.method == "node.invoke.result" {
			result, ok := entry.params.(*InvokeResult)
			if !ok {
				t.Fatalf("expected *InvokeResult, got %T", entry.params)
			}
			return result
		}
	}
	t.Fatal("missing node.invoke.result")
	return nil
}

func findNodeEvent(t *testing.T, sent []capturedNodeRequest) string {
	t.Helper()
	for _, entry := range sent {
		if entry.method != "node.event" {
			continue
		}
		params, ok := entry.params.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map params for node.event, got %T", entry.params)
		}
		event, _ := params["event"].(string)
		if event != "" {
			return event
		}
	}
	t.Fatal("missing node.event")
	return ""
}
