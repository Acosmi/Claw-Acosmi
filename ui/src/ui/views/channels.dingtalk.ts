import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { ChannelsProps } from "./channels.types.ts";

export function renderDingTalkCard(params: {
  props: ChannelsProps;
  dingtalk?: Record<string, unknown> | null;
  accountCountLabel: unknown;
}) {
  const { dingtalk, accountCountLabel } = params;
  const configured = dingtalk?.configured === true;

  return html`
    ${accountCountLabel}

    <div class="status-list">
      <div>
        <span class="label">${t("channels.configured")}</span>
        <span>${configured ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.running")}</span>
        <span>${dingtalk?.running ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.connected")}</span>
        <span>${dingtalk?.connected ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.appKey")}</span>
        <span>${dingtalk?.appKey ?? "—"}</span>
      </div>
      <div>
        <span class="label">${t("channels.robotCode")}</span>
        <span>${dingtalk?.robotCode ?? "—"}</span>
      </div>
    </div>

    ${dingtalk?.lastError
      ? html`<div class="callout danger">${dingtalk.lastError}</div>`
      : nothing
    }
  `;
}
