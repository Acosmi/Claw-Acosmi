// popup.js — OpenAcosmi Extension Popup UI

const statusEl = document.getElementById('status');
const relayUrlInput = document.getElementById('relayUrl');
const relayTokenInput = document.getElementById('relayTokenInput');
const connectBtn = document.getElementById('connectBtn');
const tabListEl = document.getElementById('tabList');
const attachCurrentBtn = document.getElementById('attachCurrentBtn');
const detachAllBtn = document.getElementById('detachAllBtn');

let currentStatus = {};

// ---- Init ----
async function init() {
  await refreshStatus();
  await refreshTabs();
}

// ---- Status ----
async function refreshStatus() {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ action: 'getStatus' }, (resp) => {
      if (chrome.runtime.lastError) {
        statusEl.textContent = 'Error';
        statusEl.className = 'status error';
        resolve();
        return;
      }
      currentStatus = resp || {};
      if (currentStatus.connected) {
        statusEl.textContent = `Connected (${currentStatus.attachedTabs?.length || 0} tabs)`;
        statusEl.className = 'status connected';
        connectBtn.textContent = 'Reconnect';
      } else {
        statusEl.textContent = 'Disconnected';
        statusEl.className = 'status';
        connectBtn.textContent = 'Connect';
      }
      if (currentStatus.relayUrl) {
        relayUrlInput.value = currentStatus.relayUrl;
      }
      if (currentStatus.hasToken && !relayTokenInput.value) {
        relayTokenInput.placeholder = 'Token (auto-discovered)';
      }
      resolve();
    });
  });
}

// ---- Tab List ----
async function refreshTabs() {
  const tabs = await chrome.tabs.query({});
  const attachedSet = new Set(currentStatus.attachedTabs || []);

  if (tabs.length === 0) {
    tabListEl.innerHTML = '<div class="empty">No tabs</div>';
    return;
  }

  tabListEl.innerHTML = '';
  for (const tab of tabs) {
    const item = document.createElement('div');
    item.className = 'tab-item';
    item.style.cursor = 'pointer';

    const isAttached = attachedSet.has(tab.id);

    const dot = document.createElement('div');
    dot.className = 'tab-dot' + (isAttached ? ' attached' : (tab.active ? ' active' : ''));

    const info = document.createElement('div');
    info.className = 'tab-info';

    const title = document.createElement('div');
    title.className = 'tab-title';
    title.textContent = tab.title || 'Untitled';

    const url = document.createElement('div');
    url.className = 'tab-url';
    url.textContent = tab.url || '';

    info.appendChild(title);
    info.appendChild(url);

    const btn = document.createElement('button');
    btn.className = 'btn btn-sm ' + (isAttached ? 'btn-danger' : 'btn-primary');
    btn.textContent = isAttached ? 'Detach' : 'Attach';
    btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      await toggleTab(tab.id);
    });

    item.appendChild(dot);
    item.appendChild(info);
    item.appendChild(btn);
    item.addEventListener('click', () => toggleTab(tab.id));

    tabListEl.appendChild(item);
  }
}

async function toggleTab(tabId) {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ action: 'toggleTab', tabId }, async () => {
      await refreshStatus();
      await refreshTabs();
      resolve();
    });
  });
}

// ---- Button Handlers ----
connectBtn.addEventListener('click', () => {
  const url = relayUrlInput.value.trim();
  const token = relayTokenInput.value.trim();
  chrome.runtime.sendMessage({
    action: 'connect',
    relayUrl: url || undefined,
    token: token || undefined,
  }, async () => {
    // Wait briefly for connection to establish.
    setTimeout(async () => {
      await refreshStatus();
    }, 500);
  });
});

attachCurrentBtn.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab) {
    await toggleTab(tab.id);
  }
});

detachAllBtn.addEventListener('click', () => {
  chrome.runtime.sendMessage({ action: 'disconnect' }, async () => {
    await refreshStatus();
    await refreshTabs();
  });
});

// ---- Start ----
init();
