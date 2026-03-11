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
  deleteDraft,
  approveDraft,
  openDraftEdit,
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
import {
  renderDraftDetailModal,
  renderDraftEditModal,
  renderPublishDetailModal,
} from "./media-dashboard-modals.ts";

export {
  renderDraftDetailModal,
  renderDraftEditModal,
  renderPublishDetailModal,
} from "./media-dashboard-modals.ts";

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
  bocha: "🟠",
  custom_openai: "🧠",
};

const SOURCE_LABELS: Record<string, string> = {
  weibo: "微博热搜",
  baidu: "百度热搜",
  zhihu: "知乎热榜",
  bocha: "Bocha API 热点",
  custom_openai: "自定义 OpenAI 兼容",
};

type MediaTone = "ok" | "warn" | "danger" | "info" | "muted";

function toneChipClass(tone: MediaTone): string {
  switch (tone) {
    case "ok":
      return "chip-ok";
    case "warn":
      return "chip-warn";
    case "danger":
      return "chip-danger";
    case "info":
      return "media-dashboard__chip--info";
    default:
      return "chip-muted";
  }
}

function renderStatusChip(label: string, tone: MediaTone, dense = false): TemplateResult {
  return html`
    <span class="chip ${toneChipClass(tone)} ${dense ? "media-dashboard__chip--dense" : ""}">
      ${label}
    </span>
  `;
}

function renderPanelBody(content: TemplateResult | typeof nothing): TemplateResult {
  return html`<div class="media-dashboard-card__body">${content}</div>`;
}

export function renderConfigPanel(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;

  if (!config) {
    return html`
      <section class="card media-dashboard-card media-dashboard-card--accent">
        ${renderPanelBody(html`
          <div class="media-dashboard-card__header">
            <div class="media-dashboard-card__hero">
              <span class="media-dashboard-card__icon">🤖</span>
              <div>
                <div class="media-dashboard-card__title">oa-media 媒体运营智能体</div>
                <div class="media-dashboard-card__sub">加载配置中...</div>
              </div>
            </div>
            <button
              class="btn btn--sm"
              @click=${() => { loadMediaConfig(state); (state as any).requestUpdate?.(); }}
            >
              刷新配置
            </button>
          </div>
        `)}
      </section>
    `;
  }

  const isConfigured = config.status === "configured";
  const statusText = isConfigured ? "已配置" : "默认配置";

  return html`
    <section class="card media-dashboard-card ${isConfigured ? "media-dashboard-card--accent" : "media-dashboard-card--warn"}">
      ${renderPanelBody(html`
        <div class="media-dashboard-card__header">
          <div class="media-dashboard-card__hero">
            <span class="media-dashboard-card__icon">🤖</span>
            <div>
              <div class="media-dashboard-card__title">${config.label}</div>
              <div class="media-dashboard-card__sub">${statusText}</div>
              <div class="media-dashboard-card__meta">
                <span class="pill media-dashboard__pill--mono">${config.agent_id}</span>
                ${renderStatusChip(config.publish_configured ? "发布已配置" : "发布未配置", config.publish_configured ? "ok" : "warn", true)}
                ${renderStatusChip(isConfigured ? "模型已接管" : "仍走默认配置", isConfigured ? "info" : "muted", true)}
              </div>
            </div>
          </div>
          <button
            class="btn btn--sm"
            @click=${() => { loadMediaConfig(state); (state as any).requestUpdate?.(); }}
          >
            刷新配置
          </button>
        </div>

        <section class="media-dashboard-section">
          <div class="media-dashboard-section__header">
            <div>
              <div class="media-dashboard-section__title">热点来源配置</div>
              <div class="media-dashboard-section__sub">检查热点抓取通道是否可用，异常时优先先修源。</div>
            </div>
            <button
              class="btn btn--sm"
              @click=${() => { checkTrendingSourceHealth(state); (state as any).requestUpdate?.(); }}
            >
              ${state.mediaTrendingHealthLoading ? "检测中..." : "🩺 全部检测"}
            </button>
          </div>
          <div class="media-dashboard-grid media-dashboard-grid--tight">
            ${config.trending_sources.map((s: MediaSourceInfo) => {
              const health = (state.mediaTrendingHealth || []).find((h: SourceHealthInfo) => h.name === s.name);
              const isOk = health?.status === "ok";
              const isError = health?.status === "error";
              return html`
                <section class="media-dashboard-mini-card ${isOk ? "is-ok" : isError ? "is-danger" : ""}">
                  <div class="media-dashboard-mini-card__head">
                    <div class="media-dashboard-mini-card__title-row">
                      <span class="media-dashboard-mini-card__icon">${SOURCE_ICONS[s.name] || "📡"}</span>
                      <strong>${SOURCE_LABELS[s.name] || s.name}</strong>
                    </div>
                    ${renderStatusChip(isOk ? "正常" : isError ? "异常" : "待检测", isOk ? "ok" : isError ? "danger" : "muted", true)}
                  </div>
                  ${health?.error
                    ? html`
                        <div class="media-dashboard-mini-card__error" title=${health.error}>
                          ⚠ ${health.error.length > 60 ? `${health.error.substring(0, 60)}...` : health.error}
                        </div>
                      `
                    : nothing}
                  <div class="media-dashboard-mini-card__note">
                    ${health
                      ? (isOk ? `返回 ${health.count} 条热点` : "连接失败，建议检查 API Key、配额或站点连通性")
                      : "点击“全部检测”查看连通性"}
                  </div>
                </section>
              `;
            })}
          </div>
          ${config.trending_sources.length === 0
            ? html`<div class="media-dashboard-empty">暂无热点来源（系统启动时自动注册微博 / 百度 / 知乎）</div>`
            : nothing}
        </section>

        <div class="media-dashboard-grid">
          <section class="media-dashboard-section">
            <div class="media-dashboard-section__header">
              <div>
                <div class="media-dashboard-section__title">工具集</div>
                <div class="media-dashboard-section__sub">把热点发现、创作、发布和互动能力串成一条流水线。</div>
              </div>
            </div>
            <div class="media-dashboard-grid media-dashboard-grid--tight">
            ${config.tools.map((tool: MediaToolInfo) => html`
                <section class="media-dashboard-mini-card">
                  <div class="media-dashboard-mini-card__head">
                    <div class="media-dashboard-mini-card__title-row">
                      <span class="media-dashboard-mini-card__icon">${TOOL_ICONS[tool.name] || "🔧"}</span>
                      <strong>${TOOL_LABELS[tool.name] || tool.name}</strong>
                    </div>
                    ${(() => {
                      let label = "已启用";
                      let tone: MediaTone = "muted";
                      switch (tool.status) {
                        case "configured":
                          label = "已配置";
                          tone = "ok";
                          break;
                        case "needs_configuration":
                          label = "待配置";
                          tone = "warn";
                          break;
                        case "builtin":
                          label = "核心能力";
                          tone = "muted";
                          break;
                        case "disabled":
                          label = "未启用";
                          tone = "muted";
                          break;
                        default:
                          label = tool.scope === "shared" ? "运行时提供" : "已启用";
                          tone = "muted";
                          break;
                      }
                      return renderStatusChip(label, tone, true);
                    })()}
                  </div>
                  <div class="media-dashboard-mini-card__note">${tool.description}</div>
                </section>
            `)}
            </div>
          </section>

          <section class="media-dashboard-section">
            <div class="media-dashboard-section__header">
              <div>
                <div class="media-dashboard-section__title">已注册发布器</div>
                <div class="media-dashboard-section__sub">发布器配置完整后，草稿就能进入真正的投放链路。</div>
              </div>
            </div>
            ${config.publishers.length > 0
              ? html`
                  <div class="media-dashboard-tag-list">
                    ${config.publishers.map((publisher: string) => html`<span class="pill">${publisher}</span>`)}
                  </div>
                `
              : html`<div class="media-dashboard-empty">还没有可用发布器，当前只支持草稿与审批流。</div>`}
          </section>
        </div>

        <section class="media-dashboard-section">
          <div class="media-dashboard-section__header">
            <div>
              <div class="media-dashboard-section__title">LLM 模型配置</div>
              <div class="media-dashboard-section__sub">这里决定媒体子智能体的创作与判断能力，也是最值得优先配好的入口。</div>
            </div>
          </div>
          <div class="media-dashboard-form-grid">
            <label class="field media-dashboard-field">
              <span>Provider</span>
              <select
                .value=${config.llm?.provider || ""}
                @change=${(e: Event) => {
                  updateMediaConfig(state, { provider: (e.target as HTMLSelectElement).value });
                }}
              >
                <option value="">未配置</option>
                <option value="deepseek">DeepSeek</option>
                <option value="anthropic">Anthropic</option>
                <option value="openai">OpenAI</option>
                <option value="zhipu">Zhipu (智谱)</option>
                <option value="qwen">Qwen (通义千问)</option>
              </select>
            </label>
            <label class="field media-dashboard-field">
              <span>Model</span>
              <input
                type="text"
                .value=${config.llm?.model || ""}
                placeholder="deepseek-chat"
                @change=${(e: Event) => {
                  updateMediaConfig(state, { model: (e.target as HTMLInputElement).value });
                }}
              />
            </label>
            <label class="field media-dashboard-field">
              <span>API Key</span>
              <input
                type="password"
                .value=${config.llm?.apiKey || ""}
                placeholder="sk-..."
                @change=${(e: Event) => {
                  const val = (e.target as HTMLInputElement).value;
                  if (val && !val.includes("****")) {
                    updateMediaConfig(state, { apiKey: val });
                  }
                }}
              />
            </label>
            <label class="field media-dashboard-field">
              <span>Base URL (可选)</span>
              <input
                type="text"
                .value=${config.llm?.baseUrl || ""}
                placeholder="https://api.deepseek.com"
                @change=${(e: Event) => {
                  updateMediaConfig(state, { baseUrl: (e.target as HTMLInputElement).value });
                }}
              />
            </label>
          </div>

          <div class="media-dashboard-inline-form">
            <label class="media-dashboard-inline-check">
            <input
              type="checkbox"
              .checked=${config.llm?.autoSpawnEnabled || false}
              @change=${(e: Event) => {
                updateMediaConfig(state, { autoSpawnEnabled: (e.target as HTMLInputElement).checked });
              }}
            />
              <span>自动 Spawn</span>
          </label>
            <label class="media-dashboard-inline-limit">
              <span>每日上限</span>
            <input
              type="number"
              min="1" max="50"
              .value=${String(config.llm?.maxAutoSpawnsPerDay || 5)}
              @change=${(e: Event) => {
                updateMediaConfig(state, { maxAutoSpawnsPerDay: Number((e.target as HTMLInputElement).value) });
              }}
            />
          </label>
          </div>
        </section>
      `)}
    </section>
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
    <section class="card media-dashboard-card">
      ${renderPanelBody(html`
        <div class="media-dashboard-card__header">
          <div>
            <div class="media-dashboard-card__title">⏱ 自动巡检任务</div>
            <div class="media-dashboard-card__sub">热点监控、发布跟踪和互动巡检会在这里显示实时状态。</div>
          </div>
          <button
            class="btn btn--sm"
            @click=${() => { loadMediaPatrolJobs(state); (state as any).requestUpdate?.(); }}
          >
            刷新
          </button>
        </div>

        <div class="media-dashboard-list">
          ${jobs.map((job: CronPatrolJob) => {
            const label = PATROL_LABELS[job.name] || job.name;
            const tone: MediaTone = job.state.lastStatus === "ok"
              ? "ok"
              : job.state.lastStatus === "error"
                ? "danger"
                : "muted";
            const statusText = job.state.lastStatus === "ok" ? "正常"
              : job.state.lastStatus === "error" ? "错误"
                : job.state.lastStatus || "未运行";
            const interval = job.schedule?.everyMs ? formatInterval(job.schedule.everyMs) : "—";

            return html`
              <article class="media-dashboard-row">
                <div class="media-dashboard-row__main">
                  <div class="media-dashboard-row__title">${label}</div>
                  <div class="media-dashboard-row__meta">
                    ${job.description}
                  </div>
                  <div class="media-dashboard-row__meta">
                    间隔 ${interval}
                    · 上次 ${formatTime(job.state.lastRunAtMs)}
                    · 下次 ${formatTime(job.state.nextRunAtMs)}
                  </div>
                </div>
                <div class="media-dashboard-row__actions">
                  ${renderStatusChip(job.enabled ? "启用" : "禁用", job.enabled ? "ok" : "muted", true)}
                  ${renderStatusChip(statusText, tone, true)}
                </div>
              </article>
            `;
          })}
        </div>
      `)}
    </section>
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
    <section class="card media-dashboard-banner media-dashboard-banner--accent">
      <div class="media-dashboard-banner__header">
        <div class="media-dashboard-banner__title-row">
          <span class="media-dashboard-banner__icon">&#9881;</span>
          <div>
            <div class="media-dashboard-banner__title">${progress.summary}</div>
            ${phase ? html`<div class="media-dashboard-banner__sub">${phase}</div>` : nothing}
          </div>
        </div>
        ${percent != null && percent > 0
          ? html`<span class="media-dashboard-banner__value">${percent}%</span>`
          : nothing}
      </div>
      ${percent != null && percent > 0
        ? html`
            <div class="media-dashboard-progress">
              <div
                class="media-dashboard-progress__fill"
                style=${`width:${Math.min(percent, 100)}%;`}
              ></div>
            </div>
          `
        : nothing}
    </section>
  `;
}

// ---------- 智能体心跳面板 ----------

export function renderHeartbeatPanel(hb: MediaHeartbeatStatus | null): TemplateResult | typeof nothing {
  if (!hb) return nothing;

  const isRunning = hb.activeJobId != null;
  const hasError = hb.lastError != null;

  let statusText: string;
  let bannerClass = "media-dashboard-banner--ok";
  if (isRunning) {
    statusText = t("media.heartbeat.running");
    bannerClass = "media-dashboard-banner--info";
  } else if (hasError) {
    statusText = t("media.heartbeat.error");
    bannerClass = "media-dashboard-banner--danger";
  } else {
    statusText = t("media.heartbeat.normal");
  }

  const lastStr = hb.lastPatrolAt ? formatRelativeTime(hb.lastPatrolAt) : "--";

  return html`
    <section class="card media-dashboard-banner ${bannerClass}">
      <div class="media-dashboard-banner__header">
        <div class="media-dashboard-banner__title-row">
          <span class="media-dashboard-banner__dot ${isRunning ? "is-pulse" : ""}"></span>
          <div>
            <div class="media-dashboard-banner__title">${t("media.heartbeat.title")}</div>
            <div class="media-dashboard-banner__sub">${t("media.heartbeat.lastPatrol")}: ${lastStr}</div>
          </div>
        </div>
        <span class="media-dashboard-banner__value">${statusText}</span>
      </div>
      <div class="media-dashboard-banner__meta">
        ${hb.autoSpawnCount != null
          ? html`<span>${t("media.heartbeat.autoSpawnCount")}: ${hb.autoSpawnCount}</span>`
          : nothing}
        ${hb.nextPatrolAt ? html`<span>下次巡检: ${formatTime(hb.nextPatrolAt)}</span>` : nothing}
        ${hasError ? html`<span class="media-dashboard-banner__error">${hb.lastError}</span>` : nothing}
      </div>
    </section>
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
  const errors = state.mediaTrendingErrors || [];
  const sources = state.mediaTrendingSources || [];
  const loading = state.mediaTrendingLoading || false;
  const selectedSource = state.mediaTrendingSelectedSource || "";

  return html`
    <section class="card media-dashboard-card">
      ${renderPanelBody(html`
        <div class="media-dashboard-card__header">
          <div>
            <div class="media-dashboard-card__title">${t("media.trending.title")}</div>
            <div class="media-dashboard-card__sub">热点列表是内容创作的上游输入，建议优先盯住信号质量。</div>
          </div>
          <div class="media-dashboard-card__actions">
          ${sources.length > 0
            ? html`
                <select
                  class="media-dashboard-control"
                  .value=${selectedSource}
                  @change=${(e: Event) => {
                    const val = (e.target as HTMLSelectElement).value;
                    state.mediaTrendingSelectedSource = val;
                    (state as any).requestUpdate?.();
                  }}
                >
                  <option value="">${t("media.trending.allSources")}</option>
                  ${sources.map((s: string) => html`<option value=${s}>${SOURCE_LABELS[s] || s}</option>`)}
                </select>
              `
            : nothing}
          <button
            class="btn btn--sm"
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
        ${errors.length > 0
          ? html`
              <div class="media-dashboard-mini-card is-danger" style="margin:12px 0;">
                <div class="media-dashboard-mini-card__head">
                  <strong>抓取异常</strong>
                  ${renderStatusChip(`${errors.length} 个源失败`, "danger", true)}
                </div>
                <div class="media-dashboard-mini-card__note">
                  ${errors.map((item) => `${SOURCE_LABELS[item.source] || item.source}: ${item.error}`).join(" | ")}
                </div>
              </div>
            `
          : nothing}
        <div class="media-dashboard-list media-dashboard-list--scroll">
        ${topics.length === 0
          ? html`<div class="media-dashboard-empty">${t("media.trending.empty")}</div>`
          : html`${topics.map((topic: TrendingTopic) => renderTrendingItem(topic))}`}
        </div>
      `)}
    </section>
  `;
}

function renderTrendingItem(topic: TrendingTopic): TemplateResult {
  const heatStr = formatHeatScore(topic.heat_score);
  return html`
    <article class="media-dashboard-row media-dashboard-row--interactive">
      <div class="media-dashboard-row__main">
        <div class="media-dashboard-row__title">
        ${topic.url
          ? html`<a href=${topic.url} target="_blank" rel="noopener" class="media-dashboard-row__link">${topic.title}</a>`
          : html`${topic.title}`}
        </div>
        <div class="media-dashboard-row__meta">
          <span class="pill">${SOURCE_LABELS[topic.source] || topic.source}</span>
          ${topic.category ? html`<span>${topic.category}</span>` : nothing}
          <span>${new Date(topic.fetched_at).toLocaleString()}</span>
        </div>
      </div>
      <div class="media-dashboard-row__actions">
        ${renderStatusChip(heatStr, "info", true)}
      </div>
    </article>
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
    <section class="card media-dashboard-card">
      ${renderPanelBody(html`
        <div class="media-dashboard-card__header">
          <div>
            <div class="media-dashboard-card__title">${t("media.drafts.title")}</div>
            <div class="media-dashboard-card__sub">草稿是媒体智能体和人工协作的主战场，适合在这里完成审核与微调。</div>
          </div>
          <div class="media-dashboard-card__actions">
          <select
            class="media-dashboard-control"
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
        <div class="media-dashboard-list media-dashboard-list--scroll">
        ${loading
          ? html`<div class="media-dashboard-empty">Loading…</div>`
          : drafts.length === 0
            ? html`<div class="media-dashboard-empty">${t("media.drafts.empty")}</div>`
            : html`${drafts.map((d: DraftEntry) => renderDraftItem(state, d))}`}
        </div>
      `)}
    </section>
  `;
}

function renderDraftItem(state: AppViewState, draft: DraftEntry): TemplateResult {
  const statusTone: MediaTone =
    draft.status === "published"
      ? "ok"
      : draft.status === "approved"
        ? "info"
        : draft.status === "pending_review"
          ? "warn"
          : "muted";
  return html`
    <article
      class="media-dashboard-row media-dashboard-row--interactive"
      @click=${() => {
        loadDraftDetail(state, draft.id);
        (state as any).requestUpdate?.();
      }}
    >
      <div class="media-dashboard-row__main">
        <div class="media-dashboard-row__title">${draft.title || "(untitled)"}</div>
        <div class="media-dashboard-row__meta">
          <span class="pill">${draft.platform}</span>
          ${renderStatusChip(STATUS_LABELS[draft.status] || draft.status, statusTone, true)}
          <span>${new Date(draft.updated_at).toLocaleString()}</span>
        </div>
      </div>
      <div class="media-dashboard-row__actions">
        <button
          class="btn btn--sm"
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
            class="btn btn--sm primary"
            @click=${(e: Event) => {
              e.stopPropagation();
              void approveDraft(state, draft.id);
            }}
          >
            ✓ 审批
          </button>
        ` : nothing}
        <button
          class="btn btn--sm danger"
          @click=${(e: Event) => {
            e.stopPropagation();
            if (!confirm(t("media.drafts.deleteConfirm"))) return;
            void deleteDraft(state, draft.id);
          }}
        >
          ${t("media.drafts.delete")}
        </button>
      </div>
    </article>
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
    <section class="card media-dashboard-card">
      ${renderPanelBody(html`
        <div class="media-dashboard-card__header">
          <div>
            <div class="media-dashboard-card__title">${t("media.publish.title")}</div>
            <div class="media-dashboard-card__sub">这里是投放结果面板，适合追踪真正发出去的内容和链接。</div>
          </div>
          <div class="media-dashboard-card__actions">
          <select
            class="media-dashboard-control"
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
            class="btn btn--sm"
            ?disabled=${loading}
            @click=${() => loadPage(page)}
          >
            ${loading ? "..." : t("media.refreshStatus")}
          </button>
        </div>
        </div>
        <div class="media-dashboard-list">
        ${records.length === 0
          ? html`<div class="media-dashboard-empty">${t("media.publish.empty")}</div>`
          : html`
                ${records.map(
                  (r) => html`
                    <article
                      class="media-dashboard-row media-dashboard-row--interactive"
                      @click=${() => {
                        loadPublishDetail(state, r.id);
                        (state as any).requestUpdate?.();
                      }}
                    >
                      <div class="media-dashboard-row__main">
                        <div class="media-dashboard-row__title media-dashboard-row__title--truncate">
                          ${r.title}
                        </div>
                        <div class="media-dashboard-row__meta">
                          <span class="pill">${r.platform}</span>
                          ${renderStatusChip(
                            r.status === "published" ? "已发布" : r.status === "failed" ? "失败" : r.status,
                            r.status === "published" ? "ok" : r.status === "failed" ? "danger" : "warn",
                            true,
                          )}
                          ${r.published_at
                            ? html`<span>${new Date(r.published_at).toLocaleString()}</span>`
                            : nothing}
                        </div>
                      </div>
                      <div class="media-dashboard-row__actions">
                        ${r.url
                          ? html`<a
                            href=${r.url}
                            target="_blank"
                            rel="noopener"
                            class="media-dashboard-row__link"
                            @click=${(e: Event) => e.stopPropagation()}
                          >${t("media.publish.viewLink")}</a>`
                          : nothing}
                      </div>
                    </article>
                  `,
                )}
            `}
        </div>
        ${(hasPrev || hasNext) ? html`
          <div class="media-dashboard-pagination">
            <button class="btn btn--sm" ?disabled=${!hasPrev || loading} @click=${() => loadPage(page - 1)}>← 上一页</button>
            <span class="media-dashboard-pagination__label">Page ${page + 1}</span>
            <button class="btn btn--sm" ?disabled=${!hasNext || loading} @click=${() => loadPage(page + 1)}>下一页 →</button>
          </div>
        ` : nothing}
      `)}
    </section>
  `;
}
