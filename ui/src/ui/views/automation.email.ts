import { html, nothing, type TemplateResult } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { loadEmailAutomationDetail } from "../controllers/automation.ts";
import { updateConfigFormValue } from "../controllers/config.ts";
import type { ChannelAccountSnapshot } from "../types.ts";
import { t } from "../i18n.ts";
import { renderChannelConfigForm } from "./channels.config.ts";
import { renderEmailCard } from "./channels.email.ts";
import { renderChannelAccountCount } from "./channels.shared.ts";

function toneChipClass(tone: "ok" | "warn" | "danger" | "muted"): string {
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

function renderSummaryCard(label: string, value: string): TemplateResult {
  return html`
    <div class="stat stat-card">
      <div class="stat-label">${label}</div>
      <div class="stat-value">${value}</div>
    </div>
  `;
}

function emailAccounts(state: AppViewState): ChannelAccountSnapshot[] {
  return state.channelsSnapshot?.channelAccounts?.email ?? [];
}

function configuredEmailAccounts(accounts: ChannelAccountSnapshot[]): number {
  return accounts.filter((account) => account.configured).length;
}

function activeEmailAccounts(accounts: ChannelAccountSnapshot[]): number {
  return accounts.filter((account) => account.connected || account.running).length;
}

function resolveDefaultAccountId(state: AppViewState): string {
  const snapshotValue = state.channelsSnapshot?.channelDefaultAccountId?.email?.trim();
  if (snapshotValue) {
    return snapshotValue;
  }
  const channels = (state.configForm?.channels ?? state.configSnapshot?.config?.channels ?? null) as
    | Record<string, unknown>
    | null;
  const emailConfig = (channels?.email ?? null) as Record<string, unknown> | null;
  const configuredValue = typeof emailConfig?.defaultAccount === "string"
    ? emailConfig.defaultAccount.trim()
    : "";
  return configuredValue || t("automation.value.notSet");
}

function resolveEmailStatus(state: AppViewState, accounts: ChannelAccountSnapshot[]) {
  if (!state.connected) {
    return { label: t("automation.status.unavailable"), tone: "muted" as const };
  }
  if (activeEmailAccounts(accounts) > 0) {
    return { label: t("channels.connected"), tone: "ok" as const };
  }
  if (configuredEmailAccounts(accounts) > 0) {
    return { label: t("plugins.status.configured"), tone: "warn" as const };
  }
  return { label: t("plugins.status.notConfigured"), tone: "warn" as const };
}

function uniqueMessages(...messages: Array<string | null | undefined>): string[] {
  return [...new Set(messages.map((entry) => entry?.trim()).filter((entry): entry is string => Boolean(entry)))];
}

export function renderEmailAutomationDetail(state: AppViewState): TemplateResult {
  const accounts = emailAccounts(state);
  const configuredCount = configuredEmailAccounts(accounts);
  const activeCount = activeEmailAccounts(accounts);
  const defaultAccountId = resolveDefaultAccountId(state);
  const status = resolveEmailStatus(state, accounts);
  const loadingRuntime = state.channelsLoading;
  const loadingConfig = state.configLoading || state.configSchemaLoading;
  const disabled = state.configSaving || state.configSchemaLoading;
  const errorMessages = uniqueMessages(state.channelsError, state.lastError);

  return html`
    <div class="automation-detail">
      <section class="card automation-detail__hero automation-detail__hero--email">
        <div class="automation-detail__toolbar">
          <button
            class="btn"
            @click=${() => {
              state.automationPanel = "hub";
              state.setTab("automation");
            }}
          >
            ${t("automation.email.back")}
          </button>
          <button
            class="btn"
            ?disabled=${loadingRuntime || loadingConfig}
            @click=${() => void loadEmailAutomationDetail(state)}
          >
            ${loadingRuntime || loadingConfig ? t("common.loading") : t("common.refresh")}
          </button>
        </div>

        <div class="automation-detail__eyebrow">
          <span class="automation-kind automation-kind--tool">${t("automation.type.tool")}</span>
          <span class="chip ${toneChipClass(status.tone)}">${status.label}</span>
          <span class="automation-detail__storage">${t("automation.email.storagePath")} <code>channels.email</code></span>
        </div>

        <div class="automation-detail__headline">
          <div class="automation-detail__title">${t("automation.email.title")}</div>
          <div class="automation-detail__desc">${t("automation.email.desc")}</div>
        </div>

        ${!state.connected
          ? html`<div class="callout info">${t("automation.hero.offline")}</div>`
          : nothing}

        ${errorMessages.map((message) => html`<div class="callout danger">${message}</div>`)}

        <div class="grid grid-cols-4 automation-detail__summary">
          ${renderSummaryCard(t("automation.metric.accounts"), String(accounts.length))}
          ${renderSummaryCard(t("automation.metric.active"), String(activeCount))}
          ${renderSummaryCard(t("automation.metric.defaultAccount"), defaultAccountId)}
          ${renderSummaryCard(
            t("automation.metric.status"),
            configuredCount > 0 ? status.label : t("plugins.status.notConfigured"),
          )}
        </div>
      </section>

      <div class="automation-detail__grid automation-detail__grid--email">
        <section class="card automation-detail__section">
          <div class="automation-detail__section-header">
            <div>
              <div class="card-title">${t("automation.email.runtimeTitle")}</div>
              <div class="card-sub">${t("automation.email.runtimeSub")}</div>
            </div>
          </div>

          ${loadingRuntime && accounts.length === 0
            ? html`<div class="muted">${t("automation.email.loadingRuntime")}</div>`
            : renderEmailCard({
              props: {
                connected: state.connected,
                loading: state.channelsLoading,
                snapshot: state.channelsSnapshot,
                lastError: state.channelsError,
                lastSuccessAt: state.channelsLastSuccess,
                whatsappMessage: state.whatsappLoginMessage,
                whatsappQrDataUrl: state.whatsappLoginQrDataUrl,
                whatsappConnected: state.whatsappLoginConnected,
                whatsappBusy: state.whatsappBusy,
                configSchema: state.configSchema,
                configSchemaLoading: state.configSchemaLoading,
                configForm: state.configForm,
                configUiHints: state.configUiHints,
                configSaving: state.configSaving,
                configFormDirty: state.configFormDirty,
                nostrProfileFormState: state.nostrProfileFormState,
                nostrProfileAccountId: state.nostrProfileAccountId,
                onRefresh: (_probe: boolean) => undefined,
                onWhatsAppStart: (_force: boolean) => undefined,
                onWhatsAppWait: () => undefined,
                onWhatsAppLogout: () => undefined,
                onConfigPatch: (_path: Array<string | number>, _value: unknown) => undefined,
                onConfigSave: async () => false,
                onConfigReload: () => undefined,
                onNostrProfileEdit: (_accountId: string, _profile) => undefined,
                onNostrProfileCancel: () => undefined,
                onNostrProfileFieldChange: (_field, _value: string) => undefined,
                onNostrProfileSave: () => undefined,
                onNostrProfileImport: () => undefined,
                onNostrProfileToggleAdvanced: () => undefined,
              },
              emailAccounts: accounts,
              accountCountLabel: renderChannelAccountCount("email", state.channelsSnapshot?.channelAccounts),
              onTestConnection: (accountId) => void state.handleEmailTest(accountId),
              emailTestLoading: state.emailTestLoading,
              emailTestResult: state.emailTestResult,
            })}
        </section>

        <section class="card automation-detail__section">
          <div class="automation-detail__section-header">
            <div>
              <div class="card-title">${t("automation.email.configTitle")}</div>
              <div class="card-sub">${t("automation.email.configSub")}</div>
            </div>
          </div>

          ${loadingConfig
            ? html`<div class="muted">${t("automation.email.loadingConfig")}</div>`
            : renderChannelConfigForm({
              channelId: "email",
              configValue: state.configForm,
              schema: state.configSchema,
              uiHints: state.configUiHints,
              disabled,
              onPatch: (path, value) => updateConfigFormValue(state, path, value),
            })}

          <div class="automation-detail__footer">
            <button class="btn" ?disabled=${disabled} @click=${() => void state.handleChannelConfigReload()}>
              ${t("channels.reload")}
            </button>
            <button
              class="btn primary"
              ?disabled=${disabled || !state.configFormDirty}
              @click=${() => void state.handleChannelConfigSave()}
            >
              ${state.configSaving ? t("channels.saving") : t("channels.save")}
            </button>
          </div>
        </section>
      </div>
    </div>
  `;
}
