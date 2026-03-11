package runner

// P3-7, P3-8: Contract tests verifying prompt ⊆ executor tool set invariant.
// The core bug (5.1) was that the prompt listed ALL tools while the executor only
// allowed the intent-filtered subset. After P3-1~P3-3, the prompt is built from
// the filtered tool set, so prompt tools must be a subset of filtered tools.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/internal/agents/configtools"
	"github.com/Acosmi/ClawAcosmi/internal/agents/gatewayclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/prompt"
)

type stubLocalMCPBridge struct {
	tools []RemoteToolDef
}

func (s *stubLocalMCPBridge) AgentLocalMcpTools() []RemoteToolDef {
	return s.tools
}

func (s *stubLocalMCPBridge) AgentCallLocalMcpTool(_ context.Context, _ string, _ json.RawMessage, _ time.Duration) (string, error) {
	return "ok", nil
}

// allTiers returns the six intent tiers for iteration.
func allTiers() []intentTier {
	return []intentTier{
		intentGreeting,
		intentQuestion,
		intentTaskLight,
		intentTaskWrite,
		intentTaskDelete,
		intentTaskMultimodal,
	}
}

// buildMockTools creates a representative tool set similar to what buildToolDefinitions returns.
func buildMockTools() []llmclient.ToolDef {
	tree := capabilities.DefaultTree()
	summaries := tree.ToolSummaries()
	var tools []llmclient.ToolDef
	for name := range summaries {
		tools = append(tools, llmclient.ToolDef{Name: name})
	}
	return tools
}

// extractPromptToolNames parses tool names from a prompt's ## Tooling section.
// Each tool line has format "- tool_name: description" or "- tool_name".
func extractPromptToolNames(systemPrompt string) []string {
	var names []string
	inTooling := false
	for _, line := range strings.Split(systemPrompt, "\n") {
		if strings.HasPrefix(line, "## Tooling") {
			inTooling = true
			continue
		}
		if inTooling && strings.HasPrefix(line, "## ") {
			break // next section
		}
		if inTooling && strings.HasPrefix(line, "- ") {
			// Extract tool name: "- tool_name: description" or "- tool_name"
			rest := strings.TrimPrefix(line, "- ")
			name := rest
			if idx := strings.Index(rest, ":"); idx > 0 {
				name = rest[:idx]
			}
			name = strings.TrimSpace(name)
			if name != "" && !strings.Contains(name, " ") {
				names = append(names, name)
			}
		}
	}
	return names
}

// TestPromptToolNames_SubsetOfFilteredTools verifies that for every intent tier,
// the tool names appearing in the prompt's ## Tooling section are a subset of
// the tools allowed by filterToolsByIntent.
// P3-7: contract test for the 5.1 bug fix.
func TestPromptToolNames_SubsetOfFilteredTools(t *testing.T) {
	allTools := buildMockTools()
	allSummaries := capabilities.TreeToolSummaries()

	for _, tier := range allTiers() {
		t.Run(string(tier), func(t *testing.T) {
			// Step 1: filter tools by intent (same as RunAttempt post-P3-1)
			filtered := filterToolsByIntent(allTools, tier)

			// Step 2: extract tool names from filtered set
			filteredNames := make(map[string]bool, len(filtered))
			toolNameList := make([]string, len(filtered))
			for i, tool := range filtered {
				filteredNames[tool.Name] = true
				toolNameList[i] = tool.Name
			}

			// Step 3: build filtered summaries (same as RunAttempt post-P3-3)
			filteredSummaries := make(map[string]string, len(toolNameList))
			for _, name := range toolNameList {
				if s, ok := allSummaries[name]; ok {
					filteredSummaries[name] = s
				}
			}

			// Step 4: build prompt with filtered tool names
			bp := prompt.BuildParams{
				Mode:          prompt.PromptModeFull,
				ToolNames:     toolNameList,
				ToolSummaries: filteredSummaries,
			}
			systemPrompt := prompt.BuildAgentSystemPrompt(bp)

			// Step 5: extract tool names from the prompt's ## Tooling section
			promptTools := extractPromptToolNames(systemPrompt)

			// Step 6: verify prompt tools ⊆ filtered tools
			for _, pt := range promptTools {
				if !filteredNames[pt] {
					t.Errorf("tier=%s: prompt contains tool %q which is NOT in the filtered tool set", tier, pt)
				}
			}
		})
	}
}

// TestGreetingTier_NoToolsInPrompt verifies that the greeting tier's prompt
// contains zero tool names in the ## Tooling section.
// P3-8: greeting tier should have no tools.
func TestGreetingTier_NoToolsInPrompt(t *testing.T) {
	allTools := buildMockTools()

	// greeting tier returns nil tools
	filtered := filterToolsByIntent(allTools, intentGreeting)
	if len(filtered) != 0 {
		t.Fatalf("greeting tier should have 0 filtered tools, got %d", len(filtered))
	}

	// Build prompt with empty tool set
	bp := prompt.BuildParams{
		Mode:          prompt.PromptModeFull,
		ToolNames:     nil,
		ToolSummaries: nil,
	}
	systemPrompt := prompt.BuildAgentSystemPrompt(bp)

	// Verify no tool names in prompt
	promptTools := extractPromptToolNames(systemPrompt)
	if len(promptTools) > 0 {
		t.Errorf("greeting tier prompt should have 0 tools, got %d: %v", len(promptTools), promptTools)
	}
}

func TestNormalizeAssistantMessageForToolCalls_ConvertsDowngradedDirective(t *testing.T) {
	msg := llmclient.TextMessage("assistant", "[[lookup_skill]]\nname: acosmi-intro")

	got := normalizeAssistantMessageForToolCalls(msg, map[string]bool{"lookup_skill": true}, nil)
	calls := extractToolCalls(got)
	if len(calls) != 1 {
		t.Fatalf("expected 1 normalized tool call, got %d", len(calls))
	}
	if calls[0].Name != "lookup_skill" {
		t.Fatalf("tool name = %q, want lookup_skill", calls[0].Name)
	}
	var input map[string]string
	if err := json.Unmarshal(calls[0].Input, &input); err != nil {
		t.Fatalf("unmarshal normalized tool input: %v", err)
	}
	if input["name"] != "acosmi-intro" {
		t.Fatalf("normalized tool input name = %q, want acosmi-intro", input["name"])
	}
	if len(got.Content) != 1 || got.Content[0].Type != "tool_use" {
		t.Fatalf("expected normalized assistant content to contain only tool_use, got %+v", got.Content)
	}
}

func TestBuildToolDefinitions_SendMediaIncludesApprovalAndSkillBinding(t *testing.T) {
	r := &EmbeddedAttemptRunner{
		MediaSender: &mockMediaSender{},
		toolBindings: map[string]string{
			"send_media": "Send approved files to the current channel",
		},
		toolBindingNames: map[string][]string{
			"send_media": {"send-media", "message"},
		},
	}

	var desc string
	for _, tool := range r.buildToolDefinitions() {
		if tool.Name == "send_media" {
			desc = tool.Description
			break
		}
	}
	if desc == "" {
		t.Fatal("send_media tool definition not found")
	}
	if !strings.Contains(desc, "data_export approval") {
		t.Fatalf("send_media description should mention data_export approval, got: %q", desc)
	}
	if !strings.Contains(desc, "Permanent full/L3 host access bypasses these approvals") {
		t.Fatalf("send_media description should explain L3 bypasses export approval, got: %q", desc)
	}
	if !strings.Contains(desc, "Bound skills: send-media, message") {
		t.Fatalf("send_media description should include bound skill names, got: %q", desc)
	}
	if !strings.Contains(desc, "Skill: Send approved files to the current channel") {
		t.Fatalf("send_media description should include primary skill guidance, got: %q", desc)
	}
}

func TestBuildToolDefinitions_IncludesGatewayWhenConfigured(t *testing.T) {
	r := &EmbeddedAttemptRunner{
		GatewayOpts: gatewayclient.GatewayOptions{URL: "ws://127.0.0.1:26222"},
	}

	var found llmclient.ToolDef
	for _, tool := range r.buildToolDefinitions() {
		if tool.Name == "gateway" {
			found = tool
			break
		}
	}

	if found.Name == "" {
		t.Fatal("gateway tool definition not found")
	}
	if !strings.Contains(found.Description, "config.patch") {
		t.Fatalf("gateway description should mention config.patch, got: %q", found.Description)
	}
	schema := string(found.InputSchema)
	if !strings.Contains(schema, "config.schema") || !strings.Contains(schema, "baseHash") || !strings.Contains(schema, "raw") {
		t.Fatalf("gateway schema should expose config workflow fields, got: %s", schema)
	}
}

func TestBuildToolDefinitions_IncludesSpecializedConfigToolsWhenGatewayConfigured(t *testing.T) {
	r := &EmbeddedAttemptRunner{
		GatewayOpts: gatewayclient.GatewayOptions{URL: "ws://127.0.0.1:26222"},
	}

	defs := r.buildToolDefinitions()
	byName := make(map[string]llmclient.ToolDef, len(defs))
	for _, tool := range defs {
		byName[tool.Name] = tool
	}

	for _, name := range configtools.ToolNames() {
		def, ok := byName[name]
		if !ok {
			t.Fatalf("specialized config tool %q not found", name)
		}
		if !strings.Contains(string(def.InputSchema), `"action"`) {
			t.Fatalf("%s schema should include action enum, got: %s", name, string(def.InputSchema))
		}
	}

	if !strings.Contains(byName["browser_config"].Description, "browser configuration") {
		t.Fatalf("browser_config description should mention browser configuration, got: %q", byName["browser_config"].Description)
	}
	if !strings.Contains(string(byName["media_config"].InputSchema), "monitorIntervalMin") {
		t.Fatalf("media_config schema should expose media-specific fields, got: %s", string(byName["media_config"].InputSchema))
	}
}

func TestBuildToolDefinitions_IncludesLocalMcpTools(t *testing.T) {
	r := &EmbeddedAttemptRunner{
		LocalMCPBridge: &stubLocalMCPBridge{
			tools: []RemoteToolDef{
				{
					Name:        "mcp_filesystem_read_file",
					Description: "Read a file from the local MCP filesystem server",
					InputSchema: json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	}

	var found llmclient.ToolDef
	for _, tool := range r.buildToolDefinitions() {
		if tool.Name == "mcp_filesystem_read_file" {
			found = tool
			break
		}
	}

	if found.Name == "" {
		t.Fatal("local MCP tool definition not found")
	}
	if !strings.Contains(found.Description, "[本地 MCP]") {
		t.Fatalf("local MCP tool description should be tagged, got: %q", found.Description)
	}
}

func TestBuildToolExecParams_PassesLocalMcpBridge(t *testing.T) {
	bridge := &stubLocalMCPBridge{}
	r := &EmbeddedAttemptRunner{LocalMCPBridge: bridge}

	tep := r.buildToolExecParams(AttemptParams{WorkspaceDir: t.TempDir()}, "full", ApprovalWorkflow{})
	if tep.LocalMCPBridge != bridge {
		t.Fatal("expected LocalMCPBridge to be passed into ToolExecParams")
	}
}

func TestBuildToolDefinitions_BrowserUsesSharedSchema(t *testing.T) {
	r := &EmbeddedAttemptRunner{
		BrowserController: noopBrowserController{},
	}

	var found llmclient.ToolDef
	for _, tool := range r.buildToolDefinitions() {
		if tool.Name == "browser" {
			found = tool
			break
		}
	}

	if found.Name == "" {
		t.Fatal("browser tool definition not found")
	}
	schema := string(found.InputSchema)
	for _, token := range []string{
		"annotate_som",
		"start_gif_recording",
		"stop_gif_recording",
		"list_tabs",
		"create_tab",
		"close_tab",
		"switch_tab",
		"target_id",
	} {
		if !strings.Contains(schema, token) {
			t.Fatalf("browser schema should include %q, got: %s", token, schema)
		}
	}
	if !strings.Contains(found.Description, "Browser Management") {
		t.Fatalf("browser description should mention Browser Management fallback, got: %q", found.Description)
	}
	if !strings.Contains(found.Description, "observe -> click_ref/fill_ref -> screenshot") {
		t.Fatalf("browser description should include the recommended workflow, got: %q", found.Description)
	}
}

func TestBuildSystemPrompt_AppliesSkillFilterToToolBindings(t *testing.T) {
	workspaceDir := t.TempDir()
	writeSkill := func(name, content string) {
		t.Helper()
		skillDir := filepath.Join(workspaceDir, ".agent", "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSkill("browser-ops", `
---
description: browser guidance
tools: browser
---
# browser-ops
`)
	writeSkill("bash-ops", `
---
description: bash guidance
tools: bash
---
# bash-ops
`)

	r := &EmbeddedAttemptRunner{}
	_ = r.buildSystemPrompt(
		AttemptParams{
			WorkspaceDir: workspaceDir,
			SkillFilter:  []string{"browser-ops"},
		},
		prompt.SessionNormal,
		intentTaskLight,
		[]string{"browser", "bash"},
		map[string]string{},
	)

	if got := r.toolBindingNames["browser"]; len(got) != 1 || got[0] != "browser-ops" {
		t.Fatalf("browser binding names = %v, want [browser-ops]", got)
	}
	if _, ok := r.toolBindingNames["bash"]; ok {
		t.Fatalf("bash binding should be filtered out, got %v", r.toolBindingNames["bash"])
	}
}
