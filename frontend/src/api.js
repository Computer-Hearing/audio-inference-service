// src/api.js
//
// Бэкенд переписан на connect-rpc: вместо REST-ручек /api/v1/* теперь один
// сервис inference.v1.api.InferenceService (контракт —
// contracts/inference/v1/inference.proto). Клиентский код сгенерирован в
// src/gen командой `make generate-js` из папки contracts/.
//
// Важно: используем только inference_pb.js — в protobuf-es v2 описание
// сервиса (InferenceService) генерируется прямо туда. Файл
// inference_connect.js от плагина protoc-gen-connect-es рассчитан на старый
// @connectrpc/connect v1 и с v2 несовместим, поэтому не импортируется.
import { createClient, ConnectError, Code } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { toJson } from '@bufbuild/protobuf';
import {
  InferenceService,
  TaskStatus,
  CreateTaskResponseSchema,
  GetTaskResponseSchema,
} from './gen/inference/v1/inference_pb.js';

// Запросы идут на <BASE_URL>/inference.v1.api.InferenceService/<Method>,
// например /inference/inference.v1.api.InferenceService/GetModels.
const API_PREFIX = import.meta.env.BASE_URL.replace(/\/$/, '');

const transport = createConnectTransport({
  baseUrl: API_PREFIX || '/',
  // Бинарный protobuf вместо JSON: аудио уходит полем bytes, и в JSON оно
  // раздувалось бы на ~33% из-за base64 — упёрлись бы в лимит бэкенда раньше.
  useBinaryFormat: true,
  // Сессия держится на HttpOnly-куке username — её нужно отправлять
  // с каждым запросом, как раньше делал credentials: 'include'.
  fetch: (input, init) => globalThis.fetch(input, { ...init, credentials: 'include' }),
});

const client = createClient(InferenceService, transport);

// Бэкенд читает не больше 4 MiB + 64 KiB на сообщение
// (connect.WithReadMaxBytes в backend/cmd/main.go) — 64 KiB это запас под
// остальные поля запроса, сам файл должен укладываться в 4 MiB.
export const MAX_AUDIO_BYTES = 4 * 1024 * 1024;

// Текст ошибки без префикса "[code]", который ConnectError добавляет в message.
function errorText(e, fallback) {
  if (e instanceof ConnectError) return e.rawMessage || Code[e.code] || fallback;
  return (e && e.message) || fallback;
}

function errorLog(method, e) {
  const err = ConnectError.from(e);
  return {
    url: method,
    status: Code[err.code] || 'error',
    body: { code: Code[err.code], message: err.rawMessage },
  };
}

// -----------------------------------------------------------------------
// Сессия пользователя.
// Register с пустым username генерирует анонимное имя вида
// "Anonim-XXXXXXXXXXXXX-1234567890" и кладёт его в HttpOnly-куку.
// Прочитать её из JS нельзя и не нужно — достаточно, что браузер её хранит.
//
// Если кука уже существует, бэкенд отвечает кодом AlreadyExists (раньше
// это был 409 Conflict) — это не ошибка, а сигнал "сессия уже есть".
// -----------------------------------------------------------------------
export async function ensureSession() {
  try {
    await client.register({ username: '' });
    return true;
  } catch (e) {
    if (e instanceof ConnectError && e.code === Code.AlreadyExists) return true;
    console.warn('ensureSession: request failed', e);
    return false;
  }
}

// Вызов с авто-восстановлением сессии: если бэкенд ответил Unauthenticated
// (кука не пришла/протухла) — один раз регистрируемся заново и повторяем.
async function authorized(call) {
  try {
    return await call();
  } catch (e) {
    if (e instanceof ConnectError && e.code === Code.Unauthenticated) {
      const recovered = await ensureSession();
      if (recovered) return call();
    }
    throw e;
  }
}

// -----------------------------------------------------------------------
// Модели.
// GetModels отдаёт inference.v1.api.Model:
//   { name, projectTitle, projectDescription, secondsPerChunk,
//     targetClassesNum, categoryClassesNum, ready, state }
// name — идентификатор для CreateTask, projectTitle — для отображения,
// secondsPerChunk — длина чанка, по ней строятся и ряды чанков, и столбцы
// гистограммы.
// -----------------------------------------------------------------------
export const DEFAULT_CHUNK_SECONDS = 2; // pkg.DefaultSecondsPerAudioChunk на бэкенде

export function modelId(m) {
  return (m && m.name) || '';
}

export function modelLabel(m) {
  if (!m) return '';
  return m.projectTitle || m.name || '';
}

export function modelChunkSeconds(m) {
  if (m && Number(m.secondsPerChunk) > 0) return Number(m.secondsPerChunk);
  return DEFAULT_CHUNK_SECONDS;
}

export async function getModels() {
  try {
    const res = await authorized(() => client.getModels({}));
    // Хранилище моделей на бэкенде — map, порядок в ответе случайный.
    // Сортируем, чтобы стрелки переключали модели в стабильном порядке.
    return [...res.models].sort((a, b) => modelLabel(a).localeCompare(modelLabel(b)));
  } catch (e) {
    console.warn('getModels failed:', errorText(e, 'unknown error'));
    return [];
  }
}

export async function createTask(file, modelName, onRaw) {
  if (file.size > MAX_AUDIO_BYTES) {
    throw new Error(`file is too large (max ${MAX_AUDIO_BYTES / 1024 / 1024} MB)`);
  }

  const audioFile = new Uint8Array(await file.arrayBuffer());
  const method = 'InferenceService/CreateTask';

  try {
    const res = await authorized(() =>
      client.createTask({ audioFile, filename: file.name, modelName })
    );
    if (onRaw) onRaw({ url: method, status: 'OK', body: toJson(CreateTaskResponseSchema, res) });
    return res.task; // { taskId, status, model }
  } catch (e) {
    if (onRaw) onRaw(errorLog(method, e));
    throw new Error(errorText(e, 'createTask failed'));
  }
}

// Временные ошибки, при которых имеет смысл продолжать опрос.
const TRANSIENT_CODES = new Set([
  Code.Internal,
  Code.Unavailable,
  Code.Unknown,
  Code.DeadlineExceeded,
]);

export async function pollTask(taskId, onRaw) {
  const started = Date.now();
  let delay = 500;
  let lastErrorText = '';
  const method = 'InferenceService/GetTask';

  while (true) {
    try {
      const res = await authorized(() => client.getTask({ taskId }));
      if (onRaw) onRaw({ url: method, status: 'OK', body: toJson(GetTaskResponseSchema, res) });

      const task = res.task;
      if (task?.status === TaskStatus.STATUS_SUCCESS) return task;
      if (task?.status === TaskStatus.STATUS_FAILURE) {
        const failed = task.result?.chunks?.find((ch) => ch.errorMessage);
        throw new Error(failed?.errorMessage || 'task failed');
      }
      // STATUS_PENDING / STATUS_PROCESSING — просто ждём дальше
    } catch (e) {
      if (!(e instanceof ConnectError)) throw e;
      if (onRaw) onRaw(errorLog(method, e));
      if (!TRANSIENT_CODES.has(e.code)) {
        // InvalidArgument / NotFound / Unauthenticated — опрашивать бессмысленно
        throw new Error(errorText(e, 'pollTask failed'));
      }
      lastErrorText = errorText(e, 'pollTask failed');
    }

    if (Date.now() - started > 90000) {
      throw new Error(lastErrorText || 'timeout');
    }

    await new Promise((r) => setTimeout(r, delay));
    delay = Math.min(delay + 500, 5000);
  }
}
