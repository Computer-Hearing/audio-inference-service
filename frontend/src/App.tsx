// src/App.tsx
//
// Главный экран инференса: выбор модели, загрузка файла, анализ, гистограмма
// с рядами чанков, карточка чанка и плеер. Ничего не знает о том, где он
// показан — на странице разработки (src/dev.tsx) или внутри портала
// (src/index.tsx, функция mount): клиент API и настройки приходят снаружи.
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
} from 'react';
import {
  modelId,
  modelLabel,
  modelChunkSeconds,
  MAX_AUDIO_BYTES,
  type InferenceApi,
  type RawLogEntry,
} from './api.ts';
import {
  decodeAudio,
  decodeAudioFull,
  encodeWavSegment,
  saveBlob,
  type AudioData,
} from './audio.ts';
import { chunkKey } from './chunks.ts';
import type { Chunk, Model } from './gen/inference/v1/inference_pb.ts';
import Waveform from './components/Waveform.tsx';
import ChunkDetails from './components/ChunkDetails.tsx';
import Player from './components/Player.tsx';

type AnalyzeState = 'idle' | 'analyzing' | 'done' | 'error';

// Выбранный чанк (по клику на плашку) и все чанки его блока — если
// несколько одинаковых чанков подряд схлопнуты в одну плашку.
interface Selection {
  key: string;
  items: Chunk[];
}

export interface AppProps {
  api: InferenceApi;
  // Своя шапка "audio inference". В портале заголовок раздела рисует сам
  // портал, поэтому там она выключена.
  header?: boolean;
}

function truncateName(name: string, max = 40): string {
  if (!name || name.length <= max) return name;
  const half = Math.floor((max - 1) / 2);
  return `${name.slice(0, half)}…${name.slice(name.length - half)}`;
}

// Простая, незамысловатая иконка — звуковая дорожка (эквалайзер),
// тематически про аудио, без лишней "сферической" декорации.
// Имя скачиваемого фрагмента: <имя файла>_<слой>_chunk<номер>_<начало>-<конец>s.wav,
// например street_base_chunk03_6.0-8.0s.wav
function chunkFileName(sourceName: string, chunk: Chunk, start: number, end: number): string {
  const base = (sourceName || 'audio').replace(/\.[^.]+$/, '').replace(/[^\w\-]+/g, '_');
  const layer = chunk.offset ? `offset${chunk.offset}s` : 'base';
  const index = String(chunk.chunkIndex).padStart(2, '0');
  return `${base}_${layer}_chunk${index}_${start.toFixed(1)}-${end.toFixed(1)}s.wav`;
}

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

function ArrowIcon({ direction }: { direction: 'left' | 'right' }) {
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

export default function App({ api, header = true }: AppProps) {
  const [models, setModels] = useState<Model[]>([]);
  const [modelIndex, setModelIndex] = useState(0);
  const [file, setFile] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);

  const [state, setState] = useState<AnalyzeState>('idle');
  const [error, setError] = useState<string | null>(null);
  const [chunks, setChunks] = useState<Chunk[] | null>(null);
  // Декодированный файл ({ samples, sampleRate, duration }) — из него
  // строится гистограмма громкости (раньше её присылал бэкенд как waves).
  const [audioData, setAudioData] = useState<AudioData | null>(null);
  // Модель, с которой реально был проанализирован текущий файл — от неё
  // зависит нарезка чанков и столбцов по времени (secondsPerChunk). Пользователь
  // может переключить модель для *следующего* анализа, не ломая
  // отображение уже готового результата.
  const [analyzedModel, setAnalyzedModel] = useState<Model | null>(null);
  const [selection, setSelection] = useState<Selection | null>(null);

  const [audioUrl, setAudioUrl] = useState<string | null>(null);
  const [duration, setDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [playing, setPlaying] = useState(false);

  const [rawLog, setRawLog] = useState<RawLogEntry[]>([]);

  const audioRef = useRef<HTMLAudioElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const rafRef = useRef<number | null>(null);
  // Номер текущего файла — чтобы результат декодирования предыдущего
  // файла (если пользователь быстро выбрал другой) не перезаписал новый.
  const decodeIdRef = useRef(0);
  // Файл, декодированный в полном качестве для скачивания чанков:
  // { file, promise }. Декодируется один раз, при первом скачивании.
  const fullAudioRef = useRef<{ file: File; promise: Promise<AudioBuffer> } | null>(null);
  // Конец фрагмента, который проигрывается кнопкой "play" в карточке чанка:
  // дойдя до него, воспроизведение останавливается.
  const segmentEndRef = useRef<number | null>(null);

  const handleRaw = (info: RawLogEntry) => setRawLog((prev) => [...prev, info]);

  useEffect(() => {
    // Список моделей — строго с бэкенда (GetModels), никаких заглушек:
    // если список пуст, значит Triton ещё не отдал ни одной модели
    // (см. models.Syncer на бэкенде) — это состояние явно показывается
    // пользователю, а не подменяется фейковыми названиями.
    let cancelled = false;
    api.getModels().then((list) => {
      if (cancelled) return;
      setModels(list);
      setModelIndex(0);
    });
    return () => {
      cancelled = true;
    };
  }, [api]);

  // Плавное обновление времени с точностью до миллисекунд во время
  // воспроизведения: событие 'timeupdate' у <audio> стреляет всего
  // несколько раз в секунду, этого мало для мс-индикации в плеере.
  useEffect(() => {
    if (!playing) {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
      return;
    }
    const tick = () => {
      const el = audioRef.current;
      if (el) {
        if (segmentEndRef.current !== null && el.currentTime >= segmentEndRef.current) {
          segmentEndRef.current = null;
          el.pause();
        }
        setCurrentTime(el.currentTime);
      }
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

  const goToModel = (delta: number) => {
    if (!models.length) return;
    setModelIndex((i) => (i + delta + models.length) % models.length);
  };

  const processFile = (f: File | null | undefined) => {
    if (!f) return;
    setFile(f);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(URL.createObjectURL(f));
    setAudioData(null);
    setChunks(null);
    setAnalyzedModel(null);
    setSelection(null);
    fullAudioRef.current = null;
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
    setSelection(null);
    setState('analyzing');
    const modelUsed = currentModel;
    try {
      const created = await api.createTask(file, currentModelId, handleRaw);
      setAnalyzedModel(modelUsed);

      const task = await api.pollTask(created.taskId, handleRaw);
      setChunks(task.result?.chunks || []);
      setState('done');
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setState('error');
      setError(
        e.message === 'timeout'
          ? 'Request timed out. Please try again later.'
          : 'Error: ' + e.message
      );
    }
  };

  const handleFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    processFile(e.target.files?.[0]);
    // Сбрасываем значение, чтобы повторный выбор того же файла тоже срабатывал
    e.target.value = '';
  };

  const handleDragOver = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = () => setDragOver(false);

  const handleDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragOver(false);
    processFile(e.dataTransfer.files?.[0]);
  };

  const handlePlayPause = () => {
    const el = audioRef.current;
    if (!el) return;
    segmentEndRef.current = null;
    if (el.paused) void el.play();
    else el.pause();
  };

  const handleSeek = (t: number) => {
    const el = audioRef.current;
    if (!el) return;
    segmentEndRef.current = null;
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
    setSelection(null);
    fullAudioRef.current = null;
    setState('idle');
    setError(null);
    setCurrentTime(0);
    setDuration(0);
    setRawLog([]);
  };

  const triggerFileSelect = () => {
    fileInputRef.current?.click();
  };

  // Клик по плашке: выбрать первый чанк блока; повторный клик по уже
  // выбранному блоку — закрыть карточку.
  const handleSelectChunk = (items: Chunk[]) => {
    if (!items || !items.length) return;
    setSelection((prev) =>
      prev && items.some((ch) => chunkKey(ch) === prev.key)
        ? null
        : { key: chunkKey(items[0]), items }
    );
  };

  const handleCloseDetails = useCallback(() => setSelection(null), []);

  // Прослушать только выбранный фрагмент
  const handlePlaySegment = (start: number, end: number) => {
    const el = audioRef.current;
    if (!el) return;
    el.currentTime = start;
    setCurrentTime(start);
    segmentEndRef.current = end;
    void el.play();
  };

  // Скачать фрагмент: файл декодируется в полном качестве (один раз на
  // файл), нужный отрезок вырезается и сохраняется в WAV.
  const handleDownloadChunk = async (chunk: Chunk, start: number, end: number) => {
    if (!file) return;
    if (!fullAudioRef.current || fullAudioRef.current.file !== file) {
      fullAudioRef.current = { file, promise: decodeAudioFull(file) };
    }
    let buffer: AudioBuffer;
    try {
      buffer = await fullAudioRef.current.promise;
    } catch {
      fullAudioRef.current = null;
      throw new Error('this audio format cannot be decoded by the browser');
    }
    const blob = encodeWavSegment(buffer, start, end);
    saveBlob(blob, chunkFileName(file.name, chunk, start, end));
  };

  const chunkSeconds = modelChunkSeconds(analyzedModel);
  // Шкала времени — та же, что у Waveform: длительность декодированного
  // файла, а пока его нет — длительность из <audio>.
  const timeline = audioData?.duration || duration;
  const selectedChunk = selection
    ? (chunks || []).find((ch) => chunkKey(ch) === selection.key) || null
    : null;

  return (
    <div className="app">
      <div className="container">
        {header && (
          <header className="header">
            <h1>audio inference</h1>
          </header>
        )}

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
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  triggerFileSelect();
                }
              }}
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

            <div className="row upload-actions">
              <button
                type="button"
                className="btn-primary"
                onClick={handleAnalyze}
                disabled={!file || !currentModelId || !currentModelReady || state === 'analyzing'}
              >
                {state === 'analyzing' ? 'analyzing...' : 'analyze'}
              </button>
              {file && (
                <button type="button" onClick={handleReset}>
                  reset
                </button>
              )}
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
            modelName={modelId(analyzedModel)}
            selectedKey={selection?.key}
            onSelectChunk={handleSelectChunk}
          />
        )}

        {/* Подробности о выбранном чанке — появляются по клику на плашку */}
        {selectedChunk && (
          <ChunkDetails
            chunk={selectedChunk}
            siblings={selection?.items ?? []}
            chunkSeconds={chunkSeconds}
            duration={timeline}
            fileName={file?.name}
            modelName={modelId(analyzedModel)}
            onSelect={(ch) =>
              setSelection((prev) => (prev ? { ...prev, key: chunkKey(ch) } : prev))
            }
            onClose={handleCloseDetails}
            onPlay={handlePlaySegment}
            onDownload={handleDownloadChunk}
          />
        )}

        {rawLog.length > 0 && (
          <div className="card">
            <strong className="card-title">debug: raw server responses</strong>
            <pre className="debug-log">
              {rawLog.map((entry, i) => (
                `[${i}] ${entry.url} → ${entry.status}\n` +
                JSON.stringify(entry.body, null, 2) +
                '\n\n'
              ))}
            </pre>
          </div>
        )}
      </div>

      {/* Плеер, прилипающий к низу экрана, — появляется, когда выбран файл */}
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
    </div>
  );
}
