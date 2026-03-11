import React from 'react';
import './Header.css';

const MODE_LABELS = {
  native: { text: 'NATIVE', cls: 'native' },
  websocket: { text: 'WS', cls: 'websocket' },
  none: { text: 'OFF', cls: 'off' },
};

export default function Header({ status, onRefresh }) {
  const mode = status.connectionMode || 'none';
  const label = MODE_LABELS[mode] || MODE_LABELS.none;
  const attached = status.attachedTabs?.length || 0;

  return (
    <header className="sp-header">
      <div className="sp-brand">
        <span className="sp-logo">CrabClaw</span>
        <span className="sp-version">v1.1</span>
      </div>

      <div className="sp-stats">
        {attached > 0 && (
          <span className="sp-stat">
            <span className="sp-stat-dot cdp" />
            {attached} CDP
          </span>
        )}
        {status.contentScripts && (
          <span className="sp-stat">
            <span className="sp-stat-dot cs" />
            CS
          </span>
        )}
      </div>

      <div className="sp-actions">
        <button className="sp-refresh" onClick={onRefresh} title="刷新">
          ↻
        </button>
        <span className={`sp-badge ${label.cls}`}>{label.text}</span>
      </div>
    </header>
  );
}
