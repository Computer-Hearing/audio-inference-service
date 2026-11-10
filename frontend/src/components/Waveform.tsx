// src/components/Waveform.tsx
//
// Диаграмма громкости во времени (RMS-амплитуда). Раньше массив waves
// приходил с бэкенда в ответе CreateTask, теперь он строится на фронтенде
// из самого файла — см. src/audio.ts.
//
// Столбцы привязаны к secondsPerChunk модели: границы столбцов совпадают
// с границами чанков (и базового слоя, и слоя со сдвигом +1s), поэтому
// чанк больше не заканчивается посреди столбца. Направляющие линии от
// границ чанков вверх, в область графика, ложатся ровно в щели между
// столбцами.
//
// Плашки чанков кликабельны: клик выбирает чанк, и App.tsx показывает
// под диаграммой карточку с подробностями о нём (ChunkDetails.tsx).
import { useEffect, useMemo, useRef } from 'react';
import { computeBars, type AudioData } from '../audio.ts';
import { argmaxIndex, chunkKey, chunkRange, type Range } from '../chunks.ts';
import { labelsFor } from '../projects.ts';
import type { Chunk } from '../gen/inference/v1/inference_pb.ts';

type Rgb = [number, number, number];

interface Group {
  start: number;
  end: number;
  catIdx: number;
  tgtIdx: number;
  error: boolean;
  errorMessage: string;
  count: number;
  items: Chunk[];
}

// Цветовая карта амплитуды: тихо (тёмно-синий) → громко (красный),
// стандартная для аудио-визуализаторов логика "холодный → тёплый".
const COLOR_STOPS: { t: number; c: Rgb }[] = [
  { t: 0.0, c: [10, 14, 30] },
  { t: 0.16, c: [32, 64, 128] },
  { t: 0.36, c: [22, 138, 148] },
  { t: 0.56, c: [72, 176, 92] },
  { t: 0.74, c: [232, 202, 42] },
  { t: 0.89, c: [240, 122, 32] },
  { t: 1.0, c: [230, 48, 56] },
];

function colormap(t: number): Rgb {
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

function rgb([r, g, b]: Rgb, factor = 1): string {
  if (factor >= 1) {
    const f = factor - 1;
    return `rgb(${r + (255 - r) * f}, ${g + (255 - g) * f}, ${b + (255 - b) * f})`;
  }
  return `rgb(${r * factor}, ${g * factor}, ${b * factor})`;
}

// Схлопывает подряд идущие чанки с одинаковым предсказанием (категория +
// звук) в один визуальный блок — если модель несколько чанков подряд
// выдаёт одно и то же, незачем рисовать это как несколько одинаковых
// табличек подряд. Ошибочные чанки друг с другом не схлопываются — каждый
// остаётся отдельным блоком, чтобы не потерять, где именно была ошибка.
function mergeGroups(ranges: Range[], chunksArr: Chunk[]): Group[] {
  const groups: Group[] = [];
  chunksArr.forEach((ch, i) => {
    const { start, end } = ranges[i];
    const catIdx = argmaxIndex(ch.category);
    const tgtIdx = argmaxIndex(ch.target);
    const isError = !!ch.errorMessage;

    const last = groups[groups.length - 1];
    const sameAsLast =
      last && !isError && !last.error && last.catIdx === catIdx && last.tgtIdx === tgtIdx;

    if (sameAsLast) {
      last.end = end;
      last.count += 1;
      last.items.push(ch);
    } else {
      groups.push({
        start, end, catIdx, tgtIdx,
        error: isError, errorMessage: ch.errorMessage, count: 1,
        // Исходные чанки блока — нужны, чтобы по клику открыть подробности
        items: [ch],
      });
    }
  });
  return groups;
}

interface WaveformProps {
  audioData: AudioData | null;
  chunks: Chunk[] | null;
  duration: number;
  currentTime: number;
  chunkSeconds: number;
  modelName: string;
  selectedKey?: string;
  onSelectChunk: (items: Chunk[]) => void;
}

export default function Waveform({
  audioData,
  chunks,
  duration: playerDuration,
  currentTime,
  chunkSeconds,
  modelName,
  selectedKey,
  onSelectChunk,
}: WaveformProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  // Длительность берём из декодированного файла — по ней же считаются
  // столбцы, так что столбцы и чанки размечаются в одной шкале времени.
  // Пока файл не декодирован — длительность из <audio>.
  const duration = audioData?.duration || playerDuration;

  const bars = useMemo(
    () => (audioData ? computeBars(audioData, chunkSeconds) : []),
    [audioData, chunkSeconds]
  );

  const baseChunks = (chunks || [])
    .filter((ch) => (ch.offset || 0) === 0)
    .slice()
    .sort((a, b) => a.chunkIndex - b.chunkIndex);

  const offsetChunks = (chunks || [])
    .filter((ch) => (ch.offset || 0) > 0)
    .slice()
    .sort((a, b) => a.chunkIndex - b.chunkIndex);
  const offsetSeconds = offsetChunks.length ? (offsetChunks[0].offset || 1) : 1;

  const baseRanges = baseChunks.map((ch) => chunkRange(0, ch.chunkIndex, chunkSeconds, duration));
  const offsetRanges = offsetChunks.map((ch) => chunkRange(offsetSeconds, ch.chunkIndex, chunkSeconds, duration));

  // Схлопнутые блоки считаем один раз — они нужны и для отрисовки рядов,
  // и для направляющих линий (линии должны идти по границам итоговых
  // блоков, а не по границам "исходных" чанков, которые в них слились).
  const baseGroups = mergeGroups(baseRanges, baseChunks);
  const offsetGroups = mergeGroups(offsetRanges, offsetChunks);

  // Столбчатая диаграмма: каждый столбик рисуется по своему отрезку
  // времени [start, end), а не равномерно по индексу — так столбцы
  // совпадают с разметкой чанков ниже.
  useEffect(() => {
    const cv = canvasRef.current;
    if (!cv) return;
    const ctx = cv.getContext('2d');
    if (!ctx) return;
    const dpr = window.devicePixelRatio || 1;
    const W = cv.clientWidth;
    const H = cv.clientHeight;
    cv.width = W * dpr;
    cv.height = H * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    if (!bars.length || !duration) return;

    const max = Math.max(...bars.map((b) => b.value), 0.001);

    const topPadding = 10;
    const floorY = H;
    const usableH = floorY - topPadding;

    for (const bar of bars) {
      const t = bar.value / max;
      const h = Math.max(2, t * usableH);
      const x = (bar.start / duration) * W;
      const fullW = ((bar.end - bar.start) / duration) * W;
      // Зазор в 1px между столбиками, пока столбики достаточно широкие
      const barW = Math.max(1, fullW > 3 ? fullW - 1 : fullW);
      const top = floorY - h;
      const color = colormap(t);

      const grad = ctx.createLinearGradient(0, top, 0, floorY);
      grad.addColorStop(0, rgb(color, 1.15));
      grad.addColorStop(1, rgb(color, 0.65));
      ctx.fillStyle = grad;
      ctx.fillRect(x, top, barW, h);
    }
  }, [bars, duration]);

  const renderChunkRow = (groups: Group[], extraClass: string) => (
    <div className={`chunk-row ${extraClass}`}>
      {groups.map((g, i) => {
        if (!duration) return null;
        const left = (g.start / duration) * 100;
        const width = ((g.end - g.start) / duration) * 100;
        const isActive = currentTime >= g.start && currentTime < g.end;
        const isSelected = g.items.some((ch) => chunkKey(ch) === selectedKey);
        // target_output есть у всех моделей, category_output — только у части.
        const item = g.items[0];
        const hasCategory = item?.category?.length > 0;
        const hasTarget = item?.target?.length > 0;
        const catLabels = labelsFor(modelName, 'category', item?.category?.length || 0);
        const tgtLabels = labelsFor(modelName, 'target', item?.target?.length || 0);
        const countBadge = g.count > 1 && !g.error
          ? <span className="chunk-box-count"> ×{g.count}</span>
          : null;

        return (
          <div
            key={`${extraClass}-${i}`}
            className={
              `chunk-box${isActive ? ' active' : ''}${isSelected ? ' selected' : ''}` +
              `${g.error ? ' has-error' : ''}`
            }
            style={{ left: `${left}%`, width: `calc(${width}% - 1px)` }}
            title={g.error ? g.errorMessage : 'show chunk details'}
            role="button"
            tabIndex={0}
            onClick={() => onSelectChunk(g.items)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onSelectChunk(g.items);
              }
            }}
          >
            {hasCategory && (
              <div className="chunk-box-category">
                {catLabels[g.catIdx] || '?'}
                {countBadge}
              </div>
            )}
            {hasTarget && (
              <div className="chunk-box-target">
                {tgtLabels[g.tgtIdx] || '?'}
                {!hasCategory && countBadge}
              </div>
            )}
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
      <strong className="card-title">waveform</strong>

      <div className="waveform-wrap">
        <canvas
          ref={canvasRef}
          className="waveform-canvas"
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
