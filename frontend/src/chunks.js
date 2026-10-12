// src/chunks.js
//
// Общие сведения о чанках, которые нужны и рядам чанков под гистограммой
// (Waveform.jsx), и карточке с подробностями о чанке (ChunkDetails.jsx).

// Названия классов модели — по порядку выходов category_output / target_output.
export const CATEGORY_LABELS = ['car', 'emv', 'motorcycle', 'tram', 'truck'];
export const TARGET_LABELS = [
  'acceleration', 'bell', 'braking', 'horn', 'idling', 'passing', 'siren',
];

export function argmaxIndex(arr) {
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
export function softmax(arr) {
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
export function chunkRange(offset, chunkIndex, chunkSeconds, duration) {
  const start = offset + chunkIndex * chunkSeconds;
  const end = Math.min(duration, start + chunkSeconds);
  return { start, end: Math.max(start, end) };
}

// Уникальный ключ чанка: номер сам по себе повторяется в каждом слое.
export function chunkKey(ch) {
  return `${ch.offset || 0}:${ch.chunkIndex}`;
}

export function layerLabel(offset) {
  return offset ? `+${offset}s` : 'base';
}
