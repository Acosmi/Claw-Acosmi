/**
 * Channel Config Modal — Overlay for editing channel configuration
 *
 * Renders the channel config form (schema-driven) inside a modal dialog
 * instead of inline expanding within the card.
 */
import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { renderChannelConfigForm } from "./channels.config.ts";
import { channelIcon } from "./channels.icons.ts";

export interface ChannelConfigModalState {
  open: boolean;
  channelId: string | null;
}

export const CHANNEL_CONFIG_MODAL_INITIAL: ChannelConfigModalState = {
  open: false,
  channelId: null,
};

export function renderChannelConfigModal(
  modalState: ChannelConfigModalState,
  props: ChannelsProps,
  onClose: () => void,
) {
  if (!modalState.open || !modalState.channelId) return nothing;

  const channelId = modalState.channelId;
  const disabled = props.configSaving || props.configSchemaLoading;

  const metaList = Array.isArray(props.snapshot?.channelMeta) ? props.snapshot.channelMeta : [];
  const meta = metaList.find((e) => e.id === channelId);
  const label =
    meta?.label ??
    props.snapshot?.channelLabels?.[channelId] ??
    t(`channels.name.${channelId}`) ??
    channelId;

  const icon = channelIcon(channelId);

  return html`
    <div
      class="channel-config-modal__overlay"
      @click=${(e: Event) => {
      if (e.target === e.currentTarget) onClose();
    }}
      @keydown=${(e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    }}
    >
      <div class="channel-config-modal__card">
        <div class="channel-config-modal__header">
          <div class="channel-config-modal__title">
            <span class="channel-config-modal__title-icon">${icon}</span>
            ${label} — ${t("channels.modal.title")}
          </div>
          <button
            class="channel-config-modal__close"
            @click=${onClose}
            title="${t("wizard.close")}"
          >
            ✕
          </button>
        </div>

        <div class="channel-config-modal__body">
          ${props.configSchemaLoading
            ? html`<div class="muted">${t("channels.loadingSchema")}</div>`
            : renderChannelConfigForm({
              channelId,
              configValue: props.configForm,
              schema: props.configSchema,
              uiHints: props.configUiHints,
              disabled,
              onPatch: props.onConfigPatch,
            })}
        </div>

        <div class="channel-config-modal__footer">
          ${props.lastError
            ? html`<div class="callout danger channel-config-modal__error">${props.lastError}</div>`
            : nothing}
          <button class="btn" ?disabled=${disabled} @click=${onClose}>
            ${t("wizard.close")}
          </button>
          <button
            class="btn"
            ?disabled=${disabled}
            @click=${() => props.onConfigReload()}
          >
            ${t("channels.reload")}
          </button>
          <button
            class="btn primary"
            ?disabled=${disabled || !props.configFormDirty}
            @click=${async () => {
              const ok = await props.onConfigSave();
              if (ok) onClose();
            }}
          >
            ${props.configSaving
              ? t("channels.saving")
              : t("channels.save")}
          </button>
        </div>
      </div>
    </div>
  `;
}
