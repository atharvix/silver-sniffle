import path from 'path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Standalone Vitest config (does not reuse vite.config.ts, which requires
// PORT/BASE_PATH env vars that are only present when the dev server runs).
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, 'src'),
    },
    dedupe: ['react', 'react-dom'],
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: [path.resolve(import.meta.dirname, 'src/test/setup.ts')],
    css: false,
  },
});
