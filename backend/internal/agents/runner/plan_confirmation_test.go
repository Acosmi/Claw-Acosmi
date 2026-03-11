package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
)

// ---------- GateMode 测试 ----------

func TestPlanConfirmation_GateMode(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	// 默认 mode = full
	if m := mgr.GateMode(); m != GateModeFull {
		t.Errorf("default GateMode() = %q, want %q", m, GateModeFull)
	}

	// 合法切换
	mgr.SetGateMode(GateModeMonitor)
	if m := mgr.GateMode(); m != GateModeMonitor {
		t.Errorf("after SetGateMode(monitor), GateMode() = %q, want %q", m, GateModeMonitor)
	}

	// 非法模式保持不变
	mgr.SetGateMode("invalid_mode")
	if m := mgr.GateMode(); m != GateModeMonitor {
		t.Errorf("after SetGateMode(invalid), GateMode() = %q, want %q", m, GateModeMonitor)
	}
}

func TestPlanConfirmation_ShouldGate(t *testing.T) {
	tests := []struct {
		mode     string
		expected bool
	}{
		{GateModeFull, true},
		{GateModeSmart, true},
		{GateModeMonitor, false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
			defer mgr.Close()
			mgr.SetGateMode(tt.mode)
			if got := mgr.ShouldGate(); got != tt.expected {
				t.Errorf("ShouldGate() with mode %q = %v, want %v", tt.mode, got, tt.expected)
			}
		})
	}
}

// ---------- RequestPlanConfirmation + ResolvePlanConfirmation 测试 ----------

func TestPlanConfirmation_Approve(t *testing.T) {
	var broadcastCalls []string
	broadcast := func(event string, payload interface{}) {
		broadcastCalls = append(broadcastCalls, event)
	}

	mgr := NewPlanConfirmationManager(broadcast, nil, 5*time.Second)
	defer mgr.Close()

	var decision PlanDecision
	var reqErr error
	done := make(chan struct{})

	go func() {
		decision, reqErr = mgr.RequestPlanConfirmation(context.Background(), PlanConfirmationRequest{
			TaskBrief:  "test task",
			IntentTier: "task_write",
		})
		close(done)
	}()

	// 等待 pending 出现
	time.Sleep(50 * time.Millisecond)
	if mgr.PendingCount() != 1 {
		t.Fatalf("PendingCount() = %d, want 1", mgr.PendingCount())
	}

	// 找到 pending ID 并 approve
	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	if err := mgr.ResolvePlanConfirmation(pendingID, PlanDecision{Action: "approve"}); err != nil {
		t.Fatalf("ResolvePlanConfirmation error: %v", err)
	}

	<-done

	if reqErr != nil {
		t.Errorf("RequestPlanConfirmation error: %v", reqErr)
	}
	if decision.Action != "approve" {
		t.Errorf("decision.Action = %q, want %q", decision.Action, "approve")
	}
	if mgr.PendingCount() != 0 {
		t.Errorf("PendingCount after resolve = %d, want 0", mgr.PendingCount())
	}

	// 验证广播事件
	if len(broadcastCalls) < 2 {
		t.Fatalf("broadcast called %d times, want >= 2", len(broadcastCalls))
	}
	if broadcastCalls[0] != "plan.confirm.requested" {
		t.Errorf("first broadcast = %q, want %q", broadcastCalls[0], "plan.confirm.requested")
	}
	if broadcastCalls[1] != "plan.confirm.resolved" {
		t.Errorf("second broadcast = %q, want %q", broadcastCalls[1], "plan.confirm.resolved")
	}
}

func TestPlanConfirmation_Reject(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	var decision PlanDecision
	done := make(chan struct{})

	go func() {
		decision, _ = mgr.RequestPlanConfirmation(context.Background(), PlanConfirmationRequest{
			TaskBrief: "delete all files",
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	_ = mgr.ResolvePlanConfirmation(pendingID, PlanDecision{
		Action:   "reject",
		Feedback: "too dangerous",
	})

	<-done

	if decision.Action != "reject" {
		t.Errorf("decision.Action = %q, want %q", decision.Action, "reject")
	}
	if decision.Feedback != "too dangerous" {
		t.Errorf("decision.Feedback = %q, want %q", decision.Feedback, "too dangerous")
	}
}

func TestPlanConfirmation_Edit(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	var decision PlanDecision
	done := make(chan struct{})

	go func() {
		decision, _ = mgr.RequestPlanConfirmation(context.Background(), PlanConfirmationRequest{
			TaskBrief: "write code",
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	_ = mgr.ResolvePlanConfirmation(pendingID, PlanDecision{
		Action:     "edit",
		EditedPlan: "write code with tests",
	})

	<-done

	if decision.Action != "edit" {
		t.Errorf("decision.Action = %q, want %q", decision.Action, "edit")
	}
	if decision.EditedPlan != "write code with tests" {
		t.Errorf("decision.EditedPlan = %q, want %q", decision.EditedPlan, "write code with tests")
	}
}

// ---------- Timeout 测试 ----------

func TestPlanConfirmation_Timeout(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 200*time.Millisecond)
	defer mgr.Close()

	decision, err := mgr.RequestPlanConfirmation(context.Background(), PlanConfirmationRequest{
		TaskBrief: "slow task",
	})

	if err != nil {
		t.Fatalf("RequestPlanConfirmation error: %v", err)
	}
	if decision.Action != "reject" {
		t.Errorf("timeout decision.Action = %q, want %q", decision.Action, "reject")
	}
	if decision.Feedback != "timeout" {
		t.Errorf("timeout decision.Feedback = %q, want %q", decision.Feedback, "timeout")
	}
}

// ---------- Context Cancellation 测试 ----------

func TestPlanConfirmation_ContextCancel(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan PlanDecision)
	go func() {
		d, _ := mgr.RequestPlanConfirmation(ctx, PlanConfirmationRequest{
			TaskBrief: "cancelled task",
		})
		done <- d
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	decision := <-done
	if decision.Action != "reject" {
		t.Errorf("cancel decision.Action = %q, want %q", decision.Action, "reject")
	}
}

// ---------- ResolvePlanConfirmation 错误路径 ----------

func TestPlanConfirmation_ResolveUnknownID(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	err := mgr.ResolvePlanConfirmation("nonexistent-id", PlanDecision{Action: "approve"})
	if err == nil {
		t.Error("ResolvePlanConfirmation with unknown ID should return error")
	}
}

func TestPlanConfirmation_ResolveInvalidAction(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	err := mgr.ResolvePlanConfirmation("any-id", PlanDecision{Action: "invalid"})
	if err == nil {
		t.Error("ResolvePlanConfirmation with invalid action should return error")
	}
}

// ---------- Default Timeout 测试 ----------

func TestPlanConfirmation_DefaultTimeout(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 0)
	defer mgr.Close()

	if mgr.Timeout() != 5*time.Minute {
		t.Errorf("default Timeout() = %v, want %v", mgr.Timeout(), 5*time.Minute)
	}
}

// ---------- Close 幂等性 ----------

func TestPlanConfirmation_CloseIdempotent(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	mgr.Close()
	mgr.Close() // 不应 panic
}

// ---------- Auto-fill ID 测试 ----------

func TestPlanConfirmation_AutoFillID(t *testing.T) {
	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()

	done := make(chan struct{})
	go func() {
		mgr.RequestPlanConfirmation(context.Background(), PlanConfirmationRequest{
			TaskBrief: "id test",
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	// 收集 ID（持锁），释放后再 resolve
	mgr.mu.Lock()
	var ids []string
	for id := range mgr.pending {
		if id == "" {
			t.Error("auto-generated ID is empty")
		}
		ids = append(ids, id)
	}
	mgr.mu.Unlock()

	for _, id := range ids {
		_ = mgr.ResolvePlanConfirmation(id, PlanDecision{Action: "approve"})
	}
	<-done
}

// ---------- DecisionLogger 测试 ----------

func TestPlanConfirmation_DecisionLogger(t *testing.T) {
	var logged []PlanDecisionRecord
	var mu sync.Mutex
	logger := &mockDecisionLogger{
		logFn: func(r PlanDecisionRecord) error {
			mu.Lock()
			logged = append(logged, r)
			mu.Unlock()
			return nil
		},
	}

	mgr := NewPlanConfirmationManager(nil, nil, 5*time.Second)
	defer mgr.Close()
	mgr.SetDecisionLogger(logger)

	done := make(chan struct{})
	go func() {
		mgr.RequestPlanConfirmation(context.Background(), PlanConfirmationRequest{
			TaskBrief: "logged task",
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	mgr.mu.Lock()
	var pendingID string
	for id := range mgr.pending {
		pendingID = id
	}
	mgr.mu.Unlock()

	_ = mgr.ResolvePlanConfirmation(pendingID, PlanDecision{Action: "approve"})
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(logged) != 1 {
		t.Fatalf("logged %d records, want 1", len(logged))
	}
	if logged[0].Decision.Action != "approve" {
		t.Errorf("logged action = %q, want %q", logged[0].Decision.Action, "approve")
	}
}

// ---------- P5-13: PlanSteps 非空 (task_write 场景) ----------

func TestPlanConfirmation_PlanStepsNonEmpty_TaskWrite(t *testing.T) {
	// Verify that RequestPlanConfirmationWithSessionKey correctly carries PlanSteps
	var captured PlanConfirmationRequest
	broadcast := func(event string, payload interface{}) {
		if event == "plan.confirm.requested" {
			captured = payload.(PlanConfirmationRequest)
		}
	}

	mgr := NewPlanConfirmationManager(broadcast, nil, 200*time.Millisecond)
	defer mgr.Close()

	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "write_code", Description: "创建新文件", ToolHint: "write_file"},
		},
	}

	tree := capabilities.DefaultTree()
	planSteps := GeneratePlanSteps(analysis, tree)

	if len(planSteps) == 0 {
		t.Fatal("GeneratePlanSteps should return non-empty steps for task_write")
	}

	req := PlanConfirmationRequest{
		TaskBrief:  "创建一个新文件",
		PlanSteps:  planSteps,
		IntentTier: "task_write",
	}

	// Start request in background (will timeout)
	done := make(chan struct{})
	go func() {
		mgr.RequestPlanConfirmationWithSessionKey(context.Background(), req, "webchat:test")
		close(done)
	}()

	// Wait for broadcast
	time.Sleep(50 * time.Millisecond)

	// Verify captured request has PlanSteps
	if len(captured.PlanSteps) == 0 {
		t.Error("PlanConfirmationRequest.PlanSteps should be non-empty for task_write")
	}

	// Verify PlanSteps content
	if captured.PlanSteps[0] != planSteps[0] {
		t.Errorf("PlanSteps[0] = %q, want %q", captured.PlanSteps[0], planSteps[0])
	}

	<-done
}

func TestPlanConfirmation_RequestCarriesApprovalSummary(t *testing.T) {
	var captured PlanConfirmationRequest
	broadcast := func(event string, payload interface{}) {
		if event == "plan.confirm.requested" {
			captured = payload.(PlanConfirmationRequest)
		}
	}

	mgr := NewPlanConfirmationManager(broadcast, nil, 200*time.Millisecond)
	defer mgr.Close()

	scope := ApprovalScope{
		Type:            "data_export",
		TTLMinutes:      5,
		NeedsOriginator: true,
		ExportFiles:     []string{"/Users/test/Desktop/report.pdf"},
		MountMode:       "ro",
		MountPath:       "/Users/test/Desktop",
	}
	scope.PrimaryApproval = scope.primaryRequirement()
	scope.AdditionalApprovals = []ApprovalRequirement{
		{
			Type:       "mount_access",
			TTLMinutes: 0,
			MountMode:  "ro",
			MountPath:  "/Users/test/Desktop",
		},
	}

	req := PlanConfirmationRequest{
		TaskBrief:           "把 report.pdf 发到飞书",
		PlanSteps:           []string{"发送文件 /Users/test/Desktop/report.pdf"},
		PrimaryApproval:     scope.PrimaryApproval,
		AdditionalApprovals: scope.AdditionalApprovals,
		ApprovalSummary:     ApprovalSummaryFromScope(scope),
		IntentTier:          "task_light",
	}

	done := make(chan struct{})
	go func() {
		mgr.RequestPlanConfirmationWithSessionKey(context.Background(), req, "feishu:test")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	if captured.PrimaryApproval.Type != "data_export" {
		t.Fatalf("PrimaryApproval.Type = %q, want data_export", captured.PrimaryApproval.Type)
	}
	if len(captured.AdditionalApprovals) != 1 {
		t.Fatalf("AdditionalApprovals len = %d, want 1", len(captured.AdditionalApprovals))
	}
	if len(captured.ApprovalSummary) != 2 {
		t.Fatalf("ApprovalSummary len = %d, want 2", len(captured.ApprovalSummary))
	}
	if !strings.Contains(captured.ApprovalSummary[0], "data_export") {
		t.Fatalf("ApprovalSummary[0] = %q, want data_export summary", captured.ApprovalSummary[0])
	}
	if !strings.Contains(captured.ApprovalSummary[1], "mount_access") {
		t.Fatalf("ApprovalSummary[1] = %q, want mount_access summary", captured.ApprovalSummary[1])
	}
	if !strings.Contains(captured.ApprovalSummary[1], "本任务") {
		t.Fatalf("ApprovalSummary[1] = %q, want task-scoped mount wording", captured.ApprovalSummary[1])
	}

	<-done
}

// ---------- P5-10: sessionKey 正确传递 ----------

func TestPlanConfirmation_SessionKeyPassthrough(t *testing.T) {
	var capturedSessionKey string
	remoteNotify := func(req PlanConfirmationRequest, sessionKey string) {
		capturedSessionKey = sessionKey
	}

	mgr := NewPlanConfirmationManager(nil, remoteNotify, 200*time.Millisecond)
	defer mgr.Close()

	req := PlanConfirmationRequest{
		TaskBrief:  "test task",
		IntentTier: "task_write",
	}

	done := make(chan struct{})
	go func() {
		mgr.RequestPlanConfirmationWithSessionKey(context.Background(), req, "feishu:oc_abc123")
		close(done)
	}()

	<-done

	if capturedSessionKey != "feishu:oc_abc123" {
		t.Errorf("sessionKey = %q, want %q", capturedSessionKey, "feishu:oc_abc123")
	}
}

// ---------- P6-11: Mutating Phase (DeriveApprovalScope) 测试 ----------

func TestDeriveApprovalScope_TaskWriteDefaultsPlanConfirm(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "write_code", ToolHint: "write_file"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "plan_confirm" {
		t.Errorf("Type = %q, want plan_confirm", scope.Type)
	}
	if scope.TTLMinutes <= 0 {
		t.Error("TTLMinutes should be > 0")
	}
}

func TestDeriveApprovalScope_BashExecEscalation(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "execute_command", ToolHint: "bash"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "exec_escalation" {
		t.Errorf("Type = %q, want exec_escalation", scope.Type)
	}
	if scope.RequestedLevel == "" {
		t.Error("RequestedLevel should be filled from EscalationHints")
	}
	if !scope.NeedsRunSession {
		t.Error("NeedsRunSession should be true for bash")
	}
}

func TestDeriveApprovalScope_SendFileDataExport(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskLight,
		RequiredActions: []IntentAction{
			{Action: "send_file", ToolHint: "send_media"},
		},
		Targets: []IntentTarget{
			{Kind: "file", Value: "report.pdf", Known: false},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "data_export" {
		t.Errorf("Type = %q, want data_export (send_file + files)", scope.Type)
	}
	if len(scope.ExportFiles) == 0 {
		t.Error("ExportFiles should contain target files")
	}
	if scope.RequestedLevel != "allowlist" {
		t.Errorf("RequestedLevel = %q, want allowlist", scope.RequestedLevel)
	}
	if scope.MountMode != "ro" {
		t.Errorf("MountMode = %q, want ro", scope.MountMode)
	}
	if !scope.NeedsOriginator {
		t.Error("NeedsOriginator should be true for send_media export")
	}
}

func TestDeriveApprovalScope_SendFileKnownAbsolutePathAddsSecondaryMountAccess(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskLight,
		RequiredActions: []IntentAction{
			{Action: "send_file", ToolHint: "send_media"},
		},
		Targets: []IntentTarget{
			{Kind: "file", Value: "/Users/test/Desktop/report.pdf", Known: true},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "data_export" {
		t.Fatalf("Type = %q, want data_export", scope.Type)
	}
	if scope.PrimaryApproval.Type != "data_export" {
		t.Fatalf("PrimaryApproval.Type = %q, want data_export", scope.PrimaryApproval.Type)
	}
	if len(scope.AdditionalApprovals) != 1 {
		t.Fatalf("AdditionalApprovals len = %d, want 1", len(scope.AdditionalApprovals))
	}
	if scope.AdditionalApprovals[0].Type != "mount_access" {
		t.Fatalf("AdditionalApprovals[0].Type = %q, want mount_access", scope.AdditionalApprovals[0].Type)
	}
	if scope.AdditionalApprovals[0].MountPath != "/Users/test/Desktop" {
		t.Fatalf("AdditionalApprovals[0].MountPath = %q, want /Users/test/Desktop", scope.AdditionalApprovals[0].MountPath)
	}
	if scope.AdditionalApprovals[0].TTLMinutes != 0 {
		t.Fatalf("AdditionalApprovals[0].TTLMinutes = %d, want 0 for task-scoped mount access", scope.AdditionalApprovals[0].TTLMinutes)
	}
}

func TestDeriveApprovalScope_SendFilePlusBashAddsSecondaryExportAndMount(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "execute_command", ToolHint: "bash"},
			{Action: "send_file", ToolHint: "send_media"},
		},
		Targets: []IntentTarget{
			{Kind: "file", Value: "/Users/test/Desktop/report.pdf", Known: true},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "exec_escalation" {
		t.Fatalf("Type = %q, want exec_escalation", scope.Type)
	}
	if scope.PrimaryApproval.Type != "exec_escalation" {
		t.Fatalf("PrimaryApproval.Type = %q, want exec_escalation", scope.PrimaryApproval.Type)
	}
	if len(scope.AdditionalApprovals) != 2 {
		t.Fatalf("AdditionalApprovals len = %d, want 2", len(scope.AdditionalApprovals))
	}

	seen := map[string]bool{}
	for _, approval := range scope.AdditionalApprovals {
		seen[approval.Type] = true
		if approval.Type == "mount_access" && approval.TTLMinutes != 0 {
			t.Fatalf("mount_access TTLMinutes = %d, want 0 for task-scoped mount access", approval.TTLMinutes)
		}
	}
	if !seen["data_export"] {
		t.Fatalf("expected additional data_export approval, got %+v", scope.AdditionalApprovals)
	}
	if !seen["mount_access"] {
		t.Fatalf("expected additional mount_access approval, got %+v", scope.AdditionalApprovals)
	}
}

func TestDeriveApprovalScope_ScopeEstimated(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "write_code", ToolHint: "write_file"},
		},
		Targets: []IntentTarget{
			{Kind: "file", Value: "/Users/test/code.go", Known: true},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if len(scope.Scope) == 0 {
		t.Error("Scope should be estimated from known file targets")
	}
}

func TestDeriveApprovalScope_NoApprovalTool(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentQuestion,
		RequiredActions: []IntentAction{
			{Action: "search_info", ToolHint: "search_skills"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "plan_confirm" {
		t.Errorf("Type = %q, want plan_confirm (default)", scope.Type)
	}
}

// ---------- P6-8: Validating Phase (ValidateApprovalScope) 测试 ----------

func TestValidateApprovalScope_ValidTypes(t *testing.T) {
	tree := capabilities.DefaultTree()
	types := []string{"plan_confirm", "exec_escalation", "mount_access", "data_export"}
	for _, typ := range types {
		scope := ApprovalScope{Type: typ, TTLMinutes: 30}
		_, err := ValidateApprovalScope(scope, tree)
		if err != nil {
			t.Errorf("ValidateApprovalScope(%q) error: %v", typ, err)
		}
	}
}

func TestValidateApprovalScope_InvalidType(t *testing.T) {
	tree := capabilities.DefaultTree()
	scope := ApprovalScope{Type: "unknown_type", TTLMinutes: 30}
	_, err := ValidateApprovalScope(scope, tree)
	if err == nil {
		t.Error("ValidateApprovalScope with invalid type should return error")
	}
}

func TestValidateApprovalScope_TTLBounds(t *testing.T) {
	tree := capabilities.DefaultTree()
	s1, _ := ValidateApprovalScope(ApprovalScope{Type: "plan_confirm", TTLMinutes: 0}, tree)
	if s1.TTLMinutes != 30 {
		t.Errorf("TTLMinutes 0 -> %d, want 30", s1.TTLMinutes)
	}
	s2, _ := ValidateApprovalScope(ApprovalScope{Type: "plan_confirm", TTLMinutes: 999}, tree)
	if s2.TTLMinutes != 480 {
		t.Errorf("TTLMinutes 999 -> %d, want 480", s2.TTLMinutes)
	}
}

func TestValidateApprovalScope_ExecEscalationDefaults(t *testing.T) {
	tree := capabilities.DefaultTree()
	scope := ApprovalScope{Type: "exec_escalation", TTLMinutes: 30}
	validated, err := ValidateApprovalScope(scope, tree)
	if err != nil {
		t.Fatalf("ValidateApprovalScope error: %v", err)
	}
	if validated.RequestedLevel != "sandboxed" {
		t.Errorf("RequestedLevel = %q, want sandboxed (safe default)", validated.RequestedLevel)
	}
}

func TestValidateApprovalScope_FullEscalationIsPermanent(t *testing.T) {
	tree := capabilities.DefaultTree()
	scope := ApprovalScope{Type: "exec_escalation", RequestedLevel: "full", TTLMinutes: 999}
	validated, err := ValidateApprovalScope(scope, tree)
	if err != nil {
		t.Fatalf("ValidateApprovalScope error: %v", err)
	}
	if validated.TTLMinutes != 0 {
		t.Errorf("TTLMinutes = %d, want 0 for permanent full escalation", validated.TTLMinutes)
	}
}

func TestValidateApprovalScope_MountAccessDefaults(t *testing.T) {
	tree := capabilities.DefaultTree()
	scope := ApprovalScope{Type: "mount_access", TTLMinutes: 30}
	validated, err := ValidateApprovalScope(scope, tree)
	if err != nil {
		t.Fatalf("ValidateApprovalScope error: %v", err)
	}
	if validated.MountMode != "ro" {
		t.Errorf("MountMode = %q, want ro (safe default)", validated.MountMode)
	}
	if validated.TTLMinutes != 0 {
		t.Errorf("TTLMinutes = %d, want 0 for task-scoped mount access", validated.TTLMinutes)
	}
}

func TestValidateApprovalScope_NormalizesAdditionalApprovals(t *testing.T) {
	tree := capabilities.DefaultTree()
	scope := ApprovalScope{
		Type:       "data_export",
		TTLMinutes: 5,
		AdditionalApprovals: []ApprovalRequirement{
			{Type: "mount_access"},
		},
	}

	validated, err := ValidateApprovalScope(scope, tree)
	if err != nil {
		t.Fatalf("ValidateApprovalScope error: %v", err)
	}
	if validated.PrimaryApproval.Type != "data_export" {
		t.Fatalf("PrimaryApproval.Type = %q, want data_export", validated.PrimaryApproval.Type)
	}
	if len(validated.AdditionalApprovals) != 1 {
		t.Fatalf("AdditionalApprovals len = %d, want 1", len(validated.AdditionalApprovals))
	}
	if validated.AdditionalApprovals[0].MountMode != "ro" {
		t.Fatalf("AdditionalApprovals[0].MountMode = %q, want ro", validated.AdditionalApprovals[0].MountMode)
	}
	if validated.AdditionalApprovals[0].TTLMinutes != 0 {
		t.Fatalf("AdditionalApprovals[0].TTLMinutes = %d, want 0", validated.AdditionalApprovals[0].TTLMinutes)
	}
}

func TestApprovalSummaryFromScope_SendMediaDualApproval(t *testing.T) {
	scope := ApprovalScope{
		Type:            "data_export",
		TTLMinutes:      5,
		NeedsOriginator: true,
		ExportFiles:     []string{"/Users/test/Desktop/report.pdf"},
		MountMode:       "ro",
		MountPath:       "/Users/test/Desktop",
	}
	scope.PrimaryApproval = scope.primaryRequirement()
	scope.AdditionalApprovals = []ApprovalRequirement{
		{
			Type:       "mount_access",
			TTLMinutes: 0,
			MountMode:  "ro",
			MountPath:  "/Users/test/Desktop",
		},
	}

	lines := ApprovalSummaryFromScope(scope)
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if !strings.Contains(lines[0], "主审批") || !strings.Contains(lines[0], "data_export") {
		t.Fatalf("unexpected primary approval summary: %q", lines[0])
	}
	if !strings.Contains(lines[1], "附加审批") || !strings.Contains(lines[1], "mount_access") {
		t.Fatalf("unexpected additional approval summary: %q", lines[1])
	}
	if !strings.Contains(lines[1], "本任务") {
		t.Fatalf("unexpected additional approval summary: %q", lines[1])
	}
}

// ---------- P6-12: EscalationHints 填入测试 ----------

func TestDeriveApprovalScope_EscalationHints_Bash(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "execute_command", ToolHint: "bash"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.RequestedLevel != "sandboxed" {
		t.Errorf("RequestedLevel = %q, want sandboxed", scope.RequestedLevel)
	}
	if scope.TTLMinutes != 30 {
		t.Errorf("TTLMinutes = %d, want 30", scope.TTLMinutes)
	}
	if !scope.NeedsRunSession {
		t.Error("NeedsRunSession should be true")
	}
}

func TestDeriveApprovalScope_EscalationHints_Gateway(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskMultimodal,
		RequiredActions: []IntentAction{
			{Action: "manage_gateway", ToolHint: "gateway"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.RequestedLevel != "full" {
		t.Errorf("RequestedLevel = %q, want full", scope.RequestedLevel)
	}
	if scope.TTLMinutes != 0 {
		t.Errorf("TTLMinutes = %d, want 0 for permanent full escalation", scope.TTLMinutes)
	}
}

func TestDeriveApprovalScope_EscalationHints_HighestPriority(t *testing.T) {
	tree := capabilities.DefaultTree()
	// Both write_file (plan_confirm) and bash (exec_escalation) — should pick exec_escalation
	analysis := IntentAnalysis{
		Tier: intentTaskWrite,
		RequiredActions: []IntentAction{
			{Action: "write_code", ToolHint: "write_file"},
			{Action: "execute_command", ToolHint: "bash"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "exec_escalation" {
		t.Errorf("Type = %q, want exec_escalation (highest priority)", scope.Type)
	}
}

// P6-audit F-01: most-permissive-wins — bash(sandboxed/30) + gateway(full/permanent) → full/permanent
func TestDeriveApprovalScope_EscalationHints_MostPermissiveWins(t *testing.T) {
	tree := capabilities.DefaultTree()
	analysis := IntentAnalysis{
		Tier: intentTaskMultimodal,
		RequiredActions: []IntentAction{
			{Action: "execute_command", ToolHint: "bash"},
			{Action: "manage_gateway", ToolHint: "gateway"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.Type != "exec_escalation" {
		t.Errorf("Type = %q, want exec_escalation", scope.Type)
	}
	if scope.RequestedLevel != "full" {
		t.Errorf("RequestedLevel = %q, want full (most-permissive-wins: full > sandboxed)", scope.RequestedLevel)
	}
	if scope.TTLMinutes != 0 {
		t.Errorf("TTLMinutes = %d, want 0 (full escalation is permanent)", scope.TTLMinutes)
	}
}

// P6-audit F-01: reverse order should yield same result
func TestDeriveApprovalScope_EscalationHints_OrderIndependent(t *testing.T) {
	tree := capabilities.DefaultTree()
	// gateway first, then bash — should still get full/permanent
	analysis := IntentAnalysis{
		Tier: intentTaskMultimodal,
		RequiredActions: []IntentAction{
			{Action: "manage_gateway", ToolHint: "gateway"},
			{Action: "execute_command", ToolHint: "bash"},
		},
	}

	scope := DeriveApprovalScope(analysis, tree)
	if scope.RequestedLevel != "full" {
		t.Errorf("RequestedLevel = %q, want full (order-independent)", scope.RequestedLevel)
	}
	if scope.TTLMinutes != 0 {
		t.Errorf("TTLMinutes = %d, want 0 (order-independent permanent full escalation)", scope.TTLMinutes)
	}
}

type mockDecisionLogger struct {
	logFn func(PlanDecisionRecord) error
}

func (m *mockDecisionLogger) LogPlanDecision(r PlanDecisionRecord) error {
	if m.logFn != nil {
		return m.logFn(r)
	}
	return nil
}
