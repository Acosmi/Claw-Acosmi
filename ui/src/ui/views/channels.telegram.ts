import { html, nothing } from "lit";
import type { ChannelAccountSnapshot, TelegramStatus } from "../types.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { formatRelativeTimestamp } from "../format.ts";
import { t } from "../i18n.ts";

export function renderTelegramCard(params: {
  props: ChannelsProps;
  telegram?: TelegramStatus;
  telegramAccounts: ChannelAccountSnapshot[];
  accountCountLabel: unknown;
}) {
  const { props, telegram, telegramAccounts, accountCountLabel } = params;
  const hasMultipleAccounts = telegramAccounts.length > 1;

  const renderAccountCard = (account: ChannelAccountSnapshot) => {
    const probe = account.probe as { bot?: { username?: string } } | undefined;
    const botUsername = probe?.bot?.username;
    const label = account.name || account.accountId;
    return html`
      <div class="account-card">
        <div class="account-card-header">
          <div class="account-card-title">
            ${botUsername ? `@${botUsername}` : label}
          </div>
          <div class="account-card-id">${account.accountId}</div>
        </div>
        <div class="status-list account-card-status">
          <div>
            <span class="label">${t("channels.running")}</span>
            <span>${account.running ? t("channels.yes") : t("channels.no")}</span>
          </div>
          <div>
            <span class="label">${t("channels.configured")}</span>
            <span>${account.configured ? t("channels.yes") : t("channels.no")}</span>
          </div>
          <div>
            <span class="label">Last inbound</span>
            <span>${account.lastInboundAt ? formatRelativeTimestamp(account.lastInboundAt) : "n/a"}</span>
          </div>
          ${account.lastError
        ? html`
              <div class="account-card-error">
                ${account.lastError}
              </div>
            `
        : nothing
      }
        </div>
      </div>
    `;
  };

  return html`
    ${accountCountLabel}

    ${hasMultipleAccounts
      ? html`
          <div class="account-card-list">
            ${telegramAccounts.map((account) => renderAccountCard(account))}
          </div>
        `
      : html`
          <div class="status-list">
            <div>
              <span class="label">${t("channels.configured")}</span>
              <span>${telegram?.configured ? t("channels.yes") : t("channels.no")}</span>
            </div>
            <div>
              <span class="label">${t("channels.running")}</span>
              <span>${telegram?.running ? t("channels.yes") : t("channels.no")}</span>
            </div>
            <div>
              <span class="label">${t("channels.mode")}</span>
              <span>${telegram?.mode ?? "n/a"}</span>
            </div>
            <div>
              <span class="label">${t("channels.lastStart")}</span>
              <span>${telegram?.lastStartAt ? formatRelativeTimestamp(telegram.lastStartAt) : "n/a"}</span>
            </div>
            <div>
              <span class="label">${t("channels.lastProbe")}</span>
              <span>${telegram?.lastProbeAt ? formatRelativeTimestamp(telegram.lastProbeAt) : "n/a"}</span>
            </div>
          </div>
        `
    }

    ${telegram?.lastError
      ? html`<div class="callout danger">
          ${telegram.lastError}
        </div>`
      : nothing
    }

    ${telegram?.probe
      ? html`<div class="callout">
          Probe ${telegram.probe.ok ? "ok" : "failed"} ·
          ${telegram.probe.status ?? ""} ${telegram.probe.error ?? ""}
        </div>`
      : nothing
    }

    <div class="row channel-card__action-row">
      <button class="btn" @click=${() => props.onRefresh(true)}>
        Probe
      </button>
    </div>
  `;
}
