// popup.js — CrabClaw Extension Popup UI (v1.1)
// 4-zone layout: Status + Tabs + Log + Settings

'use strict';

// ---- DOM refs ----
const statusBadge = document.getElementById('statusBadge');
const statusCounts = document.getElementById('statusCounts');
const tabSearch = document.getElementById('tabSearch');
const tabListEl = document.getElementById('tabList');
const attachCurrentBtn = document.getElementById('attachCurrentBtn');
const refreshBtn = document.getElementById('refreshBtn');
const disconnectBtn = document.getElementById('disconnectBtn');
const logList = document.getElementById('logList');
const clearLogBtn = document.getElementById('clearLogBtn');
const relayUrlInput = document.getElementById('relayUrl');
const relayTokenInput = document.getElementById('relayToken');
const autoConnectToggle = document.getElementById('autoConnectToggle');
const saveSettingsBtn = document.getElementById('saveSettingsBtn');

// ---- State ----
let currentStatus = {};
let logEntries = [];
const MAX_LOG = 30;

// ---- Panel collapse/expand ----
document.querySelectorAll('.panel-header').forEach((header) => {
  header.addEventListener('click', () => {
    const toggle = header.querySelector('.panel-toggle');
    const body = header.nextElementSibling;
    if (body) {
      body.classList.toggle('collapsed');
      toggle.classList.toggle('collapsed');
    }
  });
});

// ---- Status ----
async function refreshStatus() {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ action: 'getStatus' }, (resp) => {
      if (chrome.runtime.lastError) {
        statusBadge.textContent = 'ERR';
        statusBadge.className = 'status-badge off';
        resolve();
        return;
      }
      currentStatus = resp || {};
      updateStatusUI();
      resolve();
    });
  });
}

function updateStatusUI() {
  const mode = currentStatus.connectionMode || 'none';
  const attached = currentStatus.attachedTabs?.length || 0;

  if (mode === 'native') {
    statusBadge.textContent = 'NATIVE';
    statusBadge.className = 'status-badge native';
  } else if (mode === 'websocket') {
    statusBadge.textContent = 'WS';
    statusBadge.className = 'status-badge websocket';
  } else {
    statusBadge.textContent = 'OFF';
    statusBadge.className = 'status-badge off';
  }

  statusCounts.textContent = attached > 0 ? `${attached} attached` : '';

  if (currentStatus.relayUrl && !relayUrlInput.value) {
    relayUrlInput.value = currentStatus.relayUrl;
  }
  if (currentStatus.hasToken && !relayTokenInput.value) {
    relayTokenInput.placeholder = 'Token (auto-discovered)';
  }
}

// ---- Tab List ----
async function refreshTabs() {
  const tabs = await chrome.tabs.query({});
  const attachedSet = new Set(currentStatus.attachedTabs || []);
  const filter = (tabSearch.value || '').toLowerCase();

  if (tabs.length === 0) {
    tabListEl.innerHTML = '<div class="empty">No tabs</div>';
    return;
  }

  // Filter
  let filtered = tabs;
  if (filter) {
    filtered = tabs.filter((t) =>
      (t.title || '').toLowerCase().includes(filter) ||
      (t.url || '').toLowerCase().includes(filter)
    );
  }

  // Group: attached > active > content-script-eligible > other
  const attached = [];
  const active = [];
  const csEligible = [];
  const other = [];

  for (const t of filtered) {
    const isAttached = attachedSet.has(t.id);
    const url = t.url || '';
    const canCS = url.startsWith('http://') || url.startsWith('https://');

    if (isAttached) attached.push(t);
    else if (t.active) active.push(t);
    else if (canCS) csEligible.push(t);
    else other.push(t);
  }

  tabListEl.innerHTML = '';

  function addGroup(label, items, dotClass) {
    if (items.length === 0) return;
    const lbl = document.createElement('div');
    lbl.className = 'tab-group-label';
    lbl.textContent = `${label} (${items.length})`;
    tabListEl.appendChild(lbl);

    for (const t of items) {
      tabListEl.appendChild(createTabItem(t, attachedSet, dotClass));
    }
  }

  addGroup('Attached', attached, 'attached');
  addGroup('Active', active, 'active');
  addGroup('Pages', csEligible, 'cs');
  addGroup('Other', other, 'inactive');

  if (filtered.length === 0) {
    tabListEl.innerHTML = '<div class="empty">No matching tabs</div>';
  }
}

function createTabItem(tab, attachedSet, defaultDot) {
  const isAttached = attachedSet.has(tab.id);
  const url = tab.url || '';
  const canCS = url.startsWith('http://') || url.startsWith('https://');

  const item = document.createElement('div');
  item.className = 'tab-item';

  // Dot
  const dot = document.createElement('div');
  dot.className = 'tab-dot ' + (isAttached ? 'attached' : defaultDot);
  item.appendChild(dot);

  // Info
  const info = document.createElement('div');
  info.className = 'tab-info';
  const title = document.createElement('div');
  title.className = 'tab-title';
  title.textContent = tab.title || 'Untitled';
  const urlEl = document.createElement('div');
  urlEl.className = 'tab-url';
  urlEl.textContent = url;
  info.appendChild(title);
  info.appendChild(urlEl);
  item.appendChild(info);

  // Badges
  const badges = document.createElement('div');
  badges.className = 'tab-badges';
  if (isAttached) {
    const b = document.createElement('span');
    b.className = 'tab-badge cdp';
    b.textContent = 'CDP';
    badges.appendChild(b);
  }
  if (canCS) {
    const b = document.createElement('span');
    b.className = 'tab-badge cs';
    b.textContent = 'CS';
    badges.appendChild(b);
  }
  item.appendChild(badges);

  // Click to toggle CDP attach
  item.addEventListener('click', () => toggleTab(tab.id));

  return item;
}

async function toggleTab(tabId) {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ action: 'toggleTab', tabId }, async () => {
      addLog('out', `toggleTab ${tabId}`);
      await refreshStatus();
      await refreshTabs();
      resolve();
    });
  });
}

// ---- Log ----
function addLog(type, message) {
  const now = new Date();
  const time = now.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
  logEntries.push({ time, type, message });
  if (logEntries.length > MAX_LOG) logEntries.shift();
  renderLog();
}

function renderLog() {
  if (logEntries.length === 0) {
    logList.innerHTML = '<div class="empty">No log entries</div>';
    return;
  }
  logList.innerHTML = '';
  for (const entry of logEntries) {
    const item = document.createElement('div');
    item.className = 'log-item';

    const t = document.createElement('span');
    t.className = 'log-time';
    t.textContent = entry.time;

    const tp = document.createElement('span');
    tp.className = 'log-type ' + entry.type;
    tp.textContent = entry.type === 'in' ? 'IN' : entry.type === 'out' ? 'OUT' : 'ERR';

    const m = document.createElement('span');
    m.className = 'log-msg';
    m.textContent = entry.message;

    item.appendChild(t);
    item.appendChild(tp);
    item.appendChild(m);
    logList.appendChild(item);
  }
  logList.scrollTop = logList.scrollHeight;
}

// ---- Button Handlers ----
attachCurrentBtn.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab) {
    addLog('out', `attachCurrent ${tab.id}`);
    await toggleTab(tab.id);
  }
});

refreshBtn.addEventListener('click', async () => {
  addLog('out', 'refresh');
  await refreshStatus();
  await refreshTabs();
});

disconnectBtn.addEventListener('click', () => {
  chrome.runtime.sendMessage({ action: 'disconnect' }, async () => {
    addLog('out', 'disconnect');
    await refreshStatus();
    await refreshTabs();
  });
});

clearLogBtn.addEventListener('click', () => {
  logEntries = [];
  renderLog();
});

// Tab search
tabSearch.addEventListener('input', () => {
  refreshTabs();
});

// ---- Settings ----
autoConnectToggle.addEventListener('click', () => {
  autoConnectToggle.classList.toggle('on');
});

saveSettingsBtn.addEventListener('click', () => {
  const url = relayUrlInput.value.trim();
  const token = relayTokenInput.value.trim();
  const autoConnect = autoConnectToggle.classList.contains('on');

  chrome.storage.local.set({ autoConnect });

  chrome.runtime.sendMessage({
    action: 'connect',
    relayUrl: url || undefined,
    token: token || undefined,
  }, async () => {
    addLog('out', 'save & reconnect');
    setTimeout(async () => {
      await refreshStatus();
      await refreshTabs();
    }, 500);
  });
});

// Load saved settings
chrome.storage.local.get(['autoConnect'], (items) => {
  if (items.autoConnect === false) {
    autoConnectToggle.classList.remove('on');
  }
});

// ---- Init ----
async function init() {
  await refreshStatus();
  await refreshTabs();
  addLog('in', 'Popup opened');
}

init();
