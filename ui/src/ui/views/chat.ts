import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import { ref } from "lit/directives/ref.js";
import { repeat } from "lit/directives/repeat.js";
import type { SessionsListResult } from "../types.ts";
import type { ChatItem, MessageGroup } from "../types/chat-types.ts";
import type { AttachmentCategory, ChatAttachment, ChatQueueItem } from "../ui-types.ts";
import {
  renderMessageGroup,
  renderReadingIndicatorGroup,
  renderStreamingGroup,
  renderProcessingCard,
} from "../chat/grouped-render.ts";
import { normalizeMessage, normalizeRoleForGrouping } from "../chat/message-normalizer.ts";
import { initCodeBlockCopyListeners } from "../chat/code-block-actions.ts";
import { icons } from "../icons.ts";
import { renderMarkdownSidebar } from "./markdown-sidebar.ts";
import { renderPermissionPopup, type PermissionPopupCallbacks } from "./permission-popup.ts";
import "../components/resizable-divider.ts";

export type CompactionIndicatorStatus = {
  active: boolean;
  startedAt: number | null;
  completedAt: number | null;
};

export type ChatProps = {
  sessionKey: string;
  onSessionKeyChange: (next: string) => void;
  thinkingLevel: string | null;
  showThinking: boolean;
  loading: boolean;
  sending: boolean;
  canAbort?: boolean;
  compactionStatus?: CompactionIndicatorStatus | null;
  messages: unknown[];
  toolMessages: unknown[];
  stream: string | null;
  streamStartedAt: number | null;
  assistantAvatarUrl?: string | null;
  draft: string;
  queue: ChatQueueItem[];
  connected: boolean;
  canSend: boolean;
  disabledReason: string | null;
  error: string | null;
  sessions: SessionsListResult | null;
  // Focus mode
  focusMode: boolean;
  // Sidebar state
  sidebarOpen?: boolean;
  sidebarContent?: string | null;
  sidebarError?: string | null;
  splitRatio?: number;
  assistantName: string;
  assistantAvatar: string | null;
  // Image attachments
  attachments?: ChatAttachment[];
  onAttachmentsChange?: (attachments: ChatAttachment[]) => void;
  // Voice recording
  voiceRecording?: boolean;
  voiceRecordingDuration?: number;
  onVoiceStart?: () => void;
  onVoiceStop?: () => void;
  voiceSupported?: boolean;
  // Scroll control
  showNewMessages?: boolean;
  onScrollToBottom?: () => void;
  // Event handlers
  onRefresh: () => void;
  onToggleFocusMode: () => void;
  onDraftChange: (next: string) => void;
  onSend: () => void;
  onAbort?: () => void;
  onQueueRemove: (id: string) => void;
  onNewSession: () => void;
  onOpenSidebar?: (content: string) => void;
  onCloseSidebar?: () => void;
  onSplitRatioChange?: (ratio: number) => void;
  onChatScroll?: (event: Event) => void;
  // Permission popup
  permissionPopupCallbacks?: PermissionPopupCallbacks;
  requestUpdate?: () => void;
  onDismissError?: () => void;
};

const _codeBlockInitialized = new WeakSet<HTMLElement>();
const COMPACTION_TOAST_DURATION_MS = 5000;

// Close "+" menu on click outside
let _menuCloseInstalled = false;
function installMenuCloseListener() {
  if (_menuCloseInstalled) return;
  _menuCloseInstalled = true;
  document.addEventListener("click", (e) => {
    const openMenu = document.querySelector(".chat-compose__menu--open");
    if (!openMenu) return;
    const target = e.target as HTMLElement;
    if (target.closest(".chat-compose__menu") || target.closest(".chat-compose__plus")) return;
    openMenu.classList.remove("chat-compose__menu--open");
  });
}

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

function adjustTextareaHeight(el: HTMLTextAreaElement) {
  el.style.height = "auto";
  el.style.height = `${el.scrollHeight}px`;
}

function renderCompactionIndicator(status: CompactionIndicatorStatus | null | undefined) {
  if (!status) {
    return nothing;
  }

  // Show "compacting..." while active
  if (status.active) {
    return html`
      <div class="compaction-indicator compaction-indicator--active" role="status" aria-live="polite">
        ${icons.loader} ${t("chat.compacting")}
      </div>
    `;
  }

  // Show "compaction complete" briefly after completion
  if (status.completedAt) {
    const elapsed = Date.now() - status.completedAt;
    if (elapsed < COMPACTION_TOAST_DURATION_MS) {
      return html`
        <div class="compaction-indicator compaction-indicator--complete" role="status" aria-live="polite">
          ${icons.check} ${t("chat.compacted")}
        </div>
      `;
    }
  }

  return nothing;
}

function generateAttachmentId(): string {
  return `att-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}

/** Infer attachment category from mimeType. Defaults to "image" for backward compat. */
export function inferCategory(mimeType: string): AttachmentCategory {
  if (mimeType.startsWith("image/")) return "image";
  if (mimeType.startsWith("audio/")) return "audio";
  if (mimeType.startsWith("video/")) return "video";
  return "document";
}

/** Format bytes to human-readable string */
function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Maximum file size for attachments (10 MB) */
const MAX_ATTACHMENT_SIZE = 10 * 1024 * 1024;

/** Read a File into a ChatAttachment and append it to the list */
function addFileAsAttachment(file: File, props: ChatProps) {
  if (file.size > MAX_ATTACHMENT_SIZE) {
    // silently skip oversized files (UI could show a toast in the future)
    return;
  }
  const reader = new FileReader();
  reader.addEventListener("load", () => {
    const dataUrl = reader.result as string;
    const newAttachment: ChatAttachment = {
      id: generateAttachmentId(),
      dataUrl,
      mimeType: file.type || "application/octet-stream",
      category: inferCategory(file.type || "application/octet-stream"),
      fileName: file.name,
      fileSize: file.size,
    };
    const current = props.attachments ?? [];
    props.onAttachmentsChange?.([...current, newAttachment]);
  });
  reader.readAsDataURL(file);
}

function handlePaste(e: ClipboardEvent, props: ChatProps) {
  const items = e.clipboardData?.items;
  if (!items || !props.onAttachmentsChange) {
    return;
  }

  const fileItems: DataTransferItem[] = [];
  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    if (item.kind === "file") {
      fileItems.push(item);
    }
  }

  if (fileItems.length === 0) {
    return;
  }

  e.preventDefault();

  for (const item of fileItems) {
    const file = item.getAsFile();
    if (!file) {
      continue;
    }
    addFileAsAttachment(file, props);
  }
}

function handleFileDrop(e: DragEvent, props: ChatProps) {
  e.preventDefault();
  e.stopPropagation();
  if (!props.onAttachmentsChange) return;
  const files = e.dataTransfer?.files;
  if (!files) return;
  for (let i = 0; i < files.length; i++) {
    addFileAsAttachment(files[i], props);
  }
}

function handleFileSelect(e: Event, props: ChatProps) {
  const input = e.target as HTMLInputElement;
  if (!input.files || !props.onAttachmentsChange) return;
  for (let i = 0; i < input.files.length; i++) {
    addFileAsAttachment(input.files[i], props);
  }
  input.value = ""; // reset so same file can be selected again
}

function renderSingleAttachmentPreview(att: ChatAttachment) {
  const category = att.category ?? inferCategory(att.mimeType);
  if (category === "image") {
    return html`
      <img
        src=${att.dataUrl}
        alt="${att.fileName ?? t("chat.attachmentPreview")}"
        class="chat-attachment__img"
      />
    `;
  }
  if (category === "audio") {
    return html`
      <div class="chat-attachment__meta">
        <span class="chat-attachment__icon" aria-hidden="true">🎤</span>
        <span class="chat-attachment__name">${att.fileName ?? "audio"}</span>
        ${att.fileSize ? html`<span class="chat-attachment__size">${formatFileSize(att.fileSize)}</span>` : nothing}
      </div>
    `;
  }
  // document / video / other
  return html`
    <div class="chat-attachment__meta">
      <span class="chat-attachment__icon" aria-hidden="true">${icons.fileText}</span>
      <span class="chat-attachment__name">${att.fileName ?? "file"}</span>
      ${att.fileSize ? html`<span class="chat-attachment__size">${formatFileSize(att.fileSize)}</span>` : nothing}
    </div>
  `;
}

function renderAttachmentPreview(props: ChatProps) {
  const attachments = props.attachments ?? [];
  if (attachments.length === 0) {
    return nothing;
  }

  return html`
    <div class="chat-attachments">
      ${attachments.map(
    (att) => {
      const category = att.category ?? inferCategory(att.mimeType);
      return html`
          <div class="chat-attachment chat-attachment--${category}">
            ${renderSingleAttachmentPreview(att)}
            <button
              class="chat-attachment__remove"
              type="button"
              aria-label="${t("chat.removeAttachment")}"
              @click=${() => {
          const next = (props.attachments ?? []).filter((a) => a.id !== att.id);
          props.onAttachmentsChange?.(next);
        }}
            >
              ${icons.x}
            </button>
          </div>
        `;
    },
  )}
    </div>
  `;
}

export function renderChat(props: ChatProps) {
  installMenuCloseListener();
  const canCompose = props.connected;
  const isBusy = props.sending || props.stream !== null;
  const canAbort = Boolean(props.canAbort && props.onAbort);
  const activeSession = props.sessions?.sessions?.find((row) => row.key === props.sessionKey);
  const reasoningLevel = activeSession?.reasoningLevel ?? "off";
  const showReasoning = props.showThinking && reasoningLevel !== "off";
  const assistantIdentity = {
    name: props.assistantName,
    avatar: props.assistantAvatar ?? props.assistantAvatarUrl ?? null,
  };

  const hasAttachments = (props.attachments?.length ?? 0) > 0;
  const composePlaceholder = props.connected
    ? hasAttachments
      ? t("chat.placeholderImages")
      : t("chat.placeholderDefault")
    : t("chat.placeholderDisconnected");

  const splitRatio = props.splitRatio ?? 0.6;
  const sidebarOpen = Boolean(props.sidebarOpen && props.onCloseSidebar);
  const thread = html`
    <div
      class="chat-thread"
      role="log"
      aria-live="polite"
      @scroll=${props.onChatScroll}
      ${ref((el) => {
    if (el && !_codeBlockInitialized.has(el as HTMLElement)) {
      _codeBlockInitialized.add(el as HTMLElement);
      initCodeBlockCopyListeners(el as HTMLElement);
    }
  })}
    >
      ${props.loading
      ? html`
              <div class="muted">${t("chat.loadingChat")}</div>
            `
      : nothing
    }
      ${repeat(
      buildChatItems(props),
      (item) => item.key,
      (item) => {
        if (item.kind === "divider") {
          return html`
              <div class="chat-divider" role="separator" data-ts=${String(item.timestamp)}>
                <span class="chat-divider__line"></span>
                <span class="chat-divider__label">${item.label}</span>
                <span class="chat-divider__line"></span>
              </div>
            `;
        }

        if (item.kind === "reading-indicator") {
          return renderProcessingCard(
            null,
            props.streamStartedAt ?? Date.now(),
            props.onOpenSidebar,
            assistantIdentity,
          );
        }

        if (item.kind === "stream") {
          return renderProcessingCard(
            item.text,
            item.startedAt,
            props.onOpenSidebar,
            assistantIdentity,
          );
        }

        if (item.kind === "group") {
          return renderMessageGroup(item, {
            onOpenSidebar: props.onOpenSidebar,
            showReasoning,
            assistantName: props.assistantName,
            assistantAvatar: assistantIdentity.avatar,
          });
        }

        return nothing;
      },
    )}
    </div>
  `;

  return html`
    <section class="card chat">
      ${props.disabledReason ? html`<div class="callout">${props.disabledReason}</div>` : nothing}

      ${props.focusMode
      ? html`
            <button
              class="chat-focus-exit"
              type="button"
              @click=${props.onToggleFocusMode}
              aria-label="${t("chat.exitFocusMode")}"
              title="${t("chat.exitFocusMode")}"
            >
              ${icons.x}
            </button>
          `
      : nothing
    }

      <div
        class="chat-split-container ${sidebarOpen ? "chat-split-container--open" : ""}"
      >
        <div
          class="chat-main"
          style="flex: ${sidebarOpen ? `0 0 ${splitRatio * 100}%` : "1 1 100%"}"
        >
          ${thread}
        </div>

        ${sidebarOpen
      ? html`
              <resizable-divider
                .splitRatio=${splitRatio}
                @resize=${(e: CustomEvent) => props.onSplitRatioChange?.(e.detail.splitRatio)}
              ></resizable-divider>
              <div class="chat-sidebar">
                ${renderMarkdownSidebar({
        content: props.sidebarContent ?? null,
        error: props.sidebarError ?? null,
        onClose: props.onCloseSidebar!,
        onViewRawText: () => {
          if (!props.sidebarContent || !props.onOpenSidebar) {
            return;
          }
          props.onOpenSidebar(`\`\`\`\n${props.sidebarContent}\n\`\`\``);
        },
      })}
              </div>
            `
      : nothing
    }
      </div>

      ${props.queue.length
      ? html`
            <div class="chat-queue" role="status" aria-live="polite">
              <div class="chat-queue__title">${t("chat.queued", { n: props.queue.length })}</div>
              <div class="chat-queue__list">
                ${props.queue.map(
        (item) => html`
                    <div class="chat-queue__item">
                      <div class="chat-queue__text">
                        ${item.text ||
          (item.attachments?.length ? t("chat.image", { n: item.attachments.length }) : "")
          }
                      </div>
                      <button
                        class="btn chat-queue__remove"
                        type="button"
                        aria-label="${t("chat.removeQueued")}"
                        @click=${() => props.onQueueRemove(item.id)}
                      >
                        ${icons.x}
                      </button>
                    </div>
                  `,
      )}
              </div>
            </div>
          `
      : nothing
    }

      ${renderCompactionIndicator(props.compactionStatus)}

      ${props.showNewMessages
      ? html`
            <button
              class="btn chat-new-messages"
              type="button"
              @click=${props.onScrollToBottom}
            >
              ${t("chat.newMessages")} ${icons.arrowDown}
            </button>
          `
      : nothing
    }

      <div
        class="chat-compose chat-compose--gemini"
        @dragover=${(e: DragEvent) => { e.preventDefault(); e.currentTarget && (e.currentTarget as HTMLElement).classList.add("chat-compose--dragover"); }}
        @dragleave=${(e: DragEvent) => { (e.currentTarget as HTMLElement)?.classList.remove("chat-compose--dragover"); }}
        @drop=${(e: DragEvent) => { (e.currentTarget as HTMLElement)?.classList.remove("chat-compose--dragover"); handleFileDrop(e, props); }}
      >
        <div class="chat-compose__pill">
          <div class="chat-compose__top">
            ${renderAttachmentPreview(props)}
            <textarea
              class="chat-compose__textarea"
              ${ref((el) => el && adjustTextareaHeight(el as HTMLTextAreaElement))}
              .value=${props.draft}
              ?disabled=${!props.connected}
              @keydown=${(e: KeyboardEvent) => {
      if (e.key !== "Enter") {
        return;
      }
      if (e.isComposing || e.keyCode === 229) {
        return;
      }
      if (e.shiftKey) {
        return;
      }
      if (!props.connected) {
        return;
      }
      e.preventDefault();
      if (canCompose) {
        props.onSend();
      }
    }}
              @input=${(e: Event) => {
      const target = e.target as HTMLTextAreaElement;
      adjustTextareaHeight(target);
      props.onDraftChange(target.value);
    }}
              @paste=${(e: ClipboardEvent) => handlePaste(e, props)}
              placeholder=${composePlaceholder}
              rows="1"
            ></textarea>
          </div>

          <div class="chat-compose__bottom-row">
            <div class="chat-compose__left">
              <button
                class="chat-compose__action-btn"
                type="button"
                title="${t("chat.attach")}"
                aria-label="${t("chat.attach")}"
                ?disabled=${!props.connected}
                @click=${() => {
      const input = document.createElement("input");
      input.type = "file";
      input.multiple = true;
      input.accept = "image/*,audio/*,.pdf,.doc,.docx,.txt,.md,.csv,.xlsx,.xls,.pptx,.ppt,.json,.xml,.zip";
      input.addEventListener("change", (ev) => handleFileSelect(ev, props));
      input.click();
    }}
              >
                ${icons.paperclip}
              </button>
              <button
                class="chat-compose__action-btn"
                type="button"
                title="Image"
                aria-label="Attach image"
                ?disabled=${!props.connected}
                @click=${() => {
      const input = document.createElement("input");
      input.type = "file";
      input.multiple = true;
      input.accept = "image/*";
      input.addEventListener("change", (ev) => handleFileSelect(ev, props));
      input.click();
    }}
              >
                ${icons.image}
              </button>
            </div>

            <div class="chat-compose__right">
              ${props.voiceRecording
      ? html`<span class="chat-compose__voice-indicator">
                    <span class="voice-timer">${formatDuration(props.voiceRecordingDuration ?? 0)}</span>
                  </span>`
      : nothing}
              ${props.voiceSupported && !props.voiceRecording
      ? html`<button
                    class="chat-compose__action-btn ${props.voiceRecording ? "chat-compose__voice--active" : ""}"
                    type="button"
                    title="${t("voice.record")}"
                    aria-label="${t("voice.record")}"
                    ?disabled=${!props.connected}
                    @click=${() => props.onVoiceStart?.()}
                  >
                    ${icons.mic}
                  </button>`
      : nothing}
              ${props.draft.trim()
      ? html`<button
                    class="chat-compose__send"
                    type="button"
                    title="${isBusy ? t("chat.queue") : t("chat.send")}"
                    aria-label="${isBusy ? t("chat.queue") : t("chat.send")}"
                    ?disabled=${!props.connected}
                    @click=${props.onSend}
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M8 13.5V2.5M8 2.5L3 7.5M8 2.5L13 7.5"/>
                    </svg>
                  </button>`
      : nothing}
              <button
                class="chat-compose__new-session"
                type="button"
                title="${t("chat.newSession")}"
                aria-label="${t("chat.newSession")}"
                ?disabled=${!props.connected}
                @click=${() => {
      const tooltip = document.querySelector(".chat-compose__new-tooltip");
      if (tooltip) {
        tooltip.remove();
        props.onNewSession();
        return;
      }
      const btn = document.querySelector(".chat-compose__new-session") as HTMLElement;
      if (!btn) { props.onNewSession(); return; }
      const tip = document.createElement("div");
      tip.className = "chat-compose__new-tooltip";
      tip.textContent = "Click again to start new session";
      btn.style.position = "relative";
      btn.appendChild(tip);
      setTimeout(() => { if (tip.parentNode) tip.remove(); }, 3000);
    }}
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 20h9"/><path d="M16.376 3.622a1 1 0 0 1 3.002 3.002L7.368 18.635a2 2 0 0 1-.855.506l-2.872.838a.5.5 0 0 1-.62-.62l.838-2.872a2 2 0 0 1 .506-.854z"/>
                </svg>
              </button>
            </div>
          </div>
        </div>

        ${isBusy
      ? html`<div class="chat-compose__below">
              <button
                class="btn chat-compose__stop-btn"
                type="button"
                ?disabled=${!canAbort}
                @click=${canAbort ? props.onAbort : props.onNewSession}
              >
                ${canAbort ? t("chat.stop") : t("chat.newSession")}
              </button>
            </div>`
      : nothing}
      </div>
      ${props.permissionPopupCallbacks && props.requestUpdate
      ? renderPermissionPopup(props.permissionPopupCallbacks, props.requestUpdate)
      : nothing}
    </section>
  `;
}

const CHAT_HISTORY_RENDER_LIMIT = 200;

function groupMessages(items: ChatItem[]): Array<ChatItem | MessageGroup> {
  const result: Array<ChatItem | MessageGroup> = [];
  let currentGroup: MessageGroup | null = null;

  for (const item of items) {
    if (item.kind !== "message") {
      if (currentGroup) {
        result.push(currentGroup);
        currentGroup = null;
      }
      result.push(item);
      continue;
    }

    const normalized = normalizeMessage(item.message);
    const role = normalizeRoleForGrouping(normalized.role);
    const timestamp = normalized.timestamp || Date.now();

    if (!currentGroup || currentGroup.role !== role) {
      if (currentGroup) {
        result.push(currentGroup);
      }
      currentGroup = {
        kind: "group",
        key: `group:${role}:${item.key}`,
        role,
        messages: [{ message: item.message, key: item.key }],
        timestamp,
        isStreaming: false,
      };
    } else {
      currentGroup.messages.push({ message: item.message, key: item.key });
    }
  }

  if (currentGroup) {
    result.push(currentGroup);
  }
  return result;
}

function buildChatItems(props: ChatProps): Array<ChatItem | MessageGroup> {
  const items: ChatItem[] = [];
  const history = Array.isArray(props.messages) ? props.messages : [];
  const tools = Array.isArray(props.toolMessages) ? props.toolMessages : [];
  const historyStart = Math.max(0, history.length - CHAT_HISTORY_RENDER_LIMIT);
  if (historyStart > 0) {
    items.push({
      kind: "message",
      key: "chat:history:notice",
      message: {
        role: "system",
        content: t("chat.showingLast", { limit: CHAT_HISTORY_RENDER_LIMIT, hidden: historyStart }),
        timestamp: Date.now(),
      },
    });
  }
  for (let i = historyStart; i < history.length; i++) {
    const msg = history[i];
    const normalized = normalizeMessage(msg);
    const raw = msg as Record<string, unknown>;
    const marker = raw.__openacosmi as Record<string, unknown> | undefined;
    if (marker && marker.kind === "compaction") {
      items.push({
        kind: "divider",
        key:
          typeof marker.id === "string"
            ? `divider:compaction:${marker.id}`
            : `divider:compaction:${normalized.timestamp}:${i}`,
        label: t("chat.compaction"),
        timestamp: normalized.timestamp ?? Date.now(),
      });
      continue;
    }

    if (!props.showThinking && normalized.role.toLowerCase() === "toolresult") {
      continue;
    }

    items.push({
      kind: "message",
      key: messageKey(msg, i),
      message: msg,
    });
  }
  if (props.showThinking) {
    for (let i = 0; i < tools.length; i++) {
      items.push({
        kind: "message",
        key: messageKey(tools[i], i + history.length),
        message: tools[i],
      });
    }
  }

  if (props.stream !== null) {
    const key = `stream:${props.sessionKey}:${props.streamStartedAt ?? "live"}`;
    if (props.stream.trim().length > 0) {
      items.push({
        kind: "stream",
        key,
        text: props.stream,
        startedAt: props.streamStartedAt ?? Date.now(),
      });
    } else {
      items.push({ kind: "reading-indicator", key });
    }
  }

  return groupMessages(items);
}

function messageKey(message: unknown, index: number): string {
  const m = message as Record<string, unknown>;
  const toolCallId = typeof m.toolCallId === "string" ? m.toolCallId : "";
  if (toolCallId) {
    return `tool:${toolCallId}`;
  }
  const id = typeof m.id === "string" ? m.id : "";
  if (id) {
    return `msg:${id}`;
  }
  const messageId = typeof m.messageId === "string" ? m.messageId : "";
  if (messageId) {
    return `msg:${messageId}`;
  }
  const timestamp = typeof m.timestamp === "number" ? m.timestamp : null;
  const role = typeof m.role === "string" ? m.role : "unknown";
  if (timestamp != null) {
    return `msg:${role}:${timestamp}:${index}`;
  }
  return `msg:${role}:${index}`;
}
