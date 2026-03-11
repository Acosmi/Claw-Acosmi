import { html, nothing } from "lit";
import type { MemoryDetail, MemoryImportResult, MemoryItem, MemoryLLMConfig, MemorySearchResult, MemoryStats, MemoryStatus } from "../controllers/memory.ts";
import { formatRelativeTimestamp } from "../format.ts";
import { t } from "../i18n.ts";

export type MemoryProps = {
  loading: boolean;
  status: MemoryStatus | null;
  list: MemoryItem[] | null;
  total: number;
  error: string | null;
  detail: MemoryDetail | null;
  detailLevel: number;
  importing: boolean;
  importResult: MemoryImportResult | null;
  page: number;
  pageSize: number;
  filterType: string;
  filterCategory: string;
  onRefresh: () => void;
  onLoadStatus: () => void;
  onPageChange: (page: number) => void;
  onFilterType: (type: string) => void;
  onFilterCategory: (category: string) => void;
  onSelectMemory: (id: string, level: number) => void;
  onDeleteMemory: (id: string) => void;
  onImportSkills: () => void;
  onDetailLevel: (level: number) => void;
  onCloseDetail: () => void;
  // LLM config
  llmConfig: MemoryLLMConfig | null;
  llmConfigOpen: boolean;
  onLLMConfigToggle: () => void;
  onLLMConfigSave: (provider: string, model: string, baseUrl: string, apiKey?: string) => Promise<boolean>;
  // Stats & Search
  stats: MemoryStats | null;
  searchQuery: string;
  searchResults: MemorySearchResult[] | null;
  searching: boolean;
  onSearch: (query: string) => void;
  onClearSearch: () => void;
  onLoadStats: () => void;
};

const MEMORY_TYPES = ["", "episodic", "semantic", "procedural"];
const MEMORY_CATEGORIES = [
  "",
  "preference",
  "habit",
  "profile",
  "skill",
  "relationship",
  "event",
  "opinion",
  "fact",
  "goal",
  "task",
  "reminder",
  "insight",
  "summary",
];

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatTimestamp(unix: number): string {
  if (!unix) return "-";
  return new Date(unix * 1000).toLocaleString();
}

function truncateContent(content: string, max = 80): string {
  if (!content) return "-";
  return content.length > max ? content.slice(0, max) + "..." : content;
}

/** Translate a memory type key (e.g. "episodic") using i18n, fallback to raw key */
function translateType(raw: string): string {
  if (!raw) return "-";
  const key = `memory.type.${raw}`;
  const val = t(key);
  return val === key ? raw : val;
}

/** Translate a category key (e.g. "preference") using i18n, fallback to raw key */
function translateCategory(raw: string): string {
  if (!raw) return "-";
  const key = `memory.cat.${raw}`;
  const val = t(key);
  return val === key ? raw : val;
}

/** Color map for memory types */
const TYPE_COLORS: Record<string, string> = {
  episodic: "#5c7cfa",
  semantic: "#14b8a6",
  procedural: "#ffa726",
  permanent: "#7e57c2",
  imagination: "#ec407a",
};

/** Color map for decay health */
const DECAY_COLORS = {
  permanent: "#7e57c2",
  healthy: "#14b8a6",
  fading: "#ffa726",
  critical: "#ef4444",
};

// ── LLM Config inline panel — form-based with explicit Save ──

// Module-scope draft state — survives Lit re-renders
let _llmDraft: { provider: string; model: string; baseUrl: string; apiKey: string } | null = null;
let _llmSaveStatus: "idle" | "saving" | "saved" | "error" = "idle";
let _llmSaveTimer: ReturnType<typeof setTimeout> | null = null;

/** Reset draft when panel opens — sync from server config */
function ensureLLMDraft(cfg: MemoryLLMConfig | null) {
  if (!_llmDraft) {
    _llmDraft = {
      provider: cfg?.provider ?? "",
      model: cfg?.model ?? "",
      baseUrl: cfg?.baseUrl ?? "",
      apiKey: "",
    };
  }
}

export function resetLLMDraft() {
  _llmDraft = null;
  _llmSaveStatus = "idle";
  if (_llmSaveTimer) { clearTimeout(_llmSaveTimer); _llmSaveTimer = null; }
}

function renderLLMConfigPanel(props: MemoryProps) {
  const cfg = props.llmConfig;
  const providers = cfg?.providers ?? [];
  const hasOwnApiKey = cfg?.hasOwnApiKey ?? false;
  const inherited = cfg?.inherited ?? false;

  ensureLLMDraft(cfg);
  const draft = _llmDraft!;

  const currentProviderInfo = providers.find((x) => x.id === draft.provider);

  // Check if draft differs from saved config
  const hasChanges =
    draft.provider !== (cfg?.provider ?? "") ||
    draft.model !== (cfg?.model ?? "") ||
    draft.baseUrl !== (cfg?.baseUrl ?? "") ||
    draft.apiKey !== "";

  const handleProviderChange = (e: Event) => {
    const sel = (e.target as HTMLSelectElement).value;
    const p = providers.find((x) => x.id === sel);
    draft.provider = sel;
    draft.model = p?.defaultModel ?? "";
    draft.baseUrl = p?.defaultBaseUrl ?? "";
    draft.apiKey = "";
    _llmSaveStatus = "idle";
    // Force re-render: update select + dependent placeholders
    const modelEl = document.getElementById("uhms-llm-model") as HTMLInputElement | null;
    const baseUrlEl = document.getElementById("uhms-llm-baseurl") as HTMLInputElement | null;
    if (modelEl) modelEl.value = draft.model;
    if (baseUrlEl) baseUrlEl.value = draft.baseUrl;
  };

  const handleSave = async () => {
    _llmSaveStatus = "saving";
    const apiKeyParam = draft.apiKey || undefined;
    const ok = await props.onLLMConfigSave(draft.provider, draft.model, draft.baseUrl, apiKeyParam);
    if (ok) {
      _llmSaveStatus = "saved";
      draft.apiKey = ""; // reset apiKey field after successful save
      _llmDraft = null;
      // Clear the password input
      const apiKeyEl = document.getElementById("uhms-llm-apikey") as HTMLInputElement | null;
      if (apiKeyEl) apiKeyEl.value = "";
    } else {
      _llmSaveStatus = "error";
    }
    // Auto-dismiss status after 3s
    if (_llmSaveTimer) clearTimeout(_llmSaveTimer);
    _llmSaveTimer = setTimeout(() => { _llmSaveStatus = "idle"; _llmSaveTimer = null; }, 3000);
  };

  const handleClearApiKey = async () => {
    _llmSaveStatus = "saving";
    const ok = await props.onLLMConfigSave(draft.provider, draft.model, draft.baseUrl, "");
    _llmSaveStatus = ok ? "saved" : "error";
    if (ok) {
      _llmDraft = null;
    }
    if (_llmSaveTimer) clearTimeout(_llmSaveTimer);
    _llmSaveTimer = setTimeout(() => { _llmSaveStatus = "idle"; _llmSaveTimer = null; }, 3000);
  };

  const handleUsePrimaryModel = async () => {
    _llmSaveStatus = "saving";
    const ok = await props.onLLMConfigSave("", "", "", "");
    _llmSaveStatus = ok ? "saved" : "error";
    if (ok) {
      _llmDraft = null;
    }
    if (_llmSaveTimer) clearTimeout(_llmSaveTimer);
    _llmSaveTimer = setTimeout(() => { _llmSaveStatus = "idle"; _llmSaveTimer = null; }, 3000);
  };

  const handleClose = () => {
    resetLLMDraft();
    props.onLLMConfigToggle();
  };

  const statusBadge = _llmSaveStatus === "saving"
    ? html`<span style="color:#6c757d; font-size:0.82rem">${t("memory.llmSaving")}</span>`
    : _llmSaveStatus === "saved"
      ? html`<span style="color:#14b8a6; font-size:0.82rem">${t("memory.llmSaved")}</span>`
      : _llmSaveStatus === "error"
        ? html`<span style="color:#ef4444; font-size:0.82rem">${t("memory.llmSaveError")}</span>`
        : nothing;

  return html`
    <div style="
      margin-top: 0.75rem;
      padding: 1rem;
      background: #f8f9fa;
      border: 1px solid #dee2e6;
      border-radius: 8px;
    ">
      <div style="font-weight:600; margin-bottom:0.75rem; font-size:0.9rem">${t("memory.llmModel")} ${t("memory.llmConfig")}</div>
      ${inherited ? html`
        <div style="margin-bottom:0.75rem; font-size:0.82rem; color:#495057;">
          ${t("memory.llmSharedWithPrimary")}
        </div>
      ` : nothing}

      <div style="display:grid; grid-template-columns:auto 1fr; gap:8px 12px; align-items:center">
          <label style="font-size:0.85rem">${t("memory.llmProvider")}</label>
          <select class="input input--sm" id="uhms-llm-provider"
            .value=${draft.provider}
            @change=${handleProviderChange}>
            ${!draft.provider ? html`<option value="" selected disabled>${t("memory.llmSelectProvider")}</option>` : nothing}
            ${providers.map((p) => html`
              <option value=${p.id} ?selected=${p.id === draft.provider}>
                ${p.label}${p.hasApiKey ? "" : " (no key)"}
              </option>
            `)}
          </select>

          <label style="font-size:0.85rem">${t("memory.model")}</label>
          <input class="input input--sm" id="uhms-llm-model"
            .value=${draft.model}
            @input=${(e: Event) => { draft.model = (e.target as HTMLInputElement).value; _llmSaveStatus = "idle"; }} />

          <label style="font-size:0.85rem">${t("memory.llmBaseUrl")}</label>
          <input class="input input--sm" id="uhms-llm-baseurl"
            placeholder=${currentProviderInfo?.defaultBaseUrl || "https://..."}
            .value=${draft.baseUrl}
            @input=${(e: Event) => { draft.baseUrl = (e.target as HTMLInputElement).value; _llmSaveStatus = "idle"; }} />

          <label style="font-size:0.85rem">${t("memory.llmApiKey")}</label>
          <input class="input input--sm" id="uhms-llm-apikey"
            type="password"
            placeholder=${hasOwnApiKey ? t("memory.llmApiKeySet") : t("memory.llmApiKeyInherit")}
            @input=${(e: Event) => { draft.apiKey = (e.target as HTMLInputElement).value; _llmSaveStatus = "idle"; }} />
          ${hasOwnApiKey ? html`
            <span></span>
            <button class="btn btn--xs" style="justify-self:start"
              @click=${handleClearApiKey}>
              ${t("memory.llmApiKeyClear")}
            </button>
          ` : nothing}
      </div>

      <div style="margin-top:0.75rem; display:flex; align-items:center; justify-content:flex-end; gap:8px">
        ${statusBadge}
        ${!inherited && cfg?.provider ? html`
          <button class="btn btn--sm"
            ?disabled=${_llmSaveStatus === "saving"}
            @click=${handleUsePrimaryModel}>
            ${t("memory.llmUsePrimary")}
          </button>
        ` : nothing}
        <button class="btn btn--sm btn--primary"
          ?disabled=${!draft.provider || !hasChanges || _llmSaveStatus === "saving"}
          @click=${handleSave}>
          ${_llmSaveStatus === "saving" ? t("memory.llmSaving") : t("memory.llmSave")}
        </button>
        <button class="btn btn--sm" @click=${handleClose}>${t("memory.llmCancel")}</button>
      </div>
    </div>
  `;
}

// ── Card 2: Distribution & Health ──
function renderDistributionCard(stats: MemoryStats) {
  const typeEntries = Object.entries(stats.byType).sort((a, b) => b[1] - a[1]);
  const catEntries = Object.entries(stats.byCategory).sort((a, b) => b[1] - a[1]).filter(([, c]) => c > 0);
  const typeTotal = typeEntries.reduce((sum, [, c]) => sum + c, 0) || 1;
  const dh = stats.decayHealth;
  const decayTotal = dh.permanent + dh.healthy + dh.fading + dh.critical || 1;

  const decayBars = [
    { key: "permanent" as const, label: t("memory.decayPermanent"), count: dh.permanent, color: DECAY_COLORS.permanent },
    { key: "healthy" as const, label: t("memory.decayHealthy"), count: dh.healthy, color: DECAY_COLORS.healthy },
    { key: "fading" as const, label: t("memory.decayFading"), count: dh.fading, color: DECAY_COLORS.fading },
    { key: "critical" as const, label: t("memory.decayCritical"), count: dh.critical, color: DECAY_COLORS.critical },
  ];

  return html`
    <div class="card" style="height: 100%; margin-bottom: 0;">
      <div class="card__header">
        <h3>${t("memory.distribution")}</h3>
      </div>
      <div class="card__body">
        <!-- Type Distribution: stacked bar -->
        <div style="margin-bottom: 1.25rem">
          <div style="font-size:0.82rem; font-weight:600; margin-bottom:0.5rem; color:#495057">${t("memory.typeDistribution")}</div>
          ${typeEntries.length > 0 ? html`
            <div style="display:flex; height:24px; border-radius:6px; overflow:hidden; background:#f0f0f0;">
              ${typeEntries.map(([tp, count]) => {
    const pct = (count / typeTotal) * 100;
    const color = TYPE_COLORS[tp] ?? "#adb5bd";
    return html`<div style="width:${pct}%; background:${color}; min-width:2px;" title="${translateType(tp)}: ${count}"></div>`;
  })}
            </div>
            <div style="display:flex; gap:12px; margin-top:6px; flex-wrap:wrap;">
              ${typeEntries.map(([tp, count]) => {
    const color = TYPE_COLORS[tp] ?? "#adb5bd";
    return html`<span style="display:inline-flex; align-items:center; gap:4px; font-size:0.78rem; color:#495057">
                  <span style="width:10px; height:10px; border-radius:2px; background:${color}; display:inline-block"></span>
                  ${translateType(tp)}:${count}
                </span>`;
  })}
            </div>
          ` : html`<p class="muted" style="font-size:0.82rem">-</p>`}
        </div>

        <!-- Category Distribution: pill badges -->
        <div style="margin-bottom: 1.25rem">
          <div style="font-size:0.82rem; font-weight:600; margin-bottom:0.5rem; color:#495057">${t("memory.categoryDist")}</div>
          <div style="display:flex; gap:6px; flex-wrap:wrap;">
            ${catEntries.length > 0
      ? catEntries.map(([cat, count]) => html`
                <span style="
                  display:inline-flex; align-items:center; gap:4px;
                  padding:2px 10px; border-radius:12px;
                  background:#e9ecef; font-size:0.78rem; color:#495057;
                ">${translateCategory(cat)}:${count}</span>`)
      : html`<span class="muted" style="font-size:0.82rem">-</span>`}
          </div>
        </div>

        <!-- Decay Health: horizontal bars -->
        <div>
          <div style="font-size:0.82rem; font-weight:600; margin-bottom:0.5rem; color:#495057">${t("memory.decayHealthTitle")}</div>
          ${decayBars.map((bar) => html`
            <div style="display:flex; align-items:center; gap:8px; margin-bottom:6px;">
              <span style="width:48px; font-size:0.78rem; color:#495057; text-align:right">${bar.label}</span>
              <div style="flex:1; height:16px; background:#f0f0f0; border-radius:4px; overflow:hidden;">
                <div style="width:${(bar.count / decayTotal) * 100}%; height:100%; background:${bar.color}; border-radius:4px; transition:width 0.3s"></div>
              </div>
              <span style="width:28px; font-size:0.78rem; color:#495057">${bar.count}</span>
            </div>
          `)}
        </div>
      </div>
    </div>
  `;
}

// ── Decay color dot helper ──
function decayDot(decay: number, retentionPolicy: string) {
  if (retentionPolicy === "permanent") {
    return html`<span style="color:${DECAY_COLORS.permanent}" title="permanent">&#9679; ${decay.toFixed(2)}</span>`;
  }
  const color = decay >= 0.5 ? DECAY_COLORS.healthy : decay >= 0.1 ? DECAY_COLORS.fading : DECAY_COLORS.critical;
  return html`<span style="color:${color}">&#9679; ${decay.toFixed(2)}</span>`;
}

// ── Importance mini bar ──
function importanceBar(score: number) {
  const pct = Math.min(score, 1) * 100;
  const color = score >= 0.7 ? "#14b8a6" : score >= 0.4 ? "#ffa726" : "#adb5bd";
  return html`
    <div style="display:flex; align-items:center; gap:6px;">
      <div style="width:48px; height:6px; background:#f0f0f0; border-radius:3px; overflow:hidden;">
        <div style="width:${pct}%; height:100%; background:${color}; border-radius:3px;"></div>
      </div>
      <span style="font-size:0.78rem">${score.toFixed(2)}</span>
    </div>
  `;
}

// ── Header capsules (exported for app-render.ts page-meta) ──
export function renderMemoryTypeCapsules(stats?: MemoryStats | null) {
  if (stats) {
    const entries = Object.entries(stats.byType).sort((a, b) => b[1] - a[1]).slice(0, 3);
    if (entries.length > 0) {
      return html`${entries.map(([tp, count]) => {
        const color = TYPE_COLORS[tp] ?? "#adb5bd";
        return html`<span style="
          display:inline-flex; align-items:center; gap:4px;
          padding:3px 10px; border-radius:6px;
          background:${color}20; border:1px solid ${color}60;
          font-size:0.78rem; color:${color}; font-weight:500;
        ">${translateType(tp)}:${count}</span>`;
      })}`;
    }
  }
  return html`<span style="
    display:inline-flex; align-items:center; gap:4px;
    padding:3px 10px; border-radius:6px;
    background:#f0f0f020; border:1px solid #adb5bd60;
    font-size:0.78rem; color:#adb5bd; font-weight:500;
  ">${t("memory.memoryManagement")}</span>`;
}

// Module-scope debounce timer — persists across Lit re-renders
let _searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

// ── Main render function ──
export function renderMemory(props: MemoryProps) {
  const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize));
  const isSearchActive = !!(props.searchQuery && props.searchResults);

  const handleSearchInput = (e: Event) => {
    const query = (e.target as HTMLInputElement).value.trim();
    if (_searchDebounceTimer) clearTimeout(_searchDebounceTimer);
    if (!query) {
      props.onClearSearch();
      return;
    }
    _searchDebounceTimer = setTimeout(() => props.onSearch(query), 300);
  };

  return html`
    <div class="page memory-page">
      ${props.error
      ? html`<div class="alert alert--error">${props.error}</div>`
      : nothing}

      <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(400px, 1fr)); gap: 24px; margin-bottom: 24px;">
        <!-- Card 1: Status Overview -->
        <div class="card" style="height: 100%; margin-bottom: 0;">
          <div class="card__header">
            <h3>${t("memory.statusOverview")}</h3>
            <div class="card__actions">
              <button class="btn btn--sm" @click=${() => { props.onLoadStatus(); props.onLoadStats(); }}>
                ${t("memory.refresh")}
              </button>
              <button
                class="btn btn--sm btn--primary"
                ?disabled=${props.importing}
                @click=${props.onImportSkills}
              >
                ${props.importing ? t("memory.importing") : t("memory.importSkills")}
              </button>
            </div>
          </div>
          <div class="card__body">
          ${props.status
      ? html`
                <div class="stat-grid">
                  <div class="stat">
                    <span class="stat__label">${t("memory.status")}</span>
                    <span class="stat__value ${props.status.enabled ? "ok" : ""}">
                      ${props.status.enabled ? t("memory.enabled") : t("memory.disabled")}
                    </span>
                  </div>
                  <div class="stat">
                    <span class="stat__label">${t("memory.totalMemories")}</span>
                    <span class="stat__value">${props.status.memoryCount}</span>
                  </div>
                  <div class="stat">
                    <span class="stat__label">${t("memory.diskUsage")}</span>
                    <span class="stat__value">${formatBytes(props.status.diskUsage)}</span>
                  </div>
                  <div class="stat">
                    <span class="stat__label">${t("memory.totalAccess")}</span>
                    <span class="stat__value">${props.stats?.totalAccess ?? "-"}</span>
                  </div>
                  <div class="stat">
                    <span class="stat__label">${t("memory.avgImportance")}</span>
                    <span class="stat__value">${props.stats ? props.stats.avgImportance.toFixed(2) : "-"}</span>
                  </div>
                  <div class="stat">
                    <span class="stat__label">${t("memory.vectorMode")}</span>
                    <span class="stat__value">${props.status.vectorMode || "off"}</span>
                  </div>
                </div>
                <!-- LLM Config Row -->
                <div style="margin-top:0.75rem; display:flex; align-items:center; gap:8px; font-size:0.85rem">
                  <span style="font-weight:600">${t("memory.llmModel")}:</span>
                  ${props.llmConfig
          ? props.llmConfig.provider
            ? html`${props.llmConfig.provider}/${props.llmConfig.model}`
            : html`<span class="muted">${t("memory.llmNotConfigured")}</span>`
          : "-"}
                  <button class="btn btn--xs" @click=${props.onLLMConfigToggle}>${t("memory.llmConfig")}</button>
                </div>
                ${props.llmConfigOpen ? renderLLMConfigPanel(props) : nothing}
              `
      : html`<p class="muted">${t("common.loading")}</p>`}
          ${props.importResult
      ? html`
                <div class="alert alert--info" style="margin-top: 0.75rem">
                  ${t("memory.importResult")
          .replace("{imported}", String(props.importResult.imported))
          .replace("{skipped}", String(props.importResult.skipped))
          .replace("{updated}", String(props.importResult.updated))
          .replace("{failed}", String(props.importResult.failed))}
                </div>
              `
      : nothing}
        </div>
          </div>
        </div>

        <!-- Card 2: Distribution & Health -->
        ${props.stats
      ? renderDistributionCard(props.stats)
      : html`<div class="card" style="height: 100%; margin-bottom: 0; display: flex; align-items: center; justify-content: center;"><p class="muted">${t("common.loading")}</p></div>`
    }
      </div>

      <!-- Card 3: Memory List -->
      <div class="card">
        <div class="card__header" style="flex-wrap: wrap; gap: 12px;">
          <h3>${t("memory.list")} (${props.total})</h3>
          <div class="card__actions" style="flex-wrap: wrap; gap: 8px;">
            <!-- Search bar -->
            <input class="input input--sm" style="width:180px"
              placeholder=${t("memory.searchPlaceholder")}
              .value=${props.searchQuery}
              @input=${handleSearchInput}
            />
            ${isSearchActive ? html`
              <button class="btn btn--sm" @click=${props.onClearSearch}>${t("memory.clearSearch")}</button>
            ` : nothing}
            <select
              class="input input--sm"
              .value=${props.filterType}
              @change=${(e: Event) =>
      props.onFilterType((e.target as HTMLSelectElement).value)}
            >
              <option value="">${t("memory.allTypes")}</option>
              ${MEMORY_TYPES.filter(Boolean).map(
        (tp) => html`<option value=${tp}>${translateType(tp)}</option>`,
      )}
            </select>
            <select
              class="input input--sm"
              .value=${props.filterCategory}
              @change=${(e: Event) =>
      props.onFilterCategory((e.target as HTMLSelectElement).value)}
            >
              <option value="">${t("memory.allCategories")}</option>
              ${MEMORY_CATEGORIES.filter(Boolean).map(
        (cat) => html`<option value=${cat}>${translateCategory(cat)}</option>`,
      )}
            </select>
            <button class="btn btn--sm" @click=${props.onRefresh}>
              ${t("memory.refresh")}
            </button>
          </div>
        </div>
        <div class="card__body">
          ${isSearchActive
      ? renderSearchResults(props)
      : renderMemoryTable(props, totalPages)}
        </div>
      </div>

      <!-- Card 4: Memory Detail -->
      ${props.detail ? renderDetailCard(props) : nothing}
    </div>
  `;
}

// ── Search Results sub-view ──
function renderSearchResults(props: MemoryProps) {
  if (props.searching) return html`<p class="muted">Loading...</p>`;
  const results = props.searchResults;
  if (!results || results.length === 0) return html`<p class="muted">${t("memory.noResults")}</p>`;

  return html`
    <div style="font-size:0.82rem; color:#6c757d; margin-bottom:0.5rem">${t("memory.searchResults")} (${results.length})</div>
    <table class="table">
      <thead>
        <tr>
          <th>${t("memory.content")}</th>
          <th>${t("memory.type")}</th>
          <th>${t("memory.category")}</th>
          <th>${t("memory.searchScore")}</th>
        </tr>
      </thead>
      <tbody>
        ${results.map((r) => html`
          <tr class="table__row--clickable" @click=${() => props.onSelectMemory(r.id, 0)}>
            <td>
              <div style="display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; text-overflow: ellipsis; max-width: 400px;">
                ${r.content}
              </div>
            </td>
            <td><span class="badge" style="background:${TYPE_COLORS[r.type] ?? "#adb5bd"}20; color:${TYPE_COLORS[r.type] ?? "#adb5bd"}; border:1px solid ${TYPE_COLORS[r.type] ?? "#adb5bd"}40">${translateType(r.type)}</span></td>
            <td><span class="badge badge--outline">${translateCategory(r.category)}</span></td>
            <td>${r.score.toFixed(2)}</td>
          </tr>
        `)}
      </tbody>
    </table>
  `;
}

// ── Memory Table sub-view (normal list mode) ──
function renderMemoryTable(props: MemoryProps, totalPages: number) {
  if (props.loading) return html`<p class="muted">Loading...</p>`;
  if (!props.list || props.list.length === 0) return html`<p class="muted">${t("memory.noMemories")}</p>`;

  return html`
    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px;">
      ${props.list.map((mem) => html`
        <div class="card" style="margin-bottom: 0; cursor: pointer; transition: transform 0.2s, box-shadow 0.2s; border: 1px solid #e0e0e0; border-radius: 8px; overflow: hidden;"
             onmouseover="this.style.transform='translateY(-2px)'; this.style.boxShadow='0 4px 12px rgba(0,0,0,0.1)';"
             onmouseout="this.style.transform='translateY(0)'; this.style.boxShadow='none';"
             @click=${() => props.onSelectMemory(mem.id, 0)}>
          <div class="card__body" style="padding: 16px;">
            <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px;">
              <div style="display: flex; gap: 8px; flex-wrap: wrap;">
                <span class="badge" style="background:${TYPE_COLORS[mem.type] ?? "#adb5bd"}20; color:${TYPE_COLORS[mem.type] ?? "#adb5bd"}; border:1px solid ${TYPE_COLORS[mem.type] ?? "#adb5bd"}40">${translateType(mem.type)}</span>
                <span class="badge badge--outline">${translateCategory(mem.category)}</span>
              </div>
              <div style="display: flex; gap: 4px;">
                <button
                  class="btn btn--xs btn--danger"
                  style="padding: 2px 6px; font-size: 0.7rem;"
                  @click=${(e: Event) => {
      e.stopPropagation();
      if (confirm(t("memory.deleteConfirm"))) {
        props.onDeleteMemory(mem.id);
      }
    }}
                >
                  ${t("memory.delete")}
                </button>
              </div>
            </div>
            
            <div style="color: #495057; font-size: 0.95rem; line-height: 1.5; margin-bottom: 16px; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; text-overflow: ellipsis; word-break: break-all;">
              ${mem.content || t("memory.noContent")}
            </div>
            
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px; font-size: 0.8rem; border-top: 1px solid #f0f0f0; padding-top: 12px;">
              <div>
                <div style="color: #6c757d; margin-bottom: 4px; font-size: 0.75rem;">${t("memory.importance")}</div>
                ${importanceBar(mem.importanceScore)}
              </div>
              <div>
                <div style="color: #6c757d; margin-bottom: 4px; font-size: 0.75rem;">${t("memory.decayFactor")}</div>
                ${decayDot(mem.decayFactor, mem.retentionPolicy)}
              </div>
            </div>
            
            <div style="margin-top: 12px; font-size: 0.75rem; color: #adb5bd; text-align: right;">
              ${formatTimestamp(mem.createdAt)}
            </div>
          </div>
        </div>
      `)}
    </div>
    <!-- Pagination -->
    ${totalPages > 1
      ? html`
          <div class="pagination">
            <button
              class="btn btn--sm"
              ?disabled=${props.page === 0}
              @click=${() => props.onPageChange(props.page - 1)}
            >
              ${t("memory.prev")}
            </button>
            <span class="pagination__info">
              ${t("memory.page")
          .replace("{page}", String(props.page + 1))
          .replace("{total}", String(totalPages))}
            </span>
            <button
              class="btn btn--sm"
              ?disabled=${props.page >= totalPages - 1}
              @click=${() => props.onPageChange(props.page + 1)}
            >
              ${t("memory.next")}
            </button>
          </div>
        `
      : nothing}
  `;
}

// ── Detail Card sub-view ──
function renderDetailCard(props: MemoryProps) {
  const d = props.detail!;
  const tierLabels = [t("memory.tierL0"), t("memory.tierL1"), t("memory.tierL2")];

  return html`
    <div style="position: fixed; top: 0; left: 0; width: 100vw; height: 100vh; background: rgba(0,0,0,0.5); z-index: 1000; display: flex; align-items: center; justify-content: center; backdrop-filter: blur(2px);" @click=${props.onCloseDetail}>
      <div class="card" style="width: 90%; max-width: 800px; max-height: 90vh; display: flex; flex-direction: column; box-shadow: 0 10px 30px rgba(0,0,0,0.2); animation: fade-in-up 0.2s ease-out;" @click=${(e: Event) => e.stopPropagation()}>
        <div class="card__header" style="border-bottom: 1px solid #e9ecef; padding-bottom: 12px;">
          <h3 style="margin: 0;">${t("memory.detail")}</h3>
          <div class="card__actions">
            <button class="btn btn--sm" style="background: transparent; border: none; font-size: 1.5rem; line-height: 1; padding: 0 8px; color: #6c757d;" @click=${props.onCloseDetail}>&times;</button>
          </div>
        </div>
        <div class="card__body" style="overflow-y: auto; padding: 20px;">
          <!-- Top: type / category / retention pills + created time -->
          <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-bottom:1rem">
            <span class="badge" style="background:${TYPE_COLORS[d.type] ?? "#adb5bd"}20; color:${TYPE_COLORS[d.type] ?? "#adb5bd"}; border:1px solid ${TYPE_COLORS[d.type] ?? "#adb5bd"}40; font-size: 0.85rem; padding: 4px 10px;">${translateType(d.type)}</span>
            <span class="badge badge--outline" style="font-size: 0.85rem; padding: 4px 10px;">${translateCategory(d.category)}</span>
            <span class="badge badge--outline" style="font-size:0.75rem; padding: 4px 10px; border-color: #dee2e6;">${d.retentionPolicy}</span>
            <span style="margin-left:auto; font-size:0.85rem; color:#6c757d">${formatTimestamp(d.createdAt)}</span>
          </div>

          <!-- Middle: importance + decay + access stats -->
          <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(150px, 1fr)); gap:16px; margin-bottom:20px; background: #f8f9fa; padding: 16px; border-radius: 8px;">
            <div>
              <div style="font-size:0.8rem; font-weight: 500; color:#495057; margin-bottom:8px">${t("memory.importance")}</div>
              ${importanceBar(d.importanceScore)}
            </div>
            <div>
              <div style="font-size:0.8rem; font-weight: 500; color:#495057; margin-bottom:8px">${t("memory.decayFactor")}</div>
              ${decayDot(d.decayFactor, d.retentionPolicy)}
            </div>
            <div>
              <div style="font-size:0.8rem; font-weight: 500; color:#495057; margin-bottom:8px">${t("memory.accessCount")}</div>
              <div style="display: flex; align-items: baseline; gap: 8px;">
                <span style="font-size:1.1rem; font-weight: 600;">${d.accessCount}</span>
                ${d.lastAccessedAt ? html`<span style="font-size:0.75rem; color:#6c757d;">${formatRelativeTimestamp(d.lastAccessedAt * 1000)}</span>` : nothing}
              </div>
            </div>
          </div>

          <!-- Bottom: L0/L1/L2 Tabs -->
          <div class="memory-tier-tabs" style="margin-top: 1rem; border-bottom: 1px solid #dee2e6;">
            ${[0, 1, 2].map(
    (lvl) => html`
                <button
                  class="memory-tier-tab ${props.detailLevel === lvl ? "memory-tier-tab--active" : ""}"
                  type="button"
                  aria-pressed=${props.detailLevel === lvl ? "true" : "false"}
                  @click=${() => props.onDetailLevel(lvl)}
                >
                  ${tierLabels[lvl]}
                </button>
              `,
  )}
          </div>
          <div class="code-block" style="margin-top: 1rem; background: #fafafa; border: 1px solid #eaeaea; border-radius: 6px; padding: 16px; max-height: 400px; overflow-y: auto;">
            <pre style="margin: 0; font-family: var(--mono); font-size: 0.9rem; line-height: 1.6; white-space: pre-wrap; word-wrap: break-word;">${d.vfsContent || t("memory.noContent")}</pre>
          </div>
        </div>
      </div>
      <style>
        @keyframes fade-in-up {
          from { opacity: 0; transform: translateY(20px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .memory-tier-tabs {
          display: inline-flex;
          gap: 6px;
          padding-bottom: 10px;
        }
        .memory-tier-tab {
          appearance: none;
          border: 1px solid #cfd6de;
          background: #f7f9fc;
          color: #334155;
          border-radius: 8px;
          padding: 8px 16px;
          font-size: 0.92rem;
          line-height: 1.25;
          font-weight: 500;
          cursor: pointer;
          transition: all 0.16s ease;
        }
        .memory-tier-tab:hover {
          background: #eef3ff;
          border-color: #b8c5f0;
          color: #1e3a8a;
        }
        .memory-tier-tab:active {
          transform: translateY(1px);
        }
        .memory-tier-tab--active {
          background: #2f6feb;
          border-color: #2f6feb;
          color: #ffffff;
          font-weight: 600;
          box-shadow: 0 0 0 2px rgba(47, 111, 235, 0.15);
        }
      </style>
    </div>
  `;
}
