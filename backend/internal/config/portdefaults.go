package config

// 端口默认值 — 对应 src/config/port-defaults.ts (44 行)
//
// 从 gateway 端口派生 bridge、browser control、canvas、CDP 端口。
// 确保端口号在 1-65535 范围内。

// 端口默认值常量
const (
	DefaultBridgePort               = 18790
	DefaultBrowserControlPort       = 18791
	DefaultCanvasHostPort           = 18793
	DefaultBrowserCDPPortRangeStart = 18800
	DefaultBrowserCDPPortRangeEnd   = 18899
)

// PortRange 端口范围
type PortRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// isValidPort 检查端口是否有效
func isValidPort(port int) bool {
	return port > 0 && port <= 65535
}

// clampPort 限制端口到有效范围，无效则使用 fallback
func clampPort(port, fallback int) int {
	if isValidPort(port) {
		return port
	}
	return fallback
}

// derivePort 从基础端口加偏移量派生端口。
// 当 base <= 0 时直接返回 fallback，避免从无效端口派生出特权端口。
func derivePort(base, offset, fallback int) int {
	if base <= 0 {
		return fallback
	}
	return clampPort(base+offset, fallback)
}

// DeriveDefaultBridgePort 从 gateway 端口派生 bridge 端口。
// 对应 TS: deriveDefaultBridgePort(gatewayPort)
func DeriveDefaultBridgePort(gatewayPort int) int {
	return derivePort(gatewayPort, 1, DefaultBridgePort)
}

// DeriveDefaultBrowserControlPort 从 gateway 端口派生 browser control 端口。
// 对应 TS: deriveDefaultBrowserControlPort(gatewayPort)
func DeriveDefaultBrowserControlPort(gatewayPort int) int {
	return derivePort(gatewayPort, 2, DefaultBrowserControlPort)
}

// DeriveDefaultCanvasHostPort 从 gateway 端口派生 canvas host 端口。
// 对应 TS: deriveDefaultCanvasHostPort(gatewayPort)
func DeriveDefaultCanvasHostPort(gatewayPort int) int {
	return derivePort(gatewayPort, 4, DefaultCanvasHostPort)
}

// DeriveDefaultBrowserCDPPortRange 从 browser control 端口派生 CDP 端口范围。
// 对应 TS: deriveDefaultBrowserCdpPortRange(browserControlPort)
func DeriveDefaultBrowserCDPPortRange(browserControlPort int) PortRange {
	start := derivePort(browserControlPort, 9, DefaultBrowserCDPPortRangeStart)
	end := clampPort(
		start+(DefaultBrowserCDPPortRangeEnd-DefaultBrowserCDPPortRangeStart),
		DefaultBrowserCDPPortRangeEnd,
	)
	if end < start {
		return PortRange{Start: start, End: start}
	}
	return PortRange{Start: start, End: end}
}
