// Package browser provides browser automation via Chrome DevTools Protocol.
// It handles Chrome lifecycle management, CDP communication, extension relay,
// and browser session management.
package browser

import "github.com/Acosmi/ClawAcosmi/internal/config"

// 端口常量通过 Gateway 端口推导，与 config/portdefaults.go 保持一致。
// 不再硬编码独立常量，避免与推导链冲突。
var (
	// CDPPortRangeStart is the start of the CDP port range.
	CDPPortRangeStart = config.DefaultBrowserCDPPortRangeStart
	// CDPPortRangeEnd is the end of the CDP port range.
	CDPPortRangeEnd = config.DefaultBrowserCDPPortRangeEnd
)

// ResolveBrowserControlPort returns the browser control port derived from gateway port.
func ResolveBrowserControlPort() int {
	return config.DeriveDefaultBrowserControlPort(config.ResolveGatewayPort(nil))
}

// ResolveRelayPort returns the extension relay port (browser control port + 1).
func ResolveRelayPort() int {
	return ResolveBrowserControlPort() + 1
}

const (
	// DefaultHandshakeTimeoutMs is the default WebSocket handshake timeout.
	DefaultHandshakeTimeoutMs = 5000
	// DefaultFetchTimeoutMs is the default timeout for HTTP fetches.
	DefaultFetchTimeoutMs = 1500
)
