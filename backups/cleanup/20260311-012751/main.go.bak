//go:build !desktopwails

// Package main 提供桌面端的启动骨架。
//
// 当前阶段先复用 Gateway 生命周期，作为 Wails 外壳接入前的
// 可运行、可测试 bootstrap harness。后续接入 Wails 时，窗口/
// 托盘生命周期应直接复用 runtime.go 中的逻辑。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

var version = "dev"

func main() {
	portOverride := flag.Int("port", 0, "Desktop gateway port override")
	controlUIDir := flag.String("control-ui-dir", "", "Path to control UI static files for desktop dev mode")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("desktop %s\n", version)
		return
	}

	config.BuildVersion = version

	loader := config.NewConfigLoader()
	cfg, err := loader.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load config failed, continuing with defaults: %v\n", err)
	}
	opts := resolveDesktopGatewayOptions(cfg, *controlUIDir, os.Stat)

	bootstrap, err := prepareDesktopBootstrap(
		cfg,
		loader.ConfigPath(),
		*portOverride,
		opts,
		os.Stat,
		infra.WaitForGateway,
		startGatewayRuntime,
		probeDesktopURL,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "desktop bootstrap failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(
		"desktop bootstrap ready: url=%s attachedExisting=%t onboarding=%t\n",
		bootstrap.URL,
		bootstrap.AttachedExisting,
		bootstrap.NeedsOnboarding,
	)

	waitForDesktopShutdown(bootstrap.Runtime)
}

func waitForDesktopShutdown(runtime runtimeCloser) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	sig := <-sigChan
	if runtime == nil {
		fmt.Fprintf(os.Stderr, "desktop exiting on signal: %s\n", sig)
		return
	}
	if err := runtime.Close(fmt.Sprintf("desktop bootstrap signal: %s", sig)); err != nil {
		fmt.Fprintf(os.Stderr, "desktop shutdown warning: %v\n", err)
	}
}
