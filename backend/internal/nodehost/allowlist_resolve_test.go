package nodehost

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

func TestResolveExecApprovalsFromFileKeepsSandboxedSecurity(t *testing.T) {
	file := &infra.ExecApprovalsFile{
		Version: 1,
		Defaults: &infra.ExecApprovalsDefaults{
			Security: infra.ExecSecuritySandboxed,
		},
	}

	resolved := ResolveExecApprovalsFromFile(struct {
		File       *infra.ExecApprovalsFile
		AgentID    string
		Overrides  *ExecApprovalsDefaultOverrides
		Path       string
		SocketPath string
		Token      string
	}{
		File:    file,
		AgentID: "main",
	})

	if resolved.Defaults.Security != infra.ExecSecuritySandboxed {
		t.Fatalf("expected defaults security sandboxed, got %q", resolved.Defaults.Security)
	}
	if resolved.Agent.Security != infra.ExecSecuritySandboxed {
		t.Fatalf("expected agent security sandboxed, got %q", resolved.Agent.Security)
	}
}

func TestMinSecurityOrdersSandboxedBelowFull(t *testing.T) {
	if got := MinSecurity(infra.ExecSecurityFull, infra.ExecSecuritySandboxed); got != infra.ExecSecuritySandboxed {
		t.Fatalf("expected sandboxed to be the minimum, got %q", got)
	}
}
