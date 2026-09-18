// src/components/Main.jsx
import { useEffect, useRef, useState } from 'react';
import { register, getModels, createTask, pollTask } from '../api.js';
import Spectrogram from './Spectrogram.jsx';
import History from './History.jsx';
import Player from './Player.jsx';

// -----------------------------------------------------------------------
// TODO(backend): это временная заглушка списка моделей.
// Как только GET /api/v1/models начнёт отдавать реальный список — заглушка
// перестанет использоваться сама собой (см. useEffect ниже: реальный список
// с бэкенда всегда имеет приоритет, если он не пустой).
// -----------------------------------------------------------------------
const MOCK_MODELS = [
  { name: 'transport audio' },
  { name: 'siren audio' },
];

function truncateName(name, max = 22) {
  if (!name || name.length <= max) return name;
  const half = Math.floor((max - 1) / 2);
  return `${name.slice(0, half)}…${name.slice(name.length - half)}`;
}

// Иконка загрузки (стрелка в лоток)
function UploadIcon() {
  return (
    <svg className="upload-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M12 16V4" />
      <path d="M8 8l4-4 4 4" />
      <path d="M4 14v2a2 2 0 002 2h12a2 2 0 002-2v-2" />
    </svg>
  );
}

export default function Main({ displayName, onLogout, onRename }) {
  const [models, setModels] = useState([]);
  const [model, setModel] = useState('');
  const [file, setFile] = useState(null);
  const [dragOver, setDragOver] = useState(false);

  const [state, setState] = useState('idle');
  const [error, setError] = useState(null);
  const [spectrogram, setSpectrogram] = useState(null);
  const [chunks, setChunks] = useState(null);

  const [audioUrl, setAudioUrl] = useState(null);
  const [duration, setDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [playing, setPlaying] = useState(false);

  const [renameOn, setRenameOn] = useState(false);
  const [newName, setNewName] = useState(displayName);
  const [historyOn, setHistoryOn] = useState(false);

  const [rawLog, setRawLog] = useState([]);

  const audioRef = useRef(null);
  const fileInputRef = useRef(null);

  const handleRaw = (info) => setRawLog((prev) => [...prev, info]);

  useEffect(() => {
    getModels()
      .then((list) => {
        // Реальный список с бэкенда — приоритет. Пусто/бэкенд ещё не готов —
        // показываем заглушку, чтобы UI выбора моделей можно было делать уже сейчас.
        const finalList = list && list.length ? list : MOCK_MODELS;
        setModels(finalList);
        setModel(finalList[0].name || finalList[0]);
      })
      .catch(() => {
        setModels(MOCK_MODELS);
        setModel(MOCK_MODELS[0].name);
      });
  }, []);

  const processFile = (f) => {
    if (!f) return;
    setFile(f);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(URL.createObjectURL(f));
    setSpectrogram(null);
    setChunks(null);
    setState('idle');
    setCurrentTime(0);
  };

  const handleAnalyze = async () => {
    if (!file) return;
    setError(null);
    setRawLog([]);
    setState('analyzing');
    try {
      const created = await createTask(file, model, handleRaw);
      setSpectrogram(created.waves || []);

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

  const handleRenameSubmit = async (e) => {
    e.preventDefault();
    const n = newName.trim();
    if (!n || n.length < 4 || n.length > 128 || !/^[a-zA-Z0-9_]+$/.test(n)) return;
    try {
      await register(n);
      onRename(n);
      setRenameOn(false);
    } catch {}
  };

  const handleReset = () => {
    setFile(null);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(null);
    setSpectrogram(null);
    setChunks(null);
    setState('idle');
    setError(null);
    setCurrentTime(0);
    setDuration(0);
    setRawLog([]);
  };

  const triggerFileSelect = () => {
    fileInputRef.current.click();
  };

  return (
    <>
      <div className={`container${audioUrl ? ' has-player' : ''}`}>
        {/* Шапка */}
        <header className="header">
          <h1>audio inference</h1>
          <div className="row">
            <button onClick={() => setHistoryOn((v) => !v)}>history</button>
            {!renameOn ? (
              <button onClick={() => { setNewName(displayName); setRenameOn(true); }}>
                {displayName}
              </button>
            ) : (
              <form onSubmit={handleRenameSubmit} className="row">
                <input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="new name"
                />
                <button type="submit">ok</button>
                <button type="button" onClick={() => setRenameOn(false)}>cancel</button>
              </form>
            )}
            <button onClick={onLogout}>logout</button>
          </div>
        </header>

        {/* История (если включена) */}
        {historyOn && <History open={historyOn} />}

        {/* Основная область – центрирование блока загрузки */}
        <div className="main-area">
          <div className="card" style={{ maxWidth: 700, textAlign: 'center' }}>
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '18px' }}>

              {/* Объёмная сфера загрузки файла */}
              <div
                className={`upload-sphere${dragOver ? ' drag-over' : ''}${file ? ' has-file' : ''}`}
                onClick={triggerFileSelect}
                onDragOver={handleDragOver}
                onDragLeave={handleDragLeave}
                onDrop={handleDrop}
                role="button"
                tabIndex={0}
                aria-label="select audio file"
              >
                <span className="upload-sphere-ring" />
                <span className="upload-sphere-highlight" />
                <span className="upload-sphere-content">
                  <UploadIcon />
                  {file && (
                    <span className="upload-sphere-filename">{truncateName(file.name)}</span>
                  )}
                </span>
              </div>
              <input
                type="file"
                accept="audio/*,.mp3,.wav"
                onChange={handleFileChange}
                ref={fileInputRef}
              />

              {!file && (
                <p className="muted" style={{ marginTop: -6 }}>
                  click the sphere or drop an audio file
                </p>
              )}

              {/* Подпись модели под сферой + выбор из доступных моделей */}
              <div className="model-tabs">
                {models.map((m, i) => {
                  const name = m.name || m;
                  return (
                    <button
                      key={i}
                      type="button"
                      className={`model-tab${model === name ? ' active' : ''}`}
                      onClick={() => setModel(name)}
                    >
                      {name}
                    </button>
                  );
                })}
              </div>

              {/* Панель управления */}
              <div className="row" style={{ justifyContent: 'center' }}>
                <button onClick={handleAnalyze} disabled={!file || state === 'analyzing'}>
                  {state === 'analyzing' ? 'analyzing...' : 'analyze'}
                </button>
                {(file || spectrogram) && <button onClick={handleReset}>reset</button>}
              </div>
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

        {/* Спектрограмма и остальной контент – идёт ниже, но уже не центрируется */}
        {spectrogram && (
          <Spectrogram
            spectrogram={spectrogram}
            chunks={chunks}
            duration={duration}
            currentTime={currentTime}
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
          modelLabel={model}
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
