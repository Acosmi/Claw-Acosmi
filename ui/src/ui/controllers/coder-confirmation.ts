// ---------- Coder 确认流控制器 ----------
// 复用 exec-approval.ts 的模式：解析 WebSocket 事件 → 队列管理 → 过期清理。

export type CoderConfirmPreview = {
  filePath?: string;
  oldString?: string;
  newString?: string;
  content?: string;
  command?: string;
  lineCount?: number;
};

export type CoderConfirmRequest = {
  id: string;
  toolName: string; // "edit" | "write" | "bash" | "send_media"
  preview: CoderConfirmPreview;
  createdAtMs: number;
  expiresAtMs: number;
};

export type CoderConfirmResolved = {
  id: string;
  decision: string; // "allow" | "deny"
  ts?: number | null;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/**
 * 解析 "coder.confirm.requested" WebSocket 事件 payload。
 */
export function parseCoderConfirmRequested(payload: unknown): CoderConfirmRequest | null {
  if (!isRecord(payload)) return null;

  const id = typeof payload.id === "string" ? payload.id.trim() : "";
  const toolName = typeof payload.toolName === "string" ? payload.toolName.trim() : "";
  if (!id || !toolName) return null;

  const createdAtMs = typeof payload.createdAtMs === "number" ? payload.createdAtMs : 0;
  const expiresAtMs = typeof payload.expiresAtMs === "number" ? payload.expiresAtMs : 0;
  if (!createdAtMs || !expiresAtMs) return null;

  const rawPreview = isRecord(payload.preview) ? payload.preview : {};
  const preview: CoderConfirmPreview = {
    filePath: typeof rawPreview.filePath === "string" ? rawPreview.filePath : undefined,
    oldString: typeof rawPreview.oldString === "string" ? rawPreview.oldString : undefined,
    newString: typeof rawPreview.newString === "string" ? rawPreview.newString : undefined,
    content: typeof rawPreview.content === "string" ? rawPreview.content : undefined,
    command: typeof rawPreview.command === "string" ? rawPreview.command : undefined,
    lineCount: typeof rawPreview.lineCount === "number" ? rawPreview.lineCount : undefined,
  };

  return { id, toolName, preview, createdAtMs, expiresAtMs };
}

/**
 * 解析 "coder.confirm.resolved" WebSocket 事件 payload。
 */
export function parseCoderConfirmResolved(payload: unknown): CoderConfirmResolved | null {
  if (!isRecord(payload)) return null;
  const id = typeof payload.id === "string" ? payload.id.trim() : "";
  if (!id) return null;
  return {
    id,
    decision: typeof payload.decision === "string" ? payload.decision : "deny",
    ts: typeof payload.ts === "number" ? payload.ts : null,
  };
}

function pruneQueue(queue: CoderConfirmRequest[]): CoderConfirmRequest[] {
  const now = Date.now();
  return queue.filter((entry) => entry.expiresAtMs > now);
}

export function addCoderConfirm(
  queue: CoderConfirmRequest[],
  entry: CoderConfirmRequest,
): CoderConfirmRequest[] {
  const next = pruneQueue(queue).filter((item) => item.id !== entry.id);
  next.push(entry);
  return next;
}

export function removeCoderConfirm(
  queue: CoderConfirmRequest[],
  id: string,
): CoderConfirmRequest[] {
  return pruneQueue(queue).filter((entry) => entry.id !== id);
}
