package runner

// ============================================================================
// Tool Executor — 工具调用执行分发器
// 对齐 TS: agents/pi-tools.ts → createOpenAcosmiCodingTools
// ============================================================================

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/internal/sandbox"
)

// SandboxExecOptions 原生沙箱执行选项（options struct 模式，未来扩展不破坏接口）。
type SandboxExecOptions struct {
	Cmd           string
	Args          []string
	Env           map[string]string
	TimeoutMs     int64
	SecurityLevel string                   // "allowlist" (L1) | "sandboxed" (L2)
	Workspace     string                   // 实际项目工作区目录（F-07 修复）
	MountRequests []MountRequestForSandbox // L2 临时挂载请求（Phase 3.4）
}

// MountRequestForSandbox 沙箱挂载请求（runner 包本地类型，不依赖 gateway）。
type MountRequestForSandbox struct {
	HostPath  string // 宿主机绝对路径
	MountMode string // "ro" 或 "rw"
}

// NativeSandboxForAgent — runner 包不依赖 sandbox 包的接口。
// 使 executeBashSandboxed 可通过原生沙箱 Worker 执行命令（IPC），
// 而非 Docker 容器。由 gateway/server.go 的 NativeSandboxRouter adapter 实现。
//
// securityLevel 路由:
//   - "allowlist" (L1): 持久 Worker IPC (<1ms)
//   - "sandboxed" (L2): 一次性 CLI sandbox run (~50ms，仅提权期间)
type NativeSandboxForAgent interface {
	ExecuteSandboxed(ctx context.Context, opts SandboxExecOptions) (stdout, stderr string, exitCode int, err error)
}

// MediaSubsystemForAgent — runner 包不依赖 media 包的最小接口。
// 由 media.MediaSubsystem 实现，提供媒体工具的定义查询和执行。
type MediaSubsystemForAgent interface {
	// ToolNames 返回所有已注册媒体工具名列表。
	ToolNames() []string
	// GetToolDef 按名字获取 LLM 工具定义。
	GetToolDef(name string) (json.RawMessage, string, bool) // (inputSchema, description, ok)
	// ExecuteTool 执行媒体工具调用。
	ExecuteTool(ctx context.Context, name string, inputJSON json.RawMessage) (string, error)
	// BuildSystemPrompt 构建媒体子智能体系统提示词。
	// contractPrompt 为合约格式化文本（由 DelegationContract.FormatForSystemPrompt() 生成）。
	BuildSystemPrompt(task, contractPrompt, sessionKey string) string
}

// ToolExecParams 工具执行参数。
type ToolExecParams struct {
	WorkspaceDir string
	SessionID    string // 当前 session 标识（用于合约 issuedBy 等）
	RunID        string // 当前运行标识（用于 agent event 广播）
	TimeoutMs    int64
	// 权限守卫
	AllowWrite   bool // 是否允许写文件
	AllowExec    bool // 是否允许执行命令
	AllowNetwork bool // 是否允许网络访问: L2(sandboxed)/L3(full)=true, L0(deny)/L1(allowlist)=false
	SandboxMode  bool // L1 沙箱模式: bash 通过 Docker 容器执行
	// P3: 命令规则引擎
	Rules []infra.CommandRule // Allow/Ask/Deny 规则集
	// 权限拒绝事件回调
	OnPermissionDenied func(tool, detail string) // 通知网关广播 WebSocket 事件
	SecurityLevel      string                    // 当前安全级别 ("deny"/"allowlist"/"sandboxed"/"full")
	// Argus 视觉子智能体（可选，nil = 不可用）
	ArgusBridge ArgusBridgeForAgent
	// Argus 审批模式: "none" / "medium_and_above" / "all"（默认 medium_and_above）
	ArgusApprovalMode string
	// 命令审批门控（可选，nil = 不需要确认，直接执行）
	// Pre-work 升级: 通用审批门控，供 Ask 规则和 Argus 审批使用
	CoderConfirmation *CoderConfirmationManager
	// MCP 远程工具（可选，nil = 不可用）
	RemoteMCPBridge RemoteMCPBridgeForAgent
	// 原生沙箱 Worker（可选，nil = 使用 Docker fallback）
	NativeSandbox NativeSandboxForAgent
	// 技能按需加载缓存: skill name → full SKILL.md content
	SkillsCache map[string]string
	// UHMS 记忆系统 Bridge（可选，nil = 不可用）
	UHMSBridge UHMSBridgeForAgent
	// SessionKey 当前会话标识（用于判断渠道来源，如 "feishu:<chatID>"）。
	// 非 Web 渠道无法响应 coder confirm 弹窗，自动 bypass 确认。
	SessionKey string
	// SpawnSubagent 子智能体生成回调（可选，nil = spawn_coder_agent 返回合约但不启动 session）
	// 由 gateway/server.go 注入实现。
	SpawnSubagent SpawnSubagentFunc
	// 委托合约约束（可选，nil = 无合约约束）
	DelegationContract *DelegationContract
	// ScopePaths 合约允许的路径列表（从 DelegationContract.Scope 提取）。
	// 非空时，工具路径校验使用此列表替代 WorkspaceDir 单根。
	ScopePaths []string
	// WebSearchProvider 网页搜索 provider（可选，nil = web_search 工具不可用）
	WebSearchProvider interface {
		Search(ctx context.Context, query string, maxResults int) ([]WebSearchResult, error)
	}
	// BrowserEvaluateEnabled 是否允许 browser evaluate 动作（JS 执行）。
	// 由 BrowserConfig.EvaluateEnabled 控制，默认 true。
	BrowserEvaluateEnabled bool
	// Phase 6: 合约持久化（可选，nil = 恢复上下文不可用）
	ContractStore ContractPersistence
	// BrowserController 浏览器控制器（可选，nil = browser 工具不可用）
	BrowserController browser.BrowserController
	// MediaSender 媒体文件发送器（可选，nil = send_media 工具不可用）
	MediaSender interface {
		SendMedia(ctx context.Context, channelID, to string, data []byte, fileName, mimeType, message string) error
	}
	// QualityReviewFn 质量审核回调（可选，nil = 跳过 LLM 语义审核，只做规则预检）
	// Phase 2: 三级指挥体系 — 子智能体结果质量审核
	QualityReviewFn QualityReviewFunc
	// ResultApprovalMgr 结果签收管理器（可选，nil = 跳过最终交付门控）
	// Phase 3: 三级指挥体系 — 质量审核通过后用户签收
	ResultApprovalMgr *ResultApprovalManager
	// AgentChannel 异步消息通道（可选，nil = 不支持求助工具）
	// Phase 4: 三级指挥体系 — 子智能体执行中异步向主智能体求助
	AgentChannel *AgentChannel
	// MountRequests L2 提权期间的临时挂载请求（Phase 3.4，从 escalation grant 注入）。
	MountRequests []MountRequestForSandbox
	// MediaSubsystem 媒体子系统（可选，nil = 媒体工具不可用）。
	// 提供 trending_topics / content_compose / media_publish / social_interact 工具。
	MediaSubsystem MediaSubsystemForAgent
	// OnProgress 中间进度回调（可选，nil = 仅本地实时事件）。
	OnProgress func(ctx context.Context, update ProgressUpdate) ProgressReportStatus
}

// ExecuteToolCall 执行工具调用并返回文本结果。
// 当前支持: bash, read_file, write_file, list_dir, search, glob
// 延迟: message, notebook_edit 等高级工具
func ExecuteToolCall(ctx context.Context, name string, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	switch name {
	case "bash":
		if !params.AllowExec {
			var bi bashInput
			cmdStr := "(unknown)"
			if err := json.Unmarshal(inputJSON, &bi); err == nil && bi.Command != "" {
				cmdStr = bi.Command
			}
			if params.OnPermissionDenied != nil {
				params.OnPermissionDenied("bash", cmdStr)
			}
			return formatPermissionDenied("bash", cmdStr, params.SecurityLevel), nil
		}
		// Phase 8: 资源预算检查
		if params.DelegationContract != nil && params.DelegationContract.Budget != nil {
			budget := params.DelegationContract.Budget
			if exhausted, reason := budget.IsExhausted(); exhausted {
				return fmt.Sprintf("[bash] Resource budget exhausted: %s", reason), nil
			}
			budget.IncrementBashCalls()
		}
		// P3: 命令规则引擎检查
		if len(params.Rules) > 0 {
			var bi bashInput
			if err := json.Unmarshal(inputJSON, &bi); err == nil && bi.Command != "" {
				ruleResult := EvaluateCommand(bi.Command, params.Rules)
				if ruleResult.Matched {
					switch ruleResult.Action {
					case infra.RuleActionDeny:
						slog.Warn("command blocked by rule",
							"command", bi.Command,
							"rule", ruleResult.Rule.Pattern,
							"ruleId", ruleResult.Rule.ID,
						)
						return fmt.Sprintf("[Command blocked by security rule: %s] %s", ruleResult.Rule.Pattern, ruleResult.Reason), nil
					case infra.RuleActionAsk:
						slog.Info("command requires approval",
							"command", bi.Command,
							"rule", ruleResult.Rule.Pattern,
							"ruleId", ruleResult.Rule.ID,
						)
						// Fix 1: 非 Web 渠道走 CoderConfirmation（如果可用），否则 fail-closed
						if isNonWebChannel(params.SessionKey) {
							if params.CoderConfirmation != nil {
								approved, approvalErr := params.CoderConfirmation.RequestConfirmation(ctx, "bash", inputJSON, params.SessionKey)
								if approvalErr != nil {
									return fmt.Sprintf("[Command approval error on non-web channel] %v", approvalErr), nil
								}
								if !approved {
									return fmt.Sprintf("[Command denied on non-web channel: %s]", ruleResult.Rule.Pattern), nil
								}
							} else {
								slog.Warn("ask rule triggered but no approval gate on non-web channel, fail-closed deny",
									"command", bi.Command,
									"rule", ruleResult.Rule.Pattern,
									"sessionKey", params.SessionKey,
								)
								return fmt.Sprintf("[Command blocked: no approval gate available for rule %s on non-web channel]", ruleResult.Rule.Pattern), nil
							}
							// approved: fall through → 继续执行
						} else {
							// Fail-closed: 无审批门控时直接拒绝，不允许 LLM 忽略后继续执行
							if params.CoderConfirmation == nil {
								slog.Warn("ask rule triggered but no approval gate, fail-closed deny",
									"command", bi.Command,
									"rule", ruleResult.Rule.Pattern,
								)
								return fmt.Sprintf("[Command blocked: no approval gate available for rule %s] %s", ruleResult.Rule.Pattern, ruleResult.Reason), nil
							}
							// 真正阻塞：广播审批请求到前端，等待用户 allow/deny 或超时
							approved, approvalErr := params.CoderConfirmation.RequestConfirmation(ctx, "bash", inputJSON, params.SessionKey)
							if approvalErr != nil {
								slog.Error("command approval request failed",
									"command", bi.Command,
									"error", approvalErr,
								)
								return fmt.Sprintf("[Command blocked: approval error] %v", approvalErr), nil
							}
							if !approved {
								slog.Info("command denied by user approval",
									"command", bi.Command,
									"rule", ruleResult.Rule.Pattern,
								)
								return fmt.Sprintf("[Command denied by user: %s] %s", ruleResult.Rule.Pattern, ruleResult.Reason), nil
							}
							slog.Info("command approved by user",
								"command", bi.Command,
								"rule", ruleResult.Rule.Pattern,
							)
						}
						// approved / auto-approved: fall through → 继续执行
					}
				}
			}
		}
		// L1 沙箱模式: 通过 Docker 容器执行 bash
		if params.SandboxMode {
			return executeBashSandboxed(ctx, inputJSON, params)
		}
		return executeBash(ctx, inputJSON, params)
	case "read_file":
		return executeReadFile(inputJSON, params)
	case "write_file":
		if !params.AllowWrite {
			var wi struct {
				Path string `json:"file_path"`
			}
			pathStr := "(unknown)"
			if err := json.Unmarshal(inputJSON, &wi); err == nil && wi.Path != "" {
				pathStr = wi.Path
			}
			if params.OnPermissionDenied != nil {
				params.OnPermissionDenied("write_file", pathStr)
			}
			return formatPermissionDenied("write_file", pathStr, params.SecurityLevel), nil
		}
		return executeWriteFile(inputJSON, params)
	case "list_dir":
		return executeListDir(inputJSON, params)
	case "search", "grep":
		return executeSearch(inputJSON, params)
	case "glob":
		return executeGlob(inputJSON, params)
	case "lookup_skill":
		return executeLookupSkill(ctx, inputJSON, params)
	case "search_skills":
		return executeSearchSkills(ctx, inputJSON, params)
	case "spawn_coder_agent":
		return executeSpawnCoderAgent(ctx, inputJSON, params)
	case "spawn_argus_agent":
		return executeSpawnArgusAgent(ctx, inputJSON, params)
	case "spawn_media_agent":
		return executeSpawnMediaAgent(ctx, inputJSON, params)
	case "request_help":
		return ExecuteRequestHelp(inputJSON, params.AgentChannel)
	case "report_progress":
		return executeReportProgress(ctx, inputJSON, params)
	case "web_search":
		return executeWebSearch(ctx, inputJSON, params)
	case "browser":
		return executeBrowserTool(ctx, inputJSON, params)
	case "send_media":
		return executeSendMedia(ctx, inputJSON, params)
	case "memory_search":
		return executeMemorySearch(ctx, inputJSON, params)
	case "memory_get":
		return executeMemoryGet(ctx, inputJSON, params)
	case "trending_topics", "content_compose", "media_publish", "social_interact":
		if params.MediaSubsystem != nil {
			return params.MediaSubsystem.ExecuteTool(ctx, name, inputJSON)
		}
		return fmt.Sprintf("[Tool %s requires media subsystem]", name), nil
	case "notebook_edit":
		return "[Tool notebook_edit is not yet implemented in Go runtime]", nil
	case "mcp":
		return "[Tool mcp is not yet implemented in Go runtime]", nil
	default:
		if strings.HasPrefix(name, "argus_") && params.ArgusBridge != nil {
			return executeArgusTool(ctx, name, inputJSON, params)
		}
		// (Phase 2A: coder_ 分发已删除 — 由 spawn_coder_agent 替代)
		if strings.HasPrefix(name, "remote_") && params.RemoteMCPBridge != nil {
			return executeRemoteTool(ctx, name, inputJSON, params)
		}
		return fmt.Sprintf("[Tool %q is not yet implemented]", name), nil
	}
}

// ---------- report_progress ----------

// executeReportProgress 执行 report_progress 工具——发出中间进度事件供实时界面消费。
func executeReportProgress(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input struct {
		Summary string `json:"summary"`
		Percent int    `json:"percent"`
		Phase   string `json:"phase"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid report_progress input: %w", err)
	}
	if input.Summary == "" {
		return "Error: summary is required", nil
	}

	// 截断过长的摘要
	summaryRunes := []rune(input.Summary)
	if len(summaryRunes) > 300 {
		input.Summary = string(summaryRunes[:300]) + "..."
	}

	// 构建进度事件数据
	progressData := map[string]interface{}{
		"summary": input.Summary,
	}
	if input.Percent > 0 && input.Percent <= 100 {
		progressData["percent"] = input.Percent
	}
	if input.Phase != "" {
		progressData["phase"] = input.Phase
	}
	update := ProgressUpdate{
		Summary: input.Summary,
		Percent: input.Percent,
		Phase:   input.Phase,
	}

	// 发出 agent.progress 事件；当前稳定消费面是 UI/WebSocket 一类实时界面。
	if params.RunID != "" {
		infra.EmitAgentEvent(params.RunID, infra.StreamProgress, progressData, "")
	}

	status := ProgressReportStatus{}
	if params.OnProgress != nil {
		status = params.OnProgress(ctx, update)
	}

	slog.Debug("report_progress emitted",
		"summary", input.Summary,
		"percent", input.Percent,
		"phase", input.Phase,
	)

	switch {
	case status.RemoteDelivered:
		return "Progress reported to live surfaces and remote channel.", nil
	case status.Throttled:
		return "Progress reported to live surfaces. Remote update skipped (throttled).", nil
	case status.Error != "":
		slog.Warn("report_progress remote delivery failed", "error", status.Error)
		return "Progress reported to live surfaces. Remote delivery failed.", nil
	default:
		return "Progress reported to live surfaces.", nil
	}
}

// ---------- Process group management ----------

// processTracker 跟踪正在运行的子进程，供 kill-tree 使用。
var processTracker = struct {
	mu  sync.Mutex
	pgs map[int]struct{} // 进程组 ID 集合
}{pgs: make(map[int]struct{})}

// trackProcessGroup 注册进程组 ID。
func trackProcessGroup(pgid int) {
	processTracker.mu.Lock()
	processTracker.pgs[pgid] = struct{}{}
	processTracker.mu.Unlock()
}

// untrackProcessGroup 取消注册进程组 ID。
func untrackProcessGroup(pgid int) {
	processTracker.mu.Lock()
	delete(processTracker.pgs, pgid)
	processTracker.mu.Unlock()
}

// KillTree 终止进程及其所有子进程（通过进程组）。
// TS 对照: pi-tools.ts killTree()
func KillTree(pid int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		// 进程可能已退出
		return nil
	}

	slog.Debug("kill_tree: killing process group",
		"pid", pid,
		"pgid", pgid,
	)

	// 先发 SIGTERM
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		slog.Debug("kill_tree: SIGTERM failed, trying SIGKILL",
			"pgid", pgid,
			"error", err,
		)
		// 再发 SIGKILL
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}

	untrackProcessGroup(pgid)
	return nil
}

// KillAllTrackedProcesses 终止所有被追踪的子进程组。
// 在 agent run 结束时调用以确保清理。
func KillAllTrackedProcesses() {
	processTracker.mu.Lock()
	pgs := make([]int, 0, len(processTracker.pgs))
	for pgid := range processTracker.pgs {
		pgs = append(pgs, pgid)
	}
	processTracker.mu.Unlock()

	for _, pgid := range pgs {
		slog.Debug("kill_tree: cleanup tracked process group", "pgid", pgid)
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		untrackProcessGroup(pgid)
	}
}

// ---------- bash (sandboxed — L1 Docker 容器执行) ----------

// executeBashSandboxed 在沙箱中执行 bash 命令。
// 优先使用原生沙箱 Worker (IPC, <1ms)，不可用时 fallback 到 Docker 容器。
// 安全层: namespace隔离 + --no-new-privileges + --network=none + seccomp + resource limits
// 对应安全级别 L1 (sandbox): AllowExec=true, AllowWrite=true（沙箱内）
func executeBashSandboxed(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input bashInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid bash input: %w", err)
	}
	if input.Command == "" {
		return "", fmt.Errorf("bash: empty command")
	}

	// 原生沙箱写操作防护 (Bug #1 修复):
	// NativeSandbox Worker 通过 IPC 在宿主机执行，拥有完整文件系统权限。
	// 在 L1 (allowlist) 安全级别下，write_file 被 AllowWrite 门控，
	// 但 bash redirect (>, >>, tee, sed -i) 可绕过。此处是对等防线。
	if params.NativeSandbox != nil && looksLikeBashWrite(input.Command) {
		slog.Info("native sandbox write detection triggered",
			"command", input.Command,
			"security", params.SecurityLevel,
		)
		if params.CoderConfirmation != nil {
			approved, err := params.CoderConfirmation.RequestConfirmation(ctx, "bash (write detected)", inputJSON, params.SessionKey)
			if err != nil {
				return fmt.Sprintf("[Write operation approval error: %v]", err), nil
			}
			if !approved {
				return "[Command blocked: bash write operation requires approval in native sandbox mode]", nil
			}
			// approved: fall through to execution
		} else if isNonWebChannel(params.SessionKey) {
			// 非 Web 渠道无审批门控: fail-closed
			slog.Warn("native sandbox write blocked (no approval gate on non-web channel)",
				"command", input.Command,
				"security", params.SecurityLevel,
				"sessionKey", params.SessionKey,
			)
			return "[Command blocked: bash write operation in native sandbox requires approval (non-web channel)]", nil
		} else {
			// Web 渠道无审批门控: fail-closed
			return "[Command blocked: bash write operation in native sandbox requires approval]", nil
		}
	}

	// 优先使用原生沙箱 Worker（IPC 执行，延迟 <1ms）
	if params.NativeSandbox != nil {
		return executeBashNativeSandbox(ctx, input.Command, params)
	}

	slog.Info("sandbox bash exec",
		"command", input.Command,
		"mode", "docker",
		"security", params.SecurityLevel,
	)

	// 配置 Docker Runner
	cfg := sandbox.DefaultDockerConfig()
	cfg.TimeoutSecs = int(params.TimeoutMs / 1000)
	if cfg.TimeoutSecs <= 0 || cfg.TimeoutSecs > 120 {
		cfg.TimeoutSecs = 120
	}
	cfg.NetworkEnabled = params.AllowNetwork

	// 应用 L0/L1 挂载策略（工作区/技能/配置）
	sandbox.ApplyMountsToConfig(&cfg, sandbox.SandboxMountConfig{
		SecurityLevel: params.SecurityLevel,
		ProjectDir:    params.WorkspaceDir,
	})

	runner := sandbox.NewDockerRunner(cfg)

	// 通过 Docker 执行 bash 命令（工作目录由 ApplyMountsToConfig 设置为 /workspace）
	result, err := runner.Execute(ctx, "", []string{"sh", "-c", input.Command}, "")
	if err != nil {
		return fmt.Sprintf("[Sandbox execution error: %s]", err), nil
	}

	// 组装输出
	var output strings.Builder
	if result.Stdout != "" {
		output.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString(result.Stderr)
	}

	// 截断过长输出
	const maxOutput = 100 * 1024 // 100KB
	text := output.String()
	if len(text) > maxOutput {
		text = text[:maxOutput] + "\n... [output truncated]"
	}

	if result.ExitCode != 0 {
		if result.Error != "" {
			return fmt.Sprintf("%s\n[sandbox exit code: %d, error: %s]", text, result.ExitCode, result.Error), nil
		}
		return fmt.Sprintf("%s\n[sandbox exit code: %d]", text, result.ExitCode), nil
	}

	return text, nil
}

// ---------- bash (native sandbox — 原生沙箱 Worker IPC 执行) ----------

// executeBashNativeSandbox 通过原生沙箱 Worker 的 JSON-Lines IPC 执行 bash 命令。
// 比 Docker 路径快约 200x（IPC <1ms vs Docker ~215ms cold start）。
func executeBashNativeSandbox(ctx context.Context, command string, params ToolExecParams) (string, error) {
	slog.Info("sandbox bash exec",
		"command", command,
		"mode", "native",
		"security", params.SecurityLevel,
	)

	stdout, stderr, exitCode, err := params.NativeSandbox.ExecuteSandboxed(ctx, SandboxExecOptions{
		Cmd:           "sh",
		Args:          []string{"-c", command},
		TimeoutMs:     params.TimeoutMs,
		SecurityLevel: params.SecurityLevel,
		Workspace:     params.WorkspaceDir,
		MountRequests: params.MountRequests,
	})
	if err != nil {
		return fmt.Sprintf("[Native sandbox error: %s]", err), nil
	}

	// 组装输出
	var output strings.Builder
	if stdout != "" {
		output.WriteString(stdout)
	}
	if stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString(stderr)
	}

	// 截断过长输出
	const maxOutput = 100 * 1024 // 100KB
	text := output.String()
	if len(text) > maxOutput {
		text = text[:maxOutput] + "\n... [output truncated]"
	}

	if exitCode != 0 {
		return fmt.Sprintf("%s\n[sandbox exit code: %d]", text, exitCode), nil
	}

	return text, nil
}

// ---------- bash write-pattern detection ----------

// looksLikeBashWrite 检测 bash 命令是否包含文件写入操作。
// 用于 NativeSandbox 路径的写操作审批 — 防止通过 shell redirect 绕过 AllowWrite 门控。
//
// 检测策略 (defense-in-depth, 非穷举):
//   - Shell redirect: " > file" / " >> file" (排除 fd redirect 和 /dev/null)
//   - 写入命令: tee, sed -i, install
//
// 注意: Shell 是图灵完备的，此函数无法捕获所有写入方式（如 python -c, eval 等）。
// 这是一层防御，不是唯一防线。配合 command_rule_presets 的 ask/deny 规则形成纵深防御。
func looksLikeBashWrite(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))

	// 1. 写入命令检测
	writeCommands := []string{"tee ", "sed -i ", "sed -i'", "sed -i\""}
	for _, wc := range writeCommands {
		if strings.HasPrefix(lower, wc) || strings.Contains(lower, " "+wc) ||
			strings.Contains(lower, "|"+wc) || strings.Contains(lower, "| "+wc) {
			return true
		}
	}
	// install 命令（仅作为首个命令时触发，避免 "npm install" 误判）
	if strings.HasPrefix(lower, "install ") {
		return true
	}

	// 2. Shell redirect 检测: " > target" / " >> target"
	// 排除: fd redirect (2>, 1>), /dev/null 输出抑制, stream merge (&1)
	for _, op := range []string{" >> ", " > "} {
		idx := strings.Index(command, op)
		for idx >= 0 {
			// 检查 redirect 前是否为 fd 编号 (0-9)
			if idx > 0 && command[idx-1] >= '0' && command[idx-1] <= '9' {
				// fd redirect (如 2> /dev/null), 跳过
				idx = strings.Index(command[idx+len(op):], op)
				if idx >= 0 {
					idx += len(op) // 调整为原始字符串中的位置
				}
				continue
			}
			// 检查 redirect 目标是否为 /dev/null 或 fd merge (&)
			target := strings.TrimSpace(command[idx+len(op):])
			if strings.HasPrefix(target, "/dev/null") || strings.HasPrefix(target, "&") {
				idx = strings.Index(command[idx+len(op):], op)
				if idx >= 0 {
					idx += len(op)
				}
				continue
			}
			return true // 真正的文件写入 redirect
		}
	}

	return false
}

// ---------- bash (host — L2 宿主机直接执行) ----------

type bashInput struct {
	Command string `json:"command"`
}

func executeBash(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input bashInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid bash input: %w", err)
	}
	if input.Command == "" {
		return "", fmt.Errorf("bash: empty command")
	}

	timeout := time.Duration(params.TimeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 2*time.Minute {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", input.Command)
	cmd.Dir = params.WorkspaceDir
	// 使用进程组以支持 kill-tree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 限制输出大小
	output, err := cmd.CombinedOutput()
	const maxOutput = 100 * 1024 // 100KB
	if len(output) > maxOutput {
		output = append(output[:maxOutput], []byte("\n... [output truncated]")...)
	}

	// 追踪进程组用于清理
	if cmd.Process != nil {
		if pgid, pgErr := syscall.Getpgid(cmd.Process.Pid); pgErr == nil {
			trackProcessGroup(pgid)
			defer untrackProcessGroup(pgid)
		}
	}

	result := string(output)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// 超时时 kill 进程树
			if cmd.Process != nil {
				_ = KillTree(cmd.Process.Pid)
			}
			return result + "\n[command timed out]", nil
		}
		// 命令执行失败但有输出
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return fmt.Sprintf("%s\n[exit code: %d]", result, exitCode), nil
	}

	return result, nil
}

// ---------- read_file ----------

type readFileInput struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

func executeReadFile(inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input readFileInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid read_file input: %w", err)
	}

	path := resolveToolPath(input.Path, params.WorkspaceDir)

	// 全局可读: 所有安全级别均允许读取任意路径

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	content := string(data)
	const maxFileSize = 200 * 1024 // 200KB
	if len(content) > maxFileSize {
		content = content[:maxFileSize] + "\n... [file truncated]"
	}

	return content, nil
}

// ---------- write_file ----------

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func executeWriteFile(inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input writeFileInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid write_file input: %w", err)
	}

	path := resolveToolPath(input.Path, params.WorkspaceDir)

	// 路径安全验证（合约 scope 优先，fallback 到 workspace 单根）
	if err := validateToolPathScoped(path, params.ScopePaths, params.WorkspaceDir, params.MountRequests, true); err != nil {
		if params.OnPermissionDenied != nil {
			params.OnPermissionDenied("write_file", path)
		}
		return formatPermissionDenied("write_file", input.Path, params.SecurityLevel), nil
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Sprintf("Error creating directory: %s", err), nil
	}

	if err := os.WriteFile(path, []byte(input.Content), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %s", err), nil
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path), nil
}

// ---------- list_dir ----------

type listDirInput struct {
	Path string `json:"path"`
}

func executeListDir(inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input listDirInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid list_dir input: %w", err)
	}

	path := resolveToolPath(input.Path, params.WorkspaceDir)

	// 全局可读: 所有安全级别均允许列出任意目录

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Sprintf("Error listing directory: %s", err), nil
	}

	var sb strings.Builder
	for _, entry := range entries {
		prefix := "  "
		if entry.IsDir() {
			prefix = "d "
		}
		sb.WriteString(prefix)
		sb.WriteString(entry.Name())
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// ---------- search ----------

type searchInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Include string `json:"include,omitempty"`
}

func executeSearch(inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input searchInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid search input: %w", err)
	}
	if input.Pattern == "" {
		return "", fmt.Errorf("search: empty pattern")
	}

	searchPath := params.WorkspaceDir
	if input.Path != "" {
		searchPath = resolveToolPath(input.Path, params.WorkspaceDir)
	}

	// 全局可读: 所有安全级别均允许搜索任意路径

	args := []string{"-rn", "--color=never", "-m", "50"}
	if input.Include != "" {
		args = append(args, "--include="+input.Include)
	}
	args = append(args, input.Pattern, searchPath)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "grep", args...)
	output, err := cmd.CombinedOutput()

	result := string(output)
	const maxOutput = 50 * 1024 // 50KB
	if len(result) > maxOutput {
		result = result[:maxOutput] + "\n... [search results truncated]"
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		if ctx.Err() == context.DeadlineExceeded {
			return result + "\n[search timed out]", nil
		}
	}

	return result, nil
}

// ---------- glob ----------

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func executeGlob(inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input globInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid glob input: %w", err)
	}
	if input.Pattern == "" {
		return "", fmt.Errorf("glob: empty pattern")
	}

	basePath := params.WorkspaceDir
	if input.Path != "" {
		basePath = resolveToolPath(input.Path, params.WorkspaceDir)
	}

	// 全局可读: 所有安全级别均允许 glob 任意路径

	pattern := filepath.Join(basePath, input.Pattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Sprintf("Error: invalid glob pattern: %s", err), nil
	}

	if len(matches) == 0 {
		return "No files matched", nil
	}

	var sb strings.Builder
	for i, match := range matches {
		if i >= 200 { // 限制结果数
			sb.WriteString(fmt.Sprintf("\n... [%d more matches not shown]", len(matches)-200))
			break
		}
		// 相对于 workspace 显示
		rel, _ := filepath.Rel(params.WorkspaceDir, match)
		if rel == "" {
			rel = match
		}
		sb.WriteString(rel)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// ---------- lookup_skill ----------

type lookupSkillInput struct {
	Name string `json:"name"`
}

// executeLookupSkill 从缓存或 VFS 返回完整 SKILL.md 内容。
// 降级链: SkillsCache (文件扫描模式) → VFS L2 (Boot 模式)
func executeLookupSkill(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input lookupSkillInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid lookup_skill input: %w", err)
	}
	if input.Name == "" {
		return "", fmt.Errorf("lookup_skill: skill name is required")
	}

	// 主路径: 从文件扫描缓存读取
	if params.SkillsCache != nil {
		if content, ok := params.SkillsCache[input.Name]; ok {
			return content, nil
		}
		// 列出可用技能帮助 LLM 修正
		var available []string
		for name := range params.SkillsCache {
			available = append(available, name)
		}
		return fmt.Sprintf("[Skill %q not found. Available skills: %s]", input.Name, strings.Join(available, ", ")), nil
	}

	// 降级路径: Boot 模式 → VFS 搜索 + L2 读取
	if params.UHMSBridge != nil {
		// 先精确搜索找到 category
		hits, err := params.UHMSBridge.SearchSkillsVFS(ctx, input.Name, 5)
		if err != nil {
			return fmt.Sprintf("[lookup_skill: VFS search failed: %v]", err), nil
		}
		for _, h := range hits {
			if strings.EqualFold(h.Name, input.Name) {
				content, readErr := params.UHMSBridge.ReadSkillVFS(ctx, h.Category, h.Name)
				if readErr != nil {
					return fmt.Sprintf("[lookup_skill: VFS read failed for %q: %v]", input.Name, readErr), nil
				}
				return content, nil
			}
		}
		return fmt.Sprintf("[Skill %q not found in VFS index]", input.Name), nil
	}

	return fmt.Sprintf("[Skill %q not found: no skills loaded]", input.Name), nil
}

// ---------- search_skills ----------

type searchSkillsInput struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

// executeSearchSkills 搜索技能索引（VFS/Qdrant），返回匹配技能的 L0 摘要。
// 降级链: UHMSBridge.SearchSkillsVFS → SkillsCache 关键词过滤
func executeSearchSkills(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input searchSkillsInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid search_skills input: %w", err)
	}
	if input.Query == "" {
		return "", fmt.Errorf("search_skills: query is required")
	}
	topK := input.TopK
	if topK <= 0 {
		topK = 10
	}

	// 主路径: VFS/Qdrant 搜索
	if params.UHMSBridge != nil {
		distributing := params.UHMSBridge.IsSkillsDistributing()
		hits, err := params.UHMSBridge.SearchSkillsVFS(ctx, input.Query, topK)
		if err != nil {
			slog.Warn("search_skills: VFS search failed, trying cache fallback", "error", err)
			// fall through → SkillsCache 降级（F-3: 下方统一追加 distributing note）
		} else if len(hits) > 0 {
			result := formatSkillSearchHits(hits)
			if distributing {
				result += "\n[Note: Skills indexing is in progress. Some skills may not appear yet. Retry shortly.]"
			}
			return result, nil
		} else if distributing && params.SkillsCache != nil {
			// F-2: VFS 零结果 + 分发中 + cache 可用 → cache 可能有更完整数据
			slog.Debug("search_skills: VFS returned 0 during distributing, trying cache fallback")
			// fall through → SkillsCache 降级
		} else {
			// VFS 零结果（非分发中，或 cache 不可用）
			msg := fmt.Sprintf("[No matching skills found for query %q. Try broader keywords or check available skill categories.]", input.Query)
			if distributing {
				msg += "\n[Note: Skills indexing is in progress. More results may appear shortly.]"
			}
			return msg, nil
		}
	}

	// 降级路径: SkillsCache 关键词过滤
	if params.SkillsCache != nil {
		result := searchSkillsCacheFallback(input.Query, topK, params.SkillsCache)
		// F-3: VFS 错误/零结果降级到 cache，分发中追加 note
		if params.UHMSBridge != nil && params.UHMSBridge.IsSkillsDistributing() {
			result += "\n[Note: Skills indexing is in progress. Some skills may not appear yet. Retry shortly.]"
		}
		return result, nil
	}

	return "[No skills index available. Skills may still be loading — retry in a few seconds.]", nil
}

// formatSkillSearchHits 格式化搜索结果为 LLM 可读文本。
func formatSkillSearchHits(hits []SkillSearchHit) string {
	if len(hits) == 0 {
		return "[No matching skills found]"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d skills:\n\n", len(hits)))
	for i, h := range hits {
		sb.WriteString(fmt.Sprintf("%d. **%s** [%s]\n", i+1, h.Name, h.Category))
		if h.Abstract != "" {
			sb.WriteString("   ")
			sb.WriteString(h.Abstract)
			sb.WriteString("\n")
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("Use `lookup_skill` with the skill name to read full content.")
	return sb.String()
}

// searchSkillsCacheFallback 在 SkillsCache 中按关键词过滤技能。
func searchSkillsCacheFallback(query string, topK int, cache map[string]string) string {
	queryLower := strings.ToLower(query)
	terms := strings.Fields(queryLower)

	type match struct {
		name  string
		score int
	}
	var matches []match

	for name, content := range cache {
		nameLower := strings.ToLower(name)
		contentLower := strings.ToLower(content)
		score := 0
		for _, t := range terms {
			if strings.Contains(nameLower, t) {
				score += 3 // name 匹配权重高
			}
			if strings.Contains(contentLower, t) {
				score += 1
			}
		}
		if score > 0 {
			matches = append(matches, match{name: name, score: score})
		}
	}

	// 按分数降序排序
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	if len(matches) > topK {
		matches = matches[:topK]
	}

	if len(matches) == 0 {
		return fmt.Sprintf("[No skills matching %q found]", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d skills matching %q:\n\n", len(matches), query))
	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, m.name))
	}
	sb.WriteString("\nUse `lookup_skill` with the skill name to read full content.")
	return sb.String()
}

// ---------- channel detection ----------

// isNonWebChannel 判断 sessionKey 是否来自非 Web 渠道（飞书/钉钉/企微等）。
// 非 Web 渠道无法响应 coder confirm 弹窗，因此自动 bypass 确认流程。
// sessionKey 格式: "feishu:<chatID>", "dingtalk:<chatID>", "wecom:<fromUser>" 等。
func isNonWebChannel(sessionKey string) bool {
	prefixes := []string{"feishu:", "dingtalk:", "wecom:", "slack:", "discord:", "telegram:", "imessage:", "signal:", "whatsapp:", "line:"}
	for _, p := range prefixes {
		if strings.HasPrefix(sessionKey, p) {
			return true
		}
	}
	return false
}

// ---------- web_search ----------

// executeWebSearch 执行联网搜索并返回格式化结果。
func executeWebSearch(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	if params.WebSearchProvider == nil {
		return "[web_search is not available: no search provider configured. Configure Bocha (default) or Google search API key in settings.]", nil
	}

	var input struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid web_search input: %w", err)
	}
	if input.Query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}
	if input.MaxResults <= 0 {
		input.MaxResults = 8
	}

	results, err := params.WebSearchProvider.Search(ctx, input.Query, input.MaxResults)
	if err != nil {
		slog.Error("web_search failed", "query", input.Query, "error", err)
		return fmt.Sprintf("[web_search error: %v]", err), nil
	}

	if len(results) == 0 {
		return fmt.Sprintf("[No search results for %q]", input.Query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for %q (%d results):\n\n", input.Query, len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   URL: %s\n", i+1, r.Title, r.URL))
		if r.Snippet != "" {
			sb.WriteString("   ")
			sb.WriteString(r.Snippet)
			sb.WriteString("\n")
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// ---------- browser (浏览器自动化工具) ----------

// classifyBrowserError 对浏览器操作错误进行结构化分类。
// Phase 3.2: Transient(可重试) / Structural(需 LLM 重规划) / Fatal(中止)。
func classifyBrowserError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Fatal: CDP 连接断开 / 页面崩溃
	if strings.Contains(msg, "websocket") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "target closed") || strings.Contains(msg, "page crashed") ||
		strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline") {
		return "fatal"
	}
	// Transient: 元素未加载/不可见/动画中（可能很快就绪）
	if strings.Contains(msg, "not visible") || strings.Contains(msg, "not stable") ||
		strings.Contains(msg, "animating") || strings.Contains(msg, "actionability timeout") ||
		strings.Contains(msg, "not found") {
		return "transient"
	}
	// Structural: 选择器无效 / 元素不存在 / 严格模式冲突
	return "structural"
}

// executeBrowserTool 执行浏览器自动化工具调用。
// 通过 CDP 直连浏览器，比 Argus 屏幕坐标操控更高效精准。
func executeBrowserTool(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	if params.BrowserController == nil {
		return "[Browser tool is not configured. Enable browser in config and ensure CDP is running.]", nil
	}

	var input struct {
		Action   string `json:"action"`
		URL      string `json:"url"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
		Script   string `json:"script"`
		Ref      string `json:"ref"`       // Phase 1: ARIA ref 标识符（如 "e1"），用于 click_ref/fill_ref
		Goal     string `json:"goal"`      // Phase 4: ai_browse 意图目标
		TargetID string `json:"target_id"` // Tab management: target ID for close_tab/switch_tab
	}
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("parse browser input: %w", err)
	}

	bc := params.BrowserController

	switch input.Action {
	case "navigate":
		if input.URL == "" {
			return "[Error: url is required for navigate action]", nil
		}
		if err := bc.Navigate(ctx, input.URL); err != nil {
			return fmt.Sprintf("[Browser navigate error: %s]", err), nil
		}
		// Phase 3: 操作后截图验证
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("Navigated to %s", input.URL)), nil

	case "get_content":
		// Phase 1: 优先返回 ARIA 快照（语义结构 + ref 标注，token 消耗降低 ~80%）。
		// 降级到 innerText（当 ARIA 不可用时）。
		snapshot, snapErr := bc.SnapshotAI(ctx)
		if snapErr == nil && snapshot != nil {
			if snapText, ok := snapshot["snapshot"].(string); ok && snapText != "" {
				var sb strings.Builder
				sb.WriteString("## Page Content (ARIA Accessibility Tree)\n\n")
				sb.WriteString("Use element refs (e.g. e1, e2) with click_ref/fill_ref for interactions.\n\n")
				if len(snapText) > 40000 {
					snapText = truncateForEvent(snapText, 40000)
				}
				sb.WriteString(snapText)
				if refs, ok := snapshot["refs"]; ok {
					refsJSON, _ := json.Marshal(refs)
					sb.WriteString(fmt.Sprintf("\n\n## Element Refs\n%s", string(refsJSON)))
				}
				return sb.String(), nil
			}
		}
		// 降级: ARIA 不可用，使用 innerText
		content, err := bc.GetContent(ctx)
		if err != nil {
			return fmt.Sprintf("[Browser get_content error: %s]", err), nil
		}
		if len(content) > 50000 {
			content = truncateForEvent(content, 50000)
		}
		return content, nil

	case "click":
		if input.Selector == "" {
			return "[Error: selector is required for click action]", nil
		}
		if err := bc.Click(ctx, input.Selector); err != nil {
			errClass := classifyBrowserError(err)
			if errClass == "transient" {
				return fmt.Sprintf("[Browser click error (transient — element may still be loading, try wait_for or observe first): %s]", err), nil
			}
			if errClass == "fatal" {
				return fmt.Sprintf("[Browser click error (fatal — CDP connection lost): %s]", err), nil
			}
			return fmt.Sprintf("[Browser click error (structural — use observe to check page state): %s]", err), nil
		}
		// Phase 3: 操作后截图验证
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("Clicked element: %s", input.Selector)), nil

	case "type":
		if input.Selector == "" || input.Text == "" {
			return "[Error: selector and text are required for type action]", nil
		}
		if err := bc.Type(ctx, input.Selector, input.Text); err != nil {
			errClass := classifyBrowserError(err)
			if errClass == "transient" {
				return fmt.Sprintf("[Browser type error (transient — element may still be loading): %s]", err), nil
			}
			if errClass == "fatal" {
				return fmt.Sprintf("[Browser type error (fatal — CDP connection lost): %s]", err), nil
			}
			return fmt.Sprintf("[Browser type error (structural — use observe to check page state): %s]", err), nil
		}
		// Phase 3: 操作后截图验证
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("Typed text into: %s", input.Selector)), nil

	case "screenshot":
		data, mimeType, err := bc.Screenshot(ctx)
		if err != nil {
			return fmt.Sprintf("[Browser screenshot error: %s]", err), nil
		}
		// D3-F1: 将截图以多模态格式返回，使 LLM 能真正看到图像内容。
		// 参照 executeArgusTool 的 __MULTIMODAL__ 返回路径。
		if len(data) > 0 {
			blocks := []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("Screenshot captured (%s, %d bytes)", mimeType, len(data))},
				{"type": "image", "source": map[string]interface{}{
					"type":       "base64",
					"media_type": mimeType,
					"data":       base64.StdEncoding.EncodeToString(data),
				}},
			}
			if blocksJSON, jErr := json.Marshal(blocks); jErr == nil {
				return "__MULTIMODAL__" + string(blocksJSON), nil
			}
		}
		return fmt.Sprintf("Screenshot captured (%s)", mimeType), nil

	case "evaluate":
		if !params.BrowserEvaluateEnabled {
			return "[Browser evaluate is disabled by configuration. Set browser.evaluateEnabled=true to enable JavaScript execution.]", nil
		}
		if input.Script == "" {
			return "[Error: script is required for evaluate action]", nil
		}
		result, err := bc.Evaluate(ctx, input.Script)
		if err != nil {
			return fmt.Sprintf("[Browser evaluate error: %s]", err), nil
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return fmt.Sprintf("[Browser evaluate: result not serializable: %s]", err), nil
		}
		return string(resultJSON), nil

	case "wait_for":
		if input.Selector == "" {
			return "[Error: selector is required for wait_for action]", nil
		}
		if err := bc.WaitForSelector(ctx, input.Selector); err != nil {
			return fmt.Sprintf("[Browser wait_for error: %s]", err), nil
		}
		return fmt.Sprintf("Element found: %s", input.Selector), nil

	case "go_back":
		if err := bc.GoBack(ctx); err != nil {
			return fmt.Sprintf("[Browser go_back error: %s]", err), nil
		}
		return "Navigated back", nil

	case "go_forward":
		if err := bc.GoForward(ctx); err != nil {
			return fmt.Sprintf("[Browser go_forward error: %s]", err), nil
		}
		return "Navigated forward", nil

	case "get_url":
		url, err := bc.GetURL(ctx)
		if err != nil {
			return fmt.Sprintf("[Browser get_url error: %s]", err), nil
		}
		return url, nil

	// ---------- Phase 1: ARIA 快照 + ref 元素交互 ----------

	case "observe":
		// 获取 ARIA 无障碍树快照 + 截图，让 LLM 通过结构化数据理解页面。
		// 行业对标: browser-use (SOM 标注)、Anthropic CU (AX Tree)、MultiOn (AX Tree)
		snapshot, snapErr := bc.SnapshotAI(ctx)
		data, mimeType, scrErr := bc.Screenshot(ctx)

		var parts []string

		// ARIA 快照部分
		if snapErr != nil {
			parts = append(parts, fmt.Sprintf("[ARIA snapshot error: %s]", snapErr))
		} else if snapshot != nil {
			if snapText, ok := snapshot["snapshot"].(string); ok {
				parts = append(parts, "## Page Structure (ARIA Accessibility Tree)\n")
				parts = append(parts, "Use element refs (e.g. e1, e2) with click_ref/fill_ref actions.\n")
				if len(snapText) > 30000 {
					snapText = truncateForEvent(snapText, 30000)
				}
				parts = append(parts, snapText)
			}
			if refs, ok := snapshot["refs"]; ok {
				refsJSON, _ := json.Marshal(refs)
				parts = append(parts, fmt.Sprintf("\n## Element Refs\n%s", string(refsJSON)))
			}
		}

		// 以多模态格式返回截图 + ARIA 文本
		if scrErr == nil && len(data) > 0 {
			text := strings.Join(parts, "\n")
			blocks := []map[string]interface{}{
				{"type": "text", "text": text},
				{"type": "image", "source": map[string]interface{}{
					"type":       "base64",
					"media_type": mimeType,
					"data":       base64.StdEncoding.EncodeToString(data),
				}},
			}
			if blocksJSON, jErr := json.Marshal(blocks); jErr == nil {
				return "__MULTIMODAL__" + string(blocksJSON), nil
			}
		}

		// 降级: 无截图，仅返回 ARIA 文本
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), nil
		}
		return "[observe failed: no snapshot or screenshot available]", nil

	case "annotate_som":
		// Phase 4.3: SOM 视觉标注 — 注入数字编号覆盖层 + 截图。
		screenshot, mimeType, annotations, err := bc.AnnotateSOM(ctx)
		if err != nil {
			return fmt.Sprintf("[Browser annotate_som error: %s]", err), nil
		}
		var annotText strings.Builder
		annotText.WriteString(fmt.Sprintf("SOM: %d interactive elements found.\n", len(annotations)))
		for _, a := range annotations {
			text := a.Text
			if len(text) > 40 {
				text = text[:40] + "..."
			}
			annotText.WriteString(fmt.Sprintf("[%d] %s (role=%s) %q\n", a.Index, a.Tag, a.Role, text))
		}
		if len(screenshot) > 0 {
			blocks := []map[string]interface{}{
				{"type": "text", "text": annotText.String()},
				{"type": "image", "source": map[string]interface{}{
					"type":       "base64",
					"media_type": mimeType,
					"data":       base64.StdEncoding.EncodeToString(screenshot),
				}},
			}
			if blocksJSON, jErr := json.Marshal(blocks); jErr == nil {
				return "__MULTIMODAL__" + string(blocksJSON), nil
			}
		}
		return annotText.String(), nil

	case "click_ref":
		// 通过 ARIA ref 点击元素，比 CSS selector 更健壮。
		if input.Ref == "" {
			return "[Error: ref is required for click_ref action (e.g. \"e1\")]", nil
		}
		if err := bc.ClickRef(ctx, input.Ref); err != nil {
			errClass := classifyBrowserError(err)
			if errClass == "transient" {
				return fmt.Sprintf("[Browser click_ref error (transient — try observe to refresh refs): %s]", err), nil
			}
			if errClass == "fatal" {
				return fmt.Sprintf("[Browser click_ref error (fatal — CDP connection lost): %s]", err), nil
			}
			return fmt.Sprintf("[Browser click_ref error (structural — ref may be stale, run observe again): %s]", err), nil
		}
		// Phase 3: 操作后截图验证
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("Clicked element ref: %s", input.Ref)), nil

	case "fill_ref":
		// 通过 ARIA ref 填入文本。
		if input.Ref == "" || input.Text == "" {
			return "[Error: ref and text are required for fill_ref action]", nil
		}
		if err := bc.FillRef(ctx, input.Ref, input.Text); err != nil {
			errClass := classifyBrowserError(err)
			if errClass == "transient" {
				return fmt.Sprintf("[Browser fill_ref error (transient — try observe to refresh refs): %s]", err), nil
			}
			if errClass == "fatal" {
				return fmt.Sprintf("[Browser fill_ref error (fatal — CDP connection lost): %s]", err), nil
			}
			return fmt.Sprintf("[Browser fill_ref error (structural — ref may be stale, run observe again): %s]", err), nil
		}
		// Phase 3: 操作后截图验证
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("Filled text into element ref: %s", input.Ref)), nil

	case "ai_browse":
		// Phase 4: Mariner 风格意图级浏览任务。
		// 接受 goal 参数，在隔离循环中执行 observe→plan→act，最多 20 步。
		// 极大减少主对话 round-trip 和 token 消耗。
		if input.Goal == "" {
			return "[Error: goal is required for ai_browse action (e.g. \"Search for MacBook Pro on jd.com\")]", nil
		}
		result, err := bc.AIBrowse(ctx, input.Goal)
		if err != nil {
			errClass := classifyBrowserError(err)
			if errClass == "fatal" {
				return fmt.Sprintf("[Browser ai_browse error (fatal): %s]", err), nil
			}
			return fmt.Sprintf("[Browser ai_browse error: %s]", err), nil
		}
		// 附带最终截图
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("AI Browse completed.\n\n%s", result)), nil

	// ---------- Phase 4.4: GIF Recording ----------

	case "start_gif_recording":
		if bc.IsGIFRecording() {
			return "[GIF recording already in progress]", nil
		}
		bc.StartGIFRecording()
		return "GIF recording started. Subsequent browser actions will be captured.", nil

	case "stop_gif_recording":
		if !bc.IsGIFRecording() {
			return "[No GIF recording in progress]", nil
		}
		gifData, frameCount, err := bc.StopGIFRecording()
		if err != nil {
			return fmt.Sprintf("[Browser stop_gif_recording error: %s]", err), nil
		}
		blocks := []map[string]interface{}{
			{"type": "text", "text": fmt.Sprintf("GIF recording complete: %d frames, %d bytes", frameCount, len(gifData))},
			{"type": "image", "source": map[string]interface{}{
				"type":       "base64",
				"media_type": "image/gif",
				"data":       base64.StdEncoding.EncodeToString(gifData),
			}},
		}
		if blocksJSON, jErr := json.Marshal(blocks); jErr == nil {
			return "__MULTIMODAL__" + string(blocksJSON), nil
		}
		return fmt.Sprintf("GIF recording complete: %d bytes", len(gifData)), nil

	// ---------- Tab Management ----------

	case "list_tabs":
		tabs, err := bc.ListTabs(ctx)
		if err != nil {
			return fmt.Sprintf("[Browser list_tabs error: %s]", err), nil
		}
		tabsJSON, _ := json.Marshal(tabs)
		return fmt.Sprintf("## Browser Tabs\n\n%s", string(tabsJSON)), nil

	case "create_tab":
		tab, err := bc.CreateTab(ctx, input.URL)
		if err != nil {
			return fmt.Sprintf("[Browser create_tab error: %s]", err), nil
		}
		return fmt.Sprintf("Created new tab: id=%s url=%s", tab.ID, tab.URL), nil

	case "close_tab":
		if input.TargetID == "" {
			return "[Error: target_id is required for close_tab action]", nil
		}
		if err := bc.CloseTab(ctx, input.TargetID); err != nil {
			return fmt.Sprintf("[Browser close_tab error: %s]", err), nil
		}
		return fmt.Sprintf("Closed tab: %s", input.TargetID), nil

	case "switch_tab":
		if input.TargetID == "" {
			return "[Error: target_id is required for switch_tab action]", nil
		}
		if err := bc.SwitchTab(ctx, input.TargetID); err != nil {
			return fmt.Sprintf("[Browser switch_tab error: %s]", err), nil
		}
		return browserResultWithScreenshot(ctx, bc, fmt.Sprintf("Switched to tab: %s", input.TargetID)), nil

	default:
		return fmt.Sprintf("[Unknown browser action: %s]", input.Action), nil
	}
}

// browserResultWithScreenshot 在操作结果中附带截图，让 LLM 验证操作效果。
// Phase 3: Anthropic CU 核心推荐 — 每个改变页面状态的操作后自动截图。
// 如果截图失败，仅返回纯文本结果（不影响操作本身）。
// maxBrowserScreenshotBytes caps embedded screenshots at 512 KB raw (~680 KB base64).
// Larger screenshots are silently dropped to avoid bloating LLM context.
const maxBrowserScreenshotBytes = 512 * 1024

func browserResultWithScreenshot(ctx context.Context, bc interface {
	Screenshot(ctx context.Context) ([]byte, string, error)
}, textResult string) string {
	data, mimeType, err := bc.Screenshot(ctx)
	if err != nil || len(data) == 0 {
		return textResult
	}
	if len(data) > maxBrowserScreenshotBytes {
		// Screenshot too large for inline embedding; return text only.
		return textResult
	}
	blocks := []map[string]interface{}{
		{"type": "text", "text": textResult},
		{"type": "image", "source": map[string]interface{}{
			"type":       "base64",
			"media_type": mimeType,
			"data":       base64.StdEncoding.EncodeToString(data),
		}},
	}
	if blocksJSON, jErr := json.Marshal(blocks); jErr == nil {
		return "__MULTIMODAL__" + string(blocksJSON)
	}
	return textResult
}

// ---------- helpers ----------

func resolveToolPath(path, workspaceDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workspaceDir, path)
}

// ---------- argus (Argus 视觉子智能体工具) ----------

// executeArgusTool 将 argus_ 前缀的工具调用转发给 Argus MCP Bridge。
// 根据 ActionRiskLevel 决定是否需要用户审批：
//   - RiskNone（截图/读取）→ 直接执行
//   - RiskMedium/High → 按 approvalMode 判断是否需要确认
func executeArgusTool(ctx context.Context, name string, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	mcpToolName := strings.TrimPrefix(name, "argus_")

	// NoNetwork 合约约束: 阻断需要网络的 Argus 操作
	if mcpToolName == "open_url" && params.DelegationContract != nil && params.DelegationContract.Constraints.NoNetwork {
		return "[Argus open_url blocked: delegation contract prohibits network access]", nil
	}

	// 风险分级审批门
	risk := ClassifyActionRisk(mcpToolName)
	approvalMode := "medium_and_above" // 默认模式
	if params.ArgusApprovalMode != "" {
		approvalMode = params.ArgusApprovalMode
	}

	slog.Debug("argus tool call",
		"tool", mcpToolName,
		"risk", RiskLevelString(risk),
		"approvalMode", approvalMode,
	)

	if ShouldRequireApproval(risk, approvalMode) {
		if isNonWebChannel(params.SessionKey) {
			// Fix 1: 非 Web 渠道 — RiskHigh 走 escalation（飞书审批卡片），不静默放行
			if risk >= RiskHigh {
				if params.CoderConfirmation != nil {
					approved, err := params.CoderConfirmation.RequestConfirmation(ctx, name, inputJSON, params.SessionKey)
					if err != nil {
						return fmt.Sprintf("[Argus approval error: %s]", err), nil
					}
					if !approved {
						return "[User denied argus operation]", nil
					}
				} else {
					// Fix 3: Fail-closed: 无审批门控 + 高风险 → 拒绝
					return "[Argus high-risk operation blocked: no approval gate on non-web channel]", nil
				}
			} else {
				// RiskMedium: 非 Web 渠道保持 auto-approve（click/type 是常规操作）
				slog.Info("non-web channel auto-approved argus tool (medium risk)",
					"tool", mcpToolName,
					"risk", RiskLevelString(risk),
					"sessionKey", params.SessionKey,
				)
			}
		} else if params.CoderConfirmation != nil {
			// Web 频道走 CoderConfirmation 弹窗（现有逻辑不变）
			approved, err := params.CoderConfirmation.RequestConfirmation(ctx, name, inputJSON, params.SessionKey)
			if err != nil {
				return fmt.Sprintf("[Argus approval error: %s]", err), nil
			}
			if !approved {
				return "[User denied argus operation]", nil
			}
		} else {
			// Fix 3: Fail-closed: CoderConfirmation==nil + Web 频道 → 拒绝
			return "[Argus operation blocked: no approval gate available]", nil
		}
	}

	// 优先使用多模态返回（保留 image blocks），降级到纯文本
	if params.ArgusBridge != nil {
		blocks, mmErr := params.ArgusBridge.AgentCallToolMultimodal(ctx, mcpToolName, inputJSON, 30*time.Second)
		slog.Debug("argus multimodal result", "tool", mcpToolName, "blockCount", len(blocks), "error", mmErr)
		if mmErr == nil && len(blocks) > 0 {
			// 检查是否包含 image blocks
			hasImage := false
			for _, b := range blocks {
				slog.Debug("argus block", "type", b.Type, "hasSource", b.Source != nil, "textLen", len(b.Text))
				if b.Type == "image" {
					hasImage = true
				}
			}
			if hasImage {
				// 返回特殊前缀标记，由 attempt_runner 识别并构建多模态 tool_result
				jsonBytes, jErr := json.Marshal(blocks)
				if jErr == nil {
					slog.Info("argus: returning __MULTIMODAL__ with image", "tool", mcpToolName, "jsonLen", len(jsonBytes))
					return "__MULTIMODAL__" + string(jsonBytes), nil
				}
				slog.Warn("argus: json marshal failed, falling through", "error", jErr)
			}
			// 无 image，提取文本
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(b.Text)
				}
			}
			slog.Debug("argus: no image blocks, returning text", "textLen", sb.Len())
			return sb.String(), nil
		}
		// multimodal 失败，降级
		if mmErr != nil {
			slog.Debug("argus multimodal failed, falling back to text", "error", mmErr)
		}
	}

	// 降级：纯文本模式
	if params.ArgusBridge == nil {
		return "[Argus screen observer is not available. Ensure gateway is running with Argus enabled.]", nil
	}
	output, err := params.ArgusBridge.AgentCallTool(ctx, mcpToolName, inputJSON, 30*time.Second)
	if err != nil {
		return fmt.Sprintf("[Argus tool error: %s]", err), nil
	}
	return output, nil
}

// ---------- remote (MCP 远程工具) ----------

// executeRemoteTool 将 remote_ 前缀的工具调用转发给远程 MCP Bridge。
func executeRemoteTool(ctx context.Context, name string, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	mcpToolName := strings.TrimPrefix(name, "remote_")

	slog.Debug("remote tool call", "tool", mcpToolName)

	output, err := params.RemoteMCPBridge.AgentCallRemoteTool(ctx, mcpToolName, inputJSON, 30*time.Second)
	if err != nil {
		return fmt.Sprintf("[Remote tool error: %s]", err), nil
	}
	return output, nil
}

// (Phase 2A: executeCoderTool / injectContextBrief 已删除 — oa-coder 升级为 spawn_coder_agent)

// validateToolPath 验证路径不会逃逸工作空间。
// 所有文件/目录操作工具在执行前必须调用此函数。
// 如果路径不在 workspaceDir 内，返回错误以阻止越界访问。
func validateToolPath(path, workspaceDir string) error {
	if workspaceDir == "" {
		return nil // 无工作空间约束
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return fmt.Errorf("invalid workspace path: %w", err)
	}
	// 允许工作空间本身及其子路径
	if absPath == absWorkspace || strings.HasPrefix(absPath, absWorkspace+string(filepath.Separator)) {
		return nil
	}
	// 🚫 路径在工作空间外 — 拒绝访问
	return fmt.Errorf("path %q is outside workspace %q — access denied", path, workspaceDir)
}

func pathAllowedByMountRequests(path string, mountRequests []MountRequestForSandbox, requireWrite bool) bool {
	if len(mountRequests) == 0 {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, mr := range mountRequests {
		hostPath := strings.TrimSpace(mr.HostPath)
		if hostPath == "" {
			continue
		}
		absMount, err := filepath.Abs(hostPath)
		if err != nil {
			continue
		}
		mode := strings.ToLower(strings.TrimSpace(mr.MountMode))
		if requireWrite && mode != "rw" {
			continue
		}
		if absPath == absMount || strings.HasPrefix(absPath, absMount+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// validateToolPathScoped 合约多路径校验。
// 当 ScopePaths 非空时使用此函数替代 validateToolPath（单根）。
// 路径必须在至少一个 scope path、工作区，或已批准的临时挂载路径下才放行。
func validateToolPathScoped(path string, scopePaths []string, workspaceDir string, mountRequests []MountRequestForSandbox, requireWrite bool) error {
	if pathAllowedByMountRequests(path, mountRequests, requireWrite) {
		return nil
	}
	if len(scopePaths) == 0 {
		// 无合约 scope → 回退到 workspace 单根校验
		return validateToolPath(path, workspaceDir)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	for _, sp := range scopePaths {
		// scope path 可能是相对路径，基于 workspace 解析
		absSP := sp
		if !filepath.IsAbs(sp) && workspaceDir != "" {
			absSP = filepath.Join(workspaceDir, sp)
		}
		absSP, err = filepath.Abs(absSP)
		if err != nil {
			continue
		}
		if absPath == absSP || strings.HasPrefix(absPath, absSP+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside contract scope — access denied", path)
}

// permissionDeniedPrefix 权限拒绝提示的固定前缀，用于检测。
const permissionDeniedPrefix = "🚫 权限不足 | Permission Denied"

// IsPermissionDeniedOutput 检测工具输出是否为权限拒绝消息。
func IsPermissionDeniedOutput(output string) bool {
	return strings.Contains(output, permissionDeniedPrefix)
}

// formatPermissionDenied 格式化权限拒绝提示。
// 返回结构化的醒目文本，包含工具名、目标、当前安全级别和操作说明。
func formatPermissionDenied(tool, detail, level string) string {
	if level == "" {
		level = "deny"
	}
	levelDesc := map[string]string{
		"deny":      "L0 (只读/Read Only) — 不允许写入和执行",
		"allowlist": "L1 (允许列表/Allowlist) — 仅允许预批准命令",
		"full":      "L2 (完全/Full Access)",
	}
	desc := levelDesc[level]
	if desc == "" {
		desc = level
	}

	toolDesc := tool
	switch tool {
	case "bash":
		toolDesc = "bash (命令执行)"
	case "write_file":
		toolDesc = "write_file (文件写入)"
	case "send_media":
		toolDesc = "send_media (文件发送)"
	}

	return fmt.Sprintf(`🚫 权限不足 | Permission Denied
━━━━━━━━━━━━━━━━━━━━━━━━━━━
工具 Tool:   %s
目标 Target: %s
安全级别:    %s

💡 请在聊天窗口的权限弹窗中点击「临时授权」放行本次操作，
   或前往 安全设置 修改安全级别。
   Use the permission popup in chat to temporarily authorize,
   or change your security level in Settings → Security.`, toolDesc, detail, desc)
}

// ---------- send_media 工具 ----------

// sendMediaInput send_media 工具参数。
type sendMediaInput struct {
	Target      string `json:"target"`       // "channel:id" 格式，空值时使用 SessionKey
	FilePath    string `json:"file_path"`    // 本地文件路径（优先）
	FileName    string `json:"file_name"`    // 显式文件名（media_base64 模式建议提供）
	MediaBase64 string `json:"media_base64"` // base64 编码数据
	MimeType    string `json:"mime_type"`    // MIME 类型（file_path 模式下可自动检测）
	Message     string `json:"message"`      // 随文件一起发送的文字
}

// maxMediaFileSize 最大媒体文件大小（30MB，匹配飞书 UploadFile 限制）。
const maxMediaFileSize = 30 * 1024 * 1024

// executeSendMedia 执行 send_media 工具调用。
func executeSendMedia(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	if params.MediaSender == nil {
		return "[send_media] Media sender is not available. No channel is configured.", nil
	}

	var input sendMediaInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Sprintf("[send_media] Invalid input: %v", err), nil
	}

	// 解析 target
	channelID, to := parseSendMediaTarget(input.Target, params.SessionKey)
	if channelID == "" {
		return "[send_media] No target specified and no session channel available. Tip: omit 'target' entirely to use the current conversation channel. Do NOT fabricate channel IDs.", nil
	}

	var data []byte
	fileName := strings.TrimSpace(input.FileName)
	mimeType := input.MimeType

	switch {
	case input.FilePath != "":
		// 路径安全校验：workspace/scope 边界（与 read_file/write_file 一致）
		if err := validateToolPathScoped(input.FilePath, params.ScopePaths, params.WorkspaceDir, params.MountRequests, false); err != nil {
			detail := strings.TrimSpace(input.FilePath)
			if absPath, absErr := filepath.Abs(detail); absErr == nil {
				detail = absPath
			}
			if params.OnPermissionDenied != nil {
				params.OnPermissionDenied("send_media", detail)
			}
			return formatPermissionDenied("send_media", input.FilePath, params.SecurityLevel), nil
		}

		// 先检查文件大小，避免大文件 OOM
		fi, err := os.Stat(input.FilePath)
		if err != nil {
			return fmt.Sprintf("[send_media] Failed to stat file: %v. Ensure the path is absolute and the file exists. Use 'ls' to verify the file path first.", err), nil
		}
		if fi.Size() > maxMediaFileSize {
			return fmt.Sprintf("[send_media] File too large: %d bytes (max %d bytes / 30MB)", fi.Size(), maxMediaFileSize), nil
		}

		fileData, err := os.ReadFile(input.FilePath)
		if err != nil {
			return fmt.Sprintf("[send_media] Failed to read file: %v", err), nil
		}
		data = fileData
		if fileName == "" {
			fileName = filepath.Base(input.FilePath)
		}

		// MIME 自动检测
		if mimeType == "" {
			mimeType = detectMimeType(input.FilePath)
		}

	case input.MediaBase64 != "":
		decoded, err := base64.StdEncoding.DecodeString(input.MediaBase64)
		if err != nil {
			return fmt.Sprintf("[send_media] Invalid base64 data: %v", err), nil
		}
		if len(decoded) > maxMediaFileSize {
			return fmt.Sprintf("[send_media] Data too large: %d bytes (max %d bytes / 30MB)", len(decoded), maxMediaFileSize), nil
		}
		data = decoded

	default:
		return "[send_media] Either file_path or media_base64 is required. Use file_path with an absolute path (e.g. '/tmp/image.png'). To send a screenshot: first run 'screencapture -x /tmp/screenshot.png' via bash, then call send_media with file_path='/tmp/screenshot.png'.", nil
	}

	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if fileName == "" {
		fileName = defaultSendMediaFileName(mimeType)
	}

	slog.Info("send_media: sending file",
		"target", channelID+":"+to,
		"fileName", fileName,
		"mimeType", mimeType,
		"size", len(data),
	)

	if err := params.MediaSender.SendMedia(ctx, channelID, to, data, fileName, mimeType, input.Message); err != nil {
		return formatSendMediaDeliveryError(err), nil
	}

	// 保存本地副本，供 Web 端显示（内联实现，避免循环依赖）。
	var savedPath string
	if cfgDir, cfgErr := os.UserConfigDir(); cfgErr == nil {
		dir := filepath.Join(cfgDir, "openacosmi", "media", "sent-media")
		if mkErr := os.MkdirAll(dir, 0o700); mkErr == nil {
			ext := ".bin"
			if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
				ext = exts[0]
			}
			if f, fErr := os.CreateTemp(dir, "sent-*"+ext); fErr == nil {
				if _, wErr := f.Write(data); wErr == nil {
					savedPath = f.Name()
				} else {
					slog.Warn("send_media: failed to write local copy", "error", wErr)
				}
				f.Close()
			}
		}
	}

	// 构建状态 JSON（text block）。
	statusResult := map[string]interface{}{
		"status":   "sent",
		"target":   channelID + ":" + to,
		"fileName": fileName,
		"size":     len(data),
		"mimeType": mimeType,
	}
	if savedPath != "" {
		statusResult["savedPath"] = savedPath
	}
	statusJSON, _ := json.Marshal(statusResult)

	// 小于 5MB 时返回 __MULTIMODAL__，触发 mediaBlocks 传播链 → 前端显示图片。
	const maxMultimodalBytes = 5 * 1024 * 1024
	if len(data) <= maxMultimodalBytes && strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		blocks := []map[string]interface{}{
			{"type": "text", "text": string(statusJSON)},
			{"type": "image", "source": map[string]interface{}{
				"type":       "base64",
				"media_type": mimeType,
				"data":       base64.StdEncoding.EncodeToString(data),
			}},
		}
		if blocksJSON, err := json.Marshal(blocks); err == nil {
			return "__MULTIMODAL__" + string(blocksJSON), nil
		}
	}
	return string(statusJSON), nil
}

type sendMediaDeliveryErrorInfo interface {
	SendCode() string
	SendChannel() string
	SendOperation() string
	SendRetryable() bool
	SendUserMessage() string
}

func formatSendMediaDeliveryError(err error) string {
	if err == nil {
		return "[send_media] Failed to send: unknown error"
	}

	var sendErr sendMediaDeliveryErrorInfo
	if errors.As(err, &sendErr) && sendErr != nil {
		baseMessage := strings.TrimSpace(sendErr.SendUserMessage())
		if baseMessage == "" {
			baseMessage = strings.TrimSpace(err.Error())
		}

		var metaParts []string
		sendCode := strings.TrimSpace(sendErr.SendCode())
		metaParts = append(metaParts, "sendCode="+sendCode)
		if channel := strings.TrimSpace(sendErr.SendChannel()); channel != "" {
			metaParts = append(metaParts, "channel="+channel)
		}
		if op := strings.TrimSpace(sendErr.SendOperation()); op != "" {
			metaParts = append(metaParts, "op="+op)
		}
		if sendErr.SendRetryable() {
			metaParts = append(metaParts, "retryable=true")
		}

		msg := "[send_media] Delivery failed (" + strings.Join(metaParts, ", ") + "): " + baseMessage
		switch sendCode {
		case "payload_too_large":
			msg += ". 建议压缩文件或降低分辨率后重试。"
		case "unsupported_feature":
			msg += ". 当前通道暂不支持该媒体能力，可改为发送文本说明或公网媒体链接。"
		case "unauthorized":
			msg += ". 请检查通道凭证和授权状态。"
		case "unavailable", "upstream_error":
			msg += ". 通道暂时不可用，可稍后重试。"
		}
		return msg
	}

	return fmt.Sprintf("[send_media] Failed to send: %v", err)
}

// parseSendMediaTarget 解析 target 为 channelID + to。
// target 格式: "channel:id"（按第一个 ":" 分割）。
// 空值时 fallback 到 sessionKey。
func parseSendMediaTarget(target, sessionKey string) (channelID, to string) {
	raw := target
	if raw == "" {
		raw = sessionKey
	}
	if raw == "" {
		return "", ""
	}
	idx := strings.Index(raw, ":")
	if idx < 0 {
		return raw, ""
	}
	return raw[:idx], raw[idx+1:]
}

// detectMimeType 从文件扩展名推导 MIME 类型。
func detectMimeType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	// 办公文档扩展名 → MIME（mime 标准库可能不识别这些）
	officeTypes := map[string]string{
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".ppt":  "application/vnd.ms-powerpoint",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".doc":  "application/vnd.ms-word",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".xls":  "application/vnd.ms-excel",
	}
	if t, ok := officeTypes[ext]; ok {
		return t
	}

	// 标准库检测（覆盖 .pdf, .png, .jpg, .gif, .mp4 等）
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}

	return "application/octet-stream"
}

func defaultSendMediaFileName(mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		return "upload" + exts[0]
	}
	return "upload.bin"
}

// ---------- memory tools (UHMS 记忆系统) ----------

type memorySearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type memoryGetInput struct {
	ID string `json:"id"`
}

// executeMemorySearch 通过 UHMS Bridge 搜索长期记忆。
func executeMemorySearch(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	if params.UHMSBridge == nil {
		return "[memory_search unavailable: memory system not configured]", nil
	}

	var input memorySearchInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Sprintf("[memory_search input error: %v]", err), nil
	}
	if input.Query == "" {
		return "[memory_search error: query is required]", nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	results, err := params.UHMSBridge.SearchMemories(ctx, input.Query, limit)
	if err != nil {
		return fmt.Sprintf("[memory_search error: %v]", err), nil
	}
	if len(results) == 0 {
		return "No memories found matching the query.", nil
	}

	out, err := json.Marshal(results)
	if err != nil {
		return fmt.Sprintf("[memory_search marshal error: %v]", err), nil
	}
	return string(out), nil
}

// executeMemoryGet 通过 UHMS Bridge 获取单条记忆详情。
func executeMemoryGet(ctx context.Context, inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	if params.UHMSBridge == nil {
		return "[memory_get unavailable: memory system not configured]", nil
	}

	var input memoryGetInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return fmt.Sprintf("[memory_get input error: %v]", err), nil
	}
	if input.ID == "" {
		return "[memory_get error: id is required]", nil
	}

	hit, err := params.UHMSBridge.GetMemory(ctx, input.ID)
	if err != nil {
		return fmt.Sprintf("[memory_get error: %v]", err), nil
	}
	if hit == nil {
		return fmt.Sprintf("No memory found with id: %s", input.ID), nil
	}

	out, err := json.Marshal(hit)
	if err != nil {
		return fmt.Sprintf("[memory_get marshal error: %v]", err), nil
	}
	return string(out), nil
}
