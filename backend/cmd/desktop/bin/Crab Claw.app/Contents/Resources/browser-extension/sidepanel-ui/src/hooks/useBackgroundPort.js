import { useState, useEffect, useCallback, useRef } from 'react';

/**
 * Hook: connects to background.js via chrome.runtime port.
 * Provides status, log entries, and methods to send commands.
 */
export function useBackgroundPort() {
  const [status, setStatus] = useState({
    connectionMode: 'none',
    attachedTabs: [],
    contentScripts: false,
  });
  const [logs, setLogs] = useState([]);
  const [tabs, setTabs] = useState([]);
  const portRef = useRef(null);
  const maxLogs = 500;

  // Add a log entry.
  const addLog = useCallback((dir, type, detail) => {
    const now = new Date();
    const time = now.toLocaleTimeString('en-US', {
      hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
    const ms = String(now.getMilliseconds()).padStart(3, '0');
    setLogs((prev) => {
      const next = [...prev, { id: Date.now() + Math.random(), time: `${time}.${ms}`, dir, type, detail }];
      return next.length > maxLogs ? next.slice(-maxLogs) : next;
    });
  }, []);

  // Connect port.
  useEffect(() => {
    // chrome.runtime may not exist in dev mode.
    if (typeof chrome === 'undefined' || !chrome.runtime?.connect) {
      addLog('sys', 'warn', 'Not running in Chrome extension context');
      return;
    }

    function connect() {
      const port = chrome.runtime.connect({ name: 'sidepanel' });
      portRef.current = port;

      port.onMessage.addListener((msg) => {
        if (msg._log) {
          addLog(msg._log.dir, msg._log.type, msg._log.detail);
          return;
        }
        if (msg._status) {
          setStatus(msg._status);
          return;
        }
        // Generic message from relay.
        addLog('in', msg.type || '?', JSON.stringify(msg).slice(0, 200));
      });

      port.onDisconnect.addListener(() => {
        addLog('sys', 'disconnect', 'Port closed, reconnecting...');
        portRef.current = null;
        setTimeout(connect, 2000);
      });

      addLog('sys', 'connect', 'Side panel port connected');
      refreshStatus();
      refreshTabs();
    }

    connect();
    const interval = setInterval(() => {
      refreshStatus();
      refreshTabs();
    }, 5000);

    return () => clearInterval(interval);
  }, [addLog]);

  // Refresh status from background.
  const refreshStatus = useCallback(() => {
    if (typeof chrome === 'undefined' || !chrome.runtime?.sendMessage) return;
    chrome.runtime.sendMessage({ action: 'getStatus' }, (resp) => {
      if (resp) setStatus(resp);
    });
  }, []);

  // Refresh tab list.
  const refreshTabs = useCallback(async () => {
    if (typeof chrome === 'undefined' || !chrome.tabs?.query) return;
    try {
      const allTabs = await chrome.tabs.query({});
      setTabs(allTabs.map((t) => ({
        id: t.id,
        url: t.url || '',
        title: t.title || '',
        active: t.active,
        favIconUrl: t.favIconUrl,
      })));
    } catch { /* ignore */ }
  }, []);

  // Send relay command.
  const sendCommand = useCallback((payload) => {
    if (typeof chrome === 'undefined' || !chrome.runtime?.sendMessage) return;
    chrome.runtime.sendMessage({ action: 'relayCommand', payload });
    addLog('out', payload.type || 'cmd', JSON.stringify(payload).slice(0, 200));
  }, [addLog]);

  // Toggle tab CDP attach.
  const toggleTab = useCallback((tabId) => {
    if (typeof chrome === 'undefined' || !chrome.runtime?.sendMessage) return;
    chrome.runtime.sendMessage({ action: 'toggleTab', tabId }, () => {
      addLog('out', 'toggleTab', `tabId=${tabId}`);
      refreshStatus();
    });
  }, [addLog, refreshStatus]);

  // Clear logs.
  const clearLogs = useCallback(() => setLogs([]), []);

  return { status, logs, tabs, addLog, sendCommand, toggleTab, clearLogs, refreshStatus, refreshTabs };
}
