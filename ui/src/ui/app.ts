import { LitElement } from "lit";
import { customElement, state } from "lit/decorators.js";
import type { EventLogEntry } from "./app-events.ts";
import type { AppViewState } from "./app-view-state.ts";
import type { DevicePairingList } from "./controllers/devices.ts";
import type { CoderConfirmRequest } from "./controllers/coder-confirmation.ts";
import { removeCoderConfirm } from "./controllers/coder-confirmation.ts";
import type { PlanConfirmRequest } from "./controllers/plan-confirmation.ts";
import { removePlanConfirm } from "./controllers/plan-confirmation.ts";
import type { ResultReviewRequest } from "./controllers/result-review.ts";
import { removeResultReview } from "./controllers/result-review.ts";
import type { SubagentHelpRequest } from "./controllers/subagent-help.ts";
import { removeSubagentHelp } from "./controllers/subagent-help.ts";
import { createEscalationState, type EscalationState } from "./controllers/escalation.ts";
import type { ExecApprovalRequest } from "./controllers/exec-approval.ts";
import type { ExecApprovalsFile, ExecApprovalsSnapshot } from "./controllers/exec-approvals.ts";
import type { SkillMessage } from "./controllers/skills.ts";
import type { GatewayBrowserClient, GatewayHelloOk } from "./gateway.ts";
import type { Tab } from "./navigation.ts";
import type { ResolvedTheme, ThemeMode } from "./theme.ts";
import type {
  AgentsListResult,
  AgentsFilesListResult,
  AgentIdentityResult,
  ConfigSnapshot,
  ConfigUiHints,
  CronJob,
  CronRunLogEntry,
  CronStatus,
  HealthSnapshot,
  LogEntry,
  LogLevel,
  PresenceEntry,
  ChannelsStatusSnapshot,
  SessionsListResult,
  SkillStatusReport,
  StatusSummary,
  NostrProfile,
} from "./types.ts";
import type { NostrProfileFormState } from "./views/channels.nostr-profile-form.ts";
import {
  handleChannelConfigReload as handleChannelConfigReloadInternal,
  handleChannelConfigSave as handleChannelConfigSaveInternal,
  handleNostrProfileCancel as handleNostrProfileCancelInternal,
  handleNostrProfileEdit as handleNostrProfileEditInternal,
  handleNostrProfileFieldChange as handleNostrProfileFieldChangeInternal,
  handleNostrProfileImport as handleNostrProfileImportInternal,
  handleNostrProfileSave as handleNostrProfileSaveInternal,
  handleNostrProfileToggleAdvanced as handleNostrProfileToggleAdvancedInternal,
  handleWhatsAppLogout as handleWhatsAppLogoutInternal,
  handleWhatsAppStart as handleWhatsAppStartInternal,
  handleWhatsAppWait as handleWhatsAppWaitInternal,
} from "./app-channels.ts";
import {
  handleAbortChat as handleAbortChatInternal,
  handleSendChat as handleSendChatInternal,
  removeQueuedMessage as removeQueuedMessageInternal,
} from "./app-chat.ts";
import { DEFAULT_CRON_FORM, DEFAULT_LOG_LEVEL_FILTERS } from "./app-defaults.ts";
import {
  isVoiceRecordingSupported,
  VoiceRecorder,
  blobToDataUrl,
} from "./controllers/voice-recorder.ts";
import { connectGateway as connectGatewayInternal } from "./app-gateway.ts";
import {
  handleConnected,
  handleDisconnected,
  handleFirstUpdated,
  handleUpdated,
} from "./app-lifecycle.ts";
import { renderApp } from "./app-render.ts";
import {
  exportLogs as exportLogsInternal,
  handleChatScroll as handleChatScrollInternal,
  handleLogsScroll as handleLogsScrollInternal,
  resetChatScroll as resetChatScrollInternal,
  scheduleChatScroll as scheduleChatScrollInternal,
} from "./app-scroll.ts";
import {
  applySettings as applySettingsInternal,
  loadCron as loadCronInternal,
  loadOverview as loadOverviewInternal,
  setTab as setTabInternal,
  setTheme as setThemeInternal,
  onPopState as onPopStateInternal,
} from "./app-settings.ts";
import {
  resetToolStream as resetToolStreamInternal,
  type ToolStreamEntry,
  type CompactionStatus,
} from "./app-tool-stream.ts";
import { resolveInjectedAssistantIdentity } from "./assistant-identity.ts";
import { loadAssistantIdentity as loadAssistantIdentityInternal } from "./controllers/assistant-identity.ts";
import { loadSettings, type UiSettings } from "./storage.ts";
import { initLocale, onLocaleChange } from "./i18n.ts";
import { type ChatAttachment, type ChatQueueItem, type CronFormState } from "./ui-types.ts";
import {
  WIZARD_INITIAL_STATE,
  startWizard as startWizardInternal,
  cancelWizard as cancelWizardInternal,
  type WizardState,
} from "./views/wizard.ts";
import { startWizardV2 as startWizardV2Internal } from "./views/wizard-v2.ts";

declare global {
  interface Window {
    __OPENACOSMI_CONTROL_UI_BASE_PATH__?: string;
  }
}

const injectedAssistantIdentity = resolveInjectedAssistantIdentity();

function resolveOnboardingMode(): boolean {
  if (!window.location.search) {
    return false;
  }
  const params = new URLSearchParams(window.location.search);
  const raw = params.get("onboarding");
  if (!raw) {
    return false;
  }
  const normalized = raw.trim().toLowerCase();
  return normalized === "1" || normalized === "true" || normalized === "yes" || normalized === "on";
}

@customElement("openacosmi-app")
export class OpenAcosmiApp extends LitElement {
  @state() settings: UiSettings = loadSettings();
  @state() password = "";
  @state() tab: Tab = "chat";
  @state() onboarding = resolveOnboardingMode();
  @state() wizardOpen = false;
  @state() wizardV2Open = false;
  @state() wizardState: WizardState = { ...WIZARD_INITIAL_STATE };
  @state() connected = false;
  @state() theme: ThemeMode = this.settings.theme ?? "system";
  @state() themeResolved: ResolvedTheme = "dark";
  @state() hello: GatewayHelloOk | null = null;
  @state() lastError: string | null = null;
  @state() eventLog: EventLogEntry[] = [];
  private eventLogBuffer: EventLogEntry[] = [];
  private toolStreamSyncTimer: number | null = null;
  private sidebarCloseTimer: number | null = null;

  @state() assistantName = injectedAssistantIdentity.name;
  @state() assistantAvatar = injectedAssistantIdentity.avatar;
  @state() assistantAgentId = injectedAssistantIdentity.agentId ?? null;

  @state() sessionKey = this.settings.sessionKey;
  @state() chatLoading = false;
  @state() chatSending = false;
  @state() chatMessage = "";
  @state() chatMessages: unknown[] = [];
  @state() chatToolMessages: unknown[] = [];
  @state() chatStream: string | null = null;
  @state() chatStreamStartedAt: number | null = null;
  @state() chatRunId: string | null = null;
  @state() compactionStatus: CompactionStatus | null = null;
  @state() chatAvatarUrl: string | null = null;
  @state() chatThinkingLevel: string | null = null;
  @state() chatQueue: ChatQueueItem[] = [];
  @state() chatAttachments: ChatAttachment[] = [];
  @state() voiceRecording = false;
  @state() voiceRecordingDuration = 0;
  voiceSupported = isVoiceRecordingSupported();
  private _voiceRecorder: VoiceRecorder | null = null;
  @state() chatManualRefreshInFlight = false;
  // Sidebar state for tool output viewing
  @state() sidebarOpen = false;
  @state() sidebarContent: string | null = null;
  @state() sidebarError: string | null = null;
  @state() splitRatio = this.settings.splitRatio;

  @state() nodesLoading = false;
  @state() nodes: Array<Record<string, unknown>> = [];
  @state() devicesLoading = false;
  @state() devicesError: string | null = null;
  @state() devicesList: DevicePairingList | null = null;
  @state() execApprovalsLoading = false;
  @state() execApprovalsSaving = false;
  @state() execApprovalsDirty = false;
  @state() execApprovalsSnapshot: ExecApprovalsSnapshot | null = null;
  @state() execApprovalsForm: ExecApprovalsFile | null = null;
  @state() execApprovalsSelectedAgent: string | null = null;
  @state() execApprovalsTarget: "gateway" | "node" = "gateway";
  @state() execApprovalsTargetNodeId: string | null = null;
  @state() execApprovalQueue: ExecApprovalRequest[] = [];
  @state() execApprovalBusy = false;
  @state() execApprovalError: string | null = null;
  @state() coderConfirmQueue: CoderConfirmRequest[] = [];
  @state() planConfirmQueue: PlanConfirmRequest[] = [];
  @state() resultReviewQueue: ResultReviewRequest[] = [];
  @state() subagentHelpQueue: SubagentHelpRequest[] = [];
  @state() pendingGatewayUrl: string | null = null;

  // Escalation state (permission popup)
  @state() escalationState: EscalationState = createEscalationState();
  @state() escalationSelectedTtl = 30;

  // Security settings state
  @state() securityLevel = "deny";
  @state() securityLoading = false;
  @state() securityError: string | null = null;
  @state() securityLevels: import("./controllers/security-types.ts").SecurityLevelInfo[] = [];
  @state() securityHash = "";
  @state() securityConfirmOpen = false;
  @state() securityPendingLevel: string | null = null;
  @state() securityConfirmText = "";

  @state() configLoading = false;
  @state() configRaw = "{\n}\n";
  @state() configRawOriginal = "";
  @state() configValid: boolean | null = null;
  @state() configIssues: unknown[] = [];
  @state() configSaving = false;
  @state() configApplying = false;
  @state() updateRunning = false;
  @state() applySessionKey = this.settings.lastActiveSessionKey;
  @state() configSnapshot: ConfigSnapshot | null = null;
  @state() configSchema: unknown = null;
  @state() configSchemaVersion: string | null = null;
  @state() configSchemaLoading = false;
  @state() configUiHints: ConfigUiHints = {};
  @state() configForm: Record<string, unknown> | null = null;
  @state() configFormOriginal: Record<string, unknown> | null = null;
  @state() configFormDirty = false;
  @state() configFormMode: "form" | "raw" = "form";
  @state() configSearchQuery = "";
  @state() configActiveSection: string | null = null;
  @state() configActiveSubsection: string | null = null;

  @state() channelsLoading = false;
  @state() channelsSnapshot: ChannelsStatusSnapshot | null = null;
  @state() channelsError: string | null = null;
  @state() channelsLastSuccess: number | null = null;
  @state() whatsappLoginMessage: string | null = null;
  @state() whatsappLoginQrDataUrl: string | null = null;
  @state() whatsappLoginConnected: boolean | null = null;
  @state() whatsappBusy = false;
  @state() nostrProfileFormState: NostrProfileFormState | null = null;
  @state() nostrProfileAccountId: string | null = null;

  // Custom added: Notification Center state
  @state() notifications: Array<{
    id: string;
    message: string;
    timestamp: number;
    read: boolean;
    type: "error" | "info" | "success";
    sessionKey?: string;
  }> = [];
  @state() notificationsOpen = false;

  // Custom added: Remote Channel Switcher state
  @state() channelUnreadCounts: Record<string, number> = {};
  @state() isChannelDropdownOpen: boolean = false;
  @state() isSessionDropdownOpen: boolean = false;
  @state() crossChannelNotificationActive: boolean = false;
  @state() crossChannelNotificationText: string = "";
  @state() crossChannelNotificationSessionKey: string | null = null;

  @state() presenceLoading = false;
  @state() presenceEntries: PresenceEntry[] = [];
  @state() presenceError: string | null = null;
  @state() presenceStatus: string | null = null;

  // Custom added: Helper for notifications
  addNotification(message: string, type: "error" | "info" | "success" = "info", sessionKey?: string) {
    const fresh: any = {
      id: `notify-${Date.now()}-${Math.random().toString(36).substring(2, 6)}`,
      message,
      timestamp: Date.now(),
      read: false,
      type,
      sessionKey
    };
    this.notifications = [fresh, ...this.notifications].slice(0, 50); // Keep last 50
  }

  clearCrossChannelNotification() {
    this.crossChannelNotificationActive = false;
    this.crossChannelNotificationText = "";
    this.crossChannelNotificationSessionKey = null;
  }

  @state() agentsLoading = false;
  @state() agentsList: AgentsListResult | null = null;
  @state() agentsError: string | null = null;
  @state() agentsSelectedId: string | null = null;
  @state() agentsPanel: "overview" | "files" | "tools" | "skills" | "channels" | "cron" =
    "overview";
  @state() agentFilesLoading = false;
  @state() agentFilesError: string | null = null;
  @state() agentFilesList: AgentsFilesListResult | null = null;
  @state() agentFileContents: Record<string, string> = {};
  @state() agentFileDrafts: Record<string, string> = {};
  @state() agentFileActive: string | null = null;
  @state() agentFileSaving = false;
  @state() agentIdentityLoading = false;
  @state() agentIdentityError: string | null = null;
  @state() agentIdentityById: Record<string, AgentIdentityResult> = {};
  @state() agentSkillsLoading = false;
  @state() agentSkillsError: string | null = null;
  @state() agentSkillsReport: SkillStatusReport | null = null;
  @state() agentSkillsAgentId: string | null = null;

  @state() sessionsLoading = false;
  @state() sessionsResult: SessionsListResult | null = null;
  @state() sessionsError: string | null = null;
  @state() sessionsFilterActive = "";
  @state() sessionsFilterLimit = "120";
  @state() sessionsIncludeGlobal = true;
  @state() sessionsIncludeUnknown = false;

  @state() usageLoading = false;
  @state() usageResult: import("./types.js").SessionsUsageResult | null = null;
  @state() usageCostSummary: import("./types.js").CostUsageSummary | null = null;
  @state() usageError: string | null = null;
  @state() usageStartDate = (() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  })();
  @state() usageEndDate = (() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  })();
  @state() usageSelectedSessions: string[] = [];
  @state() usageSelectedDays: string[] = [];
  @state() usageSelectedHours: number[] = [];
  @state() usageChartMode: "tokens" | "cost" = "tokens";
  @state() usageDailyChartMode: "total" | "by-type" = "by-type";
  @state() usageTimeSeriesMode: "cumulative" | "per-turn" = "per-turn";
  @state() usageTimeSeriesBreakdownMode: "total" | "by-type" = "by-type";
  @state() usageTimeSeries: import("./types.js").SessionUsageTimeSeries | null = null;
  @state() usageTimeSeriesLoading = false;
  @state() usageSessionLogs: import("./views/usage.js").SessionLogEntry[] | null = null;
  @state() usageSessionLogsLoading = false;
  @state() usageSessionLogsExpanded = false;
  // Applied query (used to filter the already-loaded sessions list client-side).
  @state() usageQuery = "";
  // Draft query text (updates immediately as the user types; applied via debounce or "Search").
  @state() usageQueryDraft = "";
  @state() usageSessionSort: "tokens" | "cost" | "recent" | "messages" | "errors" = "recent";
  @state() usageSessionSortDir: "desc" | "asc" = "desc";
  @state() usageRecentSessions: string[] = [];
  @state() usageTimeZone: "local" | "utc" = "local";
  @state() usageContextExpanded = false;
  @state() usageHeaderPinned = false;
  @state() usageSessionsTab: "all" | "recent" = "all";
  @state() usageVisibleColumns: string[] = [
    "channel",
    "agent",
    "provider",
    "model",
    "messages",
    "tools",
    "errors",
    "duration",
  ];
  @state() usageLogFilterRoles: import("./views/usage.js").SessionLogRole[] = [];
  @state() usageLogFilterTools: string[] = [];
  @state() usageLogFilterHasTools = false;
  @state() usageLogFilterQuery = "";

  // Non-reactive (don’t trigger renders just for timer bookkeeping).
  usageQueryDebounceTimer: number | null = null;

  @state() cronLoading = false;
  @state() cronJobs: CronJob[] = [];
  @state() cronStatus: CronStatus | null = null;
  @state() cronError: string | null = null;
  @state() cronForm: CronFormState = { ...DEFAULT_CRON_FORM };
  @state() cronRunsJobId: string | null = null;
  @state() cronRuns: CronRunLogEntry[] = [];
  @state() cronBusy = false;

  @state() skillsLoading = false;
  @state() skillsReport: SkillStatusReport | null = null;
  @state() skillsError: string | null = null;
  @state() skillsFilter = "";
  @state() skillEdits: Record<string, string> = {};
  @state() skillsBusyKey: string | null = null;
  @state() skillMessages: Record<string, SkillMessage> = {};
  @state() distributeLoading = false;
  @state() distributeResult: string | null = null;

  @state() debugLoading = false;
  @state() debugStatus: StatusSummary | null = null;
  @state() debugHealth: HealthSnapshot | null = null;
  @state() debugModels: unknown[] = [];
  @state() debugHeartbeat: unknown = null;
  @state() debugCallMethod = "";
  @state() debugCallParams = "{}";
  @state() debugCallResult: string | null = null;
  @state() debugCallError: string | null = null;

  @state() logsLoading = false;
  @state() logsError: string | null = null;
  @state() logsFile: string | null = null;
  @state() logsEntries: LogEntry[] = [];
  @state() logsFilterText = "";
  @state() logsLevelFilters: Record<LogLevel, boolean> = {
    ...DEFAULT_LOG_LEVEL_FILTERS,
  };
  @state() logsAutoFollow = true;
  @state() logsTruncated = false;
  @state() logsCursor: number | null = null;
  @state() logsLastFetchAt: number | null = null;
  @state() logsLimit = 500;
  @state() logsMaxBytes = 250_000;
  @state() logsAtBottom = true;

  @state() memoryPanel: "sessions" | "uhms" | "media" = "uhms";

  @state() memoryLoading = false;
  @state() memoryList: import("./controllers/memory.js").MemoryItem[] | null = null;
  @state() memoryTotal = 0;
  @state() memoryError: string | null = null;
  @state() memoryDetail: import("./controllers/memory.js").MemoryDetail | null = null;
  @state() memoryStatus: import("./controllers/memory.js").MemoryStatus | null = null;
  @state() memoryImporting = false;
  @state() memoryImportResult: import("./controllers/memory.js").MemoryImportResult | null = null;
  @state() memoryPage = 0;
  @state() memoryPageSize = 50;
  @state() memoryFilterType = "";
  @state() memoryFilterCategory = "";
  @state() memoryDetailLevel = 0;
  @state() memoryLLMConfig: import("./controllers/memory.js").MemoryLLMConfig | null = null;
  @state() memoryLLMConfigOpen = false;
  @state() memoryStats: import("./controllers/memory.js").MemoryStats | null = null;
  @state() memorySearchQuery = "";
  @state() memorySearchResults: import("./controllers/memory.js").MemorySearchResult[] | null = null;
  @state() memorySearching = false;

  // Plugins & Tools
  @state() pluginsPanel: "plugins" | "tools" = "plugins";
  @state() pluginsLoading = false;
  @state() pluginsList: import("./types.js").PluginInfo[] = [];
  @state() pluginsError: string | null = null;
  @state() pluginsEditValues: Record<string, Record<string, string>> = {};
  @state() pluginsSaving: string | null = null;
  @state() toolsLoading = false;
  @state() toolsList: import("./types.js").ToolItem[] = [];
  @state() toolsError: string | null = null;
  @state() browserToolConfig: import("./types.js").BrowserToolConfig | null = null;
  @state() browserToolLoading = false;
  @state() browserToolSaving = false;
  @state() browserToolError: string | null = null;
  @state() browserToolEdits: Record<string, string | boolean> = {};

  // Media Dashboard
  @state() mediaTrendingTopics: import("./controllers/media-dashboard.js").TrendingTopic[] = [];
  @state() mediaTrendingSources: string[] = [];
  @state() mediaTrendingLoading = false;
  @state() mediaTrendingSelectedSource = "";
  @state() mediaDrafts: import("./controllers/media-dashboard.js").DraftEntry[] = [];
  @state() mediaDraftsLoading = false;
  @state() mediaDraftsSelectedPlatform = "";

  // Sub-Agents
  @state() subagentsLoading = false;
  @state() subagentsList: import("./controllers/subagents.js").SubAgentEntry[] = [];
  @state() subagentsError: string | null = null;
  @state() subagentsBusyKey: string | null = null;

  // Task Kanban
  @state() taskKanbanState: import("./controllers/task-kanban.js").TaskKanbanState = { tasks: new Map(), sortedIds: [] };

  client: GatewayBrowserClient | null = null;
  private chatScrollFrame: number | null = null;
  private chatScrollTimeout: number | null = null;
  private chatHasAutoScrolled = false;
  private chatUserNearBottom = true;
  @state() chatNewMessagesBelow = false;
  private nodesPollInterval: number | null = null;
  private logsPollInterval: number | null = null;
  private debugPollInterval: number | null = null;
  private logsScrollFrame: number | null = null;
  private toolStreamById = new Map<string, ToolStreamEntry>();
  private toolStreamOrder: string[] = [];
  refreshSessionsAfterChat = new Set<string>();
  basePath = "";
  private popStateHandler = () =>
    onPopStateInternal(this as unknown as Parameters<typeof onPopStateInternal>[0]);
  private themeMedia: MediaQueryList | null = null;
  private themeMediaHandler: ((event: MediaQueryListEvent) => void) | null = null;
  private topbarObserver: ResizeObserver | null = null;

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    initLocale(this.settings.locale);
    onLocaleChange(() => this.requestUpdate());
    handleConnected(this as unknown as Parameters<typeof handleConnected>[0]);
  }

  protected firstUpdated() {
    handleFirstUpdated(this as unknown as Parameters<typeof handleFirstUpdated>[0]);
  }

  disconnectedCallback() {
    handleDisconnected(this as unknown as Parameters<typeof handleDisconnected>[0]);
    super.disconnectedCallback();
  }

  protected updated(changed: Map<PropertyKey, unknown>) {
    handleUpdated(this as unknown as Parameters<typeof handleUpdated>[0], changed);
  }

  connect() {
    connectGatewayInternal(this as unknown as Parameters<typeof connectGatewayInternal>[0]);
  }

  handleChatScroll(event: Event) {
    handleChatScrollInternal(
      this as unknown as Parameters<typeof handleChatScrollInternal>[0],
      event,
    );
  }

  handleLogsScroll(event: Event) {
    handleLogsScrollInternal(
      this as unknown as Parameters<typeof handleLogsScrollInternal>[0],
      event,
    );
  }

  exportLogs(lines: string[], label: string) {
    exportLogsInternal(lines, label);
  }

  resetToolStream() {
    resetToolStreamInternal(this as unknown as Parameters<typeof resetToolStreamInternal>[0]);
  }

  resetChatScroll() {
    resetChatScrollInternal(this as unknown as Parameters<typeof resetChatScrollInternal>[0]);
  }

  scrollToBottom(opts?: { smooth?: boolean }) {
    resetChatScrollInternal(this as unknown as Parameters<typeof resetChatScrollInternal>[0]);
    scheduleChatScrollInternal(
      this as unknown as Parameters<typeof scheduleChatScrollInternal>[0],
      true,
      Boolean(opts?.smooth),
    );
  }

  async loadAssistantIdentity() {
    await loadAssistantIdentityInternal(this);
  }

  applySettings(next: UiSettings) {
    applySettingsInternal(this as unknown as Parameters<typeof applySettingsInternal>[0], next);
  }

  setTab(next: Tab) {
    setTabInternal(this as unknown as Parameters<typeof setTabInternal>[0], next);
  }

  setTheme(next: ThemeMode, context?: Parameters<typeof setThemeInternal>[2]) {
    setThemeInternal(this as unknown as Parameters<typeof setThemeInternal>[0], next, context);
  }

  async loadOverview() {
    await loadOverviewInternal(this as unknown as Parameters<typeof loadOverviewInternal>[0]);
  }

  async loadCron() {
    await loadCronInternal(this as unknown as Parameters<typeof loadCronInternal>[0]);
  }

  async handleAbortChat() {
    await handleAbortChatInternal(this as unknown as Parameters<typeof handleAbortChatInternal>[0]);
  }

  removeQueuedMessage(id: string) {
    removeQueuedMessageInternal(
      this as unknown as Parameters<typeof removeQueuedMessageInternal>[0],
      id,
    );
  }

  async handleSendChat(
    messageOverride?: string,
    opts?: Parameters<typeof handleSendChatInternal>[2],
  ) {
    await handleSendChatInternal(
      this as unknown as Parameters<typeof handleSendChatInternal>[0],
      messageOverride,
      opts,
    );
  }

  async handleVoiceStart() {
    if (this.voiceRecording) return;
    try {
      this._voiceRecorder = new VoiceRecorder();
      await this._voiceRecorder.start((seconds) => {
        this.voiceRecordingDuration = seconds;
      });
      this.voiceRecording = true;
      this.voiceRecordingDuration = 0;
    } catch {
      this.voiceRecording = false;
      // M-13: cancel() 确保 MediaStream tracks 被释放
      this._voiceRecorder?.cancel();
      this._voiceRecorder = null;
    }
  }

  async handleVoiceStop() {
    if (!this._voiceRecorder || !this.voiceRecording) return;
    try {
      const result = await this._voiceRecorder.stop();
      this.voiceRecording = false;
      this.voiceRecordingDuration = 0;
      this._voiceRecorder = null;
      // 转为 data URL 并作为附件添加
      const dataUrl = await blobToDataUrl(result.blob);
      const att: ChatAttachment = {
        id: `voice-${Date.now()}`,
        dataUrl,
        mimeType: result.mimeType,
        category: "audio",
        fileName: `recording-${Date.now()}.webm`,
        fileSize: result.blob.size,
      };
      this.chatAttachments = [...this.chatAttachments, att];
    } catch {
      this.voiceRecording = false;
      this.voiceRecordingDuration = 0;
      this._voiceRecorder = null;
    }
  }

  async handleWhatsAppStart(force: boolean) {
    await handleWhatsAppStartInternal(this, force);
  }

  async handleWhatsAppWait() {
    await handleWhatsAppWaitInternal(this);
  }

  async handleWhatsAppLogout() {
    await handleWhatsAppLogoutInternal(this);
  }

  async handleChannelConfigSave() {
    return handleChannelConfigSaveInternal(this);
  }

  async handleChannelConfigReload() {
    await handleChannelConfigReloadInternal(this);
  }

  handleNostrProfileEdit(accountId: string, profile: NostrProfile | null) {
    handleNostrProfileEditInternal(this, accountId, profile);
  }

  handleNostrProfileCancel() {
    handleNostrProfileCancelInternal(this);
  }

  handleNostrProfileFieldChange(field: keyof NostrProfile, value: string) {
    handleNostrProfileFieldChangeInternal(this, field, value);
  }

  async handleNostrProfileSave() {
    await handleNostrProfileSaveInternal(this);
  }

  async handleNostrProfileImport() {
    await handleNostrProfileImportInternal(this);
  }

  handleNostrProfileToggleAdvanced() {
    handleNostrProfileToggleAdvancedInternal(this);
  }

  async handleExecApprovalDecision(decision: "allow-once" | "allow-always" | "deny") {
    const active = this.execApprovalQueue[0];
    if (!active || !this.client || this.execApprovalBusy) {
      return;
    }
    this.execApprovalBusy = true;
    this.execApprovalError = null;
    try {
      await this.client.request("exec.approval.resolve", {
        id: active.id,
        decision,
      });
      this.execApprovalQueue = this.execApprovalQueue.filter((entry) => entry.id !== active.id);
    } catch (err) {
      this.execApprovalError = `Exec approval failed: ${String(err)}`;
    } finally {
      this.execApprovalBusy = false;
    }
  }

  async handleCoderConfirmDecision(id: string, decision: "allow" | "deny") {
    if (!this.client) return;
    try {
      await this.client.request("coder.confirm.resolve", { id, decision });
      this.coderConfirmQueue = removeCoderConfirm(this.coderConfirmQueue, id);
    } catch (err) {
      console.error("coder confirm resolve failed:", err);
    }
  }

  async handlePlanConfirmDecision(id: string, action: "approve" | "reject" | "edit", editedPlan?: string) {
    if (!this.client) return;
    try {
      await this.client.request("plan.confirm.resolve", { id, action, editedPlan });
      this.planConfirmQueue = removePlanConfirm(this.planConfirmQueue, id);
    } catch (err) {
      console.error("plan confirm resolve failed:", err);
    }
  }

  async handleResultReviewDecision(id: string, action: "approve" | "reject", feedback?: string) {
    if (!this.client) return;
    try {
      await this.client.request("result.approve.resolve", { id, action, feedback });
      this.resultReviewQueue = removeResultReview(this.resultReviewQueue, id);
    } catch (err) {
      console.error("result approve resolve failed:", err);
    }
  }

  async handleSubagentHelpRespond(id: string, response: string) {
    if (!this.client) return;
    try {
      await this.client.request("subagent.help.resolve", { id, response });
      this.subagentHelpQueue = removeSubagentHelp(this.subagentHelpQueue, id);
    } catch (err) {
      console.error("subagent help resolve failed:", err);
    }
  }

  handleGatewayUrlConfirm() {
    const nextGatewayUrl = this.pendingGatewayUrl;
    if (!nextGatewayUrl) {
      return;
    }
    this.pendingGatewayUrl = null;
    applySettingsInternal(this as unknown as Parameters<typeof applySettingsInternal>[0], {
      ...this.settings,
      gatewayUrl: nextGatewayUrl,
    });
    this.connect();
  }

  handleGatewayUrlCancel() {
    this.pendingGatewayUrl = null;
  }

  // Sidebar handlers for tool output viewing
  handleOpenSidebar(content: string) {
    if (this.sidebarCloseTimer != null) {
      window.clearTimeout(this.sidebarCloseTimer);
      this.sidebarCloseTimer = null;
    }
    this.sidebarContent = content;
    this.sidebarError = null;
    this.sidebarOpen = true;
  }

  handleCloseSidebar() {
    this.sidebarOpen = false;
    // Clear content after transition
    if (this.sidebarCloseTimer != null) {
      window.clearTimeout(this.sidebarCloseTimer);
    }
    this.sidebarCloseTimer = window.setTimeout(() => {
      if (this.sidebarOpen) {
        return;
      }
      this.sidebarContent = null;
      this.sidebarError = null;
      this.sidebarCloseTimer = null;
    }, 200);
  }

  handleSplitRatioChange(ratio: number) {
    const newRatio = Math.max(0.4, Math.min(0.7, ratio));
    this.splitRatio = newRatio;
    this.applySettings({ ...this.settings, splitRatio: newRatio });
  }

  async handleStartWizard() {
    await startWizardInternal(this as unknown as AppViewState);
  }

  handleStartWizardV2() {
    startWizardV2Internal(this as unknown as AppViewState);
  }

  async handleCancelWizard() {
    await cancelWizardInternal(this as unknown as AppViewState);
  }

  render() {
    return renderApp(this as unknown as AppViewState);
  }
}
