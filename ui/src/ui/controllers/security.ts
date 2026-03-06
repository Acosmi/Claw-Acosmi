import type { GatewayBrowserClient } from "../gateway.ts";
import type { SecurityLevelInfo } from "./security-types.ts";

// ---------- State 类型 ----------

export type { SecurityLevelInfo };

export interface SecurityState {
    client: GatewayBrowserClient | null;
    connected: boolean;
    securityLevel: string;
    securityLoading: boolean;
    securityError: string | null;
    securityLevels: SecurityLevelInfo[];
    securityHash: string;
    securityConfirmOpen: boolean;
    securityPendingLevel: string | null;
    securityConfirmText: string;
}

// ---------- 加载安全级别 ----------

export async function loadSecurity(state: SecurityState): Promise<void> {
    if (!state.client || !state.connected) return;
    state.securityLoading = true;
    state.securityError = null;
    try {
        const result = await state.client.request<{
            currentLevel: string;
            levels: SecurityLevelInfo[];
            hash: string;
        }>("security.get", {});
        state.securityLevel = result.currentLevel ?? "deny";
        state.securityLevels = result.levels ?? [];
        state.securityHash = result.hash ?? "";
    } catch (err) {
        state.securityError = err instanceof Error ? err.message : String(err);
    } finally {
        state.securityLoading = false;
    }
}

// ---------- 更新安全级别 ----------

export async function updateSecurityLevel(
    state: SecurityState,
    level: string,
): Promise<void> {
    if (!state.client || !state.connected) return;
    state.securityLoading = true;
    state.securityError = null;
    try {
        // 先获取最新的 exec approvals snapshot（需要 hash 进行 OCC）
        const snapshot = await state.client.request<{
            hash: string;
            file: Record<string, unknown>;
        }>("exec.approvals.get", {});

        const existingFile = snapshot.file ?? {};
        const existingDefaults =
            (existingFile.defaults as Record<string, unknown>) ?? {};
        const escalationFallback =
            typeof existingDefaults.escalationFallback === "string"
                ? existingDefaults.escalationFallback
                : "base";

        // 构建更新后的 file 对象
        const nextFile = {
            ...existingFile,
            version: 1,
            defaults: {
                ...existingDefaults,
                security: level,
                escalationFallback,
            },
        };

        // 写入更新
        await state.client.request("exec.approvals.set", {
            baseHash: snapshot.hash,
            file: nextFile,
        });

        // 刷新安全状态
        await loadSecurity(state);
    } catch (err) {
        state.securityError = err instanceof Error ? err.message : String(err);
    } finally {
        state.securityLoading = false;
        state.securityConfirmOpen = false;
        state.securityPendingLevel = null;
        state.securityConfirmText = "";
    }
}
