import { html, nothing, type TemplateResult } from "lit";
import { t } from "../i18n.ts";
import type { SubAgentEntry } from "../controllers/subagents.ts";

// ---------- SubAgents View ----------
// @deprecated 子智能体已统一到 agents 标签页侧边栏（views/agents.ts renderSubagentDetailPanel）。
// 此文件保留用于向后兼容，可在后续版本移除。

export type SubAgentsProps = {
  loading: boolean;
  agents: SubAgentEntry[];
  error: string | null;
  busyKey: string | null;
  activeTab: string;
  onTabChange: (tabId: string) => void;
  onNavigateToMedia: () => void;
  onStartMediaWizard?: () => void;
  onToggle: (agentId: string, enabled: boolean) => void;
  onSetInterval: (agentId: string, ms: number) => void;
  onSetGoal: (agentId: string, goal: string) => void;
  onSetModel: (agentId: string, model: string) => void;
  onRefresh: () => void;
  onStartOpenCoderWizard?: () => void;
};

const VLA_MODELS = [
  { value: "none", label: "None (Screenshot Only)" },
  { value: "anthropic", label: "Claude Vision" },
  { value: "gemini", label: "Gemini Flash" },
  { value: "qwen", label: "Qwen VL" },
  { value: "ollama", label: "Ollama (Local)" },
];

export function renderSubAgents(props: SubAgentsProps) {
  const hasActiveAgent = props.agents.some((agent) => agent.id === props.activeTab);
  const activeTab = hasActiveAgent
    ? props.activeTab
    : props.agents[0]?.id ?? "";
  const activeAgent = props.agents.find((agent) => agent.id === activeTab);

  return html`
    <section class="card">
      <div class="row" style="justify-content: space-between;">
        <div>
          <div class="card-title">${t("subagents.title")}</div>
          <div class="card-sub">${t("subagents.subtitle")}</div>
        </div>
        <div class="row" style="gap: 8px;">
          <button class="btn" ?disabled=${props.loading} @click=${props.onRefresh}>
            ${props.loading ? t("common.loading") : t("common.refresh")}
          </button>
        </div>
      </div>

      ${props.error
      ? html`<div class="callout danger" style="margin-top: 12px;">${props.error}</div>`
      : nothing}

      <div class="agent-tabs" style="margin-top: 16px;">
        ${props.agents.map(
        (agent) => html`
            <button
              class="agent-tab ${activeTab === agent.id ? "active" : ""}"
              @click=${() => props.onTabChange(agent.id)}
            >
              ${agent.label}
            </button>
          `,
      )}
      </div>

      ${props.agents.length === 0
      ? html`<div class="muted" style="margin-top: 16px;">${t("subagents.empty")}</div>`
      : activeAgent
        ? html`<div class="subagents-list" style="margin-top: 16px;">${renderSubAgentCard(activeAgent, props)}</div>`
        : html`<div class="muted" style="margin-top: 16px;">${t("subagents.empty")}</div>`}
    </section>
  `;
}

function renderSubAgentCard(agent: SubAgentEntry, props: SubAgentsProps) {
  const busy = props.busyKey === agent.id;
  const statusClass =
    agent.status === "running" || agent.status === "available"
      ? "chip-ok"
      : agent.status === "error" || agent.status === "degraded"
        ? "chip-danger"
        : agent.status === "starting"
          ? "chip-warn"
          : "chip-muted"; // stopped
  const statusLabel =
    agent.status === "running" ? t("subagents.status.running")
      : agent.status === "available" ? t("subagents.status.available")
        : agent.status === "degraded" ? t("subagents.status.degraded")
          : agent.status === "starting" ? t("subagents.status.starting")
            : agent.status === "error" ? t("subagents.status.error")
              : t("subagents.status.stopped");

  return html`
    <div class="subagent-card">
      <div class="subagent-header">
        <div class="subagent-info">
          <span class="subagent-icon">${agent.id === "argus-screen" ? "👁" : agent.id === "oa-media" ? "🎬" : "🔧"}</span>
          <div>
            <div class="subagent-name">${agent.label}</div>
            <div class="subagent-id muted">${agent.id}</div>
          </div>
        </div>
        <div class="row" style="gap: 8px; align-items: center;">
          <span class="chip ${statusClass}">${statusLabel}</span>
          <button
            class="btn ${agent.enabled ? "" : "primary"}"
            ?disabled=${busy}
            @click=${() => props.onToggle(agent.id, !agent.enabled)}
          >
            ${busy
      ? t("common.loading")
      : agent.enabled
        ? t("subagents.disable")
        : t("subagents.enable")}
          </button>
        </div>
      </div>

      ${agent.id === "argus-screen"
      ? html`
            <div class="subagent-controls">
              <label class="field">
                <span>${t("subagents.model")}</span>
                <select
                  .value=${agent.model}
                  @change=${(e: Event) =>
          props.onSetModel(agent.id, (e.target as HTMLSelectElement).value)}
                  ?disabled=${busy}
                >
                  ${VLA_MODELS.map(
            (m) => html`
                      <option value=${m.value} ?selected=${m.value === agent.model}>
                        ${m.label}
                      </option>
                    `,
          )}
                </select>
              </label>

              <label class="field">
                <span>${t("subagents.interval")}</span>
                <div class="row" style="gap: 8px; align-items: center;">
                  <input
                    type="range"
                    min="200"
                    max="5000"
                    step="100"
                    .value=${String(agent.intervalMs)}
                    @change=${(e: Event) =>
          props.onSetInterval(
            agent.id,
            parseInt((e.target as HTMLInputElement).value, 10),
          )}
                    ?disabled=${busy}
                    style="flex: 1;"
                  />
                  <span class="muted" style="min-width: 50px; text-align: right;"
                    >${agent.intervalMs}ms</span
                  >
                </div>
              </label>

              <label class="field">
                <span>${t("subagents.goal")}</span>
                <input
                  type="text"
                  .value=${agent.goal}
                  @change=${(e: Event) =>
          props.onSetGoal(agent.id, (e.target as HTMLInputElement).value)}
                  ?disabled=${busy}
                  placeholder=${t("subagents.goalPlaceholder")}
                />
              </label>
            </div>
          `
      : nothing}

      ${agent.id === "oa-coder"
      ? renderOpenCoderControls(agent, props)
      : nothing}

      ${agent.id === "oa-media"
      ? renderMediaControls(agent, props)
      : nothing}

      ${agent.error
      ? html`<div class="callout danger" style="margin-top: 8px;">${agent.error}</div>`
      : nothing}
    </div>
  `;
}

function renderOpenCoderControls(agent: SubAgentEntry, props: SubAgentsProps) {
  const configured = agent.configured ?? false;
  const configChipClass = configured ? "chip-ok" : "chip-muted";
  const configLabel = configured
    ? t("subagents.openCoder.configured")
    : t("subagents.openCoder.notConfigured");

  // 未配置时显示"跟随主配置"，不暴露上游供应商品牌
  const displayProvider = configured
    ? (agent.provider || "—")
    : t("subagents.openCoder.followsMain");
  const displayModel = configured
    ? (agent.model || "—")
    : t("subagents.openCoder.followsMain");

  return html`
    <div class="subagent-controls" style="gap: 10px;">
      <div class="row" style="gap: 12px; flex-wrap: wrap; align-items: center;">
        <div class="row" style="gap: 6px; align-items: center;">
          <span class="muted" style="font-size: 12px;">${t("subagents.openCoder.provider")}</span>
          <span class="chip chip-muted">${displayProvider}</span>
        </div>
        <div class="row" style="gap: 6px; align-items: center;">
          <span class="muted" style="font-size: 12px;">${t("subagents.openCoder.model")}</span>
          <span class="chip chip-muted">${displayModel}</span>
        </div>
        <span class="chip ${configChipClass}">${configLabel}</span>
      </div>
      ${props.onStartOpenCoderWizard
      ? html`
        <button
          class="btn primary"
          style="align-self: flex-start; margin-top: 4px;"
          @click=${() => props.onStartOpenCoderWizard!()}
        >
          ${configured
          ? t("subagents.openCoder.reconfigure")
          : t("subagents.openCoder.setup")}
        </button>
      `
      : nothing}
    </div>
  `;
}

function renderMediaControls(agent: SubAgentEntry, props: SubAgentsProps) {
  const configured = agent.configured ?? false;
  const configChipClass = configured ? "chip-ok" : "chip-muted";
  const configLabel = configured
    ? t("subagents.media.configured")
    : t("subagents.media.notConfigured");

  const displayProvider = configured ? (agent.provider || "—") : t("subagents.media.followsMain");
  const displayModel = configured ? (agent.model || "—") : t("subagents.media.followsMain");

  return html`
    <div class="subagent-controls" style="gap: 10px;">
      <div class="row" style="gap: 12px; flex-wrap: wrap; align-items: center;">
        <div class="row" style="gap: 6px; align-items: center;">
          <span class="muted" style="font-size: 12px;">${t("subagents.media.provider")}</span>
          <span class="chip chip-muted">${displayProvider}</span>
        </div>
        <div class="row" style="gap: 6px; align-items: center;">
          <span class="muted" style="font-size: 12px;">${t("subagents.media.model")}</span>
          <span class="chip chip-muted">${displayModel}</span>
        </div>
        <span class="chip ${configChipClass}">${configLabel}</span>
      </div>
      <div class="row" style="gap: 8px; margin-top: 4px;">
        <button
          class="btn"
          style="align-self: flex-start;"
          @click=${() => props.onNavigateToMedia()}
        >
          ${t("subagents.media.manage")}
        </button>
        ${props.onStartMediaWizard
        ? html`
          <button
            class="btn primary"
            style="align-self: flex-start;"
            @click=${() => props.onStartMediaWizard!()}
          >
            ${configured
            ? t("subagents.media.reconfigure")
            : t("subagents.media.setup")}
          </button>
        `
        : nothing}
      </div>
    </div>
  `;
}
