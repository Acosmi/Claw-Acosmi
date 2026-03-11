package gateway

import (
	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/infra"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func resolveConfigExecSecurity(cfg *types.OpenAcosmiConfig) string {
	if cfg == nil || cfg.Tools == nil || cfg.Tools.Exec == nil {
		return ""
	}
	return cfg.Tools.Exec.Security
}

func resolveBaseSecurityLevelWithConfig(cfg *types.OpenAcosmiConfig) string {
	return string(infra.ResolveBaseExecSecurity(resolveConfigExecSecurity(cfg)))
}

func resolveEffectiveSecurityLevelWithManager(mgr *EscalationManager, cfg *types.OpenAcosmiConfig) string {
	if mgr != nil {
		if level := infra.NormalizeExecSecurityValue(infra.ExecSecurity(mgr.GetEffectiveLevel())); level != "" {
			return string(level)
		}
	}
	return resolveBaseSecurityLevelWithConfig(cfg)
}

func resolveActiveMountRequests(mgr *EscalationManager, runID string) []runner.MountRequestForSandbox {
	if mgr == nil {
		return nil
	}
	mounts := mgr.GetActiveMountRequestsForRun(runID)
	if len(mounts) == 0 {
		return nil
	}
	result := make([]runner.MountRequestForSandbox, len(mounts))
	for i, mount := range mounts {
		result[i] = runner.MountRequestForSandbox{
			HostPath:  mount.HostPath,
			MountMode: mount.MountMode,
		}
	}
	return result
}

func resolveApprovalWaitOutcome(initialLevel, requestID string, status EscalationStatus) (done bool, approved bool) {
	if requestID != "" && status.HasPending && status.Pending != nil && status.Pending.ID == requestID {
		return false, false
	}

	if requestID != "" {
		for _, grant := range status.ActiveGrants {
			if grant != nil && grant.ID == requestID {
				return true, true
			}
		}
	}
	if requestID == "" && status.HasActive {
		return true, true
	}

	initial := infra.NormalizeExecSecurityValue(infra.ExecSecurity(initialLevel))
	if initial == "" {
		initial = infra.ExecSecurityDeny
	}
	current := infra.NormalizeExecSecurityValue(infra.ExecSecurity(status.ActiveLevel))
	if current == "" {
		current = infra.ExecSecurityDeny
	}
	return true, infra.LevelOrder(current) > infra.LevelOrder(initial)
}
