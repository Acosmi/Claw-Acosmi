//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchdService 实现 macOS LaunchAgent 服务管理
type launchdService struct{}

func (s *launchdService) Label() string         { return "LaunchAgent" }
func (s *launchdService) LoadedText() string    { return "loaded" }
func (s *launchdService) NotLoadedText() string { return "not loaded" }

// Install 安装 LaunchAgent
// 对应 TS: launchd.ts installLaunchAgent
func (s *launchdService) Install(args GatewayServiceInstallArgs) error {
	plistPath := resolveLaunchAgentPlistPath(args.Env)

	// 确保目录存在
	dir := filepath.Dir(plistPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}

	// 解析日志路径
	stateDir, err := ResolveGatewayStateDir(args.Env)
	if err != nil {
		return fmt.Errorf("解析状态目录失败: %w", err)
	}
	logDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	label := resolveInstallLaunchdLabel(args.Env)
	stdoutPath := filepath.Join(logDir, "gateway.stdout.log")
	stderrPath := filepath.Join(logDir, "gateway.stderr.log")

	plist := BuildLaunchAgentPlist(LaunchAgentPlistParams{
		Label:            label,
		Comment:          args.Description,
		ProgramArguments: args.ProgramArguments,
		WorkingDirectory: args.WorkingDirectory,
		StdoutPath:       stdoutPath,
		StderrPath:       stderrPath,
		Environment:      args.Environment,
	})

	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("写入 plist 文件失败: %w", err)
	}

	// launchctl bootstrap
	uid := os.Getuid()
	target := fmt.Sprintf("gui/%d", uid)
	cmd := exec.Command("launchctl", "bootstrap", target, plistPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap 失败: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// Uninstall 卸载 LaunchAgent
// 对应 TS: launchd.ts uninstallLaunchAgent
func (s *launchdService) Uninstall(env map[string]string) error {
	plistPath := resolveLaunchAgentPlistPath(env)
	label := resolveInstallLaunchdLabel(env)

	// launchctl bootout
	uid := os.Getuid()
	target := fmt.Sprintf("gui/%d/%s", uid, label)
	cmd := exec.Command("launchctl", "bootout", target)
	_ = cmd.Run() // 忽略错误（可能已经卸载）

	// 删除 plist 文件
	_ = os.Remove(plistPath)
	return nil
}

// Stop 停止 LaunchAgent
// 对应 TS: launchd.ts stopLaunchAgent
func (s *launchdService) Stop(env map[string]string) error {
	label := resolveInstallLaunchdLabel(env)
	uid := os.Getuid()
	target := fmt.Sprintf("gui/%d/%s", uid, label)
	cmd := exec.Command("launchctl", "bootout", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootout 失败: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Restart 重启 LaunchAgent
// 对应 TS: launchd.ts restartLaunchAgent
func (s *launchdService) Restart(env map[string]string) error {
	label := resolveInstallLaunchdLabel(env)
	uid := os.Getuid()

	// 先 bootout（停止），忽略错误
	target := fmt.Sprintf("gui/%d/%s", uid, label)
	stopCmd := exec.Command("launchctl", "bootout", target)
	_ = stopCmd.Run()

	// 再 bootstrap（启动）
	plistPath := resolveLaunchAgentPlistPath(env)
	bootstrapTarget := fmt.Sprintf("gui/%d", uid)
	startCmd := exec.Command("launchctl", "bootstrap", bootstrapTarget, plistPath)
	if out, err := startCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap 失败: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// IsLoaded 检查 LaunchAgent 是否已加载
// 对应 TS: launchd.ts isLaunchAgentLoaded
func (s *launchdService) IsLoaded(env map[string]string) (bool, error) {
	label := resolveInstallLaunchdLabel(env)
	uid := os.Getuid()
	target := fmt.Sprintf("gui/%d/%s", uid, label)
	cmd := exec.Command("launchctl", "print", target)
	err := cmd.Run()
	return err == nil, nil
}

// ReadCommand 读取已安装 LaunchAgent 的命令配置
// 对应 TS: launchd.ts readLaunchAgentProgramArguments
func (s *launchdService) ReadCommand(env map[string]string) (*GatewayServiceCommand, error) {
	plistPath := resolveLaunchAgentPlistPath(env)
	return ReadLaunchAgentProgramArgumentsFromFile(plistPath)
}

// ReadRuntime 读取 LaunchAgent 运行时状态
// 对应 TS: launchd.ts readLaunchAgentRuntime
func (s *launchdService) ReadRuntime(env map[string]string) (GatewayServiceRuntime, error) {
	label := resolveInstallLaunchdLabel(env)
	uid := os.Getuid()
	target := fmt.Sprintf("gui/%d/%s", uid, label)

	cmd := exec.Command("launchctl", "print", target)
	out, err := cmd.Output()
	if err != nil {
		// 检查 plist 是否存在
		plistPath := resolveLaunchAgentPlistPath(env)
		if _, statErr := os.Stat(plistPath); statErr == nil {
			return GatewayServiceRuntime{
				Status:      "stopped",
				Detail:      "plist exists but service not loaded",
				CachedLabel: true,
			}, nil
		}
		return GatewayServiceRuntime{
			Status:      "stopped",
			MissingUnit: true,
		}, nil
	}

	kv := ParseKeyValueOutput(string(out), "=")
	runtime := GatewayServiceRuntime{
		Status: "running",
	}
	if v, ok := kv["state"]; ok {
		runtime.State = v
		if strings.EqualFold(v, "not running") {
			runtime.Status = "stopped"
		}
	}
	if v, ok := kv["pid"]; ok {
		if pid, err := parseInt(v); err == nil {
			runtime.PID = pid
		}
	}
	if v, ok := kv["last exit status"]; ok {
		if code, err := parseInt(v); err == nil {
			runtime.LastExitStatus = code
		}
	}

	return runtime, nil
}

// resolveLaunchAgentPlistPath 解析 LaunchAgent plist 文件路径
// 对应 TS: launchd.ts resolveLaunchAgentPlistPath
func resolveLaunchAgentPlistPath(env map[string]string) string {
	home, err := ResolveHomeDir(env)
	if err != nil {
		home, _ = os.UserHomeDir()
	}
	label := ResolveGatewayLaunchAgentLabel(env["OPENACOSMI_PROFILE"])
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// ResolveLaunchAgentPlistPath 导出版本，供外部模块调用
func ResolveLaunchAgentPlistPath(env map[string]string) string {
	return resolveLaunchAgentPlistPath(env)
}

// resolveInstallLaunchdLabel 解析安装时使用的 launchd 标签
func resolveInstallLaunchdLabel(env map[string]string) string {
	if label, ok := env["OPENACOSMI_LAUNCHD_LABEL"]; ok && label != "" {
		return label
	}
	return ResolveGatewayLaunchAgentLabel(env["OPENACOSMI_PROFILE"])
}

// ResolveGatewayLogPathsDarwin 解析 macOS 下的 gateway 日志路径
// 对应 TS: launchd.ts resolveGatewayLogPaths
func ResolveGatewayLogPathsDarwin(env map[string]string) (stdoutPath, stderrPath string) {
	stateDir, err := ResolveGatewayStateDir(env)
	if err != nil {
		stateDir = fallbackGatewayStateDir(env)
	}
	logDir := filepath.Join(stateDir, "logs")
	prefix := "gateway"
	if v, ok := env["OPENACOSMI_LOG_PREFIX"]; ok && v != "" {
		prefix = v
	}
	return filepath.Join(logDir, prefix+".stdout.log"), filepath.Join(logDir, prefix+".stderr.log")
}

// parseInt 简单的整数解析
func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// newLaunchdService 创建 launchd 服务实例
func newLaunchdService() GatewayService {
	return &launchdService{}
}
