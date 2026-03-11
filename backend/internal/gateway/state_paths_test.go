package gateway

import (
	"path/filepath"
	"testing"
)

func TestGatewayStatePaths_UseResolvedStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	if got := gatewayTokenFilePath(); got != filepath.Join(stateDir, "gateway-token") {
		t.Fatalf("gatewayTokenFilePath() = %q", got)
	}

	notifier := &RemoteApprovalNotifier{}
	if got := notifier.configFilePath(); got != filepath.Join(stateDir, remoteApprovalConfigFile) {
		t.Fatalf("configFilePath() = %q", got)
	}

	logger := NewEscalationAuditLogger()
	if got := logger.FilePath(); got != filepath.Join(stateDir, defaultAuditLogFile) {
		t.Fatalf("EscalationAuditLogger.FilePath() = %q", got)
	}

	if got := taskPresetsFilePath(); got != filepath.Join(stateDir, taskPresetsFile) {
		t.Fatalf("taskPresetsFilePath() = %q", got)
	}

	if got := resolveMediaDir(); got != filepath.Join(stateDir, "media") {
		t.Fatalf("resolveMediaDir() = %q", got)
	}
}
