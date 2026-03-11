package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
	"github.com/Acosmi/ClawAcosmi/internal/autoreply"
	sessiontypes "github.com/Acosmi/ClawAcosmi/internal/session"
)

func TestTaskKanbanHandlers_ListIncludesProgressFields(t *testing.T) {
	store := NewSessionStore("")
	now := time.Now().UnixMilli()
	store.Save(&SessionEntry{
		SessionKey: "task:run-progress",
		SessionId:  "run-progress",
		Label:      "refresh task",
		CreatedAt:  now - 1000,
		UpdatedAt:  now,
		TaskMeta: &sessiontypes.TaskMeta{
			Status:           "progress",
			Async:            true,
			ToolName:         "bash",
			ProgressPhase:    "end",
			ProgressText:     "[结果] ls ok (42ms)",
			ProgressIsError:  false,
			ProgressDuration: 42,
			ProgressAt:       now,
			StartedAt:        now - 500,
		},
	})

	payload := callTasksList(t, store, map[string]interface{}{})
	tasks, ok := payload["tasks"].([]TaskListEntry)
	if !ok {
		t.Fatalf("expected []TaskListEntry, got %T", payload["tasks"])
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	got := tasks[0]
	if got.Status != "progress" {
		t.Fatalf("status = %q, want progress", got.Status)
	}
	if got.ToolName != "bash" {
		t.Fatalf("toolName = %q, want bash", got.ToolName)
	}
	if got.Phase != "end" {
		t.Fatalf("phase = %q, want end", got.Phase)
	}
	if got.ProgressText != "[结果] ls ok (42ms)" {
		t.Fatalf("progressText = %q", got.ProgressText)
	}
	if got.IsError {
		t.Fatal("isError should be false")
	}
	if got.Duration != 42 {
		t.Fatalf("duration = %d, want 42", got.Duration)
	}
	if got.ProgressAt != now {
		t.Fatalf("progressAt = %d, want %d", got.ProgressAt, now)
	}
}

func TestTaskKanbanHandlers_ListFiltersByTaskID(t *testing.T) {
	store := NewSessionStore("")
	now := time.Now().UnixMilli()
	store.Save(&SessionEntry{
		SessionKey: "task:run-a",
		SessionId:  "run-a",
		Label:      "task a",
		CreatedAt:  now - 2000,
		UpdatedAt:  now - 2000,
		TaskMeta:   &sessiontypes.TaskMeta{Status: "completed"},
	})
	store.Save(&SessionEntry{
		SessionKey: "task:run-b",
		SessionId:  "run-b",
		Label:      "task b",
		CreatedAt:  now - 1000,
		UpdatedAt:  now - 1000,
		TaskMeta:   &sessiontypes.TaskMeta{Status: "failed"},
	})

	payload := callTasksList(t, store, map[string]interface{}{"taskId": "run-b"})
	tasks, ok := payload["tasks"].([]TaskListEntry)
	if !ok {
		t.Fatalf("expected []TaskListEntry, got %T", payload["tasks"])
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 filtered task, got %d", len(tasks))
	}
	if tasks[0].TaskID != "run-b" {
		t.Fatalf("taskId = %q, want run-b", tasks[0].TaskID)
	}
}

func TestChatSend_PersistsTaskProgressForTasksList(t *testing.T) {
	sessionID := "tasks-progress-session"
	sessionKey := "agent:default:tasks-progress"
	storePath, store := setupE2EStore(t, sessionKey, sessionID)
	chatState := NewChatRunState()
	broadcaster := NewBroadcaster()
	progressReady := make(chan struct{})
	unblock := make(chan struct{})

	dispatcher := func(ctx context.Context, msgCtx *autoreply.MsgContext, opts *autoreply.GetReplyOptions) ([]autoreply.ReplyPayload, error) {
		cb, ok := opts.OnToolEvent.(func(runner.ToolEvent))
		if !ok || cb == nil {
			return nil, fmt.Errorf("expected OnToolEvent callback")
		}
		cb(runner.ToolEvent{
			Phase:    "start",
			ToolName: "bash",
			ToolID:   "tool_1",
			Args:     "ls -la",
		})
		close(progressReady)
		<-unblock
		cb(runner.ToolEvent{
			Phase:    "end",
			ToolName: "bash",
			ToolID:   "tool_1",
			Result:   "ok",
			Duration: 42,
		})
		return []autoreply.ReplyPayload{{Text: "done"}}, nil
	}

	taskStore := NewTaskStore("")

	r := NewMethodRegistry()
	r.RegisterAll(ChatHandlers())
	r.RegisterAll(TaskKanbanHandlers())

	var sendPayload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "chat.send", Params: map[string]interface{}{
		"sessionKey": sessionKey,
		"text":       "show task progress",
		"agentId":    "default",
		"async":      true,
	}}, nil, &GatewayMethodContext{
		ChatState:          chatState,
		StorePath:          storePath,
		SessionStore:       store,
		TaskStore:          taskStore,
		PipelineDispatcher: dispatcher,
		Broadcaster:        broadcaster,
	}, func(ok bool, payload interface{}, err *ErrorShape) {
		if !ok {
			t.Fatalf("chat.send failed: %+v", err)
		}
		sendPayload = payload
	})

	sendResp, ok := sendPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected send payload map, got %T", sendPayload)
	}
	runID, _ := sendResp["runId"].(string)
	if runID == "" {
		t.Fatal("expected runId")
	}

	select {
	case <-progressReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task progress")
	}

	progressTasks := extractTasksFromPayload(t, callTasksList(t, store, map[string]interface{}{"taskId": runID}, taskStore))
	if len(progressTasks) != 1 {
		t.Fatalf("expected 1 progress task, got %d", len(progressTasks))
	}
	progress := progressTasks[0]
	if progress.Status != "progress" {
		t.Fatalf("status = %q, want progress", progress.Status)
	}
	if progress.ToolName != "bash" {
		t.Fatalf("toolName = %q, want bash", progress.ToolName)
	}
	if progress.Phase != "start" {
		t.Fatalf("phase = %q, want start", progress.Phase)
	}
	if !strings.Contains(progress.ProgressText, "[工具] bash: ls -la") {
		t.Fatalf("unexpected progressText: %q", progress.ProgressText)
	}

	close(unblock)

	var completed TaskListEntry
	waitForTaskListEntry(t, 2*time.Second, func() (TaskListEntry, bool) {
		tasks := extractTasksFromPayload(t, callTasksList(t, store, map[string]interface{}{"taskId": runID}, taskStore))
		if len(tasks) != 1 {
			return TaskListEntry{}, false
		}
		if tasks[0].Status != "completed" {
			return TaskListEntry{}, false
		}
		return tasks[0], true
	}, &completed)

	if completed.ToolName != "bash" {
		t.Fatalf("completed toolName = %q, want bash", completed.ToolName)
	}
	if completed.Phase != "end" {
		t.Fatalf("completed phase = %q, want end", completed.Phase)
	}
	if !strings.Contains(completed.ProgressText, "[结果] ok (42ms)") {
		t.Fatalf("unexpected completed progressText: %q", completed.ProgressText)
	}
	if completed.Duration != 42 {
		t.Fatalf("completed duration = %d, want 42", completed.Duration)
	}
	if completed.Summary != "done" {
		t.Fatalf("summary = %q, want done", completed.Summary)
	}
}

func callTasksList(t *testing.T, store *SessionStore, params map[string]interface{}, taskStores ...*TaskStore) map[string]interface{} {
	t.Helper()
	r := NewMethodRegistry()
	r.RegisterAll(TaskKanbanHandlers())

	mctx := &GatewayMethodContext{SessionStore: store}
	if len(taskStores) > 0 {
		mctx.TaskStore = taskStores[0]
	}

	var payload interface{}
	HandleGatewayRequest(r, &RequestFrame{Method: "tasks.list", Params: params}, nil, mctx, func(ok bool, got interface{}, err *ErrorShape) {
		if !ok {
			t.Fatalf("tasks.list failed: %+v", err)
		}
		payload = got
	})

	result, ok := payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected tasks.list map payload, got %T", payload)
	}
	return result
}

func extractTasksFromPayload(t *testing.T, payload map[string]interface{}) []TaskListEntry {
	t.Helper()
	tasks, ok := payload["tasks"].([]TaskListEntry)
	if !ok {
		t.Fatalf("expected []TaskListEntry, got %T", payload["tasks"])
	}
	return tasks
}

func waitForTaskListEntry(t *testing.T, timeout time.Duration, fn func() (TaskListEntry, bool), dest *TaskListEntry) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if entry, ok := fn(); ok {
			*dest = entry
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for task list entry")
}
