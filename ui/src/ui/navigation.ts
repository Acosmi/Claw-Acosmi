import type { IconName } from "./icons.js";
import { t } from "./i18n.ts";

export function getTabGroups() {
  return [
    {
      label: t("nav.group.agent"),
      hideLabel: true,
      tabs: ["chat", "tasks", "agents", "nodes", "automation"] as const,
    },
    {
      label: t("nav.group.control"),
      tabs: ["overview", "channels", "plugins", "mcp", "memory", "cron", "security"] as const,
    },
    { label: t("nav.group.settings"), tabs: ["config", "debug", "logs"] as const },
  ];
}

export type Tab =
  | "agents"
  | "automation"
  | "overview"
  | "channels"
  | "plugins"
  | "instances"
  | "usage"
  | "cron"
  | "skills"
  | "nodes"
  | "chat"
  | "memory"
  | "security"
  | "config"
  | "debug"
  | "logs"
  | "subagents"
  | "media"
  | "tasks"
  | "mcp";

const TAB_PATHS: Record<Tab, string> = {
  agents: "/agents",
  automation: "/automation",
  overview: "/overview",
  channels: "/channels",
  plugins: "/plugins",
  instances: "/instances",
  usage: "/usage",
  cron: "/cron",
  skills: "/skills",
  nodes: "/nodes",
  memory: "/memory",
  chat: "/chat",
  security: "/security",
  config: "/config",
  debug: "/debug",
  logs: "/logs",
  subagents: "/subagents", // deprecated: 重定向到 /agents
  media: "/media",
  tasks: "/tasks",
  mcp: "/mcp",
};

const PATH_TO_TAB = new Map(Object.entries(TAB_PATHS).map(([tab, path]) => [path, tab as Tab]));
const AUTOMATION_PANEL_PATHS = {
  hub: TAB_PATHS.automation,
  email: `${TAB_PATHS.automation}/email`,
} as const;

export function normalizeBasePath(basePath: string): string {
  if (!basePath) {
    return "";
  }
  let base = basePath.trim();
  if (!base.startsWith("/")) {
    base = `/${base}`;
  }
  if (base === "/") {
    return "";
  }
  if (base.endsWith("/")) {
    base = base.slice(0, -1);
  }
  return base;
}

export function normalizePath(path: string): string {
  if (!path) {
    return "/";
  }
  let normalized = path.trim();
  if (!normalized.startsWith("/")) {
    normalized = `/${normalized}`;
  }
  if (normalized.length > 1 && normalized.endsWith("/")) {
    normalized = normalized.slice(0, -1);
  }
  return normalized;
}

export function pathForTab(tab: Tab, basePath = ""): string {
  const base = normalizeBasePath(basePath);
  const path = TAB_PATHS[tab];
  return base ? `${base}${path}` : path;
}

export function pathForAutomationPanel(panel: "hub" | "email", basePath = ""): string {
  const base = normalizeBasePath(basePath);
  const path = AUTOMATION_PANEL_PATHS[panel] ?? AUTOMATION_PANEL_PATHS.hub;
  return base ? `${base}${path}` : path;
}

function isAutomationPath(path: string): boolean {
  return path === TAB_PATHS.automation || path.startsWith(`${TAB_PATHS.automation}/`);
}

function isRecognizedTabPath(path: string): boolean {
  return PATH_TO_TAB.has(path) || path === "/sessions" || isAutomationPath(path);
}

export function tabFromPath(pathname: string, basePath = ""): Tab | null {
  const base = normalizeBasePath(basePath);
  let path = pathname || "/";
  if (base) {
    if (path === base) {
      path = "/";
    } else if (path.startsWith(`${base}/`)) {
      path = path.slice(base.length);
    }
  }
  let normalized = normalizePath(path).toLowerCase();
  if (normalized.endsWith("/index.html")) {
    normalized = "/";
  }
  if (normalized === "/") {
    return "chat";
  }
  // Legacy redirect: /sessions → memory
  if (normalized === "/sessions") {
    return "memory";
  }
  // Legacy redirect: /subagents → agents（子智能体已统一到代理标签页）
  if (normalized === "/subagents") {
    return "agents";
  }
  if (isAutomationPath(normalized)) {
    return "automation";
  }
  return PATH_TO_TAB.get(normalized) ?? null;
}

export function automationPanelFromPath(pathname: string, basePath = ""): "hub" | "email" {
  const base = normalizeBasePath(basePath);
  let path = pathname || "/";
  if (base) {
    if (path === base) {
      path = "/";
    } else if (path.startsWith(`${base}/`)) {
      path = path.slice(base.length);
    }
  }
  const normalized = normalizePath(path).toLowerCase();
  if (normalized === AUTOMATION_PANEL_PATHS.email) {
    return "email";
  }
  return "hub";
}

export function inferBasePathFromPathname(pathname: string): string {
  let normalized = normalizePath(pathname);
  if (normalized.endsWith("/index.html")) {
    normalized = normalizePath(normalized.slice(0, -"/index.html".length));
  }
  if (normalized === "/") {
    return "";
  }
  const segments = normalized.split("/").filter(Boolean);
  if (segments.length === 0) {
    return "";
  }
  for (let i = 0; i < segments.length; i++) {
    const candidate = `/${segments.slice(i).join("/")}`.toLowerCase();
    // Legacy path: /sessions is now served by /memory
    if (isRecognizedTabPath(candidate)) {
      const prefix = segments.slice(0, i);
      return prefix.length ? `/${prefix.join("/")}` : "";
    }
  }
  return `/${segments.join("/")}`;
}

export function iconForTab(tab: Tab): IconName {
  switch (tab) {
    case "agents":
      return "agentSwarm";
    case "automation":
      return "automationHub";
    case "chat":
      return "chatSpark";
    case "overview":
      return "wizardDashboard";
    case "channels":
      return "channelBridge";
    case "plugins":
      return "pluginCircuit";
    case "instances":
      return "nodeMesh";
    case "usage":
      return "wizardDashboard";
    case "cron":
      return "cronOrbit";
    case "skills":
      return "pluginCircuit";
    case "memory":
      return "memoryVault";
    case "nodes":
      return "nodeMesh";
    case "security":
      return "securityPulse";
    case "config":
      return "configSliders";
    case "debug":
      return "debugRadar";
    case "logs":
      return "logStack";
    case "subagents":
      return "agentSwarm";
    case "media":
      return "navMedia";
    case "mcp":
      return "mcpBridge";
    case "tasks":
      return "taskOrbit";
    default:
      return "folder";
  }
}

export function titleForTab(tab: Tab) {
  return t(`nav.tab.${tab}`);
}

export function subtitleForTab(tab: Tab) {
  return t(`nav.sub.${tab}`);
}
