import React, { useState, useRef } from 'react';
import './CommandBar.css';

const PRESETS = [
  { label: '列出标签', cmd: { type: 'list_tabs' } },
  { label: '截图', cmd: { type: 'cs_screenshot' } },
  { label: '读取文本', cmd: { type: 'cs_get_text' } },
  { label: 'SOM标注', cmd: { type: 'cs_annotate_som' } },
  { label: '清除标注', cmd: { type: 'cs_clear_annotations' } },
];

export default function CommandBar({ onSend }) {
  const [input, setInput] = useState('');
  const [history, setHistory] = useState([]);
  const [histIdx, setHistIdx] = useState(-1);
  const inputRef = useRef(null);

  const send = () => {
    const raw = input.trim();
    if (!raw) return;
    try {
      const msg = JSON.parse(raw);
      onSend(msg);
      setHistory((h) => [...h.slice(-20), raw]);
      setHistIdx(-1);
      setInput('');
    } catch {
      // Try as shorthand: type only
      if (!raw.includes('{')) {
        onSend({ type: raw });
        setHistory((h) => [...h.slice(-20), raw]);
        setHistIdx(-1);
        setInput('');
      }
    }
  };

  const handleKey = (e) => {
    if (e.key === 'Enter') {
      send();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (history.length > 0) {
        const idx = histIdx < 0 ? history.length - 1 : Math.max(0, histIdx - 1);
        setHistIdx(idx);
        setInput(history[idx]);
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (histIdx >= 0) {
        const idx = histIdx + 1;
        if (idx >= history.length) {
          setHistIdx(-1);
          setInput('');
        } else {
          setHistIdx(idx);
          setInput(history[idx]);
        }
      }
    }
  };

  return (
    <div className="cmd-bar">
      <div className="cmd-presets">
        {PRESETS.map((p) => (
          <button
            key={p.label}
            className="cmd-preset"
            onClick={() => onSend(p.cmd)}
            title={JSON.stringify(p.cmd)}
          >
            {p.label}
          </button>
        ))}
      </div>
      <div className="cmd-input-row">
        <input
          ref={inputRef}
          className="cmd-input"
          placeholder='输入 JSON 命令 或 类型名（如 list_tabs）'
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKey}
        />
        <button className="cmd-send" onClick={send}>发送</button>
      </div>
    </div>
  );
}
