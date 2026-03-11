import { render } from "lit";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createChatReadonlyRunState } from "../chat/readonly-run-state.ts";
import type { SessionsListResult } from "../types.ts";
import { renderChat, type ChatProps } from "./chat.ts";

function createSessions(): SessionsListResult {
  return {
    ts: 0,
    path: "",
    count: 0,
    defaults: { model: null, contextTokens: null },
    sessions: [],
  };
}

function createProps(overrides: Partial<ChatProps> = {}): ChatProps {
  return {
    sessionKey: "main",
    gatewayUrl: "ws://127.0.0.1:19001/ws",
    onSessionKeyChange: () => undefined,
    thinkingLevel: null,
    showThinking: false,
    loading: false,
    sending: false,
    canAbort: false,
    compactionStatus: null,
    messages: [],
    toolMessages: [],
    uxMode: "classic",
    readonlyRun: createChatReadonlyRunState("main"),
    readonlyRunHistory: [],
    stream: null,
    streamStartedAt: null,
    assistantAvatarUrl: null,
    draft: "",
    queue: [],
    connected: true,
    canSend: true,
    disabledReason: null,
    error: null,
    sessions: createSessions(),
    focusMode: false,
    assistantName: "OpenAcosmi",
    assistantAvatar: null,
    onRefresh: () => undefined,
    onToggleFocusMode: () => undefined,
    onDraftChange: () => undefined,
    onSend: () => undefined,
    onQueueRemove: () => undefined,
    onNewSession: () => undefined,
    ...overrides,
  };
}

describe("chat view", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("renders compacting indicator as a badge", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          compactionStatus: {
            active: true,
            startedAt: Date.now(),
            completedAt: null,
          },
        }),
      ),
      container,
    );

    const indicator = container.querySelector(".compaction-indicator--active");
    expect(indicator).not.toBeNull();
    expect(indicator?.textContent).toMatch(/Compacting context|正在压缩上下文/);
  });

  it("renders completion indicator shortly after compaction", () => {
    const container = document.createElement("div");
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(1_000);
    render(
      renderChat(
        createProps({
          compactionStatus: {
            active: false,
            startedAt: 900,
            completedAt: 900,
          },
        }),
      ),
      container,
    );

    const indicator = container.querySelector(".compaction-indicator--complete");
    expect(indicator).not.toBeNull();
    expect(indicator?.textContent).toMatch(/Context compacted|上下文已压缩/);
    nowSpy.mockRestore();
  });

  it("hides stale compaction completion indicator", () => {
    const container = document.createElement("div");
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(10_000);
    render(
      renderChat(
        createProps({
          compactionStatus: {
            active: false,
            startedAt: 0,
            completedAt: 0,
          },
        }),
      ),
      container,
    );

    expect(container.querySelector(".compaction-indicator")).toBeNull();
    nowSpy.mockRestore();
  });

  it("renders browser extension guide against gateway url", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          gatewayUrl: "ws://10.0.0.8:19001/ws",
          messages: [
            {
              content: [{ type: "text", text: "Browser tool is not available" }],
            },
          ],
        }),
      ),
      container,
    );

    const link = container.querySelector(".browser-ext-banner a") as HTMLAnchorElement | null;
    expect(link?.getAttribute("href")).toBe("http://10.0.0.8:19001/browser-extension/");
  });

  it("shows a stop button when aborting is available", () => {
    const container = document.createElement("div");
    const onAbort = vi.fn();
    render(
      renderChat(
        createProps({
          sending: true,
          canAbort: true,
          onAbort,
        }),
      ),
      container,
    );

    const stopButton = container.querySelector(".chat-compose__send--abort") as HTMLButtonElement | null;
    expect(stopButton).not.toBeNull();
    expect(container.querySelector(".chat-compose__below")).toBeNull();
    stopButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onAbort).toHaveBeenCalledTimes(1);
  });

  it("shows chat errors inside the chat card", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          error: "gateway offline",
        }),
      ),
      container,
    );

    const callout = container.querySelector(".callout.danger");
    expect(callout?.textContent).toContain("gateway offline");
  });

  it("shows a new session button when aborting is unavailable", () => {
    const container = document.createElement("div");
    const onNewSession = vi.fn();
    render(
      renderChat(
        createProps({
          sending: true,
          canAbort: false,
          onNewSession,
        }),
      ),
      container,
    );

    const newSessionButton = container.querySelector(".chat-compose__new-session") as HTMLButtonElement | null;
    expect(newSessionButton).not.toBeNull();
    newSessionButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onNewSession).toHaveBeenCalledTimes(0);
  });

  it("renders the codex readonly surface without the classic processing card", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          stream: "",
          streamStartedAt: 1_000,
          readonlyRun: {
            runId: "run-1",
            sessionKey: "main",
            phase: "working",
            startedAt: 1_000,
            updatedAt: 1_100,
            completedAt: null,
            latestProgress: "Scanning the workspace and gathering context.",
            progressPhase: "analysis",
            draftingText: null,
            lastToolName: "read_files",
            activity: [
              {
                id: "phase:starting",
                kind: "phase",
                phase: "starting",
                ts: 1_000,
                updatedAt: 1_000,
              },
              {
                id: "progress:1100",
                kind: "progress",
                summary: "Scanning the workspace and gathering context.",
                progressPhase: "analysis",
                ts: 1_100,
                updatedAt: 1_100,
              },
            ],
            toolSteps: [
              {
                toolCallId: "tool-1",
                name: "read_files",
                detail: "ui/src/ui/views/chat.ts",
                status: "running",
                startedAt: 1_000,
                updatedAt: 1_100,
                outputPreview: null,
              },
            ],
            lastError: null,
            finalMessageId: null,
            finalMessageTimestamp: null,
            finalMessageText: null,
          },
        }),
      ),
      container,
    );

    expect(container.querySelector(".chat-readonly-run")).not.toBeNull();
    expect(container.querySelector(".chat-processing-card")).toBeNull();
    expect(container.textContent).toMatch(/Agent Working|智能体工作中/);
    expect(container.textContent).toContain("Scanning the workspace and gathering context.");
  });

  it("announces readonly status changes without exposing the timer as a live region update", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          readonlyRun: {
            runId: "run-2",
            sessionKey: "main",
            phase: "working",
            startedAt: 1_000,
            updatedAt: 1_100,
            completedAt: null,
            latestProgress: "Scanning the workspace and gathering context.",
            progressPhase: "analysis",
            draftingText: null,
            lastToolName: "read_files",
            activity: [
              {
                id: "phase:starting",
                kind: "phase",
                phase: "starting",
                ts: 1_000,
                updatedAt: 1_000,
              },
              {
                id: "progress:1100",
                kind: "progress",
                summary: "Scanning the workspace and gathering context.",
                progressPhase: "analysis",
                ts: 1_100,
                updatedAt: 1_100,
              },
            ],
            toolSteps: [],
            lastError: null,
            finalMessageId: null,
            finalMessageTimestamp: null,
            finalMessageText: null,
          },
        }),
      ),
      container,
    );

    const surface = container.querySelector(".chat-readonly-run");
    const liveStatus = container.querySelector(".chat-readonly-run__sr-status");
    const timer = container.querySelector(".chat-readonly-run__timer");
    expect(surface?.getAttribute("aria-live")).toBeNull();
    expect(liveStatus?.getAttribute("role")).toBe("status");
    expect(liveStatus?.getAttribute("aria-live")).toBe("polite");
    expect(timer?.getAttribute("aria-hidden")).toBe("true");
  });

  it("does not keep a failed workflow card visible without a final reply anchor", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          messages: [
            {
              role: "user",
              content: [{ type: "text", text: "帮我执行任务" }],
              timestamp: 1_000,
            },
          ],
          readonlyRun: {
            ...createChatReadonlyRunState("main"),
            phase: "error",
            startedAt: 1_100,
            updatedAt: 1_300,
            completedAt: 1_300,
            lastError: "model overloaded",
            activity: [],
            toolSteps: [],
          },
        }),
      ),
      container,
    );

    expect(container.querySelector(".chat-readonly-run")).toBeNull();
    expect(container.textContent).toContain("帮我执行任务");
  });

  it("keeps the workflow surface before the final assistant reply after completion", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          messages: [
            {
              role: "user",
              content: [{ type: "text", text: "修一下聊天页" }],
              timestamp: 1_000,
            },
            {
              id: "msg-final",
              role: "assistant",
              content: [{ type: "text", text: "已经修好了。" }],
              timestamp: 2_000,
            },
          ],
          readonlyRun: {
            runId: null,
            sessionKey: "main",
            phase: "complete",
            startedAt: 1_100,
            updatedAt: 1_900,
            completedAt: 1_950,
            latestProgress: "已经完成聊天页工作流展示的整理。",
            progressPhase: "implementation",
            draftingText: "已经把工作流摘要保留在最终回复前。",
            lastToolName: "apply_patch",
            activity: [
              {
                id: "tool:tool-1",
                kind: "tool",
                toolCallId: "tool-1",
                name: "lookup_skill",
                detail: "chat-ui",
                status: "done",
                phase: "result",
                outputPreview: "Read SKILL.md",
                ts: 1_300,
                updatedAt: 1_500,
              },
            ],
            toolSteps: [
              {
                toolCallId: "tool-1",
                name: "lookup_skill",
                detail: "chat-ui",
                status: "done",
                startedAt: 1_300,
                updatedAt: 1_500,
                outputPreview: "Read SKILL.md",
              },
            ],
            lastError: null,
            finalMessageId: "msg-final",
            finalMessageTimestamp: 2_000,
            finalMessageText: "已经修好了。",
          },
        }),
      ),
      container,
    );

    const thread = container.querySelector(".chat-thread");
    const readonlyRun = thread?.querySelector(".chat-readonly-run");
    const reply = thread?.querySelector(".chat-readonly-run__reply");
    const details = thread?.querySelector(".chat-readonly-run__details") as HTMLDetailsElement | null;
    expect(readonlyRun).not.toBeNull();
    expect(readonlyRun?.textContent).toContain("Read SKILL.md");
    expect(reply?.textContent).toContain("已经修好了。");
    expect(details?.open).toBe(false);
  });

  it("keeps long tool chips within the workflow card bounds", () => {
    const container = document.createElement("div");
    container.style.width = "420px";
    document.body.appendChild(container);

    try {
      render(
        renderChat(
          createProps({
            uxMode: "codex-readonly",
            readonlyRun: {
              runId: "run-overflow",
              sessionKey: "main",
              phase: "working",
              startedAt: 1_100,
              updatedAt: 1_500,
              completedAt: null,
              latestProgress: "正在检查远程频道配置。",
              progressPhase: "analysis",
              draftingText: null,
              lastToolName: "bash",
              activity: [],
              toolSteps: [
                {
                  toolCallId: "tool-overflow",
                  name: "bash",
                  detail: "security find-generic-password -s \"openacosmi-email\" -w 2>/dev/null && echo \"Email credentials found\" || echo \"Email credentials NOT found\"",
                  status: "running",
                  startedAt: 1_200,
                  updatedAt: 1_500,
                  outputPreview: null,
                },
              ],
              lastError: null,
              finalMessageId: null,
              finalMessageTimestamp: null,
              finalMessageText: null,
            },
          }),
        ),
        container,
      );

      const readonlyRun = container.querySelector(".chat-readonly-run") as HTMLElement | null;
      const chip = container.querySelector(".chat-readonly-run__tool-chip") as HTMLElement | null;
      const chipText = container.querySelector(".chat-readonly-run__tool-chip-text") as HTMLElement | null;
      expect(readonlyRun).not.toBeNull();
      expect(chip).not.toBeNull();
      expect(chipText).not.toBeNull();

      const runRect = readonlyRun!.getBoundingClientRect();
      const chipRect = chip!.getBoundingClientRect();
      expect(chipRect.right).toBeLessThanOrEqual(runRect.right + 1);
    } finally {
      container.remove();
    }
  });

  it("keeps earlier workflow cards attached after newer replies arrive", () => {
    const container = document.createElement("div");
    render(
      renderChat(
        createProps({
          uxMode: "codex-readonly",
          messages: [
            {
              role: "user",
              content: [{ type: "text", text: "第一问" }],
              timestamp: 1_000,
            },
            {
              id: "msg-1",
              role: "assistant",
              content: [{ type: "text", text: "第一答" }],
              timestamp: 2_000,
            },
            {
              role: "user",
              content: [{ type: "text", text: "第二问" }],
              timestamp: 3_000,
            },
            {
              id: "msg-2",
              role: "assistant",
              content: [{ type: "text", text: "第二答" }],
              timestamp: 4_000,
            },
          ],
          readonlyRun: createChatReadonlyRunState("main"),
          readonlyRunHistory: [
            {
              ...createChatReadonlyRunState("main"),
              phase: "complete",
              startedAt: 1_100,
              updatedAt: 1_900,
              completedAt: 1_950,
              activity: [],
              toolSteps: [],
              finalMessageId: "msg-1",
              finalMessageTimestamp: 2_000,
              finalMessageText: "第一答",
            },
            {
              ...createChatReadonlyRunState("main"),
              phase: "complete",
              startedAt: 3_100,
              updatedAt: 3_900,
              completedAt: 3_950,
              activity: [],
              toolSteps: [],
              finalMessageId: "msg-2",
              finalMessageTimestamp: 4_000,
              finalMessageText: "第二答",
            },
          ],
        }),
      ),
      container,
    );

    const workflowReplies = [...container.querySelectorAll(".chat-readonly-run__reply")]
      .map((node) => node.textContent ?? "");
    expect(workflowReplies).toHaveLength(2);
    expect(workflowReplies[0]).toContain("第一答");
    expect(workflowReplies[1]).toContain("第二答");
  });

  it("renders the composer model selector and keeps send visible when empty", () => {
    const container = document.createElement("div");
    const onOpenModelConfig = vi.fn();
    render(
      renderChat(
        createProps({
          models: [
            {
              id: "gpt-4o",
              name: "GPT-4o",
              provider: "openai",
              source: "custom",
            },
            {
              id: "gemini-2.5-flash",
              name: "Gemini 2.5 Flash",
              provider: "google",
              source: "custom",
            },
          ],
          currentModel: "openai/gpt-4o",
          onOpenModelConfig,
        }),
      ),
      container,
    );

    const modelShell = container.querySelector(".chat-compose__model-shell");
    const modelSelect = container.querySelector(".chat-compose__model-select");
    const modelChips = Array.from(container.querySelectorAll(".chat-compose__model-chip")).map((node) =>
      node.textContent?.replace(/\s+/g, " ").trim() ?? ""
    );
    const activeChip = container.querySelector(".chat-compose__model-chip--active");
    const placeholderChip = container.querySelector(".chat-compose__model-chip--placeholder");
    const promo = container.querySelector(".chat-compose__model-popover");
    const groupLabels = Array.from(container.querySelectorAll(".chat-compose__model-group-label")).map((node) =>
      node.textContent?.trim() ?? ""
    );
    const addButton = container.querySelector(".chat-compose__model-add") as HTMLButtonElement | null;
    const sendButton = container.querySelector(".chat-compose__send") as HTMLButtonElement | null;
    expect(modelShell).not.toBeNull();
    expect(modelSelect?.textContent).toContain("OpenAI");
    expect(modelSelect?.textContent).toContain("GPT-4o");
    expect(modelChips).toContain("内置");
    expect(modelChips).toContain("自定义");
    expect(activeChip?.textContent?.trim()).toBe("自定义");
    expect(placeholderChip?.textContent?.trim()).toBe("内置");
    expect(promo?.textContent).toContain("免费 tk");
    expect(container.querySelector(".chat-compose__model-dot")).toBeNull();
    expect(groupLabels).toContain("Google");
    expect(groupLabels).toContain("OpenAI");
    expect(addButton?.textContent?.trim()).toBe("新增自定义");
    addButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onOpenModelConfig).toHaveBeenCalledTimes(1);
    expect(sendButton).not.toBeNull();
    expect(sendButton?.disabled).toBe(true);
  });
});
