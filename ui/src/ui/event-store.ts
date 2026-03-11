// event-store.ts — P4 事件收敛: 全局事件归档
//
// 所有 chat 终态事件（final/error/aborted）都先归档到此 store，
// 再由视图层根据当前 sessionKey 决定是否渲染。
// 这确保跨 session 的 final 事件不会因 sessionKey 不匹配而静默丢失。
//
// 用途:
// - 跨 session 事件恢复（切回 session 时可查看已完成的事件）
// - 消除 app-gateway.ts 中的二级回退 hack
// - 提供事件溯源审计能力

import type { ChatEventPayload } from "./controllers/chat.ts";

/** 归档后的事件条目。 */
export type StoredChatEvent = ChatEventPayload & {
  /** 归档时间戳 (ms) */
  timestamp: number;
};

// 每个 session 最多保留的终态事件数
const MAX_EVENTS_PER_SESSION = 10;
// store 最多跟踪的 session 数
const MAX_TRACKED_SESSIONS = 200;

/**
 * GlobalChatEventStore — 全局 chat 事件归档。
 *
 * 设计要点:
 * - 仅归档终态事件（final/error/aborted），delta 不归档（瞬态流数据无需持久化）
 * - 以 sessionKey 为分区键，每个 session 保留最近 N 条终态事件
 * - 提供 drain 语义：消费后清除，避免重复处理
 * - 单例模式，整个 UI 生命周期共享
 */
class GlobalChatEventStore {
  private terminalBySession = new Map<string, StoredChatEvent[]>();

  /** 归档一个终态事件。delta 事件会被忽略。 */
  record(payload: ChatEventPayload): void {
    if (!payload.sessionKey) return;
    if (payload.state !== "final" && payload.state !== "error" && payload.state !== "aborted") {
      return; // 仅归档终态
    }

    const key = payload.sessionKey;
    let events = this.terminalBySession.get(key);
    if (!events) {
      events = [];
      this.terminalBySession.set(key, events);
    }

    events.push({ ...payload, timestamp: Date.now() });

    // 保留最近 N 条
    if (events.length > MAX_EVENTS_PER_SESSION) {
      events.splice(0, events.length - MAX_EVENTS_PER_SESSION);
    }

    // 超出 session 数限制时淘汰最早的 session
    if (this.terminalBySession.size > MAX_TRACKED_SESSIONS) {
      const iter = this.terminalBySession.keys();
      const oldest = iter.next().value;
      if (oldest !== undefined) {
        this.terminalBySession.delete(oldest);
      }
    }
  }

  /**
   * 消费指定 session 的所有终态事件（drain 语义：读后清除）。
   * 典型用于切换 session 时检查是否有待处理的已完成事件。
   */
  drainTerminalEvents(sessionKey: string): StoredChatEvent[] {
    const events = this.terminalBySession.get(sessionKey);
    if (!events || events.length === 0) return [];
    this.terminalBySession.delete(sessionKey);
    return events;
  }

  /** 查询指定 session 是否有待处理的终态事件（不消费）。 */
  hasTerminalEvents(sessionKey: string): boolean {
    const events = this.terminalBySession.get(sessionKey);
    return events !== undefined && events.length > 0;
  }

  /** 查询指定 session 的终态事件数量。 */
  countTerminalEvents(sessionKey: string): number {
    return this.terminalBySession.get(sessionKey)?.length ?? 0;
  }

  /** 查询所有有待处理终态事件的 sessionKey 列表。 */
  sessionsWithPendingEvents(): string[] {
    const result: string[] = [];
    for (const [key, events] of this.terminalBySession) {
      if (events.length > 0) {
        result.push(key);
      }
    }
    return result;
  }

  /** 清空所有归档事件。 */
  clear(): void {
    this.terminalBySession.clear();
  }
}

/** 全局单例。 */
export const globalChatEventStore = new GlobalChatEventStore();
