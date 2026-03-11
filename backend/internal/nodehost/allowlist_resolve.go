package nodehost

// allowlist_resolve.go — 审批配置解析 + Unix Socket IPC 审批请求
// 对应 TS: exec-approvals.ts L318-388 + L1476-1541

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// ---------- 常量 ----------

const (
	defaultExecSecurity    infra.ExecSecurity = infra.ExecSecurityDeny
	defaultExecAsk         infra.ExecAsk      = infra.ExecAskOnMiss
	defaultExecAskFallback infra.ExecSecurity = infra.ExecSecurityDeny
	defaultAutoAllowSkills                    = false
	defaultAgentID                            = "main"
)

// ---------- ExecApprovalsResolved ----------

// ExecApprovalsResolved 完整解析后的审批配置（含 agent + defaults + allowlist）。
// 对应 TS: ExecApprovalsResolved (L49-57)
type ExecApprovalsResolved struct {
	Path       string
	SocketPath string
	Token      string
	Defaults   ResolvedExecDefaults
	Agent      ResolvedExecDefaults
	Allowlist  []infra.ExecAllowlistEntry
	File       *infra.ExecApprovalsFile
}

// ResolvedExecDefaults 已解析的 defaults/agent 字段（全部非 nil）。
type ResolvedExecDefaults struct {
	Security        infra.ExecSecurity
	Ask             infra.ExecAsk
	AskFallback     infra.ExecSecurity
	AutoAllowSkills bool
}

// ---------- ResolveExecApprovalsFromFile ----------

// ResolveExecApprovalsFromFile 从已加载的 ExecApprovalsFile 解析完整配置。
// 支持 wildcard agent ("*")、agent 级覆盖、defaults 层级合并。
// 对应 TS: resolveExecApprovalsFromFile (L333-388)
func ResolveExecApprovalsFromFile(params struct {
	File       *infra.ExecApprovalsFile
	AgentID    string
	Overrides  *ExecApprovalsDefaultOverrides
	Path       string
	SocketPath string
	Token      string
}) ExecApprovalsResolved {
	file := params.File
	if file == nil {
		file = &infra.ExecApprovalsFile{Version: 1}
	}

	// overrides 优先级最低（作为 fallback）
	overrides := params.Overrides
	fallbackSecurity := defaultExecSecurity
	fallbackAsk := defaultExecAsk
	fallbackAskFallback := defaultExecAskFallback
	fallbackAutoAllowSkills := defaultAutoAllowSkills
	if overrides != nil {
		if overrides.Security != "" {
			fallbackSecurity = overrides.Security
		}
		if overrides.Ask != "" {
			fallbackAsk = overrides.Ask
		}
		if overrides.AskFallback != "" {
			fallbackAskFallback = overrides.AskFallback
		}
		if overrides.AutoAllowSkills != nil {
			fallbackAutoAllowSkills = *overrides.AutoAllowSkills
		}
	}

	// defaults 层
	var defs infra.ExecApprovalsDefaults
	if file.Defaults != nil {
		defs = *file.Defaults
	}
	resolvedDefaults := ResolvedExecDefaults{
		Security:        normalizeSecurity(defs.Security, fallbackSecurity),
		Ask:             normalizeAsk(defs.Ask, fallbackAsk),
		AskFallback:     normalizeSecurity(defs.AskFallback, fallbackAskFallback),
		AutoAllowSkills: normalizeBool(defs.AutoAllowSkills, fallbackAutoAllowSkills),
	}

	// agent 层（含 wildcard "*" 合并）
	agentKey := params.AgentID
	if agentKey == "" {
		agentKey = defaultAgentID
	}
	var agent, wildcard infra.ExecApprovalsAgent
	if file.Agents != nil {
		if a, ok := file.Agents[agentKey]; ok && a != nil {
			agent = *a
		}
		if w, ok := file.Agents["*"]; ok && w != nil {
			wildcard = *w
		}
	}
	resolvedAgent := ResolvedExecDefaults{
		Security: normalizeSecurity(
			firstNonEmptySecurity(agent.Security, wildcard.Security, resolvedDefaults.Security),
			resolvedDefaults.Security,
		),
		Ask: normalizeAsk(
			firstNonEmptyAsk(agent.Ask, wildcard.Ask, resolvedDefaults.Ask),
			resolvedDefaults.Ask,
		),
		AskFallback: normalizeSecurity(
			firstNonEmptySecurity(agent.AskFallback, wildcard.AskFallback, resolvedDefaults.AskFallback),
			resolvedDefaults.AskFallback,
		),
		AutoAllowSkills: normalizeBool(
			firstNonNilBool(agent.AutoAllowSkills, wildcard.AutoAllowSkills),
			resolvedDefaults.AutoAllowSkills,
		),
	}

	// allowlist = wildcard + agent（wildcard 在前）
	var allowlist []infra.ExecAllowlistEntry
	allowlist = append(allowlist, wildcard.Allowlist...)
	allowlist = append(allowlist, agent.Allowlist...)

	// socket 路径
	socketPath := params.SocketPath
	if socketPath == "" && file.Socket != nil {
		socketPath = file.Socket.Path
	}
	if socketPath == "" {
		socketPath = infra.ResolveExecApprovalsSocketPath()
	}

	token := params.Token
	if token == "" && file.Socket != nil {
		token = file.Socket.Token
	}

	filePath := params.Path
	if filePath == "" {
		filePath = infra.ResolveExecApprovalsPath()
	}

	return ExecApprovalsResolved{
		Path:       filePath,
		SocketPath: socketPath,
		Token:      token,
		Defaults:   resolvedDefaults,
		Agent:      resolvedAgent,
		Allowlist:  allowlist,
		File:       file,
	}
}

// ExecApprovalsDefaultOverrides 覆盖默认值（优先级低于文件配置）。
type ExecApprovalsDefaultOverrides struct {
	Security        infra.ExecSecurity
	Ask             infra.ExecAsk
	AskFallback     infra.ExecSecurity
	AutoAllowSkills *bool
}

// ---------- 辅助函数 ----------

func normalizeSecurity(v, fallback infra.ExecSecurity) infra.ExecSecurity {
	if normalized := infra.NormalizeExecSecurityValue(v); normalized != "" {
		return normalized
	}
	if normalized := infra.NormalizeExecSecurityValue(fallback); normalized != "" {
		return normalized
	}
	return defaultExecSecurity
}

func normalizeAsk(v, fallback infra.ExecAsk) infra.ExecAsk {
	switch v {
	case infra.ExecAskOff, infra.ExecAskOnMiss, infra.ExecAskAlways:
		return v
	}
	return fallback
}

func normalizeBool(v *bool, fallback bool) bool {
	if v != nil {
		return *v
	}
	return fallback
}

func firstNonEmptySecurity(vals ...infra.ExecSecurity) infra.ExecSecurity {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptyAsk(vals ...infra.ExecAsk) infra.ExecAsk {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonNilBool(vals ...*bool) *bool {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// MinSecurity 返回安全级别较低的一个。
// 对应 TS: minSecurity (L1466-1469)
func MinSecurity(a, b infra.ExecSecurity) infra.ExecSecurity {
	a = normalizeSecurity(a, defaultExecSecurity)
	b = normalizeSecurity(b, defaultExecSecurity)
	order := map[infra.ExecSecurity]int{
		infra.ExecSecurityDeny:      0,
		infra.ExecSecurityAllowlist: 1,
		infra.ExecSecuritySandboxed: 2,
		infra.ExecSecurityFull:      3,
	}
	if order[a] <= order[b] {
		return a
	}
	return b
}

// MaxAsk 返回询问策略较高的一个。
// 对应 TS: maxAsk (L1471-1474)
func MaxAsk(a, b infra.ExecAsk) infra.ExecAsk {
	order := map[infra.ExecAsk]int{
		infra.ExecAskOff:    0,
		infra.ExecAskOnMiss: 1,
		infra.ExecAskAlways: 2,
	}
	if order[a] >= order[b] {
		return a
	}
	return b
}

// ---------- Unix Socket IPC 审批请求 ----------

// ExecApprovalDecision 审批决定。
// 对应 TS: ExecApprovalDecision (L1476)
type ExecApprovalDecision string

const (
	ExecApprovalAllowOnce   ExecApprovalDecision = "allow-once"
	ExecApprovalAllowAlways ExecApprovalDecision = "allow-always"
	ExecApprovalDeny        ExecApprovalDecision = "deny"
)

// RequestExecApprovalViaSocket 通过 Unix socket 向审批服务请求决策。
// 协议：发送换行分隔的 JSON 请求，等待换行分隔的 JSON 响应。
// 对应 TS: requestExecApprovalViaSocket (L1478-1541)
func RequestExecApprovalViaSocket(ctx context.Context, socketPath, token string, request map[string]interface{}, timeoutMs int) (ExecApprovalDecision, error) {
	if socketPath == "" || token == "" {
		return "", fmt.Errorf("socket path and token are required")
	}
	if timeoutMs <= 0 {
		timeoutMs = 15_000
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	deadline := time.Now().Add(timeout)

	// 生成请求 ID
	idBuf := make([]byte, 16)
	_, _ = rand.Read(idBuf)
	reqID := fmt.Sprintf("%x-%x-%x-%x-%x",
		idBuf[0:4], idBuf[4:6], idBuf[6:8], idBuf[8:10], idBuf[10:])

	payload, err := json.Marshal(map[string]interface{}{
		"type":    "request",
		"token":   token,
		"id":      reqID,
		"request": request,
	})
	if err != nil {
		return "", fmt.Errorf("marshal approval request: %w", err)
	}

	// 连接 Unix socket
	var d net.Dialer
	connCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	conn, err := d.DialContext(connCtx, "unix", socketPath)
	if err != nil {
		return "", fmt.Errorf("connect approval socket: %w", err)
	}
	defer conn.Close()

	// 设置整体超时
	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("set socket deadline: %w", err)
	}

	// 发送请求（换行分隔）
	if _, err := fmt.Fprintf(conn, "%s\n", payload); err != nil {
		return "", fmt.Errorf("write approval request: %w", err)
	}

	// 读取响应（换行分隔 JSON）
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg struct {
			Type     string               `json:"type"`
			Decision ExecApprovalDecision `json:"decision"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type == "decision" && msg.Decision != "" {
			return msg.Decision, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read approval response: %w", err)
	}
	return "", fmt.Errorf("approval socket closed without decision")
}
