package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
)

const (
	xhsLoginOpenURL   = "https://creator.xiaohongshu.com/publish/publish"
	xhsLoginCookieTTL = 15 * time.Minute
)

// BrowserRuntimeResolver resolves a usable PlaywrightTools + CDP runtime on demand.
// It allows the XHS client to participate in the gateway's lazy browser lifecycle.
type BrowserRuntimeResolver func(context.Context) (browser.PlaywrightTools, string, *browser.ChromeInstance, error)

// BrowserLaunchHook records a lazily auto-launched Chrome instance into the gateway lifecycle.
type BrowserLaunchHook func(*browser.ChromeInstance)

// LoginSessionState tracks a single interactive login capture flow.
type LoginSessionState struct {
	AccountID    string     `json:"accountId"`
	TargetID     string     `json:"targetId,omitempty"`
	LoginURL     string     `json:"loginUrl"`
	CookiePath   string     `json:"cookiePath"`
	Status       string     `json:"status"`
	Message      string     `json:"message,omitempty"`
	CurrentURL   string     `json:"currentUrl,omitempty"`
	CookieCount  int        `json:"cookieCount,omitempty"`
	BrowserReady bool       `json:"browserReady"`
	StartedAt    time.Time  `json:"startedAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	SavedAt      *time.Time `json:"savedAt,omitempty"`
	LastError    string     `json:"lastError,omitempty"`
}

// XHSAuthState summarizes the currently known auth state for UI display.
type XHSAuthState struct {
	Status       string
	Message      string
	UpdatedAt    *time.Time
	BrowserReady bool
	CookiePath   string
}

type loginCaptureResult struct {
	Authenticated bool
	TargetID      string
	CurrentURL    string
	Cookies       []CookieEntry
}

var listLoginCandidateTargets = func(ctx context.Context, cdpURL string) ([]browser.CDPTarget, error) {
	baseURL := strings.TrimSpace(cdpURL)
	if baseURL == "" {
		return nil, fmt.Errorf("cdp url is empty")
	}
	switch {
	case strings.HasPrefix(baseURL, "ws://"):
		baseURL = "http://" + strings.TrimPrefix(baseURL, "ws://")
	case strings.HasPrefix(baseURL, "wss://"):
		baseURL = "https://" + strings.TrimPrefix(baseURL, "wss://")
	}
	if idx := strings.Index(baseURL, "/devtools/"); idx >= 0 {
		baseURL = baseURL[:idx]
	}
	endpoint, err := browser.AppendCdpPath(baseURL, "/json")
	if err != nil {
		return nil, err
	}
	var targets []browser.CDPTarget
	if err := browser.FetchJSON(ctx, endpoint, &targets, 3000); err != nil {
		return nil, err
	}
	return targets, nil
}

// SetBrowserRuntimeResolver wires the client into the gateway/browser lazy-init chain.
func (c *XHSRPAClient) SetBrowserRuntimeResolver(
	resolver BrowserRuntimeResolver,
	onLaunch BrowserLaunchHook,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.browserResolver = resolver
	c.onBrowserLaunch = onLaunch
}

func normalizeCDPWebSocketURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("cdp url is empty")
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		wsURL, err := browser.GetChromeWebSocketURL(trimmed, 2000)
		if err != nil {
			return "", fmt.Errorf("resolve cdp websocket url: %w", err)
		}
		return wsURL, nil
	}
	return trimmed, nil
}

func cookieEntriesValid(cookies []CookieEntry) bool {
	if len(cookies) == 0 {
		return false
	}
	now := time.Now().Unix()
	for _, cookie := range cookies {
		if cookie.Expires > 0 && cookie.Expires < now {
			return false
		}
	}
	return true
}

func filterXiaohongshuCookies(cookies []CookieEntry) []CookieEntry {
	filtered := make([]CookieEntry, 0, len(cookies))
	for _, cookie := range cookies {
		domain := strings.ToLower(strings.TrimSpace(cookie.Domain))
		if domain == "" || !strings.Contains(domain, "xiaohongshu.com") {
			continue
		}
		filtered = append(filtered, cookie)
	}
	return filtered
}

func cookieNames(cookies []CookieEntry) map[string]struct{} {
	names := make(map[string]struct{}, len(cookies))
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}
	return names
}

func looksLikeAuthenticatedXHS(currentURL string, cookies []CookieEntry) bool {
	if len(cookies) < 2 {
		return false
	}

	urlLower := strings.ToLower(strings.TrimSpace(currentURL))
	names := cookieNames(cookies)
	if _, ok := names["web_session"]; ok {
		return true
	}
	if urlLower == "" || !strings.Contains(urlLower, "xiaohongshu.com") {
		return false
	}
	if strings.Contains(urlLower, "login") {
		return false
	}
	if _, ok := names["a1"]; ok {
		return true
	}
	return len(cookies) >= 4
}

func isXiaohongshuURL(raw string) bool {
	urlLower := strings.ToLower(strings.TrimSpace(raw))
	return urlLower != "" && strings.Contains(urlLower, "xiaohongshu.com")
}

func mergeLoginTargetCandidates(sessionTargetID string, targets []browser.CDPTarget) []string {
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(targets)+1)
	if strings.TrimSpace(sessionTargetID) != "" {
		ordered = append(ordered, sessionTargetID)
		seen[sessionTargetID] = struct{}{}
	}
	for _, target := range targets {
		if target.Type != "" && target.Type != "page" {
			continue
		}
		if !isXiaohongshuURL(target.URL) {
			continue
		}
		if strings.TrimSpace(target.ID) == "" {
			continue
		}
		if _, ok := seen[target.ID]; ok {
			continue
		}
		ordered = append(ordered, target.ID)
		seen[target.ID] = struct{}{}
	}
	return ordered
}

func betterPendingCapture(current, candidate *loginCaptureResult, sessionTargetID string) *loginCaptureResult {
	switch {
	case candidate == nil:
		return current
	case current == nil:
		return candidate
	case candidate.TargetID == sessionTargetID && current.TargetID != sessionTargetID:
		return candidate
	case len(candidate.Cookies) > len(current.Cookies):
		return candidate
	case current.CurrentURL == "" && candidate.CurrentURL != "":
		return candidate
	default:
		return current
	}
}

func toCookieEntries(raw []map[string]any) []CookieEntry {
	entries := make([]CookieEntry, 0, len(raw))
	for _, item := range raw {
		name, _ := item["name"].(string)
		value, _ := item["value"].(string)
		domain, _ := item["domain"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		entry := CookieEntry{
			Name:   name,
			Value:  value,
			Domain: domain,
		}
		if path, ok := item["path"].(string); ok {
			entry.Path = path
		}
		switch expires := item["expires"].(type) {
		case float64:
			entry.Expires = int64(expires)
		case int64:
			entry.Expires = expires
		case int:
			entry.Expires = int64(expires)
		}
		entries = append(entries, entry)
	}
	return entries
}

func cloneLoginSession(session *LoginSessionState) *LoginSessionState {
	if session == nil {
		return nil
	}
	copySession := *session
	if session.SavedAt != nil {
		savedAt := *session.SavedAt
		copySession.SavedAt = &savedAt
	}
	return &copySession
}

func (c *XHSRPAClient) ensureBrowserRuntime(ctx context.Context) error {
	c.mu.Lock()
	pwTools := c.pwTools
	cdpURL := c.cdpURL
	hasBrowser := c.browser != nil
	resolver := c.browserResolver
	onLaunch := c.onBrowserLaunch
	errDir := c.errShotDir
	c.mu.Unlock()

	if pwTools != nil && strings.TrimSpace(cdpURL) != "" {
		if !hasBrowser {
			c.SetBrowserFromPlaywright(pwTools, cdpURL, errDir)
		}
		return nil
	}
	if resolver == nil {
		ensured, err := browser.EnsureChrome(ctx, slog.Default())
		if err != nil {
			return fmt.Errorf("ensure chrome: %w", err)
		}
		runtimeURL := ensured.WSURL
		if runtimeURL == "" {
			runtimeURL = ensured.CDPURL
		}
		if strings.TrimSpace(runtimeURL) == "" {
			if ensured.Instance != nil {
				_ = ensured.Instance.Stop()
			}
			return fmt.Errorf("chrome CDP endpoint unavailable")
		}
		c.SetBrowserFromPlaywright(browser.NewCDPPlaywrightTools(runtimeURL, slog.Default()), runtimeURL, errDir)
		return nil
	}

	tools, resolvedURL, instance, err := resolver(ctx)
	if err != nil {
		return fmt.Errorf("resolve browser runtime: %w", err)
	}
	if onLaunch != nil && instance != nil {
		onLaunch(instance)
	}
	if strings.TrimSpace(resolvedURL) == "" {
		return fmt.Errorf("browser runtime returned empty cdp url")
	}
	c.SetBrowserFromPlaywright(tools, resolvedURL, errDir)
	return nil
}

func (c *XHSRPAClient) readLoginTargetCapture(
	ctx context.Context,
	cdpURL string,
	pwTools browser.PlaywrightTools,
	targetID string,
) (*loginCaptureResult, error) {
	target := browser.PWTargetOpts{
		CDPURL:   cdpURL,
		TargetID: targetID,
	}

	rawURL, err := pwTools.Evaluate(ctx, browser.PWEvaluateOpts{
		PWTargetOpts: target,
		Expression:   "window.location.href",
	})
	if err != nil {
		return nil, fmt.Errorf("read current url: %w", err)
	}
	var currentURL string
	if err := json.Unmarshal(rawURL, &currentURL); err != nil {
		return nil, fmt.Errorf("decode current url: %w", err)
	}

	rawCookies, err := pwTools.CookiesGet(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("read cookies: %w", err)
	}
	cookies := filterXiaohongshuCookies(toCookieEntries(rawCookies))
	return &loginCaptureResult{
		Authenticated: looksLikeAuthenticatedXHS(currentURL, cookies),
		TargetID:      strings.TrimSpace(targetID),
		CurrentURL:    currentURL,
		Cookies:       cookies,
	}, nil
}

func (c *XHSRPAClient) captureLoginSession(ctx context.Context, session *LoginSessionState) (*loginCaptureResult, error) {
	c.mu.Lock()
	pwTools := c.pwTools
	cdpURL := c.cdpURL
	c.mu.Unlock()

	if pwTools == nil || strings.TrimSpace(cdpURL) == "" {
		return nil, fmt.Errorf("browser runtime not ready")
	}

	targets, err := listLoginCandidateTargets(ctx, cdpURL)
	if err != nil {
		slog.Warn("xiaohongshu login: list candidate targets failed", "error", err)
	}
	candidateIDs := mergeLoginTargetCandidates(session.TargetID, targets)

	var pending *loginCaptureResult
	var lastErr error
	for _, targetID := range candidateIDs {
		capture, err := c.readLoginTargetCapture(ctx, cdpURL, pwTools, targetID)
		if err != nil {
			lastErr = err
			continue
		}
		pending = betterPendingCapture(pending, capture, strings.TrimSpace(session.TargetID))
		if capture.Authenticated {
			return capture, nil
		}
	}
	if pending != nil {
		return pending, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return &loginCaptureResult{
		TargetID: strings.TrimSpace(session.TargetID),
	}, nil
}

func persistCookieEntries(cookiePath string, cookies []CookieEntry) error {
	if strings.TrimSpace(cookiePath) == "" {
		return fmt.Errorf("cookie path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(cookiePath), 0o755); err != nil {
		return fmt.Errorf("create cookie dir: %w", err)
	}
	payload, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cookies: %w", err)
	}
	if err := os.WriteFile(cookiePath, payload, 0o600); err != nil {
		return fmt.Errorf("write cookies: %w", err)
	}
	return nil
}

func (c *XHSRPAClient) StartLoginFlow(ctx context.Context, accountID string) (*LoginSessionState, error) {
	if err := c.ensureBrowserRuntime(ctx); err != nil {
		return nil, err
	}

	now := time.Now()
	c.mu.Lock()
	if c.loginSession != nil &&
		c.loginSession.Status == "waiting_login" &&
		now.Sub(c.loginSession.StartedAt) < xhsLoginCookieTTL {
		existing := cloneLoginSession(c.loginSession)
		c.mu.Unlock()
		return existing, nil
	}
	cookiePath := strings.TrimSpace(c.cfg.CookiePath)
	cdpURL := c.cdpURL
	c.mu.Unlock()

	wsURL, err := normalizeCDPWebSocketURL(cdpURL)
	if err != nil {
		return nil, err
	}
	targetID, err := browser.NewCDPClient(wsURL, nil).CreateTarget(ctx, xhsLoginOpenURL)
	if err != nil {
		return nil, fmt.Errorf("open xiaohongshu login page: %w", err)
	}

	session := &LoginSessionState{
		AccountID:    strings.TrimSpace(accountID),
		TargetID:     targetID,
		LoginURL:     xhsLoginOpenURL,
		CookiePath:   cookiePath,
		Status:       "waiting_login",
		Message:      "已打开小红书创作者页，请在浏览器完成扫码/登录后点击“检查并保存登录态”。",
		BrowserReady: true,
		StartedAt:    now,
		UpdatedAt:    now,
	}

	c.mu.Lock()
	c.loginSession = session
	c.mu.Unlock()
	return cloneLoginSession(session), nil
}

func (c *XHSRPAClient) WaitLoginFlow(ctx context.Context, timeout time.Duration) (*LoginSessionState, error) {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if err := c.ensureBrowserRuntime(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	session := cloneLoginSession(c.loginSession)
	c.mu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("no active xiaohongshu login session")
	}

	deadline := time.Now().Add(timeout)
	for {
		capture, err := c.captureLoginSession(ctx, session)
		now := time.Now()
		currentURL := ""
		cookies := []CookieEntry(nil)
		authenticated := false
		if capture != nil {
			currentURL = capture.CurrentURL
			cookies = capture.Cookies
			authenticated = capture.Authenticated
			if strings.TrimSpace(capture.TargetID) != "" {
				session.TargetID = capture.TargetID
			}
		}

		c.mu.Lock()
		live := c.loginSession
		if live != nil {
			live.BrowserReady = true
			live.UpdatedAt = now
			if capture != nil && strings.TrimSpace(capture.TargetID) != "" {
				live.TargetID = capture.TargetID
			}
			live.CurrentURL = currentURL
			if err != nil {
				live.LastError = err.Error()
				live.Status = "error"
				live.Message = "读取浏览器登录状态失败，请确认浏览器仍保持打开。"
			}
		}
		c.mu.Unlock()

		if err != nil {
			c.mu.Lock()
			snapshot := cloneLoginSession(c.loginSession)
			c.mu.Unlock()
			return snapshot, err
		}

		if authenticated {
			if err := persistCookieEntries(session.CookiePath, cookies); err != nil {
				return nil, err
			}
			_ = c.LoadCookies()

			savedAt := time.Now()
			c.mu.Lock()
			if c.loginSession != nil {
				c.loginSession.Status = "authenticated"
				c.loginSession.Message = fmt.Sprintf("已保存 %d 个小红书 Cookie，发布与互动链路现在可直接复用。", len(cookies))
				c.loginSession.CookieCount = len(cookies)
				c.loginSession.SavedAt = &savedAt
				c.loginSession.UpdatedAt = savedAt
				c.loginSession.LastError = ""
			}
			snapshot := cloneLoginSession(c.loginSession)
			c.mu.Unlock()
			return snapshot, nil
		}

		if time.Now().After(deadline) {
			c.mu.Lock()
			if c.loginSession != nil {
				c.loginSession.Status = "waiting_login"
				c.loginSession.Message = "还未检测到有效登录态。请确认浏览器内已完成扫码/登录，然后再次点击检查。"
				c.loginSession.CookieCount = len(cookies)
				c.loginSession.UpdatedAt = now
			}
			snapshot := cloneLoginSession(c.loginSession)
			c.mu.Unlock()
			return snapshot, nil
		}

		time.Sleep(1500 * time.Millisecond)
	}
}

func (c *XHSRPAClient) AuthState() XHSAuthState {
	_ = c.LoadCookiesIfAvailable()

	c.mu.Lock()
	session := cloneLoginSession(c.loginSession)
	cfg := c.cfg
	pwTools := c.pwTools
	cdpURL := c.cdpURL
	hasBrowser := c.browser != nil
	hasResolver := c.browserResolver != nil
	cookies := make([]CookieEntry, len(c.cookies))
	copy(cookies, c.cookies)
	c.mu.Unlock()

	browserReady := hasBrowser || (pwTools != nil && strings.TrimSpace(cdpURL) != "") || hasResolver
	if session != nil {
		return XHSAuthState{
			Status:       session.Status,
			Message:      session.Message,
			UpdatedAt:    &session.UpdatedAt,
			BrowserReady: browserReady,
			CookiePath:   session.CookiePath,
		}
	}

	if cfg == nil || strings.TrimSpace(cfg.CookiePath) == "" {
		return XHSAuthState{
			Status:       "cookie_path_missing",
			Message:      "尚未设置 Cookie 保存路径，首次登录时系统会自动分配默认路径。",
			BrowserReady: browserReady,
		}
	}

	info, err := os.Stat(cfg.CookiePath)
	if err != nil {
		return XHSAuthState{
			Status:       "not_logged_in",
			Message:      "尚未采集到小红书登录态。",
			BrowserReady: browserReady,
			CookiePath:   cfg.CookiePath,
		}
	}

	modifiedAt := info.ModTime()
	if cookieEntriesValid(cookies) {
		return XHSAuthState{
			Status:       "authenticated",
			Message:      "检测到有效 Cookie，可直接发布和互动。",
			UpdatedAt:    &modifiedAt,
			BrowserReady: browserReady,
			CookiePath:   cfg.CookiePath,
		}
	}

	return XHSAuthState{
		Status:       "cookie_invalid",
		Message:      "Cookie 文件存在，但当前未检测到有效登录态，建议重新采集。",
		UpdatedAt:    &modifiedAt,
		BrowserReady: browserReady,
		CookiePath:   cfg.CookiePath,
	}
}
