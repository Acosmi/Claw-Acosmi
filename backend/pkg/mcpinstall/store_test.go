package mcpinstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRegistry_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.SchemaVersion != 1 {
		t.Errorf("expected schema_version=1, got %d", reg.SchemaVersion)
	}
	if len(reg.Servers) != 0 {
		t.Errorf("expected empty servers, got %d", len(reg.Servers))
	}
}

func TestSaveAndLoadRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	reg := &McpServerRegistry{
		SchemaVersion: 1,
		Servers: map[string]InstalledMcpServer{
			"test-server": {
				Name:       "test-server",
				SourceURL:  "https://github.com/test/server",
				Transport:  TransportStdio,
				BinaryPath: "/tmp/test-server",
				Env:        map[string]string{"API_KEY": "secret"},
			},
		},
	}

	if err := SaveRegistry(path, reg); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}

	// Reload
	loaded, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(loaded.Servers))
	}
	s, ok := loaded.Servers["test-server"]
	if !ok {
		t.Fatal("test-server not found")
	}
	if s.SourceURL != "https://github.com/test/server" {
		t.Errorf("unexpected source_url: %s", s.SourceURL)
	}
	if s.Env["API_KEY"] != "secret" {
		t.Errorf("unexpected env API_KEY: %s", s.Env["API_KEY"])
	}
}

func TestDefaultRegistryPath_UsesResolvedStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	path, err := DefaultRegistryPath()
	if err != nil {
		t.Fatalf("DefaultRegistryPath() error = %v", err)
	}
	if path != filepath.Join(stateDir, "mcp-servers", "registry.json") {
		t.Fatalf("DefaultRegistryPath() = %q", path)
	}
}

func TestValidateBinaryPath_UsesResolvedStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	managedDir := filepath.Join(stateDir, "mcp-servers")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatalf("mkdir managed dir: %v", err)
	}
	managedBinary := filepath.Join(managedDir, "test-server")
	if err := os.WriteFile(managedBinary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := validateBinaryPath(managedBinary); err != nil {
		t.Fatalf("validateBinaryPath(managed) error = %v", err)
	}

	outsideBinary := filepath.Join(t.TempDir(), "outside-server")
	if err := os.WriteFile(outsideBinary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write outside binary: %v", err)
	}
	if err := validateBinaryPath(outsideBinary); err == nil {
		t.Fatal("validateBinaryPath(outside) expected error")
	}
}

func TestManagerParsePrefixedToolName(t *testing.T) {
	tests := []struct {
		input      string
		wantServer string
		wantTool   string
		wantErr    bool
	}{
		{"mcp_myserver_read", "myserver", "read", false},
		{"mcp_my-server_tool_name", "my-server", "tool_name", false},
		{"remote_foo", "", "", true},             // wrong prefix
		{"mcp_nounderscoresuffix", "", "", true}, // no tool separator
	}
	for _, tt := range tests {
		server, tool, err := parsePrefixedToolName(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parsePrefixedToolName(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePrefixedToolName(%q): %v", tt.input, err)
			continue
		}
		if server != tt.wantServer || tool != tt.wantTool {
			t.Errorf("parsePrefixedToolName(%q) = (%q, %q), want (%q, %q)", tt.input, server, tool, tt.wantServer, tt.wantTool)
		}
	}
}

func TestMcpToolCallResultToText(t *testing.T) {
	result := &McpToolCallResult{
		Content: []McpToolCallContent{
			{Type: "text", Text: "hello"},
			{Type: "text", Text: "world"},
		},
	}
	got := McpToolCallResultToText(result)
	if got != "hello\nworld" {
		t.Errorf("unexpected: %q", got)
	}

	errorResult := &McpToolCallResult{
		Content: []McpToolCallContent{{Type: "text", Text: "oops"}},
		IsError: true,
	}
	got2 := McpToolCallResultToText(errorResult)
	if got2 != "[MCP tool error] oops" {
		t.Errorf("unexpected error text: %q", got2)
	}
}
