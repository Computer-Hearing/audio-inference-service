// src/components/Waveform.jsx
//
// Раньше компонент назывался Spectrogram, хотя настоящим спектрограммой
// (частота × время) не является: бэкенд отдаёт всего один плоский массив
// RMS-амплитуды по времени (pkg.AudioWaveBucketsLen = 40 "бакетов" на весь
// файл, см. chunks.audioWaveform на бэкенде). Это обычная диаграмма
// громкости во времени — переименовано в Waveform, чтобы не вводить в
// заблуждение.
//
// Главная идея переработки: диаграмма и ряды чанков ниже — это одна общая
// сетка по времени, а не два независимых виджета. Столбцы диаграммы
// пересчитываются (агрегируются) так, чтобы их границы ровно совпадали с
// границами чанков базового слоя (offset=0) — то есть чанки служат
// "засечками"/направляющими для графика. Слой со сдвигом +1s рисуется
// отдельной строкой прямо над базовым рядом чанков — как бы поверх сетки.
import { useEffect, useRef } from 'react';

const CATEGORY = { 0: 'car', 1: 'emv', 2: 'motorcycle', 3: 'tram', 4: 'truck' };
const TARGET = {
  0: 'acceleration', 1: 'bell', 2: 'braking', 3: 'horn',
  4: 'idling', 5: 'passing', 6: 'siren',
};

// Цветовая карта амплитуды: тихо (тёмно-синий) → громко (красный),
// стандартная для аудио-визуализаторов логика "холодный → тёплый".
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

function rgb([r, g, b], factor = 1) {
  if (factor >= 1) {
    const f = factor - 1;
    return `rgb(${r + (255 - r) * f}, ${g + (255 - g) * f}, ${b + (255 - b) * f})`;
  }
  return `rgb(${r * factor}, ${g * factor}, ${b * factor})`;
}

function argmaxIndex(arr) {
  if (!arr || !arr.length) return -1;
  let best = 0;
  for (let i = 1; i < arr.length; i++) if (arr[i] > arr[best]) best = i;
  return best;
}

// -----------------------------------------------------------------------
// Границы чанков по времени.
// Правило (согласовано с бэкендом, см. internal/chunks/audio_chunk.go):
//  - базовый слой (offset=0): чанки фиксированного размера chunkSeconds,
//    последний чанк может быть короче (хвост длительностью duration % chunkSeconds,
//    если этот хвост длится дольше ~0.5s — иначе бэкенд его просто отбрасывает,
//    и в chunks его тоже не будет).
//  - слои со сдвигом (offset=1s и т.д.): те же чанки фиксированного размера,
//    только считаются от отметки offset, и хвост НЕ создаётся (отбрасывается
//    целиком, если не наберётся полный chunkSeconds).
// Опираемся на chunk_index и offset, которые уже возвращает бэкенд —
// поэтому здесь просто считаем, а не гадаем количество чанков.
// -----------------------------------------------------------------------
function chunkRange(offset, chunkIndex, chunkSeconds, duration) {
  const start = offset + chunkIndex * chunkSeconds;
  const end = Math.min(duration, start + chunkSeconds);
  return { start, end: Math.max(start, end) };
}

// Пересчитывает 40-точечный (или любой другой длины) массив амплитуд,
// равномерно покрывающий [0, duration], в один агрегированный уровень на
// каждый переданный временной диапазон — усредняя перекрывающиеся "бакеты"
// пропорционально площади перекрытия. Так столбец диаграммы получает
// границы ровно там же, где границы чанка.
function resampleToRanges(waves, duration, ranges) {
  if (!waves || !waves.length || !duration) return ranges.map(() => 0);
  const n = waves.length;
  const bucketDur = duration / n;

  return ranges.map(({ start, end }) => {
    if (end <= start) return 0;
    let sum = 0;
    let weight = 0;
    for (let i = 0; i < n; i++) {
      const bStart = i * bucketDur;
      const bEnd = bStart + bucketDur;
      const overlap = Math.min(end, bEnd) - Math.max(start, bStart);
      if (overlap > 0) {
        sum += waves[i] * overlap;
        weight += overlap;
      }
    }
    return weight > 0 ? sum / weight : 0;
  });
}

export default function Waveform({ waves, chunks, duration, currentTime, chunkSeconds }) {
  const canvasRef = useRef(null);

  const baseChunks = (chunks || [])
    .filter((ch) => (ch.offset || 0) === 0)
    .slice()
    .sort((a, b) => a.chunk_index - b.chunk_index);

  const offsetChunks = (chunks || [])
    .filter((ch) => (ch.offset || 0) > 0)
    .slice()
    .sort((a, b) => a.chunk_index - b.chunk_index);
  const offsetSeconds = offsetChunks.length ? (offsetChunks[0].offset || 1) : 1;

  const baseRanges = baseChunks.map((ch) => chunkRange(0, ch.chunk_index, chunkSeconds, duration));
  const offsetRanges = offsetChunks.map((ch) => chunkRange(offsetSeconds, ch.chunk_index, chunkSeconds, duration));

  // Отрисовка диаграммы — плоские (2D) столбцы, без объёма, без "линии спектра".
  useEffect(() => {
    const cv = canvasRef.current;
    if (!cv || !duration || !baseRanges.length) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const W = cv.clientWidth;
    const H = cv.clientHeight;
    cv.width = W * dpr;
    cv.height = H * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    const amps = resampleToRanges(waves, duration, baseRanges);
    const max = Math.max(...amps, 0.001);

    const topPadding = 10;
    const floorY = H;
    const usableH = floorY - topPadding;

    baseRanges.forEach((range, i) => {
      const t = amps[i] / max;
      const h = Math.max(2, t * usableH);
      const x = (range.start / duration) * W;
      const w = Math.max(1, ((range.end - range.start) / duration) * W - 1);
      const top = floorY - h;
      const color = colormap(t);

      const grad = ctx.createLinearGradient(0, top, 0, floorY);
      grad.addColorStop(0, rgb(color, 1.15));
      grad.addColorStop(1, rgb(color, 0.65));
      ctx.fillStyle = grad;
      ctx.fillRect(x, top, w, h);

      // Ошибочный чанк — гасим цвет, чтобы было видно проблемное место
      const chunk = baseChunks[i];
      if (chunk?.error) {
        ctx.fillStyle = 'rgba(10,10,10,0.55)';
        ctx.fillRect(x, top, w, h);
      }
    });

    // Направляющие линии по границам базовых чанков — визуально "сшивают"
    // диаграмму с рядами чанков ниже (единая сетка по времени).
    ctx.strokeStyle = 'rgba(255,255,255,0.08)';
    ctx.lineWidth = 1;
    baseRanges.forEach((range) => {
      const x = Math.round((range.start / duration) * W) + 0.5;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, H);
      ctx.stroke();
    });
    const lastX = Math.round(W) - 0.5;
    ctx.beginPath();
    ctx.moveTo(lastX, 0);
    ctx.lineTo(lastX, H);
    ctx.stroke();
  }, [waves, chunks, duration, chunkSeconds]);

  const renderChunkRow = (ranges, rowChunks, extraClass) => (
    <div className={`chunk-row ${extraClass}`}>
      {rowChunks.map((ch, i) => {
        const { start, end } = ranges[i];
        if (!duration) return null;
        const left = (start / duration) * 100;
        const width = ((end - start) / duration) * 100;
        const isActive = currentTime >= start && currentTime < end;

        const catIdx = argmaxIndex(ch.category);
        const tgtIdx = argmaxIndex(ch.target);

        return (
          <div
            key={`${extraClass}-${ch.chunk_index}`}
            className={`chunk-box${isActive ? ' active' : ''}${ch.error ? ' has-error' : ''}`}
            style={{ left: `${left}%`, width: `calc(${width}% - 1px)` }}
          >
            <div className="chunk-box-category">{CATEGORY[catIdx] || '?'}</div>
            <div className="chunk-box-target">{TARGET[tgtIdx] || '?'}</div>
            {ch.error && <div className="chunk-box-error">error</div>}
          </div>
        );
      })}
    </div>
  );

  return (
    <div className="card">
      <strong style={{ color: '#aaa', letterSpacing: 1 }}>waveform</strong>

      <div className="waveform-canvas-wrap">
        <canvas
          ref={canvasRef}
          style={{ width: '100%', height: 150, display: 'block', marginTop: 10 }}
        />
      </div>

      {baseChunks.length > 0 && (
        <div className="chunk-rows">
          {offsetChunks.length > 0 && (
            <>
              <span className="chunk-row-tag">+{offsetSeconds}s</span>
              {renderChunkRow(offsetRanges, offsetChunks, 'chunk-row-offset')}
            </>
          )}
          <span className="chunk-row-tag">base</span>
          {renderChunkRow(baseRanges, baseChunks, 'chunk-row-base')}
        </div>
      )}

      {/* Легенда цветовой шкалы амплитуды */}
      <div className="waveform-legend">
        <span className="muted">quiet</span>
        <div className="waveform-legend-bar" />
        <span className="muted">loud</span>
      </div>
    </div>
  );
}
