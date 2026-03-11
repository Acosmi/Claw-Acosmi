package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------- resolveArgusBinaryFull 基础测试 ----------

func TestResolveArgus_EnvOverride(t *testing.T) {
	// 设置 $ARGUS_BINARY_PATH 指向一个真实可执行文件
	tmpDir := t.TempDir()
	tmpBin := filepath.Join(tmpDir, "argus-sensory")
	if err := os.WriteFile(tmpBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGUS_BINARY_PATH", tmpBin)

	result := resolveArgusBinaryFull("")
	if result.Path != tmpBin {
		t.Errorf("expected path %q from env, got %q", tmpBin, result.Path)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}
	// trace 应有 env 层
	if len(result.Trace) == 0 || result.Trace[0].Layer != "env" || !result.Trace[0].Found {
		t.Errorf("expected trace[0] = env/found, got %+v", result.Trace)
	}
}

func TestResolveArgus_EnvInvalid(t *testing.T) {
	// 设置 $ARGUS_BINARY_PATH 指向不存在的路径
	t.Setenv("ARGUS_BINARY_PATH", "/nonexistent/argus-xyz-12345")

	result := resolveArgusBinaryFull("")
	if result.Error == nil {
		t.Fatal("expected error for invalid env path")
	}
	if result.Error.Reason != "env_path_invalid" {
		t.Errorf("expected reason env_path_invalid, got %q", result.Error.Reason)
	}
	if result.Error.Phase != "resolve" {
		t.Errorf("expected phase resolve, got %q", result.Error.Phase)
	}
}

func TestResolveArgus_ConfigPath(t *testing.T) {
	// 确保 $ARGUS_BINARY_PATH 不干扰
	t.Setenv("ARGUS_BINARY_PATH", "")

	tmpDir := t.TempDir()
	tmpBin := filepath.Join(tmpDir, "argus-sensory")
	if err := os.WriteFile(tmpBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := resolveArgusBinaryFull(tmpBin)
	if result.Path != tmpBin {
		t.Errorf("expected path %q from config, got %q", tmpBin, result.Path)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}
	// trace 应有 config 层
	found := false
	for _, tr := range result.Trace {
		if tr.Layer == "config" && tr.Found {
			found = true
		}
	}
	if !found {
		t.Errorf("expected config layer in trace, got %+v", result.Trace)
	}
}

func TestResolveArgus_ConfigPathInvalid_FallsThrough(t *testing.T) {
	// 配置路径无效时应继续搜索后续层
	t.Setenv("ARGUS_BINARY_PATH", "")

	result := resolveArgusBinaryFull("/nonexistent/argus-config-path-xyz")
	// 只要 config 层标记为 not found，应继续搜索
	configFound := false
	for _, tr := range result.Trace {
		if tr.Layer == "config" && !tr.Found {
			configFound = true
		}
	}
	if !configFound {
		t.Errorf("expected config layer marked as not-found in trace, got %+v", result.Trace)
	}
	// trace 应包含后续层（至少 app_bundle）
	hasAppBundle := false
	for _, tr := range result.Trace {
		if tr.Layer == "app_bundle" {
			hasAppBundle = true
		}
	}
	if !hasAppBundle {
		t.Errorf("expected app_bundle layer in trace after config fallthrough, got %+v", result.Trace)
	}
}

func TestResolveArgus_NoneFound(t *testing.T) {
	// 清除所有可能的路径
	t.Setenv("ARGUS_BINARY_PATH", "")
	// 修改 PATH 确保找不到 argus-sensory
	t.Setenv("PATH", t.TempDir())

	result := resolveArgusBinaryFull("")
	if result.Error == nil {
		// 在测试机上可能有实际的 argus-sensory 安装，跳过
		t.Skip("argus-sensory found on this machine, skipping not-found test")
	}
	if result.Error.Reason != "binary_not_found" {
		t.Errorf("expected reason binary_not_found, got %q", result.Error.Reason)
	}
	if result.Path != "" {
		t.Errorf("expected empty path, got %q", result.Path)
	}
}

func TestResolveArgus_TraceOutput(t *testing.T) {
	// 验证 trace 数据结构始终包含 Layer 字段
	t.Setenv("ARGUS_BINARY_PATH", "")
	t.Setenv("PATH", t.TempDir()) // 确保 PATH 搜索失败
	// mock HOME 到空目录，防止 user_bin 层发现真实安装的 argus-sensory
	t.Setenv("HOME", t.TempDir())

	result := resolveArgusBinaryFull("")
	for i, tr := range result.Trace {
		if tr.Layer == "" {
			t.Errorf("trace[%d] has empty Layer", i)
		}
	}
	// 至少应有 app_bundle、user_bin、path 三个 trace 条目
	if len(result.Trace) < 3 {
		t.Errorf("expected at least 3 trace entries, got %d: %+v", len(result.Trace), result.Trace)
	}
}

func TestResolveArgus_EnvTakesPriority(t *testing.T) {
	// 同时设置 env 和 config path，env 应优先
	tmpDir := t.TempDir()
	envBin := filepath.Join(tmpDir, "argus-env")
	configBin := filepath.Join(tmpDir, "argus-config")
	os.WriteFile(envBin, []byte("#!/bin/sh\n"), 0o755)
	os.WriteFile(configBin, []byte("#!/bin/sh\n"), 0o755)

	t.Setenv("ARGUS_BINARY_PATH", envBin)

	result := resolveArgusBinaryFull(configBin)
	if result.Path != envBin {
		t.Errorf("expected env path %q to take priority over config %q, got %q", envBin, configBin, result.Path)
	}
}

func TestResolveNativeSandboxBinaryPath_UsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	binDir := filepath.Join(stateDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(binDir, "openacosmi")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")
	t.Setenv("OA_CLI_BINARY", "")

	if got := resolveNativeSandboxBinaryPath(); got != binPath {
		t.Fatalf("resolveNativeSandboxBinaryPath() = %q, want %q", got, binPath)
	}
}
