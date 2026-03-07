// cmd/browser-mcp — Standalone MCP server for OpenAcosmi browser automation.
// Runs as stdio transport for MCP clients (Claude Code, Cursor, VS Code).
//
// Usage:
//   openacosmi-browser-mcp [--cdp-url ws://127.0.0.1:9222]
//
// MCP config (claude_desktop_config.json):
//   {
//     "mcpServers": {
//       "openacosmi-browser": {
//         "command": "openacosmi-browser-mcp",
//         "args": ["--cdp-url", "ws://127.0.0.1:9222"]
//       }
//     }
//   }
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
	"github.com/Acosmi/ClawAcosmi/internal/browser/mcpserver"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cdpURL := flag.String("cdp-url", "", "CDP WebSocket URL (auto-discovers if empty)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Resolve CDP URL: explicit flag → auto-discover → auto-launch.
	resolvedURL := *cdpURL
	var managedChrome *browser.ChromeInstance
	if resolvedURL == "" {
		result, err := browser.EnsureChrome(context.Background(), logger)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		resolvedURL = result.WSURL
		if result.Instance != nil {
			managedChrome = result.Instance
			logger.Info("auto-launched Chrome, will stop on exit")
		}
	}

	logger.Info("browser MCP server starting", "cdpURL", resolvedURL)

	// Create browser controller.
	cdpTools := browser.NewCDPPlaywrightTools(resolvedURL, logger)
	controller := browser.NewPlaywrightBrowserController(cdpTools, resolvedURL)

	// Create MCP server.
	srv := mcpserver.NewServer(controller, logger)

	// Run with stdio transport.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	defer func() {
		if managedChrome != nil {
			logger.Info("stopping managed Chrome instance")
			_ = managedChrome.Stop()
		}
	}()

	transport := &sdkmcp.StdioTransport{}
	if err := srv.Run(ctx, transport); err != nil {
		logger.Error("MCP server error", "err", err)
		os.Exit(1)
	}
}
