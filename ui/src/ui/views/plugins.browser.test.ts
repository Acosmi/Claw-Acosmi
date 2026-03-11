import { render } from "lit";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { initLocale } from "../i18n.ts";
import { renderPlugins, type PluginsProps } from "./plugins.ts";

initLocale("en");

function createProps(overrides: Partial<PluginsProps> = {}): PluginsProps {
  return {
    panel: "tools",
    loading: false,
    plugins: [],
    error: null,
    editValues: {},
    saving: null,
    toolsLoading: false,
    tools: [],
    toolsError: null,
    browserConfig: {
      enabled: true,
      cdpUrl: "",
      evaluateEnabled: true,
      headless: false,
      configured: false,
    },
    browserLoading: false,
    browserSaving: false,
    browserError: null,
    browserEdits: {},
    gatewayUrl: "ws://127.0.0.1:19001/ws",
    packagesLoading: false,
    packagesItems: [],
    packagesTotal: 0,
    packagesError: null,
    packagesKindFilter: "all",
    packagesKeyword: "",
    packagesBusyId: null,
    onEditChange: vi.fn(),
    onSave: vi.fn(),
    onGoToChannels: vi.fn(),
    onPanelChange: vi.fn(),
    onBrowserEditChange: vi.fn(),
    onBrowserSave: vi.fn(),
    onPackagesKindChange: vi.fn(),
    onPackagesKeywordChange: vi.fn(),
    onPackagesSearch: vi.fn(),
    onPackagesInstall: vi.fn(),
    onPackagesRemove: vi.fn(),
    onPackagesLoadMore: vi.fn(),
    ...overrides,
  };
}

describe("plugins browser card", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn(() =>
      Promise.resolve({
        json: () => Promise.resolve({ port: 0, connected: false }),
      } as Response),
    ));
  });

  it("shows configured when cdpUrl exists even if the backend flag is stale", () => {
    const container = document.createElement("div");
    render(
      renderPlugins(createProps({
        browserConfig: {
          enabled: true,
          cdpUrl: "ws://127.0.0.1:9222",
          evaluateEnabled: true,
          headless: false,
          configured: false,
        },
      })),
      container,
    );

    const chips = Array.from(container.querySelectorAll(".chip"));
    expect(chips.some((chip) => chip.textContent?.trim() === "Configured")).toBe(true);
  });

  it("shows configured when unsaved edits fill in cdpUrl", () => {
    const container = document.createElement("div");
    render(
      renderPlugins(createProps({
        browserEdits: {
          cdpUrl: "ws://127.0.0.1:9222",
        },
      })),
      container,
    );

    const chips = Array.from(container.querySelectorAll(".chip"));
    expect(chips.some((chip) => chip.textContent?.trim() === "Configured")).toBe(true);
  });

  it("opens the bundled extension guide through the external opener", () => {
    const onOpenExternal = vi.fn();
    const container = document.createElement("div");
    render(
      renderPlugins(createProps({
        onOpenExternal,
      })),
      container,
    );

    const button = Array.from(container.querySelectorAll("button"))
      .find((entry) => entry.textContent?.trim() === "Extension Install Guide");
    button?.click();

    expect(onOpenExternal).toHaveBeenCalledWith("http://127.0.0.1:19001/browser-extension/");
  });
});
