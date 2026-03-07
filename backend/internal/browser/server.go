package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BrowserServer is an HTTP control server for browser automation.
// It exposes routes for navigate, act, screenshot, console, etc.
// TS source: bridge-server.ts (77L) + control-service.ts (88L).
type BrowserServer struct {
	mu      sync.Mutex
	client  *Client
	config  *ResolvedBrowserConfig
	tools   PlaywrightTools
	server  *http.Server
	port    int
	host    string
	token   string
	logger  *slog.Logger
	running bool
}

// ServerOptions configures the BrowserServer.
type ServerOptions struct {
	Host            string // default "127.0.0.1"
	Port            int    // default ResolveBrowserControlPort()
	AuthToken       string // optional Bearer token
	Logger          *slog.Logger
	PlaywrightTools PlaywrightTools // optional; defaults to StubPlaywrightTools
}

// NewBrowserServer creates a new browser HTTP control server.
func NewBrowserServer(config *ResolvedBrowserConfig, client *Client, opts ServerOptions) *BrowserServer {
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.Port
	if port == 0 {
		port = ResolveBrowserControlPort()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	tools := opts.PlaywrightTools
	if tools == nil {
		tools = &StubPlaywrightTools{}
	}
	return &BrowserServer{
		client: client,
		config: config,
		tools:  tools,
		host:   host,
		port:   port,
		token:  strings.TrimSpace(opts.AuthToken),
		logger: logger,
	}
}

// Start begins listening on the configured address.
func (s *BrowserServer) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := http.Handler(mux)
	// Auth middleware
	if s.token != "" {
		handler = s.authMiddleware(mux)
	}
	// Body size limit (1MB, matching express json limit)
	handler = http.MaxBytesHandler(handler, 1<<20)

	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	s.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("browser server: listen %s: %w", addr, err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.running = true

	s.logger.Info("browser control server started", "addr", addr, "port", s.port)

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("browser control server error", "err", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the server.
func (s *BrowserServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.server == nil {
		return nil
	}
	s.running = false
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.logger.Info("browser control server stopping")
	return s.server.Shutdown(shutdownCtx)
}

// Port returns the actual listening port.
func (s *BrowserServer) Port() int {
	return s.port
}

// BaseURL returns the server base URL.
func (s *BrowserServer) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", s.host, s.port)
}

// authMiddleware validates Bearer token.
func (s *BrowserServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if auth == "Bearer "+s.token {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// registerRoutes sets up the HTTP routes.
func (s *BrowserServer) registerRoutes(mux *http.ServeMux) {
	// Basic control routes (bridge-server.ts + control-service.ts)
	mux.HandleFunc("POST /navigate", s.handleNavigate)
	mux.HandleFunc("POST /screenshot", s.handleScreenshot)
	mux.HandleFunc("POST /evaluate", s.handleEvaluate)
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("POST /launch", s.handleLaunch)
	mux.HandleFunc("POST /close", s.handleClose)

	// Agent proxy routes (routes/agent.*.ts)
	RegisterAgentRoutes(mux, s.tools, s.client, s.logger)
}

// ── Route handlers ──

func (s *BrowserServer) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
		URL     string `json:"url"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile := s.resolveProfile(req.Profile)
	if err := s.client.Navigate(r.Context(), profile, req.URL); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *BrowserServer) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile := s.resolveProfile(req.Profile)
	data, err := s.client.Screenshot(r.Context(), profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

func (s *BrowserServer) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile    string `json:"profile"`
		Expression string `json:"expression"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile := s.resolveProfile(req.Profile)
	result, err := s.client.Evaluate(r.Context(), profile, req.Expression)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": result})
}

func (s *BrowserServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	profiles := make([]string, 0)
	if s.config != nil {
		for name := range s.config.Profiles {
			profiles = append(profiles, name)
		}
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"profiles": profiles,
		"port":     s.port,
	})
}

func (s *BrowserServer) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile := s.resolveProfile(req.Profile)
	if _, err := s.client.Launch(r.Context(), profile); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *BrowserServer) handleClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile := s.resolveProfile(req.Profile)
	if err := s.client.CloseSession(profile); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ── Helpers ──

func (s *BrowserServer) resolveProfile(name string) string {
	if name != "" {
		return name
	}
	if s.config != nil && s.config.DefaultProfile != "" {
		return s.config.DefaultProfile
	}
	return "default"
}

func readJSON(r *http.Request, v any) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
