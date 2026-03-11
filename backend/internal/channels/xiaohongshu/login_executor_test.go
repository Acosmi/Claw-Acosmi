package xiaohongshu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
)

type loginMockPlaywrightTools struct {
	browser.StubPlaywrightTools
	evaluateResult json.RawMessage
	cookiesResult  []map[string]any
	targetURLs     map[string]json.RawMessage
	targetCookies  map[string][]map[string]any
}

func (m *loginMockPlaywrightTools) Evaluate(_ context.Context, opts browser.PWEvaluateOpts) (json.RawMessage, error) {
	if opts.TargetID != "" && m.targetURLs != nil {
		if result, ok := m.targetURLs[opts.TargetID]; ok {
			return result, nil
		}
	}
	if m.evaluateResult != nil {
		return m.evaluateResult, nil
	}
	return json.RawMessage(`"https://creator.xiaohongshu.com/login"`), nil
}

func (m *loginMockPlaywrightTools) CookiesGet(_ context.Context, opts browser.PWTargetOpts) ([]map[string]any, error) {
	if opts.TargetID != "" && m.targetCookies != nil {
		if result, ok := m.targetCookies[opts.TargetID]; ok {
			return result, nil
		}
	}
	return m.cookiesResult, nil
}

func stubLoginTargets(
	t *testing.T,
	targets []browser.CDPTarget,
	err error,
) {
	t.Helper()
	prev := listLoginCandidateTargets
	listLoginCandidateTargets = func(context.Context, string) ([]browser.CDPTarget, error) {
		return targets, err
	}
	t.Cleanup(func() {
		listLoginCandidateTargets = prev
	})
}

func TestXHSRPAClient_WaitLoginFlow_SavesCookies(t *testing.T) {
	stubLoginTargets(t, nil, nil)

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "xhs-cookies.json")
	client := NewXHSRPAClient(&XiaohongshuConfig{
		Enabled:            true,
		CookiePath:         cookiePath,
		RateLimitSeconds:   5,
		ErrorScreenshotDir: filepath.Join(tmpDir, "errors"),
	})

	mockTools := &loginMockPlaywrightTools{
		evaluateResult: json.RawMessage(`"https://creator.xiaohongshu.com/creator/home"`),
		cookiesResult: []map[string]any{
			{
				"name":    "a1",
				"value":   "cookie-a1",
				"domain":  ".xiaohongshu.com",
				"path":    "/",
				"expires": float64(time.Now().Add(2 * time.Hour).Unix()),
			},
			{
				"name":    "web_session",
				"value":   "cookie-session",
				"domain":  ".xiaohongshu.com",
				"path":    "/",
				"expires": float64(time.Now().Add(2 * time.Hour).Unix()),
			},
		},
	}
	client.SetBrowserFromPlaywright(mockTools, "ws://127.0.0.1:9222/devtools/browser/mock", "")
	client.loginSession = &LoginSessionState{
		AccountID:  "default",
		TargetID:   "target-1",
		LoginURL:   xhsLoginOpenURL,
		CookiePath: cookiePath,
		Status:     "waiting_login",
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	session, err := client.WaitLoginFlow(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("WaitLoginFlow: %v", err)
	}
	if session.Status != "authenticated" {
		t.Fatalf("status = %q, want authenticated", session.Status)
	}
	if session.CookieCount != 2 {
		t.Fatalf("cookieCount = %d, want 2", session.CookieCount)
	}
	if session.SavedAt == nil {
		t.Fatal("expected SavedAt to be populated")
	}
	if !client.CheckCookieValid() {
		t.Fatal("expected client cookies to be valid after login capture")
	}

	data, err := os.ReadFile(cookiePath)
	if err != nil {
		t.Fatalf("read cookie file: %v", err)
	}
	if !strings.Contains(string(data), "web_session") {
		t.Fatalf("cookie file missing expected session cookie: %s", string(data))
	}
}

func TestXHSRPAClient_WaitLoginFlow_TimeoutKeepsPendingState(t *testing.T) {
	stubLoginTargets(t, nil, nil)

	tmpDir := t.TempDir()
	client := NewXHSRPAClient(&XiaohongshuConfig{
		CookiePath:         filepath.Join(tmpDir, "xhs-cookies.json"),
		RateLimitSeconds:   5,
		ErrorScreenshotDir: filepath.Join(tmpDir, "errors"),
	})

	mockTools := &loginMockPlaywrightTools{
		evaluateResult: json.RawMessage(`"https://creator.xiaohongshu.com/login"`),
		cookiesResult:  nil,
	}
	client.SetBrowserFromPlaywright(mockTools, "ws://127.0.0.1:9222/devtools/browser/mock", "")
	client.loginSession = &LoginSessionState{
		AccountID:  "default",
		TargetID:   "target-2",
		LoginURL:   xhsLoginOpenURL,
		CookiePath: client.cfg.CookiePath,
		Status:     "waiting_login",
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	session, err := client.WaitLoginFlow(context.Background(), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitLoginFlow: %v", err)
	}
	if session.Status != "waiting_login" {
		t.Fatalf("status = %q, want waiting_login", session.Status)
	}
	if !strings.Contains(session.Message, "还未检测到有效登录态") {
		t.Fatalf("message = %q, want pending login hint", session.Message)
	}
}

func TestXHSRPAClient_WaitLoginFlow_AcceptsSessionCookieOnLoginPage(t *testing.T) {
	stubLoginTargets(t, nil, nil)

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "xhs-cookies.json")
	client := NewXHSRPAClient(&XiaohongshuConfig{
		Enabled:            true,
		CookiePath:         cookiePath,
		RateLimitSeconds:   5,
		ErrorScreenshotDir: filepath.Join(tmpDir, "errors"),
	})

	mockTools := &loginMockPlaywrightTools{
		evaluateResult: json.RawMessage(`"https://creator.xiaohongshu.com/login"`),
		cookiesResult: []map[string]any{
			{
				"name":    "a1",
				"value":   "cookie-a1",
				"domain":  ".xiaohongshu.com",
				"path":    "/",
				"expires": float64(time.Now().Add(2 * time.Hour).Unix()),
			},
			{
				"name":    "web_session",
				"value":   "cookie-session",
				"domain":  ".xiaohongshu.com",
				"path":    "/",
				"expires": float64(time.Now().Add(2 * time.Hour).Unix()),
			},
		},
	}
	client.SetBrowserFromPlaywright(mockTools, "ws://127.0.0.1:9222/devtools/browser/mock", "")
	client.loginSession = &LoginSessionState{
		AccountID:  "default",
		TargetID:   "target-login",
		LoginURL:   xhsLoginOpenURL,
		CookiePath: cookiePath,
		Status:     "waiting_login",
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	session, err := client.WaitLoginFlow(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("WaitLoginFlow: %v", err)
	}
	if session.Status != "authenticated" {
		t.Fatalf("status = %q, want authenticated", session.Status)
	}
	if session.CookieCount != 2 {
		t.Fatalf("cookieCount = %d, want 2", session.CookieCount)
	}
}

func TestXHSRPAClient_WaitLoginFlow_ScansOtherXiaohongshuTargets(t *testing.T) {
	stubLoginTargets(t, []browser.CDPTarget{
		{
			ID:   "target-original",
			Type: "page",
			URL:  "https://creator.xiaohongshu.com/login",
		},
		{
			ID:   "target-authenticated",
			Type: "page",
			URL:  "https://creator.xiaohongshu.com/creator/home",
		},
	}, nil)

	tmpDir := t.TempDir()
	cookiePath := filepath.Join(tmpDir, "xhs-cookies.json")
	client := NewXHSRPAClient(&XiaohongshuConfig{
		Enabled:            true,
		CookiePath:         cookiePath,
		RateLimitSeconds:   5,
		ErrorScreenshotDir: filepath.Join(tmpDir, "errors"),
	})

	mockTools := &loginMockPlaywrightTools{
		targetURLs: map[string]json.RawMessage{
			"target-original":      json.RawMessage(`"https://creator.xiaohongshu.com/login"`),
			"target-authenticated": json.RawMessage(`"https://creator.xiaohongshu.com/creator/home"`),
		},
		targetCookies: map[string][]map[string]any{
			"target-original": nil,
			"target-authenticated": {
				{
					"name":    "a1",
					"value":   "cookie-a1",
					"domain":  ".xiaohongshu.com",
					"path":    "/",
					"expires": float64(time.Now().Add(2 * time.Hour).Unix()),
				},
				{
					"name":    "web_session",
					"value":   "cookie-session",
					"domain":  ".xiaohongshu.com",
					"path":    "/",
					"expires": float64(time.Now().Add(2 * time.Hour).Unix()),
				},
			},
		},
	}
	client.SetBrowserFromPlaywright(mockTools, "ws://127.0.0.1:9222/devtools/browser/mock", "")
	client.loginSession = &LoginSessionState{
		AccountID:  "default",
		TargetID:   "target-original",
		LoginURL:   xhsLoginOpenURL,
		CookiePath: cookiePath,
		Status:     "waiting_login",
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	session, err := client.WaitLoginFlow(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("WaitLoginFlow: %v", err)
	}
	if session.Status != "authenticated" {
		t.Fatalf("status = %q, want authenticated", session.Status)
	}
	if session.TargetID != "target-authenticated" {
		t.Fatalf("targetID = %q, want target-authenticated", session.TargetID)
	}
}
