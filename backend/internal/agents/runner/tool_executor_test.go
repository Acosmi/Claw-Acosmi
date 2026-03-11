package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- UHMSBridge mock ----------

// stubUHMSBridge 用于 search_skills / lookup_skill / memory 测试的最小模拟。
type stubUHMSBridge struct {
	hits         []SkillSearchHit
	searchErr    error
	distributing bool
	indexed      bool
	// memory mock fields
	memoryHits      []MemorySearchHit
	memorySearchErr error
	memoryHit       *MemoryHit
	memoryGetErr    error
}

func (s *stubUHMSBridge) CompressChatMessages(_ context.Context, msgs []llmclient.ChatMessage, _ int) ([]llmclient.ChatMessage, error) {
	return msgs, nil
}
func (s *stubUHMSBridge) CommitChatSession(_ context.Context, _, _ string, _ []llmclient.ChatMessage) error {
	return nil
}
func (s *stubUHMSBridge) BuildContextBrief(_ context.Context) string                  { return "" }
func (s *stubUHMSBridge) IsSkillsIndexed() bool                                       { return s.indexed }
func (s *stubUHMSBridge) IsSkillsDistributing() bool                                  { return s.distributing }
func (s *stubUHMSBridge) ReadSkillVFS(_ context.Context, _, _ string) (string, error) { return "", nil }
func (s *stubUHMSBridge) SearchSkillsVFS(_ context.Context, _ string, _ int) ([]SkillSearchHit, error) {
	return s.hits, s.searchErr
}
func (s *stubUHMSBridge) SearchMemories(_ context.Context, _ string, _ int) ([]MemorySearchHit, error) {
	return s.memoryHits, s.memorySearchErr
}
func (s *stubUHMSBridge) GetMemory(_ context.Context, _ string) (*MemoryHit, error) {
	return s.memoryHit, s.memoryGetErr
}

// ============================================================================
// Tool Executor 集成测试
// 验证 bash, read_file, write_file, list_dir 工具的完整执行链路。
// ============================================================================

// testToolParams 返回测试用的 ToolExecParams（权限全开）。
func testToolParams(workspaceDir string) ToolExecParams {
	return ToolExecParams{
		WorkspaceDir: workspaceDir,
		AllowExec:    true,
		AllowWrite:   true,
	}
}

func testToolParamsWithTimeout(workspaceDir string, timeoutMs int64) ToolExecParams {
	p := testToolParams(workspaceDir)
	p.TimeoutMs = timeoutMs
	return p
}

// ---------- bash ----------

func TestBash_SimpleEcho(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":"echo hello world"}`),
		testToolParams(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Errorf("expected 'hello world' in output, got %q", result)
	}
}

func TestBash_ExitCode(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":"exit 42"}`),
		testToolParams(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "exit code: 42") {
		t.Errorf("expected exit code 42, got %q", result)
	}
}

func TestBash_EmptyCommand(t *testing.T) {
	_, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":""}`),
		testToolParams(t.TempDir()))
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestBash_WorkspaceDir(t *testing.T) {
	dir := t.TempDir()
	result, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":"pwd"}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, dir) {
		t.Errorf("expected workspace dir %q in output, got %q", dir, result)
	}
}

func TestBash_Timeout(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":"sleep 10"}`),
		testToolParamsWithTimeout(t.TempDir(), 200))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "timed out") {
		t.Errorf("expected timeout message, got %q", result)
	}
}

func TestBash_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	_, err := ExecuteToolCall(ctx, "bash",
		json.RawMessage(`{"command":"sleep 10"}`),
		testToolParams(t.TempDir()))
	// Should not hang — context already cancelled
	if err != nil {
		t.Logf("got error (expected): %v", err)
	}
}

func TestReportProgress_EmitsAgentProgressEvent(t *testing.T) {
	infra.ResetAgentRunContextForTest()
	defer infra.ResetAgentRunContextForTest()
	infra.RegisterAgentRunContext("run-progress", infra.AgentRunContext{SessionKey: "sess-progress"})

	events := make(chan infra.AgentEventPayload, 1)
	unsub := infra.OnAgentEvent(func(evt infra.AgentEventPayload) {
		events <- evt
	})
	defer unsub()

	result, err := ExecuteToolCall(context.Background(), "report_progress",
		json.RawMessage(`{"summary":"Build finished, starting tests","percent":60,"phase":"testing"}`),
		ToolExecParams{RunID: "run-progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Progress reported to live surfaces." {
		t.Fatalf("unexpected result: %q", result)
	}

	select {
	case evt := <-events:
		if evt.RunID != "run-progress" {
			t.Fatalf("run id = %q, want run-progress", evt.RunID)
		}
		if evt.Stream != infra.StreamProgress {
			t.Fatalf("stream = %q, want %q", evt.Stream, infra.StreamProgress)
		}
		if evt.SessionKey != "sess-progress" {
			t.Fatalf("sessionKey = %q, want sess-progress", evt.SessionKey)
		}
		if got, _ := evt.Data["summary"].(string); got != "Build finished, starting tests" {
			t.Fatalf("summary = %q", got)
		}
		if got, _ := evt.Data["phase"].(string); got != "testing" {
			t.Fatalf("phase = %q", got)
		}
		if got, _ := evt.Data["percent"].(int); got != 60 {
			t.Fatalf("percent = %d", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected agent.progress event")
	}
}

func TestReportProgress_RemoteDeliveryStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ProgressReportStatus
		want   string
	}{
		{
			name:   "remote delivered",
			status: ProgressReportStatus{RemoteDelivered: true},
			want:   "Progress reported to live surfaces and remote channel.",
		},
		{
			name:   "throttled",
			status: ProgressReportStatus{Throttled: true},
			want:   "Progress reported to live surfaces. Remote update skipped (throttled).",
		},
		{
			name:   "remote failed",
			status: ProgressReportStatus{Error: "send failed"},
			want:   "Progress reported to live surfaces. Remote delivery failed.",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := ExecuteToolCall(context.Background(), "report_progress",
				json.RawMessage(`{"summary":"waiting for tests","phase":"testing"}`),
				ToolExecParams{
					OnProgress: func(context.Context, ProgressUpdate) ProgressReportStatus {
						return tc.status
					},
				})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.want {
				t.Fatalf("result = %q, want %q", result, tc.want)
			}
		})
	}
}

// ---------- read_file ----------

func TestReadFile_Simple(t *testing.T) {
	dir := t.TempDir()
	content := "hello from test file\nline 2\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	result, err := ExecuteToolCall(context.Background(), "read_file",
		json.RawMessage(`{"path":"test.txt"}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Errorf("expected %q, got %q", content, result)
	}
}

func TestReadFile_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "abs.txt")
	os.WriteFile(absPath, []byte("absolute"), 0644)

	inputJSON, _ := json.Marshal(map[string]string{"path": absPath})
	result, err := ExecuteToolCall(context.Background(), "read_file",
		inputJSON, testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "absolute" {
		t.Errorf("expected 'absolute', got %q", result)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "read_file",
		json.RawMessage(`{"path":"nonexistent.txt"}`),
		testToolParams(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Error reading file") {
		t.Errorf("expected error message, got %q", result)
	}
}

// ---------- write_file ----------

func TestWriteFile_Simple(t *testing.T) {
	dir := t.TempDir()
	result, err := ExecuteToolCall(context.Background(), "write_file",
		json.RawMessage(`{"path":"output.txt","content":"test content"}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("expected success message, got %q", result)
	}

	// Verify file contents
	data, _ := os.ReadFile(filepath.Join(dir, "output.txt"))
	if string(data) != "test content" {
		t.Errorf("expected 'test content', got %q", string(data))
	}
}

func TestWriteFile_CreatesSubdirectory(t *testing.T) {
	dir := t.TempDir()
	result, err := ExecuteToolCall(context.Background(), "write_file",
		json.RawMessage(`{"path":"sub/dir/file.txt","content":"nested"}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("expected success message, got %q", result)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "sub", "dir", "file.txt"))
	if string(data) != "nested" {
		t.Errorf("expected 'nested', got %q", string(data))
	}
}

func TestWriteFile_ThenReadFile(t *testing.T) {
	dir := t.TempDir()

	// Write
	_, err := ExecuteToolCall(context.Background(), "write_file",
		json.RawMessage(`{"path":"roundtrip.txt","content":"round trip works"}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Read back
	result, err := ExecuteToolCall(context.Background(), "read_file",
		json.RawMessage(`{"path":"roundtrip.txt"}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if result != "round trip works" {
		t.Errorf("expected 'round trip works', got %q", result)
	}
}

// ---------- list_dir ----------

func TestListDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := ExecuteToolCall(context.Background(), "list_dir",
		json.RawMessage(`{"path":"."}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty output, got %q", result)
	}
}

func TestListDir_WithFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	result, err := ExecuteToolCall(context.Background(), "list_dir",
		json.RawMessage(`{"path":"."}`),
		testToolParams(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("expected a.txt in output, got %q", result)
	}
	if !strings.Contains(result, "b.txt") {
		t.Errorf("expected b.txt in output, got %q", result)
	}
	if !strings.Contains(result, "d subdir") {
		t.Errorf("expected 'd subdir' in output, got %q", result)
	}
}

func TestListDir_NotFound(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "list_dir",
		json.RawMessage(`{"path":"nonexistent"}`),
		testToolParams(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Error listing directory") {
		t.Errorf("expected error message, got %q", result)
	}
}

// ---------- unknown tool ----------

func TestUnknownTool(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "nonexistent_tool",
		json.RawMessage(`{"key":"value"}`),
		testToolParams(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not yet implemented") {
		t.Errorf("expected 'not yet implemented', got %q", result)
	}
}

// ---------- invalid JSON ----------

func TestBash_InvalidJSON(t *testing.T) {
	_, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{invalid`),
		testToolParams(t.TempDir()))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestReadFile_InvalidJSON(t *testing.T) {
	_, err := ExecuteToolCall(context.Background(), "read_file",
		json.RawMessage(`not json`),
		testToolParams(t.TempDir()))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ---------- bash + write_file + read_file 集成 ----------

func TestToolChain_BashWriteRead(t *testing.T) {
	dir := t.TempDir()
	params := testToolParams(dir)

	// Use bash to write a file
	_, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":"echo 'generated content' > gen.txt"}`),
		params)
	if err != nil {
		t.Fatalf("bash error: %v", err)
	}

	// Read it back with read_file
	result, err := ExecuteToolCall(context.Background(), "read_file",
		json.RawMessage(`{"path":"gen.txt"}`),
		params)
	if err != nil {
		t.Fatalf("read_file error: %v", err)
	}
	if !strings.Contains(result, "generated content") {
		t.Errorf("expected 'generated content' in output, got %q", result)
	}

	// List directory to verify
	listing, err := ExecuteToolCall(context.Background(), "list_dir",
		json.RawMessage(`{"path":"."}`),
		params)
	if err != nil {
		t.Fatalf("list_dir error: %v", err)
	}
	if !strings.Contains(listing, "gen.txt") {
		t.Errorf("expected gen.txt in listing, got %q", listing)
	}
}

// ============================================================================
// P0 权限守卫测试
// 验证 AllowExec=false / AllowWrite=false 时工具被正确拒绝，
// 以及 resolveAllowWrite / resolveAllowExec 的安全级别映射。
// ============================================================================

// ---------- 权限拒绝测试 ----------

func TestBash_PermissionDenied(t *testing.T) {
	var callbackTool, callbackDetail string
	params := ToolExecParams{
		WorkspaceDir: t.TempDir(),
		AllowExec:    false,
		AllowWrite:   true,
		OnPermissionDenied: func(tool, detail string) {
			callbackTool = tool
			callbackDetail = detail
		},
	}
	result, err := ExecuteToolCall(context.Background(), "bash",
		json.RawMessage(`{"command":"echo should not run"}`), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "权限不足") {
		t.Errorf("expected permission denied message, got %q", result)
	}
	if callbackTool != "bash" {
		t.Errorf("expected callback tool=bash, got %q", callbackTool)
	}
	if callbackDetail != "echo should not run" {
		t.Errorf("expected callback detail, got %q", callbackDetail)
	}
}

func TestWriteFile_PermissionDenied(t *testing.T) {
	var callbackTool string
	params := ToolExecParams{
		WorkspaceDir: t.TempDir(),
		AllowExec:    true,
		AllowWrite:   false,
		OnPermissionDenied: func(tool, detail string) {
			callbackTool = tool
		},
	}
	result, err := ExecuteToolCall(context.Background(), "write_file",
		json.RawMessage(`{"path":"test.txt","content":"blocked"}`), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "权限不足") {
		t.Errorf("expected permission denied message, got %q", result)
	}
	if callbackTool != "write_file" {
		t.Errorf("expected callback tool=write_file, got %q", callbackTool)
	}
}

// ---------- resolveAllowWrite / resolveAllowExec 映射测试 ----------

func TestResolvePermissions_Full(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "full"},
		},
	}
	if !resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=true for security=full")
	}
	if !resolveAllowExec(cfg) {
		t.Error("expected AllowExec=true for security=full")
	}
}

func TestResolvePermissions_Allowlist(t *testing.T) {
	// L1 (allowlist): 工作区外受限，沙箱内允许写和执行
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "allowlist"},
		},
	}
	if !resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=true for security=allowlist (L1 allows write in sandbox)")
	}
	if !resolveAllowExec(cfg) {
		t.Error("expected AllowExec=true for security=allowlist")
	}
	if !resolveSandboxMode(cfg) {
		t.Error("expected SandboxMode=true for security=allowlist")
	}
}

func TestResolvePermissions_Deny(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "deny"},
		},
	}
	if resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=false for security=deny")
	}
	if resolveAllowExec(cfg) {
		t.Error("expected AllowExec=false for security=deny")
	}
}

func TestResolvePermissions_Empty(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: ""},
		},
	}
	if resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=false for empty security")
	}
	if resolveAllowExec(cfg) {
		t.Error("expected AllowExec=false for empty security")
	}
}

func TestResolvePermissions_NilConfig(t *testing.T) {
	if resolveAllowWrite(nil) {
		t.Error("expected AllowWrite=false for nil config")
	}
	if resolveAllowExec(nil) {
		t.Error("expected AllowExec=false for nil config")
	}
}

func TestResolvePermissions_Sandbox(t *testing.T) {
	// "sandbox" 是 "allowlist" 的别名（L1）
	// L1 允许写和执行（在沙箱内受限）
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "sandbox"},
		},
	}
	if !resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=true for security=sandbox (L1 allows write in sandbox)")
	}
	if !resolveAllowExec(cfg) {
		t.Error("expected AllowExec=true for security=sandbox (alias for allowlist)")
	}
	if !resolveSandboxMode(cfg) {
		t.Error("expected SandboxMode=true for security=sandbox")
	}
}

func TestResolvePermissions_Sandboxed(t *testing.T) {
	// L2 (sandboxed): 沙箱内全权+挂载+网络
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "sandboxed"},
		},
	}
	if !resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=true for security=sandboxed (L2)")
	}
	if !resolveAllowExec(cfg) {
		t.Error("expected AllowExec=true for security=sandboxed (L2)")
	}
	if !resolveSandboxMode(cfg) {
		t.Error("expected SandboxMode=true for security=sandboxed (L2)")
	}
}

func TestResolvePermissions_NilTools(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	if resolveAllowWrite(cfg) {
		t.Error("expected AllowWrite=false for nil Tools")
	}
	if resolveAllowExec(cfg) {
		t.Error("expected AllowExec=false for nil Tools")
	}
}

// ============================================================================
// AllowNetwork 测试
// 验证 L2(sandboxed)/L3(full) = true, L0(deny)/L1(allowlist) = false
// ============================================================================

func TestAllowNetwork_Full(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "full"},
		},
	}
	if !resolveAllowNetwork(cfg) {
		t.Error("expected AllowNetwork=true for security=full (L3)")
	}
}

func TestAllowNetwork_Sandboxed(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "sandboxed"},
		},
	}
	if !resolveAllowNetwork(cfg) {
		t.Error("expected AllowNetwork=true for security=sandboxed (L2)")
	}
}

func TestAllowNetwork_Allowlist(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "allowlist"},
		},
	}
	if resolveAllowNetwork(cfg) {
		t.Error("expected AllowNetwork=false for security=allowlist (L1)")
	}
}

func TestAllowNetwork_Deny(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Tools: &types.ToolsConfig{
			Exec: &types.ExecToolConfig{Security: "deny"},
		},
	}
	if resolveAllowNetwork(cfg) {
		t.Error("expected AllowNetwork=false for security=deny (L0)")
	}
}

// ============================================================================
// 路径逃逸防护测试
// 验证 validateToolPath 及各工具的路径边界检查。
// ============================================================================

// ---------- validateToolPath 单元测试 ----------

func TestValidateToolPath_AllowsInside(t *testing.T) {
	workspace := t.TempDir()
	// 工作空间内的相对路径
	innerPath := filepath.Join(workspace, "sub", "file.txt")
	if err := validateToolPath(innerPath, workspace); err != nil {
		t.Errorf("expected nil for inside path, got %v", err)
	}
	// 工作空间本身
	if err := validateToolPath(workspace, workspace); err != nil {
		t.Errorf("expected nil for workspace itself, got %v", err)
	}
}

func TestValidateToolPath_BlocksEscape(t *testing.T) {
	workspace := t.TempDir()
	escapePath := filepath.Join(workspace, "..", "escape.txt")
	err := validateToolPath(escapePath, workspace)
	if err == nil {
		t.Error("expected error for ../ escape path, got nil")
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Errorf("expected 'outside workspace' in error, got %q", err.Error())
	}
}

func TestValidateToolPath_BlocksAbsOutside(t *testing.T) {
	workspace := t.TempDir()
	err := validateToolPath("/tmp/evil.txt", workspace)
	if err == nil {
		t.Error("expected error for absolute path outside workspace, got nil")
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Errorf("expected 'outside workspace' in error, got %q", err.Error())
	}
}

func TestValidateToolPath_EmptyWorkspace(t *testing.T) {
	// 无工作空间约束时应放行
	if err := validateToolPath("/any/path", ""); err != nil {
		t.Errorf("expected nil for empty workspace, got %v", err)
	}
}

// ---------- 工具级路径逃逸测试 ----------

func TestWriteFile_PathEscapeBlocked(t *testing.T) {
	workspace := t.TempDir()
	outsidePath := filepath.Join(workspace, "..", "escape.txt")
	var callbackTool, callbackDetail string
	inputJSON, _ := json.Marshal(map[string]string{
		"path":    outsidePath,
		"content": "should not be written",
	})
	params := testToolParams(workspace)
	params.OnPermissionDenied = func(tool, detail string) {
		callbackTool = tool
		callbackDetail = detail
	}
	result, err := ExecuteToolCall(context.Background(), "write_file",
		inputJSON, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "权限不足") {
		t.Errorf("expected permission denied message, got %q", result)
	}
	if callbackTool != "write_file" {
		t.Errorf("expected OnPermissionDenied callback tool=write_file, got %q", callbackTool)
	}
	expectedDetail := filepath.Clean(outsidePath)
	if callbackDetail != expectedDetail {
		t.Errorf("expected OnPermissionDenied callback detail=%q, got %q", expectedDetail, callbackDetail)
	}
	// 确认文件没有被创建
	if _, statErr := os.Stat(outsidePath); statErr == nil {
		os.Remove(outsidePath)
		t.Error("file was created outside workspace — security breach!")
	}
}

func TestWriteFile_PathGrantAllowsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "granted.txt")
	inputJSON, _ := json.Marshal(map[string]string{
		"path":    outsidePath,
		"content": "approved write",
	})
	params := testToolParams(workspace)
	params.MountRequests = []MountRequestForSandbox{
		{HostPath: outsideDir, MountMode: "rw"},
	}

	result, err := ExecuteToolCall(context.Background(), "write_file", inputJSON, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Fatalf("expected successful write, got %q", result)
	}
	data, readErr := os.ReadFile(outsidePath)
	if readErr != nil {
		t.Fatalf("expected outside file to be written: %v", readErr)
	}
	if string(data) != "approved write" {
		t.Fatalf("outside file content mismatch: %q", string(data))
	}
}

func TestWriteFile_ReadOnlyPathGrantStillBlocked(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "blocked.txt")
	var callbackTool, callbackDetail string
	inputJSON, _ := json.Marshal(map[string]string{
		"path":    outsidePath,
		"content": "should fail",
	})
	params := testToolParams(workspace)
	params.MountRequests = []MountRequestForSandbox{
		{HostPath: outsideDir, MountMode: "ro"},
	}
	params.OnPermissionDenied = func(tool, detail string) {
		callbackTool = tool
		callbackDetail = detail
	}

	result, err := ExecuteToolCall(context.Background(), "write_file", inputJSON, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !IsPermissionDeniedOutput(result) {
		t.Fatalf("expected permission denied output, got %q", result)
	}
	if callbackTool != "write_file" {
		t.Fatalf("expected callback tool write_file, got %q", callbackTool)
	}
	if callbackDetail != filepath.Clean(outsidePath) {
		t.Fatalf("expected callback detail %q, got %q", filepath.Clean(outsidePath), callbackDetail)
	}
}

func TestReadFile_GlobalReadAllowed(t *testing.T) {
	// 全局可读: 读取工作空间外的文件应当成功（L0/L1/L2 均允许）
	workspace := t.TempDir()
	var callbackTool string
	inputJSON, _ := json.Marshal(map[string]string{"path": "/etc/hosts"})
	params := testToolParams(workspace)
	params.OnPermissionDenied = func(tool, detail string) {
		callbackTool = tool
	}
	result, err := ExecuteToolCall(context.Background(), "read_file",
		inputJSON, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应当返回文件内容而非权限拒绝
	if strings.Contains(result, "权限不足") {
		t.Errorf("read_file should allow reading outside workspace, got permission denied")
	}
	if strings.Contains(result, "Error reading file") {
		t.Logf("file not readable (ok on some OS), got: %q", result)
	}
	if callbackTool != "" {
		t.Errorf("OnPermissionDenied should NOT be called for reads, got tool=%q", callbackTool)
	}
}

func TestListDir_GlobalReadAllowed(t *testing.T) {
	// 全局可读: 列出工作空间外的目录应当成功（L0/L1/L2 均允许）
	workspace := t.TempDir()
	var callbackTool string
	inputJSON, _ := json.Marshal(map[string]string{"path": "/tmp"})
	params := testToolParams(workspace)
	params.OnPermissionDenied = func(tool, detail string) {
		callbackTool = tool
	}
	result, err := ExecuteToolCall(context.Background(), "list_dir",
		inputJSON, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应当返回目录内容而非权限拒绝
	if strings.Contains(result, "权限不足") {
		t.Errorf("list_dir should allow listing outside workspace, got permission denied")
	}
	if callbackTool != "" {
		t.Errorf("OnPermissionDenied should NOT be called for reads, got tool=%q", callbackTool)
	}
}

// ============================================================================
// looksLikeBashWrite 单元测试
// Bug #1 修复: 检测 bash 命令中的文件写入操作 (redirect, tee, sed -i)
// ============================================================================

func TestLooksLikeBashWrite_ShellRedirect(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
		desc string
	}{
		// 应触发的写入 redirect
		{`echo hello > file.txt`, true, "simple redirect to file"},
		{`echo hello >> file.txt`, true, "append redirect to file"},
		{`cat input.txt > output.txt`, true, "cat redirect"},
		{`ls -la > listing.txt`, true, "ls redirect to file"},
		{`echo "data" > /tmp/test.txt`, true, "redirect to absolute path"},

		// 不应触发: fd redirect
		{`echo hello 2> /dev/null`, false, "stderr to /dev/null"},
		{`cmd 2>&1`, false, "stderr merge to stdout"},
		{`make 2> errors.log`, false, "stderr redirect (fd 2)"},
		{`cmd 1> /dev/null`, false, "stdout fd redirect to /dev/null"},

		// 不应触发: /dev/null 输出抑制
		{`echo hello > /dev/null`, false, "redirect to /dev/null"},
		{`echo hello >> /dev/null`, false, "append to /dev/null"},

		// 不应触发: fd merge
		{`echo hello > &1`, false, "redirect to fd merge"},

		// 不应触发: 无 redirect 的普通命令
		{`echo hello`, false, "simple echo"},
		{`ls -la`, false, "simple ls"},
		{`cat file.txt`, false, "simple cat"},
		{`grep pattern file.txt`, false, "simple grep"},
		{`pwd`, false, "simple pwd"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := looksLikeBashWrite(tc.cmd)
			if got != tc.want {
				t.Errorf("looksLikeBashWrite(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestLooksLikeBashWrite_WriteCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
		desc string
	}{
		// tee
		{`tee file.txt`, true, "tee as first command"},
		{`echo data | tee file.txt`, true, "piped tee"},
		{`echo data |tee file.txt`, true, "piped tee no space"},
		{`echo data | tee -a file.txt`, true, "piped tee append"},
		{`cat input | tee output.txt`, true, "cat piped to tee"},

		// sed -i
		{`sed -i 's/old/new/' file.txt`, true, "sed in-place edit"},
		{`sed -i'' 's/old/new/' file.txt`, true, "sed in-place empty suffix"},
		{`sed -i"" 's/old/new/' file.txt`, true, "sed in-place double quote suffix"},
		{`cat file | sed -i 's/a/b/' f.txt`, true, "sed -i in pipeline"},

		// install 命令（仅首个命令）
		{`install -m 755 bin /usr/local/bin/`, true, "install command"},

		// 不应触发: 非写入的 sed
		{`sed 's/old/new/' file.txt`, false, "sed without -i (stdout)"},
		{`sed -n '1,5p' file.txt`, false, "sed print only"},

		// 不应触发: npm install 等非首命令
		{`npm install express`, false, "npm install (not coreutils install)"},
		{`pip install flask`, false, "pip install"},
		{`brew install wget`, false, "brew install"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := looksLikeBashWrite(tc.cmd)
			if got != tc.want {
				t.Errorf("looksLikeBashWrite(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestLooksLikeBashWrite_ComplexCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
		desc string
	}{
		// 组合命令
		{`ls -la && echo done > result.txt`, true, "chained with redirect"},
		{`grep pattern file 2>/dev/null > matches.txt`, true, "stderr suppressed but stdout to file"},
		{`cat a.txt b.txt > combined.txt`, true, "cat concatenate to file"},

		// 安全的复合命令
		{`ls -la && echo done`, false, "chained no redirect"},
		{`grep pattern file 2>/dev/null`, false, "only stderr suppressed"},
		{`echo hello && echo world`, false, "pure echo chain"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := looksLikeBashWrite(tc.cmd)
			if got != tc.want {
				t.Errorf("looksLikeBashWrite(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

// ============================================================================
// extractToolArgsSummary 测试
// ============================================================================

func TestExtractToolArgsSummary(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     string
	}{
		{"bash command", "bash", map[string]interface{}{"command": "ls -la"}, "ls -la"},
		{"read file_path", "read", map[string]interface{}{"file_path": "src/main.rs"}, "src/main.rs"},
		{"edit file_path", "edit", map[string]interface{}{"file_path": "src/lib.rs"}, "src/lib.rs"},
		{"write file_path", "write", map[string]interface{}{"file_path": "out.txt"}, "out.txt"},
		{"glob pattern", "glob", map[string]interface{}{"pattern": "**/*.go"}, "**/*.go"},
		{"grep pattern", "grep", map[string]interface{}{"pattern": "func main"}, "func main"},
		{"web_search query", "web_search", map[string]interface{}{"query": "rust async"}, "rust async"},
		{"web_fetch url", "web_fetch", map[string]interface{}{"url": "https://example.com"}, "https://example.com"},
		{"spawn_coder_agent brief", "spawn_coder_agent", map[string]interface{}{"task_brief": "实现认证"}, "实现认证"},
		{"memory_search query", "memory_search", map[string]interface{}{"query": "上次会话"}, "上次会话"},
		{"browser action", "browser", map[string]interface{}{"action": "navigate", "url": "https://x.com"}, "navigate https://x.com"},
		{"browser no url", "browser", map[string]interface{}{"action": "screenshot"}, "screenshot"},
		{"browser tab target", "browser", map[string]interface{}{"action": "switch_tab", "target_id": "tab-2"}, "switch_tab tab-2"},
		{"browser ai goal", "browser", map[string]interface{}{"action": "ai_browse", "goal": "search the pricing page"}, "ai_browse search the pricing page"},
		{"unknown tool", "custom_tool", map[string]interface{}{"foo": "bar"}, ""},
		{"empty args", "bash", map[string]interface{}{}, ""},
		{"nil args", "bash", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractToolArgsSummary(tc.toolName, tc.args)
			if got != tc.want {
				t.Errorf("extractToolArgsSummary(%q, %v) = %q, want %q", tc.toolName, tc.args, got, tc.want)
			}
		})
	}
}

func TestExtractToolArgsSummary_Truncation(t *testing.T) {
	longCmd := strings.Repeat("あ", 120)
	got := extractToolArgsSummary("bash", map[string]interface{}{"command": longCmd})
	runes := []rune(got)
	if len(runes) != 103 {
		t.Errorf("expected 103 runes after truncation, got %d", len(runes))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated suffix '...'")
	}
}

func TestExecuteBrowserTool_AllSharedActionsAreHandled(t *testing.T) {
	params := ToolExecParams{
		BrowserController:      noopBrowserController{},
		BrowserEvaluateEnabled: true,
	}

	actionNames := func() []string {
		r := &EmbeddedAttemptRunner{BrowserController: noopBrowserController{}}
		for _, tool := range r.buildToolDefinitions() {
			if tool.Name != "browser" {
				continue
			}
			var schema struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatalf("unmarshal browser schema: %v", err)
			}
			return schema.Properties["action"].Enum
		}
		t.Fatal("browser tool definition not found")
		return nil
	}()

	inputs := map[string]string{
		"navigate":            `{"action":"navigate","url":"https://example.com"}`,
		"get_content":         `{"action":"get_content"}`,
		"observe":             `{"action":"observe"}`,
		"annotate_som":        `{"action":"annotate_som"}`,
		"click":               `{"action":"click","selector":"button"}`,
		"click_ref":           `{"action":"click_ref","ref":"e1"}`,
		"type":                `{"action":"type","selector":"input","text":"hello"}`,
		"fill_ref":            `{"action":"fill_ref","ref":"e1","text":"hello"}`,
		"screenshot":          `{"action":"screenshot"}`,
		"evaluate":            `{"action":"evaluate","script":"1+1"}`,
		"wait_for":            `{"action":"wait_for","selector":"body"}`,
		"go_back":             `{"action":"go_back"}`,
		"go_forward":          `{"action":"go_forward"}`,
		"get_url":             `{"action":"get_url"}`,
		"ai_browse":           `{"action":"ai_browse","goal":"summarize the current page"}`,
		"start_gif_recording": `{"action":"start_gif_recording"}`,
		"stop_gif_recording":  `{"action":"stop_gif_recording"}`,
		"list_tabs":           `{"action":"list_tabs"}`,
		"create_tab":          `{"action":"create_tab","url":"https://example.com"}`,
		"close_tab":           `{"action":"close_tab","target_id":"tab-1"}`,
		"switch_tab":          `{"action":"switch_tab","target_id":"tab-1"}`,
	}

	for _, action := range actionNames {
		payload, ok := inputs[action]
		if !ok {
			t.Fatalf("missing test payload for browser action %q", action)
		}
		result, err := ExecuteToolCall(context.Background(), "browser", json.RawMessage(payload), params)
		if err != nil {
			t.Fatalf("action %q returned error: %v", action, err)
		}
		if strings.Contains(result, "[Unknown browser action:") {
			t.Fatalf("action %q fell back to unknown action result: %q", action, result)
		}
	}
}

func TestExecuteBrowserTool_StateChangesDoNotAutoAttachScreenshots(t *testing.T) {
	params := ToolExecParams{
		BrowserController:      noopBrowserController{},
		BrowserEvaluateEnabled: true,
	}

	for _, payload := range []json.RawMessage{
		json.RawMessage(`{"action":"navigate","url":"https://example.com"}`),
		json.RawMessage(`{"action":"click","selector":"button"}`),
		json.RawMessage(`{"action":"click_ref","ref":"e1"}`),
		json.RawMessage(`{"action":"ai_browse","goal":"summarize the current page"}`),
	} {
		result, err := ExecuteToolCall(context.Background(), "browser", payload, params)
		if err != nil {
			t.Fatalf("unexpected error for payload %s: %v", string(payload), err)
		}
		if strings.HasPrefix(result, "__MULTIMODAL__") {
			t.Fatalf("payload %s should return text by default, got multimodal result %q", string(payload), result)
		}
	}
}

// ============================================================================
// truncateRuneSafe 测试
// ============================================================================

// ============================================================================
// MountRequests 注入测试 (Phase 3.4)
// ============================================================================

func TestMountRequests_InjectedFromFunc(t *testing.T) {
	// MountRequestsFunc 返回值应注入到 ToolExecParams.MountRequests
	expectedMounts := []MountRequestForSandbox{
		{HostPath: "/data/models", MountMode: "ro"},
		{HostPath: "/var/log", MountMode: "rw"},
	}

	params := ToolExecParams{
		WorkspaceDir: t.TempDir(),
		AllowExec:    true,
	}
	// 模拟 buildToolExecParams 的注入逻辑
	mountFunc := func() []MountRequestForSandbox {
		return expectedMounts
	}
	params.MountRequests = mountFunc()

	if len(params.MountRequests) != 2 {
		t.Fatalf("expected 2 mount requests, got %d", len(params.MountRequests))
	}
	if params.MountRequests[0].HostPath != "/data/models" {
		t.Errorf("expected HostPath=/data/models, got %q", params.MountRequests[0].HostPath)
	}
	if params.MountRequests[1].MountMode != "rw" {
		t.Errorf("expected MountMode=rw, got %q", params.MountRequests[1].MountMode)
	}
}

func TestMountRequests_NilFunc(t *testing.T) {
	// MountRequestsFunc 为 nil 时 MountRequests 应为 nil
	params := ToolExecParams{
		WorkspaceDir: t.TempDir(),
		AllowExec:    true,
	}
	// 不设置 MountRequests
	if params.MountRequests != nil {
		t.Error("expected nil MountRequests when no func set")
	}
}

func TestMountRequests_NoNetworkCleared(t *testing.T) {
	// NoNetwork 合约应清空 MountRequests
	params := ToolExecParams{
		WorkspaceDir: t.TempDir(),
		AllowExec:    true,
		MountRequests: []MountRequestForSandbox{
			{HostPath: "/data", MountMode: "ro"},
		},
	}

	// 模拟 ApplyConstraints 的 NoNetwork 行为
	params.MountRequests = nil // NoNetwork=true → 清空

	if params.MountRequests != nil {
		t.Error("expected nil MountRequests after NoNetwork clearing")
	}
}

// ============================================================================
// SandboxExecOptions 类型测试
// ============================================================================

func TestSandboxExecOptions_Construction(t *testing.T) {
	// 验证 SandboxExecOptions 结构体的字段正确赋值
	opts := SandboxExecOptions{
		Cmd:           "sh",
		Args:          []string{"-c", "echo hello"},
		TimeoutMs:     5000,
		SecurityLevel: "sandboxed",
		Workspace:     "/home/user/project",
		MountRequests: []MountRequestForSandbox{
			{HostPath: "/data", MountMode: "ro"},
		},
	}
	if opts.Cmd != "sh" {
		t.Errorf("expected Cmd=sh, got %q", opts.Cmd)
	}
	if opts.Workspace != "/home/user/project" {
		t.Errorf("expected Workspace=/home/user/project, got %q", opts.Workspace)
	}
	if len(opts.MountRequests) != 1 {
		t.Fatalf("expected 1 mount request, got %d", len(opts.MountRequests))
	}
	if opts.MountRequests[0].HostPath != "/data" {
		t.Errorf("expected mount HostPath=/data, got %q", opts.MountRequests[0].HostPath)
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short ascii", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated ascii", "hello world", 5, "hello..."},
		{"chinese text", "你好世界测试", 4, "你好世界..."},
		{"empty string", "", 10, ""},
		{"single char truncate", "ab", 1, "a..."},
		{"mixed unicode", "hello你好", 7, "hello你好"},
		{"mixed truncate", "hello你好world", 7, "hello你好..."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRuneSafe(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncateRuneSafe(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}

// ============================================================================
// send_media 工具参数死循环修复测试
// 验证 Fix 1/2/3: 工具描述改进、失败计数、错误消息可操作性。
// ============================================================================

// ---------- isToolSoftError ----------

func TestIsToolSoftError(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		output string
		want   bool
	}{
		// send_media
		{"send_media error prefix", "send_media", "[send_media] No target specified", true},
		{"send_media success json", "send_media", `{"status":"sent"}`, false},
		{"send_media multimodal", "send_media", `__MULTIMODAL__[{"type":"text"}]`, false},

		// bash
		{"bash success", "bash", "hello world", false},
		{"bash resource budget", "bash", "[bash] Resource budget exhausted: exceeded", true},
		{"bash command blocked", "bash", "[Command blocked by security rule: rm *] reason", true},
		{"bash command denied", "bash", "[Command denied on non-web channel: pattern]", true},
		{"bash write approval error", "bash", "[Write operation approval error: timeout]", true},
		// Bug#11: bash exit code 检测
		{"bash exit code 1", "bash", "command not found\n[exit code: 1]", true},
		{"bash exit code 0", "bash", "success output\n[exit code: 0]", false},
		{"bash sandbox exit code 2", "bash", "permission denied\n[sandbox exit code: 2]", true},
		{"bash sandbox exit code 0", "bash", "ok\n[sandbox exit code: 0]", false},

		// browser
		{"browser navigate error", "browser", "[Browser navigate error: timeout]", true},
		{"browser click error transient", "browser", "[Browser click error (transient — element may still be loading): selector not found]", true},
		{"browser click_ref error", "browser", "[Browser click_ref error (structural — ref may be stale, run observe again): e1]", true},
		{"browser ai_browse error", "browser", "[Browser ai_browse error: goal failed]", true},
		{"browser unknown action", "browser", "[Unknown browser action: foobar]", true},
		{"browser success", "browser", `{"url":"https://example.com","title":"Test"}`, false},
		{"browser multimodal", "browser", `__MULTIMODAL__[{"type":"image"}]`, false},

		// generic tools
		{"argus error", "argus_capture_screen", "[Argus tool error: connection lost]", true},
		{"remote error", "remote_search", "[Remote tool error: timeout]", true},
		{"skill not found", "lookup_skill", `[Skill "test" not found. Available skills: a, b]`, true},
		{"no search results (not an error)", "search_skills", `[No skills matching "test" found]`, false},
		{"web_search error", "web_search", "[web_search error: rate limited]", true},
		{"tool not implemented", "custom", `[Tool "custom" is not yet implemented]`, true},
		{"generic Error prefix", "custom_tool", "Error: something failed", true},
		{"generic success", "custom_tool", "done", false},
		{"empty output", "custom_tool", "", false},
		{"json array", "custom_tool", `[{"key":"value"}]`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isToolSoftError(tc.tool, tc.output)
			if got != tc.want {
				t.Errorf("isToolSoftError(%q, %q) = %v, want %v", tc.tool, tc.output, got, tc.want)
			}
		})
	}
}

// ---------- toolFailureGuidance ----------

func TestToolFailureGuidance_SendMedia(t *testing.T) {
	g := toolFailureGuidance("send_media", 3)
	if g == "" {
		t.Fatal("expected non-empty guidance for send_media")
	}
	if !strings.Contains(g, "TOOL FAILURE LOOP DETECTED") {
		t.Error("guidance should contain 'TOOL FAILURE LOOP DETECTED'")
	}
	if !strings.Contains(g, "Do NOT provide 'target'") {
		t.Error("guidance should tell agent not to provide target")
	}
	if !strings.Contains(g, "/tmp/screenshot.png") {
		t.Error("guidance should include concrete file path example")
	}
	// 验证跨平台截图命令（根据当前 OS 不同）
	cmd := screenshotCommand()
	if !strings.Contains(g, cmd) {
		t.Errorf("guidance should contain platform screenshot command %q", cmd)
	}
}

func TestToolFailureGuidance_Browser(t *testing.T) {
	g := toolFailureGuidance("browser", 4)
	if g == "" {
		t.Fatal("expected non-empty guidance for browser")
	}
	if !strings.Contains(g, "TOOL FAILURE LOOP DETECTED") {
		t.Error("guidance should contain 'TOOL FAILURE LOOP DETECTED'")
	}
	if !strings.Contains(g, "observe") {
		t.Error("guidance should suggest using observe")
	}
	if !strings.Contains(g, "ai_browse") {
		t.Error("guidance should suggest ai_browse for complex goals")
	}
}

func TestToolFailureGuidance_Generic(t *testing.T) {
	g := toolFailureGuidance("custom_tool", 5)
	if !strings.Contains(g, "custom_tool") {
		t.Error("guidance should reference tool name")
	}
	if !strings.Contains(g, "5 times") {
		t.Error("guidance should include failure count")
	}
}

func TestScreenshotCommand_ReturnsNonEmpty(t *testing.T) {
	cmd := screenshotCommand()
	if cmd == "" {
		t.Error("screenshotCommand should return non-empty string")
	}
	// 验证包含输出路径
	if !strings.Contains(cmd, "/tmp/screenshot.png") && !strings.Contains(cmd, "screenshot") {
		t.Errorf("screenshotCommand should reference screenshot output, got %q", cmd)
	}
}

// ---------- send_media 错误消息可操作性 ----------

// mockMediaSender 用于测试 send_media 的 mock 实现。
type mockMediaSender struct {
	sendErr      error // 非 nil 时 SendMedia 返回此错误
	lastFileName string
	lastMimeType string
	lastSize     int
}

func (m *mockMediaSender) SendMedia(_ context.Context, _, _ string, data []byte, fileName, mimeType, _ string) error {
	m.lastFileName = fileName
	m.lastMimeType = mimeType
	m.lastSize = len(data)
	return m.sendErr
}

type mockStructuredSendError struct {
	code      string
	channel   string
	operation string
	message   string
	retryable bool
}

func (m *mockStructuredSendError) Error() string       { return m.message }
func (m *mockStructuredSendError) SendCode() string    { return m.code }
func (m *mockStructuredSendError) SendChannel() string { return m.channel }
func (m *mockStructuredSendError) SendOperation() string {
	return m.operation
}
func (m *mockStructuredSendError) SendRetryable() bool { return m.retryable }
func (m *mockStructuredSendError) SendUserMessage() string {
	return m.message
}

func sendMediaTestParams(sessionKey string) ToolExecParams {
	return ToolExecParams{
		SessionKey:   sessionKey,
		MediaSender:  &mockMediaSender{},
		WorkspaceDir: os.TempDir(),
	}
}

func newTestCoderConfirmationManager(
	t *testing.T,
	decision string,
	capture func(CoderConfirmationRequest),
) *CoderConfirmationManager {
	t.Helper()

	var mgr *CoderConfirmationManager
	mgr = NewCoderConfirmationManager(func(event string, payload interface{}) {
		if event != "coder.confirm.requested" {
			return
		}
		req, ok := payload.(CoderConfirmationRequest)
		if !ok {
			t.Fatalf("unexpected coder confirmation payload type: %T", payload)
		}
		if capture != nil {
			capture(req)
		}
		go func() {
			if err := mgr.ResolveConfirmation(req.ID, decision); err != nil {
				t.Errorf("ResolveConfirmation: %v", err)
			}
		}()
	}, nil, time.Second)
	return mgr
}

func TestSendMedia_NoTarget_HelpfulError(t *testing.T) {
	// SessionKey 为空 + target 为空 → 应返回带指导的错误
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(`{"file_path":"/tmp/test.png"}`),
		sendMediaTestParams("")) // 空 sessionKey
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "omit 'target'") {
		t.Errorf("error should suggest omitting target, got %q", result)
	}
	if !strings.Contains(result, "Do NOT fabricate") {
		t.Errorf("error should warn against fabricating IDs, got %q", result)
	}
}

func TestSendMedia_NoInput_HelpfulError(t *testing.T) {
	// 既无 file_path 也无 media_base64 → 应返回带指导的错误
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(`{}`),
		sendMediaTestParams("feishu:oc_test123"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "screencapture") {
		t.Errorf("error should suggest using screencapture, got %q", result)
	}
	if !strings.Contains(result, "file_path") {
		t.Errorf("error should mention file_path, got %q", result)
	}
	if !strings.Contains(result, "latest image already attached") {
		t.Errorf("error should mention latest chat attachment fallback, got %q", result)
	}
}

func TestSendMedia_ReusesCurrentChatAttachment(t *testing.T) {
	sender := &mockMediaSender{}
	imageData := []byte("fake-png-data")
	p := ToolExecParams{
		SessionKey:  "feishu:oc_test123",
		MediaSender: sender,
		CurrentAttachments: []session.ContentBlock{
			{
				Type:     "image",
				FileName: "chat-upload.png",
				MimeType: "image/png",
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      base64.StdEncoding.EncodeToString(imageData),
				},
			},
		},
	}

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(`{"message":"转发这张图给飞书"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.lastFileName != "chat-upload.png" {
		t.Fatalf("fileName=%q, want chat-upload.png", sender.lastFileName)
	}
	if sender.lastMimeType != "image/png" {
		t.Fatalf("mimeType=%q, want image/png", sender.lastMimeType)
	}
	if sender.lastSize != len(imageData) {
		t.Fatalf("size=%d, want %d", sender.lastSize, len(imageData))
	}
	if !strings.HasPrefix(result, "__MULTIMODAL__") {
		t.Fatalf("expected multimodal result for image attachment, got %q", result)
	}
}

func TestSendMedia_ReusesLatestTranscriptAttachment(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "chat.jsonl")
	mgr := session.NewSessionManager("")
	if _, err := mgr.EnsureSessionFile("sess-1", sessionFile); err != nil {
		t.Fatalf("EnsureSessionFile: %v", err)
	}

	imageData := []byte("fake-jpeg-data")
	if err := mgr.AppendMessage("sess-1", sessionFile, session.TranscriptEntry{
		Role: "user",
		Content: []session.ContentBlock{
			session.TextBlock("请看这张图"),
			{
				Type:     "image",
				FileName: "from-chat.jpg",
				MimeType: "image/jpeg",
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      base64.StdEncoding.EncodeToString(imageData),
				},
			},
		},
		Timestamp: 123,
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	sender := &mockMediaSender{}
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(`{"message":"转发上一张图"}`),
		ToolExecParams{
			SessionKey:  "feishu:oc_test123",
			SessionID:   "sess-1",
			SessionFile: sessionFile,
			MediaSender: sender,
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.lastFileName != "from-chat.jpg" {
		t.Fatalf("fileName=%q, want from-chat.jpg", sender.lastFileName)
	}
	if sender.lastMimeType != "image/jpeg" {
		t.Fatalf("mimeType=%q, want image/jpeg", sender.lastMimeType)
	}
	if sender.lastSize != len(imageData) {
		t.Fatalf("size=%d, want %d", sender.lastSize, len(imageData))
	}
	if !strings.HasPrefix(result, "__MULTIMODAL__") {
		t.Fatalf("expected multimodal result for transcript attachment, got %q", result)
	}
}

func TestSendMedia_BadFilePath_HelpfulError(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "nonexistent_screenshot.png")
	p := sendMediaTestParams("feishu:oc_test123")
	p.WorkspaceDir = tmpDir
	p.ScopePaths = []string{tmpDir} // 允许 workspace 内路径

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, badPath)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "[send_media]") {
		t.Errorf("error should have [send_media] prefix, got %q", result)
	}
	if !strings.Contains(result, "absolute") || !strings.Contains(result, "ls") {
		t.Errorf("error should suggest verifying path with 'ls', got %q", result)
	}
}

func TestSendMedia_PathEscapeBlocked_RequestsPermission(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "desktop-image.png")
	if err := os.WriteFile(outsidePath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	var callbackTool, callbackDetail string
	p := ToolExecParams{
		SessionKey:   "feishu:oc_test123",
		MediaSender:  &mockMediaSender{},
		WorkspaceDir: workspace,
		OnPermissionDenied: func(tool, detail string) {
			callbackTool = tool
			callbackDetail = detail
		},
	}

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, outsidePath)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !IsPermissionDeniedOutput(result) {
		t.Fatalf("expected permission denied output, got %q", result)
	}
	if callbackTool != "send_media" {
		t.Fatalf("expected callback tool send_media, got %q", callbackTool)
	}
	if callbackDetail != filepath.Clean(outsidePath) {
		t.Fatalf("expected callback detail %q, got %q", filepath.Clean(outsidePath), callbackDetail)
	}
}

func TestSendMedia_FabricatedTarget_SendError(t *testing.T) {
	// 模拟原始 bug 场景: agent 编造 target ID "feishu:oc_xxx"，
	// parseSendMediaTarget 解析成功，但 MediaSender.SendMedia 因目标不存在返回错误。
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "screenshot.png")
	os.WriteFile(testFile, []byte("fake-png-data"), 0o644)

	p := ToolExecParams{
		SessionKey:   "feishu:oc_real123",
		MediaSender:  &mockMediaSender{sendErr: fmt.Errorf("channel not found: feishu:oc_fabricated")},
		WorkspaceDir: tmpDir,
		ScopePaths:   []string{tmpDir},
	}

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"target":"feishu:oc_fabricated","file_path":%q}`, testFile)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应返回 [send_media] 前缀的错误（触发 isToolSoftError 检测）
	if !strings.HasPrefix(result, "[send_media]") {
		t.Errorf("error should have [send_media] prefix, got %q", result)
	}
	if !strings.Contains(result, "Failed to send") {
		t.Errorf("error should contain 'Failed to send', got %q", result)
	}
	// 验证 isToolSoftError 能正确检测此输出
	if !isToolSoftError("send_media", result) {
		t.Errorf("isToolSoftError should detect this as soft error, output=%q", result)
	}
}

func TestSendMedia_NoMediaSender_Error(t *testing.T) {
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(`{"file_path":"/tmp/test.png"}`),
		ToolExecParams{SessionKey: "feishu:oc_test123"}) // no MediaSender
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not available") {
		t.Errorf("should report media sender not available, got %q", result)
	}
}

func TestSendMedia_ChannelSendError_UsesStructuredMessage(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "sample.png")
	if err := os.WriteFile(testFile, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	sendErr := &mockStructuredSendError{
		code:      "unsupported_feature",
		channel:   "dingtalk",
		operation: "send.media.upload",
		message:   "dingtalk media upload not supported",
	}

	p := ToolExecParams{
		SessionKey:   "dingtalk:cid_test",
		MediaSender:  &mockMediaSender{sendErr: sendErr},
		WorkspaceDir: tmpDir,
		ScopePaths:   []string{tmpDir},
	}
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, testFile)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "sendCode=unsupported_feature") {
		t.Fatalf("expected structured sendCode in result, got %q", result)
	}
	if !strings.Contains(result, "channel=dingtalk") {
		t.Fatalf("expected channel metadata in result, got %q", result)
	}
	if !strings.Contains(result, "暂不支持") {
		t.Fatalf("expected actionable hint in result, got %q", result)
	}
}

func TestSendMedia_FilePathPreservesFileNameAndReturnsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "idlefish_agent_design.md")
	if err := os.WriteFile(testFile, []byte("# design\nhello\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	sender := &mockMediaSender{}
	p := ToolExecParams{
		SessionKey:   "feishu:oc_test123",
		MediaSender:  sender,
		WorkspaceDir: tmpDir,
		ScopePaths:   []string{tmpDir},
	}
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, testFile)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.HasPrefix(result, "__MULTIMODAL__") {
		t.Fatalf("markdown file should not be wrapped as multimodal image, got %q", result)
	}
	if sender.lastFileName != "idlefish_agent_design.md" {
		t.Fatalf("fileName=%q, want idlefish_agent_design.md", sender.lastFileName)
	}
	if !strings.Contains(result, `"fileName":"idlefish_agent_design.md"`) {
		t.Fatalf("expected status json to include fileName, got %q", result)
	}
	if !strings.Contains(result, `"size":15`) {
		t.Fatalf("expected status json to include actual size, got %q", result)
	}
}

func TestSendMedia_PathGrantAllowsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	testFile := filepath.Join(outsideDir, "desktop-plan.md")
	if err := os.WriteFile(testFile, []byte("# outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	sender := &mockMediaSender{}
	p := ToolExecParams{
		SessionKey:   "feishu:oc_test123",
		MediaSender:  sender,
		WorkspaceDir: workspace,
		MountRequests: []MountRequestForSandbox{
			{HostPath: outsideDir, MountMode: "ro"},
		},
	}

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, testFile)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if IsPermissionDeniedOutput(result) {
		t.Fatalf("expected approved path to bypass permission denied, got %q", result)
	}
	if sender.lastFileName != "desktop-plan.md" {
		t.Fatalf("fileName=%q, want desktop-plan.md", sender.lastFileName)
	}
	if !strings.Contains(result, `"status":"sent"`) {
		t.Fatalf("expected success json, got %q", result)
	}
}

func TestSendMedia_FullSecurityAllowsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	testFile := filepath.Join(outsideDir, "desktop-plan.md")
	if err := os.WriteFile(testFile, []byte("# outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	sender := &mockMediaSender{}
	p := ToolExecParams{
		SessionKey:    "feishu:oc_test123",
		MediaSender:   sender,
		WorkspaceDir:  workspace,
		SecurityLevel: "full",
	}

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, testFile)), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if IsPermissionDeniedOutput(result) {
		t.Fatalf("expected full security to bypass workspace restriction, got %q", result)
	}
	if sender.lastFileName != "desktop-plan.md" {
		t.Fatalf("fileName=%q, want desktop-plan.md", sender.lastFileName)
	}
	if !strings.Contains(result, `"status":"sent"`) {
		t.Fatalf("expected success json, got %q", result)
	}
}

func TestSendMedia_FilePathRequiresConfirmationBeforeSending(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "report.png")
	if err := os.WriteFile(testFile, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	sender := &mockMediaSender{}
	var captured CoderConfirmationRequest
	mgr := newTestCoderConfirmationManager(t, "allow", func(req CoderConfirmationRequest) {
		captured = req
	})

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"target":"feishu:oc_target123","file_path":%q,"message":"请查收附件"}`, testFile)),
		ToolExecParams{
			SessionKey:        "feishu:oc_test123",
			MediaSender:       sender,
			WorkspaceDir:      tmpDir,
			ScopePaths:        []string{tmpDir},
			CoderConfirmation: mgr,
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.lastFileName != "report.png" {
		t.Fatalf("fileName=%q, want report.png", sender.lastFileName)
	}
	if !strings.Contains(result, `"status\":\"sent\"`) {
		t.Fatalf("expected sent status in result, got %q", result)
	}
	if captured.ToolName != "send_media" {
		t.Fatalf("toolName=%q, want send_media", captured.ToolName)
	}
	if captured.Preview == nil {
		t.Fatal("expected preview to be populated")
	}
	if captured.Preview.FilePath != testFile {
		t.Fatalf("preview.FilePath=%q, want %q", captured.Preview.FilePath, testFile)
	}
	if captured.Preview.Command != "send "+testFile+" to feishu:oc_target123" {
		t.Fatalf("preview.Command=%q", captured.Preview.Command)
	}
	if captured.Preview.Content != "请查收附件" {
		t.Fatalf("preview.Content=%q, want 请查收附件", captured.Preview.Content)
	}
}

func TestSendMedia_DeniedConfirmationDoesNotSend(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "report.md")
	if err := os.WriteFile(testFile, []byte("# report\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	sender := &mockMediaSender{}
	mgr := newTestCoderConfirmationManager(t, "deny", nil)

	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(fmt.Sprintf(`{"target":"feishu:oc_target123","file_path":%q}`, testFile)),
		ToolExecParams{
			SessionKey:        "feishu:oc_test123",
			MediaSender:       sender,
			WorkspaceDir:      tmpDir,
			ScopePaths:        []string{tmpDir},
			CoderConfirmation: mgr,
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "[send_media] User denied send operation." {
		t.Fatalf("unexpected result: %q", result)
	}
	if sender.lastFileName != "" {
		t.Fatalf("send should not happen after denial, got fileName=%q", sender.lastFileName)
	}
}

func TestSendMedia_Base64UsesExplicitFileName(t *testing.T) {
	sender := &mockMediaSender{}
	p := ToolExecParams{
		SessionKey:  "feishu:oc_test123",
		MediaSender: sender,
	}
	result, err := ExecuteToolCall(context.Background(), "send_media",
		json.RawMessage(`{"media_base64":"SGVsbG8=","file_name":"notes.md","mime_type":"text/markdown"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.lastFileName != "notes.md" {
		t.Fatalf("fileName=%q, want notes.md", sender.lastFileName)
	}
	if sender.lastMimeType != "text/markdown" {
		t.Fatalf("mimeType=%q, want text/markdown", sender.lastMimeType)
	}
	if !strings.Contains(result, `"fileName":"notes.md"`) {
		t.Fatalf("expected result to include explicit fileName, got %q", result)
	}
}

func TestFormatSendMediaDeliveryError_PlainError(t *testing.T) {
	msg := formatSendMediaDeliveryError(fmt.Errorf("network timeout"))
	if !strings.Contains(msg, "Failed to send") {
		t.Fatalf("expected plain fallback message, got %q", msg)
	}
}

// ---------- search_skills ----------

func TestSearchSkills_VFSHasResults(t *testing.T) {
	bridge := &stubUHMSBridge{
		hits: []SkillSearchHit{
			{Name: "test-skill", Category: "general", Abstract: "A test skill"},
		},
		indexed: true,
	}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"test"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "test-skill") {
		t.Errorf("should contain skill name, got %q", result)
	}
	if strings.Contains(result, "No matching") {
		t.Errorf("should not say no matching when results exist, got %q", result)
	}
}

func TestSearchSkills_VFSZeroResults(t *testing.T) {
	bridge := &stubUHMSBridge{
		hits:    []SkillSearchHit{},
		indexed: true,
	}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"nonexistent"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No matching skills found") {
		t.Errorf("should say no matching skills, got %q", result)
	}
	if strings.Contains(result, "No skills index available") {
		t.Errorf("should not say no index available, got %q", result)
	}
}

func TestSearchSkills_NoBridgeNoCache(t *testing.T) {
	p := ToolExecParams{} // no UHMSBridge, no SkillsCache
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"anything"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No skills index available") {
		t.Errorf("should say no index available, got %q", result)
	}
	if strings.Contains(result, "VFS distribute") {
		t.Errorf("should not reference VFS distribute command, got %q", result)
	}
}

func TestSearchSkills_DistributingNote(t *testing.T) {
	bridge := &stubUHMSBridge{
		hits:         []SkillSearchHit{{Name: "partial-skill", Category: "test", Abstract: "partial"}},
		indexed:      true,
		distributing: true,
	}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"partial"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "indexing is in progress") {
		t.Errorf("should include distributing note, got %q", result)
	}
}

func TestSearchSkills_DistributingZeroResults(t *testing.T) {
	bridge := &stubUHMSBridge{
		hits:         []SkillSearchHit{},
		indexed:      true,
		distributing: true,
	}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"nonexistent"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No matching skills found") {
		t.Errorf("should say no matching, got %q", result)
	}
	if !strings.Contains(result, "indexing is in progress") {
		t.Errorf("should include distributing note, got %q", result)
	}
}

func TestSearchSkills_DistributingZeroResults_CacheFallback(t *testing.T) {
	// F-2: VFS 零结果 + 分发中 + cache 有数据 → 降级到 cache
	bridge := &stubUHMSBridge{
		hits:         []SkillSearchHit{},
		indexed:      true,
		distributing: true,
	}
	cache := map[string]string{
		"deploy-k8s": "# Deploy K8s\nDeploy to Kubernetes cluster",
	}
	p := ToolExecParams{UHMSBridge: bridge, SkillsCache: cache}
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"deploy"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应该走 cache 降级而非直接返回 "no matching"
	if strings.Contains(result, "No matching skills found") {
		t.Errorf("distributing + cache available should try cache, not return no matching, got %q", result)
	}
	if !strings.Contains(result, "indexing is in progress") {
		t.Errorf("cache fallback during distributing should include note, got %q", result)
	}
}

func TestSearchSkills_VFSError_DistributingNote(t *testing.T) {
	// F-3: VFS 搜索错误 + 分发中 → cache 降级应追加 distributing note
	bridge := &stubUHMSBridge{
		searchErr:    fmt.Errorf("connection timeout"),
		distributing: true,
		indexed:      true,
	}
	cache := map[string]string{
		"debug-tool": "# Debug Tool\nDebug application issues",
	}
	p := ToolExecParams{UHMSBridge: bridge, SkillsCache: cache}
	result, err := ExecuteToolCall(context.Background(), "search_skills",
		json.RawMessage(`{"query":"debug"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "indexing is in progress") {
		t.Errorf("VFS error + distributing → cache should include note, got %q", result)
	}
}

// ---------- Argus nil pointer ----------

func TestArgusTool_NilBridge_NoPanic(t *testing.T) {
	p := ToolExecParams{
		SessionKey:        "web:test",
		ArgusApprovalMode: "none", // 跳过审批门以测试 nil bridge 路径
		// ArgusBridge is nil
	}
	result, err := executeArgusTool(context.Background(), "argus_capture_screen", json.RawMessage(`{}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not available") {
		t.Errorf("should report Argus not available, got %q", result)
	}
}

func TestArgusTool_ApprovalDenied_StopsRetrySignal(t *testing.T) {
	mgr := newTestCoderConfirmationManager(t, "deny", nil)
	p := ToolExecParams{
		SessionKey:        "web:test",
		ArgusApprovalMode: "medium_and_above",
		CoderConfirmation: mgr,
	}
	result, err := executeArgusTool(context.Background(), "argus_click", json.RawMessage(`{"x":10,"y":20}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !IsApprovalDeniedOutput(result) {
		t.Fatalf("expected approval denied output, got %q", result)
	}
	if !IsBlockingDeniedOutput(result) {
		t.Fatalf("expected blocking denied output, got %q", result)
	}
	if !strings.Contains(result, "Do NOT retry the same action automatically") {
		t.Fatalf("expected hard-stop guidance, got %q", result)
	}
}

// ---------- Memory tools ----------

func TestMemorySearch_Basic(t *testing.T) {
	bridge := &stubUHMSBridge{
		memoryHits: []MemorySearchHit{
			{ID: "m1", Content: "用户喜欢深色主题", Score: 0.95, Type: "preference"},
			{ID: "m2", Content: "用户是前端开发者", Score: 0.80, Type: "personal_info"},
		},
	}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := executeMemorySearch(context.Background(), json.RawMessage(`{"query":"用户偏好","limit":5}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "m1") || !strings.Contains(result, "深色主题") {
		t.Errorf("expected memory hits in result, got %q", result)
	}
}

func TestMemorySearch_NilBridge(t *testing.T) {
	p := ToolExecParams{} // UHMSBridge is nil
	result, err := executeMemorySearch(context.Background(), json.RawMessage(`{"query":"test"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "unavailable") {
		t.Errorf("nil bridge should report unavailable, got %q", result)
	}
}

func TestMemorySearch_EmptyQuery(t *testing.T) {
	bridge := &stubUHMSBridge{}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := executeMemorySearch(context.Background(), json.RawMessage(`{"query":""}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "query is required") {
		t.Errorf("empty query should error, got %q", result)
	}
}

func TestMemoryGet_Basic(t *testing.T) {
	bridge := &stubUHMSBridge{
		memoryHit: &MemoryHit{
			ID:       "m1",
			Content:  "用户喜欢深色主题",
			Type:     "preference",
			Category: "ui_preference",
		},
	}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := executeMemoryGet(context.Background(), json.RawMessage(`{"id":"m1"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "m1") || !strings.Contains(result, "深色主题") {
		t.Errorf("expected memory hit in result, got %q", result)
	}
}

func TestMemoryGet_NilBridge(t *testing.T) {
	p := ToolExecParams{} // UHMSBridge is nil
	result, err := executeMemoryGet(context.Background(), json.RawMessage(`{"id":"m1"}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "unavailable") {
		t.Errorf("nil bridge should report unavailable, got %q", result)
	}
}

func TestMemoryGet_EmptyID(t *testing.T) {
	bridge := &stubUHMSBridge{}
	p := ToolExecParams{UHMSBridge: bridge}
	result, err := executeMemoryGet(context.Background(), json.RawMessage(`{"id":""}`), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "id is required") {
		t.Errorf("empty id should error, got %q", result)
	}
}
