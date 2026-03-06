package gateway

import (
	"os"
	"testing"
	"time"
)

// ---------- RemoteApprovalNotifier ----------

func TestRemoteApprovalNotifier_DefaultConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)
	cfg := n.GetConfig()

	if cfg.Enabled {
		t.Error("expected disabled by default")
	}
	if len(n.EnabledProviderNames()) != 0 {
		t.Errorf("expected 0 providers, got %d", len(n.EnabledProviderNames()))
	}
}

func TestRemoteApprovalNotifier_UpdateConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)

	cfg := RemoteApprovalConfig{
		Enabled:     true,
		CallbackURL: "https://example.com/callback",
		DingTalk: &DingTalkProviderConfig{
			Enabled:    true,
			WebhookURL: "https://oapi.dingtalk.com/robot/send?access_token=test",
		},
	}

	if err := n.UpdateConfig(cfg); err != nil {
		t.Fatalf("update config failed: %v", err)
	}

	providers := n.EnabledProviderNames()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0] != "dingtalk" {
		t.Errorf("expected 'dingtalk', got %q", providers[0])
	}

	// Reload from disk
	n2 := NewRemoteApprovalNotifier(nil)
	cfg2 := n2.GetConfig()
	if !cfg2.Enabled {
		t.Error("expected enabled after reload")
	}
	if cfg2.CallbackURL != "https://example.com/callback" {
		t.Errorf("callback URL mismatch: %q", cfg2.CallbackURL)
	}
}

func TestRemoteApprovalNotifier_SanitizedConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)
	cfg := RemoteApprovalConfig{
		Enabled: true,
		Feishu: &FeishuProviderConfig{
			Enabled:   true,
			AppID:     "cli_test",
			AppSecret: "supersecret123",
			ChatID:    "oc_test",
		},
	}
	n.UpdateConfig(cfg)

	sanitized := n.GetConfigSanitized()
	if sanitized.Feishu.AppSecret != "***" {
		t.Errorf("expected sanitized secret '***', got %q", sanitized.Feishu.AppSecret)
	}
	if sanitized.Feishu.AppID != "cli_test" {
		t.Errorf("appId should not be sanitized: %q", sanitized.Feishu.AppID)
	}
}

func TestRemoteApprovalNotifier_NotifyAllNoProviders(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)
	// Should not panic with no providers
	n.NotifyAll(ApprovalCardRequest{
		EscalationID:   "test_001",
		RequestedLevel: "full",
		Reason:         "test",
		TTLMinutes:     30,
		RequestedAt:    time.Now(),
	})
}

func TestRemoteApprovalNotifier_UpdateLastKnownFeishuTarget(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)
	// Inject feishu channel config (simulating server startup)
	n.InjectChannelFeishuConfig("cli_test123", "secret_test", "")

	// Initially no LastKnown values
	cfg := n.GetConfig()
	if cfg.Feishu.LastKnownChatID != "" {
		t.Errorf("expected empty LastKnownChatID, got %q", cfg.Feishu.LastKnownChatID)
	}

	// Simulate receiving a feishu message
	n.UpdateLastKnownFeishuTarget("oc_test_chat_123", "ou_test_user_456")

	cfg = n.GetConfig()
	if cfg.Feishu.LastKnownChatID != "oc_test_chat_123" {
		t.Errorf("expected LastKnownChatID='oc_test_chat_123', got %q", cfg.Feishu.LastKnownChatID)
	}
	if cfg.Feishu.LastKnownUserID != "ou_test_user_456" {
		t.Errorf("expected LastKnownUserID='ou_test_user_456', got %q", cfg.Feishu.LastKnownUserID)
	}

	// Simulate restart: create new notifier and verify persistence
	n2 := NewRemoteApprovalNotifier(nil)
	cfg2 := n2.GetConfig()
	if cfg2.Feishu == nil {
		t.Fatal("Feishu config should be persisted after restart")
	}
	if cfg2.Feishu.LastKnownChatID != "oc_test_chat_123" {
		t.Errorf("after restart: expected LastKnownChatID='oc_test_chat_123', got %q", cfg2.Feishu.LastKnownChatID)
	}
	if cfg2.Feishu.LastKnownUserID != "ou_test_user_456" {
		t.Errorf("after restart: expected LastKnownUserID='ou_test_user_456', got %q", cfg2.Feishu.LastKnownUserID)
	}
}

func TestRemoteApprovalNotifier_UpdateLastKnownIdempotent(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)
	n.InjectChannelFeishuConfig("cli_test", "secret", "")

	// First update
	n.UpdateLastKnownFeishuTarget("oc_chat", "ou_user")
	// Same values — should be idempotent (no extra disk write)
	n.UpdateLastKnownFeishuTarget("oc_chat", "ou_user")
	// Empty values — should be ignored
	n.UpdateLastKnownFeishuTarget("", "")

	cfg := n.GetConfig()
	if cfg.Feishu.LastKnownChatID != "oc_chat" {
		t.Errorf("expected 'oc_chat', got %q", cfg.Feishu.LastKnownChatID)
	}
}

func TestRemoteApprovalNotifier_InjectPreservesLastKnown(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Step 1: Create notifier, inject config, learn target
	n1 := NewRemoteApprovalNotifier(nil)
	n1.InjectChannelFeishuConfig("cli_app", "secret_app", "")
	n1.UpdateLastKnownFeishuTarget("oc_preserved", "ou_preserved")

	// Step 2: Simulate restart — new notifier loads from disk
	// Then InjectChannelFeishuConfig is called again (as server.go does on startup)
	n2 := NewRemoteApprovalNotifier(nil)
	// The loaded config already has Feishu enabled with AppID, so Inject should skip
	n2.InjectChannelFeishuConfig("cli_app", "secret_app", "")

	cfg := n2.GetConfig()
	if cfg.Feishu.LastKnownChatID != "oc_preserved" {
		t.Errorf("Inject should preserve LastKnownChatID, got %q", cfg.Feishu.LastKnownChatID)
	}
}

func TestRemoteApprovalNotifier_UpdateConfig_PreservesFeishuFallbackFields(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	n := NewRemoteApprovalNotifier(nil)
	n.InjectChannelFeishuConfig("cli_app", "secret_app", "oc_approval_group")
	n.UpdateLastKnownFeishuTarget("oc_last_chat", "ou_last_user")

	if err := n.UpdateConfig(RemoteApprovalConfig{
		Enabled:     true,
		CallbackURL: "https://example.com/callback",
		Feishu: &FeishuProviderConfig{
			Enabled:   true,
			AppID:     "cli_app",
			AppSecret: "***",
			ChatID:    "oc_new_chat",
			UserID:    "ou_new_user",
			// 故意不回传 ApprovalChatID / LastKnown*，验证不会被覆盖清空。
		},
	}); err != nil {
		t.Fatalf("update config failed: %v", err)
	}

	cfg := n.GetConfig()
	if cfg.Feishu == nil {
		t.Fatal("expected feishu config after update")
	}
	if cfg.Feishu.ApprovalChatID != "oc_approval_group" {
		t.Fatalf("approvalChatId should be preserved, got %q", cfg.Feishu.ApprovalChatID)
	}
	if cfg.Feishu.LastKnownChatID != "oc_last_chat" {
		t.Fatalf("lastKnownChatId should be preserved, got %q", cfg.Feishu.LastKnownChatID)
	}
	if cfg.Feishu.LastKnownUserID != "ou_last_user" {
		t.Fatalf("lastKnownUserId should be preserved, got %q", cfg.Feishu.LastKnownUserID)
	}
}

// ---------- TaskPresetManager ----------

func TestTaskPresetManager_AddAndList(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()

	preset := TaskPreset{
		ID:          "tp_test_001",
		Name:        "Deploy Tasks",
		Pattern:     "deploy*",
		Level:       "full",
		AutoApprove: true,
		MaxTTL:      60,
		Description: "Auto-approve deploy tasks",
	}

	if err := m.Add(preset); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	list := m.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(list))
	}
	if list[0].Name != "Deploy Tasks" {
		t.Errorf("name mismatch: %q", list[0].Name)
	}
}

func TestTaskPresetManager_Update(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	m.Add(TaskPreset{ID: "tp_001", Name: "Test", Pattern: "test*", Level: "sandbox", MaxTTL: 30})

	err := m.Update("tp_001", TaskPreset{Name: "Updated", Level: "full", MaxTTL: 120})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	list := m.List()
	if list[0].Name != "Updated" {
		t.Errorf("expected 'Updated', got %q", list[0].Name)
	}
	if list[0].Level != "full" {
		t.Errorf("expected 'full', got %q", list[0].Level)
	}
}

func TestTaskPresetManager_Remove(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	m.Add(TaskPreset{ID: "tp_001", Name: "Test", Pattern: "test*", Level: "sandbox", MaxTTL: 30})
	m.Add(TaskPreset{ID: "tp_002", Name: "Test2", Pattern: "test2*", Level: "full", MaxTTL: 60})

	if err := m.Remove("tp_001"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if len(m.List()) != 1 {
		t.Errorf("expected 1 preset after remove, got %d", len(m.List()))
	}
}

func TestTaskPresetManager_MatchExact(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	m.Add(TaskPreset{ID: "tp_001", Name: "Deploy", Pattern: "deploy-production", Level: "full", MaxTTL: 60})

	result := m.Match("deploy-production")
	if !result.Matched {
		t.Error("expected exact match")
	}
	if result.MatchedBy != "exact" {
		t.Errorf("expected matchedBy='exact', got %q", result.MatchedBy)
	}
}

func TestTaskPresetManager_MatchPrefix(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	m.Add(TaskPreset{ID: "tp_001", Name: "Deploy", Pattern: "deploy-*", Level: "full", MaxTTL: 60})

	result := m.Match("deploy-staging")
	if !result.Matched {
		t.Error("expected prefix match")
	}
	if result.MatchedBy != "prefix" {
		t.Errorf("expected matchedBy='prefix', got %q", result.MatchedBy)
	}
}

func TestTaskPresetManager_MatchGlob(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	m.Add(TaskPreset{ID: "tp_001", Name: "Build", Pattern: "ci-*-build", Level: "sandbox", MaxTTL: 30})

	result := m.Match("ci-frontend-build")
	if !result.Matched {
		t.Error("expected glob match")
	}
	if result.MatchedBy != "glob" {
		t.Errorf("expected matchedBy='glob', got %q", result.MatchedBy)
	}
}

func TestTaskPresetManager_NoMatch(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	m.Add(TaskPreset{ID: "tp_001", Name: "Deploy", Pattern: "deploy-*", Level: "full", MaxTTL: 60})

	result := m.Match("test-something")
	if result.Matched {
		t.Error("should not match")
	}
}

func TestTaskPresetManager_InvalidLevel(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m := NewTaskPresetManager()
	err := m.Add(TaskPreset{ID: "tp_001", Name: "Test", Pattern: "test*", Level: "invalid", MaxTTL: 30})
	if err == nil {
		t.Error("expected error for invalid level")
	}
}

func TestTaskPresetManager_Persistence(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	m1 := NewTaskPresetManager()
	m1.Add(TaskPreset{ID: "tp_001", Name: "Test", Pattern: "test*", Level: "sandbox", MaxTTL: 30})
	m1.Add(TaskPreset{ID: "tp_002", Name: "Deploy", Pattern: "deploy*", Level: "full", MaxTTL: 90})

	// Load from disk
	m2 := NewTaskPresetManager()
	list := m2.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 presets after reload, got %d", len(list))
	}
}

// ---------- GlobMatch ----------

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*", "anything", true},
		{"deploy-*", "deploy-prod", true},
		{"deploy-*", "test-prod", false},
		{"ci-*-build", "ci-frontend-build", true},
		{"ci-*-build", "ci-build", false},
		{"exact", "exact", true},
		{"exact", "other", false},
		{"?est", "test", true},
		{"?est", "best", true},
		{"?est", "invalid", false},
	}
	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}
