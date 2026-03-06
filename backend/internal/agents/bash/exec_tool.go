// bash/exec_tool.go — createExecTool 工厂等价实现。
// TS 参考：src/agents/bash-tools.exec.ts L800-1631
//
// 包含 ExecToolConfig 构造 + Execute 主入口 + host 路由（sandbox/gateway/node）
// + elevated 提权 + yieldMs/后台化 + 审批流。
package bash

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/tools"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/internal/nodehost"
	"github.com/google/uuid"
)

// ========== ExecToolConfig ==========

// ExecToolConfig 从 ExecToolDefaults 解析出的运行时配置。
// 对应 TS: createExecTool 闭包内的局部变量 L804-824
type ExecToolConfig struct {
	defaultBackgroundMs     int
	allowBackground         bool
	defaultTimeoutSec       int
	defaultPathPrepend      []string
	safeBins                map[string]struct{}
	notifyOnExit            bool
	notifySessionKey        string
	approvalRunningNoticeMs int
	agentID                 string
	defaults                *ExecToolDefaults

	// 依赖注入 — 调用方设置
	NodeLoader  tools.NodeLoader
	GatewayOpts tools.GatewayOptions
}

// ExecToolParams execute 方法的入参。
// 对应 TS: createExecTool.execute params L833-846
type ExecToolParams struct {
	Command    string            `json:"command"`
	Workdir    string            `json:"workdir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	YieldMs    *int              `json:"yieldMs,omitempty"`
	Background bool              `json:"background,omitempty"`
	Timeout    *int              `json:"timeout,omitempty"`
	PTY        bool              `json:"pty,omitempty"`
	Elevated   *bool             `json:"elevated,omitempty"`
	Host       string            `json:"host,omitempty"`
	Security   string            `json:"security,omitempty"`
	Ask        string            `json:"ask,omitempty"`
	Node       string            `json:"node,omitempty"`
}

// ExecToolResult execute 方法的返回值。
type ExecToolResult struct {
	Text    string          `json:"text"`
	Details ExecToolDetails `json:"details"`
}

// NewExecToolConfig 创建 ExecToolConfig。
// 对应 TS: createExecTool L800-825
func NewExecToolConfig(defaults *ExecToolDefaults) *ExecToolConfig {
	if defaults == nil {
		defaults = &ExecToolDefaults{}
	}

	var bgMsPtr *int
	if defaults.BackgroundMs != 0 {
		bgMsPtr = &defaults.BackgroundMs
	} else if envVal, ok := ReadEnvInt("PI_BASH_YIELD_MS"); ok {
		bgMsPtr = &envVal
	}
	bgMs := ClampNumber(bgMsPtr, 10_000, 10, 120_000)

	allowBg := true
	if defaults.BackgroundMs != 0 || defaults.AllowBackground {
		allowBg = defaults.AllowBackground
	}

	timeoutSec := defaults.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}

	safeBins := nodehost.ResolveSafeBins(defaults.SafeBins)
	sk := strings.TrimSpace(defaults.SessionKey)

	approvalMs := ResolveApprovalRunningNoticeMs(defaults.ApprovalRunningNoticeMs)

	parsed := ParseAgentSessionKey(defaults.SessionKey)
	agentID := defaults.AgentID
	if agentID == "" && parsed != nil {
		agentID = ResolveAgentIdFromSessionKey(defaults.SessionKey)
	}

	return &ExecToolConfig{
		defaultBackgroundMs:     bgMs,
		allowBackground:         allowBg,
		defaultTimeoutSec:       timeoutSec,
		defaultPathPrepend:      NormalizePathPrepend(defaults.PathPrepend),
		safeBins:                safeBins,
		notifyOnExit:            defaults.NotifyOnExit,
		notifySessionKey:        sk,
		approvalRunningNoticeMs: approvalMs,
		agentID:                 agentID,
		defaults:                defaults,
	}
}

// ========== 辅助函数 ==========

// coalesce 返回第一个非零 int。
func coalesce(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

// ClampNumber 和 ReadEnvInt 已在 shared.go 中定义，此处不重复。

// BuildNodeShellCommand 构建节点 shell 命令 argv。
// TS 参考: src/infra/node-shell.ts
func BuildNodeShellCommand(command, platform string) []string {
	norm := strings.TrimSpace(strings.ToLower(platform))
	if strings.HasPrefix(norm, "win") {
		return []string{"cmd.exe", "/d", "/s", "/c", command}
	}
	return []string{"/bin/sh", "-lc", command}
}

// currentPlatform 返回当前运行时平台（对齐 TS process.platform）。
func currentPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// 确保 math 包被使用
var _ = math.IsInf

// ========== Execute 主入口 ==========

// Execute 执行 bash 命令。
// 对应 TS: createExecTool.execute L833-1628
func (c *ExecToolConfig) Execute(ctx context.Context, params ExecToolParams) (*ExecToolResult, error) {
	command := strings.TrimSpace(params.Command)
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// 1. elevated 判断 (TS L907-935)
	elevatedRequested := params.Elevated != nil && *params.Elevated
	var elevatedMode string
	if elevatedRequested {
		elev := c.defaults.Elevated
		if elev == nil || !elev.Enabled {
			return nil, fmt.Errorf("elevated execution is not enabled")
		}
		if !elev.Allowed {
			return nil, fmt.Errorf("elevated execution is not allowed")
		}
		elevatedMode = elev.DefaultLevel // "on"|"off"|"ask"|"full"
		if elevatedMode == "" || elevatedMode == "off" {
			elevatedRequested = false
		}
	}

	configuredHost := c.defaults.Host
	if configuredHost == "" {
		configuredHost = ExecHostSandbox
	}
	requestedHost := NormalizeExecHost(params.Host)
	host := configuredHost
	if requestedHost != "" {
		host = requestedHost
	}

	// host mismatch 检查 (TS L927-932)
	if !elevatedRequested && requestedHost != "" && requestedHost != configuredHost {
		return nil, fmt.Errorf(
			"exec host not allowed (requested %s; configure tools.exec.host=%s to allow)",
			RenderExecHostLabel(requestedHost), RenderExecHostLabel(configuredHost))
	}
	if elevatedRequested {
		host = ExecHostGateway
	}

	// 2. security / ask 合并 (TS L937-949)
	configuredSecurity := c.defaults.Security
	if configuredSecurity == "" {
		if host == ExecHostSandbox {
			configuredSecurity = ExecSecurityDeny
		} else {
			configuredSecurity = ExecSecurityAllowlist
		}
	}
	requestSec := NormalizeExecSecurity(params.Security)
	security := configuredSecurity
	if requestSec != "" {
		security = ExecSecurity(nodehost.MinSecurity(infra.ExecSecurity(configuredSecurity), infra.ExecSecurity(requestSec)))
	}
	if elevatedRequested && elevatedMode == "full" {
		security = ExecSecurityFull
	}

	configuredAsk := c.defaults.Ask
	if configuredAsk == "" {
		configuredAsk = ExecAsk(infra.ExecAskOnMiss)
	}
	requestAsk := NormalizeExecAsk(params.Ask)
	ask := configuredAsk
	if requestAsk != "" {
		ask = ExecAsk(nodehost.MaxAsk(infra.ExecAsk(configuredAsk), infra.ExecAsk(requestAsk)))
	}
	bypassApprovals := elevatedRequested && elevatedMode == "full"
	if bypassApprovals {
		ask = ExecAsk(infra.ExecAskOff)
	}

	// 3. sandbox / workdir (TS L951-992)
	var warnings []string
	sandbox := func() *BashSandboxConfig {
		if host == ExecHostSandbox {
			return c.defaults.Sandbox
		}
		return nil
	}()
	rawWorkdir := strings.TrimSpace(params.Workdir)
	if rawWorkdir == "" {
		rawWorkdir = c.defaults.Cwd
	}
	cwd := rawWorkdir

	// 4. env (TS L982-993)
	env := make(map[string]string)
	for k, v := range params.Env {
		env[k] = v
	}
	if host != ExecHostSandbox && sandbox == nil {
		ApplyShellPath(env, "")
	}
	ApplyPathPrepend(env, c.defaultPathPrepend, false)

	// 5. 超时
	timeoutSec := c.defaultTimeoutSec
	if params.Timeout != nil && *params.Timeout > 0 {
		timeoutSec = *params.Timeout
	}

	// 6. host 路由
	execCtx := &execContext{
		command: command, cwd: cwd, env: env, sandbox: sandbox,
		timeoutSec: timeoutSec, warnings: warnings,
		security: security, ask: ask, bypassApprovals: bypassApprovals,
	}
	switch host {
	case ExecHostNode:
		return c.executeNode(ctx, params, execCtx)
	case ExecHostGateway:
		if !bypassApprovals {
			return c.executeGateway(ctx, params, execCtx)
		}
		// bypassApprovals：跳过审批，直接本地执行
		return c.executeLocal(ctx, params, execCtx)
	default:
		return c.executeLocal(ctx, params, execCtx)
	}
}

// execContext 各分支共享的执行上下文。
type execContext struct {
	command         string
	cwd             string
	env             map[string]string
	sandbox         *BashSandboxConfig
	timeoutSec      int
	warnings        []string
	security        ExecSecurity
	ask             ExecAsk
	bypassApprovals bool
}

// ========== executeLocal ==========

// executeLocal 本地/sandbox 执行。
// 对应 TS: createExecTool.execute sandbox 分支 + 最终 runExecProcess 路径 L1500-1628
func (c *ExecToolConfig) executeLocal(
	ctx context.Context,
	params ExecToolParams,
	ec *execContext,
) (*ExecToolResult, error) {
	command := ec.command
	cwd := ec.cwd
	env := ec.env
	warnings := ec.warnings
	timeoutSec := ec.timeoutSec

	// sandbox workdir
	sandbox := ec.sandbox
	var containerWorkdir string
	if sandbox != nil {
		hostCwd, ctrCwd := ResolveSandboxWorkdir(cwd, *sandbox, &warnings)
		cwd = hostCwd
		containerWorkdir = ctrCwd
	} else {
		cwd = ResolveWorkdir(cwd, &warnings)
	}

	// yieldMs / background
	bgMs := c.defaultBackgroundMs
	if params.YieldMs != nil {
		bgMs = *params.YieldMs
	}
	background := params.Background

	// 调用 RunExecProcess
	handle, err := RunExecProcess(ctx, RunExecProcessOpts{
		Command:          command,
		Workdir:          cwd,
		Env:              env,
		Sandbox:          sandbox,
		ContainerWorkdir: containerWorkdir,
		UsePTY:           params.PTY,
		Warnings:         &warnings,
		MaxOutput:        DefaultMaxOutputLen,
		PendingMaxOutput: ResolveBashPendingMaxOutputChars(),
		NotifyOnExit:     c.notifyOnExit,
		ScopeKey:         c.defaults.ScopeKey,
		SessionKey:       c.defaults.SessionKey,
		TimeoutSec:       timeoutSec,
	})
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	// GAP-5: warnings 前缀
	warningPrefix := getWarningText(warnings)

	// 后台化：不等结果
	if background && c.allowBackground {
		DefaultRegistry.MarkBackgrounded(handle.Session)
		return &ExecToolResult{
			Text: fmt.Sprintf("%sCommand started in background (session=%s, pid=%d)", warningPrefix, handle.Session.ID, handle.PID),
			Details: ExecToolDetails{
				Status:    "running",
				SessionID: handle.Session.ID,
				PID:       handle.PID,
				StartedAt: handle.StartedAt,
				Cwd:       cwd,
				Host:      c.defaults.Host,
				Command:   command,
			},
		}, nil
	}

	// yieldMs：等待部分时间后 yield (TS L1564-1586 onYieldNow)
	yielded := false
	if bgMs > 0 && c.allowBackground {
		timer := time.NewTimer(time.Duration(bgMs) * time.Millisecond)
		select {
		case <-handle.Done:
			timer.Stop()
			// 已完成，走下面的结果收集
		case <-timer.C:
			// 超时还在运行 → 后台化 + yield (TS L1577-1584)
			yielded = true
			DefaultRegistry.MarkBackgrounded(handle.Session)
			return &ExecToolResult{
				Text: fmt.Sprintf("%sCommand still running after %dms (session=%s)", warningPrefix, bgMs, handle.Session.ID),
				Details: ExecToolDetails{
					Status:    "running",
					SessionID: handle.Session.ID,
					PID:       handle.PID,
					StartedAt: handle.StartedAt,
					Cwd:       cwd,
					Host:      c.defaults.Host,
					Command:   command,
				},
			}, nil
		case <-ctx.Done():
			// GAP-8: abort 时检查 backgrounded 状态 (TS L1527-1532)
			if !handle.Session.Backgrounded {
				handle.Kill()
			}
			return nil, ctx.Err()
		}
	} else {
		// 同步等待
		select {
		case <-handle.Done:
		case <-ctx.Done():
			// GAP-8: abort 时检查 backgrounded 状态
			if !handle.Session.Backgrounded {
				handle.Kill()
			}
			return nil, ctx.Err()
		}
	}
	_ = yielded // 防止未使用警告

	// 收集结果 (TS L1588-1614)
	return c.buildCompletedResult(handle, cwd, command, warningPrefix)
}

// getWarningText 构建 warnings 前缀文本。
// 对应 TS: getWarningText (L1508-1510)
func getWarningText(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return strings.Join(warnings, "\n") + "\n\n"
}

// buildCompletedResult 从已完成的进程句柄构建结果。
// 对应 TS: run.promise.then outcome 处理 (L1588-1614)
func (c *ExecToolConfig) buildCompletedResult(handle *ExecProcessHandle, cwd, command, warningPrefix string) (*ExecToolResult, error) {
	session := handle.Session
	stdout, stderr := DefaultRegistry.DrainSession(session)

	exitCode := 0
	if session.ExitCode != nil {
		exitCode = *session.ExitCode
	}

	// 如果已 backgrounded，不再输出结果 (TS L1593-1594)
	if session.Backgrounded {
		return nil, nil
	}

	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}

	aggregated := stdout
	if stderr != "" {
		if aggregated != "" {
			aggregated += "\n"
		}
		aggregated += stderr
	}
	aggregated = truncateOutput(aggregated, DefaultMaxOutputLen)

	displayText := aggregated
	if displayText == "" {
		displayText = "(no output)"
	}

	return &ExecToolResult{
		Text: warningPrefix + displayText,
		Details: ExecToolDetails{
			Status:     status,
			SessionID:  session.ID,
			PID:        handle.PID,
			StartedAt:  handle.StartedAt,
			Cwd:        cwd,
			ExitCode:   session.ExitCode,
			DurationMs: time.Since(time.UnixMilli(handle.StartedAt)).Milliseconds(),
			Aggregated: aggregated,
			Host:       c.defaults.Host,
			Command:    command,
		},
	}, nil
}

// ========== executeGateway ==========

// executeGateway Gateway host 执行路径。
// 对应 TS: createExecTool.execute gateway 分支 L1273-1500
func (c *ExecToolConfig) executeGateway(
	ctx context.Context,
	params ExecToolParams,
	ec *execContext,
) (*ExecToolResult, error) {
	command := ec.command
	cwd := ec.cwd
	env := ec.env
	warnings := ec.warnings

	// 1. 验证 host env
	if err := ValidateHostEnv(params.Env); err != nil {
		return nil, err
	}

	// 2. 解析审批配置 (TS L1274)
	snapshot := infra.ReadExecApprovalsSnapshot()
	resolved := nodehost.ResolveExecApprovalsFromFile(struct {
		File       *infra.ExecApprovalsFile
		AgentID    string
		Overrides  *nodehost.ExecApprovalsDefaultOverrides
		Path       string
		SocketPath string
		Token      string
	}{
		File:    snapshot.File,
		AgentID: c.agentID,
		Path:    snapshot.Path,
	})

	// 3. 安全级别合并 (TS L1274-1277)
	hostSecurity := nodehost.MinSecurity(infra.ExecSecurity(ec.security), resolved.Agent.Security)
	hostAsk := nodehost.MaxAsk(infra.ExecAsk(ec.ask), resolved.Agent.Ask)
	askFallback := resolved.Agent.AskFallback

	// 4. deny 检查 (TS L1278-1280)
	if hostSecurity == infra.ExecSecurityDeny {
		return nil, fmt.Errorf("exec denied: host=gateway security=deny")
	}

	// 5. allowlist 评估 (TS L1281-1298)
	analysis := nodehost.EvaluateShellAllowlist(
		command,
		resolved.Allowlist,
		c.safeBins,
		cwd,
		env,
		nil, // skillBins
		resolved.Agent.AutoAllowSkills,
		currentPlatform(),
	)
	allowlistMatches := analysis.AllowlistMatches
	analysisOk := analysis.AnalysisOk
	allowlistSatisfied := false
	if hostSecurity == infra.ExecSecurityAllowlist && analysisOk {
		allowlistSatisfied = analysis.AllowlistSatisfied
	}

	// 6. 审批检查 (TS L1293-1298)
	requiresAsk := nodehost.RequiresExecApproval(hostAsk, hostSecurity, analysisOk, allowlistSatisfied)

	if requiresAsk {
		// TS L1300-1477: 异步 fire-and-forget 审批
		approvalID := uuid.NewString()
		approvalSlug := CreateApprovalSlug(approvalID)
		expiresAtMs := time.Now().UnixMilli() + DefaultApprovalTimeoutMs
		contextKey := "exec:" + approvalID
		noticeSeconds := math.Max(1, math.Round(float64(c.approvalRunningNoticeMs)/1000))
		warningText := getWarningText(warnings)

		// 解析 resolvedPath
		var resolvedPath string
		if len(analysis.Segments) > 0 && analysis.Segments[0].Resolution != nil {
			resolvedPath = analysis.Segments[0].Resolution.ResolvedPath
		}
		effectiveTimeout := ec.timeoutSec

		// 后台 goroutine: 请求审批 → 执行 (TS L1312-1457)
		go func() {
			// 请求审批决定 (TS L1314-1342)
			decision, err := nodehost.RequestExecApprovalViaSocket(
				context.Background(),
				resolved.SocketPath,
				resolved.Token,
				map[string]interface{}{
					"id":           approvalID,
					"command":      command,
					"cwd":          cwd,
					"host":         "gateway",
					"security":     string(hostSecurity),
					"ask":          string(hostAsk),
					"agentId":      c.agentID,
					"resolvedPath": resolvedPath,
					"sessionKey":   c.defaults.SessionKey,
					"timeoutMs":    DefaultApprovalTimeoutMs,
				},
				DefaultApprovalRequestTimeoutMs,
			)
			if err != nil {
				EmitExecSystemEvent(
					fmt.Sprintf("Exec denied (gateway id=%s, approval-request-failed): %s", approvalID, command),
					c.notifySessionKey, contextKey,
				)
				return
			}

			// 决策处理 (TS L1344-1389)
			approvedByAsk := false
			deniedReason := ""

			switch decision {
			case nodehost.ExecApprovalDeny:
				deniedReason = "user-denied"
			case "":
				// 超时：使用 askFallback (TS L1349-1360)
				switch askFallback {
				case infra.ExecSecurityFull:
					approvedByAsk = true
				case infra.ExecSecurityAllowlist:
					if !analysisOk || !allowlistSatisfied {
						deniedReason = "approval-timeout (allowlist-miss)"
					} else {
						approvedByAsk = true
					}
				default:
					deniedReason = "approval-timeout"
				}
			case nodehost.ExecApprovalAllowOnce:
				approvedByAsk = true
			case nodehost.ExecApprovalAllowAlways:
				approvedByAsk = true
				// 添加到 allowlist (TS L1365-1372)
				if hostSecurity == infra.ExecSecurityAllowlist {
					for _, seg := range analysis.Segments {
						if seg.Resolution != nil && seg.Resolution.ResolvedPath != "" {
							nodehost.AddAllowlistEntry(resolved.File, c.agentID, seg.Resolution.ResolvedPath)
						}
					}
				}
			}

			// 最终 allowlist 检查 (TS L1375-1381)
			if hostSecurity == infra.ExecSecurityAllowlist && (!analysisOk || !allowlistSatisfied) && !approvedByAsk {
				if deniedReason == "" {
					deniedReason = "allowlist-miss"
				}
			}

			if deniedReason != "" {
				EmitExecSystemEvent(
					fmt.Sprintf("Exec denied (gateway id=%s, %s): %s", approvalID, deniedReason, command),
					c.notifySessionKey, contextKey,
				)
				return
			}

			// 记录 allowlist 使用 (TS L1391-1406)
			if len(allowlistMatches) > 0 {
				seen := make(map[string]bool)
				for _, match := range allowlistMatches {
					if seen[match.Pattern] {
						continue
					}
					seen[match.Pattern] = true
					nodehost.RecordAllowlistUse(resolved.File, c.agentID, match, command, resolvedPath)
				}
			}

			// 启动本地进程 (TS L1408-1431)
			handle, err := RunExecProcess(context.Background(), RunExecProcessOpts{
				Command:          command,
				Workdir:          cwd,
				Env:              env,
				UsePTY:           params.PTY && ec.sandbox == nil,
				Warnings:         &warnings,
				MaxOutput:        DefaultMaxOutputLen,
				PendingMaxOutput: ResolveBashPendingMaxOutputChars(),
				NotifyOnExit:     false,
				ScopeKey:         c.defaults.ScopeKey,
				SessionKey:       c.notifySessionKey,
				TimeoutSec:       effectiveTimeout,
			})
			if err != nil {
				EmitExecSystemEvent(
					fmt.Sprintf("Exec denied (gateway id=%s, spawn-failed): %s", approvalID, command),
					c.notifySessionKey, contextKey,
				)
				return
			}

			// 标记后台化 (TS L1433)
			DefaultRegistry.MarkBackgrounded(handle.Session)

			// running 通知计时器 (TS L1435-1443)
			var runningTimer *time.Timer
			if c.approvalRunningNoticeMs > 0 {
				runningTimer = time.AfterFunc(time.Duration(c.approvalRunningNoticeMs)*time.Millisecond, func() {
					EmitExecSystemEvent(
						fmt.Sprintf("Exec running (gateway id=%s, session=%s, >%.0fs): %s",
							approvalID, handle.Session.ID, noticeSeconds, command),
						c.notifySessionKey, contextKey,
					)
				})
			}

			// 等结果 (TS L1445-1456)
			<-handle.Done
			if runningTimer != nil {
				runningTimer.Stop()
			}
			stdout, stderr := DefaultRegistry.DrainSession(handle.Session)
			aggregated := stdout
			if stderr != "" {
				if aggregated != "" {
					aggregated += "\n"
				}
				aggregated += stderr
			}
			output := NormalizeNotifyOutput(Tail(aggregated, DefaultNotifyTailChars))
			exitLabel := fmt.Sprintf("code %d", func() int {
				if handle.Session.ExitCode != nil {
					return *handle.Session.ExitCode
				}
				return -1
			}())
			timedOut := handle.Session.ExitCode != nil && *handle.Session.ExitCode == -1
			if timedOut {
				exitLabel = "timeout"
			}
			summary := fmt.Sprintf("Exec finished (gateway id=%s, session=%s, %s)", approvalID, handle.Session.ID, exitLabel)
			if output != "" {
				summary += "\n" + output
			}
			EmitExecSystemEvent(summary, c.notifySessionKey, contextKey)
		}()

		// 立即返回 approval-pending (TS L1459-1477)
		return &ExecToolResult{
			Text: fmt.Sprintf("%sApproval required (id %s). Approve to run; updates will arrive after completion.", warningText, approvalSlug),
			Details: ExecToolDetails{
				Status:       "approval-pending",
				ApprovalID:   approvalID,
				ApprovalSlug: approvalSlug,
				ExpiresAtMs:  expiresAtMs,
				Host:         ExecHostGateway,
				Command:      command,
				Cwd:          cwd,
			},
		}, nil
	}

	// GAP-7: 非审批路径的 allowlist-miss deny (TS L1480-1482)
	if hostSecurity == infra.ExecSecurityAllowlist && (!analysisOk || !allowlistSatisfied) {
		return nil, fmt.Errorf("exec denied: allowlist miss")
	}

	// 记录 allowlist 使用 (TS L1484-1499)
	if len(allowlistMatches) > 0 {
		seen := make(map[string]bool)
		for _, match := range allowlistMatches {
			if seen[match.Pattern] {
				continue
			}
			seen[match.Pattern] = true
			var rp string
			if len(analysis.Segments) > 0 && analysis.Segments[0].Resolution != nil {
				rp = analysis.Segments[0].Resolution.ResolvedPath
			}
			nodehost.RecordAllowlistUse(resolved.File, c.agentID, match, command, rp)
		}
	}

	// 无需审批 → 直接本地执行 (TS 分支落到 L1500+ 的执行路径)
	return c.executeLocal(ctx, params, ec)
}

// ========== executeNode ==========

// executeNode Node host 执行路径 — 通过 gateway 代理远程执行。
// 对应 TS: createExecTool.execute node 分支 L995-1271
func (c *ExecToolConfig) executeNode(
	ctx context.Context,
	params ExecToolParams,
	ec *execContext,
) (*ExecToolResult, error) {
	command := ec.command
	cwd := ec.cwd
	env := ec.env
	warnings := ec.warnings

	// 1. 验证 host env
	if err := ValidateHostEnv(params.Env); err != nil {
		return nil, err
	}

	// 2. 解析审批配置 (TS L996)
	snapshot := infra.ReadExecApprovalsSnapshot()
	resolved := nodehost.ResolveExecApprovalsFromFile(struct {
		File       *infra.ExecApprovalsFile
		AgentID    string
		Overrides  *nodehost.ExecApprovalsDefaultOverrides
		Path       string
		SocketPath string
		Token      string
	}{
		File:      snapshot.File,
		AgentID:   c.agentID,
		Overrides: &nodehost.ExecApprovalsDefaultOverrides{Security: infra.ExecSecurity(ec.security), Ask: infra.ExecAsk(ec.ask)},
		Path:      snapshot.Path,
	})

	hostSecurity := nodehost.MinSecurity(infra.ExecSecurity(ec.security), resolved.Agent.Security)
	hostAsk := nodehost.MaxAsk(infra.ExecAsk(ec.ask), resolved.Agent.Ask)
	askFallback := resolved.Agent.AskFallback

	// 3. deny 检查 (TS L1000-1002)
	if hostSecurity == infra.ExecSecurityDeny {
		return nil, fmt.Errorf("exec denied: host=node security=deny")
	}

	// 4. 解析 node ID (TS L1003-1026)
	boundNode := strings.TrimSpace(c.defaults.Node)
	requestedNode := strings.TrimSpace(params.Node)
	if boundNode != "" && requestedNode != "" && boundNode != requestedNode {
		return nil, fmt.Errorf("exec node not allowed (bound to %s)", boundNode)
	}
	nodeQuery := boundNode
	if nodeQuery == "" {
		nodeQuery = requestedNode
	}

	nodes, err := tools.ListNodesViaLoader(ctx, c.NodeLoader)
	if err != nil {
		return nil, fmt.Errorf("load nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("exec host=node requires a paired node (none available). This requires a companion app or node host.")
	}

	nodeID, err := tools.ResolveNodeIDFromList(nodes, nodeQuery, nodeQuery == "")
	if err != nil {
		if nodeQuery == "" && strings.Contains(err.Error(), "node required") {
			return nil, fmt.Errorf("exec host=node requires a node id when multiple nodes are available (set tools.exec.node or exec.node)")
		}
		return nil, err
	}

	// GAP-3: system.run 支持检查 (TS L1028-1035)
	var nodeInfo *tools.NodeListNode
	for i, n := range nodes {
		if n.NodeID == nodeID {
			nodeInfo = &nodes[i]
			break
		}
	}
	supportsSystemRun := false
	if nodeInfo != nil && len(nodeInfo.Commands) > 0 {
		for _, cmd := range nodeInfo.Commands {
			if cmd == "system.run" {
				supportsSystemRun = true
				break
			}
		}
	}
	if !supportsSystemRun {
		return nil, fmt.Errorf("exec host=node requires a node that supports system.run (companion app or node host)")
	}

	// 5. 构建 shell 命令 (TS L1036)
	nodePlatform := ""
	if nodeInfo != nil {
		nodePlatform = nodeInfo.Platform
	}
	shellArgs := BuildNodeShellCommand(command, nodePlatform)

	// 6. node 环境变量 (TS L1038-1042)
	var nodeEnv map[string]string
	if len(params.Env) > 0 {
		nodeEnv = make(map[string]string)
		for k, v := range params.Env {
			nodeEnv[k] = v
		}
		ApplyPathPrepend(nodeEnv, c.defaultPathPrepend, true)
	}

	// 7. allowlist 评估
	// B 方案下 node 审批文件由 nodehost 本地维护，Gateway 仅做语法分析与自身 ask/security 约束，
	// 不再尝试通过 exec.approvals.node.get 远程读取节点 allowlist。
	baseAllowlistEval := nodehost.EvaluateShellAllowlist(
		command,
		nil,                       // empty allowlist
		make(map[string]struct{}), // empty safeBins
		cwd,
		env,
		nil,
		false,
		nodePlatform,
	)
	analysisOk := baseAllowlistEval.AnalysisOk
	allowlistSatisfied := false

	// 8. 审批检查 (TS L1086-1090)
	requiresAsk := nodehost.RequiresExecApproval(hostAsk, hostSecurity, analysisOk, allowlistSatisfied)

	// FIND-2: invokeTimeoutMs 安全余量 (TS L1093-1096)
	invokeTimeoutMs := int(math.Max(10000, float64(ec.timeoutSec*1000+5000)))
	// FIND-1: buildInvokeParams 对齐 TS `params` 扁平结构 (TS L1097-1118)
	buildInvokeParams := func(approved bool, approvalDecision string, runID ...string) map[string]any {
		invokeParams := map[string]any{
			"command":    shellArgs,
			"rawCommand": command,
			"cwd":        cwd,
			"env":        nodeEnv,
			"timeoutMs":  invokeTimeoutMs,
			"agentId":    c.agentID,
			"sessionKey": c.defaults.SessionKey,
			"approved":   approved,
		}
		if approvalDecision != "" {
			invokeParams["approvalDecision"] = approvalDecision
		}
		if len(runID) > 0 && runID[0] != "" {
			invokeParams["runId"] = runID[0]
		}
		p := map[string]any{
			"nodeId":         nodeID,
			"command":        "system.run",
			"params":         invokeParams,
			"idempotencyKey": uuid.NewString(),
		}
		return p
	}

	if requiresAsk {
		// TS L1120-1238: 异步 fire-and-forget 审批
		approvalID := uuid.NewString()
		approvalSlug := CreateApprovalSlug(approvalID)
		contextKey := "exec:" + approvalID
		noticeSeconds := math.Max(1, math.Round(float64(c.approvalRunningNoticeMs)/1000))
		warningText := getWarningText(warnings)

		go func() {
			// 请求审批 (TS L1128-1158)
			decision, err := nodehost.RequestExecApprovalViaSocket(
				context.Background(),
				resolved.SocketPath,
				resolved.Token,
				map[string]interface{}{
					"id":         approvalID,
					"command":    command,
					"cwd":        cwd,
					"host":       "node",
					"security":   string(hostSecurity),
					"ask":        string(hostAsk),
					"agentId":    c.agentID,
					"sessionKey": c.defaults.SessionKey,
					"timeoutMs":  DefaultApprovalTimeoutMs,
				},
				DefaultApprovalRequestTimeoutMs,
			)
			if err != nil {
				EmitExecSystemEvent(
					fmt.Sprintf("Exec denied (node=%s id=%s, approval-request-failed): %s", nodeID, approvalID, command),
					c.notifySessionKey, contextKey,
				)
				return
			}

			// 决策处理 (TS L1160-1189)
			approvedByAsk := false
			var approvalDecisionStr string
			deniedReason := ""

			switch decision {
			case nodehost.ExecApprovalDeny:
				deniedReason = "user-denied"
			case "":
				switch askFallback {
				case infra.ExecSecurityFull:
					approvedByAsk = true
					approvalDecisionStr = "allow-once"
				case infra.ExecSecurityAllowlist:
					// defer to node host
				default:
					deniedReason = "approval-timeout"
				}
			case nodehost.ExecApprovalAllowOnce:
				approvedByAsk = true
				approvalDecisionStr = "allow-once"
			case nodehost.ExecApprovalAllowAlways:
				approvedByAsk = true
				approvalDecisionStr = "allow-always"
			}

			if deniedReason != "" {
				EmitExecSystemEvent(
					fmt.Sprintf("Exec denied (node=%s id=%s, %s): %s", nodeID, approvalID, deniedReason, command),
					c.notifySessionKey, contextKey,
				)
				return
			}

			// running 通知 (TS L1191-1198)
			var runningTimer *time.Timer
			if c.approvalRunningNoticeMs > 0 {
				runningTimer = time.AfterFunc(time.Duration(c.approvalRunningNoticeMs)*time.Millisecond, func() {
					EmitExecSystemEvent(
						fmt.Sprintf("Exec running (node=%s id=%s, >%.0fs): %s",
							nodeID, approvalID, noticeSeconds, command),
						c.notifySessionKey, contextKey,
					)
				})
			}

			// 调用 node.invoke (TS L1201-1216)
			_, err = tools.CallGateway(
				context.Background(), c.GatewayOpts, "nodes.invoke",
				buildInvokeParams(approvedByAsk, approvalDecisionStr, approvalID),
			)
			if runningTimer != nil {
				runningTimer.Stop()
			}
			if err != nil {
				EmitExecSystemEvent(
					fmt.Sprintf("Exec denied (node=%s id=%s, invoke-failed): %s", nodeID, approvalID, command),
					c.notifySessionKey, contextKey,
				)
			}
		}()

		// FIND-6: 立即返回 approval-pending + expiresAtMs/approvalSlug (TS L1219-1238)
		nodeExpiresAtMs := time.Now().UnixMilli() + DefaultApprovalTimeoutMs
		return &ExecToolResult{
			Text: fmt.Sprintf("%sApproval required (id %s). Approve to run; updates will arrive after completion.", warningText, approvalSlug),
			Details: ExecToolDetails{
				Status:       "approval-pending",
				ApprovalID:   approvalID,
				ApprovalSlug: approvalSlug,
				ExpiresAtMs:  nodeExpiresAtMs,
				Host:         ExecHostNode,
				NodeID:       nodeID,
				Command:      command,
				Cwd:          cwd,
			},
		}, nil
	}

	// 无需审批 → 直接调用 node.invoke (TS L1241-1270)
	startedAt := time.Now()
	raw, err := tools.CallGateway(ctx, c.GatewayOpts, "nodes.invoke",
		buildInvokeParams(false, ""))
	if err != nil {
		return &ExecToolResult{
			Text: fmt.Sprintf("Node execution failed: %v", err),
			Details: ExecToolDetails{
				Status:  "failed",
				Host:    ExecHostNode,
				NodeID:  nodeID,
				Command: command,
			},
		}, nil
	}

	// GAP-4: 正确解析 payload 层级 (TS L1247-1269)
	payloadRaw, _ := raw["payload"]
	payloadObj, _ := payloadRaw.(map[string]any)
	if payloadObj == nil {
		payloadObj = raw // fallback: 直接使用顶层
	}
	stdout, _ := payloadObj["stdout"].(string)
	stderr, _ := payloadObj["stderr"].(string)
	errorText, _ := payloadObj["error"].(string)
	success, _ := payloadObj["success"].(bool)
	exitCodeF, _ := payloadObj["exitCode"].(float64)
	exitCode := int(exitCodeF)

	status := "failed"
	if success {
		status = "completed"
	}

	aggregated := stdout
	if stderr != "" {
		if aggregated != "" {
			aggregated += "\n"
		}
		aggregated += stderr
	}
	if errorText != "" {
		if aggregated != "" {
			aggregated += "\n"
		}
		aggregated += errorText
	}

	displayText := stdout
	if displayText == "" {
		displayText = stderr
	}
	if displayText == "" {
		displayText = errorText
	}

	return &ExecToolResult{
		Text: truncateOutput(displayText, DefaultMaxOutputLen),
		Details: ExecToolDetails{
			Status:     status,
			ExitCode:   &exitCode,
			DurationMs: time.Since(startedAt).Milliseconds(),
			Aggregated: aggregated,
			Cwd:        cwd,
			Host:       ExecHostNode,
			NodeID:     nodeID,
			Command:    command,
		},
	}, nil
}
