package argus

import (
	"os"
	"testing"
	"time"
)

// ---------- TCCStatus 方法测试 ----------

func TestTCCStatus_HasRequiredPermissions_AllGranted(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermGranted, Accessibility: PermGranted}
	if !s.HasRequiredPermissions() {
		t.Error("expected true when all granted")
	}
}

func TestTCCStatus_HasRequiredPermissions_ScreenDenied(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermDenied, Accessibility: PermGranted}
	if s.HasRequiredPermissions() {
		t.Error("expected false when screen recording denied")
	}
}

func TestTCCStatus_HasRequiredPermissions_AccessibilityDenied(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermGranted, Accessibility: PermDenied}
	if s.HasRequiredPermissions() {
		t.Error("expected false when accessibility denied")
	}
}

func TestTCCStatus_HasRequiredPermissions_BothDenied(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermDenied, Accessibility: PermDenied}
	if s.HasRequiredPermissions() {
		t.Error("expected false when both denied")
	}
}

func TestTCCStatus_HasRequiredPermissions_Unknown(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermUnknown, Accessibility: PermGranted}
	if s.HasRequiredPermissions() {
		t.Error("expected false when screen recording unknown")
	}
}

func TestTCCStatus_Recovery_AllGranted(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermGranted, Accessibility: PermGranted}
	if r := s.Recovery(); r != "" {
		t.Errorf("expected empty recovery, got %q", r)
	}
}

func TestTCCStatus_Recovery_ScreenDenied(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermDenied, Accessibility: PermGranted}
	r := s.Recovery()
	if r == "" {
		t.Error("expected non-empty recovery")
	}
	if !contains(r, "Screen Recording") {
		t.Errorf("recovery should mention Screen Recording: %q", r)
	}
}

func TestTCCStatus_Recovery_BothDenied(t *testing.T) {
	s := TCCStatus{ScreenRecording: PermDenied, Accessibility: PermDenied}
	r := s.Recovery()
	if !contains(r, "Screen Recording") || !contains(r, "Accessibility") {
		t.Errorf("recovery should mention both: %q", r)
	}
}

func TestTCCStatus_Recovery_Expiring(t *testing.T) {
	s := TCCStatus{
		ScreenRecording:         PermGranted,
		Accessibility:           PermGranted,
		ScreenRecordingExpiring: true,
		ScreenRecordingDaysLeft: 3,
	}
	r := s.Recovery()
	if r == "" {
		t.Error("expected non-empty recovery for expiring permission")
	}
	if !contains(r, "3 days") {
		t.Errorf("recovery should mention days left: %q", r)
	}
}

func TestTCCStatus_Recovery_HelperAccessibilityUnknown(t *testing.T) {
	s := TCCStatus{
		ScreenRecording:   PermGranted,
		Accessibility:     PermUnknown,
		PermissionSubject: "helper_process",
	}
	r := s.Recovery()
	if !contains(r, "Argus helper") {
		t.Errorf("recovery should mention Argus helper: %q", r)
	}
}

// ---------- Sequoia 过期计算测试 ----------

func TestSequoiaExpiryDaysFromModTime_Fresh(t *testing.T) {
	// 刚授权 — 应有约 30 天剩余
	modTime := time.Now()
	days := SequoiaExpiryDaysFromModTime(modTime)
	if days < 29 || days > 30 {
		t.Errorf("expected ~30 days for fresh authorization, got %d", days)
	}
}

func TestSequoiaExpiryDaysFromModTime_HalfExpired(t *testing.T) {
	// 15 天前授权 — 应有约 15 天剩余
	modTime := time.Now().Add(-15 * 24 * time.Hour)
	days := SequoiaExpiryDaysFromModTime(modTime)
	if days < 14 || days > 16 {
		t.Errorf("expected ~15 days for 15-day old authorization, got %d", days)
	}
}

func TestSequoiaExpiryDaysFromModTime_AlmostExpired(t *testing.T) {
	// 28 天前授权 — 应有约 2 天剩余
	modTime := time.Now().Add(-28 * 24 * time.Hour)
	days := SequoiaExpiryDaysFromModTime(modTime)
	if days < 1 || days > 3 {
		t.Errorf("expected ~2 days for 28-day old authorization, got %d", days)
	}
}

func TestSequoiaExpiryDaysFromModTime_Expired(t *testing.T) {
	// 35 天前授权 — 已过期
	modTime := time.Now().Add(-35 * 24 * time.Hour)
	days := SequoiaExpiryDaysFromModTime(modTime)
	if days != 0 {
		t.Errorf("expected 0 for expired authorization, got %d", days)
	}
}

// ---------- CheckTCCPermissions 集成测试 ----------

func TestCheckTCCPermissions_NoError(t *testing.T) {
	// 不论平台，CheckTCCPermissions 不应 panic
	status := CheckTCCPermissions()
	t.Logf("TCC status: screen=%s, accessibility=%s, days_left=%d",
		status.ScreenRecording, status.Accessibility, status.ScreenRecordingDaysLeft)

	// 验证返回值是有效的 PermissionState
	validStates := map[PermissionState]bool{PermGranted: true, PermDenied: true, PermUnknown: true}
	if !validStates[status.ScreenRecording] {
		t.Errorf("invalid screen recording state: %q", status.ScreenRecording)
	}
	if !validStates[status.Accessibility] {
		t.Errorf("invalid accessibility state: %q", status.Accessibility)
	}
}

func TestDetectPermissionSubject_CurrentProcess(t *testing.T) {
	execPath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	subject, path := detectPermissionSubject(execPath)
	if subject != "main_app" {
		t.Fatalf("expected main_app, got %q", subject)
	}
	if path == "" {
		t.Fatal("expected non-empty permission path")
	}
}

func TestDetectPermissionSubject_HelperProcess(t *testing.T) {
	subject, path := detectPermissionSubject("/tmp/argus-sensory")
	if subject != "helper_process" {
		t.Fatalf("expected helper_process, got %q", subject)
	}
	if path == "" {
		t.Fatal("expected non-empty helper path")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
