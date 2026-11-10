// src/chunks.ts
//
// Общие сведения о чанках, которые нужны и рядам чанков под гистограммой
// (Waveform.tsx), и карточке с подробностями о чанке (ChunkDetails.tsx).
import type { Chunk } from './gen/inference/v1/inference_pb.ts';

export function argmaxIndex(arr: readonly number[] | undefined): number {
  if (!arr || !arr.length) return -1;
  let best = 0;
  for (let i = 1; i < arr.length; i++) if (arr[i] > arr[best]) best = i;
  return best;
}

// Бэкенд присылает в category/target "сырые" выходы нейросети (логиты —
// см. CategoryLogits/TargetLogits в predictor.go): в ONNX-модели после
// последнего линейного слоя нет Softmax. Чтобы показать проценты, переводим
// логиты в вероятности сами. Вычитание максимума — стандартный приём,
// чтобы exp() не переполнялся на больших значениях; на результат не влияет.
export function softmax(arr: readonly number[] | undefined): number[] {
  if (!arr || !arr.length) return [];
  const max = Math.max(...arr);
  const exps = arr.map((v) => Math.exp(v - max));
  const sum = exps.reduce((a, b) => a + b, 0);
  return exps.map((v) => v / sum);
}

// -----------------------------------------------------------------------
// Границы чанков по времени.
// Правило (согласовано с бэкендом, см. internal/modules/audio/splitter.go):
//  - базовый слой (offset=0): чанки фиксированного размера chunkSeconds,
//    последний чанк может быть короче (хвост длительностью duration % chunkSeconds,
//    если этот хвост длится дольше ~0.5s — иначе бэкенд его просто отбрасывает).
//  - слои со сдвигом (offset=1s и т.д.): те же чанки фиксированного размера,
//    считаются от отметки offset, хвост НЕ создаётся.
// Опираемся на chunkIndex и offset, которые уже возвращает бэкенд.
// -----------------------------------------------------------------------
export interface Range {
  start: number;
  end: number;
}

export function chunkRange(
  offset: number,
  chunkIndex: number,
  chunkSeconds: number,
  duration: number,
): Range {
  const start = offset + chunkIndex * chunkSeconds;
  const end = Math.min(duration, start + chunkSeconds);
  return { start, end: Math.max(start, end) };
}

// Уникальный ключ чанка: номер сам по себе повторяется в каждом слое.
export function chunkKey(ch: Pick<Chunk, 'offset' | 'chunkIndex'>): string {
  return `${ch.offset || 0}:${ch.chunkIndex}`;
}

export function layerLabel(offset: number): string {
  return offset ? `+${offset}s` : 'base';
}

// -----------------------------------------------------------------------
// Имя скачиваемого фрагмента.
// По умолчанию: <имя файла>_<слой>_chunk<номер>_<начало>-<конец>s,
// например street_base_chunk03_6.0-8.0s (".wav" добавляется при сохранении).
// -----------------------------------------------------------------------
export function chunkFileStem(
  sourceName: string | undefined,
  chunk: Pick<Chunk, 'offset' | 'chunkIndex'>,
  start: number,
  end: number,
): string {
  const base = (sourceName || 'audio').replace(/\.[^.]+$/, '').replace(/[^\w\-]+/g, '_');
  const layer = chunk.offset ? `offset${chunk.offset}s` : 'base';
  const index = String(chunk.chunkIndex).padStart(2, '0');
  return `${base}_${layer}_chunk${index}_${start.toFixed(1)}-${end.toFixed(1)}s`;
}

/**
 * Имя файла, которое ввёл пользователь, — в безопасный вид: без символов,
 * запрещённых в именах файлов Windows (\ / : * ? " < > |), без точек и
 * пробелов по краям, с расширением .wav. Пустое имя — имя по умолчанию.
 */
export function toWavFileName(input: string, fallbackStem: string): string {
  const stem = input
    .replace(/\.wav$/i, '')
    .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, '_')
    .trim()
    .replace(/^[.\s]+|[.\s]+$/g, '')
    .slice(0, 120);
  return `${stem || fallbackStem}.wav`;
}
