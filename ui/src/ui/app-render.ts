import { html, nothing } from "lit";
import type { AppViewState } from "./app-view-state.ts";
import type { UsageState } from "./controllers/usage.ts";
import { parseAgentSessionKey } from "./session-key.ts";
import { refreshChatAvatar } from "./app-chat.ts";
import { renderChatControls, renderLocaleSwitch, renderTab, renderThemeToggle } from "./app-render.helpers.ts";
import { initLocale, t } from "./i18n.ts";
import { loadAgentFileContent, loadAgentFiles, saveAgentFile } from "./controllers/agent-files.ts";
import { loadAgentIdentities, loadAgentIdentity } from "./controllers/agent-identity.ts";
import { loadAgentSkills } from "./controllers/agent-skills.ts";
import { loadAgents } from "./controllers/agents.ts";
import { loadChannels } from "./controllers/channels.ts";
import { loadChatHistory } from "./controllers/chat.ts";
import {
  applyConfig,
  loadConfig,
  runUpdate,
  saveConfig,
  updateConfigFormValue,
  removeConfigFormValue,
} from "./controllers/config.ts";
import {
  loadCronRuns,
  toggleCronJob,
  runCronJob,
  removeCronJob,
  addCronJob,
} from "./controllers/cron.ts";
import { loadDebug, callDebugMethod } from "./controllers/debug.ts";
import {
  approveDevicePairing,
  loadDevices,
  rejectDevicePairing,
  revokeDeviceToken,
  rotateDeviceToken,
} from "./controllers/devices.ts";
import {
  loadExecApprovals,
  removeExecApprovalsFormValue,
  saveExecApprovals,
  updateExecApprovalsFormValue,
} from "./controllers/exec-approvals.ts";
import { loadSecurity, updateSecurityLevel } from "./controllers/security.ts";
import { loadLogs } from "./controllers/logs.ts";
import { loadNodes } from "./controllers/nodes.ts";
import { loadPresence } from "./controllers/presence.ts";
import {
  clearMemorySearch,
  deleteMemory,
  importSkills,
  loadMemoryDetail,
  loadMemoryLLMConfig,
  loadMemoryList,
  loadMemoryStats,
  loadMemoryStatus,
  saveMemoryLLMConfig,
  searchMemories,
} from "./controllers/memory.ts";
import { deleteSession, loadSessions, patchSession } from "./controllers/sessions.ts";
import {
  distributeSkills,
  installSkill,
  loadSkills,
  saveSkillApiKey,
  updateSkillEdit,
  updateSkillEnabled,
} from "./controllers/skills.ts";
import { loadUsage, loadSessionTimeSeries, loadSessionLogs } from "./controllers/usage.ts";
import { icons } from "./icons.ts";
import { normalizeBasePath, getTabGroups, subtitleForTab, titleForTab } from "./navigation.ts";

// Module-scope debounce for usage date changes (avoids type-unsafe hacks on state object)
let usageDateDebounceTimeout: number | null = null;
const debouncedLoadUsage = (state: UsageState) => {
  if (usageDateDebounceTimeout) {
    clearTimeout(usageDateDebounceTimeout);
  }
  usageDateDebounceTimeout = window.setTimeout(() => void loadUsage(state), 400);
};
import { renderAgents } from "./views/agents.ts";
import { renderPlugins } from "./views/plugins.ts";
import { savePluginConfig, loadTools, loadBrowserToolConfig, saveBrowserToolConfig } from "./controllers/plugins.ts";
import { renderChannels } from "./views/channels.ts";
import { renderChat } from "./views/chat.ts";
import { showPermissionPopup, hidePermissionPopup } from "./views/permission-popup.ts";
import { renderConfig } from "./views/config.ts";
import { renderCron } from "./views/cron.ts";
import { renderDebug } from "./views/debug.ts";
import { renderCoderConfirmPrompt } from "./views/coder-confirm.ts";
import { renderEscalationPopup } from "./views/escalation-popup.ts";
import { renderPlanConfirmPopup } from "./views/plan-confirmation-popup.ts";
import { renderResultReviewPopup } from "./views/result-review-popup.ts";
import { renderSubagentHelpPopup } from "./views/subagent-help-popup.ts";
import { resolveEscalation, revokeEscalation } from "./controllers/escalation.ts";
import { renderExecApprovalPrompt } from "./views/exec-approval.ts";
import { renderGatewayUrlConfirmation } from "./views/gateway-url-confirmation.ts";
import { renderInstances } from "./views/instances.ts";
import { renderLogs } from "./views/logs.ts";
import { renderNodes } from "./views/nodes.ts";
import { renderOverview } from "./views/overview.ts";
import { renderSecurity } from "./views/security.ts";
import { renderMemory, renderMemoryTypeCapsules, resetLLMDraft } from "./views/memory.ts";
import { renderSessions } from "./views/sessions.ts";
import { renderSkills } from "./views/skills.ts";
import { renderSubAgents } from "./views/subagents.ts";
import { renderUsage } from "./views/usage.ts";
import { renderWizard } from "./views/wizard.ts";
import { renderWizardV2 } from "./views/wizard-v2.ts";
import { renderChannelWizard, openChannelWizard } from "./views/wizard-channel.ts";
import { renderNotificationCenter } from "./views/notification-center.ts";
import { renderMediaConfig, loadMediaConfig } from "./views/media-config.ts";
import { renderMediaDashboard } from "./views/media-dashboard.ts";
import { loadMediaDashboard } from "./controllers/media-dashboard.ts";
import { renderTaskKanban } from "./views/task-kanban.ts";

const AVATAR_DATA_RE = /^data:/i;
const AVATAR_HTTP_RE = /^https?:\/\//i;

function resolveAssistantAvatarUrl(state: AppViewState): string | undefined {
  const list = state.agentsList?.agents ?? [];
  const parsed = parseAgentSessionKey(state.sessionKey);
  const agentId = parsed?.agentId ?? state.agentsList?.defaultId ?? "main";
  const agent = list.find((entry) => entry.id === agentId);
  const identity = agent?.identity;
  const candidate = identity?.avatarUrl ?? identity?.avatar;
  if (!candidate) {
    return undefined;
  }
  if (AVATAR_DATA_RE.test(candidate) || AVATAR_HTTP_RE.test(candidate)) {
    return candidate;
  }
  return identity?.avatarUrl;
}

export function renderApp(state: AppViewState) {
  const presenceCount = state.presenceEntries.length;
  const sessionsCount = state.sessionsResult?.count ?? null;
  const cronNext = state.cronStatus?.nextWakeAtMs ?? null;
  const chatDisabledReason = state.connected ? null : "Disconnected from gateway.";
  const isChat = state.tab === "chat";
  const chatFocus = isChat && (state.settings.chatFocusMode || state.onboarding);
  const showThinking = state.onboarding ? false : state.settings.chatShowThinking;
  const assistantAvatarUrl = resolveAssistantAvatarUrl(state);
  const chatAvatarUrl = state.chatAvatarUrl ?? assistantAvatarUrl ?? null;
  const configValue =
    state.configForm ?? (state.configSnapshot?.config as Record<string, unknown> | null);
  const basePath = normalizeBasePath(state.basePath ?? "");
  const resolvedAgentId =
    state.agentsSelectedId ??
    state.agentsList?.defaultId ??
    state.agentsList?.agents?.[0]?.id ??
    null;

  return html`
    <div class="shell ${isChat ? "shell--chat" : ""} ${chatFocus ? "shell--chat-focus" : ""} ${state.settings.navCollapsed ? "shell--nav-collapsed" : ""} ${state.onboarding ? "shell--onboarding" : ""}">
      <header class="topbar">
        <div class="topbar-left">
          <button
            class="nav-collapse-toggle"
            @click=${() =>
      state.applySettings({
        ...state.settings,
        navCollapsed: !state.settings.navCollapsed,
      })}
            title="${state.settings.navCollapsed ? t("topbar.expandSidebar") : t("topbar.collapseSidebar")}"
            aria-label="${state.settings.navCollapsed ? t("topbar.expandSidebar") : t("topbar.collapseSidebar")}"
          >
            <span class="nav-collapse-toggle__icon">${icons.menu}</span>
          </button>
          <div class="brand">
            <div class="brand-logo">
              <img src=${basePath ? `${basePath}/crab-logo.png` : "/crab-logo.png"} alt="OpenAcosmi" />
            </div>
            <div class="brand-text">
              <div class="brand-title">
                <span style="font-weight: bold; line-height: 28px;">${t("topbar.brandPrimary")}</span><span style="font-weight: 300; line-height: 28px;">${t("topbar.brandSecondary")}</span>
              </div>
            </div>
          </div>
        </div>
        <div class="topbar-status">
          <div class="pill">
            <span class="statusDot ${state.connected ? "ok" : ""}"></span>
            <span>${t("topbar.health")}</span>
            <span class="mono">${state.connected ? t("topbar.ok") : t("topbar.offline")}</span>
          </div>
          
          ${renderLocaleSwitch(state)}
            ${renderThemeToggle(state)}

          <!-- Notification Bell and Dropdown Container -->
          <div class="notification-container" style="position: relative;">
            <button
              class="notification-bell"
              @click=${(e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      state.notificationsOpen = !state.notificationsOpen;
      (state as any).requestUpdate?.();
    }}
              title="Notifications"
              aria-label="Notifications"
            >
              ${icons.bell}
              ${(() => {
      const unreadCount = state.notifications?.filter((n) => !n.read).length || 0;
      if (unreadCount > 0) {
        return html`<span class="notification-badge">${unreadCount > 99 ? '99+' : unreadCount}</span>`;
      }
      return nothing;
    })()}
            </button>
            ${renderNotificationCenter(state)}
          </div>
        </div>
      </header>
      <aside class="nav ${state.settings.navCollapsed ? "nav--collapsed" : ""}">
        ${getTabGroups().map((group) => {
      const isGroupCollapsed = state.settings.navGroupsCollapsed[group.label] ?? false;
      const hasActiveTab = group.tabs.some((tab) => tab === state.tab);
      return html`
            <div class="nav-group ${isGroupCollapsed && !hasActiveTab ? "nav-group--collapsed" : ""}">
              <button
                class="nav-label"
                @click=${() => {
          const next = { ...state.settings.navGroupsCollapsed };
          next[group.label] = !isGroupCollapsed;
          state.applySettings({
            ...state.settings,
            navGroupsCollapsed: next,
          });
        }}
                aria-expanded=${!isGroupCollapsed}
              >
                <span class="nav-label__text">${group.label}</span>
                <span class="nav-label__chevron">${isGroupCollapsed ? "+" : "−"}</span>
              </button>
              <div class="nav-group__items">
                ${group.tabs.map((tab) => renderTab(state, tab))}
              </div>
            </div>
          `;
    })}
        <div class="nav-group nav-group--links">
          <div class="nav-label nav-label--static">
            <span class="nav-label__text">${t("nav.group.resources")}</span>
          </div>
          <div class="nav-group__items">
            <a
              class="nav-item nav-item--external"
              href="https://github.com/Acosmi/Claw-Acismi"
              target="_blank"
              rel="noreferrer"
              title="${t("topbar.docsTooltip")}"
            >
              <span class="nav-item__icon" aria-hidden="true">${icons.book}</span>
              <span class="nav-item__text">${t("topbar.docs")}</span>
            </a>
          </div>
        </div>
      </aside>
      <main class="content ${isChat ? "content--chat" : ""}">
        <section class="content-header">
          <div>
            ${state.tab === "usage" ? nothing : html`<div class="page-title">${titleForTab(state.tab)}</div>`}
            ${state.tab === "usage" ? nothing : html`<div class="page-sub">${subtitleForTab(state.tab)}</div>`}
          </div>
          <div class="page-meta">
            ${state.tab === "memory" && state.memoryPanel === "uhms" ? html`
              <div style="display:flex;gap:8px;align-items:center;">
                ${renderMemoryTypeCapsules(state.memoryStats)}
              </div>
            ` : nothing}
            ${state.tab === "memory" && state.memoryPanel === "media" ? html`
              <div style="display:flex;gap:8px;align-items:center;">
                <span class="pill ${(state.sttWizard as Record<string, unknown> | undefined)?.configured ? "success" : "warning"}" style="font-size:11px;">
                  🎙 ${(state.sttWizard as Record<string, unknown> | undefined)?.configured ? t("media.stt.configured") : t("media.stt.notConfigured")}
                </span>
                <span class="pill ${(state.docConvWizard as Record<string, unknown> | undefined)?.configured ? "success" : "warning"}" style="font-size:11px;">
                  📄 ${(state.docConvWizard as Record<string, unknown> | undefined)?.configured ? t("media.docconv.configured") : t("media.docconv.notConfigured")}
                </span>
              </div>
            ` : nothing}
            ${state.lastError ? html`<div class="pill danger">${state.lastError}</div>` : nothing}
            ${isChat ? renderChatControls(state) : nothing}
          </div>
        </section>

        ${state.tab === "overview"
      ? renderOverview({
        connected: state.connected,
        hello: state.hello,
        settings: state.settings,
        password: state.password,
        lastError: state.lastError,
        presenceCount,
        sessionsCount,
        cronEnabled: state.cronStatus?.enabled ?? null,
        cronNext,
        lastChannelsRefresh: state.channelsLastSuccess,
        onSettingsChange: (next) => state.applySettings(next),
        onPasswordChange: (next) => (state.password = next),
        onSessionKeyChange: (next) => {
          state.sessionKey = next;
          state.chatMessage = "";
          state.resetToolStream();
          state.applySettings({
            ...state.settings,
            sessionKey: next,
            lastActiveSessionKey: next,
          });
          void state.loadAssistantIdentity();
        },
        onConnect: () => state.connect(),
        onRefresh: () => state.loadOverview(),
        onStartWizard: () => void state.handleStartWizard(),
        onStartWizardV2: state.handleStartWizardV2 ? () => state.handleStartWizardV2!() : undefined,
      })
      : nothing
    }

        ${state.tab === "channels"
      ? renderChannels({
        connected: state.connected,
        loading: state.channelsLoading,
        snapshot: state.channelsSnapshot,
        lastError: state.channelsError,
        lastSuccessAt: state.channelsLastSuccess,
        whatsappMessage: state.whatsappLoginMessage,
        whatsappQrDataUrl: state.whatsappLoginQrDataUrl,
        whatsappConnected: state.whatsappLoginConnected,
        whatsappBusy: state.whatsappBusy,
        configSchema: state.configSchema,
        configSchemaLoading: state.configSchemaLoading,
        configForm: state.configForm,
        configUiHints: state.configUiHints,
        configSaving: state.configSaving,
        configFormDirty: state.configFormDirty,
        nostrProfileFormState: state.nostrProfileFormState,
        nostrProfileAccountId: state.nostrProfileAccountId,
        onRefresh: (probe) => loadChannels(state, probe),
        onWhatsAppStart: (force) => state.handleWhatsAppStart(force),
        onWhatsAppWait: () => state.handleWhatsAppWait(),
        onWhatsAppLogout: () => state.handleWhatsAppLogout(),
        onConfigPatch: (path, value) => updateConfigFormValue(state, path, value),
        onConfigSave: () => state.handleChannelConfigSave(),
        onConfigReload: () => state.handleChannelConfigReload(),
        onNostrProfileEdit: (accountId, profile) =>
          state.handleNostrProfileEdit(accountId, profile),
        onNostrProfileCancel: () => state.handleNostrProfileCancel(),
        onNostrProfileFieldChange: (field, value) =>
          state.handleNostrProfileFieldChange(field, value),
        onNostrProfileSave: () => state.handleNostrProfileSave(),
        onNostrProfileImport: () => state.handleNostrProfileImport(),
        onNostrProfileToggleAdvanced: () => state.handleNostrProfileToggleAdvanced(),
        onConfigureChannel: (channelId: string) => {
          openChannelWizard(state);
          // Skip platform select, go directly to credentials step
          (state as any).channelWizardState = {
            ...(state as any).channelWizardState,
            selectedPlatform: channelId,
            step: 1,
            skippedPlatformSelect: true,
          };
        },
        requestUpdate: () => { (state as unknown as { requestUpdate: () => void }).requestUpdate?.(); },
      })
      : nothing
    }
    ${state.tab === "channels" ? renderChannelWizard(state) : nothing}

        ${state.tab === "plugins"
      ? renderPlugins({
        panel: state.pluginsPanel,
        loading: state.pluginsLoading,
        plugins: state.pluginsList,
        error: state.pluginsError,
        editValues: state.pluginsEditValues,
        saving: state.pluginsSaving,
        toolsLoading: state.toolsLoading,
        tools: state.toolsList,
        toolsError: state.toolsError,
        browserConfig: state.browserToolConfig,
        browserLoading: state.browserToolLoading,
        browserSaving: state.browserToolSaving,
        browserError: state.browserToolError,
        browserEdits: state.browserToolEdits,
        onEditChange: (pluginId, key, value) => {
          state.pluginsEditValues = {
            ...state.pluginsEditValues,
            [pluginId]: { ...(state.pluginsEditValues[pluginId] ?? {}), [key]: value },
          };
        },
        onSave: (pluginId) => void savePluginConfig(state, pluginId, state.pluginsEditValues[pluginId] ?? {}),
        onGoToChannels: () => state.setTab("channels"),
        onPanelChange: (panel) => {
          state.pluginsPanel = panel;
          if (panel === "tools") {
            if (state.toolsList.length === 0) void loadTools(state);
            if (!state.browserToolConfig) void loadBrowserToolConfig(state);
          }
        },
        onBrowserEditChange: (key, value) => {
          state.browserToolEdits = { ...state.browserToolEdits, [key]: value };
        },
        onBrowserSave: () => void saveBrowserToolConfig(state),
      })
      : nothing
    }

        ${state.tab === "instances"
      ? renderInstances({
        loading: state.presenceLoading,
        entries: state.presenceEntries,
        lastError: state.presenceError,
        statusMessage: state.presenceStatus,
        onRefresh: () => loadPresence(state),
      })
      : nothing
    }

        ${state.tab === "usage"
      ? renderUsage({
        loading: state.usageLoading,
        error: state.usageError,
        startDate: state.usageStartDate,
        endDate: state.usageEndDate,
        sessions: state.usageResult?.sessions ?? [],
        sessionsLimitReached: (state.usageResult?.sessions?.length ?? 0) >= 1000,
        totals: state.usageResult?.totals ?? null,
        aggregates: state.usageResult?.aggregates ?? null,
        costDaily: state.usageCostSummary?.daily ?? [],
        selectedSessions: state.usageSelectedSessions,
        selectedDays: state.usageSelectedDays,
        selectedHours: state.usageSelectedHours,
        chartMode: state.usageChartMode,
        dailyChartMode: state.usageDailyChartMode,
        timeSeriesMode: state.usageTimeSeriesMode,
        timeSeriesBreakdownMode: state.usageTimeSeriesBreakdownMode,
        timeSeries: state.usageTimeSeries,
        timeSeriesLoading: state.usageTimeSeriesLoading,
        sessionLogs: state.usageSessionLogs,
        sessionLogsLoading: state.usageSessionLogsLoading,
        sessionLogsExpanded: state.usageSessionLogsExpanded,
        logFilterRoles: state.usageLogFilterRoles,
        logFilterTools: state.usageLogFilterTools,
        logFilterHasTools: state.usageLogFilterHasTools,
        logFilterQuery: state.usageLogFilterQuery,
        query: state.usageQuery,
        queryDraft: state.usageQueryDraft,
        sessionSort: state.usageSessionSort,
        sessionSortDir: state.usageSessionSortDir,
        recentSessions: state.usageRecentSessions,
        sessionsTab: state.usageSessionsTab,
        visibleColumns:
          state.usageVisibleColumns as import("./views/usage.ts").UsageColumnId[],
        timeZone: state.usageTimeZone,
        contextExpanded: state.usageContextExpanded,
        headerPinned: state.usageHeaderPinned,
        onStartDateChange: (date) => {
          state.usageStartDate = date;
          state.usageSelectedDays = [];
          state.usageSelectedHours = [];
          state.usageSelectedSessions = [];
          debouncedLoadUsage(state);
        },
        onEndDateChange: (date) => {
          state.usageEndDate = date;
          state.usageSelectedDays = [];
          state.usageSelectedHours = [];
          state.usageSelectedSessions = [];
          debouncedLoadUsage(state);
        },
        onRefresh: () => loadUsage(state),
        onTimeZoneChange: (zone) => {
          state.usageTimeZone = zone;
        },
        onToggleContextExpanded: () => {
          state.usageContextExpanded = !state.usageContextExpanded;
        },
        onToggleSessionLogsExpanded: () => {
          state.usageSessionLogsExpanded = !state.usageSessionLogsExpanded;
        },
        onLogFilterRolesChange: (next) => {
          state.usageLogFilterRoles = next;
        },
        onLogFilterToolsChange: (next) => {
          state.usageLogFilterTools = next;
        },
        onLogFilterHasToolsChange: (next) => {
          state.usageLogFilterHasTools = next;
        },
        onLogFilterQueryChange: (next) => {
          state.usageLogFilterQuery = next;
        },
        onLogFilterClear: () => {
          state.usageLogFilterRoles = [];
          state.usageLogFilterTools = [];
          state.usageLogFilterHasTools = false;
          state.usageLogFilterQuery = "";
        },
        onToggleHeaderPinned: () => {
          state.usageHeaderPinned = !state.usageHeaderPinned;
        },
        onSelectHour: (hour, shiftKey) => {
          if (shiftKey && state.usageSelectedHours.length > 0) {
            const allHours = Array.from({ length: 24 }, (_, i) => i);
            const lastSelected =
              state.usageSelectedHours[state.usageSelectedHours.length - 1];
            const lastIdx = allHours.indexOf(lastSelected);
            const thisIdx = allHours.indexOf(hour);
            if (lastIdx !== -1 && thisIdx !== -1) {
              const [start, end] =
                lastIdx < thisIdx ? [lastIdx, thisIdx] : [thisIdx, lastIdx];
              const range = allHours.slice(start, end + 1);
              state.usageSelectedHours = [
                ...new Set([...state.usageSelectedHours, ...range]),
              ];
            }
          } else {
            if (state.usageSelectedHours.includes(hour)) {
              state.usageSelectedHours = state.usageSelectedHours.filter((h) => h !== hour);
            } else {
              state.usageSelectedHours = [...state.usageSelectedHours, hour];
            }
          }
        },
        onQueryDraftChange: (query) => {
          state.usageQueryDraft = query;
          if (state.usageQueryDebounceTimer) {
            window.clearTimeout(state.usageQueryDebounceTimer);
          }
          state.usageQueryDebounceTimer = window.setTimeout(() => {
            state.usageQuery = state.usageQueryDraft;
            state.usageQueryDebounceTimer = null;
          }, 250);
        },
        onApplyQuery: () => {
          if (state.usageQueryDebounceTimer) {
            window.clearTimeout(state.usageQueryDebounceTimer);
            state.usageQueryDebounceTimer = null;
          }
          state.usageQuery = state.usageQueryDraft;
        },
        onClearQuery: () => {
          if (state.usageQueryDebounceTimer) {
            window.clearTimeout(state.usageQueryDebounceTimer);
            state.usageQueryDebounceTimer = null;
          }
          state.usageQueryDraft = "";
          state.usageQuery = "";
        },
        onSessionSortChange: (sort) => {
          state.usageSessionSort = sort;
        },
        onSessionSortDirChange: (dir) => {
          state.usageSessionSortDir = dir;
        },
        onSessionsTabChange: (tab) => {
          state.usageSessionsTab = tab;
        },
        onToggleColumn: (column) => {
          if (state.usageVisibleColumns.includes(column)) {
            state.usageVisibleColumns = state.usageVisibleColumns.filter(
              (entry) => entry !== column,
            );
          } else {
            state.usageVisibleColumns = [...state.usageVisibleColumns, column];
          }
        },
        onSelectSession: (key, shiftKey) => {
          state.usageTimeSeries = null;
          state.usageSessionLogs = null;
          state.usageRecentSessions = [
            key,
            ...state.usageRecentSessions.filter((entry) => entry !== key),
          ].slice(0, 8);

          if (shiftKey && state.usageSelectedSessions.length > 0) {
            // Shift-click: select range from last selected to this session
            // Sort sessions same way as displayed (by tokens or cost descending)
            const isTokenMode = state.usageChartMode === "tokens";
            const sortedSessions = [...(state.usageResult?.sessions ?? [])].toSorted(
              (a, b) => {
                const valA = isTokenMode
                  ? (a.usage?.totalTokens ?? 0)
                  : (a.usage?.totalCost ?? 0);
                const valB = isTokenMode
                  ? (b.usage?.totalTokens ?? 0)
                  : (b.usage?.totalCost ?? 0);
                return valB - valA;
              },
            );
            const allKeys = sortedSessions.map((s) => s.key);
            const lastSelected =
              state.usageSelectedSessions[state.usageSelectedSessions.length - 1];
            const lastIdx = allKeys.indexOf(lastSelected);
            const thisIdx = allKeys.indexOf(key);
            if (lastIdx !== -1 && thisIdx !== -1) {
              const [start, end] =
                lastIdx < thisIdx ? [lastIdx, thisIdx] : [thisIdx, lastIdx];
              const range = allKeys.slice(start, end + 1);
              const newSelection = [...new Set([...state.usageSelectedSessions, ...range])];
              state.usageSelectedSessions = newSelection;
            }
          } else {
            // Regular click: focus a single session (so details always open).
            // Click the focused session again to clear selection.
            if (
              state.usageSelectedSessions.length === 1 &&
              state.usageSelectedSessions[0] === key
            ) {
              state.usageSelectedSessions = [];
            } else {
              state.usageSelectedSessions = [key];
            }
          }

          // Load timeseries/logs only if exactly one session selected
          if (state.usageSelectedSessions.length === 1) {
            void loadSessionTimeSeries(state, state.usageSelectedSessions[0]);
            void loadSessionLogs(state, state.usageSelectedSessions[0]);
          }
        },
        onSelectDay: (day, shiftKey) => {
          if (shiftKey && state.usageSelectedDays.length > 0) {
            // Shift-click: select range from last selected to this day
            const allDays = (state.usageCostSummary?.daily ?? []).map((d) => d.date);
            const lastSelected =
              state.usageSelectedDays[state.usageSelectedDays.length - 1];
            const lastIdx = allDays.indexOf(lastSelected);
            const thisIdx = allDays.indexOf(day);
            if (lastIdx !== -1 && thisIdx !== -1) {
              const [start, end] =
                lastIdx < thisIdx ? [lastIdx, thisIdx] : [thisIdx, lastIdx];
              const range = allDays.slice(start, end + 1);
              // Merge with existing selection
              const newSelection = [...new Set([...state.usageSelectedDays, ...range])];
              state.usageSelectedDays = newSelection;
            }
          } else {
            // Regular click: toggle single day
            if (state.usageSelectedDays.includes(day)) {
              state.usageSelectedDays = state.usageSelectedDays.filter((d) => d !== day);
            } else {
              state.usageSelectedDays = [day];
            }
          }
        },
        onChartModeChange: (mode) => {
          state.usageChartMode = mode;
        },
        onDailyChartModeChange: (mode) => {
          state.usageDailyChartMode = mode;
        },
        onTimeSeriesModeChange: (mode) => {
          state.usageTimeSeriesMode = mode;
        },
        onTimeSeriesBreakdownChange: (mode) => {
          state.usageTimeSeriesBreakdownMode = mode;
        },
        onClearDays: () => {
          state.usageSelectedDays = [];
        },
        onClearHours: () => {
          state.usageSelectedHours = [];
        },
        onClearSessions: () => {
          state.usageSelectedSessions = [];
          state.usageTimeSeries = null;
          state.usageSessionLogs = null;
        },
        onClearFilters: () => {
          state.usageSelectedDays = [];
          state.usageSelectedHours = [];
          state.usageSelectedSessions = [];
          state.usageTimeSeries = null;
          state.usageSessionLogs = null;
        },
      })
      : nothing
    }

        ${state.tab === "cron"
      ? renderCron({
        basePath: state.basePath,
        loading: state.cronLoading,
        status: state.cronStatus,
        jobs: state.cronJobs,
        error: state.cronError,
        busy: state.cronBusy,
        form: state.cronForm,
        channels: state.channelsSnapshot?.channelMeta?.length
          ? state.channelsSnapshot.channelMeta.map((entry) => entry.id)
          : (state.channelsSnapshot?.channelOrder ?? []),
        channelLabels: state.channelsSnapshot?.channelLabels ?? {},
        channelMeta: state.channelsSnapshot?.channelMeta ?? [],
        runsJobId: state.cronRunsJobId,
        runs: state.cronRuns,
        onFormChange: (patch) => (state.cronForm = { ...state.cronForm, ...patch }),
        onRefresh: () => state.loadCron(),
        onAdd: () => addCronJob(state),
        onToggle: (job, enabled) => toggleCronJob(state, job, enabled),
        onRun: (job) => runCronJob(state, job),
        onRemove: (job) => removeCronJob(state, job),
        onLoadRuns: (jobId) => loadCronRuns(state, jobId),
      })
      : nothing
    }

        ${state.tab === "agents"
      ? renderAgents({
        loading: state.agentsLoading,
        error: state.agentsError,
        agentsList: state.agentsList,
        selectedAgentId: resolvedAgentId,
        activePanel: state.agentsPanel,
        configForm: configValue,
        configLoading: state.configLoading,
        configSaving: state.configSaving,
        configDirty: state.configFormDirty,
        channelsLoading: state.channelsLoading,
        channelsError: state.channelsError,
        channelsSnapshot: state.channelsSnapshot,
        channelsLastSuccess: state.channelsLastSuccess,
        cronLoading: state.cronLoading,
        cronStatus: state.cronStatus,
        cronJobs: state.cronJobs,
        cronError: state.cronError,
        agentFilesLoading: state.agentFilesLoading,
        agentFilesError: state.agentFilesError,
        agentFilesList: state.agentFilesList,
        agentFileActive: state.agentFileActive,
        agentFileContents: state.agentFileContents,
        agentFileDrafts: state.agentFileDrafts,
        agentFileSaving: state.agentFileSaving,
        agentIdentityLoading: state.agentIdentityLoading,
        agentIdentityError: state.agentIdentityError,
        agentIdentityById: state.agentIdentityById,
        agentSkillsLoading: state.agentSkillsLoading,
        agentSkillsReport: state.agentSkillsReport,
        agentSkillsError: state.agentSkillsError,
        agentSkillsAgentId: state.agentSkillsAgentId,
        skillsFilter: state.skillsFilter,
        onRefresh: async () => {
          await loadAgents(state);
          const agentIds = state.agentsList?.agents?.map((entry) => entry.id) ?? [];
          if (agentIds.length > 0) {
            void loadAgentIdentities(state, agentIds);
          }
        },
        onSelectAgent: (agentId) => {
          if (state.agentsSelectedId === agentId) {
            return;
          }
          state.agentsSelectedId = agentId;
          state.agentFilesList = null;
          state.agentFilesError = null;
          state.agentFilesLoading = false;
          state.agentFileActive = null;
          state.agentFileContents = {};
          state.agentFileDrafts = {};
          state.agentSkillsReport = null;
          state.agentSkillsError = null;
          state.agentSkillsAgentId = null;
          void loadAgentIdentity(state, agentId);
          if (state.agentsPanel === "files") {
            void loadAgentFiles(state, agentId);
          }
          if (state.agentsPanel === "skills") {
            void loadAgentSkills(state, agentId);
          }
        },
        onSelectPanel: (panel) => {
          state.agentsPanel = panel;
          if (panel === "files" && resolvedAgentId) {
            if (state.agentFilesList?.agentId !== resolvedAgentId) {
              state.agentFilesList = null;
              state.agentFilesError = null;
              state.agentFileActive = null;
              state.agentFileContents = {};
              state.agentFileDrafts = {};
              void loadAgentFiles(state, resolvedAgentId);
            }
          }
          if (panel === "skills") {
            if (resolvedAgentId) {
              void loadAgentSkills(state, resolvedAgentId);
            }
          }
          if (panel === "channels") {
            void loadChannels(state, false);
          }
          if (panel === "cron") {
            void state.loadCron();
          }
        },
        onLoadFiles: (agentId) => loadAgentFiles(state, agentId),
        onSelectFile: (name) => {
          state.agentFileActive = name;
          if (!resolvedAgentId) {
            return;
          }
          void loadAgentFileContent(state, resolvedAgentId, name);
        },
        onFileDraftChange: (name, content) => {
          state.agentFileDrafts = { ...state.agentFileDrafts, [name]: content };
        },
        onFileReset: (name) => {
          const base = state.agentFileContents[name] ?? "";
          state.agentFileDrafts = { ...state.agentFileDrafts, [name]: base };
        },
        onFileSave: (name) => {
          if (!resolvedAgentId) {
            return;
          }
          const content =
            state.agentFileDrafts[name] ?? state.agentFileContents[name] ?? "";
          void saveAgentFile(state, resolvedAgentId, name, content);
        },
        onToolsProfileChange: (agentId, profile, clearAllow) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          const basePath = ["agents", "list", index, "tools"];
          if (profile) {
            updateConfigFormValue(state, [...basePath, "profile"], profile);
          } else {
            removeConfigFormValue(state, [...basePath, "profile"]);
          }
          if (clearAllow) {
            removeConfigFormValue(state, [...basePath, "allow"]);
          }
        },
        onToolsOverridesChange: (agentId, alsoAllow, deny) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          const basePath = ["agents", "list", index, "tools"];
          if (alsoAllow.length > 0) {
            updateConfigFormValue(state, [...basePath, "alsoAllow"], alsoAllow);
          } else {
            removeConfigFormValue(state, [...basePath, "alsoAllow"]);
          }
          if (deny.length > 0) {
            updateConfigFormValue(state, [...basePath, "deny"], deny);
          } else {
            removeConfigFormValue(state, [...basePath, "deny"]);
          }
        },
        onConfigReload: () => loadConfig(state),
        onConfigSave: () => saveConfig(state),
        onChannelsRefresh: () => loadChannels(state, false),
        onCronRefresh: () => state.loadCron(),
        onSkillsFilterChange: (next) => (state.skillsFilter = next),
        onSkillsRefresh: () => {
          if (resolvedAgentId) {
            void loadAgentSkills(state, resolvedAgentId);
          }
        },
        onAgentSkillToggle: (agentId, skillName, enabled) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          const entry = list[index] as { skills?: unknown };
          const normalizedSkill = skillName.trim();
          if (!normalizedSkill) {
            return;
          }
          const allSkills =
            state.agentSkillsReport?.skills?.map((skill) => skill.name).filter(Boolean) ??
            [];
          const existing = Array.isArray(entry.skills)
            ? entry.skills.map((name) => String(name).trim()).filter(Boolean)
            : undefined;
          const base = existing ?? allSkills;
          const next = new Set(base);
          if (enabled) {
            next.add(normalizedSkill);
          } else {
            next.delete(normalizedSkill);
          }
          updateConfigFormValue(state, ["agents", "list", index, "skills"], [...next]);
        },
        onAgentSkillsClear: (agentId) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          removeConfigFormValue(state, ["agents", "list", index, "skills"]);
        },
        onAgentSkillsDisableAll: (agentId) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          updateConfigFormValue(state, ["agents", "list", index, "skills"], []);
        },
        onModelChange: (agentId, modelId) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          const basePath = ["agents", "list", index, "model"];
          if (!modelId) {
            removeConfigFormValue(state, basePath);
            return;
          }
          const entry = list[index] as { model?: unknown };
          const existing = entry?.model;
          if (existing && typeof existing === "object" && !Array.isArray(existing)) {
            const fallbacks = (existing as { fallbacks?: unknown }).fallbacks;
            const next = {
              primary: modelId,
              ...(Array.isArray(fallbacks) ? { fallbacks } : {}),
            };
            updateConfigFormValue(state, basePath, next);
          } else {
            updateConfigFormValue(state, basePath, modelId);
          }
        },
        onModelFallbacksChange: (agentId, fallbacks) => {
          if (!configValue) {
            return;
          }
          const list = (configValue as { agents?: { list?: unknown[] } }).agents?.list;
          if (!Array.isArray(list)) {
            return;
          }
          const index = list.findIndex(
            (entry) =>
              entry &&
              typeof entry === "object" &&
              "id" in entry &&
              (entry as { id?: string }).id === agentId,
          );
          if (index < 0) {
            return;
          }
          const basePath = ["agents", "list", index, "model"];
          const entry = list[index] as { model?: unknown };
          const normalized = fallbacks.map((name) => name.trim()).filter(Boolean);
          const existing = entry.model;
          const resolvePrimary = () => {
            if (typeof existing === "string") {
              return existing.trim() || null;
            }
            if (existing && typeof existing === "object" && !Array.isArray(existing)) {
              const primary = (existing as { primary?: unknown }).primary;
              if (typeof primary === "string") {
                const trimmed = primary.trim();
                return trimmed || null;
              }
            }
            return null;
          };
          const primary = resolvePrimary();
          if (normalized.length === 0) {
            if (primary) {
              updateConfigFormValue(state, basePath, primary);
            } else {
              removeConfigFormValue(state, basePath);
            }
            return;
          }
          const next = primary
            ? { primary, fallbacks: normalized }
            : { fallbacks: normalized };
          updateConfigFormValue(state, basePath, next);
        },
      })
      : nothing
    }

        ${state.tab === "skills"
      ? renderSkills({
        loading: state.skillsLoading,
        report: state.skillsReport,
        error: state.skillsError,
        filter: state.skillsFilter,
        edits: state.skillEdits,
        messages: state.skillMessages,
        busyKey: state.skillsBusyKey,
        distributeLoading: state.distributeLoading,
        distributeResult: state.distributeResult,
        onFilterChange: (next) => (state.skillsFilter = next),
        onRefresh: () => loadSkills(state, { clearMessages: true }),
        onToggle: (key, enabled) => updateSkillEnabled(state, key, enabled),
        onEdit: (key, value) => updateSkillEdit(state, key, value),
        onSaveKey: (key) => saveSkillApiKey(state, key),
        onInstall: (skillKey, name, installId) =>
          installSkill(state, skillKey, name, installId),
        onDistribute: () => distributeSkills(state),
      })
      : nothing
    }

        ${state.tab === "subagents"
      ? renderSubAgents({
        loading: state.subagentsLoading,
        agents: state.subagentsList ?? [],
        error: state.subagentsError ?? null,
        busyKey: state.subagentsBusyKey ?? null,
        onToggle: (agentId, enabled) => {
          import("./controllers/subagents.ts").then((m) =>
            m.toggleSubAgent(state as any, agentId, enabled),
          );
        },
        onSetInterval: (agentId, ms) => {
          import("./controllers/subagents.ts").then((m) =>
            m.setSubAgentInterval(state as any, agentId, ms),
          );
        },
        onSetGoal: (agentId, goal) => {
          import("./controllers/subagents.ts").then((m) =>
            m.setSubAgentGoal(state as any, agentId, goal),
          );
        },
        onSetModel: (agentId, model) => {
          import("./controllers/subagents.ts").then((m) =>
            m.setSubAgentModel(state as any, agentId, model),
          );
        },
        onRefresh: () => {
          import("./controllers/subagents.ts").then((m) =>
            m.loadSubAgents(state as any),
          );
        },
        onStartOpenCoderWizard: () => {
          import("./views/wizard.ts").then((m) =>
            m.startOpenCoderWizard(state as any),
          );
        },
      })
      : nothing
    }

        ${state.tab === "media"
      ? renderMediaDashboard(state)
      : nothing
    }

        ${state.tab === "tasks"
      ? renderTaskKanban({
        kanbanState: state.taskKanbanState,
        onPrune: () => {
          import("./controllers/task-kanban.ts").then((m) => {
            state.taskKanbanState = m.pruneCompletedTasks(state.taskKanbanState);
            state.requestUpdate();
          });
        },
      })
      : nothing
    }

        ${state.tab === "nodes"
      ? renderNodes({
        loading: state.nodesLoading,
        nodes: state.nodes,
        devicesLoading: state.devicesLoading,
        devicesError: state.devicesError,
        devicesList: state.devicesList,
        configForm:
          state.configForm ??
          (state.configSnapshot?.config as Record<string, unknown> | null),
        configLoading: state.configLoading,
        configSaving: state.configSaving,
        configDirty: state.configFormDirty,
        configFormMode: state.configFormMode,
        execApprovalsLoading: state.execApprovalsLoading,
        execApprovalsSaving: state.execApprovalsSaving,
        execApprovalsDirty: state.execApprovalsDirty,
        execApprovalsSnapshot: state.execApprovalsSnapshot,
        execApprovalsForm: state.execApprovalsForm,
        execApprovalsSelectedAgent: state.execApprovalsSelectedAgent,
        execApprovalsTarget: state.execApprovalsTarget,
        execApprovalsTargetNodeId: state.execApprovalsTargetNodeId,
        onRefresh: () => loadNodes(state),
        onDevicesRefresh: () => loadDevices(state),
        onDeviceApprove: (requestId) => approveDevicePairing(state, requestId),
        onDeviceReject: (requestId) => rejectDevicePairing(state, requestId),
        onDeviceRotate: (deviceId, role, scopes) =>
          rotateDeviceToken(state, { deviceId, role, scopes }),
        onDeviceRevoke: (deviceId, role) => revokeDeviceToken(state, { deviceId, role }),
        onLoadConfig: () => loadConfig(state),
        onLoadExecApprovals: () => {
          const target =
            state.execApprovalsTarget === "node" && state.execApprovalsTargetNodeId
              ? { kind: "node" as const, nodeId: state.execApprovalsTargetNodeId }
              : { kind: "gateway" as const };
          return loadExecApprovals(state, target);
        },
        onBindDefault: (nodeId) => {
          if (nodeId) {
            updateConfigFormValue(state, ["tools", "exec", "node"], nodeId);
          } else {
            removeConfigFormValue(state, ["tools", "exec", "node"]);
          }
        },
        onBindAgent: (agentIndex, nodeId) => {
          const basePath = ["agents", "list", agentIndex, "tools", "exec", "node"];
          if (nodeId) {
            updateConfigFormValue(state, basePath, nodeId);
          } else {
            removeConfigFormValue(state, basePath);
          }
        },
        onSaveBindings: () => saveConfig(state),
        onExecApprovalsTargetChange: (kind, nodeId) => {
          state.execApprovalsTarget = kind;
          state.execApprovalsTargetNodeId = nodeId;
          state.execApprovalsSnapshot = null;
          state.execApprovalsForm = null;
          state.execApprovalsDirty = false;
          state.execApprovalsSelectedAgent = null;
        },
        onExecApprovalsSelectAgent: (agentId) => {
          state.execApprovalsSelectedAgent = agentId;
        },
        onExecApprovalsPatch: (path, value) =>
          updateExecApprovalsFormValue(state, path, value),
        onExecApprovalsRemove: (path) => removeExecApprovalsFormValue(state, path),
        onSaveExecApprovals: () => {
          const target =
            state.execApprovalsTarget === "node" && state.execApprovalsTargetNodeId
              ? { kind: "node" as const, nodeId: state.execApprovalsTargetNodeId }
              : { kind: "gateway" as const };
          return saveExecApprovals(state, target);
        },
      })
      : nothing
    }

        ${state.tab === "memory"
      ? html`
        <div class="agent-tabs" style="margin-bottom:16px;">
          <button class="agent-tab ${state.memoryPanel === "sessions" ? "active" : ""}"
            @click=${() => {
          state.memoryPanel = "sessions";
          loadSessions(state);
        }}>${t("memory.tab.sessions")}</button>
          <button class="agent-tab ${state.memoryPanel === "uhms" ? "active" : ""}"
            @click=${() => {
          state.memoryPanel = "uhms";
          loadMemoryStatus(state);
          loadMemoryList(state);
          loadMemoryStats(state);
        }}>${t("memory.tab.uhms")}</button>
          <button class="agent-tab ${state.memoryPanel === "media" ? "active" : ""}"
            @click=${() => {
          state.memoryPanel = "media";
          loadMediaConfig(state);
        }}>${t("media.tab.title")}</button>
        </div>
        ${state.memoryPanel === "media"
          ? renderMediaConfig(state)
          : state.memoryPanel === "sessions"
            ? renderSessions({
              loading: state.sessionsLoading,
              result: state.sessionsResult,
              error: state.sessionsError,
              activeMinutes: state.sessionsFilterActive,
              limit: state.sessionsFilterLimit,
              includeGlobal: state.sessionsIncludeGlobal,
              includeUnknown: state.sessionsIncludeUnknown,
              basePath: state.basePath,
              onFiltersChange: (next) => {
                state.sessionsFilterActive = next.activeMinutes;
                state.sessionsFilterLimit = next.limit;
                state.sessionsIncludeGlobal = next.includeGlobal;
                state.sessionsIncludeUnknown = next.includeUnknown;
              },
              onRefresh: () => loadSessions(state),
              onPatch: (key, patch) => patchSession(state, key, patch),
              onDelete: (key) => deleteSession(state, key),
            })
            : renderMemory({
              loading: state.memoryLoading,
              status: state.memoryStatus,
              list: state.memoryList,
              total: state.memoryTotal,
              error: state.memoryError,
              detail: state.memoryDetail,
              detailLevel: state.memoryDetailLevel,
              importing: state.memoryImporting,
              importResult: state.memoryImportResult,
              page: state.memoryPage,
              pageSize: state.memoryPageSize,
              filterType: state.memoryFilterType,
              filterCategory: state.memoryFilterCategory,
              onRefresh: () => loadMemoryList(state),
              onLoadStatus: () => loadMemoryStatus(state),
              onPageChange: (page) => loadMemoryList(state, { page }),
              onFilterType: (type) => {
                state.memoryPage = 0;
                loadMemoryList(state, { page: 0, type });
              },
              onFilterCategory: (category) => {
                state.memoryPage = 0;
                loadMemoryList(state, { page: 0, category });
              },
              onSelectMemory: (id, level) => loadMemoryDetail(state, id, level),
              onDeleteMemory: (id) => deleteMemory(state, id),
              onImportSkills: () => importSkills(state),
              onDetailLevel: (level) => {
                if (state.memoryDetail) {
                  loadMemoryDetail(state, state.memoryDetail.id, level);
                }
              },
              onCloseDetail: () => {
                state.memoryDetail = null;
              },
              llmConfig: state.memoryLLMConfig ?? null,
              llmConfigOpen: state.memoryLLMConfigOpen ?? false,
              onLLMConfigToggle: () => {
                state.memoryLLMConfigOpen = !state.memoryLLMConfigOpen;
                if (state.memoryLLMConfigOpen) {
                  resetLLMDraft(); // re-sync draft from server config on open
                  if (!state.memoryLLMConfig) {
                    loadMemoryLLMConfig(state);
                  }
                }
              },
              onLLMConfigSave: (provider, model, baseUrl, apiKey) => {
                return saveMemoryLLMConfig(state, { provider, model, baseUrl, apiKey });
              },
              stats: state.memoryStats ?? null,
              searchQuery: state.memorySearchQuery,
              searchResults: state.memorySearchResults ?? null,
              searching: state.memorySearching,
              onSearch: (query) => searchMemories(state, query),
              onClearSearch: () => clearMemorySearch(state),
              onLoadStats: () => loadMemoryStats(state),
            })
        }
      `
      : nothing
    }

        ${state.tab === "chat"
      ? html`
        ${renderChat({
        sessionKey: state.sessionKey,
        onSessionKeyChange: (next) => {
          state.sessionKey = next;
          state.chatMessage = "";
          state.chatAttachments = [];
          state.chatStream = null;
          state.chatStreamStartedAt = null;
          state.chatRunId = null;
          state.chatQueue = [];
          state.resetToolStream();
          state.resetChatScroll();
          state.applySettings({
            ...state.settings,
            sessionKey: next,
            lastActiveSessionKey: next,
          });
          void state.loadAssistantIdentity();
          void loadChatHistory(state);
          void refreshChatAvatar(state);
        },
        thinkingLevel: state.chatThinkingLevel,
        showThinking,
        loading: state.chatLoading,
        sending: state.chatSending,
        compactionStatus: state.compactionStatus,
        assistantAvatarUrl: chatAvatarUrl,
        messages: state.chatMessages,
        toolMessages: state.chatToolMessages,
        stream: state.chatStream,
        streamStartedAt: state.chatStreamStartedAt,
        draft: state.chatMessage,
        queue: state.chatQueue,
        connected: state.connected,
        canSend: state.connected,
        disabledReason: chatDisabledReason,
        error: state.lastError,
        sessions: state.sessionsResult,
        focusMode: chatFocus,
        onRefresh: () => {
          state.resetToolStream();
          return Promise.all([loadChatHistory(state), refreshChatAvatar(state)]);
        },
        onToggleFocusMode: () => {
          if (state.onboarding) {
            return;
          }
          state.applySettings({
            ...state.settings,
            chatFocusMode: !state.settings.chatFocusMode,
          });
        },
        onChatScroll: (event) => state.handleChatScroll(event),
        onDraftChange: (next) => (state.chatMessage = next),
        attachments: state.chatAttachments,
        onAttachmentsChange: (next) => (state.chatAttachments = next),
        voiceRecording: state.voiceRecording,
        voiceRecordingDuration: state.voiceRecordingDuration,
        voiceSupported: state.voiceSupported,
        onVoiceStart: () => state.handleVoiceStart(),
        onVoiceStop: () => state.handleVoiceStop(),
        onSend: () => state.handleSendChat(),
        canAbort: Boolean(state.chatRunId),
        onAbort: () => void state.handleAbortChat(),
        onQueueRemove: (id) => state.removeQueuedMessage(id),
        onNewSession: () => state.handleSendChat("/new", { restoreDraft: true }),
        showNewMessages: state.chatNewMessagesBelow && !state.chatManualRefreshInFlight,
        onScrollToBottom: () => state.scrollToBottom(),
        // Sidebar props for tool output viewing
        sidebarOpen: state.sidebarOpen,
        sidebarContent: state.sidebarContent,
        sidebarError: state.sidebarError,
        splitRatio: state.splitRatio,
        onOpenSidebar: (content: string) => state.handleOpenSidebar(content),
        onCloseSidebar: () => state.handleCloseSidebar(),
        onSplitRatioChange: (ratio: number) => state.handleSplitRatioChange(ratio),
        assistantName: state.assistantName,
        assistantAvatar: state.assistantAvatar,
        requestUpdate: () => { (state as unknown as { requestUpdate: () => void }).requestUpdate?.(); },
        permissionPopupCallbacks: {
          // 自动提权已在后端创建 pending 请求（OnPermissionDenied 回调），
          // 这里只需直接 resolve 该请求即可。不再重复 request。
          onAllowOnce: async () => {
            try {
              await state.client?.request("security.escalation.resolve", {
                approve: true,
                ttlMinutes: 1,
              });
            } catch { /* pending 可能已超时 */ }
          },
          onAllowSession: async () => {
            try {
              await state.client?.request("security.escalation.resolve", {
                approve: true,
                ttlMinutes: 60,
              });
            } catch { /* pending 可能已超时 */ }
          },
          onAllowPermanent: async () => {
            await updateSecurityLevel(state, "full");
            // 同时批准当前 pending 请求
            try {
              await state.client?.request("security.escalation.resolve", {
                approve: true,
                ttlMinutes: 0, // 永久变更，TTL 无意义
              });
            } catch { /* ignore */ }
          },
          onDeny: () => {
            hidePermissionPopup();
            // 明确拒绝当前 pending 请求，让 WaitForApproval 立即退出
            void state.client?.request("security.escalation.resolve", {
              approve: false,
            }).catch(() => {/* ignore */ });
          },
        },
      })}
      `
      : nothing
    }

        ${state.tab === "security"
      ? renderSecurity({
        loading: state.securityLoading,
        error: state.securityError,
        currentLevel: state.securityLevel,
        levels: state.securityLevels,
        confirmOpen: state.securityConfirmOpen,
        pendingLevel: state.securityPendingLevel,
        confirmText: state.securityConfirmText,
        onLevelChange: (level) => updateSecurityLevel(state, level),
        onConfirmOpen: (level) => {
          state.securityPendingLevel = level;
          state.securityConfirmOpen = true;
          state.securityConfirmText = "";
        },
        onConfirmCancel: () => {
          state.securityConfirmOpen = false;
          state.securityPendingLevel = null;
          state.securityConfirmText = "";
        },
        onConfirmTextChange: (text) => {
          state.securityConfirmText = text;
        },
        onConfirmSubmit: () => {
          if (state.securityPendingLevel) {
            updateSecurityLevel(state, state.securityPendingLevel);
          }
        },
        onRefresh: () => loadSecurity(state),
      })
      : nothing
    }

        ${state.tab === "config"
      ? renderConfig({
        raw: state.configRaw,
        originalRaw: state.configRawOriginal,
        valid: state.configValid,
        issues: state.configIssues,
        loading: state.configLoading,
        saving: state.configSaving,
        applying: state.configApplying,
        updating: state.updateRunning,
        connected: state.connected,
        schema: state.configSchema,
        schemaLoading: state.configSchemaLoading,
        uiHints: state.configUiHints,
        formMode: state.configFormMode,
        formValue: state.configForm,
        originalValue: state.configFormOriginal,
        searchQuery: state.configSearchQuery,
        activeSection: state.configActiveSection,
        activeSubsection: state.configActiveSubsection,
        onRawChange: (next) => {
          state.configRaw = next;
        },
        onFormModeChange: (mode) => (state.configFormMode = mode),
        onFormPatch: (path, value) => updateConfigFormValue(state, path, value),
        onSearchChange: (query) => (state.configSearchQuery = query),
        onSectionChange: (section) => {
          state.configActiveSection = section;
          state.configActiveSubsection = null;
        },
        onSubsectionChange: (section) => (state.configActiveSubsection = section),
        onReload: () => loadConfig(state),
        onSave: () => saveConfig(state),
        onApply: () => applyConfig(state),
        onUpdate: () => runUpdate(state),
      })
      : nothing
    }

        ${state.tab === "debug"
      ? renderDebug({
        loading: state.debugLoading,
        status: state.debugStatus,
        health: state.debugHealth,
        models: state.debugModels,
        heartbeat: state.debugHeartbeat,
        eventLog: state.eventLog,
        callMethod: state.debugCallMethod,
        callParams: state.debugCallParams,
        callResult: state.debugCallResult,
        callError: state.debugCallError,
        onCallMethodChange: (next) => (state.debugCallMethod = next),
        onCallParamsChange: (next) => (state.debugCallParams = next),
        onRefresh: () => loadDebug(state),
        onCall: () => callDebugMethod(state),
      })
      : nothing
    }

        ${state.tab === "logs"
      ? renderLogs({
        loading: state.logsLoading,
        error: state.logsError,
        file: state.logsFile,
        entries: state.logsEntries,
        filterText: state.logsFilterText,
        levelFilters: state.logsLevelFilters,
        autoFollow: state.logsAutoFollow,
        truncated: state.logsTruncated,
        onFilterTextChange: (next) => (state.logsFilterText = next),
        onLevelToggle: (level, enabled) => {
          state.logsLevelFilters = { ...state.logsLevelFilters, [level]: enabled };
        },
        onToggleAutoFollow: (next) => (state.logsAutoFollow = next),
        onRefresh: () => loadLogs(state, { reset: true }),
        onExport: (lines, label) => state.exportLogs(lines, label),
        onScroll: (event) => state.handleLogsScroll(event),
      })
      : nothing
    }
      </main>
      ${renderExecApprovalPrompt(state)}
      ${renderEscalationPopup({
      visible: state.escalationState.popupVisible,
      request: state.escalationState.request,
      activeGrant: state.escalationState.activeGrant,
      loading: state.escalationState.loading,
      selectedTtl: state.escalationSelectedTtl,
      onApprove: async (ttlMinutes) => {
        state.escalationState = { ...state.escalationState, loading: true };
        try {
          await resolveEscalation(state.client!, true, ttlMinutes);
        } catch { /* pending 可能已超时 */ }
        state.escalationState = { ...state.escalationState, loading: false, popupVisible: false, request: null };
      },
      onDeny: async () => {
        state.escalationState = { ...state.escalationState, loading: true };
        try {
          await resolveEscalation(state.client!, false, 0);
        } catch { /* ignore */ }
        state.escalationState = { ...state.escalationState, loading: false, popupVisible: false, request: null };
      },
      onRevoke: async () => {
        state.escalationState = { ...state.escalationState, loading: true };
        try {
          await revokeEscalation(state.client!);
        } catch { /* ignore */ }
        state.escalationState = { ...state.escalationState, loading: false, activeGrant: null };
      },
      onTtlChange: (ttl) => { state.escalationSelectedTtl = ttl; },
      onClose: () => { state.escalationState = { ...state.escalationState, popupVisible: false }; },
    })}
      ${"" /* TODO(coder-terminal): renderCoderConfirmPrompt(state) 已禁用，待终端式 UI */}
      ${renderPlanConfirmPopup({
      queue: state.planConfirmQueue ?? [],
      onApprove: (id) => state.handlePlanConfirmDecision(id, "approve"),
      onReject: (id) => state.handlePlanConfirmDecision(id, "reject"),
      onEdit: (id, editedPlan) => state.handlePlanConfirmDecision(id, "edit", editedPlan),
    })}
      ${renderResultReviewPopup({
      queue: state.resultReviewQueue ?? [],
      onApprove: (id) => state.handleResultReviewDecision(id, "approve"),
      onReject: (id, feedback) => state.handleResultReviewDecision(id, "reject", feedback),
    })}
      ${renderSubagentHelpPopup({
      queue: state.subagentHelpQueue ?? [],
      onRespond: (id, response) => state.handleSubagentHelpRespond(id, response),
    })}
      ${renderGatewayUrlConfirmation(state)}
        ${renderWizard(state)}
        ${renderWizardV2(state)}
    </div>
  `;
}
