import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { SubAgentEntry, SubAgentsState } from "../controllers/subagents.ts";

// ---------- SubAgents View ----------

export type SubAgentsProps = {
    loading: boolean;
    agents: SubAgentEntry[];
    error: string | null;
    busyKey: string | null;
    onToggle: (agentId: string, enabled: boolean) => void;
    onSetInterval: (agentId: string, ms: number) => void;
    onSetGoal: (agentId: string, goal: string) => void;
    onSetModel: (agentId: string, model: string) => void;
    onRefresh: () => void;
};

const VLA_MODELS = [
    { value: "none", label: "None (Screenshot Only)" },
    { value: "anthropic", label: "Claude Vision" },
    { value: "gemini", label: "Gemini Flash" },
    { value: "qwen", label: "Qwen VL" },
    { value: "ollama", label: "Ollama (Local)" },
];

export function renderSubAgents(props: SubAgentsProps) {
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

      <div class="subagents-list" style="margin-top: 16px;">
        ${props.agents.length === 0
            ? html`<div class="muted">${t("subagents.empty")}</div>`
            : props.agents.map((agent) => renderSubAgentCard(agent, props))}
      </div>
    </section>
  `;
}

function renderSubAgentCard(agent: SubAgentEntry, props: SubAgentsProps) {
    const busy = props.busyKey === agent.id;
    const statusClass =
        agent.status === "running"
            ? "chip-ok"
            : agent.status === "error"
                ? "chip-danger"
                : "chip-muted";
    const statusLabel =
        agent.status === "running"
            ? t("subagents.status.running")
            : agent.status === "error"
                ? t("subagents.status.error")
                : t("subagents.status.stopped");

    return html`
    <div class="subagent-card">
      <div class="subagent-header">
        <div class="subagent-info">
          <span class="subagent-icon">${agent.id === "argus-screen" ? "👁" : "🔧"}</span>
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

      ${agent.error
            ? html`<div class="callout danger" style="margin-top: 8px;">${agent.error}</div>`
            : nothing}
    </div>
  `;
}
