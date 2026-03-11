package gateway

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- Auth 类型定义 ----------

// AuthMode 网关认证模式。
type AuthMode string

const (
	AuthModeToken    AuthMode = "token"
	AuthModePassword AuthMode = "password"
)

// ResolvedGatewayAuth 解析后的网关认证配置。
type ResolvedGatewayAuth struct {
	Mode           AuthMode
	Token          string
	Password       string
	AllowTailscale bool
}

// GatewayAuthResult 认证结果。
type GatewayAuthResult struct {
	OK     bool   `json:"ok"`
	Method string `json:"method,omitempty"` // "token" | "password" | "tailscale"
	User   string `json:"user,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ConnectAuth 客户端提供的认证凭据。
type ConnectAuth struct {
	Token    string
	Password string
}

// GatewayAuthConfig 网关认证配置（对应 config 层）。
type GatewayAuthConfig struct {
	Mode           AuthMode `json:"mode,omitempty"`
	Token          string   `json:"token,omitempty"`
	Password       string   `json:"password,omitempty"`
	AllowTailscale *bool    `json:"allowTailscale,omitempty"`
}

// TailscaleUser Tailscale 用户信息。
type TailscaleUser struct {
	Login      string `json:"login"`
	Name       string `json:"name"`
	ProfilePic string `json:"profilePic,omitempty"`
}

// TailscaleWhoisLookup Tailscale whois 查询函数签名。
type TailscaleWhoisLookup func(ip string) (*TailscaleUser, error)

// ---------- 恒定时间比较 ----------

// SafeEqual 恒定时间字符串比较，防止时序攻击。
// 始终执行 ConstantTimeCompare，避免通过响应时间泄漏 token 长度。
func SafeEqual(a, b string) bool {
	// 长度比较也使用恒定时间
	lenMatch := subtle.ConstantTimeEq(int32(len(a)), int32(len(b)))
	// 始终对齐到较长的长度后比较，避免长度侧信道
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return lenMatch == 1
	}
	// 将两个字符串 pad 到相同长度
	padA := make([]byte, maxLen)
	padB := make([]byte, maxLen)
	copy(padA, []byte(a))
	copy(padB, []byte(b))
	return subtle.ConstantTimeCompare(padA, padB) == 1 && lenMatch == 1
}

// ---------- 认证配置解析 ----------

// ResolveGatewayAuth 从配置和环境变量解析网关认证。
func ResolveGatewayAuth(authConfig *GatewayAuthConfig, tailscaleMode string) ResolvedGatewayAuth {
	cfg := GatewayAuthConfig{}
	if authConfig != nil {
		cfg = *authConfig
	}

	token := cfg.Token
	if token == "" {
		token = preferredGatewayEnvValue("CRABCLAW_GATEWAY_TOKEN", "OPENACOSMI_GATEWAY_TOKEN")
	}
	if token == "" {
		token = os.Getenv("CLAWDBOT_GATEWAY_TOKEN")
	}
	if token == "" {
		token = ReadOrGenerateGatewayToken()
	}

	password := cfg.Password
	if password == "" {
		password = preferredGatewayEnvValue("CRABCLAW_GATEWAY_PASSWORD", "OPENACOSMI_GATEWAY_PASSWORD")
	}
	if password == "" {
		password = os.Getenv("CLAWDBOT_GATEWAY_PASSWORD")
	}

	mode := cfg.Mode
	if mode == "" {
		if password != "" {
			mode = AuthModePassword
		} else {
			mode = AuthModeToken
		}
	}

	allowTailscale := false
	if cfg.AllowTailscale != nil {
		allowTailscale = *cfg.AllowTailscale
	} else {
		allowTailscale = tailscaleMode == "serve" && mode != AuthModePassword
	}

	return ResolvedGatewayAuth{
		Mode:           mode,
		Token:          token,
		Password:       password,
		AllowTailscale: allowTailscale,
	}
}

// ---------- 自动 Token 生成（VS Code Server / Jupyter 模式） ----------

// gatewayTokenFilePath 返回持久化 token 文件路径: ~/.openacosmi/gateway-token
func gatewayTokenFilePath() string {
	return filepath.Join(config.ResolveStateDir(), "gateway-token")
}

// ReadOrGenerateGatewayToken 读取或自动生成网关 token。
// 优先级: 磁盘文件 → 自动生成 + 持久化。
// 采用 VS Code Server / Jupyter Notebook 模式:
//   - 首次启动: crypto/rand 生成 32 字节 hex token → 写入 ~/.openacosmi/gateway-token (0600)
//   - 后续启动: 直接读取文件
func ReadOrGenerateGatewayToken() string {
	tokenFile := gatewayTokenFilePath()
	if tokenFile == "" {
		return generateRandomToken()
	}

	// 1. 尝试读取已有 token
	if data, err := os.ReadFile(tokenFile); err == nil {
		token := strings.TrimSpace(string(data))
		if len(token) >= 8 {
			return token
		}
	}

	// 2. 自动生成新 token
	token := generateRandomToken()

	// 3. 持久化到磁盘 (目录可能不存在)
	dir := filepath.Dir(tokenFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		slog.Warn("gateway: cannot create token dir", "dir", dir, "error", err)
		return token
	}
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0600); err != nil {
		slog.Warn("gateway: cannot persist token", "file", tokenFile, "error", err)
	} else {
		slog.Info("🔑 gateway: auto-generated token", "file", tokenFile)
	}

	return token
}

// generateRandomToken 生成 32 字节的加密安全随机 hex token。
func generateRandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// fallback: 不应发生，但避免 panic
		return fmt.Sprintf("fallback-%d", os.Getpid())
	}
	return hex.EncodeToString(b)
}

// AssertGatewayAuthConfigured 校验认证配置是否完整，缺失则 panic。
func AssertGatewayAuthConfigured(auth ResolvedGatewayAuth) error {
	if auth.Mode == AuthModeToken && auth.Token == "" {
		if auth.AllowTailscale {
			return nil
		}
		return &AuthConfigError{
			Message: "gateway auth mode is token, but no token was configured (set gateway.auth.token or CRABCLAW_GATEWAY_TOKEN / OPENACOSMI_GATEWAY_TOKEN)",
		}
	}
	if auth.Mode == AuthModePassword && auth.Password == "" {
		return &AuthConfigError{
			Message: "gateway auth mode is password, but no password was configured",
		}
	}
	return nil
}

// AuthConfigError 认证配置错误。
type AuthConfigError struct {
	Message string
}

func (e *AuthConfigError) Error() string {
	return e.Message
}

// ---------- 本地请求检测 ----------

// IsLocalDirectRequest 判断请求是否为本地直连（非代理转发）。
func IsLocalDirectRequest(r *http.Request, trustedProxies []string) bool {
	if r == nil {
		return false
	}
	clientIP := ResolveGatewayClientIP(
		r.RemoteAddr,
		r.Header.Get("X-Forwarded-For"),
		r.Header.Get("X-Real-IP"),
		trustedProxies,
	)
	if !IsLoopbackAddress(clientIP) {
		return false
	}

	host := GetHostName(r.Host)
	hostIsLocal := host == "localhost" || host == "127.0.0.1" || host == "::1"
	hostIsTailscaleServe := strings.HasSuffix(host, ".ts.net")

	hasForwarded := r.Header.Get("X-Forwarded-For") != "" ||
		r.Header.Get("X-Real-IP") != "" ||
		r.Header.Get("X-Forwarded-Host") != ""

	remoteIP := StripOptionalPort(r.RemoteAddr)
	remoteIsTrustedProxy := IsTrustedProxyAddress(remoteIP, trustedProxies)

	return (hostIsLocal || hostIsTailscaleServe) && (!hasForwarded || remoteIsTrustedProxy)
}

// ---------- Tailscale 认证 ----------

func getTailscaleUser(r *http.Request) *TailscaleUser {
	if r == nil {
		return nil
	}
	login := strings.TrimSpace(r.Header.Get("Tailscale-User-Login"))
	if login == "" {
		return nil
	}
	name := strings.TrimSpace(r.Header.Get("Tailscale-User-Name"))
	if name == "" {
		name = login
	}
	profilePic := strings.TrimSpace(r.Header.Get("Tailscale-User-Profile-Pic"))
	return &TailscaleUser{Login: login, Name: name, ProfilePic: profilePic}
}

func hasTailscaleProxyHeaders(r *http.Request) bool {
	if r == nil {
		return false
	}
	return r.Header.Get("X-Forwarded-For") != "" &&
		r.Header.Get("X-Forwarded-Proto") != "" &&
		r.Header.Get("X-Forwarded-Host") != ""
}

func isTailscaleProxyRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	remoteIP := StripOptionalPort(r.RemoteAddr)
	return IsLoopbackAddress(remoteIP) && hasTailscaleProxyHeaders(r)
}

// ---------- 核心授权函数 ----------

// AuthorizeGatewayConnect 执行网关连接授权。
func AuthorizeGatewayConnect(params AuthorizeParams) GatewayAuthResult {
	auth := params.Auth
	localDirect := IsLocalDirectRequest(params.Req, params.TrustedProxies)

	// Tailscale 认证路径
	if auth.AllowTailscale && !localDirect && params.Req != nil && params.TailscaleWhois != nil {
		tsResult := resolveVerifiedTailscaleUser(params.Req, params.TailscaleWhois)
		if tsResult.OK {
			return GatewayAuthResult{OK: true, Method: "tailscale", User: tsResult.User}
		}
	}

	// Token 认证
	if auth.Mode == AuthModeToken {
		// localhost 直连免认证 — 与 OpenAcosmi 原版开发体验一致
		// 当请求来自回环地址(127.0.0.1/::1) + host 为 localhost + 无代理转发头时放行
		if localDirect {
			return GatewayAuthResult{OK: true, Method: "local"}
		}
		if auth.Token == "" {
			return GatewayAuthResult{OK: false, Reason: "token_missing_config"}
		}
		if params.ConnectAuth == nil || params.ConnectAuth.Token == "" {
			return GatewayAuthResult{OK: false, Reason: "token_missing"}
		}
		if !SafeEqual(params.ConnectAuth.Token, auth.Token) {
			return GatewayAuthResult{OK: false, Reason: "token_mismatch"}
		}
		return GatewayAuthResult{OK: true, Method: "token"}
	}

	// Password 认证
	if auth.Mode == AuthModePassword {
		if auth.Password == "" {
			return GatewayAuthResult{OK: false, Reason: "password_missing_config"}
		}
		if params.ConnectAuth == nil || params.ConnectAuth.Password == "" {
			return GatewayAuthResult{OK: false, Reason: "password_missing"}
		}
		if !SafeEqual(params.ConnectAuth.Password, auth.Password) {
			return GatewayAuthResult{OK: false, Reason: "password_mismatch"}
		}
		return GatewayAuthResult{OK: true, Method: "password"}
	}

	return GatewayAuthResult{OK: false, Reason: "unauthorized"}
}

// AuthorizeParams 授权参数。
type AuthorizeParams struct {
	Auth           ResolvedGatewayAuth
	ConnectAuth    *ConnectAuth
	Req            *http.Request
	TrustedProxies []string
	TailscaleWhois TailscaleWhoisLookup
}

func resolveVerifiedTailscaleUser(r *http.Request, whoisLookup TailscaleWhoisLookup) GatewayAuthResult {
	tsUser := getTailscaleUser(r)
	if tsUser == nil {
		return GatewayAuthResult{OK: false, Reason: "tailscale_user_missing"}
	}
	if !isTailscaleProxyRequest(r) {
		return GatewayAuthResult{OK: false, Reason: "tailscale_proxy_missing"}
	}
	clientIP := ParseForwardedForClientIP(r.Header.Get("X-Forwarded-For"))
	if clientIP == "" {
		return GatewayAuthResult{OK: false, Reason: "tailscale_whois_failed"}
	}
	whois, err := whoisLookup(clientIP)
	if err != nil || whois == nil || whois.Login == "" {
		return GatewayAuthResult{OK: false, Reason: "tailscale_whois_failed"}
	}
	if !strings.EqualFold(strings.TrimSpace(whois.Login), strings.TrimSpace(tsUser.Login)) {
		return GatewayAuthResult{OK: false, Reason: "tailscale_user_mismatch"}
	}
	name := whois.Name
	if name == "" {
		name = tsUser.Name
	}
	return GatewayAuthResult{OK: true, Method: "tailscale", User: whois.Login}
}
