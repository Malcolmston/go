// @vitest-environment node
//
// Unit tests for the parity summary Route Handler (app/api/parity/route.ts) and
// for summarizeParity(), the projection it shares with
// scripts/build-graph-data.ts (which writes the same shape to
// public/parity.json, the file src/components/Explore.tsx fetches).
import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockGetParitySummary } = vi.hoisted(() => ({ mockGetParitySummary: vi.fn() }));

vi.mock('../../../api/_lib/data', () => ({ getParitySummary: mockGetParitySummary }));

import { GET, OPTIONS } from '../../../app/api/parity/route';
import { summarizeParity } from '../../../api/_lib/data';

const entry = (over: Record<string, unknown> = {}) => ({
  parityPercent: 98.91,
  total: 184,
  match: 182,
  mismatch: 2,
  deviations: 0,
  upstream: { raw: 'express@4.21.2', name: 'express', version: '4.21.2' },
  ...over,
});

const summary = () => ({
  generatedAt: '2026-01-01T00:00:00.000Z',
  libraries: { express: entry(), lodash: entry({ parityPercent: 91.4 }) },
});

beforeEach(() => {
  mockGetParitySummary.mockReset().mockReturnValue(summary());
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

const get = (url: string) => GET(new Request(url));

describe('OPTIONS', () => {
  it('returns 204 with CORS headers', async () => {
    const res = await OPTIONS();
    expect(res.status).toBe(204);
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
  });
});

describe('GET /api/parity', () => {
  it('returns the whole summary keyed by library id', async () => {
    const res = await get('http://localhost/api/parity');
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.generatedAt).toBe('2026-01-01T00:00:00.000Z');
    expect(Object.keys(body.libraries)).toEqual(['express', 'lodash']);
    expect(body.libraries.express.parityPercent).toBe(98.91);
    expect(body.libraries.express.upstream.raw).toBe('express@4.21.2');
  });

  it('is small enough for the frontend size guard and carries no per-case detail', async () => {
    const text = await (await get('http://localhost/api/parity')).text();
    expect(text.length).toBeLessThan(524288);
    expect(text).not.toContain('"cases"');
    expect(text).not.toContain('"coverage"');
  });

  it('returns one library for ?lib=, case-insensitively', async () => {
    const body = await (await get('http://localhost/api/parity?lib=EXPRESS')).json();
    expect(body.library).toBe('express');
    expect(body.match).toBe(182);
    expect(body.total).toBe(184);
  });

  it('404s an unknown library without leaking the id list', async () => {
    const res = await get('http://localhost/api/parity?lib=nope');
    expect(res.status).toBe(404);
    expect(await res.json()).toEqual({ error: 'unknown library' });
  });

  it('does not treat inherited Object properties as libraries', async () => {
    const res = await get('http://localhost/api/parity?lib=constructor');
    expect(res.status).toBe(404);
  });

  it('reports 503 (not a crash) when the corpus cannot be read', async () => {
    mockGetParitySummary.mockImplementation(() => {
      throw new Error('ENOENT /var/task/api/_data/parity.json');
    });
    const res = await get('http://localhost/api/parity');
    expect(res.status).toBe(503);
    const body = await res.json();
    expect(body.error).toBe('parity data unavailable');
    expect(JSON.stringify(body)).not.toContain('ENOENT');
  });

  it('is CDN-cacheable but revalidating', async () => {
    const res = await get('http://localhost/api/parity');
    expect(res.headers.get('Cache-Control')).toContain('s-maxage=300');
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
  });
});

describe('summarizeParity', () => {
  // The module under test is mocked for the route tests above, so reach the real
  // implementation through a fresh, unmocked import.
  const load = async () => {
    const mod = await vi.importActual<typeof import('../../../api/_lib/data')>(
      '../../../api/_lib/data'
    );
    return mod.summarizeParity;
  };

  it('projects only the headline fields, dropping the per-case corpus', async () => {
    const fn = await load();
    const out = fn({
      generatedAt: 'now',
      libraries: {
        Express: {
          parityPercent: 98.91,
          total: 184,
          match: 182,
          mismatch: 2,
          deviations: 0,
          upstream: { raw: 'express@4.21.2', name: 'express', version: '4.21.2' },
          // extra corpus fields the summary must not carry through
          cases: [{ id: 'c1' }],
          coverage: [{ upstreamSymbol: 'app.use' }],
          nested: { qs: {} },
        },
      },
    } as never);
    expect(Object.keys(out.libraries)).toEqual(['express']); // lowercased key
    expect(out.libraries.express).toEqual({
      parityPercent: 98.91,
      total: 184,
      match: 182,
      mismatch: 2,
      deviations: 0,
      upstream: { raw: 'express@4.21.2', name: 'express', version: '4.21.2' },
    });
  });

  it('emits libraries in sorted order and coerces non-finite numbers to 0', async () => {
    const fn = await load();
    const mk = (over: Record<string, unknown>) =>
      ({
        parityPercent: 0,
        total: 0,
        match: 0,
        mismatch: 0,
        deviations: 0,
        upstream: { raw: '', name: '', version: '' },
        ...over,
      }) as never;
    const out = fn({
      generatedAt: 'now',
      libraries: {
        zod: mk({ parityPercent: 10 }),
        axios: mk({ parityPercent: Number.NaN, total: undefined }),
      },
    } as never);
    expect(Object.keys(out.libraries)).toEqual(['axios', 'zod']);
    expect(out.libraries.axios.parityPercent).toBe(0);
    expect(out.libraries.axios.total).toBe(0);
  });

  it('survives an empty or malformed corpus', async () => {
    const fn = await load();
    expect(fn({ generatedAt: '', libraries: {} })).toEqual({ generatedAt: '', libraries: {} });
    expect(fn(undefined as never)).toEqual({ generatedAt: '', libraries: {} });
  });
});
