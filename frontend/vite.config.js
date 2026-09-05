import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const BASE = '/inference/';
const API_PATH = BASE.replace(/\/$/, '') + '/api';

export default defineConfig({
  base: BASE,
  plugins: [react()],
  server: {
    proxy: {
      [API_PATH]: {
        target: 'http://localhost:80',
        changeOrigin: true,
      },
    },
  },
});