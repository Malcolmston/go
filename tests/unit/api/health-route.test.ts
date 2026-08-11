// @vitest-environment node
//
// Unit tests for the health Route Handler (app/api/health/route.ts). The es and
// data libs are mocked so the route's own logic — the stable {ok, es} contract,
// the additive corpus diagnostics, and its refusal to fail when the bundled data
// is unreadable — is exercised without touching the filesystem.
import { describe, it, expect, vi, beforeEach } from 'vitest';

const { mockEsEnabled, mockGetGraph, mockGetSymbols } = vi.hoisted(() => ({
  mockEsEnabled: vi.fn(),
  mockGetGraph: vi.fn(),
  mockGetSymbols: vi.fn(),
}));

vi.mock('../../../api/_lib/es', () => ({ esEnabled: mockEsEnabled }));
vi.mock('../../../api/_lib/data', () => ({
  getGraph: mockGetGraph,
  getSymbols: mockGetSymbols,
}));

import { GET, OPTIONS } from '../../../app/api/health/route';

const graph = (over: Record<string, unknown> = {}) => ({
  generatedAt: '2026-01-01T00:00:00.000Z',
  libraries: [{ id: 'express' }],
  packages: [{ id: 'a' }, { id: 'b' }],
  edges: [],
  ...over,
});

beforeEach(() => {
  mockEsEnabled.mockReset().mockReturnValue(false);
  mockGetGraph.mockReset().mockReturnValue(graph());
  mockGetSymbols.mockReset().mockReturnValue([{ id: 's1' }, { id: 's2' }, { id: 's3' }]);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

describe('OPTIONS', () => {
  it('returns 204 with CORS headers', async () => {
    const res = await OPTIONS();
    expect(res.status).toBe(204);
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
    expect(res.headers.get('Access-Control-Allow-Methods')).toContain('GET');
  });
});

describe('GET', () => {
  it('reports the stable {ok, es} contract the frontend probe depends on', async () => {
    mockEsEnabled.mockReturnValue(true);
    const body = await (await GET()).json();
    expect(body.ok).toBe(true);
    expect(body.es).toBe(true);
  });

  it('mirrors esEnabled() when Upstash is not configured', async () => {
    mockEsEnabled.mockReturnValue(false);
    const body = await (await GET()).json();
    expect(body.es).toBe(false);
  });

  it('reports corpus counters from the bundled data', async () => {
    const body = await (await GET()).json();
    expect(body.data).toEqual({
      ok: true,
      symbols: 3,
      packages: 2,
      libraries: 1,
      generatedAt: '2026-01-01T00:00:00.000Z',
    });
  });

  it('marks data as not ok when the corpus is empty (files did not ship)', async () => {
    mockGetSymbols.mockReturnValue([]);
    const body = await (await GET()).json();
    expect(body.ok).toBe(true); // the function itself is still up
    expect(body.data.ok).toBe(false);
    expect(body.data.symbols).toBe(0);
  });

  it('still answers 200 when reading the bundled data throws', async () => {
    mockGetSymbols.mockImplementation(() => {
      throw new Error('ENOENT /var/task/api/_data/symbols.json');
    });
    const res = await GET();
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.ok).toBe(true);
    expect(body.data.ok).toBe(false);
    // The internal error (which discloses the filesystem layout) is not echoed.
    expect(JSON.stringify(body)).not.toContain('ENOENT');
  });

  it('is never cached and carries CORS headers', async () => {
    const res = await GET();
    expect(res.headers.get('Cache-Control')).toBe('no-store');
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
  });
});
