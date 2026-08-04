import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5175,
    strictPort: true,
    proxy: {
      '/api': {
        target: process.env.VITE_SESSION_MANAGER_URL || 'http://127.0.0.1:8082',
        changeOrigin: true,
      },
      '/ws': {
        target: process.env.VITE_SESSION_MANAGER_WS_URL || 'ws://127.0.0.1:8082',
        ws: true,
        changeOrigin: true,
      },
    },
  },
});
