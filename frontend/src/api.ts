// src/api.ts
//
// Общение с бэкендом инференса — сервис inference.v1.api.InferenceService
// (контракт — contracts/inference/v1/inference.proto). Клиентский код
// сгенерирован в src/gen командой `make generate-js` из папки contracts/.
//
// Клиент создаётся функцией createInferenceApi(baseUrl), а не один раз на
// весь модуль: адрес API передаёт тот, кто встраивает инференс (портал),
// либо он берётся по умолчанию — см. src/index.tsx.
import { createClient, ConnectError, Code } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { toJson, type JsonValue } from '@bufbuild/protobuf';
import {
  InferenceService,
  TaskStatus,
  CreateTaskResponseSchema,
  GetTaskResponseSchema,
  type Model,
  type Task,
} from './gen/inference/v1/inference_pb.ts';

// Бэкенд читает не больше 4 MiB + 64 KiB на сообщение
// (connect.WithReadMaxBytes в backend/cmd/main.go) — 64 KiB это запас под
// остальные поля запроса, сам файл должен укладываться в 4 MiB.
export const MAX_AUDIO_BYTES = 4 * 1024 * 1024;

// Запись для блока "debug: raw server responses"
export interface RawLogEntry {
  url: string;
  status: string;
  body: JsonValue;
}
export type OnRaw = (entry: RawLogEntry) => void;

// Текст ошибки без префикса "[code]", который ConnectError добавляет в message.
function errorText(e: unknown, fallback: string): string {
  if (e instanceof ConnectError) return e.rawMessage || Code[e.code] || fallback;
  if (e instanceof Error) return e.message || fallback;
  return fallback;
}

function errorLog(method: string, e: unknown): RawLogEntry {
  const err = ConnectError.from(e);
  return {
    url: method,
    status: Code[err.code] || 'error',
    body: { code: Code[err.code], message: err.rawMessage },
  };
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

export function modelId(m: Model | null | undefined): string {
  return m?.name || '';
}

export function modelLabel(m: Model | null | undefined): string {
  if (!m) return '';
  return m.projectTitle || m.name || '';
}

export function modelChunkSeconds(m: Model | null | undefined): number {
  if (m && m.secondsPerChunk > 0) return m.secondsPerChunk;
  return DEFAULT_CHUNK_SECONDS;
}

// Временные ошибки, при которых имеет смысл продолжать опрос.
const TRANSIENT_CODES = new Set<Code>([
  Code.Internal,
  Code.Unavailable,
  Code.Unknown,
  Code.DeadlineExceeded,
]);

export interface InferenceApi {
  getModels(): Promise<Model[]>;
  createTask(file: File, modelName: string, onRaw?: OnRaw): Promise<Task>;
  pollTask(taskId: string, onRaw?: OnRaw): Promise<Task>;
}

/**
 * Клиент API инференса.
 * @param baseUrl адрес, к которому дописывается
 *   /inference.v1.api.InferenceService/<Метод>, например
 *   https://inference.kuronami.fun или '' (тот же сайт).
 */
export function createInferenceApi(baseUrl: string): InferenceApi {
  const transport = createConnectTransport({
    baseUrl: baseUrl || '/',
    // Бинарный protobuf вместо JSON: аудио уходит полем bytes, и в JSON оно
    // раздувалось бы на ~33% из-за base64 — упёрлись бы в лимит бэкенда раньше.
    useBinaryFormat: true,
  });
  const client = createClient(InferenceService, transport);

  async function getModels(): Promise<Model[]> {
    try {
      const res = await client.getModels({});
      // Хранилище моделей на бэкенде — map, порядок в ответе случайный.
      // Сортируем, чтобы стрелки переключали модели в стабильном порядке.
      return [...res.models].sort((a, b) => modelLabel(a).localeCompare(modelLabel(b)));
    } catch (e) {
      console.warn('getModels failed:', errorText(e, 'unknown error'));
      return [];
    }
  }

  async function createTask(file: File, modelName: string, onRaw?: OnRaw): Promise<Task> {
    if (file.size > MAX_AUDIO_BYTES) {
      throw new Error(`file is too large (max ${MAX_AUDIO_BYTES / 1024 / 1024} MB)`);
    }

    const audioFile = new Uint8Array(await file.arrayBuffer());
    const method = 'InferenceService/CreateTask';

    try {
      const res = await client.createTask({ audioFile, filename: file.name, modelName });
      onRaw?.({ url: method, status: 'OK', body: toJson(CreateTaskResponseSchema, res) });
      if (!res.task) throw new Error('empty response');
      return res.task; // { taskId, status, model }
    } catch (e) {
      onRaw?.(errorLog(method, e));
      throw new Error(errorText(e, 'createTask failed'));
    }
  }

  async function pollTask(taskId: string, onRaw?: OnRaw): Promise<Task> {
    const started = Date.now();
    let delay = 500;
    let lastErrorText = '';
    const method = 'InferenceService/GetTask';

    while (true) {
      try {
        const res = await client.getTask({ taskId });
        onRaw?.({ url: method, status: 'OK', body: toJson(GetTaskResponseSchema, res) });

        const task = res.task;
        if (task?.status === TaskStatus.STATUS_SUCCESS) return task;
        if (task?.status === TaskStatus.STATUS_FAILURE) {
          const failed = task.result?.chunks?.find((ch) => ch.errorMessage);
          throw new Error(failed?.errorMessage || 'task failed');
        }
        // STATUS_PENDING / STATUS_PROCESSING — просто ждём дальше
      } catch (e) {
        if (!(e instanceof ConnectError)) throw e;
        onRaw?.(errorLog(method, e));
        if (!TRANSIENT_CODES.has(e.code)) {
          // InvalidArgument / NotFound — опрашивать бессмысленно
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

  return { getModels, createTask, pollTask };
}
