package llmclient

import (
	"path/filepath"
	"testing"
)

func TestResolvePayloadLogFilePath_UsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")
	t.Setenv("OPENACOSMI_ANTHROPIC_PAYLOAD_LOG_FILE", "")

	want := filepath.Join(stateDir, "logs", "anthropic-payload.jsonl")
	if got := resolvePayloadLogFilePath(); got != want {
		t.Fatalf("resolvePayloadLogFilePath() = %q, want %q", got, want)
	}
}
