// views/media-manage.ts — 媒体运营管理页面（独立侧栏入口）
// 轻苹果风壳层 + 子 tab 分发，保留现有媒体控制器与业务面板。

import { html, nothing, type TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { t } from "../i18n.ts";
import { openMediaManageWindow } from "../media-manage-window.ts";
import { pathForTab } from "../navigation.ts";
import {
  renderPatrolPanel,
  renderProgressBanner,
  renderHeartbeatPanel,
  renderTrendingPanel,
  renderDraftsPanel,
  renderPublishPanel,
  renderDraftDetailModal,
  renderPublishDetailModal,
  renderDraftEditModal,
} from "./media-dashboard.ts";
import {
  startMediaPublisherLogin,
  toggleMediaTool,
  toggleMediaSource,
  updateMediaConfig,
  waitMediaPublisherLogin,
  type MediaBochaTrendingProfile,
  type MediaCustomOpenAITrendingProfile,
  type MediaPublisherProfileBase,
  type MediaPublisherProfiles,
  type MediaWeChatProfile,
  type MediaXiaohongshuProfile,
  type MediaWebsiteProfile,
  type MediaSourceInfo,
  type MediaToolInfo,
} from "../controllers/media-dashboard.ts";

export type MediaSubTab = "overview" | "llm" | "sources" | "tools" | "strategy" | "drafts" | "publish" | "patrol";

const MEDIA_SUBTAB_QUERY = "mediaSubTab";

const SUB_TABS: { id: MediaSubTab; labelKey: string }[] = [
  { id: "overview", labelKey: "media.subtab.overview" },
  { id: "llm", labelKey: "media.subtab.llm" },
  { id: "sources", labelKey: "media.subtab.sources" },
  { id: "tools", labelKey: "media.subtab.tools" },
  { id: "strategy", labelKey: "media.subtab.strategy" },
  { id: "drafts", labelKey: "media.subtab.drafts" },
  { id: "publish", labelKey: "media.subtab.publish" },
  { id: "patrol", labelKey: "media.subtab.patrol" },
];

const SUB_TAB_IDS = new Set<MediaSubTab>(SUB_TABS.map((tab) => tab.id));

const TOOL_ICONS: Record<string, string> = {
  trending_topics: "📊",
  content_compose: "✍️",
  media_publish: "🚀",
  social_interact: "💬",
  web_search: "🔍",
  report_progress: "📢",
};

const TOOL_LABELS: Record<string, string> = {
  trending_topics: "热点发现",
  content_compose: "内容创作",
  media_publish: "多平台发布",
  social_interact: "社交互动",
  web_search: "网页搜索",
  report_progress: "进度汇报",
};

const TOGGLEABLE_TOOLS = new Set(["media_publish", "social_interact"]);

const ALL_SOURCES = ["weibo", "baidu", "zhihu", "bocha", "custom_openai"] as const;

const SOURCE_LABELS: Record<string, string> = {
  weibo: "微博热搜",
  baidu: "百度热搜",
  zhihu: "知乎热榜",
  bocha: "Bocha API 热点",
  custom_openai: "自定义 OpenAI 兼容",
};

function isToolEnabled(tool: Pick<MediaToolInfo, "enabled" | "status">): boolean {
  return tool.enabled !== false && tool.status !== "disabled";
}

function isSourceEnabled(source: Pick<MediaSourceInfo, "enabled" | "status">): boolean {
  if (typeof source.enabled === "boolean") {
    return source.enabled;
  }
  return source.status !== "disabled";
}

function toolStatusMeta(tool: Pick<MediaToolInfo, "enabled" | "status" | "configured" | "scope">): {
  label: string;
  chipClass: string;
} {
  if (!isToolEnabled(tool)) {
    return { label: "未启用", chipClass: "chip-muted" };
  }
  switch (tool.status) {
    case "configured":
      return { label: "已配置", chipClass: "chip-ok" };
    case "needs_configuration":
      return { label: "待配置", chipClass: "chip-warn" };
    case "builtin":
      return { label: "核心能力", chipClass: "chip-muted" };
    default:
      if (tool.scope === "shared") {
        return { label: "运行时提供", chipClass: "chip-muted" };
      }
      return { label: "已启用", chipClass: "chip-muted" };
  }
}

function isMediaSubTab(value: string | null | undefined): value is MediaSubTab {
  return Boolean(value && SUB_TAB_IDS.has(value as MediaSubTab));
}

function normalizeMediaSubTab(value: string | null | undefined): MediaSubTab {
  return isMediaSubTab(value) ? value : "overview";
}

function readMediaSubTabFromUrl(): MediaSubTab | null {
  if (typeof window === "undefined") {
    return null;
  }
  const value = new URL(window.location.href).searchParams.get(MEDIA_SUBTAB_QUERY);
  return isMediaSubTab(value) ? value : null;
}

function syncMediaSubTabInUrl(basePath: string, tab: MediaSubTab) {
  if (typeof window === "undefined") {
    return;
  }
  const url = new URL(window.location.href);
  url.pathname = pathForTab("media", basePath);
  if (tab === "overview") {
    url.searchParams.delete(MEDIA_SUBTAB_QUERY);
  } else {
    url.searchParams.set(MEDIA_SUBTAB_QUERY, tab);
  }
  window.history.replaceState({}, "", url.toString());
}

export function buildMediaManageUrl(basePath: string, tab: MediaSubTab = "overview"): string {
  const targetPath = pathForTab("media", basePath);
  if (typeof window === "undefined") {
    return tab === "overview"
      ? targetPath
      : `${targetPath}?${MEDIA_SUBTAB_QUERY}=${encodeURIComponent(tab)}`;
  }
  const url = new URL(targetPath, window.location.href);
  if (tab !== "overview") {
    url.searchParams.set(MEDIA_SUBTAB_QUERY, tab);
  }
  return url.toString();
}

function formatTimestamp(timestamp: number | null | undefined): string {
  if (!timestamp) {
    return "—";
  }
  try {
    return new Intl.DateTimeFormat(undefined, {
      month: "numeric",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(timestamp);
  } catch {
    return new Date(timestamp).toLocaleString();
  }
}

function renderTagList(items: string[], emptyLabel = "—"): TemplateResult {
  if (items.length === 0) {
    return html`<span class="media-manage__meta-empty">${emptyLabel}</span>`;
  }
  return html`
    <div class="media-manage__tag-list">
      ${items.map((item) => html`<span class="media-manage__tag">${item}</span>`)}
    </div>
  `;
}

export function renderMediaManage(state: AppViewState): TemplateResult {
  const activeSubTab = readMediaSubTabFromUrl() ?? normalizeMediaSubTab(state.mediaManageSubTab);
  const config = state.mediaConfig;
  const isConfigured = config?.status === "configured";

  const setSubTab = (tab: MediaSubTab) => {
    state.mediaManageSubTab = tab;
    syncMediaSubTabInUrl(state.basePath, tab);
    (state as { requestUpdate?: () => void }).requestUpdate?.();
  };

  const configureLlm = () => setSubTab("llm");

  return html`
    <section class="media-manage">
      <div class="media-manage__shell">
        <div class="media-manage__page-head">
          <div class="media-manage__hero-copy">
            <div class="media-manage__eyebrow">${t("nav.tab.media")}</div>
            <div class="media-manage__headline-row">
              <div>
                <h1 class="media-manage__headline">${t("media.manage.title")}</h1>
                <p class="media-manage__lede">${t("media.manage.subtitle")}</p>
              </div>
              <span class="media-manage__hero-badge ${isConfigured ? "is-ready" : "is-warning"}">
                <span class="media-manage__hero-badge-dot"></span>
                ${isConfigured ? t("media.manage.statusReady") : t("media.manage.statusSetup")}
              </span>
            </div>
          </div>

          <div class="media-manage__hero-actions">
            <button class="btn primary" @click=${configureLlm}>
              ${t("media.manage.configureModel")}
            </button>
            <button
              class="btn"
              @click=${() => {
                void openMediaManageWindow(buildMediaManageUrl(state.basePath, activeSubTab), activeSubTab, "media-page");
              }}
            >
              ${t("media.manage.openWindow")}
            </button>
          </div>
        </div>

        <div class="media-manage__subtabs" role="tablist" aria-label=${t("media.manage.title")}>
          ${SUB_TABS.map(
            (tab) => html`
              <button
                class="media-manage__subtab ${activeSubTab === tab.id ? "is-active" : ""}"
                @click=${() => setSubTab(tab.id)}
              >
                ${t(tab.labelKey)}
              </button>
            `,
          )}
        </div>

        <div class="media-manage__content">
          ${dispatchSubTab(activeSubTab, state, {
            configureLlm,
            setSubTab,
            openInWindow: () => openMediaManageWindow(
              buildMediaManageUrl(state.basePath, activeSubTab),
              activeSubTab,
              "media-page",
            ),
          })}
        </div>
      </div>
    </section>

    ${renderDraftDetailModal(state)}
    ${renderPublishDetailModal(state)}
    ${renderDraftEditModal(state)}
  `;
}

function dispatchSubTab(
  tab: MediaSubTab,
  state: AppViewState,
  actions: {
    configureLlm: () => void;
    setSubTab: (tab: MediaSubTab) => void;
    openInWindow: () => void;
  },
): TemplateResult | typeof nothing {
  switch (tab) {
    case "overview":
      return renderOverviewTab(state, actions);
    case "llm":
      return renderConfigurationTab(state, actions);
    case "sources":
      return renderSourcesTab(state);
    case "tools":
      return renderToolsTab(state);
    case "strategy":
      return renderStrategyTab(state);
    case "drafts":
      return html`<div class="media-manage__panel-stack">${renderDraftsPanel(state)}</div>`;
    case "publish":
      return html`<div class="media-manage__panel-stack">${renderPublishPanel(state)}</div>`;
    case "patrol":
      return renderPatrolTab(state);
    default:
      return nothing;
  }
}

function renderOverviewTab(
  state: AppViewState,
  actions: {
    configureLlm: () => void;
    setSubTab: (tab: MediaSubTab) => void;
    openInWindow: () => void;
  },
): TemplateResult {
  const config = state.mediaConfig;
  const isConfigured = config?.status === "configured";
  const toolCount = (config?.tools ?? []).filter((tool: MediaToolInfo) =>
    tool.scope === "shared" ? tool.enabled !== false : isToolEnabled(tool),
  ).length;
  const sourceCount = config?.enabled_sources?.length
    ?? (config?.trending_sources ?? []).filter((source: MediaSourceInfo) => isSourceEnabled(source)).length;
  const draftCount = state.mediaDrafts?.length ?? 0;
  const publisherCount = config?.publishers?.length ?? 0;
  const provider = config?.llm?.provider || "—";
  const model = config?.llm?.model || "—";
  const autoSpawnCount = state.mediaHeartbeat?.autoSpawnCount ?? 0;
  const toolNames = (config?.tools ?? [])
    .filter((tool: MediaToolInfo) => tool.scope !== "shared" && isToolEnabled(tool))
    .map((tool: MediaToolInfo) => TOOL_LABELS[tool.name] || tool.name);
  const sourceNames = (config?.enabled_sources?.length
    ? (config.enabled_sources as string[])
    : (config?.trending_sources ?? [])
        .filter((source: MediaSourceInfo) => isSourceEnabled(source))
        .map((source: MediaSourceInfo) => source.name))
    .map((name: string) => SOURCE_LABELS[name] || name);
  const publishers = config?.publishers ?? [];

  return html`
    <div class="media-manage__overview">
      <div class="media-manage__hero">
        <div class="media-manage__hero-copy">
          <div class="media-manage__eyebrow">${t("media.manage.runbook")}</div>
          <div class="media-manage__headline-row">
            <div>
              <h2 class="media-manage__headline media-manage__headline--section">
                ${isConfigured ? t("media.manage.statusReady") : t("media.manage.statusSetup")}
              </h2>
              <p class="media-manage__lede">
                ${isConfigured
                  ? `${config?.llm?.provider || "LLM"} · ${config?.llm?.model || "—"}`
                  : t("media.overview.notConfigured")}
              </p>
            </div>
            <span class="media-manage__hero-badge ${isConfigured ? "is-ready" : "is-warning"}">
              <span class="media-manage__hero-badge-dot"></span>
              ${isConfigured ? t("media.manage.statusReady") : t("media.manage.statusSetup")}
            </span>
          </div>

          <p class="media-manage__summary">${t("media.manage.summary")}</p>

          <div class="media-manage__hero-actions">
            <button class="btn primary" @click=${actions.configureLlm}>
              ${t("media.manage.configureModel")}
            </button>
            <button class="btn" @click=${() => actions.setSubTab("sources")}>
              ${t("media.subtab.sources")}
            </button>
            <button class="btn" @click=${actions.openInWindow}>
              ${t("media.manage.openWindow")}
            </button>
          </div>
        </div>

        <div class="media-manage__hero-panel">
          <div class="media-manage__hero-metrics">
            <div class="media-manage__hero-metric">
              <span>${t("media.subtab.tools")}</span>
              <strong>${toolCount}</strong>
            </div>
            <div class="media-manage__hero-metric">
              <span>${t("media.subtab.sources")}</span>
              <strong>${sourceCount}</strong>
            </div>
            <div class="media-manage__hero-metric">
              <span>${t("media.subtab.drafts")}</span>
              <strong>${draftCount}</strong>
            </div>
            <div class="media-manage__hero-metric">
              <span>${t("media.manage.publishers")}</span>
              <strong>${publisherCount}</strong>
            </div>
          </div>

          <div class="media-manage__hero-facts">
            <div class="media-manage__hero-fact">
              <span>${t("media.subtab.llm")}</span>
              <strong>${provider}</strong>
              <small>${model}</small>
            </div>
            <div class="media-manage__hero-fact">
              <span>${t("media.manage.nextPatrol")}</span>
              <strong>${formatTimestamp(state.mediaHeartbeat?.nextPatrolAt)}</strong>
              <small>${t("media.heartbeat.lastPatrol")} ${formatTimestamp(state.mediaHeartbeat?.lastPatrolAt)}</small>
            </div>
            <div class="media-manage__hero-fact">
              <span>${t("media.heartbeat.autoSpawnCount")}</span>
              <strong>${autoSpawnCount}</strong>
              <small>${t("media.manage.currentWindow")} · ${t("media.subtab.overview")}</small>
            </div>
          </div>
        </div>
      </div>

      <div class="media-manage__cockpit">
        <section class="media-glass-card">
          <div class="media-glass-card__eyebrow">${t("media.manage.toolsReady")}</div>
          <div class="media-glass-card__body">
            ${renderTagList(toolNames)}
          </div>
        </section>

        <section class="media-glass-card">
          <div class="media-glass-card__eyebrow">${t("media.manage.sourceMix")}</div>
          <div class="media-glass-card__body">
            ${renderTagList(sourceNames)}
          </div>
        </section>

        <section class="media-glass-card">
          <div class="media-glass-card__eyebrow">${t("media.manage.outputLane")}</div>
          <div class="media-glass-card__body">
            ${renderTagList(publishers)}
          </div>
        </section>
      </div>

      ${!isConfigured
        ? html`
            <div class="callout" style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;">
              <span>${t("media.overview.notConfigured")}</span>
              <button class="btn primary" @click=${actions.configureLlm}>
                ${t("media.overview.configureNow")}
              </button>
            </div>
          `
        : nothing}

      <div class="media-manage__panel-stack">
        ${renderProgressBanner(state)}
        ${renderHeartbeatPanel(state.mediaHeartbeat)}
      </div>
    </div>
  `;
}

function renderOverviewStat(label: string, value: string): TemplateResult {
  return html`
    <section class="media-glass-card media-glass-card--stat">
      <div class="media-glass-card__eyebrow">${label}</div>
      <div class="media-glass-card__value">${value}</div>
    </section>
  `;
}

function resolvePublisherProfiles(
  profiles: MediaPublisherProfiles | undefined,
): MediaPublisherProfiles {
  return profiles ?? {
    wechat: {
      enabled: false,
      configured: false,
      status: "disabled",
      missing: [],
    },
    xiaohongshu: {
      enabled: false,
      configured: false,
      status: "disabled",
      missing: [],
      autoInteractInterval: 30,
      rateLimitSeconds: 5,
      errorScreenshotDir: "_media/xhs/errors",
    },
    website: {
      enabled: false,
      configured: false,
      status: "disabled",
      missing: [],
      authType: "bearer",
      timeoutSeconds: 30,
      maxRetries: 3,
    },
  };
}

function resolveBochaTrendingProfile(
  profile: MediaBochaTrendingProfile | undefined,
): MediaBochaTrendingProfile {
  return profile ?? {
    configured: false,
    status: "needs_configuration",
    missing: ["apiKey"],
    authMode: "api_key",
    apiKey: "",
    baseUrl: "https://api.bochaai.com",
    freshness: "oneDay",
  };
}

function resolveCustomOpenAITrendingProfile(
  profile: MediaCustomOpenAITrendingProfile | undefined,
): MediaCustomOpenAITrendingProfile {
  return profile ?? {
    configured: false,
    status: "needs_configuration",
    missing: ["apiKey", "baseUrl", "model"],
    authMode: "api_key",
    apiKey: "",
    baseUrl: "https://api.openai.com/v1",
    model: "",
    systemPrompt: "",
    requestExtras: "",
  };
}

function publisherStatusMeta(profile: MediaPublisherProfileBase): {
  label: string;
  chipClass: string;
} {
  switch (profile.status) {
    case "configured":
      return { label: "已配置", chipClass: "chip-ok" };
    case "needs_configuration":
      return { label: "待配置", chipClass: "chip-warn" };
    case "disabled":
    default:
      return { label: "未启用", chipClass: "chip-muted" };
  }
}

function missingFieldLabel(field: string): string {
  switch (field) {
    case "apiKey":
      return "API Key";
    case "appId":
      return "AppID";
    case "appSecret":
      return "AppSecret";
    case "baseUrl":
      return "Base URL";
    case "model":
      return "Model";
    case "cookiePath":
      return "Cookie 文件";
    case "apiUrl":
      return "API URL";
    case "authType":
      return "认证方式";
    case "authToken":
      return "认证 Token";
    default:
      return field;
  }
}

function renderProfileMissing(profile: MediaPublisherProfileBase): TemplateResult {
  const missing = profile.missing ?? [];
  if (!profile.enabled) {
    return html`<div class="media-manage__account-note">当前已停用，不会进入发布与互动链路。</div>`;
  }
  if (missing.length === 0) {
    return html`<div class="media-manage__account-note">配置完整，媒体子智能体可直接使用该通道。</div>`;
  }
  return html`
    <div class="media-manage__account-note media-manage__account-note--warn">
      缺少字段：${missing.map((field, index) => html`${index > 0 ? "、" : nothing}${missingFieldLabel(field)}`)}
    </div>
  `;
}

function renderSourceProfileMissing(missing?: string[]): TemplateResult {
  const fields = missing ?? [];
  if (fields.length === 0) {
    return html`<div class="media-manage__account-note">配置完整，启用后即可接入 API 型热点发现。</div>`;
  }
  return html`
    <div class="media-manage__account-note media-manage__account-note--warn">
      缺少字段：${fields.map((field, index) => html`${index > 0 ? "、" : nothing}${missingFieldLabel(field)}`)}
    </div>
  `;
}

function xhsAuthBadge(profile: MediaXiaohongshuProfile): { label: string; chipClass: string } {
  switch (profile.authStatus) {
    case "authenticated":
      return { label: "登录态有效", chipClass: "chip-ok" };
    case "waiting_login":
      return { label: "等待登录", chipClass: "chip-warn" };
    case "error":
    case "cookie_invalid":
      return { label: "需重采集", chipClass: "chip-warn" };
    case "cookie_present":
      return { label: "待校验", chipClass: "chip-muted" };
    case "browser_unavailable":
      return { label: "浏览器未就绪", chipClass: "chip-muted" };
    case "cookie_path_missing":
    case "not_logged_in":
    default:
      return { label: "未登录", chipClass: "chip-muted" };
  }
}

function formatAuthUpdatedAt(value: string | undefined): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
}

function renderXhsAuthCallout(state: AppViewState, profile: MediaXiaohongshuProfile): TemplateResult {
  const badge = xhsAuthBadge(profile);
  const updatedAt = formatAuthUpdatedAt(profile.authUpdatedAt);
  return html`
    <div class="callout" style="display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap;">
      <div style="display:grid;gap:6px;">
        <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
          <strong>自动登录执行器</strong>
          <span class="chip ${badge.chipClass}">${badge.label}</span>
          <span class="chip ${profile.browserReady ? "chip-ok" : "chip-muted"}">
            ${profile.browserReady ? "浏览器已就绪" : "浏览器按需启动"}
          </span>
        </div>
        <div>${profile.authMessage || "系统会打开小红书创作者页，登录成功后自动落 Cookie 并回填到当前账号。"}
        </div>
        ${updatedAt
          ? html`<div class="muted">最近校验：${updatedAt}</div>`
          : nothing}
      </div>
      <div class="row" style="gap:8px;flex-wrap:wrap;">
        <button class="btn primary" @click=${() => void startMediaPublisherLogin(state, "xiaohongshu")}>
          打开登录页
        </button>
        <button class="btn" @click=${() => void waitMediaPublisherLogin(state, "xiaohongshu")}>
          检查并保存登录态
        </button>
      </div>
    </div>
  `;
}

function renderConfigurationTab(
  state: AppViewState,
  actions: {
    configureLlm: () => void;
    setSubTab: (tab: MediaSubTab) => void;
    openInWindow: () => void;
  },
): TemplateResult {
  const config = state.mediaConfig;
  if (!config) {
    return html`<div class="muted">${t("common.loading")}</div>`;
  }

  const profiles = resolvePublisherProfiles(config.publisher_profiles);
  const llm = config.llm ?? {
    provider: "",
    model: "",
    apiKey: "",
    baseUrl: "",
    autoSpawnEnabled: false,
    maxAutoSpawnsPerDay: 5,
  };

  const renderWeChatCard = (profile: MediaWeChatProfile) => {
    const badge = publisherStatusMeta(profile);
    return html`
      <section class="media-manage__account-card">
        <div class="media-manage__toggle-title-row">
          <div>
            <strong>微信公众号</strong>
            <p>当前通过 AppID + AppSecret 接入，适合正式 API 发布与状态回查。</p>
          </div>
          <span class="chip ${badge.chipClass}">${badge.label}</span>
        </div>

        <label class="media-manage__switch-row">
          <input
            type="checkbox"
            .checked=${profile.enabled}
            @change=${(event: Event) => {
              void updateMediaConfig(state, {
                wechat: { enabled: (event.target as HTMLInputElement).checked },
              });
            }}
          />
          <span>启用公众号发布</span>
        </label>

        ${renderProfileMissing(profile)}

        <div class="media-manage__account-form">
          <label class="field media-dashboard-field">
            <span>账号名称</span>
            <input
              type="text"
              .value=${profile.accountName || ""}
              placeholder="品牌公众号"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  wechat: { accountName: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>账号标识</span>
            <input
              type="text"
              .value=${profile.accountId || ""}
              placeholder="gh_xxx"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  wechat: { accountId: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>AppID</span>
            <input
              type="text"
              .value=${profile.appId || ""}
              placeholder="wx1234567890"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  wechat: { appId: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>AppSecret</span>
            <input
              type="password"
              .value=${profile.appSecret || ""}
              placeholder="••••••••"
              @change=${(event: Event) => {
                const value = (event.target as HTMLInputElement).value;
                if (!value.includes("****")) {
                  void updateMediaConfig(state, { wechat: { appSecret: value } });
                }
              }}
            />
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>Token 缓存路径</span>
            <input
              type="text"
              .value=${profile.tokenCachePath || ""}
              placeholder="_media/wechat/token.json"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  wechat: { tokenCachePath: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
        </div>
      </section>
    `;
  };

  const renderXhsCard = (profile: MediaXiaohongshuProfile) => {
    const badge = publisherStatusMeta(profile);
    return html`
      <section class="media-manage__account-card">
        <div class="media-manage__toggle-title-row">
          <div>
            <strong>小红书</strong>
            <p>当前通过浏览器自动登录执行器采集 Cookie，支持发布和互动巡检；无需再手工导出 Cookie 文件。</p>
          </div>
          <span class="chip ${badge.chipClass}">${badge.label}</span>
        </div>

        <label class="media-manage__switch-row">
          <input
            type="checkbox"
            .checked=${profile.enabled}
            @change=${(event: Event) => {
              void updateMediaConfig(state, {
                xiaohongshu: { enabled: (event.target as HTMLInputElement).checked },
              });
            }}
          />
          <span>启用小红书通道</span>
        </label>

        ${renderProfileMissing(profile)}
        ${renderXhsAuthCallout(state, profile)}

        <div class="media-manage__account-form">
          <label class="field media-dashboard-field">
            <span>账号名称</span>
            <input
              type="text"
              .value=${profile.accountName || ""}
              placeholder="品牌小红书"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  xiaohongshu: { accountName: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>账号 ID</span>
            <input
              type="text"
              .value=${profile.accountId || ""}
              placeholder="xhs_001"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  xiaohongshu: { accountId: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>Cookie 文件路径</span>
            <input
              type="text"
              .value=${profile.cookiePath || ""}
              placeholder="/path/to/xhs-cookie.json"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  xiaohongshu: { cookiePath: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>自动互动间隔（分钟）</span>
            <input
              type="number"
              min="0"
              max="1440"
              .value=${String(profile.autoInteractInterval ?? 30)}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  xiaohongshu: {
                    autoInteractInterval: Number((event.target as HTMLInputElement).value),
                  },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>最小操作间隔（秒）</span>
            <input
              type="number"
              min="3"
              max="300"
              .value=${String(profile.rateLimitSeconds ?? 5)}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  xiaohongshu: {
                    rateLimitSeconds: Number((event.target as HTMLInputElement).value),
                  },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>错误截图目录</span>
            <input
              type="text"
              .value=${profile.errorScreenshotDir || "_media/xhs/errors"}
              placeholder="_media/xhs/errors"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  xiaohongshu: { errorScreenshotDir: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
        </div>
      </section>
    `;
  };

  const renderWebsiteCard = (profile: MediaWebsiteProfile) => {
    const badge = publisherStatusMeta(profile);
    return html`
      <section class="media-manage__account-card">
        <div class="media-manage__toggle-title-row">
          <div>
            <strong>自有网站</strong>
            <p>适合官网、博客或 CMS 投放，可作为正式发布链路的第三个出口。</p>
          </div>
          <span class="chip ${badge.chipClass}">${badge.label}</span>
        </div>

        <label class="media-manage__switch-row">
          <input
            type="checkbox"
            .checked=${profile.enabled}
            @change=${(event: Event) => {
              void updateMediaConfig(state, {
                website: { enabled: (event.target as HTMLInputElement).checked },
              });
            }}
          />
          <span>启用网站发布</span>
        </label>

        ${renderProfileMissing(profile)}

        <div class="media-manage__account-form">
          <label class="field media-dashboard-field">
            <span>站点名称</span>
            <input
              type="text"
              .value=${profile.siteName || ""}
              placeholder="品牌官网"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  website: { siteName: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>认证方式</span>
            <select
              .value=${profile.authType || "bearer"}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  website: { authType: (event.target as HTMLSelectElement).value },
                });
              }}
            >
              <option value="bearer">Bearer</option>
              <option value="api_key">API Key</option>
              <option value="basic">Basic</option>
            </select>
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>API URL</span>
            <input
              type="text"
              .value=${profile.apiUrl || ""}
              placeholder="https://example.com/api/posts"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  website: { apiUrl: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>认证 Token</span>
            <input
              type="password"
              .value=${profile.authToken || ""}
              placeholder="token"
              @change=${(event: Event) => {
                const value = (event.target as HTMLInputElement).value;
                if (!value.includes("****")) {
                  void updateMediaConfig(state, { website: { authToken: value } });
                }
              }}
            />
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>图片上传 URL</span>
            <input
              type="text"
              .value=${profile.imageUploadUrl || ""}
              placeholder="https://example.com/api/media"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  website: { imageUploadUrl: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>超时（秒）</span>
            <input
              type="number"
              min="1"
              max="600"
              .value=${String(profile.timeoutSeconds ?? 30)}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  website: { timeoutSeconds: Number((event.target as HTMLInputElement).value) },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>最大重试</span>
            <input
              type="number"
              min="0"
              max="10"
              .value=${String(profile.maxRetries ?? 3)}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  website: { maxRetries: Number((event.target as HTMLInputElement).value) },
                });
              }}
            />
          </label>
        </div>
      </section>
    `;
  };

  return html`
    <div class="media-manage__panel-stack">
      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">${t("media.subtab.llm")}</div>
            <div class="media-glass-card__body">将媒体子智能体的模型、API Key 和发布账号集中放在一处维护。</div>
          </div>
          <div class="media-manage__hero-actions">
            <button class="btn" @click=${() => actions.setSubTab("sources")}>
              ${t("media.subtab.sources")}
            </button>
            <button class="btn" @click=${() => actions.setSubTab("strategy")}>
              ${t("media.subtab.strategy")}
            </button>
            <button class="btn" @click=${actions.openInWindow}>
              ${t("media.manage.openWindow")}
            </button>
          </div>
        </div>

        <div class="media-manage__config-grid">
          <label class="field media-dashboard-field">
            <span>Provider</span>
            <select
              .value=${llm.provider || ""}
              @change=${(event: Event) => {
                void updateMediaConfig(state, { provider: (event.target as HTMLSelectElement).value });
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
              .value=${llm.model || ""}
              placeholder="deepseek-chat"
              @change=${(event: Event) => {
                void updateMediaConfig(state, { model: (event.target as HTMLInputElement).value });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>API Key</span>
            <input
              type="password"
              .value=${llm.apiKey || ""}
              placeholder="sk-..."
              @change=${(event: Event) => {
                const value = (event.target as HTMLInputElement).value;
                if (!value.includes("****")) {
                  void updateMediaConfig(state, { apiKey: value });
                }
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>Base URL</span>
            <input
              type="text"
              .value=${llm.baseUrl || ""}
              placeholder="https://api.deepseek.com"
              @change=${(event: Event) => {
                void updateMediaConfig(state, { baseUrl: (event.target as HTMLInputElement).value });
              }}
            />
          </label>
        </div>

        <div class="media-dashboard-inline-form">
          <label class="media-dashboard-inline-check">
            <input
              type="checkbox"
              .checked=${llm.autoSpawnEnabled || false}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  autoSpawnEnabled: (event.target as HTMLInputElement).checked,
                });
              }}
            />
            <span>自动 Spawn</span>
          </label>
          <label class="media-dashboard-inline-limit">
            <span>每日上限</span>
            <input
              type="number"
              min="1"
              max="50"
              .value=${String(llm.maxAutoSpawnsPerDay || 5)}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  maxAutoSpawnsPerDay: Number((event.target as HTMLInputElement).value),
                });
              }}
            />
          </label>
        </div>
      </section>

      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">媒体账号与发布器</div>
            <div class="media-glass-card__body">公众号、小红书、自有网站的接入状态会直接影响“发布记录”和“社交互动”链路是否可用。</div>
          </div>
        </div>

        <div class="media-manage__account-grid">
          ${renderWeChatCard(profiles.wechat)}
          ${renderXhsCard(profiles.xiaohongshu)}
          ${renderWebsiteCard(profiles.website)}
        </div>
      </section>

      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">热点配置入口</div>
            <div class="media-glass-card__body">热点抓取与筛选规则没有删除，已拆分到单独 tab，避免和账号配置混在一起。</div>
          </div>
          <div class="media-manage__hero-actions">
            <button class="btn" @click=${() => actions.setSubTab("sources")}>去看热点来源</button>
            <button class="btn" @click=${() => actions.setSubTab("strategy")}>去看热点策略</button>
          </div>
        </div>
      </section>
    </div>
  `;
}


function renderToolsTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  if (!config) {
    return html`<div class="muted">${t("common.loading")}</div>`;
  }

  const mediaTools = (config.tools || []).filter((tool: { scope?: string }) => tool.scope !== "shared");
  const sharedTools = (config.tools || []).filter((tool: { scope?: string }) => tool.scope === "shared");

  const renderToolCard = (tool: MediaToolInfo, toggleable: boolean) => {
    const enabled = isToolEnabled(tool);
    const badge = toolStatusMeta(tool);
    return html`
      <section class="media-manage__toggle-card ${enabled ? "" : "is-muted"}">
        <div class="media-manage__toggle-icon">${TOOL_ICONS[tool.name] || "🔧"}</div>
        <div class="media-manage__toggle-copy">
          <div class="media-manage__toggle-title-row">
            <strong>${TOOL_LABELS[tool.name] || tool.name}</strong>
            <span class="chip ${badge.chipClass}">
              ${badge.label}
            </span>
          </div>
          <p>${tool.description || TOOL_LABELS[tool.name] || tool.name}</p>

          ${toggleable
            ? html`
                <label class="media-manage__switch-row">
                  <input
                    type="checkbox"
                    .checked=${enabled}
                    @change=${(event: Event) => {
                      const checked = (event.target as HTMLInputElement).checked;
                      void toggleMediaTool(state, tool.name, checked);
                    }}
                  />
                  <span>${enabled ? "启用中" : "已关闭"}</span>
                </label>
              `
            : html`
                <div class="media-manage__switch-row media-manage__switch-row--static">
                  <span>${tool.scope === "shared"
                    ? "共享能力，随运行环境自动提供"
                    : tool.status === "builtin"
                      ? "核心能力，默认可用，不依赖单独账号配置"
                      : tool.status === "needs_configuration"
                        ? "需要先补齐媒体账号或发布目标"
                        : "能力已接入，可直接参与媒体流程"}</span>
                </div>
              `}
        </div>
      </section>
    `;
  };

  return html`
    <div class="media-manage__panel-stack">
      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">${t("media.subtab.tools")}</div>
            <div class="media-glass-card__body">${t("media.manage.summary")}</div>
          </div>
        </div>

        <div class="media-manage__collection-grid">
          ${mediaTools.map((tool: MediaToolInfo) =>
            renderToolCard(tool, TOGGLEABLE_TOOLS.has(tool.name)),
          )}
        </div>
      </section>

      ${sharedTools.length > 0
        ? html`
            <section class="media-glass-card">
              <div class="media-glass-card__header">
                <div>
                  <div class="media-glass-card__eyebrow">共享工具</div>
                  <div class="media-glass-card__body">这些能力由主运行时提供，媒体子智能体会按需自动调用。</div>
                </div>
              </div>
              <div class="media-manage__collection-grid">
                ${sharedTools.map((tool: MediaToolInfo) =>
                  renderToolCard(tool, false),
                )}
              </div>
            </section>
          `
        : nothing}
    </div>
  `;
}

function renderSourcesTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  if (!config) {
    return html`<div class="muted">${t("common.loading")}</div>`;
  }

  const bocha = resolveBochaTrendingProfile(config.trending_source_profiles?.bocha);
  const customOpenAI = resolveCustomOpenAITrendingProfile(config.trending_source_profiles?.custom_openai);
  const registeredSources = Array.isArray(config.enabled_sources)
    ? config.enabled_sources
    : (config.trending_sources || [])
        .filter((source: MediaSourceInfo) => isSourceEnabled(source))
        .map((source: MediaSourceInfo) => source.name);
  const hasExplicitConfig = config.enabled_sources_configured === true;
  const allEnabled = !hasExplicitConfig;

  return html`
    <div class="media-manage__panel-stack">
      <section class="media-glass-card">
        <div class="media-glass-card__header">
          <div>
            <div class="media-glass-card__eyebrow">${t("media.subtab.sources")}</div>
            <div class="media-glass-card__body">${t("media.manage.sourcesHint")}</div>
          </div>
        </div>

        <div class="media-manage__collection-grid">
          ${ALL_SOURCES.map((name) => {
            const source = (config.trending_sources || []).find((entry: MediaSourceInfo) => entry.name === name);
            const enabled = source ? isSourceEnabled(source) : (allEnabled || registeredSources.includes(name));
            return html`
              <label class="media-manage__source-card ${enabled ? "" : "is-muted"}">
                <div class="media-manage__source-card-head">
                  <strong>${SOURCE_LABELS[name] || name}</strong>
                  <input
                    type="checkbox"
                    .checked=${enabled}
                    @change=${(event: Event) => {
                      const checked = (event.target as HTMLInputElement).checked;
                      void toggleMediaSource(state, name, checked);
                    }}
                  />
                </div>
                <span>${source?.status === "needs_configuration"
                  ? "需先补充 API Key，保存后才能稳定抓取"
                  : enabled
                    ? hasExplicitConfig ? "当前参与热点抓取" : "默认启用，尚未显式配置"
                    : hasExplicitConfig ? "已从抓取名单移除" : "当前未启用"}</span>
              </label>
            `;
          })}
        </div>
      </section>

      <section class="media-manage__account-card">
        <div class="media-manage__toggle-title-row">
          <div>
            <strong>Bocha API 热点发现</strong>
            <p>按官方 Web Search API 接入，使用 API Key 获取近实时热点候选，不再依赖公开页面直抓。</p>
          </div>
          <span class="chip ${bocha.configured ? "chip-ok" : "chip-warn"}">
            ${bocha.configured ? "已配置" : "待配置"}
          </span>
        </div>

        ${renderSourceProfileMissing(bocha.missing)}

        <div class="media-manage__account-form">
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>API Key</span>
            <input
              type="password"
              .value=${bocha.apiKey || ""}
              placeholder="sk-..."
              @change=${(event: Event) => {
                const value = (event.target as HTMLInputElement).value;
                if (!value.includes("****")) {
                  void updateMediaConfig(state, {
                    trendingBocha: { apiKey: value },
                  });
                }
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>Base URL</span>
            <input
              type="text"
              .value=${bocha.baseUrl || "https://api.bochaai.com"}
              placeholder="https://api.bochaai.com"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  trendingBocha: { baseUrl: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>Freshness</span>
            <select
              .value=${bocha.freshness || "oneDay"}
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  trendingBocha: { freshness: (event.target as HTMLSelectElement).value },
                });
              }}
            >
              <option value="oneDay">最近 1 天</option>
              <option value="oneWeek">最近 1 周</option>
              <option value="oneMonth">最近 1 月</option>
              <option value="oneYear">最近 1 年</option>
              <option value="noLimit">不限时间</option>
            </select>
          </label>
        </div>

        <div class="media-manage__account-note">
          官方字段已按 Bearer API Key、/v1/web-search 与 freshness 接入。启用上方 Bocha API 热点开关后即可参与抓取。
        </div>
      </section>

      <section class="media-manage__account-card">
        <div class="media-manage__toggle-title-row">
          <div>
            <strong>自定义 OpenAI 兼容热点源</strong>
            <p>适配任意 OpenAI-compatible chat completions 端点。是否具备实时热点能力，取决于你接入的上游模型是否支持联网或搜索。</p>
          </div>
          <span class="chip ${customOpenAI.configured ? "chip-ok" : "chip-warn"}">
            ${customOpenAI.configured ? "已配置" : "待配置"}
          </span>
        </div>

        ${renderSourceProfileMissing(customOpenAI.missing)}

        <div class="media-manage__account-form">
          <label class="field media-dashboard-field">
            <span>API Key</span>
            <input
              type="password"
              .value=${customOpenAI.apiKey || ""}
              placeholder="sk-..."
              @change=${(event: Event) => {
                const value = (event.target as HTMLInputElement).value;
                if (!value.includes("****")) {
                  void updateMediaConfig(state, {
                    trendingCustomOpenAI: { apiKey: value },
                  });
                }
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>Base URL</span>
            <input
              type="text"
              .value=${customOpenAI.baseUrl || "https://api.openai.com/v1"}
              placeholder="https://api.openai.com/v1"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  trendingCustomOpenAI: { baseUrl: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field">
            <span>Model</span>
            <input
              type="text"
              .value=${customOpenAI.model || ""}
              placeholder="gpt-4.1 / sonar / your-model"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  trendingCustomOpenAI: { model: (event.target as HTMLInputElement).value },
                });
              }}
            />
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>System Prompt</span>
            <textarea
              rows="4"
              .value=${customOpenAI.systemPrompt || ""}
              placeholder="为空则使用系统默认热点发现提示词"
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  trendingCustomOpenAI: { systemPrompt: (event.target as HTMLTextAreaElement).value },
                });
              }}
            ></textarea>
          </label>
          <label class="field media-dashboard-field media-manage__account-form-span">
            <span>Request Extras (JSON)</span>
            <textarea
              rows="5"
              .value=${customOpenAI.requestExtras || ""}
              placeholder='例如: {"web_search": true}'
              @change=${(event: Event) => {
                void updateMediaConfig(state, {
                  trendingCustomOpenAI: { requestExtras: (event.target as HTMLTextAreaElement).value },
                });
              }}
            ></textarea>
          </label>
        </div>

        <div class="media-manage__account-note">
          Request Extras 会透传到请求体，适合填 provider 私有的搜索开关。若上游不具备联网能力，系统会要求其返回空数组，而不是伪造实时热点。
        </div>
      </section>

      ${renderTrendingPanel(state)}
    </div>
  `;
}

function renderStrategyTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  const strategy = config?.trending_strategy;
  const hotKeywords = strategy?.hotKeywords ?? [];
  const monitorInterval = strategy?.monitorIntervalMin ?? 30;
  const threshold = strategy?.trendingThreshold ?? 10000;
  const categories = strategy?.contentCategories ?? [];
  const autoDraft = strategy?.autoDraftEnabled ?? false;

  return html`
    <div style="display:flex;flex-direction:column;gap:16px;">
      <div class="card media-strategy-panel">
        <div class="media-section-title" style="margin-bottom:12px;">热点策略配置</div>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">热度阈值（低于此值的话题将被跳过）</span>
          <input
            type="number"
            class="media-strategy-input"
            .value=${String(threshold)}
            @change=${(e: Event) => {
              const v = parseFloat((e.target as HTMLInputElement).value);
              if (!isNaN(v) && v >= 0) void updateMediaConfig(state, { trendingThreshold: v });
            }}
          />
        </label>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">监控频率（分钟）</span>
          <input
            type="number"
            class="media-strategy-input"
            min="5"
            max="1440"
            .value=${String(monitorInterval)}
            @change=${(e: Event) => {
              const v = parseInt((e.target as HTMLInputElement).value, 10);
              if (!isNaN(v) && v >= 5) void updateMediaConfig(state, { monitorIntervalMin: v });
            }}
          />
        </label>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">自定义关键词（逗号分隔）</span>
          <input
            type="text"
            class="media-strategy-input media-strategy-input--wide"
            .value=${hotKeywords.join(", ")}
            @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              const keywords = v.split(",").map(s => s.trim()).filter(Boolean);
              void updateMediaConfig(state, { hotKeywords: keywords });
            }}
            placeholder="例如: AI, 科技, 创业"
          />
        </label>

        <label class="field" style="margin-bottom:12px;">
          <span class="media-config-field-label">内容领域偏好（逗号分隔）</span>
          <input
            type="text"
            class="media-strategy-input media-strategy-input--wide"
            .value=${categories.join(", ")}
            @change=${(e: Event) => {
              const v = (e.target as HTMLInputElement).value;
              const cats = v.split(",").map(s => s.trim()).filter(Boolean);
              void updateMediaConfig(state, { contentCategories: cats });
            }}
            placeholder="例如: 科技, 教育, 商业"
          />
        </label>

        <label style="display:flex;align-items:center;gap:8px;cursor:pointer;font-size:13px;">
          <input
            type="checkbox"
            .checked=${autoDraft}
            @change=${(e: Event) => {
              const checked = (e.target as HTMLInputElement).checked;
              void updateMediaConfig(state, { autoDraftEnabled: checked });
            }}
          />
          自动生成草稿（发现匹配热点时自动创建内容草稿）
        </label>
      </div>
    </div>
  `;
}

function renderPatrolTab(state: AppViewState): TemplateResult {
  return html`
    <div class="media-manage__panel-stack">
      ${renderPatrolPanel(state)}
      ${renderProgressBanner(state)}
      ${renderHeartbeatPanel(state.mediaHeartbeat)}
    </div>
  `;
}
