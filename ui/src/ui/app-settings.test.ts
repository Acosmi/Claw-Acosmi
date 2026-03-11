import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatUxMode } from "./chat/readonly-run-state.ts";
import type { Tab } from "./navigation.ts";
import { applySettings, applySettingsFromUrl, refreshActiveTab, setTabFromRoute } from "./app-settings.ts";

type SettingsHost = Parameters<typeof setTabFromRoute>[0] & {
  logsPollInterval: number | null;
  debugPollInterval: number | null;
  chatUxMode?: ChatUxMode;
  pendingGatewayUrl?: string | null;
  client?: { request: ReturnType<typeof vi.fn> } | null;
  chatLoading?: boolean;
  chatMessages?: unknown[];
  chatThinkingLevel?: string | null;
  chatModels?: Array<{ id: string; name: string; provider: string; source: string }>;
  chatCurrentModel?: string | null;
  debugModels?: unknown[];
  sessionsResult?: unknown;
  chatAvatarUrl?: string | null;
  updateComplete?: Promise<unknown>;
  querySelector?: (selectors: string) => Element | null;
  style?: CSSStyleDeclaration;
  chatScrollFrame?: number | null;
  chatScrollTimeout?: number | null;
  chatUserNearBottom?: boolean;
  chatNewMessagesBelow?: boolean;
  logsScrollFrame?: number | null;
  hello?: unknown;
};

const createHost = (tab: Tab): SettingsHost => ({
  settings: {
    gatewayUrl: "",
    token: "",
    sessionKey: "main",
    lastActiveSessionKey: "main",
    theme: "system",
    chatFocusMode: false,
    chatShowThinking: true,
    chatUxMode: "classic",
    splitRatio: 0.6,
    navCollapsed: false,
    navGroupsCollapsed: {},
    locale: "zh",
  },
  theme: "system",
  themeResolved: "dark",
  applySessionKey: "main",
  sessionKey: "main",
  tab,
  connected: false,
  chatHasAutoScrolled: false,
  logsAtBottom: false,
  eventLog: [],
  eventLogBuffer: [],
  basePath: "",
  themeMedia: null,
  themeMediaHandler: null,
  logsPollInterval: null,
  debugPollInterval: null,
  client: null,
  chatLoading: false,
  chatMessages: [],
  chatThinkingLevel: null,
  chatModels: [],
  chatCurrentModel: null,
  debugModels: [],
  sessionsResult: null,
  chatAvatarUrl: null,
  updateComplete: Promise.resolve(),
  querySelector: () => null,
  style: document.documentElement.style,
  chatScrollFrame: null,
  chatScrollTimeout: null,
  chatUserNearBottom: true,
  chatNewMessagesBelow: false,
  logsScrollFrame: null,
  hello: null,
});

describe("setTabFromRoute", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal("localStorage", {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    });
    window.history.replaceState({}, "", "/chat");
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("starts and stops log polling based on the tab", () => {
    const host = createHost("chat");

    setTabFromRoute(host, "logs");
    expect(host.logsPollInterval).not.toBeNull();
    expect(host.debugPollInterval).toBeNull();

    setTabFromRoute(host, "chat");
    expect(host.logsPollInterval).toBeNull();
  });

  it("starts and stops debug polling based on the tab", () => {
    const host = createHost("chat");

    setTabFromRoute(host, "debug");
    expect(host.debugPollInterval).not.toBeNull();
    expect(host.logsPollInterval).toBeNull();

    setTabFromRoute(host, "chat");
    expect(host.debugPollInterval).toBeNull();
  });

  it("syncs chat ux mode onto the live host state", () => {
    const host = createHost("chat");
    host.chatUxMode = "classic";

    applySettings(host, {
      ...host.settings,
      chatUxMode: "codex-readonly",
    });

    expect(host.settings.chatUxMode).toBe("codex-readonly");
    expect(host.chatUxMode).toBe("codex-readonly");
  });

  it("reads chat ux mode from the url and strips the query param", () => {
    const host = createHost("chat");
    host.chatUxMode = "classic";
    window.history.replaceState({}, "", "/chat?chatUx=codex");
    const replaceState = vi.spyOn(window.history, "replaceState");

    applySettingsFromUrl(host);

    expect(host.settings.chatUxMode).toBe("codex-readonly");
    expect(host.chatUxMode).toBe("codex-readonly");
    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(String(replaceState.mock.calls[0]?.[2] ?? "")).not.toContain("chatUx");
  });

  it("refreshes chat models when entering the chat tab", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => ({
      ok: false,
      json: async () => ({}),
    })));
    const request = vi.fn(async (method: string) => {
      if (method === "chat.history") return { messages: [], thinkingLevel: null };
      if (method === "sessions.list") return { sessions: [], count: 0 };
      if (method === "assistant.identity.get") return {};
      if (method === "models.list") {
        return { models: [{ id: "gpt-4.1", name: "GPT-4.1", provider: "openai", source: "managed" }] };
      }
      if (method === "models.default.get") return { model: "openai/gpt-4.1" };
      if (method === "config.get") {
        return { config: { agents: { defaults: { model: { primary: "openai/gpt-4.1" } } } } };
      }
      throw new Error(`unexpected method ${method}`);
    });
    const host = createHost("chat");
    host.connected = true;
    host.client = { request };

    await refreshActiveTab(host);

    expect(request).toHaveBeenCalledWith("models.list", {});
    expect(request).toHaveBeenCalledWith("models.default.get", {});
    expect(host.chatCurrentModel).toBe("openai/gpt-4.1");
  });
});
