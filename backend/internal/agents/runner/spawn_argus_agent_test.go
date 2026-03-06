package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------- SpawnArgusAgentToolDef ----------

func TestSpawnArgusAgentToolDef_Name(t *testing.T) {
	def := SpawnArgusAgentToolDef()
	if def.Name != "spawn_argus_agent" {
		t.Errorf("tool name = %q, want %q", def.Name, "spawn_argus_agent")
	}
}

func TestSpawnArgusAgentToolDef_Schema(t *testing.T) {
	def := SpawnArgusAgentToolDef()
	if def.InputSchema == nil {
		t.Fatal("InputSchema should not be nil")
	}

	// 验证 required 字段包含 task_brief
	raw, err := json.Marshal(def.InputSchema)
	if err != nil {
		t.Fatalf("marshal InputSchema error: %v", err)
	}
	schema := string(raw)
	if !strings.Contains(schema, "task_brief") {
		t.Error("InputSchema should contain task_brief")
	}
}

func TestBuildArgusSubagentSystemPrompt_PrefersShortestVisualPath(t *testing.T) {
	prompt := BuildArgusSubagentSystemPrompt(ArgusSubagentPromptParams{
		Task: "Open Terminal and find the latest build status",
	})

	if !strings.Contains(prompt, "Pick the shortest entry") {
		t.Fatal("prompt should instruct Argus to pick the shortest entry path first")
	}
	if !strings.Contains(prompt, "`argus_macos_shortcut`") {
		t.Fatal("prompt should mention argus_macos_shortcut")
	}
	if !strings.Contains(prompt, "`argus_read_text` before `argus_describe_scene`") {
		t.Fatal("prompt should prioritize OCR over full scene description")
	}
	if !strings.Contains(prompt, "Do not default to full-screen capture on every step") {
		t.Fatal("prompt should explicitly reject screenshot-first loops")
	}
	if strings.Contains(prompt, "Always start with `argus_capture_screen`") {
		t.Fatal("prompt should no longer force screenshot-first workflow")
	}
}

// ---------- formatArgusSpawnResult ----------

func TestFormatArgusSpawnResult_NilOutcome(t *testing.T) {
	contract := &DelegationContract{
		ContractID: "argus-001",
		TaskBrief:  "capture screen",
	}

	result := formatArgusSpawnResult(contract, nil)
	if result == "" {
		t.Error("nil outcome should return non-empty string")
	}
	if !strings.Contains(result, "no outcome") {
		t.Error("nil outcome result should mention 'no outcome'")
	}
}

func TestFormatArgusSpawnResult_Completed(t *testing.T) {
	contract := &DelegationContract{
		ContractID: "argus-002",
		TaskBrief:  "analyze page",
	}
	outcome := &SubagentRunOutcome{
		Status: "ok",
		ThoughtResult: &ThoughtResult{
			Status: ThoughtCompleted,
			Result: "Found 3 buttons and 2 input fields",
		},
	}

	result := formatArgusSpawnResult(contract, outcome)
	if !strings.Contains(result, "argus-002") {
		t.Error("result should contain contract ID")
	}
	if !strings.Contains(result, "Found 3 buttons") {
		t.Error("result should contain ThoughtResult.Result")
	}
}

func TestFormatArgusSpawnResult_NeedsAuth(t *testing.T) {
	contract := &DelegationContract{
		ContractID: "argus-003",
		TaskBrief:  "login page",
	}
	outcome := &SubagentRunOutcome{
		Status: "ok",
		ThoughtResult: &ThoughtResult{
			Status: ThoughtNeedsAuth,
			AuthRequest: &AuthRequest{
				Reason:    "need click permission",
				RiskLevel: "low",
			},
		},
	}

	result := formatArgusSpawnResult(contract, outcome)
	if !strings.Contains(result, "needs_auth") && !strings.Contains(result, "auth") {
		t.Error("needs_auth result should mention auth")
	}
}

func TestFormatArgusSpawnResult_NilThoughtResult(t *testing.T) {
	contract := &DelegationContract{
		ContractID: "argus-004",
		TaskBrief:  "screenshot",
	}
	outcome := &SubagentRunOutcome{
		Status: "error",
		Error:  "browser crashed",
	}

	result := formatArgusSpawnResult(contract, outcome)
	if result == "" {
		t.Error("nil ThoughtResult should return non-empty string")
	}
}

func TestFormatArgusSpawnResult_WithScopeViolations(t *testing.T) {
	contract := &DelegationContract{
		ContractID: "argus-005",
		TaskBrief:  "restricted task",
	}
	outcome := &SubagentRunOutcome{
		Status: "ok",
		ThoughtResult: &ThoughtResult{
			Status:          ThoughtCompleted,
			Result:          "done",
			ScopeViolations: []string{"accessed /etc/passwd"},
		},
	}

	result := formatArgusSpawnResult(contract, outcome)
	if !strings.Contains(result, "scope") && !strings.Contains(result, "Scope") && !strings.Contains(result, "/etc/passwd") {
		t.Error("result should mention scope violations")
	}
}

func TestFormatArgusSpawnResult_WithReasoningSummary(t *testing.T) {
	contract := &DelegationContract{
		ContractID: "argus-006",
		TaskBrief:  "complex visual task",
	}
	outcome := &SubagentRunOutcome{
		Status: "ok",
		ThoughtResult: &ThoughtResult{
			Status:           ThoughtCompleted,
			Result:           "task completed",
			ReasoningSummary: "analyzed 5 screenshots, identified target element",
		},
	}

	result := formatArgusSpawnResult(contract, outcome)
	if !strings.Contains(result, "analyzed 5 screenshots") {
		t.Error("result should contain reasoning summary")
	}
}
