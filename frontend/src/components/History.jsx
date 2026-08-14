import { useEffect, useState } from 'react';
import { getHistory, clearHistory } from '../api.js';

export default function History({ open }) {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    getHistory()
      .then((data) => setItems(Array.isArray(data) ? data : []))
      .catch(() => setItems([]))
      .finally(() => setLoading(false));
  }, [open]);

  const handleClear = async () => {
    await clearHistory();
    setItems([]);
  };

  return (
    <div className="card">
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 12 }}>
        <strong>История запросов</strong>
        <button onClick={handleClear} disabled={!items.length}>Очистить</button>
      </div>
      {loading && <p className="muted">Загрузка...</p>}
      {!loading && items.length === 0 && <p className="muted">Пусто</p>}
      <ul style={{ paddingLeft: 20, margin: 0 }}>
        {items.map((it, i) => (
          <li key={i} className="muted" style={{ marginBottom: 4 }}>
            {it.filename || '(без имени)'} — {it.chunks?.length || 0} чанков
          </li>
        ))}
      </ul>
    </div>
  );
}