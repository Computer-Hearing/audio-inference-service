// src/components/Waveform.jsx
//
// Раньше компонент назывался Spectrogram, хотя настоящей спектрограммой
// (частота × время) не является: бэкенд отдаёт всего один плоский массив
// RMS-амплитуды по времени (pkg.AudioWaveBucketsLen = 40 "бакетов" на весь
// файл, см. chunks.audioWaveform на бэкенде). Это обычная диаграмма
// громкости во времени — переименовано в Waveform, чтобы не вводить в
// заблуждение.
//
// Диаграмма рисуется по старому, простому принципу: один столбик — один
// элемент массива waves, столбики равномерно распределены по всей ширине
// (без привязки/пересчёта под границы чанков). Ряды чанков под диаграммой —
// отдельная, независимая по разметке штука, как и раньше. Единственная
// связь между ними — тонкие направляющие линии, идущие от начала каждого
// базового чанка вверх, в область графика.
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

// Схлопывает подряд идущие чанки с одинаковым предсказанием (категория +
// звук) в один визуальный блок — если модель несколько чанков подряд
// выдаёт одно и то же, незачем рисовать это как несколько одинаковых
// табличек подряд. Ошибочные чанки друг с другом не схлопываются — каждый
// остаётся отдельным блоком, чтобы не потерять, где именно была ошибка.
function mergeGroups(ranges, chunksArr) {
  const groups = [];
  chunksArr.forEach((ch, i) => {
    const { start, end } = ranges[i];
    const catIdx = argmaxIndex(ch.category);
    const tgtIdx = argmaxIndex(ch.target);
    const isError = !!ch.error;

    const last = groups[groups.length - 1];
    const sameAsLast =
      last && !isError && !last.error && last.catIdx === catIdx && last.tgtIdx === tgtIdx;

    if (sameAsLast) {
      last.end = end;
      last.count += 1;
    } else {
      groups.push({ start, end, catIdx, tgtIdx, error: isError, count: 1 });
    }
  });
  return groups;
}

// -----------------------------------------------------------------------
// Границы чанков по времени (нужны только для рядов чанков ниже графика).
// Правило (согласовано с бэкендом, см. internal/chunks/audio_chunk.go):
//  - базовый слой (offset=0): чанки фиксированного размера chunkSeconds,
//    последний чанк может быть короче (хвост длительностью duration % chunkSeconds,
//    если этот хвост длится дольше ~0.5s — иначе бэкенд его просто отбрасывает).
//  - слои со сдвигом (offset=1s и т.д.): те же чанки фиксированного размера,
//    считаются от отметки offset, хвост НЕ создаётся.
// Опираемся на chunk_index и offset, которые уже возвращает бэкенд.
// -----------------------------------------------------------------------
function chunkRange(offset, chunkIndex, chunkSeconds, duration) {
  const start = offset + chunkIndex * chunkSeconds;
  const end = Math.min(duration, start + chunkSeconds);
  return { start, end: Math.max(start, end) };
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

  // Схлопнутые блоки считаем один раз — они нужны и для отрисовки рядов,
  // и для направляющих линий (линии должны идти по границам итоговых
  // блоков, а не по границам "исходных" чанков, которые в них слились).
  const baseGroups = mergeGroups(baseRanges, baseChunks);
  const offsetGroups = mergeGroups(offsetRanges, offsetChunks);

  // Обычная столбчатая диаграмма: один столбик — один элемент waves,
  // столбики равномерно распределены по ширине (без привязки к чанкам).
  useEffect(() => {
    const cv = canvasRef.current;
    if (!cv || !waves || !waves.length) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const W = cv.clientWidth;
    const H = cv.clientHeight;
    cv.width = W * dpr;
    cv.height = H * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    const n = waves.length;
    const max = Math.max(...waves, 0.001);
    const gap = 1;
    const barW = Math.max(1, W / n - gap);

    const topPadding = 10;
    const floorY = H;
    const usableH = floorY - topPadding;

    for (let i = 0; i < n; i++) {
      const t = waves[i] / max;
      const h = Math.max(2, t * usableH);
      const x = (i / n) * W;
      const top = floorY - h;
      const color = colormap(t);

      const grad = ctx.createLinearGradient(0, top, 0, floorY);
      grad.addColorStop(0, rgb(color, 1.15));
      grad.addColorStop(1, rgb(color, 0.65));
      ctx.fillStyle = grad;
      ctx.fillRect(x, top, barW, h);
    }
  }, [waves]);

  const renderChunkRow = (groups, extraClass) => (
    <div className={`chunk-row ${extraClass}`}>
      {groups.map((g, i) => {
        if (!duration) return null;
        const left = (g.start / duration) * 100;
        const width = ((g.end - g.start) / duration) * 100;
        const isActive = currentTime >= g.start && currentTime < g.end;

        return (
          <div
            key={`${extraClass}-${i}`}
            className={`chunk-box${isActive ? ' active' : ''}${g.error ? ' has-error' : ''}`}
            style={{ left: `${left}%`, width: `calc(${width}% - 1px)` }}
          >
            <div className="chunk-box-category">
              {CATEGORY[g.catIdx] || '?'}
              {g.count > 1 && !g.error && <span className="chunk-box-count"> ×{g.count}</span>}
            </div>
            <div className="chunk-box-target">{TARGET[g.tgtIdx] || '?'}</div>
            {g.error && <div className="chunk-box-error">error</div>}
          </div>
        );
      })}
    </div>
  );

  // Направляющие линии вверх, в область графика — по границам уже
  // схлопнутых блоков (начало слева, конец справа), а не по границам
  // "исходных" чанков внутри одного блока — они больше не нужны, раз
  // блок визуально единый. Базовая сетка (offset=0) — сплошные линии,
  // сетка со сдвигом +1s — пунктирные.
  const baseGuideLines = duration
    ? [...baseGroups.map((g) => g.start), duration]
    : [];
  const offsetGuideLines = duration
    ? [
        ...offsetGroups.map((g) => g.start),
        ...(offsetGroups.length ? [offsetGroups[offsetGroups.length - 1].end] : []),
      ]
    : [];

  return (
    <div className="card">
      <strong style={{ color: '#aaa', letterSpacing: 1 }}>waveform</strong>

      <div className="waveform-wrap">
        <canvas
          ref={canvasRef}
          className="waveform-canvas"
          style={{ width: '100%', height: 150, display: 'block', marginTop: 10 }}
        />

        {baseChunks.length > 0 && (
          <div className="chunk-rows">
            {offsetChunks.length > 0 && (
              <>
                <span className="chunk-row-tag">+{offsetSeconds}s</span>
                {renderChunkRow(offsetGroups, 'chunk-row-offset')}
              </>
            )}
            <span className="chunk-row-tag">base</span>
            {renderChunkRow(baseGroups, 'chunk-row-base')}
          </div>
        )}

        {baseGuideLines.map((sec, i) => (
          <div
            key={`base-${i}`}
            className="waveform-guide-line waveform-guide-line-base"
            style={{ left: `${(sec / duration) * 100}%` }}
          />
        ))}
        {offsetGuideLines.map((sec, i) => (
          <div
            key={`offset-${i}`}
            className="waveform-guide-line waveform-guide-line-offset"
            style={{ left: `${(sec / duration) * 100}%` }}
          />
        ))}
      </div>
    </div>
  );
}
