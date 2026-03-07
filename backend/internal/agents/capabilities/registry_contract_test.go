package capabilities

import (
	"testing"
)

// TestRegistryHasNoDuplicateIDs 确保注册表中没有重复 ID。
func TestRegistryHasNoDuplicateIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range Registry {
		if seen[spec.ID] {
			t.Errorf("duplicate registry ID: %s", spec.ID)
		}
		seen[spec.ID] = true
	}
}

// TestRegistryHasNoDuplicateToolNames 确保注册表中没有重复 ToolName。
func TestRegistryHasNoDuplicateToolNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range Registry {
		if spec.ToolName == "" {
			continue
		}
		if seen[spec.ToolName] {
			t.Errorf("duplicate ToolName: %s (ID: %s)", spec.ToolName, spec.ID)
		}
		seen[spec.ToolName] = true
	}
}

// TestRegistryToolSummariesComplete 确保每个工具都有 PromptSummary。
func TestRegistryToolSummariesComplete(t *testing.T) {
	for _, spec := range Registry {
		if spec.PromptSummary == "" {
			t.Errorf("registry entry %s has empty PromptSummary", spec.ID)
		}
	}
}

// TestToolSummariesMatchRegistry 确保 ToolSummaries() 返回的映射覆盖所有有 ToolName 的条目。
func TestToolSummariesMatchRegistry(t *testing.T) {
	summaries := ToolSummaries()
	for _, spec := range Registry {
		if spec.ToolName == "" {
			continue
		}
		if _, ok := summaries[spec.ToolName]; !ok {
			t.Errorf("ToolSummaries() missing entry for %s", spec.ToolName)
		}
	}
}

// TestLookupByToolName 确保 LookupByToolName 正确查找和返回 nil。
func TestLookupByToolName(t *testing.T) {
	// Known tools should be found
	for _, name := range []string{"bash", "web_search", "browser", "memory_search", "web_fetch", "canvas", "sessions_list"} {
		if spec := LookupByToolName(name); spec == nil {
			t.Errorf("LookupByToolName(%q) = nil, expected non-nil", name)
		}
	}
	// Unknown tools should return nil
	for _, name := range []string{"unknown", "brave_search", "perplexity", ""} {
		if spec := LookupByToolName(name); spec != nil {
			t.Errorf("LookupByToolName(%q) = %+v, expected nil", name, spec)
		}
	}
}

// TestSkillBindableConsistency 确保 sub-agent 和 internal 工具不可被技能绑定。
func TestSkillBindableConsistency(t *testing.T) {
	for _, spec := range Registry {
		if spec.Kind == KindSubagentEntry && spec.SkillBindable {
			t.Errorf("sub-agent entry %s should not be SkillBindable", spec.ID)
		}
		if (spec.ID == "report_progress" || spec.ID == "request_help") && spec.SkillBindable {
			t.Errorf("internal tool %s should not be SkillBindable", spec.ID)
		}
	}
}

// TestRegistryCoversPromptSections 确保 prompt_sections.go 中引用的工具都在 Registry 中。
func TestRegistryCoversPromptSections(t *testing.T) {
	promptTools := []string{
		"bash", "read_file", "write_file", "list_dir", "apply_patch",
		"web_search", "web_fetch", "browser", "canvas",
		"agents_list", "sessions_list", "sessions_history", "sessions_send",
		"sessions_spawn", "session_status", "image",
		"memory_search", "memory_get",
	}
	for _, name := range promptTools {
		if !IsRegistered(name) {
			t.Errorf("prompt_sections tool %q is not registered", name)
		}
	}
}

// TestRegistryRequiredFields 确保每个条目都有必填字段。
func TestRegistryRequiredFields(t *testing.T) {
	for i, spec := range Registry {
		if spec.ID == "" {
			t.Errorf("Registry[%d] has empty ID", i)
		}
		if spec.Kind == "" {
			t.Errorf("Registry[%d] (%s) has empty Kind", i, spec.ID)
		}
		if spec.RuntimeOwner == "" {
			t.Errorf("Registry[%d] (%s) has empty RuntimeOwner", i, spec.ID)
		}
		if spec.EnabledWhen == "" {
			t.Errorf("Registry[%d] (%s) has empty EnabledWhen", i, spec.ID)
		}
	}
}
