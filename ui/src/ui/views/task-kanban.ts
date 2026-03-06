import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import type { TaskKanbanItem, TaskKanbanState } from "../controllers/task-kanban.ts";
import { groupByColumn } from "../controllers/task-kanban.ts";

// ---------- Task Kanban View (Apple-Style Redesign) ----------

export type TaskKanbanProps = {
  kanbanState: TaskKanbanState;
  onPrune: () => void;
  onRequestUpdate: () => void;
};

// Module-level UI state (persisted across re-renders)
type FilterTab = "all" | "queued" | "running" | "done";
let _activeFilter: FilterTab = "all";
let _selectedTaskId: string | null = null;

export function renderTaskKanban(props: TaskKanbanProps) {
  const { queued, running, done } = groupByColumn(props.kanbanState);
  const all = [...queued, ...running, ...done];
  const total = all.length;

  // Derive filtered list
  const filtered: TaskKanbanItem[] =
    _activeFilter === "queued" ? queued
      : _activeFilter === "running" ? running
        : _activeFilter === "done" ? done
          : all;

  // Find selected task for detail modal
  const selectedTask = _selectedTaskId
    ? props.kanbanState.tasks.get(_selectedTaskId) ?? null
    : null;

  // If selected task was pruned, clear selection
  if (_selectedTaskId && !selectedTask) {
    _selectedTaskId = null;
  }

  return html`
    <section class="card">
      <div class="tk-header">
        <div>
          <div class="card-title">${t("tasks.title")}</div>
          <div class="card-sub">${t("tasks.subtitle")}</div>
        </div>
        <div class="tk-header__actions">
          ${done.length > 0
      ? html`<button class="btn btn--sm" @click=${props.onPrune}>
                ${t("tasks.prune")}
              </button>`
      : nothing}
        </div>
      </div>

      ${renderSegmentedControl(queued.length, running.length, done.length, total, props)}

      ${total === 0
      ? renderEmptyState()
      : filtered.length === 0
        ? renderEmptyFilterState()
        : html`
          <div class="tk-list">
            ${filtered.map((task) => renderTaskCard(task, props))}
          </div>
        `}
    </section>
    ${selectedTask ? renderDetailModal(selectedTask, props) : nothing}
  `;
}

// ---------- Segmented Control ----------

function renderSegmentedControl(
  queuedCount: number,
  runningCount: number,
  doneCount: number,
  totalCount: number,
  props: TaskKanbanProps,
) {
  const tabs: { key: FilterTab; label: string; count: number }[] = [
    { key: "all", label: t("tasks.filter.all"), count: totalCount },
    { key: "queued", label: t("tasks.col.queued"), count: queuedCount },
    { key: "running", label: t("tasks.col.running"), count: runningCount },
    { key: "done", label: t("tasks.col.done"), count: doneCount },
  ];

  return html`
    <div class="tk-seg">
      ${tabs.map(
    (tab) => html`
          <button
            class="tk-seg__item ${_activeFilter === tab.key ? "tk-seg__item--active" : ""}"
            @click=${() => {
        _activeFilter = tab.key;
        props.onRequestUpdate();
      }}
          >
            ${tab.label}
            ${tab.count > 0
        ? html`<span class="tk-seg__count">${tab.count}</span>`
        : nothing}
          </button>
        `,
  )}
    </div>
  `;
}

// ---------- Task Card ----------

function renderTaskCard(task: TaskKanbanItem, props: TaskKanbanProps) {
  const elapsed = formatElapsed(task);

  return html`
    <div
      class="tk-card"
      @click=${() => {
      _selectedTaskId = task.taskId;
      props.onRequestUpdate();
    }}
    >
      <div class="tk-card__body">
        <div class="tk-card__header">
          ${renderStatusPill(task.status)}
          ${task.async
      ? html`<span class="tk-pill tk-pill--async">async</span>`
      : nothing}
        </div>
        ${task.text
      ? html`<div class="tk-card__text">${truncate(task.text, 120)}</div>`
      : task.summary
        ? html`<div class="tk-card__text">${truncate(task.summary, 120)}</div>`
        : nothing}
        <div class="tk-card__meta">
          ${task.toolName
      ? html`<code>${task.toolName}</code>`
      : nothing}
          ${task.toolName && task.phase
      ? html`<span class="tk-card__meta-sep">·</span>`
      : nothing}
          ${task.phase
      ? html`<span>${task.phase}</span>`
      : nothing}
          ${task.isError
      ? html`<span style="color: var(--danger);">error</span>`
      : nothing}
        </div>
      </div>
      <div class="tk-card__trail">
        ${elapsed ? html`<span>${elapsed}</span>` : nothing}
        <svg class="tk-card__chevron" viewBox="0 0 24 24">
          <polyline points="9 18 15 12 9 6" stroke="currentColor" fill="none" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </div>
    </div>
  `;
}

// ---------- Status Pill ----------

function renderStatusPill(status: string) {
  const config = statusPillConfig(status);
  return html`
    <span class="tk-pill ${config.cls}">
      ${config.icon} ${config.label}
    </span>
  `;
}

function statusPillConfig(status: string): { cls: string; label: string; icon: string } {
  switch (status) {
    case "queued":
      return { cls: "tk-pill--queued", label: t("tasks.col.queued"), icon: "⏳" };
    case "started":
    case "progress":
      return { cls: "tk-pill--running", label: t("tasks.col.running"), icon: "▶" };
    case "completed":
      return { cls: "tk-pill--done", label: t("tasks.col.done"), icon: "✓" };
    case "failed":
      return { cls: "tk-pill--failed", label: t("tasks.status.failed"), icon: "✗" };
    default:
      return { cls: "", label: status, icon: "?" };
  }
}

// ---------- Empty State ----------

function renderEmptyState() {
  return html`
    <div class="tk-empty">
      <div class="tk-empty__icon">
        <svg viewBox="0 0 24 24">
          <path d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2"/>
          <rect x="9" y="3" width="6" height="4" rx="1"/>
          <path d="M9 14l2 2 4-4"/>
        </svg>
      </div>
      <div class="tk-empty__text">${t("tasks.empty")}</div>
    </div>
  `;
}

function renderEmptyFilterState() {
  return html`
    <div class="tk-empty">
      <div class="tk-empty__icon">
        <svg viewBox="0 0 24 24">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
      </div>
      <div class="tk-empty__text">${t("tasks.filter.empty")}</div>
    </div>
  `;
}

// ---------- Detail Modal ----------

function renderDetailModal(task: TaskKanbanItem, props: TaskKanbanProps) {
  const closeModal = () => {
    _selectedTaskId = null;
    props.onRequestUpdate();
  };

  return html`
    <div class="tk-overlay" @click=${(e: Event) => {
      if ((e.target as HTMLElement).classList.contains("tk-overlay")) {
        closeModal();
      }
    }}>
      <div class="tk-modal" @click=${(e: Event) => e.stopPropagation()}>
        <div class="tk-modal__header">
          <span class="tk-modal__title">${t("tasks.detail.title")}</span>
          <button class="tk-modal__close" @click=${closeModal}>
            <svg viewBox="0 0 24 24">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>

        <div class="tk-detail">
          <!-- Status -->
          <div class="tk-detail__row">
            <span class="tk-detail__label">${t("tasks.detail.status")}</span>
            <span class="tk-detail__value">${renderStatusPill(task.status)}</span>
          </div>

          <!-- Task Text -->
          ${task.text
      ? html`
              <div class="tk-detail__block">
                <div class="tk-detail__block-label">${t("tasks.detail.task")}</div>
                <div class="tk-detail__block-text">${task.text}</div>
              </div>
            `
      : nothing}

          <!-- Tool -->
          ${task.toolName
      ? html`
              <div class="tk-detail__row">
                <span class="tk-detail__label">${t("tasks.detail.tool")}</span>
                <span class="tk-detail__value"><code>${task.toolName}</code></span>
              </div>
            `
      : nothing}

          <!-- Phase -->
          ${task.phase
      ? html`
              <div class="tk-detail__row">
                <span class="tk-detail__label">${t("tasks.detail.phase")}</span>
                <span class="tk-detail__value">${task.phase}</span>
              </div>
            `
      : nothing}

          <!-- Recent Step -->
          ${task.progressText
      ? html`
              <div class="tk-detail__block">
                <div class="tk-detail__block-label">${t("tasks.detail.progress")}</div>
                <div class="tk-detail__block-text">${task.progressText}</div>
              </div>
            `
      : nothing}

          <!-- Summary -->
          ${task.summary
      ? html`
              <div class="tk-detail__block">
                <div class="tk-detail__block-label">${t("tasks.detail.summary")}</div>
                <div class="tk-detail__block-text">${task.summary}</div>
              </div>
            `
      : nothing}

          <!-- Error -->
          ${task.error
      ? html`
              <div class="tk-detail__block">
                <div class="tk-detail__block-label">${t("tasks.detail.error")}</div>
                <div class="tk-detail__block-text tk-detail__value--error">${task.error}</div>
              </div>
            `
      : nothing}

          <!-- Timeline -->
          ${task.queuedAt
      ? html`
              <div class="tk-detail__row">
                <span class="tk-detail__label">${t("tasks.detail.queued")}</span>
                <span class="tk-detail__value">${formatTime(task.queuedAt)}</span>
              </div>
            `
      : nothing}

          ${task.startedAt
      ? html`
              <div class="tk-detail__row">
                <span class="tk-detail__label">${t("tasks.detail.started")}</span>
                <span class="tk-detail__value">${formatTime(task.startedAt)}</span>
              </div>
            `
      : nothing}

          ${task.completedAt
      ? html`
              <div class="tk-detail__row">
                <span class="tk-detail__label">${t("tasks.detail.completed")}</span>
                <span class="tk-detail__value">${formatTime(task.completedAt)}</span>
              </div>
            `
      : nothing}

          <!-- Duration -->
          ${task.startedAt && task.completedAt
      ? html`
              <div class="tk-detail__row">
                <span class="tk-detail__label">${t("tasks.detail.duration")}</span>
                <span class="tk-detail__value">${formatDuration(task.completedAt - task.startedAt)}</span>
              </div>
            `
      : nothing}
        </div>
      </div>
    </div>
  `;
}

// ---------- Helpers ----------

function formatElapsed(task: TaskKanbanItem): string {
  if (task.status === "completed" || task.status === "failed") {
    if (task.startedAt && task.completedAt) {
      const ms = task.completedAt - task.startedAt;
      return formatDuration(ms);
    }
    return "";
  }
  if (task.startedAt) {
    const ms = Date.now() - task.startedAt;
    return `${formatDuration(ms)}…`;
  }
  return "";
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remainSecs = secs % 60;
  return `${mins}m${remainSecs > 0 ? ` ${remainSecs}s` : ""}`;
}

function formatTime(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + "…";
}
