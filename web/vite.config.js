import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8420', changeOrigin: true },
      // SSE must not be buffered by the proxy.
      '/events': { target: 'http://127.0.0.1:8420', changeOrigin: true, ws: false },
    },
  },
});
