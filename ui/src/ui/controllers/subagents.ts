import type { GatewayBrowserClient } from "../gateway.ts";

// ---------- SubAgents Controller ----------

export type SubAgentEntry = {
    id: string;           // "argus-screen" | "oa-coder"
    label: string;
    enabled: boolean;
    model: string;
    intervalMs: number;
    goal: string;
    status: "running" | "stopped" | "error";
    error?: string;
};

export type SubAgentsState = {
    client: GatewayBrowserClient | null;
    connected: boolean;
    subagentsLoading: boolean;
    subagentsList: SubAgentEntry[];
    subagentsError: string | null;
    subagentsBusyKey: string | null;
};

function defaultSubAgents(): SubAgentEntry[] {
    return [
        {
            id: "argus-screen",
            label: "Vision Observer",
            enabled: false,
            model: "none",
            intervalMs: 1000,
            goal: "",
            status: "stopped",
        },
        {
            id: "oa-coder",
            label: "Coder Agent",
            enabled: false,
            model: "none",
            intervalMs: 0,
            goal: "",
            status: "stopped",
        },
    ];
}

export async function loadSubAgents(state: SubAgentsState) {
    if (state.subagentsLoading) return;
    // 当前无 subagent.list 方法，使用本地默认值
    // 未来可对接 subagent.list WS 方法
    if (state.subagentsList.length === 0) {
        state.subagentsList = defaultSubAgents();
    }
    state.subagentsLoading = false;
}

export async function toggleSubAgent(state: SubAgentsState, agentId: string, enabled: boolean) {
    if (!state.client || !state.connected) return;
    state.subagentsBusyKey = agentId;
    state.subagentsError = null;
    try {
        await state.client.request("subagent.ctl", {
            agent_id: agentId,
            action: "set_enabled",
            value: enabled,
        });
        // 更新本地状态
        const entry = state.subagentsList.find((e) => e.id === agentId);
        if (entry) {
            entry.enabled = enabled;
            entry.status = enabled ? "running" : "stopped";
        }
    } catch (err) {
        state.subagentsError = err instanceof Error ? err.message : String(err);
    } finally {
        state.subagentsBusyKey = null;
    }
}

export async function setSubAgentInterval(
    state: SubAgentsState,
    agentId: string,
    intervalMs: number,
) {
    if (!state.client || !state.connected) return;
    state.subagentsBusyKey = agentId;
    try {
        await state.client.request("subagent.ctl", {
            agent_id: agentId,
            action: "set_interval_ms",
            value: intervalMs,
        });
        const entry = state.subagentsList.find((e) => e.id === agentId);
        if (entry) entry.intervalMs = intervalMs;
    } catch (err) {
        state.subagentsError = err instanceof Error ? err.message : String(err);
    } finally {
        state.subagentsBusyKey = null;
    }
}

export async function setSubAgentGoal(state: SubAgentsState, agentId: string, goal: string) {
    if (!state.client || !state.connected) return;
    state.subagentsBusyKey = agentId;
    try {
        await state.client.request("subagent.ctl", {
            agent_id: agentId,
            action: "set_goal",
            value: goal,
        });
        const entry = state.subagentsList.find((e) => e.id === agentId);
        if (entry) entry.goal = goal;
    } catch (err) {
        state.subagentsError = err instanceof Error ? err.message : String(err);
    } finally {
        state.subagentsBusyKey = null;
    }
}

export async function setSubAgentModel(state: SubAgentsState, agentId: string, model: string) {
    if (!state.client || !state.connected) return;
    state.subagentsBusyKey = agentId;
    try {
        await state.client.request("subagent.ctl", {
            agent_id: agentId,
            action: "set_vla_model",
            value: model,
        });
        const entry = state.subagentsList.find((e) => e.id === agentId);
        if (entry) entry.model = model;
    } catch (err) {
        state.subagentsError = err instanceof Error ? err.message : String(err);
    } finally {
        state.subagentsBusyKey = null;
    }
}
