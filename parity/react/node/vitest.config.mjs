import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['parity.test.mjs'],
    // One file, one worker: the suite owns a single long-lived Go subprocess and
    // rewrites ../parity.json at the end, so nothing here may run in parallel
    // with another copy of itself.
    fileParallelism: false,
    // `go build` of the runner happens in beforeAll and pays for a cold module
    // cache on a fresh checkout.
    hookTimeout: 240_000,
    testTimeout: 20_000,
    reporters: ['default'],
  },
});
