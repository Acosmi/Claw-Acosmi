package capabilities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// P0-15: Generate and validate initial capability_tree.json
// ---------------------------------------------------------------------------

func TestGenerateTreeFromRegistry(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	if tree.NodeCount() == 0 {
		t.Fatal("generated tree has zero nodes")
	}

	// Verify basic structure
	groups := 0
	tools := 0
	subagents := 0
	for _, n := range tree.Nodes {
		switch n.Kind {
		case NodeKindGroup:
			groups++
		case NodeKindTool:
			tools++
		case NodeKindSubagent:
			subagents++
		}
	}
	t.Logf("Tree: %d groups, %d tools, %d subagents = %d total", groups, tools, subagents, tree.NodeCount())

	if groups == 0 {
		t.Error("no group nodes in tree")
	}
	if tools == 0 {
		t.Error("no tool nodes in tree")
	}
	if subagents == 0 {
		t.Error("no subagent nodes in tree")
	}
}

// ---------------------------------------------------------------------------
// P0-16: tree.AllStaticTools() ⊇ registry.AllToolNames()
// ---------------------------------------------------------------------------

func TestTreeCoversRegistry(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treeTools := make(map[string]bool)
	for _, name := range tree.AllStaticTools() {
		treeTools[name] = true
	}

	for _, spec := range Registry {
		if !treeTools[spec.ToolName] {
			t.Errorf("Registry tool %q (ID=%s) not found in tree.AllStaticTools()", spec.ToolName, spec.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-17: tree.AllStaticTools() ⊇ runtime.StaticToolDefs()
// Including send_email which is NOT in Registry but IS in runtime.
// ---------------------------------------------------------------------------

func TestTreeCoversRuntimeTools(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treeTools := make(map[string]bool)
	for _, name := range tree.AllStaticTools() {
		treeTools[name] = true
	}

	// send_email is the known runtime-only tool (not in Registry)
	runtimeOnlyTools := []string{"send_email"}
	for _, name := range runtimeOnlyTools {
		if !treeTools[name] {
			t.Errorf("runtime tool %q not found in tree.AllStaticTools()", name)
		}
	}

	// Also verify all Registry tools are covered (cross-check with P0-16)
	for _, spec := range Registry {
		if !treeTools[spec.ToolName] {
			t.Errorf("Registry tool %q not in tree static tools", spec.ToolName)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-18: tree.DynamicGroups() covers argus_/remote_/mcp_ prefixes
// ---------------------------------------------------------------------------

func TestTreeDynamicGroupsCoverPrefixes(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	prefixes := tree.DynamicGroupPrefixes()

	requiredPrefixes := []string{"argus_", "mcp_", "remote_"}
	prefixSet := make(map[string]bool)
	for _, p := range prefixes {
		prefixSet[p] = true
	}

	for _, req := range requiredPrefixes {
		if !prefixSet[req] {
			t.Errorf("dynamic group prefix %q not found in tree (have: %v)", req, prefixes)
		}
	}
}

func TestTreeDynamicGroupsHaveDiscoveryFields(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	for _, g := range tree.DynamicGroups() {
		if g.Runtime == nil {
			t.Errorf("dynamic group %q has nil Runtime", g.ID)
			continue
		}
		if g.Runtime.NamePrefix == "" {
			t.Errorf("dynamic group %q has empty NamePrefix", g.ID)
		}
		if g.Runtime.DiscoverySource == "" {
			t.Errorf("dynamic group %q has empty DiscoverySource", g.ID)
		}
		if g.Runtime.ListMethod == "" {
			t.Errorf("dynamic group %q has empty ListMethod", g.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-19: tree.AllowlistForTier(t) matches expected per-tier tool sets
// ---------------------------------------------------------------------------

func TestTreeAllowlistForTiers(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	// greeting tier should have no tools
	greeting := tree.AllowlistForTier("greeting")
	if len(greeting) != 0 {
		t.Errorf("greeting tier should have 0 tools, got %d: %v", len(greeting), mapKeys(greeting))
	}

	// question tier should include memory and skills tools
	question := tree.AllowlistForTier("question")
	for _, expected := range []string{"search_skills", "lookup_skill", "memory_search", "memory_get"} {
		if !question[expected] {
			t.Errorf("question tier missing %q", expected)
		}
	}

	// task_light should include bash, read_file, list_dir, browser, web_search
	taskLight := tree.AllowlistForTier("task_light")
	for _, expected := range []string{"bash", "read_file", "list_dir", "browser", "web_search", "report_progress"} {
		if !taskLight[expected] {
			t.Errorf("task_light tier missing %q", expected)
		}
	}
	// task_light should NOT include write_file
	if taskLight["write_file"] {
		t.Error("task_light tier should not include write_file")
	}

	// task_write should include write_file, send_media, send_email, spawn_coder_agent
	taskWrite := tree.AllowlistForTier("task_write")
	for _, expected := range []string{
		"write_file", "send_media", "send_email", "spawn_coder_agent", "spawn_media_agent",
		"browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config",
	} {
		if !taskWrite[expected] {
			t.Errorf("task_write tier missing %q", expected)
		}
	}
	for _, excluded := range []string{"browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config"} {
		if taskLight[excluded] {
			t.Errorf("task_light tier should not include %q", excluded)
		}
	}

	// task_delete should NOT include memory_search, browser, web_search, write_file
	taskDelete := tree.AllowlistForTier("task_delete")
	for _, excluded := range []string{
		"memory_search", "browser", "web_search", "write_file", "send_media",
		"browser_config", "remote_approval_config", "image_config", "stt_config", "docconv_config", "media_config",
	} {
		if taskDelete[excluded] {
			t.Errorf("task_delete tier should not include %q", excluded)
		}
	}
	// task_delete SHOULD include bash, read_file, list_dir, search_skills, lookup_skill, report_progress
	for _, expected := range []string{"bash", "read_file", "list_dir", "search_skills", "lookup_skill", "report_progress"} {
		if !taskDelete[expected] {
			t.Errorf("task_delete tier missing %q", expected)
		}
	}

	// task_multimodal should include everything
	taskMultimodal := tree.AllowlistForTier("task_multimodal")
	for _, name := range tree.AllStaticTools() {
		if !taskMultimodal[name] {
			t.Errorf("task_multimodal tier missing static tool %q", name)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-20: tree.ToolSummaries() == capabilities.ToolSummaries()
// ---------------------------------------------------------------------------

func TestTreeToolSummariesMatchRegistry(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treeSummaries := tree.ToolSummaries()
	registrySummaries := ToolSummaries()

	// Every registry tool summary must be in the tree
	for name, regSummary := range registrySummaries {
		treeSummary, ok := treeSummaries[name]
		if !ok {
			t.Errorf("tree missing summary for registry tool %q", name)
			continue
		}
		if treeSummary != regSummary {
			t.Errorf("summary mismatch for %q:\n  registry: %q\n  tree:     %q", name, regSummary, treeSummary)
		}
	}

	// Tree should have at least as many summaries as registry (tree includes send_email)
	if len(treeSummaries) < len(registrySummaries) {
		t.Errorf("tree has fewer summaries (%d) than registry (%d)", len(treeSummaries), len(registrySummaries))
	}
}

// ---------------------------------------------------------------------------
// P0-21: All SKILL.md tools: references ∈ tree.AllTools() ∪ tree.DynamicGroups()
// (This test validates that known tool names referenced by skills are in the tree)
// ---------------------------------------------------------------------------

func TestTreeCoversSkillBindableTools(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treeBindable := make(map[string]bool)
	for _, name := range tree.BindableTools() {
		treeBindable[name] = true
	}

	// All registry SkillBindable tools should be in tree.BindableTools()
	for _, spec := range Registry {
		if spec.SkillBindable {
			if !treeBindable[spec.ToolName] {
				t.Errorf("registry SkillBindable tool %q not in tree.BindableTools()", spec.ToolName)
			}
		}
	}
}

func TestTreeSkillBindableConsistency(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	for _, n := range tree.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		// Subagents should not be skill-bindable
		if n.Kind == NodeKindSubagent && n.Skills != nil && n.Skills.Bindable {
			t.Errorf("subagent %q should not be SkillBindable", n.ID)
		}
		// Internal tools should not be skill-bindable
		if (n.Name == "report_progress" || n.Name == "request_help") && n.Skills != nil && n.Skills.Bindable {
			t.Errorf("internal tool %q should not be SkillBindable", n.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// Additional contract tests
// ---------------------------------------------------------------------------

func TestTreeNodesHaveValidParents(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	for _, n := range tree.Nodes {
		if n.Parent == "" {
			continue
		}
		if _, ok := tree.Nodes[n.Parent]; !ok {
			t.Errorf("node %q has parent %q which does not exist", n.ID, n.Parent)
		}
	}
}

func TestTreeNoDuplicateIDs(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	seen := make(map[string]bool)
	for id := range tree.Nodes {
		if seen[id] {
			t.Errorf("duplicate node ID: %s", id)
		}
		seen[id] = true
	}
}

func TestTreeNoDuplicateToolNames(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	seen := make(map[string]string) // name -> first id
	for _, n := range tree.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if prevID, ok := seen[n.Name]; ok {
			t.Errorf("duplicate tool Name %q: IDs %s and %s", n.Name, prevID, n.ID)
		}
		seen[n.Name] = n.ID
	}
}

func TestTreeToolNodesHaveSummary(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	for _, n := range tree.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Prompt == nil || n.Prompt.Summary == "" {
			t.Errorf("tool/subagent %q has no Prompt.Summary", n.ID)
		}
	}
}

func TestTreeToolNodesHaveRuntime(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	for _, n := range tree.Nodes {
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Runtime == nil {
			t.Errorf("tool/subagent %q has nil Runtime", n.ID)
			continue
		}
		if n.Runtime.Owner == "" {
			t.Errorf("tool/subagent %q has empty Runtime.Owner", n.ID)
		}
		if n.Runtime.EnabledWhen == "" {
			t.Errorf("tool/subagent %q has empty Runtime.EnabledWhen", n.ID)
		}
	}
}

func TestTreeToolNodesHaveValidMinTier(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	validTierSet := make(map[string]bool)
	for _, tier := range validTiers {
		validTierSet[tier] = true
	}

	for _, n := range tree.Nodes {
		if n.Kind == NodeKindGroup && n.Routing != nil && n.Routing.MinTier != "" {
			// Dynamic groups may have MinTier
			if !validTierSet[n.Routing.MinTier] {
				t.Errorf("group %q has invalid MinTier %q", n.ID, n.Routing.MinTier)
			}
			continue
		}
		if n.Kind == NodeKindGroup {
			continue
		}
		if n.Routing == nil || n.Routing.MinTier == "" {
			t.Errorf("tool/subagent %q has no Routing.MinTier", n.ID)
			continue
		}
		if !validTierSet[n.Routing.MinTier] {
			t.Errorf("tool/subagent %q has invalid MinTier %q", n.ID, n.Routing.MinTier)
		}
	}
}

func TestTreePolicyGroupsMatchRegistry(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treePG := tree.PolicyGroups()
	registryPG := AllToolGroups()

	// Every registry group -> member should be in the tree
	for group, members := range registryPG {
		treeMembers, ok := treePG[group]
		if !ok {
			t.Errorf("registry policy group %q not found in tree", group)
			continue
		}
		treeMemberSet := make(map[string]bool)
		for _, m := range treeMembers {
			treeMemberSet[m] = true
		}
		for _, m := range members {
			if !treeMemberSet[m] {
				t.Errorf("registry group %q member %q not found in tree policy groups", group, m)
			}
		}
	}
}

func TestTreeToRegistryRoundTrip(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	specs := tree.ToRegistry()

	// Should have at least as many specs as the original Registry
	if len(specs) < len(Registry) {
		t.Errorf("ToRegistry() returned %d specs, want >= %d", len(specs), len(Registry))
	}

	specMap := make(map[string]CapabilitySpec)
	for _, s := range specs {
		specMap[s.ToolName] = s
	}

	// Verify original Registry entries are present
	for _, orig := range Registry {
		derived, ok := specMap[orig.ToolName]
		if !ok {
			t.Errorf("ToRegistry() missing tool %q", orig.ToolName)
			continue
		}
		if derived.Kind != orig.Kind {
			t.Errorf("tool %q Kind mismatch: got %q, want %q", orig.ToolName, derived.Kind, orig.Kind)
		}
		if derived.RuntimeOwner != orig.RuntimeOwner {
			t.Errorf("tool %q RuntimeOwner mismatch: got %q, want %q", orig.ToolName, derived.RuntimeOwner, orig.RuntimeOwner)
		}
		if derived.SkillBindable != orig.SkillBindable {
			t.Errorf("tool %q SkillBindable mismatch: got %v, want %v", orig.ToolName, derived.SkillBindable, orig.SkillBindable)
		}
	}
}

func TestTreeLookupByToolHint(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	// Known tools should be found
	for _, name := range []string{"bash", "read_file", "send_media", "send_email", "spawn_coder_agent", "browser_config"} {
		node := tree.LookupByToolHint(name)
		if node == nil {
			t.Errorf("LookupByToolHint(%q) = nil, want non-nil", name)
		}
	}

	// Groups should NOT be found
	for _, name := range []string{"fs", "web", "runtime"} {
		node := tree.LookupByToolHint(name)
		if node != nil {
			t.Errorf("LookupByToolHint(%q) found group node, want nil", name)
		}
	}

	// Unknown tools should return nil
	for _, name := range []string{"unknown_tool", "brave_search"} {
		node := tree.LookupByToolHint(name)
		if node != nil {
			t.Errorf("LookupByToolHint(%q) = %v, want nil", name, node.ID)
		}
	}
}

func TestTreeMatchesDynamicGroup(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	tests := []struct {
		toolName string
		wantNil  bool
		prefix   string
	}{
		{"argus_click", false, "argus_"},
		{"argus_describe_scene", false, "argus_"},
		{"remote_calendar_list", false, "remote_"},
		{"mcp_filesystem_read", false, "mcp_"},
		{"bash", true, ""},
		{"read_file", true, ""},
	}

	for _, tt := range tests {
		g := tree.MatchesDynamicGroup(tt.toolName)
		if tt.wantNil && g != nil {
			t.Errorf("MatchesDynamicGroup(%q) = %v, want nil", tt.toolName, g.ID)
		}
		if !tt.wantNil && g == nil {
			t.Errorf("MatchesDynamicGroup(%q) = nil, want prefix %q", tt.toolName, tt.prefix)
		}
		if !tt.wantNil && g != nil && g.Runtime.NamePrefix != tt.prefix {
			t.Errorf("MatchesDynamicGroup(%q) prefix = %q, want %q", tt.toolName, g.Runtime.NamePrefix, tt.prefix)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-15: JSON serialization round-trip
// ---------------------------------------------------------------------------

func TestTreeJSONRoundTrip(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	// Marshal
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		t.Fatalf("marshal tree: %v", err)
	}

	// Unmarshal
	tree2 := &CapabilityTree{}
	if err := json.Unmarshal(data, tree2); err != nil {
		t.Fatalf("unmarshal tree: %v", err)
	}
	if tree2.Nodes == nil {
		t.Fatal("unmarshaled tree has nil Nodes")
	}

	// Compare node count
	if len(tree2.Nodes) != len(tree.Nodes) {
		t.Errorf("node count mismatch after round-trip: %d vs %d", len(tree2.Nodes), len(tree.Nodes))
	}

	// Compare a few specific nodes
	for _, name := range []string{"bash", "send_email", "spawn_coder_agent"} {
		n1 := tree.LookupByToolHint(name)
		n2 := tree2.LookupByToolHint(name)
		if n1 == nil || n2 == nil {
			t.Errorf("round-trip: missing node %q", name)
			continue
		}
		if n1.ID != n2.ID || n1.Name != n2.Name || n1.Kind != n2.Kind {
			t.Errorf("round-trip mismatch for %q: %+v vs %+v", name, n1, n2)
		}
	}
}

func TestTreeSaveAndLoadJSON(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	dir := t.TempDir()
	path := filepath.Join(dir, "capability_tree.json")

	if err := tree.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	// Verify file exists and is non-empty
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("saved JSON file is empty")
	}

	tree2, err := LoadTreeFromJSON(path)
	if err != nil {
		t.Fatalf("LoadTreeFromJSON: %v", err)
	}

	if len(tree2.Nodes) != len(tree.Nodes) {
		t.Errorf("loaded tree has %d nodes, want %d", len(tree2.Nodes), len(tree.Nodes))
	}
}

// ---------------------------------------------------------------------------
// P1-14: Per-tier allowlist equivalence contract test
// Ensures tree-derived allowlists are monotonically increasing across tiers
// (except task_delete which has exclusions).
// ---------------------------------------------------------------------------

func TestTreeAllowlistMonotonicity(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	// Tiers in escalating order (except task_delete is special)
	escalatingTiers := []string{"greeting", "question", "task_light", "task_write", "task_multimodal"}

	prev := map[string]bool{}
	prevTier := ""
	for _, tier := range escalatingTiers {
		current := tree.AllowlistForTier(tier)
		// Every tool in prev tier should be in current tier
		for tool := range prev {
			if !current[tool] {
				t.Errorf("tool %q in tier %q but missing in higher tier %q (monotonicity violation)", tool, prevTier, tier)
			}
		}
		prev = current
		prevTier = tier
	}

	// task_delete should be a strict subset of task_multimodal
	taskDelete := tree.AllowlistForTier("task_delete")
	taskMultimodal := tree.AllowlistForTier("task_multimodal")
	for tool := range taskDelete {
		if !taskMultimodal[tool] {
			t.Errorf("task_delete tool %q not in task_multimodal (subset violation)", tool)
		}
	}
}

func TestTreeAllowlistTierCounts(t *testing.T) {
	tree := GenerateTreeFromRegistry()

	// Verify expected approximate sizes per tier
	tiers := map[string]struct{ min, max int }{
		"greeting":        {0, 0},
		"question":        {3, 10},
		"task_light":      {6, 20},
		"task_write":      {8, 35},
		"task_delete":     {4, 15},
		"task_multimodal": {15, 50},
	}
	for tier, bounds := range tiers {
		allowed := tree.AllowlistForTier(tier)
		count := len(allowed)
		if count < bounds.min || count > bounds.max {
			t.Errorf("tier %q has %d tools, expected %d-%d: %v", tier, count, bounds.min, bounds.max, mapKeys(allowed))
		}
	}
}

// ---------------------------------------------------------------------------
// P1-15: Binding validation covers Argus tools in dynamic group
// ---------------------------------------------------------------------------

func TestTreeDynamicGroupToolsBindable(t *testing.T) {
	// Argus dynamic group tools should be recognized by IsInTreeOrDynamic
	argusTools := []string{"argus_capture_screen", "argus_click", "argus_describe_scene", "argus_type_text"}
	for _, tool := range argusTools {
		if !IsInTreeOrDynamic(tool) {
			t.Errorf("IsInTreeOrDynamic(%q) = false, want true (argus dynamic group)", tool)
		}
	}

	// Remote MCP tools
	remoteTools := []string{"remote_calendar_list", "remote_gmail_send"}
	for _, tool := range remoteTools {
		if !IsInTreeOrDynamic(tool) {
			t.Errorf("IsInTreeOrDynamic(%q) = false, want true (remote_ dynamic group)", tool)
		}
	}

	// Local MCP tools
	mcpTools := []string{"mcp_filesystem_read", "mcp_slack_send"}
	for _, tool := range mcpTools {
		if !IsInTreeOrDynamic(tool) {
			t.Errorf("IsInTreeOrDynamic(%q) = false, want true (mcp_ dynamic group)", tool)
		}
	}

	// Dynamic group tools should be bindable
	for _, tool := range argusTools {
		if !IsTreeBindable(tool) {
			t.Errorf("IsTreeBindable(%q) = false, want true (argus dynamic group)", tool)
		}
	}
}

func TestTreeStaticToolsNotDynamic(t *testing.T) {
	// Static tools should NOT match dynamic groups
	staticTools := []string{"bash", "read_file", "write_file", "browser", "send_media"}
	tree := GenerateTreeFromRegistry()
	for _, tool := range staticTools {
		if g := tree.MatchesDynamicGroup(tool); g != nil {
			t.Errorf("static tool %q matched dynamic group %q", tool, g.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// P1-16: Tree ↔ Registry equivalence (backward compatibility)
// ---------------------------------------------------------------------------

func TestTreeRegistryEquivalence(t *testing.T) {
	tree := GenerateTreeFromRegistry()
	treeSummaries := tree.ToolSummaries()
	registrySummaries := ToolSummaries()

	// Registry summaries should be a subset of tree summaries
	for name, regSummary := range registrySummaries {
		treeSummary, ok := treeSummaries[name]
		if !ok {
			t.Errorf("registry tool %q missing from tree summaries", name)
			continue
		}
		if treeSummary != regSummary {
			t.Errorf("summary mismatch for %q:\n  registry: %q\n  tree:     %q", name, regSummary, treeSummary)
		}
	}

	// Registry tool groups should match tree policy groups for registered tools
	regGroups := AllToolGroups()
	treeGroups := tree.PolicyGroups()
	for group, regMembers := range regGroups {
		treeMembers, ok := treeGroups[group]
		if !ok {
			t.Errorf("registry group %q missing from tree policy groups", group)
			continue
		}
		treeMemberSet := make(map[string]bool)
		for _, m := range treeMembers {
			treeMemberSet[m] = true
		}
		for _, m := range regMembers {
			if !treeMemberSet[m] {
				t.Errorf("group %q: registry member %q missing from tree", group, m)
			}
		}
	}

	// Registry skill bindable tools should match tree bindable tools
	regBindable := make(map[string]bool)
	for _, spec := range Registry {
		if spec.SkillBindable {
			regBindable[spec.ToolName] = true
		}
	}
	treeBindable := make(map[string]bool)
	for _, name := range tree.BindableTools() {
		treeBindable[name] = true
	}
	for tool := range regBindable {
		if !treeBindable[tool] {
			t.Errorf("registry SkillBindable tool %q not in tree.BindableTools()", tool)
		}
	}
}

// ---------------------------------------------------------------------------
// P1-17: Tree PolicyGroups ↔ tool_policy.go ToolGroups consistency
// (Tests via exported TreePolicyGroups function since tool_policy is in scope package)
// ---------------------------------------------------------------------------

func TestTreePolicyGroupsConsistency(t *testing.T) {
	treeGroups := TreePolicyGroups()

	// Verify all expected groups exist
	expectedGroups := []string{"group:fs", "group:web", "group:runtime", "group:sessions", "group:memory", "group:messaging"}
	for _, g := range expectedGroups {
		if _, ok := treeGroups[g]; !ok {
			t.Errorf("expected policy group %q not found in TreePolicyGroups", g)
		}
	}

	// Verify each group has at least one member
	for group, members := range treeGroups {
		if len(members) == 0 {
			t.Errorf("policy group %q has no members", group)
		}
	}

	// Verify no duplicate members within a group
	for group, members := range treeGroups {
		seen := make(map[string]bool)
		for _, m := range members {
			if seen[m] {
				t.Errorf("policy group %q has duplicate member %q", group, m)
			}
			seen[m] = true
		}
	}

	// Verify key tool -> group membership
	groupMembership := map[string]string{
		"bash":       "group:runtime",
		"read_file":  "group:fs",
		"write_file": "group:fs",
		"list_dir":   "group:fs",
		"web_search": "group:web",
		"web_fetch":  "group:web",
	}
	for tool, expectedGroup := range groupMembership {
		members, ok := treeGroups[expectedGroup]
		if !ok {
			t.Errorf("group %q not found for tool %q", expectedGroup, tool)
			continue
		}
		found := false
		for _, m := range members {
			if m == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q not found in group %q (members: %v)", tool, expectedGroup, members)
		}
	}
}

// TestTreeIntentGroupSummariesCoverage 验证 TreeIntentGroupSummaries 为非空 tier 返回内容。
func TestTreeIntentGroupSummariesCoverage(t *testing.T) {
	summaries := TreeIntentGroupSummaries()

	// greeting should have no summary (no tools)
	if _, ok := summaries["greeting"]; ok {
		t.Error("greeting tier should have no group summary")
	}

	// Other tiers should have at least one group summary
	for _, tier := range []string{"question", "task_light", "task_write", "task_multimodal"} {
		gs, ok := summaries[tier]
		if !ok || gs == "" {
			t.Errorf("tier %q should have non-empty group summary", tier)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
