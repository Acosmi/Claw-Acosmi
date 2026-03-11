// =============================================================================
// 文件: backend/internal/sandbox/docker_runner.go | 模块: sandbox | 职责: Docker 容器安全隔离执行引擎
// 审计: V12 2026-02-21 | 状态: ✅ 已审计
// =============================================================================

// Package sandbox provides sandboxed code execution capabilities.
//
// [C-2.6] Docker 兜底路径 — 当 Wasm 不适用时 (如需要文件系统、网络) 使用 Alpine 容器。
// 安全层: --read-only + --no-new-privileges + --network=none + seccomp + resource limits
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
)

// DockerExecutionResult mirrors the Wasm ExecutionResult for Docker-based execution.
type DockerExecutionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
	// Duration in milliseconds
	DurationMs int64 `json:"duration_ms"`
}

// DockerRunnerConfig configures the Docker sandbox executor.
type DockerRunnerConfig struct {
	// DockerImage is the container image to use (default: "alpine:3.19")
	DockerImage string
	// MemoryLimitMB is the memory limit in MB (default: 256)
	MemoryLimitMB int
	// CPUQuota limits CPU (default: 1.0 = 1 core)
	CPUQuota float64
	// TimeoutSecs is max execution time in seconds (default: 300)
	TimeoutSecs int
	// NetworkEnabled enables network access (default: false)
	NetworkEnabled bool
	// ReadOnlyRoot makes the root filesystem read-only (default: true)
	ReadOnlyRoot bool
	// TmpfsSizeMB is the writable tmpfs size in MB (default: 64)
	TmpfsSizeMB int
	// Binds is a list of bind mounts in "host:container[:ro]" format
	Binds []string
	// WorkDir sets the working directory inside the container (default: empty = image default)
	WorkDir string
	// User sets the UID:GID to run as inside the container (default: empty = root)
	User string
}

// DefaultDockerConfig returns safe default configuration.
func DefaultDockerConfig() DockerRunnerConfig {
	return DockerRunnerConfig{
		DockerImage:    "alpine:3.19",
		MemoryLimitMB:  256,
		CPUQuota:       1.0,
		TimeoutSecs:    300,
		NetworkEnabled: false,
		ReadOnlyRoot:   true,
		TmpfsSizeMB:    64,
	}
}

// DockerRunner executes code in an isolated Docker container.
type DockerRunner struct {
	config DockerRunnerConfig
}

// NewDockerRunner creates a new Docker-based sandbox executor.
func NewDockerRunner(config DockerRunnerConfig) *DockerRunner {
	return &DockerRunner{config: config}
}

// IsDockerAvailable checks if Docker daemon is reachable.
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// Execute runs a command inside a sandboxed Docker container.
//
// Parameters:
//   - ctx: context for cancellation
//   - image: Docker image override (empty = use config default)
//   - command: the command to execute inside the container
//   - stdinData: data to pass via stdin
func (r *DockerRunner) Execute(ctx context.Context, image string, command []string, stdinData string) (*DockerExecutionResult, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("command is empty")
	}

	if image == "" {
		image = r.config.DockerImage
	}

	timeout := time.Duration(r.config.TimeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build docker run arguments with security constraints
	args := r.buildDockerArgs(image, command)

	slog.Debug("sandbox docker exec",
		"image", image,
		"command", strings.Join(command, " "),
		"memory_mb", r.config.MemoryLimitMB,
		"timeout_s", r.config.TimeoutSecs,
	)

	start := time.Now()
	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if stdinData != "" {
		cmd.Stdin = strings.NewReader(stdinData)
	}

	err := cmd.Run()
	duration := time.Since(start)

	result := &DockerExecutionResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: duration.Milliseconds(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = 137 // SIGKILL
			result.Error = fmt.Sprintf("execution timed out after %ds", r.config.TimeoutSecs)
		} else {
			result.ExitCode = 1
			result.Error = err.Error()
		}
	}

	return result, nil
}

// ExecuteScript writes a script to tmpfs and executes it.
//
// Parameters:
//   - ctx: context for cancellation
//   - image: Docker image (e.g., "python:3.12-alpine", "node:20-alpine")
//   - scriptContent: the script source code
//   - runtime: the runtime command (e.g., "python3", "node")
func (r *DockerRunner) ExecuteScript(ctx context.Context, image string, scriptContent string, runtime string) (*DockerExecutionResult, error) {
	// Strategy: pass script via stdin and use runtime's stdin-reading mode
	command := []string{runtime}

	// python and node can read from stdin by default
	// For safety, we use the explicit stdin flag where available
	switch runtime {
	case "python3", "python":
		command = []string{runtime, "-c", scriptContent}
		return r.Execute(ctx, image, command, "")
	case "node":
		command = []string{runtime, "-e", scriptContent}
		return r.Execute(ctx, image, command, "")
	case "sh", "bash":
		command = []string{runtime, "-c", scriptContent}
		return r.Execute(ctx, image, command, "")
	default:
		return r.Execute(ctx, image, command, scriptContent)
	}
}

// buildDockerArgs constructs the docker run arguments with security constraints.
func (r *DockerRunner) buildDockerArgs(image string, command []string) []string {
	args := []string{
		"run",
		"--rm",                // 自动清理容器
		"--pids-limit", "256", // 限制进程数
		fmt.Sprintf("--memory=%dm", r.config.MemoryLimitMB),
		fmt.Sprintf("--cpus=%.1f", r.config.CPUQuota),
	}

	// 网络隔离
	if !r.config.NetworkEnabled {
		args = append(args, "--network=none")
	}

	// 只读根文件系统
	if r.config.ReadOnlyRoot {
		args = append(args, "--read-only")
		// 提供可写 tmpfs
		args = append(args, fmt.Sprintf("--tmpfs=/tmp:rw,noexec,nosuid,size=%dm", r.config.TmpfsSizeMB))
	}

	// Bind mounts（工作区/技能/配置）
	for _, bind := range r.config.Binds {
		args = append(args, "-v", bind)
	}

	// 工作目录
	if r.config.WorkDir != "" {
		args = append(args, "-w", r.config.WorkDir)
	}

	// 运行用户
	if r.config.User != "" {
		args = append(args, "--user", r.config.User)
	}

	// Seccomp 限制 (使用 Docker 默认 profile, 已屏蔽 ~40 个危险系统调用)
	args = append(args, "--security-opt", "no-new-privileges:true")

	// Drop all capabilities, only add what's strictly needed
	args = append(args, "--cap-drop=ALL")

	// Image and command
	args = append(args, image)
	args = append(args, command...)

	return args
}

// ToJSON serializes the result to JSON.
func (r *DockerExecutionResult) ToJSON() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// ---------- L0/L1 挂载策略 ----------

// SandboxMountConfig 沙箱挂载参数。
type SandboxMountConfig struct {
	// SecurityLevel: "deny" (L0, 全局只读) 或 "sandbox" (L1, 工作区读写)
	SecurityLevel string
	// ProjectDir 用户项目工作区路径（宿主机绝对路径）
	ProjectDir string
	// HomeDir 用户家目录（留空则自动推导），用于挂载 ~/.openacosmi
	HomeDir string
	// SkillsDir 技能目录路径（留空则自动推导为 ~/.openacosmi/skills）
	SkillsDir string
	// ConfigFile 配置文件路径（留空则自动推导为 ~/.openacosmi/config.json）
	ConfigFile string
}

// ResolveSandboxMounts 根据安全级别计算 Docker bind mount 列表。
//
// 两级都保证全局可读——区别仅在于工作区是否可写：
//
//	L0 (deny):    工作区:ro + 技能:ro + 配置:ro  — 全局只读
//	L1 (sandbox): 工作区:rw + 技能:ro + 配置:ro  — 沙箱内完整读写
func ResolveSandboxMounts(mc SandboxMountConfig) (binds []string, workDir string, readOnlyRoot bool) {
	workDir = "/workspace"

	// 自动推导路径
	homeDir := mc.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	skillsDir := mc.SkillsDir
	if skillsDir == "" && homeDir != "" {
		skillsDir = filepath.Join(statepaths.ResolveStateDir(), "skills")
	}
	configFile := mc.ConfigFile
	if configFile == "" && homeDir != "" {
		configFile = config.ResolveCanonicalConfigPath()
	}

	// ── 公共只读挂载（L0 和 L1 共享）──
	if skillsDir != "" {
		binds = append(binds, skillsDir+":/skills:ro")
	}
	if configFile != "" {
		binds = append(binds, configFile+":/etc/acosmi/config.json:ro")
	}

	switch mc.SecurityLevel {
	case "sandbox": // L1: 工作区读写，容器根可写
		readOnlyRoot = false
		if mc.ProjectDir != "" {
			binds = append(binds, mc.ProjectDir+":/workspace:rw")
		}

	default: // L0 / deny: 全局只读
		readOnlyRoot = true
		if mc.ProjectDir != "" {
			binds = append(binds, mc.ProjectDir+":/workspace:ro")
		}
	}

	return binds, workDir, readOnlyRoot
}

// ApplyMountsToConfig 将挂载策略应用到 DockerRunnerConfig。
func ApplyMountsToConfig(cfg *DockerRunnerConfig, mc SandboxMountConfig) {
	binds, workDir, readOnlyRoot := ResolveSandboxMounts(mc)
	cfg.Binds = binds
	cfg.WorkDir = workDir
	cfg.ReadOnlyRoot = readOnlyRoot
}
