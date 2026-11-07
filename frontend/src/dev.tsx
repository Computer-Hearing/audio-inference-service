// src/dev.tsx
//
// Страница для локальной разработки и тестирования (npm run dev).
// Использует ту же функцию mount, что и портал, — поэтому здесь проверяется
// ровно то, что будет встроено. Запросы к API идут на этот же dev-сервер,
// а Vite пересылает их на настоящий бэкенд (см. vite.config.ts).
import { mount } from './index.tsx';

const target = document.getElementById('inference');
if (!target) throw new Error('#inference element not found');

mount(target, { apiBaseUrl: window.location.origin });
