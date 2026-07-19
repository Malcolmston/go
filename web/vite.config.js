import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

// The go repo is served as a GitHub *project* page at
// https://malcolmston.github.io/go/, so assets must be based under /go/.
// On Vercel (which sets VERCEL=1 during the build) the app is served from the
// domain root, so base becomes '/'. Docs fetch uses import.meta.env.BASE_URL,
// so it follows whichever base is active automatically.
export default defineConfig({
  base: process.env.VERCEL ? '/' : '/go/',
  plugins: [react()],
  resolve: {
    alias: {
      // Import the vendored shared library from source.
      'go-ui': fileURLToPath(new URL('../ui/src/index.ts', import.meta.url)),
    },
  },
  build: { outDir: 'dist', emptyOutDir: true },
});
