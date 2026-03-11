import React from 'react';
import './PlanViewer.css';

const STATUS_ICONS = {
  pending: '○',
  running: '◉',
  done: '●',
  failed: '✕',
  skipped: '–',
};

const STATUS_CLASSES = {
  pending: 'pending',
  running: 'running',
  done: 'done',
  failed: 'failed',
  skipped: 'skipped',
};

export default function PlanViewer({ plan }) {
  if (!plan || !plan.steps || plan.steps.length === 0) {
    return (
      <div className="plan-panel">
        <div className="plan-empty">
          暂无执行计划。<br />
          Agent 发送计划后将在此显示。
        </div>
      </div>
    );
  }

  const done = plan.steps.filter((s) => s.status === 'done').length;
  const total = plan.steps.length;

  return (
    <div className="plan-panel">
      <div className="plan-header">
        <div className="plan-title">{plan.title || '执行计划'}</div>
        <div className="plan-progress">
          <div className="plan-bar">
            <div className="plan-bar-fill" style={{ width: `${(done / total) * 100}%` }} />
          </div>
          <span className="plan-count">{done}/{total}</span>
        </div>
      </div>

      <div className="plan-steps">
        {plan.steps.map((step, i) => (
          <div key={i} className={`plan-step ${STATUS_CLASSES[step.status] || 'pending'}`}>
            <span className="step-icon">{STATUS_ICONS[step.status] || '○'}</span>
            <div className="step-info">
              <div className="step-name">{step.name || `步骤 ${i + 1}`}</div>
              {step.detail && <div className="step-detail">{step.detail}</div>}
              {step.error && <div className="step-error">{step.error}</div>}
            </div>
            {step.duration && (
              <span className="step-duration">{step.duration}</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
