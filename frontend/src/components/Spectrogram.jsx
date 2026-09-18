// src/components/Spectrogram.jsx
import { useEffect, useRef, useState } from 'react';

const CATEGORY = { 0: 'car', 1: 'emv', 2: 'motorcycle', 3: 'tram', 4: 'truck' };
const TARGET = {
  0: 'acceleration', 1: 'bell', 2: 'braking', 3: 'horn',
  4: 'idling', 5: 'passing', 6: 'siren',
};

// ---------------------------------------------------------------------------
// Цветовая карта амплитуды.
// Раньше цвета были декоративным произвольным сине-фиолетовым градиентом.
// Сейчас — стандартная для аудио-спектрограмм логика "тихо → громко":
// тёмный синий (низкая энергия) → бирюза → зелёный → жёлтый → оранжевый → красный
// (пик). Это ближе к общепринятым колормапам (turbo/inferno), используемым
// в аудио-анализаторах, и читается интуитивно: чем "теплее" цвет, тем громче.
// ---------------------------------------------------------------------------
const COLOR_STOPS = [
  { t: 0.0, c: [10, 14, 30] },
  { t: 0.16, c: [32, 64, 128] },
  { t: 0.36, c: [22, 138, 148] },
  { t: 0.56, c: [72, 176, 92] },
  { t: 0.74, c: [232, 202, 42] },
  { t: 0.89, c: [240, 122, 32] },
  { t: 1.0, c: [230, 48, 56] },
];

function colormap(t) {
  t = Math.min(1, Math.max(0, t || 0));
  for (let i = 0; i < COLOR_STOPS.length - 1; i++) {
    const a = COLOR_STOPS[i];
    const b = COLOR_STOPS[i + 1];
    if (t >= a.t && t <= b.t) {
      const lt = (t - a.t) / (b.t - a.t || 1);
      return [
        Math.round(a.c[0] + (b.c[0] - a.c[0]) * lt),
        Math.round(a.c[1] + (b.c[1] - a.c[1]) * lt),
        Math.round(a.c[2] + (b.c[2] - a.c[2]) * lt),
      ];
    }
  }
  return COLOR_STOPS[COLOR_STOPS.length - 1].c;
}

// factor > 1 — светлее (к белому), factor < 1 — темнее (к чёрному)
function shade([r, g, b], factor) {
  if (factor >= 1) {
    const f = factor - 1;
    return `rgb(${r + (255 - r) * f}, ${g + (255 - g) * f}, ${b + (255 - b) * f})`;
  }
  return `rgb(${r * factor}, ${g * factor}, ${b * factor})`;
}

function rgba([r, g, b], a) {
  return `rgba(${r}, ${g}, ${b}, ${a})`;
}

// Уменьшаем число столбцов до аккуратного количества "объёмных" блоков —
// с сотнями тонких палок 3D-эффект не читается, а с ~60-80 крупными
// блоками спектрограмма выглядит как объёмный эквалайзер.
function resample(arr, targetCount) {
  if (!arr || arr.length <= targetCount) return arr || [];
  const out = new Array(targetCount).fill(0);
  const bucket = arr.length / targetCount;
  for (let i = 0; i < targetCount; i++) {
    const start = Math.floor(i * bucket);
    const end = Math.max(start + 1, Math.floor((i + 1) * bucket));
    let sum = 0;
    let count = 0;
    for (let j = start; j < end && j < arr.length; j++) {
      sum += arr[j];
      count++;
    }
    out[i] = count ? sum / count : 0;
  }
  return out;
}

// Полифилл roundRect выполняем один раз на модуль, а не при каждом рендере.
if (typeof CanvasRenderingContext2D !== 'undefined' && !CanvasRenderingContext2D.prototype.roundRect) {
  CanvasRenderingContext2D.prototype.roundRect = function (x, y, w, h, r) {
    if (r > w / 2) r = w / 2;
    if (r > h / 2) r = h / 2;
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

const MAX_BARS = 72;
const DEPTH = 7; // px "толщина" объёмного блока (смещение изометрической грани)

// Рисует один "объёмный" столбец: лицевая грань + верхняя грань + боковая грань.
function drawBlock(ctx, x, w, top, floor, color) {
  const h = floor - top;
  if (h <= 0) return;

  // Лицевая грань — вертикальный градиент (ярче сверху, темнее у основания)
  const front = ctx.createLinearGradient(0, top, 0, floor);
  front.addColorStop(0, shade(color, 1.12));
  front.addColorStop(1, shade(color, 0.5));
  ctx.fillStyle = front;
  ctx.fillRect(x, top, w, h);

  // Верхняя грань (параллелограмм) — имитирует свет сверху-слева, даёт объём
  ctx.beginPath();
  ctx.moveTo(x, top);
  ctx.lineTo(x + DEPTH, top - DEPTH);
  ctx.lineTo(x + w + DEPTH, top - DEPTH);
  ctx.lineTo(x + w, top);
  ctx.closePath();
  ctx.fillStyle = shade(color, 1.4);
  ctx.fill();

  // Боковая грань (правая) — теневая сторона блока
  ctx.beginPath();
  ctx.moveTo(x + w, top);
  ctx.lineTo(x + w + DEPTH, top - DEPTH);
  ctx.lineTo(x + w + DEPTH, floor - DEPTH);
  ctx.lineTo(x + w, floor);
  ctx.closePath();
  ctx.fillStyle = shade(color, 0.6);
  ctx.fill();

  // Тонкий блик по верхней кромке лицевой грани
  ctx.fillStyle = rgba(color, 0.12);
  ctx.fillRect(x, top, w, Math.min(3, h));
}

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

  // Отрисовка объёмной спектрограммы
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

    // Фон под canvas — совпадает с фоном страницы, чтобы отражение снизу
    // могло красиво "затухать" в него.
    ctx.fillStyle = '#0a0a0a';
    ctx.fillRect(0, 0, W, H);

    const bars = resample(spectrogram, MAX_BARS);
    const barCount = bars.length;
    const max = Math.max(...bars, 0.001);

    const topPadding = 26; // место для верхних граней блоков и искр
    const floorY = H * 0.66; // линия "пола", ниже неё — отражение
    const usableH = floorY - topPadding;

    const totalGap = W - DEPTH; // резервируем DEPTH под последнюю изометрическую грань
    const slot = totalGap / barCount;
    const barW = Math.max(1, slot * 0.72);

    const amps = []; // запоминаем для отражения и искр

    for (let i = 0; i < barCount; i++) {
      const t = bars[i] / max;
      const h = Math.max(2, t * usableH);
      const x = i * slot;
      const top = floorY - h;
      const color = colormap(t);
      drawBlock(ctx, x, barW, top, floorY, color);
      amps.push({ x, barW, t, top, color });
    }

    // "Пол" — тонкая линия основания
    ctx.strokeStyle = 'rgba(255,255,255,0.06)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, floorY);
    ctx.lineTo(W, floorY);
    ctx.stroke();

    // Отражение блоков в "полу" — затухающее к низу, усиливает ощущение объёма
    ctx.save();
    ctx.globalAlpha = 0.22;
    amps.forEach(({ x, barW: w, t, color }) => {
      const reflH = Math.max(2, t * (H - floorY) * 0.9);
      const grad = ctx.createLinearGradient(0, floorY, 0, floorY + reflH);
      grad.addColorStop(0, shade(color, 0.9));
      grad.addColorStop(1, 'rgba(10,10,10,0)');
      ctx.fillStyle = grad;
      ctx.fillRect(x, floorY, w, reflH);
    });
    ctx.restore();

    // Искры над пиками — светящиеся частицы, цвет берём из той же колормапы
    const particleCount = 46;
    for (let i = 0; i < particleCount; i++) {
      const src = amps[Math.floor(Math.random() * amps.length)];
      if (!src || src.t < 0.18) continue;
      const cx = src.x + src.barW / 2 + (Math.random() - 0.5) * src.barW;
      const cy = src.top - Math.random() * 16 * src.t;
      const radius = 1.2 + src.t * 3;
      const alpha = 0.25 + src.t * 0.5;
      const [r, g, b] = colormap(Math.min(1, src.t + 0.1));
      ctx.beginPath();
      ctx.arc(cx, cy, radius, 0, Math.PI * 2);
      ctx.fillStyle = rgba([r, g, b], alpha);
      ctx.fill();
      const glow = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius * 3.2);
      glow.addColorStop(0, rgba([r, g, b], alpha * 0.35));
      glow.addColorStop(1, rgba([r, g, b], 0));
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(cx, cy, radius * 3.2, 0, Math.PI * 2);
      ctx.fill();
    }

    // Линия текущего времени воспроизведения
    if (duration > 0) {
      const x = (currentTime / duration) * W;
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 1;
      ctx.shadowColor = '#ffffff';
      ctx.shadowBlur = 10;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, H);
      ctx.stroke();
      ctx.shadowBlur = 0;
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

  const baseLayer = layersMap[0] || [];
  const baseCount = baseLayer.length;
  const chunkSec = duration > 0 && baseCount > 0 ? duration / baseCount : 2;

  return (
    <div className="card">
      <strong style={{ color: '#aaa', letterSpacing: 1 }}>spectrogram</strong>
      <div className="spectrogram-canvas-wrap">
        <canvas
          ref={canvasRef}
          style={{ width: '100%', height: 220, display: 'block', marginTop: 10 }}
        />
      </div>

      {/* Легенда цветовой шкалы амплитуды */}
      <div className="spectrogram-legend">
        <span className="muted">quiet</span>
        <div className="spectrogram-legend-bar" />
        <span className="muted">loud</span>
      </div>

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

                  const catIdx = ch.category.indexOf(Math.max(...ch.category));
                  const tgtIdx = ch.target.indexOf(Math.max(...ch.target));

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
