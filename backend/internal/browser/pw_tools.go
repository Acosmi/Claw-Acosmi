// Package browser — Playwright tools skeleton.
// TS 参考: src/browser/pw-tools-core.ts (re-exports from 8 sub-modules)
//   - pw-tools-core.interactions.ts  (click, fill, hover, highlight, select, …)
//   - pw-tools-core.snapshot.ts      (snapshotAria, snapshotAi)
//   - pw-tools-core.storage.ts       (cookiesGet, cookiesSet, cookiesClear, localStorageGet, …)
//   - pw-tools-core.downloads.ts     (waitNextDownload, armUpload, armDialog)
//   - pw-tools-core.responses.ts     (responseBody)
//   - pw-tools-core.activity.ts      (waitForActivity)
//   - pw-tools-core.state.ts         (ensurePageState)
//   - pw-tools-core.trace.ts         (startTrace, stopTrace)
package browser

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrNotImplemented is returned by stub methods that have not yet been implemented.
var ErrNotImplemented = errors.New("playwright tool: not implemented")

// ---------- Parameter types ----------

// PWTargetOpts identifies the Playwright page to operate on.
type PWTargetOpts struct {
	CDPURL   string
	TargetID string // optional
}

// PWClickOpts parameters for Click / DoubleClick.
type PWClickOpts struct {
	PWTargetOpts
	Ref         string
	DoubleClick bool
	Button      string // "left" | "right" | "middle"
	Modifiers   []string
	TimeoutMs   int
}

// PWFillOpts parameters for Fill.
type PWFillOpts struct {
	PWTargetOpts
	Ref       string
	Value     string
	TimeoutMs int
}

// PWSnapshotOpts parameters for Aria/AI snapshots.
type PWSnapshotOpts struct {
	PWTargetOpts
	Limit int
}

// PWCookieSetOpts parameters for CookiesSet.
type PWCookieSetOpts struct {
	PWTargetOpts
	Name     string
	Value    string
	URL      string
	Domain   string
	Path     string
	Expires  float64
	HTTPOnly bool
	Secure   bool
	SameSite string // "Lax" | "None" | "Strict"
}

// PWDownloadOpts parameters for WaitNextDownload.
type PWDownloadOpts struct {
	PWTargetOpts
	Ref       string // optional element ref to click
	TimeoutMs int
	OutputDir string
}

// PWResponseBodyOpts parameters for ResponseBody.
type PWResponseBodyOpts struct {
	PWTargetOpts
	URLPattern string
	TimeoutMs  int
	MaxChars   int
}

// PWDragOpts parameters for Drag.
type PWDragOpts struct {
	PWTargetOpts
	StartRef  string
	EndRef    string
	TimeoutMs int
}

// PWSelectOptionOpts parameters for SelectOption.
type PWSelectOptionOpts struct {
	PWTargetOpts
	Ref       string
	Values    []string
	TimeoutMs int
}

// PWPressKeyOpts parameters for PressKey.
type PWPressKeyOpts struct {
	PWTargetOpts
	Key     string
	DelayMs int
}

// PWTypeOpts parameters for Type (character-level input).
type PWTypeOpts struct {
	PWTargetOpts
	Ref       string
	Text      string
	Submit    bool
	Slowly    bool
	TimeoutMs int
}

// PWScrollIntoViewOpts parameters for ScrollIntoView.
type PWScrollIntoViewOpts struct {
	PWTargetOpts
	Ref       string
	TimeoutMs int
}

// PWEvaluateOpts parameters for Evaluate.
type PWEvaluateOpts struct {
	PWTargetOpts
	Expression string
	Ref        string // optional: evaluate in element context
}

// PWWaitForOpts parameters for WaitFor.
type PWWaitForOpts struct {
	PWTargetOpts
	TimeMs    int
	Text      string
	TextGone  string
	URL       string
	LoadState string // "load" | "domcontentloaded" | "networkidle"
	Fn        string // JS function to evaluate
	TimeoutMs int
}

// PWSetInputFilesOpts parameters for SetInputFiles.
type PWSetInputFilesOpts struct {
	PWTargetOpts
	Ref     string
	Element string
	Paths   []string
}

// PWResponseBodyResult result from ResponseBody.
type PWResponseBodyResult struct {
	URL       string
	Status    int
	Headers   map[string]string
	Body      string
	Truncated bool
}

// PWStorageGetOpts parameters for StorageGet.
type PWStorageGetOpts struct {
	PWTargetOpts
	Kind string `json:"kind"` // "local" | "session"
	Key  string `json:"key,omitempty"`
}

// PWStorageSetOpts parameters for StorageSet.
type PWStorageSetOpts struct {
	PWTargetOpts
	Kind  string `json:"kind"` // "local" | "session"
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PWStorageClearOpts parameters for StorageClear.
type PWStorageClearOpts struct {
	PWTargetOpts
	Kind string `json:"kind"` // "local" | "session"
}

// PWNavigateOpts parameters for Navigate.
type PWNavigateOpts struct {
	PWTargetOpts
	URL       string `json:"url"`
	WaitUntil string `json:"waitUntil,omitempty"` // "load" | "domcontentloaded" | "networkidle"
	TimeoutMs int    `json:"timeoutMs,omitempty"`
}

// PWPrintPDFOpts parameters for PrintPDF.
type PWPrintPDFOpts struct {
	PWTargetOpts
	Path string `json:"path,omitempty"`
}

// PWTraceStartOpts parameters for TraceStart.
type PWTraceStartOpts struct {
	PWTargetOpts
	Screenshots bool `json:"screenshots,omitempty"`
	Snapshots   bool `json:"snapshots,omitempty"`
}

// PWTraceStopOpts parameters for TraceStop.
type PWTraceStopOpts struct {
	PWTargetOpts
	Path string `json:"path"`
}

// PWConsoleMessagesOpts parameters for GetConsoleMessages.
type PWConsoleMessagesOpts struct {
	PWTargetOpts
	Level string `json:"level,omitempty"` // "error" | "warning" | "info" | "log" | "debug"
}

// PWNetworkRequestsOpts parameters for GetNetworkRequests.
type PWNetworkRequestsOpts struct {
	PWTargetOpts
	Filter string `json:"filter,omitempty"`
}

// BrowserConsoleMessage represents a console message captured from the page.
type BrowserConsoleMessage struct {
	Type    string `json:"type"` // "log" | "warning" | "error" | "info" | "debug"
	Text    string `json:"text"`
	URL     string `json:"url,omitempty"`
	LineNum int    `json:"lineNumber,omitempty"`
}

// BrowserNetworkRequest represents a network request captured from the page.
type BrowserNetworkRequest struct {
	URL        string `json:"url"`
	Method     string `json:"method"`
	Status     int    `json:"status"`
	StatusText string `json:"statusText,omitempty"`
	Type       string `json:"type,omitempty"`
}

// BrowserPageError represents a JS error captured from the page.
type BrowserPageError struct {
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
	LineNum int    `json:"lineNumber,omitempty"`
}

// ---------- Interface ----------

// PlaywrightTools defines browser automation operations backed by Playwright.
// Each method maps to one or more exported functions in the TS pw-tools-core.* modules.
//
// Implementation status legend (per method):
//
//	TODO:UNIMPLEMENTED — Go implementation not yet written; stub returns ErrNotImplemented.
//	TODO:PARTIAL       — Partial implementation; some edge cases missing.
type PlaywrightTools interface {
	// --- Interactions (pw-tools-core.interactions.ts) ---

	// Click clicks the element identified by ref.
	// TODO:UNIMPLEMENTED
	Click(ctx context.Context, opts PWClickOpts) error

	// Fill types text into the element identified by ref.
	// TODO:UNIMPLEMENTED
	Fill(ctx context.Context, opts PWFillOpts) error

	// Hover moves the mouse over the element identified by ref.
	// TODO:UNIMPLEMENTED
	Hover(ctx context.Context, opts PWTargetOpts, ref string, timeoutMs int) error

	// Highlight visually highlights the element identified by ref (debugging aid).
	// TODO:UNIMPLEMENTED
	Highlight(ctx context.Context, opts PWTargetOpts, ref string) error

	// Drag drags from startRef to endRef.
	Drag(ctx context.Context, opts PWDragOpts) error

	// SelectOption selects option(s) in a <select> element.
	SelectOption(ctx context.Context, opts PWSelectOptionOpts) error

	// PressKey presses a keyboard key.
	PressKey(ctx context.Context, opts PWPressKeyOpts) error

	// Type types text character-by-character, optionally slowly.
	Type(ctx context.Context, opts PWTypeOpts) error

	// ScrollIntoView scrolls the element into the viewport.
	ScrollIntoView(ctx context.Context, opts PWScrollIntoViewOpts) error

	// Evaluate runs a JS expression in the page (or element) context.
	Evaluate(ctx context.Context, opts PWEvaluateOpts) (json.RawMessage, error)

	// WaitFor waits for a condition (time, text, URL, loadState, function).
	WaitFor(ctx context.Context, opts PWWaitForOpts) error

	// SetInputFiles sets files on a file input element.
	SetInputFiles(ctx context.Context, opts PWSetInputFilesOpts) error

	// --- Snapshot (pw-tools-core.snapshot.ts) ---

	// SnapshotAria returns the Accessibility tree as an aria snapshot.
	// TODO:UNIMPLEMENTED
	SnapshotAria(ctx context.Context, opts PWSnapshotOpts) ([]map[string]any, error)

	// SnapshotAI returns a role-annotated AI-friendly snapshot.
	// TODO:UNIMPLEMENTED
	SnapshotAI(ctx context.Context, opts PWSnapshotOpts) (map[string]any, error)

	// Screenshot captures a screenshot as PNG bytes.
	// TODO:UNIMPLEMENTED
	Screenshot(ctx context.Context, opts PWTargetOpts) ([]byte, error)

	// Phase 4.3: SOM visual annotation — inject numbered overlays and capture screenshot.
	AnnotateSOM(ctx context.Context, opts PWTargetOpts) (screenshot []byte, annotations []SOMAnnotation, err error)

	// --- Storage (pw-tools-core.storage.ts) ---

	// CookiesGet returns all cookies for the page context.
	// TODO:UNIMPLEMENTED
	CookiesGet(ctx context.Context, opts PWTargetOpts) ([]map[string]any, error)

	// CookiesSet adds or overwrites a cookie in the page context.
	// TODO:UNIMPLEMENTED
	CookiesSet(ctx context.Context, opts PWCookieSetOpts) error

	// CookiesClear removes all cookies from the page context.
	// TODO:UNIMPLEMENTED
	CookiesClear(ctx context.Context, opts PWTargetOpts) error

	// LocalStorageGet returns all localStorage entries for the page.
	// TODO:UNIMPLEMENTED
	LocalStorageGet(ctx context.Context, opts PWTargetOpts) (map[string]string, error)

	// StorageGet returns localStorage or sessionStorage entries with optional key filter.
	StorageGet(ctx context.Context, opts PWStorageGetOpts) (map[string]string, error)

	// StorageSet sets a key-value pair in localStorage or sessionStorage.
	StorageSet(ctx context.Context, opts PWStorageSetOpts) error

	// StorageClear clears all entries in localStorage or sessionStorage.
	StorageClear(ctx context.Context, opts PWStorageClearOpts) error

	// --- Page lifecycle ---

	// Navigate navigates the page to a URL.
	Navigate(ctx context.Context, opts PWNavigateOpts) error

	// ResizeViewport changes the viewport size.
	ResizeViewport(ctx context.Context, opts PWTargetOpts, width, height int) error

	// ClosePage closes the target page.
	ClosePage(ctx context.Context, opts PWTargetOpts) error

	// PrintPDF generates a PDF of the page.
	PrintPDF(ctx context.Context, opts PWPrintPDFOpts) ([]byte, error)

	// --- Activity diagnostics (pw-tools-core.activity.ts) ---

	// GetConsoleMessages returns captured console messages from the page.
	GetConsoleMessages(ctx context.Context, opts PWConsoleMessagesOpts) ([]BrowserConsoleMessage, error)

	// GetNetworkRequests returns captured network requests from the page.
	GetNetworkRequests(ctx context.Context, opts PWNetworkRequestsOpts) ([]BrowserNetworkRequest, error)

	// GetPageErrors returns captured JS errors from the page.
	GetPageErrors(ctx context.Context, opts PWTargetOpts) ([]BrowserPageError, error)

	// --- Trace (pw-tools-core.trace.ts) ---

	// TraceStart starts CDP tracing.
	TraceStart(ctx context.Context, opts PWTraceStartOpts) error

	// TraceStop stops CDP tracing and saves data to path.
	TraceStop(ctx context.Context, opts PWTraceStopOpts) error

	// --- Downloads (pw-tools-core.downloads.ts) ---

	// WaitNextDownload arms a download waiter, optionally clicking ref to trigger it.
	// Returns the local path where the file was saved.
	// TODO:UNIMPLEMENTED
	WaitNextDownload(ctx context.Context, opts PWDownloadOpts) (string, error)

	// --- Response interception (pw-tools-core.responses.ts) ---

	// ResponseBody intercepts the response body for a URL pattern.
	// TODO:UNIMPLEMENTED
	ResponseBody(ctx context.Context, opts PWResponseBodyOpts) (*PWResponseBodyResult, error)
}

// ---------- Stub implementation ----------

// StubPlaywrightTools is a no-op implementation of PlaywrightTools.
// All methods return ErrNotImplemented. Use this as a placeholder until
// the real Playwright integration is wired up.
type StubPlaywrightTools struct{}

var _ PlaywrightTools = (*StubPlaywrightTools)(nil)

func (*StubPlaywrightTools) Click(_ context.Context, _ PWClickOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Fill(_ context.Context, _ PWFillOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Hover(_ context.Context, _ PWTargetOpts, _ string, _ int) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Highlight(_ context.Context, _ PWTargetOpts, _ string) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Drag(_ context.Context, _ PWDragOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) SelectOption(_ context.Context, _ PWSelectOptionOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) PressKey(_ context.Context, _ PWPressKeyOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Type(_ context.Context, _ PWTypeOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) ScrollIntoView(_ context.Context, _ PWScrollIntoViewOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Evaluate(_ context.Context, _ PWEvaluateOpts) (json.RawMessage, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) WaitFor(_ context.Context, _ PWWaitForOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) SetInputFiles(_ context.Context, _ PWSetInputFilesOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) SnapshotAria(_ context.Context, _ PWSnapshotOpts) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) SnapshotAI(_ context.Context, _ PWSnapshotOpts) (map[string]any, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) Screenshot(_ context.Context, _ PWTargetOpts) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) AnnotateSOM(_ context.Context, _ PWTargetOpts) ([]byte, []SOMAnnotation, error) {
	return nil, nil, ErrNotImplemented
}

func (*StubPlaywrightTools) CookiesGet(_ context.Context, _ PWTargetOpts) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) CookiesSet(_ context.Context, _ PWCookieSetOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) CookiesClear(_ context.Context, _ PWTargetOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) LocalStorageGet(_ context.Context, _ PWTargetOpts) (map[string]string, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) StorageGet(_ context.Context, _ PWStorageGetOpts) (map[string]string, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) StorageSet(_ context.Context, _ PWStorageSetOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) StorageClear(_ context.Context, _ PWStorageClearOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) Navigate(_ context.Context, _ PWNavigateOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) ResizeViewport(_ context.Context, _ PWTargetOpts, _, _ int) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) ClosePage(_ context.Context, _ PWTargetOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) PrintPDF(_ context.Context, _ PWPrintPDFOpts) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) GetConsoleMessages(_ context.Context, _ PWConsoleMessagesOpts) ([]BrowserConsoleMessage, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) GetNetworkRequests(_ context.Context, _ PWNetworkRequestsOpts) ([]BrowserNetworkRequest, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) GetPageErrors(_ context.Context, _ PWTargetOpts) ([]BrowserPageError, error) {
	return nil, ErrNotImplemented
}

func (*StubPlaywrightTools) TraceStart(_ context.Context, _ PWTraceStartOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) TraceStop(_ context.Context, _ PWTraceStopOpts) error {
	return ErrNotImplemented
}

func (*StubPlaywrightTools) WaitNextDownload(_ context.Context, _ PWDownloadOpts) (string, error) {
	return "", ErrNotImplemented
}

func (*StubPlaywrightTools) ResponseBody(_ context.Context, _ PWResponseBodyOpts) (*PWResponseBodyResult, error) {
	return nil, ErrNotImplemented
}
