import { render } from "lit";
import { describe, expect, it, vi } from "vitest";
import { initLocale } from "../i18n.ts";
import type { ChannelsProps } from "./channels.types.ts";
import { renderChannels } from "./channels.ts";

initLocale("en");

function createProps(): ChannelsProps {
  return {
    connected: false,
    loading: false,
    snapshot: {
      ts: 0,
      channelOrder: ["email", "feishu"],
      channelLabels: { email: "Email", feishu: "Feishu" },
      channelMeta: [
        { id: "email", label: "Email", detailLabel: "Email" },
        { id: "feishu", label: "Feishu", detailLabel: "Feishu" },
      ],
      channels: {
        email: { configured: false, running: false, connected: false },
        feishu: { configured: false, running: false, connected: false },
      },
      channelAccounts: {
        email: [],
        feishu: [],
      },
      channelDefaultAccountId: {},
    },
    lastError: null,
    lastSuccessAt: null,
    whatsappMessage: null,
    whatsappQrDataUrl: null,
    whatsappConnected: null,
    whatsappBusy: false,
    configSchema: null,
    configSchemaLoading: false,
    configForm: {},
    configUiHints: {},
    configSaving: false,
    configFormDirty: false,
    nostrProfileFormState: null,
    nostrProfileAccountId: null,
    onRefresh: vi.fn(),
    onWhatsAppStart: vi.fn(),
    onWhatsAppWait: vi.fn(),
    onWhatsAppLogout: vi.fn(),
    onConfigPatch: vi.fn(),
    onConfigSave: vi.fn(async () => false),
    onConfigReload: vi.fn(),
    onNostrProfileEdit: vi.fn(),
    onNostrProfileCancel: vi.fn(),
    onNostrProfileFieldChange: vi.fn(),
    onNostrProfileSave: vi.fn(),
    onNostrProfileImport: vi.fn(),
    onNostrProfileToggleAdvanced: vi.fn(),
    requestUpdate: vi.fn(),
  };
}

describe("channels view", () => {
  it("hides email from the remote chat channel grid", () => {
    const container = document.createElement("div");
    render(renderChannels(createProps()), container);

    const names = Array.from(container.querySelectorAll(".channel-card__name"))
      .map((entry) => entry.textContent?.trim());

    expect(names).toContain("Feishu");
    expect(names).not.toContain("Email");
  });
});
