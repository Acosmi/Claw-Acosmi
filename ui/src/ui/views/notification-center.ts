import { html, nothing } from "lit";
import type { AppViewState } from "../app-view-state.ts";
import { icons } from "../icons.ts";
import { t } from "../i18n.ts";
import { loadChatHistory } from "../controllers/chat.ts";

export function renderNotificationCenter(state: AppViewState) {
  if (!state.notificationsOpen) {
    return nothing;
  }

  // Define format time helper
  const formatTime = (ts: number) => {
    const d = new Date(ts);
    return `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
  };

  const handleNotificationClick = (n: any) => {
    // 标记已读
    n.read = true;
    state.setTab("chat");
    // 如果有 sessionKey，跳转到对应会话
    if (n.sessionKey && n.sessionKey !== state.sessionKey) {
      state.sessionKey = n.sessionKey;
      (state as any).chatMessage = "";
      (state as any).chatStream = null;
      (state as any).chatStreamStartedAt = null;
      // chatRunId 保留：与修复 B 一致，保留活跃 run 追踪
      state.applySettings({
        ...state.settings,
        sessionKey: n.sessionKey,
        lastActiveSessionKey: n.sessionKey,
      });
      void loadChatHistory(state as any);
      // 预填充：从缓存消费该 session 的入站消息（与 applySessionSwitch 逻辑一致）
      const pendingMsgs = (state as any)._pendingChannelMsgs as Record<string, { text: string; ts: number }> | undefined;
      if (pendingMsgs?.[n.sessionKey]) {
        const pending = pendingMsgs[n.sessionKey];
        delete pendingMsgs[n.sessionKey];
        (state as any).chatMessages = [{
          role: "user",
          content: [{ type: "text", text: pending.text }],
          timestamp: pending.ts,
        }];
        (state as any)._skipEmptyHistory = true;
        // 根因 B 修复：远程频道无 delta 流式事件，手动触发思考动画
        if (!(state as any).chatRunId) {
          (state as any).chatRunId = `remote-switch-${Date.now()}`;
          (state as any).chatStream = "";
          (state as any).chatStreamStartedAt = pending.ts;
        }
      }
    }
    state.notificationsOpen = false;
    (state as any).requestUpdate?.();
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
              <div
                class="notification-item ${n.read ? "read" : "unread"} ${n.type}"
                style="cursor: ${n.sessionKey ? "pointer" : "default"}"
                @click=${() => handleNotificationClick(n)}
              >
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
