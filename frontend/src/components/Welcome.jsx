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
      setError('Имя должно быть от 4 до 128 символов');
      return;
    }
    if (!/^[a-zA-Z0-9_]+$/.test(trimmed)) {
      setError('Имя может содержать только буквы, цифры и _');
      return;
    }
    
    setLoading(true);
    setError(null);
    try {
      await register(trimmed);
      onRegistered(trimmed);
    } catch (err) {
      setError('Не удалось зарегистрироваться: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="container">
      <h1>Привет 👋</h1>
      <div className="card">
        <p className="muted">Сервис анализа звуков машин. Введите имя, чтобы начать.</p>
        <form onSubmit={submit} className="row">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Ваше имя"
            autoFocus
          />
          <button type="submit" disabled={loading}>
            {loading ? '...' : 'Войти'}
          </button>
        </form>
        {error && <p className="error">{error}</p>}
      </div>
    </div>
  );
}