import type { EventLogEntry } from "./app-events.ts";
import type { OpenAcosmiApp } from "./app.ts";
import { t } from "./i18n.ts";
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
import { loadChannels } from "./controllers/channels.ts";
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
  loadEscalationStatus,
  createEscalationState,
} from "./controllers/escalation.ts";
import {
  addPlanConfirm,
  parsePlanConfirmRequested,
  parsePlanConfirmResolved,
  removePlanConfirm,
} from "./controllers/plan-confirmation.ts";
import {
  addResultReview,
  parseResultReviewRequested,
  parseResultReviewResolved,
  removeResultReview,
} from "./controllers/result-review.ts";
import {
  addSubagentHelp,
  parseSubagentHelpRequested,
  parseSubagentHelpResolved,
  removeSubagentHelp,
} from "./controllers/subagent-help.ts";
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
  planConfirmQueue: import("./controllers/plan-confirmation.ts").PlanConfirmRequest[];
  resultReviewQueue: import("./controllers/result-review.ts").ResultReviewRequest[];
  subagentHelpQueue: import("./controllers/subagent-help.ts").SubagentHelpRequest[];
  mediaHeartbeat: import("./app-view-state.ts").MediaHeartbeatStatus | null;
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
      void loadChannels(host as unknown as OpenAcosmiApp, false);
      void refreshActiveTab(host as unknown as Parameters<typeof refreshActiveTab>[0]);
      // WS 重连时同步 escalation 状态（防止断连期间状态变更导致前端过时）
      {
        const app = host as unknown as OpenAcosmiApp;
        if (app.client) {
          void loadEscalationStatus(app.client).then((status) => {
            if (status.hasActive && status.active) {
              app.escalationState = { ...app.escalationState, activeGrant: status.active, popupVisible: false, request: null };
            } else if (status.hasPending && status.pending) {
              app.escalationState = { ...app.escalationState, request: status.pending, popupVisible: true, activeGrant: null };
            } else {
              app.escalationState = createEscalationState();
            }
          }).catch(() => { /* escalation status load failure is non-critical */ });
        }
      }
      // Auto-start setup wizard when ?onboarding=1 is in the URL (once only)
      if (host.onboarding && !host.wizardAutoStarted) {
        host.wizardAutoStarted = true;
        const app = host as unknown as OpenAcosmiApp;
        void app.handleStartWizardV2();
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
      // 清除进度指示器
      (host as any).chatProgress = null;
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
    } else if (state === null && payload?.state === "final" && payload?.sessionKey) {
      // 跨 session 回归：handleChatEvent 因 sessionKey 不匹配返回 null，
      // 但任务已完成（state=final）。自动切回原始 session 并刷新历史，
      // 确保用户提问和回复对用户可见。
      const app = host as unknown as OpenAcosmiApp;
      const completedSession = payload.sessionKey;
      app.sessionKey = completedSession;
      app.chatRunId = null;
      (app as any).chatStream = null;
      (app as any).chatStreamStartedAt = null;
      app.applySettings?.({
        ...app.settings,
        sessionKey: completedSession,
        lastActiveSessionKey: completedSession,
      });
      void loadChatHistory(app);
      if (app.crossChannelNotificationActive) {
        app.clearCrossChannelNotification?.();
      }
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

  // 异步进度推送（chat.progress 事件）
  if (evt.event === "chat.progress") {
    const payload = evt.payload as {
      sessionKey?: string;
      summary?: string;
      phase?: string;
      percent?: number;
      ts?: number;
    } | undefined;
    if (payload?.sessionKey === host.sessionKey && payload?.summary) {
      const app = host as unknown as OpenAcosmiApp;
      (app as any).chatProgress = {
        summary: payload.summary,
        phase: payload.phase,
        percent: payload.percent,
        ts: payload.ts ?? Date.now(),
      };
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
      mediaBase64?: string;
      mediaMimeType?: string;
      mediaItems?: Array<{
        mediaBase64?: string;
        mediaMimeType?: string;
      }>;
      ts?: number;
    } | undefined;
    const hasMediaItems = Boolean(payload?.mediaItems?.some((item) => Boolean(item?.mediaBase64)));
    if ((payload?.text || payload?.mediaBase64 || hasMediaItems) &&
      (!payload?.sessionKey || payload.sessionKey === host.sessionKey)) {
      const content: Array<Record<string, unknown>> = [];
      if (payload.text) {
        content.push({ type: "text", text: payload.text });
      }
      const mediaItems = payload.mediaItems ?? [];
      if (mediaItems.length > 0) {
        for (const item of mediaItems) {
          if (!item?.mediaBase64) {
            continue;
          }
          content.push({
            type: "image",
            source: {
              type: "base64",
              data: item.mediaBase64,
              media_type: item.mediaMimeType || "image/png",
            },
          });
        }
      } else if (payload.mediaBase64) {
        content.push({
          type: "image",
          source: {
            type: "base64",
            data: payload.mediaBase64,
            media_type: payload.mediaMimeType || "image/png",
          },
        });
      }
      const msg = {
        role: payload.role ?? "user",
        content,
        timestamp: payload.ts ?? Date.now(),
        channel: payload.channel,
      };
      const app = host as unknown as OpenAcosmiApp;
      app.chatMessages = [...app.chatMessages, msg];

      // 远程频道动画：user 消息到达 → 显示思考动画；assistant 回复到达 → 清除
      const isRemoteChannel = payload.channel && payload.channel !== "web" && payload.channel !== "webchat";
      if (isRemoteChannel) {
        const role = payload.role ?? "user";
        if (role === "user") {
          app.chatRunId = app.chatRunId ?? `remote-${Date.now()}`;
          (app as unknown as { chatStream: string | null }).chatStream =
            (app as unknown as { chatStream: string | null }).chatStream ?? "";
          (app as unknown as { chatStreamStartedAt: number | null }).chatStreamStartedAt =
            (app as unknown as { chatStreamStartedAt: number | null }).chatStreamStartedAt ?? Date.now();
        } else if (role === "assistant" && app.chatRunId?.startsWith("remote-")) {
          app.chatRunId = null;
          (app as unknown as { chatStream: string | null }).chatStream = null;
          (app as unknown as { chatStreamStartedAt: number | null }).chatStreamStartedAt = null;
        }
      }

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

      // Trigger red dot jump only if the notification comes from a session we're not currently viewing
      if (payload.sessionKey !== host.sessionKey) {
        app.crossChannelNotificationActive = true;
        app.crossChannelNotificationSessionKey = payload.sessionKey;
        app.crossChannelNotificationText = payload.text;

        // 缓存入站消息文本，供切换到该 session 时预填充（不依赖红点 3s 超时）
        if (payload.text) {
          if (!(host as any)._pendingChannelMsgs) {
            (host as any)._pendingChannelMsgs = {};
          }
          (host as any)._pendingChannelMsgs[payload.sessionKey] = {
            text: payload.text,
            ts: payload.ts ?? Date.now(),
          };
        }

        const channelKey = payload.channel || "web";
        app.channelUnreadCounts = {
          ...(app.channelUnreadCounts || {}),
          [channelKey]: (app.channelUnreadCounts?.[channelKey] || 0) + 1
        };

        if ((app as any)._crossChannelTimeout) {
          clearTimeout((app as any)._crossChannelTimeout);
        }
        (app as any)._crossChannelTimeout = setTimeout(() => {
          app.crossChannelNotificationActive = false;
        }, 3000);
      }

      if (typeof app.addNotification === "function") {
        app.addNotification(msg, "info", payload.sessionKey);
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
    const app = host as unknown as OpenAcosmiApp;
    if (payload?.state === "stopped") {
      const reason = payload.reason ?? "unknown error";
      const msg = `[Argus] Visual agent stopped: ${reason}. Use argus.restart to recover.`;
      app.lastError = msg;
      if (typeof app.addNotification === "function") {
        app.addNotification(msg, "error");
      }
    }
    // 同步 subagentsList 中 argus-screen 状态
    syncArgusSubagentStatus(app, mapArgusState(payload?.state), payload?.state === "stopped" || payload?.state === "degraded" ? payload?.reason : undefined);
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
    // 同步 subagentsList 中 argus-screen 状态
    syncArgusSubagentStatus(app, "stopped", reason);
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
    return;
  }

  // ---------- 方案确认门控事件 (Phase 1: 三级指挥体系) ----------

  if (evt.event === "plan.confirm.requested") {
    const entry = parsePlanConfirmRequested(evt.payload);
    if (entry) {
      host.planConfirmQueue = addPlanConfirm(host.planConfirmQueue ?? [], entry);
      // 自动过期清理
      const delay = Math.max(0, entry.expiresAtMs - Date.now() + 500);
      window.setTimeout(() => {
        host.planConfirmQueue = removePlanConfirm(host.planConfirmQueue ?? [], entry.id);
      }, delay);
    }
    return;
  }

  if (evt.event === "plan.confirm.resolved") {
    const resolved = parsePlanConfirmResolved(evt.payload);
    if (resolved) {
      host.planConfirmQueue = removePlanConfirm(host.planConfirmQueue ?? [], resolved.id);
    }
    return;
  }

  // ---------- 结果签收门控事件 (Phase 3: 三级指挥体系) ----------

  if (evt.event === "result.approve.requested") {
    const entry = parseResultReviewRequested(evt.payload);
    if (entry) {
      host.resultReviewQueue = addResultReview(host.resultReviewQueue ?? [], entry);
      // 自动过期清理
      const delay = Math.max(0, entry.expiresAtMs - Date.now() + 500);
      window.setTimeout(() => {
        host.resultReviewQueue = removeResultReview(host.resultReviewQueue ?? [], entry.id);
      }, delay);
    }
    return;
  }

  if (evt.event === "result.approve.resolved") {
    const resolved = parseResultReviewResolved(evt.payload);
    if (resolved) {
      host.resultReviewQueue = removeResultReview(host.resultReviewQueue ?? [], resolved.id);
    }
    return;
  }

  // ---------- 任务看板事件 (task.*) ----------

  if (evt.event.startsWith("task.")) {
    const app = host as unknown as OpenAcosmiApp;
    import("./controllers/task-kanban.ts").then((m) => {
      app.taskKanbanState = m.handleTaskEvent(
        app.taskKanbanState,
        evt.event,
        evt.payload as Record<string, unknown> | undefined,
      );
    });
    return;
  }

  // ---------- 子智能体求助事件 (Phase 4: 三级指挥体系) ----------

  if (evt.event === "subagent.help.requested") {
    const entry = parseSubagentHelpRequested(evt.payload);
    if (entry) {
      host.subagentHelpQueue = addSubagentHelp(host.subagentHelpQueue ?? [], entry);
    }
    return;
  }

  if (evt.event === "subagent.help.resolved") {
    const resolved = parseSubagentHelpResolved(evt.payload);
    if (resolved) {
      host.subagentHelpQueue = removeSubagentHelp(host.subagentHelpQueue ?? [], resolved.id);
    }
  }

  // Phase 4: 媒体自动创作通知
  if (evt.event === "media.auto_spawn") {
    const app = host as unknown as OpenAcosmiApp;
    if (typeof app.addNotification === "function") {
      app.addNotification(t("media.autoSpawn.notify"), "info");
    }
    // 递增心跳面板的自动创作计数
    const prev = host.mediaHeartbeat;
    host.mediaHeartbeat = {
      ...prev,
      lastPatrolAt: prev?.lastPatrolAt ?? null,
      nextPatrolAt: prev?.nextPatrolAt ?? null,
      activeJobId: prev?.activeJobId ?? null,
      lastError: prev?.lastError ?? null,
      autoSpawnCount: (prev?.autoSpawnCount ?? 0) + 1,
    };
    if (typeof (host as any).requestUpdate === "function") {
      (host as any).requestUpdate();
    }
    return;
  }

  // Phase 3: 媒体巡检心跳事件
  if (evt.event === "media.heartbeat") {
    const payload = evt.payload as { jobId?: string; kind?: string; error?: string } | undefined;
    if (payload) {
      const now = Date.now();
      const prev = host.mediaHeartbeat;
      host.mediaHeartbeat = {
        lastPatrolAt: payload.kind === "jobDone" || payload.kind === "jobRun" ? now : prev?.lastPatrolAt ?? null,
        nextPatrolAt: prev?.nextPatrolAt ?? null,
        activeJobId: payload.kind === "jobRun" ? (payload.jobId ?? null) : null,
        lastError: payload.kind === "jobError" ? (payload.error ?? "unknown error") : null,
      };
      if (typeof (host as any).requestUpdate === "function") {
        (host as any).requestUpdate();
      }
    }
  }
}

// ---------- Argus → subagentsList 同步辅助 ----------

type ArgusSubagentStatus = "running" | "degraded" | "starting" | "stopped";

function mapArgusState(state: string | undefined): ArgusSubagentStatus {
  switch (state) {
    case "ready": return "running";
    case "degraded": return "degraded";
    case "starting": return "starting";
    default: return "stopped";
  }
}

function syncArgusSubagentStatus(app: OpenAcosmiApp, status: ArgusSubagentStatus, errorReason?: string) {
  if (!app.subagentsList?.length) return;
  const idx = app.subagentsList.findIndex((e) => e.id === "argus-screen");
  if (idx < 0) return;
  const updated = [...app.subagentsList];
  updated[idx] = {
    ...updated[idx],
    status,
    enabled: status === "running" || status === "degraded" || status === "starting",
    error: errorReason ?? undefined,
  };
  app.subagentsList = updated;
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
