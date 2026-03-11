import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { ChannelsProps } from "./channels.types.ts";

export function renderWeComCard(params: {
  props: ChannelsProps;
  wecom?: Record<string, unknown> | null;
  accountCountLabel: unknown;
}) {
  const { wecom, accountCountLabel } = params;
  const configured = wecom?.configured === true;

  return html`
    ${accountCountLabel}

    <div class="status-list">
      <div>
        <span class="label">${t("channels.configured")}</span>
        <span>${configured ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.running")}</span>
        <span>${wecom?.running ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.connected")}</span>
        <span>${wecom?.connected ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.corpId")}</span>
        <span>${wecom?.corpId ?? "—"}</span>
      </div>
      <div>
        <span class="label">${t("channels.agentId")}</span>
        <span>${wecom?.agentId ?? "—"}</span>
      </div>
    </div>

    ${wecom?.lastError
      ? html`<div class="callout danger">${wecom.lastError}</div>`
      : nothing
    }
  `;
}
