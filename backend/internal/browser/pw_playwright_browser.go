// pw_playwright_browser.go — BrowserController implementation backed by
// PlaywrightTools. This bridges the tools.BrowserController interface
// (used by browser_tool.go) to the PlaywrightTools abstraction.
//
// TS source: browser-tool.ts (724L)
package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// PlaywrightBrowserController implements BrowserController using
// a PlaywrightTools backend (either CDP or Playwright-native).
type PlaywrightBrowserController struct {
	tools   PlaywrightTools
	target  PWTargetOpts
	planner AIPlanner // Phase 4: 可选 AI 规划器，通过 SetAIPlanner 注入

	cdpOnce sync.Once
	cdp     *CDPClient // cached CDP client for direct CDP operations

	// Phase 4.4: GIF recording state.
	gifRecorder *GIFRecorder
}

// NewPlaywrightBrowserController creates a BrowserController backed by
// any PlaywrightTools implementation.
func NewPlaywrightBrowserController(tools PlaywrightTools, cdpURL string) *PlaywrightBrowserController {
	return &PlaywrightBrowserController{
		tools: tools,
		target: PWTargetOpts{
			CDPURL: cdpURL,
		},
	}
}

// cdpClient returns the cached CDPClient, creating it on first use.
func (c *PlaywrightBrowserController) cdpClient() *CDPClient {
	c.cdpOnce.Do(func() {
		c.cdp = NewCDPClient(c.target.CDPURL, nil)
	})
	return c.cdp
}

// Navigate navigates to the given URL.
func (c *PlaywrightBrowserController) Navigate(ctx context.Context, url string) error {
	c.captureGIFFrame(ctx, 30) // pre-nav frame
	cdp := c.cdpClient()
	err := cdp.Navigate(ctx, url)
	if err == nil {
		time.Sleep(200 * time.Millisecond)
		c.captureGIFFrame(ctx, 80) // post-nav frame
	}
	return err
}

// GetContent returns the page's text content via JS evaluation.
func (c *PlaywrightBrowserController) GetContent(ctx context.Context) (string, error) {
	cdp := c.cdpClient()
	raw, err := cdp.Evaluate(ctx, "document.body.innerText || document.body.textContent || ''")
	if err != nil {
		return "", fmt.Errorf("get content: %w", err)
	}
	var resp struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse content: %w", err)
	}
	return resp.Result.Value, nil
}

// Click clicks the element matching the CSS selector.
func (c *PlaywrightBrowserController) Click(ctx context.Context, selector string) error {
	return c.tools.Click(ctx, PWClickOpts{
		PWTargetOpts: c.target,
		Ref:          selector,
	})
}

// Type types text into the element matching the CSS selector.
func (c *PlaywrightBrowserController) Type(ctx context.Context, selector, text string) error {
	return c.tools.Fill(ctx, PWFillOpts{
		PWTargetOpts: c.target,
		Ref:          selector,
		Value:        text,
	})
}

// Screenshot captures a JPEG screenshot and returns (data, mimeType, error).
// Phase 2: 截图格式从 PNG 改为 JPEG q75，体积缩减 3-5x。
func (c *PlaywrightBrowserController) Screenshot(ctx context.Context) ([]byte, string, error) {
	data, err := c.tools.Screenshot(ctx, c.target)
	if err != nil {
		return nil, "", err
	}
	// Screenshot returns base64-encoded JPEG from CDP.
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		// If not base64, return raw bytes.
		return data, "image/jpeg", nil
	}
	return decoded, "image/jpeg", nil
}

// Evaluate executes JavaScript and returns the result.
func (c *PlaywrightBrowserController) Evaluate(ctx context.Context, script string) (any, error) {
	cdp := c.cdpClient()
	raw, err := cdp.Evaluate(ctx, script)
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		return string(raw), nil
	}
	return result, nil
}

// WaitForSelector waits for an element matching the selector to appear.
// D4-F1: 轮询 + 超时机制，替代原先一次性 JS 检查。
// 默认超时 10s，轮询间隔 100ms。ctx 取消时提前退出。
func (c *PlaywrightBrowserController) WaitForSelector(ctx context.Context, selector string) error {
	const (
		pollInterval = 100 * time.Millisecond
		maxTimeout   = 10 * time.Second
	)

	timeoutCtx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	cdp := c.cdpClient()
	script := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		return el !== null;
	})()`, selector)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		raw, err := cdp.Evaluate(timeoutCtx, script)
		if err != nil {
			return fmt.Errorf("wait_for selector %q: evaluate failed: %w", selector, err)
		}
		// CDP Evaluate 返回 {"result":{"type":"boolean","value":true}}，
		// 需嵌套 struct 解析（参照 GetURL() L167-175 的正确写法）。
		// D4-F1-FIX: 修复直接 Unmarshal 为 bool 永远失败的问题。
		var resp struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &resp) == nil && resp.Result.Value {
			return nil
		}

		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("wait_for selector %q: timed out after %s", selector, maxTimeout)
		case <-ticker.C:
			// 继续轮询
		}
	}
}

// GoBack navigates back in browser history.
func (c *PlaywrightBrowserController) GoBack(ctx context.Context) error {
	cdp := c.cdpClient()
	_, err := cdp.Evaluate(ctx, "window.history.back()")
	return err
}

// GoForward navigates forward in browser history.
func (c *PlaywrightBrowserController) GoForward(ctx context.Context) error {
	cdp := c.cdpClient()
	_, err := cdp.Evaluate(ctx, "window.history.forward()")
	return err
}

// GetURL returns the current page URL.
func (c *PlaywrightBrowserController) GetURL(ctx context.Context) (string, error) {
	cdp := c.cdpClient()
	raw, err := cdp.Evaluate(ctx, "window.location.href")
	if err != nil {
		return "", err
	}
	var resp struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	return resp.Result.Value, nil
}

// ---------- Phase 1: ARIA 快照 + ref 元素交互 ----------

// SnapshotAI returns an ARIA accessibility tree snapshot with ref-annotated
// interactive elements. Delegates to PlaywrightTools.SnapshotAI().
func (c *PlaywrightBrowserController) SnapshotAI(ctx context.Context) (map[string]any, error) {
	return c.tools.SnapshotAI(ctx, PWSnapshotOpts{PWTargetOpts: c.target})
}

// ClickRef clicks an element by its ARIA ref identifier (e.g. "e1").
// More robust than CSS selectors — works as long as the ARIA role exists.
func (c *PlaywrightBrowserController) ClickRef(ctx context.Context, ref string) error {
	c.captureGIFFrame(ctx, 30) // pre-click frame
	err := c.tools.Click(ctx, PWClickOpts{
		PWTargetOpts: c.target,
		Ref:          ref,
	})
	if err == nil {
		time.Sleep(200 * time.Millisecond)
		c.captureGIFFrame(ctx, 50) // post-click frame
	}
	return err
}

// FillRef fills text into an element by its ARIA ref identifier.
func (c *PlaywrightBrowserController) FillRef(ctx context.Context, ref, text string) error {
	c.captureGIFFrame(ctx, 30) // pre-fill frame
	err := c.tools.Fill(ctx, PWFillOpts{
		PWTargetOpts: c.target,
		Ref:          ref,
		Value:        text,
	})
	if err == nil {
		c.captureGIFFrame(ctx, 50) // post-fill frame
	}
	return err
}

// ---------- Phase 4: Mariner AI 循环 ----------

// SetAIPlanner 注入 AI 规划器，激活 ai_browse 意图级任务能力。
// 由 gateway 在创建 BrowserController 时调用。
func (c *PlaywrightBrowserController) SetAIPlanner(planner AIPlanner) {
	c.planner = planner
}

// AIBrowse 执行 Mariner 风格 observe→plan→act 循环。
// 接受意图级目标（如 "在京东搜索 MacBook Pro"），自动执行多步操作。
// 返回 JSON 格式的执行结果。
func (c *PlaywrightBrowserController) AIBrowse(ctx context.Context, goal string) (string, error) {
	if c.planner == nil {
		return "", fmt.Errorf("ai_browse is not available: no AI planner configured. " +
			"Use observe + click_ref/fill_ref for manual step-by-step browsing instead")
	}

	loop := NewAIBrowseLoop(c.tools, c.planner, AIBrowseLoopConfig{
		MaxSteps:          20,
		ScreenshotEnabled: true,
	})

	result, err := loop.Run(ctx, goal, c.target)
	if err != nil {
		return "", fmt.Errorf("ai_browse: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("ai_browse marshal result: %w", err)
	}
	return string(resultJSON), nil
}

// ---------- Phase 4.3: SOM Visual Annotation ----------

// AnnotateSOM injects numbered bounding boxes on interactive elements,
// captures a screenshot, and removes overlays.
func (c *PlaywrightBrowserController) AnnotateSOM(ctx context.Context) ([]byte, string, []SOMAnnotation, error) {
	data, annotations, err := c.tools.AnnotateSOM(ctx, c.target)
	if err != nil {
		return nil, "", nil, err
	}
	// Screenshot is base64-encoded JPEG from CDP.
	decoded, decErr := base64.StdEncoding.DecodeString(string(data))
	if decErr != nil {
		// If not base64, return raw bytes.
		return data, "image/jpeg", annotations, nil
	}
	return decoded, "image/jpeg", annotations, nil
}

// ---------- Phase 4.4: GIF Recording ----------

// StartGIFRecording begins capturing frames on each browser action.
func (c *PlaywrightBrowserController) StartGIFRecording() {
	c.gifRecorder = NewGIFRecorder(GIFRecorderConfig{MaxWidth: 800}, nil)
}

// StopGIFRecording stops recording and returns the animated GIF + frame count.
func (c *PlaywrightBrowserController) StopGIFRecording() ([]byte, int, error) {
	if c.gifRecorder == nil {
		return nil, 0, fmt.Errorf("no GIF recording in progress")
	}
	frameCount := c.gifRecorder.FrameCount()
	data, err := c.gifRecorder.Encode()
	c.gifRecorder = nil
	return data, frameCount, err
}

// IsGIFRecording returns true if GIF recording is active.
func (c *PlaywrightBrowserController) IsGIFRecording() bool {
	return c.gifRecorder != nil
}

// captureGIFFrame captures a screenshot frame if GIF recording is active.
func (c *PlaywrightBrowserController) captureGIFFrame(ctx context.Context, delayCs int) {
	if c.gifRecorder == nil {
		return
	}
	if err := c.gifRecorder.CaptureScreenshotFrame(ctx, c.tools, c.target, delayCs); err != nil {
		slog.Debug("gif: frame capture failed", "err", err)
	}
}

// ---------- Tab Management ----------

// ListTabs returns all browser tabs via CDP Target.getTargets or /json list.
func (c *PlaywrightBrowserController) ListTabs(ctx context.Context) ([]TabInfo, error) {
	cdp := c.cdpClient()
	targets, err := cdp.ListTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tabs: %w", err)
	}
	var tabs []TabInfo
	for _, t := range targets {
		if t.Type == "page" {
			tabs = append(tabs, TabInfo{
				ID:    t.ID,
				URL:   t.URL,
				Title: t.Title,
				Type:  t.Type,
			})
		}
	}
	return tabs, nil
}

// CreateTab creates a new browser tab with the given URL.
func (c *PlaywrightBrowserController) CreateTab(ctx context.Context, url string) (*TabInfo, error) {
	if url == "" {
		url = "about:blank"
	}
	cdp := c.cdpClient()
	targetID, err := cdp.CreateTarget(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("create tab: %w", err)
	}
	return &TabInfo{
		ID:  targetID,
		URL: url,
	}, nil
}

// CloseTab closes a tab by target ID via CDP Target.closeTarget.
func (c *PlaywrightBrowserController) CloseTab(ctx context.Context, targetID string) error {
	cdp := c.cdpClient()
	return WithCdpSocket(ctx, cdp.wsURL, func(send CdpSendFn) error {
		_, err := send("Target.closeTarget", map[string]any{"targetId": targetID})
		return err
	})
}

// SwitchTab activates a tab by target ID via CDP Target.activateTarget.
func (c *PlaywrightBrowserController) SwitchTab(ctx context.Context, targetID string) error {
	cdp := c.cdpClient()
	return WithCdpSocket(ctx, cdp.wsURL, func(send CdpSendFn) error {
		_, err := send("Target.activateTarget", map[string]any{"targetId": targetID})
		return err
	})
}
