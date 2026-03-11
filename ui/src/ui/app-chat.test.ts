import { beforeEach, describe, expect, it, vi } from "vitest";
import { createChatReadonlyRunState } from "./chat/readonly-run-state.ts";
import { handleSendChat, type ChatHost } from "./app-chat.ts";

const mocks = vi.hoisted(() => ({
  loadChatHistoryMock: vi.fn(async () => undefined),
  loadSessionsMock: vi.fn(async () => undefined),
  scheduleChatScrollMock: vi.fn(),
  setLastActiveSessionKeyMock: vi.fn(),
  resetToolStreamMock: vi.fn(),
}));

vi.mock("./controllers/chat.ts", () => ({
  abortChatRun: vi.fn(async () => undefined),
  loadChatHistory: mocks.loadChatHistoryMock,
  sendChatMessage: vi.fn(async () => "run-1"),
}));

vi.mock("./controllers/sessions.ts", () => ({
  loadSessions: mocks.loadSessionsMock,
}));

vi.mock("./app-scroll.ts", () => ({
  scheduleChatScroll: mocks.scheduleChatScrollMock,
}));

vi.mock("./app-settings.ts", () => ({
  setLastActiveSessionKey: mocks.setLastActiveSessionKeyMock,
}));

vi.mock("./app-tool-stream.ts", () => ({
  resetToolStream: mocks.resetToolStreamMock,
}));

vi.mock("./uuid.ts", () => ({
  generateUUID: () => "session-123",
}));

function createHost(): ChatHost {
  return {
    connected: true,
    chatMessage: "保留草稿",
    chatAttachments: [],
    chatQueue: [],
    chatRunId: "run-old",
    chatSending: false,
    sessionKey: "main",
    basePath: "",
    hello: null,
    chatAvatarUrl: null,
    refreshSessionsAfterChat: new Set<string>(),
    chatReadonlyRun: createChatReadonlyRunState("main"),
    chatReadonlyRunHistory: [],
    setTab: vi.fn(),
  };
}

describe("handleSendChat", () => {
  beforeEach(() => {
    mocks.loadChatHistoryMock.mockClear();
    mocks.loadSessionsMock.mockClear();
    mocks.scheduleChatScrollMock.mockClear();
    mocks.setLastActiveSessionKeyMock.mockClear();
    mocks.resetToolStreamMock.mockClear();
  });

  it("switches to chat before creating a new session", async () => {
    const host = createHost();

    await handleSendChat(host, "/new", { restoreDraft: true });

    expect(host.setTab).toHaveBeenCalledWith("chat");
    expect(host.sessionKey).toBe("user:session-123");
    expect(host.chatMessage).toBe("保留草稿");
    expect(host.chatRunId).toBeNull();
    expect(host.chatReadonlyRun?.sessionKey).toBe("user:session-123");
    expect(host.chatReadonlyRunHistory).toEqual([]);
    expect(mocks.loadChatHistoryMock).toHaveBeenCalledTimes(1);
    expect(mocks.loadSessionsMock).toHaveBeenCalledTimes(1);
  });
});
