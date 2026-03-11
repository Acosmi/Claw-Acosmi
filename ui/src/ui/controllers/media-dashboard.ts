// controllers/media-dashboard.ts — 媒体运营仪表盘控制器
// 管理热点话题、草稿列表的数据加载逻辑。

import type { AppViewState } from "../app-view-state.ts";
import { runMediaMutation } from "./media-mutations.ts";

// ---------- 配置类型 ----------

export interface MediaToolInfo {
  name: string;
  description: string;
  status?: string; // "builtin" | "configured" | "needs_configuration" | "enabled" | "disabled"
  configured?: boolean;
  enabled?: boolean;
  scope?: string; // "media" | "shared"
}

export interface MediaSourceInfo {
  name: string;
  status?: string; // "default_enabled" | "configured" | "disabled" | "needs_configuration"
  configured?: boolean;
  enabled?: boolean;
  source_configured?: boolean;
  requires_credential?: boolean;
}

export interface TrendingStrategy {
  hotKeywords: string[];
  monitorIntervalMin: number;
  trendingThreshold: number;
  contentCategories: string[];
  autoDraftEnabled: boolean;
}

export interface MediaPublisherProfileBase {
  enabled: boolean;
  configured: boolean;
  status: string;
  missing?: string[];
  authMode?: string;
  authStatus?: string;
  authMessage?: string;
  authUpdatedAt?: string;
  browserReady?: boolean;
}

export interface MediaWeChatProfile extends MediaPublisherProfileBase {
  accountName?: string;
  accountId?: string;
  appId?: string;
  appSecret?: string;
  tokenCachePath?: string;
}

export interface MediaXiaohongshuProfile extends MediaPublisherProfileBase {
  accountName?: string;
  accountId?: string;
  cookiePath?: string;
  autoInteractInterval?: number;
  rateLimitSeconds?: number;
  errorScreenshotDir?: string;
}

export interface MediaWebsiteProfile extends MediaPublisherProfileBase {
  siteName?: string;
  apiUrl?: string;
  authType?: string;
  authToken?: string;
  imageUploadUrl?: string;
  timeoutSeconds?: number;
  maxRetries?: number;
}

export interface MediaPublisherProfiles {
  wechat: MediaWeChatProfile;
  xiaohongshu: MediaXiaohongshuProfile;
  website: MediaWebsiteProfile;
}

export interface MediaBochaTrendingProfile {
  configured: boolean;
  status: string;
  missing?: string[];
  authMode?: string;
  apiKey?: string;
  baseUrl?: string;
  freshness?: string;
}

export interface MediaCustomOpenAITrendingProfile {
  configured: boolean;
  status: string;
  missing?: string[];
  authMode?: string;
  apiKey?: string;
  baseUrl?: string;
  model?: string;
  systemPrompt?: string;
  requestExtras?: string;
}

export interface MediaTrendingSourceProfiles {
  bocha: MediaBochaTrendingProfile;
  custom_openai: MediaCustomOpenAITrendingProfile;
}

export interface MediaConfig {
  agent_id: string;
  label: string;
  status: string; // "default" | "configured"
  trending_sources: MediaSourceInfo[];
  trending_source_profiles?: MediaTrendingSourceProfiles;
  tools: MediaToolInfo[];
  publishers: string[];
  publisher_profiles?: MediaPublisherProfiles;
  publish_enabled: boolean;
  publish_configured: boolean;
  llm: {
    provider: string;
    model: string;
    apiKey: string;
    baseUrl: string;
    autoSpawnEnabled: boolean;
    maxAutoSpawnsPerDay: number;
  };
  trending_strategy?: TrendingStrategy;
  enabled_sources_configured?: boolean;
  enabled_sources?: string[];
}

export interface SourceHealthInfo {
  name: string;
  status: string; // "ok" | "error"
  error?: string;
  count: number;
}

export interface TrendingFetchError {
  source: string;
  error: string;
}

// ---------- 类型 ----------

export interface TrendingTopic {
  title: string;
  source: string;
  url?: string;
  heat_score: number;
  category?: string;
  fetched_at: string;
}

export interface DraftEntry {
  id: string;
  title: string;
  body: string;
  platform: string;
  style: string;
  status: string;
  created_at: string;
  updated_at: string;
  tags?: string[];
  images?: string[];
}

// ---------- 加载函数 ----------

export async function loadTrendingSources(state: AppViewState): Promise<void> {
  if (!state.client || !state.connected) return;
  try {
    const res = await state.client.request<{ sources: string[] }>("media.trending.sources");
    if (res?.sources) {
      state.mediaTrendingSources = res.sources;
    }
  } catch {
    // 忽略加载失败
  }
}

export async function loadTrendingTopics(
  state: AppViewState,
  source?: string,
  category?: string,
): Promise<void> {
  if (!state.client || !state.connected) return;
  state.mediaTrendingLoading = true;
  try {
    const params: Record<string, unknown> = { limit: 30 };
    if (source) params.source = source;
    if (category) params.category = category;

    const res = await state.client.request<{
      topics: TrendingTopic[];
      count: number;
      errors?: TrendingFetchError[];
    }>("media.trending.fetch", params);

    if (res) {
      state.mediaTrendingTopics = res.topics || [];
      state.mediaTrendingErrors = res.errors || [];
    }
  } catch (error) {
    state.mediaTrendingTopics = [];
    state.mediaTrendingErrors = [{
      source: source || "all",
      error: error instanceof Error ? error.message : "fetch trending failed",
    }];
  } finally {
    state.mediaTrendingLoading = false;
  }
}

export async function loadDraftsList(state: AppViewState, platform?: string): Promise<void> {
  if (!state.client || !state.connected) return;
  state.mediaDraftsLoading = true;
  try {
    const params: Record<string, unknown> = {};
    if (platform) params.platform = platform;

    const res = await state.client.request<{
      drafts: DraftEntry[];
      count: number;
    }>("media.drafts.list", params);

    if (res) {
      state.mediaDrafts = res.drafts || [];
    }
  } catch {
    state.mediaDrafts = [];
  } finally {
    state.mediaDraftsLoading = false;
  }
}

export async function deleteDraft(state: AppViewState, id: string): Promise<boolean> {
  return runMediaMutation(state, {
    label: "deleteDraft",
    run: (client) => client.request("media.drafts.delete", { id }),
    onSuccess: (nextState) => {
      if (nextState.mediaDraftDetail?.id === id) {
        nextState.mediaDraftDetail = null;
      }
      if (nextState.mediaDraftEdit?.id === id) {
        nextState.mediaDraftEdit = null;
      }
    },
    invalidate: [
      async (nextState) => {
        await loadDraftsList(nextState, selectedDraftPlatform(nextState));
      },
    ],
  });
}

export async function approveDraft(state: AppViewState, id: string): Promise<boolean> {
  return runMediaMutation(state, {
    label: "approveDraft",
    run: (client) => client.request("media.drafts.approve", { id }),
    invalidate: [
      async (nextState) => {
        await loadDraftsList(nextState, selectedDraftPlatform(nextState));
      },
      async (nextState) => {
        if (nextState.mediaDraftDetail?.id === id) {
          await loadDraftDetail(nextState, id);
        }
      },
    ],
  });
}

export async function updateDraft(
  state: AppViewState,
  id: string,
  updates: { title?: string; body?: string; platform?: string; tags?: string[] },
): Promise<boolean> {
  return runMediaMutation(state, {
    label: "updateDraft",
    run: (client) => client.request("media.drafts.update", { id, ...updates }),
    onSuccess: (nextState) => {
      nextState.mediaDraftEdit = null;
    },
    invalidate: [
      async (nextState) => {
        await loadDraftsList(nextState, selectedDraftPlatform(nextState));
      },
      async (nextState) => {
        if (nextState.mediaDraftDetail?.id === id) {
          await loadDraftDetail(nextState, id);
        }
      },
    ],
  });
}

export function openDraftEdit(state: AppViewState, draft: DraftEntry): void {
  state.mediaDraftEdit = { ...draft };
}

export function closeDraftEdit(state: AppViewState): void {
  state.mediaDraftEdit = null;
}

export interface PublishRecord {
  id: string;
  draft_id: string;
  title: string;
  platform: string;
  post_id?: string;
  url?: string;
  status: string;
  published_at: string;
}

export async function loadPublishHistory(
  state: AppViewState,
  opts?: { limit?: number; offset?: number },
): Promise<void> {
  if (!state.client || !state.connected) return;
  state.mediaPublishLoading = true;
  try {
    const params: Record<string, unknown> = {};
    if (opts?.limit) params.limit = opts.limit;
    if (opts?.offset) params.offset = opts.offset;

    const res = await state.client.request<{
      records: PublishRecord[];
      count: number;
    }>("media.publish.list", params);

    if (res) {
      state.mediaPublishRecords = res.records || [];
    }
  } catch {
    state.mediaPublishRecords = [];
  } finally {
    state.mediaPublishLoading = false;
  }
}

// ---------- 详情加载 ----------

export async function loadDraftDetail(state: AppViewState, id: string): Promise<void> {
  if (!state.client || !state.connected) return;
  state.mediaDraftDetailLoading = true;
  try {
    const res = await state.client.request<{ draft: DraftEntry }>("media.drafts.get", { id });
    if (res?.draft) {
      state.mediaDraftDetail = res.draft;
    }
  } catch {
    state.mediaDraftDetail = null;
  } finally {
    state.mediaDraftDetailLoading = false;
  }
}

export async function loadPublishDetail(state: AppViewState, id: string): Promise<void> {
  if (!state.client || !state.connected) return;
  state.mediaPublishDetailLoading = true;
  try {
    const res = await state.client.request<{ record: PublishRecord }>("media.publish.get", { id });
    if (res?.record) {
      state.mediaPublishDetail = res.record;
    }
  } catch {
    state.mediaPublishDetail = null;
  } finally {
    state.mediaPublishDetailLoading = false;
  }
}

export function closeDraftDetail(state: AppViewState): void {
  state.mediaDraftDetail = null;
}

export function closePublishDetail(state: AppViewState): void {
  state.mediaPublishDetail = null;
}

export async function loadMediaConfig(state: AppViewState): Promise<void> {
  if (!state.client || !state.connected) return;
  try {
    const res = await state.client.request<MediaConfig>("media.config.get");
    if (res) {
      state.mediaConfig = res;
    }
  } catch {
    state.mediaConfig = null;
  }
}

export async function updateMediaConfig(state: AppViewState, patch: Record<string, unknown>): Promise<void> {
  await runMediaMutation(state, {
    label: "updateMediaConfig",
    run: (client) => client.request("media.config.update", patch),
    invalidate: [
      async (nextState) => {
        await loadMediaConfig(nextState);
      },
    ],
  });
}

export interface MediaPublisherLoginResult {
  platform: string;
  status: string;
  message?: string;
  cookiePath?: string;
  loginUrl?: string;
  targetId?: string;
  currentUrl?: string;
  browserReady?: boolean;
  cookieCount?: number;
  startedAt?: string;
  updatedAt?: string;
  savedAt?: string;
  lastError?: string;
}

export async function startMediaPublisherLogin(
  state: AppViewState,
  platform: "xiaohongshu",
): Promise<boolean> {
  return runMediaMutation(state, {
    label: "startMediaPublisherLogin",
    run: (client) => client.request<MediaPublisherLoginResult>("media.publisher.login.start", { platform }),
    onSuccess: (nextState, result) => {
      if (result?.message) {
        nextState.addNotification?.(result.message, "info");
      }
    },
    invalidate: [
      async (nextState) => {
        await loadMediaConfig(nextState);
      },
    ],
  });
}

export async function waitMediaPublisherLogin(
  state: AppViewState,
  platform: "xiaohongshu",
): Promise<boolean> {
  return runMediaMutation(state, {
    label: "waitMediaPublisherLogin",
    run: (client) =>
      client.request<MediaPublisherLoginResult>("media.publisher.login.wait", {
        platform,
        timeoutMs: 90000,
      }),
    onSuccess: (nextState, result) => {
      if (result?.message) {
        nextState.addNotification?.(
          result.message,
          result.status === "authenticated" ? "success" : "info",
        );
      }
    },
    invalidate: [
      async (nextState) => {
        await loadMediaConfig(nextState);
      },
    ],
  });
}

// ---------- 巡检任务 ----------

export interface CronPatrolJob {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  schedule: { kind: string; everyMs?: number };
  state: {
    nextRunAtMs?: number;
    lastRunAtMs?: number;
    lastStatus?: string;
    lastError?: string;
  };
}

export async function loadMediaPatrolJobs(state: AppViewState): Promise<void> {
  if (!state.client || !state.connected) return;
  try {
    const res = await state.client.request<{ jobs: CronPatrolJob[] }>("cron.list", { includeDisabled: true });
    if (res?.jobs) {
      state.mediaPatrolJobs = res.jobs.filter((j) => j.name.startsWith("media.patrol."));
    }
  } catch {
    state.mediaPatrolJobs = [];
  }
}

export async function checkTrendingSourceHealth(state: AppViewState): Promise<void> {
  if (!state.client || !state.connected) return;
  state.mediaTrendingHealthLoading = true;
  try {
    const res = await state.client.request<{ sources: SourceHealthInfo[] }>("media.trending.health");
    if (res?.sources) {
      state.mediaTrendingHealth = res.sources;
    }
  } catch {
    state.mediaTrendingHealth = [];
  } finally {
    state.mediaTrendingHealthLoading = false;
  }
}

// ---------- 工具/源 Toggle ----------

export async function toggleMediaTool(state: AppViewState, tool: string, enabled: boolean): Promise<void> {
  await runMediaMutation(state, {
    label: "toggleMediaTool",
    run: (client) => client.request("media.tools.toggle", { tool, enabled }),
    invalidate: [
      async (nextState) => {
        await loadMediaConfig(nextState);
      },
    ],
  });
}

export async function toggleMediaSource(state: AppViewState, source: string, enabled: boolean): Promise<void> {
  await runMediaMutation(state, {
    label: "toggleMediaSource",
    run: (client) => client.request("media.sources.toggle", { source, enabled }),
    invalidate: [
      async (nextState) => {
        await loadMediaConfig(nextState);
      },
    ],
  });
}

export async function loadMediaDashboard(state: AppViewState): Promise<void> {
  await Promise.all([
    loadMediaConfig(state),
    loadTrendingSources(state),
    loadDraftsList(state),
    loadPublishHistory(state),
    loadMediaPatrolJobs(state),
    checkTrendingSourceHealth(state),
  ]);
}

function selectedDraftPlatform(state: AppViewState): string | undefined {
  const platform = state.mediaDraftsSelectedPlatform?.trim();
  return platform ? platform : undefined;
}
