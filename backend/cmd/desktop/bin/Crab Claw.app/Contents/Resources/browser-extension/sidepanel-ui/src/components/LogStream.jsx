import React, { useRef, useEffect, useState } from 'react';
import './LogStream.css';

const DIR_ICONS = { in: '<', out: '>', sys: '*', err: '!' };
const DIR_CLASSES = { in: 'in', out: 'out', sys: 'sys', err: 'err' };

export default function LogStream({ logs, onClear }) {
  const listRef = useRef(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [filter, setFilter] = useState('');

  // Auto-scroll on new logs.
  useEffect(() => {
    if (autoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // Detect manual scroll.
  const handleScroll = () => {
    if (!listRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = listRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40);
  };

  const filtered = filter
    ? logs.filter((l) =>
        l.type.toLowerCase().includes(filter) ||
        (l.detail || '').toLowerCase().includes(filter)
      )
    : logs;

  return (
    <div className="log-panel">
      <div className="log-toolbar">
        <input
          className="log-filter"
          placeholder="过滤日志..."
          value={filter}
          onChange={(e) => setFilter(e.target.value.toLowerCase())}
        />
        <span className="log-count">{filtered.length}</span>
        <button className="log-clear" onClick={onClear}>清空</button>
      </div>

      <div className="log-list" ref={listRef} onScroll={handleScroll}>
        {filtered.length === 0 ? (
          <div className="log-empty">等待活动...</div>
        ) : (
          filtered.map((entry) => (
            <div key={entry.id} className="log-row">
              <span className="log-time">{entry.time}</span>
              <span className={`log-dir ${DIR_CLASSES[entry.dir] || ''}`}>
                {DIR_ICONS[entry.dir] || '?'}
              </span>
              <span className="log-type">{entry.type}</span>
              <span className="log-detail">{entry.detail || ''}</span>
            </div>
          ))
        )}
      </div>

      {!autoScroll && (
        <button
          className="log-scroll-btn"
          onClick={() => {
            setAutoScroll(true);
            if (listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight;
          }}
        >
          ↓ 滚到底部
        </button>
      )}
    </div>
  );
}
