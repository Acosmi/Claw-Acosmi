// fix.go — 安全修复操作。
//
// TS 对照: security/fix.ts (542L) — 全量对齐
//
// 根据审计发现自动修复安全问题：
//   - safeChmod / safeAclReset — 文件权限修复（含 symlink/类型检查/already skip）
//   - applyConfigFixes — 配置加固（logging.redactSensitive、groupPolicy → allowlist）
//   - setGroupPolicyAllowlist / setWhatsAppGroupAllowFromFromStore — 通道策略修复
//   - collectIncludePathsRecursive / listDirectIncludes — Include 路径递归发现
//   - chmodCredentialsAndAgentState — 凭证和 Agent 状态目录权限修复
//   - fixSecurityFootguns — 主入口
package security

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ---------- 类型定义 ----------

// SecurityFixChmodAction chmod 修复动作。
// TS 对照: fix.ts SecurityFixChmodAction
type SecurityFixChmodAction struct {
	Kind    string `json:"kind"` // "chmod"
	Path    string `json:"path"`
	Mode    int    `json:"mode"`
	OK      bool   `json:"ok"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SecurityFixIcaclsAction icacls 修复动作。
// TS 对照: fix.ts SecurityFixIcaclsAction
type SecurityFixIcaclsAction struct {
	Kind    string `json:"kind"` // "icacls"
	Path    string `json:"path"`
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SecurityFixAction 统一修复动作接口。
// TS 对照: fix.ts SecurityFixAction
type SecurityFixAction interface {
	IsOK() bool
	GetPath() string
	GetKind() string
}

func (a SecurityFixChmodAction) IsOK() bool      { return a.OK }
func (a SecurityFixChmodAction) GetPath() string { return a.Path }
func (a SecurityFixChmodAction) GetKind() string { return "chmod" }

func (a SecurityFixIcaclsAction) IsOK() bool      { return a.OK }
func (a SecurityFixIcaclsAction) GetPath() string { return a.Path }
func (a SecurityFixIcaclsAction) GetKind() string { return "icacls" }

// SecurityFixResult 修复结果。
// TS 对照: fix.ts SecurityFixResult
type SecurityFixResult struct {
	OK            bool                `json:"ok"`
	StateDir      string              `json:"stateDir"`
	ConfigPath    string              `json:"configPath"`
	ConfigWritten bool                `json:"configWritten"`
	Changes       []string            `json:"changes"`
	Actions       []SecurityFixAction `json:"actions"`
	Errors        []string            `json:"errors"`
}

// SecurityFixOptions 修复选项。
// TS 对照: fix.ts fixSecurityFootguns opts
type SecurityFixOptions struct {
	StateDir   string
	ConfigPath string
	Platform   string // "darwin" | "linux" | "windows"
}

// ---------- safeChmod ----------

// safeChmod 安全 chmod，含 symlink/类型/already 检查。
// TS 对照: fix.ts safeChmod()
func safeChmod(targetPath string, mode os.FileMode, require string) SecurityFixChmodAction {
	result := SecurityFixChmodAction{
		Kind: "chmod",
		Path: targetPath,
		Mode: int(mode),
	}

	info, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Skipped = "missing"
			return result
		}
		result.Error = fmt.Sprintf("lstat: %s", err)
		return result
	}

	// Symlink 检查
	if info.Mode()&os.ModeSymlink != 0 {
		result.Skipped = "symlink"
		return result
	}

	// 类型检查
	if require == "dir" && !info.IsDir() {
		result.Skipped = "not-a-directory"
		return result
	}
	if require == "file" && info.IsDir() {
		result.Skipped = "not-a-file"
		return result
	}

	// Already 检查
	current := info.Mode().Perm()
	if current == mode {
		result.Skipped = "already"
		return result
	}

	// 执行 chmod
	if err := os.Chmod(targetPath, mode); err != nil {
		result.Error = fmt.Sprintf("chmod: %s", err)
		return result
	}

	result.OK = true
	return result
}

// ---------- safeAclReset ----------

// safeAclReset Windows icacls ACL 重置。
// TS 对照: fix.ts safeAclReset()
func safeAclReset(targetPath, require string) SecurityFixIcaclsAction {
	display := fmt.Sprintf("icacls %q /reset", targetPath)
	if require == "dir" {
		display += " /T"
	}

	result := SecurityFixIcaclsAction{
		Kind:    "icacls",
		Path:    targetPath,
		Command: display,
	}

	info, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Skipped = "missing"
			return result
		}
		result.Error = fmt.Sprintf("lstat: %s", err)
		return result
	}

	if info.Mode()&os.ModeSymlink != 0 {
		result.Skipped = "symlink"
		return result
	}
	if require == "dir" && !info.IsDir() {
		result.Skipped = "not-a-directory"
		return result
	}
	if require == "file" && info.IsDir() {
		result.Skipped = "not-a-file"
		return result
	}

	// 构建 icacls 命令
	args := []string{targetPath, "/reset"}
	if info.IsDir() {
		args = append(args, "/T")
	}

	// 获取当前用户名用于 /grant
	username := os.Getenv("USERNAME")
	if username == "" {
		username = os.Getenv("USER")
	}
	if username == "" {
		result.Skipped = "missing-user"
		return result
	}

	cmd := exec.Command("icacls", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Error = fmt.Sprintf("icacls: %s — %s", err, strings.TrimSpace(string(output)))
		return result
	}

	// /reset 后授予当前用户完全控制权，避免 ACL 重置后用户自身丢失访问权
	grantArgs := []string{targetPath, "/grant", username + ":(F)"}
	if info.IsDir() {
		grantArgs = append(grantArgs, "/T")
	}
	grantCmd := exec.Command("icacls", grantArgs...)
	if grantOut, grantErr := grantCmd.CombinedOutput(); grantErr != nil {
		result.Error = fmt.Sprintf("icacls grant: %s — %s", grantErr, strings.TrimSpace(string(grantOut)))
		return result
	}

	result.OK = true
	result.Command = display + fmt.Sprintf(" && icacls %q /grant %s:(F)", targetPath, username)
	return result
}

// ---------- applyPerms ----------

// applyPerms 平台自适应权限修复。
// TS 对照: fix.ts applyPerms lambda
func applyPerms(targetPath string, mode os.FileMode, require string) SecurityFixAction {
	if runtime.GOOS == "windows" {
		return safeAclReset(targetPath, require)
	}
	return safeChmod(targetPath, mode, require)
}

// ---------- setGroupPolicyAllowlist ----------

// setGroupPolicyAllowlist 将 groupPolicy=open 修改为 allowlist。
// TS 对照: fix.ts setGroupPolicyAllowlist()
func setGroupPolicyAllowlist(
	cfg map[string]interface{},
	channel string,
	changes *[]string,
	policyFlips map[string]bool,
) {
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok || channels == nil {
		return
	}
	section, ok := channels[channel].(map[string]interface{})
	if !ok || section == nil {
		return
	}

	if gp, ok := section["groupPolicy"].(string); ok && gp == "open" {
		section["groupPolicy"] = "allowlist"
		*changes = append(*changes, fmt.Sprintf("channels.%s.groupPolicy=open -> allowlist", channel))
		policyFlips[fmt.Sprintf("channels.%s.", channel)] = true
	}

	accounts, ok := section["accounts"].(map[string]interface{})
	if !ok || accounts == nil {
		return
	}
	for accountID, accountVal := range accounts {
		account, ok := accountVal.(map[string]interface{})
		if !ok || account == nil {
			continue
		}
		if gp, ok := account["groupPolicy"].(string); ok && gp == "open" {
			account["groupPolicy"] = "allowlist"
			*changes = append(*changes, fmt.Sprintf(
				"channels.%s.accounts.%s.groupPolicy=open -> allowlist", channel, accountID,
			))
			policyFlips[fmt.Sprintf("channels.%s.accounts.%s.", channel, accountID)] = true
		}
	}
}

// ---------- setWhatsAppGroupAllowFromFromStore ----------

// setWhatsAppGroupAllowFromFromStore 从 pairing store 注入 groupAllowFrom。
// TS 对照: fix.ts setWhatsAppGroupAllowFromFromStore()
func setWhatsAppGroupAllowFromFromStore(
	cfg map[string]interface{},
	storeAllowFrom []string,
	changes *[]string,
	policyFlips map[string]bool,
) {
	if len(storeAllowFrom) == 0 {
		return
	}
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok || channels == nil {
		return
	}
	section, ok := channels["whatsapp"].(map[string]interface{})
	if !ok || section == nil {
		return
	}

	maybeApply := func(prefix string, obj map[string]interface{}) {
		if !policyFlips[prefix] {
			return
		}
		if af, ok := obj["allowFrom"].([]interface{}); ok && len(af) > 0 {
			return
		}
		if gaf, ok := obj["groupAllowFrom"].([]interface{}); ok && len(gaf) > 0 {
			return
		}
		obj["groupAllowFrom"] = storeAllowFrom
		*changes = append(*changes, prefix+"groupAllowFrom=pairing-store")
	}

	maybeApply("channels.whatsapp.", section)

	accounts, ok := section["accounts"].(map[string]interface{})
	if !ok || accounts == nil {
		return
	}
	for accountID, accountVal := range accounts {
		account, ok := accountVal.(map[string]interface{})
		if !ok || account == nil {
			continue
		}
		maybeApply(fmt.Sprintf("channels.whatsapp.accounts.%s.", accountID), account)
	}
}

// ---------- applyConfigFixes ----------

// applyConfigFixes 应用配置加固修复。
// TS 对照: fix.ts applyConfigFixes()
func applyConfigFixes(cfg map[string]interface{}) (map[string]interface{}, []string, map[string]bool) {
	// 深拷贝
	data, _ := json.Marshal(cfg)
	var next map[string]interface{}
	_ = json.Unmarshal(data, &next)
	if next == nil {
		next = make(map[string]interface{})
	}

	var changes []string
	policyFlips := make(map[string]bool)

	// logging.redactSensitive 修复
	if logging, ok := next["logging"].(map[string]interface{}); ok {
		if rs, ok := logging["redactSensitive"].(string); ok && rs == "off" {
			logging["redactSensitive"] = "tools"
			changes = append(changes, `logging.redactSensitive=off -> "tools"`)
		}
	}

	// 所有通道的 groupPolicy 修复
	for _, channel := range []string{
		"telegram", "whatsapp", "discord", "signal",
		"imessage", "slack", "msteams",
	} {
		setGroupPolicyAllowlist(next, channel, &changes, policyFlips)
	}

	return next, changes, policyFlips
}

// ---------- listDirectIncludes ----------

// listDirectIncludes 从解析后的配置中提取直接 include 路径。
// TS 对照: fix.ts listDirectIncludes()
func listDirectIncludes(parsed interface{}) []string {
	const includeKey = "$include"
	var out []string

	var visit func(value interface{})
	visit = func(value interface{}) {
		if value == nil {
			return
		}
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				visit(item)
			}
		case map[string]interface{}:
			if inc, ok := v[includeKey]; ok {
				switch iv := inc.(type) {
				case string:
					out = append(out, iv)
				case []interface{}:
					for _, item := range iv {
						if s, ok := item.(string); ok {
							out = append(out, s)
						}
					}
				}
			}
			for _, child := range v {
				visit(child)
			}
		}
	}
	visit(parsed)
	return out
}

// ---------- collectIncludePathsRecursive ----------

// MaxIncludeDepth 最大 include 递归深度。
const MaxIncludeDepth = 10

// collectIncludePathsRecursive 递归收集所有 include 文件路径。
// TS 对照: fix.ts collectIncludePathsRecursive()
func collectIncludePathsRecursive(configPath string, parsed interface{}) []string {
	visited := make(map[string]bool)
	var result []string

	var walk func(basePath string, parsed interface{}, depth int)
	walk = func(basePath string, parsed interface{}, depth int) {
		if depth > MaxIncludeDepth {
			return
		}
		for _, raw := range listDirectIncludes(parsed) {
			resolved := resolveIncludePath(basePath, raw)
			if visited[resolved] {
				continue
			}
			visited[resolved] = true
			result = append(result, resolved)

			data, err := os.ReadFile(resolved)
			if err != nil {
				continue
			}
			var nestedParsed interface{}
			if err := ParseJSONC(data, &nestedParsed); err != nil {
				continue
			}
			walk(resolved, nestedParsed, depth+1)
		}
	}

	walk(configPath, parsed, 0)
	return result
}

// resolveIncludePath 解析 include 路径。
// TS 对照: fix.ts resolveIncludePath()
func resolveIncludePath(baseConfigPath, includePath string) string {
	if filepath.IsAbs(includePath) {
		return filepath.Clean(includePath)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(baseConfigPath), includePath))
}

// ---------- chmodCredentialsAndAgentState ----------

// chmodCredentialsAndAgentState 批量修复凭证和 Agent 状态目录权限。
// TS 对照: fix.ts chmodCredentialsAndAgentState()
func chmodCredentialsAndAgentState(
	stateDir string,
	agentIDs []string,
	actions *[]SecurityFixAction,
) {
	// OAuth 目录
	credsDir := filepath.Join(stateDir, "oauth")
	*actions = append(*actions, applyPerms(credsDir, 0o700, "dir"))

	// OAuth 目录下的 JSON 文件
	entries, err := os.ReadDir(credsDir)
	if err == nil {
		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			p := filepath.Join(credsDir, e.Name())
			*actions = append(*actions, applyPerms(p, 0o600, "file"))
		}
	}

	// 各 Agent 目录
	seen := make(map[string]bool)
	for _, agentID := range agentIDs {
		if seen[agentID] {
			continue
		}
		seen[agentID] = true

		agentRoot := filepath.Join(stateDir, "agents", agentID)
		agentDir := filepath.Join(agentRoot, "agent")
		sessionsDir := filepath.Join(agentRoot, "sessions")

		*actions = append(*actions, applyPerms(agentRoot, 0o700, "dir"))
		*actions = append(*actions, applyPerms(agentDir, 0o700, "dir"))

		authPath := filepath.Join(agentDir, "auth-profiles.json")
		*actions = append(*actions, applyPerms(authPath, 0o600, "file"))

		*actions = append(*actions, applyPerms(sessionsDir, 0o700, "dir"))

		storePath := filepath.Join(sessionsDir, "sessions.json")
		*actions = append(*actions, applyPerms(storePath, 0o600, "file"))
	}
}

// ---------- FixSecurityFootguns ----------

// FixSecurityFootguns 自动修复安全问题主入口。
// TS 对照: fix.ts fixSecurityFootguns()
func FixSecurityFootguns(opts SecurityFixOptions) SecurityFixResult {
	stateDir := opts.StateDir
	configPath := opts.ConfigPath

	var actions []SecurityFixAction
	var errors []string
	var changes []string
	configWritten := false

	// 读取配置文件
	var cfg map[string]interface{}
	var parsed interface{}
	configExists := false

	data, err := os.ReadFile(configPath)
	if err == nil {
		configExists = true
		if err := ParseJSONC(data, &cfg); err != nil {
			errors = append(errors, fmt.Sprintf("config parse: %s", err))
			cfg = make(map[string]interface{})
		}
		_ = ParseJSONC(data, &parsed)
	} else {
		cfg = make(map[string]interface{})
	}

	// 配置加固
	if configExists {
		fixed, fixChanges, _ := applyConfigFixes(cfg)
		changes = fixChanges

		if len(fixChanges) > 0 {
			fixedData, err := json.MarshalIndent(fixed, "", "  ")
			if err == nil {
				if err := os.WriteFile(configPath, fixedData, 0o600); err == nil {
					configWritten = true
				} else {
					errors = append(errors, fmt.Sprintf("writeConfigFile: %s", err))
				}
			}
		}
	}

	// State 目录权限
	actions = append(actions, applyPerms(stateDir, 0o700, "dir"))

	// Config 文件权限
	actions = append(actions, applyPerms(configPath, 0o600, "file"))

	// Include 文件权限
	if configExists && parsed != nil {
		includePaths := collectIncludePathsRecursive(configPath, parsed)
		for _, p := range includePaths {
			actions = append(actions, applyPerms(p, 0o600, "file"))
		}
	}

	// 凭证和 Agent 状态目录
	agentIDs := resolveAgentIDsFromConfig(cfg)
	chmodCredentialsAndAgentState(stateDir, agentIDs, &actions)

	return SecurityFixResult{
		OK:            len(errors) == 0,
		StateDir:      stateDir,
		ConfigPath:    configPath,
		ConfigWritten: configWritten,
		Changes:       changes,
		Actions:       actions,
		Errors:        errors,
	}
}

// ---------- resolveAgentIDsFromConfig ----------

// resolveAgentIDsFromConfig 从配置中提取 Agent ID 列表。
func resolveAgentIDsFromConfig(cfg map[string]interface{}) []string {
	ids := make(map[string]bool)

	// 默认 agent
	if defaultID, ok := cfg["defaultAgentId"].(string); ok && defaultID != "" {
		ids[defaultID] = true
	} else {
		ids["default"] = true
	}

	// agents.list
	agents, ok := cfg["agents"].(map[string]interface{})
	if !ok {
		var idList []string
		for id := range ids {
			idList = append(idList, id)
		}
		return idList
	}

	list, ok := agents["list"].([]interface{})
	if !ok {
		var idList []string
		for id := range ids {
			idList = append(idList, id)
		}
		return idList
	}

	for _, entry := range list {
		agent, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := agent["id"].(string); ok {
			id = strings.TrimSpace(id)
			if id != "" {
				ids[id] = true
			}
		}
	}

	var result []string
	for id := range ids {
		result = append(result, id)
	}
	return result
}

// ---------- CollectFixableFindings ----------

// fixablePermCheckIDs 可修复的权限 checkID 集合。
var fixablePermCheckIDs = map[string]struct{}{
	"fs.state_dir.perms_world_writable":      {},
	"fs.state_dir.perms_group_writable":      {},
	"fs.state_dir.perms_readable":            {},
	"fs.config.perms_writable":               {},
	"fs.config.perms_world_readable":         {},
	"fs.config.perms_group_readable":         {},
	"fs.config_include.perms_writable":       {},
	"fs.config_include.perms_world_readable": {},
	"fs.config_include.perms_group_readable": {},
	"fs.credentials_dir.perms_writable":      {},
	"fs.credentials_dir.perms_readable":      {},
	"fs.auth_profiles.perms_writable":        {},
	"fs.auth_profiles.perms_readable":        {},
	"fs.sessions_store.perms_readable":       {},
	"fs.log_file.perms_readable":             {},
}

// CollectFixableFindings 从审计报告中提取可自动修复的发现。
// TS 对照: fix.ts collectFixableFindings()
func CollectFixableFindings(report *SecurityAuditReport) []SecurityAuditFinding {
	if report == nil {
		return nil
	}
	var fixable []SecurityAuditFinding
	for _, f := range report.Findings {
		if _, ok := fixablePermCheckIDs[f.CheckID]; ok {
			fixable = append(fixable, f)
		}
	}
	return fixable
}
