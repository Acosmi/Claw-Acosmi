import type { GatewayBrowserClient } from "../gateway.ts";

// ── Types ──

export type MemoryItem = {
  id: string;
  userId: string;
  content: string;
  type: string;
  category: string;
  importanceScore: number;
  decayFactor: number;
  retentionPolicy: string;
  accessCount: number;
  createdAt: number;
  updatedAt: number;
  lastAccessedAt?: number;
  archivedAt?: number;
  vfsPath?: string;
  vfsContent?: string;
};

export type MemoryDetail = MemoryItem & {
  vfsContent?: string;
  vfsLevel?: number;
};

export type MemoryStatus = {
  enabled: boolean;
  vectorMode: string;
  dbPath: string;
  vfsPath: string;
  vectorReady: boolean;
  memoryCount: number;
  diskUsage: number;
};

export type MemoryImportResult = {
  imported: number;
  skipped: number;
  updated: number;
  failed: number;
  total: number;
  skills: Array<{ name: string; id?: string; status: string; error?: string }>;
};

export type MemoryLLMConfig = {
  provider: string;
  model: string;
  baseUrl: string;
  hasApiKey: boolean;
  hasOwnApiKey: boolean;
  inherited: boolean;
  providers: { id: string; label: string; hasApiKey: boolean; defaultModel: string; defaultBaseUrl: string }[];
};

export type MemoryStats = {
  byType: Record<string, number>;
  byCategory: Record<string, number>;
  byRetention: Record<string, number>;
  decayHealth: {
    healthy: number;
    fading: number;
    critical: number;
    permanent: number;
  };
  totalAccess: number;
  avgImportance: number;
  oldestAt: number;
  newestAt: number;
};

export type MemorySearchResult = {
  id: string;
  content: string;
  type: string;
  category: string;
  score: number;
  source: string;
};

export type MemoryState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  memoryLoading: boolean;
  memoryList: MemoryItem[] | null;
  memoryTotal: number;
  memoryError: string | null;
  memoryDetail: MemoryDetail | null;
  memoryStatus: MemoryStatus | null;
  memoryImporting: boolean;
  memoryImportResult: MemoryImportResult | null;
  memoryPage: number;
  memoryPageSize: number;
  memoryFilterType: string;
  memoryFilterCategory: string;
  memoryDetailLevel: number;
  memoryLLMConfig: MemoryLLMConfig | null;
  memoryLLMConfigOpen: boolean;
  memoryStats: MemoryStats | null;
  memorySearchQuery: string;
  memorySearchResults: MemorySearchResult[] | null;
  memorySearching: boolean;
};

// ── Controller functions ──

export async function loadMemoryStatus(state: MemoryState) {
  if (!state.client || !state.connected) return;
  try {
    const res = await state.client.request<MemoryStatus>("memory.uhms.status");
    if (res) {
      state.memoryStatus = res;
    }
  } catch (err) {
    state.memoryError = String(err);
  }
}

export async function loadMemoryList(
  state: MemoryState,
  opts?: { page?: number; type?: string; category?: string },
) {
  if (!state.client || !state.connected) return;
  if (state.memoryLoading) return;
  state.memoryLoading = true;
  state.memoryError = null;
  try {
    const page = opts?.page ?? state.memoryPage;
    const filterType = opts?.type ?? state.memoryFilterType;
    const filterCategory = opts?.category ?? state.memoryFilterCategory;
    const params: Record<string, unknown> = {
      limit: state.memoryPageSize,
      offset: page * state.memoryPageSize,
    };
    if (filterType) params.type = filterType;
    if (filterCategory) params.category = filterCategory;

    const res = await state.client.request<{
      memories: MemoryItem[];
      total: number;
      limit: number;
      offset: number;
    }>("memory.list", params);

    if (res) {
      state.memoryList = res.memories;
      state.memoryTotal = res.total;
      state.memoryPage = page;
      state.memoryFilterType = filterType;
      state.memoryFilterCategory = filterCategory;
    }
  } catch (err) {
    state.memoryError = String(err);
  } finally {
    state.memoryLoading = false;
  }
}

export async function loadMemoryDetail(state: MemoryState, id: string, level: number) {
  if (!state.client || !state.connected) return;
  // 立即更新选中层级，确保 UI 点击反馈及时可见。
  const mutableState = state as MemoryState & { __memoryDetailReqToken?: number };
  mutableState.memoryDetailLevel = level;
  const reqToken = (mutableState.__memoryDetailReqToken ?? 0) + 1;
  mutableState.__memoryDetailReqToken = reqToken;
  try {
    const res = await state.client.request<MemoryDetail>("memory.get", { id, level });
    if (mutableState.__memoryDetailReqToken !== reqToken) {
      // 忽略过期响应，避免快速切换标签时内容“跳回”旧层级。
      return;
    }
    if (res) {
      state.memoryDetail = res;
      if (typeof res.vfsLevel === "number" && Number.isFinite(res.vfsLevel)) {
        state.memoryDetailLevel = Math.max(0, Math.min(2, Math.trunc(res.vfsLevel)));
      }
    }
  } catch (err) {
    if (mutableState.__memoryDetailReqToken !== reqToken) return;
    state.memoryError = String(err);
  }
}

export async function deleteMemory(state: MemoryState, id: string) {
  if (!state.client || !state.connected) return;
  try {
    await state.client.request("memory.delete", { id });
    // Refresh list after delete
    await loadMemoryList(state);
    // Clear detail if deleted item was shown
    if (state.memoryDetail?.id === id) {
      state.memoryDetail = null;
    }
  } catch (err) {
    state.memoryError = String(err);
  }
}

export async function importSkills(state: MemoryState) {
  if (!state.client || !state.connected) return;
  if (state.memoryImporting) return;
  state.memoryImporting = true;
  state.memoryImportResult = null;
  state.memoryError = null;
  try {
    const res = await state.client.request<MemoryImportResult>("memory.import.skills");
    if (res) {
      state.memoryImportResult = res;
      // Refresh list after import
      await loadMemoryList(state);
    }
  } catch (err) {
    state.memoryError = String(err);
  } finally {
    state.memoryImporting = false;
  }
}

// ── LLM Config ──

export async function loadMemoryLLMConfig(state: MemoryState): Promise<MemoryLLMConfig | null> {
  if (!state.client || !state.connected) return null;
  try {
    const res = await state.client.request<MemoryLLMConfig>("memory.uhms.llm.get");
    if (res) {
      state.memoryLLMConfig = res;
    }
    return res;
  } catch (err) {
    state.memoryError = String(err);
    return null;
  }
}

export async function saveMemoryLLMConfig(
  state: MemoryState,
  params: { provider: string; model: string; baseUrl?: string; apiKey?: string },
): Promise<boolean> {
  if (!state.client || !state.connected) return false;
  try {
    await state.client.request("memory.uhms.llm.set", params);
    // Refresh LLM config after save
    await loadMemoryLLMConfig(state);
    return true;
  } catch (err) {
    state.memoryError = String(err);
    return false;
  }
}

// ── Stats ──

export async function loadMemoryStats(state: MemoryState) {
  if (!state.client || !state.connected) return;
  try {
    const res = await state.client.request<MemoryStats>("memory.stats");
    if (res) {
      state.memoryStats = res;
    }
  } catch (err) {
    state.memoryError = String(err);
  }
}

// ── Search ──

export async function searchMemories(state: MemoryState, query: string) {
  if (!state.client || !state.connected) return;
  if (state.memorySearching) return;
  state.memorySearching = true;
  state.memorySearchQuery = query;
  try {
    const res = await state.client.request<{ results: MemorySearchResult[]; count: number }>(
      "memory.uhms.search",
      { query, topK: 20 },
    );
    if (res) {
      state.memorySearchResults = res.results;
    }
  } catch (err) {
    state.memoryError = String(err);
  } finally {
    state.memorySearching = false;
  }
}

export function clearMemorySearch(state: MemoryState) {
  state.memorySearchQuery = "";
  state.memorySearchResults = null;
}
