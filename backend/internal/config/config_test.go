package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolvePaths(t *testing.T) {
	// 基本路径解析
	home := ResolveHomeDir()
	if home == "" {
		t.Fatal("expected non-empty home dir")
	}

	// 状态目录
	stateDir := ResolveStateDir()
	if stateDir == "" {
		t.Fatal("expected non-empty state dir")
	}

	// 配置路径
	cfgPath := ResolveCanonicalConfigPath()
	if cfgPath == "" {
		t.Fatal("expected non-empty config path")
	}

	// 网关锁目录
	lockDir := ResolveGatewayLockDir()
	if lockDir == "" {
		t.Fatal("expected non-empty lock dir")
	}

	// OAuth 路径
	oauthPath := ResolveOAuthPath()
	if oauthPath == "" {
		t.Fatal("expected non-empty oauth path")
	}
}

func TestResolveGatewayPort(t *testing.T) {
	// 默认端口
	if port := ResolveGatewayPort(nil); port != DefaultGatewayPort {
		t.Errorf("expected default port %d, got %d", DefaultGatewayPort, port)
	}

	// config 覆盖
	p := 9999
	if port := ResolveGatewayPort(&p); port != 9999 {
		t.Errorf("expected port 9999, got %d", port)
	}
}

func TestDerivePort_ZeroBase(t *testing.T) {
	// When base port is 0, derivePort must return the fallback, not a privileged port.
	got := DeriveDefaultBrowserControlPort(0)
	if got != DefaultBrowserControlPort {
		t.Errorf("DeriveDefaultBrowserControlPort(0) = %d, want fallback %d", got, DefaultBrowserControlPort)
	}

	got = DeriveDefaultBridgePort(0)
	if got != DefaultBridgePort {
		t.Errorf("DeriveDefaultBridgePort(0) = %d, want fallback %d", got, DefaultBridgePort)
	}

	got = DeriveDefaultCanvasHostPort(0)
	if got != DefaultCanvasHostPort {
		t.Errorf("DeriveDefaultCanvasHostPort(0) = %d, want fallback %d", got, DefaultCanvasHostPort)
	}

	// Negative base must also return fallback.
	got = DeriveDefaultBrowserControlPort(-1)
	if got != DefaultBrowserControlPort {
		t.Errorf("DeriveDefaultBrowserControlPort(-1) = %d, want fallback %d", got, DefaultBrowserControlPort)
	}

	// Normal derivation should still work.
	got = DeriveDefaultBrowserControlPort(19001)
	if got != 19003 {
		t.Errorf("DeriveDefaultBrowserControlPort(19001) = %d, want 19003", got)
	}
}

func TestConfigCandidates(t *testing.T) {
	candidates := ResolveConfigCandidates()
	if len(candidates) == 0 {
		t.Fatal("expected non-empty candidates")
	}
	// 第一个候选文件名应该仍然是 openacosmi.json（目录优先级可变，文件名在本阶段不变）
	if filepath.Base(candidates[0]) != ConfigFilename {
		t.Errorf("expected first candidate to be %s, got %s", ConfigFilename, filepath.Base(candidates[0]))
	}
}

func TestResolveStateDirPrefersCrabClawWhenItContainsManagedState(t *testing.T) {
	tmpHome := t.TempDir()
	oldHome := os.Getenv("OPENACOSMI_HOME")
	t.Cleanup(func() {
		_ = os.Setenv("OPENACOSMI_HOME", oldHome)
	})
	_ = os.Setenv("OPENACOSMI_HOME", tmpHome)

	crabDir := filepath.Join(tmpHome, CompatibilityStateDirname)
	openDir := filepath.Join(tmpHome, NewStateDirname)
	if err := os.MkdirAll(filepath.Join(crabDir, "credentials"), 0o755); err != nil {
		t.Fatalf("mkdir crab credentials: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(openDir, "credentials"), 0o755); err != nil {
		t.Fatalf("mkdir open credentials: %v", err)
	}

	got := ResolveStateDir()
	if got != crabDir {
		t.Fatalf("expected state dir %s, got %s", crabDir, got)
	}
}

func TestResolveStateDirKeepsOpenAcosmiWhenCrabClawIsEmpty(t *testing.T) {
	tmpHome := t.TempDir()
	oldHome := os.Getenv("OPENACOSMI_HOME")
	t.Cleanup(func() {
		_ = os.Setenv("OPENACOSMI_HOME", oldHome)
	})
	_ = os.Setenv("OPENACOSMI_HOME", tmpHome)

	crabDir := filepath.Join(tmpHome, CompatibilityStateDirname)
	openDir := filepath.Join(tmpHome, NewStateDirname)
	if err := os.MkdirAll(crabDir, 0o755); err != nil {
		t.Fatalf("mkdir crab dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(openDir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir open sessions: %v", err)
	}

	got := ResolveStateDir()
	if got != openDir {
		t.Fatalf("expected state dir %s, got %s", openDir, got)
	}
}

func TestResolveConfigPathPrefersCrabClawConfigWhenPresent(t *testing.T) {
	tmpHome := t.TempDir()
	oldHome := os.Getenv("OPENACOSMI_HOME")
	t.Cleanup(func() {
		_ = os.Setenv("OPENACOSMI_HOME", oldHome)
	})
	_ = os.Setenv("OPENACOSMI_HOME", tmpHome)

	crabConfig := filepath.Join(tmpHome, CompatibilityStateDirname, ConfigFilename)
	openConfig := filepath.Join(tmpHome, NewStateDirname, ConfigFilename)
	if err := os.MkdirAll(filepath.Dir(crabConfig), 0o755); err != nil {
		t.Fatalf("mkdir crab dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(openConfig), 0o755); err != nil {
		t.Fatalf("mkdir open dir: %v", err)
	}
	if err := os.WriteFile(openConfig, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write open config: %v", err)
	}
	if err := os.WriteFile(crabConfig, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write crab config: %v", err)
	}

	got := ResolveConfigPath()
	if got != crabConfig {
		t.Fatalf("expected config path %s, got %s", crabConfig, got)
	}
}

func TestResolveStateDirUsesCrabClawEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	oldCrab := os.Getenv("CRABCLAW_STATE_DIR")
	oldOpen := os.Getenv("OPENACOSMI_STATE_DIR")
	t.Cleanup(func() {
		_ = os.Setenv("CRABCLAW_STATE_DIR", oldCrab)
		_ = os.Setenv("OPENACOSMI_STATE_DIR", oldOpen)
	})
	_ = os.Setenv("OPENACOSMI_STATE_DIR", "")
	_ = os.Setenv("CRABCLAW_STATE_DIR", tmpDir)

	got := ResolveStateDir()
	if got != tmpDir {
		t.Fatalf("expected state dir %s, got %s", tmpDir, got)
	}
}

func TestResolveConfigCandidatesUseProfileSuffix(t *testing.T) {
	tmpHome := t.TempDir()
	oldHome := os.Getenv("OPENACOSMI_HOME")
	oldProfile := os.Getenv("OPENACOSMI_PROFILE")
	oldCrabProfile := os.Getenv("CRABCLAW_PROFILE")
	t.Cleanup(func() {
		_ = os.Setenv("OPENACOSMI_HOME", oldHome)
		_ = os.Setenv("OPENACOSMI_PROFILE", oldProfile)
		_ = os.Setenv("CRABCLAW_PROFILE", oldCrabProfile)
	})
	_ = os.Setenv("OPENACOSMI_HOME", tmpHome)
	_ = os.Setenv("OPENACOSMI_PROFILE", "staging")
	_ = os.Setenv("CRABCLAW_PROFILE", "")

	candidates := ResolveConfigCandidates()
	want := filepath.Join(tmpHome, NewStateDirname+"-staging", ConfigFilename)
	found := false
	for _, candidate := range candidates {
		if candidate == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ResolveConfigCandidates() missing %q in %v", want, candidates)
	}
}

func TestIsNixMode(t *testing.T) {
	// 默认应该是 false
	if IsNixMode() {
		t.Skip("OPENACOSMI_NIX_MODE is set, skipping")
	}
}

func TestExpandTilde(t *testing.T) {
	result := expandTilde("/normal/path")
	if result != "/normal/path" {
		t.Errorf("expected /normal/path, got %s", result)
	}

	result = expandTilde("")
	if result != "" {
		t.Errorf("expected empty, got %s", result)
	}
}

func TestConfigLoaderEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")

	loader := NewConfigLoader(WithConfigPath(cfgPath))
	if loader.ConfigPath() != cfgPath {
		t.Errorf("expected config path %s, got %s", cfgPath, loader.ConfigPath())
	}

	// 文件不存在时应返回空配置
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestConfigLoaderReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")

	loader := NewConfigLoader(WithConfigPath(cfgPath))

	// 写入配置
	cfg := &types.OpenAcosmiConfig{
		Logging: &types.LoggingConfig{Level: "debug"},
	}
	if err := loader.WriteConfigFile(cfg); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("config file should exist after write")
	}

	// 重新加载
	loader.ClearCache()
	loaded, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Logging == nil || loaded.Logging.Level != "debug" {
		t.Error("expected logging level 'debug' after reload")
	}
}

func TestConfigLoaderSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")

	loader := NewConfigLoader(WithConfigPath(cfgPath))

	// 快照 — 文件不存在
	snap, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Exists {
		t.Error("expected exists=false for missing file")
	}
	if !snap.Valid {
		t.Error("expected valid=true for missing file (empty config is valid)")
	}

	// 写有效配置
	cfg := &types.OpenAcosmiConfig{
		Update: &types.OpenAcosmiUpdateConfig{Channel: "stable"},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(cfgPath, data, 0600)

	snap, err = loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snap.Exists {
		t.Error("expected exists=true")
	}
	if !snap.Valid {
		t.Error("expected valid=true")
	}
	if snap.Hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestConfigLoaderBackupRotation(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")

	loader := NewConfigLoader(WithConfigPath(cfgPath))

	// 写入多次触发备份轮换
	for i := 0; i < 3; i++ {
		cfg := &types.OpenAcosmiConfig{
			Logging: &types.LoggingConfig{Level: "info"},
		}
		if err := loader.WriteConfigFile(cfg); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	// 检查备份文件
	if _, err := os.Stat(cfgPath + ".bak"); os.IsNotExist(err) {
		t.Error("expected .bak file to exist after multiple writes")
	}
}

func TestConfigLoaderCaching(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")

	cfg := &types.OpenAcosmiConfig{
		Logging: &types.LoggingConfig{Level: "warn"},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(cfgPath, data, 0600)

	loader := NewConfigLoader(WithConfigPath(cfgPath))

	// 第一次加载
	c1, _ := loader.LoadConfig()
	// 第二次加载应该走缓存
	c2, _ := loader.LoadConfig()
	if c1 != c2 {
		t.Error("expected cached config to be same pointer")
	}

	// 清除缓存后应该不同
	loader.ClearCache()
	c3, _ := loader.LoadConfig()
	if c3 == c1 {
		t.Error("expected different pointer after cache clear")
	}
}

func TestCollectConfigEnvVars(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Env: &types.OpenAcosmiEnvConfig{
			Vars: map[string]string{
				"MY_KEY":    "my_value",
				"EMPTY_KEY": "",
			},
		},
	}

	vars := CollectConfigEnvVars(cfg)
	if vars["MY_KEY"] != "my_value" {
		t.Errorf("expected MY_KEY=my_value, got %q", vars["MY_KEY"])
	}
	if _, exists := vars["EMPTY_KEY"]; exists {
		t.Error("expected empty values to be excluded")
	}

	// nil case
	vars = CollectConfigEnvVars(nil)
	if len(vars) != 0 {
		t.Error("expected empty map for nil config")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}
	result := ApplyDefaults(cfg)

	// A2 修复后: logging 不再自动创建
	if result.Logging != nil {
		t.Errorf("expected nil logging when not in config (TS parity)")
	}

	if result.Agents == nil || result.Agents.Defaults == nil {
		t.Fatal("expected agents.defaults to be initialized")
	}

	d := result.Agents.Defaults
	// F1: TS 不在 config defaults 注入 contextTokens/timeoutSeconds/mediaMaxMB
	if d.ContextTokens != nil {
		t.Errorf("expected contextTokens to be nil (not injected at config layer)")
	}
	if d.MaxConcurrent == nil || *d.MaxConcurrent != DefaultAgentMaxConcurrent {
		t.Errorf("expected maxConcurrent == %d", DefaultAgentMaxConcurrent)
	}
	if d.TimeoutSeconds != nil {
		t.Errorf("expected timeoutSeconds to be nil (not injected at config layer)")
	}
	if d.MediaMaxMB != nil {
		t.Errorf("expected mediaMaxMB to be nil (not injected at config layer)")
	}
}

func TestApplyDefaultsPreserves(t *testing.T) {
	// 已设置的值不应被覆盖
	existingTokens := 50000
	existingLevel := types.LogDebug
	cfg := &types.OpenAcosmiConfig{
		Logging: &types.LoggingConfig{Level: existingLevel},
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				ContextTokens: &existingTokens,
			},
		},
	}

	result := ApplyDefaults(cfg)

	if result.Logging.Level != existingLevel {
		t.Errorf("expected log level to be preserved as %q, got %q", existingLevel, result.Logging.Level)
	}
	if *result.Agents.Defaults.ContextTokens != existingTokens {
		t.Errorf("expected contextTokens to be preserved as %d", existingTokens)
	}
}

func TestApplyContextPruningDefaults(t *testing.T) {
	softRatio := 0.5
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				ContextPruning: &types.AgentContextPruningConfig{
					SoftTrimRatio: &softRatio,
				},
			},
		},
	}

	// A1 修复后: 无 Anthropic auth 时 applyContextPruningDefaults 直接返回，
	// fillContextPruningFields 不会被调用 —— hardClearRatio 不会被填充。
	// 测试调整: 添加 Anthropic auth 使其触发 ContextPruning 默认值填充。
	cfg.Auth = &types.AuthConfig{
		Profiles: map[string]*types.AuthProfileConfig{
			"main": {Provider: "anthropic", Mode: types.AuthModeAPIKey},
		},
	}
	ApplyDefaults(cfg)
	cp := cfg.Agents.Defaults.ContextPruning

	// softTrimRatio 应保留
	if *cp.SoftTrimRatio != 0.5 {
		t.Errorf("expected softTrimRatio 0.5, got %f", *cp.SoftTrimRatio)
	}
	// F3: hardClearRatio 不再由 config defaults 注入
	if cp.HardClearRatio != nil {
		t.Errorf("expected hardClearRatio to remain nil (not injected at config layer)")
	}
}

func TestApplyCompactionDefaults(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Compaction: &types.AgentCompactionConfig{},
			},
		},
	}

	ApplyDefaults(cfg)
	comp := cfg.Agents.Defaults.Compaction

	// F2: TS 不在 config defaults 注入 maxHistoryShare/reserveTokensFloor
	if comp.MaxHistoryShare != nil {
		t.Errorf("expected maxHistoryShare to remain nil (not injected at config layer)")
	}
	if comp.ReserveTokensFloor != nil {
		t.Errorf("expected reserveTokensFloor to remain nil (not injected at config layer)")
	}
}

// TestLoadConfigWithEnvSubstitution 验证 LoadConfig 也能正确处理环境变量替换
// 回归测试 BUG-2: loadFromDisk 跳过配置管道
func TestLoadConfigWithEnvSubstitution(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json5")
	_ = os.WriteFile(cfgPath, []byte(`{
		"logging": { "level": "${TEST_ACOSMI_LOG_LEVEL}" }
	}`), 0600)
	_ = os.Setenv("TEST_ACOSMI_LOG_LEVEL", "debug")
	defer func() { _ = os.Unsetenv("TEST_ACOSMI_LOG_LEVEL") }()

	loader := NewConfigLoader(WithConfigPath(cfgPath))
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Logging == nil || cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level='debug' after env substitution, got %v", cfg.Logging)
	}
}

// TestLoadConfigAppliesDefaults 验证 LoadConfig 会应用默认值
// 回归测试 BUG-2: loadFromDisk 跳过默认值应用
func TestLoadConfigAppliesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")
	_ = os.WriteFile(cfgPath, []byte(`{}`), 0600)

	loader := NewConfigLoader(WithConfigPath(cfgPath))
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A2 修复后: 空 JSON 不会创建 logging
	if cfg.Logging != nil {
		t.Errorf("expected nil logging after LoadConfig on empty JSON (TS parity)")
	}
}

func TestLoadConfigRepairsLegacyQwenPortalRuntimeProvider(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "qwen-portal.json")
	raw := `{
		"models": {
			"providers": {
				"qwen-portal": {
					"apiKey": "qwen-key"
				}
			}
		},
		"agents": {
			"defaults": {
				"model": {
					"fallbacks": ["qwen-portal/qwen3.5-plus"]
				}
			}
		}
	}`
	_ = os.WriteFile(cfgPath, []byte(raw), 0600)

	loader := NewConfigLoader(WithConfigPath(cfgPath))
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Models == nil || cfg.Models.Providers == nil {
		t.Fatal("models.providers should exist")
	}
	provCfg := cfg.Models.Providers["qwen-portal"]
	if provCfg == nil {
		t.Fatal("qwen-portal provider should exist")
	}
	if provCfg.API != "openai-completions" {
		t.Fatalf("provider API = %q, want %q", provCfg.API, "openai-completions")
	}
	if provCfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("provider BaseURL = %q", provCfg.BaseURL)
	}
	if provCfg.APIKey != "qwen-key" {
		t.Fatalf("provider APIKey = %q", provCfg.APIKey)
	}
	if len(provCfg.Models) == 0 {
		t.Fatal("provider models should be repaired")
	}

	runtimeCfg := cfg.Models.Providers["qwen"]
	if runtimeCfg == nil {
		t.Fatal("qwen runtime provider should be mirrored from legacy qwen-portal config")
	}
	if runtimeCfg.APIKey != "qwen-key" {
		t.Fatalf("runtime provider APIKey = %q", runtimeCfg.APIKey)
	}
	if runtimeCfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("runtime provider BaseURL = %q", runtimeCfg.BaseURL)
	}
}

func TestLoadConfigRepairsMissingMinimaxProviderFromModelRefs(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "minimax.json")
	raw := `{
		"agents": {
			"defaults": {
				"model": {
					"primary": "minimax/MiniMax-M2.5"
				}
			}
		}
	}`
	_ = os.WriteFile(cfgPath, []byte(raw), 0600)

	loader := NewConfigLoader(WithConfigPath(cfgPath))
	cfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Models == nil || cfg.Models.Providers == nil {
		t.Fatal("models.providers should exist")
	}
	provCfg := cfg.Models.Providers["minimax"]
	if provCfg == nil {
		t.Fatal("minimax provider should be restored from model refs")
	}
	if provCfg.API != "openai-completions" {
		t.Fatalf("provider API = %q, want %q", provCfg.API, "openai-completions")
	}
	if provCfg.BaseURL != "https://api.minimax.io/v1" {
		t.Fatalf("provider BaseURL = %q", provCfg.BaseURL)
	}
	if len(provCfg.Models) == 0 {
		t.Fatal("provider models should be restored")
	}
}
