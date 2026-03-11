export type ChatUxMode = "classic" | "codex-readonly";

export type ReadonlyRunPhase =
  | "idle"
  | "starting"
  | "working"
  | "drafting"
  | "finalizing"
  | "complete"
  | "aborted"
  | "error";

export type ReadonlyToolStepStatus = "running" | "done" | "error";

export type ReadonlyToolStep = {
  toolCallId: string;
  name: string;
  detail: string | null;
  status: ReadonlyToolStepStatus;
  startedAt: number;
  updatedAt: number;
  outputPreview: string | null;
};

export type ReadonlyRunActivity =
  | {
    id: string;
    kind: "phase";
    phase: Exclude<ReadonlyRunPhase, "idle">;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "progress";
    summary: string;
    progressPhase: string | null;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "tool";
    toolCallId: string;
    name: string;
    detail: string | null;
    status: ReadonlyToolStepStatus;
    phase: string;
    outputPreview: string | null;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "draft";
    text: string;
    ts: number;
    updatedAt: number;
  }
  | {
    id: string;
    kind: "error";
    message: string;
    ts: number;
    updatedAt: number;
  };

export type ChatReadonlyRunState = {
  runId: string | null;
  sessionKey: string;
  phase: ReadonlyRunPhase;
  startedAt: number | null;
  updatedAt: number | null;
  completedAt: number | null;
  latestProgress: string | null;
  progressPhase: string | null;
  draftingText: string | null;
  lastToolName: string | null;
  toolSteps: ReadonlyToolStep[];
  activity: ReadonlyRunActivity[];
  lastError: string | null;
  finalMessageId: string | null;
  finalMessageTimestamp: number | null;
  finalMessageText: string | null;
};

export type ChatReadonlyRunHost = {
  sessionKey: string;
  chatReadonlyRun: ChatReadonlyRunState;
  chatReadonlyRunHistory?: ChatReadonlyRunState[];
};

export type ChatReadonlyRunStorageState = {
  current: ChatReadonlyRunState | null;
  history: ChatReadonlyRunState[];
};

type RunBindingInput = {
  runId: string;
  sessionKey?: string | null;
  ts: number;
};

type ProgressInput = RunBindingInput & {
  summary: string;
  phase?: string | null;
};

type ToolInput = RunBindingInput & {
  toolCallId: string;
  name: string;
  detail?: string | null;
  phase: string;
  isError?: boolean;
  output?: string | null;
};

type ChatInput = {
  runId: string;
  sessionKey: string;
  state: "delta" | "final" | "aborted" | "error";
  ts: number;
  text?: string | null;
  errorMessage?: string | null;
  messageId?: string | null;
  messageTimestamp?: number | null;
};

const READONLY_TOOL_STEP_LIMIT = 8;
const READONLY_ACTIVITY_LIMIT = 12;
const READONLY_OUTPUT_PREVIEW_LIMIT = 240;
const READONLY_RUN_HISTORY_LIMIT = 24;
const READONLY_RUN_STORAGE_PREFIX = "openacosmi.control.chat-readonly-run.v1";

export function createChatReadonlyRunState(sessionKey: string): ChatReadonlyRunState {
  return {
    runId: null,
    sessionKey,
    phase: "idle",
    startedAt: null,
    updatedAt: null,
    completedAt: null,
    latestProgress: null,
    progressPhase: null,
    draftingText: null,
    lastToolName: null,
    toolSteps: [],
    activity: [],
    lastError: null,
    finalMessageId: null,
    finalMessageTimestamp: null,
    finalMessageText: null,
  };
}

export function normalizeReadonlyRunAnchorText(text: string | null | undefined): string | null {
  if (typeof text !== "string") {
    return null;
  }
  const normalized = text.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return null;
  }
  return normalized.slice(0, 280);
}

function readonlyRunStorageKey(sessionKey: string): string {
  return `${READONLY_RUN_STORAGE_PREFIX}:${sessionKey}`;
}

function isPersistableReadonlyRunPhase(
  phase: ChatReadonlyRunState["phase"] | null | undefined,
): phase is Exclude<ChatReadonlyRunState["phase"], "idle"> {
  return (
    phase === "starting" ||
    phase === "working" ||
    phase === "drafting" ||
    phase === "finalizing" ||
    phase === "complete" ||
    phase === "aborted" ||
    phase === "error"
  );
}

export function isReadonlyRunTerminal(
  run: ChatReadonlyRunState | null | undefined,
): run is ChatReadonlyRunState & { phase: "complete" | "aborted" | "error" } {
  return Boolean(run && isTerminalReadonlyRunPhase(run.phase));
}

function isTerminalReadonlyRunPhase(
  phase: ChatReadonlyRunState["phase"] | null | undefined,
): phase is "complete" | "aborted" | "error" {
  return phase === "complete" || phase === "aborted" || phase === "error";
}

function shouldPersistReadonlyRun(run: ChatReadonlyRunState | null | undefined): boolean {
  if (!run) {
    return false;
  }
  if (!isPersistableReadonlyRunPhase(run.phase)) {
    return false;
  }
  if (isTerminalReadonlyRunPhase(run.phase) && !hasReadonlyRunAnchor(run)) {
    return false;
  }
  if (
    (run.phase === "starting" || run.phase === "working" || run.phase === "drafting" || run.phase === "finalizing") &&
    !run.runId
  ) {
    return false;
  }
  return true;
}

function normalizePersistedReadonlyRun(
  value: unknown,
  sessionKey: string,
): ChatReadonlyRunState | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const parsed = value as Partial<ChatReadonlyRunState>;
  if (
    typeof parsed.sessionKey !== "string" ||
    parsed.sessionKey !== sessionKey ||
    !isPersistableReadonlyRunPhase(parsed.phase)
  ) {
    return null;
  }
  const base = createChatReadonlyRunState(sessionKey);
  return {
    ...base,
    ...parsed,
    sessionKey,
    phase: parsed.phase,
    runId: typeof parsed.runId === "string" && parsed.runId.trim() ? parsed.runId : null,
    startedAt: typeof parsed.startedAt === "number" ? parsed.startedAt : null,
    updatedAt: typeof parsed.updatedAt === "number" ? parsed.updatedAt : null,
    completedAt: typeof parsed.completedAt === "number" ? parsed.completedAt : null,
    latestProgress: typeof parsed.latestProgress === "string" ? parsed.latestProgress : null,
    progressPhase: typeof parsed.progressPhase === "string" ? parsed.progressPhase : null,
    draftingText: typeof parsed.draftingText === "string" ? parsed.draftingText : null,
    lastToolName: typeof parsed.lastToolName === "string" ? parsed.lastToolName : null,
    toolSteps: Array.isArray(parsed.toolSteps) ? parsed.toolSteps : [],
    activity: Array.isArray(parsed.activity) ? parsed.activity : [],
    lastError: typeof parsed.lastError === "string" ? parsed.lastError : null,
    finalMessageId: typeof parsed.finalMessageId === "string" && parsed.finalMessageId.trim()
      ? parsed.finalMessageId
      : null,
    finalMessageTimestamp: typeof parsed.finalMessageTimestamp === "number"
      ? parsed.finalMessageTimestamp
      : null,
    finalMessageText: normalizeReadonlyRunAnchorText(parsed.finalMessageText),
  };
}

function readonlyRunHistoryIdentity(run: ChatReadonlyRunState): string {
  const finalMessageId = run.finalMessageId?.trim();
  if (finalMessageId) {
    return `msg:${finalMessageId}`;
  }
  if (typeof run.finalMessageTimestamp === "number") {
    return `ts:${run.finalMessageTimestamp}`;
  }
  if (run.finalMessageText) {
    return `text:${run.finalMessageText}`;
  }
  if (run.runId) {
    return `run:${run.runId}`;
  }
  return `fallback:${run.sessionKey}:${run.startedAt ?? "na"}:${run.completedAt ?? run.updatedAt ?? "na"}:${run.phase}`;
}

function sortReadonlyRuns(
  left: ChatReadonlyRunState,
  right: ChatReadonlyRunState,
): number {
  const leftTs = left.finalMessageTimestamp ?? left.completedAt ?? left.updatedAt ?? left.startedAt ?? 0;
  const rightTs = right.finalMessageTimestamp ?? right.completedAt ?? right.updatedAt ?? right.startedAt ?? 0;
  if (leftTs !== rightTs) {
    return leftTs - rightTs;
  }
  return readonlyRunHistoryIdentity(left).localeCompare(readonlyRunHistoryIdentity(right));
}

function mergeReadonlyRunHistory(
  history: Array<ChatReadonlyRunState | null | undefined>,
): ChatReadonlyRunState[] {
  const byId = new Map<string, ChatReadonlyRunState>();
  for (const item of history) {
    if (!isReadonlyRunTerminal(item)) {
      continue;
    }
    const key = readonlyRunHistoryIdentity(item);
    const previous = byId.get(key);
    if (!previous || sortReadonlyRuns(previous, item) <= 0) {
      byId.set(key, item);
    }
  }
  return [...byId.values()].sort(sortReadonlyRuns).slice(-READONLY_RUN_HISTORY_LIMIT);
}

function loadPersistedReadonlyRunState(sessionKey: string): ChatReadonlyRunStorageState {
  const targetSessionKey = sessionKey.trim();
  if (!targetSessionKey) {
    return { current: null, history: [] };
  }
  try {
    const raw = window.localStorage.getItem(readonlyRunStorageKey(targetSessionKey));
    if (!raw) {
      return { current: null, history: [] };
    }
    const parsed = JSON.parse(raw) as Record<string, unknown> | null;
    if (!parsed || typeof parsed !== "object") {
      return { current: null, history: [] };
    }

    if ("current" in parsed || "history" in parsed) {
      const current = normalizePersistedReadonlyRun(parsed.current, targetSessionKey);
      const history = mergeReadonlyRunHistory(
        Array.isArray(parsed.history)
          ? parsed.history.map((item) => normalizePersistedReadonlyRun(item, targetSessionKey))
          : [],
      );
      return { current, history };
    }

    const legacy = normalizePersistedReadonlyRun(parsed, targetSessionKey);
    return {
      current: legacy,
      history: isReadonlyRunTerminal(legacy) ? [legacy] : [],
    };
  } catch {
    return { current: null, history: [] };
  }
}

export function loadPersistedChatReadonlyRunHistory(sessionKey: string): ChatReadonlyRunState[] {
  return loadPersistedReadonlyRunState(sessionKey).history;
}

export function persistChatReadonlyRun(
  run: ChatReadonlyRunState | null | undefined,
  sessionKey?: string,
  history?: ChatReadonlyRunState[] | null,
) {
  const targetSessionKey =
    sessionKey?.trim() ||
    run?.sessionKey?.trim() ||
    history?.find((item) => item?.sessionKey?.trim())?.sessionKey?.trim() ||
    "";
  if (!targetSessionKey) {
    return;
  }
  const key = readonlyRunStorageKey(targetSessionKey);
  try {
    const current =
      shouldPersistReadonlyRun(run) && run?.sessionKey === targetSessionKey
        ? normalizePersistedReadonlyRun(run, targetSessionKey)
        : null;
    const nextHistory = mergeReadonlyRunHistory([
      ...(Array.isArray(history) ? history : []),
      current && isReadonlyRunTerminal(current) ? current : null,
    ]);
    if (!current && nextHistory.length === 0) {
      window.localStorage.removeItem(key);
      return;
    }
    window.localStorage.setItem(key, JSON.stringify({
      current,
      history: nextHistory,
    }));
  } catch {
    // Ignore persistence failures in private/test contexts.
  }
}

export function loadPersistedChatReadonlyRun(sessionKey: string): ChatReadonlyRunState | null {
  return loadPersistedReadonlyRunState(sessionKey).current;
}

export function isReadonlyRunVisible(
  run: ChatReadonlyRunState | null | undefined,
  sessionKey: string,
): boolean {
  if (!run) {
    return false;
  }
  if (run.sessionKey !== sessionKey) {
    return false;
  }
  if (run.phase === "idle") {
    return false;
  }
  return run.runId !== null || hasReadonlyRunAnchor(run);
}

export function isReadonlyRunActive(run: ChatReadonlyRunState | null | undefined): boolean {
  if (!run) {
    return false;
  }
  if (!run.runId) {
    return false;
  }
  return (
    run.phase === "starting" ||
    run.phase === "working" ||
    run.phase === "drafting" ||
    run.phase === "finalizing"
  );
}

function archiveReadonlyRun(host: ChatReadonlyRunHost, run: ChatReadonlyRunState) {
  if (!isReadonlyRunTerminal(run) || !hasReadonlyRunAnchor(run)) {
    return;
  }
  host.chatReadonlyRunHistory = mergeReadonlyRunHistory([
    ...(host.chatReadonlyRunHistory ?? []),
    run,
  ]);
}

export function resetChatReadonlyRun(
  host: ChatReadonlyRunHost,
  sessionKey: string = host.sessionKey,
) {
  host.chatReadonlyRun = createChatReadonlyRunState(sessionKey);
}

export function syncChatReadonlyRunSession(host: ChatReadonlyRunHost) {
  const current = host.chatReadonlyRun;
  if (current.runId !== null) {
    return;
  }
  if (current.phase === "aborted" || current.phase === "error") {
    return;
  }
  if (current.phase === "complete") {
    return;
  }
  if (current.sessionKey === host.sessionKey) {
    return;
  }
  host.chatReadonlyRun = createChatReadonlyRunState(host.sessionKey);
}

export function startChatReadonlyRun(
  host: ChatReadonlyRunHost,
  runId: string,
  startedAt: number,
  sessionKey: string = host.sessionKey,
) {
  host.chatReadonlyRun = {
    runId,
    sessionKey,
    phase: "starting",
    startedAt,
    updatedAt: startedAt,
    completedAt: null,
    latestProgress: null,
    progressPhase: null,
    draftingText: null,
    lastToolName: null,
    toolSteps: [],
    activity: [createPhaseActivity("starting", startedAt)],
    lastError: null,
    finalMessageId: null,
    finalMessageTimestamp: null,
    finalMessageText: null,
  };
}

function hasReadonlyRunAnchor(run: ChatReadonlyRunState | null | undefined): boolean {
  if (!run) {
    return false;
  }
  if (run.finalMessageId?.trim()) {
    return true;
  }
  if (typeof run.finalMessageTimestamp === "number") {
    return true;
  }
  return normalizeReadonlyRunAnchorText(run.finalMessageText) !== null;
}

export function rebindChatReadonlyRun(
  host: ChatReadonlyRunHost,
  nextRunId: string,
  opts?: { previousRunId?: string | null; sessionKey?: string | null; ts?: number | null },
): boolean {
  const current = host.chatReadonlyRun;
  const targetRunId = nextRunId.trim();
  if (!targetRunId || isReadonlyRunTerminal(current)) {
    return false;
  }
  const previousRunId = opts?.previousRunId?.trim() || current.runId;
  if (!previousRunId || current.runId !== previousRunId || previousRunId === targetRunId) {
    return false;
  }
  host.chatReadonlyRun = {
    ...current,
    runId: targetRunId,
    sessionKey: opts?.sessionKey?.trim() || current.sessionKey,
    updatedAt: opts?.ts ?? current.updatedAt ?? Date.now(),
  };
  return true;
}

export function setChatReadonlyRunTerminal(
  host: ChatReadonlyRunHost,
  phase: "aborted" | "error",
  opts?: { runId?: string | null; sessionKey?: string | null; ts?: number; errorMessage?: string | null },
) {
  const ts = opts?.ts ?? Date.now();
  const nextSessionKey = opts?.sessionKey?.trim() || host.chatReadonlyRun.sessionKey || host.sessionKey;
  const currentRunId = host.chatReadonlyRun.runId;
  const targetRunId = opts?.runId?.trim();
  if (targetRunId && currentRunId && targetRunId !== currentRunId) {
    return;
  }
  const message =
    phase === "error"
      ? opts?.errorMessage?.trim() || host.chatReadonlyRun.lastError || null
      : null;
  let activity = upsertActivity(host.chatReadonlyRun.activity, createPhaseActivity(phase, ts));
  if (message) {
    activity = upsertActivity(activity, createErrorActivity(message, ts));
  }
  host.chatReadonlyRun = {
    ...host.chatReadonlyRun,
    runId: null,
    sessionKey: nextSessionKey,
    phase,
    updatedAt: ts,
    completedAt: ts,
    activity,
    lastError: message,
  };
  archiveReadonlyRun(host, host.chatReadonlyRun);
}

export function updateChatReadonlyRunFromLifecycle(
  host: ChatReadonlyRunHost,
  input: RunBindingInput & { phase: string },
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, input);
  if (!current) {
    return;
  }

  if (input.phase === "start") {
    host.chatReadonlyRun = {
      ...current,
      phase: current.draftingText?.trim() ? "drafting" : "starting",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("starting", input.ts)),
    };
    return;
  }

  if (input.phase === "end") {
    host.chatReadonlyRun = {
      ...current,
      phase: "finalizing",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("finalizing", input.ts)),
    };
    return;
  }

  if (input.phase === "error") {
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "error",
      updatedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("error", input.ts)),
    };
    archiveReadonlyRun(host, host.chatReadonlyRun);
  }
}

export function updateChatReadonlyRunFromProgress(
  host: ChatReadonlyRunHost,
  input: ProgressInput,
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, input);
  if (!current) {
    return;
  }

  const summary = input.summary.trim();
  host.chatReadonlyRun = {
    ...current,
    phase: resolveActivePhase(current, "working"),
    updatedAt: input.ts,
    latestProgress: summary,
    progressPhase: input.phase?.trim() || null,
    activity: appendProgressActivity(current.activity, summary, input.phase?.trim() || null, input.ts),
  };
}

export function updateChatReadonlyRunFromTool(
  host: ChatReadonlyRunHost,
  input: ToolInput,
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, input);
  if (!current) {
    return;
  }

  const toolSteps = upsertToolStep(current.toolSteps, input);
  const step = toolSteps.find((item) => item.toolCallId === input.toolCallId) ?? null;
  host.chatReadonlyRun = {
    ...current,
    phase: resolveActivePhase(current, "working"),
    updatedAt: input.ts,
    lastToolName: input.name,
    toolSteps,
    activity: step
      ? upsertToolActivity(current.activity, step, input.phase, input.ts)
      : current.activity,
  };
}

export function updateChatReadonlyRunFromChat(
  host: ChatReadonlyRunHost,
  input: ChatInput,
) {
  const current = bindReadonlyRun(host.chatReadonlyRun, {
    runId: input.runId,
    sessionKey: input.sessionKey,
    ts: input.ts,
  });
  if (!current) {
    return;
  }

  if (input.state === "delta") {
    const nextText = input.text?.trim() ? input.text : current.draftingText;
    const activity = nextText?.trim()
      ? upsertDraftActivity(current.activity, nextText, input.ts)
      : current.activity;
    host.chatReadonlyRun = {
      ...current,
      phase: nextText?.trim() ? "drafting" : current.phase,
      updatedAt: input.ts,
      draftingText: nextText ?? null,
      activity,
    };
    return;
  }

  if (input.state === "final") {
    const nextText = input.text?.trim() ? input.text : current.draftingText;
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      sessionKey: input.sessionKey,
      phase: "complete",
      updatedAt: input.ts,
      completedAt: input.ts,
      draftingText: nextText ?? current.draftingText,
      activity: upsertActivity(current.activity, createPhaseActivity("complete", input.ts)),
      finalMessageId: input.messageId?.trim() || null,
      finalMessageTimestamp: input.messageTimestamp ?? input.ts,
      finalMessageText: normalizeReadonlyRunAnchorText(nextText ?? current.draftingText),
    };
    archiveReadonlyRun(host, host.chatReadonlyRun);
    return;
  }

  if (input.state === "aborted") {
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "aborted",
      updatedAt: input.ts,
      completedAt: input.ts,
      activity: upsertActivity(current.activity, createPhaseActivity("aborted", input.ts)),
    };
    archiveReadonlyRun(host, host.chatReadonlyRun);
    return;
  }

  if (input.state === "error") {
    const message = input.errorMessage?.trim() || current.lastError;
    let activity = upsertActivity(current.activity, createPhaseActivity("error", input.ts));
    if (message) {
      activity = upsertActivity(activity, createErrorActivity(message, input.ts));
    }
    host.chatReadonlyRun = {
      ...current,
      runId: null,
      phase: "error",
      updatedAt: input.ts,
      completedAt: input.ts,
      lastError: message,
      activity,
    };
    archiveReadonlyRun(host, host.chatReadonlyRun);
  }
}

function bindReadonlyRun(
  current: ChatReadonlyRunState,
  input: RunBindingInput,
): ChatReadonlyRunState | null {
  if (!input.runId) {
    return null;
  }
  if (current.runId && current.runId !== input.runId) {
    return null;
  }

  const nextSessionKey = input.sessionKey?.trim() || current.sessionKey;
  if (!current.runId) {
    return {
      ...createChatReadonlyRunState(nextSessionKey),
      runId: input.runId,
      sessionKey: nextSessionKey,
      phase: "starting",
      startedAt: input.ts,
      updatedAt: input.ts,
      completedAt: null,
      activity: [createPhaseActivity("starting", input.ts)],
    };
  }

  return {
    ...current,
    sessionKey: nextSessionKey,
    startedAt: current.startedAt ?? input.ts,
    activity: current.activity ?? [],
  };
}

function resolveActivePhase(
  current: ChatReadonlyRunState,
  fallback: "working" | "drafting",
): "working" | "drafting" | "finalizing" {
  if (current.phase === "finalizing") {
    return "finalizing";
  }
  if (current.draftingText?.trim()) {
    return "drafting";
  }
  return fallback;
}

function upsertToolStep(
  steps: ReadonlyToolStep[],
  input: ToolInput,
): ReadonlyToolStep[] {
  const startedAt = input.ts;
  const outputPreview = truncatePreview(input.output);
  const nextStatus = resolveToolStepStatus(input.phase, Boolean(input.isError));
  const nextSteps = [...steps];
  const existingIndex = nextSteps.findIndex((item) => item.toolCallId === input.toolCallId);

  if (existingIndex === -1) {
    nextSteps.push({
      toolCallId: input.toolCallId,
      name: input.name,
      detail: input.detail?.trim() || null,
      status: nextStatus,
      startedAt,
      updatedAt: input.ts,
      outputPreview,
    });
    return trimToolSteps(nextSteps);
  }

  const existing = nextSteps[existingIndex];
  nextSteps[existingIndex] = {
      ...existing,
      name: input.name,
      detail: input.detail?.trim() || existing.detail,
      status: nextStatus,
      updatedAt: input.ts,
      outputPreview: outputPreview ?? existing.outputPreview,
  };
  return trimToolSteps(nextSteps);
}

function trimToolSteps(steps: ReadonlyToolStep[]): ReadonlyToolStep[] {
  if (steps.length <= READONLY_TOOL_STEP_LIMIT) {
    return steps;
  }
  return steps.slice(steps.length - READONLY_TOOL_STEP_LIMIT);
}

function resolveToolStepStatus(
  phase: string,
  isError: boolean,
): ReadonlyToolStepStatus {
  if (phase === "result") {
    return isError ? "error" : "done";
  }
  return "running";
}

function createPhaseActivity(
  phase: Exclude<ReadonlyRunPhase, "idle">,
  ts: number,
): ReadonlyRunActivity {
  return {
    id: `phase:${phase}`,
    kind: "phase",
    phase,
    ts,
    updatedAt: ts,
  };
}

function createErrorActivity(message: string, ts: number): ReadonlyRunActivity {
  return {
    id: "error:terminal",
    kind: "error",
    message,
    ts,
    updatedAt: ts,
  };
}

function appendProgressActivity(
  activity: ReadonlyRunActivity[],
  summary: string,
  progressPhase: string | null,
  ts: number,
): ReadonlyRunActivity[] {
  if (!summary) {
    return activity;
  }
  const next = [...activity];
  const last = next[next.length - 1];
  if (last?.kind === "progress" && last.summary === summary) {
    next[next.length - 1] = {
      ...last,
      progressPhase,
      updatedAt: ts,
    };
    return trimActivity(next);
  }
  next.push({
    id: `progress:${ts}`,
    kind: "progress",
    summary,
    progressPhase,
    ts,
    updatedAt: ts,
  });
  return trimActivity(next);
}

function upsertToolActivity(
  activity: ReadonlyRunActivity[],
  step: ReadonlyToolStep,
  phase: string,
  ts: number,
): ReadonlyRunActivity[] {
  const id = `tool:${step.toolCallId}`;
  const next: ReadonlyRunActivity = {
    id,
    kind: "tool",
    toolCallId: step.toolCallId,
    name: step.name,
    detail: step.detail,
    status: step.status,
    phase,
    outputPreview: step.outputPreview,
    ts: step.startedAt,
    updatedAt: ts,
  };
  return upsertActivity(activity, next);
}

function upsertDraftActivity(
  activity: ReadonlyRunActivity[],
  text: string,
  ts: number,
): ReadonlyRunActivity[] {
  const existing = activity.find((item) => item.kind === "draft");
  return upsertActivity(activity, {
    id: "draft:current",
    kind: "draft",
    text,
    ts: existing?.kind === "draft" ? existing.ts : ts,
    updatedAt: ts,
  });
}

function upsertActivity(
  activity: ReadonlyRunActivity[],
  next: ReadonlyRunActivity,
): ReadonlyRunActivity[] {
  const items = [...activity];
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) {
    items.push(next);
    return trimActivity(items);
  }
  items[index] = next;
  return trimActivity(items);
}

function trimActivity(activity: ReadonlyRunActivity[]): ReadonlyRunActivity[] {
  if (activity.length <= READONLY_ACTIVITY_LIMIT) {
    return activity;
  }
  return activity.slice(activity.length - READONLY_ACTIVITY_LIMIT);
}

function truncatePreview(text: string | null | undefined): string | null {
  const value = text?.trim();
  if (!value) {
    return null;
  }
  if (value.length <= READONLY_OUTPUT_PREVIEW_LIMIT) {
    return value;
  }
  return `${value.slice(0, READONLY_OUTPUT_PREVIEW_LIMIT - 1)}…`;
}
