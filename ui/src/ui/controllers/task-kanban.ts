// ---------- Task Kanban Controller ----------
// 处理后端 task.* WS 事件，维护前端任务状态 map。

export type TaskStatus = "queued" | "started" | "progress" | "completed" | "failed";

export type TaskKanbanItem = {
  taskId: string;
  sessionKey: string;
  text: string;
  status: TaskStatus;
  async: boolean;
  toolName?: string;
  toolId?: string;
  phase?: string;
  progressText?: string;
  isError?: boolean;
  duration?: number;
  summary?: string;
  error?: string;
  queuedAt: number;
  startedAt?: number;
  completedAt?: number;
};

export type TaskKanbanState = {
  tasks: Map<string, TaskKanbanItem>;
  /** 排序后的任务 ID 列表缓存（避免每次渲染重排） */
  sortedIds: string[];
};

export function createTaskKanbanState(): TaskKanbanState {
  return { tasks: new Map(), sortedIds: [] };
}

/** 从 WS task.* 事件更新状态，返回新的 state（不可变更新）。 */
export function handleTaskEvent(
  state: TaskKanbanState,
  event: string,
  payload: Record<string, unknown> | undefined,
): TaskKanbanState {
  if (!payload) return state;

  const taskId = payload.taskId as string | undefined;
  if (!taskId) return state;

  const next = new Map(state.tasks);

  switch (event) {
    case "task.queued": {
      next.set(taskId, {
        taskId,
        sessionKey: (payload.sessionKey as string) ?? "",
        text: (payload.text as string) ?? "",
        status: "queued",
        async: (payload.async as boolean) ?? false,
        queuedAt: (payload.ts as number) ?? Date.now(),
      });
      break;
    }
    case "task.started": {
      const existing = next.get(taskId);
      if (existing) {
        next.set(taskId, {
          ...existing,
          status: "started",
          startedAt: (payload.ts as number) ?? Date.now(),
        });
      } else {
        // 可能没收到 queued 事件（非 async 任务）
        next.set(taskId, {
          taskId,
          sessionKey: (payload.sessionKey as string) ?? "",
          text: "",
          status: "started",
          async: false,
          queuedAt: (payload.ts as number) ?? Date.now(),
          startedAt: (payload.ts as number) ?? Date.now(),
        });
      }
      break;
    }
    case "task.progress": {
      const existing = next.get(taskId);
      if (existing) {
        next.set(taskId, {
          ...existing,
          status: "progress",
          toolName: (payload.toolName as string) ?? existing.toolName,
          toolId: (payload.toolId as string) ?? existing.toolId,
          phase: (payload.phase as string) ?? existing.phase,
          progressText: (payload.text as string) ?? existing.progressText,
          isError: (payload.isError as boolean) ?? false,
          duration: (payload.duration as number) ?? existing.duration,
        });
      } else {
        // 兜底：页面刷新后收到在途任务的 progress 事件，创建最小条目
        next.set(taskId, {
          taskId,
          sessionKey: (payload.sessionKey as string) ?? "",
          text: "",
          status: "progress",
          async: false,
          toolName: (payload.toolName as string) ?? undefined,
          toolId: (payload.toolId as string) ?? undefined,
          phase: (payload.phase as string) ?? undefined,
          progressText: (payload.text as string) ?? undefined,
          isError: (payload.isError as boolean) ?? false,
          duration: (payload.duration as number) ?? undefined,
          queuedAt: (payload.ts as number) ?? Date.now(),
          startedAt: (payload.ts as number) ?? Date.now(),
        });
      }
      break;
    }
    case "task.completed": {
      const existing = next.get(taskId);
      if (existing) {
        next.set(taskId, {
          ...existing,
          status: "completed",
          summary: (payload.summary as string) ?? "",
          completedAt: (payload.ts as number) ?? Date.now(),
        });
      } else {
        // 兜底：页面刷新后收到在途任务的 completed 事件
        next.set(taskId, {
          taskId,
          sessionKey: (payload.sessionKey as string) ?? "",
          text: "",
          status: "completed",
          async: false,
          summary: (payload.summary as string) ?? "",
          queuedAt: (payload.ts as number) ?? Date.now(),
          completedAt: (payload.ts as number) ?? Date.now(),
        });
      }
      break;
    }
    case "task.failed": {
      const existing = next.get(taskId);
      if (existing) {
        next.set(taskId, {
          ...existing,
          status: "failed",
          error: (payload.error as string) ?? "unknown error",
          completedAt: (payload.ts as number) ?? Date.now(),
        });
      } else {
        // 兜底：页面刷新后收到在途任务的 failed 事件
        next.set(taskId, {
          taskId,
          sessionKey: (payload.sessionKey as string) ?? "",
          text: "",
          status: "failed",
          async: false,
          error: (payload.error as string) ?? "unknown error",
          queuedAt: (payload.ts as number) ?? Date.now(),
          completedAt: (payload.ts as number) ?? Date.now(),
        });
      }
      break;
    }
    default:
      return state;
  }

  return {
    tasks: next,
    sortedIds: buildSortedIds(next),
  };
}

/** 清除已完成/失败超过 5 分钟的任务，保持列表整洁。 */
export function pruneCompletedTasks(state: TaskKanbanState): TaskKanbanState {
  const now = Date.now();
  const cutoff = 5 * 60 * 1000; // 5 分钟
  const next = new Map(state.tasks);
  let changed = false;

  for (const [id, task] of next) {
    if (
      (task.status === "completed" || task.status === "failed") &&
      task.completedAt &&
      now - task.completedAt > cutoff
    ) {
      next.delete(id);
      changed = true;
    }
  }

  if (!changed) return state;
  return { tasks: next, sortedIds: buildSortedIds(next) };
}

/** 按状态分组并按时间排序。 */
function buildSortedIds(tasks: Map<string, TaskKanbanItem>): string[] {
  const arr = Array.from(tasks.values());
  // 状态优先级：queued > started/progress > completed > failed
  const statusOrder: Record<TaskStatus, number> = {
    queued: 0,
    started: 1,
    progress: 1,
    completed: 2,
    failed: 3,
  };
  arr.sort((a, b) => {
    const so = statusOrder[a.status] - statusOrder[b.status];
    if (so !== 0) return so;
    return a.queuedAt - b.queuedAt;
  });
  return arr.map((t) => t.taskId);
}

// ---------- 持久化加载 ----------

/** tasks.list RPC 响应条目（与后端 TaskListEntry 对齐）。 */
export type TaskListEntry = {
  taskId: string;
  sessionKey: string;
  text: string;
  status: string;
  async?: boolean;
  summary?: string;
  error?: string;
  toolName?: string;
  phase?: string;
  progressText?: string;
  isError?: boolean;
  duration?: number;
  queuedAt: number;
  progressAt?: number;
  startedAt?: number;
  completedAt?: number;
};

/** 将 tasks.list RPC 返回的历史任务合并入看板状态（WS 实时事件优先）。 */
export function mergeLoadedTasks(
  state: TaskKanbanState,
  loaded: TaskListEntry[],
): TaskKanbanState {
  const tasks = new Map(state.tasks);
  for (const t of loaded) {
    // WS 实时事件优先（已有的不覆盖）
    if (tasks.has(t.taskId)) continue;
    tasks.set(t.taskId, {
      taskId: t.taskId,
      sessionKey: t.sessionKey,
      text: t.text,
      status: mapStatus(t.status),
      async: t.async ?? false,
      summary: t.summary,
      error: t.error,
      toolName: t.toolName,
      phase: t.phase,
      progressText: t.progressText,
      isError: t.isError ?? false,
      duration: t.duration,
      queuedAt: t.queuedAt,
      startedAt: t.startedAt,
      completedAt: t.completedAt,
    });
  }
  return { tasks, sortedIds: buildSortedIds(tasks) };
}

function mapStatus(s: string): TaskStatus {
  if (s === "queued" || s === "started" || s === "progress" || s === "completed" || s === "failed") {
    return s;
  }
  return "queued";
}

/** 辅助：按看板列分组。 */
export function groupByColumn(state: TaskKanbanState): {
  queued: TaskKanbanItem[];
  running: TaskKanbanItem[];
  done: TaskKanbanItem[];
} {
  const queued: TaskKanbanItem[] = [];
  const running: TaskKanbanItem[] = [];
  const done: TaskKanbanItem[] = [];

  for (const id of state.sortedIds) {
    const task = state.tasks.get(id);
    if (!task) continue;
    switch (task.status) {
      case "queued":
        queued.push(task);
        break;
      case "started":
      case "progress":
        running.push(task);
        break;
      case "completed":
      case "failed":
        done.push(task);
        break;
    }
  }

  return { queued, running, done };
}
