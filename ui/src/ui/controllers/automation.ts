import type { AppViewState } from "../app-view-state.ts";
import { loadChannels } from "./channels.ts";
import { loadConfig, loadConfigSchema } from "./config.ts";
import { loadMcpDashboard } from "./mcp-servers.ts";
import { loadMediaConfig } from "./media-dashboard.ts";
import { loadBrowserToolConfig } from "./plugins.ts";
import { loadSubAgents } from "./subagents.ts";

export async function loadAutomationHub(state: AppViewState): Promise<void> {
  if (!state.client || !state.connected) {
    return;
  }
  await Promise.all([
    loadChannels(state, false),
    loadBrowserToolConfig(state),
    loadMcpDashboard(state),
    loadSubAgents(state as unknown as Parameters<typeof loadSubAgents>[0]),
    loadMediaConfig(state),
  ]);
}

export async function loadEmailAutomationDetail(state: AppViewState): Promise<void> {
  if (!state.client || !state.connected) {
    return;
  }
  await Promise.all([
    loadChannels(state, true),
    loadConfigSchema(state),
    loadConfig(state),
  ]);
}
