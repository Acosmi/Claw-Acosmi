// Package browser — CDP-based PlaywrightTools implementation.
// TS source: pw-tools-core.interactions.ts (547L) + pw-tools-core.snapshot.ts (206L)
//   - pw-tools-core.storage.ts (129L) + pw-tools-core.downloads.ts (252L)
//   - pw-tools-core.responses.ts (124L) + pw-tools-core.state.ts (210L)
//   - pw-tools-core.activity.ts (69L) + pw-tools-core.trace.ts (38L)
//
// Instead of using playwright-go, this implementation issues raw CDP
// (Chrome DevTools Protocol) commands via WebSocket to achieve equivalent
// functionality.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CDPPlaywrightTools implements PlaywrightTools using raw CDP protocol commands.
// This is the production implementation that replaces StubPlaywrightTools.
type CDPPlaywrightTools struct {
	cdpURL string
	logger *slog.Logger

	// BR-M08: roleRef cache to avoid redundant accessibility tree lookups.
	cachedSnapshotRefs RoleRefMap
	cachedSnapshotTime time.Time
	cachedSnapshotTTL  time.Duration // default 5s

	// Phase 4.2: Stagehand-style selector cache.
	// After resolving ref → objectID, we generate a CSS selector and cache it.
	// On subsequent access (same page URL), we try cached selector first (zero JS eval).
	selectorCache    map[string]string // key: "url|ref" → CSS selector
	selectorCacheURL string            // URL when cache was populated; invalidated on navigation

	// Cached page-level WS URL resolved from browser-level WS URL.
	// Page/Runtime domain commands only work on page-level WebSocket connections.
	cachedPageWsURL string
}

// NewCDPPlaywrightTools creates a new CDP-based PlaywrightTools implementation.
func NewCDPPlaywrightTools(cdpURL string, logger *slog.Logger) *CDPPlaywrightTools {
	if logger == nil {
		logger = slog.Default()
	}
	return &CDPPlaywrightTools{
		cdpURL:            cdpURL,
		logger:            logger,
		cachedSnapshotTTL: 5 * time.Second,
	}
}

var _ PlaywrightTools = (*CDPPlaywrightTools)(nil)

// ---------- Interactions ----------

// Click clicks the element identified by ref via CDP.
// CDP flow: resolve ref → get box → dispatch mouse events.
func (t *CDPPlaywrightTools) Click(ctx context.Context, opts PWClickOpts) error {
	ref, err := RequireRef(opts.Ref)
	if err != nil {
		return err
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 8000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		// Phase 3.1: Playwright 式 actionability 预检 — 确保元素可交互。
		objectID, resolveErr := t.resolveRefToObjectID(send, ref)
		var x, y float64
		if resolveErr == nil {
			cx, cy, actionErr := EnsureActionable(send, objectID, timeout)
			if actionErr != nil {
				return ToAIFriendlyError(fmt.Errorf("click %q: %w", ref, actionErr), ref)
			}
			x, y = cx, cy
		} else {
			// 降级: 无法 resolve objectID 时使用原始 getElementCenter
			var err2 error
			x, y, err2 = t.getElementCenter(send, ref)
			if err2 != nil {
				return ToAIFriendlyError(err2, ref)
			}
		}

		button := "left"
		if opts.Button != "" {
			button = opts.Button
		}
		clickCount := 1
		if opts.DoubleClick {
			clickCount = 2
		}

		// Build modifier flags for CDP (BR-M15).
		modifiers := buildModifierFlags(opts.Modifiers)

		// mousePressed
		mouseParams := map[string]any{
			"type":       "mousePressed",
			"x":          x,
			"y":          y,
			"button":     button,
			"clickCount": clickCount,
		}
		if modifiers > 0 {
			mouseParams["modifiers"] = modifiers
		}
		if _, err := send("Input.dispatchMouseEvent", mouseParams); err != nil {
			return ToAIFriendlyError(err, ref)
		}
		// mouseReleased
		mouseParams["type"] = "mouseReleased"
		if _, err := send("Input.dispatchMouseEvent", mouseParams); err != nil {
			return ToAIFriendlyError(err, ref)
		}

		return nil
	})
}

// Fill fills the element identified by ref with text.
// CDP flow: focus element → clear → insert text.
func (t *CDPPlaywrightTools) Fill(ctx context.Context, opts PWFillOpts) error {
	ref, err := RequireRef(opts.Ref)
	if err != nil {
		return err
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 8000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		objectID, err := t.resolveRefToObjectID(send, ref)
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Actionability check: wait for element to be visible and enabled.
		if _, _, err := EnsureActionable(send, objectID, timeout); err != nil {
			return ToAIFriendlyError(fmt.Errorf("fill %q: %w", ref, err), ref)
		}

		// Focus the element
		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId":            objectID,
			"functionDeclaration": "function() { this.focus(); }",
		}); err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Select all + delete to clear
		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId":            objectID,
			"functionDeclaration": "function() { this.value = ''; this.dispatchEvent(new Event('input', {bubbles:true})); }",
		}); err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Insert text
		if _, err := send("Input.insertText", map[string]any{
			"text": opts.Value,
		}); err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Dispatch change event
		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId":            objectID,
			"functionDeclaration": "function() { this.dispatchEvent(new Event('change', {bubbles:true})); }",
		}); err != nil {
			t.logger.Warn("fill: dispatch change event failed", "err", err)
		}

		return nil
	})
}

// Hover moves the mouse over the element identified by ref.
func (t *CDPPlaywrightTools) Hover(ctx context.Context, opts PWTargetOpts, ref string, timeoutMs int) error {
	validRef, err := RequireRef(ref)
	if err != nil {
		return err
	}
	timeout := NormalizeTimeoutMs(timeoutMs, 8000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		x, y, err := t.getElementCenter(send, validRef)
		if err != nil {
			return ToAIFriendlyError(err, validRef)
		}

		if _, err := send("Input.dispatchMouseEvent", map[string]any{
			"type": "mouseMoved",
			"x":    x,
			"y":    y,
		}); err != nil {
			return ToAIFriendlyError(err, validRef)
		}

		return nil
	})
}

// Highlight visually highlights the element identified by ref.
func (t *CDPPlaywrightTools) Highlight(ctx context.Context, opts PWTargetOpts, ref string) error {
	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		objectID, err := t.resolveRefToObjectID(send, ref)
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Get node ID from object
		raw, err := send("DOM.requestNode", map[string]any{
			"objectId": objectID,
		})
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}
		var nodeResp struct {
			NodeID int `json:"nodeId"`
		}
		if err := json.Unmarshal(raw, &nodeResp); err != nil {
			return err
		}

		// Highlight the node
		if _, err := send("Overlay.highlightNode", map[string]any{
			"nodeId": nodeResp.NodeID,
			"highlightConfig": map[string]any{
				"showInfo":          true,
				"contentColor":      map[string]any{"r": 111, "g": 168, "b": 220, "a": 0.66},
				"paddingColor":      map[string]any{"r": 147, "g": 196, "b": 125, "a": 0.55},
				"borderColor":       map[string]any{"r": 255, "g": 229, "b": 153, "a": 0.75},
				"marginColor":       map[string]any{"r": 246, "g": 178, "b": 107, "a": 0.55},
				"showAccessibility": true,
			},
		}); err != nil {
			// Overlay domain might not be available, fall back silently
			t.logger.Debug("highlight: Overlay.highlightNode failed, trying DOM.highlightNode", "err", err)
		}

		return nil
	})
}

// ---------- Snapshots ----------

// SnapshotAria returns the Accessibility tree via CDP Accessibility.getFullAXTree.
func (t *CDPPlaywrightTools) SnapshotAria(ctx context.Context, opts PWSnapshotOpts) ([]map[string]any, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}

	var nodes []map[string]any
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		// Enable Accessibility domain
		if _, err := send("Accessibility.enable", nil); err != nil {
			t.logger.Debug("Accessibility.enable failed", "err", err)
		}

		raw, err := send("Accessibility.getFullAXTree", nil)
		if err != nil {
			return fmt.Errorf("get accessibility tree: %w", err)
		}

		var resp struct {
			Nodes []map[string]any `json:"nodes"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse accessibility tree: %w", err)
		}

		nodes = resp.Nodes
		if len(nodes) > limit {
			nodes = nodes[:limit]
		}
		return nil
	})

	return nodes, err
}

// SnapshotAI returns a role-annotated AI-friendly snapshot built from the
// Accessibility tree, with refs assigned to interactable elements.
func (t *CDPPlaywrightTools) SnapshotAI(ctx context.Context, opts PWSnapshotOpts) (map[string]any, error) {
	nodes, err := t.SnapshotAria(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build aria text from nodes
	var lines []string
	for _, node := range nodes {
		role, _ := extractStringValue(node, "role")
		name, _ := extractStringValue(node, "name")
		if role == "" {
			continue
		}
		line := "- " + role
		if name != "" {
			line += fmt.Sprintf(` "%s"`, name)
		}
		lines = append(lines, line)
	}

	ariaText := strings.Join(lines, "\n")
	result := BuildRoleSnapshotFromAriaSnapshot(ariaText, RoleSnapshotOptions{})

	// BR-M08: Update roleRef cache.
	t.cachedSnapshotRefs = result.Refs
	t.cachedSnapshotTime = time.Now()

	// Phase 4.2: Invalidate selector cache — new snapshot means new ref assignments.
	t.invalidateSelectorCache()

	return map[string]any{
		"snapshot": result.Snapshot,
		"refs":     result.Refs,
	}, nil
}

// GetCachedRefs returns the cached roleRef map if still valid, nil otherwise.
func (t *CDPPlaywrightTools) GetCachedRefs() RoleRefMap {
	if t.cachedSnapshotRefs == nil {
		return nil
	}
	if time.Since(t.cachedSnapshotTime) > t.cachedSnapshotTTL {
		return nil
	}
	return t.cachedSnapshotRefs
}

// ---------- Phase 4.2: Stagehand-style selector cache ----------

// selectorCacheKey builds the cache key for a given ref on the current page URL.
func selectorCacheKey(pageURL, ref string) string {
	return pageURL + "|" + ref
}

// getCachedSelector returns a cached CSS selector for (pageURL, ref), or "".
func (t *CDPPlaywrightTools) getCachedSelector(pageURL, ref string) string {
	if t.selectorCache == nil || t.selectorCacheURL != pageURL {
		return ""
	}
	return t.selectorCache[selectorCacheKey(pageURL, ref)]
}

// putCachedSelector stores a CSS selector for (pageURL, ref).
func (t *CDPPlaywrightTools) putCachedSelector(pageURL, ref, selector string) {
	if t.selectorCache == nil || t.selectorCacheURL != pageURL {
		t.selectorCache = make(map[string]string)
		t.selectorCacheURL = pageURL
	}
	t.selectorCache[selectorCacheKey(pageURL, ref)] = selector
}

// invalidateSelectorCache clears the selector cache (called on navigation).
func (t *CDPPlaywrightTools) invalidateSelectorCache() {
	t.selectorCache = nil
	t.selectorCacheURL = ""
}

// Screenshot captures a screenshot via CDP.
// Phase 2: JPEG q75 + optimizeForSpeed — base64 体积比 PNG 缩减 3-5x。
// 行业对标: Anthropic CU 推荐 XGA JPEG，CDP 原生支持 format+quality 参数。
func (t *CDPPlaywrightTools) Screenshot(ctx context.Context, opts PWTargetOpts) ([]byte, error) {
	var screenshot []byte
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		raw, err := send("Page.captureScreenshot", map[string]any{
			"format":           "jpeg",
			"quality":          75,
			"optimizeForSpeed": true,
		})
		if err != nil {
			return fmt.Errorf("capture screenshot: %w", err)
		}

		var resp struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse screenshot response: %w", err)
		}

		screenshot = []byte(resp.Data)
		return nil
	})
	return screenshot, err
}

// ---------- Phase 4.3: SOM Visual Annotation ----------

// SOMAnnotation describes a single annotated interactive element.
type SOMAnnotation struct {
	Index    int    `json:"index"`
	Tag      string `json:"tag"`
	Role     string `json:"role"`
	Text     string `json:"text"`
	Selector string `json:"selector"`
}

// AnnotateSOM injects visual bounding boxes with numeric IDs on all interactive
// elements, captures a screenshot, removes overlays, and returns the annotated
// screenshot + element list.
//
// This enables Set-of-Mark prompting: the LLM sees a screenshot with numbered
// elements and can reference them by number.
func (t *CDPPlaywrightTools) AnnotateSOM(ctx context.Context, opts PWTargetOpts) ([]byte, []SOMAnnotation, error) {
	var screenshot []byte
	var annotations []SOMAnnotation

	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		// Step 1: Inject annotation overlays and collect element info.
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    somInjectJS,
			"returnByValue": true,
			"awaitPromise":  false,
		})
		if err != nil {
			return fmt.Errorf("som inject: %w", err)
		}

		// Parse element info from injection result.
		var injectResp struct {
			Result struct {
				Value []struct {
					Index    int    `json:"index"`
					Tag      string `json:"tag"`
					Role     string `json:"role"`
					Text     string `json:"text"`
					Selector string `json:"selector"`
				} `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &injectResp); err != nil {
			t.logger.Warn("som: parse inject result failed", "err", err)
		} else {
			for _, el := range injectResp.Result.Value {
				annotations = append(annotations, SOMAnnotation{
					Index:    el.Index,
					Tag:      el.Tag,
					Role:     el.Role,
					Text:     el.Text,
					Selector: el.Selector,
				})
			}
		}

		// Step 2: Capture screenshot with overlays visible.
		ssRaw, err := send("Page.captureScreenshot", map[string]any{
			"format":           "jpeg",
			"quality":          75,
			"optimizeForSpeed": true,
		})
		if err != nil {
			return fmt.Errorf("som screenshot: %w", err)
		}
		var ssResp struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(ssRaw, &ssResp); err != nil {
			return fmt.Errorf("parse som screenshot: %w", err)
		}
		screenshot = []byte(ssResp.Data)

		// Step 3: Remove overlays.
		if _, err := send("Runtime.evaluate", map[string]any{
			"expression": somCleanupJS,
		}); err != nil {
			t.logger.Warn("som: cleanup overlays failed", "err", err)
		}

		return nil
	})

	return screenshot, annotations, err
}

// somInjectJS is the JavaScript that injects SOM annotation overlays.
// Returns an array of element descriptors.
const somInjectJS = `(function() {
	// Remove any previous SOM overlays.
	document.querySelectorAll('[data-som-overlay]').forEach(function(e) { e.remove(); });

	var all = document.querySelectorAll('*');
	var interactive = [];
	for (var i = 0; i < all.length; i++) {
		var el = all[i];
		var tag = el.tagName.toLowerCase();
		var role = el.getAttribute('role') || '';
		if (tag === 'button' || tag === 'a' || tag === 'input' ||
				tag === 'select' || tag === 'textarea' ||
				role === 'button' || role === 'link' || role === 'textbox' ||
				role === 'checkbox' || role === 'radio' || role === 'combobox' ||
				role === 'menuitem' || role === 'tab' || role === 'switch' ||
				role === 'option' || role === 'treeitem' ||
				el.tabIndex >= 0) {
			var rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0) {
				interactive.push({el: el, rect: rect, tag: tag, role: role});
			}
		}
	}

	var colors = ['#FF0000','#00AA00','#0066FF','#FF6600','#9933FF',
	              '#00CCCC','#FF3399','#66CC00','#3366FF','#CC3300'];
	var result = [];
	for (var i = 0; i < interactive.length; i++) {
		var item = interactive[i];
		var rect = item.rect;
		var color = colors[i % colors.length];

		// Bounding box overlay.
		var box = document.createElement('div');
		box.setAttribute('data-som-overlay', 'true');
		box.style.cssText = 'position:fixed;z-index:2147483647;pointer-events:none;' +
			'border:2px solid ' + color + ';' +
			'left:' + rect.left + 'px;top:' + rect.top + 'px;' +
			'width:' + rect.width + 'px;height:' + rect.height + 'px;';

		// Numeric label.
		var label = document.createElement('div');
		label.setAttribute('data-som-overlay', 'true');
		label.style.cssText = 'position:fixed;z-index:2147483647;pointer-events:none;' +
			'background:' + color + ';color:#fff;font:bold 11px monospace;' +
			'padding:1px 4px;border-radius:3px;' +
			'left:' + Math.max(0, rect.left - 2) + 'px;' +
			'top:' + Math.max(0, rect.top - 16) + 'px;';
		label.textContent = '' + (i + 1);

		document.body.appendChild(box);
		document.body.appendChild(label);

		var text = (item.el.textContent || item.el.value || item.el.placeholder || '').trim().slice(0, 50);
		result.push({
			index: i + 1,
			tag: item.tag,
			role: item.role || item.tag,
			text: text,
			selector: item.tag + (item.el.id ? '#' + item.el.id : '')
		});
	}
	return result;
})()`

// somCleanupJS removes all SOM annotation overlays from the page.
const somCleanupJS = `(function() {
	document.querySelectorAll('[data-som-overlay]').forEach(function(e) { e.remove(); });
})()`

// ---------- Storage ----------

// CookiesGet returns all cookies for the current page context via CDP.
func (t *CDPPlaywrightTools) CookiesGet(ctx context.Context, opts PWTargetOpts) ([]map[string]any, error) {
	var cookies []map[string]any
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		raw, err := send("Network.getCookies", nil)
		if err != nil {
			return fmt.Errorf("get cookies: %w", err)
		}

		var resp struct {
			Cookies []map[string]any `json:"cookies"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse cookies: %w", err)
		}

		cookies = resp.Cookies
		return nil
	})
	return cookies, err
}

// CookiesSet adds or overwrites a cookie via CDP.
func (t *CDPPlaywrightTools) CookiesSet(ctx context.Context, opts PWCookieSetOpts) error {
	if opts.Name == "" {
		return fmt.Errorf("cookie name is required")
	}

	hasURL := strings.TrimSpace(opts.URL) != ""
	hasDomainPath := strings.TrimSpace(opts.Domain) != "" && strings.TrimSpace(opts.Path) != ""
	if !hasURL && !hasDomainPath {
		return fmt.Errorf("cookie requires url, or domain+path")
	}

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		params := map[string]any{
			"name":  opts.Name,
			"value": opts.Value,
		}
		if opts.URL != "" {
			params["url"] = opts.URL
		}
		if opts.Domain != "" {
			params["domain"] = opts.Domain
		}
		if opts.Path != "" {
			params["path"] = opts.Path
		}
		if opts.Expires > 0 {
			params["expires"] = opts.Expires
		}
		if opts.HTTPOnly {
			params["httpOnly"] = true
		}
		if opts.Secure {
			params["secure"] = true
		}
		if opts.SameSite != "" {
			params["sameSite"] = opts.SameSite
		}

		if _, err := send("Network.setCookie", params); err != nil {
			return fmt.Errorf("set cookie: %w", err)
		}
		return nil
	})
}

// CookiesClear removes all cookies via CDP.
func (t *CDPPlaywrightTools) CookiesClear(ctx context.Context, opts PWTargetOpts) error {
	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		if _, err := send("Network.clearBrowserCookies", nil); err != nil {
			return fmt.Errorf("clear cookies: %w", err)
		}
		return nil
	})
}

// LocalStorageGet returns localStorage entries via CDP Runtime.evaluate.
func (t *CDPPlaywrightTools) LocalStorageGet(ctx context.Context, opts PWTargetOpts) (map[string]string, error) {
	var result map[string]string
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts), func(send CdpSendFn) error {
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression": `(function() {
				var out = {};
				for (var i = 0; i < localStorage.length; i++) {
					var k = localStorage.key(i);
					if (k) { out[k] = localStorage.getItem(k) || ''; }
				}
				return JSON.stringify(out);
			})()`,
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("get localStorage: %w", err)
		}

		var resp struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse localStorage response: %w", err)
		}

		result = make(map[string]string)
		if resp.Result.Value != "" {
			if err := json.Unmarshal([]byte(resp.Result.Value), &result); err != nil {
				return fmt.Errorf("decode localStorage JSON: %w", err)
			}
		}
		return nil
	})
	return result, err
}

// StorageGet returns localStorage or sessionStorage entries with optional key filter via CDP.
// CDP command: Runtime.evaluate
func (t *CDPPlaywrightTools) StorageGet(ctx context.Context, opts PWStorageGetOpts) (map[string]string, error) {
	kind := opts.Kind
	if kind != "session" {
		kind = "local"
	}
	storeName := kind + "Storage"

	var result map[string]string
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		var js string
		if opts.Key != "" {
			// Single key lookup.
			js = fmt.Sprintf(`(function() {
				var v = %s.getItem(%q);
				return v === null ? '{}' : JSON.stringify({%q: v});
			})()`, storeName, opts.Key, opts.Key)
		} else {
			// All keys.
			js = fmt.Sprintf(`(function() {
				var s = %s, out = {};
				for (var i = 0; i < s.length; i++) {
					var k = s.key(i);
					if (k) { out[k] = s.getItem(k) || ''; }
				}
				return JSON.stringify(out);
			})()`, storeName)
		}
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    js,
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("get %s: %w", storeName, err)
		}
		var resp struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse %s response: %w", storeName, err)
		}
		result = make(map[string]string)
		if resp.Result.Value != "" {
			if err := json.Unmarshal([]byte(resp.Result.Value), &result); err != nil {
				return fmt.Errorf("decode %s JSON: %w", storeName, err)
			}
		}
		return nil
	})
	return result, err
}

// StorageSet sets a key-value pair in localStorage or sessionStorage via CDP.
// CDP command: Runtime.evaluate
func (t *CDPPlaywrightTools) StorageSet(ctx context.Context, opts PWStorageSetOpts) error {
	if opts.Key == "" {
		return fmt.Errorf("key is required")
	}
	kind := opts.Kind
	if kind != "session" {
		kind = "local"
	}
	storeName := kind + "Storage"

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		js := fmt.Sprintf(`%s.setItem(%q, %q)`, storeName, opts.Key, opts.Value)
		_, err := send("Runtime.evaluate", map[string]any{
			"expression":    js,
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("set %s: %w", storeName, err)
		}
		return nil
	})
}

// StorageClear clears all entries in localStorage or sessionStorage via CDP.
// CDP command: Runtime.evaluate
func (t *CDPPlaywrightTools) StorageClear(ctx context.Context, opts PWStorageClearOpts) error {
	kind := opts.Kind
	if kind != "session" {
		kind = "local"
	}
	storeName := kind + "Storage"

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		_, err := send("Runtime.evaluate", map[string]any{
			"expression":    storeName + ".clear()",
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("clear %s: %w", storeName, err)
		}
		return nil
	})
}

// ---------- Downloads ----------

// WaitNextDownload arms a download waiter.
// CDP flow: set download behavior → wait for download event.
func (t *CDPPlaywrightTools) WaitNextDownload(ctx context.Context, opts PWDownloadOpts) (string, error) {
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 120_000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	outputDir := strings.TrimSpace(opts.OutputDir)
	if outputDir == "" {
		outputDir = defaultDownloadDir()
	}
	// Ensure download directory exists.
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create download dir: %w", err)
	}

	// Snapshot directory contents before triggering download.
	existingFiles := listDirFiles(outputDir)

	var downloadPath string
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		// Set download behavior to allow downloads to our directory.
		if _, err := send("Page.setDownloadBehavior", map[string]any{
			"behavior":     "allow",
			"downloadPath": outputDir,
		}); err != nil {
			return fmt.Errorf("set download behavior: %w", err)
		}

		// If a ref was provided, click it to trigger the download.
		if opts.Ref != "" {
			ref, err := RequireRef(opts.Ref)
			if err != nil {
				return err
			}
			x, y, err := t.getElementCenter(send, ref)
			if err != nil {
				return ToAIFriendlyError(err, ref)
			}
			if _, err := send("Input.dispatchMouseEvent", map[string]any{
				"type": "mousePressed", "x": x, "y": y,
				"button": "left", "clickCount": 1,
			}); err != nil {
				return ToAIFriendlyError(err, ref)
			}
			if _, err := send("Input.dispatchMouseEvent", map[string]any{
				"type": "mouseReleased", "x": x, "y": y,
				"button": "left", "clickCount": 1,
			}); err != nil {
				return ToAIFriendlyError(err, ref)
			}
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	// Poll for new files in the download directory.
	// Chrome creates .crdownload files during download and renames on completion.
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("download timeout: no new file appeared in %s", outputDir)
		case <-time.After(500 * time.Millisecond):
		}

		newFiles := listDirFiles(outputDir)
		for name := range newFiles {
			if _, existed := existingFiles[name]; !existed {
				// Skip .crdownload (in-progress) files.
				if strings.HasSuffix(name, ".crdownload") {
					continue
				}
				downloadPath = filepath.Join(outputDir, name)
				return downloadPath, nil
			}
		}
	}
}

func defaultDownloadDir() string {
	return filepath.Join(os.TempDir(), "openacosmi", "downloads")
}

// listDirFiles returns a set of filenames in a directory.
func listDirFiles(dir string) map[string]struct{} {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]struct{}{}
	}
	result := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			result[e.Name()] = struct{}{}
		}
	}
	return result
}

// ---------- Response interceptor ----------

// ResponseBody fetches the response body for a resource matching the URL pattern.
// Uses Fetch.enable to intercept matching requests, or falls back to
// Network.getResponseBody for already-loaded resources.
func (t *CDPPlaywrightTools) ResponseBody(ctx context.Context, opts PWResponseBodyOpts) (*PWResponseBodyResult, error) {
	pattern := strings.TrimSpace(opts.URLPattern)
	if pattern == "" {
		return nil, fmt.Errorf("url pattern is required")
	}
	maxChars := opts.MaxChars
	if maxChars <= 0 {
		maxChars = 200_000
	}
	if maxChars > 5_000_000 {
		maxChars = 5_000_000
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 20_000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	var result *PWResponseBodyResult
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		// Use Runtime.evaluate + fetch() to retrieve the resource.
		// This is more reliable than Network.getResponseBody which requires
		// catching the request in-flight.
		fetchJS := fmt.Sprintf(`(async function() {
  try {
    var resp = await fetch(%s);
    var status = resp.status;
    var hdrs = {};
    resp.headers.forEach(function(v, k) { hdrs[k] = v; });
    var body = await resp.text();
    var truncated = false;
    if (body.length > %d) { body = body.slice(0, %d); truncated = true; }
    return JSON.stringify({url: resp.url, status: status, headers: hdrs, body: body, truncated: truncated});
  } catch(e) {
    return JSON.stringify({error: e.message});
  }
})()`, jsonStringLiteral(pattern), maxChars, maxChars)

		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    fetchJS,
			"returnByValue": true,
			"awaitPromise":  true,
		})
		if err != nil {
			return fmt.Errorf("fetch resource: %w", err)
		}

		var resp struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parse fetch response: %w", err)
		}

		var s string
		if err := json.Unmarshal(resp.Result.Value, &s); err != nil {
			return fmt.Errorf("decode fetch result: %w", err)
		}

		var fetchResult struct {
			URL       string            `json:"url"`
			Status    int               `json:"status"`
			Headers   map[string]string `json:"headers"`
			Body      string            `json:"body"`
			Truncated bool              `json:"truncated"`
			Error     string            `json:"error"`
		}
		if err := json.Unmarshal([]byte(s), &fetchResult); err != nil {
			return fmt.Errorf("decode fetch JSON: %w", err)
		}
		if fetchResult.Error != "" {
			return fmt.Errorf("fetch failed: %s", fetchResult.Error)
		}

		result = &PWResponseBodyResult{
			URL:       fetchResult.URL,
			Status:    fetchResult.Status,
			Headers:   fetchResult.Headers,
			Body:      fetchResult.Body,
			Truncated: fetchResult.Truncated,
		}
		return nil
	})
	return result, err
}

// ---------- New interactions (BR-H17~H24) ----------

// Drag drags from startRef to endRef via CDP mouse events.
func (t *CDPPlaywrightTools) Drag(ctx context.Context, opts PWDragOpts) error {
	startRef, err := RequireRef(opts.StartRef)
	if err != nil {
		return err
	}
	endRef, err := RequireRef(opts.EndRef)
	if err != nil {
		return err
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 8000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		sx, sy, err := t.getElementCenter(send, startRef)
		if err != nil {
			return ToAIFriendlyError(err, startRef)
		}
		ex, ey, err := t.getElementCenter(send, endRef)
		if err != nil {
			return ToAIFriendlyError(err, endRef)
		}

		// Move to start
		if _, err := send("Input.dispatchMouseEvent", map[string]any{
			"type": "mouseMoved", "x": sx, "y": sy,
		}); err != nil {
			return err
		}
		// Press at start
		if _, err := send("Input.dispatchMouseEvent", map[string]any{
			"type": "mousePressed", "x": sx, "y": sy, "button": "left", "clickCount": 1,
		}); err != nil {
			return err
		}
		// Move to end (with intermediate step for better drag detection)
		midX, midY := (sx+ex)/2, (sy+ey)/2
		if _, err := send("Input.dispatchMouseEvent", map[string]any{
			"type": "mouseMoved", "x": midX, "y": midY,
		}); err != nil {
			return err
		}
		if _, err := send("Input.dispatchMouseEvent", map[string]any{
			"type": "mouseMoved", "x": ex, "y": ey,
		}); err != nil {
			return err
		}
		// Release at end
		if _, err := send("Input.dispatchMouseEvent", map[string]any{
			"type": "mouseReleased", "x": ex, "y": ey, "button": "left", "clickCount": 1,
		}); err != nil {
			return err
		}
		return nil
	})
}

// SelectOption selects option(s) in a <select> element via CDP.
func (t *CDPPlaywrightTools) SelectOption(ctx context.Context, opts PWSelectOptionOpts) error {
	ref, err := RequireRef(opts.Ref)
	if err != nil {
		return err
	}
	if len(opts.Values) == 0 {
		return fmt.Errorf("values are required")
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 8000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		objectID, err := t.resolveRefToObjectID(send, ref)
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Build JS array of values to select
		valuesJSON, _ := json.Marshal(opts.Values)

		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId": objectID,
			"functionDeclaration": fmt.Sprintf(`function() {
				var values = %s;
				var el = this;
				if (el.tagName !== 'SELECT') {
					throw new Error('Element is not a <select>');
				}
				for (var i = 0; i < el.options.length; i++) {
					var opt = el.options[i];
					opt.selected = values.indexOf(opt.value) >= 0 || values.indexOf(opt.textContent.trim()) >= 0;
				}
				el.dispatchEvent(new Event('input', {bubbles: true}));
				el.dispatchEvent(new Event('change', {bubbles: true}));
			}`, string(valuesJSON)),
		}); err != nil {
			return ToAIFriendlyError(err, ref)
		}
		return nil
	})
}

// PressKey presses a keyboard key via CDP Input.dispatchKeyEvent.
func (t *CDPPlaywrightTools) PressKey(ctx context.Context, opts PWPressKeyOpts) error {
	key := strings.TrimSpace(opts.Key)
	if key == "" {
		return fmt.Errorf("key is required")
	}

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		keyDef := resolveKeyDefinition(key)

		// keyDown
		if _, err := send("Input.dispatchKeyEvent", map[string]any{
			"type":                  "keyDown",
			"key":                   keyDef.key,
			"code":                  keyDef.code,
			"windowsVirtualKeyCode": keyDef.keyCode,
			"nativeVirtualKeyCode":  keyDef.keyCode,
		}); err != nil {
			return fmt.Errorf("keyDown %q: %w", key, err)
		}

		// If it's a printable character, also send char event
		if len(keyDef.text) > 0 {
			if _, err := send("Input.dispatchKeyEvent", map[string]any{
				"type": "char",
				"text": keyDef.text,
			}); err != nil {
				t.logger.Debug("char event failed", "key", key, "err", err)
			}
		}

		if opts.DelayMs > 0 {
			time.Sleep(time.Duration(opts.DelayMs) * time.Millisecond)
		}

		// keyUp
		if _, err := send("Input.dispatchKeyEvent", map[string]any{
			"type":                  "keyUp",
			"key":                   keyDef.key,
			"code":                  keyDef.code,
			"windowsVirtualKeyCode": keyDef.keyCode,
			"nativeVirtualKeyCode":  keyDef.keyCode,
		}); err != nil {
			return fmt.Errorf("keyUp %q: %w", key, err)
		}
		return nil
	})
}

// Type types text character-by-character. If Slowly, each keystroke has a 75ms delay.
func (t *CDPPlaywrightTools) Type(ctx context.Context, opts PWTypeOpts) error {
	ref, err := RequireRef(opts.Ref)
	if err != nil {
		return err
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 8000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		objectID, err := t.resolveRefToObjectID(send, ref)
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Focus the element
		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId":            objectID,
			"functionDeclaration": "function() { this.focus(); }",
		}); err != nil {
			return ToAIFriendlyError(err, ref)
		}

		if opts.Slowly {
			// Type character by character with delay
			for _, ch := range opts.Text {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				charStr := string(ch)
				if _, err := send("Input.dispatchKeyEvent", map[string]any{
					"type": "char",
					"text": charStr,
				}); err != nil {
					return ToAIFriendlyError(err, ref)
				}
				time.Sleep(75 * time.Millisecond)
			}
		} else {
			// Fast path: clear and insert
			if _, err := send("Runtime.callFunctionOn", map[string]any{
				"objectId":            objectID,
				"functionDeclaration": "function() { this.value = ''; this.dispatchEvent(new Event('input', {bubbles:true})); }",
			}); err != nil {
				return ToAIFriendlyError(err, ref)
			}
			if _, err := send("Input.insertText", map[string]any{
				"text": opts.Text,
			}); err != nil {
				return ToAIFriendlyError(err, ref)
			}
		}

		// Dispatch change
		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId":            objectID,
			"functionDeclaration": "function() { this.dispatchEvent(new Event('change', {bubbles:true})); }",
		}); err != nil {
			t.logger.Debug("type: dispatch change failed", "err", err)
		}

		if opts.Submit {
			if _, err := send("Input.dispatchKeyEvent", map[string]any{
				"type":                  "keyDown",
				"key":                   "Enter",
				"code":                  "Enter",
				"windowsVirtualKeyCode": 13,
			}); err != nil {
				return fmt.Errorf("submit Enter keyDown: %w", err)
			}
			if _, err := send("Input.dispatchKeyEvent", map[string]any{
				"type":                  "keyUp",
				"key":                   "Enter",
				"code":                  "Enter",
				"windowsVirtualKeyCode": 13,
			}); err != nil {
				t.logger.Debug("submit Enter keyUp failed", "err", err)
			}
		}

		return nil
	})
}

// ScrollIntoView scrolls the element into the viewport via CDP.
func (t *CDPPlaywrightTools) ScrollIntoView(ctx context.Context, opts PWScrollIntoViewOpts) error {
	ref, err := RequireRef(opts.Ref)
	if err != nil {
		return err
	}
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 20000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		objectID, err := t.resolveRefToObjectID(send, ref)
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}

		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId":            objectID,
			"functionDeclaration": "function() { this.scrollIntoView({block:'center',inline:'center',behavior:'smooth'}); }",
		}); err != nil {
			return ToAIFriendlyError(err, ref)
		}

		// Brief pause for smooth scroll to complete
		time.Sleep(200 * time.Millisecond)
		return nil
	})
}

// Evaluate runs a JS expression in the page context via CDP Runtime.evaluate.
func (t *CDPPlaywrightTools) Evaluate(ctx context.Context, opts PWEvaluateOpts) (json.RawMessage, error) {
	expr := strings.TrimSpace(opts.Expression)
	if expr == "" {
		return nil, fmt.Errorf("expression is required")
	}

	var result json.RawMessage
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		if opts.Ref != "" {
			// Evaluate in element context
			objectID, err := t.resolveRefToObjectID(send, opts.Ref)
			if err != nil {
				return ToAIFriendlyError(err, opts.Ref)
			}
			raw, err := send("Runtime.callFunctionOn", map[string]any{
				"objectId": objectID,
				"functionDeclaration": fmt.Sprintf(`function() {
					"use strict";
					try {
						var candidate = eval("(%s)");
						return typeof candidate === "function" ? candidate(this) : candidate;
					} catch (err) {
						throw new Error("Invalid evaluate function: " + (err && err.message ? err.message : String(err)));
					}
				}`, expr),
				"returnByValue": true,
			})
			if err != nil {
				return fmt.Errorf("evaluate on element: %w", err)
			}
			result = raw
			return nil
		}

		raw, err := send("Runtime.evaluate", map[string]any{
			"expression": fmt.Sprintf(`(function() {
				"use strict";
				try {
					var candidate = eval("(%s)");
					return typeof candidate === "function" ? candidate() : candidate;
				} catch (err) {
					throw new Error("Invalid evaluate function: " + (err && err.message ? err.message : String(err)));
				}
			})()`, expr),
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("evaluate: %w", err)
		}
		result = raw
		return nil
	})
	return result, err
}

// WaitFor waits for conditions via CDP polling.
func (t *CDPPlaywrightTools) WaitFor(ctx context.Context, opts PWWaitForOpts) error {
	timeout := NormalizeTimeoutMs(opts.TimeoutMs, 20000)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	// 1. Static wait
	if opts.TimeMs > 0 {
		ms := opts.TimeMs
		if ms > 120_000 {
			ms = 120_000
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(ms) * time.Millisecond):
		}
	}

	// 2. Wait for text visible
	if opts.Text != "" {
		if err := t.pollCondition(ctx, opts.PWTargetOpts, fmt.Sprintf(
			`document.body && document.body.innerText.includes(%s)`,
			jsonStringLiteral(opts.Text),
		)); err != nil {
			return fmt.Errorf("waitFor text %q: %w", opts.Text, err)
		}
	}

	// 3. Wait for text gone
	if opts.TextGone != "" {
		if err := t.pollCondition(ctx, opts.PWTargetOpts, fmt.Sprintf(
			`!document.body || !document.body.innerText.includes(%s)`,
			jsonStringLiteral(opts.TextGone),
		)); err != nil {
			return fmt.Errorf("waitFor textGone %q: %w", opts.TextGone, err)
		}
	}

	// 4. Wait for URL
	if opts.URL != "" {
		if err := t.pollCondition(ctx, opts.PWTargetOpts, fmt.Sprintf(
			`window.location.href.includes(%s)`,
			jsonStringLiteral(opts.URL),
		)); err != nil {
			return fmt.Errorf("waitFor url %q: %w", opts.URL, err)
		}
	}

	// 5. Wait for load state
	if opts.LoadState != "" {
		var expr string
		switch opts.LoadState {
		case "load":
			expr = `document.readyState === "complete"`
		case "domcontentloaded":
			expr = `document.readyState === "complete" || document.readyState === "interactive"`
		case "networkidle":
			// Best approximation: document is complete
			expr = `document.readyState === "complete"`
		default:
			expr = `document.readyState === "complete"`
		}
		if err := t.pollCondition(ctx, opts.PWTargetOpts, expr); err != nil {
			return fmt.Errorf("waitFor loadState %q: %w", opts.LoadState, err)
		}
	}

	// 6. Wait for JS function
	if opts.Fn != "" {
		fn := strings.TrimSpace(opts.Fn)
		if err := t.pollCondition(ctx, opts.PWTargetOpts, fmt.Sprintf(
			`(function() { try { return !!(%s); } catch(e) { return false; } })()`, fn,
		)); err != nil {
			return fmt.Errorf("waitFor fn: %w", err)
		}
	}

	return nil
}

// pollCondition polls a JS expression until it returns truthy or context expires.
func (t *CDPPlaywrightTools) pollCondition(ctx context.Context, target PWTargetOpts, jsExpr string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ok, err := t.evaluateBool(ctx, target, jsExpr)
			if err != nil {
				continue // Ignore transient errors
			}
			if ok {
				return nil
			}
		}
	}
}

// evaluateBool evaluates a JS expression and returns its boolean result.
func (t *CDPPlaywrightTools) evaluateBool(ctx context.Context, target PWTargetOpts, expr string) (bool, error) {
	var result bool
	err := WithCdpSocket(ctx, t.resolveTargetWsURL(target), func(send CdpSendFn) error {
		raw, err := send("Runtime.evaluate", map[string]any{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			return err
		}
		var resp struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		result = resp.Result.Value
		return nil
	})
	return result, err
}

// SetInputFiles sets files on a file input element via CDP.
func (t *CDPPlaywrightTools) SetInputFiles(ctx context.Context, opts PWSetInputFilesOpts) error {
	ref := strings.TrimSpace(opts.Ref)
	element := strings.TrimSpace(opts.Element)
	if ref == "" && element == "" {
		return fmt.Errorf("ref or element is required")
	}
	if ref != "" && element != "" {
		return fmt.Errorf("ref and element are mutually exclusive")
	}
	if len(opts.Paths) == 0 {
		return fmt.Errorf("paths are required")
	}

	return WithCdpSocket(ctx, t.resolveTargetWsURL(opts.PWTargetOpts), func(send CdpSendFn) error {
		var objectID string
		var err error

		if ref != "" {
			objectID, err = t.resolveRefToObjectID(send, ref)
		} else {
			// Resolve by CSS selector
			raw, qerr := send("Runtime.evaluate", map[string]any{
				"expression": fmt.Sprintf(`document.querySelector(%s)`, jsonStringLiteral(element)),
			})
			if qerr != nil {
				return fmt.Errorf("querySelector %q: %w", element, qerr)
			}
			var resp struct {
				Result struct {
					ObjectID string `json:"objectId"`
				} `json:"result"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			objectID = resp.Result.ObjectID
		}
		if err != nil {
			return ToAIFriendlyError(err, ref)
		}
		if objectID == "" {
			return fmt.Errorf("element not found")
		}

		// Get node ID
		raw, err := send("DOM.requestNode", map[string]any{"objectId": objectID})
		if err != nil {
			return fmt.Errorf("DOM.requestNode: %w", err)
		}
		var nodeResp struct {
			NodeID int `json:"nodeId"`
		}
		if err := json.Unmarshal(raw, &nodeResp); err != nil {
			return err
		}

		// Set files via DOM.setFileInputFiles
		if _, err := send("DOM.setFileInputFiles", map[string]any{
			"files":  opts.Paths,
			"nodeId": nodeResp.NodeID,
		}); err != nil {
			return fmt.Errorf("setFileInputFiles: %w", err)
		}

		// Dispatch input+change events
		if _, err := send("Runtime.callFunctionOn", map[string]any{
			"objectId": objectID,
			"functionDeclaration": `function() {
				this.dispatchEvent(new Event('input', {bubbles:true}));
				this.dispatchEvent(new Event('change', {bubbles:true}));
			}`,
		}); err != nil {
			t.logger.Debug("setInputFiles: dispatch events failed", "err", err)
		}
		return nil
	})
}

// ---------- Key definition helpers ----------

type keyDefinition struct {
	key     string
	code    string
	keyCode int
	text    string
}

// buildModifierFlags converts modifier string names to CDP bitmask.
// CDP modifiers: Alt=1, Ctrl=2, Meta/Command=4, Shift=8
func buildModifierFlags(modifiers []string) int {
	flags := 0
	for _, m := range modifiers {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "alt":
			flags |= 1
		case "ctrl", "control":
			flags |= 2
		case "meta", "command", "cmd":
			flags |= 4
		case "shift":
			flags |= 8
		}
	}
	return flags
}

// resolveKeyDefinition maps a key name to CDP key event properties.
func resolveKeyDefinition(key string) keyDefinition {
	switch key {
	case "Enter":
		return keyDefinition{"Enter", "Enter", 13, "\r"}
	case "Tab":
		return keyDefinition{"Tab", "Tab", 9, "\t"}
	case "Backspace":
		return keyDefinition{"Backspace", "Backspace", 8, ""}
	case "Delete":
		return keyDefinition{"Delete", "Delete", 46, ""}
	case "Escape":
		return keyDefinition{"Escape", "Escape", 27, ""}
	case "ArrowUp":
		return keyDefinition{"ArrowUp", "ArrowUp", 38, ""}
	case "ArrowDown":
		return keyDefinition{"ArrowDown", "ArrowDown", 40, ""}
	case "ArrowLeft":
		return keyDefinition{"ArrowLeft", "ArrowLeft", 37, ""}
	case "ArrowRight":
		return keyDefinition{"ArrowRight", "ArrowRight", 39, ""}
	case "Home":
		return keyDefinition{"Home", "Home", 36, ""}
	case "End":
		return keyDefinition{"End", "End", 35, ""}
	case "PageUp":
		return keyDefinition{"PageUp", "PageUp", 33, ""}
	case "PageDown":
		return keyDefinition{"PageDown", "PageDown", 34, ""}
	case " ", "Space":
		return keyDefinition{" ", "Space", 32, " "}
	default:
		// For single printable characters
		if len(key) == 1 {
			ch := key[0]
			code := "Key" + strings.ToUpper(key)
			if ch >= '0' && ch <= '9' {
				code = "Digit" + key
			}
			return keyDefinition{key, code, int(ch), key}
		}
		// Unknown key: pass through as-is
		return keyDefinition{key, key, 0, ""}
	}
}

// jsonStringLiteral returns a JSON-escaped string literal for use in JS expressions.
func jsonStringLiteral(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ---------- Internal CDP helpers ----------

// resolveTargetWsURL maps a PWTargetOpts to a WebSocket URL.
// If TargetID is specified, we use target-specific CDP endpoint.
func (t *CDPPlaywrightTools) resolveTargetWsURL(opts PWTargetOpts) string {
	cdpURL := opts.CDPURL
	if cdpURL == "" {
		cdpURL = t.cdpURL
	}
	if opts.TargetID != "" {
		// For target-specific connections, append /devtools/page/<targetId>
		base := strings.TrimSuffix(cdpURL, "/")
		// Replace any existing /devtools/... path with the target-specific one
		if idx := strings.Index(base, "/devtools/"); idx >= 0 {
			base = base[:idx]
		}
		return base + "/devtools/page/" + opts.TargetID
	}
	// If browser-level URL, resolve first page target's WS URL.
	// Page/Runtime domain commands don't work on browser-level WebSocket.
	if IsBrowserWsURL(cdpURL) {
		if t.cachedPageWsURL != "" {
			return t.cachedPageWsURL
		}
		if pageURL := ResolveFirstPageWsURL(cdpURL); pageURL != "" {
			t.cachedPageWsURL = pageURL
			t.logger.Debug("cdp tools: resolved page-level WS URL", "page", pageURL)
			return pageURL
		}
	}
	return cdpURL
}

// resolveRefToObjectID resolves a role ref (e.g. "e12") to a CDP remote object ID.
// Phase 4.2: Tries cached CSS selector first (Stagehand pattern), then falls back
// to full DOM scan. On success, generates and caches a CSS selector for next use.
func (t *CDPPlaywrightTools) resolveRefToObjectID(send CdpSendFn, ref string) (string, error) {
	// Get current page URL for cache keying.
	pageURL := t.getCurrentPageURL(send)

	// Phase 4.2: Try cached CSS selector first (zero full-DOM-scan path).
	if cached := t.getCachedSelector(pageURL, ref); cached != "" {
		objectID, err := t.resolveBySelector(send, cached)
		if err == nil && objectID != "" {
			t.logger.Debug("selector cache hit", "ref", ref, "selector", cached)
			return objectID, nil
		}
		// Cache miss (DOM changed) — fall through to full scan.
		t.logger.Debug("selector cache stale", "ref", ref, "selector", cached)
	}

	// Full DOM scan: query all elements, filter interactive ones, pick by index.
	raw, err := send("Runtime.evaluate", map[string]any{
		"expression": fmt.Sprintf(`(function() {
			var all = document.querySelectorAll('*');
			var idx = parseInt('%s'.replace('e',''), 10) - 1;
			var interactive = [];
			for (var i = 0; i < all.length; i++) {
				var el = all[i];
				var tag = el.tagName.toLowerCase();
				var role = el.getAttribute('role') || '';
				if (tag === 'button' || tag === 'a' || tag === 'input' ||
						tag === 'select' || tag === 'textarea' ||
						role === 'button' || role === 'link' || role === 'textbox' ||
						role === 'checkbox' || role === 'radio' || role === 'combobox' ||
						role === 'menuitem' || role === 'tab' || role === 'switch' ||
						role === 'option' || role === 'treeitem' ||
						el.tabIndex >= 0) {
					interactive.push(el);
				}
			}
			if (idx >= 0 && idx < interactive.length) {
				return interactive[idx];
			}
			return null;
		})()`, ref),
	})
	if err != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, err)
	}

	var resp struct {
		Result struct {
			ObjectID string `json:"objectId"`
			Type     string `json:"type"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse ref resolution: %w", err)
	}

	if resp.Result.ObjectID == "" || resp.Result.Type == "undefined" {
		return "", fmt.Errorf("element %q not found or not visible", ref)
	}

	// Phase 4.2: Generate CSS selector for the resolved element and cache it.
	if selector := t.generateCSSSelector(send, resp.Result.ObjectID); selector != "" {
		t.putCachedSelector(pageURL, ref, selector)
		t.logger.Debug("selector cached", "ref", ref, "selector", selector)
	}

	return resp.Result.ObjectID, nil
}

// resolveBySelector resolves a CSS selector to a CDP remote object ID.
func (t *CDPPlaywrightTools) resolveBySelector(send CdpSendFn, selector string) (string, error) {
	raw, err := send("Runtime.evaluate", map[string]any{
		"expression":    fmt.Sprintf("document.querySelector(%q)", selector),
		"returnByValue": false,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Result struct {
			ObjectID string `json:"objectId"`
			Type     string `json:"type"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.Result.ObjectID == "" || resp.Result.Type == "undefined" {
		return "", fmt.Errorf("selector %q: element not found", selector)
	}
	return resp.Result.ObjectID, nil
}

// generateCSSSelector generates a unique CSS selector for an element by objectID.
// Uses a JS function that walks up the DOM building tag[nth-child] path.
func (t *CDPPlaywrightTools) generateCSSSelector(send CdpSendFn, objectID string) string {
	raw, err := send("Runtime.callFunctionOn", map[string]any{
		"objectId": objectID,
		"functionDeclaration": `function() {
			var el = this;
			var parts = [];
			while (el && el.nodeType === 1) {
				var tag = el.tagName.toLowerCase();
				if (el.id) {
					parts.unshift('#' + CSS.escape(el.id));
					break;
				}
				var idx = 1;
				var sib = el.previousElementSibling;
				while (sib) {
					if (sib.tagName === el.tagName) idx++;
					sib = sib.previousElementSibling;
				}
				parts.unshift(tag + ':nth-child(' + idx + ')');
				el = el.parentElement;
			}
			return parts.join(' > ');
		}`,
		"returnByValue": true,
	})
	if err != nil {
		return ""
	}
	var resp struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Result.Value == "" {
		return ""
	}
	return resp.Result.Value
}

// getCurrentPageURL retrieves the current page URL for cache keying.
func (t *CDPPlaywrightTools) getCurrentPageURL(send CdpSendFn) string {
	raw, err := send("Runtime.evaluate", map[string]any{
		"expression":    "window.location.href",
		"returnByValue": true,
	})
	if err != nil {
		return ""
	}
	var resp struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		return ""
	}
	return resp.Result.Value
}

// getElementCenter resolves a ref and returns the center coordinates of the element.
func (t *CDPPlaywrightTools) getElementCenter(send CdpSendFn, ref string) (float64, float64, error) {
	objectID, err := t.resolveRefToObjectID(send, ref)
	if err != nil {
		return 0, 0, err
	}

	// Use actionability checks (Playwright pattern) to wait for element to be
	// attached, visible, stable, not obscured, and enabled.
	cx, cy, err := EnsureActionable(send, objectID, 8000)
	if err != nil {
		return 0, 0, fmt.Errorf("element %q: %w", ref, err)
	}

	return cx, cy, nil
}

// extractStringValue extracts a string from a nested AX node property.
func extractStringValue(node map[string]any, key string) (string, bool) {
	v, ok := node[key]
	if !ok {
		return "", false
	}
	switch val := v.(type) {
	case string:
		return val, true
	case map[string]any:
		if s, ok := val["value"].(string); ok {
			return s, true
		}
	}
	return "", false
}
