import { beforeEach, describe, expect, it } from "vitest";
import { globalChatEventStore, type StoredChatEvent } from "./event-store.ts";
import type { ChatEventPayload } from "./controllers/chat.ts";

beforeEach(() => {
  globalChatEventStore.clear();
});

function makePayload(overrides: Partial<ChatEventPayload> = {}): ChatEventPayload {
  return {
    runId: "run-1",
    sessionKey: "session-a",
    state: "final",
    ...overrides,
  };
}

describe("GlobalChatEventStore", () => {
  it("records terminal events (final/error/aborted)", () => {
    globalChatEventStore.record(makePayload({ state: "final" }));
    globalChatEventStore.record(makePayload({ state: "error", runId: "run-2" }));
    globalChatEventStore.record(makePayload({ state: "aborted", runId: "run-3" }));

    expect(globalChatEventStore.countTerminalEvents("session-a")).toBe(3);
  });

  it("ignores delta events", () => {
    globalChatEventStore.record(makePayload({ state: "delta" }));

    expect(globalChatEventStore.hasTerminalEvents("session-a")).toBe(false);
    expect(globalChatEventStore.countTerminalEvents("session-a")).toBe(0);
  });

  it("ignores payloads without sessionKey", () => {
    globalChatEventStore.record({ ...makePayload(), sessionKey: "" });

    expect(globalChatEventStore.sessionsWithPendingEvents()).toHaveLength(0);
  });

  it("drainTerminalEvents returns and clears events", () => {
    globalChatEventStore.record(makePayload({ runId: "run-1" }));
    globalChatEventStore.record(makePayload({ runId: "run-2" }));

    const events = globalChatEventStore.drainTerminalEvents("session-a");
    expect(events).toHaveLength(2);
    expect(events[0].runId).toBe("run-1");
    expect(events[1].runId).toBe("run-2");

    // After drain, no more events
    expect(globalChatEventStore.hasTerminalEvents("session-a")).toBe(false);
    expect(globalChatEventStore.drainTerminalEvents("session-a")).toHaveLength(0);
  });

  it("drainTerminalEvents returns empty for unknown session", () => {
    expect(globalChatEventStore.drainTerminalEvents("unknown")).toHaveLength(0);
  });

  it("separates events by sessionKey", () => {
    globalChatEventStore.record(makePayload({ sessionKey: "session-a" }));
    globalChatEventStore.record(makePayload({ sessionKey: "session-b" }));
    globalChatEventStore.record(makePayload({ sessionKey: "session-a", runId: "run-2" }));

    expect(globalChatEventStore.countTerminalEvents("session-a")).toBe(2);
    expect(globalChatEventStore.countTerminalEvents("session-b")).toBe(1);
  });

  it("sessionsWithPendingEvents lists all active sessions", () => {
    globalChatEventStore.record(makePayload({ sessionKey: "session-a" }));
    globalChatEventStore.record(makePayload({ sessionKey: "session-b" }));

    const sessions = globalChatEventStore.sessionsWithPendingEvents();
    expect(sessions).toContain("session-a");
    expect(sessions).toContain("session-b");
    expect(sessions).toHaveLength(2);
  });

  it("caps events per session at limit", () => {
    for (let i = 0; i < 15; i++) {
      globalChatEventStore.record(makePayload({ runId: `run-${i}` }));
    }

    // MAX_EVENTS_PER_SESSION = 10
    expect(globalChatEventStore.countTerminalEvents("session-a")).toBe(10);

    // Should keep the latest ones
    const events = globalChatEventStore.drainTerminalEvents("session-a");
    expect(events[0].runId).toBe("run-5"); // oldest kept
    expect(events[9].runId).toBe("run-14"); // newest
  });

  it("clear removes all events", () => {
    globalChatEventStore.record(makePayload({ sessionKey: "session-a" }));
    globalChatEventStore.record(makePayload({ sessionKey: "session-b" }));
    globalChatEventStore.clear();

    expect(globalChatEventStore.sessionsWithPendingEvents()).toHaveLength(0);
  });

  it("stored events include timestamp", () => {
    const before = Date.now();
    globalChatEventStore.record(makePayload());
    const after = Date.now();

    const events = globalChatEventStore.drainTerminalEvents("session-a");
    expect(events[0].timestamp).toBeGreaterThanOrEqual(before);
    expect(events[0].timestamp).toBeLessThanOrEqual(after);
  });
});
