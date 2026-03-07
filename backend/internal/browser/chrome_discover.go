// chrome_discover.go — Auto-discover running Chrome instances with CDP enabled.
// Probes common debugging ports to find reachable Chrome DevTools endpoints.
// Phase 4.1: EnsureChrome adds zero-config auto-launch when no Chrome is found.
package browser

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// DefaultDiscoveryPorts lists ports to probe for running Chrome CDP instances.
// Order: OpenAcosmi default range → standard Chrome debugging port → other common ports.
var DefaultDiscoveryPorts = []int{
	18800, // OpenAcosmi default profile
	18801, // OpenAcosmi secondary profile
	9222,  // Chrome default --remote-debugging-port
	9229,  // Alternative debugging port
}

// DiscoveryCDPResult holds the result of a successful discovery.
type DiscoveryCDPResult struct {
	Port    int
	CDPURL  string // HTTP endpoint (e.g. "http://127.0.0.1:9222")
	WSURL   string // WebSocket URL for CDP
	Browser string // Browser name from /json/version
}

// DiscoverRunningChrome probes the given ports for a reachable Chrome CDP endpoint.
// Returns the first successful result, or nil if none found.
func DiscoverRunningChrome(ports []int, logger *slog.Logger) *DiscoveryCDPResult {
	if logger == nil {
		logger = slog.Default()
	}
	if len(ports) == 0 {
		ports = DefaultDiscoveryPorts
	}

	for _, port := range ports {
		cdpURL := fmt.Sprintf("http://127.0.0.1:%d", port)
		if !IsChromeReachable(cdpURL, 300) {
			continue
		}

		// Get WebSocket URL and version info.
		version, err := FetchChromeVersion(cdpURL, 500)
		if err != nil {
			logger.Debug("chrome discovery: reachable but version failed", "port", port, "err", err)
			continue
		}

		wsURL, err := GetChromeWebSocketURL(cdpURL, 500)
		if err != nil {
			logger.Debug("chrome discovery: version ok but ws url failed", "port", port, "err", err)
			continue
		}

		logger.Info("chrome auto-discovered",
			"port", port,
			"browser", version.Browser,
			"wsURL", wsURL,
		)

		return &DiscoveryCDPResult{
			Port:    port,
			CDPURL:  cdpURL,
			WSURL:   wsURL,
			Browser: version.Browser,
		}
	}

	logger.Debug("chrome discovery: no running instances found", "ports", ports)
	return nil
}

// DiscoverOrDefault tries auto-discovery on default ports, returning the
// WebSocket URL if found, or empty string if no Chrome instance is available.
func DiscoverOrDefault(logger *slog.Logger) string {
	result := DiscoverRunningChrome(nil, logger)
	if result != nil {
		return result.WSURL
	}
	return ""
}

// ---------- Phase 4.1: Zero-config auto-launch ----------

// EnsureChromeResult holds the result of EnsureChrome().
type EnsureChromeResult struct {
	WSURL    string          // CDP WebSocket URL
	CDPURL   string          // HTTP CDP endpoint
	Instance *ChromeInstance // Non-nil only if we launched Chrome (caller must Stop())
	Launched bool            // True if we auto-launched, false if discovered existing
}

// EnsureChrome discovers a running Chrome, or auto-launches one if none found.
// If Chrome is auto-launched, the caller owns the returned ChromeInstance and
// must call Instance.Stop() on shutdown.
//
// Flow: discover → if not found → find executable → launch → return CDP URL.
func EnsureChrome(ctx context.Context, logger *slog.Logger) (*EnsureChromeResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Step 1: Try auto-discovery on default ports.
	if found := DiscoverRunningChrome(nil, logger); found != nil {
		return &EnsureChromeResult{
			WSURL:  found.WSURL,
			CDPURL: found.CDPURL,
		}, nil
	}

	// Step 2: No running Chrome — find executable and launch.
	logger.Info("no running Chrome found, attempting auto-launch")

	exe := ResolveBrowserExecutable(nil)
	if exe == nil {
		return nil, fmt.Errorf("no Chrome/Chromium browser found on this system; " +
			"install Chrome or start it manually with --remote-debugging-port=9222")
	}

	logger.Info("found Chrome executable", "kind", exe.Kind, "path", exe.Path)

	cdpPort := CDPPortRangeStart // 18800
	profile := &ResolvedBrowserProfile{
		Name:    DefaultProfileName,
		CDPPort: cdpPort,
		Color:   "#FF4500",
	}

	instance, err := LaunchOpenAcosmiChrome(ctx, ChromeStartConfig{
		Profile:    profile,
		Executable: exe,
		Logger:     logger,
	})
	if err != nil {
		return nil, fmt.Errorf("auto-launch Chrome failed: %w", err)
	}

	// Get the WebSocket URL.
	wsURL, err := instance.WaitForCDP(5 * time.Second)
	if wsURL == "" && err == nil {
		// WaitForCDP returned empty — try GetChromeWebSocketURL on the CDP port.
		cdpURL := CdpURLForPort(cdpPort)
		wsURL, err = GetChromeWebSocketURL(cdpURL, 2000)
	}
	if err != nil || wsURL == "" {
		_ = instance.Stop()
		return nil, fmt.Errorf("auto-launched Chrome but CDP not ready: %v", err)
	}

	cdpHTTP := CdpURLForPort(cdpPort)
	logger.Info("chrome auto-launched successfully",
		"pid", instance.cmd.Process.Pid,
		"cdpURL", cdpHTTP,
		"wsURL", wsURL,
	)

	return &EnsureChromeResult{
		WSURL:    wsURL,
		CDPURL:   cdpHTTP,
		Instance: instance,
		Launched: true,
	}, nil
}
