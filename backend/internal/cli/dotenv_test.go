package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDotEnv_StateDirFallback 验证 LoadDotEnv 在不同 OPENACOSMI_STATE_DIR 设置下的行为。
// CMD-5: OPENACOSMI_STATE_DIR 多级推断路径对齐验证。
func TestLoadDotEnv_StateDirFallback(t *testing.T) {
	// 创建临时目录模拟配置
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, ".openacosmi")
	if err := os.MkdirAll(globalDir, 0700); err != nil {
		t.Fatal(err)
	}

	// 写入全局 .env
	envContent := "TEST_DOTENV_GLOBAL=hello_from_global\n"
	if err := os.WriteFile(filepath.Join(globalDir, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatal(err)
	}

	// 设置 OPENACOSMI_STATE_DIR 指向临时目录
	t.Setenv("OPENACOSMI_STATE_DIR", globalDir)
	// 确保测试变量未被残留
	os.Unsetenv("TEST_DOTENV_GLOBAL")

	LoadDotEnv(true)

	got := os.Getenv("TEST_DOTENV_GLOBAL")
	if got != "hello_from_global" {
		t.Errorf("TEST_DOTENV_GLOBAL = %q, want %q", got, "hello_from_global")
	}
}

func TestLoadDotEnv_CrabClawStateDirFallback(t *testing.T) {
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, ".crabclaw")
	if err := os.MkdirAll(globalDir, 0700); err != nil {
		t.Fatal(err)
	}

	envContent := "TEST_DOTENV_CRAB=hello_from_crab\n"
	if err := os.WriteFile(filepath.Join(globalDir, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CRABCLAW_STATE_DIR", globalDir)
	os.Unsetenv("TEST_DOTENV_CRAB")

	LoadDotEnv(true)

	if got := os.Getenv("TEST_DOTENV_CRAB"); got != "hello_from_crab" {
		t.Errorf("TEST_DOTENV_CRAB = %q, want %q", got, "hello_from_crab")
	}
}

func TestLoadDotEnv_ProfileAwareFallback(t *testing.T) {
	tmpHome := t.TempDir()
	stateDir := filepath.Join(tmpHome, ".openacosmi-staging")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".env"), []byte("TEST_DOTENV_PROFILE=from_profile\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OPENACOSMI_HOME", tmpHome)
	t.Setenv("OPENACOSMI_PROFILE", "staging")
	t.Setenv("CRABCLAW_STATE_DIR", "")
	t.Setenv("OPENACOSMI_STATE_DIR", "")
	os.Unsetenv("TEST_DOTENV_PROFILE")

	LoadDotEnv(true)

	if got := os.Getenv("TEST_DOTENV_PROFILE"); got != "from_profile" {
		t.Fatalf("TEST_DOTENV_PROFILE = %q, want %q", got, "from_profile")
	}
}

// TestLoadDotEnv_CwdPriority 验证 CWD .env 优先于全局 .env。
func TestLoadDotEnv_CwdPriority(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(globalDir, 0700); err != nil {
		t.Fatal(err)
	}

	// 全局 .env
	globalContent := "TEST_DOTENV_PRIORITY=from_global\n"
	if err := os.WriteFile(filepath.Join(globalDir, ".env"), []byte(globalContent), 0600); err != nil {
		t.Fatal(err)
	}

	// CWD .env (创建在当前工作目录)
	cwdDir := t.TempDir()
	cwdContent := "TEST_DOTENV_PRIORITY=from_cwd\n"
	if err := os.WriteFile(filepath.Join(cwdDir, ".env"), []byte(cwdContent), 0600); err != nil {
		t.Fatal(err)
	}

	// 切换到 CWD（注意 LoadDotEnv 使用相对路径 ".env"）
	origDir, _ := os.Getwd()
	if err := os.Chdir(cwdDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Setenv("OPENACOSMI_STATE_DIR", globalDir)
	os.Unsetenv("TEST_DOTENV_PRIORITY")

	LoadDotEnv(true)

	got := os.Getenv("TEST_DOTENV_PRIORITY")
	if got != "from_cwd" {
		t.Errorf("TEST_DOTENV_PRIORITY = %q, want %q (CWD should take priority)", got, "from_cwd")
	}
}

// TestLoadDotEnv_NoOverwrite 验证不覆盖已有环境变量。
func TestLoadDotEnv_NoOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	envContent := "TEST_DOTENV_NOOVERWRITE=from_file\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatal(err)
	}

	// 预设环境变量
	t.Setenv("TEST_DOTENV_NOOVERWRITE", "pre_existing")
	t.Setenv("OPENACOSMI_STATE_DIR", tmpDir)

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	LoadDotEnv(true)

	got := os.Getenv("TEST_DOTENV_NOOVERWRITE")
	if got != "pre_existing" {
		t.Errorf("TEST_DOTENV_NOOVERWRITE = %q, want %q (should not overwrite)", got, "pre_existing")
	}
}

// TestUnquoteDotEnvValue_EdgeCases 验证 .env 值引号剥离的边界情况。
func TestUnquoteDotEnvValue_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello"`, "hello"},
		{`'hello'`, "hello"},
		{`hello`, "hello"},
		{`""`, ""},
		{`''`, ""},
		{`"`, `"`},
		{`'`, `'`},
		{``, ``},
		{`"mismatched'`, `"mismatched'`},
		{`"with spaces"`, "with spaces"},
		{`'with "inner" quotes'`, `with "inner" quotes`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := unquoteDotEnvValue(tt.input)
			if got != tt.want {
				t.Errorf("unquoteDotEnvValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
