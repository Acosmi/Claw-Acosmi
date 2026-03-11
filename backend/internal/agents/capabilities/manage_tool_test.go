package capabilities

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: execute capability_manage and parse result
// ---------------------------------------------------------------------------

func execManage(t *testing.T, input ManageInput) ManageResult {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	raw, err := ExecuteManageTool(data)
	if err != nil {
		t.Fatalf("ExecuteManageTool error: %v", err)
	}
	var result ManageResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal result: %v (raw: %s)", err, raw)
	}
	return result
}

// ---------------------------------------------------------------------------
// P4-1: Dispatch tests
// ---------------------------------------------------------------------------

func TestManageUnknownAction(t *testing.T) {
	r := execManage(t, ManageInput{Action: "foobar"})
	if r.Success {
		t.Error("expected failure for unknown action")
	}
	if !strings.Contains(r.Error, "unknown action") {
		t.Errorf("expected 'unknown action' error, got: %s", r.Error)
	}
}

func TestManageMissingAction(t *testing.T) {
	r := execManage(t, ManageInput{})
	if r.Success {
		t.Error("expected failure for empty action")
	}
}

// ---------------------------------------------------------------------------
// P4-2: action=tree tests
// ---------------------------------------------------------------------------

func TestManageTreeFull(t *testing.T) {
	r := execManage(t, ManageInput{Action: "tree"})
	if !r.Success {
		t.Fatalf("tree action failed: %s", r.Error)
	}
	// Data should be a slice of TreeNodeView
	data, err := json.Marshal(r.Data)
	if err != nil {
		t.Fatal(err)
	}
	var views []*TreeNodeView
	if err := json.Unmarshal(data, &views); err != nil {
		t.Fatalf("unmarshal tree views: %v", err)
	}
	if len(views) == 0 {
		t.Error("tree returned no root nodes")
	}
	// Should have the meta group
	found := false
	for _, v := range views {
		if v.Name == "meta" {
			found = true
			break
		}
	}
	if !found {
		t.Error("tree missing 'meta' group")
	}
}

func TestManageTreeSubtree(t *testing.T) {
	r := execManage(t, ManageInput{Action: "tree", NodeID: "fs"})
	if !r.Success {
		t.Fatalf("subtree action failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var view TreeNodeView
	if err := json.Unmarshal(data, &view); err != nil {
		t.Fatalf("unmarshal subtree: %v", err)
	}
	if view.ID != "fs" {
		t.Errorf("expected subtree root 'fs', got %q", view.ID)
	}
	if len(view.Children) == 0 {
		t.Error("fs subtree has no children")
	}
}

func TestManageTreeDepthLimit(t *testing.T) {
	r := execManage(t, ManageInput{Action: "tree", Depth: 1})
	if !r.Success {
		t.Fatalf("depth-limited tree failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var views []*TreeNodeView
	if err := json.Unmarshal(data, &views); err != nil {
		t.Fatal(err)
	}
	// With depth=1, root children should have no grandchildren
	for _, v := range views {
		for _, child := range v.Children {
			if len(child.Children) > 0 {
				t.Errorf("depth=1 but node %q has children", child.ID)
			}
		}
	}
}

func TestManageTreeNotFound(t *testing.T) {
	r := execManage(t, ManageInput{Action: "tree", NodeID: "nonexistent"})
	if r.Success {
		t.Error("expected failure for nonexistent node")
	}
}

// ---------------------------------------------------------------------------
// P4-3: action=inspect tests
// ---------------------------------------------------------------------------

func TestManageInspectByID(t *testing.T) {
	r := execManage(t, ManageInput{Action: "inspect", NodeID: "fs/read_file"})
	if !r.Success {
		t.Fatalf("inspect failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var node CapabilityNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	if node.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %q", node.Name)
	}
	// Verify all 7 dimensions present
	if node.Runtime == nil {
		t.Error("missing Runtime")
	}
	if node.Prompt == nil {
		t.Error("missing Prompt")
	}
	if node.Routing == nil {
		t.Error("missing Routing")
	}
	if node.Perms == nil {
		t.Error("missing Perms")
	}
	if node.Skills == nil {
		t.Error("missing Skills")
	}
	if node.Display == nil {
		t.Error("missing Display")
	}
	if node.Policy == nil {
		t.Error("missing Policy")
	}
}

func TestManageInspectByName(t *testing.T) {
	r := execManage(t, ManageInput{Action: "inspect", NodeID: "bash"})
	if !r.Success {
		t.Fatalf("inspect by name failed: %s", r.Error)
	}
}

func TestManageInspectMissingID(t *testing.T) {
	r := execManage(t, ManageInput{Action: "inspect"})
	if r.Success {
		t.Error("expected failure for missing node_id")
	}
}

func TestManageInspectCapabilityManage(t *testing.T) {
	r := execManage(t, ManageInput{Action: "inspect", NodeID: "meta/capability_manage"})
	if !r.Success {
		t.Fatalf("inspect capability_manage failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var node CapabilityNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	if node.Name != "capability_manage" {
		t.Errorf("expected name 'capability_manage', got %q", node.Name)
	}
	if node.Routing == nil || node.Routing.MinTier != "task_light" {
		t.Error("capability_manage should have MinTier=task_light") // S2: 从 question 提升
	}
}

// ---------------------------------------------------------------------------
// P4-12: validate L1 — Node self-check tests
// ---------------------------------------------------------------------------

func TestManageValidateL1Pass(t *testing.T) {
	r := execManage(t, ManageInput{Action: "validate", Level: 1})
	if !r.Success {
		t.Fatalf("validate L1 failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var vr ValidationResult
	if err := json.Unmarshal(data, &vr); err != nil {
		t.Fatal(err)
	}
	if !vr.Level1Pass {
		for _, issue := range vr.Issues {
			if issue.Level == 1 {
				t.Errorf("L1 issue: [%s] %s", issue.NodeID, issue.Message)
			}
		}
	}
}

func TestManageValidateL1SummaryNonEmpty(t *testing.T) {
	tree := DefaultTree()
	tree.Walk(func(n *CapabilityNode) bool {
		if n.Kind == NodeKindGroup {
			return true
		}
		if n.Prompt == nil || n.Prompt.Summary == "" {
			t.Errorf("L1: node %q has empty Prompt.Summary", n.ID)
		}
		return true
	})
}

func TestManageValidateL1MinTierValid(t *testing.T) {
	tree := DefaultTree()
	tree.Walk(func(n *CapabilityNode) bool {
		if n.Kind == NodeKindGroup {
			return true
		}
		if n.Routing == nil || n.Routing.MinTier == "" {
			t.Errorf("L1: node %q has empty MinTier", n.ID)
			return true
		}
		if tierIndex(n.Routing.MinTier) < 0 {
			t.Errorf("L1: node %q has invalid MinTier %q", n.ID, n.Routing.MinTier)
		}
		return true
	})
}

func TestManageValidateL1OwnerNonEmpty(t *testing.T) {
	tree := DefaultTree()
	tree.Walk(func(n *CapabilityNode) bool {
		if n.Kind == NodeKindGroup {
			return true
		}
		if n.Runtime != nil && n.Runtime.Dynamic {
			return true
		}
		if n.Runtime == nil || n.Runtime.Owner == "" {
			t.Errorf("L1: non-dynamic node %q has empty Runtime.Owner", n.ID)
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// P4-13: validate L2 — Cross-node consistency tests
// ---------------------------------------------------------------------------

func TestManageValidateL2Pass(t *testing.T) {
	r := execManage(t, ManageInput{Action: "validate", Level: 2})
	if !r.Success {
		t.Fatalf("validate L2 failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var vr ValidationResult
	if err := json.Unmarshal(data, &vr); err != nil {
		t.Fatal(err)
	}
	if !vr.Level2Pass {
		for _, issue := range vr.Issues {
			if issue.Level == 2 {
				t.Errorf("L2 issue: [%s] %s", issue.NodeID, issue.Message)
			}
		}
	}
}

func TestManageValidateL2ChildMinTierGeParent(t *testing.T) {
	tree := DefaultTree()
	tree.Walk(func(n *CapabilityNode) bool {
		if n.Kind != NodeKindGroup || n.Routing == nil {
			return true
		}
		parentIdx := tierIndex(n.Routing.MinTier)
		if parentIdx < 0 {
			return true
		}
		for _, childID := range n.Children {
			child := tree.Nodes[childID]
			if child == nil || child.Kind == NodeKindGroup {
				continue
			}
			childTier := "task_multimodal"
			if child.Routing != nil && child.Routing.MinTier != "" {
				childTier = child.Routing.MinTier
			}
			childIdx := tierIndex(childTier)
			if childIdx >= 0 && childIdx < parentIdx {
				t.Errorf("L2: child %q MinTier %q < parent %q MinTier %q",
					child.ID, childTier, n.ID, n.Routing.MinTier)
			}
		}
		return true
	})
}

func TestManageValidateL2DynamicPrefixNoOverlap(t *testing.T) {
	tree := DefaultTree()
	seen := make(map[string]string)
	for _, g := range tree.DynamicGroups() {
		if g.Runtime == nil || g.Runtime.NamePrefix == "" {
			continue
		}
		if existing, ok := seen[g.Runtime.NamePrefix]; ok {
			t.Errorf("L2: duplicate NamePrefix %q: %s and %s", g.Runtime.NamePrefix, existing, g.ID)
		}
		seen[g.Runtime.NamePrefix] = g.ID
	}
}

// ---------------------------------------------------------------------------
// P4-14: validate L3 — System cross-layer contract tests
// ---------------------------------------------------------------------------

func TestManageValidateL3Pass(t *testing.T) {
	r := execManage(t, ManageInput{Action: "validate", Level: 3})
	if !r.Success {
		t.Fatalf("validate L3 failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var vr ValidationResult
	if err := json.Unmarshal(data, &vr); err != nil {
		t.Fatal(err)
	}
	if !vr.Level3Pass {
		for _, issue := range vr.Issues {
			if issue.Level == 3 {
				t.Errorf("L3 issue: [%s] %s", issue.NodeID, issue.Message)
			}
		}
	}
}

func TestManageValidateL3RegistrySubsetTree(t *testing.T) {
	tree := DefaultTree()
	treeTools := make(map[string]bool)
	for _, name := range tree.AllStaticTools() {
		treeTools[name] = true
	}
	for _, spec := range Registry {
		if !treeTools[spec.ToolName] {
			t.Errorf("L3: Registry tool %q not in tree", spec.ToolName)
		}
	}
}

func TestManageValidateL3AllowlistMonotonicity(t *testing.T) {
	tree := DefaultTree()
	for i := 1; i < len(validTiers); i++ {
		lower := tree.AllowlistForTier(validTiers[i-1])
		higher := tree.AllowlistForTier(validTiers[i])
		for name := range lower {
			if higher[name] {
				continue
			}
			// Check if excluded
			node := tree.LookupByToolHint(name)
			excluded := false
			if node != nil && node.Routing != nil {
				for _, ex := range node.Routing.ExcludeFrom {
					if ex == validTiers[i] {
						excluded = true
						break
					}
				}
			}
			if !excluded {
				t.Errorf("L3: tool %q in %s but not in %s (monotonicity)", name, validTiers[i-1], validTiers[i])
			}
		}
	}
}

func TestManageValidateL3SummaryCoverage(t *testing.T) {
	tree := DefaultTree()
	summaries := tree.ToolSummaries()
	tree.Walk(func(n *CapabilityNode) bool {
		if n.Kind == NodeKindGroup {
			return true
		}
		if _, ok := summaries[n.Name]; !ok {
			t.Errorf("L3: tool %q has no prompt summary", n.ID)
		}
		return true
	})
}

func TestManageValidateL3PolicyGroupNaming(t *testing.T) {
	tree := DefaultTree()
	for groupName := range tree.PolicyGroups() {
		if !strings.HasPrefix(groupName, "group:") {
			t.Errorf("L3: policy group %q doesn't follow 'group:*' naming", groupName)
		}
	}
}

func TestManageValidateAllLevels(t *testing.T) {
	r := execManage(t, ManageInput{Action: "validate"})
	if !r.Success {
		t.Fatalf("validate all failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var vr ValidationResult
	if err := json.Unmarshal(data, &vr); err != nil {
		t.Fatal(err)
	}
	if !vr.Level1Pass || !vr.Level2Pass || !vr.Level3Pass {
		for _, issue := range vr.Issues {
			t.Errorf("validate issue L%d [%s]: %s", issue.Level, issue.NodeID, issue.Message)
		}
	}
}

// ---------------------------------------------------------------------------
// P4-15: action=diagnose tests
// ---------------------------------------------------------------------------

func TestManageDiagnoseOutput(t *testing.T) {
	r := execManage(t, ManageInput{Action: "diagnose"})
	if !r.Success {
		t.Fatalf("diagnose failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var dr DiagnoseResult
	if err := json.Unmarshal(data, &dr); err != nil {
		t.Fatalf("unmarshal diagnose result: %v", err)
	}

	// Inventory should mention static tools and dynamic groups
	if dr.Inventory == "" {
		t.Error("empty inventory")
	}
	if !strings.Contains(dr.Inventory, "static") {
		t.Errorf("inventory missing 'static': %s", dr.Inventory)
	}
	if !strings.Contains(dr.Inventory, "dynamic") {
		t.Errorf("inventory missing 'dynamic': %s", dr.Inventory)
	}

	// Should have checks
	if len(dr.Checks) == 0 {
		t.Error("diagnose produced no checks")
	}

	// Should have D1, D3, D5 checks
	checkStr := strings.Join(dr.Checks, "\n")
	for _, want := range []string{"D1", "D3", "D5", "D7", "D8", "D9"} {
		if !strings.Contains(checkStr, want) {
			t.Errorf("diagnose missing %s check", want)
		}
	}
}

func TestManageDiagnoseRegistryAlignment(t *testing.T) {
	r := execManage(t, ManageInput{Action: "diagnose"})
	if !r.Success {
		t.Fatalf("diagnose failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var dr DiagnoseResult
	if err := json.Unmarshal(data, &dr); err != nil {
		t.Fatal(err)
	}
	checkStr := strings.Join(dr.Checks, "\n")
	if !strings.Contains(checkStr, "Registry") {
		t.Error("diagnose should include Registry check")
	}
}

// ---------------------------------------------------------------------------
// P4-6: action=generate_prompt tests
// ---------------------------------------------------------------------------

func TestManageGeneratePromptTaskLight(t *testing.T) {
	r := execManage(t, ManageInput{Action: "generate_prompt", Tier: "task_light"})
	if !r.Success {
		t.Fatalf("generate_prompt failed: %s", r.Error)
	}
	prompt, ok := r.Data.(string)
	if !ok {
		// May be deserialized as interface{}, convert
		data, _ := json.Marshal(r.Data)
		prompt = string(data)
	}
	if !strings.Contains(prompt, "Tooling") {
		t.Error("prompt should contain 'Tooling' header")
	}
	// bash should be in task_light
	if !strings.Contains(prompt, "bash") {
		t.Error("task_light prompt should include bash")
	}
}

func TestManageGeneratePromptGreeting(t *testing.T) {
	r := execManage(t, ManageInput{Action: "generate_prompt", Tier: "greeting"})
	if !r.Success {
		t.Fatalf("generate_prompt failed: %s", r.Error)
	}
	// Greeting tier should have no tools
	data, _ := json.Marshal(r.Data)
	prompt := string(data)
	if strings.Contains(prompt, "bash") {
		t.Error("greeting prompt should not include bash")
	}
}

func TestManageGeneratePromptInvalidTier(t *testing.T) {
	r := execManage(t, ManageInput{Action: "generate_prompt", Tier: "invalid"})
	if r.Success {
		t.Error("expected failure for invalid tier")
	}
}

func TestManageGeneratePromptMissingTier(t *testing.T) {
	r := execManage(t, ManageInput{Action: "generate_prompt"})
	if r.Success {
		t.Error("expected failure for missing tier")
	}
}

// ---------------------------------------------------------------------------
// P4-7: action=generate_allowlist tests
// ---------------------------------------------------------------------------

func TestManageGenerateAllowlistTaskWrite(t *testing.T) {
	r := execManage(t, ManageInput{Action: "generate_allowlist", Tier: "task_write"})
	if !r.Success {
		t.Fatalf("generate_allowlist failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result["tier"] != "task_write" {
		t.Errorf("expected tier 'task_write', got %v", result["tier"])
	}
	count, ok := result["count"].(float64)
	if !ok || count == 0 {
		t.Error("allowlist count should be > 0")
	}
	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Error("allowlist tools should be non-empty")
	}
	// write_file should be in task_write
	found := false
	for _, tool := range tools {
		if tool == "write_file" {
			found = true
			break
		}
	}
	if !found {
		t.Error("task_write allowlist should include write_file")
	}
}

func TestManageGenerateAllowlistInvalidTier(t *testing.T) {
	r := execManage(t, ManageInput{Action: "generate_allowlist", Tier: "invalid"})
	if r.Success {
		t.Error("expected failure for invalid tier")
	}
}

// ---------------------------------------------------------------------------
// Integration: capability_manage is in the tree
// ---------------------------------------------------------------------------

func TestCapabilityManageInTree(t *testing.T) {
	tree := DefaultTree()
	node := tree.Lookup("meta/capability_manage")
	if node == nil {
		t.Fatal("capability_manage not found in tree")
	}
	if node.Kind != NodeKindTool {
		t.Errorf("expected kind 'tool', got %q", node.Kind)
	}
	if node.Parent != "meta" {
		t.Errorf("expected parent 'meta', got %q", node.Parent)
	}
}

func TestCapabilityManageToolDef(t *testing.T) {
	name, desc, schema := CapabilityManageToolDef()
	if name != "capability_manage" {
		t.Errorf("expected name 'capability_manage', got %q", name)
	}
	if desc == "" {
		t.Error("description should not be empty")
	}
	if len(schema) == 0 {
		t.Error("schema should not be empty")
	}
	// Verify schema is valid JSON
	var s map[string]interface{}
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Errorf("schema is not valid JSON: %v", err)
	}
	// P7: Verify new actions are in the enum
	if !strings.Contains(desc, "propose_register") {
		t.Error("description should mention propose_register")
	}
	if !strings.Contains(desc, "apply_patch") {
		t.Error("description should mention apply_patch")
	}
}

// ===========================================================================
// P7-10: Test propose_register produces correct patch diff
// ===========================================================================

func TestManageProposeRegisterProducesCorrectPatch(t *testing.T) {
	defer ClearPatchStoreForTesting()

	r := execManage(t, ManageInput{
		Action: "propose_register",
		NodeSpec: &ProposeNodeSpec{
			Name:    "test_p7_tool",
			Parent:  "runtime",
			Kind:    NodeKindTool,
			Prompt:  &NodePrompt{Summary: "A P7 test tool", SortOrder: 99},
			Routing: &NodeRouting{MinTier: "task_write"},
		},
	})
	if !r.Success {
		t.Fatalf("propose_register failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}

	if patch.Status != "proposed" {
		t.Errorf("expected status 'proposed', got %q", patch.Status)
	}
	if patch.Action != "register" {
		t.Errorf("expected action 'register', got %q", patch.Action)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(patch.Operations))
	}
	if patch.Operations[0].Op != "add" {
		t.Errorf("expected op 'add', got %q", patch.Operations[0].Op)
	}
	if patch.Operations[0].Path != "runtime/test_p7_tool" {
		t.Errorf("expected path 'runtime/test_p7_tool', got %q", patch.Operations[0].Path)
	}

	// Verify the node in the patch value is valid
	var node CapabilityNode
	if err := json.Unmarshal(patch.Operations[0].Value, &node); err != nil {
		t.Fatalf("unmarshal node from patch: %v", err)
	}
	if node.Name != "test_p7_tool" {
		t.Errorf("expected name 'test_p7_tool', got %q", node.Name)
	}
	if node.Prompt == nil || node.Prompt.Summary != "A P7 test tool" {
		t.Error("prompt summary not preserved in patch")
	}
	// Verify defaults were filled
	if node.Runtime == nil || node.Runtime.Owner == "" {
		t.Error("runtime defaults not filled")
	}
	if node.Perms == nil {
		t.Error("perms defaults not filled")
	}
	if node.Skills == nil {
		t.Error("skills defaults not filled")
	}
}

func TestManageProposeRegisterMissingSpec(t *testing.T) {
	r := execManage(t, ManageInput{Action: "propose_register"})
	if r.Success {
		t.Error("expected failure for missing node_spec")
	}
}

func TestManageProposeRegisterDuplicate(t *testing.T) {
	r := execManage(t, ManageInput{
		Action: "propose_register",
		NodeSpec: &ProposeNodeSpec{
			Name:   "bash",
			Parent: "runtime",
			Kind:   NodeKindTool,
		},
	})
	if r.Success {
		t.Error("expected failure for duplicate node")
	}
	if !strings.Contains(r.Error, "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", r.Error)
	}
}

func TestManageProposeRegisterBadParent(t *testing.T) {
	r := execManage(t, ManageInput{
		Action: "propose_register",
		NodeSpec: &ProposeNodeSpec{
			Name:   "orphan_tool",
			Parent: "nonexistent_group",
			Kind:   NodeKindTool,
		},
	})
	if r.Success {
		t.Error("expected failure for missing parent")
	}
}

// ===========================================================================
// P7-2: Test propose_update
// ===========================================================================

func TestManageProposeUpdatePrompt(t *testing.T) {
	defer ClearPatchStoreForTesting()

	updates, _ := json.Marshal(ProposeNodeSpec{
		Prompt: &NodePrompt{Summary: "Updated bash summary", SortOrder: 1},
	})
	r := execManage(t, ManageInput{
		Action:  "propose_update",
		NodeID:  "runtime/bash",
		Updates: updates,
	})
	if !r.Success {
		t.Fatalf("propose_update failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	json.Unmarshal(data, &patch)

	if patch.Action != "update" {
		t.Errorf("expected action 'update', got %q", patch.Action)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(patch.Operations))
	}
	if patch.Operations[0].Field != "prompt" {
		t.Errorf("expected field 'prompt', got %q", patch.Operations[0].Field)
	}
	if patch.Operations[0].Op != "replace" {
		t.Errorf("expected op 'replace', got %q", patch.Operations[0].Op)
	}
	// Verify old value is captured
	if len(patch.Operations[0].Old) == 0 {
		t.Error("old value should be captured in replace operation")
	}
}

func TestManageProposeUpdateMissingNode(t *testing.T) {
	updates, _ := json.Marshal(ProposeNodeSpec{
		Prompt: &NodePrompt{Summary: "nope"},
	})
	r := execManage(t, ManageInput{Action: "propose_update", NodeID: "nonexistent", Updates: updates})
	if r.Success {
		t.Error("expected failure for missing node")
	}
}

// ===========================================================================
// P7-3: Test propose_routing
// ===========================================================================

func TestManageProposeRouting(t *testing.T) {
	defer ClearPatchStoreForTesting()

	updates, _ := json.Marshal(NodeRouting{
		MinTier:        "task_delete",
		IntentKeywords: IntentKeywords{ZH: []string{"测试"}, EN: []string{"test"}},
		IntentPriority: 30,
	})
	r := execManage(t, ManageInput{
		Action:  "propose_routing",
		NodeID:  "runtime/bash",
		Updates: updates,
	})
	if !r.Success {
		t.Fatalf("propose_routing failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	// Should have patch
	if result["patch"] == nil {
		t.Fatal("result should contain patch")
	}
	// Should have intent_router_suggestion
	suggestion, _ := result["intent_router_suggestion"].(string)
	if suggestion == "" {
		t.Error("result should contain intent_router_suggestion when keywords are updated")
	}
	if !strings.Contains(suggestion, "TreeClassificationKeywords") {
		t.Errorf("suggestion should mention TreeClassificationKeywords, got: %s", suggestion)
	}
}

func TestManageProposeRoutingInvalidTier(t *testing.T) {
	updates, _ := json.Marshal(NodeRouting{MinTier: "invalid_tier"})
	r := execManage(t, ManageInput{Action: "propose_routing", NodeID: "runtime/bash", Updates: updates})
	if r.Success {
		t.Error("expected failure for invalid tier")
	}
}

// ===========================================================================
// P7-4: Test propose_binding
// ===========================================================================

func TestManageProposeBinding(t *testing.T) {
	defer ClearPatchStoreForTesting()

	updates, _ := json.Marshal(NodeSkillBinding{
		Bindable:    true,
		BoundSkills: []string{"skill:test"},
		Guidance:    true,
	})
	r := execManage(t, ManageInput{
		Action:  "propose_binding",
		NodeID:  "runtime/bash",
		Updates: updates,
	})
	if !r.Success {
		t.Fatalf("propose_binding failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	json.Unmarshal(data, &patch)

	if patch.Action != "binding" {
		t.Errorf("expected action 'binding', got %q", patch.Action)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(patch.Operations))
	}
	if patch.Operations[0].Field != "skills" {
		t.Errorf("expected field 'skills', got %q", patch.Operations[0].Field)
	}
}

// ===========================================================================
// P7-11: Test apply_patch without approval — must be rejected
// ===========================================================================

func TestManageApplyPatchWithoutApproval(t *testing.T) {
	defer ClearPatchStoreForTesting()

	// Create a patch first
	r := execManage(t, ManageInput{
		Action: "propose_register",
		NodeSpec: &ProposeNodeSpec{
			Name: "unapproved_tool", Parent: "runtime", Kind: NodeKindTool,
			Prompt: &NodePrompt{Summary: "Should not be applied", SortOrder: 99},
		},
	})
	if !r.Success {
		t.Fatalf("propose_register failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	json.Unmarshal(data, &patch)

	// Try to apply without approval
	r2 := execManage(t, ManageInput{
		Action:   "apply_patch",
		PatchID:  patch.ID,
		Approved: false,
	})
	if r2.Success {
		t.Error("apply_patch should fail without approval")
	}
	if !strings.Contains(r2.Error, "exec_escalation") {
		t.Errorf("error should mention exec_escalation, got: %s", r2.Error)
	}

	// Verify node was NOT added to tree
	tree := DefaultTree()
	if tree.Lookup("runtime/unapproved_tool") != nil {
		t.Error("node should not exist in tree after rejected apply_patch")
	}
}

func TestManageApplyPatchMissingID(t *testing.T) {
	r := execManage(t, ManageInput{Action: "apply_patch", Approved: true})
	if r.Success {
		t.Error("expected failure for missing patch_id")
	}
}

func TestManageApplyPatchNotFound(t *testing.T) {
	r := execManage(t, ManageInput{Action: "apply_patch", PatchID: "nonexistent", Approved: true})
	if r.Success {
		t.Error("expected failure for nonexistent patch")
	}
}

// ===========================================================================
// P7-12: Test apply_patch with approval — auto-validate must pass
// ===========================================================================

func TestManageApplyPatchWithApprovalAndAutoValidate(t *testing.T) {
	defer func() {
		ResetDefaultTreeForTesting()
		ClearPatchStoreForTesting()
	}()

	// Create a fully-specified valid node patch
	r := execManage(t, ManageInput{
		Action: "propose_register",
		NodeSpec: &ProposeNodeSpec{
			Name:    "validated_p7_tool",
			Parent:  "runtime",
			Kind:    NodeKindTool,
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Validated P7 test tool", SortOrder: 99},
			Routing: &NodeRouting{MinTier: "task_write"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
			Display: &NodeDisplay{Icon: "🔧", Title: "Validated P7 Tool", Verb: "Execute"},
			Policy:  &NodePolicy{PolicyGroups: []string{"group:runtime"}, Profiles: []string{"full"}},
		},
	})
	if !r.Success {
		t.Fatalf("propose_register failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	json.Unmarshal(data, &patch)

	// Apply with approval
	r2 := execManage(t, ManageInput{
		Action:   "apply_patch",
		PatchID:  patch.ID,
		Approved: true,
	})
	if !r2.Success {
		t.Fatalf("apply_patch failed: %s", r2.Error)
	}

	// Verify result contains validation and derivation info
	resultData, _ := json.Marshal(r2.Data)
	resultStr := string(resultData)
	if !strings.Contains(resultStr, "validation") {
		t.Error("apply_patch result should contain validation")
	}
	if !strings.Contains(resultStr, "derivation") {
		t.Error("apply_patch result should contain derivation info")
	}

	// Verify the node was actually added to the tree
	tree := DefaultTree()
	node := tree.Lookup("runtime/validated_p7_tool")
	if node == nil {
		t.Fatal("validated_p7_tool not found in tree after apply_patch")
	}
	if node.Name != "validated_p7_tool" {
		t.Errorf("expected name 'validated_p7_tool', got %q", node.Name)
	}
	if node.Prompt == nil || node.Prompt.Summary != "Validated P7 test tool" {
		t.Error("prompt not applied correctly")
	}

	// Verify the patch status changed to "applied"
	storedPatch := loadPatch(patch.ID)
	if storedPatch == nil || storedPatch.Status != "applied" {
		t.Error("patch status should be 'applied'")
	}

	// Verify validate passes after apply (P7-12 core assertion)
	vr := execManage(t, ManageInput{Action: "validate"})
	if !vr.Success {
		t.Fatalf("validate after apply failed: %s", vr.Error)
	}
	vrData, _ := json.Marshal(vr.Data)
	var valResult ValidationResult
	json.Unmarshal(vrData, &valResult)
	if !valResult.Level1Pass || !valResult.Level2Pass {
		for _, issue := range valResult.Issues {
			t.Errorf("post-apply validation issue L%d [%s]: %s", issue.Level, issue.NodeID, issue.Message)
		}
	}
}

func TestManageApplyPatchDoubleApply(t *testing.T) {
	defer func() {
		ResetDefaultTreeForTesting()
		ClearPatchStoreForTesting()
	}()

	r := execManage(t, ManageInput{
		Action: "propose_register",
		NodeSpec: &ProposeNodeSpec{
			Name: "double_apply_tool", Parent: "runtime", Kind: NodeKindTool,
			Runtime: &NodeRuntime{Owner: "attempt_runner", EnabledWhen: "always"},
			Prompt:  &NodePrompt{Summary: "Double apply test", SortOrder: 99},
			Routing: &NodeRouting{MinTier: "task_write"},
			Perms:   &NodePermissions{MinSecurityLevel: "allowlist", FileAccess: "none", ApprovalType: "none", ScopeCheck: "none"},
			Skills:  &NodeSkillBinding{Bindable: false},
		},
	})
	if !r.Success {
		t.Fatalf("propose_register failed: %s", r.Error)
	}
	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	json.Unmarshal(data, &patch)

	// First apply
	r2 := execManage(t, ManageInput{Action: "apply_patch", PatchID: patch.ID, Approved: true})
	if !r2.Success {
		t.Fatalf("first apply_patch failed: %s", r2.Error)
	}

	// Second apply should fail
	r3 := execManage(t, ManageInput{Action: "apply_patch", PatchID: patch.ID, Approved: true})
	if r3.Success {
		t.Error("second apply_patch should fail")
	}
	if !strings.Contains(r3.Error, "already applied") {
		t.Errorf("expected 'already applied' error, got: %s", r3.Error)
	}
}

// ===========================================================================
// P7: propose_update + apply_patch end-to-end
// ===========================================================================

func TestManageProposeUpdateAndApply(t *testing.T) {
	defer func() {
		ResetDefaultTreeForTesting()
		ClearPatchStoreForTesting()
	}()

	// Propose updating bash prompt summary
	updates, _ := json.Marshal(ProposeNodeSpec{
		Prompt: &NodePrompt{Summary: "Execute commands in workspace (P7 updated)", SortOrder: 1},
	})
	r := execManage(t, ManageInput{
		Action:  "propose_update",
		NodeID:  "runtime/bash",
		Updates: updates,
	})
	if !r.Success {
		t.Fatalf("propose_update failed: %s", r.Error)
	}

	data, _ := json.Marshal(r.Data)
	var patch TreePatch
	json.Unmarshal(data, &patch)

	// Apply
	r2 := execManage(t, ManageInput{Action: "apply_patch", PatchID: patch.ID, Approved: true})
	if !r2.Success {
		t.Fatalf("apply_patch failed: %s", r2.Error)
	}

	// Verify the prompt was updated in the tree
	tree := DefaultTree()
	node := tree.Lookup("runtime/bash")
	if node == nil {
		t.Fatal("bash not found")
	}
	if node.Prompt == nil || node.Prompt.Summary != "Execute commands in workspace (P7 updated)" {
		t.Errorf("prompt not updated, got: %v", node.Prompt)
	}
}
