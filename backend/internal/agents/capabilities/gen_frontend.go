// gen_frontend.go generates frontend TypeScript/JSON artifacts from the
// capability tree, eliminating manual synchronization between backend and frontend.
//
// Derivation targets:
//   D6: TOOL_GROUPS + TOOL_PROFILES → tool-policy.ts constants
//   D7: tool-display.json → from Display fields
//   D8: wizard-v2 skill groups → from Policy.WizardGroup
//
// Usage:
//   go run ./cmd/gen-frontend                              (standalone CLI tool)
//   go generate ./internal/agents/capabilities/...          (via go generate)
//
// Design doc: docs/codex/2026-03-09-能力树与自治能力管理系统架构设计-v2.md §Phase 2
// Tracking:   docs/claude/tracking/tracking-2026-03-09-capability-tree-implementation.md
//go:generate go run ../../../cmd/gen-frontend

package capabilities

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// P2-1: gen_policy_ts — Generate TOOL_GROUPS + TOOL_PROFILES TS constants
// ---------------------------------------------------------------------------

// GenerateToolPolicyTS produces the TOOL_GROUPS and TOOL_PROFILES constant
// declarations for ui/src/ui/tool-policy.ts from the capability tree.
//
// The output is a valid TypeScript source fragment that can replace the
// hand-written constants in tool-policy.ts. The functions and type definitions
// remain hand-written; only the data constants are generated.
func GenerateToolPolicyTS(tree *CapabilityTree) string {
	var sb strings.Builder

	// --- TOOL_GROUPS ---
	groups := tree.PolicyGroups()

	// Add backward-compat groups that aren't modeled as policy groups in tree
	backcompat := map[string][]string{
		"group:automation": {"cron", "gateway"},
		"group:nodes":      {"nodes"},
		"group:openacosmi": {
			"browser", "canvas", "nodes", "cron", "message", "gateway",
			"agents_list", "sessions_list", "sessions_history", "sessions_send",
			"sessions_spawn", "session_status", "memory_search", "memory_get",
			"web_search", "web_fetch", "image",
		},
	}
	for g, members := range backcompat {
		if _, ok := groups[g]; !ok {
			groups[g] = members
		}
	}

	// Sort group keys for deterministic output
	groupKeys := make([]string, 0, len(groups))
	for k := range groups {
		groupKeys = append(groupKeys, k)
	}
	sort.Strings(groupKeys)

	sb.WriteString("const TOOL_GROUPS: Record<string, string[]> = {\n")
	for _, gk := range groupKeys {
		members := groups[gk]
		// Use frontend tool name aliases (read_file→read, write_file→write, etc.)
		aliased := make([]string, len(members))
		for i, m := range members {
			aliased[i] = backendToFrontendToolName(m)
		}
		sort.Strings(aliased)

		sb.WriteString(fmt.Sprintf("  %q: [", gk))
		for i, m := range aliased {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", m))
		}
		sb.WriteString("],\n")
	}
	sb.WriteString("};\n")

	// --- TOOL_PROFILES ---
	sb.WriteString("\n")

	profiles := treeToolProfiles(tree)
	profileOrder := []string{"minimal", "coding", "messaging", "full"}

	sb.WriteString("const TOOL_PROFILES: Record<ToolProfileId, ToolProfilePolicy> = {\n")
	for _, pid := range profileOrder {
		policy, ok := profiles[pid]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s: ", pid))
		if policy == nil || (len(policy.Allow) == 0 && len(policy.Deny) == 0) {
			sb.WriteString("{},\n")
			continue
		}
		sb.WriteString("{\n")
		if len(policy.Allow) > 0 {
			sb.WriteString("    allow: [")
			for i, a := range policy.Allow {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%q", a))
			}
			sb.WriteString("],\n")
		}
		if len(policy.Deny) > 0 {
			sb.WriteString("    deny: [")
			for i, d := range policy.Deny {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%q", d))
			}
			sb.WriteString("],\n")
		}
		sb.WriteString("  },\n")
	}
	sb.WriteString("};\n")

	return sb.String()
}

// tsProfilePolicy mirrors the frontend ToolProfilePolicy type.
type tsProfilePolicy struct {
	Allow []string
	Deny  []string
}

// treeToolProfiles returns the profile → policy mapping.
// Currently returns the canonical profiles matching the hand-written structure.
// Future: derive allow lists dynamically from tree node Policy.Profiles fields.
func treeToolProfiles(_ *CapabilityTree) map[string]*tsProfilePolicy {
	return map[string]*tsProfilePolicy{
		"minimal": {Allow: []string{"session_status"}},
		"coding":  {Allow: []string{"group:fs", "group:runtime", "group:sessions", "group:memory", "image"}},
		"messaging": {Allow: []string{
			"group:messaging", "sessions_list", "sessions_history",
			"sessions_send", "session_status",
		}},
		"full": nil, // no restrictions
	}
}

// backendToFrontendToolName converts backend tool names to frontend aliases.
// The frontend uses shorter names for some tools (e.g. read_file → read).
// This mirrors the reverse of backend toolNameAliases in scope/tool_policy.go.
var backendToFrontendAliases = map[string]string{
	"read_file":  "read",
	"write_file": "write",
	"bash":       "exec",
}

func backendToFrontendToolName(name string) string {
	if alias, ok := backendToFrontendAliases[name]; ok {
		return alias
	}
	return name
}

// ---------------------------------------------------------------------------
// P2-3: gen_display_json — Generate tool-display.json from tree Display fields
// ---------------------------------------------------------------------------

// ToolDisplayJSON represents the tool-display.json structure.
type ToolDisplayJSON struct {
	Generated string                       `json:"_generated,omitempty"`
	Version   int                          `json:"version"`
	Fallback  *ToolDisplayFallback         `json:"fallback"`
	Tools     map[string]*ToolDisplayEntry `json:"tools"`
}

// ToolDisplayFallback is the fallback display config for unknown tools.
type ToolDisplayFallback struct {
	Icon       string   `json:"icon"`
	DetailKeys []string `json:"detailKeys"`
}

// ToolDisplayEntry is a single tool's display configuration.
type ToolDisplayEntry struct {
	Icon       string                        `json:"icon"`
	Title      string                        `json:"title"`
	DetailKeys []string                      `json:"detailKeys,omitempty"`
	Actions    map[string]*ToolDisplayAction `json:"actions,omitempty"`
}

// ToolDisplayAction is a single action within a multi-action tool.
type ToolDisplayAction struct {
	Label      string   `json:"label"`
	DetailKeys []string `json:"detailKeys,omitempty"`
}

// GenerateToolDisplayJSON produces the tool-display.json content from the tree.
// It merges tree-derived Display fields with the existing hand-written entries
// that contain action definitions (browser, canvas, nodes, cron, gateway, etc.)
// which are too complex to model in the tree's flat Display structure.
//
// Strategy: tree Display fields provide icon/title/detailKeys for simple tools;
// complex tools with action sub-dispatch retain their existing JSON structure.
func GenerateToolDisplayJSON(tree *CapabilityTree, existingJSON []byte) ([]byte, error) {
	// Parse existing JSON to preserve complex action-based entries
	var existing ToolDisplayJSON
	if len(existingJSON) > 0 {
		if err := json.Unmarshal(existingJSON, &existing); err != nil {
			return nil, fmt.Errorf("parse existing tool-display.json: %w", err)
		}
	}

	// Start from existing structure (preserves fallback, version, complex tools)
	result := existing
	if result.Tools == nil {
		result.Tools = make(map[string]*ToolDisplayEntry)
	}
	result.Generated = "D7 derivation from capability tree — simple tool entries are generated, action-based tools are preserved. Source: gen_frontend.go"

	// Overlay tree-derived display for simple tools (no actions)
	specs := tree.DisplaySpecs()
	for toolName, display := range specs {
		frontName := backendToFrontendToolName(toolName)

		// Skip tools that have complex action-based entries in existing JSON
		if entry, ok := result.Tools[frontName]; ok && entry.Actions != nil {
			continue
		}

		// Build entry from tree Display fields
		entry := &ToolDisplayEntry{
			Icon:  treeIconToJSONIcon(display.Icon),
			Title: display.Title,
		}
		if display.DetailKeys != "" {
			entry.DetailKeys = strings.Split(display.DetailKeys, ",")
		}
		result.Tools[frontName] = entry
	}

	return json.MarshalIndent(result, "", "  ")
}

// treeIconToJSONIcon converts tree emoji icons to tool-display.json icon names.
// The frontend uses lucide icon names, not emojis.
var emojiToIconName = map[string]string{
	"🛠️": "wrench",
	"📖":  "fileText",
	"✍️": "edit",
	"📂":  "folder",
	"🔎":  "search",
	"📄":  "fileText",
	"🌐":  "globe",
	"🧠":  "search",
	"📓":  "fileText",
	"📎":  "paperclip",
	"📧":  "mail",
	"💬":  "messageSquare",
	"🖥️": "smartphone",
	"⚡":  "wrench",
	"🔍":  "search",
}

func treeIconToJSONIcon(emoji string) string {
	if name, ok := emojiToIconName[emoji]; ok {
		return name
	}
	return "puzzle" // fallback
}

// ---------------------------------------------------------------------------
// P2-5: Generate tool sections for agents.ts from tree Policy
// ---------------------------------------------------------------------------

// ToolSectionEntry represents a tool section in the agents.ts getToolSections.
type ToolSectionEntry struct {
	ID    string
	Tools []string // Tool IDs (frontend names)
}

// GenerateToolSections produces tool section entries from the tree.
// Each policy group maps to a section, using frontend tool name aliases.
func GenerateToolSections(tree *CapabilityTree) []ToolSectionEntry {
	// Use wizard groups as the section basis (they map to UI sections)
	wizGroups := tree.WizardGroups()

	// Define the section order matching the current hand-written order
	sectionOrder := []string{
		"fs", "runtime", "web", "memory", "sessions", "ui",
		"messaging", "system", "ai",
	}

	// Section ID remapping for frontend compatibility
	sectionRemap := map[string]string{
		"system": "automation", // tree uses "system" but frontend uses "automation"
	}

	var sections []ToolSectionEntry
	for _, wg := range sectionOrder {
		tools, ok := wizGroups[wg]
		if !ok {
			continue
		}
		sectionID := wg
		if remap, ok := sectionRemap[wg]; ok {
			sectionID = remap
		}

		// Convert to frontend names
		frontTools := make([]string, 0, len(tools))
		for _, t := range tools {
			frontTools = append(frontTools, backendToFrontendToolName(t))
		}
		sort.Strings(frontTools)

		sections = append(sections, ToolSectionEntry{
			ID:    sectionID,
			Tools: frontTools,
		})
	}

	return sections
}

// ---------------------------------------------------------------------------
// P2-6: Generate wizard-v2 skill group definitions from tree
// ---------------------------------------------------------------------------

// WizardSkillGroup represents a skill group in wizard-v2.
type WizardSkillGroup struct {
	Key       string   `json:"key"`
	DefaultOn bool     `json:"defaultOn"`
	Tools     []string `json:"tools"` // Tool names (for display)
}

// GenerateWizardSkillGroups produces wizard skill group definitions from the tree.
func GenerateWizardSkillGroups(tree *CapabilityTree) []WizardSkillGroup {
	wizGroups := tree.WizardGroups()

	// Order and default-on settings matching current hand-written wizard-v2
	type wizDef struct {
		key       string
		defaultOn bool
	}
	defs := []wizDef{
		{"fs", true},
		{"runtime", true},
		{"ui", true},
		{"web", true},
		{"memory", true},
		{"sessions", true},
		{"ai", true},
		{"system", true},
		{"messaging", true},
	}

	// Check if nodes wizard group exists
	if _, ok := wizGroups["system"]; ok {
		// "nodes" is in system group in tree, but separate in wizard
		// Keep as-is since tree models it differently
	}

	var result []WizardSkillGroup
	for _, d := range defs {
		tools, ok := wizGroups[d.key]
		if !ok {
			continue
		}
		frontTools := make([]string, 0, len(tools))
		for _, t := range tools {
			frontTools = append(frontTools, backendToFrontendToolName(t))
		}

		result = append(result, WizardSkillGroup{
			Key:       d.key,
			DefaultOn: d.defaultOn,
			Tools:     frontTools,
		})
	}

	return result
}
