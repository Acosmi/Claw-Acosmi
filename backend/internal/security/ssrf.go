package security

// TS 对照: infra/net/ssrf.ts (309L)
// SSRF 防护 — 私有 IP 检测、阻止主机名检查、安全 HTTP 请求。

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ---------- 错误类型 ----------

// SsrfBlockedError SSRF 拦截错误。
type SsrfBlockedError struct {
	Message string
}

func (e *SsrfBlockedError) Error() string {
	return e.Message
}

// ---------- 策略 ----------

// SsrfPolicy SSRF 防护策略。
// TS 对照: ssrf.ts L20-23
type SsrfPolicy struct {
	AllowPrivateNetwork bool
	AllowedHostnames    []string
}

// ---------- 常量 ----------

// blockedHostnames 始终阻止的主机名集合。
// TS 对照: ssrf.ts L26
var blockedHostnames = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true,
}

// privateIPv6Prefixes IPv6 私有地址前缀。
// TS 对照: ssrf.ts L25
var privateIPv6Prefixes = []string{"fe80:", "fec0:", "fc", "fd"}

// defaultSafeFetchTimeout 安全请求默认超时。
const defaultSafeFetchTimeout = 30 * time.Second

// ---------- 公开函数 ----------

// IsPrivateIP 判断 IP 地址是否为私有/内部地址。
// 覆盖: IPv4 私有段 + IPv6 链路本地/ULA + IPv4-mapped-in-IPv6 + loopback。
// TS 对照: ssrf.ts isPrivateIpAddress (L112-141)
func IsPrivateIP(address string) bool {
	normalized := strings.TrimSpace(strings.ToLower(address))
	// 去除 IPv6 bracket 包裹
	if strings.HasPrefix(normalized, "[") && strings.HasSuffix(normalized, "]") {
		normalized = normalized[1 : len(normalized)-1]
	}
	if normalized == "" {
		return false
	}

	// IPv4-mapped IPv6: ::ffff:x.x.x.x
	if strings.HasPrefix(normalized, "::ffff:") {
		mapped := normalized[len("::ffff:"):]
		if ip := net.ParseIP(mapped); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				return isPrivateIPv4(v4)
			}
		}
	}

	// IPv6
	if strings.Contains(normalized, ":") {
		if normalized == "::" || normalized == "::1" {
			return true
		}
		for _, prefix := range privateIPv6Prefixes {
			if strings.HasPrefix(normalized, prefix) {
				return true
			}
		}
		return false
	}

	// IPv4
	ip := net.ParseIP(normalized)
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return isPrivateIPv4(v4)
}

// IsBlockedHostname 判断主机名是否被阻止。
// TS 对照: ssrf.ts isBlockedHostname (L143-156)
func IsBlockedHostname(hostname string) bool {
	normalized := normalizeHostname(hostname)
	if normalized == "" {
		return false
	}
	if blockedHostnames[normalized] {
		return true
	}
	return strings.HasSuffix(normalized, ".localhost") ||
		strings.HasSuffix(normalized, ".local") ||
		strings.HasSuffix(normalized, ".internal")
}

// SafeFetchURL 执行带 SSRF 防护的 HTTP GET 请求。
// 检查流程: URL 解析 → 主机名检查 → DNS 解析 → IP 检查 → 发送请求。
func SafeFetchURL(rawURL string, policy *SsrfPolicy) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("ssrf: 无效 URL: %w", err)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, &SsrfBlockedError{Message: "ssrf: 空主机名"}
	}

	allowPrivate := policy != nil && policy.AllowPrivateNetwork
	isExplicitAllowed := policy != nil && isHostnameAllowed(hostname, policy.AllowedHostnames)

	if !allowPrivate && !isExplicitAllowed {
		// 步骤 1: 主机名黑名单
		if IsBlockedHostname(hostname) {
			return nil, &SsrfBlockedError{Message: fmt.Sprintf("ssrf: 阻止主机名: %s", hostname)}
		}
		// 步骤 2: 直接 IP 输入检查
		if IsPrivateIP(hostname) {
			return nil, &SsrfBlockedError{Message: "ssrf: 阻止私有/内部 IP 地址"}
		}
	}

	// 步骤 3: DNS 解析后检查
	if !allowPrivate && !isExplicitAllowed {
		addrs, err := net.DefaultResolver.LookupHost(context.Background(), hostname)
		if err != nil {
			// 仅当输入是直接 IP 时放行（ParseIP 不为 nil），否则阻止
			if net.ParseIP(hostname) == nil {
				return nil, &SsrfBlockedError{
					Message: fmt.Sprintf("ssrf: DNS 解析失败且非直接 IP: %s: %v", hostname, err),
				}
			}
		} else {
			for _, addr := range addrs {
				if IsPrivateIP(addr) {
					return nil, &SsrfBlockedError{
						Message: fmt.Sprintf("ssrf: 主机名 %s 解析到私有 IP: %s", hostname, addr),
					}
				}
			}
		}
	}

	// 步骤 4: 使用策略感知的 DNS-pinning 客户端发送请求
	// 统一防止重定向时的 DNS rebinding 攻击，重定向次数限制为 3 次。
	// 当 AllowPrivateNetwork 或 AllowedHostnames 放行时，跳过 DialContext/CheckRedirect 中的私有 IP 检查。
	client := createPinnedHTTPClientWithPolicy(defaultSafeFetchTimeout, allowPrivate, policyAllowedHostnames(policy))

	resp, err := client.Get(rawURL) //nolint:gosec // URL 已通过 SSRF 检查
	if err != nil {
		return nil, fmt.Errorf("ssrf: 请求失败: %w", err)
	}
	return resp, nil
}

// CreatePinnedHTTPClient 创建带 SSRF 防护的 HTTP 客户端。
// 对每次 DNS 解析结果和每次重定向目标都执行 IP 检查，防止 DNS rebinding 攻击。
// TS 对照: ssrf.ts createPinnedDispatcher (L277-283)
func CreatePinnedHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("ssrf: invalid address %s: %w", addr, err)
			}
			ips, err := net.DefaultResolver.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("ssrf: DNS lookup failed for %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("ssrf: no addresses resolved for %s", host)
			}
			for _, ip := range ips {
				if IsPrivateIP(ip) {
					return nil, fmt.Errorf("ssrf: blocked private IP %s for host %s", ip, host)
				}
			}
			// 使用第一个已验证安全的 IP 发起连接
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("ssrf: too many redirects")
			}
			host := req.URL.Hostname()
			if IsBlockedHostname(host) {
				return fmt.Errorf("ssrf: redirect blocked to hostname %s", host)
			}
			if IsPrivateIP(host) {
				return fmt.Errorf("ssrf: redirect blocked to private IP %s", host)
			}
			ips, err := net.DefaultResolver.LookupHost(req.Context(), host)
			if err != nil {
				return fmt.Errorf("ssrf: redirect DNS lookup failed for %s: %w", host, err)
			}
			for _, ip := range ips {
				if IsPrivateIP(ip) {
					return fmt.Errorf("ssrf: redirect blocked to private IP %s", ip)
				}
			}
			return nil
		},
	}
}

// createPinnedHTTPClientWithPolicy 创建策略感知的 DNS-pinning HTTP 客户端。
// 当 allowPrivate 或 allowedHostnames 匹配时，跳过 DialContext/CheckRedirect 中的私有 IP 检查。
func createPinnedHTTPClientWithPolicy(timeout time.Duration, allowPrivate bool, allowedHostnames []string) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("ssrf: invalid address %s: %w", addr, err)
			}
			skipIPCheck := allowPrivate || isHostnameAllowed(host, allowedHostnames)
			ips, err := net.DefaultResolver.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("ssrf: DNS lookup failed for %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("ssrf: no addresses resolved for %s", host)
			}
			if !skipIPCheck {
				for _, ip := range ips {
					if IsPrivateIP(ip) {
						return nil, fmt.Errorf("ssrf: blocked private IP %s for host %s", ip, host)
					}
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("ssrf: too many redirects")
			}
			host := req.URL.Hostname()
			skipCheck := allowPrivate || isHostnameAllowed(host, allowedHostnames)
			if !skipCheck {
				if IsBlockedHostname(host) {
					return fmt.Errorf("ssrf: redirect blocked to hostname %s", host)
				}
				if IsPrivateIP(host) {
					return fmt.Errorf("ssrf: redirect blocked to private IP %s", host)
				}
				ips, err := net.DefaultResolver.LookupHost(req.Context(), host)
				if err != nil {
					return fmt.Errorf("ssrf: redirect DNS lookup failed for %s: %w", host, err)
				}
				for _, ip := range ips {
					if IsPrivateIP(ip) {
						return fmt.Errorf("ssrf: redirect blocked to private IP %s", ip)
					}
				}
			}
			return nil
		},
	}
}

// policyAllowedHostnames 从策略中提取白名单主机名列表。
func policyAllowedHostnames(policy *SsrfPolicy) []string {
	if policy == nil {
		return nil
	}
	return policy.AllowedHostnames
}

// ---------- 内部函数 ----------

// isPrivateIPv4 判断 IPv4 是否为私有地址。
// TS 对照: ssrf.ts isPrivateIpv4 (L86-110)
func isPrivateIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	o1, o2 := v4[0], v4[1]
	if o1 == 0 { // 0.0.0.0/8
		return true
	}
	if o1 == 10 { // 10.0.0.0/8
		return true
	}
	if o1 == 127 { // 127.0.0.0/8
		return true
	}
	if o1 == 169 && o2 == 254 { // 169.254.0.0/16
		return true
	}
	if o1 == 172 && o2 >= 16 && o2 <= 31 { // 172.16.0.0/12
		return true
	}
	if o1 == 192 && o2 == 168 { // 192.168.0.0/16
		return true
	}
	if o1 == 100 && o2 >= 64 && o2 <= 127 { // 100.64.0.0/10 (CGNAT)
		return true
	}
	return false
}

// normalizeHostname 规范化主机名。
// TS 对照: ssrf.ts normalizeHostname (L28-34)
func normalizeHostname(hostname string) string {
	normalized := strings.TrimSpace(strings.ToLower(hostname))
	normalized = strings.TrimSuffix(normalized, ".")
	if strings.HasPrefix(normalized, "[") && strings.HasSuffix(normalized, "]") {
		normalized = normalized[1 : len(normalized)-1]
	}
	return normalized
}

// isHostnameAllowed 检查主机名是否在白名单中。
func isHostnameAllowed(hostname string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	normalized := normalizeHostname(hostname)
	for _, h := range allowed {
		if normalizeHostname(h) == normalized {
			return true
		}
	}
	return false
}
