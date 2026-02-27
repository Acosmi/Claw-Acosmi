import { html, nothing } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { icons } from "../icons.ts";
import { t } from "../i18n.ts";

export function renderNotificationCenter(state: AppViewState) {
  if (!state.notificationsOpen) {
    return nothing;
  }

  // Define format time helper
  const formatTime = (ts: number) => {
    const d = new Date(ts);
    return `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
  };

  return html`
    <div class="notification-center-overlay" @click=${() => {
      state.notificationsOpen = false;
      (state as any).requestUpdate?.();
    }}></div>
    <div class="notification-center-dropdown">
      <div class="notification-header">
        <h3>${t("notifications.title")}</h3>
        ${state.notifications.length > 0
      ? html`
              <button 
                class="clear-all-btn"
                @click=${() => {
          state.notifications = [];
          (state as any).requestUpdate?.();
        }}
              >
                ${t("notifications.clearAll")}
              </button>
            `
      : nothing}
      </div>
      <div class="notification-body">
        ${state.notifications.length === 0
      ? html`
              <div class="notification-empty">
                ${icons.bell}
                <p>${t("notifications.empty")}</p>
              </div>
            `
      : state.notifications.map((n) => html`
              <div class="notification-item ${n.read ? "read" : "unread"} ${n.type}">
                <div class="notification-icon">
                  ${n.type === "error" ? icons.x : icons.check}
                </div>
                <div class="notification-content">
                  <div class="notification-message">${n.message}</div>
                  <div class="notification-time">${formatTime(n.timestamp)}</div>
                </div>
                ${!n.read ? html`<div class="notification-unread-dot"></div>` : nothing}
              </div>
            `)}
      </div>
    </div>
  `;
}
