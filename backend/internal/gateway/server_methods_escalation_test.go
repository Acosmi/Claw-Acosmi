package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

func setBaseSecurityLevelForTest(t *testing.T, level infra.ExecSecurity) {
	t.Helper()
	snapshot := infra.ReadExecApprovalsSnapshot()
	if snapshot == nil || snapshot.File == nil {
		t.Fatal("expected exec-approvals snapshot/file")
	}
	if snapshot.File.Defaults == nil {
		snapshot.File.Defaults = &infra.ExecApprovalsDefaults{}
	}
	snapshot.File.Defaults.Security = level
	if err := infra.SaveExecApprovals(snapshot.File); err != nil {
		t.Fatalf("save exec-approvals: %v", err)
	}
}

func setEscalationFallbackForTest(t *testing.T, mode infra.ExecEscalationFallback) {
	t.Helper()
	snapshot := infra.ReadExecApprovalsSnapshot()
	if snapshot == nil || snapshot.File == nil {
		t.Fatal("expected exec-approvals snapshot/file")
	}
	if snapshot.File.Defaults == nil {
		snapshot.File.Defaults = &infra.ExecApprovalsDefaults{}
	}
	snapshot.File.Defaults.EscalationFallback = mode
	if err := infra.SaveExecApprovals(snapshot.File); err != nil {
		t.Fatalf("save exec-approvals: %v", err)
	}
}

// ---------- Escalation Request ----------

func TestEscalationRequest_Success(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	bc := NewBroadcaster()
	auditLogger := NewEscalationAuditLogger()
	mgr := NewEscalationManager(bc, auditLogger, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	err := mgr.RequestEscalation("esc_001", "full", "Need write access", "run-1", "session-1", "", "", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := mgr.GetStatus()
	if !status.HasPending {
		t.Error("expected hasPending=true")
	}
	if status.Pending == nil || status.Pending.ID != "esc_001" {
		t.Error("pending request ID mismatch")
	}
	if status.Pending.RequestedLevel != "full" {
		t.Errorf("expected level 'full', got %q", status.Pending.RequestedLevel)
	}
}

func TestEscalationRequest_DuplicateRejected(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_001", "full", "reason", "", "", "", "", 30); err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}
	if err := mgr.RequestEscalation("esc_002", "full", "reason", "", "", "", "", 30); err == nil {
		t.Error("duplicate request should fail")
	}
}

func TestEscalationRequest_InvalidLevel(t *testing.T) {
	mgr := NewEscalationManager(nil, nil, nil)
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_001", "invalid", "reason", "", "", "", "", 30); err == nil {
		t.Error("invalid level should fail")
	}
}

func TestEscalationRequest_SameLevelSandboxedWithMountAllowed(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	setBaseSecurityLevelForTest(t, infra.ExecSecuritySandboxed)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	err := mgr.RequestEscalation(
		"esc_mount_001",
		"sandboxed",
		"Need temporary mount",
		"run-1",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: filepath.Join(tmpHome, "external"), MountMode: "rw"},
	)
	if err != nil {
		t.Fatalf("same-level mount extension should be allowed: %v", err)
	}

	status := mgr.GetStatus()
	if !status.HasPending || status.Pending == nil {
		t.Fatal("expected pending request")
	}
	if got := len(status.Pending.MountRequests); got != 1 {
		t.Fatalf("expected 1 mount request, got %d", got)
	}
	if status.Pending.MountRequests[0].MountMode != "rw" {
		t.Fatalf("expected rw mount mode, got %q", status.Pending.MountRequests[0].MountMode)
	}
	if !status.Pending.TaskScoped {
		t.Fatal("expected task-scoped mount request")
	}
	if status.Pending.TTLMinutes != 0 {
		t.Fatalf("expected task-scoped mount ttl=0, got %d", status.Pending.TTLMinutes)
	}
}

func TestEscalationRequest_SameLevelSandboxedWithoutMountRejected(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	setBaseSecurityLevelForTest(t, infra.ExecSecuritySandboxed)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	err := mgr.RequestEscalation("esc_no_mount_001", "sandboxed", "no mount needed", "", "", "", "", 30)
	if err == nil {
		t.Fatal("expected same-level request without mount to fail")
	}
	if !strings.Contains(err.Error(), "already satisfies") {
		t.Fatalf("expected base-satisfied error, got %v", err)
	}
}

func TestEscalationRequest_ActiveSandboxedAllowsMountExtension(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_active_001", "sandboxed", "need l2", "run-1", "", "", "", 30); err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 30); err != nil {
		t.Fatalf("initial approve failed: %v", err)
	}

	secondMount := filepath.Join(tmpHome, "second")
	if err := mgr.RequestEscalation(
		"esc_active_002",
		"sandboxed",
		"extend mount",
		"run-2",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: secondMount, MountMode: "rw"},
	); err != nil {
		t.Fatalf("mount extension request should be allowed with active sandboxed grant: %v", err)
	}

	status := mgr.GetStatus()
	if !status.HasActive {
		t.Fatal("expected existing active grant to remain")
	}
	if !status.HasPending || status.Pending == nil {
		t.Fatal("expected pending extension request")
	}
	if got := len(status.Pending.MountRequests); got != 1 {
		t.Fatalf("expected 1 pending mount request, got %d", got)
	}
	if !status.Pending.TaskScoped {
		t.Fatal("expected task-scoped pending mount request")
	}
	if status.Pending.RunID != "run-2" {
		t.Fatalf("expected pending runID run-2, got %q", status.Pending.RunID)
	}
	if status.Pending.TTLMinutes != 0 {
		t.Fatalf("expected task-scoped mount ttl=0, got %d", status.Pending.TTLMinutes)
	}
}

func TestEscalationRequest_ActiveAllowlistRejectsMountExtension(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_allow_active_001", "allowlist", "need l1", "", "", "", "", 30); err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 30); err != nil {
		t.Fatalf("initial approve failed: %v", err)
	}

	secondMount := filepath.Join(tmpHome, "second")
	if err := mgr.RequestEscalation(
		"esc_allow_active_002",
		"allowlist",
		"extend allowlist path",
		"run-2",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: secondMount, MountMode: "ro"},
	); err == nil {
		t.Fatal("expected allowlist mount extension to be rejected")
	} else if !strings.Contains(err.Error(), "mount access requires sandboxed level") {
		t.Fatalf("expected sandboxed-only mount error, got %v", err)
	}

	status := mgr.GetStatus()
	if !status.HasActive {
		t.Fatal("expected existing active grant to remain")
	}
	if status.HasPending {
		t.Fatal("expected no pending request after invalid allowlist mount attempt")
	}
}

func TestEscalationResolve_TaskScopedMountCreatesSeparateGrant(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	secondMount := filepath.Join(tmpHome, "second")

	if err := mgr.RequestEscalation(
		"esc_merge_001",
		"sandboxed",
		"initial sandbox grant",
		"run-1",
		"",
		"",
		"",
		30,
	); err != nil {
		t.Fatalf("initial sandbox request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 30); err != nil {
		t.Fatalf("initial sandbox approve failed: %v", err)
	}

	if err := mgr.RequestEscalation(
		"esc_merge_002",
		"sandboxed",
		"add mount",
		"run-2",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: secondMount, MountMode: "rw"},
	); err != nil {
		t.Fatalf("extension request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 30); err != nil {
		t.Fatalf("extension approve failed: %v", err)
	}

	status := mgr.GetStatus()
	if !status.HasActive || status.Active == nil {
		t.Fatal("expected active grant")
	}
	if got := len(status.ActiveGrants); got != 2 {
		t.Fatalf("expected 2 active grants, got %d", got)
	}
	if mounts := mgr.GetActiveMountRequestsForRun("run-1"); len(mounts) != 0 {
		t.Fatalf("expected run-1 to have no task-scoped mounts, got %+v", mounts)
	}
	mounts := mgr.GetActiveMountRequestsForRun("run-2")
	if len(mounts) != 1 || mounts[0].HostPath != secondMount {
		t.Fatalf("expected run-2 mount %q, got %+v", secondMount, mounts)
	}
}

func TestEscalationResolve_AllowlistGrantCoexistsWithTaskScopedSandboxMount(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	mountPath := filepath.Join(tmpHome, "task-mount")

	if err := mgr.RequestEscalation("esc_allow_001", "allowlist", "need l1", "", "", "", "", 30); err != nil {
		t.Fatalf("allowlist request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 30); err != nil {
		t.Fatalf("allowlist approve failed: %v", err)
	}

	if err := mgr.RequestEscalation(
		"esc_mount_001",
		"sandboxed",
		"need task mount",
		"run-2",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: mountPath, MountMode: "ro"},
	); err != nil {
		t.Fatalf("task-scoped mount request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 0); err != nil {
		t.Fatalf("task-scoped mount approve failed: %v", err)
	}

	status := mgr.GetStatus()
	if status.ActiveLevel != "sandboxed" {
		t.Fatalf("expected effective level sandboxed, got %q", status.ActiveLevel)
	}
	if got := len(status.ActiveGrants); got != 2 {
		t.Fatalf("expected 2 active grants, got %d", got)
	}
	if mounts := mgr.GetActiveMountRequestsForRun("run-2"); len(mounts) != 1 || mounts[0].HostPath != mountPath {
		t.Fatalf("expected run-2 mount %q, got %+v", mountPath, mounts)
	}
	if mounts := mgr.GetActiveMountRequestsForRun("run-other"); len(mounts) != 0 {
		t.Fatalf("expected unrelated run to see no task mount, got %+v", mounts)
	}
}

func TestEscalationResolve_TaskScopedMountsAreRunScoped(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	firstMount := filepath.Join(tmpHome, "first")
	secondMount := filepath.Join(tmpHome, "second")

	if err := mgr.RequestEscalation(
		"esc_scope_001",
		"sandboxed",
		"mount for run 1",
		"run-1",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: firstMount, MountMode: "ro"},
	); err != nil {
		t.Fatalf("run-1 mount request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 0); err != nil {
		t.Fatalf("run-1 mount approve failed: %v", err)
	}

	if err := mgr.RequestEscalation(
		"esc_scope_002",
		"sandboxed",
		"mount for run 2",
		"run-2",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: secondMount, MountMode: "rw"},
	); err != nil {
		t.Fatalf("run-2 mount request failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 0); err != nil {
		t.Fatalf("run-2 mount approve failed: %v", err)
	}

	if mounts := mgr.GetActiveMountRequests(); len(mounts) != 0 {
		t.Fatalf("expected global mount lookup to hide task-scoped grants, got %+v", mounts)
	}
	run1Mounts := mgr.GetActiveMountRequestsForRun("run-1")
	if len(run1Mounts) != 1 || run1Mounts[0].HostPath != firstMount {
		t.Fatalf("expected run-1 mount %q, got %+v", firstMount, run1Mounts)
	}
	run2Mounts := mgr.GetActiveMountRequestsForRun("run-2")
	if len(run2Mounts) != 1 || run2Mounts[0].HostPath != secondMount {
		t.Fatalf("expected run-2 mount %q, got %+v", secondMount, run2Mounts)
	}
}

func TestEscalationRestoreFromDisk_PreservesMountRequests(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr1 := NewEscalationManager(nil, nil, nil)
	mgr1.SetMaxAllowedLevel("sandboxed")
	mountPath := filepath.Join(tmpHome, "persisted")
	if err := mgr1.RequestEscalation(
		"esc_restore_001",
		"sandboxed",
		"persist mount request",
		"run-restore",
		"",
		"",
		"",
		30,
		MountRequest{HostPath: mountPath, MountMode: "rw"},
	); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	mgr1.Close()

	mgr2 := NewEscalationManager(nil, nil, nil)
	mgr2.SetMaxAllowedLevel("sandboxed")
	defer mgr2.Close()
	mgr2.RestoreFromDisk()

	status := mgr2.GetStatus()
	if !status.HasPending || status.Pending == nil {
		t.Fatal("expected restored pending request")
	}
	if got := len(status.Pending.MountRequests); got != 1 {
		t.Fatalf("expected 1 restored mount request, got %d", got)
	}
	if status.Pending.MountRequests[0].HostPath != mountPath {
		t.Fatalf("unexpected restored mount path: %q", status.Pending.MountRequests[0].HostPath)
	}
	if !status.Pending.TaskScoped {
		t.Fatal("expected restored mount request to remain task-scoped")
	}
}

// ---------- Escalation Approve ----------

func TestEscalationApprove(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	mgr.RequestEscalation("esc_001", "full", "reason", "run-1", "", "", "", 30)

	if err := mgr.ResolveEscalation(true, 15); err != nil {
		t.Fatalf("approve should succeed: %v", err)
	}

	status := mgr.GetStatus()
	if status.HasPending {
		t.Error("pending should be cleared after approve")
	}
	if status.HasActive {
		t.Error("full approval should not create temporary active grant")
	}
	if status.BaseLevel != "full" {
		t.Errorf("expected baseLevel 'full', got %q", status.BaseLevel)
	}
	if status.ActiveLevel != "full" {
		t.Errorf("expected activeLevel 'full', got %q", status.ActiveLevel)
	}
}

func TestEscalationApprove_BaseRaisedDuringPendingSkipsTemporaryGrant(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_perm_001", "sandboxed", "reason", "run-1", "", "", "", 30); err != nil {
		t.Fatalf("request should succeed: %v", err)
	}
	setBaseSecurityLevelForTest(t, infra.ExecSecurityFull)

	if err := mgr.ResolveEscalation(true, 0); err != nil {
		t.Fatalf("approve should succeed: %v", err)
	}

	status := mgr.GetStatus()
	if status.HasPending {
		t.Error("pending should be cleared after approve")
	}
	if status.HasActive {
		t.Error("base-satisfied approval should not create temporary active grant")
	}
	if status.BaseLevel != "full" || status.ActiveLevel != "full" {
		t.Fatalf("expected effective level to stay full, got base=%q active=%q", status.BaseLevel, status.ActiveLevel)
	}
}

// ---------- Escalation Deny ----------

func TestEscalationDeny(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	defer mgr.Close()

	mgr.RequestEscalation("esc_001", "allowlist", "reason", "", "", "", "", 30)

	if err := mgr.ResolveEscalation(false, 0); err != nil {
		t.Fatalf("deny should succeed: %v", err)
	}

	status := mgr.GetStatus()
	if status.HasPending {
		t.Error("pending should be cleared after deny")
	}
	if status.HasActive {
		t.Error("should NOT have active grant after deny")
	}
}

// ---------- Auto De-escalate (TTL) ----------

func TestEscalationAutoDeescalate(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	mgr.RequestEscalation("esc_001", "sandboxed", "reason", "", "", "", "", 1) // 1 minute TTL

	// Override TTL to very short for testing
	mgr.mu.Lock()
	mgr.pending = nil
	mgr.activeGrants = []*ActiveEscalationGrant{{
		ID:        "esc_001",
		Level:     "sandboxed",
		GrantedAt: time.Now(),
		ExpiresAt: time.Now().Add(100 * time.Millisecond),
	}}
	mgr.rescheduleDeescalateTimerLocked()
	mgr.mu.Unlock()

	// Wait for auto-deescalation
	time.Sleep(300 * time.Millisecond)

	status := mgr.GetStatus()
	if status.HasActive {
		t.Error("should not have active grant after TTL expiry")
	}
}

func TestEscalationApprove_FullIgnoresFallbackBaseMode(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	setBaseSecurityLevelForTest(t, infra.ExecSecurityAllowlist)
	setEscalationFallbackForTest(t, infra.ExecEscalationFallbackBase)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_fb_base", "full", "reason", "", "", "", "", 30); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 15); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	status := mgr.GetStatus()
	if status.BaseLevel != string(infra.ExecSecurityFull) {
		t.Fatalf("expected base level upgraded to full, got %q", status.BaseLevel)
	}
	if status.ActiveLevel != string(infra.ExecSecurityFull) {
		t.Fatalf("expected active level stay full, got %q", status.ActiveLevel)
	}
}

func TestEscalationApprove_FullIgnoresFallbackSandboxedMode(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	setBaseSecurityLevelForTest(t, infra.ExecSecurityDeny)
	setEscalationFallbackForTest(t, infra.ExecEscalationFallbackSandboxed)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_fb_sandboxed", "full", "reason", "", "", "", "", 30); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 15); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	status := mgr.GetStatus()
	if status.BaseLevel != string(infra.ExecSecurityFull) {
		t.Fatalf("expected base level upgraded to full, got %q", status.BaseLevel)
	}
	if status.ActiveLevel != string(infra.ExecSecurityFull) {
		t.Fatalf("expected active level stay full, got %q", status.ActiveLevel)
	}
}

func TestEscalationApprove_MountAccessOnBaseFullBypassesApproval(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	setBaseSecurityLevelForTest(t, infra.ExecSecurityFull)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	mountDir := t.TempDir()
	if err := mgr.RequestEscalationWithMetadata(EscalationRequestOptions{
		ID:         "esc_mount_full_base",
		Level:      string(infra.ExecSecuritySandboxed),
		Reason:     "temporary mount access",
		RunID:      "run-1",
		TTLMinutes: 30,
		MountRequests: []MountRequest{
			{HostPath: mountDir, MountMode: "ro"},
		},
	}); err == nil {
		t.Fatal("expected base full security to bypass mount approval request")
	}

	status := mgr.GetStatus()
	if status.HasActive || status.Active != nil {
		t.Fatal("expected no active mount grant when base level is full")
	}
	if status.BaseLevel != string(infra.ExecSecurityFull) {
		t.Fatalf("expected base level to remain full, got %q", status.BaseLevel)
	}
	if status.ActiveLevel != string(infra.ExecSecurityFull) {
		t.Fatalf("effective level = %q, want full", status.ActiveLevel)
	}
	if got := mgr.GetEffectiveLevel(); got != string(infra.ExecSecurityFull) {
		t.Fatalf("GetEffectiveLevel() = %q, want full", got)
	}
	mounts := mgr.GetActiveMountRequests()
	if len(mounts) != 0 {
		t.Fatalf("active mounts = %+v, want none", mounts)
	}
}

// ---------- Task Complete ----------

func TestEscalationTaskComplete_ClearsPendingTaskScopedMountGrant(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	mountDir := t.TempDir()
	if err := mgr.RequestEscalationWithMetadata(EscalationRequestOptions{
		ID:         "esc_pending_mount",
		Level:      "sandboxed",
		Reason:     "reason",
		RunID:      "run-1",
		TTLMinutes: 30,
		MountRequests: []MountRequest{
			{HostPath: mountDir, MountMode: "ro"},
		},
	}); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}

	mgr.TaskComplete("run-1")

	status := mgr.GetStatus()
	if status.HasPending || status.Pending != nil {
		t.Fatalf("expected pending task-scoped mount request to be cleared, got %+v", status.Pending)
	}
	if status.HasActive {
		t.Fatal("expected no active grant after clearing pending task-scoped mount request")
	}
}

func TestEscalationTaskComplete_TaskScopedMountGrant(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	mountDir := t.TempDir()
	if err := mgr.RequestEscalationWithMetadata(EscalationRequestOptions{
		ID:         "esc_001",
		Level:      "sandboxed",
		Reason:     "reason",
		RunID:      "run-1",
		TTLMinutes: 30,
		MountRequests: []MountRequest{
			{HostPath: mountDir, MountMode: "ro"},
		},
	}); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}

	status := mgr.GetStatus()
	if status.Pending == nil || !status.Pending.TaskScoped {
		t.Fatal("expected pending task-scoped mount request")
	}
	if status.Pending.TTLMinutes != 0 {
		t.Fatalf("pending TTLMinutes = %d, want 0 for task-scoped mount grant", status.Pending.TTLMinutes)
	}
	if err := mgr.ResolveEscalation(true, 0); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	// Task complete with matching runID
	mgr.TaskComplete("run-1")

	status = mgr.GetStatus()
	if status.HasActive {
		t.Error("should not have active grant after task complete")
	}
}

func TestEscalationTaskComplete_TaskScopedMountGrantWrongRunID(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	mountDir := t.TempDir()
	if err := mgr.RequestEscalationWithMetadata(EscalationRequestOptions{
		ID:         "esc_001",
		Level:      "sandboxed",
		Reason:     "reason",
		RunID:      "run-1",
		TTLMinutes: 30,
		MountRequests: []MountRequest{
			{HostPath: mountDir, MountMode: "ro"},
		},
	}); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 0); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	// Task complete with wrong runID should NOT deescalate
	mgr.TaskComplete("run-other")

	status := mgr.GetStatus()
	if !status.HasActive {
		t.Error("should still have active task-scoped grant (wrong runID)")
	}
	if status.Active == nil || !status.Active.TaskScoped {
		t.Fatal("expected active task-scoped mount grant")
	}
}

func TestEscalationTaskComplete_IgnoresNonTaskScopedGrant(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_001", "sandboxed", "reason", "run-1", "", "", "", 30); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}
	if err := mgr.ResolveEscalation(true, 30); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	mgr.TaskComplete("run-1")

	status := mgr.GetStatus()
	if !status.HasActive {
		t.Error("non-task-scoped grant should remain active after task complete")
	}
}

// ---------- Audit Logger ----------

func TestEscalationAuditLog(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	logger := NewEscalationAuditLogger()

	logger.Log(EscalationAuditEntry{
		Timestamp:      time.Now(),
		Event:          AuditEventRequest,
		RequestID:      "esc_001",
		RequestedLevel: "full",
		Reason:         "test reason",
		TTLMinutes:     30,
	})
	logger.Log(EscalationAuditEntry{
		Timestamp:      time.Now(),
		Event:          AuditEventApprove,
		RequestID:      "esc_001",
		RequestedLevel: "full",
		TTLMinutes:     30,
	})

	entries, err := logger.ReadRecent(10)
	if err != nil {
		t.Fatalf("read recent failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Most recent first
	if entries[0].Event != AuditEventApprove {
		t.Errorf("expected first entry to be 'approve', got %q", entries[0].Event)
	}
	if entries[1].Event != AuditEventRequest {
		t.Errorf("expected second entry to be 'request', got %q", entries[1].Event)
	}

	// Verify file exists
	auditFile := filepath.Join(tmpHome, ".openacosmi", "escalation-audit.log")
	if _, err := os.Stat(auditFile); os.IsNotExist(err) {
		t.Error("audit log file should exist")
	}
}

// ---------- Gateway Method Handlers ----------

func TestEscalationHandlers_RequestAndStatus(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	bc := NewBroadcaster()
	auditLogger := NewEscalationAuditLogger()
	mgr := NewEscalationManager(bc, auditLogger, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	r := NewMethodRegistry()
	r.RegisterAll(EscalationHandlers())

	// 1. Request escalation
	req := &RequestFrame{Method: "security.escalation.request", Params: map[string]interface{}{
		"level":      "full",
		"reason":     "Need full access for deployment",
		"ttlMinutes": float64(15),
	}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{EscalationMgr: mgr}, respond)
	if !gotOK {
		t.Fatal("request should succeed")
	}
	result := gotPayload.(map[string]interface{})
	if result["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", result["status"])
	}
	id, _ := result["id"].(string)
	if !strings.HasPrefix(id, "esc_") {
		t.Errorf("expected ID prefix 'esc_', got %q", id)
	}

	// 2. Check status
	req = &RequestFrame{Method: "security.escalation.status", Params: map[string]interface{}{}}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{EscalationMgr: mgr}, respond)
	if !gotOK {
		t.Fatal("status should succeed")
	}
	statusResult := gotPayload.(EscalationStatus)
	if !statusResult.HasPending {
		t.Error("expected hasPending=true")
	}
}

func TestEscalationHandlers_RequestMountAccessReturnsTaskScoped(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	r := NewMethodRegistry()
	r.RegisterAll(EscalationHandlers())

	mountDir := t.TempDir()
	req := &RequestFrame{Method: "security.escalation.request", Params: map[string]interface{}{
		"level":      "sandboxed",
		"reason":     "Need temporary workspace mount",
		"runId":      "run-1",
		"ttlMinutes": float64(30),
		"mountRequests": []interface{}{
			map[string]interface{}{"hostPath": mountDir, "mountMode": "ro"},
		},
	}}

	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{EscalationMgr: mgr}, respond)
	if !gotOK {
		t.Fatal("request should succeed")
	}

	result := gotPayload.(map[string]interface{})
	if result["status"] != "pending" {
		t.Fatalf("expected status pending, got %v", result["status"])
	}
	if taskScoped, _ := result["taskScoped"].(bool); !taskScoped {
		t.Fatalf("expected taskScoped=true, got %v", result["taskScoped"])
	}
	if ttl, _ := result["ttlMinutes"].(int); ttl != 0 {
		t.Fatalf("expected ttlMinutes=0 for task-scoped mount request, got %v", result["ttlMinutes"])
	}
}

func TestEscalationHandlers_Resolve(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	r := NewMethodRegistry()
	r.RegisterAll(EscalationHandlers())

	// Setup: create a pending request
	mgr.RequestEscalation("esc_001", "full", "reason", "", "", "", "", 30)

	// Approve
	req := &RequestFrame{Method: "security.escalation.resolve", Params: map[string]interface{}{
		"approve":    true,
		"ttlMinutes": float64(30),
	}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{EscalationMgr: mgr}, respond)
	if !gotOK {
		t.Fatal("resolve should succeed")
	}
	result := gotPayload.(map[string]interface{})
	if result["status"] != "approved" {
		t.Errorf("expected status 'approved', got %v", result["status"])
	}

	// Verify active grant
	status := mgr.GetStatus()
	if status.HasActive {
		t.Error("full approval should not leave temporary active grant")
	}
	if status.BaseLevel != "full" {
		t.Errorf("expected base level to be full after approval, got %q", status.BaseLevel)
	}
}

func TestEscalationHandlers_Revoke(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	r := NewMethodRegistry()
	r.RegisterAll(EscalationHandlers())

	// Setup: request + approve
	mgr.RequestEscalation("esc_001", "full", "reason", "", "", "", "", 30)
	mgr.ResolveEscalation(true, 30)

	// Revoke
	req := &RequestFrame{Method: "security.escalation.revoke", Params: map[string]interface{}{}}
	var gotOK bool
	respond := func(ok bool, _ interface{}, _ *ErrorShape) {
		gotOK = ok
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{EscalationMgr: mgr}, respond)
	if !gotOK {
		t.Fatal("revoke should succeed")
	}

	status := mgr.GetStatus()
	if status.HasActive {
		t.Error("should not have active grant after revoke")
	}
}

func TestEscalationHandlers_ResolveNoPending(t *testing.T) {
	mgr := NewEscalationManager(nil, nil, nil)
	defer mgr.Close()

	r := NewMethodRegistry()
	r.RegisterAll(EscalationHandlers())

	req := &RequestFrame{Method: "security.escalation.resolve", Params: map[string]interface{}{
		"approve": true,
	}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{EscalationMgr: mgr}, respond)
	if gotOK {
		t.Error("resolve without pending should fail")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request error, got %v", gotErr)
	}
}
