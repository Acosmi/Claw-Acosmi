import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type {
  ChannelAccountSnapshot,
  ChannelUiMetaEntry,
  ChannelsStatusSnapshot,
  DiscordStatus,
  GoogleChatStatus,
  IMessageStatus,
  NostrProfile,
  NostrStatus,
  SignalStatus,
  SlackStatus,
  TelegramStatus,
  WhatsAppStatus,
} from "../types.ts";
import type { ChannelKey, ChannelsChannelData, ChannelsProps } from "./channels.types.ts";
import { formatRelativeTimestamp } from "../format.ts";
import { renderDingTalkCard } from "./channels.dingtalk.ts";
import { renderDiscordCard } from "./channels.discord.ts";
import { renderFeishuCard } from "./channels.feishu.ts";
import { renderGoogleChatCard } from "./channels.googlechat.ts";
import { renderIMessageCard } from "./channels.imessage.ts";
import { renderNostrCard } from "./channels.nostr.ts";
import { channelEnabled, renderChannelAccountCount } from "./channels.shared.ts";
import { renderSignalCard } from "./channels.signal.ts";
import { renderSlackCard } from "./channels.slack.ts";
import { renderTelegramCard } from "./channels.telegram.ts";
import { renderWeComCard } from "./channels.wecom.ts";
import { renderWhatsAppCard } from "./channels.whatsapp.ts";
import {
  renderChannelConfigModal,
  type ChannelConfigModalState,
  CHANNEL_CONFIG_MODAL_INITIAL,
} from "./channel-config-modal.ts";
import { channelIcon } from "./channels.icons.ts";

const HIDDEN_CHANNELS = new Set<ChannelKey>(["email"]);

// ─── Config Modal State (module-level) ───

let _configModalState: ChannelConfigModalState = { ...CHANNEL_CONFIG_MODAL_INITIAL };
let _requestUpdate: (() => void) | null = null;

function openConfigModal(channelId: string, requestUpdate?: () => void, onReload?: () => void) {
  _configModalState = { open: true, channelId };
  if (requestUpdate) _requestUpdate = requestUpdate;
  _requestUpdate?.();
  // Ensure config is loaded so configSnapshot.hash exists for save
  onReload?.();
}

function closeConfigModal() {
  _configModalState = { ...CHANNEL_CONFIG_MODAL_INITIAL };
  _requestUpdate?.();
}

// ─── Main Render ───

export function renderChannels(props: ChannelsProps) {
  const channels = props.snapshot?.channels as Record<string, unknown> | null;
  const whatsapp = (channels?.whatsapp ?? undefined) as WhatsAppStatus | undefined;
  const telegram = (channels?.telegram ?? undefined) as TelegramStatus | undefined;
  const discord = (channels?.discord ?? null) as DiscordStatus | null;
  const googlechat = (channels?.googlechat ?? null) as GoogleChatStatus | null;
  const slack = (channels?.slack ?? null) as SlackStatus | null;
  const signal = (channels?.signal ?? null) as SignalStatus | null;
  const imessage = (channels?.imessage ?? null) as IMessageStatus | null;
  const nostr = (channels?.nostr ?? null) as NostrStatus | null;
  const channelOrder = resolveChannelOrder(props.snapshot).filter((key) => !HIDDEN_CHANNELS.has(key));
  const orderedChannels = channelOrder
    .map((key, index) => ({
      key,
      enabled: channelEnabled(key, props),
      order: index,
    }))
    .toSorted((a, b) => {
      if (a.enabled !== b.enabled) {
        return a.enabled ? -1 : 1;
      }
      return a.order - b.order;
    });

  // Always capture requestUpdate so modal open/close can trigger re-render
  const requestUpdate = props.requestUpdate ?? undefined;
  if (requestUpdate) _requestUpdate = requestUpdate;

  const data: ChannelsChannelData = {
    whatsapp,
    telegram,
    discord,
    googlechat,
    slack,
    signal,
    imessage,
    nostr,
    channelAccounts: props.snapshot?.channelAccounts ?? null,
  };

  return html`
    <section class="channels-grid">
      ${orderedChannels.map((channel) =>
        renderChannelCard(channel.key, props, data, requestUpdate, () => props.onConfigReload()),
      )}
    </section>

    ${renderChannelConfigModal(_configModalState, props, closeConfigModal)}

    <details class="card channel-card channel-card--health">
      <summary class="channel-card__summary">
        <div class="channel-card__summary-content">
          <div>
            <div class="channel-card__keyline">
              <code class="channel-card__key">health</code>
            </div>
            <div class="channel-card__name">${t("channels.healthTitle")}</div>
            <div class="channel-card__desc">${t("channels.healthSub")}</div>
          </div>
          <div class="channel-card__summary-time">
            ${props.lastSuccessAt ? formatRelativeTimestamp(props.lastSuccessAt) : "n/a"}
          </div>
        </div>
      </summary>
      ${props.lastError
      ? html`<div class="callout danger channel-card__debug-callout">
            ${props.lastError}
          </div>`
      : nothing
    }
      <pre class="code-block channel-card__debug">
${props.snapshot ? JSON.stringify(props.snapshot, null, 2) : t("channels.noSnapshot")}
      </pre>
    </details>
  `;
}

// ─── Unified Card Wrapper ───

function renderChannelCard(
  key: ChannelKey,
  props: ChannelsProps,
  data: ChannelsChannelData,
  requestUpdate?: () => void,
  onReload?: () => void,
) {
  const isEnabled = channelEnabled(key, props);
  const label = resolveChannelLabel(props.snapshot, key);
  const desc = t(`channels.desc.${key}`) || t("channels.genericSub");
  const icon = channelIcon(key);

  const stateClass = isEnabled ? "channel-card--configured" : "channel-card--unconfigured";

  return html`
    <div
      class="card channel-card ${stateClass}"
      @click=${(e: Event) => {
        // Don't trigger if clicking a button or link inside.
        const target = e.target as HTMLElement;
        if (target.closest("button") || target.closest("a")) return;
        openConfigModal(key, requestUpdate, onReload);
      }}
    >
      <div class="channel-card__top">
        <div class="channel-card__keyline">
          <span class="channel-card__icon">${icon}</span>
          <code class="channel-card__key">${key}</code>
        </div>
        <div
          class="chip channel-card__badge ${isEnabled ? "chip-ok" : "channel-card__badge--pending"}"
        >
          ${isEnabled ? t("channels.badge.configured") : t("channels.badge.notConfigured")}
        </div>
      </div>

      <div class="channel-card__name">${label}</div>
      <div class="channel-card__desc">${desc}</div>

      <div class="channel-card__body">
        ${renderChannelBody(key, props, data)}
      </div>

      <div class="channel-card__actions">
        ${renderCardActions(key, props, isEnabled, requestUpdate, onReload)}
      </div>
    </div>
  `;
}

// ─── Card Body (channel-specific content) ───

function renderChannelBody(key: ChannelKey, props: ChannelsProps, data: ChannelsChannelData) {
  const accountCountLabel = renderChannelAccountCount(key, data.channelAccounts);
  switch (key) {
    case "whatsapp":
      return renderWhatsAppCard({
        props,
        whatsapp: data.whatsapp,
        accountCountLabel,
      });
    case "telegram":
      return renderTelegramCard({
        props,
        telegram: data.telegram,
        telegramAccounts: data.channelAccounts?.telegram ?? [],
        accountCountLabel,
      });
    case "discord":
      return renderDiscordCard({
        props,
        discord: data.discord,
        accountCountLabel,
      });
    case "googlechat":
      return renderGoogleChatCard({
        props,
        googleChat: data.googlechat,
        accountCountLabel,
      });
    case "slack":
      return renderSlackCard({
        props,
        slack: data.slack,
        accountCountLabel,
      });
    case "signal":
      return renderSignalCard({
        props,
        signal: data.signal,
        accountCountLabel,
      });
    case "imessage":
      return renderIMessageCard({
        props,
        imessage: data.imessage,
        accountCountLabel,
      });
    case "nostr": {
      const nostrAccounts = data.channelAccounts?.nostr ?? [];
      const primaryAccount = nostrAccounts[0];
      const accountId = primaryAccount?.accountId ?? "default";
      const profile =
        (primaryAccount as { profile?: NostrProfile | null } | undefined)?.profile ?? null;
      const showForm =
        props.nostrProfileAccountId === accountId ? props.nostrProfileFormState : null;
      const profileFormCallbacks = showForm
        ? {
          onFieldChange: props.onNostrProfileFieldChange,
          onSave: props.onNostrProfileSave,
          onImport: props.onNostrProfileImport,
          onCancel: props.onNostrProfileCancel,
          onToggleAdvanced: props.onNostrProfileToggleAdvanced,
        }
        : null;
      return renderNostrCard({
        props,
        nostr: data.nostr,
        nostrAccounts,
        accountCountLabel,
        profileFormState: showForm,
        profileFormCallbacks,
        onEditProfile: () => props.onNostrProfileEdit(accountId, profile),
      });
    }
    case "wecom": {
      const wecomStatus = props.snapshot?.channels?.["wecom"] as Record<string, unknown> | null;
      return renderWeComCard({ props, wecom: wecomStatus, accountCountLabel });
    }
    case "dingtalk": {
      const dingtalkStatus = props.snapshot?.channels?.["dingtalk"] as Record<string, unknown> | null;
      return renderDingTalkCard({ props, dingtalk: dingtalkStatus, accountCountLabel });
    }
    case "feishu": {
      const feishuStatus = props.snapshot?.channels?.["feishu"] as Record<string, unknown> | null;
      return renderFeishuCard({ props, feishu: feishuStatus, accountCountLabel });
    }
    default:
      return renderGenericChannelBody(key, props, data.channelAccounts ?? {});
  }
}

// ─── Card Actions ───

function renderCardActions(
  key: ChannelKey,
  props: ChannelsProps,
  isEnabled: boolean,
  requestUpdate?: () => void,
  onReload?: () => void,
) {
  // Chinese channels get the wizard button
  const isChineseChannel = ["feishu", "dingtalk", "wecom"].includes(key);

  return html`
    ${isChineseChannel
      ? html`
        <button
          class="btn btn--sm"
          @click=${() => {
          if (props.onConfigureChannel) {
            props.onConfigureChannel(key);
          }
        }}
        >
          ${t("channels.action.wizard")}
        </button>
      `
      : nothing
    }
    <button
      class="btn btn--sm ${isEnabled ? "" : "primary"}"
      @click=${() => openConfigModal(key, requestUpdate, onReload)}
    >
      ${isEnabled ? t("channels.action.editConfig") : t("channels.action.configure")}
    </button>
  `;
}

// ─── Generic Channel Body ───

function renderGenericChannelBody(
  key: ChannelKey,
  props: ChannelsProps,
  channelAccounts: Record<string, ChannelAccountSnapshot[]>,
) {
  const status = props.snapshot?.channels?.[key] as Record<string, unknown> | undefined;
  const configured = typeof status?.configured === "boolean" ? status.configured : undefined;
  const running = typeof status?.running === "boolean" ? status.running : undefined;
  const connected = typeof status?.connected === "boolean" ? status.connected : undefined;
  const lastError = typeof status?.lastError === "string" ? status.lastError : undefined;
  const accounts = channelAccounts[key] ?? [];
  const accountCountLabel = renderChannelAccountCount(key, channelAccounts);

  return html`
    ${accountCountLabel}

    ${accounts.length > 0
      ? html`
            <div class="account-card-list">
              ${accounts.map((account) => renderGenericAccount(account))}
            </div>
          `
      : html`
            <div class="status-list">
              <div>
                <span class="label">${t("channels.configured")}</span>
                <span>${configured == null ? "n/a" : configured ? t("channels.yes") : t("channels.no")}</span>
              </div>
              <div>
                <span class="label">${t("channels.running")}</span>
                <span>${running == null ? "n/a" : running ? t("channels.yes") : t("channels.no")}</span>
              </div>
              <div>
                <span class="label">${t("channels.connected")}</span>
                <span>${connected == null ? "n/a" : connected ? t("channels.yes") : t("channels.no")}</span>
              </div>
            </div>
          `
    }

    ${lastError
      ? html`<div class="callout danger">
            ${lastError}
          </div>`
      : nothing
    }
  `;
}

// ─── Helper Functions ───

function resolveChannelOrder(snapshot: ChannelsStatusSnapshot | null): ChannelKey[] {
  if (snapshot?.channelMeta?.length) {
    return snapshot.channelMeta.map((entry) => entry.id);
  }
  if (snapshot?.channelOrder?.length) {
    return snapshot.channelOrder;
  }
  return ["wecom", "dingtalk", "feishu", "whatsapp", "telegram", "discord", "googlechat", "slack", "signal", "imessage", "nostr"];
}

function resolveChannelMetaMap(
  snapshot: ChannelsStatusSnapshot | null,
): Record<string, ChannelUiMetaEntry> {
  if (!snapshot?.channelMeta?.length) {
    return {};
  }
  return Object.fromEntries(snapshot.channelMeta.map((entry) => [entry.id, entry]));
}

function resolveChannelLabel(snapshot: ChannelsStatusSnapshot | null, key: string): string {
  const meta = resolveChannelMetaMap(snapshot)[key];
  return meta?.label ?? snapshot?.channelLabels?.[key] ?? t(`channels.name.${key}`) ?? key;
}

const RECENT_ACTIVITY_THRESHOLD_MS = 10 * 60 * 1000; // 10 minutes

function hasRecentActivity(account: ChannelAccountSnapshot): boolean {
  if (!account.lastInboundAt) {
    return false;
  }
  return Date.now() - account.lastInboundAt < RECENT_ACTIVITY_THRESHOLD_MS;
}

function deriveRunningStatus(account: ChannelAccountSnapshot): string {
  if (account.running) {
    return t("channels.yes");
  }
  if (hasRecentActivity(account)) {
    return t("channels.active");
  }
  return t("channels.no");
}

function deriveConnectedStatus(account: ChannelAccountSnapshot): string {
  if (account.connected === true) {
    return t("channels.yes");
  }
  if (account.connected === false) {
    return t("channels.no");
  }
  if (hasRecentActivity(account)) {
    return t("channels.active");
  }
  return "n/a";
}

function renderGenericAccount(account: ChannelAccountSnapshot) {
  const runningStatus = deriveRunningStatus(account);
  const connectedStatus = deriveConnectedStatus(account);

  return html`
    <div class="account-card">
      <div class="account-card-header">
        <div class="account-card-title">${account.name || account.accountId}</div>
        <div class="account-card-id">${account.accountId}</div>
      </div>
      <div class="status-list account-card-status">
        <div>
          <span class="label">${t("channels.running")}</span>
          <span>${runningStatus}</span>
        </div>
        <div>
          <span class="label">${t("channels.configured")}</span>
          <span>${account.configured ? t("channels.yes") : t("channels.no")}</span>
        </div>
        <div>
          <span class="label">${t("channels.connected")}</span>
          <span>${connectedStatus}</span>
        </div>
        <div>
          <span class="label">${t("channels.lastInbound")}</span>
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
}
