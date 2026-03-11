package gateway

import (
	"net/http/httptest"
	"testing"
)

func TestSafeEqual(t *testing.T) {
	if !SafeEqual("secret123", "secret123") {
		t.Error("same strings should be equal")
	}
	if SafeEqual("secret123", "secret124") {
		t.Error("different strings should not be equal")
	}
	if SafeEqual("short", "longer-string") {
		t.Error("different length strings should not be equal")
	}
	if SafeEqual("", "notempty") {
		t.Error("empty vs non-empty should not be equal")
	}
	if !SafeEqual("", "") {
		t.Error("two empty strings should be equal")
	}
}

func TestResolveGatewayAuth_Token(t *testing.T) {
	cfg := &GatewayAuthConfig{Token: "my-token"}
	auth := ResolveGatewayAuth(cfg, "")
	if auth.Mode != AuthModeToken {
		t.Errorf("mode = %q, want token", auth.Mode)
	}
	if auth.Token != "my-token" {
		t.Errorf("token = %q, want my-token", auth.Token)
	}
}

func TestResolveGatewayAuth_Password(t *testing.T) {
	cfg := &GatewayAuthConfig{Password: "pass123"}
	auth := ResolveGatewayAuth(cfg, "")
	if auth.Mode != AuthModePassword {
		t.Errorf("mode = %q, want password", auth.Mode)
	}
}

func TestResolveGatewayAuth_TailscaleServe(t *testing.T) {
	cfg := &GatewayAuthConfig{Token: "tok"}
	auth := ResolveGatewayAuth(cfg, "serve")
	if !auth.AllowTailscale {
		t.Error("tailscaleMode=serve should enable allowTailscale")
	}
}

func TestAssertGatewayAuthConfigured(t *testing.T) {
	// Token mode without token → error
	err := AssertGatewayAuthConfigured(ResolvedGatewayAuth{Mode: AuthModeToken})
	if err == nil {
		t.Error("expected error for token mode without token")
	}
	// Token mode without token but tailscale allowed → OK
	err = AssertGatewayAuthConfigured(ResolvedGatewayAuth{Mode: AuthModeToken, AllowTailscale: true})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Token mode with token → OK
	err = AssertGatewayAuthConfigured(ResolvedGatewayAuth{Mode: AuthModeToken, Token: "tk"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Password mode without password → error
	err = AssertGatewayAuthConfigured(ResolvedGatewayAuth{Mode: AuthModePassword})
	if err == nil {
		t.Error("expected error for password mode without password")
	}
}

func TestAuthorizeGatewayConnect_Token(t *testing.T) {
	auth := ResolvedGatewayAuth{Mode: AuthModeToken, Token: "secret"}
	// 正确 token
	result := AuthorizeGatewayConnect(AuthorizeParams{
		Auth:        auth,
		ConnectAuth: &ConnectAuth{Token: "secret"},
	})
	if !result.OK {
		t.Errorf("should authorize, reason=%s", result.Reason)
	}
	if result.Method != "token" {
		t.Errorf("method = %q, want token", result.Method)
	}
	// 错误 token
	result = AuthorizeGatewayConnect(AuthorizeParams{
		Auth:        auth,
		ConnectAuth: &ConnectAuth{Token: "wrong"},
	})
	if result.OK {
		t.Error("should reject wrong token")
	}
	if result.Reason != "token_mismatch" {
		t.Errorf("reason = %q, want token_mismatch", result.Reason)
	}
	// 缺失 token
	result = AuthorizeGatewayConnect(AuthorizeParams{
		Auth: auth,
	})
	if result.OK {
		t.Error("should reject missing token")
	}
}

func TestAuthorizeGatewayConnect_Password(t *testing.T) {
	auth := ResolvedGatewayAuth{Mode: AuthModePassword, Password: "pass123"}
	result := AuthorizeGatewayConnect(AuthorizeParams{
		Auth:        auth,
		ConnectAuth: &ConnectAuth{Password: "pass123"},
	})
	if !result.OK {
		t.Errorf("should authorize, reason=%s", result.Reason)
	}
	if result.Method != "password" {
		t.Errorf("method = %q, want password", result.Method)
	}
}

func TestIsLocalDirectRequest(t *testing.T) {
	// 本地直连请求
	req := httptest.NewRequest("GET", "http://localhost/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if !IsLocalDirectRequest(req, nil) {
		t.Error("127.0.0.1 → localhost should be local direct")
	}
	// 非本地地址
	req = httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	if IsLocalDirectRequest(req, nil) {
		t.Error("external IP should not be local direct")
	}
	// 带 X-Forwarded-For 但 remote 不在代理列表
	req = httptest.NewRequest("GET", "http://localhost/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.5")
	if IsLocalDirectRequest(req, nil) {
		t.Error("forwarded request without trusted proxy should not be local direct")
	}
}

func TestAuthorizeGatewayConnect_LocalDirect(t *testing.T) {
	auth := ResolvedGatewayAuth{Mode: AuthModeToken, Token: "secret"}

	// localhost 直连 + 无 token → 应放行
	req := httptest.NewRequest("GET", "http://localhost/ws", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	result := AuthorizeGatewayConnect(AuthorizeParams{
		Auth: auth,
		Req:  req,
	})
	if !result.OK {
		t.Errorf("localhost direct should be authorized, reason=%s", result.Reason)
	}
	if result.Method != "local" {
		t.Errorf("method should be 'local', got %q", result.Method)
	}

	// 非 localhost → 仍需 token
	reqRemote := httptest.NewRequest("GET", "http://example.com/ws", nil)
	reqRemote.RemoteAddr = "192.168.1.100:12345"
	result2 := AuthorizeGatewayConnect(AuthorizeParams{
		Auth: auth,
		Req:  reqRemote,
	})
	if result2.OK {
		t.Error("remote request without token should be rejected")
	}
	if result2.Reason != "token_missing" {
		t.Errorf("reason should be 'token_missing', got %q", result2.Reason)
	}
}
