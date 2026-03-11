package gateway

// server_methods_exec_approvals.go — exec.approvals.* 方法处理器
// 对应 TS: src/gateway/server-methods/exec-approvals.ts
//
// 管理执行审批配置。exec.approvals.get/set 操作本地 JSON 文件。
// node 审批文件改为节点本地维护；Gateway 不再代理 exec.approvals.node.get/set。

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// ExecApprovalsHandlers 返回 exec.approvals.* 方法处理器映射。
func ExecApprovalsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"exec.approvals.get":      handleExecApprovalsGet,
		"exec.approvals.set":      handleExecApprovalsSet,
		"exec.approvals.node.get": handleExecApprovalsNodeGetUnsupported,
		"exec.approvals.node.set": handleExecApprovalsNodeSetUnsupported,
	}
}

// ---------- exec.approvals.get ----------

func handleExecApprovalsGet(ctx *MethodHandlerContext) {
	snapshot := infra.ReadExecApprovalsSnapshot()
	ctx.Respond(true, map[string]interface{}{
		"path":   snapshot.Path,
		"exists": snapshot.Exists,
		"hash":   snapshot.Hash,
		"file":   infra.RedactExecApprovals(snapshot.File),
	}, nil)
}

// ---------- exec.approvals.set ----------

func handleExecApprovalsSet(ctx *MethodHandlerContext) {
	// 提取 file 参数（在获取锁之前做参数校验，减少持锁时间）
	fileParam, ok := ctx.Params["file"]
	if !ok || fileParam == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "exec approvals file is required"))
		return
	}
	fileMap, ok := fileParam.(map[string]interface{})
	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "exec approvals file must be an object"))
		return
	}
	incoming := parseExecApprovalsFromMap(fileMap)

	// 获取 exec-approvals 锁保护整个 read-validate-write 过程
	unlock := infra.AcquireExecApprovalsLock()
	defer unlock()

	snapshot := infra.ReadExecApprovalsSnapshot()

	// 乐观并发控制：验证 baseHash
	if snapshot.Exists {
		if snapshot.Hash == "" {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
				"exec approvals base hash unavailable; re-run exec.approvals.get and retry"))
			return
		}

		baseHashRaw, _ := ctx.Params["baseHash"].(string)
		baseHash := strings.TrimSpace(baseHashRaw)
		if baseHash == "" {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
				"exec approvals base hash required; re-run exec.approvals.get and retry"))
			return
		}
		if baseHash != snapshot.Hash {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
				"exec approvals changed since last load; re-run exec.approvals.get and retry"))
			return
		}
	}

	if _, err := infra.EnsureExecApprovalsLocked(); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to ensure exec-approvals: "+err.Error()))
		return
	}
	snapshot = infra.ReadExecApprovalsSnapshot()

	// 构建更新后的 ExecApprovalsFile
	// 保留当前 socket path/token 作为回退
	currentSocketPath := ""
	currentToken := ""
	if snapshot.File != nil && snapshot.File.Socket != nil {
		currentSocketPath = snapshot.File.Socket.Path
		currentToken = snapshot.File.Socket.Token
	}

	socketPath := ""
	if incoming.Socket != nil && strings.TrimSpace(incoming.Socket.Path) != "" {
		socketPath = strings.TrimSpace(incoming.Socket.Path)
	} else if currentSocketPath != "" {
		socketPath = currentSocketPath
	} else {
		socketPath = infra.ResolveExecApprovalsSocketPath()
	}

	token := ""
	if incoming.Socket != nil && strings.TrimSpace(incoming.Socket.Token) != "" {
		token = strings.TrimSpace(incoming.Socket.Token)
	} else if currentToken != "" {
		token = currentToken
	}

	next := incoming
	next.Socket = &infra.ExecApprovalsSocket{
		Path:  socketPath,
		Token: token,
	}

	if err := infra.SaveExecApprovals(next); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save exec-approvals: "+err.Error()))
		return
	}

	// 重新读取以获取新 hash
	nextSnapshot := infra.ReadExecApprovalsSnapshot()
	ctx.Respond(true, map[string]interface{}{
		"path":   nextSnapshot.Path,
		"exists": nextSnapshot.Exists,
		"hash":   nextSnapshot.Hash,
		"file":   infra.RedactExecApprovals(nextSnapshot.File),
	}, nil)
}

// ---------- exec.approvals.node.get / node.set ----------
// node 审批文件由节点主机本地维护；Gateway 不做代理。

func handleExecApprovalsNodeGetUnsupported(ctx *MethodHandlerContext) {
	nodeId, _ := ctx.Params["nodeId"].(string)
	if strings.TrimSpace(nodeId) == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "nodeId required"))
		return
	}
	ctx.Respond(false, nil, NewErrorShape(
		ErrCodeUnsupportedFeature,
		"remote node exec approvals are maintained on the node host; gateway proxying is not supported",
	).WithDetails(map[string]interface{}{
		"method": "exec.approvals.node.get",
		"nodeId": strings.TrimSpace(nodeId),
	}))
}

func handleExecApprovalsNodeSetUnsupported(ctx *MethodHandlerContext) {
	nodeId, _ := ctx.Params["nodeId"].(string)
	if strings.TrimSpace(nodeId) == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "nodeId required"))
		return
	}
	ctx.Respond(false, nil, NewErrorShape(
		ErrCodeUnsupportedFeature,
		"remote node exec approvals are maintained on the node host; gateway proxying is not supported",
	).WithDetails(map[string]interface{}{
		"method": "exec.approvals.node.set",
		"nodeId": strings.TrimSpace(nodeId),
	}))
}

// ---------- 辅助：从 map 解析 ExecApprovalsFile ----------

func parseExecApprovalsFromMap(m map[string]interface{}) *infra.ExecApprovalsFile {
	file := &infra.ExecApprovalsFile{
		Version: 1,
	}

	// socket
	if socketMap, ok := m["socket"].(map[string]interface{}); ok {
		file.Socket = &infra.ExecApprovalsSocket{}
		if p, ok := socketMap["path"].(string); ok {
			file.Socket.Path = p
		}
		if t, ok := socketMap["token"].(string); ok {
			file.Socket.Token = t
		}
	}

	// defaults
	if defaultsMap, ok := m["defaults"].(map[string]interface{}); ok {
		file.Defaults = parseExecApprovalsDefaults(defaultsMap)
	}

	// agents
	if agentsMap, ok := m["agents"].(map[string]interface{}); ok {
		file.Agents = make(map[string]*infra.ExecApprovalsAgent, len(agentsMap))
		for key, val := range agentsMap {
			if agentMap, ok := val.(map[string]interface{}); ok {
				agent := &infra.ExecApprovalsAgent{}
				agent.ExecApprovalsDefaults = *parseExecApprovalsDefaults(agentMap)
				if allowlistRaw, ok := agentMap["allowlist"].([]interface{}); ok {
					for _, entryRaw := range allowlistRaw {
						if entryMap, ok := entryRaw.(map[string]interface{}); ok {
							entry := infra.ExecAllowlistEntry{}
							if id, ok := entryMap["id"].(string); ok {
								entry.ID = id
							}
							if pattern, ok := entryMap["pattern"].(string); ok {
								entry.Pattern = pattern
							}
							agent.Allowlist = append(agent.Allowlist, entry)
						}
					}
				}
				file.Agents[key] = agent
			}
		}
	}

	return file
}

func parseExecApprovalsDefaults(m map[string]interface{}) *infra.ExecApprovalsDefaults {
	d := &infra.ExecApprovalsDefaults{}
	if s, ok := m["security"].(string); ok {
		d.Security = infra.ExecSecurity(s)
	}
	if ef, ok := m["escalationFallback"].(string); ok {
		d.EscalationFallback = infra.ExecEscalationFallback(ef)
	}
	if a, ok := m["ask"].(string); ok {
		d.Ask = infra.ExecAsk(a)
	}
	if af, ok := m["askFallback"].(string); ok {
		d.AskFallback = infra.ExecSecurity(af)
	}
	if aas, ok := m["autoAllowSkills"].(bool); ok {
		d.AutoAllowSkills = &aas
	}
	return d
}
