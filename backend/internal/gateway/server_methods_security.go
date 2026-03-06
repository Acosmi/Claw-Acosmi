package gateway

// server_methods_security.go — security.* 方法处理器
// 提供安全级别的聚合查询 API，供前端安全设置页面使用。
//
// security.get 返回当前全局安全级别（从 exec-approvals.json 的 defaults.security 读取）。
// 写入操作复用 exec.approvals.set API。

import (
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// SecurityHandlers 返回 security.* 方法处理器映射。
func SecurityHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"security.get": handleSecurityGet,
	}
}

// ---------- security.get ----------
// 返回当前安全级别信息，供前端安全设置页面使用。

func handleSecurityGet(ctx *MethodHandlerContext) {
	if _, err := infra.EnsureExecApprovals(); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to ensure exec-approvals: "+err.Error()))
		return
	}

	snapshot := infra.ReadExecApprovalsSnapshot()
	file := snapshot.File

	// 解析当前安全级别
	currentLevel := string(infra.ExecSecurityDeny)
	if file != nil && file.Defaults != nil && file.Defaults.Security != "" {
		currentLevel = string(file.Defaults.Security)
	}

	// 判断是否为永久授权（full 模式）
	isPermanentFull := currentLevel == string(infra.ExecSecurityFull)

	// 构建安全级别描述（L0-L3 四层模型）
	levels := []map[string]interface{}{
		{
			"id":            string(infra.ExecSecurityDeny),
			"label":         "L0 — Read Only",
			"labelZh":       "L0 — 只读",
			"description":   "Agent can only read files and analyze code. No write or execute permissions.",
			"descriptionZh": "智能体只能读取文件和分析代码。没有写入或执行权限。",
			"risk":          "low",
			"active":        currentLevel == string(infra.ExecSecurityDeny),
		},
		{
			"id":            string(infra.ExecSecurityAllowlist),
			"label":         "L1 — Allowlist",
			"labelZh":       "L1 — 工作区受限",
			"description":   "Agent can execute pre-approved commands from the allowlist. Sandbox enforced, no network.",
			"descriptionZh": "智能体可以执行允许列表中预批准的命令。强制沙箱，无网络。",
			"risk":          "medium",
			"active":        currentLevel == string(infra.ExecSecurityAllowlist),
		},
		{
			"id":            string(infra.ExecSecuritySandboxed),
			"label":         "L2 — Sandboxed Full",
			"labelZh":       "L2 — 沙箱全权限",
			"description":   "Full permissions within sandbox. No network. Optional host directory mounts with approval.",
			"descriptionZh": "沙箱内全权限执行，无网络。可按审批挂载宿主机目录。",
			"risk":          "high",
			"active":        currentLevel == string(infra.ExecSecuritySandboxed),
		},
		{
			"id":            string(infra.ExecSecurityFull),
			"label":         "L3 — Bare Machine Full",
			"labelZh":       "L3 — 裸机全权限",
			"description":   "Unrestricted host access with full network. All tool calls audited. Temporary escalation grants are capped at 60 minutes.",
			"descriptionZh": "宿主机无限制访问，网络全开。所有工具调用记录审计日志。临时提权授权最长 60 分钟。",
			"risk":          "critical",
			"active":        currentLevel == string(infra.ExecSecurityFull),
		},
	}

	ctx.Respond(true, map[string]interface{}{
		"currentLevel":    currentLevel,
		"isPermanentFull": isPermanentFull,
		"levels":          levels,
		"hash":            snapshot.Hash,
	}, nil)
}
