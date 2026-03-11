import { html, nothing } from "lit";
import type { WhatsAppStatus } from "../types.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { formatRelativeTimestamp, formatDurationHuman } from "../format.ts";
import { t } from "../i18n.ts";

export function renderWhatsAppCard(params: {
  props: ChannelsProps;
  whatsapp?: WhatsAppStatus;
  accountCountLabel: unknown;
}) {
  const { props, whatsapp, accountCountLabel } = params;

  return html`
    ${accountCountLabel}

    <div class="status-list">
      <div>
        <span class="label">${t("channels.configured")}</span>
        <span>${whatsapp?.configured ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.linked")}</span>
        <span>${whatsapp?.linked ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.running")}</span>
        <span>${whatsapp?.running ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.connected")}</span>
        <span>${whatsapp?.connected ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.lastConnect")}</span>
        <span>
          ${whatsapp?.lastConnectedAt ? formatRelativeTimestamp(whatsapp.lastConnectedAt) : "n/a"}
        </span>
      </div>
      <div>
        <span class="label">${t("channels.lastMessage")}</span>
        <span>
          ${whatsapp?.lastMessageAt ? formatRelativeTimestamp(whatsapp.lastMessageAt) : "n/a"}
        </span>
      </div>
      <div>
        <span class="label">${t("channels.authAge")}</span>
        <span>
          ${whatsapp?.authAgeMs != null ? formatDurationHuman(whatsapp.authAgeMs) : "n/a"}
        </span>
      </div>
    </div>

    ${whatsapp?.lastError
      ? html`<div class="callout danger">
          ${whatsapp.lastError}
        </div>`
      : nothing
    }

    ${props.whatsappMessage
      ? html`<div class="callout">
          ${props.whatsappMessage}
        </div>`
      : nothing
    }

    ${props.whatsappQrDataUrl
      ? html`<div class="qr-wrap">
          <img src=${props.whatsappQrDataUrl} alt="WhatsApp QR" />
        </div>`
      : nothing
    }

    <div class="row channel-card__action-row">
      <button
        class="btn primary"
        ?disabled=${props.whatsappBusy}
        @click=${() => props.onWhatsAppStart(false)}
      >
        ${props.whatsappBusy ? "Working…" : "Show QR"}
      </button>
      <button
        class="btn"
        ?disabled=${props.whatsappBusy}
        @click=${() => props.onWhatsAppStart(true)}
      >
        Relink
      </button>
      <button
        class="btn"
        ?disabled=${props.whatsappBusy}
        @click=${() => props.onWhatsAppWait()}
      >
        Wait for scan
      </button>
      <button
        class="btn danger"
        ?disabled=${props.whatsappBusy}
        @click=${() => props.onWhatsAppLogout()}
      >
        Logout
      </button>
      <button class="btn" @click=${() => props.onRefresh(true)}>
        Refresh
      </button>
    </div>
  `;
}
