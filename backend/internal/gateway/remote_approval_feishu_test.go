package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

type feishuRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *feishuRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = t.target.Scheme
	cloned.URL.Host = t.target.Host
	return t.base.RoundTrip(cloned)
}

func TestFeishuBroadcastCard_FallbackToOpenIDWhenChatTargetsFail(t *testing.T) {
	var chatCalls atomic.Int32
	var userCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		targetType := r.URL.Query().Get("receive_id_type")
		switch targetType {
		case "chat_id":
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":999,"msg":"chat failed"}`))
		case "open_id":
			userCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		default:
			_, _ = w.Write([]byte(`{"code":998,"msg":"unknown target"}`))
		}
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{
			ChatID: "oc_chat_1",
			UserID: "ou_user_1",
		},
		client: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &feishuRewriteTransport{
				target: targetURL,
				base:   http.DefaultTransport,
			},
		},
	}

	card := map[string]interface{}{
		"header": map[string]interface{}{"title": "test"},
	}
	if err := provider.broadcastCard(context.Background(), "dummy-token", card); err != nil {
		t.Fatalf("broadcast card should succeed via open_id fallback, got: %v", err)
	}

	if chatCalls.Load() != 1 {
		t.Fatalf("expected 1 chat send attempt, got %d", chatCalls.Load())
	}
	if userCalls.Load() != 1 {
		t.Fatalf("expected 1 open_id fallback send attempt, got %d", userCalls.Load())
	}
}

func TestFeishuBroadcastCard_NoOpenIDWhenChatHasSuccess(t *testing.T) {
	var chatCalls atomic.Int32
	var userCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		targetType := r.URL.Query().Get("receive_id_type")
		switch targetType {
		case "chat_id":
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		case "open_id":
			userCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		default:
			_, _ = w.Write([]byte(`{"code":998,"msg":"unknown target"}`))
		}
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{
			ChatID: "oc_chat_1",
			UserID: "ou_user_1",
		},
		client: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &feishuRewriteTransport{
				target: targetURL,
				base:   http.DefaultTransport,
			},
		},
	}

	card := map[string]interface{}{
		"header": map[string]interface{}{"title": "test"},
	}
	if err := provider.broadcastCard(context.Background(), "dummy-token", card); err != nil {
		t.Fatalf("broadcast card should succeed on chat target, got: %v", err)
	}

	if chatCalls.Load() != 1 {
		t.Fatalf("expected 1 chat send attempt, got %d", chatCalls.Load())
	}
	if userCalls.Load() != 0 {
		t.Fatalf("expected no open_id fallback when chat succeeds, got %d", userCalls.Load())
	}
}

func TestParseFeishuConfig_ParsesFallbackFields(t *testing.T) {
	raw := map[string]interface{}{
		"enabled":         true,
		"appId":           "cli_test",
		"appSecret":       "secret",
		"chatId":          "oc_chat",
		"userId":          "ou_user",
		"approvalChatId":  "oc_approval",
		"lastKnownChatId": "oc_last",
		"lastKnownUserId": "ou_last",
	}

	cfg := parseFeishuConfig(raw)
	if cfg == nil {
		t.Fatal("expected parsed feishu config")
	}
	if !cfg.Enabled || cfg.AppID != "cli_test" || cfg.AppSecret != "secret" {
		t.Fatalf("basic fields parse mismatch: %+v", cfg)
	}
	if cfg.ApprovalChatID != "oc_approval" {
		t.Fatalf("approvalChatId parse mismatch: %q", cfg.ApprovalChatID)
	}
	if cfg.LastKnownChatID != "oc_last" {
		t.Fatalf("lastKnownChatId parse mismatch: %q", cfg.LastKnownChatID)
	}
	if cfg.LastKnownUserID != "ou_last" {
		t.Fatalf("lastKnownUserId parse mismatch: %q", cfg.LastKnownUserID)
	}
}

func TestFeishuBroadcastCard_RequestBodyStillValidJSON(t *testing.T) {
	var decodeErr atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			decodeErr.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	provider := &feishuProvider{
		config: &FeishuProviderConfig{UserID: "ou_user_1"},
		client: &http.Client{Transport: &feishuRewriteTransport{target: targetURL, base: http.DefaultTransport}},
	}
	if err := provider.broadcastCard(context.Background(), "dummy-token", map[string]interface{}{"k": "v"}); err != nil {
		t.Fatalf("broadcast card should succeed: %v", err)
	}
	if decodeErr.Load() {
		t.Fatal("expected valid JSON request body")
	}
}
