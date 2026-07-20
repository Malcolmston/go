import { defineConfig, devices } from '@playwright/test';

// Playwright ships 100+ device descriptors. The full sweep runs every one
// (forced onto chromium so the whole matrix runs where only chromium is
// installed) — that is the exhaustive per-device responsive check, kept for
// `main` pushes and the nightly schedule.
const deviceNames = Object.keys(devices);

// Guard: the "100+ device profiles" guarantee must never silently regress.
if (deviceNames.length < 100) {
  throw new Error(`Expected >= 100 device descriptors, got ${deviceNames.length}`);
}

// On pull requests the full matrix is overkill and slow, so E2E_SUBSET=1 runs a
// small, representative slice — a couple of desktops, phones and tablets that
// span the responsive breakpoints (mobile menu / overflow strip / inline tabs).
// Preferred names are filtered to those the installed Playwright actually ships
// (device names drift across versions), then topped up from the full list so we
// always get a stable ~8-project subset even if a name was renamed.
const SUBSET_PREFERRED = [
  'Desktop Chrome',
  'Desktop Safari',
  'Desktop Edge',
  'iPhone 15',
  'iPhone SE',
  'Pixel 7',
  'iPad Pro 11',
  'Galaxy S9+',
];
const SUBSET_SIZE = 8;

function subsetNames(): string[] {
  const present = SUBSET_PREFERRED.filter((n) => deviceNames.includes(n));
  for (const n of deviceNames) {
    if (present.length >= SUBSET_SIZE) break;
    if (!present.includes(n)) present.push(n);
  }
  return present;
}

const selectedNames = process.env.E2E_SUBSET === '1' ? subsetNames() : deviceNames;

const projects = selectedNames.map((name) => ({
  name,
  use: {
    ...devices[name],
    browserName: 'chromium' as const,
    defaultBrowserType: 'chromium' as const,
  },
}));

const BASE_URL = 'http://localhost:4173/';

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  // Each worker drives its own client-only Chromium context against the single
  // `next start` server. GitHub's runners have ~4 cores, so 24 workers (the old
  // value) oversubscribed the CPU ~6x — the client-side render starved and the
  // page/nav waits timed out. Match workers to cores on CI for a stable render;
  // locally fall back to Playwright's default (cores/2).
  workers: process.env.CI ? 4 : undefined,
  // Generous per-test timeout: the link/nav sweeps navigate every page.
  timeout: 60_000,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['list']] : [['list']],
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
  },
  projects,
  webServer: {
    // Production build, then serve the Next app at the domain root on 4173.
    command: 'pnpm run build && pnpm exec next start -p 4173',
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
