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
