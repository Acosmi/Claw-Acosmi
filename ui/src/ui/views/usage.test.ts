import { render } from "lit";
import { describe, expect, it } from "vitest";
import {
  renderUsage,
  type UsageColumnId,
  type UsageProps,
  type UsageSessionEntry,
  type UsageTotals,
} from "./usage.ts";

const totals: UsageTotals = {
  input: 120,
  output: 48,
  cacheRead: 0,
  cacheWrite: 0,
  totalTokens: 168,
  totalCost: 0,
  inputCost: 0,
  outputCost: 0,
  cacheReadCost: 0,
  cacheWriteCost: 0,
  missingCostEntries: 0,
};

const visibleColumns: UsageColumnId[] = [
  "channel",
  "agent",
  "provider",
  "model",
  "messages",
  "tools",
  "errors",
  "duration",
];

function buildSession(): UsageSessionEntry {
  return {
    key: "session-1",
    updatedAt: Date.now(),
    agentId: "main",
    channel: "task",
    modelProvider: "qwen",
    model: "qwen3.5-plus",
    usage: {
      ...totals,
      firstActivity: Date.now() - 60_000,
      lastActivity: Date.now(),
      durationMs: 60_000,
      activityDates: ["2026-03-10"],
      dailyBreakdown: [{ date: "2026-03-10", tokens: totals.totalTokens, cost: 0 }],
      dailyMessageCounts: [
        {
          date: "2026-03-10",
          total: 2,
          user: 1,
          assistant: 1,
          toolCalls: 0,
          toolResults: 0,
          errors: 0,
        },
      ],
      dailyModelUsage: [],
      dailyLatency: [],
      messageCounts: {
        total: 2,
        user: 1,
        assistant: 1,
        toolCalls: 0,
        toolResults: 0,
        errors: 0,
      },
      toolUsage: {
        totalCalls: 0,
        uniqueTools: 0,
        tools: null as unknown as Array<{ name: string; count: number }>,
      },
      modelUsage: [],
      latency: {
        count: 1,
        avgMs: 60_000,
        p95Ms: 60_000,
        minMs: 60_000,
        maxMs: 60_000,
      },
    },
    contextWeight: null,
  };
}

function buildProps(session: UsageSessionEntry): UsageProps {
  return {
    loading: false,
    error: null,
    startDate: "2026-03-01",
    endDate: "2026-03-10",
    sessions: [session],
    sessionsLimitReached: false,
    totals,
    aggregates: null,
    costDaily: [{ date: "2026-03-10", ...totals }],
    selectedSessions: [session.key],
    selectedDays: [],
    selectedHours: [],
    chartMode: "tokens",
    dailyChartMode: "total",
    timeSeriesMode: "cumulative",
    timeSeriesBreakdownMode: "total",
    timeSeries: null,
    timeSeriesLoading: false,
    sessionLogs: null,
    sessionLogsLoading: false,
    sessionLogsExpanded: false,
    logFilterRoles: [],
    logFilterTools: [],
    logFilterHasTools: false,
    logFilterQuery: "",
    query: "",
    queryDraft: "",
    sessionSort: "tokens",
    sessionSortDir: "desc",
    recentSessions: [session.key],
    sessionsTab: "all",
    visibleColumns,
    timeZone: "local",
    contextExpanded: false,
    headerPinned: false,
    onStartDateChange: () => undefined,
    onEndDateChange: () => undefined,
    onRefresh: () => undefined,
    onTimeZoneChange: () => undefined,
    onToggleContextExpanded: () => undefined,
    onToggleHeaderPinned: () => undefined,
    onToggleSessionLogsExpanded: () => undefined,
    onLogFilterRolesChange: () => undefined,
    onLogFilterToolsChange: () => undefined,
    onLogFilterHasToolsChange: () => undefined,
    onLogFilterQueryChange: () => undefined,
    onLogFilterClear: () => undefined,
    onSelectSession: () => undefined,
    onChartModeChange: () => undefined,
    onDailyChartModeChange: () => undefined,
    onTimeSeriesModeChange: () => undefined,
    onTimeSeriesBreakdownChange: () => undefined,
    onSelectDay: () => undefined,
    onSelectHour: () => undefined,
    onClearDays: () => undefined,
    onClearHours: () => undefined,
    onClearSessions: () => undefined,
    onClearFilters: () => undefined,
    onQueryDraftChange: () => undefined,
    onApplyQuery: () => undefined,
    onClearQuery: () => undefined,
    onSessionSortChange: () => undefined,
    onSessionSortDirChange: () => undefined,
    onSessionsTabChange: () => undefined,
    onToggleColumn: () => undefined,
  };
}

describe("usage view", () => {
  it("renders legacy null toolUsage.tools payloads without crashing", async () => {
    const container = document.createElement("div");

    expect(() => render(renderUsage(buildProps(buildSession())), container)).not.toThrow();
    await Promise.resolve();

    expect(container.textContent).toContain("用量");
    expect(container.textContent).toContain("0");
  });
});
