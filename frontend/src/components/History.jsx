// src/components/History.jsx
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
      <div className="row" style={{ justifyContent: 'space-between', marginBottom: 16 }}>
        <strong style={{ fontSize: 18, color: '#aaa', letterSpacing: 1 }}>history</strong>
        <button onClick={handleClear} disabled={!items.length}>clear</button>
      </div>
      {loading && <p className="muted" style={{ fontSize: 16 }}>loading...</p>}
      {!loading && items.length === 0 && <p className="muted" style={{ fontSize: 16 }}>empty</p>}
      <ul style={{ paddingLeft: 20, margin: 0 }}>
        {items.map((it, i) => (
          <li key={i} className="muted" style={{ marginBottom: 6, fontSize: 15 }}>
            {it.filename || '(no name)'} — {it.chunks?.length || 0} chunks
          </li>
        ))}
      </ul>
    </div>
  );
}