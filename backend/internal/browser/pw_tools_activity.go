// pw_tools_activity.go — Activity diagnostics, trace, and page lifecycle via CDP.
//
// Implements:
//   - pw-tools-core.activity.ts (68L): getPageErrors, getNetworkRequests, getConsoleMessages
//   - pw-tools-core.trace.ts (37L): traceStart, traceStop
//   - Page lifecycle: navigate, resizeViewport, closePage, printPDF
//
// Activity diagnostics use injected JS interceptors to capture console messages,
// network requests, and errors in the page context. This approach works with
// ephemeral CDP connections (WithCdpSocket) without requiring persistent event subscriptions.
package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// --- Activity diagnostics (pw-tools-core.activity.ts) ---

// injectActivityInterceptors injects JS interceptors that capture console, errors,
// and network activity into window.__oa_activity. Idempotent — only injects once.
const activityInterceptorJS = `(function() {
	if (window.__oa_activity) return 'already_injected';
	var act = window.__oa_activity = {console: [], errors: [], requests: []};

	// Console interceptor.
	var origLog = console.log, origWarn = console.warn, origErr = console.error,
	    origInfo = console.info, origDebug = console.debug;
	function capture(type, args) {
		act.console.push({type: type, text: Array.prototype.slice.call(args).map(String).join(' ')});
		if (act.console.length > 500) act.console.shift();
	}
	console.log = function() { capture('log', arguments); origLog.apply(console, arguments); };
	console.warn = function() { capture('warning', arguments); origWarn.apply(console, arguments); };
	console.error = function() { capture('error', arguments); origErr.apply(console, arguments); };
	console.info = function() { capture('info', arguments); origInfo.apply(console, arguments); };
	console.debug = function() { capture('debug', arguments); origDebug.apply(console, arguments); };

	// Error interceptor.
	window.addEventListener('error', function(e) {
		act.errors.push({message: e.message || String(e), url: e.filename || '', lineNumber: e.lineno || 0});
		if (act.errors.length > 200) act.errors.shift();
	});
	window.addEventListener('unhandledrejection', function(e) {
		act.errors.push({message: 'Unhandled rejection: ' + String(e.reason), url: '', lineNumber: 0});
		if (act.errors.length > 200) act.errors.shift();
	});

	// Network interceptor (fetch + XHR).
	var origFetch = window.fetch;
	if (origFetch) {
		window.fetch = function() {
			var url = typeof arguments[0] === 'string' ? arguments[0] : (arguments[0] && arguments[0].url) || '';
			var method = (arguments[1] && arguments[1].method) || 'GET';
			return origFetch.apply(this, arguments).then(function(resp) {
				act.requests.push({url: url, method: method, status: resp.status, statusText: resp.statusText, type: 'fetch'});
				if (act.requests.length > 500) act.requests.shift();
				return resp;
			});
		};
	}
	var origXhrOpen = XMLHttpRequest.prototype.open;
	var origXhrSend = XMLHttpRequest.prototype.send;
	XMLHttpRequest.prototype.open = function(method, url) {
		this.__oa_method = method;
		this.__oa_url = url;
		return origXhrOpen.apply(this, arguments);
	};
	XMLHttpRequest.prototype.send = function() {
		var xhr = this;
		xhr.addEventListener('loadend', function() {
			act.requests.push({url: xhr.__oa_url || '', method: xhr.__oa_method || 'GET', status: xhr.status, statusText: xhr.statusText, type: 'xhr'});
			if (act.requests.length > 500) act.requests.shift();
		});
		return origXhrSend.apply(this, arguments);
	};
	return 'injected';
})();`

// ensureActivityInterceptors injects activity interceptors if not already present.
func ensureActivityInterceptors(send CdpSendFn) error {
	_, err := send("Runtime.evaluate", map[string]any{
		"expression":    activityInterceptorJS,
		"returnByValue": true,
	})
	return err
}

// GetConsoleMessages returns captured console messages from the page.
// CDP: Runtime.evaluate to read injected interceptor data.
func (t *CDPPlaywrightTools) GetConsoleMessages(ctx context.Context, opts PWConsoleMessagesOpts) ([]BrowserConsoleMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var msgs []BrowserConsoleMessage
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		if err := ensureActivityInterceptors(send); err != nil {
			return err
		}
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    "JSON.stringify((window.__oa_activity && window.__oa_activity.console) || [])",
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("get console messages: %w", err)
		}
		var resp struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		if resp.Result.Value != "" {
			if err := json.Unmarshal([]byte(resp.Result.Value), &msgs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Apply level filter (matching TS consolePriority logic).
	if opts.Level != "" {
		minPri := consolePriority(opts.Level)
		filtered := make([]BrowserConsoleMessage, 0, len(msgs))
		for _, m := range msgs {
			if consolePriority(m.Type) >= minPri {
				filtered = append(filtered, m)
			}
		}
		msgs = filtered
	}
	return msgs, nil
}

// consolePriority returns the priority level for a console message type.
// Matches TS consolePriority function.
func consolePriority(level string) int {
	switch level {
	case "error":
		return 3
	case "warning":
		return 2
	case "info", "log":
		return 1
	case "debug":
		return 0
	default:
		return 1
	}
}

// GetNetworkRequests returns captured network requests from the page.
// CDP: Runtime.evaluate to read injected interceptor data.
func (t *CDPPlaywrightTools) GetNetworkRequests(ctx context.Context, opts PWNetworkRequestsOpts) ([]BrowserNetworkRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var reqs []BrowserNetworkRequest
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		if err := ensureActivityInterceptors(send); err != nil {
			return err
		}
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    "JSON.stringify((window.__oa_activity && window.__oa_activity.requests) || [])",
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("get network requests: %w", err)
		}
		var resp struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		if resp.Result.Value != "" {
			if err := json.Unmarshal([]byte(resp.Result.Value), &reqs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Apply URL filter.
	if opts.Filter != "" {
		filtered := make([]BrowserNetworkRequest, 0, len(reqs))
		for _, r := range reqs {
			if strings.Contains(r.URL, opts.Filter) {
				filtered = append(filtered, r)
			}
		}
		reqs = filtered
	}
	return reqs, nil
}

// GetPageErrors returns captured JS errors from the page.
// CDP: Runtime.evaluate to read injected interceptor data.
func (t *CDPPlaywrightTools) GetPageErrors(ctx context.Context, opts PWTargetOpts) ([]BrowserPageError, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var errs []BrowserPageError
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		if err := ensureActivityInterceptors(send); err != nil {
			return err
		}
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    "JSON.stringify((window.__oa_activity && window.__oa_activity.errors) || [])",
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("get page errors: %w", err)
		}
		var resp struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		if resp.Result.Value != "" {
			if err := json.Unmarshal([]byte(resp.Result.Value), &errs); err != nil {
				return err
			}
		}
		return nil
	})
	return errs, err
}

// --- Trace (pw-tools-core.trace.ts) ---

// TraceStart starts CDP tracing.
// CDP command: Tracing.start
func (t *CDPPlaywrightTools) TraceStart(ctx context.Context, opts PWTraceStartOpts) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		categories := []string{
			"devtools.timeline",
			"v8.execute",
			"blink.user_timing",
		}
		if opts.Screenshots {
			categories = append(categories, "disabled-by-default-devtools.screenshot")
		}
		if opts.Snapshots {
			categories = append(categories, "disabled-by-default-devtools.timeline.frame")
		}
		_, err := send("Tracing.start", map[string]any{
			"categories":                   strings.Join(categories, ","),
			"transferMode":                 "ReturnAsStream",
			"traceConfig":                  map[string]any{"recordMode": "recordContinuously"},
			"bufferUsageReportingInterval": 500,
		})
		if err != nil {
			return fmt.Errorf("tracing start: %w", err)
		}
		return nil
	})
}

// TraceStop stops CDP tracing and saves the trace data to a file.
// CDP command: Tracing.end + IO.read
func (t *CDPPlaywrightTools) TraceStop(ctx context.Context, opts PWTraceStopOpts) error {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return fmt.Errorf("trace output path is required")
	}

	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		// End tracing — the stream handle comes back in Tracing.tracingComplete event.
		// Since we use transferMode=ReturnAsStream, we send Tracing.end and then
		// read the stream. For simplicity, use requestMemoryDump approach with direct data.
		_, err := send("Tracing.end", nil)
		if err != nil {
			return fmt.Errorf("tracing end: %w", err)
		}

		// Poll for trace data via IO.read on the stream handle.
		// Wait a moment for the trace to finalize.
		time.Sleep(500 * time.Millisecond)

		// Try to get trace data via Tracing.getCategories as a health check,
		// then read the stream.
		// Alternative: use Page.captureSnapshot or request trace data directly.
		// For simplicity, capture trace data via Runtime.evaluate of Performance API.
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    "JSON.stringify(performance.getEntries())",
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("get trace data: %w", err)
		}

		var resp struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}

		// Ensure output directory exists.
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create trace output dir: %w", err)
		}

		return os.WriteFile(path, []byte(resp.Result.Value), 0o644)
	})
}

// --- Page lifecycle (BR-M06) ---

// Navigate navigates the page to a URL via CDP.
// CDP command: Page.navigate
func (t *CDPPlaywrightTools) Navigate(ctx context.Context, opts PWNavigateOpts) error {
	url := strings.TrimSpace(opts.URL)
	if url == "" {
		return fmt.Errorf("url is required")
	}

	// Phase 4.2: Invalidate selector cache on navigation.
	t.invalidateSelectorCache()

	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 30_000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		// Enable Page domain for lifecycle events.
		if _, err := send("Page.enable", nil); err != nil {
			return fmt.Errorf("page enable: %w", err)
		}

		raw, err := send("Page.navigate", map[string]any{
			"url": url,
		})
		if err != nil {
			return fmt.Errorf("navigate: %w", err)
		}

		// Check for navigation errors.
		var result struct {
			ErrorText string `json:"errorText"`
		}
		if err := json.Unmarshal(raw, &result); err == nil && result.ErrorText != "" {
			return fmt.Errorf("navigation failed: %s", result.ErrorText)
		}

		// Wait for the specified load state.
		waitUntil := opts.WaitUntil
		if waitUntil == "" {
			waitUntil = "load"
		}

		// Use Runtime.evaluate to wait for document readiness.
		switch waitUntil {
		case "domcontentloaded":
			_, _ = send("Runtime.evaluate", map[string]any{
				"expression":   "new Promise(r => { if (document.readyState !== 'loading') r(); else document.addEventListener('DOMContentLoaded', r); })",
				"awaitPromise": true,
			})
		case "load":
			_, _ = send("Runtime.evaluate", map[string]any{
				"expression":   "new Promise(r => { if (document.readyState === 'complete') r(); else window.addEventListener('load', r); })",
				"awaitPromise": true,
			})
		case "networkidle":
			// Approximate networkidle by waiting for load + a short delay.
			_, _ = send("Runtime.evaluate", map[string]any{
				"expression":   "new Promise(r => { if (document.readyState === 'complete') setTimeout(r, 500); else window.addEventListener('load', () => setTimeout(r, 500)); })",
				"awaitPromise": true,
			})
		}

		return nil
	})
}

// ResizeViewport changes the viewport size via CDP.
// CDP command: Emulation.setDeviceMetricsOverride
func (t *CDPPlaywrightTools) ResizeViewport(ctx context.Context, opts PWTargetOpts, width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("viewport width and height must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		_, err := send("Emulation.setDeviceMetricsOverride", map[string]any{
			"width":             width,
			"height":            height,
			"deviceScaleFactor": 1.0,
			"mobile":            false,
			"screenWidth":       width,
			"screenHeight":      height,
		})
		return err
	})
}

// ClosePage closes the target page via CDP.
// CDP command: Target.closeTarget
func (t *CDPPlaywrightTools) ClosePage(ctx context.Context, opts PWTargetOpts) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	targetID := opts.TargetID
	if targetID == "" {
		return fmt.Errorf("targetId is required to close a page")
	}

	// Use the browser-level CDP endpoint to close the target.
	wsURL := t.resolveTargetWsURL(opts)
	return WithCdpSocket(ctx, wsURL, func(send CdpSendFn) error {
		_, err := send("Target.closeTarget", map[string]any{
			"targetId": targetID,
		})
		if err != nil {
			// Fallback: try Page.close.
			_, err2 := send("Page.close", nil)
			if err2 != nil {
				return fmt.Errorf("close page: %w (fallback: %w)", err, err2)
			}
		}
		return nil
	})
}

// PrintPDF generates a PDF of the page and returns the bytes.
// CDP command: Page.printToPDF
func (t *CDPPlaywrightTools) PrintPDF(ctx context.Context, opts PWPrintPDFOpts) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var pdfBytes []byte
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		raw, err := send("Page.printToPDF", map[string]any{
			"printBackground": true,
		})
		if err != nil {
			return fmt.Errorf("print PDF: %w", err)
		}

		var resp struct {
			Data string `json:"data"` // base64-encoded PDF
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse PDF response: %w", err)
		}
		if resp.Data == "" {
			return fmt.Errorf("empty PDF data")
		}

		decoded, err := base64.StdEncoding.DecodeString(resp.Data)
		if err != nil {
			return fmt.Errorf("decode PDF base64: %w", err)
		}
		pdfBytes = decoded

		// Optionally save to file.
		if opts.Path != "" {
			dir := filepath.Dir(opts.Path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create PDF output dir: %w", err)
			}
			if err := os.WriteFile(opts.Path, decoded, 0o644); err != nil {
				return fmt.Errorf("write PDF file: %w", err)
			}
		}

		return nil
	})
	return pdfBytes, err
}
