import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const BASE = '/inference/';
// connect-rpc: все методы сервиса идут на <BASE>/inference.v1.api.InferenceService/<Method>
const API_PATH = BASE.replace(/\/$/, '') + '/inference.v1.api.InferenceService';

export default defineConfig({
  base: BASE,
  plugins: [react()],
  server: {
    proxy: {
      [API_PATH]: {
        target: 'https://tools.kuronami.fun',
        changeOrigin: true,
        secure: true,
      },
    },
  },
});