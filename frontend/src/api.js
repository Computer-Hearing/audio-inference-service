const API_PREFIX = import.meta.env.BASE_URL.replace(/\/$/, '');

function safeParse(text) {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

// Бэкенд отдаёт ошибки как { "error": "..." } (см. pkg.SendError / pkg.jsonError),
// но некоторые старые места ожидали { "message": "..." } — поддерживаем оба
// варианта, чтобы не потерять текст ошибки, если формат вдруг изменится.
function extractErrorMessage(body, fallback) {
  if (body && typeof body === 'object') {
    return body.error || body.message || fallback;
  }
  if (typeof body === 'string' && body.trim()) return body;
  return fallback;
}

// -----------------------------------------------------------------------
// Сессия пользователя.
// По требованиям бэкенда имя пользователя больше не вводится вручную:
// POST /api/v1/register с пустым username генерирует анонимное имя вида
// "Anonim-XXXXXXXXXXXXX-1234567890" (см. pkg.UsernameGenerator) и кладёт его
// в HttpOnly-куку. Кука httpOnly — прочитать её из JS нельзя и не нужно,
// достаточно, что она есть у браузера и подставляется в credentials: 'include'.
//
// Если кука уже существует, бэкенд отвечает 409 Conflict — это не ошибка,
// а сигнал "сессия уже есть", поэтому обрабатываем его как успех.
// -----------------------------------------------------------------------
export async function ensureSession() {
  try {
    const res = await fetch(`${API_PREFIX}/api/v1/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ username: '' }),
    });

    if (res.ok || res.status === 409) return true;

    const text = await res.text().catch(() => '');
    console.warn('ensureSession: unexpected response', res.status, text);
    return false;
  } catch (e) {
    console.warn('ensureSession: request failed', e);
    return false;
  }
}

// Универсальный fetch с авто-восстановлением сессии: если бэкенд вдруг
// ответил 401 (кука не пришла/протухла) — один раз пытаемся зарегистрироваться
// заново и повторяем запрос.
async function authorizedFetch(url, options, _retried = false) {
  const res = await fetch(url, { credentials: 'include', ...options });
  if (res.status === 401 && !_retried) {
    const recovered = await ensureSession();
    if (recovered) return authorizedFetch(url, options, true);
  }
  return res;
}

// -----------------------------------------------------------------------
// Модели.
// Сейчас GET /api/v1/models отдаёт catalog.ModelInfo:
//   { name, backend, version, state, ready, usable, inputs, outputs }
// В ближайшем будущем формат расширится до отдельной сущности со своими
// полями отображения и параметрами нарезки:
//   { id, title, model_name, description, seconds_per_chunk }
// Хелперы ниже понимают оба варианта, чтобы фронтенд не пришлось трогать,
// когда бэкенд переключится на новый формат.
// -----------------------------------------------------------------------
export const DEFAULT_CHUNK_SECONDS = 2; // pkg.DefaultSecondsPerAudioChunk на бэкенде

export function modelId(m) {
  if (!m) return '';
  if (typeof m === 'string') return m;
  return m.model_name || m.name || '';
}

export function modelLabel(m) {
  if (!m) return '';
  if (typeof m === 'string') return m;
  return m.title || m.name || m.model_name || '';
}

export function modelChunkSeconds(m) {
  if (m && typeof m === 'object' && Number(m.seconds_per_chunk) > 0) {
    return Number(m.seconds_per_chunk);
  }
  return DEFAULT_CHUNK_SECONDS;
}

export async function getModels() {
  const res = await authorizedFetch(`${API_PREFIX}/api/v1/models`);

  if (!res.ok) {
    console.warn('getModels failed:', res.status);
    return [];
  }

  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function createTask(file, modelName, onRaw) {
  const fd = new FormData();
  fd.append('audio', file);

  const headers = {};
  if (modelName) {
    headers['X-Model'] = modelName;
  }

  const res = await authorizedFetch(`${API_PREFIX}/api/v1/tasks`, {
    method: 'POST',
    headers,
    body: fd,
  });

  const bodyText = await res.text();
  const body = safeParse(bodyText);
  if (onRaw) onRaw({ url: `POST ${API_PREFIX}/api/v1/tasks`, status: res.status, body });

  if (!res.ok) {
    throw new Error(extractErrorMessage(body, 'createTask failed: ' + res.status));
  }

  return body; // { task_id, waves, model }
}

export async function pollTask(taskId, onRaw) {
  const started = Date.now();
  let delay = 500;
  let lastErrorText = '';

  while (true) {
    const res = await authorizedFetch(`${API_PREFIX}/api/v1/tasks/${taskId}`);

    const bodyText = await res.text();
    const body = safeParse(bodyText);
    if (onRaw) onRaw({ url: `GET ${API_PREFIX}/api/v1/tasks/${taskId}`, status: res.status, body });

    if (res.ok) {
      if (body.status === 'success') return body;
      if (body.status === 'failure') {
        throw new Error(extractErrorMessage(body?.result, 'task failed'));
      }
      // status === 'pending' / 'processing' — просто ждём дальше
    } else if (res.status === 500) {
      // Известный краевой случай: result ещё NULL, пока задача pending.
      // НЕ падаем — продолжаем опрашивать, запоминаем текст ошибки на случай таймаута.
      lastErrorText = extractErrorMessage(body, 'pollTask failed: ' + res.status);
    } else {
      // 400 / 401 / 404 — реальная ошибка, опрашивать бессмысленно
      throw new Error(extractErrorMessage(body, 'pollTask failed: ' + res.status));
    }

    if (Date.now() - started > 90000) {
      throw new Error(lastErrorText || 'timeout');
    }

    await new Promise((r) => setTimeout(r, delay));
    delay = Math.min(delay + 500, 5000);
  }
}

export async function getHistory() {
  const res = await authorizedFetch(`${API_PREFIX}/api/v1/tasks/history`);
  if (!res.ok) {
    const body = safeParse(await res.text().catch(() => ''));
    throw new Error(extractErrorMessage(body, 'getHistory failed: ' + res.status));
  }
  const data = await res.json();
  // Бэкенд отдаёт []*chunks.FileInferenceResult — [{ filename, chunks }, ...]
  return Array.isArray(data) ? data : [];
}

export async function clearHistory() {
  const res = await authorizedFetch(`${API_PREFIX}/api/v1/tasks/history`, {
    method: 'DELETE',
  });
  if (!res.ok) {
    const body = safeParse(await res.text().catch(() => ''));
    throw new Error(extractErrorMessage(body, 'clearHistory failed: ' + res.status));
  }
}