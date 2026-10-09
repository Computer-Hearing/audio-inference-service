import { useEffect, useState } from 'react';
import Main from './components/Main.jsx';
import { ensureSession } from './api.js';

export default function App() {
  // Имя пользователя больше не вводится вручную и не хранится на фронтенде —
  // бэкенд сам генерирует анонимное имя (Anonim-XXXXXXXXXXXXX-1234567890) и
  // кладёт его в HttpOnly-куку при первом вызове Register.
  // Нам достаточно один раз убедиться, что сессия установлена, прежде чем
  // дергать остальные методы (GetModels/CreateTask/GetTask), которым кука обязательна.
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    ensureSession().finally(() => {
      if (!cancelled) setReady(true);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!ready) {
    // Обычно занимает доли секунды — просто не даём странице мигнуть
    // ошибками Unauthenticated от компонентов, которые запросят данные раньше времени.
    return (
      <div className="container" style={{ minHeight: '100vh', justifyContent: 'center', alignItems: 'center' }}>
        <p className="muted">connecting…</p>
      </div>
    );
  }

  return <Main />;
}
