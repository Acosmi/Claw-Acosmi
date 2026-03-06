package gateway

import (
	"os"
	"testing"
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
	if !status.HasActive || status.Active == nil {
		t.Fatalf("expected active grant after approve")
	}
	if status.Active.Level != "full" {
		t.Fatalf("expected full active level, got %q", status.Active.Level)
	}
}
