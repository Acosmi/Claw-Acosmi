import type { EventLogEntry } from "./app-events.ts";
import type { OpenAcosmiApp } from "./app.ts";
import type { CoderConfirmRequest } from "./controllers/coder-confirmation.ts";
import type { ExecApprovalRequest } from "./controllers/exec-approval.ts";
import type { GatewayEventFrame, GatewayHelloOk } from "./gateway.ts";
import type { PermissionDeniedEvent } from "./views/permission-popup.ts";
import { showPermissionPopup } from "./views/permission-popup.ts";
import type { Tab } from "./navigation.ts";
import type { UiSettings } from "./storage.ts";
import type { AgentsListResult, PresenceEntry, HealthSnapshot, StatusSummary } from "./types.ts";
import { CHAT_SESSIONS_ACTIVE_MINUTES, flushChatQueueForEvent } from "./app-chat.ts";
import {
  applySettings,
  loadCron,
  refreshActiveTab,
  setLastActiveSessionKey,
} from "./app-settings.ts";
import { handleAgentEvent, resetToolStream, type AgentEventPayload } from "./app-tool-stream.ts";
import { loadAgents } from "./controllers/agents.ts";
import { loadAssistantIdentity } from "./controllers/assistant-identity.ts";
import { loadChatHistory } from "./controllers/chat.ts";
import { handleChatEvent, type ChatEventPayload } from "./controllers/chat.ts";
import { loadDevices } from "./controllers/devices.ts";
import {
  addCoderConfirm,
  parseCoderConfirmRequested,
  parseCoderConfirmResolved,
  removeCoderConfirm,
} from "./controllers/coder-confirmation.ts";
import {
  addExecApproval,
  parseExecApprovalRequested,
  parseExecApprovalResolved,
  removeExecApproval,
} from "./controllers/exec-approval.ts";
import {
  isEscalationEvent,
  handleEscalationRequested,
  handleEscalationResolved,
} from "./controllers/escalation.ts";
import { loadNodes } from "./controllers/nodes.ts";
import { loadSessions } from "./controllers/sessions.ts";
import { GatewayBrowserClient } from "./gateway.ts";

type GatewayHost = {
  settings: UiSettings;
  password: string;
  client: GatewayBrowserClient | null;
  connected: boolean;
  hello: GatewayHelloOk | null;
  lastError: string | null;
  onboarding?: boolean;
  wizardAutoStarted?: boolean;
  eventLogBuffer: EventLogEntry[];
  eventLog: EventLogEntry[];
  tab: Tab;
  presenceEntries: PresenceEntry[];
  presenceError: string | null;
  presenceStatus: StatusSummary | null;
  agentsLoading: boolean;
  agentsList: AgentsListResult | null;
  agentsError: string | null;
  debugHealth: HealthSnapshot | null;
  assistantName: string;
  assistantAvatar: string | null;
  assistantAgentId: string | null;
  sessionKey: string;
  chatRunId: string | null;
  refreshSessionsAfterChat: Set<string>;
  execApprovalQueue: ExecApprovalRequest[];
  execApprovalError: string | null;
  coderConfirmQueue: CoderConfirmRequest[];
};

type SessionDefaultsSnapshot = {
  defaultAgentId?: string;
  mainKey?: string;
  mainSessionKey?: string;
  scope?: string;
};

function normalizeSessionKeyForDefaults(
  value: string | undefined,
  defaults: SessionDefaultsSnapshot,
): string {
  const raw = (value ?? "").trim();
  const mainSessionKey = defaults.mainSessionKey?.trim();
  if (!mainSessionKey) {
    return raw;
  }
  if (!raw) {
    return mainSessionKey;
  }
  const mainKey = defaults.mainKey?.trim() || "main";
  const defaultAgentId = defaults.defaultAgentId?.trim();
  const isAlias =
    raw === "main" ||
    raw === mainKey ||
    (defaultAgentId &&
      (raw === `agent:${defaultAgentId}:main` || raw === `agent:${defaultAgentId}:${mainKey}`));
  return isAlias ? mainSessionKey : raw;
}

function applySessionDefaults(host: GatewayHost, defaults?: SessionDefaultsSnapshot) {
  if (!defaults?.mainSessionKey) {
    return;
  }
  const resolvedSessionKey = normalizeSessionKeyForDefaults(host.sessionKey, defaults);
  const resolvedSettingsSessionKey = normalizeSessionKeyForDefaults(
    host.settings.sessionKey,
    defaults,
  );
  const resolvedLastActiveSessionKey = normalizeSessionKeyForDefaults(
    host.settings.lastActiveSessionKey,
    defaults,
  );
  const nextSessionKey = resolvedSessionKey || resolvedSettingsSessionKey || host.sessionKey;
  const nextSettings = {
    ...host.settings,
    sessionKey: resolvedSettingsSessionKey || nextSessionKey,
    lastActiveSessionKey: resolvedLastActiveSessionKey || nextSessionKey,
  };
  const shouldUpdateSettings =
    nextSettings.sessionKey !== host.settings.sessionKey ||
    nextSettings.lastActiveSessionKey !== host.settings.lastActiveSessionKey;
  if (nextSessionKey !== host.sessionKey) {
    host.sessionKey = nextSessionKey;
  }
  if (shouldUpdateSettings) {
    applySettings(host as unknown as Parameters<typeof applySettings>[0], nextSettings);
  }
}

export function connectGateway(host: GatewayHost) {
  host.lastError = null;
  host.hello = null;
  host.connected = false;
  host.execApprovalQueue = [];
  host.execApprovalError = null;
  host.coderConfirmQueue = [];

  host.client?.stop();
  host.client = new GatewayBrowserClient({
    url: host.settings.gatewayUrl,
    token: host.settings.token.trim() ? host.settings.token : undefined,
    password: host.password.trim() ? host.password : undefined,
    clientName: "openacosmi-control-ui",
    mode: "webchat",
    onHello: (hello) => {
      host.connected = true;
      host.lastError = null;
      host.hello = hello;
      applySnapshot(host, hello);
      // Reset orphaned chat run state from before disconnect.
      // Any in-flight run's final event was lost during the disconnect window.
      host.chatRunId = null;
      (host as unknown as { chatStream: string | null }).chatStream = null;
      (host as unknown as { chatStreamStartedAt: number | null }).chatStreamStartedAt = null;
      resetToolStream(host as unknown as Parameters<typeof resetToolStream>[0]);
      void loadAssistantIdentity(host as unknown as OpenAcosmiApp);
      void loadAgents(host as unknown as OpenAcosmiApp);
      void loadNodes(host as unknown as OpenAcosmiApp, { quiet: true });
      void loadDevices(host as unknown as OpenAcosmiApp, { quiet: true });
      void refreshActiveTab(host as unknown as Parameters<typeof refreshActiveTab>[0]);
      // Auto-start setup wizard when ?onboarding=1 is in the URL (once only)
      if (host.onboarding && !host.wizardAutoStarted) {
        host.wizardAutoStarted = true;
        const app = host as unknown as OpenAcosmiApp;
        void app.handleStartWizard();
      }
    },
    onClose: ({ code, reason }) => {
      host.connected = false;
      // Code 1012 = Service Restart (expected during config saves, don't show as error)
      if (code !== 1012) {
        const msg = `disconnected (${code}): ${reason || "no reason"}`;
        host.lastError = msg;
        const app = host as unknown as OpenAcosmiApp;
        if (typeof app.addNotification === "function") {
          app.addNotification(msg, "error");
        }
      }
    },
    onEvent: (evt) => handleGatewayEvent(host, evt),
    onGap: ({ expected, received }) => {
      const msg = `event gap detected (expected seq ${expected}, got ${received}); refresh recommended`;
      host.lastError = msg;
      const app = host as unknown as OpenAcosmiApp;
      if (typeof app.addNotification === "function") {
        app.addNotification(msg, "error");
      }
    },
  });
  host.client.start();
}

export function handleGatewayEvent(host: GatewayHost, evt: GatewayEventFrame) {
  try {
    handleGatewayEventUnsafe(host, evt);
  } catch (err) {
    console.error("[gateway] handleGatewayEvent error:", evt.event, err);
  }
}

function handleGatewayEventUnsafe(host: GatewayHost, evt: GatewayEventFrame) {
  host.eventLogBuffer = [
    { ts: Date.now(), event: evt.event, payload: evt.payload },
    ...host.eventLogBuffer,
  ].slice(0, 250);
  if (host.tab === "debug") {
    host.eventLog = host.eventLogBuffer;
  }

  if (evt.event === "agent") {
    if (host.onboarding) {
      return;
    }
    handleAgentEvent(
      host as unknown as Parameters<typeof handleAgentEvent>[0],
      evt.payload as AgentEventPayload | undefined,
    );
    return;
  }

  if (evt.event === "chat") {
    const payload = evt.payload as ChatEventPayload | undefined;
    if (payload?.sessionKey) {
      setLastActiveSessionKey(
        host as unknown as Parameters<typeof setLastActiveSessionKey>[0],
        payload.sessionKey,
      );
    }
    const state = handleChatEvent(host as unknown as OpenAcosmiApp, payload);
    if (state === "final" || state === "error" || state === "aborted") {
      resetToolStream(host as unknown as Parameters<typeof resetToolStream>[0]);
      void flushChatQueueForEvent(host as unknown as Parameters<typeof flushChatQueueForEvent>[0]);
      const runId = payload?.runId;
      if (runId && host.refreshSessionsAfterChat.has(runId)) {
        host.refreshSessionsAfterChat.delete(runId);
        if (state === "final") {
          void loadSessions(host as unknown as OpenAcosmiApp, {
            activeMinutes: CHAT_SESSIONS_ACTIVE_MINUTES,
          });
        }
      }
    }
    if (state === "final") {
      void loadChatHistory(host as unknown as OpenAcosmiApp);
    }
    return;
  }

  if (evt.event === "presence") {
    const payload = evt.payload as { presence?: PresenceEntry[] } | undefined;
    if (payload?.presence && Array.isArray(payload.presence)) {
      host.presenceEntries = payload.presence;
      host.presenceError = null;
      host.presenceStatus = null;
    }
    return;
  }

  if (evt.event === "cron" && host.tab === "cron") {
    void loadCron(host as unknown as Parameters<typeof loadCron>[0]);
  }

  if (evt.event === "device.pair.requested" || evt.event === "device.pair.resolved") {
    void loadDevices(host as unknown as OpenAcosmiApp, { quiet: true });
  }

  if (evt.event === "permission_denied") {
    const payload = evt.payload as PermissionDeniedEvent | undefined;
    if (payload) {
      showPermissionPopup(payload);
      const app = host as unknown as OpenAcosmiApp;
      if (typeof app.requestUpdate === "function") {
        app.requestUpdate();
      }
    }
    return;
  }

  // Bug C fix: 远程频道（飞书/钉钉/企微）聊天消息
  if (evt.event === "chat.message") {
    const payload = evt.payload as {
      sessionKey?: string;
      channel?: string;
      role?: string;
      text?: string;
      ts?: number;
    } | undefined;
    if (payload?.text && (!payload.sessionKey || payload.sessionKey === host.sessionKey)) {
      const msg = {
        role: payload.role ?? "user",
        content: [{ type: "text", text: payload.text }],
        timestamp: payload.ts ?? Date.now(),
        channel: payload.channel,
      };
      const app = host as unknown as OpenAcosmiApp;
      app.chatMessages = [...app.chatMessages, msg];
      if (typeof app.requestUpdate === "function") {
        app.requestUpdate();
      }
    }
    return;
  }

  // 跨会话频道消息通知（飞书等远程频道）
  if (evt.event === "channel.message.incoming") {
    const payload = evt.payload as {
      sessionKey?: string;
      channel?: string;
      text?: string;
      from?: string;
      label?: string;
      ts?: number;
    } | undefined;
    if (payload?.sessionKey && payload?.text) {
      const app = host as unknown as OpenAcosmiApp;
      const fromStr = payload.from ? `[${payload.from}] ` : "";
      const msg = `${fromStr}${payload.text}`;
      if (typeof app.addNotification === "function") {
        app.addNotification(msg, "info");
      }
      if (typeof app.requestUpdate === "function") {
        app.requestUpdate();
      }
    }
    return;
  }

  // Argus 视觉子智能体状态变更通知
  if (evt.event === "argus.status.changed") {
    const payload = evt.payload as {
      state?: string;
      reason?: string;
    } | undefined;
    if (payload?.state === "stopped") {
      const app = host as unknown as OpenAcosmiApp;
      const reason = payload.reason ?? "unknown error";
      const msg = `[Argus] Visual agent stopped: ${reason}. Use argus.restart to recover.`;
      app.lastError = msg;
      if (typeof app.addNotification === "function") {
        app.addNotification(msg, "error");
      }
      if (typeof app.requestUpdate === "function") {
        app.requestUpdate();
      }
    }
    return;
  }

  // Argus 熔断崩溃通知
  if (evt.event === "argus.crash.notify") {
    const payload = evt.payload as { reason?: string } | undefined;
    const app = host as unknown as OpenAcosmiApp;
    const reason = payload?.reason ?? "rapid crash detected";
    const msg = `[Argus] Visual agent stopped due to crash: ${reason}. Send 'argus restart' to recover.`;
    app.lastError = msg;
    if (typeof app.addNotification === "function") {
      app.addNotification(msg, "error");
    }
    if (typeof app.requestUpdate === "function") {
      app.requestUpdate();
    }
    return;
  }

  if (evt.event === "exec.approval.requested") {
    // P2: 区分 escalation 请求（esc_ 前缀）和传统 exec approval
    if (isEscalationEvent(evt.payload)) {
      const app = host as unknown as { escalationState: ReturnType<typeof import("./controllers/escalation.ts").createEscalationState> };
      if (app.escalationState) {
        app.escalationState = handleEscalationRequested(app.escalationState, evt.payload);
      }
      return;
    }
    const entry = parseExecApprovalRequested(evt.payload);
    if (entry) {
      host.execApprovalQueue = addExecApproval(host.execApprovalQueue, entry);
      host.execApprovalError = null;
      const delay = Math.max(0, entry.expiresAtMs - Date.now() + 500);
      window.setTimeout(() => {
        host.execApprovalQueue = removeExecApproval(host.execApprovalQueue, entry.id);
      }, delay);
    }
    return;
  }

  if (evt.event === "exec.approval.resolved") {
    // P2: 区分 escalation resolved（esc_ 前缀）和传统 exec approval
    if (isEscalationEvent(evt.payload)) {
      const app = host as unknown as { escalationState: ReturnType<typeof import("./controllers/escalation.ts").createEscalationState> };
      if (app.escalationState) {
        app.escalationState = handleEscalationResolved(app.escalationState, evt.payload);
      }
      return;
    }
    const resolved = parseExecApprovalResolved(evt.payload);
    if (resolved) {
      host.execApprovalQueue = removeExecApproval(host.execApprovalQueue, resolved.id);
    }
    return;
  }

  // ---------- Coder 确认流事件 ----------

  if (evt.event === "coder.confirm.requested") {
    const entry = parseCoderConfirmRequested(evt.payload);
    if (entry) {
      host.coderConfirmQueue = addCoderConfirm(host.coderConfirmQueue ?? [], entry);
      // 自动过期清理
      const delay = Math.max(0, entry.expiresAtMs - Date.now() + 500);
      window.setTimeout(() => {
        host.coderConfirmQueue = removeCoderConfirm(host.coderConfirmQueue ?? [], entry.id);
      }, delay);
    }
    return;
  }

  if (evt.event === "coder.confirm.resolved") {
    const resolved = parseCoderConfirmResolved(evt.payload);
    if (resolved) {
      host.coderConfirmQueue = removeCoderConfirm(host.coderConfirmQueue ?? [], resolved.id);
    }
  }
}

export function applySnapshot(host: GatewayHost, hello: GatewayHelloOk) {
  const snapshot = hello.snapshot as
    | {
      presence?: PresenceEntry[];
      health?: HealthSnapshot;
      sessionDefaults?: SessionDefaultsSnapshot;
    }
    | undefined;
  if (snapshot?.presence && Array.isArray(snapshot.presence)) {
    host.presenceEntries = snapshot.presence;
  }
  if (snapshot?.health) {
    host.debugHealth = snapshot.health;
  }
  if (snapshot?.sessionDefaults) {
    applySessionDefaults(host, snapshot.sessionDefaults);
  }
}
