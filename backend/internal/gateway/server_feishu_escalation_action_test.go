package gateway

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
)

func TestHandleFeishuEscalationAction_RejectsMismatchedCardID(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_pending", "full", "need full", "", "", "", "", 30); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}

	state := &GatewayState{escalationMgr: mgr}
	resp, err := handleFeishuEscalationAction(state, "esc_old", "approve", map[string]interface{}{"ttl": float64(15)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "warning" {
		t.Fatalf("expected warning toast for mismatched card ID, got %+v", resp)
	}

	status := mgr.GetStatus()
	if !status.HasPending || status.Pending == nil || status.Pending.ID != "esc_pending" {
		t.Fatalf("pending request should remain unchanged, got %+v", status.Pending)
	}
	if status.HasActive {
		t.Fatalf("should not activate escalation on mismatched ID")
	}
}

func TestHandleFeishuEscalationAction_ApproveWithMatchingCardID(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_pending", "full", "need full", "", "", "", "", 30); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}

	state := &GatewayState{escalationMgr: mgr}
	resp, err := handleFeishuEscalationAction(state, "esc_pending", "approve", map[string]interface{}{"ttl": float64(15)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "success" {
		t.Fatalf("expected success toast for matching card ID, got %+v", resp)
	}

	status := mgr.GetStatus()
	if status.ActiveLevel != "full" {
		t.Fatalf("expected effective full level after approve, got %q", status.ActiveLevel)
	}
	if status.BaseLevel != "full" {
		t.Fatalf("expected permanent base full level, got %q", status.BaseLevel)
	}
	if status.HasActive {
		t.Fatalf("full approval should not create a temporary active grant, got %+v", status.Active)
	}
}

func TestHandleFeishuTypedApprovalAction_MountAccessApproveTaskScoped(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("full")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_mount", "sandboxed", "need mount", "run-1", "", "", "", 30, MountRequest{
		HostPath:  "/Users/test/Desktop",
		MountMode: "ro",
	}); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}

	state := &GatewayState{escalationMgr: mgr}
	resp, err := handleFeishuTypedApprovalAction(state, "esc_mount", ApprovalTypeMountAccess, "approve", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "success" {
		t.Fatalf("expected success toast, got %+v", resp)
	}

	status := mgr.GetStatus()
	if !status.HasActive || status.Active == nil {
		t.Fatalf("expected active grant after typed mount approval")
	}
	if len(status.Active.MountRequests) != 1 || status.Active.MountRequests[0].HostPath != "/Users/test/Desktop" {
		t.Fatalf("expected approved mount request to survive, got %+v", status.Active)
	}
	if !status.Active.TaskScoped {
		t.Fatalf("expected typed mount approval to create task-scoped grant, got %+v", status.Active)
	}
	if !status.Active.ExpiresAt.IsZero() {
		t.Fatalf("expected task-scoped typed mount approval to have no expiry, got %+v", status.Active)
	}
}

func TestHandleFeishuEscalationAction_TaskScopedMountIgnoresTTLOverride(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	mgr := NewEscalationManager(nil, nil, nil)
	mgr.SetMaxAllowedLevel("sandboxed")
	defer mgr.Close()

	if err := mgr.RequestEscalation("esc_mount", "sandboxed", "need mount", "run-1", "", "", "", 30, MountRequest{
		HostPath:  "/Users/test/Desktop",
		MountMode: "ro",
	}); err != nil {
		t.Fatalf("request escalation failed: %v", err)
	}

	state := &GatewayState{escalationMgr: mgr}
	resp, err := handleFeishuEscalationAction(state, "esc_mount", "approve", map[string]interface{}{"ttl": float64(30)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "success" {
		t.Fatalf("expected success toast, got %+v", resp)
	}

	status := mgr.GetStatus()
	if !status.HasActive || status.Active == nil {
		t.Fatalf("expected active grant after mount approval")
	}
	if !status.Active.TaskScoped {
		t.Fatalf("expected task-scoped grant, got %+v", status.Active)
	}
	if !status.Active.ExpiresAt.IsZero() {
		t.Fatalf("expected task-scoped grant to ignore ttl override, got %+v", status.Active)
	}
}

func TestHandleFeishuTypedApprovalAction_DataExportDeny(t *testing.T) {
	idCh := make(chan string, 1)
	confirmMgr := runner.NewCoderConfirmationManager(func(event string, payload interface{}) {
		if event != "coder.confirm.requested" {
			return
		}
		req, ok := payload.(runner.CoderConfirmationRequest)
		if !ok || req.ID == "" {
			return
		}
		select {
		case idCh <- req.ID:
		default:
		}
	}, nil, time.Second)

	done := make(chan bool, 1)
	go func() {
		approved, err := confirmMgr.RequestConfirmation(
			context.Background(),
			"send_media",
			[]byte(`{"target":"feishu:oc_target123","file_path":"/tmp/report.pdf"}`),
			"feishu:oc_origin",
		)
		if err != nil {
			t.Errorf("RequestConfirmation error: %v", err)
		}
		done <- approved
	}()

	var confirmID string
	select {
	case confirmID = <-idCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for coder confirmation request")
	}

	state := &GatewayState{coderConfirmMgr: confirmMgr}
	resp, err := handleFeishuTypedApprovalAction(state, confirmID, ApprovalTypeDataExport, "deny", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "info" {
		t.Fatalf("expected info toast, got %+v", resp)
	}
	if resp.Toast.Content != "❌ 数据导出已拒绝" {
		t.Fatalf("unexpected toast content: %+v", resp.Toast)
	}

	select {
	case approved := <-done:
		if approved {
			t.Fatal("expected send_media confirmation to be denied")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coder confirmation resolution")
	}
}
