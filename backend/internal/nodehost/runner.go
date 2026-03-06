package nodehost

// runner.go — Node Host 运行器主入口
// 对应 TS: runner.ts L560-651 (runNodeHost) + L1242-1308 (sendInvokeResult/sendNodeEvent)
//
// Node Host 是一个 WS 客户端进程，连接到 Gateway 后侦听 node.invoke.request 事件，
// 在本地执行命令并通过 node.invoke.result / node.event 返回结果。
//
// 注意：本文件定义核心结构和公共 API。实际的 WS 客户端连接由 gateway 包的
// GatewayClient 处理，此处定义 HandleInvoke 回调供 gateway 注册使用。

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"strings"

	"github.com/google/uuid"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

const (
	defaultNodeApprovalTimeoutMs        = 120_000
	defaultNodeApprovalRequestTimeoutMs = 130_000
)

// ---------- 公开 API ----------

// NodeHostService 封装 node host 运行时状态。
type NodeHostService struct {
	config    *Config
	skillBins *SkillBinsCache
	logger    *slog.Logger

	// 发送请求到 gateway 的回调（fire-and-forget，用于 node.invoke.result / node.event）
	sendRequest func(method string, params interface{}) error
	// 请求-响应回调（发送请求并等待解码响应到 result，用于 skills.bins 等）
	requestFunc func(method string, params interface{}, result interface{}) error

	// 浏览器代理（可选，通过 SetBrowserProxy 注入）
	browserProxy       BrowserProxyHandler
	browserProxyConfig BrowserProxyConfig
}

// RequestFunc 是请求-响应回调的签名。
// 发送 method + params，等待响应并解码到 result 指针。
type RequestFunc func(method string, params interface{}, result interface{}) error

// NewNodeHostService 创建 node host 服务。
// sendRequest: fire-and-forget 回调（node.invoke.result / node.event）
// reqFunc: 请求-响应回调（skills.bins 等需要等待响应的场景），可为 nil（降级为 sendRequest + 忽略响应）
func NewNodeHostService(cfg *Config, logger *slog.Logger, sendRequest func(string, interface{}) error, reqFunc RequestFunc) *NodeHostService {
	svc := &NodeHostService{
		config:      cfg,
		logger:      logger,
		sendRequest: sendRequest,
		requestFunc: reqFunc,
	}
	svc.skillBins = NewSkillBinsCache(func() ([]string, error) {
		// 通过 gateway 请求 skills.bins
		var result struct {
			Bins []interface{} `json:"bins"`
		}
		if err := svc.requestJSON("skills.bins", map[string]interface{}{}, &result); err != nil {
			return nil, err
		}
		bins := make([]string, 0, len(result.Bins))
		for _, b := range result.Bins {
			if s, ok := b.(string); ok {
				bins = append(bins, s)
			}
		}
		return bins, nil
	})
	return svc
}

// SetBrowserProxy 注入浏览器代理处理器。
func (s *NodeHostService) SetBrowserProxy(handler BrowserProxyHandler, cfg BrowserProxyConfig) {
	s.browserProxy = handler
	s.browserProxyConfig = cfg
}

// HandleInvoke 处理 node.invoke.request 事件。
// 实现命令分派：system.run / system.which / system.execApprovals.get/set / browser.proxy
func (s *NodeHostService) HandleInvoke(payload interface{}) {
	frame := CoerceNodeInvokePayload(payload)
	if frame == nil {
		return
	}

	command := strings.TrimSpace(frame.Command)
	s.logger.Debug("node invoke", "command", command, "id", frame.ID)

	switch command {
	case "system.execApprovals.get":
		s.handleExecApprovalsGet(frame)
	case "system.execApprovals.set":
		s.handleExecApprovalsSet(frame)
	case "system.which":
		s.handleSystemWhich(frame)
	case "system.run":
		s.handleSystemRun(frame)
	case "browser.proxy":
		s.handleBrowserProxy(frame)
	default:
		s.sendInvokeResult(frame, false, "", NewInvokeError("UNAVAILABLE", "command not supported").ToShape())
	}
}

// ---------- 命令处理器 ----------

func (s *NodeHostService) handleExecApprovalsGet(frame *NodeInvokeRequest) {
	if _, err := infra.EnsureExecApprovals(); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INTERNAL", Message: err.Error()})
		return
	}
	snapshot := infra.ReadExecApprovalsSnapshot()
	payload := map[string]interface{}{
		"path":   snapshot.Path,
		"exists": snapshot.Exists,
		"hash":   snapshot.Hash,
		"file":   infra.RedactExecApprovals(snapshot.File),
	}
	data, _ := json.Marshal(payload)
	s.sendInvokeResult(frame, true, string(data), nil)
}

func (s *NodeHostService) handleExecApprovalsSet(frame *NodeInvokeRequest) {
	var params ExecApprovalsSetParams
	if err := DecodeParams(frame.ParamsJSON, &params); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}
	if params.File == nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "exec approvals file required"})
		return
	}

	if _, err := infra.EnsureExecApprovals(); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INTERNAL", Message: err.Error()})
		return
	}

	snapshot := infra.ReadExecApprovalsSnapshot()

	// OCC: 验证 baseHash
	if snapshot.Exists {
		if snapshot.Hash == "" {
			s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "exec approvals base hash unavailable; reload and retry"})
			return
		}
		baseHash := ""
		if params.BaseHash != nil {
			baseHash = strings.TrimSpace(*params.BaseHash)
		}
		if baseHash == "" {
			s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "exec approvals base hash required; reload and retry"})
			return
		}
		if baseHash != snapshot.Hash {
			s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "exec approvals changed; reload and retry"})
			return
		}
	}

	// 序列化 → 保存 → 读取新 snapshot
	fileData, err := json.Marshal(params.File)
	if err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "invalid file format"})
		return
	}
	var incoming infra.ExecApprovalsFile
	if err := json.Unmarshal(fileData, &incoming); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "invalid file format"})
		return
	}

	// 保留 socket path + token
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

	incoming.Socket = &infra.ExecApprovalsSocket{Path: socketPath, Token: token}
	if err := infra.SaveExecApprovals(&incoming); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INTERNAL", Message: err.Error()})
		return
	}

	nextSnapshot := infra.ReadExecApprovalsSnapshot()
	payload := map[string]interface{}{
		"path":   nextSnapshot.Path,
		"exists": nextSnapshot.Exists,
		"hash":   nextSnapshot.Hash,
		"file":   infra.RedactExecApprovals(nextSnapshot.File),
	}
	data, _ := json.Marshal(payload)
	s.sendInvokeResult(frame, true, string(data), nil)
}

func (s *NodeHostService) handleSystemWhich(frame *NodeInvokeRequest) {
	var params SystemWhichParams
	if err := DecodeParams(frame.ParamsJSON, &params); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}
	if len(params.Bins) == 0 {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "bins required"})
		return
	}

	env := SanitizeEnv(nil)
	found := HandleSystemWhich(params.Bins, env)
	payload := map[string]interface{}{"bins": found}
	data, _ := json.Marshal(payload)
	s.sendInvokeResult(frame, true, string(data), nil)
}

func (s *NodeHostService) handleSystemRun(frame *NodeInvokeRequest) {
	var params SystemRunParams
	if err := DecodeParams(frame.ParamsJSON, &params); err != nil {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}

	if len(params.Command) == 0 {
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "INVALID_REQUEST", Message: "command required"})
		return
	}

	argv := make([]string, len(params.Command))
	copy(argv, params.Command)

	rawCommand := ""
	if params.RawCommand != nil {
		rawCommand = strings.TrimSpace(*params.RawCommand)
	}
	cmdText := rawCommand
	if cmdText == "" {
		cmdText = FormatCommand(argv)
	}

	agentID := StringOrDefault(params.AgentID, "main")
	sessionKey := StringOrDefault(params.SessionKey, "node")
	runID := StringOrDefault(params.RunID, uuid.NewString())
	env := SanitizeEnv(params.Env)
	cwd := ""
	if params.Cwd != nil {
		cwd = strings.TrimSpace(*params.Cwd)
	}

	// 安全检查: screenRecording
	if params.NeedsScreenRecording != nil && *params.NeedsScreenRecording {
		s.sendNodeEvent("exec.denied", BuildExecEventPayload(&ExecEventPayload{
			SessionKey: sessionKey, RunID: runID, Host: "node",
			Command: cmdText, Reason: "permission:screenRecording",
		}))
		s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "UNAVAILABLE", Message: "PERMISSION_MISSING: screenRecording"})
		return
	}

	// 节点本地审批文件是 system.run 的唯一 allowlist/ask 数据源。
	snapshot := infra.ReadExecApprovalsSnapshot()
	resolved := ResolveExecApprovalsFromFile(struct {
		File       *infra.ExecApprovalsFile
		AgentID    string
		Overrides  *ExecApprovalsDefaultOverrides
		Path       string
		SocketPath string
		Token      string
	}{
		File:    snapshot.File,
		AgentID: agentID,
		Path:    snapshot.Path,
	})

	hostSecurity := resolved.Agent.Security
	hostAsk := resolved.Agent.Ask

	if hostSecurity == infra.ExecSecurityDeny {
		s.denySystemRun(frame, sessionKey, runID, cmdText, "security:deny", "SYSTEM_RUN_DENIED: security=deny")
		return
	}

	safeBins := ResolveSafeBins(nil)
	var skillBins map[string]struct{}
	if resolved.Agent.AutoAllowSkills && s.skillBins != nil {
		skillBins = s.skillBins.Current(false)
	}
	analysis := EvaluateShellAllowlist(
		cmdText,
		resolved.Allowlist,
		safeBins,
		cwd,
		env,
		skillBins,
		resolved.Agent.AutoAllowSkills,
		currentNodePlatform(),
	)
	analysisOk := analysis.AnalysisOk
	allowlistSatisfied := false
	if hostSecurity == infra.ExecSecurityAllowlist && analysisOk {
		allowlistSatisfied = analysis.AllowlistSatisfied
	}

	approvedByAsk, approvalDecision := approvedSystemRunDecision(params)
	requiresAsk := RequiresExecApproval(hostAsk, hostSecurity, analysisOk, allowlistSatisfied)
	if requiresAsk && !approvedByAsk {
		decision, err := RequestExecApprovalViaSocket(
			context.Background(),
			resolved.SocketPath,
			resolved.Token,
			map[string]interface{}{
				"id":         runID,
				"command":    cmdText,
				"cwd":        cwd,
				"host":       "node",
				"security":   string(hostSecurity),
				"ask":        string(hostAsk),
				"agentId":    agentID,
				"sessionKey": sessionKey,
				"timeoutMs":  defaultNodeApprovalTimeoutMs,
			},
			defaultNodeApprovalRequestTimeoutMs,
		)
		if err != nil {
			s.denySystemRun(frame, sessionKey, runID, cmdText, "approval-request-failed", "SYSTEM_RUN_DENIED: approval request failed")
			return
		}

		switch decision {
		case ExecApprovalDeny:
			s.denySystemRun(frame, sessionKey, runID, cmdText, "user-denied", "SYSTEM_RUN_DENIED: user denied")
			return
		case ExecApprovalAllowAlways:
			approvedByAsk = true
			approvalDecision = decision
		case ExecApprovalAllowOnce:
			approvedByAsk = true
			approvalDecision = decision
		default:
			switch resolved.Agent.AskFallback {
			case infra.ExecSecurityFull:
				approvedByAsk = true
				approvalDecision = ExecApprovalAllowOnce
			case infra.ExecSecurityAllowlist:
				if analysisOk && allowlistSatisfied {
					approvedByAsk = true
					approvalDecision = ExecApprovalAllowOnce
				} else {
					s.denySystemRun(frame, sessionKey, runID, cmdText, "allowlist-miss", "SYSTEM_RUN_DENIED: allowlist miss")
					return
				}
			default:
				s.denySystemRun(frame, sessionKey, runID, cmdText, "approval-timeout", "SYSTEM_RUN_DENIED: approval timeout")
				return
			}
		}
	}

	if hostSecurity == infra.ExecSecurityAllowlist && (!analysisOk || !allowlistSatisfied) && !approvedByAsk {
		reason := "allowlist-miss"
		message := "SYSTEM_RUN_DENIED: allowlist miss"
		if !analysisOk {
			reason = "allowlist-analysis-failed"
			message = "SYSTEM_RUN_DENIED: allowlist analysis failed"
		}
		s.denySystemRun(frame, sessionKey, runID, cmdText, reason, message)
		return
	}

	if approvalDecision == ExecApprovalAllowAlways && hostSecurity == infra.ExecSecurityAllowlist {
		recordAllowAlwaysEntries(resolved.File, agentID, analysis.Segments)
	}
	recordAllowlistMatches(resolved.File, agentID, analysis.AllowlistMatches, cmdText, firstResolvedSegmentPath(analysis.Segments))

	// 执行命令
	timeoutMs := 0
	if params.TimeoutMs != nil {
		timeoutMs = *params.TimeoutMs
	}
	result := RunCommand(argv, cwd, env, timeoutMs)
	if result.Truncated {
		suffix := "... (truncated)"
		if strings.TrimSpace(result.Stderr) != "" {
			result.Stderr += "\n" + suffix
		} else {
			result.Stdout += "\n" + suffix
		}
	}

	combined := joinNonEmpty("\n", result.Stdout, result.Stderr, result.Error)

	timedOut := result.TimedOut
	success := result.Success
	s.sendNodeEvent("exec.finished", BuildExecEventPayload(&ExecEventPayload{
		SessionKey: sessionKey, RunID: runID, Host: "node",
		Command: cmdText, ExitCode: result.ExitCode,
		TimedOut: &timedOut, Success: &success, Output: combined,
	}))

	payload := map[string]interface{}{
		"exitCode": result.ExitCode,
		"timedOut": result.TimedOut,
		"success":  result.Success,
		"stdout":   result.Stdout,
		"stderr":   result.Stderr,
		"error":    nil,
	}
	if result.Error != "" {
		payload["error"] = result.Error
	}
	data, _ := json.Marshal(payload)
	s.sendInvokeResult(frame, true, string(data), nil)
}

func (s *NodeHostService) denySystemRun(
	frame *NodeInvokeRequest,
	sessionKey, runID, cmdText, reason, message string,
) {
	s.sendNodeEvent("exec.denied", BuildExecEventPayload(&ExecEventPayload{
		SessionKey: sessionKey,
		RunID:      runID,
		Host:       "node",
		Command:    cmdText,
		Reason:     reason,
	}))
	s.sendInvokeResult(frame, false, "", &InvokeErrorShape{Code: "FORBIDDEN", Message: message})
}

func approvedSystemRunDecision(params SystemRunParams) (bool, ExecApprovalDecision) {
	if params.Approved == nil || !*params.Approved {
		return false, ""
	}
	if params.ApprovalDecision == nil {
		return true, ExecApprovalAllowOnce
	}
	switch ExecApprovalDecision(strings.TrimSpace(*params.ApprovalDecision)) {
	case ExecApprovalAllowAlways:
		return true, ExecApprovalAllowAlways
	case ExecApprovalAllowOnce:
		return true, ExecApprovalAllowOnce
	default:
		return true, ExecApprovalAllowOnce
	}
}

func recordAllowAlwaysEntries(file *infra.ExecApprovalsFile, agentID string, segments []ExecCommandSegment) {
	seen := make(map[string]struct{})
	for _, seg := range segments {
		if seg.Resolution == nil || strings.TrimSpace(seg.Resolution.ResolvedPath) == "" {
			continue
		}
		resolvedPath := strings.TrimSpace(seg.Resolution.ResolvedPath)
		if _, ok := seen[resolvedPath]; ok {
			continue
		}
		seen[resolvedPath] = struct{}{}
		AddAllowlistEntry(file, agentID, resolvedPath)
	}
}

func recordAllowlistMatches(
	file *infra.ExecApprovalsFile,
	agentID string,
	matches []infra.ExecAllowlistEntry,
	command, resolvedPath string,
) {
	if file == nil || len(matches) == 0 {
		return
	}
	seen := make(map[string]struct{})
	for _, match := range matches {
		pattern := strings.TrimSpace(match.Pattern)
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		RecordAllowlistUse(file, agentID, match, command, resolvedPath)
	}
}

func firstResolvedSegmentPath(segments []ExecCommandSegment) string {
	for _, seg := range segments {
		if seg.Resolution == nil {
			continue
		}
		resolvedPath := strings.TrimSpace(seg.Resolution.ResolvedPath)
		if resolvedPath != "" {
			return resolvedPath
		}
	}
	return ""
}

// ---------- 通信辅助 ----------

func (s *NodeHostService) sendInvokeResult(frame *NodeInvokeRequest, ok bool, payloadJSON string, invokeErr *InvokeErrorShape) {
	result := BuildInvokeResult(frame, ok, payloadJSON, invokeErr)
	if err := s.sendRequest("node.invoke.result", result); err != nil {
		s.logger.Warn("failed to send invoke result", "id", frame.ID, "error", err)
	}
}

func (s *NodeHostService) sendNodeEvent(event string, payload interface{}) {
	var payloadJSON *string
	if payload != nil {
		data, err := json.Marshal(payload)
		if err == nil {
			s := string(data)
			payloadJSON = &s
		}
	}
	params := map[string]interface{}{
		"event":       event,
		"payloadJSON": payloadJSON,
	}
	if err := s.sendRequest("node.event", params); err != nil {
		s.logger.Warn("failed to send node event", "event", event, "error", err)
	}
}

func (s *NodeHostService) requestJSON(method string, params interface{}, result interface{}) error {
	if s.requestFunc != nil {
		return s.requestFunc(method, params, result)
	}
	// 降级：fire-and-forget（不解码响应）
	s.logger.Warn("requestJSON: no requestFunc, falling back to sendRequest", "method", method)
	return s.sendRequest(method, params)
}

// ---------- 内部辅助 ----------

func joinNonEmpty(sep string, parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, sep)
}

// IsDarwin 返回当前平台是否为 macOS。
func IsDarwin() bool {
	return runtime.GOOS == "darwin"
}

func currentNodePlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}
