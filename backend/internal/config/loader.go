package config

// 配置加载器 — 对应 src/config/io.ts (617 行)
//
// 提供配置文件的加载、解析、验证、写入能力。
// 主要流程: 读取 JSON/JSON5 → env 替换 → 验证 → 应用默认值 → 缓存。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/log"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/tailscale/hujson"
)

// BuildVersion 构建版本号 (由 main.go 注入)
// 对应 TS: import { VERSION } from "../version.js"
var BuildVersion = "dev"

// ConfigLoader 配置加载器
type ConfigLoader struct {
	configPath string
	logger     *log.Logger
	mu         sync.RWMutex
	cache      *configCacheEntry
}

type configCacheEntry struct {
	configPath string
	expiresAt  time.Time
	config     *types.OpenAcosmiConfig
}

// DefaultConfigCacheMs 默认配置缓存毫秒
const DefaultConfigCacheMs = 200

// ShellEnvExpectedKeys 预期的 shell 环境变量
var ShellEnvExpectedKeys = []string{
	"OPENAI_API_KEY",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_OAUTH_TOKEN",
	"GEMINI_API_KEY",
	"ZAI_API_KEY",
	"OPENROUTER_API_KEY",
	"AI_GATEWAY_API_KEY",
	"MINIMAX_API_KEY",
	"SYNTHETIC_API_KEY",
	"ELEVENLABS_API_KEY",
	"TELEGRAM_BOT_TOKEN",
	"DISCORD_BOT_TOKEN",
	"SLACK_BOT_TOKEN",
	"SLACK_APP_TOKEN",
	"CRABCLAW_GATEWAY_TOKEN",
	"CRABCLAW_GATEWAY_PASSWORD",
	"OPENACOSMI_GATEWAY_TOKEN",
	"OPENACOSMI_GATEWAY_PASSWORD",
}

// NewConfigLoader 创建配置加载器
func NewConfigLoader(opts ...LoaderOption) *ConfigLoader {
	loader := &ConfigLoader{
		configPath: ResolveConfigPath(),
		logger:     log.New("config"),
	}
	for _, opt := range opts {
		opt(loader)
	}
	return loader
}

// LoaderOption 加载器选项
type LoaderOption func(*ConfigLoader)

// WithConfigPath 指定配置文件路径
func WithConfigPath(p string) LoaderOption {
	return func(l *ConfigLoader) {
		l.configPath = p
	}
}

// WithLogger 指定日志器
func WithLogger(logger *log.Logger) LoaderOption {
	return func(l *ConfigLoader) {
		l.logger = logger
	}
}

// ConfigPath 返回配置文件路径
func (l *ConfigLoader) ConfigPath() string {
	return l.configPath
}

// LoadConfig 加载并验证配置
// 流程: 读取文件 → JSON 解析 → env 替换 → 验证 → 应用默认值
func (l *ConfigLoader) LoadConfig() (*types.OpenAcosmiConfig, error) {
	// 检查缓存
	if cfg := l.getCached(); cfg != nil {
		return cfg, nil
	}

	cfg, err := l.loadFromDisk()
	if err != nil {
		return &types.OpenAcosmiConfig{}, err
	}

	l.setCache(cfg)
	return cfg, nil
}

// ReadConfigFileSnapshot 读取配置文件快照（不走缓存）
// 返回 types.ConfigFileSnapshot 以保持与 pkg/types 一致
func (l *ConfigLoader) ReadConfigFileSnapshot() (*types.ConfigFileSnapshot, error) {
	if !fileExists(l.configPath) {
		hash := hashConfigRaw("")
		cfg := types.OpenAcosmiConfig{}
		// F4: TS readConfigFileSnapshot 在文件不存在时也应用完整 defaults 链
		result := ApplyDefaults(&cfg)
		applyTalkApiKey(result)
		return &types.ConfigFileSnapshot{
			Path:     l.configPath,
			Exists:   false,
			Raw:      nil,
			Parsed:   map[string]interface{}{},
			Valid:    true,
			Config:   *result,
			Hash:     hash,
			Issues:   []types.ConfigValidationIssue{},
			Warnings: []types.ConfigValidationIssue{},
		}, nil
	}

	data, err := os.ReadFile(l.configPath)
	if err != nil {
		return &types.ConfigFileSnapshot{
			Path:   l.configPath,
			Exists: true,
			Raw:    nil,
			Parsed: map[string]interface{}{},
			Valid:  false,
			Config: types.OpenAcosmiConfig{},
			Hash:   hashConfigRaw(""),
			Issues: []types.ConfigValidationIssue{{Message: fmt.Sprintf("read failed: %v", err)}},
		}, nil
	}

	raw := string(data)
	hash := hashConfigRaw(raw)

	var parsed interface{}
	if err := parseJSON5(data, &parsed); err != nil {
		return &types.ConfigFileSnapshot{
			Path:   l.configPath,
			Exists: true,
			Raw:    &raw,
			Parsed: map[string]interface{}{},
			Valid:  false,
			Config: types.OpenAcosmiConfig{},
			Hash:   hash,
			Issues: []types.ConfigValidationIssue{{Message: fmt.Sprintf("JSON5 parse failed: %v", err)}},
		}, nil
	}

	// ── Pipeline: $include → env subst → path norm → legacy → overrides ──
	parsedMap, isMap := parsed.(map[string]interface{})

	// Step 1: $include 解析
	if isMap {
		if _, hasInclude := parsedMap[IncludeKey]; hasInclude {
			resolver := IncludeResolver{
				ReadFile: func(p string) (string, error) {
					d, err := os.ReadFile(p)
					return string(d), err
				},
				ParseJSON: func(r string) (interface{}, error) {
					var v interface{}
					if err := parseJSON5([]byte(r), &v); err != nil {
						return nil, err
					}
					return v, nil
				},
			}
			resolved, err := ResolveConfigIncludes(parsed, l.configPath, resolver)
			if err != nil {
				l.logger.Warn("config $include resolution failed: %v", err)
			} else {
				parsed = resolved
				if m, ok := resolved.(map[string]interface{}); ok {
					parsedMap = m
				}
			}
		}
	}

	// Step 2: 环境变量替换
	if isMap {
		substituted, err := ResolveConfigEnvVarsWithLookup(parsedMap, os.LookupEnv)
		if err != nil {
			l.logger.Warn("config env substitution failed: %v", err)
		} else if m, ok := substituted.(map[string]interface{}); ok {
			parsedMap = m
			parsed = m
		}
	}

	// Step 3: 路径归一化 (~/ 展开)
	if isMap {
		NormalizeConfigPaths(parsedMap)
	}

	// Step 4: Legacy 迁移
	var warnings []types.ConfigValidationIssue
	if isMap {
		legacyIssues := FindLegacyConfigIssues(parsedMap)
		for _, li := range legacyIssues {
			warnings = append(warnings, types.ConfigValidationIssue{
				Path:    li.Path,
				Message: li.Message,
			})
		}
		migResult := ApplyLegacyMigrations(parsedMap)
		if migResult.Next != nil {
			parsedMap = migResult.Next
			parsed = migResult.Next
			for _, change := range migResult.Changes {
				l.logger.Info("config migration: %s", change)
			}
		}
	}

	// Step 5: Runtime overrides
	if isMap {
		parsedMap = ApplyConfigOverrides(parsedMap)
		parsed = parsedMap
	}

	// 反序列化为结构体
	processedJSON, err := json.Marshal(parsed)
	if err != nil {
		return &types.ConfigFileSnapshot{
			Path: l.configPath, Exists: true, Raw: &raw, Parsed: parsed,
			Valid: false, Config: types.OpenAcosmiConfig{}, Hash: hash,
			Issues: []types.ConfigValidationIssue{{Message: fmt.Sprintf("re-marshal failed: %v", err)}},
		}, nil
	}

	var cfg types.OpenAcosmiConfig
	if err := json.Unmarshal(processedJSON, &cfg); err != nil {
		return &types.ConfigFileSnapshot{
			Path: l.configPath, Exists: true, Raw: &raw, Parsed: parsed,
			Valid: false, Config: types.OpenAcosmiConfig{}, Hash: hash,
			Issues: []types.ConfigValidationIssue{{Message: fmt.Sprintf("config unmarshal failed: %v", err)}},
		}, nil
	}
	for _, change := range repairRuntimeProviderConfigs(&cfg) {
		warnings = append(warnings, types.ConfigValidationIssue{Message: change})
	}

	// 验证
	issues := ValidateOpenAcosmiConfig(&cfg)
	var cfgIssues []types.ConfigValidationIssue
	for _, iss := range issues {
		cfgIssues = append(cfgIssues, types.ConfigValidationIssue{
			Path:    iss.Field,
			Message: iss.Message,
		})
	}

	valid := len(cfgIssues) == 0

	// F4: TS readConfigFileSnapshot 对有效配置也应用完整 defaults 链
	// TS: applyTalkApiKey(applyModelDefaults(applyAgentDefaults(applySessionDefaults(
	//       applyLoggingDefaults(applyMessageDefaults(validated.config))))))
	var finalCfg types.OpenAcosmiConfig
	if valid {
		result := ApplyDefaults(&cfg)
		applyTalkApiKey(result)
		// F6: warnIfConfigFromFuture
		warnIfConfigFromFuture(result, l.logger)
		finalCfg = *result
	} else {
		finalCfg = cfg
	}

	return &types.ConfigFileSnapshot{
		Path:     l.configPath,
		Exists:   true,
		Raw:      &raw,
		Parsed:   parsed,
		Valid:    valid,
		Config:   finalCfg,
		Hash:     hash,
		Issues:   cfgIssues,
		Warnings: warnings,
	}, nil
}

// WriteConfigFile 写入配置文件（原子操作 + 轮换备份）
func (l *ConfigLoader) WriteConfigFile(cfg *types.OpenAcosmiConfig) error {
	l.ClearCache()

	// 验证
	if issues := ValidateOpenAcosmiConfig(cfg); len(issues) > 0 {
		return fmt.Errorf("config validation failed: %s: %s", issues[0].Field, issues[0].Message)
	}

	// F7: stampConfigVersion — 对应 TS io.ts:498
	stamped := stampConfigVersion(cfg)
	// F12: applyModelDefaults — 对应 TS io.ts:498
	applyModelDefaults(stamped)

	// [NEW] 抽出敏感数据存入 OS Keyring，获取安全的占位符副本
	safeMap, err := MapStructToMapForKeyring(stamped)
	if err != nil {
		return fmt.Errorf("failed to convert config for keyring: %w", err)
	}
	safeConfigData, err := StoreSensitiveToKeyring(safeMap)
	if err != nil {
		return fmt.Errorf("keyring storage failed: %w", err)
	}

	// 序列化安全副本
	data, err := json.MarshalIndent(safeConfigData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(l.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// 写临时文件
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.%d.tmp", filepath.Base(l.configPath), os.Getpid()))
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write tmp config: %w", err)
	}

	// 轮换备份
	if fileExists(l.configPath) {
		l.rotateBackups()
		_ = copyFile(l.configPath, l.configPath+".bak")
	}

	// 原子替换
	if err := os.Rename(tmpFile, l.configPath); err != nil {
		// 回退: 复制方式
		if copyErr := copyFile(tmpFile, l.configPath); copyErr != nil {
			_ = os.Remove(tmpFile)
			return fmt.Errorf("failed to write config: %w (copy fallback also failed: %v)", err, copyErr)
		}
		_ = os.Remove(tmpFile)
	}

	return nil
}

// ClearCache 清除配置缓存
func (l *ConfigLoader) ClearCache() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = nil
}

// ----- 内部方法 -----

// loadFromDisk 从磁盘加载配置并应用完整管道
// 对应 TS io.ts 中 loadConfig() 的完整流程
func (l *ConfigLoader) loadFromDisk() (*types.OpenAcosmiConfig, error) {
	if !fileExists(l.configPath) {
		return &types.OpenAcosmiConfig{}, nil
	}

	data, err := os.ReadFile(l.configPath)
	if err != nil {
		l.logger.Error("Failed to read config", "path", l.configPath, "error", err)
		return &types.OpenAcosmiConfig{}, fmt.Errorf("failed to read config: %w", err)
	}

	cfg, err := l.applyConfigPipeline(data)
	if err != nil {
		l.logger.Error("Config pipeline failed", "path", l.configPath, "error", err)
		return &types.OpenAcosmiConfig{}, err
	}

	return cfg, nil
}

// applyConfigPipeline 对原始 JSON 数据应用完整的配置管道
// Pipeline: $include → env subst → path norm → legacy migration → runtime overrides → defaults
// 对应 TS io.ts loadConfig() 中的 L228-L310 管道逻辑
func (l *ConfigLoader) applyConfigPipeline(data []byte) (*types.OpenAcosmiConfig, error) {
	var parsed interface{}
	if err := parseJSON5(data, &parsed); err != nil {
		return nil, fmt.Errorf("JSON5 parse failed: %w", err)
	}

	parsedMap, isMap := parsed.(map[string]interface{})

	// Step 1: $include 解析
	if isMap {
		if _, hasInclude := parsedMap[IncludeKey]; hasInclude {
			resolver := IncludeResolver{
				ReadFile: func(p string) (string, error) {
					d, err := os.ReadFile(p)
					return string(d), err
				},
				ParseJSON: func(r string) (interface{}, error) {
					var v interface{}
					if err := parseJSON5([]byte(r), &v); err != nil {
						return nil, err
					}
					return v, nil
				},
			}
			resolved, err := ResolveConfigIncludes(parsed, l.configPath, resolver)
			if err != nil {
				l.logger.Warn("config $include resolution failed", "error", err)
			} else {
				parsed = resolved
				if m, ok := resolved.(map[string]interface{}); ok {
					parsedMap = m
					isMap = true
				}
			}
		}
	}

	// Step 1.5: 应用 config.env 到 process.env（在 env subst 之前）
	// 对应 TS: applyConfigEnv(resolved, deps.env) BEFORE substitution
	// 这样 ${VAR} 可以引用配置文件中定义的环境变量
	if isMap {
		var tempCfg types.OpenAcosmiConfig
		if tempJSON, err := json.Marshal(parsedMap); err == nil {
			if err := json.Unmarshal(tempJSON, &tempCfg); err == nil {
				ApplyConfigEnv(&tempCfg)
			}
		}
	}

	// Step 2: 环境变量替换
	if isMap {
		substituted, err := ResolveConfigEnvVarsWithLookup(parsedMap, os.LookupEnv)
		if err != nil {
			l.logger.Warn("config env substitution failed", "error", err)
		} else if m, ok := substituted.(map[string]interface{}); ok {
			parsedMap = m
			parsed = m
		}
	}

	// F5: warnOnConfigMiskeys — 对应 TS io.ts:122-135
	if isMap {
		warnOnConfigMiskeys(parsedMap, l.logger)
	}

	// [NEW] 反序列化成结构体之前，从 OS Keyring 还原敏感明文被替换进去
	if isMap {
		if err := RestoreFromKeyring(parsedMap); err != nil {
			l.logger.Warn("Failed to restore some sensitive keys from OS Keyring", "error", err)
		}
	}

	// Step 3: 路径归一化 (~/ 展开) — 在 raw map 上先执行一次
	if isMap {
		NormalizeConfigPaths(parsedMap)
	}

	// Step 4: Legacy 迁移
	if isMap {
		migResult := ApplyLegacyMigrations(parsedMap)
		if migResult.Next != nil {
			parsedMap = migResult.Next
			parsed = migResult.Next
			for _, change := range migResult.Changes {
				l.logger.Info("config migration applied", "change", change)
			}
		}
	}

	// Step 5: Runtime overrides
	if isMap {
		parsedMap = ApplyConfigOverrides(parsedMap)
		parsed = parsedMap
	}

	// F8: findDuplicateAgentDirs — 预验证检查 (对应 TS io.ts:250-256)
	// 先反序列化一次用于 dup 检查
	if isMap {
		var preCfg types.OpenAcosmiConfig
		if preJSON, marshalErr := json.Marshal(parsed); marshalErr == nil {
			if json.Unmarshal(preJSON, &preCfg) == nil {
				if dups := FindDuplicateAgentDirs(&preCfg); len(dups) > 0 {
					for _, dup := range dups {
						l.logger.Error("Duplicate agent directory detected", "dir", dup.AgentDir, "agents", dup.AgentIDs)
					}
				}
			}
		}
	}

	// 反序列化为结构体
	processedJSON, err := json.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("re-marshal after pipeline failed: %w", err)
	}

	var cfg types.OpenAcosmiConfig
	if err := json.Unmarshal(processedJSON, &cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal failed: %w", err)
	}
	for _, change := range repairRuntimeProviderConfigs(&cfg) {
		l.logger.Info("config runtime repair applied", "change", change)
	}

	// 验证（仅记录警告，不阻断加载）
	if issues := ValidateOpenAcosmiConfig(&cfg); len(issues) > 0 {
		for _, iss := range issues {
			l.logger.Warn("Config validation issue", "field", iss.Field, "message", iss.Message)
		}
	}

	// 应用默认值
	result := ApplyDefaults(&cfg)

	// F10: normalizeConfigPaths 在 defaults 之后再执行一次 (对应 TS io.ts:287)
	// TS 在 defaults 之后调用，确保 defaults 新增的路径也被 normalize
	normalizeStructConfigPaths(result)

	// F8: findDuplicateAgentDirs — 后验证 (对应 TS io.ts:289-295)
	if dups := FindDuplicateAgentDirs(result); len(dups) > 0 {
		for _, dup := range dups {
			l.logger.Error("Duplicate agent directory detected (post-defaults)", "dir", dup.AgentDir, "agents", dup.AgentIDs)
		}
	}

	// 再次应用 config.env（defaults 之后可能新增 env 配置）
	// 对应 TS: applyConfigEnv(cfg, deps.env) 在 defaults 之后
	ApplyConfigEnv(result)

	// F6: warnIfConfigFromFuture — 对应 TS io.ts:277
	warnIfConfigFromFuture(result, l.logger)

	// applyConfigOverrides 在 TS 中最后调用 (io.ts:310)
	// Go 已在 Step 5 处理过 raw map 覆盖，此处无需重复

	return result, nil
}

func (l *ConfigLoader) getCached() *types.OpenAcosmiConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cache == nil {
		return nil
	}
	if time.Now().After(l.cache.expiresAt) {
		return nil
	}
	if l.cache.configPath != l.configPath {
		return nil
	}
	return l.cache.config
}

func (l *ConfigLoader) setCache(cfg *types.OpenAcosmiConfig) {
	// 对应 TS: config/io.ts:559-572 — resolveConfigCacheMs(env)
	cacheMs := resolveConfigCacheMs()
	if cacheMs <= 0 {
		return // 缓存被禁用
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = &configCacheEntry{
		configPath: l.configPath,
		expiresAt:  time.Now().Add(time.Duration(cacheMs) * time.Millisecond),
		config:     cfg,
	}
}

// resolveConfigCacheMs 解析配置缓存 TTL。
// 对应 TS: config/io.ts:559-572 — resolveConfigCacheMs(env)
func resolveConfigCacheMs() int {
	raw := compatEnvValue("CRABCLAW_CONFIG_CACHE_MS", "OPENACOSMI_CONFIG_CACHE_MS")
	if raw == "" {
		return DefaultConfigCacheMs
	}
	if raw == "0" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultConfigCacheMs
	}
	if parsed < 0 {
		return 0
	}
	return parsed
}

func (l *ConfigLoader) rotateBackups() {
	base := l.configPath + ".bak"
	maxIdx := ConfigBackupCount - 1
	_ = os.Remove(fmt.Sprintf("%s.%d", base, maxIdx))
	for i := maxIdx - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", base, i), fmt.Sprintf("%s.%d", base, i+1))
	}
	_ = os.Rename(base, base+".1")
}

// ----- 工具函数 -----

// parseJSON5 解析 JSON5/JWCC 格式数据。
// 使用 hujson 先去除注释和尾逗号，再用标准 encoding/json 反序列化。
// 对应 TS 版本中 `import JSON5 from "json5"` 的功能。
func parseJSON5(data []byte, v interface{}) error {
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return fmt.Errorf("JSON5 syntax error: %w", err)
	}
	return json.Unmarshal(standardized, v)
}

func hashConfigRaw(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// ----- F5/F6/F7/F10 辅助函数 -----

// warnOnConfigMiskeys 检测常见的配置键名误用
// 对应 TS io.ts:122-135
func warnOnConfigMiskeys(raw map[string]interface{}, logger *log.Logger) {
	gateway, ok := raw["gateway"]
	if !ok {
		return
	}
	gatewayMap, ok := gateway.(map[string]interface{})
	if !ok {
		return
	}
	if _, hasToken := gatewayMap["token"]; hasToken {
		logger.Warn(`Config uses "gateway.token". This key is ignored; use "gateway.auth.token" instead.`)
	}
}

// warnIfConfigFromFuture 检测配置是否由更新的版本写入
// 对应 TS io.ts:149-163
func warnIfConfigFromFuture(cfg *types.OpenAcosmiConfig, logger *log.Logger) {
	if cfg.Meta == nil {
		return
	}
	touched := cfg.Meta.LastTouchedVersion
	if touched == "" {
		return
	}
	cmp, ok := CompareOpenAcosmiVersions(BuildVersion, touched)
	if !ok {
		return
	}
	if cmp < 0 {
		logger.Warn("Config was last written by a newer Crab Claw（蟹爪） build (%s); current version is %s.", touched, BuildVersion)
	}
}

// stampConfigVersion 在配置中注入版本和时间戳
// 对应 TS io.ts:137-147
func stampConfigVersion(cfg *types.OpenAcosmiConfig) *types.OpenAcosmiConfig {
	now := time.Now().UTC().Format(time.RFC3339)
	result := *cfg // 浅拷贝
	if result.Meta == nil {
		result.Meta = &types.OpenAcosmiMeta{}
	}
	result.Meta.LastTouchedVersion = BuildVersion
	result.Meta.LastTouchedAt = now
	return &result
}

// normalizeStructConfigPaths 对结构体进行路径归一化
// 通过 JSON roundtrip 转 map 后调用 NormalizeConfigPaths
// 对应 TS io.ts:287 在 defaults 之后调用 normalizeConfigPaths
func normalizeStructConfigPaths(cfg *types.OpenAcosmiConfig) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	NormalizeConfigPaths(m)
	data2, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data2, cfg)
}

// ----- 环境变量收集 (env-vars.ts) -----

// CollectConfigEnvVars 从配置中收集环境变量
// 对应 src/config/env-vars.ts
func CollectConfigEnvVars(cfg *types.OpenAcosmiConfig) map[string]string {
	result := make(map[string]string)
	if cfg == nil || cfg.Env == nil {
		return result
	}

	envCfg := cfg.Env
	if envCfg.Vars != nil {
		for k, v := range envCfg.Vars {
			if v != "" {
				result[k] = v
			}
		}
	}

	return result
}

// ApplyConfigEnv 将配置中定义的环境变量应用到 os.Environ
func ApplyConfigEnv(cfg *types.OpenAcosmiConfig) {
	entries := CollectConfigEnvVars(cfg)
	for k, v := range entries {
		if existing := os.Getenv(k); existing != "" {
			continue // 不覆盖已有环境变量
		}
		_ = os.Setenv(k, v)
	}
}
