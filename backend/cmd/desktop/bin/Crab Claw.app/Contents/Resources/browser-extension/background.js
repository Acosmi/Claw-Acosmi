// background.js — CrabClaw Chrome Extension Service Worker
// Manages CDP debugging sessions and relay connection.
//
// Connection strategy (same pattern as Claude-in-Chrome / 1Password):
//   1. Primary: Native Messaging (connectNative) → strong keepalive, SW never suspends
//   2. Fallback: WebSocket + heartbeat ping → resets 30s idle timer (Chrome 116+)

// ---- Constants ----
const NATIVE_HOST_NAME = 'com.acosmi.crabclaw';
const DEFAULT_RELAY_URL = 'ws://127.0.0.1:19004/ws';
const RECONNECT_BASE_MS = 2000;
const RECONNECT_MAX_MS = 60000;
const WS_HEARTBEAT_MS = 20000; // 20s ping keeps SW alive (must be < 30s)

// ---- State ----
let nativePort = null;
let relayWs = null;
let relayToken = '';
let relayUrl = DEFAULT_RELAY_URL;
let attachedTabs = new Map(); // tabId -> { debuggee, attached }
let reconnectAttempts = 0;
let reconnectTimer = null;
let heartbeatTimer = null;
let lastConnectOpened = false;
let connectionMode = 'none'; // 'native' | 'websocket' | 'none'

// ---- Badge & Status ----
const STATUS = {
  OFF: { text: '', color: '#888888' },
  CONNECTING: { text: '...', color: '#FFA500' },
  ON: { text: 'ON', color: '#00AA00' },
  NATIVE: { text: 'N', color: '#0066CC' }, // native messaging connected
  ERROR: { text: '!', color: '#FF0000' },
};

function setBadge(status) {
  chrome.action.setBadgeText({ text: status.text });
  chrome.action.setBadgeBackgroundColor({ color: status.color });
}

function updateBadge() {
  const connected = connectionMode === 'native' ||
    (relayWs && relayWs.readyState === WebSocket.OPEN);

  if (attachedTabs.size === 0) {
    if (connectionMode === 'native') {
      setBadge(STATUS.NATIVE);
    } else if (connected) {
      setBadge(STATUS.OFF);
    } else if (relayWs && relayWs.readyState === WebSocket.CONNECTING) {
      setBadge(STATUS.CONNECTING);
    } else {
      setBadge(STATUS.OFF);
    }
    return;
  }

  if (!connected) {
    setBadge(STATUS.ERROR);
    return;
  }

  setBadge(connectionMode === 'native' ? STATUS.NATIVE : STATUS.ON);
}

// ---- Token Auto-Discovery ----
async function fetchRelayToken(baseUrl) {
  try {
    const httpUrl = baseUrl
      .replace(/^ws:\/\//, 'http://')
      .replace(/^wss:\/\//, 'https://')
      .replace(/\/ws\/?$/, '/json/version');
    const resp = await fetch(httpUrl, { signal: AbortSignal.timeout(3000) });
    if (!resp.ok) return '';
    const info = await resp.json();
    const wsDebugUrl = info.webSocketDebuggerUrl || '';
    const match = wsDebugUrl.match(/[?&]token=([^&]+)/);
    return match ? match[1] : '';
  } catch {
    return '';
  }
}

async function isRelayReachable(baseUrl) {
  try {
    const httpUrl = baseUrl
      .replace(/^ws:\/\//, 'http://')
      .replace(/^wss:\/\//, 'https://')
      .replace(/\/ws\/?$/, '/health');
    const resp = await fetch(httpUrl, { signal: AbortSignal.timeout(1500) });
    return resp.ok;
  } catch {
    return false;
  }
}

// ---- Native Messaging (Primary Path) ----

function connectNative() {
  try {
    nativePort = chrome.runtime.connectNative(NATIVE_HOST_NAME);
  } catch (e) {
    console.log('[CrabClaw] connectNative() threw:', e.message);
    return false;
  }

  nativePort.onMessage.addListener((msg) => {
    // Native messaging delivers parsed JSON objects directly.
    handleRelayMessage(msg);
  });

  nativePort.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError;
    console.log('[CrabClaw] Native port disconnected:', err?.message || 'unknown');
    nativePort = null;
    connectionMode = 'none';
    updateBadge();
    // Fall back to WebSocket.
    console.log('[CrabClaw] Falling back to WebSocket connection');
    connectWebSocket();
  });

  connectionMode = 'native';
  reconnectAttempts = 0;
  updateBadge();
  sendTabList();
  broadcastLog('sys', 'connect', 'Native messaging connected');
  broadcastStatus();
  console.log('[CrabClaw] Connected via native messaging (strong keepalive)');
  return true;
}

// ---- WebSocket Connection (Fallback Path) ----

async function connectWebSocket() {
  if (connectionMode === 'native') return; // native is active, skip
  if (relayWs && (relayWs.readyState === WebSocket.OPEN || relayWs.readyState === WebSocket.CONNECTING)) {
    return;
  }

  setBadge(STATUS.CONNECTING);

  // Auto-discover token if needed.
  if (!relayToken || !lastConnectOpened) {
    if (relayToken && !lastConnectOpened) {
      console.log('[CrabClaw] Previous WS connection failed before open — clearing stale token');
      relayToken = '';
      chrome.storage.local.remove('relayToken');
    }
    const discovered = await fetchRelayToken(relayUrl);
    if (discovered) {
      relayToken = discovered;
      chrome.storage.local.set({ relayToken });
      console.log('[CrabClaw] Auto-discovered relay token');
    }
  }

  if (!(await isRelayReachable(relayUrl))) {
    relayWs = null;
    updateBadge();
    scheduleReconnect();
    return;
  }

  // Token is required — never connect without one to avoid 401 noise.
  if (!relayToken) {
    console.log('[CrabClaw] No relay token available, deferring connection');
    updateBadge();
    scheduleReconnect();
    return;
  }

  lastConnectOpened = false;
  const url = `${relayUrl}?token=${relayToken}`;
  relayWs = new WebSocket(url);

  relayWs.onopen = () => {
    console.log('[CrabClaw] Relay connected (WebSocket fallback)');
    lastConnectOpened = true;
    reconnectAttempts = 0;
    connectionMode = 'websocket';
    updateBadge();
    sendTabList();
    broadcastLog('sys', 'connect', 'WebSocket connected');
    broadcastStatus();

    // Start heartbeat to keep Service Worker alive (must be < 30s).
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      if (relayWs && relayWs.readyState === WebSocket.OPEN) {
        relayWs.send(JSON.stringify({ type: 'ping' }));
      } else {
        stopHeartbeat();
      }
    }, WS_HEARTBEAT_MS);
  };

  relayWs.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleRelayMessage(msg);
    } catch {
      console.warn('[CrabClaw] Non-JSON relay message:', event.data);
    }
  };

  relayWs.onclose = (event) => {
    console.log('[CrabClaw] Relay disconnected', event.code, event.reason);
    relayWs = null;
    connectionMode = 'none';
    stopHeartbeat();
    updateBadge();
    scheduleReconnect();
  };

  relayWs.onerror = () => {
    console.warn('[CrabClaw] Relay connection failed, will retry');
    updateBadge();
  };
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
}

function scheduleReconnect() {
  if (connectionMode === 'native') return; // native is handling it
  if (reconnectTimer) return;

  reconnectAttempts++;
  const base = Math.min(RECONNECT_BASE_MS * Math.pow(2, reconnectAttempts - 1), RECONNECT_MAX_MS);
  const jitter = Math.random() * base * 0.3;
  const delay = Math.round(base + jitter);
  console.log(`[CrabClaw] Reconnect #${reconnectAttempts} in ${delay}ms`);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connectRelay();
  }, delay);
}

// ---- Unified Connection Entry Point ----

async function connectRelay() {
  // Already connected?
  if (connectionMode === 'native' && nativePort) return;
  if (connectionMode === 'websocket' && relayWs && relayWs.readyState === WebSocket.OPEN) return;

  // Try native messaging first.
  if (!nativePort) {
    if (connectNative()) return;
  }

  // Fall back to WebSocket.
  await connectWebSocket();
}

// ---- Send to Relay (unified) ----

function sendToRelay(data) {
  const payload = typeof data === 'object' ? data : JSON.parse(data);

  // Prefer native messaging.
  if (connectionMode === 'native' && nativePort) {
    try {
      nativePort.postMessage(payload);
      return true;
    } catch (e) {
      console.warn('[CrabClaw] Native send failed:', e.message);
      nativePort = null;
      connectionMode = 'none';
    }
  }

  // WebSocket fallback.
  if (relayWs && relayWs.readyState === WebSocket.OPEN) {
    relayWs.send(JSON.stringify(payload));
    return true;
  }

  return false;
}

// ---- Content Script Communication ----

/**
 * Send a message to the content script in the specified tab.
 * Returns a Promise that resolves with the content script's response.
 */
function sendToContentScript(tabId, message) {
  return new Promise((resolve) => {
    chrome.tabs.sendMessage(tabId, message, (response) => {
      if (chrome.runtime.lastError) {
        resolve({ error: chrome.runtime.lastError.message });
      } else {
        resolve(response || { error: 'No response from content script' });
      }
    });
  });
}

/**
 * Check if content script is alive in a tab.
 */
async function isContentScriptReady(tabId) {
  try {
    const resp = await sendToContentScript(tabId, { action: 'cs_ping' });
    return resp && resp.ok === true;
  } catch {
    return false;
  }
}

/**
 * Resolve the target tabId — use explicit tabId, or fall back to active tab.
 */
async function resolveTabId(tabId) {
  if (tabId) return tabId;
  try {
    const [activeTab] = await chrome.tabs.query({ active: true, currentWindow: true });
    return activeTab ? activeTab.id : null;
  } catch {
    return null;
  }
}

/**
 * S1-8: Capture screenshot via chrome.tabs.captureVisibleTab (no CDP needed).
 */
async function captureScreenshotViaTab(tabId, requestId) {
  try {
    // Ensure the target tab is active (captureVisibleTab captures the active tab).
    if (tabId) {
      const tab = await chrome.tabs.get(tabId);
      if (!tab.active) {
        await chrome.tabs.update(tabId, { active: true });
        // Brief delay for tab activation.
        await new Promise((r) => setTimeout(r, 150));
      }
    }

    const dataUrl = await chrome.tabs.captureVisibleTab(null, {
      format: 'png',
      quality: 90,
    });

    sendToRelay({
      type: 'cs_response',
      action: 'cs_screenshot',
      id: requestId,
      tabId,
      result: {
        dataUrl,
        format: 'png',
        url: undefined, // filled by caller if needed
      },
    });
  } catch (err) {
    sendToRelay({
      type: 'cs_response',
      action: 'cs_screenshot',
      id: requestId,
      tabId,
      error: err.message,
    });
  }
}

/**
 * S1-9: Route a content script command from relay to the target tab's content script.
 * S1-10: If content script is unreachable and command has a CDP equivalent, fall back to CDP.
 */
async function routeContentScriptCommand(msg) {
  const { action, id: requestId } = msg;
  const tabId = await resolveTabId(msg.tabId);

  if (!tabId) {
    sendToRelay({
      type: 'cs_response',
      action,
      id: requestId,
      error: 'No target tab specified or available',
    });
    return;
  }

  // Screenshot is special — handled by background.js, not content script.
  if (action === 'cs_screenshot') {
    await captureScreenshotViaTab(tabId, requestId);
    return;
  }

  // Try content script first.
  const csReady = await isContentScriptReady(tabId);
  if (csReady) {
    const result = await sendToContentScript(tabId, msg);
    sendToRelay({
      type: 'cs_response',
      action,
      id: requestId,
      tabId,
      result,
    });
    return;
  }

  // S1-10: Content script not available — try CDP fallback for supported commands.
  const cdpFallback = contentScriptToCdpFallback(action, msg);
  if (cdpFallback && attachedTabs.has(tabId)) {
    console.log(`[CrabClaw] Content script unreachable in tab ${tabId}, falling back to CDP for ${action}`);
    const debuggee = attachedTabs.get(tabId).debuggee;
    try {
      const cdpResult = await chrome.debugger.sendCommand(debuggee, cdpFallback.method, cdpFallback.params);
      sendToRelay({
        type: 'cs_response',
        action,
        id: requestId,
        tabId,
        result: cdpFallback.transform(cdpResult),
        via: 'cdp_fallback',
      });
      return;
    } catch (err) {
      sendToRelay({
        type: 'cs_response',
        action,
        id: requestId,
        tabId,
        error: `CDP fallback failed: ${err.message}`,
        via: 'cdp_fallback',
      });
      return;
    }
  }

  // Neither path available.
  sendToRelay({
    type: 'cs_response',
    action,
    id: requestId,
    tabId,
    error: `Content script not available in tab ${tabId} (page may be chrome://, about:, or not yet loaded). CDP fallback not attached.`,
  });
}

/**
 * S1-10: Map content script actions to CDP equivalents for fallback.
 * Returns { method, params, transform } or null if no fallback exists.
 */
function contentScriptToCdpFallback(action, msg) {
  switch (action) {
    case 'cs_get_text': {
      // Use JSON.stringify to safely serialize selector, preventing JS injection.
      const safeSelector = JSON.stringify(msg.selector || 'body');
      return {
        method: 'Runtime.evaluate',
        params: {
          expression: `(document.querySelector(${safeSelector}) || document.body).innerText || ''`,
          returnByValue: true,
        },
        transform: (r) => ({
          text: r.result?.value || '',
          url: '',
          title: '',
          via: 'cdp',
        }),
      };
    }

    case 'cs_get_html': {
      const safeSelector = JSON.stringify(msg.selector || 'html');
      return {
        method: 'Runtime.evaluate',
        params: {
          expression: `(document.querySelector(${safeSelector}) || document.documentElement).outerHTML`,
          returnByValue: true,
        },
        transform: (r) => ({
          html: r.result?.value || '',
          url: '',
          title: '',
          via: 'cdp',
        }),
      };
    }

    case 'cs_screenshot':
      // Screenshot fallback uses Page.captureScreenshot (CDP).
      return {
        method: 'Page.captureScreenshot',
        params: { format: 'png' },
        transform: (r) => ({
          dataUrl: r.data ? `data:image/png;base64,${r.data}` : '',
          format: 'png',
          via: 'cdp',
        }),
      };

    default:
      return null; // No CDP fallback available.
  }
}

// ---- Relay Message Handling ----
function handleRelayMessage(msg) {
  // msg is already a parsed object (from native messaging or JSON.parse).
  const { type, tabId, method, params, id } = msg;

  // Broadcast to Side Panel for live log.
  broadcastLog('in', type || 'unknown', JSON.stringify(msg).slice(0, 120));

  switch (type) {
    case 'cdp':
      forwardCdpToTab(tabId, method, params, id);
      break;
    case 'list_tabs':
      sendTabList();
      break;
    case 'attach':
      attachTab(tabId);
      break;
    case 'detach':
      detachTab(tabId);
      break;
    case 'navigate':
      if (tabId && msg.url) {
        chrome.tabs.update(tabId, { url: msg.url });
      }
      break;
    case 'create_tab':
      chrome.tabs.create({ url: msg.url || 'about:blank' }, (tab) => {
        sendToRelay({ type: 'tab_created', tabId: tab.id, url: tab.url });
      });
      break;
    case 'close_tab':
      if (tabId) {
        detachTab(tabId);
        chrome.tabs.remove(tabId);
      }
      break;
    case 'switch_tab':
      if (tabId) {
        chrome.tabs.update(tabId, { active: true });
      }
      break;
    case 'config':
      if (msg.relayUrl) relayUrl = msg.relayUrl;
      if (msg.token) relayToken = msg.token;
      break;
    case 'pong':
      // Heartbeat response from relay — no action needed.
      break;

    // S1-11 + S2-10: Content script message types from relay.
    case 'cs_get_text':
    case 'cs_get_html':
    case 'cs_query':
    case 'cs_query_all':
    case 'cs_accessibility_tree':
    case 'cs_screenshot':
    case 'cs_click':
    case 'cs_fill':
    case 'cs_scroll':
    case 'cs_highlight':
    case 'cs_annotate_som':
    case 'cs_clear_annotations':
    case 'cs_upload_file':
      routeContentScriptCommand(msg);
      break;

    // resize_window: 视口/窗口尺寸控制
    case 'resize_window':
      handleResizeWindow(msg);
      break;

    // S3: Downloads, Notifications, TabGroups.
    case 'save_file':
      handleSaveFile(msg);
      break;
    case 'save_screenshot':
      handleSaveScreenshot(msg);
      break;
    case 'notify':
      handleNotify(msg);
      break;
    case 'create_tab_group':
      handleCreateTabGroup(msg);
      break;
    case 'add_to_tab_group':
      handleAddToTabGroup(msg);
      break;
    case 'remove_from_tab_group':
      handleRemoveFromTabGroup(msg);
      break;

    default:
      console.warn('[CrabClaw] Unknown relay message type:', type);
  }
}

// ---- CDP Debugger ----
async function attachTab(tabId) {
  if (attachedTabs.has(tabId)) {
    console.log('[CrabClaw] Tab already attached:', tabId);
    return true;
  }

  const debuggee = { tabId };
  try {
    await chrome.debugger.attach(debuggee, '1.3');
    attachedTabs.set(tabId, { debuggee, attached: true });
    console.log('[CrabClaw] Attached to tab:', tabId);

    await chrome.debugger.sendCommand(debuggee, 'Page.enable');
    await chrome.debugger.sendCommand(debuggee, 'Runtime.enable');
    await chrome.debugger.sendCommand(debuggee, 'DOM.enable');
    await chrome.debugger.sendCommand(debuggee, 'Accessibility.enable');

    updateBadge();
    sendToRelay({ type: 'tab_attached', tabId });
    return true;
  } catch (err) {
    console.error('[CrabClaw] Attach failed:', tabId, err.message);
    sendToRelay({ type: 'error', tabId, error: err.message });
    return false;
  }
}

async function detachTab(tabId) {
  const info = attachedTabs.get(tabId);
  if (!info) return;

  try {
    await chrome.debugger.detach(info.debuggee);
  } catch {
    // Tab may already be closed.
  }
  attachedTabs.delete(tabId);
  updateBadge();
  sendToRelay({ type: 'tab_detached', tabId });
  console.log('[CrabClaw] Detached from tab:', tabId);
}

async function forwardCdpToTab(tabId, method, params, requestId) {
  let targetTabId = tabId;
  if (!targetTabId && attachedTabs.size > 0) {
    targetTabId = attachedTabs.keys().next().value;
  }

  if (!targetTabId || !attachedTabs.has(targetTabId)) {
    sendToRelay({
      type: 'cdp_response',
      id: requestId,
      error: 'No attached tab for CDP command',
    });
    return;
  }

  const debuggee = attachedTabs.get(targetTabId).debuggee;

  try {
    const result = await chrome.debugger.sendCommand(debuggee, method, params || {});
    sendToRelay({
      type: 'cdp_response',
      id: requestId,
      tabId: targetTabId,
      result,
    });
  } catch (err) {
    sendToRelay({
      type: 'cdp_response',
      id: requestId,
      tabId: targetTabId,
      error: err.message,
    });
  }
}

// ---- resize_window ----

async function handleResizeWindow(msg) {
  try {
    const width = msg.width || 1280;
    const height = msg.height || 720;

    if (msg.viewport && msg.tabId) {
      // CDP Emulation: 精确控制视口尺寸（不改变窗口大小）
      if (attachedTabs.has(msg.tabId)) {
        const debuggee = attachedTabs.get(msg.tabId).debuggee;
        await chrome.debugger.sendCommand(debuggee, 'Emulation.setDeviceMetricsOverride', {
          width,
          height,
          deviceScaleFactor: msg.deviceScaleFactor || 1,
          mobile: msg.mobile || false,
        });
        sendToRelay({
          type: 'resize_response',
          id: msg.id,
          result: { width, height, method: 'cdp_emulation' },
        });
        return;
      }
    }

    // 默认：调整浏览器窗口大小
    const [currentWindow] = await chrome.windows.getAll({ populate: false });
    if (currentWindow) {
      await chrome.windows.update(currentWindow.id, { width, height });
      sendToRelay({
        type: 'resize_response',
        id: msg.id,
        result: { width, height, method: 'window_resize' },
      });
    } else {
      sendToRelay({ type: 'resize_response', id: msg.id, error: 'No window found' });
    }
  } catch (err) {
    sendToRelay({ type: 'resize_response', id: msg.id, error: err.message });
  }
}

// ---- S3: Downloads API ----

async function handleSaveFile(msg) {
  try {
    const url = msg.url || msg.dataUrl;
    if (!url) {
      sendToRelay({ type: 's3_response', action: 'save_file', id: msg.id, error: 'Missing url or dataUrl' });
      return;
    }
    const downloadId = await chrome.downloads.download({
      url,
      filename: msg.filename || undefined,
      saveAs: msg.saveAs || false,
    });
    sendToRelay({ type: 's3_response', action: 'save_file', id: msg.id, result: { downloadId } });
  } catch (err) {
    sendToRelay({ type: 's3_response', action: 'save_file', id: msg.id, error: err.message });
  }
}

async function handleSaveScreenshot(msg) {
  try {
    const tabId = await resolveTabId(msg.tabId);
    if (tabId) {
      const tab = await chrome.tabs.get(tabId);
      if (!tab.active) {
        await chrome.tabs.update(tabId, { active: true });
        await new Promise((r) => setTimeout(r, 150));
      }
    }

    const dataUrl = await chrome.tabs.captureVisibleTab(null, {
      format: msg.format || 'png',
      quality: msg.quality || 90,
    });

    // Convert to blob URL for download.
    const resp = await fetch(dataUrl);
    const blob = await resp.blob();
    const blobUrl = URL.createObjectURL(blob);

    const filename = msg.filename || `screenshot-${Date.now()}.${msg.format || 'png'}`;
    const downloadId = await chrome.downloads.download({
      url: blobUrl,
      filename,
      saveAs: msg.saveAs || false,
    });

    // Clean up blob URL after a delay.
    setTimeout(() => URL.revokeObjectURL(blobUrl), 5000);

    sendToRelay({
      type: 's3_response',
      action: 'save_screenshot',
      id: msg.id,
      result: { downloadId, filename, dataUrl },
    });
  } catch (err) {
    sendToRelay({ type: 's3_response', action: 'save_screenshot', id: msg.id, error: err.message });
  }
}

// ---- S3: Notifications API ----

function handleNotify(msg) {
  try {
    const notifId = msg.notifId || `crabclaw-${Date.now()}`;
    chrome.notifications.create(notifId, {
      type: 'basic',
      iconUrl: msg.iconUrl || 'icons/icon128.png',
      title: msg.title || 'CrabClaw',
      message: msg.message || '',
      priority: msg.priority || 0,
    }, (createdId) => {
      sendToRelay({ type: 's3_response', action: 'notify', id: msg.id, result: { notifId: createdId } });
    });
  } catch (err) {
    sendToRelay({ type: 's3_response', action: 'notify', id: msg.id, error: err.message });
  }
}

// ---- S3: TabGroups API ----

async function handleCreateTabGroup(msg) {
  try {
    const tabIds = msg.tabIds || [];
    if (tabIds.length === 0) {
      sendToRelay({ type: 's3_response', action: 'create_tab_group', id: msg.id, error: 'No tabIds provided' });
      return;
    }

    const groupId = await chrome.tabs.group({ tabIds });

    await chrome.tabGroups.update(groupId, {
      title: msg.title || 'CrabClaw',
      color: msg.color || 'blue',
      collapsed: msg.collapsed || false,
    });

    sendToRelay({ type: 's3_response', action: 'create_tab_group', id: msg.id, result: { groupId } });
  } catch (err) {
    sendToRelay({ type: 's3_response', action: 'create_tab_group', id: msg.id, error: err.message });
  }
}

async function handleAddToTabGroup(msg) {
  try {
    if (!msg.groupId || !msg.tabIds || msg.tabIds.length === 0) {
      sendToRelay({ type: 's3_response', action: 'add_to_tab_group', id: msg.id, error: 'Missing groupId or tabIds' });
      return;
    }
    await chrome.tabs.group({ tabIds: msg.tabIds, groupId: msg.groupId });
    sendToRelay({ type: 's3_response', action: 'add_to_tab_group', id: msg.id, result: { ok: true } });
  } catch (err) {
    sendToRelay({ type: 's3_response', action: 'add_to_tab_group', id: msg.id, error: err.message });
  }
}

async function handleRemoveFromTabGroup(msg) {
  try {
    if (!msg.tabIds || msg.tabIds.length === 0) {
      sendToRelay({ type: 's3_response', action: 'remove_from_tab_group', id: msg.id, error: 'Missing tabIds' });
      return;
    }
    await chrome.tabs.ungroup(msg.tabIds);
    sendToRelay({ type: 's3_response', action: 'remove_from_tab_group', id: msg.id, result: { ok: true } });
  } catch (err) {
    sendToRelay({ type: 's3_response', action: 'remove_from_tab_group', id: msg.id, error: err.message });
  }
}

// ---- Tab Management ----
async function sendTabList() {
  try {
    const tabs = await chrome.tabs.query({});
    const tabList = tabs.map((t) => {
      // Content scripts cannot inject into chrome://, about:, chrome-extension:// etc.
      const url = t.url || '';
      const csEligible = url.startsWith('http://') || url.startsWith('https://');
      return {
        id: t.id,
        url: t.url,
        title: t.title,
        active: t.active,
        attached: attachedTabs.has(t.id),
        contentScript: csEligible, // whether content script can run in this tab
      };
    });
    sendToRelay({ type: 'tab_list', tabs: tabList });
  } catch (err) {
    console.error('[CrabClaw] Failed to query tabs:', err);
  }
}

// ---- CDP Event Forwarding ----
chrome.debugger.onEvent.addListener((source, method, params) => {
  if (!source.tabId || !attachedTabs.has(source.tabId)) return;

  sendToRelay({
    type: 'cdp_event',
    tabId: source.tabId,
    method,
    params,
  });
});

chrome.debugger.onDetach.addListener((source, reason) => {
  if (source.tabId && attachedTabs.has(source.tabId)) {
    attachedTabs.delete(source.tabId);
    updateBadge();
    sendToRelay({ type: 'tab_detached', tabId: source.tabId, reason });
    console.log('[CrabClaw] Debugger detached:', source.tabId, reason);
  }
});

chrome.tabs.onRemoved.addListener((tabId) => {
  if (attachedTabs.has(tabId)) {
    attachedTabs.delete(tabId);
    updateBadge();
    sendToRelay({ type: 'tab_closed', tabId });
  }
});

// ---- S5: UI Port Connections (Popup + Side Panel) ----
const uiPorts = new Set();

chrome.runtime.onConnect.addListener((port) => {
  if (port.name === 'sidepanel' || port.name === 'popup') {
    uiPorts.add(port);
    port.onDisconnect.addListener(() => uiPorts.delete(port));
  }
});

/**
 * Broadcast a log event to all connected UI ports (Side Panel, Popup).
 */
function broadcastLog(dir, type, detail) {
  const msg = { _log: { dir, type, detail } };
  for (const port of uiPorts) {
    try { port.postMessage(msg); } catch { uiPorts.delete(port); }
  }
}

/**
 * Broadcast status update to all UI ports.
 */
function broadcastStatus() {
  const msg = {
    _status: {
      connectionMode,
      attachedTabs: Array.from(attachedTabs.keys()),
    },
  };
  for (const port of uiPorts) {
    try { port.postMessage(msg); } catch { uiPorts.delete(port); }
  }
}

// ---- Popup Communication ----
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  switch (msg.action) {
    case 'getStatus':
      sendResponse({
        connected: connectionMode !== 'none',
        connectionMode,
        attachedTabs: Array.from(attachedTabs.keys()),
        relayUrl,
        hasToken: !!relayToken,
        contentScripts: true, // Content scripts are now available
      });
      return true;

    case 'toggleTab': {
      const tabId = msg.tabId;
      if (attachedTabs.has(tabId)) {
        detachTab(tabId).then(() => sendResponse({ attached: false }));
      } else {
        attachTab(tabId).then((ok) => sendResponse({ attached: ok }));
      }
      return true;
    }

    case 'connect':
      if (msg.relayUrl) relayUrl = msg.relayUrl;
      relayToken = msg.token || '';
      chrome.storage.local.set({ relayUrl, relayToken });
      reconnectAttempts = 0;
      // Disconnect existing connections.
      if (nativePort) {
        nativePort.disconnect();
        nativePort = null;
      }
      if (relayWs) {
        relayWs.close();
        relayWs = null;
      }
      connectionMode = 'none';
      stopHeartbeat();
      connectRelay();
      sendResponse({ ok: true });
      return true;

    case 'disconnect':
      if (nativePort) {
        nativePort.disconnect();
        nativePort = null;
      }
      if (relayWs) {
        relayWs.close();
        relayWs = null;
      }
      connectionMode = 'none';
      stopHeartbeat();
      for (const tabId of attachedTabs.keys()) {
        detachTab(tabId);
      }
      setBadge(STATUS.OFF);
      sendResponse({ ok: true });
      return true;

    case 'relayCommand':
      // Side panel command input: forward raw JSON to relay.
      if (msg.payload) {
        sendToRelay(msg.payload);
        broadcastLog('out', msg.payload.type || 'cmd', JSON.stringify(msg.payload).slice(0, 120));
      }
      sendResponse({ ok: true });
      return true;

    default:
      sendResponse({ error: 'Unknown action' });
      return true;
  }
});

// ---- Startup ----
chrome.storage.local.get(['relayUrl', 'relayToken'], (items) => {
  if (items.relayUrl) relayUrl = items.relayUrl;
  if (items.relayToken) relayToken = items.relayToken;

  // Connect: tries native messaging first, falls back to WebSocket.
  connectRelay();
});

console.log('[CrabClaw] Service worker initialized (native messaging + WebSocket fallback)');
