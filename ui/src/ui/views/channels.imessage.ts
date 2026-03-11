import { html, nothing } from "lit";
import type { IMessageStatus } from "../types.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { formatRelativeTimestamp } from "../format.ts";
import { t } from "../i18n.ts";

export function renderIMessageCard(params: {
  props: ChannelsProps;
  imessage?: IMessageStatus | null;
  accountCountLabel: unknown;
}) {
  const { props, imessage, accountCountLabel } = params;

  return html`
    ${accountCountLabel}

    <div class="status-list">
      <div>
        <span class="label">${t("channels.configured")}</span>
        <span>${imessage?.configured ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.running")}</span>
        <span>${imessage?.running ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.lastStart")}</span>
        <span>${imessage?.lastStartAt ? formatRelativeTimestamp(imessage.lastStartAt) : "n/a"}</span>
      </div>
      <div>
        <span class="label">${t("channels.lastProbe")}</span>
        <span>${imessage?.lastProbeAt ? formatRelativeTimestamp(imessage.lastProbeAt) : "n/a"}</span>
      </div>
    </div>

    ${imessage?.lastError
      ? html`<div class="callout danger">
          ${imessage.lastError}
        </div>`
      : nothing
    }

    ${imessage?.probe
      ? html`<div class="callout">
          Probe ${imessage.probe.ok ? "ok" : "failed"} ·
          ${imessage.probe.error ?? ""}
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
