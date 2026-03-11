const KEY = "openacosmi.control.settings.v1";
const ARGUS_POST_AUTH_NEW_CHAT_KEY = "openacosmi.argus.post_auth_new_chat";
export const UI_SETTINGS_VERSION = 2;

import type { Locale } from "./i18n.js";
import type { ChatUxMode } from "./chat/readonly-run-state.ts";
import type { ThemeMode } from "./theme.js";
import { generateUUID } from "./uuid.ts";

export type UiSettings = {
  settingsVersion?: number;
  gatewayUrl: string;
  token: string;
  sessionKey: string;
  lastActiveSessionKey: string;
  theme: ThemeMode;
  locale: Locale; // UI language: 'zh' | 'en', default 'zh'
  chatFocusMode: boolean;
  chatShowThinking: boolean;
  chatUxMode: ChatUxMode;
  splitRatio: number; // Sidebar split ratio (0.4 to 0.7, default 0.6)
  navCollapsed: boolean; // Collapsible sidebar state
  navGroupsCollapsed: Record<string, boolean>; // Which nav groups are collapsed
  lastSessionByChannel?: Record<string, string>; // History for cross-channel navigation
};

export function markArgusPostAuthNewChat() {
  localStorage.setItem(ARGUS_POST_AUTH_NEW_CHAT_KEY, "1");
}

export function clearArgusPostAuthNewChat() {
  localStorage.removeItem(ARGUS_POST_AUTH_NEW_CHAT_KEY);
}

export function loadSettings(): UiSettings {
  const defaultUrl = (() => {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    // In dev mode (Vite on 26222), the Vite proxy handles /ws → ws://localhost:19001/ws
    // so we just use the same origin. In production, the gateway serves everything.
    return `${proto}://${location.host}`;
  })();

  const defaults: UiSettings = {
    settingsVersion: UI_SETTINGS_VERSION,
    gatewayUrl: defaultUrl,
    token: "",
    sessionKey: "main",
    lastActiveSessionKey: "main",
    theme: "system",
    locale: "zh",
    chatFocusMode: false,
    chatShowThinking: true,
    chatUxMode: "codex-readonly",
    splitRatio: 0.6,
    navCollapsed: false,
    navGroupsCollapsed: {},
  };

  try {
    const needsFreshChatAfterArgusAuth =
      localStorage.getItem(ARGUS_POST_AUTH_NEW_CHAT_KEY) === "1";
    const raw = localStorage.getItem(KEY);
    if (!raw) {
      if (!needsFreshChatAfterArgusAuth) {
        return defaults;
      }
      clearArgusPostAuthNewChat();
      // P2 身份收敛: 同步上下文无法调用 sessions.create API，
      // 保留本地生成，chat.send auto-create 会在首次对话时注册。
      const freshSessionKey = `user:${generateUUID()}`;
      return {
        ...defaults,
        sessionKey: freshSessionKey,
        lastActiveSessionKey: freshSessionKey,
        lastSessionByChannel: {},
      };
    }
    const parsed = JSON.parse(raw) as Partial<UiSettings>;
    const storedVersion =
      typeof parsed.settingsVersion === "number" && Number.isFinite(parsed.settingsVersion)
        ? parsed.settingsVersion
        : 0;
    const migratedChatUxMode =
      storedVersion < UI_SETTINGS_VERSION
        ? defaults.chatUxMode
        : parsed.chatUxMode === "classic" || parsed.chatUxMode === "codex-readonly"
          ? parsed.chatUxMode
          : defaults.chatUxMode;
    const loaded = {
      settingsVersion: UI_SETTINGS_VERSION,
      gatewayUrl:
        typeof parsed.gatewayUrl === "string" && parsed.gatewayUrl.trim()
          ? parsed.gatewayUrl.trim()
          : defaults.gatewayUrl,
      token: typeof parsed.token === "string" ? parsed.token : defaults.token,
      sessionKey:
        typeof parsed.sessionKey === "string" && parsed.sessionKey.trim()
          ? parsed.sessionKey.trim()
          : defaults.sessionKey,
      lastActiveSessionKey:
        typeof parsed.lastActiveSessionKey === "string" && parsed.lastActiveSessionKey.trim()
          ? parsed.lastActiveSessionKey.trim()
          : (typeof parsed.sessionKey === "string" && parsed.sessionKey.trim()) ||
          defaults.lastActiveSessionKey,
      theme:
        parsed.theme === "light" || parsed.theme === "dark" || parsed.theme === "system"
          ? parsed.theme
          : defaults.theme,
      locale:
        parsed.locale === "zh" || parsed.locale === "en"
          ? parsed.locale
          : defaults.locale,
      chatFocusMode:
        typeof parsed.chatFocusMode === "boolean" ? parsed.chatFocusMode : defaults.chatFocusMode,
      chatShowThinking:
        typeof parsed.chatShowThinking === "boolean"
          ? parsed.chatShowThinking
          : defaults.chatShowThinking,
      chatUxMode: migratedChatUxMode,
      splitRatio:
        typeof parsed.splitRatio === "number" &&
          parsed.splitRatio >= 0.4 &&
          parsed.splitRatio <= 0.7
          ? parsed.splitRatio
          : defaults.splitRatio,
      navCollapsed:
        typeof parsed.navCollapsed === "boolean" ? parsed.navCollapsed : defaults.navCollapsed,
      navGroupsCollapsed:
        typeof parsed.navGroupsCollapsed === "object" && parsed.navGroupsCollapsed !== null
          ? parsed.navGroupsCollapsed
          : defaults.navGroupsCollapsed,
      lastSessionByChannel:
        typeof parsed.lastSessionByChannel === "object" && parsed.lastSessionByChannel !== null
          ? parsed.lastSessionByChannel
          : {},
    };
    if (!needsFreshChatAfterArgusAuth) {
      return loaded;
    }
    clearArgusPostAuthNewChat();
    // P2 身份收敛: 同步上下文 — 本地生成 session key，待 WS 连接后由网关注册
    const freshSessionKey = `user:${generateUUID()}`;
    return {
      ...loaded,
      sessionKey: freshSessionKey,
      lastActiveSessionKey: freshSessionKey,
      lastSessionByChannel: {},
    };
  } catch {
    if (localStorage.getItem(ARGUS_POST_AUTH_NEW_CHAT_KEY) === "1") {
      clearArgusPostAuthNewChat();
      // P2 身份收敛: 同步上下文 — 本地生成 session key，待 WS 连接后由网关注册
      const freshSessionKey = `user:${generateUUID()}`;
      return {
        ...defaults,
        sessionKey: freshSessionKey,
        lastActiveSessionKey: freshSessionKey,
        lastSessionByChannel: {},
      };
    }
    return defaults;
  }
}

export function saveSettings(next: UiSettings) {
  localStorage.setItem(KEY, JSON.stringify({
    ...next,
    settingsVersion: UI_SETTINGS_VERSION,
  }));
}
