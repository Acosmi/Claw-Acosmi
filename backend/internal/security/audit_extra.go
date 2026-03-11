// audit_extra.go — 安全审计扩展检查。
//
// TS 对照: security/audit-extra.ts (~1306L)
//
// 包含：攻击面汇总、云同步目录检测、密钥泄露检查、
// 钩子加固检查、模型卫生检查、小模型风险、
// 插件信任、include 文件权限、深度文件系统检查、
// 暴露矩阵、插件/技能代码安全。
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------- collectAttackSurfaceSummaryFindings ----------

// CollectAttackSurfaceSummaryFindings 汇总攻击面。
// TS 对照: audit-extra.ts collectAttackSurfaceSummaryFindings()
func CollectAttackSurfaceSummaryFindings(opts SecurityAuditOptions) []SecurityAuditFinding {
	elevated := false
	hooksEnabled := false
	browserEnabled := false

	if opts.ElevatedConfig != nil {
		elevated = opts.ElevatedConfig.Enabled
	}
	if opts.HooksConfig != nil {
		hooksEnabled = opts.HooksConfig.Enabled
	}
	if opts.BrowserConfig != nil {
		browserEnabled = opts.BrowserConfig.Enabled
	}

	formatBool := func(b bool) string {
		if b {
			return "enabled"
		}
		return "disabled"
	}

	detail := fmt.Sprintf(
		"tools.elevated: %s\nhooks: %s\nbrowser control: %s",
		formatBool(elevated),
		formatBool(hooksEnabled),
		formatBool(browserEnabled),
	)

	return []SecurityAuditFinding{
		{
			CheckID:  "summary.attack_surface",
			Severity: SeverityInfo,
			Title:    "Attack surface summary",
			Detail:   detail,
		},
	}
}

// ---------- collectSyncedFolderFindings ----------

// isProbablySyncedPath 检测路径是否在云同步目录内。
// TS 对照: audit-extra.ts isProbablySyncedPath()
func isProbablySyncedPath(p string) bool {
	s := strings.ToLower(p)
	return strings.Contains(s, "icloud") ||
		strings.Contains(s, "dropbox") ||
		strings.Contains(s, "google drive") ||
		strings.Contains(s, "googledrive") ||
		strings.Contains(s, "onedrive")
}

// CollectSyncedFolderFindings 检测 state/config 是否在云同步目录。
// TS 对照: audit-extra.ts collectSyncedFolderFindings()
func CollectSyncedFolderFindings(stateDir, configPath string) []SecurityAuditFinding {
	if isProbablySyncedPath(stateDir) || isProbablySyncedPath(configPath) {
		return []SecurityAuditFinding{
			{
				CheckID:     "fs.synced_dir",
				Severity:    SeverityWarn,
				Title:       "State/config path looks like a synced folder",
				Detail:      fmt.Sprintf("stateDir=%s, configPath=%s. Synced folders (iCloud/Dropbox/OneDrive/Google Drive) can leak tokens and transcripts onto other devices.", stateDir, configPath),
				Remediation: "Keep OPENACOSMI_STATE_DIR on a local-only volume.",
			},
		}
	}
	return nil
}

// ---------- collectSecretsInConfigFindings ----------

// looksLikeEnvRef 检测值是否像一个环境变量引用 ${…}。
// TS 对照: audit-extra.ts looksLikeEnvRef()
func looksLikeEnvRef(value string) bool {
	v := strings.TrimSpace(value)
	return strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}")
}

// CollectSecretsInConfigFindings 检查配置中是否有硬编码密钥。
// TS 对照: audit-extra.ts collectSecretsInConfigFindings()
func CollectSecretsInConfigFindings(gw *GatewayConfigSnapshot, hooks *HooksConfigSnapshot) []SecurityAuditFinding {
	var findings []SecurityAuditFinding

	if gw != nil {
		password := strings.TrimSpace(gw.AuthPassword)
		if password != "" && !looksLikeEnvRef(password) {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "config.secrets.gateway_password_in_config",
				Severity:    SeverityWarn,
				Title:       "Gateway password is stored in config",
				Detail:      "gateway.auth.password is set in the config file; prefer environment variables for secrets when possible.",
				Remediation: "Prefer OPENACOSMI_GATEWAY_PASSWORD (env) and remove gateway.auth.password from disk.",
			})
		}
	}

	if hooks != nil && hooks.Enabled {
		token := strings.TrimSpace(hooks.Token)
		if token != "" && !looksLikeEnvRef(token) {
			findings = append(findings, SecurityAuditFinding{
				CheckID:  "config.secrets.hooks_token_in_config",
				Severity: SeverityInfo,
				Title:    "Hooks token is stored in config",
				Detail:   "hooks.token is set in the config file; keep config perms tight and treat it like an API secret.",
			})
		}
	}

	return findings
}

// ---------- collectHooksHardeningFindings ----------

// CollectHooksHardeningFindings 审计钩子系统安全加固。
// TS 对照: audit-extra.ts collectHooksHardeningFindings()
func CollectHooksHardeningFindings(hc *HooksConfigSnapshot) []SecurityAuditFinding {
	if hc == nil || !hc.Enabled {
		return nil
	}

	var findings []SecurityAuditFinding
	token := strings.TrimSpace(hc.Token)

	// Token 太短
	if token != "" && len(token) < 24 {
		findings = append(findings, SecurityAuditFinding{
			CheckID:  "hooks.token_too_short",
			Severity: SeverityWarn,
			Title:    "Hooks token looks short",
			Detail:   fmt.Sprintf("hooks.token is %d chars; prefer a long random token.", len(token)),
		})
	}

	// Token 复用 Gateway token
	gatewayToken := strings.TrimSpace(hc.GatewayToken)
	if token != "" && gatewayToken != "" && token == gatewayToken {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "hooks.token_reuse_gateway_token",
			Severity:    SeverityWarn,
			Title:       "Hooks token reuses the Gateway token",
			Detail:      "hooks.token matches gateway.auth token; compromise of hooks expands blast radius to the Gateway API.",
			Remediation: "Use a separate hooks.token dedicated to hook ingress.",
		})
	}

	// Path = "/"
	if hc.Path == "/" {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "hooks.path_root",
			Severity:    SeverityCritical,
			Title:       "Hooks base path is '/'",
			Detail:      "hooks.path='/' would shadow other HTTP endpoints and is unsafe.",
			Remediation: "Use a dedicated path like '/hooks'.",
		})
	}

	return findings
}

// ---------- collectModelHygieneFindings ----------

var (
	legacyModelPatterns = []struct {
		id    string
		re    *regexp.Regexp
		label string
	}{
		{"openai.gpt35", regexp.MustCompile(`(?i)\bgpt-3\.5\b`), "GPT-3.5 family"},
		{"anthropic.claude2", regexp.MustCompile(`(?i)\bclaude-(instant|2)\b`), "Claude 2/Instant family"},
		{"openai.gpt4_legacy", regexp.MustCompile(`(?i)\bgpt-4-(0314|0613)\b`), "Legacy GPT-4 snapshots"},
	}

	weakTierModelPatterns = []struct {
		id    string
		re    *regexp.Regexp
		label string
	}{
		{"anthropic.haiku", regexp.MustCompile(`(?i)\bhaiku\b`), "Haiku tier (smaller model)"},
	}

	reGptModel         = regexp.MustCompile(`(?i)\bgpt-`)
	reGpt5OrHigher     = regexp.MustCompile(`(?i)\bgpt-5(?:\b|[.-])`)
	reClaudeModel      = regexp.MustCompile(`(?i)\bclaude-`)
	reClaude45OrHigher = regexp.MustCompile(`(?i)\bclaude-[^\s/]*?(?:-4-?(?:[5-9]|[1-9]\d)\b|4\.(?:[5-9]|[1-9]\d)\b|-[5-9](?:\b|[.-]))`)
)

// CollectModelHygieneFindings 审计模型卫生。
// TS 对照: audit-extra.ts collectModelHygieneFindings()
func CollectModelHygieneFindings(models []ModelRef) []SecurityAuditFinding {
	if len(models) == 0 {
		return nil
	}

	var findings []SecurityAuditFinding

	// 遗留模型检查
	type match struct {
		model  string
		source string
		reason string
	}
	var legacyMatches []match

	// 弱模型检查
	type weakEntry struct {
		model   string
		source  string
		reasons []string
	}
	weakMap := make(map[string]*weakEntry) // key: model@@source

	for _, entry := range models {
		// 遗留模型
		for _, pat := range legacyModelPatterns {
			if pat.re.MatchString(entry.ID) {
				legacyMatches = append(legacyMatches, match{
					model: entry.ID, source: entry.Source, reason: pat.label,
				})
				break
			}
		}

		// 弱模型
		for _, pat := range weakTierModelPatterns {
			if pat.re.MatchString(entry.ID) {
				key := entry.ID + "@@" + entry.Source
				if existing, ok := weakMap[key]; ok {
					existing.reasons = append(existing.reasons, pat.label)
				} else {
					weakMap[key] = &weakEntry{
						model: entry.ID, source: entry.Source,
						reasons: []string{pat.label},
					}
				}
				break
			}
		}

		// GPT < 5
		if reGptModel.MatchString(entry.ID) && !reGpt5OrHigher.MatchString(entry.ID) {
			key := entry.ID + "@@" + entry.Source
			reason := "Below GPT-5 family"
			if existing, ok := weakMap[key]; ok {
				existing.reasons = append(existing.reasons, reason)
			} else {
				weakMap[key] = &weakEntry{
					model: entry.ID, source: entry.Source,
					reasons: []string{reason},
				}
			}
		}

		// Claude < 4.5
		if reClaudeModel.MatchString(entry.ID) && !reClaude45OrHigher.MatchString(entry.ID) {
			key := entry.ID + "@@" + entry.Source
			reason := "Below Claude 4.5"
			if existing, ok := weakMap[key]; ok {
				existing.reasons = append(existing.reasons, reason)
			} else {
				weakMap[key] = &weakEntry{
					model: entry.ID, source: entry.Source,
					reasons: []string{reason},
				}
			}
		}
	}

	// 遗留模型发现
	if len(legacyMatches) > 0 {
		var lines []string
		limit := 12
		if len(legacyMatches) < limit {
			limit = len(legacyMatches)
		}
		for _, m := range legacyMatches[:limit] {
			lines = append(lines, fmt.Sprintf("- %s (%s) @ %s", m.model, m.reason, m.source))
		}
		detail := "Older/legacy models can be less robust against prompt injection and tool misuse.\n" +
			strings.Join(lines, "\n")
		if len(legacyMatches) > 12 {
			detail += fmt.Sprintf("\n…%d more", len(legacyMatches)-12)
		}
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "models.legacy",
			Severity:    SeverityWarn,
			Title:       "Some configured models look legacy",
			Detail:      detail,
			Remediation: "Prefer modern, instruction-hardened models for any bot that can run tools.",
		})
	}

	// 弱模型发现
	if len(weakMap) > 0 {
		var lines []string
		count := 0
		for _, entry := range weakMap {
			if count >= 12 {
				break
			}
			lines = append(lines, fmt.Sprintf("- %s (%s) @ %s",
				entry.model, strings.Join(entry.reasons, "; "), entry.source))
			count++
		}
		detail := "Smaller/older models are generally more susceptible to prompt injection and tool misuse.\n" +
			strings.Join(lines, "\n")
		if len(weakMap) > 12 {
			detail += fmt.Sprintf("\n…%d more", len(weakMap)-12)
		}
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "models.weak_tier",
			Severity:    SeverityWarn,
			Title:       "Some configured models are below recommended tiers",
			Detail:      detail,
			Remediation: "Use the latest, top-tier model for any bot with tools or untrusted inboxes. Avoid Haiku tiers; prefer GPT-5+ and Claude 4.5+.",
		})
	}

	return findings
}

// ---------- collectSmallModelRiskFindings ----------

// SmallModelRiskParams 小模型风险检查参数。
type SmallModelRiskParams struct {
	Models         []ModelRef
	SandboxEnabled bool
}

// CollectSmallModelRiskFindings 检查小模型在无沙箱下的风险。
// TS 对照: audit-extra.ts collectSmallModelRiskFindings()
func CollectSmallModelRiskFindings(params SmallModelRiskParams) []SecurityAuditFinding {
	if params.SandboxEnabled || len(params.Models) == 0 {
		return nil
	}

	var smallModels []string
	for _, entry := range params.Models {
		isSmall := false
		for _, pat := range weakTierModelPatterns {
			if pat.re.MatchString(entry.ID) {
				isSmall = true
				break
			}
		}
		if isSmall {
			smallModels = append(smallModels, fmt.Sprintf("- %s @ %s", entry.ID, entry.Source))
		}
	}

	if len(smallModels) == 0 {
		return nil
	}

	limit := 12
	if len(smallModels) < limit {
		limit = len(smallModels)
	}
	detail := "Small models are more susceptible to prompt injection. When sandboxing is off, tool calls from these models run with full host access.\n" +
		strings.Join(smallModels[:limit], "\n")
	if len(smallModels) > 12 {
		detail += fmt.Sprintf("\n…%d more", len(smallModels)-12)
	}

	return []SecurityAuditFinding{
		{
			CheckID:     "models.small_unsandboxed",
			Severity:    SeverityWarn,
			Title:       "Small models used without sandbox enforcement",
			Detail:      detail,
			Remediation: "Enable sandbox.enabled=true or disable web/browser tools for smaller models.",
		},
	}
}

// ---------- collectPluginsTrustFindings ----------

// PluginsTrustParams 插件信任检查参数。
type PluginsTrustParams struct {
	PluginsAllow     []string // plugins.allow 配置
	InstalledPlugins []string // 已安装的插件名
	StateDir         string
}

// CollectPluginsTrustFindings 检查 plugins.allow 与已安装扩展的匹配。
// TS 对照: audit-extra.ts collectPluginsTrustFindings()
func CollectPluginsTrustFindings(params PluginsTrustParams) []SecurityAuditFinding {
	var findings []SecurityAuditFinding

	extensionsDir := filepath.Join(params.StateDir, "extensions")
	entries, err := os.ReadDir(extensionsDir)
	if err != nil {
		// 如果目录不存在或不可读，跳过
		return nil
	}

	var installedNames []string
	for _, e := range entries {
		if e.IsDir() {
			installedNames = append(installedNames, e.Name())
		}
	}

	if len(installedNames) == 0 {
		return nil
	}

	// 构建允许列表 set
	allowSet := make(map[string]bool)
	for _, name := range params.PluginsAllow {
		name = strings.TrimSpace(name)
		if name != "" {
			allowSet[name] = true
		}
	}

	var unallowed []string
	for _, name := range installedNames {
		if !allowSet[name] && !allowSet["*"] {
			unallowed = append(unallowed, name)
		}
	}

	if len(unallowed) > 0 {
		detail := fmt.Sprintf(
			"Found %d installed plugin(s) not present in plugins.allow:\n- %s",
			len(unallowed),
			strings.Join(unallowed, "\n- "),
		)
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "plugins.trust.not_in_allowlist",
			Severity:    SeverityWarn,
			Title:       "Installed plugins not in plugins.allow",
			Detail:      detail,
			Remediation: "Add trusted plugins to plugins.allow in config, or remove untrusted extensions.",
		})
	}

	return findings
}

// ---------- collectIncludeFilePermFindings ----------

// IncludeFilePermParams include 文件权限检查参数。
type IncludeFilePermParams struct {
	IncludePaths []string
}

// CollectIncludeFilePermFindings 检查配置 include 文件的权限。
// TS 对照: audit-extra.ts collectIncludeFilePermFindings()
func CollectIncludeFilePermFindings(params IncludeFilePermParams) []SecurityAuditFinding {
	var findings []SecurityAuditFinding

	for _, p := range params.IncludePaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		perms := InspectPathPermissions(p)
		if !perms.OK {
			continue
		}

		if perms.WorldWritable || perms.GroupWritable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.config_include.perms_writable",
				Severity:    SeverityCritical,
				Title:       "Config include file is writable by others",
				Detail:      FormatPermissionDetail(p, perms) + "; another user could influence your effective config.",
				Remediation: FormatPermissionRemediation(p, perms, false, 0o600),
			})
		} else if perms.WorldReadable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.config_include.perms_world_readable",
				Severity:    SeverityCritical,
				Title:       "Config include file is world-readable",
				Detail:      FormatPermissionDetail(p, perms) + "; include files can contain tokens and private settings.",
				Remediation: FormatPermissionRemediation(p, perms, false, 0o600),
			})
		} else if perms.GroupReadable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.config_include.perms_group_readable",
				Severity:    SeverityWarn,
				Title:       "Config include file is group-readable",
				Detail:      FormatPermissionDetail(p, perms) + "; include files can contain tokens and private settings.",
				Remediation: FormatPermissionRemediation(p, perms, false, 0o600),
			})
		}
	}

	return findings
}

// ---------- collectStateDeepFilesystemFindings ----------

// StateDeepFSParams 深度文件系统检查参数。
type StateDeepFSParams struct {
	StateDir    string
	AgentIDs    []string
	LogFilePath string
}

// CollectStateDeepFilesystemFindings 深度检查 state 目录内的凭证/会话文件权限。
// TS 对照: audit-extra.ts collectStateDeepFilesystemFindings()
func CollectStateDeepFilesystemFindings(params StateDeepFSParams) []SecurityAuditFinding {
	var findings []SecurityAuditFinding

	// OAuth 目录检查
	oauthDir := filepath.Join(params.StateDir, "oauth")
	oauthPerms := InspectPathPermissions(oauthDir)
	if oauthPerms.OK && oauthPerms.IsDir {
		if oauthPerms.WorldWritable || oauthPerms.GroupWritable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.credentials_dir.perms_writable",
				Severity:    SeverityCritical,
				Title:       "Credentials dir is writable by others",
				Detail:      FormatPermissionDetail(oauthDir, oauthPerms) + "; another user could drop/modify credential files.",
				Remediation: FormatPermissionRemediation(oauthDir, oauthPerms, true, 0o700),
			})
		} else if oauthPerms.GroupReadable || oauthPerms.WorldReadable {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "fs.credentials_dir.perms_readable",
				Severity:    SeverityWarn,
				Title:       "Credentials dir is readable by others",
				Detail:      FormatPermissionDetail(oauthDir, oauthPerms) + "; credentials and allowlists can be sensitive.",
				Remediation: FormatPermissionRemediation(oauthDir, oauthPerms, true, 0o700),
			})
		}
	}

	// 各 agent 的 auth-profiles.json 和 sessions.json
	for _, agentID := range params.AgentIDs {
		agentDir := filepath.Join(params.StateDir, "agents", agentID, "agent")
		authPath := filepath.Join(agentDir, "auth-profiles.json")
		authPerms := InspectPathPermissions(authPath)
		if authPerms.OK {
			if authPerms.WorldWritable || authPerms.GroupWritable {
				findings = append(findings, SecurityAuditFinding{
					CheckID:     "fs.auth_profiles.perms_writable",
					Severity:    SeverityCritical,
					Title:       "auth-profiles.json is writable by others",
					Detail:      FormatPermissionDetail(authPath, authPerms) + "; another user could inject credentials.",
					Remediation: FormatPermissionRemediation(authPath, authPerms, false, 0o600),
				})
			} else if authPerms.WorldReadable || authPerms.GroupReadable {
				findings = append(findings, SecurityAuditFinding{
					CheckID:     "fs.auth_profiles.perms_readable",
					Severity:    SeverityWarn,
					Title:       "auth-profiles.json is readable by others",
					Detail:      FormatPermissionDetail(authPath, authPerms) + "; auth-profiles.json contains API keys and OAuth tokens.",
					Remediation: FormatPermissionRemediation(authPath, authPerms, false, 0o600),
				})
			}
		}

		storePath := filepath.Join(params.StateDir, "agents", agentID, "sessions", "sessions.json")
		storePerms := InspectPathPermissions(storePath)
		if storePerms.OK {
			if storePerms.WorldReadable || storePerms.GroupReadable {
				findings = append(findings, SecurityAuditFinding{
					CheckID:     "fs.sessions_store.perms_readable",
					Severity:    SeverityWarn,
					Title:       "sessions.json is readable by others",
					Detail:      FormatPermissionDetail(storePath, storePerms) + "; routing and transcript metadata can be sensitive.",
					Remediation: FormatPermissionRemediation(storePath, storePerms, false, 0o600),
				})
			}
		}
	}

	// 日志文件检查
	if params.LogFilePath != "" {
		logPerms := InspectPathPermissions(params.LogFilePath)
		if logPerms.OK {
			if logPerms.WorldReadable || logPerms.GroupReadable {
				findings = append(findings, SecurityAuditFinding{
					CheckID:     "fs.log_file.perms_readable",
					Severity:    SeverityWarn,
					Title:       "Log file is readable by others",
					Detail:      FormatPermissionDetail(params.LogFilePath, logPerms) + "; logs can contain private messages and tool output.",
					Remediation: FormatPermissionRemediation(params.LogFilePath, logPerms, false, 0o600),
				})
			}
		}
	}

	return findings
}

// ---------- collectExposureMatrixFindings ----------

// listGroupPolicyOpen 列出所有 groupPolicy="open" 的通道。
// TS 对照: audit-extra.ts listGroupPolicyOpen()
func listGroupPolicyOpen(channels map[string]interface{}) []string {
	var out []string
	if channels == nil {
		return out
	}
	for channelID, value := range channels {
		section, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		if gp, ok := section["groupPolicy"].(string); ok && gp == "open" {
			out = append(out, fmt.Sprintf("channels.%s.groupPolicy", channelID))
		}
		accounts, ok := section["accounts"].(map[string]interface{})
		if !ok {
			continue
		}
		for accountID, accountVal := range accounts {
			acc, ok := accountVal.(map[string]interface{})
			if !ok {
				continue
			}
			if gp, ok := acc["groupPolicy"].(string); ok && gp == "open" {
				out = append(out, fmt.Sprintf("channels.%s.accounts.%s.groupPolicy", channelID, accountID))
			}
		}
	}
	return out
}

// ExposureMatrixParams 暴露矩阵检查参数。
type ExposureMatrixParams struct {
	Channels        map[string]interface{} // channels 配置（动态结构）
	ElevatedEnabled bool
}

// CollectExposureMatrixFindings 检查开放 groupPolicy 与特权工具的组合风险。
// TS 对照: audit-extra.ts collectExposureMatrixFindings()
func CollectExposureMatrixFindings(params ExposureMatrixParams) []SecurityAuditFinding {
	var findings []SecurityAuditFinding
	openGroups := listGroupPolicyOpen(params.Channels)
	if len(openGroups) == 0 {
		return findings
	}

	if params.ElevatedEnabled {
		detail := fmt.Sprintf(
			"Found groupPolicy=\"open\" at:\n%s\nWith tools.elevated enabled, a prompt injection in those rooms can become a high-impact incident.",
			"- "+strings.Join(openGroups, "\n- "),
		)
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "security.exposure.open_groups_with_elevated",
			Severity:    SeverityCritical,
			Title:       "Open groupPolicy with elevated tools enabled",
			Detail:      detail,
			Remediation: `Set groupPolicy="allowlist" and keep elevated allowlists extremely tight.`,
		})
	}

	return findings
}

// ---------- collectPluginsCodeSafetyFindings ----------

// CollectPluginsCodeSafetyFindings 扫描插件扩展目录中的代码安全问题。
// TS 对照: audit-extra.ts collectPluginsCodeSafetyFindings()
func CollectPluginsCodeSafetyFindings(stateDir string) []SecurityAuditFinding {
	var findings []SecurityAuditFinding
	extensionsDir := filepath.Join(stateDir, "extensions")

	st := SafeStat(extensionsDir)
	if !st.OK || !st.IsDir {
		return findings
	}

	entries, err := os.ReadDir(extensionsDir)
	if err != nil {
		findings = append(findings, SecurityAuditFinding{
			CheckID:     "plugins.code_safety.scan_failed",
			Severity:    SeverityWarn,
			Title:       "Plugin extensions directory scan failed",
			Detail:      fmt.Sprintf("Static code scan could not list extensions directory: %s", err),
			Remediation: "Check file permissions and plugin layout, then rerun `crabclaw security audit --deep`.",
		})
		return findings
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginName := e.Name()
		pluginPath := filepath.Join(extensionsDir, pluginName)

		summary, err := ScanDirectoryWithSummary(pluginPath, nil)
		if err != nil {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "plugins.code_safety.scan_failed",
				Severity:    SeverityWarn,
				Title:       fmt.Sprintf("Plugin %q code scan failed", pluginName),
				Detail:      fmt.Sprintf("Static code scan could not complete: %s", err),
				Remediation: "Check file permissions and plugin layout, then rerun `crabclaw security audit --deep`.",
			})
			continue
		}

		if summary.Critical > 0 {
			details := formatCodeSafetyDetails(summary.Findings, pluginPath, "critical")
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "plugins.code_safety",
				Severity:    SeverityCritical,
				Title:       fmt.Sprintf("Plugin %q contains dangerous code patterns", pluginName),
				Detail:      fmt.Sprintf("Found %d critical issue(s) in %d scanned file(s):\n%s", summary.Critical, summary.ScannedFiles, details),
				Remediation: "Review the plugin source code carefully before use. If untrusted, remove the plugin from your Crab Claw（蟹爪） extensions state directory.",
			})
		} else if summary.Warn > 0 {
			details := formatCodeSafetyDetails(summary.Findings, pluginPath, "warn")
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "plugins.code_safety",
				Severity:    SeverityWarn,
				Title:       fmt.Sprintf("Plugin %q contains suspicious code patterns", pluginName),
				Detail:      fmt.Sprintf("Found %d warning(s) in %d scanned file(s):\n%s", summary.Warn, summary.ScannedFiles, details),
				Remediation: "Review the flagged code to ensure it is intentional and safe.",
			})
		}
	}

	return findings
}

// ---------- collectInstalledSkillsCodeSafetyFindings ----------

// InstalledSkillEntry 已安装技能条目。
type InstalledSkillEntry struct {
	Name    string
	BaseDir string
	Source  string // "openacosmi-bundled" | "user" | "plugin" | ...
}

// CollectInstalledSkillsCodeSafetyFindings 扫描已安装技能的代码安全问题。
// TS 对照: audit-extra.ts collectInstalledSkillsCodeSafetyFindings()
func CollectInstalledSkillsCodeSafetyFindings(skills []InstalledSkillEntry, stateDir string) []SecurityAuditFinding {
	var findings []SecurityAuditFinding
	pluginExtensionsDir := filepath.Join(stateDir, "extensions")
	scannedDirs := make(map[string]bool)

	for _, entry := range skills {
		if entry.Source == "openacosmi-bundled" {
			continue
		}

		skillDir, err := filepath.Abs(entry.BaseDir)
		if err != nil {
			continue
		}

		// 跳过已被 plugins.code_safety 覆盖的路径
		if isPathInside(pluginExtensionsDir, skillDir) {
			continue
		}
		if scannedDirs[skillDir] {
			continue
		}
		scannedDirs[skillDir] = true

		skillName := entry.Name
		summary, err := ScanDirectoryWithSummary(skillDir, nil)
		if err != nil {
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "skills.code_safety.scan_failed",
				Severity:    SeverityWarn,
				Title:       fmt.Sprintf("Skill %q code scan failed", skillName),
				Detail:      fmt.Sprintf("Static code scan could not complete for %s: %s", skillDir, err),
				Remediation: "Check file permissions and skill layout, then rerun `crabclaw security audit --deep`.",
			})
			continue
		}

		if summary.Critical > 0 {
			details := formatCodeSafetyDetails(summary.Findings, skillDir, "critical")
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "skills.code_safety",
				Severity:    SeverityCritical,
				Title:       fmt.Sprintf("Skill %q contains dangerous code patterns", skillName),
				Detail:      fmt.Sprintf("Found %d critical issue(s) in %d scanned file(s) under %s:\n%s", summary.Critical, summary.ScannedFiles, skillDir, details),
				Remediation: fmt.Sprintf("Review the skill source code before use. If untrusted, remove %q.", skillDir),
			})
		} else if summary.Warn > 0 {
			details := formatCodeSafetyDetails(summary.Findings, skillDir, "warn")
			findings = append(findings, SecurityAuditFinding{
				CheckID:     "skills.code_safety",
				Severity:    SeverityWarn,
				Title:       fmt.Sprintf("Skill %q contains suspicious code patterns", skillName),
				Detail:      fmt.Sprintf("Found %d warning(s) in %d scanned file(s) under %s:\n%s", summary.Warn, summary.ScannedFiles, skillDir, details),
				Remediation: "Review flagged lines to ensure the behavior is intentional and safe.",
			})
		}
	}

	return findings
}

// ---------- 辅助函数 ----------

// isPathInside 检查 candidate 是否在 basePath 内部。
// TS 对照: audit-extra.ts isPathInside()
func isPathInside(basePath, candidatePath string) bool {
	base, _ := filepath.Abs(basePath)
	candidate, _ := filepath.Abs(candidatePath)
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

// formatCodeSafetyDetails 格式化代码安全扫描发现的详情。
// TS 对照: audit-extra.ts formatCodeSafetyDetails()
func formatCodeSafetyDetails(findings []SkillScanFinding, rootDir, severity string) string {
	var lines []string
	for _, f := range findings {
		if string(f.Severity) != severity {
			continue
		}
		relPath, err := filepath.Rel(rootDir, f.File)
		if err != nil || relPath == "" || relPath == "." || strings.HasPrefix(relPath, "..") {
			relPath = filepath.Base(f.File)
		}
		normalizedPath := strings.ReplaceAll(relPath, "\\", "/")
		lines = append(lines, fmt.Sprintf("  - [%s] %s (%s:%d)", f.RuleID, f.Message, normalizedPath, f.Line))
	}
	return strings.Join(lines, "\n")
}

// ---------- readConfigSnapshotForAudit ----------

// ConfigFileSnapshot 配置文件快照（用于审计）。
// TS 对照: audit-extra.ts / config.ts ConfigFileSnapshot
type ConfigFileSnapshot struct {
	Exists bool
	Valid  bool
	Path   string
	Parsed interface{} // 原始解析结果
	Issues []ConfigIssue
}

// ConfigIssue 配置问题。
type ConfigIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ReadConfigSnapshotForAudit 读取配置文件快照用于审计。
// TS 对照: audit-extra.ts readConfigSnapshotForAudit()
func ReadConfigSnapshotForAudit(configPath string) ConfigFileSnapshot {
	snap := ConfigFileSnapshot{
		Path: configPath,
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return snap
		}
		snap.Issues = append(snap.Issues, ConfigIssue{
			Path:    configPath,
			Message: fmt.Sprintf("read error: %s", err),
		})
		return snap
	}
	snap.Exists = true

	var parsed interface{}
	if err := ParseJSONC(data, &parsed); err != nil {
		snap.Issues = append(snap.Issues, ConfigIssue{
			Path:    configPath,
			Message: fmt.Sprintf("parse error: %s", err),
		})
		return snap
	}

	snap.Valid = true
	snap.Parsed = parsed
	return snap
}

// ---------- extensionUsesSkippedScannerPath ----------

// extensionUsesSkippedScannerPath 检查插件扩展入口是否使用隐藏路径或 node_modules。
// TS 对照: audit-extra.ts extensionUsesSkippedScannerPath()
func extensionUsesSkippedScannerPath(entry string) bool {
	// 将 \ 和 / 分割为段
	segments := strings.FieldsFunc(entry, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if seg == "node_modules" {
			return true
		}
		if len(seg) > 1 && seg[0] == '.' && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

// ---------- readPluginManifestExtensions ----------

// ManifestKey package.json 中的 openacosmi 键名。
const ManifestKey = "openacosmi"

// readPluginManifestExtensions 从插件 package.json 读取 openacosmi.extensions 列表。
// TS 对照: audit-extra.ts readPluginManifestExtensions()
func readPluginManifestExtensions(pluginPath string) []string {
	manifestPath := filepath.Join(pluginPath, "package.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}

	var parsed map[string]interface{}
	if err := ParseJSONC(data, &parsed); err != nil {
		return nil
	}

	openacosmiVal, ok := parsed[ManifestKey]
	if !ok {
		return nil
	}
	openacosmiMap, ok := openacosmiVal.(map[string]interface{})
	if !ok {
		return nil
	}
	extVal, ok := openacosmiMap["extensions"]
	if !ok {
		return nil
	}
	extArr, ok := extVal.([]interface{})
	if !ok {
		return nil
	}

	var result []string
	for _, item := range extArr {
		if s, ok := item.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, s)
			}
		}
	}
	return result
}
