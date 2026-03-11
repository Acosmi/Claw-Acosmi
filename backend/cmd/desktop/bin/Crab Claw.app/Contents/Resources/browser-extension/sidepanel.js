// sidepanel.js — CrabClaw Side Panel
// Real-time log stream + command panel + status overview.
// Connects to background.js via long-lived port.

'use strict';

// ---- DOM ----
const spStatus = document.getElementById('spStatus');
const spTabCount = document.getElementById('spTabCount');
const spCdpCount = document.getElementById('spCdpCount');
const spMsgCount = document.getElementById('spMsgCount');
const spLog = document.getElementById('spLog');
const spCmdInput = document.getElementById('spCmdInput');
const spCmdSend = document.getElementById('spCmdSend');

// ---- State ----
let port = null;
let msgCount = 0;
let logEntries = [];
const MAX_LOG = 200;

// ---- Connect to background ----
function connectPort() {
  port = chrome.runtime.connect({ name: 'sidepanel' });

  port.onMessage.addListener((msg) => {
    if (msg._log) {
      addLog(msg._log.dir, msg._log.type, msg._log.detail);
      return;
    }
    if (msg._status) {
      updateStatus(msg._status);
      return;
    }
    // Generic relay message — log it.
    msgCount++;
    spMsgCount.textContent = String(msgCount);
    addLog('in', msg.type || 'unknown', truncate(JSON.stringify(msg), 120));
  });

  port.onDisconnect.addListener(() => {
    addLog('sys', 'disconnect', 'Background port closed');
    port = null;
    // Retry after a delay.
    setTimeout(connectPort, 2000);
  });

  addLog('sys', 'connect', 'Side panel connected to background');
  requestStatus();
}

function requestStatus() {
  chrome.runtime.sendMessage({ action: 'getStatus' }, (resp) => {
    if (resp) updateStatus(resp);
  });
}

function updateStatus(s) {
  const mode = s.connectionMode || 'none';
  if (mode === 'native') {
    spStatus.textContent = 'NATIVE';
    spStatus.className = 'sp-status native';
  } else if (mode === 'websocket') {
    spStatus.textContent = 'WS';
    spStatus.className = 'sp-status websocket';
  } else {
    spStatus.textContent = 'OFF';
    spStatus.className = 'sp-status off';
  }

  spCdpCount.textContent = String(s.attachedTabs?.length || 0);
}

// ---- Log ----
function addLog(dir, type, detail) {
  const now = new Date();
  const time = now.toLocaleTimeString('en-US', {
    hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit',
  });

  logEntries.push({ time, dir, type, detail });
  if (logEntries.length > MAX_LOG) logEntries.shift();

  renderLogItem({ time, dir, type, detail });
}

function renderLogItem(entry) {
  // Remove empty placeholder if present.
  const empty = spLog.querySelector('.sp-empty');
  if (empty) empty.remove();

  const item = document.createElement('div');
  item.className = 'sp-log-item';

  const t = document.createElement('span');
  t.className = 'sp-log-time';
  t.textContent = entry.time;

  const d = document.createElement('span');
  d.className = 'sp-log-dir ' + entry.dir;
  d.textContent = entry.dir === 'in' ? '<' : entry.dir === 'out' ? '>' : entry.dir === 'sys' ? '*' : '!';

  const tp = document.createElement('span');
  tp.className = 'sp-log-type';
  tp.textContent = entry.type;

  const dt = document.createElement('span');
  dt.className = 'sp-log-detail';
  dt.textContent = entry.detail || '';

  item.appendChild(t);
  item.appendChild(d);
  item.appendChild(tp);
  item.appendChild(dt);
  spLog.appendChild(item);

  // Auto-scroll to bottom.
  spLog.scrollTop = spLog.scrollHeight;
}

// ---- Command Input ----
function sendCommand() {
  const raw = spCmdInput.value.trim();
  if (!raw) return;

  try {
    const msg = JSON.parse(raw);
    chrome.runtime.sendMessage({ action: 'relayCommand', payload: msg });
    addLog('out', msg.type || 'cmd', truncate(raw, 120));
    spCmdInput.value = '';
  } catch {
    addLog('err', 'parse', 'Invalid JSON: ' + truncate(raw, 80));
  }
}

spCmdSend.addEventListener('click', sendCommand);
spCmdInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') sendCommand();
});

// ---- Tab count polling ----
async function updateTabCount() {
  try {
    const tabs = await chrome.tabs.query({});
    spTabCount.textContent = String(tabs.length);
  } catch {
    // ignore
  }
}

// ---- Helpers ----
function truncate(s, max) {
  return s.length > max ? s.slice(0, max) + '...' : s;
}

// ---- Init ----
connectPort();
updateTabCount();

// Refresh status periodically.
setInterval(() => {
  requestStatus();
  updateTabCount();
}, 5000);
