import React, { useState } from 'react';
import { useBackgroundPort } from './hooks/useBackgroundPort';
import Header from './components/Header';
import TabList from './components/TabList';
import LogStream from './components/LogStream';
import ScreenshotViewer from './components/ScreenshotViewer';
import CommandBar from './components/CommandBar';
import PlanViewer from './components/PlanViewer';
import './styles/app.css';

const PANELS = ['log', 'tabs', 'screenshot', 'plan'];

export default function App() {
  const bg = useBackgroundPort();
  const [activePanel, setActivePanel] = useState('log');
  const [screenshot, setScreenshot] = useState(null);
  const [plan, setPlan] = useState(null);

  // Intercept screenshot and plan messages from log.
  React.useEffect(() => {
    // Check latest log for screenshots.
    if (bg.logs.length > 0) {
      const last = bg.logs[bg.logs.length - 1];
      if (last.type === 'cs_screenshot' && last.detail) {
        try {
          const data = JSON.parse(last.detail);
          if (data.result?.dataUrl) {
            setScreenshot({ url: data.result.dataUrl, time: last.time });
          }
        } catch { /* ignore */ }
      }
    }
  }, [bg.logs]);

  return (
    <div className="app">
      <Header
        status={bg.status}
        onRefresh={() => { bg.refreshStatus(); bg.refreshTabs(); }}
      />

      <nav className="panel-tabs">
        {PANELS.map((p) => (
          <button
            key={p}
            className={`panel-tab ${activePanel === p ? 'active' : ''}`}
            onClick={() => setActivePanel(p)}
          >
            {p === 'log' ? '日志' : p === 'tabs' ? '标签页' : p === 'screenshot' ? '截图' : '计划'}
          </button>
        ))}
      </nav>

      <div className="panel-content">
        {activePanel === 'log' && (
          <LogStream logs={bg.logs} onClear={bg.clearLogs} />
        )}
        {activePanel === 'tabs' && (
          <TabList
            tabs={bg.tabs}
            attachedTabs={bg.status.attachedTabs || []}
            onToggle={bg.toggleTab}
          />
        )}
        {activePanel === 'screenshot' && (
          <ScreenshotViewer
            screenshot={screenshot}
            onCapture={() => bg.sendCommand({ type: 'cs_screenshot' })}
          />
        )}
        {activePanel === 'plan' && (
          <PlanViewer plan={plan} />
        )}
      </div>

      <CommandBar onSend={bg.sendCommand} />
    </div>
  );
}
