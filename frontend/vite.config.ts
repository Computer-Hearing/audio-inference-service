// Две роли одного проекта:
//
//  npm run dev   — локальная страница для разработки и тестирования
//                  (index.html -> src/dev.tsx). Запросы к API пересылаются
//                  через прокси Vite на сервер из INFERENCE_API
//                  (по умолчанию https://inference.kuronami.fun).
//
//  npm run build — сборка "библиотеки": один файл dist/inference.js со всем
//                  кодом, React и стилями внутри. Его раздаёт Caddy
//                  (см. Dockerfile), а портал загружает по адресу
//                  https://inference.kuronami.fun/inference.js и вызывает mount().
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const API_TARGET = process.env.INFERENCE_API ?? 'https://inference.kuronami.fun';

export default defineConfig(({ command }) => ({
  plugins: [react()],

  // В режиме библиотеки Vite не подставляет process.env.NODE_ENV сам,
  // а React по нему выбирает production-сборку (без отладочных проверок).
  // В npm run dev оставляем как есть — там нужны предупреждения React.
  define:
    command === 'build' ? { 'process.env.NODE_ENV': JSON.stringify('production') } : {},

  server: {
    proxy: {
      '/inference.v1.api.InferenceService': {
        target: API_TARGET,
        changeOrigin: true,
        secure: true,
      },
    },
  },

  build: {
    lib: {
      entry: 'src/index.tsx',
      formats: ['es'],
      fileName: () => 'inference.js',
    },
    // В библиотечном режиме Vite сжимает ES-сборку не полностью (оставляет
    // пробелы и комментарии ради tree-shaking у тех, кто собирает библиотеку
    // дальше). inference.js никто дальше не собирает — его грузит браузер,
    // поэтому сжимаем полностью (см. output.minify ниже).
    // favicon и прочее из public/ нужно только странице разработки
    copyPublicDir: false,
    rolldownOptions: {
      output: {
        // Всё в один файл, без дополнительных чанков
        inlineDynamicImports: true,
        minify: true,
      },
    },
  },
}));
