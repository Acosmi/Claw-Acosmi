package mcpinstall

// store.go — Read/write the MCP server registry.json.
// Path: ~/.openacosmi/mcp-servers/registry.json
// Permissions: 0600 (P5-5 security requirement).

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// DefaultRegistryPath returns the default registry.json path.
func DefaultRegistryPath() (string, error) {
	return filepath.Join(managedMcpServersDir(), "registry.json"), nil
}

func managedMcpServersDir() string {
	return filepath.Join(config.ResolveStateDir(), "mcp-servers")
}

// LoadRegistry reads the registry from disk.
// Returns empty registry if the file doesn't exist.
func LoadRegistry(path string) (*McpServerRegistry, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &McpServerRegistry{
			SchemaVersion: 1,
			Servers:       make(map[string]InstalledMcpServer),
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var reg McpServerRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	if reg.Servers == nil {
		reg.Servers = make(map[string]InstalledMcpServer)
	}
	return &reg, nil
}

// SaveRegistry writes the registry to disk with 0600 permissions.
func SaveRegistry(path string, reg *McpServerRegistry) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	slog.Debug("mcpinstall: registry saved", "path", path, "servers", len(reg.Servers))
	return nil
}
