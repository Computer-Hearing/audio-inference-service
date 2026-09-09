const API_PREFIX = import.meta.env.BASE_URL.replace(/\/$/, '');

function safeParse(text) {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export async function register(name) {
  const res = await fetch(`${API_PREFIX}/api/v1/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ username: name }),
  });

  if (!res.ok) {
    if (res.status === 409) {
      localStorage.setItem('displayName', name);
      return name;
    }
    const err = await res.json().catch(() => ({}));
    throw new Error(err.message || 'register failed: ' + res.status);
  }

  localStorage.setItem('displayName', name);
  return name;
}

export async function getModels() {
  const res = await fetch(`${API_PREFIX}/api/v1/models`, {
    credentials: 'include',
  });

  if (!res.ok) {
    console.warn('getModels failed:', res.status);
    return [];
  }

  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function createTask(file, model, onRaw) {
  const fd = new FormData();
  fd.append('audio', file);

  const headers = {};
  if (model) {
    headers['X-Model'] = model;
  }

  const res = await fetch(`${API_PREFIX}/api/v1/tasks`, {
    method: 'POST',
    credentials: 'include',
    headers: headers,
    body: fd,
  });

  const bodyText = await res.text();
  const body = safeParse(bodyText);
  console.log('[createTask]', res.status, body);
  if (onRaw) onRaw({ url: `POST ${API_PREFIX}/api/v1/tasks`, status: res.status, body });

  if (!res.ok) {
    throw new Error(body?.message || 'createTask failed: ' + res.status);
  }

  return body;
}

export async function pollTask(taskId, onRaw) {
  const started = Date.now();
  let delay = 500;
  let lastErrorText = '';

  while (true) {
    const res = await fetch(`${API_PREFIX}/api/v1/tasks/${taskId}`, {
      credentials: 'include',
    });

    const bodyText = await res.text();
    const body = safeParse(bodyText);
    console.log('[pollTask]', res.status, body);
    if (onRaw) onRaw({ url: `GET ${API_PREFIX}/api/v1/tasks/${taskId}`, status: res.status, body });

    if (res.ok) {
      if (body.status === 'success') return body;
      if (body.status === 'failure') {
        throw new Error(body.result?.error || 'task failed');
      }
      // status === 'pending' — просто ждём дальше
    } else if (res.status === 500) {
      // Известный баг бэкенда: result=NULL, пока задача pending.
      // НЕ падаем — продолжаем опрашивать, запоминаем текст ошибки.
      lastErrorText = body?.message || 'pollTask failed: ' + res.status;
    } else {
      // 400 / 401 / 404 — реальная ошибка, опрашивать бессмысленно
      throw new Error(body?.message || 'pollTask failed: ' + res.status);
    }

    if (Date.now() - started > 90000) {
      throw new Error(lastErrorText || 'timeout');
    }

    await new Promise((r) => setTimeout(r, delay));
    delay = Math.min(delay + 500, 5000);
  }
}

export async function getHistory() {
  const res = await fetch(`${API_PREFIX}/api/v1/tasks/history`, {
    credentials: 'include',
  });
  if (!res.ok) throw new Error('getHistory failed: ' + res.status);
  return res.json();
}

export async function clearHistory() {
  const res = await fetch(`${API_PREFIX}/api/v1/tasks/history`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!res.ok) throw new Error('clearHistory failed: ' + res.status);
}