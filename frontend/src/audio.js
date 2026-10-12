// src/audio.js
//
// Гистограмма громкости теперь строится на фронтенде: CreateTask больше не
// возвращает waves. Файл декодируется прямо в браузере (Web Audio API),
// и по сэмплам считается RMS-амплитуда для каждого столбца.
//
// Столбцы привязаны к secondsPerChunk модели, чтобы границы столбцов и
// чанков "стыковались" — раньше бэкенд отдавал фиксированные 40 бакетов на
// весь файл, и чанк мог заканчиваться посреди столбца.

// Частота, до которой браузер передискретизирует файл при декодировании.
// Для огибающей громкости высокая частота не нужна, а памяти так уходит
// в разы меньше (4-минутный mp3 — ~5 млн сэмплов вместо ~11 млн).
// 22050 Гц — минимум, который гарантированно поддерживают все браузеры.
const DECODE_SAMPLE_RATE = 22050;

// Сколько столбцов стараемся уложить на весь файл.
const TARGET_BARS = 160;
// Не дробим столбец мельче 1/10 секунды — на коротких файлах иначе
// получается "гребёнка" без смысла.
const MAX_BARS_PER_SECOND = 10;

/**
 * Декодирует аудиофайл и сводит все каналы в моно.
 * @returns {Promise<{ samples: Float32Array, sampleRate: number, duration: number }>}
 */
export async function decodeAudio(file) {
  const AudioCtx = window.OfflineAudioContext || window.webkitOfflineAudioContext;
  const ctx = new AudioCtx(1, 1, DECODE_SAMPLE_RATE);
  const buffer = await ctx.decodeAudioData(await file.arrayBuffer());

  const channels = buffer.numberOfChannels;
  const samples = new Float32Array(buffer.length);
  for (let c = 0; c < channels; c++) {
    const data = buffer.getChannelData(c);
    for (let i = 0; i < data.length; i++) samples[i] += data[i] / channels;
  }

  return { samples, sampleRate: buffer.sampleRate, duration: buffer.duration };
}

/**
 * Длительность одного столбца в секундах.
 *
 * Бэкенд режет файл на два слоя чанков: базовый (с 0s) и со сдвигом +1s,
 * длина чанка — secondsPerChunk (целое число секунд). Чтобы границы
 * столбцов совпадали с границами чанков ОБОИХ слоёв, столбец должен делить
 * и secondsPerChunk, и 1 секунду, т.е. быть равен secondsPerChunk / N, где
 * N = secondsPerChunk * m — ровно m столбцов на секунду, N на чанк.
 *
 * На очень длинных файлах (больше TARGET_BARS секунд) столбец на секунду
 * дал бы слишком частую гребёнку — тогда столбец охватывает целое число
 * чанков: стыковка остаётся с базовым слоем, слой +1s на таком масштабе
 * всё равно не читается.
 */
export function barSecondsFor(duration, chunkSeconds) {
  if (!(duration > 0) || !(chunkSeconds > 0)) return chunkSeconds || 1;

  if (duration <= TARGET_BARS) {
    const perSecond = Math.min(MAX_BARS_PER_SECOND, Math.max(1, Math.floor(TARGET_BARS / duration)));
    const barsPerChunk = chunkSeconds * perSecond;
    return chunkSeconds / barsPerChunk;
  }

  const chunksPerBar = Math.ceil(duration / chunkSeconds / TARGET_BARS);
  return chunkSeconds * chunksPerBar;
}

/**
 * Разбивает файл на столбцы и считает RMS-амплитуду каждого.
 * Последний столбец может быть короче — как и последний чанк.
 * @returns {{ start: number, end: number, value: number }[]}
 */
export function computeBars({ samples, sampleRate, duration }, chunkSeconds) {
  if (!samples || !samples.length || !(duration > 0)) return [];

  const barSeconds = barSecondsFor(duration, chunkSeconds);
  const count = Math.ceil(duration / barSeconds - 1e-9);
  const bars = [];

  for (let i = 0; i < count; i++) {
    const start = i * barSeconds;
    const end = Math.min(duration, start + barSeconds);
    const from = Math.min(samples.length, Math.round(start * sampleRate));
    const to = Math.min(samples.length, Math.round(end * sampleRate));

    let sum = 0;
    for (let j = from; j < to; j++) sum += samples[j] * samples[j];
    const value = to > from ? Math.sqrt(sum / (to - from)) : 0;

    bars.push({ start, end, value });
  }

  return bars;
}
