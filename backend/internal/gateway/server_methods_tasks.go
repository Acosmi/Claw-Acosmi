package gateway

// server_methods_tasks.go — tasks.list RPC（任务看板持久化）
//
// 从 SessionStore 读取 task: 前缀的 session，将 TaskMeta 组装为看板条目返回前端。

import (
	"sort"
	"strings"
)

// TaskKanbanHandlers 返回 tasks.* 方法处理器映射。
func TaskKanbanHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"tasks.list": handleTasksList,
	}
}

// TaskListEntry tasks.list 响应条目。
type TaskListEntry struct {
	TaskID       string `json:"taskId"`
	SessionKey   string `json:"sessionKey"`
	Text         string `json:"text"`
	Status       string `json:"status"`
	Async        bool   `json:"async,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Error        string `json:"error,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	Phase        string `json:"phase,omitempty"`
	ProgressText string `json:"progressText,omitempty"`
	IsError      bool   `json:"isError,omitempty"`
	Duration     int64  `json:"duration,omitempty"`
	QueuedAt     int64  `json:"queuedAt"`
	ProgressAt   int64  `json:"progressAt,omitempty"`
	StartedAt    int64  `json:"startedAt,omitempty"`
	CompletedAt  int64  `json:"completedAt,omitempty"`
}

func handleTasksList(ctx *MethodHandlerContext) {
	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(true, map[string]interface{}{"tasks": []interface{}{}}, nil)
		return
	}

	limitF, _ := ctx.Params["limit"].(float64)
	limit := int(limitF)
	if limit <= 0 {
		limit = 100
	}

	statusFilter, _ := ctx.Params["status"].(string)
	taskIDFilter, _ := ctx.Params["taskId"].(string)

	entries := store.List()
	var tasks []TaskListEntry
	for _, entry := range entries {
		if !IsTaskSessionKey(entry.SessionKey) {
			continue
		}
		if entry.TaskMeta == nil {
			continue
		}

		if statusFilter != "" && entry.TaskMeta.Status != statusFilter {
			continue
		}

		taskId := strings.TrimPrefix(entry.SessionKey, "task:")
		if taskIDFilter != "" && taskId != taskIDFilter {
			continue
		}

		tasks = append(tasks, TaskListEntry{
			TaskID:       taskId,
			SessionKey:   entry.SessionKey,
			Text:         entry.Label,
			Status:       entry.TaskMeta.Status,
			Async:        entry.TaskMeta.Async,
			Summary:      entry.TaskMeta.Summary,
			Error:        entry.TaskMeta.Error,
			ToolName:     entry.TaskMeta.ToolName,
			Phase:        entry.TaskMeta.ProgressPhase,
			ProgressText: entry.TaskMeta.ProgressText,
			IsError:      entry.TaskMeta.ProgressIsError,
			Duration:     entry.TaskMeta.ProgressDuration,
			QueuedAt:     entry.CreatedAt,
			ProgressAt:   entry.TaskMeta.ProgressAt,
			StartedAt:    entry.TaskMeta.StartedAt,
			CompletedAt:  entry.TaskMeta.CompletedAt,
		})
	}

	// 按 QueuedAt 倒序
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].QueuedAt > tasks[j].QueuedAt })
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}

	// 确保 JSON 输出 [] 而非 null
	if tasks == nil {
		tasks = []TaskListEntry{}
	}

	ctx.Respond(true, map[string]interface{}{"tasks": tasks}, nil)
}
