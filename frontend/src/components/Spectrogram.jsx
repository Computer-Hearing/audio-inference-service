import { useEffect, useRef } from 'react';

const CATEGORY = { 0: 'car', 1: 'emv', 2: 'motorcycle', 3: 'tram', 4: 'truck' };
const TARGET = {
  0: 'acceleration', 1: 'bell', 2: 'braking', 3: 'horn',
  4: 'idling', 5: 'passing', 6: 'siren',
};

export default function Spectrogram({ spectrogram, chunks, duration, currentTime }) {
  const canvasRef = useRef(null);

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

  const totalChunks = chunks ? chunks.length : 0;

  return (
    <div className="card">
      <strong>Спектрограмма</strong>
      <canvas
        ref={canvasRef}
        style={{ width: '100%', height: 160, display: 'block', marginTop: 8 }}
      />
      {totalChunks > 0 && (
        <div style={{ display: 'flex', marginTop: 8 }}>
          {chunks.map((ch, i) => {
            const catIdx = ch.category.indexOf(Math.max(...ch.category));
            const tgtIdx = ch.target.indexOf(Math.max(...ch.target));
            const isActive =
              duration > 0 &&
              (i / totalChunks) * duration <= currentTime &&
              ((i + 1) / totalChunks) * duration > currentTime;
            return (
              <div
                key={ch.chunk_index || i}
                style={{
                  flex: 1,
                  background: isActive ? '#2a3a46' : '#18212a',
                  border: '1px solid #2e3a44',
                  padding: '6px 4px',
                  fontSize: 11,
                  color: isActive ? '#ffcc66' : '#d5dde4',
                  overflow: 'hidden',
                  opacity: ch.error ? 0.5 : 1,
                }}
              >
                <div>{CATEGORY[catIdx] || '?'}</div>
                <div>{TARGET[tgtIdx] || '?'}</div>
                {ch.error && <div style={{ color: '#f5785a', fontSize: 9 }}>err</div>}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}