// background.js — OpenAcosmi Chrome Extension Service Worker
// Manages CDP debugging sessions and WebSocket relay connection.

// Relay 端口 = gateway_port + 3。开发环境 gateway=19001 → relay=19004。
const DEFAULT_RELAY_URL = 'ws://127.0.0.1:19004/ws';
const RECONNECT_DELAY_MS = 3000;
const MAX_RECONNECT_ATTEMPTS = 10;

// ---- State ----
let relayWs = null;
let relayToken = '';
let relayUrl = DEFAULT_RELAY_URL;
let attachedTabs = new Map(); // tabId -> { debuggee, attached }
let reconnectAttempts = 0;
let reconnectTimer = null;

// ---- Badge & Status ----
const STATUS = {
  OFF: { text: '', color: '#888888' },
  CONNECTING: { text: '...', color: '#FFA500' },
  ON: { text: 'ON', color: '#00AA00' },
  ERROR: { text: '!', color: '#FF0000' },
};

function setBadge(status) {
  chrome.action.setBadgeText({ text: status.text });
  chrome.action.setBadgeBackgroundColor({ color: status.color });
}

function updateBadge() {
  if (attachedTabs.size === 0) {
    if (relayWs && relayWs.readyState === WebSocket.OPEN) {
      setBadge(STATUS.OFF);
    } else if (relayWs && relayWs.readyState === WebSocket.CONNECTING) {
      setBadge(STATUS.CONNECTING);
    } else {
      setBadge(STATUS.OFF);
    }
    return;
  }

  if (!relayWs || relayWs.readyState !== WebSocket.OPEN) {
    setBadge(STATUS.ERROR);
    return;
  }

  setBadge(STATUS.ON);
}

// ---- Token Auto-Discovery ----
// Fetches the relay auth token from /json/version (loopback only).
// The relay exposes the token in the webSocketDebuggerUrl field.
async function fetchRelayToken(baseUrl) {
  try {
    // Convert ws:// URL to http:// for REST call.
    const httpUrl = baseUrl
      .replace(/^ws:\/\//, 'http://')
      .replace(/^wss:\/\//, 'https://')
      .replace(/\/ws\/?$/, '/json/version');
    const resp = await fetch(httpUrl, { signal: AbortSignal.timeout(3000) });
    if (!resp.ok) return '';
    const info = await resp.json();
    const wsDebugUrl = info.webSocketDebuggerUrl || '';
    // Extract token from ?token=XXX in the URL.
    const match = wsDebugUrl.match(/[?&]token=([^&]+)/);
    return match ? match[1] : '';
  } catch {
    return '';
  }
}

// ---- Relay Connection ----
async function connectRelay() {
  if (relayWs && (relayWs.readyState === WebSocket.OPEN || relayWs.readyState === WebSocket.CONNECTING)) {
    return;
  }

  setBadge(STATUS.CONNECTING);

  // Auto-discover token if not manually configured.
  if (!relayToken) {
    const discovered = await fetchRelayToken(relayUrl);
    if (discovered) {
      relayToken = discovered;
      chrome.storage.local.set({ relayToken });
      console.log('[OpenAcosmi] Auto-discovered relay token');
    }
  }

  const url = relayToken ? `${relayUrl}?token=${relayToken}` : relayUrl;
  relayWs = new WebSocket(url);

  relayWs.onopen = () => {
    console.log('[OpenAcosmi] Relay connected');
    reconnectAttempts = 0;
    updateBadge();

    // Send initial tab list to relay.
    sendTabList();
  };

  relayWs.onmessage = (event) => {
    handleRelayMessage(event.data);
  };

  relayWs.onclose = (event) => {
    console.log('[OpenAcosmi] Relay disconnected', event.code, event.reason);
    relayWs = null;
    updateBadge();
    scheduleReconnect();
  };

  relayWs.onerror = (error) => {
    console.error('[OpenAcosmi] Relay error', error);
    updateBadge();
  };
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    console.log('[OpenAcosmi] Max reconnect attempts reached');
    setBadge(STATUS.ERROR);
    return;
  }

  reconnectAttempts++;
  const delay = RECONNECT_DELAY_MS * Math.min(reconnectAttempts, 5);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connectRelay();
  }, delay);
}

function sendToRelay(data) {
  if (relayWs && relayWs.readyState === WebSocket.OPEN) {
    relayWs.send(typeof data === 'string' ? data : JSON.stringify(data));
    return true;
  }
  return false;
}

// ---- Relay Message Handling ----
function handleRelayMessage(raw) {
  let msg;
  try {
    msg = JSON.parse(raw);
  } catch {
    console.warn('[OpenAcosmi] Non-JSON relay message:', raw);
    return;
  }

  // Relay commands from the agent.
  const { type, tabId, method, params, id } = msg;

  switch (type) {
    case 'cdp':
      // Forward CDP command to attached tab.
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
      // Update relay configuration.
      if (msg.relayUrl) relayUrl = msg.relayUrl;
      if (msg.token) relayToken = msg.token;
      break;

    default:
      console.warn('[OpenAcosmi] Unknown relay message type:', type);
  }
}

// ---- CDP Debugger ----
async function attachTab(tabId) {
  if (attachedTabs.has(tabId)) {
    console.log('[OpenAcosmi] Tab already attached:', tabId);
    return true;
  }

  const debuggee = { tabId };
  try {
    await chrome.debugger.attach(debuggee, '1.3');
    attachedTabs.set(tabId, { debuggee, attached: true });
    console.log('[OpenAcosmi] Attached to tab:', tabId);

    // Enable required CDP domains.
    await chrome.debugger.sendCommand(debuggee, 'Page.enable');
    await chrome.debugger.sendCommand(debuggee, 'Runtime.enable');
    await chrome.debugger.sendCommand(debuggee, 'DOM.enable');
    await chrome.debugger.sendCommand(debuggee, 'Accessibility.enable');

    updateBadge();
    sendToRelay({ type: 'tab_attached', tabId });
    return true;
  } catch (err) {
    console.error('[OpenAcosmi] Attach failed:', tabId, err.message);
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
  console.log('[OpenAcosmi] Detached from tab:', tabId);
}

async function forwardCdpToTab(tabId, method, params, requestId) {
  // If no specific tab, use first attached tab.
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

// ---- Tab Management ----
async function sendTabList() {
  try {
    const tabs = await chrome.tabs.query({});
    const tabList = tabs.map((t) => ({
      id: t.id,
      url: t.url,
      title: t.title,
      active: t.active,
      attached: attachedTabs.has(t.id),
    }));
    sendToRelay({ type: 'tab_list', tabs: tabList });
  } catch (err) {
    console.error('[OpenAcosmi] Failed to query tabs:', err);
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
    console.log('[OpenAcosmi] Debugger detached:', source.tabId, reason);
  }
});

// Listen for tab close events.
chrome.tabs.onRemoved.addListener((tabId) => {
  if (attachedTabs.has(tabId)) {
    attachedTabs.delete(tabId);
    updateBadge();
    sendToRelay({ type: 'tab_closed', tabId });
  }
});

// ---- Popup Communication ----
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  switch (msg.action) {
    case 'getStatus':
      sendResponse({
        connected: relayWs && relayWs.readyState === WebSocket.OPEN,
        attachedTabs: Array.from(attachedTabs.keys()),
        relayUrl,
        hasToken: !!relayToken,
      });
      return true;

    case 'toggleTab': {
      const tabId = msg.tabId;
      if (attachedTabs.has(tabId)) {
        detachTab(tabId).then(() => sendResponse({ attached: false }));
      } else {
        attachTab(tabId).then((ok) => sendResponse({ attached: ok }));
      }
      return true; // async response
    }

    case 'connect':
      if (msg.relayUrl) relayUrl = msg.relayUrl;
      // Token: use manual value if provided, else clear for auto-discovery.
      relayToken = msg.token || '';
      // Save config.
      chrome.storage.local.set({ relayUrl, relayToken });
      reconnectAttempts = 0;
      if (relayWs) {
        relayWs.close();
        relayWs = null;
      }
      connectRelay();
      sendResponse({ ok: true });
      return true;

    case 'disconnect':
      if (relayWs) {
        relayWs.close();
        relayWs = null;
      }
      // Detach all tabs.
      for (const tabId of attachedTabs.keys()) {
        detachTab(tabId);
      }
      setBadge(STATUS.OFF);
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

  // Auto-connect on startup.
  connectRelay();
});

console.log('[OpenAcosmi] Service worker initialized');
