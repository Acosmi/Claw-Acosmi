import { render } from "lit";
import { describe, expect, it, vi } from "vitest";
import type { AppViewState } from "../app-view-state.ts";
import { initLocale } from "../i18n.ts";
import { renderAutomation } from "./automation.ts";

initLocale("en");

function createState(): AppViewState {
  return {
    automationPanel: "hub",
    connected: false,
    client: null,
    channelsSnapshot: {
      ts: 0,
      channelOrder: ["email"],
      channelLabels: { email: "Email" },
      channels: { email: { configured: true, running: true, connected: true } },
      channelAccounts: {
        email: [{ accountId: "ops", configured: true, running: true, connected: true }],
      },
      channelDefaultAccountId: { email: "ops" },
    },
    channelsLoading: false,
    channelsError: null,
    channelsLastSuccess: null,
    browserToolConfig: {
      enabled: true,
      cdpUrl: "",
      evaluateEnabled: true,
      headless: false,
      configured: true,
    },
    browserToolLoading: false,
    browserToolError: null,
    mcpServersList: [],
    mcpServersLoading: false,
    mcpToolsList: [],
    mcpToolsLoading: false,
    subagentsList: [],
    subagentsLoading: false,
    mediaConfig: {
      enabled_sources: [],
      publishers: [],
    },
    configLoading: false,
    configSaving: false,
    configSchemaLoading: false,
    configFormDirty: false,
    configForm: { channels: { email: { defaultAccount: "ops" } } },
    configSnapshot: null,
    configSchema: null,
    configUiHints: {},
    lastError: null,
    emailTestLoading: false,
    emailTestResult: null,
    whatsappLoginMessage: null,
    whatsappLoginQrDataUrl: null,
    whatsappLoginConnected: null,
    whatsappBusy: false,
    nostrProfileFormState: null,
    nostrProfileAccountId: null,
    handleEmailTest: vi.fn(async () => undefined),
    handleChannelConfigReload: vi.fn(async () => undefined),
    handleChannelConfigSave: vi.fn(async () => true),
    handleStartWizardV2: vi.fn(async () => undefined),
    setTab: vi.fn(),
    pluginsPanel: "tools",
    mcpSubTab: "servers",
    agentsSelectedId: null,
    mediaManageSubTab: "overview",
  } as unknown as AppViewState;
}

describe("automation email detail", () => {
  it("opens a dedicated email automation workspace from the hub card", () => {
    const state = createState();
    const container = document.createElement("div");
    render(renderAutomation(state), container);

    const emailCard = Array.from(container.querySelectorAll("button")).find((entry) =>
      entry.textContent?.includes("Email Automation")
    );
    expect(emailCard).not.toBeNull();

    emailCard?.click();
    expect(state.automationPanel).toBe("email");

    render(renderAutomation(state), container);

    expect(container.textContent).toContain("Mailbox Runtime");
    expect(container.textContent).toContain("Storage path:");
    expect(container.textContent).not.toContain("Automation Hub");
  });
});
