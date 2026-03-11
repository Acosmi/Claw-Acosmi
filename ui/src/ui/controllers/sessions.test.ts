import { describe, expect, it, vi } from "vitest";
import { loadSessions, type SessionsState } from "./sessions.ts";

function createState(requestImpl: (...args: unknown[]) => Promise<unknown>): SessionsState {
  return {
    client: {
      request: requestImpl,
    } as NonNullable<SessionsState["client"]>,
    connected: true,
    sessionsLoading: false,
    sessionsResult: null,
    sessionsError: null,
    sessionsFilterActive: "",
    sessionsFilterLimit: "",
    sessionsIncludeGlobal: false,
    sessionsIncludeUnknown: false,
  };
}

describe("loadSessions", () => {
  it("queues one more refresh when a remote event arrives during an active load", async () => {
    let resolveFirst: ((value: { sessions: never[]; count: number }) => void) | null = null;
    const request = vi.fn()
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveFirst = resolve as typeof resolveFirst;
      }))
      .mockResolvedValueOnce({ sessions: [], count: 0 });
    const state = createState(request);

    const firstLoad = loadSessions(state, { activeMinutes: 60 });
    await Promise.resolve();
    expect(state.sessionsLoading).toBe(true);

    await loadSessions(state, { activeMinutes: 5 });
    expect(request).toHaveBeenCalledTimes(1);

    resolveFirst?.({ sessions: [], count: 0 });
    await firstLoad;

    await vi.waitFor(() => {
      expect(request).toHaveBeenCalledTimes(2);
    });
    expect(request).toHaveBeenNthCalledWith(2, "sessions.list", {
      includeGlobal: false,
      includeUnknown: false,
      activeMinutes: 5,
    });
  });
});
