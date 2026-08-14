import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:80', // caddy проксирует /api в backend:6767
        changeOrigin: true,
      },
    },
  },
});