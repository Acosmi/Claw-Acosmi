import { html, nothing } from "lit";
import type { SignalStatus } from "../types.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { formatRelativeTimestamp } from "../format.ts";
import { t } from "../i18n.ts";

export function renderSignalCard(params: {
  props: ChannelsProps;
  signal?: SignalStatus | null;
  accountCountLabel: unknown;
}) {
  const { props, signal, accountCountLabel } = params;

  return html`
    ${accountCountLabel}

    <div class="status-list">
      <div>
        <span class="label">${t("channels.configured")}</span>
        <span>${signal?.configured ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.running")}</span>
        <span>${signal?.running ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">Base URL</span>
        <span>${signal?.baseUrl ?? "n/a"}</span>
      </div>
      <div>
        <span class="label">${t("channels.lastStart")}</span>
        <span>${signal?.lastStartAt ? formatRelativeTimestamp(signal.lastStartAt) : "n/a"}</span>
      </div>
      <div>
        <span class="label">${t("channels.lastProbe")}</span>
        <span>${signal?.lastProbeAt ? formatRelativeTimestamp(signal.lastProbeAt) : "n/a"}</span>
      </div>
    </div>

    ${signal?.lastError
      ? html`<div class="callout danger">
          ${signal.lastError}
        </div>`
      : nothing
    }

    ${signal?.probe
      ? html`<div class="callout">
          Probe ${signal.probe.ok ? "ok" : "failed"} ·
          ${signal.probe.status ?? ""} ${signal.probe.error ?? ""}
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
