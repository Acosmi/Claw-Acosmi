import { html, nothing } from "lit";
import type { AppViewState } from "../app-view-state.ts";

// ---------- Coder 确认弹窗 ----------

function formatRemaining(ms: number): string {
  const remaining = Math.max(0, ms);
  const totalSeconds = Math.floor(remaining / 1000);
  if (totalSeconds < 60) return `${totalSeconds}s`;
  const minutes = Math.floor(totalSeconds / 60);
  return `${minutes}m`;
}

export function renderCoderConfirmPrompt(state: AppViewState) {
  const queue = state.coderConfirmQueue ?? [];
  const active = queue[0];
  if (!active) return nothing;

  const remainingMs = active.expiresAtMs - Date.now();
  const remaining = remainingMs > 0 ? `${formatRemaining(remainingMs)}` : "expired";
  const queueCount = queue.length;
  const preview = active.preview;
  const toolLabel =
    active.toolName === "edit"
      ? "Coder Edit"
      : active.toolName === "write"
        ? "Coder Write"
        : active.toolName === "send_media"
          ? "Data Export"
        : "Coder Bash";

  return html`
    <div class="coder-confirm-card" role="dialog" aria-live="polite">
      <div class="coder-confirm-header">
        <span>${toolLabel} — confirmation needed</span>
        ${queueCount > 1 ? html`<span class="muted">(${queueCount} pending)</span>` : nothing}
      </div>

      <div class="coder-confirm-preview">
        ${active.toolName === "edit" ? renderEditPreview(preview) : nothing}
        ${active.toolName === "write" ? renderWritePreview(preview) : nothing}
        ${active.toolName === "bash" ? renderBashPreview(preview) : nothing}
        ${active.toolName === "send_media" ? renderSendMediaPreview(preview) : nothing}
      </div>

      <div class="coder-confirm-actions">
        <button
          class="coder-confirm-btn-allow"
          @click=${() => state.handleCoderConfirmDecision(active.id, "allow")}
        >
          Allow
        </button>
        <button
          class="coder-confirm-btn-deny"
          @click=${() => state.handleCoderConfirmDecision(active.id, "deny")}
        >
          Deny
        </button>
        <span class="coder-confirm-timer">${remaining}</span>
      </div>
    </div>
  `;
}

function renderEditPreview(preview: AppViewState["coderConfirmQueue"][0]["preview"]) {
  return html`
    ${preview.filePath ? html`<div class="coder-file-header mono">${preview.filePath}</div>` : nothing}
    <div class="coder-diff-preview mono">
      ${preview.oldString
        ? html`${preview.oldString.split("\n").slice(0, 3).map((line) => html`<div class="coder-diff-del">- ${line}</div>`)}`
        : nothing}
      ${preview.newString
        ? html`${preview.newString.split("\n").slice(0, 3).map((line) => html`<div class="coder-diff-add">+ ${line}</div>`)}`
        : nothing}
    </div>
  `;
}

function renderWritePreview(preview: AppViewState["coderConfirmQueue"][0]["preview"]) {
  return html`
    ${preview.filePath ? html`<div class="coder-file-header mono">${preview.filePath}</div>` : nothing}
    ${preview.lineCount ? html`<div class="muted">${preview.lineCount} lines</div>` : nothing}
    ${preview.content
      ? html`<div class="coder-command-mono mono">${preview.content.length > 200 ? preview.content.slice(0, 200) + "..." : preview.content}</div>`
      : nothing}
  `;
}

function renderBashPreview(preview: AppViewState["coderConfirmQueue"][0]["preview"]) {
  return html`
    ${preview.command ? html`<div class="coder-command-mono mono">${preview.command}</div>` : nothing}
  `;
}

function renderSendMediaPreview(preview: AppViewState["coderConfirmQueue"][0]["preview"]) {
  return html`
    ${preview.filePath ? html`<div class="coder-file-header mono">${preview.filePath}</div>` : nothing}
    ${preview.command ? html`<div class="coder-command-mono mono">${preview.command}</div>` : nothing}
    ${preview.content ? html`<div class="coder-command-mono mono">${preview.content}</div>` : nothing}
  `;
}
