package scope

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func boolPtr(b bool) *bool { return &b }

func TestListAgentIds_Empty(t *testing.T) {
	ids := ListAgentIds(nil)
	if len(ids) != 1 || ids[0] != DefaultAgentID {
		t.Errorf("ListAgentIds(nil) = %v, want [default]", ids)
	}
}

func TestListAgentIds_WithAgents(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "Bot1"},
				{ID: "BOT1"}, // 重复 (case-insensitive)
				{ID: "helper"},
			},
		},
	}
	ids := ListAgentIds(cfg)
	if len(ids) != 2 {
		t.Fatalf("ListAgentIds = %v, want 2 unique", ids)
	}
	if ids[0] != "bot1" || ids[1] != "helper" {
		t.Errorf("ListAgentIds = %v, want [bot1, helper]", ids)
	}
}

func TestResolveDefaultAgentId(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "first"},
				{ID: "second", Default: boolPtr(true)},
			},
		},
	}
	id := ResolveDefaultAgentId(cfg)
	if id != "second" {
		t.Errorf("ResolveDefaultAgentId = %q, want second", id)
	}
}

func TestResolveDefaultAgentId_NoDefault(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "alpha"},
				{ID: "beta"},
			},
		},
	}
	id := ResolveDefaultAgentId(cfg)
	if id != "alpha" {
		t.Errorf("ResolveDefaultAgentId = %q, want alpha (first)", id)
	}
}

func TestResolveAgentConfig(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{
					ID:   "mybot",
					Name: "MyBot",
					Identity: &types.IdentityConfig{
						Name:  "Helper",
						Emoji: "🤖",
					},
				},
			},
		},
	}
	ac := ResolveAgentConfig(cfg, "MYBOT") // case insensitive
	if ac == nil {
		t.Fatal("ResolveAgentConfig returned nil")
	}
	if ac.Name != "MyBot" {
		t.Errorf("Name = %q, want MyBot", ac.Name)
	}
	if ac.Identity == nil || ac.Identity.Name != "Helper" {
		t.Error("Identity not properly resolved")
	}
}

func TestResolveSessionAgentId(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "main", Default: boolPtr(true)},
			},
		},
	}

	// 无 session key → 默认
	id := ResolveSessionAgentId("", cfg)
	if id != "main" {
		t.Errorf("empty key = %q, want main", id)
	}

	// 有 session key
	id = ResolveSessionAgentId("agent:helper:abc123", cfg)
	if id != "helper" {
		t.Errorf("agent:helper:abc123 = %q, want helper", id)
	}
}

func TestResolveAgentModelPrimary(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{
					ID: "a1",
					Model: &types.AgentModelConfig{
						Primary: "claude-3.5",
					},
				},
			},
		},
	}
	model := ResolveAgentModelPrimary(cfg, "a1")
	if model != "claude-3.5" {
		t.Errorf("model = %q, want claude-3.5", model)
	}
	// 不存在的 agent
	model = ResolveAgentModelPrimary(cfg, "missing")
	if model != "" {
		t.Errorf("missing agent model = %q, want empty", model)
	}
}

func TestResolveAgentWorkspaceDir_NonDefault(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{ID: "main", Default: boolPtr(true)},
				{ID: "helper"},
			},
		},
	}
	dir := ResolveAgentWorkspaceDir(cfg, "helper")
	stateDir := resolveStateDir()
	expected := stateDir + "/workspace-helper"
	if dir != expected {
		t.Errorf("non-default agent dir = %q, want %q", dir, expected)
	}
}

func TestResolveDefaultAgentWorkspaceDir_Profile(t *testing.T) {
	t.Setenv("OPENACOSMI_HOME", t.TempDir())
	t.Setenv("OPENACOSMI_PROFILE", "staging")
	dir := resolveDefaultAgentWorkspaceDir()
	if dir == "" {
		t.Fatal("resolveDefaultAgentWorkspaceDir returned empty")
	}
	if !contains(dir, ".openacosmi-staging/workspace") {
		t.Errorf("dir = %q, want to contain .openacosmi-staging/workspace", dir)
	}
}

func TestResolveDefaultAgentWorkspaceDir_Default(t *testing.T) {
	t.Setenv("OPENACOSMI_HOME", t.TempDir())
	t.Setenv("OPENACOSMI_PROFILE", "")
	dir := resolveDefaultAgentWorkspaceDir()
	if dir == "" {
		t.Fatal("resolveDefaultAgentWorkspaceDir returned empty")
	}
	// 无 profile 时应为 .openacosmi/workspace
	if !contains(dir, ".openacosmi/workspace") {
		t.Errorf("dir = %q, want to contain .openacosmi/workspace", dir)
	}
	// 不应包含 workspace- 后缀
	if contains(dir, "workspace-") {
		t.Errorf("dir = %q, should not have profile suffix", dir)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
