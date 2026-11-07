// src/components/ChunkDetails.tsx
//
// Карточка с подробностями о выбранном чанке: открывается по клику на
// плашку чанка под гистограммой (Waveform.tsx) и показывает всё, что
// известно о чанке, — слой, номер, время, ответ модели и полное
// распределение вероятностей по всем классам. В правом верхнем углу —
// кнопки: прослушать фрагмент, скачать его в WAV и закрыть карточку.
import { useEffect, useRef, useState } from 'react';
import type { Chunk } from '../gen/inference/v1/inference_pb.ts';
import {
  labelsFor,
  argmaxIndex,
  chunkKey,
  chunkRange,
  layerLabel,
  softmax,
} from '../chunks.ts';

function formatSec(sec: number): string {
  return `${sec.toFixed(3)} s`;
}

function formatPct(p: number): string {
  return `${(p * 100).toFixed(1)}%`;
}

function DownloadIcon() {
  return (
    <svg
      className="chunk-details-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path d="M12 4v11M7 10l5 5 5-5M5 20h14" />
    </svg>
  );
}

function PlayIcon() {
  return (
    <svg className="chunk-details-icon chunk-details-icon-fill" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M7 4.5v15l13-7.5-13-7.5z" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg
      className="chunk-details-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path d="M5 5l14 14M19 5L5 19" />
    </svg>
  );
}

// Распределение вероятностей по классам: строки отсортированы по убыванию,
// у каждой — название, полоска, процент и "сырое" значение (логит),
// которое прислал бэкенд.
function Distribution({
  title,
  labels,
  logits,
}: {
  title: string;
  labels: string[];
  logits: number[];
}) {
  const probs = softmax(logits);
  const rows = probs
    .map((p, i) => ({ label: labels[i] || `class ${i}`, p, logit: logits[i] }))
    .sort((a, b) => b.p - a.p);

  return (
    <div className="chunk-dist">
      <div className="chunk-dist-title">{title}</div>
      {rows.map((r, i) => (
        <div key={r.label} className={`chunk-dist-row${i === 0 ? ' top' : ''}`}>
          <span className="chunk-dist-label">{r.label}</span>
          <span className="chunk-dist-bar">
            <span className="chunk-dist-bar-fill" style={{ width: `${r.p * 100}%` }} />
          </span>
          <span className="chunk-dist-pct">{formatPct(r.p)}</span>
          <span className="chunk-dist-logit" title="raw model output (logit)">
            {r.logit.toFixed(2)}
          </span>
        </div>
      ))}
    </div>
  );
}

interface ChunkDetailsProps {
  chunk: Chunk;
  siblings: Chunk[];
  chunkSeconds: number;
  duration: number;
  fileName?: string;
  modelName: string;
  onSelect: (chunk: Chunk) => void;
  onClose: () => void;
  onPlay: (start: number, end: number) => void;
  onDownload: (chunk: Chunk, start: number, end: number) => Promise<void>;
}

export default function ChunkDetails({
  chunk,
  siblings,
  chunkSeconds,
  duration,
  fileName,
  modelName,
  onSelect,
  onClose,
  onPlay,
  onDownload,
}: ChunkDetailsProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  const offset = chunk.offset || 0;
  const { start, end } = chunkRange(offset, chunk.chunkIndex, chunkSeconds, duration);
  const hasError = !!chunk.errorMessage;
  // target_output есть у всех моделей, category_output — только у части.
  const hasCategory = chunk.category?.length > 0;
  const hasTarget = chunk.target?.length > 0;
  const hasPrediction = hasCategory || hasTarget;

  const catProbs = softmax(chunk.category);
  const tgtProbs = softmax(chunk.target);
  const catIdx = argmaxIndex(chunk.category);
  const tgtIdx = argmaxIndex(chunk.target);

  const catLabels = labelsFor(modelName, 'category', chunk.category?.length || 0);
  const tgtLabels = labelsFor(modelName, 'target', chunk.target?.length || 0);

  // Закрытие по Esc
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  // При выборе другого чанка — прокрутить страницу так, чтобы карточка
  // была видна (она появляется под диаграммой и может оказаться ниже экрана)
  useEffect(() => {
    rootRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    setDownloadError(null);
  }, [chunk]);

  const handleDownload = async () => {
    setDownloading(true);
    setDownloadError(null);
    try {
      await onDownload(chunk, start, end);
    } catch (e) {
      setDownloadError(e instanceof Error ? e.message : 'download failed');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="card chunk-details" ref={rootRef} role="region" aria-label="chunk details">
      <div className="chunk-details-actions">
        <button
          type="button"
          className="chunk-details-btn"
          onClick={() => onPlay(start, end)}
          title="play this chunk"
          disabled={end <= start}
        >
          <PlayIcon />
          <span>play</span>
        </button>
        <button
          type="button"
          className="chunk-details-btn"
          onClick={handleDownload}
          title="download this chunk as wav"
          disabled={downloading || end <= start}
        >
          <DownloadIcon />
          <span>{downloading ? 'saving…' : 'wav'}</span>
        </button>
        <button
          type="button"
          className="chunk-details-btn chunk-details-close"
          onClick={onClose}
          aria-label="close chunk details"
          title="close (esc)"
        >
          <CloseIcon />
        </button>
      </div>

      <strong className="card-title">
        chunk {layerLabel(offset)} #{chunk.chunkIndex}
      </strong>
      <div className="chunk-details-time muted">
        {formatSec(start)} – {formatSec(end)} · {formatSec(end - start)}
      </div>

      {downloadError && <p className="error">Error: {downloadError}</p>}

      {siblings.length > 1 && (
        <div className="chunk-details-siblings">
          <span className="muted">merged block:</span>
          {siblings.map((ch) => (
            <button
              key={chunkKey(ch)}
              type="button"
              className={`chunk-details-sibling${chunkKey(ch) === chunkKey(chunk) ? ' current' : ''}`}
              onClick={() => onSelect(ch)}
            >
              #{ch.chunkIndex}
            </button>
          ))}
        </div>
      )}

      {hasError && (
        <p className="error chunk-details-error">
          inference error: {chunk.errorMessage}
        </p>
      )}

      {hasPrediction && (
        <>
          <div className="chunk-details-summary">
            {hasCategory && (
              <div className="chunk-details-fact">
                <span className="muted">category</span>
                <span className="chunk-details-fact-value">
                  {catLabels[catIdx] || '?'}
                </span>
                <span className="chunk-details-fact-pct">{formatPct(catProbs[catIdx])}</span>
              </div>
            )}
            {hasTarget && (
              <div className="chunk-details-fact">
                <span className="muted">sound</span>
                <span className="chunk-details-fact-value">
                  {tgtLabels[tgtIdx] || '?'}
                </span>
                <span className="chunk-details-fact-pct">{formatPct(tgtProbs[tgtIdx])}</span>
              </div>
            )}
          </div>

          <div className="chunk-details-dists">
            {hasCategory && (
              <Distribution title="category" labels={catLabels} logits={chunk.category} />
            )}
            {hasTarget && (
              <Distribution title="sound" labels={tgtLabels} logits={chunk.target} />
            )}
          </div>
        </>
      )}

      <div className="chunk-details-meta muted">
        <span>layer {chunk.layer || 0}</span>
        <span>offset {offset} s</span>
        <span>index {chunk.chunkIndex}</span>
        <span>length {chunkSeconds} s</span>
        {modelName && <span>model {modelName}</span>}
        {fileName && <span title={fileName}>file {fileName}</span>}
      </div>
    </div>
  );
}
