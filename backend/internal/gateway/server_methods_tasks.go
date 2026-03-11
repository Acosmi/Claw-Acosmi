package gateway

// server_methods_tasks.go — tasks.list RPC（任务看板持久化）
//
// P1 域隔离: 优先从独立 TaskStore 读取，同时回退读取 SessionStore 中的遗留 task: 条目。
// 过渡期结束（P3 完成后）可删除 SessionStore 回退逻辑。

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
	limitF, _ := ctx.Params["limit"].(float64)
	limit := int(limitF)
	if limit <= 0 {
		limit = 100
	}

	statusFilter, _ := ctx.Params["status"].(string)
	taskIDFilter, _ := ctx.Params["taskId"].(string)

	// 去重 map: taskId → true（TaskStore 优先，SessionStore 遗留条目不覆盖）
	seen := make(map[string]bool)
	var tasks []TaskListEntry

	// 1. 优先从 TaskStore 读取（P1 新路径）
	if ts := ctx.Context.TaskStore; ts != nil {
		for _, entry := range ts.List() {
			if statusFilter != "" && entry.Status != statusFilter {
				continue
			}
			if taskIDFilter != "" && entry.TaskID != taskIDFilter {
				continue
			}
			seen[entry.TaskID] = true

			listEntry := TaskListEntry{
				TaskID:     entry.TaskID,
				SessionKey: entry.SessionKey,
				Text:       entry.Label,
				Status:     entry.Status,
				Async:      entry.Async,
				Summary:    entry.Summary,
				Error:      entry.Error,
				QueuedAt:   entry.CreatedAt,
			}
			if entry.TaskMeta != nil {
				listEntry.ToolName = entry.TaskMeta.ToolName
				listEntry.Phase = entry.TaskMeta.ProgressPhase
				listEntry.ProgressText = entry.TaskMeta.ProgressText
				listEntry.IsError = entry.TaskMeta.ProgressIsError
				listEntry.Duration = entry.TaskMeta.ProgressDuration
				listEntry.ProgressAt = entry.TaskMeta.ProgressAt
				listEntry.StartedAt = entry.TaskMeta.StartedAt
				listEntry.CompletedAt = entry.TaskMeta.CompletedAt
			}
			tasks = append(tasks, listEntry)
		}
	}

	// 2. 回退：从 SessionStore 读取遗留 task: 条目（过渡期兼容）
	if store := ctx.Context.SessionStore; store != nil {
		for _, entry := range store.List() {
			if !IsTaskSessionKey(entry.SessionKey) {
				continue
			}
			if entry.TaskMeta == nil {
				continue
			}
			taskId := strings.TrimPrefix(entry.SessionKey, "task:")
			if seen[taskId] {
				continue // TaskStore 已有此条目，跳过遗留
			}
			if statusFilter != "" && entry.TaskMeta.Status != statusFilter {
				continue
			}
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
