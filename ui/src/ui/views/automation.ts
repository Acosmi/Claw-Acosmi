import { html, nothing, type TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import type { SubAgentEntry } from "../controllers/subagents.ts";
import { loadAutomationHub, loadEmailAutomationDetail } from "../controllers/automation.ts";
import { t } from "../i18n.ts";
import { titleForTab } from "../navigation.ts";
import type { ChannelAccountSnapshot } from "../types.ts";
import { renderEmailAutomationDetail } from "./automation.email.ts";

type AutomationKind = "tool" | "subagent";
type AutomationTone = "ok" | "warn" | "danger" | "muted";

type AutomationCardModel = {
  id: string;
  kind: AutomationKind;
  title: string;
  description: string;
  statusLabel: string;
  tone: AutomationTone;
  targetLabel: string;
  metrics: Array<{ label: string; value: string }>;
  onOpen: () => void;
};

function toneChipClass(tone: AutomationTone): string {
  switch (tone) {
    case "ok":
      return "chip-ok";
    case "warn":
      return "chip-warn";
    case "danger":
      return "chip-danger";
    default:
      return "";
  }
}

function renderStatusChip(label: string, tone: AutomationTone): TemplateResult {
  return html`<span class="chip ${toneChipClass(tone)}">${label}</span>`;
}

function subagentTone(status: SubAgentEntry["status"] | undefined): AutomationTone {
  switch (status) {
    case "running":
    case "available":
      return "ok";
    case "degraded":
    case "error":
      return "danger";
    case "starting":
      return "muted";
    case "stopped":
    default:
      return "warn";
  }
}

function subagentStatusLabel(status: SubAgentEntry["status"] | undefined): string {
  switch (status) {
    case "running":
      return t("subagents.status.running");
    case "available":
      return t("subagents.status.available");
    case "degraded":
      return t("subagents.status.degraded");
    case "starting":
      return t("subagents.status.starting");
    case "error":
      return t("subagents.status.error");
    case "stopped":
      return t("subagents.status.stopped");
    default:
      return t("automation.status.unavailable");
  }
}

function subagentEntry(state: AppViewState, id: string): SubAgentEntry | null {
  return state.subagentsList.find((entry) => entry.id === id) ?? null;
}

function emailAccounts(state: AppViewState): ChannelAccountSnapshot[] {
  const snapshot = state.channelsSnapshot;
  if (!snapshot?.channelAccounts) {
    return [];
  }
  const accounts = snapshot.channelAccounts.email;
  return Array.isArray(accounts) ? accounts : [];
}

function configuredEmailAccounts(accounts: ChannelAccountSnapshot[]): number {
  return accounts.filter((account) => account.configured).length;
}

function activeEmailAccounts(accounts: ChannelAccountSnapshot[]): number {
  return accounts.filter((account) => account.connected || account.running).length;
}

function renderSummaryCard(label: string, value: string): TemplateResult {
  return html`
    <div class="stat stat-card">
      <div class="stat-label">${label}</div>
      <div class="stat-value">${value}</div>
    </div>
  `;
}

function renderAutomationCard(card: AutomationCardModel): TemplateResult {
  return html`
    <button type="button" class="automation-card automation-card--${card.kind}" @click=${card.onOpen}>
      <div class="automation-card__header">
        <div class="automation-card__heading">
          <div class="automation-card__title">${card.title}</div>
          <div class="automation-card__desc">${card.description}</div>
        </div>
        <div class="automation-card__badges">
          <span class="automation-kind automation-kind--${card.kind}">
            ${card.kind === "tool" ? t("automation.type.tool") : t("automation.type.subagent")}
          </span>
          ${renderStatusChip(card.statusLabel, card.tone)}
        </div>
      </div>

      <div class="automation-card__metrics">
        ${card.metrics.map(
          (metric) => html`
            <div class="automation-card__metric">
              <span class="automation-card__metric-label">${metric.label}</span>
              <span class="automation-card__metric-value">${metric.value}</span>
            </div>
          `,
        )}
      </div>

      <div class="automation-card__footer">
        <span>${t("automation.action.openDetail")}</span>
        <span class="automation-card__target">
          ${t("automation.openTarget", { target: card.targetLabel })}
        </span>
      </div>
    </button>
  `;
}

function buildToolCards(state: AppViewState): AutomationCardModel[] {
  const browser = (() => {
    if (!state.connected) {
      return {
        statusLabel: t("automation.status.unavailable"),
        tone: "muted" as AutomationTone,
      };
    }
    if (state.browserToolError) {
      return {
        statusLabel: t("subagents.status.error"),
        tone: "danger" as AutomationTone,
      };
    }
    if (state.browserToolConfig?.configured && state.browserToolConfig.enabled) {
      return {
        statusLabel: t("channels.active"),
        tone: "ok" as AutomationTone,
      };
    }
    if (state.browserToolConfig?.configured) {
      return {
        statusLabel: t("automation.status.disabled"),
        tone: "warn" as AutomationTone,
      };
    }
    return {
      statusLabel: t("plugins.status.notConfigured"),
      tone: "warn" as AutomationTone,
    };
  })();

  const browserConfig = state.browserToolConfig;
  const browserMode = !browserConfig?.configured
    ? t("automation.value.notSet")
    : browserConfig.cdpUrl?.trim()
      ? t("automation.value.remote")
      : browserConfig.headless
        ? t("automation.value.headless")
        : t("automation.value.local");

  const emails = emailAccounts(state);
  const emailConfigured = configuredEmailAccounts(emails);
  const emailActive = activeEmailAccounts(emails);
  const emailStatus = !state.connected
    ? { statusLabel: t("automation.status.unavailable"), tone: "muted" as AutomationTone }
    : emailActive > 0
      ? { statusLabel: t("channels.connected"), tone: "ok" as AutomationTone }
      : emailConfigured > 0
        ? { statusLabel: t("plugins.status.configured"), tone: "warn" as AutomationTone }
        : { statusLabel: t("plugins.status.notConfigured"), tone: "warn" as AutomationTone };

  const mcpServerCount = state.mcpServersList.length;
  const mcpRunningCount = state.mcpServersList.filter((server) =>
    server.state === "ready" || server.state === "starting"
  ).length;
  const mcpToolCount = state.mcpToolsList.length;
  const mcpStatus = !state.connected
    ? { statusLabel: t("automation.status.unavailable"), tone: "muted" as AutomationTone }
    : mcpRunningCount > 0
      ? { statusLabel: t("mcp.state.ready"), tone: "ok" as AutomationTone }
      : mcpServerCount > 0
        ? { statusLabel: t("mcp.state.stopped"), tone: "warn" as AutomationTone }
        : { statusLabel: t("automation.status.empty"), tone: "warn" as AutomationTone };

  return [
    {
      id: "config-assistant",
      kind: "tool",
      title: t("automation.card.configAssistant.title"),
      description: t("automation.card.configAssistant.desc"),
      statusLabel: t("automation.status.placeholder"),
      tone: "muted",
      targetLabel: titleForTab("overview"),
      metrics: [
        {
          label: t("automation.metric.scope"),
          value: t("automation.value.system"),
        },
        {
          label: t("automation.metric.entry"),
          value: t("automation.value.wizard"),
        },
      ],
      onOpen: () => {
        if (state.handleStartWizardV2) {
          void state.handleStartWizardV2();
          return;
        }
        state.setTab("overview");
      },
    },
    {
      id: "browser",
      kind: "tool",
      title: t("tools.browser.title"),
      description: t("tools.browser.desc"),
      statusLabel: browser.statusLabel,
      tone: browser.tone,
      targetLabel: titleForTab("plugins"),
      metrics: [
        {
          label: t("automation.metric.mode"),
          value: browserMode,
        },
        {
          label: t("automation.metric.execution"),
          value: browserConfig?.evaluateEnabled ? t("automation.value.enabled") : t("automation.value.disabled"),
        },
      ],
      onOpen: () => {
        state.pluginsPanel = "tools";
        state.setTab("plugins");
      },
    },
    {
      id: "email",
      kind: "tool",
      title: t("automation.email.title"),
      description: t("automation.email.desc"),
      statusLabel: emailStatus.statusLabel,
      tone: emailStatus.tone,
      targetLabel: t("automation.target.email"),
      metrics: [
        {
          label: t("automation.metric.accounts"),
          value: String(emails.length),
        },
        {
          label: t("automation.metric.active"),
          value: String(emailActive),
        },
      ],
      onOpen: () => {
        state.automationPanel = "email";
        state.setTab("automation");
        void loadEmailAutomationDetail(state);
      },
    },
    {
      id: "mcp",
      kind: "tool",
      title: t("mcp.title"),
      description: t("nav.sub.mcp"),
      statusLabel: mcpStatus.statusLabel,
      tone: mcpStatus.tone,
      targetLabel: titleForTab("mcp"),
      metrics: [
        {
          label: t("automation.metric.servers"),
          value: String(mcpServerCount),
        },
        {
          label: t("automation.metric.tools"),
          value: String(mcpToolCount),
        },
      ],
      onOpen: () => {
        state.mcpSubTab = "servers";
        state.setTab("mcp");
      },
    },
  ];
}

function buildSubagentCards(state: AppViewState): AutomationCardModel[] {
  const coder = subagentEntry(state, "oa-coder");
  const argus = subagentEntry(state, "argus-screen");
  const media = subagentEntry(state, "oa-media");
  const mediaConfig = state.mediaConfig;
  const enabledSourceCount = mediaConfig?.enabled_sources?.length ?? mediaConfig?.trending_sources?.length ?? 0;

  return [
    {
      id: "oa-coder",
      kind: "subagent",
      title: coder?.label || "Open Coder",
      description: t("automation.card.coder.desc"),
      statusLabel: subagentStatusLabel(coder?.status),
      tone: subagentTone(coder?.status),
      targetLabel: titleForTab("agents"),
      metrics: [
        {
          label: t("automation.metric.provider"),
          value: coder?.configured ? (coder.provider || t("automation.value.notSet")) : t("subagents.openCoder.followsMain"),
        },
        {
          label: t("automation.metric.model"),
          value: coder?.configured ? (coder.model || t("automation.value.notSet")) : t("subagents.openCoder.followsMain"),
        },
      ],
      onOpen: () => {
        state.agentsSelectedId = "oa-coder";
        state.setTab("agents");
      },
    },
    {
      id: "oa-media",
      kind: "subagent",
      title: mediaConfig?.label || media?.label || t("nav.tab.media"),
      description: t("nav.sub.media"),
      statusLabel: media
        ? subagentStatusLabel(media.status)
        : mediaConfig?.publish_configured
          ? t("subagents.media.configured")
          : t("subagents.media.notConfigured"),
      tone: media ? subagentTone(media.status) : mediaConfig?.publish_configured ? "ok" : "warn",
      targetLabel: titleForTab("media"),
      metrics: [
        {
          label: t("automation.metric.sources"),
          value: String(enabledSourceCount),
        },
        {
          label: t("automation.metric.publishers"),
          value: String(mediaConfig?.publishers?.length ?? 0),
        },
      ],
      onOpen: () => {
        state.mediaManageSubTab = "overview";
        state.setTab("media");
      },
    },
    {
      id: "argus-screen",
      kind: "subagent",
      title: argus?.label || "Vision Observer",
      description: t("automation.card.argus.desc"),
      statusLabel: subagentStatusLabel(argus?.status),
      tone: subagentTone(argus?.status),
      targetLabel: titleForTab("agents"),
      metrics: [
        {
          label: t("automation.metric.model"),
          value: argus?.model && argus.model !== "none" ? argus.model : t("automation.value.notSet"),
        },
        {
          label: t("automation.metric.interval"),
          value: argus?.intervalMs ? `${argus.intervalMs}ms` : t("automation.value.notSet"),
        },
      ],
      onOpen: () => {
        state.agentsSelectedId = "argus-screen";
        state.setTab("agents");
      },
    },
  ];
}

export function renderAutomation(state: AppViewState): TemplateResult {
  if (state.automationPanel === "email") {
    return renderEmailAutomationDetail(state);
  }

  const toolCards = buildToolCards(state);
  const subagentCards = buildSubagentCards(state);
  const isRefreshing =
    state.channelsLoading ||
    state.browserToolLoading ||
    state.mcpServersLoading ||
    state.mcpToolsLoading ||
    state.subagentsLoading;

  return html`
    <div class="automation-hub">
      <section class="card automation-hero">
        <div class="automation-hero__header">
          <div>
            <div class="card-title">${t("automation.hero.title")}</div>
            <div class="card-sub">${t("automation.hero.sub")}</div>
          </div>
          <button class="btn" ?disabled=${isRefreshing} @click=${() => void loadAutomationHub(state)}>
            ${isRefreshing ? t("common.loading") : t("common.refresh")}
          </button>
        </div>

        ${!state.connected
          ? html`<div class="callout info" style="margin-top: 14px;">${t("automation.hero.offline")}</div>`
          : nothing}

        <div class="grid grid-cols-3 automation-summary-grid">
          ${renderSummaryCard(t("automation.summary.total"), String(toolCards.length + subagentCards.length))}
          ${renderSummaryCard(t("automation.summary.tools"), String(toolCards.length))}
          ${renderSummaryCard(t("automation.summary.subagents"), String(subagentCards.length))}
        </div>
      </section>

      <section class="automation-section">
        <div class="automation-section__header">
          <div class="card-title">${t("automation.section.tools")}</div>
          <div class="card-sub">${t("automation.section.toolsSub")}</div>
        </div>
        <div class="automation-card-grid">
          ${toolCards.map((card) => renderAutomationCard(card))}
        </div>
      </section>

      <section class="automation-section">
        <div class="automation-section__header">
          <div class="card-title">${t("automation.section.subagents")}</div>
          <div class="card-sub">${t("automation.section.subagentsSub")}</div>
        </div>
        <div class="automation-card-grid">
          ${subagentCards.map((card) => renderAutomationCard(card))}
        </div>
      </section>
    </div>
  `;
}
