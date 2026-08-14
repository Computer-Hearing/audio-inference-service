import { useEffect, useRef, useState } from 'react';
import { register, getModels, createTask, pollTask } from '../api.js';
import Spectrogram from './Spectrogram.jsx';
import History from './History.jsx';

export default function Main({ displayName, onLogout, onRename }) {
  const [models, setModels] = useState([]);
  const [model, setModel] = useState('');
  const [file, setFile] = useState(null);

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

  // отладочный лог: все сырые запросы/ответы сервера
  const [rawLog, setRawLog] = useState([]);

  const audioRef = useRef(null);

  const handleRaw = (info) => setRawLog((prev) => [...prev, info]);

  useEffect(() => {
    getModels()
      .then((list) => {
        setModels(list);
        if (list.length) setModel(list[0].name || list[0]);
      })
      .catch(() => {});
  }, []);

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
          ? 'Слишком долго. Результат может появиться в истории позже.'
          : 'Ошибка: ' + e.message
      );
    }
  };

  const handleFileChange = (e) => {
    const f = e.target.files && e.target.files[0];
    if (!f) return;
    setFile(f);
    if (audioUrl) URL.revokeObjectURL(audioUrl);
    setAudioUrl(URL.createObjectURL(f));
    setSpectrogram(null);
    setChunks(null);
    setState('idle');
    setCurrentTime(0);
  };

  const handlePlayPause = () => {
    const el = audioRef.current;
    if (!el) return;
    if (el.paused) el.play();
    else el.pause();
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

  return (
    <div className="container">
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 20 }}>
        <h1 style={{ margin: 0 }}>Анализ звука машин</h1>
        <div className="row">
          <button onClick={() => setHistoryOn((v) => !v)}>История</button>
          {!renameOn ? (
            <button onClick={() => { setNewName(displayName); setRenameOn(true); }}>
              {displayName}
            </button>
          ) : (
            <form onSubmit={handleRenameSubmit} className="row">
              <input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="новое имя"
              />
              <button type="submit">ок</button>
              <button type="button" onClick={() => setRenameOn(false)}>✕</button>
            </form>
          )}
          <button onClick={onLogout}>выйти</button>
        </div>
      </div>

      {historyOn && <History open={historyOn} />}

      <div className="card">
        <div className="row">
          <input type="file" accept="audio/*,.mp3,.wav" onChange={handleFileChange} />
          {models.length > 0 && (
            <select value={model} onChange={(e) => setModel(e.target.value)}>
              {models.map((m, i) => (
                <option key={i} value={m.name || m}>{m.name || m}</option>
              ))}
            </select>
          )}
          <button onClick={handleAnalyze} disabled={!file || state === 'analyzing'}>
            {state === 'analyzing' ? 'Анализ...' : 'Анализировать'}
          </button>
          {audioUrl && (
            <button onClick={handlePlayPause}>{playing ? 'Стоп' : 'Слушать'}</button>
          )}
          {(file || spectrogram) && <button onClick={handleReset}>сброс</button>}
        </div>

        {state === 'error' && <p className="error">{error}</p>}

        {audioUrl && (
          <audio
            ref={audioRef}
            src={audioUrl}
            controls
            onPlay={() => setPlaying(true)}
            onPause={() => setPlaying(false)}
            onTimeUpdate={(e) => setCurrentTime(e.currentTarget.currentTime)}
            onLoadedMetadata={(e) => setDuration(e.currentTarget.duration || 0)}
            style={{ width: '100%', marginTop: 12 }}
          />
        )}
      </div>

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
          <strong>Отладка: сырые ответы сервера</strong>
          <pre
            style={{
              whiteSpace: 'pre-wrap',
              fontSize: 12,
              color: '#9fb3c8',
              maxHeight: 300,
              overflow: 'auto',
              marginTop: 8,
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
  );
}