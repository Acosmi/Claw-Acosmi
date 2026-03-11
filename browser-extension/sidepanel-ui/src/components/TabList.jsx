import React, { useState, useMemo } from 'react';
import './TabList.css';

export default function TabList({ tabs, attachedTabs, onToggle }) {
  const [search, setSearch] = useState('');
  const attachedSet = useMemo(() => new Set(attachedTabs), [attachedTabs]);

  const filtered = useMemo(() => {
    const q = search.toLowerCase();
    const list = q
      ? tabs.filter((t) =>
          t.title.toLowerCase().includes(q) || t.url.toLowerCase().includes(q))
      : tabs;

    // Group: attached → active → http(s) → other
    const attached = [], active = [], pages = [], other = [];
    for (const t of list) {
      if (attachedSet.has(t.id)) attached.push(t);
      else if (t.active) active.push(t);
      else if (t.url.startsWith('http')) pages.push(t);
      else other.push(t);
    }
    return { attached, active, pages, other };
  }, [tabs, attachedSet, search]);

  return (
    <div className="tab-panel">
      <div className="tab-toolbar">
        <input
          className="tab-search"
          placeholder="搜索标签页..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <span className="tab-total">{tabs.length} 个</span>
      </div>

      <div className="tab-scroll">
        <TabGroup label="已附加 CDP" items={filtered.attached} attachedSet={attachedSet} onToggle={onToggle} color="green" />
        <TabGroup label="当前活动" items={filtered.active} attachedSet={attachedSet} onToggle={onToggle} color="blue" />
        <TabGroup label="页面" items={filtered.pages} attachedSet={attachedSet} onToggle={onToggle} color="purple" />
        <TabGroup label="其他" items={filtered.other} attachedSet={attachedSet} onToggle={onToggle} color="gray" />
      </div>
    </div>
  );
}

function TabGroup({ label, items, attachedSet, onToggle, color }) {
  if (items.length === 0) return null;

  return (
    <div className="tab-group">
      <div className="tab-group-label">
        <span className={`tab-group-dot ${color}`} />
        {label} ({items.length})
      </div>
      {items.map((tab) => (
        <TabRow
          key={tab.id}
          tab={tab}
          isAttached={attachedSet.has(tab.id)}
          onToggle={() => onToggle(tab.id)}
        />
      ))}
    </div>
  );
}

function TabRow({ tab, isAttached, onToggle }) {
  const canCS = tab.url.startsWith('http://') || tab.url.startsWith('https://');

  return (
    <div className="tab-row" onClick={onToggle}>
      {tab.favIconUrl && (
        <img className="tab-favicon" src={tab.favIconUrl} alt="" />
      )}
      <div className="tab-info">
        <div className="tab-title">{tab.title || '无标题'}</div>
        <div className="tab-url">{tab.url}</div>
      </div>
      <div className="tab-badges">
        {isAttached && <span className="badge cdp">CDP</span>}
        {canCS && <span className="badge cs">CS</span>}
      </div>
      <button
        className={`tab-action ${isAttached ? 'detach' : 'attach'}`}
        onClick={(e) => { e.stopPropagation(); onToggle(); }}
      >
        {isAttached ? '解除' : '附加'}
      </button>
    </div>
  );
}
