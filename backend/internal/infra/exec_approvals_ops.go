package infra

// exec_approvals_ops.go — Exec Approvals 补齐操作
// 对应 TS: exec-approvals.ts normalizeExecApprovals, addAllowlistEntry,
//   recordAllowlistUse, requiresExecApproval

import (
	"strings"
	"time"
)

// NormalizeExecApprovals 填充默认值、确保 agents map 存在。
func NormalizeExecApprovals(file *ExecApprovalsFile) {
	if file == nil {
		return
	}
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Agents == nil {
		file.Agents = make(map[string]*ExecApprovalsAgent)
	}
	if file.Defaults == nil {
		file.Defaults = &ExecApprovalsDefaults{}
	}
}

// NormalizeExecSecurityValue 规范化安全级别别名和值。
func NormalizeExecSecurityValue(value ExecSecurity) ExecSecurity {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case "deny", "off":
		return ExecSecurityDeny
	case "allowlist":
		return ExecSecurityAllowlist
	case "sandbox", "sandboxed":
		return ExecSecuritySandboxed
	case "full":
		return ExecSecurityFull
	default:
		return ""
	}
}

// ResolveBaseExecSecurity 优先从 exec-approvals 读取持久化基础安全级别，
// 文件未设置时回落到调用方提供的配置值。
func ResolveBaseExecSecurity(configFallback string) ExecSecurity {
	snapshot := ReadExecApprovalsSnapshot()
	if snapshot != nil && snapshot.File != nil && snapshot.File.Defaults != nil {
		if level := NormalizeExecSecurityValue(snapshot.File.Defaults.Security); level != "" {
			return level
		}
	}
	if level := NormalizeExecSecurityValue(ExecSecurity(configFallback)); level != "" {
		return level
	}
	return ExecSecurityDeny
}

// AddAllowlistEntry 为指定 agent 添加白名单条目。
func AddAllowlistEntry(file *ExecApprovalsFile, agentID, pattern string) {
	NormalizeExecApprovals(file)
	if agentID == "" {
		agentID = "main"
	}
	agent, ok := file.Agents[agentID]
	if !ok {
		agent = &ExecApprovalsAgent{}
		file.Agents[agentID] = agent
	}
	// 去重
	for _, e := range agent.Allowlist {
		if e.Pattern == pattern {
			return
		}
	}
	agent.Allowlist = append(agent.Allowlist, ExecAllowlistEntry{
		Pattern: pattern,
	})
}

// RecordAllowlistUse 记录白名单条目使用情况。
func RecordAllowlistUse(file *ExecApprovalsFile, agentID string, pattern, command, resolvedPath string) {
	NormalizeExecApprovals(file)
	if agentID == "" {
		agentID = "main"
	}
	agent, ok := file.Agents[agentID]
	if !ok {
		return
	}
	now := time.Now().UnixMilli()
	for i, e := range agent.Allowlist {
		if e.Pattern == pattern {
			agent.Allowlist[i].LastUsedAt = &now
			agent.Allowlist[i].LastUsedCommand = command
			agent.Allowlist[i].LastResolvedPath = resolvedPath
			return
		}
	}
}

// RequiresExecApproval 判断是否需要执行审批。
func RequiresExecApproval(ask ExecAsk, security ExecSecurity, analysisOk, allowlistSatisfied bool) bool {
	if security == ExecSecurityFull {
		return false
	}
	if security == ExecSecurityDeny {
		return true
	}
	if security == ExecSecuritySandboxed {
		return false // L2: 沙箱内全权限，不需要逐次审批
	}
	// security == allowlist (L1)
	if allowlistSatisfied {
		return false
	}
	if ask == ExecAskAlways {
		return true
	}
	if ask == ExecAskOnMiss {
		return !analysisOk
	}
	// ask == off
	return !analysisOk
}
