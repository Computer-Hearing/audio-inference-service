// src/components/Welcome.jsx
// ОРИГИНАЛЬНАЯ ВЕРСИЯ: работает с бэкендом через API
import { useState } from 'react';
import { register } from '../api.js';

export default function Welcome({ onRegistered }) {
  const [name, setName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const submit = async (e) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;

    if (trimmed.length < 4 || trimmed.length > 128) {
      setError('Name must be between 4 and 128 characters');
      return;
    }
    if (!/^[a-zA-Z0-9_]+$/.test(trimmed)) {
      setError('Only letters, digits and underscore allowed');
      return;
    }

    setLoading(true);
    setError(null);
    try {
      await register(trimmed);
      onRegistered(trimmed);
    } catch (err) {
      setError('Registration failed: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="container" style={{ maxWidth: 600, minHeight: '100vh', justifyContent: 'center' }}>
      <div style={{ textAlign: 'center' }}>
        <h1 style={{ fontSize: 36, marginBottom: 12 }}>audio inference</h1>
        <p className="muted" style={{ fontSize: 18, marginBottom: 32 }}>
          machine sound analysis service
        </p>
        <form onSubmit={submit} style={{ display: 'flex', gap: 16, justifyContent: 'center', alignItems: 'center' }}>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="your username"
            autoFocus
            style={{ minWidth: 220, fontSize: 18, padding: '10px 8px' }}
          />
          <button type="submit" disabled={loading} style={{ fontSize: 18 }}>
            {loading ? '...' : 'enter'}
          </button>
        </form>
        {error && <p className="error" style={{ fontSize: 16 }}>{error}</p>}
      </div>
    </div>
  );
}