// src/components/Main.jsx
import { useEffect, useRef, useState } from 'react';
import {
  getModels,
  createTask,
  pollTask,
  modelId,
  modelLabel,
  modelChunkSeconds,
} from '../api.js';
import Waveform from './Waveform.jsx';
import History from './History.jsx';
import Player from './Player.jsx';

// -----------------------------------------------------------------------
// TODO(backend): временная заглушка на случай, если каталог моделей пуст
// (бэкенд ещё не поднял Triton / список моделей недоступен). Как только
// GET /api/v1/models начнёт стабильно отдавать реальные модели — заглушка
// просто не будет использоваться (см. useEffect ниже).
// -----------------------------------------------------------------------
const MOCK_MODELS = [
  { name: 'transport audio' },
  { name: 'siren audio' },
];

function truncateName(name, max = 40) {
  if (!name || name.length <= max) return name;
  const half = Math.floor((max - 1) / 2);
  return `${name.slice(0, half)}…${name.slice(name.length - half)}`;
}

// Простая, незамысловатая иконка — звуковая дорожка (эквалайзер),
// тематически про аудио, без лишней "сферической" декорации.
function UploadIcon() {
  return (
    <svg
      className="upload-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      xmlns="http://www.w3.org/2000/svg"
    >
      <line x1="3" y1="10" x2="3" y2="14" />
      <line x1="8" y1="6" x2="8" y2="18" />
      <line x1="13" y1="2" x2="13" y2="22" />
      <line x1="18" y1="6" x2="18" y2="18" />
      <line x1="21.5" y1="10" x2="21.5" y2="14" />
    </svg>
  );
}

function ArrowIcon({ direction }) {
  const d = direction === 'left' ? 'M14 4l-8 8 8 8' : 'M10 4l8 8-8 8';
  return (
    <svg
      className="model-arrow-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path d={d} />
    </svg>
  );
}

export default function Main() {
  const [models, setModels] = useState([]);
  const [modelIndex, setModelIndex] = useState(0);
  const [file, setFile] = useState(null);
  const [dragOver, setDragOver] = useState(false);

  const [state, setState] = useState('idle');
  const [error, setError] = useState(null);
  const [waves, setWaves] = useState(null);
  const [chunks, setChunks] = useState(null);
  // Модель, с которой реально был проанализирован текущий файл — от неё
  // зависит нарезка чанков по времени (seconds_per_chunk). Пользователь
  // может переключить модель для *следующего* анализа, не ломая
  // отображение уже готового результата.
  const [analyzedModel, setAnalyzedModel] = useState(null);

  const [audioUrl, setAudioUrl] = useState(null);
  const [duration, setDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [playing, setPlaying] = useState(false);

  const [historyOn, setHistoryOn] = useState(false);

  const [rawLog, setRawLog] = useState([]);

  const audioRef = useRef(null);
  const fileInputRef = useRef(null);
  const rafRef = useRef(null);

  const handleRaw = (info) => setRawLog((prev) => [...prev, info]);

  useEffect(() => {
    getModels()
      .then((list) => {
        const finalList = list && list.length ? list : MOCK_MODELS;
        setModels(finalList);
        setModelIndex(0);
      })
      .catch(() => {
        setModels(MOCK_MODELS);
        setModelIndex(0);
      });
  }, []);

  // Плавное обновление времени с точностью до миллисекунд во время
  // воспроизведения: событие 'timeupdate' у <audio> стреляет всего
  // несколько раз в секунду, этого мало для мс-индикации в плеере.
  useEffect(() => {
    if (!playing) {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
      return;
    }
    const tick = () => {
      if (audioRef.current) setCurrentTime(audioRef.current.currentTime);
      rafRef.current = requestAnimationFrame(tick);
    };
    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [playing]);

  const currentModel = models[modelIndex] || null;
  const currentModelId = modelId(currentModel);
  const currentModelLabel = modelLabel(currentModel) || '—';

  const goToModel = (delta) => {
    if (!models.length) return;
    setModelIndex((i) => (i + delta + models.length) % models.length);
  };

  const processFile = (f) => {
    if (!f) return;
    setFile(f);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(URL.createObjectURL(f));
    setWaves(null);
    setChunks(null);
    setAnalyzedModel(null);
    setState('idle');
    setCurrentTime(0);
  };

  const handleAnalyze = async () => {
    if (!file) return;
    setError(null);
    setRawLog([]);
    setState('analyzing');
    const modelUsed = currentModel;
    try {
      const created = await createTask(file, currentModelId, handleRaw);
      setWaves(created.waves || []);
      setAnalyzedModel(modelUsed);

      const result = await pollTask(created.task_id, handleRaw);
      setChunks(result.result?.chunks || []);
      setState('done');
    } catch (e) {
      setState('error');
      setError(
        e.message === 'timeout'
          ? 'Request timed out. Result may appear in history later.'
          : 'Error: ' + e.message
      );
    }
  };

  const handleFileChange = (e) => {
    const f = e.target.files && e.target.files[0];
    processFile(f);
  };

  const handleDragOver = (e) => {
    e.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = () => setDragOver(false);

  const handleDrop = (e) => {
    e.preventDefault();
    setDragOver(false);
    const f = e.dataTransfer.files && e.dataTransfer.files[0];
    processFile(f);
  };

  const handlePlayPause = () => {
    const el = audioRef.current;
    if (!el) return;
    if (el.paused) el.play();
    else el.pause();
  };

  const handleSeek = (t) => {
    const el = audioRef.current;
    if (!el) return;
    el.currentTime = t;
    setCurrentTime(t);
  };

  const handleReset = () => {
    setFile(null);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(null);
    setWaves(null);
    setChunks(null);
    setAnalyzedModel(null);
    setState('idle');
    setError(null);
    setCurrentTime(0);
    setDuration(0);
    setRawLog([]);
  };

  const triggerFileSelect = () => {
    fileInputRef.current.click();
  };

  const chunkSeconds = modelChunkSeconds(analyzedModel);

  return (
    <>
      <div className={`container${audioUrl ? ' has-player' : ''}`}>
        {/* Шапка */}
        <header className="header">
          <h1>audio inference</h1>
          <div className="row">
            <button onClick={() => setHistoryOn((v) => !v)}>history</button>
          </div>
        </header>

        {/* Название текущей модели — компактно, слева под шапкой */}
        <div className="model-indicator">
          model: <span className="model-indicator-value">{currentModelLabel}</span>
        </div>

        {/* История (если включена) */}
        {historyOn && <History open={historyOn} />}

        {/* Основная область – центрирование блока загрузки */}
        <div className="main-area">
          <div
            className={`card upload-card${dragOver ? ' drag-over' : ''}`}
            style={{ maxWidth: 700, textAlign: 'center' }}
          >
            {models.length > 1 && (
              <>
                <button
                  type="button"
                  className="model-arrow model-arrow-left"
                  onClick={() => goToModel(-1)}
                  aria-label="previous model"
                  title="previous model"
                >
                  <ArrowIcon direction="left" />
                </button>
                <button
                  type="button"
                  className="model-arrow model-arrow-right"
                  onClick={() => goToModel(1)}
                  aria-label="next model"
                  title="next model"
                >
                  <ArrowIcon direction="right" />
                </button>
              </>
            )}

            <div
              className="upload-drop"
              onClick={triggerFileSelect}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
              role="button"
              tabIndex={0}
              aria-label="select audio file"
            >
              <UploadIcon />
              <p className="muted upload-hint">
                {file ? truncateName(file.name) : 'click or drop an audio file'}
              </p>
            </div>
            <input
              type="file"
              accept="audio/*,.mp3,.wav"
              onChange={handleFileChange}
              ref={fileInputRef}
            />

            <div className="row" style={{ justifyContent: 'center', marginTop: 18 }}>
              <button onClick={handleAnalyze} disabled={!file || state === 'analyzing'}>
                {state === 'analyzing' ? 'analyzing...' : 'analyze'}
              </button>
              {(file || waves) && <button onClick={handleReset}>reset</button>}
            </div>

            {state === 'error' && <p className="error">{error}</p>}

            {/* Сам <audio> элемент скрыт — воспроизведением управляет плеер снизу страницы */}
            {audioUrl && (
              <audio
                ref={audioRef}
                src={audioUrl}
                onPlay={() => setPlaying(true)}
                onPause={() => setPlaying(false)}
                onTimeUpdate={(e) => setCurrentTime(e.currentTarget.currentTime)}
                onLoadedMetadata={(e) => setDuration(e.currentTarget.duration || 0)}
                style={{ display: 'none' }}
              />
            )}
          </div>
        </div>

        {/* Диаграмма анализа – идёт ниже, но уже не центрируется */}
        {waves && (
          <Waveform
            waves={waves}
            chunks={chunks}
            duration={duration}
            currentTime={currentTime}
            chunkSeconds={chunkSeconds}
          />
        )}

        {rawLog.length > 0 && (
          <div className="card">
            <strong>debug: raw server responses</strong>
            <pre
              style={{
                whiteSpace: 'pre-wrap',
                fontSize: 13,
                color: '#888',
                maxHeight: 300,
                overflow: 'auto',
                marginTop: 12,
                background: 'rgba(255,255,255,0.03)',
                padding: 16,
                border: '1px solid rgba(255,255,255,0.05)',
              }}
            >
              {rawLog.map((entry, i) => (
                `[${i}] ${entry.url} → HTTP ${entry.status}\n` +
                JSON.stringify(entry.body, null, 2) +
                '\n\n'
              ))}
            </pre>
          </div>
        )}
      </div>

      {/* Плеер, закреплённый внизу страницы — появляется, когда выбран файл */}
      {audioUrl && (
        <Player
          fileName={file?.name}
          modelLabel={currentModelLabel}
          playing={playing}
          currentTime={currentTime}
          duration={duration}
          onPlayPause={handlePlayPause}
          onSeek={handleSeek}
          onClose={handleReset}
        />
      )}
    </>
  );
}
