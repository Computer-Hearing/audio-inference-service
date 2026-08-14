import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://192.168.3.160:6767', // адрес бэкенда в сети
        changeOrigin: true,
      },
    },
  },
});