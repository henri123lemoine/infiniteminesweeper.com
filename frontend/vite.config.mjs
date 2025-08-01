import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  build: {
    outDir: '../backend/dist',
    emptyOutDir: true,
    manifest: false,
  },
  server: {
    proxy: {
      '/ws': { target: 'http://localhost:8080', ws: true },
    },
  },
  define: {
    __DEV__: mode === 'development',
  },
}));
