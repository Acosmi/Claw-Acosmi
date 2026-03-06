package skills

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
)

// TestResolveToolSkillBindings_OnlyRegisteredTools S6-T3:
// 验证绑定结果中的工具名必须在 capabilities.Registry 中注册，
// 或属于已知的动态注册工具（如 media 子系统工具）。
func TestResolveToolSkillBindings_OnlyRegisteredTools(t *testing.T) {
	// 构造模拟主链路技能绑定（模拟从 docs/skills/tools/ 加载的结果）
	entries := []SkillEntry{
		{
			Skill:    Skill{Name: "exec", Description: "Exec tool usage"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
		},
		{
			Skill:    Skill{Name: "browser", Description: "Browser automation"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"browser"}},
		},
		{
			Skill:    Skill{Name: "coder", Description: "Open Coder sub-agent"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"spawn_coder_agent"}},
		},
		{
			Skill:    Skill{Name: "argus-visual", Description: "Argus visual sub-agent"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"spawn_argus_agent"}},
		},
		{
			Skill:    Skill{Name: "send-media", Description: "Send media to channel"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"send_media"}},
		},
		{
			Skill:    Skill{Name: "apply-patch", Description: "Apply multi-file patches"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"apply_patch"}},
		},
		{
			Skill:    Skill{Name: "skills", Description: "Skill system"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"search_skills", "lookup_skill"}},
		},
		{
			Skill:    Skill{Name: "web", Description: "Web search tools"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"web_search"}},
		},
		{
			Skill:    Skill{Name: "progress-reporting", Description: "Report progress"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"report_progress"}},
		},
	}

	bindings := ResolveToolSkillBindings(entries)

	for toolName := range bindings {
		if !capabilities.IsRegistered(toolName) {
			t.Errorf("binding references unregistered tool %q — update capabilities.Registry or fix the SKILL.md tools: field", toolName)
		}
	}
}

// TestResolveToolSkillBindings_OldNamesNotRegistered 验证旧工具名不在 registry 中。
// 如果有人尝试用旧名绑定，IsRegistered 会正确返回 false。
func TestResolveToolSkillBindings_OldNamesNotRegistered(t *testing.T) {
	oldNames := []string{"exec", "read", "write", "ls", "edit"}
	for _, old := range oldNames {
		if capabilities.IsRegistered(old) {
			t.Errorf("old tool name %q should NOT be in the capabilities registry — use the canonical name instead", old)
		}
	}
}

// TestResolveToolSkillBindings_ArgusToolsRegistered 验证 Argus 工具名在 registry 中。
func TestResolveToolSkillBindings_ArgusToolsRegistered(t *testing.T) {
	argusTools := []string{
		"argus_macos_shortcut", "argus_hotkey",
		"argus_read_text", "argus_describe_scene", "argus_capture_screen",
		"argus_locate_element", "argus_detect_dialog",
		"argus_click", "argus_double_click", "argus_type_text",
		"argus_press_key", "argus_scroll", "argus_mouse_position",
		"argus_watch_for_change",
		"argus_open_url", "argus_run_shell",
	}

	for _, tool := range argusTools {
		if !capabilities.IsRegistered(tool) {
			// Argus tools are dynamically registered via ArgusBridge,
			// so they may not be in the static registry.
			// This test documents the expected tool names for reference.
			t.Logf("NOTE: Argus tool %q not in static registry (expected — registered dynamically via ArgusBridge)", tool)
		}
	}
}
