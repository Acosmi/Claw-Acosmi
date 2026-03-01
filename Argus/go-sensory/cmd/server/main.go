package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"Argus-compound/go-sensory/internal/agent"
	"Argus-compound/go-sensory/internal/api"
	"Argus-compound/go-sensory/internal/capture"
	"Argus-compound/go-sensory/internal/imaging"
	"Argus-compound/go-sensory/internal/input"
	"Argus-compound/go-sensory/internal/ipc"
	"Argus-compound/go-sensory/internal/mcp"
	"Argus-compound/go-sensory/internal/pipeline"
	"Argus-compound/go-sensory/internal/vlm"
)

// cliFlags holds parsed command-line arguments.
type cliFlags struct {
	fps           int
	port          int
	backend       string
	shmEnabled    bool
	openBrowser   bool
	vlmConfigPath string
	mcpMode       bool
}

func parseFlags() cliFlags {
	fps := flag.Int("fps", 0, "Screen capture FPS; 0=auto (display refresh rate / 6, min 5)")
	port := flag.Int("port", 8090, "API server port")
	backend := flag.String("backend", "sck", "Capture backend: sck (ScreenCaptureKit) or cg (CoreGraphics)")
	shmEnabled := flag.Bool("shm", true, "Enable shared memory IPC for interprocess communication")
	openBrowser := flag.Bool("open-browser", true, "Automatically open dashboard in browser on startup")
	vlmConfigPath := flag.String("vlm-config", "", "Path to VLM provider config JSON (optional, falls back to env vars)")
	mcpMode := flag.Bool("mcp", false, "Run as MCP (Model Context Protocol) server on stdio")
	flag.Parse()

	// Auto-detect VLM config from .app bundle Resources directory
	if *vlmConfigPath == "" {
		if bundleConfig := detectBundleVLMConfig(); bundleConfig != "" {
			*vlmConfigPath = bundleConfig
		}
	}

	return cliFlags{
		fps:           *fps,
		port:          *port,
		backend:       *backend,
		shmEnabled:    *shmEnabled,
		openBrowser:   *openBrowser,
		vlmConfigPath: *vlmConfigPath,
		mcpMode:       *mcpMode,
	}
}

// detectBundleVLMConfig checks if the binary is running inside a .app bundle
// and returns the path to vlm-config.json in the Resources directory.
func detectBundleVLMConfig() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	// Resolve symlinks to get the real path
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return ""
	}

	// Check if we're inside a .app/Contents/MacOS/ structure
	dir := filepath.Dir(exePath)
	if filepath.Base(dir) != "MacOS" {
		return ""
	}
	contentsDir := filepath.Dir(dir)
	if filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	bundleDir := filepath.Dir(contentsDir)
	if !strings.HasSuffix(bundleDir, ".app") {
		return ""
	}

	// Look for vlm-config.json in Resources
	configPath := filepath.Join(contentsDir, "Resources", "vlm-config.json")
	if _, err := os.Stat(configPath); err == nil {
		log.Printf("[Bundle] Found VLM config: %s", configPath)
		return configPath
	}
	return ""
}

// initCapture creates and starts the screen capturer + optional SHM writer.
func initCapture(flags cliFlags) (capture.Capturer, *ipc.ShmWriter) {
	config := capture.DefaultConfig()
	config.Backend = capture.CaptureBackend(flags.backend)

	capturer, err := capture.NewCapturer(config)
	if err != nil {
		log.Fatalf("Failed to create capturer: %v", err)
	}

	dispInfo := capturer.DisplayInfo()
	fmt.Printf("Display: %s\n", dispInfo)

	// Auto-detect FPS from display refresh rate if not specified
	actualFPS := flags.fps
	if actualFPS <= 0 {
		if dispInfo.RefreshRateHz > 0 {
			actualFPS = dispInfo.RefreshRateHz / 6
		}
		if actualFPS < 5 {
			actualFPS = 5
		}
		if actualFPS > 30 {
			actualFPS = 30
		}
	}
	config.FPS = actualFPS

	if err := capturer.Start(actualFPS); err != nil {
		log.Fatalf("Failed to start capture: %v", err)
	}
	fmt.Printf("Screen capture started at %d FPS\n", actualFPS)

	// Shared memory IPC (optional)
	var shmWriter *ipc.ShmWriter
	if flags.shmEnabled {
		ipc.CleanupDefaults()
		shmWriter, err = ipc.NewShmWriter(dispInfo.Width*2, dispInfo.Height*2, 4) // *2 for Retina
		if err != nil {
			log.Printf("Warning: failed to create ShmWriter: %v (continuing without IPC)", err)
		} else {
			fmt.Printf("Shared memory IPC enabled: %v\n", shmWriter.ShmInfo())
			shmFrames := capturer.Subscribe()
			go func() {
				for frame := range shmFrames {
					if err := shmWriter.WriteFrame(frame.Width, frame.Height, frame.Channels, frame.Pixels); err != nil {
						log.Printf("SHM write error: %v", err)
					}
				}
			}()
		}
	}

	return capturer, shmWriter
}

// initVLM loads VLM config and creates the router + health checker.
// Always returns a non-nil Router so config CRUD routes are available
// even when no providers are initially configured.
func initVLM(configPath string) *vlm.Router {
	var vlmCfg *vlm.VLMConfig

	// 1) Try explicit config path
	// 2) Try persisted vlm-config.json
	// 3) Fall back to env vars
	if configPath != "" {
		var err error
		vlmCfg, err = vlm.LoadConfigFromFile(configPath)
		if err != nil {
			log.Printf("Warning: failed to load VLM config from %s: %v (falling back)", configPath, err)
			vlmCfg = nil
		}
	}

	if vlmCfg == nil {
		// Try loading persisted config file
		defaultPath := "vlm-config.json"
		if cfg, err := vlm.LoadConfigFromFile(defaultPath); err == nil {
			log.Printf("[VLM] Loaded persisted config from %s (%d providers)", defaultPath, len(cfg.Providers))
			vlmCfg = cfg
		} else {
			vlmCfg = vlm.LoadConfigFromEnv()
		}
	}

	// Always create router (even with 0 providers) so config CRUD is available
	router, err := vlm.NewRouter(vlmCfg)
	if err != nil {
		log.Printf("Warning: failed to create VLM router: %v", err)
		// Create empty router as last resort
		router, _ = vlm.NewRouter(&vlm.VLMConfig{})
	}

	if len(vlmCfg.Providers) > 0 {
		hc := vlm.NewHealthChecker(router, 30*time.Second)
		router.SetHealthChecker(hc)
		hc.Start()
	}

	return router
}

// initPipeline creates the frame processing pipeline and wires it to the capturer.
func initPipeline(capturer capture.Capturer) (*pipeline.Pipeline, *pipeline.KeyframeExtractor) {
	kfExtractor := pipeline.NewKeyframeExtractor(pipeline.DefaultKeyframeConfig())
	piiFilter := pipeline.NewPIIFilter()
	pipe := pipeline.NewPipeline(kfExtractor, piiFilter, 32)
	pipe.Start()
	fmt.Println("Frame pipeline initialized (keyframe + PII)")

	pipeFrames := capturer.Subscribe()
	go func() {
		var frameNo int64
		for frame := range pipeFrames {
			frameNo++
			pipe.Submit(pipeline.FrameInput{
				Pixels:    frame.Pixels,
				Width:     frame.Width,
				Height:    frame.Height,
				FrameNo:   frameNo,
				Timestamp: float64(frame.FrameNo),
			})
		}
	}()

	return pipe, kfExtractor
}

// printEndpoints logs all available endpoints.
func printEndpoints(port int, hasVLM bool) {
	fmt.Printf("API server started on port %d\n", port)
	fmt.Printf("  ├── Dashboard:         http://localhost:%d\n", port)
	fmt.Printf("  ├── WebSocket frames:  ws://localhost:%d/ws/frames\n", port)
	fmt.Printf("  ├── WebSocket control: ws://localhost:%d/ws/control\n", port)
	fmt.Printf("  ├── Action REST:       http://localhost:%d/api/action\n", port)
	fmt.Printf("  ├── Status:            http://localhost:%d/api/status\n", port)
	if hasVLM {
		fmt.Printf("  ├── VLM Chat:          http://localhost:%d/v1/chat/completions\n", port)
		fmt.Printf("  ├── VLM Health:        http://localhost:%d/api/vlm/health\n", port)
		fmt.Printf("  ├── Config API:        http://localhost:%d/api/config/providers\n", port)
	} else {
		fmt.Printf("  ├── VLM: not configured (set VLM_API_BASE or use -vlm-config)\n")
	}
	fmt.Printf("  └── Pipeline Stats:    http://localhost:%d/api/pipeline/stats\n", port)
}

// waitForShutdown blocks until SIGINT/SIGTERM, then performs graceful shutdown.
func waitForShutdown(capturer capture.Capturer, pipe *pipeline.Pipeline, shmWriter *ipc.ShmWriter, vlmRouter *vlm.Router, monitor *pipeline.Monitor) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("\nPress Ctrl+C to stop.")

	<-sigCh
	fmt.Println("\nShutting down...")

	// Second Ctrl+C forces immediate exit
	go func() {
		<-sigCh
		fmt.Println("\nForce exit!")
		os.Exit(1)
	}()

	done := make(chan struct{})
	go func() {
		if monitor != nil {
			monitor.Stop()
		}
		capturer.Stop()
		pipe.Stop()
		if shmWriter != nil {
			shmWriter.Close()
		}
		if vlmRouter != nil {
			vlmRouter.Close()
		}
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("Bye!")
	case <-time.After(3 * time.Second):
		fmt.Println("Shutdown timeout, force exit!")
		os.Exit(1)
	}
}

// checkPermissions verifies macOS permissions and prints guidance.
// If permissions are missing, opens System Settings directly.
func checkPermissions() {
	axClient := agent.NewRustAccessibility()
	perms := axClient.CheckPermissions()

	allOK := true
	if !perms.Accessibility {
		fmt.Println("⚠️  请在 系统设置 → 隐私与安全 → 辅助功能 中启用 Argus")
		allOK = false
	}
	if !perms.ScreenRecording {
		fmt.Println("⚠️  请在 系统设置 → 隐私与安全 → 屏幕录制 中启用 Argus")
		allOK = false
	}
	if !allOK {
		fmt.Println("ℹ️  部分功能将受限运行，授权后重启应用即可完全启用")

		// Open System Settings
		if !perms.Accessibility {
			_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Start()
		}

		if !perms.ScreenRecording {
			_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture").Start()
			// Explicitly trigger the permission request (system prompt)
			axClient.RequestScreenCapture()
		}

		// Show GUI Guide Dialog (Blocking)
		script := `display dialog "Argus 需要屏幕录制权限才能运行。" & return & return & "请在随后的系统设置窗口中找到 Argus 并开启权限，然后重启应用。" with title "权限申请" buttons {"好的"} default button "好的" with icon caution`
		_ = exec.Command("osascript", "-e", script).Run()
	} else {
		fmt.Println("✅ 系统权限检查通过")
	}
}

func main() {
	flags := parseFlags()

	// ── MCP stdio mode ──
	if flags.mcpMode {
		runMCPServer(flags)
		return
	}

	// ── Standard HTTP server mode ──
	fmt.Println("╔═══════════════════════════════════════════════╗")
	fmt.Println("║   Argus Compound — Go Sensory Motor System    ║")
	fmt.Println("╚═══════════════════════════════════════════════╝")

	// ── Permission check ──
	checkPermissions()

	capturer, shmWriter := initCapture(flags)
	inputCtrl := input.NewInputController()
	fmt.Println("Input controller initialized")

	vlmRouter := initVLM(flags.vlmConfigPath)
	guardrails := input.NewActionGuardrails("")
	fmt.Println("Action guardrails initialized")

	pipe, kfExtractor := initPipeline(capturer)

	// Initialize dual-track image scaler (VLM: 1024px+q50, Display: original+q80)
	scaler := imaging.NewScaler()

	server := api.NewServer(capturer, inputCtrl, flags.port, vlmRouter, scaler)
	server.SetGuardrails(guardrails)
	server.SetPipeline(pipe, kfExtractor)

	// Initialize VLM monitor (periodic screen analysis, default every 30s)
	var monitor *pipeline.Monitor
	if vlmRouter != nil && vlmRouter.ActiveProvider() != nil {
		monitor = pipeline.NewMonitor(capturer, scaler, vlmRouter.ActiveProvider())
		server.SetMonitor(monitor)
		monitor.Start()
	}

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("API server error: %v", err)
		}
	}()

	printEndpoints(flags.port, vlmRouter != nil)

	if flags.openBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			dashboardURL := fmt.Sprintf("http://localhost:%d", flags.port)
			if err := exec.Command("open", dashboardURL).Start(); err != nil {
				log.Printf("Warning: failed to open browser: %v", err)
			}
		}()
	}

	waitForShutdown(capturer, pipe, shmWriter, vlmRouter, monitor)
}

// runMCPServer starts the MCP server in stdio mode.
//
// This is the integration point that wires all MCP tools to the
// same core components used by the HTTP server.
func runMCPServer(flags cliFlags) {
	log.SetOutput(os.Stderr) // stdout is reserved for MCP protocol
	log.Println("[MCP] Starting Argus Sensory MCP Server...")

	// Initialize core components (shared with HTTP mode)
	capturer, _ := initCapture(flags)
	defer capturer.Stop()

	inputCtrl := input.NewInputController()
	scaler := imaging.NewScaler()

	// Initialize VLM (optional)
	vlmRouter := initVLM(flags.vlmConfigPath)
	var vlmProvider vlm.Provider
	if vlmRouter != nil && vlmRouter.ActiveProvider() != nil {
		vlmProvider = vlmRouter.ActiveProvider()
	}

	// Initialize ApprovalGateway for MCP mode.
	// AutoMode=true: 主系统 (Claw Acismi gateway) 统一负责审批，
	// Argus 作为 MCP 子进程只负责执行。Guardrails（硬拦截 sudo/rm/dd）仍然生效。
	guardrails := input.NewActionGuardrails("")
	gateway := input.NewApprovalGateway(input.GatewayConfig{
		Guardrails: guardrails,
		Enabled:    true,
		AutoMode:   true, // MCP mode: approval handled by main system
	})

	// Build registry and register all tools
	registry := mcp.NewRegistry()

	// Create UI parser with AX-first detection + VLM fallback
	axClient := agent.NewRustAccessibility()
	uiParser := agent.NewUIParser(vlmProvider, scaler, axClient)

	// Phase 3: Perception tools
	mcp.RegisterPerceptionTools(registry, mcp.PerceptionDeps{
		Capturer: capturer,
		Scaler:   scaler,
		VLM:      vlmProvider,
		Parser:   uiParser,
	})

	// Phase 4: Action tools
	mcp.RegisterActionTools(registry, mcp.ActionDeps{
		Input:   inputCtrl,
		Gateway: gateway,
	})

	// Phase 5: macOS tools
	mcp.RegisterMacOSTools(registry, mcp.MacOSDeps{
		Input:   inputCtrl,
		Gateway: gateway,
	})

	// Phase 5.5: Shell tools
	mcp.RegisterShellTools(registry, mcp.ShellDeps{
		Gateway: gateway,
	})

	tools := registry.List()
	log.Printf("[MCP] Registered %d tools", len(tools))
	for _, tool := range tools {
		log.Printf("[MCP]   ├── %s (%s, risk=%d)", tool.Name(), tool.Category(), tool.Risk())
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	srv := mcp.NewServer(registry)
	go func() {
		<-sigCh
		log.Println("[MCP] Received shutdown signal")
		srv.Stop()
	}()

	if err := srv.Run(); err != nil && err != context.Canceled {
		log.Fatalf("[MCP] Server error: %v", err)
	}

	log.Println("[MCP] Server stopped")
	os.Exit(0)
}
