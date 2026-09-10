// src/components/Spectrogram.jsx
import { useEffect, useRef, useState } from 'react';

const CATEGORY = { 0: 'car', 1: 'emv', 2: 'motorcycle', 3: 'tram', 4: 'truck' };
const TARGET = {
  0: 'acceleration', 1: 'bell', 2: 'braking', 3: 'horn',
  4: 'idling', 5: 'passing', 6: 'siren',
};

export default function Spectrogram({ spectrogram, chunks, duration, currentTime }) {
  const canvasRef = useRef(null);
  const [visibleLayers, setVisibleLayers] = useState(new Set([0, 1]));

  // Группировка по слоям
  const layersMap = {};
  (chunks || []).forEach((ch) => {
    const l = ch.layer ?? 0;
    (layersMap[l] = layersMap[l] || []).push(ch);
  });
  const layerIds = Object.keys(layersMap).map(Number).sort((a, b) => a - b);

  // Отрисовка 3D-спектрограммы с частицами
  useEffect(() => {
    const cv = canvasRef.current;
    if (!cv || !spectrogram || !spectrogram.length) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const W = cv.clientWidth;
    const H = cv.clientHeight;
    cv.width = W * dpr;
    cv.height = H * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    const bars = spectrogram.length;
    const barW = W / bars;
    const max = Math.max(...spectrogram, 0.001);

    for (let i = 0; i < bars; i++) {
      const t = spectrogram[i] / max;
      const h = t * (H - 20);
      const grad = ctx.createLinearGradient(0, H - h, 0, H);
      const r = Math.round(30 + t * 200);
      const g = Math.round(100 + t * 100);
      const b = Math.round(200 + t * 55);
      grad.addColorStop(0, `rgb(${r}, ${g}, ${b})`);
      grad.addColorStop(1, `rgb(${Math.round(r*0.4)}, ${Math.round(g*0.3)}, ${Math.round(b*0.5)})`);

      ctx.fillStyle = grad;
      const x = i * barW;
      const y = H - h;
      const w = Math.max(1, barW - 1.5);
      ctx.beginPath();
      // roundRect polyfill (если нужно)
      if (ctx.roundRect) {
        ctx.roundRect(x, y, w, h, 4);
      } else {
        ctx.rect(x, y, w, h);
      }
      ctx.fill();

      // Блик
      ctx.fillStyle = `rgba(255,255,255,${0.05 + t * 0.15})`;
      ctx.beginPath();
      ctx.rect(x + 2, y + 2, w * 0.3, 4);
      ctx.fill();
    }

    // Частицы
    const particleCount = 80;
    for (let i = 0; i < particleCount; i++) {
      const idx = Math.floor(Math.random() * bars);
      const t = spectrogram[idx] / max;
      if (t < 0.15) continue;
      const x = idx * barW + barW / 2 + (Math.random() - 0.5) * barW * 1.2;
      const y = H - 10 - t * (H - 20) - Math.random() * 15 * t;
      const radius = 1.5 + t * 3.5;
      const alpha = 0.3 + t * 0.6;
      const hue = 210 + t * 60;
      ctx.beginPath();
      ctx.arc(x, y, radius, 0, Math.PI * 2);
      ctx.fillStyle = `hsla(${hue}, 90%, 70%, ${alpha})`;
      ctx.fill();
      const glow = ctx.createRadialGradient(x, y, 0, x, y, radius * 3);
      glow.addColorStop(0, `hsla(${hue}, 90%, 70%, ${alpha * 0.3})`);
      glow.addColorStop(1, `hsla(${hue}, 90%, 70%, 0)`);
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y, radius * 3, 0, Math.PI * 2);
      ctx.fill();
    }

    // Линия времени
    if (duration > 0) {
      const x = (currentTime / duration) * W;
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 1;
      ctx.shadowColor = '#ffffff';
      ctx.shadowBlur = 12;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, H);
      ctx.stroke();
      ctx.shadowBlur = 0;
    }

    // Сетка (очень слабая)
    ctx.strokeStyle = 'rgba(255,255,255,0.03)';
    ctx.lineWidth = 0.5;
    for (let i = 0; i < 10; i++) {
      const y = (i / 10) * H;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(W, y);
      ctx.stroke();
    }
  }, [spectrogram, currentTime, duration]);

  // Полифилл для roundRect, если браузер не поддерживает
  if (!CanvasRenderingContext2D.prototype.roundRect) {
    CanvasRenderingContext2D.prototype.roundRect = function (x, y, w, h, r) {
      if (r > w/2) r = w/2;
      if (r > h/2) r = h/2;
      this.moveTo(x + r, y);
      this.lineTo(x + w - r, y);
      this.quadraticCurveTo(x + w, y, x + w, y + r);
      this.lineTo(x + w, y + h - r);
      this.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
      this.lineTo(x + r, y + h);
      this.quadraticCurveTo(x, y + h, x, y + h - r);
      this.lineTo(x, y + r);
      this.quadraticCurveTo(x, y, x + r, y);
      this.closePath();
      return this;
    };
  }

  const toggleLayer = (l) => {
    setVisibleLayers((prev) => {
      const next = new Set(prev);
      if (next.has(l)) next.delete(l);
      else next.add(l);
      return next;
    });
  };

  const baseLayer = layersMap[0] || [];
  const baseCount = baseLayer.length;
  const chunkSec = duration > 0 && baseCount > 0 ? duration / baseCount : 2;

  return (
    <div className="card">
      <strong style={{ color: '#aaa', letterSpacing: 1 }}>spectrogram</strong>
      <canvas
        ref={canvasRef}
        style={{ width: '100%', height: 200, display: 'block', marginTop: 10 }}
      />

      {layerIds.length > 1 && (
        <div className="row" style={{ marginTop: 12 }}>
          <span className="muted">layers:</span>
          {layerIds.map((l) => {
            const offset = (layersMap[l]?.[0]?.offset ?? 0);
            return (
              <button
                key={l}
                onClick={() => toggleLayer(l)}
                style={
                  visibleLayers.has(l)
                    ? { color: '#fff', borderBottomColor: '#fff' }
                    : { opacity: 0.3 }
                }
              >
                {l} (offset {offset}s)
              </button>
            );
          })}
        </div>
      )}

      {layerIds.length > 0 && baseCount > 0 && (
        <div style={{ marginTop: 16 }}>
          {[...layerIds].reverse().map((layerId) => {
            if (!visibleLayers.has(layerId)) return null;
            const layerChunks = layersMap[layerId] || [];
            const offset = layerChunks[0]?.offset ?? 0;

            return (
              <div key={layerId} style={{ position: 'relative', height: 56, marginBottom: 6 }}>
                <div className="muted" style={{ position: 'absolute', left: 0, top: 0, fontSize: 10 }}>
                  L{layerId}
                </div>
                {layerChunks.map((ch, i) => {
                  const startSec = offset + ch.chunk_index * chunkSec;
                  const endSec = startSec + chunkSec;
                  const isActive = chunkSec > 0 && currentTime >= startSec && currentTime < endSec;

                  const left = (startSec / duration) * 100;
                  const width = (chunkSec / duration) * 100;

                  const catIdx = ch.category?.indexOf(Math.max(...ch.category)) ?? -1;
                  const tgtIdx = ch.target?.indexOf(Math.max(...ch.target)) ?? -1;

                  return (
                    <div
                      key={`${layerId}-${ch.chunk_index}-${i}`}
                      style={{
                        position: 'absolute',
                        top: 18,
                        bottom: 0,
                        left: `${left}%`,
                        width: `calc(${width}% - 2px)`,
                        background: isActive ? 'rgba(255,255,255,0.08)' : 'rgba(255,255,255,0.02)',
                        border: isActive ? '1px solid #fff' : '1px solid rgba(255,255,255,0.06)',
                        padding: '4px 6px',
                        fontSize: 10,
                        color: isActive ? '#fff' : '#aaa',
                        overflow: 'hidden',
                        opacity: ch.error ? 0.3 : 1,
                      }}
                    >
                      <div style={{ fontWeight: 600 }}>{CATEGORY[catIdx] || '?'}</div>
                      <div style={{ fontSize: 9, opacity: 0.7 }}>{TARGET[tgtIdx] || '?'}</div>
                      {ch.error && <div style={{ color: '#ff6b5a', fontSize: 8 }}>error</div>}
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}