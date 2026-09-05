import { useEffect, useRef, useState } from 'react';

const CATEGORY = { 0: 'car', 1: 'emv', 2: 'motorcycle', 3: 'tram', 4: 'truck' };
const TARGET = {
  0: 'acceleration', 1: 'bell', 2: 'braking', 3: 'horn',
  4: 'idling', 5: 'passing', 6: 'siren',
};

export default function Spectrogram({ spectrogram, chunks, duration, currentTime }) {
  const canvasRef = useRef(null);
  const [visibleLayers, setVisibleLayers] = useState(new Set([0, 1]));

  // группируем чанки по слоям
  const layersMap = {};
  (chunks || []).forEach((ch) => {
    const l = ch.layer ?? 0;
    (layersMap[l] = layersMap[l] || []).push(ch);
  });
  const layerIds = Object.keys(layersMap).map(Number).sort((a, b) => a - b);

  // ---- отрисовка столбиков спектрограммы ----
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
      const h = t * (H - 10);
      const r = Math.round(40 + t * 200);
      const g = Math.round(180 - t * 60);
      const b = Math.round(220 - t * 140);
      ctx.fillStyle = `rgb(${r},${g},${b})`;
      ctx.fillRect(i * barW, H - h, Math.max(1, barW - 1), h);
    }

    // жёлтая линия — текущее время
    if (duration > 0) {
      const x = (currentTime / duration) * W;
      ctx.strokeStyle = '#ffcc66';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, H);
      ctx.stroke();
    }
  }, [spectrogram, currentTime, duration]);

  const toggleLayer = (l) => {
    setVisibleLayers((prev) => {
      const next = new Set(prev);
      if (next.has(l)) next.delete(l);
      else next.add(l);
      return next;
    });
  };

  // базовый слой (layer 0) для расчёта позиций
  const baseLayer = layersMap[0] || [];
  const baseCount = baseLayer.length;
  const chunkSec = duration > 0 && baseCount > 0 ? duration / baseCount : 2;

  return (
    <div className="card">
      <strong>Спектрограмма</strong>
      <canvas
        ref={canvasRef}
        style={{ width: '100%', height: 160, display: 'block', marginTop: 8 }}
      />

      {/* переключатели слоёв */}
      {layerIds.length > 1 && (
        <div className="row" style={{ marginTop: 8 }}>
          <span className="muted">Слои:</span>
          {layerIds.map((l) => {
            const offset = (layersMap[l]?.[0]?.offset ?? 0);
            return (
              <button
                key={l}
                onClick={() => toggleLayer(l)}
                style={
                  visibleLayers.has(l)
                    ? { background: '#2a3a46', color: '#ffcc66' }
                    : { opacity: 0.5 }
                }
              >
                {l} (offset {offset}s)
              </button>
            );
          })}
        </div>
      )}

      {/* пирамида слоёв: сверху вниз (layer N, N-1, ..., 0) */}
      {layerIds.length > 0 && baseCount > 0 && (
        <div style={{ marginTop: 12 }}>
          {[...layerIds].reverse().map((layerId) => {
            if (!visibleLayers.has(layerId)) return null;
            const layerChunks = layersMap[layerId] || [];
            const offset = layerChunks[0]?.offset ?? 0;

            return (
              <div key={layerId} style={{ position: 'relative', height: 56, marginBottom: 4 }}>
                <div
                  className="muted"
                  style={{
                    position: 'absolute',
                    left: 0,
                    top: 0,
                    fontSize: 10,
                    color: '#7a8a99',
                  }}
                >
                  L{layerId}
                </div>
                {layerChunks.map((ch, i) => {
                  const startSec = offset + ch.chunk_index * chunkSec;
                  const endSec = startSec + chunkSec;
                  const isActive =
                    chunkSec > 0 && currentTime >= startSec && currentTime < endSec;

                  const left = (startSec / duration) * 100;
                  const width = (chunkSec / duration) * 100;

                  const catIdx = ch.category?.indexOf(Math.max(...ch.category)) ?? -1;
                  const tgtIdx = ch.target?.indexOf(Math.max(...ch.target)) ?? -1;

                  return (
                    <div
                      key={`${layerId}-${ch.chunk_index}-${i}`}
                      style={{
                        position: 'absolute',
                        top: 16,
                        bottom: 0,
                        left: `${left}%`,
                        width: `calc(${width}% - 2px)`,
                        background: isActive ? '#2a3a46' : '#18212a',
                        border: '1px solid #2e3a44',
                        padding: '4px 4px',
                        fontSize: 10,
                        color: isActive ? '#ffcc66' : '#d5dde4',
                        overflow: 'hidden',
                        opacity: ch.error ? 0.5 : 1,
                      }}
                    >
                      <div>{CATEGORY[catIdx] || '?'}</div>
                      <div>{TARGET[tgtIdx] || '?'}</div>
                      {ch.error && <div style={{ color: '#f5785a', fontSize: 8 }}>err</div>}
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
