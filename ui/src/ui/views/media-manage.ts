// views/media-manage.ts — 媒体运营管理页面（独立侧栏入口）
// 子 tab 导航 + 按 tab 分发到各面板渲染函数

import { html, nothing } from "lit";
import type { TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { t } from "../i18n.ts";
import {
  renderConfigPanel,
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
import { toggleMediaTool, toggleMediaSource } from "../controllers/media-dashboard.ts";

export type MediaSubTab = "overview" | "llm" | "sources" | "tools" | "drafts" | "publish" | "patrol";

const SUB_TABS: { id: MediaSubTab; labelKey: string }[] = [
  { id: "overview", labelKey: "media.subtab.overview" },
  { id: "llm", labelKey: "media.subtab.llm" },
  { id: "sources", labelKey: "media.subtab.sources" },
  { id: "tools", labelKey: "media.subtab.tools" },
  { id: "drafts", labelKey: "media.subtab.drafts" },
  { id: "publish", labelKey: "media.subtab.publish" },
  { id: "patrol", labelKey: "media.subtab.patrol" },
];

export function renderMediaManage(state: AppViewState): TemplateResult {
  const activeSubTab = (state.mediaManageSubTab || "overview") as MediaSubTab;

  const setSubTab = (tab: MediaSubTab) => {
    state.mediaManageSubTab = tab;
    (state as any).requestUpdate?.();
  };

  return html`
    <section class="card">
      <div class="row" style="justify-content: space-between;">
        <div>
          <div class="card-title">${t("media.manage.title")}</div>
          <div class="card-sub">${t("nav.sub.media")}</div>
        </div>
      </div>

      <div class="agent-tabs" style="margin-top: 16px;">
        ${SUB_TABS.map(
          (tab) => html`
            <button
              class="agent-tab ${activeSubTab === tab.id ? "active" : ""}"
              @click=${() => setSubTab(tab.id)}
            >
              ${t(tab.labelKey)}
            </button>
          `,
        )}
      </div>

      <div style="margin-top: 16px;">
        ${dispatchSubTab(activeSubTab, state)}
      </div>
    </section>

    ${renderDraftDetailModal(state)}
    ${renderPublishDetailModal(state)}
    ${renderDraftEditModal(state)}
  `;
}

function dispatchSubTab(tab: MediaSubTab, state: AppViewState): TemplateResult | typeof nothing {
  switch (tab) {
    case "overview":
      return renderOverviewTab(state);
    case "llm":
      return renderConfigPanel(state);
    case "sources":
      return renderSourcesTab(state);
    case "tools":
      return renderToolsTab(state);
    case "drafts":
      return renderDraftsPanel(state);
    case "publish":
      return renderPublishPanel(state);
    case "patrol":
      return renderPatrolTab(state);
    default:
      return nothing;
  }
}

// ---------- Overview 子 tab ----------

function renderOverviewTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  const isConfigured = config?.status === "configured";
  const toolCount = config?.tools?.length ?? 0;
  const sourceCount = config?.trending_sources?.length ?? 0;
  const draftCount = state.mediaDrafts?.length ?? 0;

  return html`
    <div style="display:flex;flex-direction:column;gap:16px;">
      ${renderProgressBanner(state)}
      ${renderHeartbeatPanel(state.mediaHeartbeat)}

      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:12px;">
        <div style="padding:16px;border-radius:10px;background:var(--bg-secondary);text-align:center;">
          <div style="font-size:24px;font-weight:700;color:var(--accent);">${toolCount}</div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">${t("media.subtab.tools")}</div>
        </div>
        <div style="padding:16px;border-radius:10px;background:var(--bg-secondary);text-align:center;">
          <div style="font-size:24px;font-weight:700;color:var(--accent);">${sourceCount}</div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">${t("media.subtab.sources")}</div>
        </div>
        <div style="padding:16px;border-radius:10px;background:var(--bg-secondary);text-align:center;">
          <div style="font-size:24px;font-weight:700;color:var(--accent);">${draftCount}</div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">${t("media.subtab.drafts")}</div>
        </div>
        <div style="padding:16px;border-radius:10px;background:var(--bg-secondary);text-align:center;">
          <div style="font-size:24px;font-weight:700;color:${isConfigured ? "var(--accent)" : "#f59e0b"};">
            ${isConfigured ? "LLM" : "—"}
          </div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">${t("media.subtab.llm")}</div>
        </div>
      </div>

      ${!isConfigured ? html`
        <div class="callout" style="display:flex;align-items:center;gap:12px;">
          <span>${t("media.overview.notConfigured")}</span>
          <button
            class="btn primary"
            @click=${() => {
              state.mediaManageSubTab = "llm";
              (state as any).requestUpdate?.();
            }}
          >
            ${t("media.overview.configureNow")}
          </button>
        </div>
      ` : nothing}
    </div>
  `;
}

// ---------- Tools 子 tab ----------

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

// 可切换的工具（media_publish / social_interact）
const TOGGLEABLE_TOOLS = new Set(["media_publish", "social_interact"]);

function renderToolsTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  if (!config) {
    return html`<div class="muted">${t("common.loading")}</div>`;
  }

  // 从 config 中推断当前启用状态
  const enabledTools = new Set((config.tools || []).map((ti: any) => ti.name));

  // 所有已知工具（含未启用的）
  const allToolNames = ["trending_topics", "content_compose", "media_publish", "social_interact"];

  return html`
    <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:12px;">
      ${allToolNames.map((name) => {
        const tool = (config.tools || []).find((ti: any) => ti.name === name);
        const enabled = enabledTools.has(name);
        const toggleable = TOGGLEABLE_TOOLS.has(name);

        return html`
          <div style="padding:14px 16px;border-radius:10px;background:var(--bg-secondary);display:flex;align-items:flex-start;gap:10px;opacity:${enabled ? 1 : 0.5};">
            <span style="font-size:20px;flex-shrink:0;">${TOOL_ICONS[name] || "🔧"}</span>
            <div style="flex:1;min-width:0;">
              <div style="display:flex;align-items:center;gap:8px;">
                <span style="font-size:13px;font-weight:600;">${TOOL_LABELS[name] || name}</span>
                ${enabled
                  ? html`<span class="chip chip-ok" style="font-size:10px;">${tool?.status === "configured" ? "已配置" : "已启用"}</span>`
                  : html`<span class="chip chip-muted" style="font-size:10px;">未启用</span>`}
              </div>
              <div style="font-size:12px;color:var(--text-muted);margin-top:4px;line-height:1.4;">
                ${tool?.description || TOOL_LABELS[name] || name}
              </div>
              ${toggleable ? html`
                <label style="display:flex;align-items:center;gap:6px;margin-top:8px;cursor:pointer;font-size:12px;">
                  <input
                    type="checkbox"
                    .checked=${enabled}
                    @change=${(e: Event) => {
                      const checked = (e.target as HTMLInputElement).checked;
                      void toggleMediaTool(state, name, checked);
                    }}
                  />
                  ${enabled ? "启用中" : "已关闭"}
                </label>
              ` : nothing}
            </div>
          </div>
        `;
      })}
    </div>
  `;
}

// ---------- Sources 子 tab ----------

const ALL_SOURCES = ["weibo", "baidu", "zhihu"] as const;
const SOURCE_LABELS: Record<string, string> = {
  weibo: "微博热搜",
  baidu: "百度热搜",
  zhihu: "知乎热榜",
};

function renderSourcesTab(state: AppViewState): TemplateResult {
  const config = state.mediaConfig;
  // 当前启用的源（空=全部启用）
  const registeredSources = (config?.trending_sources || []).map((si: any) => si.name);
  // nil（未配置）= 全部启用；空数组 = 全部禁用
  const hasExplicitConfig = Array.isArray(config?.trending_sources) && config.trending_sources.length > 0;
  const allEnabled = !hasExplicitConfig;

  return html`
    <div style="display:flex;flex-direction:column;gap:16px;">
      <div class="card" style="padding:14px 16px;">
        <div style="font-size:13px;font-weight:600;margin-bottom:10px;">热点来源开关</div>
        <div style="display:flex;gap:16px;flex-wrap:wrap;">
          ${ALL_SOURCES.map((name) => {
            const enabled = allEnabled || registeredSources.includes(name);
            return html`
              <label style="display:flex;align-items:center;gap:6px;cursor:pointer;font-size:13px;">
                <input
                  type="checkbox"
                  .checked=${enabled}
                  @change=${(e: Event) => {
                    const checked = (e.target as HTMLInputElement).checked;
                    void toggleMediaSource(state, name, checked);
                  }}
                />
                ${SOURCE_LABELS[name] || name}
              </label>
            `;
          })}
        </div>
        <div style="font-size:11px;color:var(--text-muted);margin-top:8px;">
          取消勾选将禁用该来源的热点抓取（需重启子系统生效）
        </div>
      </div>
      ${renderTrendingPanel(state)}
    </div>
  `;
}

// ---------- Patrol 子 tab ----------

function renderPatrolTab(state: AppViewState): TemplateResult {
  return html`
    <div style="display:flex;flex-direction:column;gap:16px;">
      ${renderPatrolPanel(state)}
      ${renderHeartbeatPanel(state.mediaHeartbeat)}
    </div>
  `;
}
