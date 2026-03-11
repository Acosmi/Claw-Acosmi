package browser

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// AllowedExtensionOrigins are the Chrome extension origins allowed to connect.
var AllowedExtensionOrigins = []string{
	"chrome-extension://",
}

// ExtensionRelay acts as a WebSocket relay between a Chrome extension
// and the backend, forwarding CDP messages. Ported from extension-relay.ts (790L).
type ExtensionRelay struct {
	mu        sync.RWMutex
	server    *http.Server
	listener  net.Listener
	port      int
	authToken string
	logger    *slog.Logger
	upgrader  websocket.Upgrader
	closed    bool

	// Target tracking (BR-H14).
	targets     sync.Map // map[targetID]*relayTarget
	sessions    sync.Map // map[sessionID]*relaySession
	nextSession int64

	// Extension mode: when Chrome extension connects without a CDP target,
	// it enters extension mode where chrome.debugger API handles CDP.
	extConn    *websocket.Conn // active extension connection
	extMu      sync.Mutex
	extPending sync.Map // map[requestID]chan json.RawMessage — pending CDP requests
	extNextID  int64
}

// relayTarget tracks a connected CDP target.
type relayTarget struct {
	ID    string
	URL   string
	Title string
	Type  string
	Conn  *websocket.Conn
}

// relaySession tracks an active relay session (extension ↔ target).
type relaySession struct {
	ID        string
	TargetID  string
	StartedAt time.Time
}

// ExtensionRelayConfig configures the extension relay.
type ExtensionRelayConfig struct {
	Port            int
	Logger          *slog.Logger
	AllowedOrigins  []string // custom allowed origins (optional)
	ValidateOrigins bool     // if true, enforce origin checking
	TokenFile       string   // path to persist the relay auth token (survives restarts)
}

// NewExtensionRelay creates and starts a new extension relay server.
func NewExtensionRelay(cfg ExtensionRelayConfig) (*ExtensionRelay, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Persist token across restarts: load from file if available, else generate and save.
	token, err := loadOrGenerateToken(cfg.TokenFile, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("relay auth token: %w", err)
	}

	allowedOrigins := cfg.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = AllowedExtensionOrigins
	}

	relay := &ExtensionRelay{
		authToken: token,
		logger:    cfg.Logger,
	}
	relay.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if !cfg.ValidateOrigins {
				return true
			}
			return relay.isAllowedOrigin(r, allowedOrigins)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", relay.handleWS)
	mux.HandleFunc("/health", relay.handleHealth)
	// BR-H15: /json/* endpoints — expose target/version info through relay.
	mux.HandleFunc("/json", relay.handleJSONTargets)
	mux.HandleFunc("/json/list", relay.handleJSONTargets)
	mux.HandleFunc("/json/version", relay.handleJSONVersion)
	mux.HandleFunc("/json/protocol", relay.handleJSONProtocol)

	addr := "127.0.0.1:" + strconv.Itoa(cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("extension relay listen: %w", err)
	}

	relay.listener = listener
	relay.port = listener.Addr().(*net.TCPAddr).Port
	relay.server = &http.Server{Handler: relay.withCORS(mux, allowedOrigins, cfg.ValidateOrigins)}

	go func() {
		if err := relay.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			cfg.Logger.Error("extension relay serve error", "err", err)
		}
	}()

	// BR-M11: Start keepalive goroutine — ping all targets every 30s.
	go relay.keepaliveLoop()

	cfg.Logger.Info("extension relay started", "port", relay.port)
	return relay, nil
}

// keepaliveLoop sends periodic pings to connected targets and cleans up stale sessions.
func (r *ExtensionRelay) keepaliveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		r.mu.RLock()
		closed := r.closed
		r.mu.RUnlock()
		if closed {
			return
		}
		<-ticker.C
		r.PingAllTargets()
		// BR-M12: Clean up stale sessions (older than 1 hour with no target).
		r.cleanupStaleSessions(1 * time.Hour)
	}
}

// cleanupStaleSessions removes sessions older than maxAge.
func (r *ExtensionRelay) cleanupStaleSessions(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	r.sessions.Range(func(key, value any) bool {
		s := value.(*relaySession)
		if s.StartedAt.Before(cutoff) {
			r.sessions.Delete(key)
			r.logger.Debug("cleaned up stale session", "id", s.ID, "age", time.Since(s.StartedAt))
		}
		return true
	})
}

// Port returns the port the relay is listening on.
func (r *ExtensionRelay) Port() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.port
}

// AuthToken returns the authentication token for the relay.
func (r *ExtensionRelay) AuthToken() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.authToken
}

// ExtensionConnected reports whether a browser extension is currently connected.
func (r *ExtensionRelay) ExtensionConnected() bool {
	r.extMu.Lock()
	defer r.extMu.Unlock()
	return r.extConn != nil
}

func (r *ExtensionRelay) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok")) //nolint:errcheck
}

func (r *ExtensionRelay) handleWS(w http.ResponseWriter, req *http.Request) {
	// Verify auth token.
	token := req.URL.Query().Get("token")
	if token != r.authToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// CDP target URL is passed as a query parameter.
	cdpTarget := req.URL.Query().Get("target")

	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		r.logger.Error("extension relay upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	r.logger.Info("extension connected", "cdpTarget", cdpTarget)

	// Extension mode: no CDP target = Chrome extension using chrome.debugger API.
	if cdpTarget == "" {
		r.handleExtensionMode(conn)
		return
	}

	// Connect to the CDP target.
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	targetConn, _, err := dialer.Dial(cdpTarget, nil)
	if err != nil {
		r.logger.Error("extension relay: failed to connect to CDP target", "target", cdpTarget, "err", err)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "failed to connect to CDP target"))
		return
	}
	defer targetConn.Close()

	r.logger.Info("extension relay: CDP target connected", "target", cdpTarget)

	// Register session for tracking (BR-H14).
	sessionID := r.registerSession(cdpTarget)
	defer r.unregisterSession(sessionID)

	// Bidirectional relay with CDP command routing (BR-H13):
	// Extension ↔ CDP target, with message inspection for target tracking.
	var wg sync.WaitGroup
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }

	// Extension → CDP target
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer closeDone()
		for {
			select {
			case <-done:
				return
			default:
			}
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					r.logger.Debug("extension→target read error", "err", err)
				}
				return
			}
			// Route CDP commands (inspect, track targets).
			if msgType == websocket.TextMessage {
				r.routeCdpMessage(msg, sessionID)
			}
			if err := targetConn.WriteMessage(msgType, msg); err != nil {
				r.logger.Debug("extension→target write error", "err", err)
				return
			}
		}
	}()

	// CDP target → Extension
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer closeDone()
		for {
			select {
			case <-done:
				return
			default:
			}
			msgType, msg, err := targetConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					r.logger.Debug("target→extension read error", "err", err)
				}
				return
			}
			// Route CDP events (track target creation/destruction).
			if msgType == websocket.TextMessage {
				r.routeCdpMessage(msg, sessionID)
			}
			if err := conn.WriteMessage(msgType, msg); err != nil {
				r.logger.Debug("target→extension write error", "err", err)
				return
			}
		}
	}()

	wg.Wait()
	r.logger.Info("extension relay: connection closed", "target", cdpTarget, "session", sessionID)
}

// --- /json/* endpoint handlers (BR-H15) ---

// handleJSONTargets returns connected targets as CDP-compatible /json response.
func (r *ExtensionRelay) handleJSONTargets(w http.ResponseWriter, req *http.Request) {
	var targets []map[string]string
	r.targets.Range(func(key, value any) bool {
		t := value.(*relayTarget)
		targets = append(targets, map[string]string{
			"id":                   t.ID,
			"type":                 t.Type,
			"title":                t.Title,
			"url":                  t.URL,
			"webSocketDebuggerUrl": fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s&target=%s", r.port, r.authToken, t.ID),
		})
		return true
	})
	if targets == nil {
		targets = []map[string]string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targets) //nolint:errcheck
}

// handleJSONVersion returns relay version info in CDP /json/version format.
func (r *ExtensionRelay) handleJSONVersion(w http.ResponseWriter, _ *http.Request) {
	info := map[string]string{
		"Browser":              "Crab Claw Extension Relay/1.0",
		"Protocol-Version":     "1.3",
		"webSocketDebuggerUrl": fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", r.port, r.authToken),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info) //nolint:errcheck
}

// handleJSONProtocol returns a minimal protocol descriptor.
func (r *ExtensionRelay) handleJSONProtocol(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"version":{"major":"1","minor":"3"}}`)) //nolint:errcheck
}

func (r *ExtensionRelay) withCORS(next http.Handler, allowed []string, validate bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if origin, ok := r.allowedCORSOrigin(req, allowed, validate); ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Vary", "Origin")
		}

		if req.Method == http.MethodOptions {
			if validate {
				origin := strings.TrimSpace(req.Header.Get("Origin"))
				if origin != "" {
					if _, ok := r.allowedCORSOrigin(req, allowed, validate); !ok {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, req)
	})
}

func (r *ExtensionRelay) allowedCORSOrigin(req *http.Request, allowed []string, validate bool) (string, bool) {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		return "", false
	}
	if !validate {
		return origin, true
	}
	if r.isAllowedOrigin(req, allowed) {
		return origin, true
	}
	return "", false
}

// --- Origin validation (BR-H16) ---

// isAllowedOrigin checks if the request origin matches allowed patterns.
func (r *ExtensionRelay) isAllowedOrigin(req *http.Request, allowed []string) bool {
	origin := req.Header.Get("Origin")
	if origin == "" {
		// No origin header — allow localhost connections.
		host := req.Host
		if host == "" {
			host = req.URL.Host
		}
		return IsLoopbackHost(strings.Split(host, ":")[0])
	}
	for _, a := range allowed {
		if strings.HasPrefix(origin, a) {
			return true
		}
	}
	r.logger.Warn("extension relay: rejected origin", "origin", origin)
	return false
}

// --- CDP command routing (BR-H13) ---

// routeCdpMessage inspects a CDP message and performs any relay-side actions.
// Returns true if the message should be forwarded, false if it was handled locally.
func (r *ExtensionRelay) routeCdpMessage(msg []byte, sessionID string) bool {
	var envelope struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return true // Forward unknown messages.
	}

	// Track target-related events.
	switch envelope.Method {
	case "Target.targetCreated":
		var params struct {
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				Title    string `json:"title"`
				URL      string `json:"url"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(msg, &struct {
			Params *struct {
				TargetInfo *struct {
					TargetID string `json:"targetId"`
					Type     string `json:"type"`
					Title    string `json:"title"`
					URL      string `json:"url"`
				} `json:"targetInfo"`
			} `json:"params"`
		}{Params: &struct {
			TargetInfo *struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				Title    string `json:"title"`
				URL      string `json:"url"`
			} `json:"targetInfo"`
		}{TargetInfo: &params.TargetInfo}}) == nil && params.TargetInfo.TargetID != "" {
			r.targets.Store(params.TargetInfo.TargetID, &relayTarget{
				ID:    params.TargetInfo.TargetID,
				Type:  params.TargetInfo.Type,
				Title: params.TargetInfo.Title,
				URL:   params.TargetInfo.URL,
			})
			r.logger.Debug("target created", "targetId", params.TargetInfo.TargetID)
		}

	case "Target.targetDestroyed":
		var params struct {
			TargetID string `json:"targetId"`
		}
		raw := struct {
			Params json.RawMessage `json:"params"`
		}{}
		if json.Unmarshal(msg, &raw) == nil {
			if json.Unmarshal(raw.Params, &params) == nil && params.TargetID != "" {
				r.targets.Delete(params.TargetID)
				r.logger.Debug("target destroyed", "targetId", params.TargetID)
			}
		}
	}

	return true // Always forward to the other side.
}

// --- Session tracking (BR-H14) ---

// registerSession creates a new relay session entry.
func (r *ExtensionRelay) registerSession(targetID string) string {
	r.mu.Lock()
	r.nextSession++
	id := fmt.Sprintf("relay-%d", r.nextSession)
	r.mu.Unlock()

	r.sessions.Store(id, &relaySession{
		ID:        id,
		TargetID:  targetID,
		StartedAt: time.Now(),
	})
	return id
}

// unregisterSession removes a relay session.
func (r *ExtensionRelay) unregisterSession(id string) {
	r.sessions.Delete(id)
}

// ConnectedTargetCount returns the number of tracked targets.
func (r *ExtensionRelay) ConnectedTargetCount() int {
	count := 0
	r.targets.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// ActiveSessionCount returns the number of active relay sessions.
func (r *ExtensionRelay) ActiveSessionCount() int {
	count := 0
	r.sessions.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// PingAllTargets sends a ping to verify target connections are alive.
func (r *ExtensionRelay) PingAllTargets() {
	r.targets.Range(func(key, value any) bool {
		t := value.(*relayTarget)
		if t.Conn != nil {
			if err := t.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(2*time.Second)); err != nil {
				r.logger.Debug("target ping failed, removing", "targetId", t.ID, "err", err)
				r.targets.Delete(key)
			}
		}
		return true
	})
}

// Close shuts down the extension relay.
func (r *ExtensionRelay) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.logger.Info("extension relay closing")
	return r.server.Close()
}

// ---- Extension Mode (chrome.debugger API bridge) ----

// handleExtensionMode manages a Chrome extension connection.
// The extension uses chrome.debugger API internally and communicates
// via JSON messages with type-based routing.
func (r *ExtensionRelay) handleExtensionMode(conn *websocket.Conn) {
	r.extMu.Lock()
	if r.extConn != nil {
		// Close previous extension connection.
		r.extConn.Close()
	}
	r.extConn = conn
	r.extMu.Unlock()

	r.logger.Info("extension connected in extension mode (chrome.debugger)")

	defer func() {
		r.extMu.Lock()
		if r.extConn == conn {
			r.extConn = nil
		}
		r.extMu.Unlock()
		r.logger.Info("extension disconnected")
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				r.logger.Debug("extension read error", "err", err)
			}
			return
		}
		r.handleExtensionMessage(msg)
	}
}

// handleExtensionMessage processes a message from the Chrome extension.
func (r *ExtensionRelay) handleExtensionMessage(msg []byte) {
	var envelope struct {
		Type   string          `json:"type"`
		ID     int64           `json:"id,omitempty"`
		TabID  int             `json:"tabId,omitempty"`
		Method string          `json:"method,omitempty"`
		Params json.RawMessage `json:"params,omitempty"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  string          `json:"error,omitempty"`
		Tabs   json.RawMessage `json:"tabs,omitempty"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		r.logger.Debug("extension message parse error", "err", err)
		return
	}

	switch envelope.Type {
	case "cdp_response":
		// Resolve pending CDP request.
		if ch, ok := r.extPending.LoadAndDelete(envelope.ID); ok {
			respCh := ch.(chan json.RawMessage)
			if envelope.Error != "" {
				// Send error as JSON.
				errJSON, _ := json.Marshal(map[string]string{"error": envelope.Error})
				respCh <- errJSON
			} else {
				respCh <- envelope.Result
			}
		}

	case "cdp_event":
		// Log CDP events from extension (could be forwarded to subscribers).
		r.logger.Debug("extension cdp event", "method", envelope.Method, "tabId", envelope.TabID)

	case "tab_list":
		r.logger.Debug("extension tab list received", "data", string(envelope.Tabs))

	case "tab_attached":
		r.logger.Info("extension tab attached", "tabId", envelope.TabID)

	case "tab_detached":
		r.logger.Info("extension tab detached", "tabId", envelope.TabID)

	case "tab_closed":
		r.logger.Info("extension tab closed", "tabId", envelope.TabID)

	case "tab_created":
		r.logger.Info("extension tab created", "tabId", envelope.TabID)

	case "cs_response":
		// Content script response from extension — resolve pending request.
		if ch, ok := r.extPending.LoadAndDelete(envelope.ID); ok {
			respCh := ch.(chan json.RawMessage)
			if envelope.Error != "" {
				errJSON, _ := json.Marshal(map[string]string{"error": envelope.Error})
				respCh <- errJSON
			} else {
				respCh <- envelope.Result
			}
		}

	case "s3_response":
		// S3 API response (downloads/notifications/tabGroups) — resolve pending.
		if ch, ok := r.extPending.LoadAndDelete(envelope.ID); ok {
			respCh := ch.(chan json.RawMessage)
			if envelope.Error != "" {
				errJSON, _ := json.Marshal(map[string]string{"error": envelope.Error})
				respCh <- errJSON
			} else {
				respCh <- envelope.Result
			}
		}

	case "ping":
		// Heartbeat from extension — respond with pong to keep connection alive.
		r.extMu.Lock()
		conn := r.extConn
		r.extMu.Unlock()
		if conn != nil {
			pong, _ := json.Marshal(map[string]string{"type": "pong"})
			if err := conn.WriteMessage(websocket.TextMessage, pong); err != nil {
				r.logger.Debug("pong write failed", "err", err)
			}
		}

	default:
		r.logger.Debug("extension unknown message type", "type", envelope.Type)
	}
}

// SendCDPToExtension sends a CDP command through the extension and waits for a response.
// This is the bridge between agent tools and the Chrome extension's chrome.debugger API.
func (r *ExtensionRelay) SendCDPToExtension(method string, params map[string]any, tabID int, timeout time.Duration) (json.RawMessage, error) {
	r.extMu.Lock()
	conn := r.extConn
	r.extMu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("no Chrome extension connected")
	}

	// Generate request ID.
	r.extMu.Lock()
	r.extNextID++
	reqID := r.extNextID
	r.extMu.Unlock()

	// Create response channel.
	respCh := make(chan json.RawMessage, 1)
	r.extPending.Store(reqID, respCh)
	defer r.extPending.Delete(reqID)

	// Send CDP command to extension.
	cmd := map[string]any{
		"type":   "cdp",
		"id":     reqID,
		"method": method,
		"tabId":  tabID,
	}
	if params != nil {
		cmd["params"] = params
	}

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal cdp command: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, cmdJSON); err != nil {
		return nil, fmt.Errorf("send cdp command to extension: %w", err)
	}

	// Wait for response with timeout.
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-respCh:
		// Check for error response.
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(result, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("cdp via extension: %s", errResp.Error)
		}
		return result, nil
	case <-timer.C:
		return nil, fmt.Errorf("cdp via extension: timeout after %s", timeout)
	}
}

// SendExtensionCommand sends a non-CDP command to the extension (e.g., list_tabs, attach, detach).
func (r *ExtensionRelay) SendExtensionCommand(cmdType string, extra map[string]any) error {
	r.extMu.Lock()
	conn := r.extConn
	r.extMu.Unlock()

	if conn == nil {
		return fmt.Errorf("no Chrome extension connected")
	}

	msg := map[string]any{"type": cmdType}
	for k, v := range extra {
		msg[k] = v
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

// SendContentScriptCommand sends a content script command to the extension
// and waits for a response. This bypasses CDP and uses the content script
// injected into the page for DOM operations.
func (r *ExtensionRelay) SendContentScriptCommand(action string, tabID int, extra map[string]any, timeout time.Duration) (json.RawMessage, error) {
	r.extMu.Lock()
	conn := r.extConn
	r.extMu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("no Chrome extension connected")
	}

	// Generate request ID.
	r.extMu.Lock()
	r.extNextID++
	reqID := r.extNextID
	r.extMu.Unlock()

	// Create response channel.
	respCh := make(chan json.RawMessage, 1)
	r.extPending.Store(reqID, respCh)
	defer r.extPending.Delete(reqID)

	// Build command.
	cmd := map[string]any{
		"type":   action, // e.g., "cs_get_text", "cs_click", "cs_fill"
		"action": action,
		"id":     reqID,
		"tabId":  tabID,
	}
	for k, v := range extra {
		cmd[k] = v
	}

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal content script command: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, cmdJSON); err != nil {
		return nil, fmt.Errorf("send content script command: %w", err)
	}

	// Wait for response.
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-respCh:
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(result, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("content script: %s", errResp.Error)
		}
		return result, nil
	case <-timer.C:
		return nil, fmt.Errorf("content script command %s: timeout after %s", action, timeout)
	}
}

// SendContentScriptOrCDP tries content script first, falls back to CDP if the
// content script is unavailable (e.g., chrome://, about: pages).
func (r *ExtensionRelay) SendContentScriptOrCDP(action string, tabID int, extra map[string]any, cdpMethod string, cdpParams map[string]any, timeout time.Duration) (json.RawMessage, error) {
	// Try content script first.
	result, err := r.SendContentScriptCommand(action, tabID, extra, timeout)
	if err == nil {
		return result, nil
	}

	r.logger.Debug("content script fallback to CDP",
		"action", action, "tabId", tabID, "csErr", err, "cdpMethod", cdpMethod)

	// Fall back to CDP.
	if cdpMethod != "" {
		return r.SendCDPToExtension(cdpMethod, cdpParams, tabID, timeout)
	}

	return nil, fmt.Errorf("content script failed and no CDP fallback: %w", err)
}

// IsExtensionConnected returns true if a Chrome extension is currently connected.
func (r *ExtensionRelay) IsExtensionConnected() bool {
	r.extMu.Lock()
	defer r.extMu.Unlock()
	return r.extConn != nil
}

func generateSecureToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// loadOrGenerateToken loads the relay token from a file, or generates a new one
// and saves it. This ensures the token survives gateway restarts so the
// extension doesn't lose its connection on every restart.
func loadOrGenerateToken(tokenFile string, logger *slog.Logger) (string, error) {
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err == nil {
			token := strings.TrimSpace(string(data))
			if len(token) == 64 { // valid hex-encoded 32-byte token
				logger.Info("relay token loaded from file", "path", tokenFile)
				return token, nil
			}
			logger.Warn("relay token file has invalid content, regenerating", "path", tokenFile)
		}
	}

	// Generate new token.
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}

	// Persist if path configured.
	if tokenFile != "" {
		dir := filepath.Dir(tokenFile)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			logger.Warn("cannot create relay token dir", "path", dir, "err", err)
		} else if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
			logger.Warn("cannot save relay token", "path", tokenFile, "err", err)
		} else {
			logger.Info("relay token generated and saved", "path", tokenFile)
		}
	}

	return token, nil
}
