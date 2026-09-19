// src/components/Player.jsx
// Плеер, закреплённый внизу страницы (по аналогии с музыкальными сервисами:
// Spotify / Apple Music / Yandex Music). Появляется, когда в Main.jsx выбран
// файл, и остаётся видимым независимо от прокрутки страницы.
import { useRef } from 'react';

function formatTime(sec) {
  if (!isFinite(sec) || sec < 0) sec = 0;
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  const ms = Math.floor((sec - Math.floor(sec)) * 1000);
  return `${m}:${s.toString().padStart(2, '0')}.${ms.toString().padStart(3, '0')}`;
}

function PlayIcon() {
  return (
    <svg className="player-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M7 4.5v15l13-7.5-13-7.5z" />
    </svg>
  );
}

function PauseIcon() {
  return (
    <svg className="player-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M6 4.5h4.5v15H6zM13.5 4.5H18v15h-4.5z" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg
      className="player-icon player-icon-small"
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

export default function Player({
  fileName,
  modelLabel,
  playing,
  currentTime,
  duration,
  onPlayPause,
  onSeek,
  onClose,
}) {
  const trackRef = useRef(null);

  const pct = duration > 0 ? Math.min(100, (currentTime / duration) * 100) : 0;

  const seekFromEvent = (e) => {
    const el = trackRef.current;
    if (!el || !duration) return;
    const rect = el.getBoundingClientRect();
    const ratio = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
    onSeek(ratio * duration);
  };

  return (
    <div className="player-bar" role="region" aria-label="audio player">
      <div className="player-bar-inner">
        <button
          type="button"
          className="player-play-btn"
          onClick={onPlayPause}
          aria-label={playing ? 'pause' : 'play'}
        >
          {playing ? <PauseIcon /> : <PlayIcon />}
        </button>

        <div className="player-track-info">
          <div className="player-track-name" title={fileName}>{fileName || 'untitled'}</div>
          <div className="player-track-model muted">{modelLabel || '—'}</div>
        </div>

        <span className="player-time">{formatTime(currentTime)}</span>

        <div className="player-progress" ref={trackRef} onClick={seekFromEvent}>
          <div className="player-progress-fill" style={{ width: `${pct}%` }} />
          <div className="player-progress-thumb" style={{ left: `${pct}%` }} />
        </div>

        <span className="player-time">{formatTime(duration)}</span>

        <button type="button" className="player-close-btn" onClick={onClose} aria-label="close player">
          <CloseIcon />
        </button>
      </div>
    </div>
  );
}
