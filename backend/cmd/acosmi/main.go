// Package main 是 Open Acosmi Gateway 服务的入口程序。
// 负责初始化配置、启动 WebSocket RPC 网关服务器。
//
// 这是 Go 端的主要二进制。CLI 命令由 Rust 版 openacosmi 承担，
// 本二进制（acosmi）专注于 Gateway 服务端职责。
// 参见 docs/adr/001-rust-cli-go-gateway.md
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Acosmi/ClawAcosmi/internal/cli"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/gateway"
	"github.com/Acosmi/ClawAcosmi/pkg/i18n"
	"github.com/Acosmi/ClawAcosmi/pkg/log"
)

// version 由构建系统注入
var version = "dev"

func main() {
	// CLI flags
	port := flag.Int("port", 0, "Gateway port (overrides config; default 19001)")
	controlUIDir := flag.String("control-ui-dir", "", "Path to control UI static files")
	profile := flag.String("profile", "", "Use a named profile (isolates state under ~/.openacosmi-<name>)")
	dev := flag.Bool("dev", false, "Use dev profile (port 19001, state under ~/.openacosmi-dev)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("acosmi %s\n", version)
		os.Exit(0)
	}

	// Profile 解析
	if *dev {
		_ = os.Setenv("OPENACOSMI_PROFILE", "dev")
	} else if *profile != "" {
		_ = os.Setenv("OPENACOSMI_PROFILE", *profile)
	}

	// 初始化国际化（默认中文）
	i18n.Init(i18n.LangZhCN)

	// 初始化日志系统
	logger := log.New("acosmi")

	// 输出彩色启动 Logo
	cli.EmitBanner()

	logger.Info(i18n.T("app.starting", map[string]string{"version": version}))

	// 注入构建版本到 config 包
	config.BuildVersion = version

	// 加载配置以解析端口
	cfgLoader := config.NewConfigLoader()
	cfg, err := cfgLoader.LoadConfig()
	if err != nil {
		logger.Warn(fmt.Sprintf("配置加载失败，使用默认值: %v", err))
	}

	// 端口优先级: --port flag > config file > default (19001)
	var resolvedPort int
	if *port > 0 {
		resolvedPort = *port
	} else {
		var cfgPort *int
		if cfg != nil && cfg.Gateway != nil && cfg.Gateway.Port != nil {
			cfgPort = cfg.Gateway.Port
		}
		resolvedPort = config.ResolveGatewayPort(cfgPort)
	}

	// 启动网关（阻塞直到收到终止信号）
	opts := gateway.GatewayServerOptions{
		ControlUIDir: *controlUIDir,
	}
	if err := gateway.RunGatewayBlocking(resolvedPort, opts); err != nil {
		logger.Error(fmt.Sprintf("网关启动失败: %v", err))
		os.Exit(1)
	}

	logger.Info(i18n.T("app.shutdown", nil))
}
