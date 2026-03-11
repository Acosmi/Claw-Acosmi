import type { GatewayBrowserClient } from "../gateway.ts";
import { clearArgusPostAuthNewChat, markArgusPostAuthNewChat } from "../storage.ts";

// ---------- SubAgents Controller ----------

export type SubAgentEntry = {
    id: string;           // "argus-screen" | "oa-coder"
    label: string;
    enabled: boolean;
    model: string;
    intervalMs: number;
    goal: string;
    status: "running" | "stopped" | "error" | "degraded" | "starting" | "available";
    error?: string;
    provider?: string;     // open-coder: 当前 provider
    configured?: boolean;  // open-coder: 是否已显式配置
};

export type SubAgentsState = {
    client: GatewayBrowserClient | null;
    connected: boolean;
    subagentsLoading: boolean;
    subagentsList: SubAgentEntry[];
    subagentsError: string | null;
    subagentsBusyKey: string | null;
};

function isSubAgentEnabled(agentId: string, status: SubAgentEntry["status"]): boolean {
    if (agentId === "argus-screen") {
        return status === "running" || status === "degraded" || status === "starting";
    }
    return (
        status === "running" ||
        status === "degraded" ||
        status === "starting" ||
        status === "available"
    );
}

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
    if (!state.client || !state.connected) return;
    if (state.subagentsLoading) return;
    state.subagentsLoading = true;
    state.subagentsError = null;
    try {
        type ListAgent = {
            id: string; label: string; status: string; error?: string;
            provider?: string; model?: string; configured?: boolean;
        };
        type ListResult = { agents: ListAgent[] };
        const res = await state.client.request<ListResult>("subagent.list", {});
        const prev = state.subagentsList;
        state.subagentsList = (res.agents ?? []).map((a) => {
            const old = prev.find((e) => e.id === a.id);
            return {
                id: a.id,
                label: a.label,
                status: a.status as SubAgentEntry["status"],
                error: a.error,
                enabled: isSubAgentEnabled(a.id, a.status as SubAgentEntry["status"]),
                model: a.model || old?.model || "none",
                intervalMs: old?.intervalMs ?? (a.id === "argus-screen" ? 1000 : 0),
                goal: old?.goal ?? "",
                provider: a.provider || "",
                configured: a.configured ?? false,
            };
        });
        const argus = state.subagentsList.find((entry) => entry.id === "argus-screen");
        if (argus && argus.enabled) {
            clearArgusPostAuthNewChat();
        }
    } catch (err) {
        state.subagentsError = err instanceof Error ? err.message : String(err);
        if (state.subagentsList.length === 0) {
            state.subagentsList = defaultSubAgents();
        }
    } finally {
        state.subagentsLoading = false;
    }
}

export async function toggleSubAgent(state: SubAgentsState, agentId: string, enabled: boolean) {
    if (!state.client || !state.connected) return;
    state.subagentsBusyKey = agentId;
    state.subagentsError = null;
    if (agentId === "argus-screen") {
        if (enabled) {
            markArgusPostAuthNewChat();
        } else {
            clearArgusPostAuthNewChat();
        }
    }
    try {
        await state.client.request("subagent.ctl", {
            agent_id: agentId,
            action: "set_enabled",
            value: enabled,
        });
        await loadSubAgents(state);
        if (agentId === "argus-screen" && enabled) {
            const argus = state.subagentsList.find((entry) => entry.id === "argus-screen");
            if (argus?.enabled) {
                clearArgusPostAuthNewChat();
            }
        }
    } catch (err: any) {
        // 优先展示结构化恢复指引（来自 ArgusStartError.recovery）
        const recovery = err?.details?.recovery;
        const message = err instanceof Error ? err.message : String(err);
        state.subagentsError = recovery ? `${message}\n${recovery}` : message;
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
