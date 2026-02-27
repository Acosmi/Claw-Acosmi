package runner

// ============================================================================
// Tool Executor — 工具调用执行分发器
// 对齐 TS: agents/pi-tools.ts → createOpenAcosmiCodingTools
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anthropic/open-acosmi/internal/infra"
	"github.com/anthropic/open-acosmi/internal/sandbox"
)

// NativeSandboxForAgent — runner 包不依赖 sandbox 包的接口。
// 使 executeBashSandboxed 可通过原生沙箱 Worker 执行命令（IPC），
// 而非 Docker 容器。由 gateway/server.go 的 adapter 实现。
type NativeSandboxForAgent interface {
	ExecuteSandboxed(ctx context.Context, cmd string, args []string, env map[string]string, timeoutMs int64) (stdout, stderr string, exitCode int, err error)
}

// ToolExecParams 工具执行参数。
type ToolExecParams struct {
	WorkspaceDir string
	TimeoutMs    int64
	// 权限守卫
	AllowWrite   bool // 是否允许写文件
	AllowExec    bool // 是否允许执行命令
	AllowNetwork bool // 是否允许网络访问（预留）
	SandboxMode  bool // L1 沙箱模式: bash 通过 Docker 容器执行
	// P3: 命令规则引擎
	Rules []infra.CommandRule // Allow/Ask/Deny 规则集
	// 权限拒绝事件回调
	OnPermissionDenied func(tool, detail string) // 通知网关广播 WebSocket 事件
	SecurityLevel      string                    // 当前安全级别 ("deny"/"allowlist"/"full")
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
	// SpawnSubagent 子智能体生成回调（可选，nil = spawn_coder_agent 返回合约但不启动 session）
	// 由 gateway/server.go 注入实现。
	SpawnSubagent SpawnSubagentFunc
	// 委托合约约束（可选，nil = 无合约约束）
	DelegationContract *DelegationContract
	// ScopePaths 合约允许的路径列表（从 DelegationContract.Scope 提取）。
	// 非空时，工具路径校验使用此列表替代 WorkspaceDir 单根。
	ScopePaths []string
}

// ExecuteToolCall 执行工具调用并返回文本结果。
// 当前支持: bash, read_file, write_file, list_dir, search, glob
// 延迟: browser, message, mcp, notebook_edit 等高级工具
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
						// Fail-closed: 无审批门控时直接拒绝，不允许 LLM 忽略后继续执行
						if params.CoderConfirmation == nil {
							slog.Warn("ask rule triggered but no approval gate, fail-closed deny",
								"command", bi.Command,
								"rule", ruleResult.Rule.Pattern,
							)
							return fmt.Sprintf("[Command blocked: no approval gate available for rule %s] %s", ruleResult.Rule.Pattern, ruleResult.Reason), nil
						}
						// 真正阻塞：广播审批请求到前端，等待用户 allow/deny 或超时
						approved, approvalErr := params.CoderConfirmation.RequestConfirmation(ctx, "bash", inputJSON)
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
						// approved: fall through → 继续执行
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
		return executeLookupSkill(inputJSON, params)
	case "spawn_coder_agent":
		return executeSpawnCoderAgent(ctx, inputJSON, params)
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

	stdout, stderr, exitCode, err := params.NativeSandbox.ExecuteSandboxed(
		ctx, "sh", []string{"-c", command}, nil, params.TimeoutMs,
	)
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
	if err := validateToolPathScoped(path, params.ScopePaths, params.WorkspaceDir); err != nil {
		if params.OnPermissionDenied != nil {
			params.OnPermissionDenied("write_file", input.Path)
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

// executeLookupSkill 从缓存返回完整 SKILL.md 内容。
func executeLookupSkill(inputJSON json.RawMessage, params ToolExecParams) (string, error) {
	var input lookupSkillInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", fmt.Errorf("invalid lookup_skill input: %w", err)
	}
	if input.Name == "" {
		return "", fmt.Errorf("lookup_skill: skill name is required")
	}

	if params.SkillsCache == nil {
		return fmt.Sprintf("[Skill %q not found: no skills loaded]", input.Name), nil
	}

	content, ok := params.SkillsCache[input.Name]
	if !ok {
		// 列出可用技能帮助 LLM 修正
		var available []string
		for name := range params.SkillsCache {
			available = append(available, name)
		}
		return fmt.Sprintf("[Skill %q not found. Available skills: %s]", input.Name, strings.Join(available, ", ")), nil
	}

	return content, nil
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

	if ShouldRequireApproval(risk, approvalMode) && params.CoderConfirmation != nil {
		approved, err := params.CoderConfirmation.RequestConfirmation(ctx, name, inputJSON)
		if err != nil {
			return fmt.Sprintf("[Argus approval error: %s]", err), nil
		}
		if !approved {
			return "[User denied argus operation]", nil
		}
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

// validateToolPathScoped 合约多路径校验。
// 当 ScopePaths 非空时使用此函数替代 validateToolPath（单根）。
// 路径必须在至少一个 scope path 下才放行。
func validateToolPathScoped(path string, scopePaths []string, workspaceDir string) error {
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
