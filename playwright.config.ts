import { defineConfig, devices } from '@playwright/test';

// Playwright ships 100+ device descriptors. The exhaustive sweep runs every one
// (forced onto chromium so the whole matrix runs where only chromium is
// installed). At ~61 tests per project that is well over 12,000 tests, which is
// not something anyone can type by accident: it is opt-in via E2E_FULL=1 and is
// reserved for the nightly schedule / manual dispatch.
const deviceNames = Object.keys(devices);

// Guard: the "100+ device profiles" guarantee must never silently regress.
if (deviceNames.length < 100) {
  throw new Error(`Expected >= 100 device descriptors, got ${deviceNames.length}`);
}

// The DEFAULT run is a small, representative slice — a couple of desktops,
// phones and tablets that span the responsive breakpoints (mobile menu /
// overflow strip / inline tabs). Preferred names are filtered to those the
// installed Playwright actually ships (device names drift across versions),
// then topped up from the full list so we always get a stable ~8-project subset
// even if a name was renamed.
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

export function subsetNames(all: string[] = deviceNames): string[] {
  const present = SUBSET_PREFERRED.filter((n) => all.includes(n));
  for (const n of all) {
    if (present.length >= SUBSET_SIZE) break;
    if (!present.includes(n)) present.push(n);
  }
  return present;
}

/**
 * Which device projects to run. Subset by default; the full 100+ device sweep
 * only when E2E_FULL=1 is set explicitly.
 */
export function selectedDeviceNames(
  env: Record<string, string | undefined> = process.env,
  all: string[] = deviceNames,
): string[] {
  return env.E2E_FULL === '1' ? all : subsetNames(all);
}

const BASE_URL = 'http://localhost:4173/';
const SERVE = 'pnpm exec next start -p 4173';

/**
 * The webServer command. Serving a build is the default: welding
 * `pnpm run build` into webServer peaked at ~2.4 GB RSS and aborted the whole
 * run ("Abort trap: 6") on a loaded machine, with no build output visible
 * because stdout was swallowed. Build separately (CI does it as its own step),
 * or set E2E_BUILD=1 for a one-shot cold run.
 */
export function webServerCommand(
  env: Record<string, string | undefined> = process.env,
): string {
  return env.E2E_BUILD === '1' ? `pnpm run build && ${SERVE}` : SERVE;
}

const projects = selectedDeviceNames().map((name) => ({
  name,
  use: {
    ...devices[name],
    browserName: 'chromium' as const,
    defaultBrowserType: 'chromium' as const,
  },
}));

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  // Each worker drives its own client-only Chromium context against the single
  // `next start` server. GitHub's runners have ~4 cores, so 24 workers (the old
  // value) oversubscribed the CPU ~6x — the client-side render starved and the
  // page/nav waits timed out. Cap at 4 everywhere: locally Playwright's default
  // (cores/2) oversubscribes a dev box that is also running a build.
  workers: 4,
  // Generous per-test timeout: the link/nav sweeps navigate every page.
  timeout: 60_000,
  forbidOnly: !!process.env.CI,
  // One retry everywhere. The suite has a small amount of render-timing flake
  // against a cold `next start`, and a local run that fails once on a first-hit
  // compile is indistinguishable from a real regression without it.
  retries: 1,
  reporter: process.env.CI ? [['github'], ['list']] : [['list']],
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
  },
  projects,
  webServer: {
    command: webServerCommand(),
    url: BASE_URL,
    // Always reuse a server already listening on 4173 — including on CI, where
    // sharded jobs otherwise each pay for their own startup.
    reuseExistingServer: true,
    timeout: 300_000,
    // Without these a server that fails to boot reports only a bare timeout.
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
