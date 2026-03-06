import type { EventLogEntry } from "./app-events.ts";
import type { AgentProgress, CompactionStatus } from "./app-tool-stream.ts";
import type { DevicePairingList } from "./controllers/devices.ts";
import type { CoderConfirmRequest } from "./controllers/coder-confirmation.ts";
import type { EscalationState } from "./controllers/escalation.ts";
import type { PlanConfirmRequest } from "./controllers/plan-confirmation.ts";
import type { ResultReviewRequest } from "./controllers/result-review.ts";
import type { SubagentHelpRequest } from "./controllers/subagent-help.ts";
import type { ExecApprovalRequest } from "./controllers/exec-approval.ts";
import type { ExecApprovalsFile, ExecApprovalsSnapshot } from "./controllers/exec-approvals.ts";
import type { SecurityLevelInfo } from "./controllers/security-types.ts";
import type { SkillMessage } from "./controllers/skills.ts";
import type { GatewayBrowserClient, GatewayHelloOk } from "./gateway.ts";
import type { Tab } from "./navigation.ts";
import type { UiSettings } from "./storage.ts";
import type { ThemeTransitionContext } from "./theme-transition.ts";
import type { ThemeMode } from "./theme.ts";
import type {
  AgentsListResult,
  AgentsFilesListResult,
  AgentIdentityResult,
  ChannelsStatusSnapshot,
  ConfigSnapshot,
  ConfigUiHints,
  CronJob,
  CronRunLogEntry,
  CronStatus,
  HealthSnapshot,
  LogEntry,
  LogLevel,
  NostrProfile,
  PluginInfo,
  PresenceEntry,
  SessionsUsageResult,
  CostUsageSummary,
  SessionUsageTimeSeries,
  SessionsListResult,
  SkillStatusReport,
  StatusSummary,
} from "./types.ts";
import type { ChatAttachment, ChatQueueItem, CronFormState } from "./ui-types.ts";
import type { NostrProfileFormState } from "./views/channels.nostr-profile-form.ts";
import type { SessionLogEntry } from "./views/usage.ts";


// Phase 3: 媒体巡检心跳状态
export type MediaHeartbeatStatus = {
  lastPatrolAt: number | null;
  nextPatrolAt: number | null;
  activeJobId: string | null;
  lastError: string | null;
  autoSpawnCount?: number;
};

export type AppViewState = {
  settings: UiSettings;
  password: string;
  tab: Tab;
  overviewPanel: "dashboard" | "instances" | "usage";
  onboarding: boolean;
  wizardV2Open: boolean;
  basePath: string;
  connected: boolean;
  theme: ThemeMode;
  themeResolved: "light" | "dark";
  hello: GatewayHelloOk | null;
  lastError: string | null;
  eventLog: EventLogEntry[];
  assistantName: string;
  assistantAvatar: string | null;
  assistantAgentId: string | null;
  sessionKey: string;
  chatLoading: boolean;
  chatSending: boolean;
  chatMessage: string;
  chatAttachments: ChatAttachment[];
  voiceRecording: boolean;
  voiceRecordingDuration: number;
  voiceSupported: boolean;
  chatMessages: unknown[];
  chatToolMessages: unknown[];
  chatStream: string | null;
  chatStreamStartedAt: number | null;
  chatRunId: string | null;
  compactionStatus: CompactionStatus | null;
  chatAvatarUrl: string | null;
  chatThinkingLevel: string | null;
  chatQueue: ChatQueueItem[];
  chatManualRefreshInFlight: boolean;
  nodesLoading: boolean;
  nodes: Array<Record<string, unknown>>;
  chatNewMessagesBelow: boolean;
  sidebarOpen: boolean;
  sidebarContent: string | null;
  sidebarError: string | null;
  splitRatio: number;
  scrollToBottom: (opts?: { smooth?: boolean }) => void;
  devicesLoading: boolean;
  devicesError: string | null;
  devicesList: DevicePairingList | null;
  execApprovalsLoading: boolean;
  execApprovalsSaving: boolean;
  execApprovalsDirty: boolean;
  execApprovalsSnapshot: ExecApprovalsSnapshot | null;
  execApprovalsForm: ExecApprovalsFile | null;
  execApprovalsSelectedAgent: string | null;
  execApprovalsTarget: "gateway" | "node";
  execApprovalsTargetNodeId: string | null;
  execApprovalQueue: ExecApprovalRequest[];
  execApprovalBusy: boolean;
  execApprovalError: string | null;
  coderConfirmQueue: CoderConfirmRequest[];
  planConfirmQueue: PlanConfirmRequest[];
  resultReviewQueue: ResultReviewRequest[];
  subagentHelpQueue: SubagentHelpRequest[];
  escalationState: EscalationState;
  escalationSelectedTtl: number;
  pendingGatewayUrl: string | null;
  securityLevel: string;
  securityLoading: boolean;
  securityError: string | null;
  securityLevels: SecurityLevelInfo[];
  securityHash: string;
  securityConfirmOpen: boolean;
  securityPendingLevel: string | null;
  securityConfirmText: string;
  remoteApprovalLoading: boolean;
  remoteApprovalError: string | null;
  remoteApprovalEnabled: boolean;
  remoteApprovalCallbackUrl: string;
  remoteApprovalEnabledProviders: string[];
  remoteApprovalFeishuEnabled: boolean;
  remoteApprovalFeishuAppId: string;
  remoteApprovalFeishuAppSecret: string;
  remoteApprovalFeishuChatId: string;
  remoteApprovalDingtalkEnabled: boolean;
  remoteApprovalDingtalkWebhookUrl: string;
  remoteApprovalDingtalkWebhookSecret: string;
  remoteApprovalWecomEnabled: boolean;
  remoteApprovalWecomCorpId: string;
  remoteApprovalWecomAgentId: string;
  remoteApprovalWecomSecret: string;
  remoteApprovalWecomToUser: string;
  remoteApprovalWecomToParty: string;
  remoteApprovalTestLoading: boolean;
  remoteApprovalTestResult: string | null;
  remoteApprovalTestError: string | null;
  remoteApprovalSaving: boolean;
  remoteApprovalSaved: boolean;
  configLoading: boolean;
  configRaw: string;
  configRawOriginal: string;
  configValid: boolean | null;
  configIssues: unknown[];
  configSaving: boolean;
  configApplying: boolean;
  updateRunning: boolean;
  applySessionKey: string;
  configSnapshot: ConfigSnapshot | null;
  configSchema: unknown;
  configSchemaVersion: string | null;
  configSchemaLoading: boolean;
  configUiHints: ConfigUiHints;
  configForm: Record<string, unknown> | null;
  configFormOriginal: Record<string, unknown> | null;
  configFormMode: "form" | "raw";
  configSearchQuery: string;
  configActiveSection: string | null;
  configActiveSubsection: string | null;
  channelsLoading: boolean;
  channelsSnapshot: ChannelsStatusSnapshot | null;
  channelsError: string | null;
  channelsLastSuccess: number | null;
  whatsappLoginMessage: string | null;
  whatsappLoginQrDataUrl: string | null;
  whatsappLoginConnected: boolean | null;
  whatsappBusy: boolean;
  nostrProfileFormState: NostrProfileFormState | null;
  nostrProfileAccountId: string | null;
  configFormDirty: boolean;

  // Custom added: Notification Center state
  notifications: Array<{
    id: string;
    message: string;
    timestamp: number;
    read: boolean;
    type: "error" | "info" | "success";
    sessionKey?: string;
  }>;
  notificationsOpen: boolean;

  presenceLoading: boolean;
  presenceEntries: PresenceEntry[];
  presenceError: string | null;
  presenceStatus: string | null;
  agentsLoading: boolean;
  agentsList: AgentsListResult | null;
  agentsError: string | null;
  agentsSelectedId: string | null;
  agentsPanel: "overview" | "files" | "tools" | "skills" | "channels" | "cron";
  agentFilesLoading: boolean;
  agentFilesError: string | null;
  agentFilesList: AgentsFilesListResult | null;
  agentFileContents: Record<string, string>;
  agentFileDrafts: Record<string, string>;
  agentFileActive: string | null;
  agentFileSaving: boolean;
  agentIdentityLoading: boolean;
  agentIdentityError: string | null;
  agentIdentityById: Record<string, AgentIdentityResult>;
  agentSkillsLoading: boolean;
  agentSkillsError: string | null;
  agentSkillsReport: SkillStatusReport | null;
  agentSkillsAgentId: string | null;
  sessionsLoading: boolean;
  sessionsResult: SessionsListResult | null;
  sessionsError: string | null;
  sessionsFilterActive: string;
  sessionsFilterLimit: string;
  sessionsIncludeGlobal: boolean;
  sessionsIncludeUnknown: boolean;
  usageLoading: boolean;
  usageResult: SessionsUsageResult | null;
  usageCostSummary: CostUsageSummary | null;
  usageError: string | null;
  usageStartDate: string;
  usageEndDate: string;
  usageSelectedSessions: string[];
  usageSelectedDays: string[];
  usageSelectedHours: number[];
  usageChartMode: "tokens" | "cost";
  usageDailyChartMode: "total" | "by-type";
  usageTimeSeriesMode: "cumulative" | "per-turn";
  usageTimeSeriesBreakdownMode: "total" | "by-type";
  usageTimeSeries: SessionUsageTimeSeries | null;
  usageTimeSeriesLoading: boolean;
  usageSessionLogs: SessionLogEntry[] | null;
  usageSessionLogsLoading: boolean;
  usageSessionLogsExpanded: boolean;
  usageQuery: string;
  usageQueryDraft: string;
  usageQueryDebounceTimer: number | null;
  usageSessionSort: "tokens" | "cost" | "recent" | "messages" | "errors";
  usageSessionSortDir: "asc" | "desc";
  usageRecentSessions: string[];
  usageTimeZone: "local" | "utc";
  usageContextExpanded: boolean;
  usageHeaderPinned: boolean;
  usageSessionsTab: "all" | "recent";
  usageVisibleColumns: string[];
  usageLogFilterRoles: import("./views/usage.js").SessionLogRole[];
  usageLogFilterTools: string[];
  usageLogFilterHasTools: boolean;
  usageLogFilterQuery: string;
  cronLoading: boolean;
  cronJobs: CronJob[];
  cronStatus: CronStatus | null;
  cronError: string | null;
  cronForm: CronFormState;
  cronRunsJobId: string | null;
  cronRuns: CronRunLogEntry[];
  cronBusy: boolean;
  skillsLoading: boolean;
  skillsReport: SkillStatusReport | null;
  skillsError: string | null;
  skillsFilter: string;
  skillEdits: Record<string, string>;
  skillMessages: Record<string, SkillMessage>;
  skillsBusyKey: string | null;
  distributeLoading: boolean;
  distributeResult: string | null;

  // Custom added: Remote Channel Switcher state
  channelUnreadCounts: Record<string, number>;
  isChannelDropdownOpen: boolean;
  isSessionDropdownOpen: boolean;
  crossChannelNotificationActive: boolean;
  crossChannelNotificationText: string;
  crossChannelNotificationSessionKey: string | null;

  // Custom added: Helper for notifications
  addNotification: (message: string, type?: "error" | "info" | "success", sessionKey?: string) => void;
  clearCrossChannelNotification?: () => void;

  // Sub-Agents — 子智能体已统一到 agents 标签页侧边栏
  subagentsLoading: boolean;
  subagentsList: import("./controllers/subagents.js").SubAgentEntry[];
  subagentsError: string | null;
  subagentsBusyKey: string | null;
  /** @deprecated 子智能体选择已由 agentsSelectedId 管理，保留用于向后兼容 */
  subagentsActiveTab: string;
  debugLoading: boolean;
  debugStatus: StatusSummary | null;
  debugHealth: HealthSnapshot | null;
  debugModels: unknown[];
  debugHeartbeat: unknown;
  debugCallMethod: string;
  debugCallParams: string;
  debugCallResult: string | null;
  debugCallError: string | null;
  logsLoading: boolean;
  logsError: string | null;
  logsFile: string | null;
  logsEntries: LogEntry[];
  logsFilterText: string;
  logsLevelFilters: Record<LogLevel, boolean>;
  logsAutoFollow: boolean;
  logsTruncated: boolean;
  logsCursor: number | null;
  logsLastFetchAt: number | null;
  logsLimit: number;
  logsMaxBytes: number;
  logsAtBottom: boolean;
  memoryPanel: "sessions" | "uhms" | "media";
  memoryLoading: boolean;
  memoryList: import("./controllers/memory.js").MemoryItem[] | null;
  memoryTotal: number;
  memoryError: string | null;
  memoryDetail: import("./controllers/memory.js").MemoryDetail | null;
  memoryStatus: import("./controllers/memory.js").MemoryStatus | null;
  memoryImporting: boolean;
  memoryImportResult: import("./controllers/memory.js").MemoryImportResult | null;
  memoryPage: number;
  memoryPageSize: number;
  memoryFilterType: string;
  memoryFilterCategory: string;
  memoryDetailLevel: number;
  memoryLLMConfig: import("./controllers/memory.js").MemoryLLMConfig | null;
  memoryLLMConfigOpen: boolean;
  memoryStats: import("./controllers/memory.js").MemoryStats | null;
  memorySearchQuery: string;
  memorySearchResults: import("./controllers/memory.js").MemorySearchResult[] | null;
  memorySearching: boolean;
  client: GatewayBrowserClient | null;
  refreshSessionsAfterChat: Set<string>;
  connect: () => void;
  setTab: (tab: Tab) => void;
  setTheme: (theme: ThemeMode, context?: ThemeTransitionContext) => void;
  applySettings: (next: UiSettings) => void;
  loadOverview: () => Promise<void>;
  loadAssistantIdentity: () => Promise<void>;
  loadCron: () => Promise<void>;
  handleWhatsAppStart: (force: boolean) => Promise<void>;
  handleWhatsAppWait: () => Promise<void>;
  handleWhatsAppLogout: () => Promise<void>;
  handleChannelConfigSave: () => Promise<boolean>;
  handleChannelConfigReload: () => Promise<void>;
  handleNostrProfileEdit: (accountId: string, profile: NostrProfile | null) => void;
  handleNostrProfileCancel: () => void;
  handleNostrProfileFieldChange: (field: keyof NostrProfile, value: string) => void;
  handleNostrProfileSave: () => Promise<void>;
  handleNostrProfileImport: () => Promise<void>;
  handleNostrProfileToggleAdvanced: () => void;
  handleExecApprovalDecision: (decision: "allow-once" | "allow-always" | "deny") => Promise<void>;
  handleCoderConfirmDecision: (id: string, decision: "allow" | "deny") => Promise<void>;
  handlePlanConfirmDecision: (id: string, action: "approve" | "reject" | "edit", editedPlan?: string) => Promise<void>;
  handleResultReviewDecision: (id: string, action: "approve" | "reject", feedback?: string) => Promise<void>;
  handleSubagentHelpRespond: (id: string, response: string) => Promise<void>;
  handleGatewayUrlConfirm: () => void;
  handleGatewayUrlCancel: () => void;
  handleConfigLoad: () => Promise<void>;
  handleConfigSave: () => Promise<void>;
  handleConfigApply: () => Promise<void>;
  handleConfigFormUpdate: (path: string, value: unknown) => void;
  handleConfigFormModeChange: (mode: "form" | "raw") => void;
  handleConfigRawChange: (raw: string) => void;
  handleInstallSkill: (key: string) => Promise<void>;
  handleUpdateSkill: (key: string) => Promise<void>;
  handleToggleSkillEnabled: (key: string, enabled: boolean) => Promise<void>;
  handleUpdateSkillEdit: (key: string, value: string) => void;
  handleSaveSkillApiKey: (key: string, apiKey: string) => Promise<void>;
  handleCronToggle: (jobId: string, enabled: boolean) => Promise<void>;
  handleCronRun: (jobId: string) => Promise<void>;
  handleCronRemove: (jobId: string) => Promise<void>;
  handleCronAdd: () => Promise<void>;
  handleCronRunsLoad: (jobId: string) => Promise<void>;
  handleCronFormUpdate: (path: string, value: unknown) => void;
  handleSessionsLoad: () => Promise<void>;
  handleSessionsPatch: (key: string, patch: unknown) => Promise<void>;
  handleLoadNodes: () => Promise<void>;
  handleLoadPresence: () => Promise<void>;
  handleLoadSkills: () => Promise<void>;
  handleLoadDebug: () => Promise<void>;
  handleLoadLogs: () => Promise<void>;
  handleDebugCall: () => Promise<void>;
  handleRunUpdate: () => Promise<void>;
  setPassword: (next: string) => void;
  setSessionKey: (next: string) => void;
  setChatMessage: (next: string) => void;
  handleSendChat: (messageOverride?: string, opts?: { restoreDraft?: boolean }) => Promise<void>;
  handleAbortChat: () => Promise<void>;
  handleVoiceStart: () => Promise<void>;
  handleVoiceStop: () => Promise<void>;
  removeQueuedMessage: (id: string) => void;
  handleChatScroll: (event: Event) => void;
  resetToolStream: () => void;
  resetChatScroll: () => void;
  exportLogs: (lines: string[], label: string) => void;
  handleLogsScroll: (event: Event) => void;
  handleOpenSidebar: (content: string) => void;
  handleCloseSidebar: () => void;
  handleSplitRatioChange: (ratio: number) => void;
  handleStartWizardV2?: () => void;
  requestUpdate: () => void;
  // Phase C/D/E: STT/DocConv/Image wizard state
  sttWizard?: Record<string, unknown>;
  docConvWizard?: Record<string, unknown>;
  imageWizard?: Record<string, unknown>;

  // Media Dashboard
  mediaTrendingTopics: import("./controllers/media-dashboard.js").TrendingTopic[];
  mediaTrendingSources: string[];
  mediaTrendingLoading: boolean;
  mediaTrendingSelectedSource: string;
  mediaDrafts: import("./controllers/media-dashboard.js").DraftEntry[];
  mediaDraftsLoading: boolean;
  mediaDraftsSelectedPlatform: string;
  mediaPublishRecords: import("./controllers/media-dashboard.js").PublishRecord[];
  mediaPublishLoading: boolean;
  mediaPublishPage: number;
  mediaPublishPageSize: number;
  mediaHeartbeat: MediaHeartbeatStatus | null;
  mediaDraftDetail: import("./controllers/media-dashboard.js").DraftEntry | null;
  mediaDraftDetailLoading: boolean;
  mediaPublishDetail: import("./controllers/media-dashboard.js").PublishRecord | null;
  mediaPublishDetailLoading: boolean;
  mediaConfig: import("./controllers/media-dashboard.js").MediaConfig | null;
  mediaDraftEdit: import("./controllers/media-dashboard.js").DraftEntry | null;
  mediaPatrolJobs: import("./controllers/media-dashboard.js").CronPatrolJob[];
  mediaTrendingHealth: import("./controllers/media-dashboard.js").SourceHealthInfo[];
  mediaTrendingHealthLoading: boolean;
  mediaManageSubTab: string;
  agentProgress: AgentProgress | null;

  // Task Kanban
  taskKanbanState: import("./controllers/task-kanban.js").TaskKanbanState;

  // Plugins & Tools
  pluginsPanel: "plugins" | "tools" | "skills";
  pluginsLoading: boolean;
  pluginsList: PluginInfo[];
  pluginsError: string | null;
  pluginsEditValues: Record<string, Record<string, string>>;
  pluginsSaving: string | null;
  toolsLoading: boolean;
  toolsList: import("./types.js").ToolItem[];
  toolsError: string | null;
  browserToolConfig: import("./types.js").BrowserToolConfig | null;
  browserToolLoading: boolean;
  browserToolSaving: boolean;
  browserToolError: string | null;
  browserToolEdits: Record<string, string | boolean>;
};
