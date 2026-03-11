import { describe, expect, it } from "vitest";
import { initLocale } from "./i18n.ts";
import {
  automationPanelFromPath,
  getTabGroups,
  iconForTab,
  inferBasePathFromPathname,
  normalizeBasePath,
  normalizePath,
  pathForAutomationPanel,
  pathForTab,
  subtitleForTab,
  tabFromPath,
  titleForTab,
  type Tab,
} from "./navigation.ts";

initLocale("en");

/** All valid tab identifiers derived from getTabGroups() */
const ALL_TABS: Tab[] = getTabGroups().flatMap((group) => group.tabs) as Tab[];

describe("iconForTab", () => {
  it("returns a non-empty string for every tab", () => {
    for (const tab of ALL_TABS) {
      const icon = iconForTab(tab);
      expect(icon).toBeTruthy();
      expect(typeof icon).toBe("string");
      expect(icon.length).toBeGreaterThan(0);
    }
  });

  it("returns stable icons for known tabs", () => {
    expect(iconForTab("agents")).toBe("agentSwarm");
    expect(iconForTab("automation")).toBe("automationHub");
    expect(iconForTab("chat")).toBe("chatSpark");
    expect(iconForTab("overview")).toBe("wizardDashboard");
    expect(iconForTab("channels")).toBe("channelBridge");
    expect(iconForTab("instances")).toBe("nodeMesh");
    expect(iconForTab("cron")).toBe("cronOrbit");
    expect(iconForTab("skills")).toBe("pluginCircuit");
    expect(iconForTab("nodes")).toBe("nodeMesh");
    expect(iconForTab("config")).toBe("configSliders");
    expect(iconForTab("debug")).toBe("debugRadar");
    expect(iconForTab("logs")).toBe("logStack");
  });

  it("returns a fallback icon for unknown tab", () => {
    // TypeScript won't allow this normally, but runtime could receive unexpected values
    const unknownTab = "unknown" as Tab;
    expect(iconForTab(unknownTab)).toBe("folder");
  });
});

describe("titleForTab", () => {
  it("returns a non-empty string for every tab", () => {
    for (const tab of ALL_TABS) {
      const title = titleForTab(tab);
      expect(title).toBeTruthy();
      expect(typeof title).toBe("string");
    }
  });

  it("returns expected titles", () => {
    expect(titleForTab("chat")).toBe("Start Chat");
    expect(titleForTab("agents")).toBe("Workspace (Agent Swarm)");
    expect(titleForTab("automation")).toBe("Automation");
    expect(titleForTab("overview")).toBe("Dashboard (Wizard)");
    expect(titleForTab("cron")).toBe("Cron Jobs");
  });
});

describe("subtitleForTab", () => {
  it("returns a string for every tab", () => {
    for (const tab of ALL_TABS) {
      const subtitle = subtitleForTab(tab);
      expect(typeof subtitle).toBe("string");
    }
  });

  it("returns descriptive subtitles", () => {
    expect(subtitleForTab("chat")).toContain("chat session");
    expect(subtitleForTab("config")).toContain("openacosmi.json");
  });
});

describe("normalizeBasePath", () => {
  it("returns empty string for falsy input", () => {
    expect(normalizeBasePath("")).toBe("");
  });

  it("adds leading slash if missing", () => {
    expect(normalizeBasePath("ui")).toBe("/ui");
  });

  it("removes trailing slash", () => {
    expect(normalizeBasePath("/ui/")).toBe("/ui");
  });

  it("returns empty string for root path", () => {
    expect(normalizeBasePath("/")).toBe("");
  });

  it("handles nested paths", () => {
    expect(normalizeBasePath("/apps/openacosmi")).toBe("/apps/openacosmi");
  });
});

describe("normalizePath", () => {
  it("returns / for falsy input", () => {
    expect(normalizePath("")).toBe("/");
  });

  it("adds leading slash if missing", () => {
    expect(normalizePath("chat")).toBe("/chat");
  });

  it("removes trailing slash except for root", () => {
    expect(normalizePath("/chat/")).toBe("/chat");
    expect(normalizePath("/")).toBe("/");
  });
});

describe("pathForTab", () => {
  it("returns correct path without base", () => {
    expect(pathForTab("chat")).toBe("/chat");
    expect(pathForTab("automation")).toBe("/automation");
    expect(pathForTab("overview")).toBe("/overview");
  });

  it("prepends base path", () => {
    expect(pathForTab("chat", "/ui")).toBe("/ui/chat");
    expect(pathForTab("memory", "/apps/openacosmi")).toBe("/apps/openacosmi/memory");
  });
});

describe("pathForAutomationPanel", () => {
  it("returns direct routes for automation detail pages", () => {
    expect(pathForAutomationPanel("hub")).toBe("/automation");
    expect(pathForAutomationPanel("email")).toBe("/automation/email");
    expect(pathForAutomationPanel("email", "/ui")).toBe("/ui/automation/email");
  });
});

describe("tabFromPath", () => {
  it("returns tab for valid path", () => {
    expect(tabFromPath("/chat")).toBe("chat");
    expect(tabFromPath("/automation")).toBe("automation");
    expect(tabFromPath("/automation/email")).toBe("automation");
    expect(tabFromPath("/overview")).toBe("overview");
    expect(tabFromPath("/sessions")).toBe("memory");
  });

  it("returns chat for root path", () => {
    expect(tabFromPath("/")).toBe("chat");
  });

  it("handles base paths", () => {
    expect(tabFromPath("/ui/chat", "/ui")).toBe("chat");
    expect(tabFromPath("/ui/automation/email", "/ui")).toBe("automation");
    expect(tabFromPath("/apps/openacosmi/sessions", "/apps/openacosmi")).toBe("memory");
  });

  it("returns null for unknown path", () => {
    expect(tabFromPath("/unknown")).toBeNull();
  });

  it("is case-insensitive", () => {
    expect(tabFromPath("/CHAT")).toBe("chat");
    expect(tabFromPath("/Overview")).toBe("overview");
  });
});

describe("inferBasePathFromPathname", () => {
  it("returns empty string for root", () => {
    expect(inferBasePathFromPathname("/")).toBe("");
  });

  it("returns empty string for direct tab path", () => {
    expect(inferBasePathFromPathname("/chat")).toBe("");
    expect(inferBasePathFromPathname("/overview")).toBe("");
  });

  it("infers base path from nested paths", () => {
    expect(inferBasePathFromPathname("/ui/chat")).toBe("/ui");
    expect(inferBasePathFromPathname("/apps/openacosmi/memory")).toBe("/apps/openacosmi");
  });

  it("infers base path for legacy /sessions path", () => {
    expect(inferBasePathFromPathname("/sessions")).toBe("");
    expect(inferBasePathFromPathname("/apps/openacosmi/sessions")).toBe("/apps/openacosmi");
  });

  it("handles index.html suffix", () => {
    expect(inferBasePathFromPathname("/index.html")).toBe("");
    expect(inferBasePathFromPathname("/ui/index.html")).toBe("/ui");
  });

  it("infers base path for automation subroutes", () => {
    expect(inferBasePathFromPathname("/ui/automation/email")).toBe("/ui");
  });
});

describe("automationPanelFromPath", () => {
  it("extracts automation detail panels from the pathname", () => {
    expect(automationPanelFromPath("/automation")).toBe("hub");
    expect(automationPanelFromPath("/automation/email")).toBe("email");
    expect(automationPanelFromPath("/ui/automation/email", "/ui")).toBe("email");
  });
});

describe("getTabGroups", () => {
  it("contains 3 groups", () => {
    const groups = getTabGroups();
    expect(groups).toHaveLength(3);
    for (const g of groups) {
      expect(g.label.length).toBeGreaterThan(0);
    }
  });

  it("all tabs are unique", () => {
    const allTabs = getTabGroups().flatMap((g) => g.tabs);
    const uniqueTabs = new Set(allTabs);
    expect(uniqueTabs.size).toBe(allTabs.length);
  });
});
