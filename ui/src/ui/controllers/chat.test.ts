import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  handleChatEvent,
  loadChatHistory,
  loadChatModels,
  type ChatEventPayload,
  type ChatModelState,
  type ChatState,
} from "./chat.ts";
import {
  createChatReadonlyRunState,
  persistChatReadonlyRun,
} from "../chat/readonly-run-state.ts";
import { globalChatEventStore } from "../event-store.ts";

function createState(overrides: Partial<ChatState> = {}): ChatState {
  return {
    chatAttachments: [],
    chatLoading: false,
    chatMessage: "",
    chatMessages: [],
    chatReadonlyRun: createChatReadonlyRunState("main"),
    chatReadonlyRunHistory: [],
    chatRunId: null,
    chatSending: false,
    chatStream: null,
    chatStreamStartedAt: null,
    chatThinkingLevel: null,
    client: null,
    connected: true,
    lastError: null,
    sessionKey: "main",
    ...overrides,
  };
}

beforeEach(() => {
  window.localStorage.clear();
});

describe("loadChatHistory", () => {
  it("restores a persisted in-progress workflow after reload when the session matches", async () => {
    const persistedRun = {
      ...createChatReadonlyRunState("main"),
      runId: "run-active",
      phase: "working" as const,
      startedAt: 100,
      updatedAt: 140,
      latestProgress: "正在读取聊天页工作流状态。",
      toolSteps: [
        {
          toolCallId: "tool-1",
          name: "read_files",
          detail: null,
          status: "running" as const,
          startedAt: 120,
          updatedAt: 140,
          outputPreview: "正在读取 chat.ts",
        },
      ],
      activity: [],
    };
    persistChatReadonlyRun(persistedRun, "main");

    const state = createState({
      client: {
        request: vi.fn(async (method: string) => {
          if (method !== "chat.history") {
            throw new Error(`unexpected method ${method}`);
          }
          return {
            messages: [],
            thinkingLevel: null,
          };
        }),
      } as never,
    });

    await loadChatHistory(state);

    expect(state.chatReadonlyRun?.phase).toBe("working");
    expect(state.chatReadonlyRun?.runId).toBe("run-active");
  });

  it("restores a persisted completed workflow when the anchored final reply is still in history", async () => {
    const persistedRun = {
      ...createChatReadonlyRunState("main"),
      phase: "complete" as const,
      startedAt: 100,
      updatedAt: 200,
      completedAt: 220,
      latestProgress: "Done",
      toolSteps: [],
      activity: [],
      finalMessageId: "msg-final",
      finalMessageTimestamp: 222,
      finalMessageText: "final",
    };
    persistChatReadonlyRun(persistedRun, "main");

    const state = createState({
      client: {
        request: vi.fn(async (method: string) => {
          if (method !== "chat.history") {
            throw new Error(`unexpected method ${method}`);
          }
          return {
            messages: [
              {
                id: "msg-final",
                role: "assistant",
                content: [{ type: "text", text: "final" }],
                timestamp: 222,
              },
            ],
            thinkingLevel: null,
          };
        }),
      } as never,
    });

    await loadChatHistory(state);

    expect(state.chatReadonlyRun?.phase).toBe("complete");
    expect(state.chatReadonlyRun?.finalMessageId).toBe("msg-final");
  });

  it("restores a persisted completed workflow by final reply text when history has no stable id or timestamp", async () => {
    const persistedRun = {
      ...createChatReadonlyRunState("main"),
      phase: "complete" as const,
      startedAt: 100,
      updatedAt: 200,
      completedAt: 220,
      latestProgress: "Done",
      toolSteps: [],
      activity: [],
      finalMessageId: null,
      finalMessageTimestamp: 999,
      finalMessageText: "final answer with normalized spaces",
    };
    persistChatReadonlyRun(persistedRun, "main");

    const state = createState({
      client: {
        request: vi.fn(async (method: string) => {
          if (method !== "chat.history") {
            throw new Error(`unexpected method ${method}`);
          }
          return {
            messages: [
              {
                role: "assistant",
                content: [{ type: "text", text: " final   answer with\nnormalized spaces " }],
              },
            ],
            thinkingLevel: null,
          };
        }),
      } as never,
    });

    await loadChatHistory(state);

    expect(state.chatReadonlyRun?.phase).toBe("complete");
    expect(state.chatReadonlyRun?.finalMessageText).toBe("final answer with normalized spaces");
  });

  it("keeps the latest optimistic local user message when history refreshes mid-run", async () => {
    const state = createState({
      chatMessages: [
        {
          role: "assistant",
          content: [{ type: "text", text: "old reply" }],
          timestamp: 1,
        },
        {
          role: "user",
          content: [{ type: "text", text: "new local question" }],
          timestamp: 999,
        },
      ],
      client: {
        request: vi.fn(async (method: string) => {
          if (method !== "chat.history") {
            throw new Error(`unexpected method ${method}`);
          }
          return {
            messages: [
              {
                role: "assistant",
                content: [{ type: "text", text: "old reply" }],
                timestamp: 1,
              },
            ],
            thinkingLevel: null,
          };
        }),
      } as never,
    });
    (state as any)._skipEmptyHistory = true;

    await loadChatHistory(state);

    expect(state.chatMessages).toHaveLength(2);
    const last = state.chatMessages.at(-1) as { role?: string; content?: Array<{ text?: string }> };
    expect(last?.role).toBe("user");
    expect(last?.content?.[0]?.text).toBe("new local question");
  });

  it("does not duplicate the optimistic local user message once transcript already contains it", async () => {
    const state = createState({
      chatMessages: [
        {
          role: "assistant",
          content: [{ type: "text", text: "old reply" }],
          timestamp: 1,
        },
        {
          role: "user",
          content: [{ type: "text", text: "new local question" }],
          timestamp: 999,
        },
      ],
      client: {
        request: vi.fn(async (method: string) => {
          if (method !== "chat.history") {
            throw new Error(`unexpected method ${method}`);
          }
          return {
            messages: [
              {
                role: "assistant",
                content: [{ type: "text", text: "old reply" }],
                timestamp: 1,
              },
              {
                role: "user",
                content: [{ type: "text", text: "new local question" }],
                timestamp: 1000,
              },
              {
                role: "assistant",
                content: [{ type: "text", text: "done" }],
                timestamp: 1001,
              },
            ],
            thinkingLevel: null,
          };
        }),
      } as never,
    });
    (state as any)._skipEmptyHistory = true;

    await loadChatHistory(state);

    expect(
      state.chatMessages.filter((message) => {
        const record = message as { role?: string; content?: Array<{ text?: string }> };
        return record.role === "user" && record.content?.[0]?.text === "new local question";
      }),
    ).toHaveLength(1);
  });

  it("restores archived workflow history for earlier assistant replies in the same session", async () => {
    const firstRun = {
      ...createChatReadonlyRunState("main"),
      phase: "complete" as const,
      startedAt: 100,
      updatedAt: 200,
      completedAt: 210,
      finalMessageId: "msg-1",
      finalMessageTimestamp: 220,
      finalMessageText: "first",
    };
    const secondRun = {
      ...createChatReadonlyRunState("main"),
      phase: "complete" as const,
      startedAt: 300,
      updatedAt: 400,
      completedAt: 410,
      finalMessageId: "msg-2",
      finalMessageTimestamp: 420,
      finalMessageText: "second",
    };
    persistChatReadonlyRun(createChatReadonlyRunState("main"), "main", [firstRun, secondRun]);

    const state = createState({
      client: {
        request: vi.fn(async () => ({
          messages: [
            { id: "msg-1", role: "assistant", content: [{ type: "text", text: "first" }], timestamp: 220 },
            { id: "msg-2", role: "assistant", content: [{ type: "text", text: "second" }], timestamp: 420 },
          ],
          thinkingLevel: null,
        })),
      } as never,
    });

    await loadChatHistory(state);

    expect(state.chatReadonlyRunHistory).toHaveLength(2);
    expect(state.chatReadonlyRunHistory?.map((run) => run.finalMessageId)).toEqual(["msg-1", "msg-2"]);
  });
});

describe("handleChatEvent", () => {
  it("returns null when payload is missing", () => {
    const state = createState();
    expect(handleChatEvent(state, undefined)).toBe(null);
  });

  it("returns null when sessionKey does not match", () => {
    const state = createState({ sessionKey: "main" });
    const payload: ChatEventPayload = {
      runId: "run-1",
      sessionKey: "other",
      state: "final",
    };
    expect(handleChatEvent(state, payload)).toBe(null);
  });

  it("returns null for delta from another run", () => {
    const state = createState({
      sessionKey: "main",
      chatRunId: "run-user",
      chatStream: "Hello",
    });
    const payload: ChatEventPayload = {
      runId: "run-announce",
      sessionKey: "main",
      state: "delta",
      message: { role: "assistant", content: [{ type: "text", text: "Done" }] },
    };
    expect(handleChatEvent(state, payload)).toBe(null);
    expect(state.chatRunId).toBe("run-user");
    expect(state.chatStream).toBe("Hello");
  });

  it("rebinds a remote placeholder run to the real run id on delta", () => {
    const state = createState({
      sessionKey: "main",
      chatRunId: "remote-123",
      chatStreamStartedAt: 99,
      chatReadonlyRun: {
        ...createChatReadonlyRunState("main"),
        runId: "remote-123",
        sessionKey: "main",
        phase: "starting",
        startedAt: 90,
        updatedAt: 99,
      },
    });
    const payload: ChatEventPayload = {
      runId: "run-feishu",
      sessionKey: "main",
      state: "delta",
    };

    expect(handleChatEvent(state, payload)).toBe("delta");
    expect(state.chatRunId).toBe("run-feishu");
    expect(state.chatStreamStartedAt).toBe(99);
    expect(state.chatReadonlyRun?.runId).toBe("run-feishu");
  });

  it("returns 'final' for final from another run (e.g. sub-agent announce) without clearing state", () => {
    const state = createState({
      sessionKey: "main",
      chatRunId: "run-user",
      chatStream: "Working...",
      chatStreamStartedAt: 123,
    });
    const payload: ChatEventPayload = {
      runId: "run-announce",
      sessionKey: "main",
      state: "final",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "Sub-agent findings" }],
      },
    };
    expect(handleChatEvent(state, payload)).toBe("final");
    expect(state.chatRunId).toBe("run-user");
    expect(state.chatStream).toBe("Working...");
    expect(state.chatStreamStartedAt).toBe(123);
  });

  it("processes final from own run and clears state", () => {
    const state = createState({
      sessionKey: "main",
      chatRunId: "run-1",
      chatStream: "Reply",
      chatStreamStartedAt: 100,
    });
    const payload: ChatEventPayload = {
      runId: "run-1",
      sessionKey: "main",
      state: "final",
    };
    expect(handleChatEvent(state, payload)).toBe("final");
    expect(state.chatRunId).toBe(null);
    expect(state.chatStream).toBe(null);
    expect(state.chatStreamStartedAt).toBe(null);
  });

  it("does not append a duplicate final assistant message when remote chat.message already rendered it", () => {
    const message = {
      role: "assistant",
      content: [{ type: "text", text: "已经处理完了" }],
      timestamp: 150,
    };
    const state = createState({
      sessionKey: "feishu:chat-a",
      chatRunId: "run-1",
      chatMessages: [message],
    });
    const payload: ChatEventPayload = {
      runId: "run-1",
      sessionKey: "feishu:chat-a",
      state: "final",
      message,
    };

    expect(handleChatEvent(state, payload)).toBe("final");
    expect(state.chatMessages).toHaveLength(1);
  });

  it("P4: records cross-session final events to globalChatEventStore even when sessionKey does not match", () => {
    globalChatEventStore.clear();
    const state = createState({ sessionKey: "main" });
    const payload: ChatEventPayload = {
      runId: "run-cross",
      sessionKey: "other-session",
      state: "final",
      message: { role: "assistant", content: [{ type: "text", text: "cross done" }] },
    };

    // handleChatEvent returns null (sessionKey mismatch)
    expect(handleChatEvent(state, payload)).toBe(null);

    // But the event is archived in the store
    expect(globalChatEventStore.hasTerminalEvents("other-session")).toBe(true);
    const events = globalChatEventStore.drainTerminalEvents("other-session");
    expect(events).toHaveLength(1);
    expect(events[0].runId).toBe("run-cross");
    expect(events[0].state).toBe("final");
  });

  it("P4: does not record delta events to globalChatEventStore", () => {
    globalChatEventStore.clear();
    const state = createState({ sessionKey: "main" });
    const payload: ChatEventPayload = {
      runId: "run-1",
      sessionKey: "main",
      state: "delta",
      message: { role: "assistant", content: [{ type: "text", text: "streaming..." }] },
    };

    handleChatEvent(state, payload);
    expect(globalChatEventStore.hasTerminalEvents("main")).toBe(false);
  });
});

function createChatModelState(overrides: Partial<ChatModelState> = {}): ChatModelState {
  return {
    client: null,
    connected: true,
    chatModels: [],
    chatCurrentModel: null,
    debugModels: [],
    ...overrides,
  };
}

describe("loadChatModels", () => {
  it("uses the config snapshot current model when models.default.get is unavailable", async () => {
    const request = vi.fn(async (method: string) => {
      if (method === "models.list") {
        return {
          models: [
            {
              id: "gemini-3.1-pro-preview",
              name: "Gemini 3.1 Pro",
              provider: "google",
              source: "managed",
            },
          ],
        };
      }
      if (method === "models.default.get") {
        throw new Error("temporary unavailable");
      }
      if (method === "config.get") {
        return {
          config: {
            agents: {
              defaults: {
                model: {
                  primary: "google/gemini-3.1-flash-lite-preview",
                },
              },
            },
            models: {
              providers: {
                google: {
                  models: [
                    {
                      id: "gemini-3.1-flash-lite-preview",
                      name: "Gemini 3.1 Flash-Lite",
                    },
                  ],
                },
              },
            },
          },
        };
      }
      throw new Error(`unexpected method ${method}`);
    });

    const state = createChatModelState({
      client: { request } as never,
    });
    await loadChatModels(state);

    expect(state.chatCurrentModel).toBe("google/gemini-3.1-flash-lite-preview");
  });

  it("prefers configured custom models over builtin placeholders for the same provider/model", async () => {
    const request = vi.fn(async (method: string) => {
      if (method === "models.list") {
        return {
          models: [
            {
              id: "gemini-3.1-pro-preview",
              name: "Gemini 3.1 Pro",
              provider: "google",
              source: "managed",
            },
            {
              id: "gemini-3.1-flash-lite-preview",
              name: "Gemini 3.1 Flash-Lite",
              provider: "google",
              source: "managed",
            },
          ],
        };
      }
      if (method === "models.default.get") {
        return {
          model: "google/gemini-3.1-flash-lite-preview",
        };
      }
      if (method === "config.get") {
        return {
          config: {
            agents: {
              defaults: {
                model: {
                  primary: "google/gemini-3.1-flash-lite-preview",
                },
              },
            },
            models: {
              providers: {
                google: {
                  models: [
                    {
                      id: "gemini-3.1-pro-preview",
                      name: "Gemini 3.1 Pro",
                    },
                    {
                      id: "gemini-3.1-flash-lite-preview",
                      name: "Gemini 3.1 Flash-Lite",
                    },
                  ],
                },
              },
            },
          },
        };
      }
      throw new Error(`unexpected method ${method}`);
    });

    const state = createChatModelState({
      client: { request } as never,
    });
    await loadChatModels(state);

    expect(state.chatModels).toEqual([
      {
        id: "gemini-3.1-pro-preview",
        name: "Gemini 3.1 Pro",
        provider: "google",
        source: "custom",
      },
      {
        id: "gemini-3.1-flash-lite-preview",
        name: "Gemini 3.1 Flash-Lite",
        provider: "google",
        source: "custom",
      },
    ]);
    expect(state.chatCurrentModel).toBe("google/gemini-3.1-flash-lite-preview");
  });
});
