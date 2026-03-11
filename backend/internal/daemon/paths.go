package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

var (
	windowsAbsolutePath = regexp.MustCompile(`^[a-zA-Z]:[/\\]`)
	windowsUNCPath      = regexp.MustCompile(`^\\\\`)
)

// ResolveHomeDir 从环境变量中解析用户主目录
// 对应 TS: paths.ts resolveHomeDir
func ResolveHomeDir(env map[string]string) (string, error) {
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		home = strings.TrimSpace(env["USERPROFILE"])
	}
	if home == "" {
		// 回退到 os.UserHomeDir
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", errors.New("missing HOME")
		}
	}
	return home, nil
}

// ResolveUserPathWithHome 解析用户路径，支持 ~ 展开
// 对应 TS: paths.ts resolveUserPathWithHome
func ResolveUserPathWithHome(input, home string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return trimmed, nil
	}

	// 处理 ~ 开头的路径
	if strings.HasPrefix(trimmed, "~") {
		if home == "" {
			return "", errors.New("missing HOME")
		}
		// 替换 ~ 前缀（仅当后续为 / 或 \ 或字符串结束时）
		if trimmed == "~" {
			return filepath.Clean(home), nil
		}
		if trimmed[1] == '/' || trimmed[1] == '\\' {
			expanded := home + trimmed[1:]
			return filepath.Clean(expanded), nil
		}
		// ~username 格式，不展开
		return filepath.Clean(trimmed), nil
	}

	// Windows 绝对路径
	if windowsAbsolutePath.MatchString(trimmed) || windowsUNCPath.MatchString(trimmed) {
		return trimmed, nil
	}

	return filepath.Clean(trimmed), nil
}

// ResolveGatewayStateDir 解析 gateway 状态目录
// 对应 TS: paths.ts resolveGatewayStateDir
func ResolveGatewayStateDir(env map[string]string) (string, error) {
	override := strings.TrimSpace(env["OPENACOSMI_STATE_DIR"])
	if override != "" {
		var home string
		if strings.HasPrefix(override, "~") {
			var err error
			home, err = ResolveHomeDir(env)
			if err != nil {
				return "", err
			}
		}
		return ResolveUserPathWithHome(override, home)
	}

	home, err := ResolveHomeDir(env)
	if err != nil {
		return "", err
	}
	suffix := ResolveGatewayProfileSuffix(env["OPENACOSMI_PROFILE"])
	return filepath.Join(home, ".openacosmi"+suffix), nil
}

// ResolveGatewayLogPaths 解析 gateway 日志文件路径
// 对应 TS: launchd.ts resolveGatewayLogPaths（部分逻辑抽取）
func ResolveGatewayLogPaths(env map[string]string) (stdoutPath, stderrPath string) {
	stateDir, err := ResolveGatewayStateDir(env)
	if err != nil {
		stateDir = fallbackGatewayStateDir(env)
	}
	logDir := filepath.Join(stateDir, "logs")

	prefix := "gateway"
	if v, ok := env["OPENACOSMI_LOG_PREFIX"]; ok && v != "" {
		prefix = v
	}

	if runtime.GOOS == "darwin" {
		stdoutPath = filepath.Join(logDir, prefix+".stdout.log")
		stderrPath = filepath.Join(logDir, prefix+".stderr.log")
	} else {
		// Linux/Windows 使用 journal 或自定义路径
		stdoutPath = filepath.Join(logDir, prefix+".stdout.log")
		stderrPath = filepath.Join(logDir, prefix+".stderr.log")
	}
	return
}

func fallbackGatewayStateDir(env map[string]string) string {
	home, err := ResolveHomeDir(env)
	if err != nil {
		home, _ = os.UserHomeDir()
	}
	if strings.TrimSpace(home) == "" {
		home = "."
	}
	suffix := ResolveGatewayProfileSuffix(env["OPENACOSMI_PROFILE"])
	return filepath.Join(home, config.NewStateDirname+suffix)
}
