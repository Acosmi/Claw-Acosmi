import { beforeEach, describe, expect, it } from "vitest";
import {
  createChatReadonlyRunState,
  isReadonlyRunActive,
  isReadonlyRunVisible,
  loadPersistedChatReadonlyRunHistory,
  loadPersistedChatReadonlyRun,
  persistChatReadonlyRun,
  setChatReadonlyRunTerminal,
  startChatReadonlyRun,
  updateChatReadonlyRunFromChat,
  updateChatReadonlyRunFromLifecycle,
  updateChatReadonlyRunFromProgress,
  updateChatReadonlyRunFromTool,
} from "./readonly-run-state.ts";

function createHost() {
  return {
    sessionKey: "main",
    chatReadonlyRun: createChatReadonlyRunState("main"),
    chatReadonlyRunHistory: [] as ReturnType<typeof loadPersistedChatReadonlyRunHistory>,
  };
}

beforeEach(() => {
  window.localStorage.clear();
});

describe("readonly run state", () => {
  it("persists an active workflow snapshot for reload recovery", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-0", 100, "main");

    updateChatReadonlyRunFromTool(host, {
      runId: "run-0",
      sessionKey: "main",
      ts: 120,
      toolCallId: "tool-1",
      name: "read_files",
      phase: "start",
    });

    persistChatReadonlyRun(host.chatReadonlyRun, "main");
    const restored = loadPersistedChatReadonlyRun("main");

    expect(restored?.phase).toBe("working");
    expect(restored?.runId).toBe("run-0");
    expect(restored?.toolSteps).toHaveLength(1);
  });

  it("transitions from starting to working to drafting", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-1", 100, "main");

    expect(host.chatReadonlyRun.phase).toBe("starting");
    expect(host.chatReadonlyRun.activity.map((item) => item.kind)).toEqual(["phase"]);

    updateChatReadonlyRunFromTool(host, {
      runId: "run-1",
      sessionKey: "main",
      ts: 120,
      toolCallId: "tool-1",
      name: "read_files",
      phase: "start",
    });

    expect(host.chatReadonlyRun.phase).toBe("working");
    expect(host.chatReadonlyRun.toolSteps).toHaveLength(1);
    expect(host.chatReadonlyRun.toolSteps[0]?.status).toBe("running");
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("tool");

    updateChatReadonlyRunFromProgress(host, {
      runId: "run-1",
      sessionKey: "main",
      ts: 150,
      summary: "Collected the relevant files.",
      phase: "analysis",
    });

    expect(host.chatReadonlyRun.latestProgress).toBe("Collected the relevant files.");
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("progress");

    updateChatReadonlyRunFromChat(host, {
      runId: "run-1",
      sessionKey: "main",
      state: "delta",
      ts: 180,
      text: "I found the issue and I'm preparing a fix.",
    });

    expect(host.chatReadonlyRun.phase).toBe("drafting");
    expect(host.chatReadonlyRun.draftingText).toContain("preparing a fix");
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("draft");
  });

  it("keeps a completed workflow summary when the run finalizes", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-2", 100, "main");

    updateChatReadonlyRunFromChat(host, {
      runId: "run-2",
      sessionKey: "main",
      state: "final",
      ts: 220,
      text: "Done.",
      messageId: "msg-final",
      messageTimestamp: 230,
    });

    expect(host.chatReadonlyRun.runId).toBeNull();
    expect(host.chatReadonlyRun.phase).toBe("complete");
    expect(host.chatReadonlyRun.sessionKey).toBe("main");
    expect(host.chatReadonlyRun.completedAt).toBe(220);
    expect(host.chatReadonlyRun.finalMessageId).toBe("msg-final");
    expect(host.chatReadonlyRun.finalMessageTimestamp).toBe(230);
    expect(host.chatReadonlyRun.finalMessageText).toBe("Done.");
    expect(host.chatReadonlyRunHistory).toHaveLength(1);
    expect(host.chatReadonlyRunHistory[0]?.finalMessageId).toBe("msg-final");
  });

  it("persists archived workflow history alongside the current run", () => {
    const completedRun = {
      ...createChatReadonlyRunState("main"),
      phase: "complete" as const,
      startedAt: 100,
      updatedAt: 180,
      completedAt: 190,
      finalMessageId: "msg-1",
      finalMessageTimestamp: 200,
      finalMessageText: "first",
    };
    const activeRun = {
      ...createChatReadonlyRunState("main"),
      runId: "run-active",
      phase: "working" as const,
      startedAt: 210,
      updatedAt: 240,
    };

    persistChatReadonlyRun(activeRun, "main", [completedRun]);

    const restoredCurrent = loadPersistedChatReadonlyRun("main");
    const restoredHistory = loadPersistedChatReadonlyRunHistory("main");

    expect(restoredCurrent?.runId).toBe("run-active");
    expect(restoredHistory).toHaveLength(1);
    expect(restoredHistory[0]?.finalMessageId).toBe("msg-1");
  });

  it("marks terminal errors without keeping the run active", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-3", 100, "main");

    expect(isReadonlyRunActive(host.chatReadonlyRun)).toBe(true);

    setChatReadonlyRunTerminal(host, "error", {
      sessionKey: "main",
      ts: 180,
      errorMessage: "disconnected (1006): no reason",
    });

    expect(host.chatReadonlyRun.runId).toBeNull();
    expect(host.chatReadonlyRun.phase).toBe("error");
    expect(host.chatReadonlyRun.lastError).toBe("disconnected (1006): no reason");
    expect(isReadonlyRunActive(host.chatReadonlyRun)).toBe(false);
    expect(host.chatReadonlyRun.activity.at(-1)?.kind).toBe("error");
  });

  it("does not archive or persist failed workflows without a final reply anchor", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-err", 100, "main");

    setChatReadonlyRunTerminal(host, "error", {
      sessionKey: "main",
      ts: 180,
      errorMessage: "upstream unavailable",
    });

    expect(host.chatReadonlyRunHistory).toHaveLength(0);
    expect(isReadonlyRunVisible(host.chatReadonlyRun, "main")).toBe(false);

    persistChatReadonlyRun(host.chatReadonlyRun, "main", host.chatReadonlyRunHistory);
    expect(loadPersistedChatReadonlyRun("main")).toBeNull();
    expect(loadPersistedChatReadonlyRunHistory("main")).toEqual([]);
  });

  it("records a finalizing phase in the activity feed when lifecycle ends", () => {
    const host = createHost();
    startChatReadonlyRun(host, "run-4", 100, "main");

    updateChatReadonlyRunFromLifecycle(host, {
      runId: "run-4",
      sessionKey: "main",
      ts: 160,
      phase: "end",
    });

    expect(host.chatReadonlyRun.phase).toBe("finalizing");
    expect(host.chatReadonlyRun.activity.some(
      (item) => item.kind === "phase" && item.phase === "finalizing",
    )).toBe(true);
  });
});
