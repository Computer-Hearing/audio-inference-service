// src/index.tsx
//
// Точка входа библиотеки. `npm run build` собирает из этого файла один
// dist/inference.js (код, React и стили внутри). Caddy раздаёт его по адресу
// https://inference.kuronami.fun/inference.js, а портал подключает так:
//
//   const { mount } = await import('https://inference.kuronami.fun/inference.js');
//   const unmount = mount(document.getElementById('inference')!, { header: false });
//   // ...когда пользователь уходит со страницы:
//   unmount();
//
// Интерфейс рисуется внутри Shadow DOM ("теневого" DOM) элемента-хозяина:
// стили инференса не протекают в портал, а глобальные стили портала — в
// инференс. CSS-переменные (дизайн-токены портала: --accent, --font-mono
// и т.д.) через эту границу наследуются — поэтому внутри портала инференс
// выглядит в его оформлении, а на странице разработки работают запасные
// значения из styles.css.
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.tsx';
import { createInferenceApi } from './api.ts';
import styles from './styles.css?inline';

export interface MountOptions {
  /**
   * Адрес API инференса — к нему дописывается
   * /inference.v1.api.InferenceService/<Метод>.
   * По умолчанию — тот же сервер, откуда загружен inference.js
   * (https://inference.kuronami.fun).
   */
  apiBaseUrl?: string;
  /** Показывать собственную шапку "audio inference". По умолчанию да. */
  header?: boolean;
}

export type Unmount = () => void;

// Сервер, с которого загружен этот файл. import.meta.url в собранном
// inference.js — его собственный адрес, например
// https://inference.kuronami.fun/inference.js -> https://inference.kuronami.fun
// (Отрезаем имя файла строкой, а не через new URL('.', import.meta.url):
// такую запись Vite при сборке принимает за ссылку на файл-ресурс и
// подменяет на встроенные данные.)
function defaultApiBaseUrl(): string {
  return import.meta.url.replace(/\/[^/]*$/, '');
}

/**
 * Показать инференс внутри элемента `target`.
 * Возвращает функцию, которая убирает его и освобождает ресурсы.
 */
export function mount(target: HTMLElement, options: MountOptions = {}): Unmount {
  const shadow = target.shadowRoot ?? target.attachShadow({ mode: 'open' });
  shadow.replaceChildren();

  const style = document.createElement('style');
  style.textContent = styles;
  const container = document.createElement('div');
  shadow.append(style, container);

  const api = createInferenceApi(options.apiBaseUrl ?? defaultApiBaseUrl());
  const root = createRoot(container);
  root.render(
    <StrictMode>
      <App api={api} header={options.header ?? true} />
    </StrictMode>,
  );

  return () => {
    root.unmount();
    shadow.replaceChildren();
  };
}
