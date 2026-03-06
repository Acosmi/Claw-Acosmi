// Package capabilities provides the Canonical Capability Registry —
// the single source of truth for all tool metadata in Claw Acosmi.
//
// All other layers (prompt, skills, tool groups, UI display, docs)
// must derive from this registry, not maintain independent copies.
//
// Design doc: docs/codex/2026-03-07-系统联动审计-第七轮-单一能力真相源设计稿-v1.md
package capabilities

// CapabilityKind classifies the type of capability.
type CapabilityKind string

const (
	KindTool          CapabilityKind = "tool"
	KindSubagentEntry CapabilityKind = "subagent_entry"
)

// CapabilitySpec defines a single capability in the registry.
type CapabilitySpec struct {
	ID            string         // Stable primary key: "bash", "read_file", "browser"
	Kind          CapabilityKind // tool / subagent_entry
	ToolName      string         // Name exposed to model in main chat path
	RuntimeOwner  string         // Who provides it: "attempt_runner", "argus_bridge", etc.
	EnabledWhen   string         // Condition: "always" / "BrowserController != nil"
	PromptSummary string         // Short description for ## Tooling section
	ToolGroups    []string       // Which group:* this belongs to
	SkillBindable bool           // Whether skills frontmatter can bind to this tool
}

// Registry is the canonical list of all capabilities.
// This is the ONLY place where tool names, summaries, and groups are defined.
var Registry = []CapabilitySpec{
	// --- Core tools (always available) ---
	{
		ID: "bash", Kind: KindTool, ToolName: "bash",
		RuntimeOwner: "attempt_runner", EnabledWhen: "always",
		PromptSummary: "Execute bash commands in the workspace",
		ToolGroups:    []string{"group:runtime"},
		SkillBindable: true,
	},
	{
		ID: "read_file", Kind: KindTool, ToolName: "read_file",
		RuntimeOwner: "attempt_runner", EnabledWhen: "always",
		PromptSummary: "Read file contents",
		ToolGroups:    []string{"group:fs"},
		SkillBindable: true,
	},
	{
		ID: "write_file", Kind: KindTool, ToolName: "write_file",
		RuntimeOwner: "attempt_runner", EnabledWhen: "always",
		PromptSummary: "Create or overwrite files",
		ToolGroups:    []string{"group:fs"},
		SkillBindable: true,
	},
	{
		ID: "list_dir", Kind: KindTool, ToolName: "list_dir",
		RuntimeOwner: "attempt_runner", EnabledWhen: "always",
		PromptSummary: "List directory contents",
		ToolGroups:    []string{"group:fs"},
		SkillBindable: true,
	},

	// --- Optional core tools ---
	{
		ID: "apply_patch", Kind: KindTool, ToolName: "apply_patch",
		RuntimeOwner: "attempt_runner", EnabledWhen: "tools.exec.applyPatch.enabled",
		PromptSummary: "Apply multi-file patches with structured patch format",
		ToolGroups:    []string{"group:fs"},
		SkillBindable: true,
	},

	// --- Skill tools ---
	{
		ID: "search_skills", Kind: KindTool, ToolName: "search_skills",
		RuntimeOwner: "attempt_runner", EnabledWhen: "UHMSBridge != nil && IsSkillsIndexed",
		PromptSummary: "Search skills index by keyword",
		SkillBindable: false,
	},
	{
		ID: "lookup_skill", Kind: KindTool, ToolName: "lookup_skill",
		RuntimeOwner: "attempt_runner", EnabledWhen: "skills available",
		PromptSummary: "Look up full content of a skill by name",
		SkillBindable: false,
	},

	// --- Conditional tools ---
	{
		ID: "web_search", Kind: KindTool, ToolName: "web_search",
		RuntimeOwner: "attempt_runner", EnabledWhen: "WebSearchProvider != nil",
		PromptSummary: "Search the web for real-time information",
		ToolGroups:    []string{"group:web"},
		SkillBindable: true,
	},
	{
		ID: "browser", Kind: KindTool, ToolName: "browser",
		RuntimeOwner: "attempt_runner", EnabledWhen: "BrowserController != nil",
		PromptSummary: "Control web browser via CDP (navigate, click, type, screenshot, ARIA refs)",
		ToolGroups:    []string{"group:ui"},
		SkillBindable: true,
	},
	{
		ID: "send_media", Kind: KindTool, ToolName: "send_media",
		RuntimeOwner: "attempt_runner", EnabledWhen: "MediaSender != nil",
		PromptSummary: "Send file/media to channel (feishu/discord/telegram/whatsapp)",
		SkillBindable: true,
	},
	{
		ID: "memory_search", Kind: KindTool, ToolName: "memory_search",
		RuntimeOwner: "attempt_runner", EnabledWhen: "UHMSBridge != nil",
		PromptSummary: "Search UHMS memory by keyword",
		ToolGroups:    []string{"group:memory"},
		SkillBindable: true,
	},
	{
		ID: "memory_get", Kind: KindTool, ToolName: "memory_get",
		RuntimeOwner: "attempt_runner", EnabledWhen: "UHMSBridge != nil",
		PromptSummary: "Get specific memory entry by ID",
		ToolGroups:    []string{"group:memory"},
		SkillBindable: true,
	},

	// --- Sub-agent entries ---
	{
		ID: "spawn_coder_agent", Kind: KindSubagentEntry, ToolName: "spawn_coder_agent",
		RuntimeOwner: "attempt_runner", EnabledWhen: "always",
		PromptSummary: "Delegate coding tasks to Open Coder sub-agent (delegation contract)",
		SkillBindable: false,
	},
	{
		ID: "spawn_argus_agent", Kind: KindSubagentEntry, ToolName: "spawn_argus_agent",
		RuntimeOwner: "attempt_runner", EnabledWhen: "ArgusBridge != nil",
		PromptSummary: "Delegate desktop/visual tasks to Argus sub-agent (screen + visual perception)",
		SkillBindable: false,
	},
	{
		ID: "spawn_media_agent", Kind: KindSubagentEntry, ToolName: "spawn_media_agent",
		RuntimeOwner: "attempt_runner", EnabledWhen: "MediaSubsystem != nil",
		PromptSummary: "Delegate media operations to media sub-agent",
		SkillBindable: false,
	},

	// --- Internal tools ---
	{
		ID: "report_progress", Kind: KindTool, ToolName: "report_progress",
		RuntimeOwner: "attempt_runner", EnabledWhen: "always",
		PromptSummary: "Report intermediate progress to user",
		SkillBindable: false,
	},
	{
		ID: "request_help", Kind: KindTool, ToolName: "request_help",
		RuntimeOwner: "attempt_runner", EnabledWhen: "AgentChannel != nil",
		PromptSummary: "Request help from parent agent (sub-agent only)",
		SkillBindable: false,
	},
}

// ToolSummaries returns a map of tool name -> prompt summary.
// Used by prompt builder for the ## Tooling section.
func ToolSummaries() map[string]string {
	m := make(map[string]string, len(Registry))
	for _, spec := range Registry {
		if spec.PromptSummary != "" {
			m[spec.ToolName] = spec.PromptSummary
		}
	}
	return m
}

// AllToolGroups returns the complete group -> members mapping,
// derived from the registry. Replaces hand-written group definitions.
func AllToolGroups() map[string][]string {
	groups := make(map[string][]string)
	for _, spec := range Registry {
		for _, g := range spec.ToolGroups {
			groups[g] = append(groups[g], spec.ToolName)
		}
	}
	return groups
}

// IsRegistered checks if a tool name exists in the registry.
func IsRegistered(toolName string) bool {
	for _, spec := range Registry {
		if spec.ToolName == toolName {
			return true
		}
	}
	return false
}

// IsSkillBindable checks if a tool name can be bound by skills frontmatter.
func IsSkillBindable(toolName string) bool {
	for _, spec := range Registry {
		if spec.ToolName == toolName {
			return spec.SkillBindable
		}
	}
	return false
}
