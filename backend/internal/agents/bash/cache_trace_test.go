package bash

import (
	"path/filepath"
	"testing"
)

func TestResolveCacheTraceConfig_UsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	cfg := resolveCacheTraceConfig(CacheTraceInit{Enabled: true})
	want := filepath.Join(stateDir, "logs", "cache-trace.jsonl")
	if cfg.FilePath != want {
		t.Fatalf("resolveCacheTraceConfig().FilePath = %q, want %q", cfg.FilePath, want)
	}
}
