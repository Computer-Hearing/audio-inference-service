// src/components/Main.jsx
import { useEffect, useRef, useState } from 'react';
import {
  getModels,
  createTask,
  pollTask,
  modelId,
  modelLabel,
  modelChunkSeconds,
  MAX_AUDIO_BYTES,
} from '../api.js';
import { decodeAudio } from '../audio.js';
import Waveform from './Waveform.jsx';
import Player from './Player.jsx';

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
  const [chunks, setChunks] = useState(null);
  // Декодированный файл ({ samples, sampleRate, duration }) — из него
  // строится гистограмма громкости (раньше её присылал бэкенд как waves).
  const [audioData, setAudioData] = useState(null);
  // Модель, с которой реально был проанализирован текущий файл — от неё
  // зависит нарезка чанков и столбцов по времени (secondsPerChunk). Пользователь
  // может переключить модель для *следующего* анализа, не ломая
  // отображение уже готового результата.
  const [analyzedModel, setAnalyzedModel] = useState(null);

  const [audioUrl, setAudioUrl] = useState(null);
  const [duration, setDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [playing, setPlaying] = useState(false);

  const [rawLog, setRawLog] = useState([]);

  const audioRef = useRef(null);
  const fileInputRef = useRef(null);
  const rafRef = useRef(null);
  // Номер текущего файла — чтобы результат декодирования предыдущего
  // файла (если пользователь быстро выбрал другой) не перезаписал новый.
  const decodeIdRef = useRef(0);

  const handleRaw = (info) => setRawLog((prev) => [...prev, info]);

  useEffect(() => {
    // Список моделей — строго с бэкенда (GetModels), никаких заглушек:
    // если список пуст, значит Triton ещё не отдал ни одной модели
    // (см. models.Syncer на бэкенде) — это состояние явно показывается
    // пользователю, а не подменяется фейковыми названиями.
    getModels()
      .then((list) => {
        setModels(list || []);
        setModelIndex(0);
      })
      .catch(() => {
        setModels([]);
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
  const currentModelLabel = modelLabel(currentModel) || 'no models available';
  const currentModelReady = !currentModel || currentModel.ready !== false;

  const goToModel = (delta) => {
    if (!models.length) return;
    setModelIndex((i) => (i + delta + models.length) % models.length);
  };

  const processFile = (f) => {
    if (!f) return;
    setFile(f);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(URL.createObjectURL(f));
    setAudioData(null);
    setChunks(null);
    setAnalyzedModel(null);
    setError(null);
    setState('idle');
    setCurrentTime(0);

    // Декодируем сразу, пока пользователь выбирает модель — к моменту
    // анализа гистограмма обычно уже готова. Если браузер не умеет этот
    // формат, гистограммы просто не будет, анализ это не ломает.
    const decodeId = ++decodeIdRef.current;
    decodeAudio(f)
      .then((data) => {
        if (decodeIdRef.current === decodeId) setAudioData(data);
      })
      .catch((e) => console.warn('decodeAudio failed:', e));
  };

  const handleAnalyze = async () => {
    if (!file) return;
    setError(null);
    setRawLog([]);
    setChunks(null);
    setState('analyzing');
    const modelUsed = currentModel;
    try {
      const created = await createTask(file, currentModelId, handleRaw);
      setAnalyzedModel(modelUsed);

      const task = await pollTask(created.taskId, handleRaw);
      setChunks(task.result?.chunks || []);
      setState('done');
    } catch (e) {
      setState('error');
      setError(
        e.message === 'timeout'
          ? 'Request timed out. Please try again later.'
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
    decodeIdRef.current += 1;
    setAudioData(null);
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
        </header>

        {/* Название текущей модели — компактно, слева под шапкой */}
        <div className="model-indicator" title={currentModel?.projectDescription || currentModelId}>
          model: <span className="model-indicator-value">{currentModelLabel}</span>
          {currentModel && <> · {modelChunkSeconds(currentModel)}s chunks</>}
          {!currentModelReady && <> · not ready</>}
        </div>

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
              {!file && (
                <p className="muted upload-limit">up to {MAX_AUDIO_BYTES / 1024 / 1024} MB</p>
              )}
            </div>
            <input
              type="file"
              accept="audio/*,.mp3,.wav"
              onChange={handleFileChange}
              ref={fileInputRef}
            />

            <div className="row" style={{ justifyContent: 'center', marginTop: 18 }}>
              <button onClick={handleAnalyze} disabled={!file || !currentModelId || !currentModelReady || state === 'analyzing'}>
                {state === 'analyzing' ? 'analyzing...' : 'analyze'}
              </button>
              {file && <button onClick={handleReset}>reset</button>}
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
        {analyzedModel && (
          <Waveform
            audioData={audioData}
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
                `[${i}] ${entry.url} → ${entry.status}\n` +
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
