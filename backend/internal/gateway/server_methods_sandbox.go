package gateway

// ============================================================================
// Sandbox 配置与状态 API
// sandbox.config.get — 获取沙箱配置
// sandbox.status — Docker 可用性检查 + 容器池状态
// sandbox.test — 测试沙箱执行
// ============================================================================

import (
	"context"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/sandbox"
)

// handleSandboxConfigGet 获取沙箱配置
func handleSandboxConfigGet(ctx *MethodHandlerContext) {
	cfg := sandbox.DefaultDockerConfig()
	dockerAvailable := sandbox.IsDockerAvailable()

	ctx.Respond(true, map[string]interface{}{
		"dockerAvailable": dockerAvailable,
		"config": map[string]interface{}{
			"image":          cfg.DockerImage,
			"memoryLimitMB":  cfg.MemoryLimitMB,
			"cpuQuota":       cfg.CPUQuota,
			"timeoutSecs":    cfg.TimeoutSecs,
			"networkEnabled": cfg.NetworkEnabled,
			"readOnlyRoot":   cfg.ReadOnlyRoot,
			"tmpfsSizeMB":    cfg.TmpfsSizeMB,
		},
	}, nil)
}

// handleSandboxStatus 检查沙箱状态
func handleSandboxStatus(ctx *MethodHandlerContext) {
	dockerAvailable := sandbox.IsDockerAvailable()

	secLevel := resolveBaseSecurityLevelWithConfig(ctx.Context.Config)

	ctx.Respond(true, map[string]interface{}{
		"dockerAvailable": dockerAvailable,
		"securityLevel":   secLevel,
		"sandboxEnabled":  secLevel == "sandboxed" || secLevel == "allowlist",
	}, nil)
}

// handleSandboxTest 测试沙箱执行
func handleSandboxTest(ctx *MethodHandlerContext) {
	if !sandbox.IsDockerAvailable() {
		ctx.Respond(false, nil, &ErrorShape{
			Code:    "sandbox.docker_unavailable",
			Message: "Docker is not available. Please install Docker and ensure the daemon is running.",
		})
		return
	}

	runner := sandbox.NewDockerRunner(sandbox.DefaultDockerConfig())

	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := runner.Execute(execCtx, "", []string{"echo", "Sandbox is working!"}, "")
	if err != nil {
		ctx.Respond(false, nil, &ErrorShape{
			Code:    "sandbox.test_failed",
			Message: "Sandbox test execution failed: " + err.Error(),
		})
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"success":    result.ExitCode == 0,
		"stdout":     result.Stdout,
		"stderr":     result.Stderr,
		"exitCode":   result.ExitCode,
		"durationMs": result.DurationMs,
	}, nil)
}

// RegisterSandboxMethods 注册沙箱相关的 API 方法
func RegisterSandboxMethods(registry *MethodRegistry) {
	registry.RegisterAll(map[string]GatewayMethodHandler{
		"sandbox.config.get": handleSandboxConfigGet,
		"sandbox.status":     handleSandboxStatus,
		"sandbox.test":       handleSandboxTest,
	})
}
