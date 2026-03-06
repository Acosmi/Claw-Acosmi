// views/media-dashboard.ts — 媒体运营仪表盘视图
// 配置面板 + 三面板布局: 热点趋势 | 内容草稿 | 发布状态 + 详情弹窗

import { html, nothing } from "lit";
import type { TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import type { MediaHeartbeatStatus } from "../app-view-state.ts";
import { t } from "../i18n.ts";
import {
  loadTrendingTopics,
  loadDraftsList,
  loadPublishHistory,
  loadDraftDetail,
  loadPublishDetail,
  closeDraftDetail,
  closePublishDetail,
  deleteDraft,
  approveDraft,
  updateDraft,
  openDraftEdit,
  closeDraftEdit,
  loadMediaConfig,
  updateMediaConfig,
  loadMediaPatrolJobs,
  checkTrendingSourceHealth,
  type TrendingTopic,
  type DraftEntry,
  type PublishRecord,
  type MediaConfig,
  type MediaToolInfo,
  type MediaSourceInfo,
  type CronPatrolJob,
  type SourceHealthInfo,
} from "../controllers/media-dashboard.ts";

export function renderMediaDashboard(state: AppViewState): TemplateResult {
  return html`
    <div style="display:flex;flex-direction:column;gap:20px;padding:0 4px;">
      ${renderConfigPanel(state)}
      ${renderPatrolPanel(state)}
      ${renderProgressBanner(state)}
      ${renderHeartbeatPanel(state.mediaHeartbeat)}
      ${renderTrendingPanel(state)}
      ${renderDraftsPanel(state)}
      ${renderPublishPanel(state)}
      ${renderDraftDetailModal(state)}
      ${renderPublishDetailModal(state)}
      ${renderDraftEditModal(state)}
    </div>
  `;
}

// ---------- 配置面板 ----------

const TOOL_ICONS: Record<string, string> = {
  trending_topics: "📊",
  content_compose: "✍️",
  media_publish: "🚀",
  social_interact: "💬",
};

const TOOL_LABELS: Record<string, string> = {
  trending_topics: "热点发现",
  content_compose: "内容创作",
  media_publish: "多平台发布",
  social_interact: "社交互动",
};

const SOURCE_ICONS: Record<string, string> = {
  weibo: "🔴",
  baidu: "🔵",
  zhihu: "🟢",
};

export function renderConfigPanel(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;

  if (!config) {
    return html`
      <div class="card" style="border-left:3px solid var(--accent);">
        <div class="card-body" style="padding:12px 16px;">
          <div style="display:flex;align-items:center;gap:10px;">
            <span style="font-size:18px;">🤖</span>
            <div>
              <div style="font-size:14px;font-weight:600;">oa-media 媒体运营智能体</div>
              <div style="font-size:12px;color:var(--text-muted);margin-top:2px;">加载配置中...</div>
            </div>
            <button
              class="btn btn-sm"
              style="margin-left:auto;"
              @click=${() => { loadMediaConfig(state); (state as any).requestUpdate?.(); }}
            >
              刷新配置
            </button>
          </div>
        </div>
      </div>
    `;
  }

  const isConfigured = config.status === "configured";
  const statusDot = isConfigured ? "background:#22c55e;" : "background:#f59e0b;";
  const statusText = isConfigured ? "已配置" : "默认配置";

  return html`
    <div class="card" style="border-left:3px solid ${isConfigured ? "var(--accent)" : "#f59e0b"};">
      <div class="card-body" style="padding:16px;">
        <!-- Agent Header -->
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:14px;">
          <span style="font-size:24px;">🤖</span>
          <div style="flex:1;">
            <div style="font-size:15px;font-weight:600;">${config.label}</div>
            <div style="display:flex;gap:8px;align-items:center;margin-top:4px;">
              <div style="width:8px;height:8px;border-radius:50%;${statusDot}"></div>
              <span style="font-size:12px;color:var(--text-muted);">${statusText}</span>
              <span class="pill" style="font-size:10px;">${config.agent_id}</span>
              ${config.publish_configured
      ? html`<span style="font-size:10px;padding:2px 6px;border-radius:8px;background:#22c55e22;color:#22c55e;">发布已配置</span>`
      : html`<span style="font-size:10px;padding:2px 6px;border-radius:8px;background:#f59e0b22;color:#f59e0b;">发布未配置</span>`}
            </div>
          </div>
          <button
            class="btn btn-sm"
            @click=${() => { loadMediaConfig(state); (state as any).requestUpdate?.(); }}
          >
            刷新配置
          </button>
        </div>

        <!-- Trending Sources 热点来源配置 -->
        <div style="margin-bottom:14px;padding-bottom:14px;border-bottom:1px solid var(--bg-tertiary);">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px;">
            <span style="font-size:12px;font-weight:600;color:var(--text-secondary);">🔥 热点来源配置</span>
            <button
              class="btn btn-sm"
              style="font-size:10px;"
              @click=${() => { checkTrendingSourceHealth(state); (state as any).requestUpdate?.(); }}
            >
              ${state.mediaTrendingHealthLoading ? "检测中..." : "🩺 全部检测"}
            </button>
          </div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:8px;">
            ${config.trending_sources.map((s: MediaSourceInfo) => {
        const health = (state.mediaTrendingHealth || []).find((h: SourceHealthInfo) => h.name === s.name);
        const isOk = health?.status === "ok";
        const isError = health?.status === "error";
        const borderColor = isOk ? "#22c55e" : isError ? "#ef4444" : "var(--bg-tertiary)";
        return html`
              <div style="padding:10px 12px;border-radius:8px;background:var(--bg-secondary);border-left:3px solid ${borderColor};">
                <div style="display:flex;align-items:center;gap:6px;margin-bottom:6px;">
                  <span style="font-size:16px;">${SOURCE_ICONS[s.name] || "📡"}</span>
                  <span style="font-size:12px;font-weight:600;flex:1;">${s.name}</span>
                  ${isOk
            ? html`<span style="font-size:9px;padding:1px 6px;border-radius:6px;background:#22c55e22;color:#22c55e;">正常</span>`
            : isError
              ? html`<span style="font-size:9px;padding:1px 6px;border-radius:6px;background:#ef444422;color:#ef4444;">异常</span>`
              : html`<span style="font-size:9px;padding:1px 6px;border-radius:6px;background:#6b728022;color:#6b7280;">待检测</span>`}
                </div>
                ${health?.error ? html`
                  <div style="font-size:10px;color:#ef4444;line-height:1.3;word-break:break-all;margin-bottom:4px;" title=${health.error}>
                    ⚠ ${health.error.length > 60 ? health.error.substring(0, 60) + "..." : health.error}
                  </div>
                ` : nothing}
                <div style="font-size:10px;color:var(--text-muted);">
                  ${health ? (isOk ? `✓ 返回 ${health.count} 条` : "✗ 连接失败") : "点击「全部检测」查看状态"}
                </div>
              </div>
            `;
      })}
          </div>
          ${config.trending_sources.length === 0 ? html`
            <div style="font-size:12px;color:var(--text-muted);text-align:center;padding:12px;">
              暂无热点来源（系统启动时自动注册微博/百度/知乎）
            </div>
          ` : nothing}
        </div>

        <!-- Tools -->
        <div style="margin-bottom:${config.publishers.length > 0 ? "12px" : "0"};">
          <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:8px;">工具集</div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:8px;">
            ${config.tools.map((tool: MediaToolInfo) => html`
              <div style="padding:10px 12px;border-radius:8px;background:var(--bg-secondary);display:flex;align-items:flex-start;gap:8px;">
                <span style="font-size:16px;flex-shrink:0;margin-top:1px;">${TOOL_ICONS[tool.name] || "🔧"}</span>
                <div style="flex:1;min-width:0;">
                  <div style="display:flex;align-items:center;gap:6px;">
                    <span style="font-size:12px;font-weight:600;">${TOOL_LABELS[tool.name] || tool.name}</span>
                    ${tool.status === "configured"
          ? html`<span style="font-size:9px;padding:1px 5px;border-radius:6px;background:#22c55e22;color:#22c55e;">已配置</span>`
          : html`<span style="font-size:9px;padding:1px 5px;border-radius:6px;background:#f59e0b22;color:#f59e0b;">已注册</span>`}
                  </div>
                  <div style="font-size:11px;color:var(--text-muted);margin-top:3px;line-height:1.3;overflow:hidden;text-overflow:ellipsis;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;">${tool.description}</div>
                </div>
              </div>
            `)}
          </div>
        </div>

        <!-- Publishers -->
        ${config.publishers.length > 0 ? html`
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:8px;">已注册发布器</div>
            <div style="display:flex;gap:6px;flex-wrap:wrap;">
              ${config.publishers.map((p: string) => html`<span class="pill" style="font-size:11px;">${p}</span>`)}
            </div>
          </div>
        ` : nothing}

        <!-- LLM 配置 -->
        <div style="margin-top:14px;padding-top:14px;border-top:1px solid var(--bg-tertiary);">
          <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:10px;">🧠 LLM 模型配置</div>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;">
            <label style="display:flex;flex-direction:column;gap:4px;">
              <span style="font-size:11px;color:var(--text-muted);">Provider</span>
              <select
                .value=${config.llm?.provider || ""}
                @change=${(e: Event) => {
      updateMediaConfig(state, { provider: (e.target as HTMLSelectElement).value });
      (state as any).requestUpdate?.();
    }}
                style="padding:6px 8px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:12px;background:var(--bg-secondary);color:var(--text-primary);"
              >
                <option value="">未配置</option>
                <option value="deepseek">DeepSeek</option>
                <option value="anthropic">Anthropic</option>
                <option value="openai">OpenAI</option>
                <option value="zhipu">Zhipu (智谱)</option>
                <option value="qwen">Qwen (通义千问)</option>
              </select>
            </label>
            <label style="display:flex;flex-direction:column;gap:4px;">
              <span style="font-size:11px;color:var(--text-muted);">Model</span>
              <input
                type="text"
                .value=${config.llm?.model || ""}
                placeholder="deepseek-chat"
                @change=${(e: Event) => {
      updateMediaConfig(state, { model: (e.target as HTMLInputElement).value });
      (state as any).requestUpdate?.();
    }}
                style="padding:6px 8px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:12px;background:var(--bg-secondary);color:var(--text-primary);"
              />
            </label>
            <label style="display:flex;flex-direction:column;gap:4px;">
              <span style="font-size:11px;color:var(--text-muted);">API Key</span>
              <input
                type="password"
                .value=${config.llm?.apiKey || ""}
                placeholder="sk-..."
                @change=${(e: Event) => {
      const val = (e.target as HTMLInputElement).value;
      if (val && !val.includes("****")) {
        updateMediaConfig(state, { apiKey: val });
        (state as any).requestUpdate?.();
      }
    }}
                style="padding:6px 8px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:12px;background:var(--bg-secondary);color:var(--text-primary);"
              />
            </label>
            <label style="display:flex;flex-direction:column;gap:4px;">
              <span style="font-size:11px;color:var(--text-muted);">Base URL (可选)</span>
              <input
                type="text"
                .value=${config.llm?.baseUrl || ""}
                placeholder="https://api.deepseek.com"
                @change=${(e: Event) => {
      updateMediaConfig(state, { baseUrl: (e.target as HTMLInputElement).value });
      (state as any).requestUpdate?.();
    }}
                style="padding:6px 8px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:12px;background:var(--bg-secondary);color:var(--text-primary);"
              />
            </label>
          </div>
        </div>

        <!-- 自动执行配置 -->
        <div style="margin-top:10px;display:flex;align-items:center;gap:12px;">
          <label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
            <input
              type="checkbox"
              .checked=${config.llm?.autoSpawnEnabled || false}
              @change=${(e: Event) => {
      updateMediaConfig(state, { autoSpawnEnabled: (e.target as HTMLInputElement).checked });
      (state as any).requestUpdate?.();
    }}
            />
            <span style="font-size:12px;">自动 Spawn</span>
          </label>
          <label style="display:flex;align-items:center;gap:4px;">
            <span style="font-size:11px;color:var(--text-muted);">每日上限</span>
            <input
              type="number"
              min="1" max="50"
              .value=${String(config.llm?.maxAutoSpawnsPerDay || 5)}
              @change=${(e: Event) => {
      updateMediaConfig(state, { maxAutoSpawnsPerDay: Number((e.target as HTMLInputElement).value) });
      (state as any).requestUpdate?.();
    }}
              style="width:50px;padding:4px 6px;border:1px solid var(--bg-tertiary);border-radius:4px;font-size:12px;background:var(--bg-secondary);color:var(--text-primary);text-align:center;"
            />
          </label>
        </div>
      </div>
    </div>
  `;
}
// ---------- 巡检任务面板 ----------

const PATROL_LABELS: Record<string, string> = {
  "media.patrol.trending": "🔍 热点监控",
  "media.patrol.publish": "📤 发布跟踪",
  "media.patrol.interact": "💬 互动巡检",
};

function formatInterval(ms: number): string {
  const h = Math.floor(ms / 3600000);
  const m = Math.floor((ms % 3600000) / 60000);
  if (h > 0 && m > 0) return `${h}h${m}m`;
  if (h > 0) return `${h}h`;
  return `${m}m`;
}

function formatTime(ms?: number): string {
  if (!ms) return "—";
  return new Date(ms).toLocaleString();
}

export function renderPatrolPanel(state: AppViewState): TemplateResult | typeof nothing {
  const jobs = state.mediaPatrolJobs || [];
  if (jobs.length === 0) return nothing;

  return html`
    <div class="card">
      <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
        <span style="font-size:13px;font-weight:600;">⏱ 自动巡检任务</span>
        <button
          class="btn btn-sm"
          @click=${() => { loadMediaPatrolJobs(state); (state as any).requestUpdate?.(); }}
        >
          刷新
        </button>
      </div>
      <div class="card-body" style="padding:0;">
        ${jobs.map((job: CronPatrolJob) => {
    const label = PATROL_LABELS[job.name] || job.name;
    const statusColor = job.state.lastStatus === "ok" ? "#22c55e"
      : job.state.lastStatus === "error" ? "#ef4444"
        : "#6b7280";
    const statusText = job.state.lastStatus === "ok" ? "正常"
      : job.state.lastStatus === "error" ? "错误"
        : job.state.lastStatus || "未运行";
    const interval = job.schedule?.everyMs ? formatInterval(job.schedule.everyMs) : "—";

    return html`
            <div style="display:flex;align-items:center;gap:12px;padding:10px 16px;border-bottom:1px solid var(--bg-tertiary);">
              <div style="flex:1;min-width:0;">
                <div style="display:flex;align-items:center;gap:8px;">
                  <span style="font-size:13px;font-weight:500;">${label}</span>
                  ${job.enabled
        ? html`<span style="font-size:9px;padding:1px 5px;border-radius:6px;background:#22c55e22;color:#22c55e;">启用</span>`
        : html`<span style="font-size:9px;padding:1px 5px;border-radius:6px;background:#6b728022;color:#6b7280;">禁用</span>`}
                  <span style="font-size:10px;color:var(--text-muted);">间隔 ${interval}</span>
                </div>
                <div style="font-size:11px;color:var(--text-muted);margin-top:3px;">
                  ${job.description}
                </div>
              </div>
              <div style="display:flex;flex-direction:column;align-items:flex-end;gap:2px;flex-shrink:0;">
                <span style="font-size:9px;padding:1px 6px;border-radius:6px;background:${statusColor}22;color:${statusColor};">${statusText}</span>
                <span style="font-size:10px;color:var(--text-muted);">上次: ${formatTime(job.state.lastRunAtMs)}</span>
                <span style="font-size:10px;color:var(--text-muted);">下次: ${formatTime(job.state.nextRunAtMs)}</span>
              </div>
            </div>
          `;
  })}
      </div>
    </div>
  `;
}

// ---------- 进度横幅 ----------

export function renderProgressBanner(state: AppViewState): TemplateResult | typeof nothing {
  const progress = state.agentProgress;
  if (!progress) return nothing;
  const percent = progress.percent;
  const phase = progress.phase;
  const elapsed = Math.round((Date.now() - progress.ts) / 1000);
  const stale = elapsed > 120; // >2min 视为过期
  if (stale) return nothing;
  return html`
    <div class="card" style="border-left:3px solid var(--accent);background:var(--bg-secondary);">
      <div class="card-body" style="padding:10px 14px;">
        <div style="display:flex;align-items:center;gap:10px;">
          <span style="font-size:14px;">&#9881;</span>
          <div style="flex:1;min-width:0;">
            <div style="font-size:13px;font-weight:500;">${progress.summary}</div>
            ${phase ? html`<div style="font-size:11px;color:var(--text-muted);margin-top:2px;">${phase}</div>` : nothing}
          </div>
          ${percent != null && percent > 0 ? html`<span style="font-size:12px;font-weight:600;color:var(--accent);white-space:nowrap;">${percent}%</span>` : nothing}
        </div>
        ${percent != null && percent > 0 ? html`
          <div style="margin-top:6px;height:4px;border-radius:2px;background:var(--bg-tertiary);overflow:hidden;">
            <div style="height:100%;width:${Math.min(percent, 100)}%;background:var(--accent);border-radius:2px;transition:width 0.3s ease;"></div>
          </div>
        ` : nothing}
      </div>
    </div>
  `;
}

// ---------- 智能体心跳面板 ----------

export function renderHeartbeatPanel(hb: MediaHeartbeatStatus | null): TemplateResult | typeof nothing {
  if (!hb) return nothing;

  const isRunning = hb.activeJobId != null;
  const hasError = hb.lastError != null;

  // 状态指示
  let statusDot: string;
  let statusText: string;
  if (isRunning) {
    statusDot = "background:#3b82f6;animation:pulse 1.5s infinite;";
    statusText = t("media.heartbeat.running");
  } else if (hasError) {
    statusDot = "background:#ef4444;";
    statusText = t("media.heartbeat.error");
  } else {
    statusDot = "background:#22c55e;";
    statusText = t("media.heartbeat.normal");
  }

  // 上次巡检相对时间
  const lastStr = hb.lastPatrolAt ? formatRelativeTime(hb.lastPatrolAt) : "--";

  return html`
    <div class="card" style="border-left:3px solid ${isRunning ? "#3b82f6" : hasError ? "#ef4444" : "#22c55e"};">
      <div class="card-body" style="padding:10px 14px;">
        <div style="display:flex;align-items:center;gap:10px;">
          <div style="width:8px;height:8px;border-radius:50%;flex-shrink:0;${statusDot}"></div>
          <span style="font-size:13px;font-weight:600;">${t("media.heartbeat.title")}</span>
          <span style="font-size:12px;color:var(--text-muted);margin-left:auto;">${statusText}</span>
        </div>
        <div style="display:flex;gap:16px;margin-top:6px;font-size:12px;color:var(--text-secondary);">
          <span>${t("media.heartbeat.lastPatrol")}: ${lastStr}</span>
          ${hb.autoSpawnCount != null && hb.autoSpawnCount > 0
      ? html`<span>${t("media.heartbeat.autoSpawnCount")}: ${hb.autoSpawnCount}</span>`
      : nothing}
          ${hasError ? html`<span style="color:#ef4444;">${hb.lastError}</span>` : nothing}
        </div>
      </div>
    </div>
  `;
}

function formatRelativeTime(ts: number): string {
  const diff = Math.floor((Date.now() - ts) / 1000);
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`;
  return `${Math.floor(diff / 86400)}d`;
}

// ---------- 热点趋势面板 ----------

export function renderTrendingPanel(state: AppViewState): TemplateResult {
  const topics = state.mediaTrendingTopics || [];
  const sources = state.mediaTrendingSources || [];
  const loading = state.mediaTrendingLoading || false;
  const selectedSource = state.mediaTrendingSelectedSource || "";

  return html`
    <div class="card">
      <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
        <h3 style="margin:0;font-size:15px;font-weight:600;">${t("media.trending.title")}</h3>
        <div style="display:flex;gap:8px;align-items:center;">
          ${sources.length > 0
      ? html`
                <select
                  class="input"
                  style="font-size:12px;padding:4px 8px;"
                  .value=${selectedSource}
                  @change=${(e: Event) => {
          const val = (e.target as HTMLSelectElement).value;
          state.mediaTrendingSelectedSource = val;
          (state as any).requestUpdate?.();
        }}
                >
                  <option value="">${t("media.trending.allSources")}</option>
                  ${sources.map((s: string) => html`<option value=${s}>${s}</option>`)}
                </select>
              `
      : nothing}
          <button
            class="btn btn-sm"
            ?disabled=${loading}
            @click=${() => {
      loadTrendingTopics(state, selectedSource || undefined);
      (state as any).requestUpdate?.();
    }}
          >
            ${loading ? t("media.trending.fetching") : t("media.trending.fetch")}
          </button>
        </div>
      </div>
      <div class="card-body" style="max-height:400px;overflow-y:auto;">
        ${topics.length === 0
      ? html`<p class="empty-hint">${t("media.trending.empty")}</p>`
      : html`
              <div style="display:flex;flex-direction:column;gap:4px;">
                ${topics.map((topic: TrendingTopic) => renderTrendingItem(topic))}
              </div>
            `}
      </div>
    </div>
  `;
}

function renderTrendingItem(topic: TrendingTopic): TemplateResult {
  const heatStr = formatHeatScore(topic.heat_score);
  return html`
    <div
      class="list-item"
      style="display:flex;justify-content:space-between;align-items:center;padding:8px 12px;border-radius:6px;"
    >
      <div style="flex:1;min-width:0;">
        ${topic.url
      ? html`<a
              href=${topic.url}
              target="_blank"
              rel="noopener"
              style="color:var(--text-primary);text-decoration:none;font-size:13px;"
              >${topic.title}</a
            >`
      : html`<span style="font-size:13px;">${topic.title}</span>`}
        <span class="pill" style="margin-left:8px;font-size:10px;">${topic.source}</span>
      </div>
      <span style="font-size:12px;color:var(--text-muted);white-space:nowrap;">
        ${heatStr}
      </span>
    </div>
  `;
}

function formatHeatScore(score: number): string {
  if (score >= 10000) {
    return (score / 10000).toFixed(1) + "万";
  }
  if (score >= 1000) {
    return (score / 1000).toFixed(1) + "k";
  }
  return String(Math.round(score));
}

// ---------- 内容草稿面板 ----------

export function renderDraftsPanel(state: AppViewState): TemplateResult {
  const drafts = state.mediaDrafts || [];
  const loading = state.mediaDraftsLoading || false;
  const selectedPlatform = state.mediaDraftsSelectedPlatform || "";

  return html`
    <div class="card">
      <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
        <h3 style="margin:0;font-size:15px;font-weight:600;">${t("media.drafts.title")}</h3>
        <div style="display:flex;gap:8px;align-items:center;">
          <select
            class="input"
            style="font-size:12px;padding:4px 8px;"
            .value=${selectedPlatform}
            @change=${(e: Event) => {
      const val = (e.target as HTMLSelectElement).value;
      state.mediaDraftsSelectedPlatform = val;
      loadDraftsList(state, val || undefined);
      (state as any).requestUpdate?.();
    }}
          >
            <option value="">${t("media.drafts.allPlatforms")}</option>
            <option value="xiaohongshu">小红书</option>
            <option value="wechat">微信</option>
            <option value="website">Website</option>
          </select>
        </div>
      </div>
      <div class="card-body" style="max-height:400px;overflow-y:auto;">
        ${loading
      ? html`<p class="empty-hint">Loading…</p>`
      : drafts.length === 0
        ? html`<p class="empty-hint">${t("media.drafts.empty")}</p>`
        : html`
                <div style="display:flex;flex-direction:column;gap:4px;">
                  ${drafts.map((d: DraftEntry) => renderDraftItem(state, d))}
                </div>
              `}
      </div>
    </div>
  `;
}

function renderDraftItem(state: AppViewState, draft: DraftEntry): TemplateResult {
  const statusColor =
    draft.status === "published"
      ? "var(--green)"
      : draft.status === "approved"
        ? "var(--blue)"
        : draft.status === "pending_review"
          ? "#f59e0b"
          : "var(--text-muted)";
  return html`
    <div
      class="list-item"
      style="display:flex;justify-content:space-between;align-items:center;padding:8px 12px;border-radius:6px;cursor:pointer;transition:background 0.15s;"
      @click=${() => {
      loadDraftDetail(state, draft.id);
      (state as any).requestUpdate?.();
    }}
    >
      <div style="flex:1;min-width:0;">
        <span style="font-size:13px;font-weight:500;">${draft.title || "(untitled)"}</span>
        <span class="pill" style="margin-left:8px;font-size:10px;">${draft.platform}</span>
        <span style="margin-left:6px;font-size:11px;color:${statusColor};">${draft.status}</span>
      </div>
      <div style="display:flex;gap:6px;">
        <button
          class="btn btn-sm"
          style="font-size:11px;"
          @click=${(e: Event) => {
      e.stopPropagation();
      openDraftEdit(state, draft);
      (state as any).requestUpdate?.();
    }}
        >
          ✎ 编辑
        </button>
        ${draft.status === "pending_review" ? html`
          <button
            class="btn btn-sm"
            style="font-size:11px;background:#22c55e;color:#fff;border:none;"
            @click=${(e: Event) => {
        e.stopPropagation();
        approveDraft(state, draft.id);
        (state as any).requestUpdate?.();
      }}
          >
            ✓ 审批
          </button>
        ` : nothing}
        <button
          class="btn btn-sm btn-danger"
          style="font-size:11px;"
          @click=${(e: Event) => {
      e.stopPropagation();
      if (!confirm(t("media.drafts.deleteConfirm"))) return;
      deleteDraft(state, draft.id);
      (state as any).requestUpdate?.();
    }}
        >
          ${t("media.drafts.delete")}
        </button>
      </div>
    </div>
  `;
}

// ---------- 发布状态面板 ----------

export function renderPublishPanel(state: AppViewState): TemplateResult {
  const records = (state.mediaPublishRecords || []) as PublishRecord[];
  const loading = state.mediaPublishLoading || false;
  const page = state.mediaPublishPage || 0;
  const pageSize = state.mediaPublishPageSize || 10;
  const hasPrev = page > 0;
  const hasNext = records.length === pageSize;

  const loadPage = (newPage: number) => {
    state.mediaPublishPage = newPage;
    loadPublishHistory(state, { limit: pageSize, offset: newPage * pageSize });
    (state as any).requestUpdate?.();
  };

  return html`
    <div class="card">
      <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
        <h3 style="margin:0;font-size:15px;font-weight:600;">${t("media.publish.title")}</h3>
        <div style="display:flex;gap:8px;align-items:center;">
          <select
            class="input"
            style="font-size:12px;padding:4px 8px;"
            .value=${String(pageSize)}
            @change=${(e: Event) => {
      state.mediaPublishPageSize = parseInt((e.target as HTMLSelectElement).value, 10);
      state.mediaPublishPage = 0;
      loadPublishHistory(state, { limit: state.mediaPublishPageSize, offset: 0 });
      (state as any).requestUpdate?.();
    }}
          >
            <option value="5">5/页</option>
            <option value="10">10/页</option>
            <option value="20">20/页</option>
            <option value="50">50/页</option>
          </select>
          <button
            class="btn btn-sm"
            ?disabled=${loading}
            @click=${() => loadPage(page)}
          >
            ${loading ? "..." : t("media.refreshStatus")}
          </button>
        </div>
      </div>
      <div class="card-body">
        ${records.length === 0
      ? html`<p class="empty-hint">${t("media.publish.empty")}</p>`
      : html`
              <div style="display:flex;flex-direction:column;gap:8px;">
                ${records.map(
        (r) => html`
                    <div
                      class="list-item"
                      style="display:flex;justify-content:space-between;align-items:center;padding:8px 12px;border-radius:6px;background:var(--bg-secondary);cursor:pointer;transition:background 0.15s;"
                      @click=${() => {
            loadPublishDetail(state, r.id);
            (state as any).requestUpdate?.();
          }}
                    >
                      <div style="flex:1;min-width:0;">
                        <div style="font-weight:500;font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
                          ${r.title}
                        </div>
                        <div style="font-size:11px;color:var(--text-tertiary);margin-top:2px;">
                          ${r.platform} &middot; ${r.status}
                          ${r.published_at
            ? ` &middot; ${new Date(r.published_at).toLocaleString()}`
            : ""}
                        </div>
                      </div>
                      ${r.url
            ? html`<a
                            href=${r.url}
                            target="_blank"
                            rel="noopener"
                            style="font-size:12px;color:var(--accent);text-decoration:none;margin-left:8px;"
                            @click=${(e: Event) => e.stopPropagation()}
                            >${t("media.publish.viewLink")}</a
                          >`
            : nothing}
                    </div>
                  `,
      )}
              </div>
            `}
        ${(hasPrev || hasNext) ? html`
          <div style="display:flex;justify-content:space-between;align-items:center;margin-top:12px;padding-top:8px;border-top:1px solid var(--bg-tertiary);">
            <button class="btn btn-sm" ?disabled=${!hasPrev || loading} @click=${() => loadPage(page - 1)}>← 上一页</button>
            <span style="font-size:12px;color:var(--text-muted);">Page ${page + 1}</span>
            <button class="btn btn-sm" ?disabled=${!hasNext || loading} @click=${() => loadPage(page + 1)}>下一页 →</button>
          </div>
        ` : nothing}
      </div>
    </div>
  `;
}

// ---------- 草稿详情弹窗 ----------

const STATUS_FLOW = ["draft", "pending_review", "approved", "published"] as const;
const STATUS_LABELS: Record<string, string> = {
  draft: "草稿",
  pending_review: "待审批",
  approved: "已审批",
  published: "已发布",
};
const STATUS_COLORS: Record<string, string> = {
  draft: "#6b7280",
  pending_review: "#f59e0b",
  approved: "#3b82f6",
  published: "#22c55e",
};

export function renderDraftDetailModal(state: AppViewState): TemplateResult | typeof nothing {
  const draft = state.mediaDraftDetail;
  if (!draft) return nothing;
  const loading = state.mediaDraftDetailLoading;

  if (loading) {
    return html`
      <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${() => { closeDraftDetail(state); (state as any).requestUpdate?.(); }}>
        <div class="modal-content" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
          <div style="padding:40px;text-align:center;color:var(--text-muted);">Loading…</div>
        </div>
      </div>
    `;
  }

  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${() => { closeDraftDetail(state); (state as any).requestUpdate?.(); }}>
      <div class="modal-content" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <!-- Header -->
        <div style="display:flex;justify-content:space-between;align-items:flex-start;padding:20px 24px 0;">
          <div style="flex:1;min-width:0;">
            <h2 style="margin:0;font-size:18px;font-weight:600;">${draft.title || "(untitled)"}</h2>
            <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap;align-items:center;">
              <span class="pill" style="font-size:11px;">${draft.platform}</span>
              ${draft.style ? html`<span class="pill" style="font-size:11px;">${draft.style}</span>` : nothing}
              <span style="font-size:11px;padding:2px 8px;border-radius:10px;color:#fff;background:${STATUS_COLORS[draft.status] || "#6b7280"};">
                ${STATUS_LABELS[draft.status] || draft.status}
              </span>
            </div>
          </div>
          <button
            style="border:none;background:none;font-size:20px;cursor:pointer;color:var(--text-muted);padding:0 4px;"
            @click=${() => { closeDraftDetail(state); (state as any).requestUpdate?.(); }}
          >&times;</button>
        </div>

        <!-- Status Flow -->
        <div style="padding:12px 24px;">
          <div style="display:flex;align-items:center;gap:4px;">
            ${STATUS_FLOW.map((s, i) => {
    const idx = STATUS_FLOW.indexOf(draft.status as typeof STATUS_FLOW[number]);
    const active = i <= idx;
    return html`
                ${i > 0 ? html`<div style="flex:1;height:2px;background:${active ? STATUS_COLORS[s] : "var(--bg-tertiary)"};"></div>` : nothing}
                <div style="display:flex;flex-direction:column;align-items:center;">
                  <div style="width:10px;height:10px;border-radius:50%;background:${active ? STATUS_COLORS[s] : "var(--bg-tertiary)"};border:2px solid ${active ? STATUS_COLORS[s] : "var(--bg-tertiary)"};"></div>
                  <span style="font-size:10px;color:${active ? STATUS_COLORS[s] : "var(--text-muted)"};margin-top:4px;white-space:nowrap;">${STATUS_LABELS[s]}</span>
                </div>
              `;
  })}
          </div>
        </div>

        <!-- Body -->
        <div style="padding:0 24px 16px;">
          ${draft.body
      ? html`<div style="font-size:13px;line-height:1.7;white-space:pre-wrap;background:var(--bg-secondary);padding:12px 16px;border-radius:8px;max-height:300px;overflow-y:auto;">${draft.body}</div>`
      : html`<p style="color:var(--text-muted);font-size:13px;">无正文内容</p>`}
        </div>

        <!-- Tags -->
        ${draft.tags && draft.tags.length > 0 ? html`
          <div style="padding:0 24px 12px;">
            <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:6px;">标签</div>
            <div style="display:flex;flex-wrap:wrap;gap:6px;">
              ${draft.tags.map((tag: string) => html`<span class="pill" style="font-size:11px;">#${tag}</span>`)}
            </div>
          </div>
        ` : nothing}

        <!-- Images -->
        ${draft.images && draft.images.length > 0 ? html`
          <div style="padding:0 24px 12px;">
            <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:6px;">图片 (${draft.images.length})</div>
            <div style="display:flex;gap:8px;flex-wrap:wrap;">
              ${draft.images.map((url: string) => html`
                <img src=${url} style="width:80px;height:80px;object-fit:cover;border-radius:6px;border:1px solid var(--bg-tertiary);" alt="draft image" />
              `)}
            </div>
          </div>
        ` : nothing}

        <!-- Meta -->
        <div style="padding:12px 24px 20px;border-top:1px solid var(--bg-tertiary);display:flex;gap:16px;font-size:11px;color:var(--text-muted);">
          <span>创建: ${new Date(draft.created_at).toLocaleString()}</span>
          <span>更新: ${new Date(draft.updated_at).toLocaleString()}</span>
          <span style="margin-left:auto;">ID: ${draft.id}</span>
        </div>
      </div>
    </div>
  `;
}

// ---------- 发布详情弹窗 ----------

export function renderPublishDetailModal(state: AppViewState): TemplateResult | typeof nothing {
  const record = state.mediaPublishDetail;
  if (!record) return nothing;
  const loading = state.mediaPublishDetailLoading;

  if (loading) {
    return html`
      <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${() => { closePublishDetail(state); (state as any).requestUpdate?.(); }}>
        <div class="modal-content" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
          <div style="padding:40px;text-align:center;color:var(--text-muted);">Loading…</div>
        </div>
      </div>
    `;
  }

  const statusColor = record.status === "published" ? "#22c55e" : record.status === "failed" ? "#ef4444" : "#f59e0b";

  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${() => { closePublishDetail(state); (state as any).requestUpdate?.(); }}>
      <div class="modal-content" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <div style="padding:20px 24px;">
          <div style="display:flex;justify-content:space-between;align-items:flex-start;">
            <div>
              <h2 style="margin:0;font-size:18px;font-weight:600;">${record.title || "(untitled)"}</h2>
              <div style="display:flex;gap:8px;margin-top:8px;align-items:center;">
                <span class="pill" style="font-size:11px;">${record.platform}</span>
                <span style="font-size:11px;padding:2px 8px;border-radius:10px;color:#fff;background:${statusColor};">${record.status}</span>
              </div>
            </div>
            <button
              style="border:none;background:none;font-size:20px;cursor:pointer;color:var(--text-muted);padding:0 4px;"
              @click=${() => { closePublishDetail(state); (state as any).requestUpdate?.(); }}
            >&times;</button>
          </div>

          <div style="margin-top:16px;display:flex;flex-direction:column;gap:10px;">
            ${record.post_id ? html`
              <div style="display:flex;align-items:center;gap:8px;">
                <span style="font-size:12px;font-weight:600;color:var(--text-secondary);min-width:80px;">Post ID</span>
                <span style="font-size:13px;font-family:monospace;">${record.post_id}</span>
              </div>
            ` : nothing}
            ${record.url ? html`
              <div style="display:flex;align-items:center;gap:8px;">
                <span style="font-size:12px;font-weight:600;color:var(--text-secondary);min-width:80px;">链接</span>
                <a href=${record.url} target="_blank" rel="noopener" style="font-size:13px;color:var(--accent);">${record.url}</a>
              </div>
            ` : nothing}
            ${record.published_at ? html`
              <div style="display:flex;align-items:center;gap:8px;">
                <span style="font-size:12px;font-weight:600;color:var(--text-secondary);min-width:80px;">发布时间</span>
                <span style="font-size:13px;">${new Date(record.published_at).toLocaleString()}</span>
              </div>
            ` : nothing}
            <div style="display:flex;align-items:center;gap:8px;">
              <span style="font-size:12px;font-weight:600;color:var(--text-secondary);min-width:80px;">草稿 ID</span>
              <span style="font-size:13px;font-family:monospace;">${record.draft_id}</span>
            </div>
          </div>
        </div>
        <div style="padding:12px 24px 20px;border-top:1px solid var(--bg-tertiary);font-size:11px;color:var(--text-muted);">
          ID: ${record.id}
        </div>
      </div>
    </div>
  `;
}

// ---------- 草稿编辑弹窗 ----------

export function renderDraftEditModal(state: AppViewState): TemplateResult | typeof nothing {
  const draft = state.mediaDraftEdit;
  if (!draft) return nothing;

  return html`
    <div class="modal-overlay" style="${OVERLAY_STYLE}" @click=${() => { closeDraftEdit(state); (state as any).requestUpdate?.(); }}>
      <div class="modal-content" style="${MODAL_STYLE}" @click=${(e: Event) => e.stopPropagation()}>
        <div style="padding:20px 24px 0;display:flex;justify-content:space-between;align-items:center;">
          <h2 style="margin:0;font-size:16px;font-weight:600;">编辑草稿</h2>
          <button
            style="border:none;background:none;font-size:20px;cursor:pointer;color:var(--text-muted);padding:0 4px;"
            @click=${() => { closeDraftEdit(state); (state as any).requestUpdate?.(); }}
          >&times;</button>
        </div>

        <div style="padding:16px 24px;display:flex;flex-direction:column;gap:12px;">
          <label class="field">
            <span style="font-size:12px;font-weight:600;">标题</span>
            <input
              type="text"
              .value=${draft.title || ""}
              @input=${(e: Event) => { draft.title = (e.target as HTMLInputElement).value; }}
              style="width:100%;padding:8px 10px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:13px;background:var(--bg-secondary);color:var(--text-primary);"
            />
          </label>

          <label class="field">
            <span style="font-size:12px;font-weight:600;">正文</span>
            <textarea
              .value=${draft.body || ""}
              @input=${(e: Event) => { draft.body = (e.target as HTMLTextAreaElement).value; }}
              rows="8"
              style="width:100%;padding:8px 10px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:13px;background:var(--bg-secondary);color:var(--text-primary);resize:vertical;font-family:inherit;"
            ></textarea>
          </label>

          <div style="display:flex;gap:12px;">
            <label class="field" style="flex:1;">
              <span style="font-size:12px;font-weight:600;">平台</span>
              <select
                .value=${draft.platform || ""}
                @change=${(e: Event) => { draft.platform = (e.target as HTMLSelectElement).value; }}
                style="width:100%;padding:8px 10px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:13px;background:var(--bg-secondary);color:var(--text-primary);"
              >
                <option value="xiaohongshu">小红书</option>
                <option value="wechat">微信</option>
                <option value="website">Website</option>
              </select>
            </label>
            <label class="field" style="flex:1;">
              <span style="font-size:12px;font-weight:600;">标签 (逗号分隔)</span>
              <input
                type="text"
                .value=${(draft.tags || []).join(", ")}
                @input=${(e: Event) => {
      draft.tags = (e.target as HTMLInputElement).value.split(",").map((t: string) => t.trim()).filter(Boolean);
    }}
                style="width:100%;padding:8px 10px;border:1px solid var(--bg-tertiary);border-radius:6px;font-size:13px;background:var(--bg-secondary);color:var(--text-primary);"
                placeholder="标签1, 标签2"
              />
            </label>
          </div>
        </div>

        <div style="padding:0 24px 20px;display:flex;gap:8px;justify-content:flex-end;">
          <button
            class="btn"
            @click=${() => { closeDraftEdit(state); (state as any).requestUpdate?.(); }}
          >
            取消
          </button>
          <button
            class="btn primary"
            @click=${() => {
      updateDraft(state, draft.id, {
        title: draft.title,
        body: draft.body,
        platform: draft.platform,
        tags: draft.tags,
      });
      (state as any).requestUpdate?.();
    }}
          >
            保存
          </button>
        </div>
      </div>
    </div>
  `;
}

// ---------- 弹窗样式常量 ----------

const OVERLAY_STYLE = "position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.5);display:flex;align-items:center;justify-content:center;z-index:1000;backdrop-filter:blur(2px);";
const MODAL_STYLE = "background:var(--bg-primary);border-radius:12px;max-width:600px;width:90%;max-height:85vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,0.3);";

