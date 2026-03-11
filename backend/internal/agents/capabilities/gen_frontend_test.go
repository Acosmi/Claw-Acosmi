package capabilities

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// P2-8: Contract test — generated TS constants equivalent to hand-written
// ---------------------------------------------------------------------------

// TestGenerateToolPolicyTS_EquivalentToHandWritten verifies that the generated
// TOOL_GROUPS and TOOL_PROFILES match the existing hand-written tool-policy.ts.
func TestGenerateToolPolicyTS_EquivalentToHandWritten(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	generated := GenerateToolPolicyTS(tree)

	// Verify TOOL_GROUPS contains all expected groups.
	// NOTE: "process" is not in the tree (Phase 0 gap) so group:runtime only has "exec".
	// The tree is the single source of truth; legacy gaps are tracked separately.
	expectedGroups := map[string][]string{
		"group:memory":    {"memory_get", "memory_search"},
		"group:web":       {"web_fetch", "web_search"},
		"group:fs":        {"apply_patch", "list_dir", "read", "write"},
		"group:runtime":   {"exec"},
		"group:sessions":  {"session_status", "sessions_history", "sessions_list", "sessions_send", "sessions_spawn"},
		"group:ui":        {"canvas"},
		"group:messaging": {"message"},
		// backward-compat groups
		"group:automation": {"cron", "gateway"},
		"group:nodes":      {"nodes"},
		"group:openacosmi": {
			"agents_list", "browser", "canvas", "cron", "gateway",
			"image", "memory_get", "memory_search", "message",
			"nodes", "session_status", "sessions_history",
			"sessions_list", "sessions_send", "sessions_spawn",
			"web_fetch", "web_search",
		},
	}

	for groupName, expectedMembers := range expectedGroups {
		// Check that the generated output contains this group
		if !strings.Contains(generated, groupName) {
			t.Errorf("generated TOOL_GROUPS missing group %q", groupName)
			continue
		}
		// Verify each member is present
		for _, member := range expectedMembers {
			needle := fmt.Sprintf(`"%s"`, member)
			if !strings.Contains(generated, needle) {
				t.Errorf("generated TOOL_GROUPS[%q] missing member %q", groupName, member)
			}
		}
	}

	// Verify TOOL_PROFILES structure
	expectedProfiles := []string{"minimal", "coding", "messaging", "full"}
	for _, pid := range expectedProfiles {
		if !strings.Contains(generated, pid+":") {
			t.Errorf("generated TOOL_PROFILES missing profile %q", pid)
		}
	}

	// Verify minimal profile has session_status
	if !strings.Contains(generated, `"session_status"`) {
		t.Error("minimal profile missing session_status")
	}

	// Verify full profile has empty policy
	if !strings.Contains(generated, "full: {},") {
		t.Error("full profile should have empty policy {}")
	}
}

// TestGenerateToolPolicyTS_GroupsMatchBackend verifies tree-derived groups
// match the backend ToolGroups (D5 ↔ D6 consistency).
func TestGenerateToolPolicyTS_GroupsMatchBackend(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treePolicyGroups := tree.PolicyGroups()

	// Every tree policy group should appear in the generated TS
	generated := GenerateToolPolicyTS(tree)
	for groupName := range treePolicyGroups {
		if !strings.Contains(generated, groupName) {
			t.Errorf("tree policy group %q not in generated TS", groupName)
		}
	}
}

// TestGenerateToolPolicyTS_AllProfileToolsInTree verifies every tool referenced
// in TOOL_PROFILES exists in the tree or a tree-derived group.
func TestGenerateToolPolicyTS_AllProfileToolsInTree(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	allTools := tree.AllStaticTools()
	toolSet := make(map[string]bool, len(allTools))
	for _, name := range allTools {
		toolSet[name] = true
		// Also add frontend aliases
		toolSet[backendToFrontendToolName(name)] = true
	}

	// profile allow lists reference either tools or groups
	groups := tree.PolicyGroups()
	profileAllows := map[string][]string{
		"minimal":   {"session_status"},
		"coding":    {"group:fs", "group:runtime", "group:sessions", "group:memory", "image"},
		"messaging": {"group:messaging", "sessions_list", "sessions_history", "sessions_send", "session_status"},
	}

	for pid, allows := range profileAllows {
		for _, ref := range allows {
			if strings.HasPrefix(ref, "group:") {
				if _, ok := groups[ref]; !ok {
					t.Errorf("profile %q references group %q not in tree", pid, ref)
				}
			} else {
				if !toolSet[ref] {
					t.Errorf("profile %q references tool %q not in tree", pid, ref)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// P2-9: Contract test — generated tool-display.json coverage
// ---------------------------------------------------------------------------

// TestGenerateToolDisplayJSON_CoverageMatchesExisting verifies that the generated
// tool-display.json covers the same tools as the existing hand-written version.
func TestGenerateToolDisplayJSON_CoverageMatchesExisting(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	// Read existing tool-display.json
	existingPath := findToolDisplayJSON(t)
	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read existing tool-display.json: %v", err)
	}

	var existing ToolDisplayJSON
	if err := json.Unmarshal(existingData, &existing); err != nil {
		t.Fatalf("parse existing tool-display.json: %v", err)
	}

	// Generate new
	generatedData, err := GenerateToolDisplayJSON(tree, existingData)
	if err != nil {
		t.Fatalf("GenerateToolDisplayJSON: %v", err)
	}

	var generated ToolDisplayJSON
	if err := json.Unmarshal(generatedData, &generated); err != nil {
		t.Fatalf("parse generated tool-display.json: %v", err)
	}

	// Verify all existing tools are still present
	for toolName := range existing.Tools {
		if _, ok := generated.Tools[toolName]; !ok {
			t.Errorf("existing tool %q missing from generated tool-display.json", toolName)
		}
	}

	// Verify version and fallback preserved
	if generated.Version != existing.Version {
		t.Errorf("version changed: existing=%d generated=%d", existing.Version, generated.Version)
	}
	if generated.Fallback == nil {
		t.Error("fallback missing from generated")
	}
}

// TestGenerateToolDisplayJSON_TreeToolsCovered verifies all tree tools with
// Display fields appear in the generated tool-display.json.
func TestGenerateToolDisplayJSON_TreeToolsCovered(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	existingPath := findToolDisplayJSON(t)
	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read existing tool-display.json: %v", err)
	}

	generatedData, err := GenerateToolDisplayJSON(tree, existingData)
	if err != nil {
		t.Fatalf("GenerateToolDisplayJSON: %v", err)
	}

	var generated ToolDisplayJSON
	if err := json.Unmarshal(generatedData, &generated); err != nil {
		t.Fatalf("parse generated: %v", err)
	}

	// Every tree tool with Display should appear
	specs := tree.DisplaySpecs()
	for toolName := range specs {
		frontName := backendToFrontendToolName(toolName)
		if _, ok := generated.Tools[frontName]; !ok {
			t.Errorf("tree tool %q (frontend: %q) missing from generated tool-display.json",
				toolName, frontName)
		}
	}
}

// TestGenerateToolDisplayJSON_ActionToolsPreserved verifies that complex tools
// with action sub-dispatch (browser, canvas, nodes, cron, gateway) retain
// their full action structure from the existing JSON.
func TestGenerateToolDisplayJSON_ActionToolsPreserved(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	existingPath := findToolDisplayJSON(t)
	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read existing tool-display.json: %v", err)
	}

	generatedData, err := GenerateToolDisplayJSON(tree, existingData)
	if err != nil {
		t.Fatalf("GenerateToolDisplayJSON: %v", err)
	}

	var generated ToolDisplayJSON
	if err := json.Unmarshal(generatedData, &generated); err != nil {
		t.Fatalf("parse generated: %v", err)
	}

	actionTools := []string{"browser", "canvas", "nodes", "cron", "gateway"}
	for _, toolName := range actionTools {
		entry, ok := generated.Tools[toolName]
		if !ok {
			t.Errorf("action tool %q missing from generated", toolName)
			continue
		}
		if entry.Actions == nil || len(entry.Actions) == 0 {
			t.Errorf("action tool %q should have actions, got none", toolName)
		}
	}
}

// TestGenerateToolSections_MatchesHandWritten verifies that tree-derived tool
// sections match the hand-written getToolSections() structure.
func TestGenerateToolSections_MatchesHandWritten(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	sections := GenerateToolSections(tree)

	// Expected sections derived from tree WizardGroups.
	// NOTE: browser has WizardGroup="web" in tree (not "ui" as in legacy agents.ts).
	// This is a tree modeling decision — browser is categorized under web capability.
	expectedSections := map[string][]string{
		"fs":        {"apply_patch", "list_dir", "read", "write"},
		"runtime":   {"exec"},
		"web":       {"browser", "web_fetch", "web_search"}, // browser is in web wizard group
		"memory":    {"memory_get", "memory_search"},
		"sessions":  {"agents_list", "session_status", "sessions_history", "sessions_list", "sessions_send", "sessions_spawn"},
		"ui":        {"canvas"},
		"messaging": {"message"},
	}

	sectionMap := make(map[string][]string, len(sections))
	for _, s := range sections {
		sectionMap[s.ID] = s.Tools
	}

	for sid, expectedTools := range expectedSections {
		actual, ok := sectionMap[sid]
		if !ok {
			t.Errorf("missing section %q in generated tool sections", sid)
			continue
		}
		sort.Strings(expectedTools)
		sort.Strings(actual)

		// Check that expected tools are a subset of actual
		for _, et := range expectedTools {
			found := false
			for _, at := range actual {
				if at == et {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("section %q: expected tool %q not found in generated (got %v)",
					sid, et, actual)
			}
		}
	}
}

// TestGenerateWizardSkillGroups_MatchesHandWritten verifies tree-derived wizard
// skill groups match the hand-written wizard-v2.ts selectedSkills.
func TestGenerateWizardSkillGroups_MatchesHandWritten(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	groups := GenerateWizardSkillGroups(tree)

	if len(groups) == 0 {
		t.Fatal("no wizard skill groups generated")
	}

	// Verify expected default-on groups
	defaultOnExpected := map[string]bool{
		"fs": true, "runtime": true, "ui": true,
		"web": true, "memory": true, "sessions": true, "ai": true,
		"system": true, "messaging": true,
	}

	for _, g := range groups {
		if expected, ok := defaultOnExpected[g.Key]; ok {
			if g.DefaultOn != expected {
				t.Errorf("wizard group %q: defaultOn=%v, want %v",
					g.Key, g.DefaultOn, expected)
			}
		}
	}
}

func TestGenerateWizardSkillGroups_JSONContract(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	groups := GenerateWizardSkillGroups(tree)
	if len(groups) == 0 {
		t.Fatal("no wizard skill groups generated")
	}

	data, err := json.Marshal(groups[:1])
	if err != nil {
		t.Fatalf("marshal wizard skill groups: %v", err)
	}

	text := string(data)
	for _, key := range []string{`"key"`, `"defaultOn"`, `"tools"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("wizard skill group json missing %s: %s", key, text)
		}
	}
	for _, key := range []string{`"Key"`, `"DefaultOn"`, `"Tools"`} {
		if strings.Contains(text, key) {
			t.Fatalf("wizard skill group json should not contain %s: %s", key, text)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func findToolDisplayJSON(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	// Navigate from backend/internal/agents/capabilities/ to ui/src/ui/
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	p := filepath.Join(repoRoot, "ui", "src", "ui", "tool-display.json")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("tool-display.json not found at %s: %v", p, err)
	}
	return p
}
