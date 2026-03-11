// permission-popup.ts — 权限审批弹窗组件
// 当 AI 工具调用被权限拒绝时，在聊天窗口弹出审批弹窗。
// 参考: Claude Code 3 层规则 + GitHub Copilot 3 级审批

import { html, nothing } from "lit";
import { t } from "../i18n.ts";

/** 权限拒绝事件数据 */
export interface PermissionDeniedMountRequest {
    hostPath: string;
    mountMode: string;
}

export interface PermissionDeniedEvent {
    tool: string;
    detail: string;
    level: string;
    requestedLevel?: string;
    approvalType?: string;
    mountRequests?: PermissionDeniedMountRequest[];
    runId?: string;
}

/** 弹窗操作回调 */
export interface PermissionPopupCallbacks {
    /** 本次放行 */
    onAllowOnce: (event: PermissionDeniedEvent) => void;
    /** 临时授权（会话级） */
    onAllowSession: (event: PermissionDeniedEvent) => void;
    /** 永久授权（修改配置） */
    onAllowPermanent: (event: PermissionDeniedEvent) => void;
    /** 拒绝 */
    onDeny: () => void;
}

type PopupState = {
    event: PermissionDeniedEvent;
    showConfirm: boolean;
    confirmInput: string;
};

let _state: PopupState | null = null;

/** 显示权限弹窗 */
export function showPermissionPopup(event: PermissionDeniedEvent): void {
    _state = { event, showConfirm: false, confirmInput: "" };
}

/** 隐藏权限弹窗 */
export function hidePermissionPopup(): void {
    _state = null;
}

/** 获取工具描述 */
function toolLabel(tool: string): string {
    switch (tool) {
        case "bash":
            return "bash — " + t("permission.popup.toolBash");
        case "write_file":
            return "write_file — " + t("permission.popup.toolWriteFile");
        case "send_media":
            return "send_media — Data Export";
        default:
            return tool;
    }
}

/** 获取安全级别描述 */
function levelLabel(level: string): string {
    switch (level) {
        case "deny":
            return "L0 " + t("permission.popup.levelDeny");
        case "allowlist":
            return "L1 " + t("permission.popup.levelAllowlist");
        case "sandboxed":
            return "L2 " + t("permission.popup.levelSandboxed");
        case "full":
            return "L3 " + t("permission.popup.levelFull");
        default:
            return level;
    }
}

function requestedApprovalLabel(event: PermissionDeniedEvent): string {
    if (event.approvalType === "mount_access") {
        const mount = event.mountRequests?.[0];
        const mode = (mount?.mountMode ?? "ro").toUpperCase();
        const target = mount?.hostPath || event.detail;
        return `Mount ${mode} — ${target}`;
    }
    return levelLabel(event.requestedLevel ?? event.level);
}

function renderMetaRow(label: string, value?: string | null) {
    if (!value) {
        return nothing;
    }
    return html`<div class="exec-approval-meta-row"><span>${label}</span><span>${value}</span></div>`;
}

/** 渲染权限弹窗 */
export function renderPermissionPopup(
    callbacks: PermissionPopupCallbacks,
    requestUpdate: () => void,
) {
    if (!_state) {
        return nothing;
    }

    const state = _state;
    const ev = state.event;
    const isMountAccess = ev.approvalType === "mount_access";
    const allowPermanent = ev.approvalType !== "mount_access" && (ev.requestedLevel ?? ev.level) === "full";
    const permanentOnly = allowPermanent;

    const handleAllowOnce = () => {
        callbacks.onAllowOnce(ev);
        hidePermissionPopup();
        requestUpdate();
    };

    const handleAllowSession = () => {
        callbacks.onAllowSession(ev);
        hidePermissionPopup();
        requestUpdate();
    };

    const handleShowConfirm = () => {
        state.showConfirm = true;
        state.confirmInput = "";
        requestUpdate();
    };

    const handleConfirmPermanent = () => {
        if (state.confirmInput.trim().toUpperCase() !== "CONFIRM") {
            return;
        }
        callbacks.onAllowPermanent(ev);
        hidePermissionPopup();
        requestUpdate();
    };

    const handleDeny = () => {
        callbacks.onDeny();
        hidePermissionPopup();
        requestUpdate();
    };

    const handleOverlayClick = (e: Event) => {
        if ((e.target as HTMLElement).classList.contains("exec-approval-overlay")) {
            handleDeny();
        }
    };

    return html`
    <div class="exec-approval-overlay" role="alertdialog" aria-modal="true" aria-live="polite" @click=${handleOverlayClick}>
      <div class="exec-approval-card" @click=${(e: Event) => e.stopPropagation()}>
        <div class="exec-approval-header">
          <div>
            <div class="exec-approval-title">${t("permission.popup.title")}</div>
            <div class="exec-approval-sub">${requestedApprovalLabel(ev)}</div>
          </div>
        </div>
        <div class="exec-approval-command mono">${ev.detail}</div>
        <div class="exec-approval-meta">
          ${renderMetaRow(t("permission.popup.tool"), toolLabel(ev.tool))}
          ${renderMetaRow(t("permission.popup.target"), ev.detail)}
          ${renderMetaRow(t("permission.popup.level"), requestedApprovalLabel(ev))}
        </div>

        ${state.showConfirm
            ? html`
            <div class="exec-approval-confirm">
              <div class="callout danger">${t("permission.popup.permanentWarn")}</div>
              <label class="field">
                <span>${t("permission.popup.typeConfirm")}</span>
                <input
                  class="exec-approval-confirm-input"
                  type="text"
                  placeholder="CONFIRM"
                  .value=${state.confirmInput}
                  @input=${(e: Event) => {
                      state.confirmInput = (e.target as HTMLInputElement).value;
                      requestUpdate();
                  }}
                  @keydown=${(e: KeyboardEvent) => {
                      if (e.key === "Enter") {
                          handleConfirmPermanent();
                      }
                  }}
                />
              </label>
            </div>
          `
            : nothing}

        <div class="exec-approval-actions">
          ${state.showConfirm
            ? html`
                <button
                  class="btn danger"
                  ?disabled=${state.confirmInput.trim().toUpperCase() !== "CONFIRM"}
                  @click=${handleConfirmPermanent}
                >
                  ${t("permission.popup.confirmPermanent")}
                </button>
                <button
                  class="btn"
                  @click=${() => {
                      state.showConfirm = false;
                      requestUpdate();
                  }}
                >
                  ${t("permission.popup.cancel")}
                </button>
              `
            : html`
              ${permanentOnly ? nothing : html`
                <button
                  class="btn primary"
                  @click=${handleAllowOnce}
                >
                  ${isMountAccess ? t("permission.popup.allowTaskMount") : t("permission.popup.allowOnce")}
                </button>
                ${isMountAccess ? nothing : html`
                  <button
                    class="btn"
                    @click=${handleAllowSession}
                  >
                    ${t("permission.popup.allowSession")}
                  </button>
                `}
              `}
              ${allowPermanent ? html`
                <button
                  class="btn"
                  @click=${handleShowConfirm}
                >
                  ${t("permission.popup.allowPermanent")}
                </button>
              ` : nothing}
              <button
                class="btn danger"
                @click=${handleDeny}
              >
                ${t("permission.popup.deny")}
              </button>
            `}
        </div>
      </div>
    </div>
  `;
}
