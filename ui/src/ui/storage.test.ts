import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clearArgusPostAuthNewChat,
  loadSettings,
  markArgusPostAuthNewChat,
  UI_SETTINGS_VERSION,
} from "./storage.ts";

type StorageMock = {
  getItem: ReturnType<typeof vi.fn>;
  setItem: ReturnType<typeof vi.fn>;
  removeItem: ReturnType<typeof vi.fn>;
  clear: ReturnType<typeof vi.fn>;
};

function createLocalStorageMock(initial: Record<string, string> = {}): StorageMock {
  const store = new Map(Object.entries(initial));
  return {
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      store.delete(key);
    }),
    clear: vi.fn(() => {
      store.clear();
    }),
  };
}

describe("loadSettings", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/chat");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("migrates legacy classic chat mode to workflow mode", () => {
    const localStorageMock = createLocalStorageMock({
      "openacosmi.control.settings.v1": JSON.stringify({
        gatewayUrl: "ws://127.0.0.1",
        sessionKey: "main",
        lastActiveSessionKey: "main",
        theme: "system",
        locale: "zh",
        chatFocusMode: false,
        chatShowThinking: true,
        chatUxMode: "classic",
        splitRatio: 0.6,
        navCollapsed: false,
        navGroupsCollapsed: {},
      }),
    });
    vi.stubGlobal("localStorage", localStorageMock);

    const settings = loadSettings();

    expect(settings.chatUxMode).toBe("codex-readonly");
    expect(settings.settingsVersion).toBe(UI_SETTINGS_VERSION);
  });

  it("preserves explicit classic mode once the settings version is current", () => {
    const localStorageMock = createLocalStorageMock({
      "openacosmi.control.settings.v1": JSON.stringify({
        settingsVersion: UI_SETTINGS_VERSION,
        gatewayUrl: "ws://127.0.0.1",
        sessionKey: "main",
        lastActiveSessionKey: "main",
        theme: "system",
        locale: "zh",
        chatFocusMode: false,
        chatShowThinking: true,
        chatUxMode: "classic",
        splitRatio: 0.6,
        navCollapsed: false,
        navGroupsCollapsed: {},
      }),
    });
    vi.stubGlobal("localStorage", localStorageMock);

    const settings = loadSettings();

    expect(settings.chatUxMode).toBe("classic");
    expect(settings.settingsVersion).toBe(UI_SETTINGS_VERSION);
  });

  it("forces a fresh chat session after pending argus authorization restart", () => {
    const localStorageMock = createLocalStorageMock({
      "openacosmi.control.settings.v1": JSON.stringify({
        settingsVersion: UI_SETTINGS_VERSION,
        gatewayUrl: "ws://127.0.0.1",
        sessionKey: "user:history",
        lastActiveSessionKey: "user:history",
        theme: "system",
        locale: "zh",
        chatFocusMode: false,
        chatShowThinking: true,
        chatUxMode: "codex-readonly",
        splitRatio: 0.6,
        navCollapsed: false,
        navGroupsCollapsed: {},
        lastSessionByChannel: { user: "user:history" },
      }),
    });
    vi.stubGlobal("localStorage", localStorageMock);
    markArgusPostAuthNewChat();

    const settings = loadSettings();

    expect(settings.sessionKey).toMatch(/^user:/);
    expect(settings.sessionKey).not.toBe("user:history");
    expect(settings.lastActiveSessionKey).toBe(settings.sessionKey);
    expect(settings.lastSessionByChannel).toEqual({});
    expect(localStorageMock.removeItem).toHaveBeenCalledWith("openacosmi.argus.post_auth_new_chat");
  });

  it("exposes helpers to set and clear the argus post-auth flag", () => {
    const localStorageMock = createLocalStorageMock();
    vi.stubGlobal("localStorage", localStorageMock);

    markArgusPostAuthNewChat();
    clearArgusPostAuthNewChat();

    expect(localStorageMock.setItem).toHaveBeenCalledWith("openacosmi.argus.post_auth_new_chat", "1");
    expect(localStorageMock.removeItem).toHaveBeenCalledWith("openacosmi.argus.post_auth_new_chat");
  });
});
