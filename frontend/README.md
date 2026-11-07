# Фронтенд инференса

React + TypeScript + Vite. Один и тот же код работает в двух режимах.

## 1. Локальная разработка и тестирование

```bash
npm install
npm run dev          # http://localhost:5173
```

Открывается страница разработки (`index.html` → `src/dev.tsx`), которая
монтирует инференс той же функцией `mount`, что и портал. Запросы к API
Vite пересылает на настоящий бэкенд — по умолчанию
`https://inference.kuronami.fun`, другой адрес можно задать так:

```bash
INFERENCE_API=http://localhost:6767 npm run dev
```

## 2. Модуль для портала

```bash
npm run build        # проверка типов + сборка
```

Результат — один файл `dist/inference.js`: весь код, React и стили внутри.
Его раздаёт Caddy по адресу `https://inference.kuronami.fun/inference.js`,
а портал загружает его во время работы:

```ts
const { mount } = await import('https://inference.kuronami.fun/inference.js');
const unmount = mount(element, { header: false });
// когда пользователь уходит со страницы:
unmount();
```

Проверить модуль внутри портала на своём компьютере, без Docker:

```bash
npm run build
npm run preview      # раздаёт dist/inference.js на http://localhost:8080
                     # и пересылает API так же, как npm run dev
```

и в папке портала: `VITE_INFERENCE_URL=http://localhost:8080/inference.js npm run dev`.

Параметры `mount` (см. `src/index.tsx`):

| Параметр | По умолчанию | Что делает |
| --- | --- | --- |
| `apiBaseUrl` | сервер, откуда загружен `inference.js` | адрес API инференса |
| `header` | `true` | показывать собственную шапку «audio inference» |

Интерфейс рисуется внутри Shadow DOM элемента: стили инференса и страницы
не влияют друг на друга. Дизайн-токены портала (CSS-переменные `--accent`,
`--font-mono`, `--surface-1` и т.д.) инференс использует — они проходят
через границу Shadow DOM. Без них работают запасные значения из
`src/styles.css`, совпадающие с порталом.

## Развёртывание

```bash
docker compose up -d --build     # из папки frontend/
```

- `Dockerfile` — двухступенчатая сборка: `node` собирает `inference.js`,
  в итоговый образ `caddy` попадают только он и `Caddyfile`.
- `Caddyfile` — раздаёт `inference.js` (с CORS, чтобы портал на другом
  домене мог его загрузить) и проксирует API
  `/inference.v1.api.InferenceService/*` в основной стек инференса.
- `docker-compose.yaml` — один сервис `inference_frontend` в общей сети
  `computer-hearing`.

Переменные окружения контейнера:

| Переменная | По умолчанию | Что делает |
| --- | --- | --- |
| `INFERENCE_API_UPSTREAM` | `inference_caddy:80` | куда проксировать API |
| `CORS_ALLOW_ORIGIN` | `*` | какому сайту можно загружать модуль и ходить в API |

## Структура

```
src/
  index.tsx        # точка входа библиотеки: mount()
  dev.tsx          # страница разработки
  App.tsx          # главный экран инференса
  api.ts           # клиент API (connect-rpc)
  audio.ts         # декодирование, гистограмма, вырезка фрагмента в WAV
  chunks.ts        # подписи классов, softmax, границы чанков
  styles.css       # стили (внутри Shadow DOM)
  components/      # Waveform, ChunkDetails, Player
  gen/             # код из контракта: cd ../contracts && make generate-js
```
