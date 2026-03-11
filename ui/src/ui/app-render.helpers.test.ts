import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  applyChatSessionSwitchState,
  isMacosWailsShell,
  type ChatSessionSwitchHost,
} from "./app-render.helpers.ts";
import { createChatReadonlyRunState, persistChatReadonlyRun } from "./chat/readonly-run-state.ts";
import type { UiSettings } from "./storage.ts";

function createSettings(): UiSettings {
  return {
    gatewayUrl: "",
    token: "",
    sessionKey: "main",
    lastActiveSessionKey: "main",
    lastSessionByChannel: {},
    theme: "system",
    locale: "zh",
    chatFocusMode: false,
    chatShowThinking: true,
    chatUxMode: "classic",
    splitRatio: 0.6,
    navCollapsed: false,
    navGroupsCollapsed: {},
  };
}

function createHost(): ChatSessionSwitchHost {
  const host: ChatSessionSwitchHost = {
    sessionKey: "main",
    chatReadonlyRun: createChatReadonlyRunState("main"),
    chatReadonlyRunHistory: [],
    chatMessage: "draft",
    chatMessages: [],
    chatStream: "working",
    chatStreamStartedAt: 123,
    chatRunId: "run-main",
    settings: createSettings(),
    resetToolStream: vi.fn(),
    resetChatScroll: vi.fn(),
    applySettings(next) {
      host.settings = next;
    },
    loadAssistantIdentity: vi.fn(),
    _pendingChannelMsgs: {},
    _skipEmptyHistory: false,
  };
  return host;
}

describe("applyChatSessionSwitchState", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("restores a persisted completed workflow when switching sessions", () => {
    const host = createHost();
    persistChatReadonlyRun({
      ...createChatReadonlyRunState("feishu:chat-a"),
      phase: "complete",
      startedAt: 100,
      updatedAt: 150,
      completedAt: 180,
      latestProgress: "done",
      toolSteps: [],
      activity: [],
      finalMessageId: "msg-1",
      finalMessageTimestamp: 200,
    }, "feishu:chat-a");

    applyChatSessionSwitchState(host, "feishu:chat-a", 1_234);

    expect(host.chatReadonlyRun.phase).toBe("complete");
    expect(host.chatReadonlyRun.finalMessageId).toBe("msg-1");
    expect(host.chatReadonlyRunHistory).toHaveLength(1);
  });

  it("clears stale run bindings before switching to a new session", () => {
    const host = createHost();

    applyChatSessionSwitchState(host, "feishu:chat-a", 1_234);

    expect(host.sessionKey).toBe("feishu:chat-a");
    expect(host.chatRunId).toBeNull();
    expect(host.chatStream).toBeNull();
    expect(host.chatStreamStartedAt).toBeNull();
    expect(host.chatReadonlyRun.phase).toBe("idle");
    expect(host.settings.lastSessionByChannel?.user).toBe("main");
  });

  it("persists the previous session workflow before switching away", () => {
    const host = createHost();
    host.chatReadonlyRun = {
      ...createChatReadonlyRunState("main"),
      runId: "run-main",
      sessionKey: "main",
      phase: "working",
      startedAt: 100,
      updatedAt: 150,
      latestProgress: "collecting data",
      activity: [],
      toolSteps: [],
      completedAt: null,
      progressPhase: null,
      draftingText: null,
      lastToolName: null,
      lastError: null,
      finalMessageId: null,
      finalMessageTimestamp: null,
      finalMessageText: null,
    };

    applyChatSessionSwitchState(host, "feishu:chat-a", 1_234);

    const persisted = JSON.parse(
      window.localStorage.getItem("openacosmi.control.chat-readonly-run.v1:main") ?? "{}",
    ) as { current?: { runId?: string; phase?: string } };

    expect(persisted.current?.runId).toBe("run-main");
    expect(persisted.current?.phase).toBe("working");
  });

  it("starts a fresh remote wait-state run for pending messages even if the previous session had an active run", () => {
    const host = createHost();
    host._pendingChannelMsgs = {
      "feishu:chat-a": {
        text: "新消息",
        ts: 888,
      },
    };

    applyChatSessionSwitchState(host, "feishu:chat-a", 9_999);

    expect(host.chatRunId).toBe("remote-switch-9999");
    expect(host.chatStream).toBe("");
    expect(host.chatStreamStartedAt).toBe(888);
    expect(host.chatReadonlyRun.runId).toBe("remote-switch-9999");
    expect(host.chatReadonlyRun.sessionKey).toBe("feishu:chat-a");
    expect(host.chatReadonlyRun.phase).toBe("starting");
    expect(host.chatMessages).toEqual([
      {
        role: "user",
        content: [{ type: "text", text: "新消息" }],
        timestamp: 888,
      },
    ]);
    expect(host._pendingChannelMsgs?.["feishu:chat-a"]).toBeUndefined();
  });
});

describe("isMacosWailsShell", () => {
  it("matches packaged macos-wails installs directly", () => {
    expect(isMacosWailsShell("macos-wails", {
      platform: "Win32",
      protocol: "http:",
      hostname: "127.0.0.1",
    })).toBe(true);
  });

  it("matches the Wails runtime host on macOS before desktop status loads", () => {
    expect(isMacosWailsShell(null, {
      platform: "MacIntel",
      protocol: "http:",
      hostname: "wails.localhost",
    })).toBe(true);
  });

  it("does not match regular mac browsers", () => {
    expect(isMacosWailsShell(null, {
      platform: "MacIntel",
      protocol: "http:",
      hostname: "127.0.0.1",
    })).toBe(false);
  });
});
