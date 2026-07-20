import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

// Vitest configuration for the component unit tests (Next 15 + React 19).
//
// The @vitejs/plugin-react plugin drives the automatic JSX runtime, so the same
// JSX the app uses transpiles here — no per-file React import needed. Tests run
// in jsdom with globals + a setup file that wires jest-dom matchers.
//
// This config is independent of next.config.mjs; Next never reads it. The app is
// now an App-Router SPA, so the view components import `next/link` and
// `next/navigation`. Those APIs need the App Router context, which does not
// exist under jsdom (the real ones throw "invariant expected app router to be
// mounted"), so both are aliased to lightweight test stubs. `go-ui` resolves to
// its TypeScript source (mirroring the app's webpack alias), and react/react-dom
// are de-duped to the single copy in web/node_modules (go-ui declares React only
// as a peer dependency) to rule out any "Invalid hook call" from a duplicated
// renderer.
export default defineConfig({
  plugins: [react()],
  resolve: {
    dedupe: ['react', 'react-dom'],
    alias: {
      'go-ui': fileURLToPath(new URL('./ui/src/index.ts', import.meta.url)),
      'next/link': fileURLToPath(new URL('./tests/mocks/next-link.tsx', import.meta.url)),
      'next/navigation': fileURLToPath(new URL('./tests/mocks/next-navigation.ts', import.meta.url)),
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./tests/setup.ts'],
    include: ['tests/unit/**/*.test.{ts,tsx}'],
    css: false,
  },
});
