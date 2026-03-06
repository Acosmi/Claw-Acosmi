// controllers/media-dashboard.ts — 媒体运营仪表盘控制器
// 管理热点话题、草稿列表的数据加载逻辑。

import type { AppViewState } from "../app-view-state.ts";

// ---------- 配置类型 ----------

export interface MediaToolInfo {
  name: string;
  description: string;
  status: string; // "registered" | "configured"
}

export interface MediaSourceInfo {
  name: string;
  status: string; // "registered" | "configured"
}

export interface MediaConfig {
  agent_id: string;
  label: string;
  status: string; // "default" | "configured"
  trending_sources: MediaSourceInfo[];
  tools: MediaToolInfo[];
  publishers: string[];
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
}

export interface SourceHealthInfo {
  name: string;
  status: string; // "ok" | "error"
  error?: string;
  count: number;
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
      errors?: Array<{ source: string; error: string }>;
    }>("media.trending.fetch", params);

    if (res) {
      state.mediaTrendingTopics = res.topics || [];
    }
  } catch {
    state.mediaTrendingTopics = [];
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
  if (!state.client || !state.connected) return false;
  try {
    await state.client.request("media.drafts.delete", { id });
    // 刷新列表
    await loadDraftsList(state);
    return true;
  } catch {
    return false;
  }
}

export async function approveDraft(state: AppViewState, id: string): Promise<boolean> {
  if (!state.client || !state.connected) return false;
  try {
    await state.client.request("media.drafts.approve", { id });
    await loadDraftsList(state);
    return true;
  } catch {
    return false;
  }
}

export async function updateDraft(
  state: AppViewState,
  id: string,
  updates: { title?: string; body?: string; platform?: string; tags?: string[] },
): Promise<boolean> {
  if (!state.client || !state.connected) return false;
  try {
    await state.client.request("media.drafts.update", { id, ...updates });
    state.mediaDraftEdit = null;
    await loadDraftsList(state);
    return true;
  } catch {
    return false;
  }
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
  if (!state.client || !state.connected) return;
  try {
    await state.client.request("media.config.update", patch);
    await loadMediaConfig(state);
  } catch (err) {
    console.error("updateMediaConfig failed", err);
  }
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
  if (!state.client || !state.connected) return;
  try {
    await state.client.request("media.tools.toggle", { tool, enabled });
    await loadMediaConfig(state);
  } catch (err) {
    console.error("toggleMediaTool failed", err);
    // 失败后强制重渲染，回滚 checkbox 视觉状态
    state.requestUpdate();
  }
}

export async function toggleMediaSource(state: AppViewState, source: string, enabled: boolean): Promise<void> {
  if (!state.client || !state.connected) return;
  try {
    await state.client.request("media.sources.toggle", { source, enabled });
    await loadMediaConfig(state);
  } catch (err) {
    console.error("toggleMediaSource failed", err);
    state.requestUpdate();
  }
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
