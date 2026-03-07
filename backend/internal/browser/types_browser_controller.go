// types_browser_controller.go — Canonical BrowserController interface.
// All consumers (tools, runner, gateway) should reference this single definition.
package browser

import "context"

// BrowserController defines the browser automation interface used by the agent tool layer.
// Implemented by PlaywrightBrowserController (this package).
type BrowserController interface {
	Navigate(ctx context.Context, url string) error
	GetContent(ctx context.Context) (string, error)
	Click(ctx context.Context, selector string) error
	Type(ctx context.Context, selector, text string) error
	Screenshot(ctx context.Context) ([]byte, string, error)
	Evaluate(ctx context.Context, script string) (any, error)
	WaitForSelector(ctx context.Context, selector string) error
	GoBack(ctx context.Context) error
	GoForward(ctx context.Context) error
	GetURL(ctx context.Context) (string, error)

	// ARIA snapshot + ref element interaction.
	// SnapshotAI returns ARIA accessibility tree with ref-annotated interactive elements.
	SnapshotAI(ctx context.Context) (map[string]any, error)
	// ClickRef clicks element by ARIA ref (e.g. "e1"). More robust than CSS selectors.
	ClickRef(ctx context.Context, ref string) error
	// FillRef fills text into element by ARIA ref.
	FillRef(ctx context.Context, ref, text string) error

	// Mariner AI browse loop — intent-level browsing.
	// AIBrowse executes observe→plan→act loop, returns JSON result.
	AIBrowse(ctx context.Context, goal string) (string, error)

	// Phase 4.3: SOM visual annotation.
	// AnnotateSOM injects numbered bounding boxes on interactive elements,
	// captures a screenshot, and removes overlays.
	AnnotateSOM(ctx context.Context) (screenshot []byte, mimeType string, annotations []SOMAnnotation, err error)

	// Phase 4.4: GIF recording.
	// StartGIFRecording begins capturing frames on each browser action.
	StartGIFRecording()
	// StopGIFRecording stops recording and returns the animated GIF bytes + frame count.
	StopGIFRecording() (gifData []byte, frameCount int, err error)
	// IsGIFRecording returns true if GIF recording is active.
	IsGIFRecording() bool

	// Tab management.
	// ListTabs returns all browser tabs/targets.
	ListTabs(ctx context.Context) ([]TabInfo, error)
	// CreateTab creates a new tab with the given URL.
	CreateTab(ctx context.Context, url string) (*TabInfo, error)
	// CloseTab closes a tab by its target ID.
	CloseTab(ctx context.Context, targetID string) error
	// SwitchTab activates a tab by its target ID.
	SwitchTab(ctx context.Context, targetID string) error
}

// TabInfo describes a browser tab/target.
type TabInfo struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Type  string `json:"type"`
}
