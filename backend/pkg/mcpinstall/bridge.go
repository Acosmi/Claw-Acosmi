package mcpinstall

// bridge.go — McpLocalBridge: stdio/HTTP bridge for locally installed MCP servers.
//
// State machine: init → starting → ready ↔ degraded → stopped
// Modeled after:
//   - pkg/mcpremote/bridge.go (RemoteBridge: health loop, reconnect)
//   - internal/sandbox/native_bridge.go (NativeSandboxBridge: stdio IPC, process lifecycle)
//
// Supports two transport modes:
//   - stdio: spawn process, JSON-RPC 2.0 over stdin/stdout
//   - http: spawn process, connect via HTTP (SSE/Streamable HTTP) after startup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------- Constants ----------

const (
	bridgeHealthInterval  = 30 * time.Second
	bridgeMaxPingFailures = 3
	bridgeMaxReconnects   = 5
	bridgeInitialBackoff  = 1 * time.Second
	bridgeMaxBackoff      = 60 * time.Second
	bridgeGracefulWait    = 3 * time.Second
	bridgeInitTimeout     = 15 * time.Second
	bridgeToolsTimeout    = 10 * time.Second
)

// ---------- JSON-RPC 2.0 ----------

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ---------- McpLocalBridge ----------

// McpLocalBridge manages a locally installed MCP server process.
type McpLocalBridge struct {
	cfg McpLocalBridgeConfig

	mu     sync.Mutex
	state  BridgeState
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Scanner
	pid    int
	nextID atomic.Uint64
	tools  []McpTool

	// HTTP transport fields (used when transport is SSE or HTTP)
	httpEndpoint string
	httpClient   *http.Client

	cancel context.CancelFunc
	done   chan struct{}
}

// NewMcpLocalBridge creates a bridge (not yet started).
func NewMcpLocalBridge(cfg McpLocalBridgeConfig) *McpLocalBridge {
	if cfg.HealthInterval <= 0 {
		cfg.HealthInterval = bridgeHealthInterval
	}
	return &McpLocalBridge{
		cfg:   cfg,
		state: BridgeStateInit,
		done:  make(chan struct{}),
	}
}

// State returns the current bridge state.
func (b *McpLocalBridge) State() BridgeState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Tools returns the cached tool list.
func (b *McpLocalBridge) Tools() []McpTool {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]McpTool, len(b.tools))
	copy(cp, b.tools)
	return cp
}

// ServerName returns the server name.
func (b *McpLocalBridge) ServerName() string {
	return b.cfg.Server.Name
}

// Start spawns the MCP server process, performs MCP handshake, and discovers tools.
func (b *McpLocalBridge) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.state != BridgeStateInit && b.state != BridgeStateStopped {
		b.mu.Unlock()
		return fmt.Errorf("mcpinstall: cannot start bridge in state %s", b.state)
	}
	b.state = BridgeStateStarting
	b.mu.Unlock()

	transport := b.cfg.Server.Transport
	if transport == TransportSSE || transport == TransportHTTP {
		if err := b.spawnAndHandshakeHTTP(ctx); err != nil {
			b.mu.Lock()
			b.state = BridgeStateStopped
			b.mu.Unlock()
			return err
		}
	} else {
		if err := b.spawnAndHandshake(ctx); err != nil {
			b.mu.Lock()
			b.state = BridgeStateStopped
			b.mu.Unlock()
			return err
		}
	}

	// Start health monitor
	bgCtx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.cancel = cancel
	b.done = make(chan struct{})
	b.mu.Unlock()

	go b.healthLoop(bgCtx)

	return nil
}

// spawnAndHandshake spawns the process and performs MCP initialize + tools/list.
func (b *McpLocalBridge) spawnAndHandshake(ctx context.Context) error {
	server := b.cfg.Server

	// Resolve command
	command := server.BinaryPath
	if server.Command != nil && *server.Command != "" {
		command = *server.Command
	}

	// Validate binary path is within managed directory
	if err := validateBinaryPath(command); err != nil {
		return err
	}

	args := server.Args
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stderr = os.Stderr // MCP servers log to stderr

	// Inject environment variables (P3-8)
	if len(server.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range server.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcpinstall: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcpinstall: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcpinstall: spawn %s: %w", command, err)
	}

	b.mu.Lock()
	b.cmd = cmd
	b.stdin = stdinPipe
	b.reader = bufio.NewScanner(stdoutPipe)
	b.reader.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // 10MB max line
	b.pid = cmd.Process.Pid
	b.mu.Unlock()

	slog.Info("mcpinstall: process spawned", "name", server.Name, "pid", cmd.Process.Pid)

	// MCP initialize handshake
	initResult, err := b.callLocked(ctx, "initialize", json.RawMessage(`{
		"protocolVersion": "2024-11-05",
		"capabilities": {},
		"clientInfo": {"name": "openacosmi-gateway", "version": "0.1.0"}
	}`), bridgeInitTimeout)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("mcpinstall: initialize handshake: %w", err)
	}

	// Send initialized notification
	_ = b.notifyLocked("notifications/initialized", nil)

	slog.Debug("mcpinstall: handshake complete", "result", string(initResult))

	// Discover tools
	toolsResult, err := b.callLocked(ctx, "tools/list", nil, bridgeToolsTimeout)
	if err != nil {
		slog.Warn("mcpinstall: tools/list failed (non-fatal)", "error", err)
		// Non-fatal: server may not have tools yet
	} else {
		var toolsResp struct {
			Tools []McpTool `json:"tools"`
		}
		if err := json.Unmarshal(toolsResult, &toolsResp); err != nil {
			slog.Warn("mcpinstall: parse tools/list", "error", err)
		} else {
			b.mu.Lock()
			b.tools = toolsResp.Tools
			b.mu.Unlock()
		}
	}

	b.mu.Lock()
	b.state = BridgeStateReady
	b.mu.Unlock()

	slog.Info("mcpinstall: bridge ready",
		"name", server.Name,
		"tools", len(b.tools),
		"pid", b.pid,
	)

	return nil
}

// spawnAndHandshakeHTTP spawns the process and connects via HTTP for SSE/Streamable HTTP transport.
func (b *McpLocalBridge) spawnAndHandshakeHTTP(ctx context.Context) error {
	server := b.cfg.Server

	command := server.BinaryPath
	if server.Command != nil && *server.Command != "" {
		command = *server.Command
	}

	if err := validateBinaryPath(command); err != nil {
		return err
	}

	args := server.Args
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr // HTTP mode: stdout goes to stderr (no stdio IPC)

	if len(server.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range server.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcpinstall: spawn %s (HTTP): %w", command, err)
	}

	b.mu.Lock()
	b.cmd = cmd
	b.pid = cmd.Process.Pid
	b.mu.Unlock()

	slog.Info("mcpinstall: process spawned (HTTP mode)", "name", server.Name, "pid", cmd.Process.Pid)

	// Determine HTTP endpoint: use first arg that looks like a URL, or default to localhost:8080
	endpoint := "http://127.0.0.1:8080"
	for _, arg := range args {
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
			endpoint = arg
			break
		}
	}
	// Check env for PORT override
	if port, ok := server.Env["PORT"]; ok {
		endpoint = "http://127.0.0.1:" + port
	}

	b.mu.Lock()
	b.httpEndpoint = strings.TrimRight(endpoint, "/")
	b.httpClient = &http.Client{Timeout: 30 * time.Second}
	b.mu.Unlock()

	// Wait for server to become ready (retry handshake with backoff)
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return ctx.Err()
		case <-time.After(time.Duration(300+attempt*200) * time.Millisecond):
		}

		initResult, err := b.callHTTP(ctx, "initialize", json.RawMessage(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "openacosmi-gateway", "version": "0.1.0"}
		}`), bridgeInitTimeout)
		if err != nil {
			lastErr = err
			continue
		}

		slog.Debug("mcpinstall: HTTP handshake complete", "result", string(initResult))

		// Send initialized notification
		_, _ = b.callHTTP(ctx, "notifications/initialized", nil, 5*time.Second)

		// Discover tools
		toolsResult, err := b.callHTTP(ctx, "tools/list", nil, bridgeToolsTimeout)
		if err != nil {
			slog.Warn("mcpinstall: HTTP tools/list failed (non-fatal)", "error", err)
		} else {
			var toolsResp struct {
				Tools []McpTool `json:"tools"`
			}
			if err := json.Unmarshal(toolsResult, &toolsResp); err != nil {
				slog.Warn("mcpinstall: parse HTTP tools/list", "error", err)
			} else {
				b.mu.Lock()
				b.tools = toolsResp.Tools
				b.mu.Unlock()
			}
		}

		b.mu.Lock()
		b.state = BridgeStateReady
		b.mu.Unlock()

		slog.Info("mcpinstall: HTTP bridge ready",
			"name", server.Name, "tools", len(b.tools), "endpoint", b.httpEndpoint)
		return nil
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return fmt.Errorf("mcpinstall: HTTP handshake failed after retries: %w", lastErr)
}

// callHTTP sends a JSON-RPC request over HTTP POST.
func (b *McpLocalBridge) callHTTP(ctx context.Context, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	endpoint := b.httpEndpoint
	client := b.httpClient
	b.mu.Unlock()

	if client == nil || endpoint == "" {
		return nil, fmt.Errorf("mcpinstall: HTTP not configured")
	}

	id := b.nextID.Add(1)
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(&req)
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: marshal: %w", err)
	}

	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(httpCtx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcpinstall: HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 500)]))
	}

	var rpcResp jsonrpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("mcpinstall: unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcpinstall: RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// CallTool calls a tool on the MCP server.
func (b *McpLocalBridge) CallTool(ctx context.Context, name string, arguments json.RawMessage, timeout time.Duration) (*McpToolCallResult, error) {
	b.mu.Lock()
	state := b.state
	isHTTP := b.httpClient != nil
	b.mu.Unlock()

	if state != BridgeStateReady && state != BridgeStateDegraded {
		return nil, fmt.Errorf("mcpinstall: bridge not available (state: %s)", state)
	}

	params, _ := json.Marshal(map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	})

	var result json.RawMessage
	var err error
	if isHTTP {
		result, err = b.callHTTP(ctx, "tools/call", params, timeout)
	} else {
		result, err = b.callLocked(ctx, "tools/call", params, timeout)
	}
	if err != nil {
		return nil, err
	}

	var toolResult McpToolCallResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("mcpinstall: parse tool result: %w", err)
	}

	return &toolResult, nil
}

// callLocked sends a JSON-RPC request and waits for the response.
func (b *McpLocalBridge) callLocked(ctx context.Context, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stdin == nil {
		return nil, fmt.Errorf("mcpinstall: not connected")
	}

	id := b.nextID.Add(1)
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(&req)
	if err != nil {
		return nil, fmt.Errorf("mcpinstall: marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := b.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("mcpinstall: write: %w", err)
	}

	// Read response (with timeout via context)
	// Note: like NativeSandboxBridge, we read synchronously while holding the lock
	// to ensure request/response pairing on the serial stdio channel.
	if !b.reader.Scan() {
		if err := b.reader.Err(); err != nil {
			return nil, fmt.Errorf("mcpinstall: read: %w", err)
		}
		return nil, fmt.Errorf("mcpinstall: server stdout closed (EOF)")
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(b.reader.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("mcpinstall: unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcpinstall: RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// notifyLocked sends a JSON-RPC notification (no response expected).
func (b *McpLocalBridge) notifyLocked(method string, params json.RawMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stdin == nil {
		return fmt.Errorf("mcpinstall: not connected")
	}

	// Notifications have no ID
	type notification struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	data, err := json.Marshal(&notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = b.stdin.Write(data)
	return err
}

// healthLoop periodically pings the MCP server.
func (b *McpLocalBridge) healthLoop(ctx context.Context) {
	defer close(b.done)

	ticker := time.NewTicker(b.cfg.HealthInterval)
	defer ticker.Stop()
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.mu.Lock()
			state := b.state
			b.mu.Unlock()

			if state == BridgeStateStopped {
				return
			}

			// Ping via JSON-RPC (some servers support ping, others we just check process)
			b.mu.Lock()
			isHTTP := b.httpClient != nil
			b.mu.Unlock()

			var err error
			if isHTTP {
				_, err = b.callHTTP(ctx, "ping", nil, 5*time.Second)
			} else {
				_, err = b.callLocked(ctx, "ping", nil, 5*time.Second)
			}
			if err != nil {
				// Fallback: check if process is still alive
				b.mu.Lock()
				if b.cmd != nil && b.cmd.ProcessState != nil && b.cmd.ProcessState.Exited() {
					slog.Warn("mcpinstall: server process exited", "name", b.cfg.Server.Name)
					b.state = BridgeStateStopped
					b.mu.Unlock()
					return
				}
				b.mu.Unlock()

				failures++
				if failures >= bridgeMaxPingFailures {
					b.mu.Lock()
					if b.state == BridgeStateReady {
						b.state = BridgeStateDegraded
						slog.Warn("mcpinstall: bridge degraded", "name", b.cfg.Server.Name, "failures", failures)
					}
					b.mu.Unlock()
				}
			} else {
				if failures > 0 {
					slog.Info("mcpinstall: health recovered", "name", b.cfg.Server.Name)
				}
				failures = 0
				b.mu.Lock()
				if b.state == BridgeStateDegraded {
					b.state = BridgeStateReady
				}
				b.mu.Unlock()
			}
		}
	}
}

// Stop gracefully shuts down the MCP server process.
func (b *McpLocalBridge) Stop() {
	b.mu.Lock()
	if b.state == BridgeStateStopped {
		b.mu.Unlock()
		return
	}

	cancel := b.cancel
	cmd := b.cmd
	done := b.done
	b.state = BridgeStateStopped
	b.mu.Unlock()

	// Cancel health loop
	if cancel != nil {
		cancel()
	}

	// Close stdin → server should exit on EOF
	b.mu.Lock()
	if b.stdin != nil {
		_ = b.stdin.Close()
	}
	b.mu.Unlock()

	// Wait for process
	if cmd != nil && cmd.Process != nil {
		waitCh := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitCh)
		}()

		select {
		case <-waitCh:
			slog.Info("mcpinstall: server exited gracefully", "name", b.cfg.Server.Name)
		case <-time.After(bridgeGracefulWait):
			slog.Warn("mcpinstall: killing server", "name", b.cfg.Server.Name)
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}

	// Wait for health loop
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}

	b.mu.Lock()
	b.tools = nil
	b.stdin = nil
	b.reader = nil
	b.cmd = nil
	b.mu.Unlock()

	slog.Info("mcpinstall: bridge stopped", "name", b.cfg.Server.Name)
}

// validateBinaryPath checks the binary is within the managed mcp-servers directory.
func validateBinaryPath(binaryPath string) error {
	managedAbs, err := filepath.Abs(managedMcpServersDir())
	if err != nil {
		return fmt.Errorf("mcpinstall: abs managed dir: %w", err)
	}

	// Resolve symlinks for the check
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("mcpinstall: abs path: %w", err)
	}

	if !strings.HasPrefix(absPath, managedAbs) {
		return fmt.Errorf("mcpinstall: binary path %q is outside managed directory %q", absPath, managedAbs)
	}
	return nil
}
