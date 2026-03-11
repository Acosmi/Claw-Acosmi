import React, { useState, useRef } from 'react';
import './ScreenshotViewer.css';

export default function ScreenshotViewer({ screenshot, onCapture }) {
  const [zoom, setZoom] = useState(1);
  const [clickOverlays, setClickOverlays] = useState([]);
  const imgRef = useRef(null);

  const handleImageClick = (e) => {
    if (!imgRef.current) return;
    const rect = imgRef.current.getBoundingClientRect();
    const x = Math.round((e.clientX - rect.left) / zoom);
    const y = Math.round((e.clientY - rect.top) / zoom);
    const id = Date.now() + Math.random();
    setClickOverlays((prev) => [...prev, { x, y, id }]);

    // Auto-remove after 3 seconds.
    setTimeout(() => {
      setClickOverlays((prev) => prev.filter((o) => o.id !== id));
    }, 3000);
  };

  return (
    <div className="ss-panel">
      <div className="ss-toolbar">
        <button className="ss-btn capture" onClick={onCapture}>
          截图
        </button>
        <div className="ss-zoom">
          <button onClick={() => setZoom((z) => Math.max(0.25, z - 0.25))}>-</button>
          <span>{Math.round(zoom * 100)}%</span>
          <button onClick={() => setZoom((z) => Math.min(3, z + 0.25))}>+</button>
          <button onClick={() => setZoom(1)}>1:1</button>
        </div>
        {screenshot && (
          <span className="ss-time">{screenshot.time}</span>
        )}
      </div>

      <div className="ss-viewport">
        {screenshot ? (
          <div
            className="ss-image-wrapper"
            style={{ transform: `scale(${zoom})`, transformOrigin: 'top left' }}
          >
            <img
              ref={imgRef}
              className="ss-image"
              src={screenshot.url}
              alt="Page screenshot"
              onClick={handleImageClick}
            />
            {clickOverlays.map((o) => (
              <div
                key={o.id}
                className="ss-click-dot"
                style={{ left: o.x, top: o.y }}
              >
                <span className="ss-click-coords">({o.x}, {o.y})</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="ss-empty">
            点击"截图"捕获当前页面
          </div>
        )}
      </div>
    </div>
  );
}
