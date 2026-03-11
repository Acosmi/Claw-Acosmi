import type { GatewayBrowserClient } from "../gateway.ts";
import type { ChatReadonlyRunState } from "../chat/readonly-run-state.ts";
import type { ChatAttachment } from "../ui-types.ts";
import { extractText } from "../chat/message-extract.ts";
import {
  createChatReadonlyRunState,
  isReadonlyRunTerminal,
  loadPersistedChatReadonlyRun,
  loadPersistedChatReadonlyRunHistory,
  rebindChatReadonlyRun,
  normalizeReadonlyRunAnchorText,
  setChatReadonlyRunTerminal,
  startChatReadonlyRun,
  updateChatReadonlyRunFromChat,
} from "../chat/readonly-run-state.ts";
import { globalChatEventStore } from "../event-store.ts";
import { generateUUID } from "../uuid.ts";

export type ChatState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  sessionKey: string;
  chatLoading: boolean;
  chatMessages: unknown[];
  chatThinkingLevel: string | null;
  chatSending: boolean;
  chatMessage: string;
  chatAttachments: ChatAttachment[];
  chatRunId: string | null;
  chatStream: string | null;
  chatStreamStartedAt: number | null;
  chatReadonlyRun?: ChatReadonlyRunState;
  chatReadonlyRunHistory?: ChatReadonlyRunState[];
  lastError: string | null;
};

export type ChatEventPayload = {
  runId: string;
  sessionKey: string;
  state: "delta" | "final" | "aborted" | "error";
  message?: unknown;
  errorMessage?: string;
};

function extractMessageId(message: unknown): string | null {
  if (!message || typeof message !== "object") {
    return null;
  }
  const record = message as Record<string, unknown>;
  if (typeof record.id === "string" && record.id.trim()) {
    return record.id;
  }
  if (typeof record.messageId === "string" && record.messageId.trim()) {
    return record.messageId;
  }
  return null;
}

function extractMessageTimestamp(message: unknown): number | null {
  if (!message || typeof message !== "object") {
    return null;
  }
  const value = (message as Record<string, unknown>).timestamp;
  return typeof value === "number" ? value : null;
}

function shouldAppendChatMessage(existing: unknown[], incoming: unknown): boolean {
  if (!incoming || typeof incoming !== "object") {
    return false;
  }
  const previous = existing[existing.length - 1];
  if (!previous || typeof previous !== "object") {
    return true;
  }
  const prevRole = typeof (previous as Record<string, unknown>).role === "string"
    ? (previous as Record<string, unknown>).role
    : "";
  const nextRole = typeof (incoming as Record<string, unknown>).role === "string"
    ? (incoming as Record<string, unknown>).role
    : "";
  if (!prevRole || prevRole !== nextRole) {
    return true;
  }
  const prevTimestamp = extractMessageTimestamp(previous);
  const nextTimestamp = extractMessageTimestamp(incoming);
  if (prevTimestamp !== null && nextTimestamp !== null && prevTimestamp !== nextTimestamp) {
    return true;
  }
  const prevText = normalizeReadonlyRunAnchorText(extractText(previous));
  const nextText = normalizeReadonlyRunAnchorText(extractText(incoming));
  if (prevText !== null || nextText !== null) {
    return prevText !== nextText;
  }
  const prevContent = JSON.stringify((previous as Record<string, unknown>).content ?? null);
  const nextContent = JSON.stringify((incoming as Record<string, unknown>).content ?? null);
  return prevContent !== nextContent;
}

function areChatMessagesEquivalentIgnoringTimestamp(left: unknown, right: unknown): boolean {
  if (!left || typeof left !== "object" || !right || typeof right !== "object") {
    return false;
  }
  const leftRole = typeof (left as Record<string, unknown>).role === "string"
    ? (left as Record<string, unknown>).role
    : "";
  const rightRole = typeof (right as Record<string, unknown>).role === "string"
    ? (right as Record<string, unknown>).role
    : "";
  if (!leftRole || leftRole !== rightRole) {
    return false;
  }
  const leftText = normalizeReadonlyRunAnchorText(extractText(left));
  const rightText = normalizeReadonlyRunAnchorText(extractText(right));
  if (leftText !== null || rightText !== null) {
    return leftText === rightText;
  }
  const leftContent = JSON.stringify((left as Record<string, unknown>).content ?? null);
  const rightContent = JSON.stringify((right as Record<string, unknown>).content ?? null);
  return leftContent === rightContent;
}

function findLatestPendingUserMessage(messages: unknown[]): unknown | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i] as Record<string, unknown> | undefined;
    if (message?.role === "user") {
      return message;
    }
  }
  return null;
}

function shouldRestorePersistedWorkflow(run: ChatReadonlyRunState, messages: unknown[]): boolean {
  if (run.phase !== "complete") {
    return true;
  }
  const finalId = run.finalMessageId?.trim() || "";
  const finalTs = run.finalMessageTimestamp ?? null;
  const finalText = normalizeReadonlyRunAnchorText(run.finalMessageText);
  if (!finalId && finalTs === null && !finalText) {
    return false;
  }
  return messages.some((message) => {
    const record = message as Record<string, unknown>;
    const role = typeof record.role === "string" ? record.role : "";
    const messageId = typeof record.id === "string"
      ? record.id
      : typeof record.messageId === "string"
        ? record.messageId
        : "";
    const timestamp = typeof record.timestamp === "number" ? record.timestamp : null;
    const text = role === "assistant" ? normalizeReadonlyRunAnchorText(extractText(message)) : null;
    return (
      (finalId && messageId === finalId) ||
      (finalTs !== null && timestamp === finalTs) ||
      (finalText !== null && text === finalText)
    );
  });
}

export async function loadChatHistory(state: ChatState) {
  if (!state.client || !state.connected) {
    return;
  }
  state.chatLoading = true;
  state.lastError = null;
  try {
    const res = await state.client.request<{ messages?: Array<unknown>; thinkingLevel?: string }>(
      "chat.history",
      {
        sessionKey: state.sessionKey,
        limit: 200,
      },
    );
    const messages = Array.isArray(res.messages) ? res.messages : [];
    if ((state as any)._skipEmptyHistory) {
      (state as any)._skipEmptyHistory = false;
      if (messages.length === 0) {
        // transcript 尚未写入，保留预填充的用户消息不清空
      } else {
        // 有历史记录时，优先保留本地最后一条 optimistic user 消息；
        // 仅当 transcript 尚未包含同等内容时才追加，避免本地发送时被历史刷新吞掉或重复。
        const pendingMsg = findLatestPendingUserMessage(state.chatMessages);
        if (pendingMsg && !messages.some((message) => areChatMessagesEquivalentIgnoringTimestamp(message, pendingMsg))) {
          state.chatMessages = [...messages, pendingMsg];
        } else {
          state.chatMessages = messages;
        }
      }
    } else {
      state.chatMessages = messages;
    }
    state.chatThinkingLevel = res.thinkingLevel ?? null;
    const persistedRun = loadPersistedChatReadonlyRun(state.sessionKey);
    const persistedHistory = loadPersistedChatReadonlyRunHistory(state.sessionKey)
      .filter((run) => shouldRestorePersistedWorkflow(run, messages));
    if ("chatReadonlyRunHistory" in state && Array.isArray(state.chatReadonlyRunHistory)) {
      state.chatReadonlyRunHistory = persistedHistory;
    }
    if ("chatReadonlyRun" in state && state.chatReadonlyRun && !state.chatRunId) {
      if (persistedRun && shouldRestorePersistedWorkflow(persistedRun, messages)) {
        state.chatReadonlyRun = persistedRun;
      } else if (isReadonlyRunTerminal(state.chatReadonlyRun)) {
        state.chatReadonlyRun = createChatReadonlyRunState(state.sessionKey);
      }
    }
  } catch (err) {
    state.lastError = String(err);
  } finally {
    state.chatLoading = false;
  }
}

function dataUrlToBase64(dataUrl: string): { content: string; mimeType: string } | null {
  const match = /^data:([^;]+);base64,(.+)$/.exec(dataUrl);
  if (!match) {
    return null;
  }
  return { mimeType: match[1], content: match[2] };
}

export async function sendChatMessage(
  state: ChatState,
  message: string,
  attachments?: ChatAttachment[],
): Promise<string | null> {
  if (!state.client || !state.connected) {
    return null;
  }
  const msg = message.trim();
  const hasAttachments = attachments && attachments.length > 0;
  if (!msg && !hasAttachments) {
    return null;
  }

  const now = Date.now();

  // Build user message content blocks
  const contentBlocks: Array<{ type: string; text?: string; source?: unknown }> = [];
  if (msg) {
    contentBlocks.push({ type: "text", text: msg });
  }
  // Add image previews to the message for display
  if (hasAttachments) {
    for (const att of attachments) {
      contentBlocks.push({
        type: att.category ?? "image",
        source: { type: "base64", media_type: att.mimeType, data: att.dataUrl },
        ...(att.fileName ? { fileName: att.fileName } : {}),
        ...(att.fileSize ? { fileSize: att.fileSize } : {}),
      });
    }
  }

  state.chatMessages = [
    ...state.chatMessages,
    {
      role: "user",
      content: contentBlocks,
      timestamp: now,
    },
  ];
  (state as any)._skipEmptyHistory = true;

  state.chatSending = true;
  state.lastError = null;
  const runId = generateUUID();
  state.chatRunId = runId;
  state.chatStream = "";
  state.chatStreamStartedAt = now;
  if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
    startChatReadonlyRun(
      state as unknown as Parameters<typeof startChatReadonlyRun>[0],
      runId,
      now,
      state.sessionKey,
    );
  }

  // Convert attachments to API format
  const apiAttachments = hasAttachments
    ? attachments
      .map((att) => {
        const parsed = dataUrlToBase64(att.dataUrl);
        if (!parsed) {
          return null;
        }
        return {
          type: att.category ?? "image",
          mimeType: parsed.mimeType,
          content: parsed.content,
          ...(att.fileName ? { fileName: att.fileName } : {}),
          ...(att.fileSize ? { fileSize: att.fileSize } : {}),
        };
      })
      .filter((a): a is NonNullable<typeof a> => a !== null)
    : undefined;

  try {
    await state.client.request("chat.send", {
      sessionKey: state.sessionKey,
      message: msg,
      deliver: false,
      idempotencyKey: runId,
      attachments: apiAttachments,
    });
    return runId;
  } catch (err) {
    const error = String(err);
    (state as any)._skipEmptyHistory = false;
    state.chatRunId = null;
    state.chatStream = null;
    state.chatStreamStartedAt = null;
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      setChatReadonlyRunTerminal(
        state as unknown as Parameters<typeof setChatReadonlyRunTerminal>[0],
        "error",
        { runId, sessionKey: state.sessionKey, ts: Date.now(), errorMessage: error },
      );
    }
    state.lastError = error;
    state.chatMessages = [
      ...state.chatMessages,
      {
        role: "assistant",
        content: [{ type: "text", text: "Error: " + error }],
        timestamp: Date.now(),
      },
    ];
    return null;
  } finally {
    state.chatSending = false;
  }
}

export async function abortChatRun(state: ChatState): Promise<boolean> {
  if (!state.client || !state.connected) {
    return false;
  }
  const runId = state.chatRunId;
  try {
    await state.client.request(
      "chat.abort",
      runId ? { sessionKey: state.sessionKey, runId } : { sessionKey: state.sessionKey },
    );
    return true;
  } catch (err) {
    state.lastError = String(err);
    return false;
  }
}

export function handleChatEvent(state: ChatState, payload?: ChatEventPayload) {
  if (!payload) {
    return null;
  }

  // P4 事件收敛: 终态事件先归档到全局 store，确保跨 session 事件不丢失
  globalChatEventStore.record(payload);

  if (payload.sessionKey !== state.sessionKey) {
    return null;
  }

  const canRebindRemotePlaceholder =
    Boolean(payload.runId) &&
    typeof state.chatRunId === "string" &&
    state.chatRunId.startsWith("remote-") &&
    payload.runId !== state.chatRunId;

  // Final from another run (e.g. sub-agent announce): refresh history to show new message.
  // See https://github.com/openacosmi/openacosmi/issues/1909
  if (payload.runId && state.chatRunId && payload.runId !== state.chatRunId) {
    if (canRebindRemotePlaceholder) {
      if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
        rebindChatReadonlyRun(
          state as unknown as Parameters<typeof rebindChatReadonlyRun>[0],
          payload.runId,
          {
            previousRunId: state.chatRunId,
            sessionKey: payload.sessionKey,
            ts: Date.now(),
          },
        );
      }
      state.chatRunId = payload.runId;
      state.chatStreamStartedAt = state.chatStreamStartedAt ?? Date.now();
    } else {
      if (payload.state === "final") {
        // Bug #9 fix: immediately append the assistant message to avoid gap
        // between clearing stream and loadChatHistory completing.
        if (payload.message && shouldAppendChatMessage(state.chatMessages, payload.message)) {
          state.chatMessages = [...state.chatMessages, payload.message];
        }
        return "final";
      }
      return null;
    }
  }

  if (payload.state === "delta") {
    // Re-bind to the active run ID if we missed it (e.g. cross channel switch)
    if (payload.runId && !state.chatRunId) {
      state.chatRunId = payload.runId;
      state.chatStreamStartedAt = state.chatStreamStartedAt ?? Date.now();
    }
    const next = payload.message ? extractText(payload.message) : null;
    if (typeof next === "string") {
      const current = state.chatStream ?? "";
      if (!current || next.length >= current.length) {
        state.chatStream = next;
      }
      if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
        updateChatReadonlyRunFromChat(
          state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
          {
            runId: payload.runId,
            sessionKey: payload.sessionKey,
            state: "delta",
            ts: Date.now(),
            text: next,
          },
        );
      }
    }
  } else if (payload.state === "final") {
    // Bug #9 fix: immediately append the final assistant message so it's
    // visible without waiting for loadChatHistory() to complete.
    // The subsequent loadChatHistory() call (in app-gateway.ts) will replace
    // chatMessages with the canonical backend transcript, which already
    // includes this message (transcript is persisted before broadcast).
    if (payload.message && shouldAppendChatMessage(state.chatMessages, payload.message)) {
      state.chatMessages = [...state.chatMessages, payload.message];
    }
    state.chatStream = null;
    state.chatRunId = null;
    state.chatStreamStartedAt = null;
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      updateChatReadonlyRunFromChat(
        state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
        {
          runId: payload.runId,
          sessionKey: payload.sessionKey,
          state: "final",
          ts: Date.now(),
          text: payload.message ? extractText(payload.message) : null,
          messageId: extractMessageId(payload.message),
          messageTimestamp: extractMessageTimestamp(payload.message),
        },
      );
    }
  } else if (payload.state === "aborted") {
    state.chatStream = null;
    state.chatRunId = null;
    state.chatStreamStartedAt = null;
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      updateChatReadonlyRunFromChat(
        state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
        {
          runId: payload.runId,
          sessionKey: payload.sessionKey,
          state: "aborted",
          ts: Date.now(),
        },
      );
    }
  } else if (payload.state === "error") {
    state.chatStream = null;
    state.chatRunId = null;
    state.chatStreamStartedAt = null;
    state.lastError = payload.errorMessage ?? "chat error";
    if ("chatReadonlyRun" in state && state.chatReadonlyRun) {
      updateChatReadonlyRunFromChat(
        state as unknown as Parameters<typeof updateChatReadonlyRunFromChat>[0],
        {
          runId: payload.runId,
          sessionKey: payload.sessionKey,
          state: "error",
          ts: Date.now(),
          errorMessage: payload.errorMessage ?? "chat error",
        },
      );
    }
  }
  return payload.state;
}

// ── Chat model selector ──

export type ChatModelState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  chatModels: Array<{ id: string; name: string; provider: string; source: string }>;
  chatCurrentModel: string | null;
  // Also sync debugModels for agents tab
  debugModels: unknown[];
};

type ChatModelSource = "builtin" | "custom";
type ChatModelEntry = { id: string; name: string; provider: string; source: ChatModelSource };

function modelIdentityKey(model: Pick<ChatModelEntry, "id" | "provider">): string {
  return model.provider ? `${model.provider}/${model.id}` : model.id;
}

function uniqueChatModelsByIdentity(
  models: ChatModelEntry[],
  blockedKeys: Set<string> = new Set<string>(),
): ChatModelEntry[] {
  const seen = new Set<string>(blockedKeys);
  const result: ChatModelEntry[] = [];
  for (const model of models) {
    const key = modelIdentityKey(model);
    if (!key || !model.id || seen.has(key)) {
      continue;
    }
    seen.add(key);
    result.push(model);
  }
  return result;
}

function normalizeChatModels(rawModels: Array<Record<string, unknown>>): ChatModelEntry[] {
  return rawModels
    .map((m) => {
      const id = String(m.id ?? "").trim();
      const provider = String(m.provider ?? "").trim();
      const rawSource = String(m.source ?? "").trim().toLowerCase();
      const source: ChatModelSource | null = rawSource === "managed"
        ? "builtin"
        : rawSource === "custom"
          ? "custom"
          : null;
      return {
        id,
        name: String(m.name ?? m.id ?? "").trim(),
        provider,
        source,
      };
    })
    .filter((m): m is ChatModelEntry => m.id !== "" && m.source !== null);
}

function extractModelsFromConfigSnapshot(snapshot: unknown): ChatModelEntry[] {
  const config = (snapshot as { config?: { models?: { providers?: Record<string, { models?: Array<Record<string, unknown>> | null } | null> } } } | undefined)?.config;
  const providers = config?.models?.providers;
  if (!providers || typeof providers !== "object") {
    return [];
  }
  const result: ChatModelEntry[] = [];
  for (const [providerName, providerConfig] of Object.entries(providers)) {
    const models = providerConfig?.models;
    if (!Array.isArray(models)) {
      continue;
    }
    for (const model of models) {
      const id = String(model.id ?? "").trim();
      if (!id) continue;
      result.push({
        id,
        name: String(model.name ?? model.id ?? "").trim(),
        provider: providerName,
        source: "custom",
      });
    }
  }
  return result;
}

function extractCurrentModelFromConfigSnapshot(snapshot: unknown): string | null {
  const primary = (snapshot as {
    config?: {
      agents?: {
        defaults?: {
          model?: {
            primary?: unknown;
          } | null;
        } | null;
      } | null;
    };
  } | undefined)?.config?.agents?.defaults?.model?.primary;
  if (typeof primary !== "string") {
    return null;
  }
  const trimmed = primary.trim();
  return trimmed || null;
}

function ensureCurrentModelOption(
  models: ChatModelEntry[],
  currentModel: string | null,
  builtinKeys: Set<string>,
): ChatModelEntry[] {
  if (!currentModel) {
    return models;
  }
  const trimmed = currentModel.trim();
  if (!trimmed) {
    return models;
  }
  const slash = trimmed.indexOf("/");
  const provider = slash >= 0 ? trimmed.slice(0, slash) : "";
  const id = slash >= 0 ? trimmed.slice(slash + 1) : trimmed;
  if (!id) {
    return models;
  }
  const exists = models.some((model) => {
    const key = model.provider ? `${model.provider}/${model.id}` : model.id;
    return key === trimmed || model.id === id;
  });
  if (exists) {
    return models;
  }
  return [
    ...models,
    {
      id,
      name: id,
      provider,
      source: builtinKeys.has(trimmed) ? "builtin" : "custom",
    },
  ];
}

export async function loadChatModels(state: ChatModelState) {
  if (!state.client || !state.connected) return;
  const [listRes, defaultRes, configRes] = await Promise.allSettled([
    state.client.request<{ models?: Array<Record<string, unknown>> }>("models.list", {}),
    state.client.request<{ model?: string }>("models.default.get", {}),
    state.client.request("config.get", {}),
  ]);

  const rawModels =
    listRes.status === "fulfilled" && Array.isArray(listRes.value?.models)
      ? listRes.value.models
      : [];
  const configModels =
    configRes.status === "fulfilled" ? extractModelsFromConfigSnapshot(configRes.value) : [];
  const configCurrentModel =
    configRes.status === "fulfilled" ? extractCurrentModelFromConfigSnapshot(configRes.value) : null;
  const currentModel =
    defaultRes.status === "fulfilled" && typeof defaultRes.value?.model === "string"
      ? (defaultRes.value.model.trim() || configCurrentModel || state.chatCurrentModel)
      : configCurrentModel || state.chatCurrentModel;

  // Configured models are always treated as custom in chat UX, even if the
  // runtime catalog also exposes the same provider/model pair as a builtin placeholder.
  const normalized = normalizeChatModels(rawModels);
  const customModels = uniqueChatModelsByIdentity(
    [
      ...configModels,
      ...normalized.filter((model) => model.source === "custom"),
    ],
  );
  const customKeys = new Set<string>(customModels.map((model) => modelIdentityKey(model)));
  const builtinModels = uniqueChatModelsByIdentity(
    normalized.filter((model) => model.source === "builtin"),
    customKeys,
  );
  const builtinKeys = new Set<string>(builtinModels.map((model) => modelIdentityKey(model)));
  const nextModels = ensureCurrentModelOption(
    [
      ...customModels,
      ...builtinModels,
    ],
    currentModel ?? null,
    builtinKeys,
  );

  state.chatModels = nextModels;
  state.chatCurrentModel = currentModel ?? null;
  if (state.debugModels.length === 0 && rawModels.length > 0) {
    state.debugModels = rawModels;
  }
}

export async function setChatModel(state: ChatModelState, model: string) {
  if (!state.client || !state.connected) return;
  state.chatCurrentModel = model; // optimistic update
  try {
    await state.client.request("models.default.set", { model });
  } catch {
    // revert on failure — reload from server
    void loadChatModels(state);
  }
}
