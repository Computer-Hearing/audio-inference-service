// src/components/History.jsx
//
// Бэкенд отдаёт историю строго как []{ filename, chunks } — без task_id,
// модели и даты (см. sqliteTaskManager.GetHistory: в БД выбирается только
// колонка result, которая и есть FileInferenceResult). Ничего из этого не
// хранится и не дорисовывается на фронтенде — только то, что реально
// прислал сервер, но представлено чуть понятнее сырого счётчика чанков.
import { useEffect, useState } from 'react';
import { getHistory, clearHistory } from '../api.js';

const CATEGORY = { 0: 'car', 1: 'emv', 2: 'motorcycle', 3: 'tram', 4: 'truck' };
const TARGET = {
  0: 'acceleration', 1: 'bell', 2: 'braking', 3: 'horn',
  4: 'idling', 5: 'passing', 6: 'siren',
};

function argmaxIndex(arr) {
  if (!arr || !arr.length) return -1;
  let best = 0;
  for (let i = 1; i < arr.length; i++) if (arr[i] > arr[best]) best = i;
  return best;
}

// Короткая сводка по файлу: какие категории/звуки были обнаружены и сколько
// чанков не удалось обработать. Считаем только по базовому слою (offset=0),
// чтобы не задваивать одни и те же события из-за перекрывающегося слоя +1s.
function summarize(chunks) {
  const base = (chunks || []).filter((ch) => (ch.offset || 0) === 0);

  const categoryCounts = new Map();
  const targetCounts = new Map();
  let errors = 0;

  base.forEach((ch) => {
    if (ch.error) {
      errors += 1;
      return;
    }
    const cat = CATEGORY[argmaxIndex(ch.category)];
    const tgt = TARGET[argmaxIndex(ch.target)];
    if (cat) categoryCounts.set(cat, (categoryCounts.get(cat) || 0) + 1);
    if (tgt) targetCounts.set(tgt, (targetCounts.get(tgt) || 0) + 1);
  });

  const topEntries = (map, limit) =>
    [...map.entries()].sort((a, b) => b[1] - a[1]).slice(0, limit);

  return {
    totalChunks: base.length,
    errors,
    categories: topEntries(categoryCounts, 4),
    targets: topEntries(targetCounts, 4),
  };
}

export default function History({ open }) {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError(null);
    getHistory()
      .then((data) => setItems(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message || 'failed to load history'))
      .finally(() => setLoading(false));
  }, [open]);

  const handleClear = async () => {
    try {
      await clearHistory();
      setItems([]);
    } catch (e) {
      setError(e.message || 'failed to clear history');
    }
  };

  return (
    <div className="card">
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 16 }}>
        <strong style={{ fontSize: 18, color: '#aaa', letterSpacing: 1 }}>history</strong>
        <button onClick={handleClear} disabled={!items.length}>clear</button>
      </div>

      {loading && <p className="muted" style={{ fontSize: 16 }}>loading...</p>}
      {!loading && error && <p className="error">{error}</p>}
      {!loading && !error && items.length === 0 && <p className="muted" style={{ fontSize: 16 }}>empty</p>}

      <ul className="history-list">
        {items.map((it, i) => {
          const summary = summarize(it.chunks);
          return (
            <li key={i} className="history-item">
              <div className="history-item-filename">{it.filename || '(no name)'}</div>
              <div className="history-item-meta muted">
                {summary.totalChunks} chunk{summary.totalChunks === 1 ? '' : 's'}
                {summary.errors > 0 && `, ${summary.errors} error${summary.errors === 1 ? '' : 's'}`}
              </div>
              {(summary.categories.length > 0 || summary.targets.length > 0) && (
                <div className="history-item-tags">
                  {summary.categories.map(([name, count]) => (
                    <span key={`c-${name}`} className="history-tag">{name} ×{count}</span>
                  ))}
                  {summary.targets.map(([name, count]) => (
                    <span key={`t-${name}`} className="history-tag history-tag-target">{name} ×{count}</span>
                  ))}
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
