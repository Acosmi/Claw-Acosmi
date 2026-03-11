import { describe, expect, it, vi } from "vitest";
import { loadSubAgents, toggleSubAgent, type SubAgentsState } from "./subagents.ts";

function createState(request = vi.fn()): SubAgentsState {
  return {
    client: { request } as unknown as SubAgentsState["client"],
    connected: true,
    subagentsLoading: false,
    subagentsList: [],
    subagentsError: null,
    subagentsBusyKey: null,
  };
}

describe("loadSubAgents", () => {
  it("does not mark argus as enabled when backend only reports available", async () => {
    const request = vi.fn().mockResolvedValue({
      agents: [
        { id: "argus-screen", label: "Argus", status: "available" },
        { id: "oa-coder", label: "Coder", status: "available" },
      ],
    });
    const state = createState(request);

    await loadSubAgents(state);

    expect(state.subagentsList).toEqual([
      expect.objectContaining({ id: "argus-screen", status: "available", enabled: false }),
      expect.objectContaining({ id: "oa-coder", status: "available", enabled: true }),
    ]);
  });
});

describe("toggleSubAgent", () => {
  it("reloads backend state instead of forcing argus to running", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce({
        agents: [
          { id: "argus-screen", label: "Argus", status: "stopped" },
        ],
      });
    const state = createState(request);
    state.subagentsList = [
      { id: "argus-screen", label: "Argus", enabled: false, model: "none", intervalMs: 1000, goal: "", status: "stopped" },
    ];

    await toggleSubAgent(state, "argus-screen", true);

    expect(request).toHaveBeenNthCalledWith(1, "subagent.ctl", {
      agent_id: "argus-screen",
      action: "set_enabled",
      value: true,
    });
    expect(request).toHaveBeenNthCalledWith(2, "subagent.list", {});
    expect(state.subagentsList[0]).toEqual(
      expect.objectContaining({ id: "argus-screen", status: "stopped", enabled: false }),
    );
  });

  it("clears the pending restart flag once argus is actually running", async () => {
    const storage = {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
      clear: vi.fn(),
    };
    vi.stubGlobal("localStorage", storage);
    const request = vi
      .fn()
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce({
        agents: [
          { id: "argus-screen", label: "Argus", status: "running" },
        ],
      });
    const state = createState(request);
    state.subagentsList = [
      { id: "argus-screen", label: "Argus", enabled: false, model: "none", intervalMs: 1000, goal: "", status: "stopped" },
    ];

    await toggleSubAgent(state, "argus-screen", true);

    expect(storage.setItem).toHaveBeenCalledWith("openacosmi.argus.post_auth_new_chat", "1");
    expect(storage.removeItem).toHaveBeenCalledWith("openacosmi.argus.post_auth_new_chat");
    vi.unstubAllGlobals();
  });
});
