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

// -----------------------------------------------------------------------
// Скачивание фрагмента (чанка) файла.
//
// Для гистограммы файл декодируется "облегчённо" (моно, 22050 Гц) — для
// скачивания так нельзя, пользователь должен получить звук в нормальном
// качестве. Поэтому для скачивания файл декодируется отдельно: все каналы,
// 44100 Гц (частота, на которой работает и сама модель в Triton).
// Нужный отрезок вырезается и сохраняется в WAV — формат без сжатия,
// который открывается любым плеером и который браузер умеет собрать сам,
// без сторонних библиотек.
// -----------------------------------------------------------------------
const EXPORT_SAMPLE_RATE = 44100;

/**
 * Декодирует файл в полном качестве (каналы сохраняются).
 * @returns {Promise<AudioBuffer>}
 */
export async function decodeAudioFull(file) {
  const AudioCtx = window.OfflineAudioContext || window.webkitOfflineAudioContext;
  const ctx = new AudioCtx(1, 1, EXPORT_SAMPLE_RATE);
  return ctx.decodeAudioData(await file.arrayBuffer());
}

/**
 * Вырезает из AudioBuffer отрезок [start, end) секунд и упаковывает его
 * в WAV (16 бит PCM, все каналы).
 * @returns {Blob}
 */
export function encodeWavSegment(buffer, start, end) {
  const sampleRate = buffer.sampleRate;
  const channels = buffer.numberOfChannels;
  const from = Math.max(0, Math.min(buffer.length, Math.round(start * sampleRate)));
  const to = Math.max(from, Math.min(buffer.length, Math.round(end * sampleRate)));
  const frames = to - from;

  const bytesPerSample = 2;
  const dataSize = frames * channels * bytesPerSample;
  const view = new DataView(new ArrayBuffer(44 + dataSize));

  const writeString = (offset, s) => {
    for (let i = 0; i < s.length; i++) view.setUint8(offset + i, s.charCodeAt(i));
  };

  // Заголовок WAV (RIFF): 44 байта с описанием формата
  writeString(0, 'RIFF');
  view.setUint32(4, 36 + dataSize, true);
  writeString(8, 'WAVE');
  writeString(12, 'fmt ');
  view.setUint32(16, 16, true); // размер блока fmt
  view.setUint16(20, 1, true); // 1 = PCM, без сжатия
  view.setUint16(22, channels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * channels * bytesPerSample, true); // байт в секунду
  view.setUint16(32, channels * bytesPerSample, true); // байт на один отсчёт всех каналов
  view.setUint16(34, bytesPerSample * 8, true); // бит на отсчёт
  writeString(36, 'data');
  view.setUint32(40, dataSize, true);

  // Сами отсчёты: каналы чередуются (Л, П, Л, П, ...), дробное число
  // от -1 до 1 переводится в целое 16-битное от -32768 до 32767
  const data = [];
  for (let c = 0; c < channels; c++) data.push(buffer.getChannelData(c));
  let offset = 44;
  for (let i = from; i < to; i++) {
    for (let c = 0; c < channels; c++) {
      const s = Math.max(-1, Math.min(1, data[c][i]));
      view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
      offset += bytesPerSample;
    }
  }

  return new Blob([view], { type: 'audio/wav' });
}

/**
 * Отдаёт Blob пользователю как скачиваемый файл.
 */
export function saveBlob(blob, fileName) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // Ссылку освобождаем не сразу — некоторые браузеры начинают скачивание асинхронно
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
