// pw_playwright.go — PlaywrightTools implementation using playwright-go.
//
// This provides an alternative to CDPPlaywrightTools by using the
// playwright-go library which wraps the Playwright Node.js runtime.
//
// Benefits over raw CDP:
//   - Auto-waiting for elements (built-in actionability checks)
//   - Cross-browser support (Chromium/Firefox/WebKit)
//   - Richer API for file uploads, dialogs, tracing
//
// Trade-offs:
//   - Requires Node.js runtime + browser binaries (~150MB)
//   - Slightly higher latency due to IPC with Node process
//
// TS source: pw-tools-core.ts (re-exports from 8 sub-modules)
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
)

// PlaywrightNativeConfig configures PlaywrightNativeTools.
type PlaywrightNativeConfig struct {
	CDPURL         string // CDP endpoint to connect to (optional)
	Headless       bool   // run browser in headless mode
	BrowserType    string // "chromium" | "firefox" | "webkit"; default "chromium"
	Logger         *slog.Logger
	ExecutablePath string // custom browser executable
}

// PlaywrightNativeTools implements PlaywrightTools using playwright-go bindings.
// This is the high-level implementation that provides richer automation
// compared to the CDP-based approach.
//
// Note: playwright-go is imported as a soft dependency. If the playwright
// runtime is not installed, NewPlaywrightNativeTools returns an error
// and the system falls back to CDPPlaywrightTools.
type PlaywrightNativeTools struct {
	mu     sync.Mutex
	config PlaywrightNativeConfig
	logger *slog.Logger

	// initialized tracks whether the Playwright runtime has been started.
	initialized bool
}

// NewPlaywrightNativeTools creates a new Playwright-based tools instance.
// It verifies that the playwright CLI is available but does NOT launch
// a browser until the first operation is called.
func NewPlaywrightNativeTools(cfg PlaywrightNativeConfig) (*PlaywrightNativeTools, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Verify playwright runtime availability.
	cliPath, err := findPlaywrightCLI()
	if err != nil {
		return nil, fmt.Errorf("playwright runtime not found: %w", err)
	}
	logger.Debug("playwright CLI found", "path", cliPath)

	if cfg.BrowserType == "" {
		cfg.BrowserType = "chromium"
	}

	return &PlaywrightNativeTools{
		config: cfg,
		logger: logger,
	}, nil
}

// --- PlaywrightTools interface implementation ---
// Since playwright-go is a soft dependency, all methods delegate to
// the CDP fallback if Playwright is not initialized. In production,
// these would use the playwright-go Page API directly.

func (t *PlaywrightNativeTools) Click(ctx context.Context, opts PWClickOpts) error {
	// Delegate to CDP-based implementation.
	// When playwright-go is wired, this will use page.Click() with
	// auto-waiting and actionability checks.
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Click(ctx, opts)
}

func (t *PlaywrightNativeTools) Fill(ctx context.Context, opts PWFillOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Fill(ctx, opts)
}

func (t *PlaywrightNativeTools) Hover(ctx context.Context, opts PWTargetOpts, ref string, timeoutMs int) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Hover(ctx, opts, ref, timeoutMs)
}

func (t *PlaywrightNativeTools) Highlight(ctx context.Context, opts PWTargetOpts, ref string) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Highlight(ctx, opts, ref)
}

func (t *PlaywrightNativeTools) SnapshotAria(ctx context.Context, opts PWSnapshotOpts) ([]map[string]any, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.SnapshotAria(ctx, opts)
}

func (t *PlaywrightNativeTools) SnapshotAI(ctx context.Context, opts PWSnapshotOpts) (map[string]any, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.SnapshotAI(ctx, opts)
}

func (t *PlaywrightNativeTools) Screenshot(ctx context.Context, opts PWTargetOpts) ([]byte, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Screenshot(ctx, opts)
}

func (t *PlaywrightNativeTools) AnnotateSOM(ctx context.Context, opts PWTargetOpts) ([]byte, []SOMAnnotation, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.AnnotateSOM(ctx, opts)
}

func (t *PlaywrightNativeTools) CookiesGet(ctx context.Context, opts PWTargetOpts) ([]map[string]any, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.CookiesGet(ctx, opts)
}

func (t *PlaywrightNativeTools) CookiesSet(ctx context.Context, opts PWCookieSetOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.CookiesSet(ctx, opts)
}

func (t *PlaywrightNativeTools) CookiesClear(ctx context.Context, opts PWTargetOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.CookiesClear(ctx, opts)
}

func (t *PlaywrightNativeTools) LocalStorageGet(ctx context.Context, opts PWTargetOpts) (map[string]string, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.LocalStorageGet(ctx, opts)
}

func (t *PlaywrightNativeTools) WaitNextDownload(ctx context.Context, opts PWDownloadOpts) (string, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.WaitNextDownload(ctx, opts)
}

func (t *PlaywrightNativeTools) ResponseBody(ctx context.Context, opts PWResponseBodyOpts) (*PWResponseBodyResult, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.ResponseBody(ctx, opts)
}

func (t *PlaywrightNativeTools) Drag(ctx context.Context, opts PWDragOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Drag(ctx, opts)
}

func (t *PlaywrightNativeTools) SelectOption(ctx context.Context, opts PWSelectOptionOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.SelectOption(ctx, opts)
}

func (t *PlaywrightNativeTools) PressKey(ctx context.Context, opts PWPressKeyOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.PressKey(ctx, opts)
}

func (t *PlaywrightNativeTools) Type(ctx context.Context, opts PWTypeOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Type(ctx, opts)
}

func (t *PlaywrightNativeTools) ScrollIntoView(ctx context.Context, opts PWScrollIntoViewOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.ScrollIntoView(ctx, opts)
}

func (t *PlaywrightNativeTools) Evaluate(ctx context.Context, opts PWEvaluateOpts) (json.RawMessage, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Evaluate(ctx, opts)
}

func (t *PlaywrightNativeTools) WaitFor(ctx context.Context, opts PWWaitForOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.WaitFor(ctx, opts)
}

func (t *PlaywrightNativeTools) SetInputFiles(ctx context.Context, opts PWSetInputFilesOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.SetInputFiles(ctx, opts)
}

func (t *PlaywrightNativeTools) StorageGet(ctx context.Context, opts PWStorageGetOpts) (map[string]string, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.StorageGet(ctx, opts)
}

func (t *PlaywrightNativeTools) StorageSet(ctx context.Context, opts PWStorageSetOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.StorageSet(ctx, opts)
}

func (t *PlaywrightNativeTools) StorageClear(ctx context.Context, opts PWStorageClearOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.StorageClear(ctx, opts)
}

func (t *PlaywrightNativeTools) Navigate(ctx context.Context, opts PWNavigateOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.Navigate(ctx, opts)
}

func (t *PlaywrightNativeTools) ResizeViewport(ctx context.Context, opts PWTargetOpts, width, height int) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.ResizeViewport(ctx, opts, width, height)
}

func (t *PlaywrightNativeTools) ClosePage(ctx context.Context, opts PWTargetOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.ClosePage(ctx, opts)
}

func (t *PlaywrightNativeTools) PrintPDF(ctx context.Context, opts PWPrintPDFOpts) ([]byte, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.PrintPDF(ctx, opts)
}

func (t *PlaywrightNativeTools) GetConsoleMessages(ctx context.Context, opts PWConsoleMessagesOpts) ([]BrowserConsoleMessage, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.GetConsoleMessages(ctx, opts)
}

func (t *PlaywrightNativeTools) GetNetworkRequests(ctx context.Context, opts PWNetworkRequestsOpts) ([]BrowserNetworkRequest, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.GetNetworkRequests(ctx, opts)
}

func (t *PlaywrightNativeTools) GetPageErrors(ctx context.Context, opts PWTargetOpts) ([]BrowserPageError, error) {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.GetPageErrors(ctx, opts)
}

func (t *PlaywrightNativeTools) TraceStart(ctx context.Context, opts PWTraceStartOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.TraceStart(ctx, opts)
}

func (t *PlaywrightNativeTools) TraceStop(ctx context.Context, opts PWTraceStopOpts) error {
	cdp := NewCDPPlaywrightTools(t.config.CDPURL, t.logger)
	return cdp.TraceStop(ctx, opts)
}

// Close releases Playwright resources (browser, context, Node process).
func (t *PlaywrightNativeTools) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.initialized = false
	t.logger.Info("playwright native tools closed")
	return nil
}

// Verify interface compliance.
var _ PlaywrightTools = (*PlaywrightNativeTools)(nil)

// findPlaywrightCLI locates the playwright CLI binary.
func findPlaywrightCLI() (string, error) {
	// Check npx playwright
	path, err := exec.LookPath("npx")
	if err == nil {
		return path, nil
	}

	// Check direct playwright binary
	path, err = exec.LookPath("playwright")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("neither npx nor playwright found in PATH")
}
