import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { ChannelsProps } from "./channels.types.ts";

export function renderFeishuCard(params: {
  props: ChannelsProps;
  feishu?: Record<string, unknown> | null;
  accountCountLabel: unknown;
}) {
  const { feishu, accountCountLabel } = params;
  const configured = feishu?.configured === true;

  return html`
    ${accountCountLabel}

    <div class="status-list">
      <div>
        <span class="label">${t("channels.configured")}</span>
        <span>${configured ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.running")}</span>
        <span>${feishu?.running ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.connected")}</span>
        <span>${feishu?.connected ? t("channels.yes") : t("channels.no")}</span>
      </div>
      <div>
        <span class="label">${t("channels.appId")}</span>
        <span>${feishu?.appId ?? "—"}</span>
      </div>
      <div>
        <span class="label">${t("channels.domain")}</span>
        <span>${feishu?.domain === "lark" ? t("channels.domainLark") : t("channels.domainFeishu")}</span>
      </div>
    </div>

    ${feishu?.lastError
      ? html`<div class="callout danger">${feishu.lastError}</div>`
      : nothing
    }
  `;
}
