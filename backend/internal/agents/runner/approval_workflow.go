package runner

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ApprovalTypePlanConfirmRunner    = "plan_confirm"
	ApprovalTypeExecEscalationRunner = "exec_escalation"
	ApprovalTypeMountAccessRunner    = "mount_access"
	ApprovalTypeDataExportRunner     = "data_export"
	ApprovalTypeResultReview         = "result_review"
)

const (
	ApprovalStagePending  = "pending"
	ApprovalStageApproved = "approved"
	ApprovalStageRejected = "rejected"
	ApprovalStageSkipped  = "skipped"
	ApprovalStageEdited   = "edited"
)

// ApprovalWorkflow 描述一条跨阶段审批工作流，用于串联方案确认、执行审批和最终签收。
type ApprovalWorkflow struct {
	ID           string                  `json:"id"`
	TaskBrief    string                  `json:"taskBrief,omitempty"`
	Status       string                  `json:"status,omitempty"`
	CurrentStage string                  `json:"currentStage,omitempty"`
	UpdatedAtMs  int64                   `json:"updatedAtMs,omitempty"`
	Stages       []ApprovalWorkflowStage `json:"stages,omitempty"`
}

// ApprovalWorkflowStage 描述工作流中的单个审批阶段。
type ApprovalWorkflowStage struct {
	Type        string `json:"type"`
	Summary     string `json:"summary,omitempty"`
	Conditional bool   `json:"conditional,omitempty"`
	Status      string `json:"status"`
	RequestID   string `json:"requestId,omitempty"`
	UpdatedAtMs int64  `json:"updatedAtMs,omitempty"`
}

// PermissionDeniedNotice 在工具因权限不足被拦截时向上层回传完整上下文。
type PermissionDeniedNotice struct {
	Tool             string           `json:"tool"`
	Detail           string           `json:"detail"`
	RunID            string           `json:"runId,omitempty"`
	SessionID        string           `json:"sessionId,omitempty"`
	ApprovalWorkflow ApprovalWorkflow `json:"approvalWorkflow,omitempty"`
}

func BuildTaskApprovalWorkflow(taskBrief string, scope ApprovalScope, includePlan bool) ApprovalWorkflow {
	workflow := ApprovalWorkflow{
		ID:        uuid.NewString(),
		TaskBrief: strings.TrimSpace(taskBrief),
	}
	stages := make([]ApprovalWorkflowStage, 0, 1+len(scope.AdditionalApprovals))
	if includePlan {
		stages = append(stages, ApprovalWorkflowStage{
			Type:    ApprovalTypePlanConfirmRunner,
			Summary: approvalRequirementSummary(ApprovalRequirement{Type: ApprovalTypePlanConfirmRunner}, false),
			Status:  ApprovalStagePending,
		})
	}
	if scope.PrimaryApproval.Type != "" && scope.PrimaryApproval.Type != ApprovalTypePlanConfirmRunner {
		stages = append(stages, ApprovalWorkflowStage{
			Type:    scope.PrimaryApproval.Type,
			Summary: approvalRequirementSummary(scope.PrimaryApproval, false),
			Status:  ApprovalStagePending,
		})
	}
	for _, extra := range scope.AdditionalApprovals {
		if extra.Type == "" {
			continue
		}
		stages = append(stages, ApprovalWorkflowStage{
			Type:        extra.Type,
			Summary:     approvalRequirementSummary(extra, true),
			Conditional: true,
			Status:      ApprovalStagePending,
		})
	}
	workflow.Stages = stages
	return workflow.recompute()
}

func NewSingleStageApprovalWorkflow(taskBrief, stageType, summary string) ApprovalWorkflow {
	workflow := ApprovalWorkflow{
		ID:        uuid.NewString(),
		TaskBrief: strings.TrimSpace(taskBrief),
		Stages: []ApprovalWorkflowStage{{
			Type:    stageType,
			Summary: strings.TrimSpace(summary),
			Status:  ApprovalStagePending,
		}},
	}
	return workflow.recompute()
}

func (w ApprovalWorkflow) MarkStagePending(stageType, requestID string) ApprovalWorkflow {
	now := time.Now().UnixMilli()
	for i := range w.Stages {
		if w.Stages[i].Type != stageType {
			continue
		}
		w.Stages[i].Status = ApprovalStagePending
		if requestID != "" {
			w.Stages[i].RequestID = requestID
		}
		w.Stages[i].UpdatedAtMs = now
		break
	}
	w.UpdatedAtMs = now
	return w.recompute()
}

func (w ApprovalWorkflow) MarkStageResolved(stageType, requestID, action string) ApprovalWorkflow {
	status := ApprovalStagePending
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve", "allow":
		status = ApprovalStageApproved
	case "edit":
		status = ApprovalStageEdited
	case "reject", "deny":
		status = ApprovalStageRejected
	case "skip":
		status = ApprovalStageSkipped
	default:
		// 未知 action 映射为 pending（安全默认），防止任意字符串被 recompute 视为 approved
		status = ApprovalStagePending
	}
	now := time.Now().UnixMilli()
	for i := range w.Stages {
		if w.Stages[i].Type != stageType {
			continue
		}
		w.Stages[i].Status = status
		if requestID != "" {
			w.Stages[i].RequestID = requestID
		}
		w.Stages[i].UpdatedAtMs = now
		break
	}
	w.UpdatedAtMs = now
	return w.recompute()
}

func (w ApprovalWorkflow) MarkStageSkipped(stageType string) ApprovalWorkflow {
	now := time.Now().UnixMilli()
	for i := range w.Stages {
		if w.Stages[i].Type != stageType {
			continue
		}
		w.Stages[i].Status = ApprovalStageSkipped
		w.Stages[i].UpdatedAtMs = now
		break
	}
	w.UpdatedAtMs = now
	return w.recompute()
}

func (w ApprovalWorkflow) StageInfo(stageType string) (ApprovalWorkflowStage, int, int, bool) {
	total := len(w.Stages)
	for i := range w.Stages {
		if w.Stages[i].Type == stageType {
			return w.Stages[i], i + 1, total, true
		}
	}
	return ApprovalWorkflowStage{}, 0, total, false
}

func (w ApprovalWorkflow) NextStageSummaries(stageType string) []string {
	_, idx, _, ok := w.StageInfo(stageType)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(w.Stages)-idx)
	for i := idx; i < len(w.Stages); i++ {
		summary := strings.TrimSpace(w.Stages[i].Summary)
		if summary == "" {
			summary = w.Stages[i].Type
		}
		out = append(out, summary)
	}
	return out
}

func (w ApprovalWorkflow) recompute() ApprovalWorkflow {
	now := time.Now().UnixMilli()
	if w.UpdatedAtMs == 0 {
		w.UpdatedAtMs = now
	}
	current := ""
	status := ApprovalStageApproved
	hasStage := false
	for i := range w.Stages {
		if w.Stages[i].UpdatedAtMs == 0 {
			w.Stages[i].UpdatedAtMs = w.UpdatedAtMs
		}
		hasStage = true
		switch w.Stages[i].Status {
		case ApprovalStageRejected:
			status = ApprovalStageRejected
			current = w.Stages[i].Type
			w.CurrentStage = current
			w.Status = status
			return w
		case ApprovalStagePending:
			if status != ApprovalStageRejected {
				status = ApprovalStagePending
			}
			if current == "" {
				current = w.Stages[i].Type
			}
		case ApprovalStageEdited:
			if status != ApprovalStageRejected && status != ApprovalStagePending {
				status = ApprovalStageEdited
			}
			if current == "" {
				current = w.Stages[i].Type
			}
		}
	}
	if !hasStage {
		status = ""
	}
	// edited 是"批准但有修改"，workflow 整体已完成。
	// 如果没有 pending/rejected，将 edited 视为 approved（附带 edited 语义）。
	if status == ApprovalStageEdited {
		status = ApprovalStageApproved
	}
	if current == "" && len(w.Stages) > 0 {
		current = w.Stages[len(w.Stages)-1].Type
	}
	w.CurrentStage = current
	w.Status = status
	return w
}

func broadcastApprovalWorkflow(broadcast CoderConfirmBroadcastFunc, workflow ApprovalWorkflow, source, requestID string) {
	if broadcast == nil || workflow.ID == "" {
		return
	}
	broadcast("approval.workflow.updated", map[string]interface{}{
		"source":    source,
		"requestId": requestID,
		"workflow":  workflow,
		"ts":        time.Now().UnixMilli(),
	})
}
