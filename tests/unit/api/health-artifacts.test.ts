// @vitest-environment node
//
// Integration-flavoured tests for the health Route Handler: unlike
// health-route.test.ts (which mocks the data libs), these run the route against
// the REAL generated artifacts in api/_data/ — the files prebuild writes — and
// then against a filesystem where one artifact is missing. That is the case the
// endpoint exists to catch: "functions are up but the data files did not ship".
//
// api/_data/*.json is gitignored, so the healthy case is skipped when the
// artifacts have not been generated yet (run `pnpm build:graph` and
// `pnpm build:security` first). The missing-artifact case needs no artifacts.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

const dataDir = fileURLToPath(new URL('../../../api/_data/', import.meta.url));
const has = (name: string) => fs.existsSync(dataDir + name);
const artifactsPresent =
  has('graph.json') && has('symbols.json') && has('parity.json') && has('examples.json') && has('security.json');

beforeEach(() => {
  vi.resetModules();
});

afterEach(() => {
  vi.doUnmock('node:fs');
  vi.resetModules();
});

describe('GET /api/health against the real artifacts', () => {
  it.skipIf(!artifactsPresent)('reports every corpus and is green when all five shipped', async () => {
    const { GET } = await import('../../../app/api/health/route');
    const body = await (await GET()).json();
    expect(body.ok).toBe(true);
    expect(body.data.ok).toBe(true);
    expect(body.data.symbols).toBeGreaterThan(0);
    expect(body.data.packages).toBeGreaterThan(0);
    expect(body.data.parityLibraries).toBeGreaterThan(0);
    expect(body.data.examples).toBeGreaterThan(0);
    expect(body.data.securityGeneratedAt).not.toBe('');
  });

  it.skipIf(!artifactsPresent)('goes red when only parity.json is unreadable', async () => {
    // The loaders swallow read errors, so a missing artifact is indistinguishable
    // from an empty one at the lib level; the route has to notice the zero.
    vi.doMock('node:fs', async () => {
      const actual = await vi.importActual<typeof import('node:fs')>('node:fs');
      const readFileSync: typeof actual.readFileSync = ((p: never, ...rest: never[]) => {
        if (String(p).endsWith('parity.json')) {
          const err = new Error('ENOENT: no such file or directory') as Error & { code?: string };
          err.code = 'ENOENT';
          throw err;
        }
        return (actual.readFileSync as (...a: never[]) => unknown)(p, ...rest);
      }) as typeof actual.readFileSync;
      return { ...actual, default: { ...actual, readFileSync }, readFileSync };
    });
    const { GET } = await import('../../../app/api/health/route');
    const res = await GET();
    expect(res.status).toBe(200);
    const body = await res.json();
    // The function itself is still up — that contract must not change.
    expect(body.ok).toBe(true);
    expect(body.data.ok).toBe(false);
    expect(body.data.parityLibraries).toBe(0);
    // The artifacts that did ship are still reported.
    expect(body.data.symbols).toBeGreaterThan(0);
    expect(body.data.examples).toBeGreaterThan(0);
  });
});
