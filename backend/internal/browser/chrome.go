package browser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ChromeInstance manages a running Chrome process.
type ChromeInstance struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	profile  *ResolvedBrowserProfile
	cdpWSURL string
	dataDir  string
	logger   *slog.Logger
	stopped  bool
}

// ChromeStartConfig configures Chrome launch parameters.
type ChromeStartConfig struct {
	Profile    *ResolvedBrowserProfile
	Executable *BrowserExecutable
	DataDir    string
	Logger     *slog.Logger
}

// StartChrome launches a Chrome process with the given configuration.
func StartChrome(ctx context.Context, cfg ChromeStartConfig) (*ChromeInstance, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Executable == nil {
		return nil, fmt.Errorf("no Chrome executable found")
	}
	if cfg.Profile == nil {
		return nil, fmt.Errorf("no browser profile configured")
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = cfg.Profile.DataDir
	}
	if dataDir == "" {
		dataDir = defaultChromeDataDir(cfg.Profile.Name)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create data dir: %w", err)
	}

	args := buildChromeArgs(cfg.Profile, dataDir)

	// Use exec.Command (NOT exec.CommandContext) so Chrome survives task
	// context cancellation. The browser should persist across tasks and only
	// be stopped explicitly via Stop(). Binding to ctx would SIGKILL Chrome
	// when any parent task finishes or times out.
	cmd := exec.Command(cfg.Executable.Path, args...)
	cmd.Dir = dataDir

	// Redirect stderr for CDP URL discovery.
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("chrome start: %w", err)
	}

	cfg.Logger.Info("chrome started",
		"pid", cmd.Process.Pid,
		"executable", cfg.Executable.Path,
		"profile", cfg.Profile.Name,
		"cdpPort", cfg.Profile.CDPPort,
	)

	instance := &ChromeInstance{
		cmd:     cmd,
		profile: cfg.Profile,
		dataDir: dataDir,
		logger:  cfg.Logger,
	}

	// Read CDP WebSocket URL from stderr (Chrome outputs it on launch).
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				return
			}
			line := string(buf[:n])
			if strings.Contains(line, "DevTools listening on") {
				parts := strings.SplitAfter(line, "DevTools listening on ")
				if len(parts) > 1 {
					url := strings.TrimSpace(parts[1])
					if nl := strings.IndexByte(url, '\n'); nl > 0 {
						url = url[:nl]
					}
					instance.mu.Lock()
					instance.cdpWSURL = url
					instance.mu.Unlock()
					cfg.Logger.Info("chrome cdp ready", "wsUrl", url)
				}
			}
		}
	}()

	return instance, nil
}

func defaultChromeDataDir(profileName string) string {
	return filepath.Join(config.ResolveStateDir(), "browser-profiles", profileName)
}

func buildChromeArgs(profile *ResolvedBrowserProfile, dataDir string) []string {
	args := []string{
		"--remote-debugging-port=" + strconv.Itoa(profile.CDPPort),
		"--user-data-dir=" + dataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-component-update",
		"--disable-features=Translate,MediaRouter",
		"--disable-session-crashed-bubble",
		"--hide-crash-restore-bubble",
		"--password-store=basic",
		"--metrics-recording-only",
		"--safebrowsing-disable-auto-update",
	}
	if profile.Headless {
		args = append(args, "--headless=new", "--disable-gpu")
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--disable-dev-shm-usage")
	}
	// Always open a blank tab.
	args = append(args, "about:blank")
	return args
}

// CdpWSURL returns the Chrome DevTools WebSocket URL.
func (c *ChromeInstance) CdpWSURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cdpWSURL
}

// WaitForCDP waits for the CDP WebSocket URL to become available.
func (c *ChromeInstance) WaitForCDP(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if url := c.CdpWSURL(); url != "" {
			return url, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for CDP WebSocket URL")
}

// Stop gracefully terminates Chrome: SIGTERM → poll (2.5s) → SIGKILL.
// Aligns with TS chrome.ts stopOpenAcosmiChrome().
func (c *ChromeInstance) Stop() error {
	return c.StopWithTimeout(2500 * time.Millisecond)
}

// StopWithTimeout gracefully shuts down Chrome with a configurable grace period.
func (c *ChromeInstance) StopWithTimeout(gracePeriod time.Duration) error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.stopped = true
	proc := c.cmd
	cdpPort := 0
	if c.profile != nil {
		cdpPort = c.profile.CDPPort
	}
	c.mu.Unlock()

	if proc == nil || proc.Process == nil {
		return nil
	}
	pid := proc.Process.Pid
	c.logger.Info("stopping chrome gracefully", "pid", pid)

	// 1. Send SIGTERM for graceful shutdown.
	if err := proc.Process.Signal(syscall.SIGTERM); err != nil {
		// Process may already be gone.
		c.logger.Debug("sigterm failed, process may be gone", "err", err)
		return nil
	}

	// 2. Poll for exit during grace period.
	deadline := time.Now().Add(gracePeriod)
	for time.Now().Before(deadline) {
		// Check if process has exited.
		if !isProcessRunning(pid) {
			c.logger.Info("chrome stopped gracefully", "pid", pid)
			return nil
		}
		// Also check if CDP is unreachable (Chrome shutting down).
		if cdpPort > 0 {
			cdpURL := CdpURLForPort(cdpPort)
			if !IsChromeReachable(cdpURL, 200) {
				c.logger.Info("chrome CDP unreachable, shutdown in progress", "pid", pid)
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 3. Force kill if still running.
	c.logger.Warn("chrome did not exit gracefully, sending SIGKILL", "pid", pid)
	_ = proc.Process.Kill()
	return nil
}

// Wait waits for the Chrome process to exit.
func (c *ChromeInstance) Wait() error {
	if c.cmd == nil {
		return nil
	}
	return c.cmd.Wait()
}

// --- Chrome health check functions (aligns with TS chrome.ts) ---

// IsChromeReachable returns true if Chrome responds to /json/version.
// Aligns with TS chrome.ts isChromeReachable().
func IsChromeReachable(cdpURL string, timeoutMs int) bool {
	if timeoutMs <= 0 {
		timeoutMs = 500
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	return FetchOK(ctx, cdpURL+"/json/version", timeoutMs) == nil
}

// ChromeVersion holds version information returned by /json/version.
type ChromeVersion struct {
	Browser              string `json:"Browser"`
	UserAgent            string `json:"User-Agent"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	V8Version            string `json:"V8-Version"`
	ProtocolVersion      string `json:"Protocol-Version"`
	WebKitVersion        string `json:"WebKit-Version"`
}

// FetchChromeVersion fetches Chrome version info from /json/version.
// Aligns with TS chrome.ts fetchChromeVersion().
func FetchChromeVersion(cdpURL string, timeoutMs int) (*ChromeVersion, error) {
	if timeoutMs <= 0 {
		timeoutMs = 500
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var version ChromeVersion
	if err := FetchJSON(ctx, cdpURL+"/json/version", &version, timeoutMs); err != nil {
		return nil, err
	}
	return &version, nil
}

// GetChromeWebSocketURL gets the normalized WebSocket URL from Chrome.
// Aligns with TS chrome.ts getChromeWebSocketUrl().
func GetChromeWebSocketURL(cdpURL string, timeoutMs int) (string, error) {
	version, err := FetchChromeVersion(cdpURL, timeoutMs)
	if err != nil {
		return "", err
	}
	wsURL := strings.TrimSpace(version.WebSocketDebuggerURL)
	if wsURL == "" {
		return "", fmt.Errorf("chrome /json/version missing webSocketDebuggerUrl")
	}
	return NormalizeCdpWsURL(wsURL, cdpURL), nil
}

// IsChromeCdpReady performs a comprehensive check: HTTP /json/version + WebSocket handshake.
// Aligns with TS chrome.ts isChromeCdpReady().
func IsChromeCdpReady(cdpURL string, httpTimeoutMs, wsHandshakeMs int) bool {
	if httpTimeoutMs <= 0 {
		httpTimeoutMs = 500
	}
	if wsHandshakeMs <= 0 {
		wsHandshakeMs = 800
	}
	wsURL, err := GetChromeWebSocketURL(cdpURL, httpTimeoutMs)
	if err != nil || wsURL == "" {
		return false
	}
	return CanOpenWebSocket(wsURL, wsHandshakeMs)
}

// CanOpenWebSocket tests if a WebSocket connection can be established.
// Aligns with TS chrome.ts canOpenWebSocket().
func CanOpenWebSocket(wsURL string, timeoutMs int) bool {
	if timeoutMs <= 0 {
		timeoutMs = 800
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs+50)*time.Millisecond)
	defer cancel()
	// Use WithCdpSocket just to test connectivity — no commands needed.
	err := WithCdpSocket(ctx, wsURL, func(send CdpSendFn) error {
		return nil
	})
	return err == nil
}

// --- Bootstrap two-stage launch (aligns with TS chrome.ts launchOpenAcosmiChrome) ---

// LaunchOpenAcosmiChrome performs the full Chrome launch flow:
//  1. Bootstrap: if profile doesn't exist, run Chrome briefly to create default files.
//  2. Decoration: set profile name/color if not already decorated.
//  3. Clean exit: ensure exit_type=Normal to suppress restore prompt.
//  4. Main launch: start Chrome and poll for CDP readiness.
//
// Aligns with TS chrome.ts launchOpenAcosmiChrome().
func LaunchOpenAcosmiChrome(ctx context.Context, cfg ChromeStartConfig) (*ChromeInstance, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = cfg.Profile.DataDir
	}
	if dataDir == "" {
		dataDir = defaultChromeDataDir(cfg.Profile.Name)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create data dir: %w", err)
	}
	// Override DataDir so StartChrome uses our resolved path.
	cfg.DataDir = dataDir

	// Phase 1: Bootstrap — create default profile files if missing.
	localStatePath := filepath.Join(dataDir, "Local State")
	prefsPath := filepath.Join(dataDir, "Default", "Preferences")
	if !fileExists(localStatePath) || !fileExists(prefsPath) {
		cfg.Logger.Info("bootstrapping chrome profile", "dataDir", dataDir)
		if err := bootstrapChromeProfile(ctx, cfg); err != nil {
			cfg.Logger.Warn("bootstrap failed, continuing anyway", "err", err)
		}
	}

	// Phase 2: Decoration — set profile name/color.
	if !IsProfileDecorated(dataDir) {
		color := "#FF4500"
		if cfg.Profile != nil && cfg.Profile.Color != "" {
			color = cfg.Profile.Color
		}
		if err := DecorateOpenAcosmiProfile(dataDir, "Crab Claw", color); err != nil {
			cfg.Logger.Warn("profile decoration failed", "err", err)
		} else {
			cfg.Logger.Info("profile decorated", "color", color)
		}
	}

	// Phase 3: Clean exit flag.
	if err := EnsureProfileCleanExit(dataDir); err != nil {
		cfg.Logger.Warn("clean exit flag failed", "err", err)
	}

	// Phase 4: Ensure port is available.
	if cfg.Profile.CDPPort > 0 {
		if err := EnsurePortAvailable(cfg.Profile.CDPPort); err != nil {
			return nil, fmt.Errorf("CDP port %d is not available: %w", cfg.Profile.CDPPort, err)
		}
	}

	// Phase 5: Main launch.
	instance, err := StartChrome(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Phase 6: Poll for CDP readiness (15s timeout).
	cdpURL := CdpURLForPort(cfg.Profile.CDPPort)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if IsChromeCdpReady(cdpURL, 500, 800) {
			cfg.Logger.Info("chrome CDP ready", "cdpURL", cdpURL)
			return instance, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	// CDP not ready — kill and return error.
	cfg.Logger.Error("chrome CDP not ready after 15s, killing")
	_ = instance.Stop()
	return nil, fmt.Errorf("chrome CDP not ready after 15 seconds on %s", cdpURL)
}

// bootstrapChromeProfile runs Chrome briefly to create default profile files,
// then terminates it.
func bootstrapChromeProfile(ctx context.Context, cfg ChromeStartConfig) error {
	// Start Chrome without URL to trigger profile creation.
	bootstrapArgs := []string{
		"--user-data-dir=" + cfg.DataDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	cmd := exec.CommandContext(ctx, cfg.Executable.Path, bootstrapArgs...)
	cmd.Dir = cfg.DataDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("bootstrap start: %w", err)
	}
	pid := cmd.Process.Pid
	cfg.Logger.Debug("bootstrap chrome started", "pid", pid)

	// Poll for profile files to appear (up to 10s).
	localStatePath := filepath.Join(cfg.DataDir, "Local State")
	prefsPath := filepath.Join(cfg.DataDir, "Default", "Preferences")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if fileExists(localStatePath) && fileExists(prefsPath) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Terminate the bootstrap process.
	_ = cmd.Process.Signal(syscall.SIGTERM)

	// Wait for exit (up to 5s).
	exitDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(exitDeadline) {
		if !isProcessRunning(pid) {
			cfg.Logger.Debug("bootstrap chrome exited", "pid", pid)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Force kill if still running.
	_ = cmd.Process.Kill()
	cfg.Logger.Debug("bootstrap chrome force killed", "pid", pid)
	return nil
}

// fileExists is defined in chrome_executables.go.

// isProcessRunning checks if a process with the given PID is still running.
func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, signal 0 checks if process exists without actually sending a signal.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
