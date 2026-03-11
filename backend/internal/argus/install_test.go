package argus

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------- EnsureUserBinLink 测试 ----------

func TestEnsureUserBinLink_EmptyPath(t *testing.T) {
	err := EnsureUserBinLink("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestEnsureUserBinLink_NonExistent(t *testing.T) {
	err := EnsureUserBinLink("/nonexistent/argus-xyz-12345")
	if err == nil {
		t.Error("expected error for non-existent source binary")
	}
}

func TestEnsureUserBinLink_CreateAndIdempotent(t *testing.T) {
	// 创建临时源二进制
	srcDir := t.TempDir()
	srcBin := filepath.Join(srcDir, "argus-sensory")
	if err := os.WriteFile(srcBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 临时 HOME 目录
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// 第一次调用 → 应创建链接
	if err := EnsureUserBinLink(srcBin); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	linkPath := filepath.Join(tmpHome, ".openacosmi", "bin", "argus-sensory")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	absSrc, _ := filepath.Abs(srcBin)
	if target != absSrc {
		t.Errorf("expected link target %q, got %q", absSrc, target)
	}

	// 第二次调用 → 幂等（不应报错）
	if err := EnsureUserBinLink(srcBin); err != nil {
		t.Fatalf("idempotent call failed: %v", err)
	}
}

func TestEnsureUserBinLink_UpdateExisting(t *testing.T) {
	// 创建旧的和新的源二进制
	srcDir := t.TempDir()
	oldBin := filepath.Join(srcDir, "argus-old")
	newBin := filepath.Join(srcDir, "argus-new")
	os.WriteFile(oldBin, []byte("#!/bin/sh\n"), 0o755)
	os.WriteFile(newBin, []byte("#!/bin/sh\n"), 0o755)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// 创建指向旧二进制的链接
	if err := EnsureUserBinLink(oldBin); err != nil {
		t.Fatalf("old link creation failed: %v", err)
	}

	// 更新到新的二进制
	if err := EnsureUserBinLink(newBin); err != nil {
		t.Fatalf("update link failed: %v", err)
	}

	linkPath := filepath.Join(tmpHome, ".openacosmi", "bin", "argus-sensory")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	absNew, _ := filepath.Abs(newBin)
	if target != absNew {
		t.Errorf("expected updated target %q, got %q", absNew, target)
	}
}

func TestEnsureUserBinLink_RespectsStateDirOverride(t *testing.T) {
	srcDir := t.TempDir()
	srcBin := filepath.Join(srcDir, "argus-sensory")
	if err := os.WriteFile(srcBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(t.TempDir(), "custom-state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	if err := EnsureUserBinLink(srcBin); err != nil {
		t.Fatalf("EnsureUserBinLink failed: %v", err)
	}

	linkPath := filepath.Join(stateDir, "bin", "argus-sensory")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	absSrc, _ := filepath.Abs(srcBin)
	if target != absSrc {
		t.Errorf("expected link target %q, got %q", absSrc, target)
	}
}

func TestEnsureUserBinLink_SkipNonSymlink(t *testing.T) {
	// 如果 ~/.openacosmi/bin/argus-sensory 是一个真实文件（非链接），不应覆盖
	srcDir := t.TempDir()
	srcBin := filepath.Join(srcDir, "argus-sensory")
	os.WriteFile(srcBin, []byte("#!/bin/sh\n"), 0o755)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// 手动创建一个真实文件（模拟用户手动拷贝）
	binDir := filepath.Join(tmpHome, ".openacosmi", "bin")
	os.MkdirAll(binDir, 0o755)
	realFile := filepath.Join(binDir, "argus-sensory")
	os.WriteFile(realFile, []byte("#!/bin/sh\necho 'real copy'\n"), 0o755)

	// 调用应成功（不覆盖）
	if err := EnsureUserBinLink(srcBin); err != nil {
		t.Fatalf("should succeed without overwriting: %v", err)
	}

	// 验证文件内容未被更改（仍然是真实文件，不是链接）
	_, err := os.Readlink(realFile)
	if err == nil {
		t.Error("file should remain a real file, not a symlink")
	}
}

// ---------- StandardInstallPaths 测试 ----------

func TestStandardInstallPaths_NotEmpty(t *testing.T) {
	paths := StandardInstallPaths()
	if len(paths) == 0 {
		t.Error("expected non-empty standard install paths")
	}
	// 验证至少包含系统级路径
	found := false
	for _, p := range paths {
		if p == "/Applications/Argus.app/Contents/MacOS/argus-sensory" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /Applications/Argus.app path in standard paths, got %v", paths)
	}
}
