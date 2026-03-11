package reply

import (
	"context"
	"reflect"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- Mock 实现 ----------

type mockModelResolver struct{}

func (m *mockModelResolver) ResolveModel(provider, modelID, agentDir string, cfg *types.OpenAcosmiConfig) runner.ModelResolveResult {
	return runner.ModelResolveResult{
		Model: &runner.ResolvedModel{
			ID:            modelID,
			Provider:      provider,
			ContextWindow: 200000,
		},
	}
}

func (m *mockModelResolver) ResolveContextWindowInfo(cfg *types.OpenAcosmiConfig, provider, modelID string, contextWindow int) runner.ContextWindowInfo {
	return runner.ContextWindowInfo{
		Tokens: contextWindow,
	}
}

type mockAttemptRunner struct {
	callCount       int
	lastExtra       string
	lastSkillFilter []string
}

func (m *mockAttemptRunner) RunAttempt(_ context.Context, params runner.AttemptParams) (*runner.AttemptResult, error) {
	m.callCount++
	m.lastExtra = params.ExtraSystemPrompt
	m.lastSkillFilter = append([]string(nil), params.SkillFilter...)
	return &runner.AttemptResult{
		AssistantTexts: []string{"hello from " + params.Provider + "/" + params.ModelID},
		SessionIDUsed:  "sess-123",
	}, nil
}

// ---------- ModelFallbackExecutor 测试 ----------

func TestModelFallbackExecutor_RunTurn_Success(t *testing.T) {
	attemptRunner := &mockAttemptRunner{}
	executor := &ModelFallbackExecutor{
		RunnerDeps: runner.EmbeddedRunDeps{
			ModelResolver: &mockModelResolver{},
			AttemptRunner: attemptRunner,
		},
	}

	result, err := executor.RunTurn(context.Background(), AgentTurnParams{
		FollowupRun: FollowupRun{
			Run: FollowupRunParams{
				SessionID:         "sess-1",
				SessionKey:        "test-key",
				SessionFile:       "/tmp/test.json",
				WorkspaceDir:      "/tmp/ws",
				Provider:          "anthropic",
				Model:             "claude-3",
				ExtraSystemPrompt: "be helpful",
				TimeoutMs:         30000,
			},
		},
		CommandBody: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Payloads) == 0 {
		t.Fatal("expected at least one payload")
	}
}

func TestModelFallbackExecutor_ExtraSystemPromptPassthrough(t *testing.T) {
	attemptRunner := &mockAttemptRunner{}
	executor := &ModelFallbackExecutor{
		RunnerDeps: runner.EmbeddedRunDeps{
			ModelResolver: &mockModelResolver{},
			AttemptRunner: attemptRunner,
		},
	}

	_, err := executor.RunTurn(context.Background(), AgentTurnParams{
		FollowupRun: FollowupRun{
			Run: FollowupRunParams{
				Provider:          "anthropic",
				Model:             "claude-3",
				ExtraSystemPrompt: "custom system",
				TimeoutMs:         10000,
			},
		},
		CommandBody:       "test",
		ExtraSystemPrompt: "custom system",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attemptRunner.lastExtra != "custom system" {
		t.Errorf("ExtraSystemPrompt not passed through, got %q", attemptRunner.lastExtra)
	}
}

func TestModelFallbackExecutor_OnModelSelected(t *testing.T) {
	attemptRunner := &mockAttemptRunner{}
	executor := &ModelFallbackExecutor{
		RunnerDeps: runner.EmbeddedRunDeps{
			ModelResolver: &mockModelResolver{},
			AttemptRunner: attemptRunner,
		},
	}

	var selectedCtx autoreply.ModelSelectedContext
	_, err := executor.RunTurn(context.Background(), AgentTurnParams{
		FollowupRun: FollowupRun{
			Run: FollowupRunParams{
				Provider:  "anthropic",
				Model:     "claude-3",
				TimeoutMs: 10000,
			},
		},
		CommandBody: "test",
		OnModelSelected: func(ctx autoreply.ModelSelectedContext) {
			selectedCtx = ctx
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selectedCtx.Provider != "anthropic" || selectedCtx.Model != "claude-3" {
		t.Errorf("OnModelSelected not called correctly, got %+v", selectedCtx)
	}
}

func TestModelFallbackExecutor_SkillFilterPassthrough(t *testing.T) {
	attemptRunner := &mockAttemptRunner{}
	executor := &ModelFallbackExecutor{
		RunnerDeps: runner.EmbeddedRunDeps{
			ModelResolver: &mockModelResolver{},
			AttemptRunner: attemptRunner,
		},
	}

	_, err := executor.RunTurn(context.Background(), AgentTurnParams{
		FollowupRun: FollowupRun{
			Run: FollowupRunParams{
				Provider:    "anthropic",
				Model:       "claude-3",
				TimeoutMs:   10000,
				SkillFilter: []string{"browser-ops"},
			},
		},
		CommandBody: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(attemptRunner.lastSkillFilter, []string{"browser-ops"}) {
		t.Fatalf("SkillFilter = %v, want [browser-ops]", attemptRunner.lastSkillFilter)
	}
}

func TestConvertEmbeddedResult_NilResult(t *testing.T) {
	result := convertEmbeddedResult(nil, "anthropic", "claude-3")
	if result == nil {
		t.Fatal("expected non-nil result for nil input")
	}
	if len(result.Payloads) != 0 {
		t.Error("expected empty payloads for nil input")
	}
}

func TestConvertEmbeddedResult_WithPayloads(t *testing.T) {
	input := &runner.EmbeddedPiRunResult{
		Payloads: []runner.RunPayload{
			{
				Text: "hello", IsError: false,
				MediaItems: []runner.MediaBlock{
					{MimeType: "image/png", Base64: "img-1"},
					{MimeType: "image/jpeg", Base64: "img-2"},
				},
				MediaBase64:   "img-2",
				MediaMimeType: "image/jpeg",
			},
			{Text: "error", IsError: true},
		},
		Meta: runner.EmbeddedPiRunMeta{
			DurationMs: 1000,
			AgentMeta: &runner.EmbeddedPiAgentMeta{
				SessionID: "sess-1",
				Provider:  "anthropic",
				Model:     "claude-3",
				Usage: &runner.EmbeddedPiAgentUsage{
					Input:  100,
					Output: 50,
				},
			},
		},
		MessagingToolSentTargets: []runner.MessagingToolSend{{Tool: "sms", Provider: "twilio"}},
	}

	result := convertEmbeddedResult(input, "anthropic", "claude-3")
	if len(result.Payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(result.Payloads))
	}
	if result.Payloads[0].Text != "hello" {
		t.Errorf("expected 'hello', got %q", result.Payloads[0].Text)
	}
	if !result.Payloads[1].IsError {
		t.Error("expected second payload to be error")
	}
	if len(result.Payloads[0].MediaItems) != 2 {
		t.Fatalf("expected 2 media items, got %d", len(result.Payloads[0].MediaItems))
	}
	if result.Payloads[0].MediaItems[0].MediaBase64 != "img-1" || result.Payloads[0].MediaItems[0].MediaMimeType != "image/png" {
		t.Fatalf("unexpected first media item: %+v", result.Payloads[0].MediaItems[0])
	}
	if result.Payloads[0].MediaBase64 != "img-2" || result.Payloads[0].MediaMimeType != "image/jpeg" {
		t.Fatalf("legacy media fields mismatch: base64=%q mime=%q", result.Payloads[0].MediaBase64, result.Payloads[0].MediaMimeType)
	}
	if result.Usage == nil {
		t.Fatal("expected usage")
	}
	if result.Usage.Input == nil || *result.Usage.Input != 100 {
		t.Error("expected input usage 100")
	}
	if result.Usage.Output == nil || *result.Usage.Output != 50 {
		t.Error("expected output usage 50")
	}
	if len(result.MessagingToolSentTargets) != 1 || result.MessagingToolSentTargets[0].Tool != "sms" {
		t.Error("messaging tool sent targets not propagated")
	}
}

func TestResolveAuthProfileID(t *testing.T) {
	run := FollowupRunParams{
		Provider:         "anthropic",
		AuthProfileID:    "prof-1",
		AuthProfileIDSrc: "user",
	}

	// Same provider → pass through
	if got := resolveAuthProfileID(run, "anthropic"); got != "prof-1" {
		t.Errorf("expected prof-1, got %q", got)
	}

	// Different provider → empty
	if got := resolveAuthProfileID(run, "openai"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ---------- Auth cooldown 测试 ----------

type mockAuthProfileChecker struct {
	profiles    map[string][]string // provider → profileIDs
	cooldownSet map[string]bool
}

func (m *mockAuthProfileChecker) ResolveProfileOrder(cfg *types.OpenAcosmiConfig, provider, preferred string) []string {
	return m.profiles[provider]
}

func (m *mockAuthProfileChecker) IsInCooldown(profileID string) bool {
	return m.cooldownSet[profileID]
}

func TestRunWithModelFallback_AuthCooldownSkip(t *testing.T) {
	checker := &mockAuthProfileChecker{
		profiles: map[string][]string{
			"anthropic": {"prof-1", "prof-2"},
			"openai":    {"prof-3"},
		},
		cooldownSet: map[string]bool{
			"prof-1": true,
			"prof-2": true,
			// prof-3 is available
		},
	}

	callLog := []string{}
	result, err := models.RunWithModelFallback(
		context.Background(),
		nil,
		"anthropic", "claude-3",
		[]string{"openai/gpt-4"},
		checker,
		func(ctx context.Context, provider, model string) (string, error) {
			callLog = append(callLog, provider+"/"+model)
			return "ok from " + provider, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// anthropic should be skipped (all in cooldown), openai should succeed
	if result.Provider != "openai" {
		t.Errorf("expected openai (skip anthropic cooldown), got %s", result.Provider)
	}
	if len(callLog) != 1 || callLog[0] != "openai/gpt-4" {
		t.Errorf("expected only openai call, got %v", callLog)
	}
	// Should have a cooldown attempt recorded
	if len(result.Attempts) != 1 {
		t.Fatalf("expected 1 cooldown skip attempt, got %d", len(result.Attempts))
	}
	if result.Attempts[0].Reason != string(models.FailoverRateLimit) {
		t.Errorf("expected rate_limit reason, got %q", result.Attempts[0].Reason)
	}
}

func TestRunWithModelFallback_NilAuthStore(t *testing.T) {
	result, err := models.RunWithModelFallback(
		context.Background(),
		nil,
		"anthropic", "claude-3",
		[]string{},
		nil, // no authStore
		func(ctx context.Context, provider, model string) (string, error) {
			return "ok", nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != "ok" {
		t.Errorf("expected 'ok', got %q", result.Result)
	}
}
