import { html, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import type { AppViewState } from "./app-view-state.ts";
import {
  createChatReadonlyRunState,
  loadPersistedChatReadonlyRun,
  loadPersistedChatReadonlyRunHistory,
  persistChatReadonlyRun,
  startChatReadonlyRun,
  type ChatUxMode,
} from "./chat/readonly-run-state.ts";
import type { ThemeTransitionContext } from "./theme-transition.ts";
import type { ThemeMode } from "./theme.ts";
import type { UiSettings } from "./storage.ts";
import type { SessionsListResult } from "./types.ts";
import { refreshChat } from "./app-chat.ts";
import { syncUrlWithSessionKey } from "./app-settings.ts";
import { OpenAcosmiApp } from "./app.ts";
import { ChatState, loadChatHistory } from "./controllers/chat.ts";
import { icons, isAccentIcon } from "./icons.ts";
import { iconForTab, pathForTab, titleForTab, type Tab } from "./navigation.ts";
import {
  type Locale,
  SUPPORTED_LOCALES,
  LOCALE_LABELS,
  getLocale,
  setLocale,
  t,
} from "./i18n.ts";
import { loadMediaConfig } from "./views/media-config.ts";

type PendingChannelMessage = {
  text: string;
  ts: number;
};

export type ChatSessionSwitchHost = {
  sessionKey: string;
  chatReadonlyRun: AppViewState["chatReadonlyRun"];
  chatReadonlyRunHistory: AppViewState["chatReadonlyRunHistory"];
  chatMessage: string;
  chatMessages: unknown[];
  chatStream: string | null;
  chatStreamStartedAt: number | null;
  chatRunId: string | null;
  settings: UiSettings;
  resetToolStream: () => void;
  resetChatScroll: () => void;
  applySettings: (next: UiSettings) => void;
  loadAssistantIdentity: () => Promise<unknown> | void;
  _pendingChannelMsgs?: Record<string, PendingChannelMessage>;
  _skipEmptyHistory?: boolean;
};

export function applyChatSessionSwitchState(
  host: ChatSessionSwitchHost,
  sessionKey: string,
  now: number = Date.now(),
) {
  const currentKey = host.sessionKey;
  let nextSettings = host.settings;
  if (currentKey && currentKey !== sessionKey) {
    persistChatReadonlyRun(host.chatReadonlyRun, currentKey, host.chatReadonlyRunHistory);
    const curPrefixMatch = currentKey.match(/^([a-z]+):/);
    const curPrefix =
      curPrefixMatch && curPrefixMatch[1] !== "global" && curPrefixMatch[1] !== "unknown"
        ? curPrefixMatch[1]
        : "user";
    nextSettings = {
      ...host.settings,
      lastSessionByChannel: {
        ...(host.settings.lastSessionByChannel || {}),
        [curPrefix]: currentKey,
      },
    };
  }

  host.sessionKey = sessionKey;
  host.chatReadonlyRun = loadPersistedChatReadonlyRun(sessionKey) ?? createChatReadonlyRunState(sessionKey);
  host.chatReadonlyRunHistory = loadPersistedChatReadonlyRunHistory(sessionKey);
  host.chatMessage = "";
  host.chatStream = null;
  host.chatStreamStartedAt = null;
  host.chatRunId = null;
  host.resetToolStream();
  host.resetChatScroll();
  host.applySettings({
    ...nextSettings,
    sessionKey,
    lastActiveSessionKey: sessionKey,
  });
  void host.loadAssistantIdentity();

  const pending = host._pendingChannelMsgs?.[sessionKey];
  if (!pending) {
    return;
  }

  delete host._pendingChannelMsgs?.[sessionKey];
  host.chatMessages = [
    {
      role: "user",
      content: [{ type: "text", text: pending.text }],
      timestamp: pending.ts,
    },
  ] as unknown[];
  host._skipEmptyHistory = true;
  host.chatRunId = `remote-switch-${now}`;
  host.chatStream = "";
  host.chatStreamStartedAt = pending.ts;
  startChatReadonlyRun(host, host.chatRunId, pending.ts, sessionKey);
}

export function isMacosWailsShell(
  installKind: string | null | undefined,
  runtime: {
    platform?: string | null;
    protocol?: string | null;
    hostname?: string | null;
  } = {
    platform: typeof navigator === "undefined" ? null : navigator.platform,
    protocol: typeof window === "undefined" ? null : window.location.protocol,
    hostname: typeof window === "undefined" ? null : window.location.hostname,
  },
): boolean {
  if (installKind === "macos-wails") {
    return true;
  }

  const platform = (runtime.platform ?? "").toLowerCase();
  const protocol = (runtime.protocol ?? "").toLowerCase();
  const hostname = (runtime.hostname ?? "").toLowerCase();

  return platform.includes("mac") && (protocol === "wails:" || hostname === "wails.localhost");
}

export function renderTab(state: AppViewState, tab: Tab, badge?: number) {
  const href = pathForTab(tab, state.basePath);
  const iconName = iconForTab(tab);
  const iconClass = isAccentIcon(iconName) ? "nav-item__icon nav-item__icon--accent" : "nav-item__icon";
  const isActive = state.tab === tab || (tab === "automation" && state.tab === "media");
  return html`
    <a
      href=${href}
      class="nav-item ${isActive ? "active" : ""}"
      @click=${(event: MouseEvent) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }
      event.preventDefault();
      if (tab === "automation") {
        state.automationPanel = "hub";
      }
      state.setTab(tab);
      }}
      title=${titleForTab(tab)}
    >
      <span class=${iconClass} aria-hidden="true">${icons[iconName]}</span>
      <span class="nav-item__text">${titleForTab(tab)}</span>
      ${badge && badge > 0 ? html`
        <span class="nav-item__badge">${badge > 99 ? '99+' : badge}</span>
      ` : nothing}
    </a>
  `;
}

export function renderChatControls(state: AppViewState) {
  const mainSessionKey = resolveMainSessionKey(state.hello, state.sessionsResult);
  const sessionOptions = resolveSessionOptions(
    state.sessionKey,
    state.sessionsResult,
    mainSessionKey
  );
  const disableThinkingToggle = state.onboarding;
  const showThinking = state.onboarding ? false : state.settings.chatShowThinking;
  const chatUxMode = state.chatUxMode ?? state.settings.chatUxMode ?? "codex-readonly";
  // Refresh icon
  const refreshIcon = html`
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path>
      <path d="M21 3v5h-5"></path>
    </svg>
  `;
  const app = state as unknown as OpenAcosmiApp;
  const applyChatUxMode = (next: ChatUxMode) => (e: Event) => {
    e.preventDefault();
    e.stopPropagation();
    if (chatUxMode === next) {
      return;
    }
    state.applySettings({
      ...state.settings,
      chatUxMode: next,
    });
  };

  // Calculate available channels and their most recent session
  const channelSessions = new Map<string, { key: string, name: string }>();

  // Gather available channel prefixes
  const channelsAvailable = new Set<string>();
  channelsAvailable.add("user"); // Always ensure user (web) is available

  const currentPrefixMatch = state.sessionKey.match(/^([a-z]+):/);
  if (currentPrefixMatch && currentPrefixMatch[1] !== "global" && currentPrefixMatch[1] !== "unknown") {
    channelsAvailable.add(currentPrefixMatch[1]);
  }

  if (app.channelsSnapshot?.channelAccounts) {
    for (const [prefix, accounts] of Object.entries(app.channelsSnapshot.channelAccounts)) {
      if (prefix !== "global" && prefix !== "unknown" && Array.isArray(accounts)) {
        // Only show Remote Channels if they have at least one valid, configured, or running account
        const hasValidAccount = accounts.some(a => a.configured || a.running || a.tokenSource || a.botTokenSource || a.connected);
        if (hasValidAccount) {
          channelsAvailable.add(prefix);
        }
      }
    }
  }

  // Populate channelSessions
  for (const prefix of channelsAvailable) {
    const memoryKey = state.settings.lastSessionByChannel?.[prefix];

    // Ignore latest list scans for the user web connection context
    const latestListKey = prefix !== "user"
      ? state.sessionsResult?.sessions.find(s => s.key.startsWith(`${prefix}:`))?.key
      : undefined;

    // For web channel ("user"), we should try to return to the main global default session instead of "user:default" phantom session.
    const defaultKey = prefix === "user"
      ? ((app.hello?.snapshot as any)?.sessionDefaults?.mainSessionKey || "main")
      : `${prefix}:default`;

    const targetKey = memoryKey || latestListKey || defaultKey;

    const mapKey = prefix === "user" ? "web" : prefix;
    const name = prefix === "user"
      ? "网页频道"
      : (app.channelsSnapshot?.channelLabels?.[prefix] || prefix);

    channelSessions.set(mapKey, { key: targetKey, name });
  }

  // 智能体频道：扫描 sessionsResult 中 coder: 前缀的 session
  if (state.sessionsResult?.sessions) {
    const coderSession = state.sessionsResult.sessions.find(s => s.key.startsWith("coder:"));
    if (coderSession) {
      const memoryKey = state.settings.lastSessionByChannel?.["coder"];
      const targetKey = memoryKey || coderSession.key;
      channelSessions.set("coder", { key: targetKey, name: "智能体" });
    }
  }
  // 当前 session 是 coder 时确保对应频道可见
  if (state.sessionKey.startsWith("coder:") && !channelSessions.has("coder")) {
    channelSessions.set("coder", { key: state.sessionKey, name: "智能体" });
  }

  const totalUnread = Object.values(app.channelUnreadCounts || {}).reduce((a, b) => a + b, 0);

  const jumpToCrossChannelSession = () => {
    const targetSession = app.crossChannelNotificationSessionKey;
    if (!app.crossChannelNotificationActive || !targetSession) {
      return;
    }

    app.clearCrossChannelNotification?.();

    const prefixMatch = targetSession.match(/^([a-z]+):/);
    const channelKey = (prefixMatch && prefixMatch[1] !== "global" && prefixMatch[1] !== "unknown")
      ? prefixMatch[1]
      : "web";
    if (app.channelUnreadCounts && app.channelUnreadCounts[channelKey]) {
      const counts = { ...app.channelUnreadCounts };
      delete counts[channelKey];
      app.channelUnreadCounts = counts;
    }

    if (state.sessionKey !== targetSession) {
      // 预填充逻辑已移入 applySessionSwitch（从 _pendingChannelMsgs 缓存统一消费）
      applySessionSwitch(targetSession);
    }
  };

  const toggleChannelDropdown = (e: Event) => {
    e.preventDefault();
    e.stopPropagation();

    // Red dot click-to-jump interception
    if (app.crossChannelNotificationActive && app.crossChannelNotificationSessionKey) {
      jumpToCrossChannelSession();
      return; // Do not open the dropdown
    }

    app.isChannelDropdownOpen = !app.isChannelDropdownOpen;
    if (app.isChannelDropdownOpen) {
      app.isSessionDropdownOpen = false;
      app.clearCrossChannelNotification?.();
    }
  };

  const toggleSessionDropdown = (e: Event) => {
    e.preventDefault();
    e.stopPropagation();
    app.isSessionDropdownOpen = !app.isSessionDropdownOpen;
    if (app.isSessionDropdownOpen) app.isChannelDropdownOpen = false;
  };

  const closeDropdowns = () => {
    app.isChannelDropdownOpen = false;
    app.isSessionDropdownOpen = false;
    document.removeEventListener('click', closeDropdowns);
  };

  if (app.isChannelDropdownOpen || app.isSessionDropdownOpen) {
    document.addEventListener('click', closeDropdowns, { once: true });
  }

  const handleChannelSwitch = (channelKey: string, sessionKey: string) => (e: Event) => {
    e.preventDefault();
    e.stopPropagation();

    if (app.channelUnreadCounts && app.channelUnreadCounts[channelKey]) {
      const counts = { ...app.channelUnreadCounts };
      delete counts[channelKey];
      app.channelUnreadCounts = counts;
    }

    closeDropdowns();

    if (state.sessionKey !== sessionKey) {
      applySessionSwitch(sessionKey);
    }
  };

  const handleSessionSwitch = (sessionKey: string) => (e: Event) => {
    e.preventDefault();
    e.stopPropagation();
    closeDropdowns();
    if (state.sessionKey !== sessionKey) {
      applySessionSwitch(sessionKey);
    }
  };

  const applySessionSwitch = (sessionKey: string) => {
    applyChatSessionSwitchState(app as unknown as ChatSessionSwitchHost, sessionKey);
    syncUrlWithSessionKey(state as unknown as Parameters<typeof syncUrlWithSessionKey>[0], sessionKey, true);
    void loadChatHistory(state as unknown as ChatState);
  };

  // Determine current active channel and session names
  let currentChannelName = "网页频道";;
  for (const [prefix, info] of channelSessions.entries()) {
    if (state.sessionKey === info.key || state.sessionKey.startsWith(prefix === "web" ? "user:" : `${prefix}:`)) {
      currentChannelName = info.name;
      break;
    }
  }

  const currentSessionObj = sessionOptions.find(s => s.key === state.sessionKey);
  const currentSessionName = currentSessionObj?.displayName ?? state.sessionKey;

  return html`
    <style>
      .split-capsule {
        display: flex;
        align-items: stretch;
        border: 1px solid var(--border-color);
        border-radius: 8px;
        background: var(--surface-1);
        box-shadow: 0 1px 3px rgba(0,0,0,0.05);
        margin-right: 8px;
        position: relative;
        transition: all 0.3s ease;
        min-width: 0;
        max-width: 100%;
      }
      
      .split-capsule.is-thinking {
        position: relative;
        overflow: hidden;
        border-color: rgba(96, 165, 250, 0.42);
        background:
          linear-gradient(135deg, rgba(239, 246, 255, 0.95) 0%, rgba(224, 242, 254, 0.94) 42%, rgba(219, 234, 254, 0.92) 100%);
        box-shadow:
          0 10px 28px rgba(59, 130, 246, 0.18),
          inset 0 1px 0 rgba(255, 255, 255, 0.88);
        backdrop-filter: blur(14px);
        -webkit-backdrop-filter: blur(14px);
        animation: capsule-breathe 2.5s ease-in-out infinite alternate;
      }
      
      .split-capsule.is-thinking::before {
        content: "";
        position: absolute;
        top: 0; bottom: 0; width: 50%;
        left: -100%;
        background: linear-gradient(
          90deg,
          transparent 0%,
          rgba(255, 255, 255, 0.18) 18%,
          rgba(147, 197, 253, 0.42) 52%,
          rgba(219, 234, 254, 0.12) 78%,
          transparent 100%
        );
        transform: skewX(-20deg);
        animation: glass-sweep 3s infinite linear;
        pointer-events: none;
      }

      .split-capsule.is-thinking::after {
        content: "";
        position: absolute;
        inset: 0;
        background:
          radial-gradient(circle at 12% 20%, rgba(191, 219, 254, 0.45), transparent 42%),
          radial-gradient(circle at 88% 82%, rgba(125, 211, 252, 0.22), transparent 38%);
        pointer-events: none;
      }
      
      @keyframes capsule-breathe {
        0% {
          box-shadow:
            0 8px 22px rgba(96, 165, 250, 0.14),
            0 0 0 1px rgba(191, 219, 254, 0.3),
            inset 0 1px 0 rgba(255, 255, 255, 0.82);
        }
        100% {
          box-shadow:
            0 14px 34px rgba(59, 130, 246, 0.22),
            0 0 0 1px rgba(147, 197, 253, 0.42),
            inset 0 1px 0 rgba(255, 255, 255, 0.96);
        }
      }
      
      @keyframes glass-sweep {
        0% { left: -100%; }
        50% { left: 200%; }
        100% { left: 200%; }
      }
      
      .split-capsule-btn {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 5px 12px;
        background: transparent;
        border: none;
        cursor: pointer;
        color: var(--text-base);
        font-size: 13px;
        font-weight: 500;
        transition: background 0.15s ease;
        min-width: 0;
      }
      .split-capsule-btn:hover { background: rgba(128,128,128,0.08); }
      .split-capsule-btn:active { background: rgba(128,128,128,0.15); }
      .split-capsule.is-thinking .split-capsule-btn:hover { background: rgba(147, 197, 253, 0.16); }
      .split-capsule.is-thinking .split-capsule-btn:active { background: rgba(96, 165, 250, 0.2); }
      .chat-ux-toggle {
        position: relative;
        display: inline-flex;
        align-items: center;
        padding: 3px;
        border: 1px solid var(--border-color);
        border-radius: 999px;
        background: color-mix(in srgb, var(--surface-1) 82%, transparent);
        box-shadow: 0 1px 3px rgba(0,0,0,0.05);
        flex-shrink: 0;
        overflow: hidden;
        isolation: isolate;
      }
      .chat-ux-toggle__thumb {
        position: absolute;
        top: 3px;
        bottom: 3px;
        left: 3px;
        width: calc(50% - 3px);
        border-radius: 999px;
        background: linear-gradient(135deg, rgba(56, 189, 248, 0.18), rgba(59, 130, 246, 0.22));
        box-shadow:
          inset 0 0 0 1px rgba(56, 189, 248, 0.18),
          0 6px 14px rgba(14, 165, 233, 0.14);
        transition:
          transform 0.24s cubic-bezier(0.22, 1, 0.36, 1),
          background 0.24s ease,
          box-shadow 0.24s ease;
        pointer-events: none;
        z-index: 0;
      }
      .chat-ux-toggle[data-mode="codex-readonly"] .chat-ux-toggle__thumb {
        transform: translateX(100%);
        background: linear-gradient(135deg, rgba(14, 165, 233, 0.24), rgba(59, 130, 246, 0.34));
        box-shadow:
          inset 0 0 0 1px rgba(14, 165, 233, 0.26),
          0 8px 18px rgba(37, 99, 235, 0.18);
      }
      .chat-ux-toggle__button {
        position: relative;
        z-index: 1;
        border: none;
        background: transparent;
        color: var(--text-dim);
        font-size: 11px;
        font-weight: 600;
        padding: 7px 10px;
        border-radius: 999px;
        cursor: pointer;
        transition: color 0.2s ease, transform 0.18s ease;
        white-space: nowrap;
      }
      .chat-ux-toggle__button:hover {
        color: var(--text-strong);
      }
      .chat-ux-toggle__button:active {
        transform: translateY(1px) scale(0.98);
      }
      .chat-ux-toggle__button.active {
        color: var(--text-strong);
      }
      @media (max-width: 960px) {
        .chat-ux-toggle__button {
          padding: 6px 9px;
          font-size: 10px;
        }
      }
      .switch-tag {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        padding: 2px 10px;
        border-radius: 999px;
        font-size: 11px;
        font-weight: 700;
        letter-spacing: 0.04em;
        color: #fff;
        background: linear-gradient(120deg, #ef4444 0%, #f59e0b 35%, #22c55e 68%, #3b82f6 100%);
        box-shadow: 0 1px 4px rgba(59, 130, 246, 0.25);
        flex-shrink: 0;
      }
      .switch-current {
        display: inline-flex;
        align-items: center;
        gap: 6px;
      }
      .switch-unread-dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: #ef4444;
        box-shadow: 0 0 0 2px var(--bg-body), 0 0 6px rgba(239, 68, 68, 0.45);
        flex-shrink: 0;
      }
      .capsule-left {
        border-right: 1px solid var(--border-color);
        border-radius: 8px 0 0 8px;
        position: relative;
        overflow: hidden;
        min-width: 96px;
      }
      .split-capsule.is-thinking .capsule-left {
        border-right-color: rgba(96, 165, 250, 0.22);
      }
      .capsule-right {
        border-radius: 0 8px 8px 0;
        width: clamp(120px, 22vw, 220px);
        overflow: hidden;
      }
      .cross-channel-badge {
        position: absolute;
        top: calc(100% + 10px);
        left: 0;
        display: inline-flex;
        align-items: center;
        gap: 6px;
        min-height: 34px;
        max-width: min(320px, calc(100vw - 120px));
        padding: 0 12px;
        background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
        border: 1px solid rgba(255, 255, 255, 0.18);
        border-radius: 14px;
        color: white;
        font-size: 12px;
        font-weight: 600;
        opacity: 0;
        visibility: hidden;
        overflow: hidden;
        text-align: left;
        box-shadow:
          0 12px 28px rgba(220, 38, 38, 0.28),
          0 0 0 1px rgba(255, 255, 255, 0.12) inset;
        transform: translateY(-8px) scale(0.98);
        transform-origin: top left;
        pointer-events: none;
        z-index: 100020;
        white-space: nowrap;
        -webkit-app-region: no-drag;
        transition:
          opacity 0.18s ease,
          transform 0.22s cubic-bezier(0.22, 1, 0.36, 1),
          visibility 0.18s ease;
      }
      .cross-channel-badge::before {
        content: "";
        position: absolute;
        top: -6px;
        left: 24px;
        width: 12px;
        height: 12px;
        border-radius: 3px;
        background: #ef4444;
        transform: rotate(45deg);
        box-shadow: -1px -1px 0 rgba(255, 255, 255, 0.12);
      }
      .cross-channel-badge__text {
        display: block;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        min-width: 0;
      }
      .cross-channel-badge.show {
        opacity: 1;
        visibility: visible;
        transform: translateY(0) scale(1);
        pointer-events: auto;
      }
      .chat-channel-switcher {
        position: relative;
      }
      .mac-dropdown {
        position: absolute;
        top: calc(100% + 6px);
        bottom: auto;
        background: var(--surface-1);
        background: color-mix(in srgb, var(--surface-1) 85%, transparent);
        backdrop-filter: blur(20px);
        -webkit-backdrop-filter: blur(20px);
        border: 1px solid var(--border-color);
        border-radius: 8px;
        box-shadow: 0 8px 30px rgba(0,0,0,0.15);
        z-index: 99999 !important; /* Forces dropdown above all chat layers */
        min-width: 200px;
        max-width: min(92vw, 360px);
        max-height: 350px;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        padding: 6px;
        animation: mac-dropdown-fade 0.15s cubic-bezier(0, 0, 0.2, 1);
      }
      .mac-dropdown.right-aligned {
        left: unset;
        right: 0;
      }
      .mac-dropdown--session {
        min-width: 220px;
        width: min(360px, calc(100vw - 24px));
      }
      @keyframes mac-dropdown-fade {
        from { opacity: 0; transform: translateY(-4px) scale(0.98); }
        to { opacity: 1; transform: translateY(0) scale(1); }
      }
      .mac-dropdown-item {
        padding: 8px 12px;
        border-radius: 6px;
        border: none;
        background: transparent;
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: space-between;
        color: var(--text-base);
        font-size: 13px;
        transition: background-color 0.1s, color 0.1s;
        text-align: left;
        line-height: 1.4;
      }
      .mac-dropdown-item:hover {
        background: #3b82f6;
        color: white !important;
      }
      .mac-dropdown-item.active {
        background: #3b82f6;
        color: white !important;
        font-weight: 600;
        box-shadow: 0 2px 6px rgba(59, 130, 246, 0.3);
      }
      .mac-dropdown-item.active:hover {
        background: #2563eb;
      }
      .session-name-truncate {
        min-width: 0;
        max-width: 100%;
        flex: 1;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    </style>

    <div class="chat-controls">
      <div class="split-capsule ${(state.chatLoading || state.chatRunId || state.chatSending || state.chatStream !== null) ? 'is-thinking' : ''}">
        <!-- Channel Selector -->
        <div class="chat-channel-switcher">
          <button class="split-capsule-btn capsule-left" @click=${toggleChannelDropdown} title="切换频道">
            <span class="switch-tag">切换</span>
            <span class="switch-current">
              <span>${currentChannelName}</span>
              ${(!app.crossChannelNotificationActive && totalUnread > 0)
      ? html`<span class="switch-unread-dot"></span>`
      : nothing}
            </span>
          </button>
          <button
            class="cross-channel-badge ${app.crossChannelNotificationActive ? 'show' : ''}"
            @click=${(e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      jumpToCrossChannelSession();
    }}
            title=${app.crossChannelNotificationText || "切换到有新消息的频道"}
            aria-hidden=${app.crossChannelNotificationActive ? "false" : "true"}
            tabindex=${app.crossChannelNotificationActive ? "0" : "-1"}
            type="button"
          >
            <span class="statusDot pulse" style="background: white; flex-shrink: 0; width: 6px; height: 6px;"></span>
            <span class="cross-channel-badge__text">${app.crossChannelNotificationText}</span>
          </button>

          ${app.isChannelDropdownOpen ? html`
            <div class="mac-dropdown">
              <div style="padding: 4px 8px; font-size: 11px; color: var(--text-dim); text-transform: uppercase; font-weight: 600; margin-bottom: 4px;">频道列表</div>
              ${Array.from(channelSessions.entries()).map(([prefix, info]) => {
        const unreadCount = app.channelUnreadCounts?.[prefix] || 0;
        const isActive = state.sessionKey === info.key || state.sessionKey.startsWith(prefix === "web" ? "user:" : `${prefix}:`);
        return html`
                  <button class="mac-dropdown-item ${isActive ? 'active' : ''}" @click=${handleChannelSwitch(prefix, info.key)}>
                    <div style="display: flex; align-items: center; gap: 8px;">
                      <span>${info.name}</span>
                      ${isActive ? html`<span style="background: #3b82f6; color: white; border-radius: 12px; font-size: 10px; padding: 2px 6px; font-weight: 600; box-shadow: 0 0 0 1px rgba(255,255,255,0.2) inset;">当前</span>` : nothing}
                    </div>
                    ${unreadCount > 0 ? html`
                      <span style="background: #ef4444; color: white; border-radius: 12px; font-size: 11px; padding: 2px 6px; font-weight: bold; margin-left: 8px; box-shadow: 0 0 4px rgba(239, 68, 68, 0.4);">
                        ${unreadCount}
                      </span>
                    ` : nothing}
                  </button>
                `;
      })}
            </div>
          ` : nothing}
        </div>

        <!-- Session Selector -->
        <div style="position: relative;">
          <button class="split-capsule-btn capsule-right" @click=${toggleSessionDropdown} title="切换会话" ?disabled=${!state.connected}>
            <span class="session-name-truncate" style="opacity: 0.8;">${currentSessionName}</span>
            <span style="font-size: 10px; opacity: 0.6; margin-left: 2px;">▼</span>
          </button>

          ${app.isSessionDropdownOpen && state.connected ? html`
            <div class="mac-dropdown right-aligned mac-dropdown--session">
              <div style="padding: 4px 8px; font-size: 11px; color: var(--text-dim); text-transform: uppercase; font-weight: 600; margin-bottom: 4px;">历史会话记录</div>
              ${sessionOptions.length === 0 ? html`
                <div style="padding: 8px 12px; font-size: 12px; color: var(--text-dim);">无更多历史记录</div>
              ` : sessionOptions.map(entry => html`
                <button class="mac-dropdown-item ${entry.key === state.sessionKey ? 'active' : ''}" @click=${handleSessionSwitch(entry.key)}>
                  <div style="display: flex; align-items: center; justify-content: space-between; width: 100%;">
                    <span style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                      ${entry.displayName ?? entry.key}
                    </span>
                    ${entry.key === state.sessionKey ? html`<span style="background: #3b82f6; color: white; border-radius: 12px; font-size: 10px; padding: 2px 6px; font-weight: 600; flex-shrink: 0; margin-left: 8px;">当前</span>` : nothing}
                  </div>
                </button>
              `)}
            </div>
          ` : nothing}
        </div>
      </div>

      <div
        class="chat-ux-toggle"
        data-mode=${chatUxMode}
        role="group"
        aria-label="${t("helpers.chatUx")}"
      >
        <span class="chat-ux-toggle__thumb" aria-hidden="true"></span>
        <button
          class="chat-ux-toggle__button ${chatUxMode === "classic" ? "active" : ""}"
          data-mode="classic"
          type="button"
          aria-pressed=${chatUxMode === "classic"}
          title="${t("helpers.chatUxClassicTitle")}"
          @click=${applyChatUxMode("classic")}
        >
          ${t("helpers.chatUxClassic")}
        </button>
        <button
          class="chat-ux-toggle__button ${chatUxMode === "codex-readonly" ? "active" : ""}"
          data-mode="codex-readonly"
          type="button"
          aria-pressed=${chatUxMode === "codex-readonly"}
          title="${t("helpers.chatUxCodexTitle")}"
          @click=${applyChatUxMode("codex-readonly")}
        >
          ${t("helpers.chatUxCodex")}
        </button>
      </div>
    </div>
  `;
}

type SessionDefaultsSnapshot = {
  mainSessionKey?: string;
  mainKey?: string;
};

function resolveMainSessionKey(
  hello: AppViewState["hello"],
  sessions: SessionsListResult | null,
): string | null {
  const snapshot = hello?.snapshot as { sessionDefaults?: SessionDefaultsSnapshot } | undefined;
  const mainSessionKey = snapshot?.sessionDefaults?.mainSessionKey?.trim();
  if (mainSessionKey) {
    return mainSessionKey;
  }
  const mainKey = snapshot?.sessionDefaults?.mainKey?.trim();
  if (mainKey) {
    return mainKey;
  }
  if (sessions?.sessions?.some((row) => row.key === "main")) {
    return "main";
  }
  return null;
}

function resolveSessionDisplayName(key: string, row?: SessionsListResult["sessions"][number]) {
  const label = row?.label?.trim() || "";
  const displayName = row?.displayName?.trim() || "";
  if (label && label !== key) {
    return `${label} (${key})`;
  }
  if (displayName && displayName !== key) {
    return `${key} (${displayName})`;
  }
  return key;
}

function resolveSessionOptions(
  sessionKey: string,
  sessions: SessionsListResult | null,
  mainSessionKey?: string | null
) {
  const seen = new Set<string>();
  const options: Array<{ key: string; displayName?: string }> = [];

  const resolvedMain = mainSessionKey ? sessions?.sessions?.find((s) => s.key === mainSessionKey) : undefined;
  const resolvedCurrent = sessions?.sessions?.find((s) => s.key === sessionKey);

  // Add main session key first
  if (mainSessionKey) {
    seen.add(mainSessionKey);
    options.push({
      key: mainSessionKey,
      displayName: resolveSessionDisplayName(mainSessionKey, resolvedMain),
    });
  }

  // Add current session key next
  if (!seen.has(sessionKey)) {
    seen.add(sessionKey);
    options.push({
      key: sessionKey,
      displayName: resolveSessionDisplayName(sessionKey, resolvedCurrent),
    });
  }

  // Add sessions from the result (without cross-channel logic)
  if (sessions?.sessions) {
    for (const s of sessions.sessions) {
      if (!seen.has(s.key)) {
        seen.add(s.key);
        options.push({
          key: s.key,
          displayName: resolveSessionDisplayName(s.key, s),
        });
      }
    }
  }

  return options;
}

const THEME_ORDER: ThemeMode[] = ["system", "light", "dark"];

export function renderThemeToggle(state: AppViewState) {
  const index = Math.max(0, THEME_ORDER.indexOf(state.theme));
  const applyTheme = (next: ThemeMode) => (event: MouseEvent) => {
    const element = event.currentTarget as HTMLElement;
    const context: ThemeTransitionContext = { element };
    if (event.clientX || event.clientY) {
      context.pointerClientX = event.clientX;
      context.pointerClientY = event.clientY;
    }
    state.setTheme(next, context);
  };

  return html`
    <div class="theme-toggle" style="--theme-index: ${index};">
      <div class="theme-toggle__track" role="group" aria-label="${t("helpers.theme")}">
        <span class="theme-toggle__indicator"></span>
        <button
          class="theme-toggle__button ${state.theme === "system" ? "active" : ""}"
          @click=${applyTheme("system")}
          aria-pressed=${state.theme === "system"}
          aria-label="${t("helpers.systemTheme")}"
          title="${t("helpers.system")}"
        >
          ${renderMonitorIcon()}
        </button>
        <button
          class="theme-toggle__button ${state.theme === "light" ? "active" : ""}"
          @click=${applyTheme("light")}
          aria-pressed=${state.theme === "light"}
          aria-label="${t("helpers.lightTheme")}"
          title="${t("helpers.light")}"
        >
          ${renderSunIcon()}
        </button>
        <button
          class="theme-toggle__button ${state.theme === "dark" ? "active" : ""}"
          @click=${applyTheme("dark")}
          aria-pressed=${state.theme === "dark"}
          aria-label="${t("helpers.darkTheme")}"
          title="${t("helpers.dark")}"
        >
          ${renderMoonIcon()}
        </button>
      </div>
    </div>
  `;
}

function renderSunIcon() {
  return html`
    <svg class="theme-icon" viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="4"></circle>
      <path d="M12 2v2"></path>
      <path d="M12 20v2"></path>
      <path d="m4.93 4.93 1.41 1.41"></path>
      <path d="m17.66 17.66 1.41 1.41"></path>
      <path d="M2 12h2"></path>
      <path d="M20 12h2"></path>
      <path d="m6.34 17.66-1.41 1.41"></path>
      <path d="m19.07 4.93-1.41 1.41"></path>
    </svg>
  `;
}

function renderMoonIcon() {
  return html`
    <svg class="theme-icon" viewBox="0 0 24 24" aria-hidden="true">
      <path
        d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"
      ></path>
    </svg>
  `;
}

function renderMonitorIcon() {
  return html`
    <svg class="theme-icon" viewBox="0 0 24 24" aria-hidden="true">
      <rect width="20" height="14" x="2" y="3" rx="2"></rect>
      <line x1="8" x2="16" y1="21" y2="21"></line>
      <line x1="12" x2="12" y1="17" y2="21"></line>
    </svg>
  `;
}

export function renderLocaleSwitch(state: AppViewState) {
  const current = getLocale();
  return html`
    <select
      class="locale-switch"
      .value=${current}
      aria-label=${t("locale.label")}
      @change=${(e: Event) => {
      const next = (e.target as HTMLSelectElement).value as Locale;
      setLocale(next);
      state.applySettings({ ...state.settings, locale: next });
    }}
    >
      ${SUPPORTED_LOCALES.map(
      (loc) => html`
          <option value=${loc} ?selected=${loc === current}>
            ${LOCALE_LABELS[loc]}
          </option>
        `,
    )}
    </select>
  `;
}
