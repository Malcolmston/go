// @vitest-environment node
//
// Unit tests for the search Route Handler (app/api/search/route.ts). The es,
// data, and analytics libs are mocked so the tests exercise the route's own
// logic (backend selection, fallback, response shape) deterministically. bm25
// is mocked to a fixed hit list so route assertions don't depend on ranking.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Shared spies, created via vi.hoisted so they exist before the (hoisted)
// vi.mock factories reference them.
const { mockEsEnabled, mockEsSearch, mockGetSymbols, mockBm25Search, mockLogSearch } = vi.hoisted(
  () => ({
    mockEsEnabled: vi.fn(),
    mockEsSearch: vi.fn(),
    mockGetSymbols: vi.fn(),
    mockBm25Search: vi.fn(),
    mockLogSearch: vi.fn(),
  }),
);

vi.mock('../../../api/_lib/es', () => ({
  esEnabled: mockEsEnabled,
  esSearch: mockEsSearch,
}));
vi.mock('../../../api/_lib/data', () => ({
  getSymbols: mockGetSymbols,
}));
vi.mock('../../../api/_lib/analytics', () => ({
  logSearch: mockLogSearch,
}));
vi.mock('../../../api/_lib/bm25', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../api/_lib/bm25')>();
  return {
    // Keep the real queryTokens so matchedTerms assertions are meaningful.
    queryTokens: actual.queryTokens,
    search: mockBm25Search,
  };
});

import { GET, OPTIONS } from '../../../app/api/search/route';
import {
  MAX_Q_LEN,
  parseKinds,
  parseLibrary,
  parseQuery,
  parseSearchParams,
  rerankEnabled,
} from '../../../app/api/search/params';

const bmHit = (over: Record<string, unknown> = {}) => ({
  id: 'x',
  name: 'Marshal',
  kind: 'func',
  packageImportPath: 'encoding/json',
  library: 'json',
  signature: null,
  doc: null,
  anchor: 'sym-Marshal',
  score: 1,
  ...over,
});

beforeEach(() => {
  mockEsEnabled.mockReset().mockReturnValue(false);
  mockEsSearch.mockReset().mockResolvedValue([]);
  mockGetSymbols.mockReset().mockReturnValue([]);
  mockBm25Search.mockReset().mockReturnValue([bmHit()]);
  mockLogSearch.mockReset();
});

afterEach(() => {
  vi.unstubAllEnvs();
});

function get(qs: string): Promise<Response> {
  return GET(new Request(`http://x/api/search?${qs}`));
}

describe('parseKinds', () => {
  it('returns [] for null / empty', () => {
    expect(parseKinds(null)).toEqual([]);
    expect(parseKinds('')).toEqual([]);
  });
  it('parses a comma list', () => {
    expect(parseKinds('func,type')).toEqual(['func', 'type']);
  });
  it('ignores invalid entries and lowercases', () => {
    expect(parseKinds('func,bogus,TYPE')).toEqual(['func', 'type']);
  });
  it('de-duplicates (case-insensitive)', () => {
    expect(parseKinds('func,func,FUNC')).toEqual(['func']);
  });
  it('trims whitespace around entries', () => {
    expect(parseKinds(' func , method ')).toEqual(['func', 'method']);
  });
  it('returns [] when all entries are invalid', () => {
    expect(parseKinds('bogus,nope')).toEqual([]);
  });
});

describe('parseQuery', () => {
  it('returns "" for null / non-strings', () => {
    expect(parseQuery(null)).toBe('');
  });
  it('trims surrounding whitespace', () => {
    expect(parseQuery('  marshal  ')).toBe('marshal');
  });
  it('caps the query at MAX_Q_LEN so BM25 term work stays bounded', () => {
    expect(parseQuery('a'.repeat(5_000))).toHaveLength(MAX_Q_LEN);
  });
});

describe('parseLibrary', () => {
  it('returns "" for null / empty / whitespace', () => {
    expect(parseLibrary(null)).toBe('');
    expect(parseLibrary('')).toBe('');
    expect(parseLibrary('   ')).toBe('');
  });
  it('accepts well-formed library slugs and lowercases them', () => {
    expect(parseLibrary('express')).toBe('express');
    expect(parseLibrary('socket.io')).toBe('socket.io');
    expect(parseLibrary('  Express  ')).toBe('express');
    expect(parseLibrary('next-mdx_2')).toBe('next-mdx_2');
  });
  it('drops values that are not library ids', () => {
    // Nothing caller-controlled may reach the Upstash metadata filter builder.
    for (const bad of [
      '../../etc/passwd',
      'express" OR "1"="1',
      'a b',
      '@metadata.kind',
      '{"$ne":null}',
      '-leading-dash',
      'x'.repeat(65),
    ]) {
      expect(parseLibrary(bad)).toBe('');
    }
  });
});

describe('parseSearchParams', () => {
  it('never throws — every field falls back to a bounded default', () => {
    const p = new URLSearchParams('q=%20hi%20&first=-3&library=NOT VALID&kind=bogus');
    expect(parseSearchParams(p)).toEqual({ q: 'hi', first: 20, library: '', kinds: [] });
  });
  it('parses a fully-specified request', () => {
    const p = new URLSearchParams('q=marshal&first=5&library=express&kind=func,TYPE');
    expect(parseSearchParams(p)).toEqual({
      q: 'marshal',
      first: 5,
      library: 'express',
      kinds: ['func', 'type'],
    });
  });
  it('defaults an entirely empty query string', () => {
    expect(parseSearchParams(new URLSearchParams())).toEqual({
      q: '',
      first: 20,
      library: '',
      kinds: [],
    });
  });
});

describe('rerankEnabled', () => {
  it('defaults to true when unset', () => {
    expect(rerankEnabled()).toBe(true);
  });
  it('is false for "0" and "false" (case-insensitive, trimmed)', () => {
    vi.stubEnv('SEARCH_RERANK', '0');
    expect(rerankEnabled()).toBe(false);
    vi.stubEnv('SEARCH_RERANK', 'FALSE');
    expect(rerankEnabled()).toBe(false);
    vi.stubEnv('SEARCH_RERANK', ' 0 ');
    expect(rerankEnabled()).toBe(false);
  });
  it('is true for other truthy-ish values', () => {
    vi.stubEnv('SEARCH_RERANK', '1');
    expect(rerankEnabled()).toBe(true);
    vi.stubEnv('SEARCH_RERANK', 'true');
    expect(rerankEnabled()).toBe(true);
    vi.stubEnv('SEARCH_RERANK', 'yes');
    expect(rerankEnabled()).toBe(true);
  });
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
  it('short-circuits an empty query without touching the backends', async () => {
    const res = await get('q=');
    const body = await res.json();
    expect(body).toMatchObject({ hits: [], backend: 'memory', total: 0, reranked: false, matchedTerms: [] });
    expect(mockEsSearch).not.toHaveBeenCalled();
    expect(mockBm25Search).not.toHaveBeenCalled();
  });

  it('uses Upstash when enabled, passing pushdown scope and reranking', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockResolvedValue([bmHit({ score: 2 })]);
    const res = await get('q=marshal&library=json&kind=func');
    const body = await res.json();
    expect(body.backend).toBe('upstash');
    expect(body.reranked).toBe(true);
    expect(mockEsSearch).toHaveBeenCalledWith('marshal', 20, {
      reranking: true,
      library: 'json',
      kinds: ['func'],
    });
  });

  it('passes library: undefined when the library param is empty', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockResolvedValue([]);
    await get('q=marshal');
    expect(mockEsSearch).toHaveBeenCalledWith('marshal', 20, {
      reranking: true,
      library: undefined,
      kinds: [],
    });
  });

  it('falls back to BM25 when Upstash rejects, over-fetching when a filter is active', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockRejectedValue(new Error('down'));
    mockGetSymbols.mockReturnValue([{ some: 'symbol' }]);
    mockBm25Search.mockReturnValue([bmHit({ library: 'json', kind: 'func' })]);
    const res = await get('q=marshal&first=5&library=json&kind=func');
    const body = await res.json();
    expect(body.backend).toBe('memory');
    expect(body.reranked).toBe(false);
    expect(mockGetSymbols).toHaveBeenCalled();
    // filter active -> over-fetch want = first * 8 = 40
    expect(mockBm25Search).toHaveBeenCalledWith([{ some: 'symbol' }], 'marshal', 40);
  });

  it('caps the BM25 over-fetch at MAX_FIRST (100)', async () => {
    mockEsEnabled.mockReturnValue(false);
    mockBm25Search.mockReturnValue([]);
    await get('q=marshal&first=50&kind=func');
    // 50 * 8 = 400 -> capped at 100
    expect(mockBm25Search).toHaveBeenCalledWith(expect.anything(), 'marshal', 100);
  });

  it('goes straight to BM25 when Upstash is not configured', async () => {
    mockEsEnabled.mockReturnValue(false);
    const res = await get('q=marshal');
    const body = await res.json();
    expect(body.backend).toBe('memory');
    expect(mockEsSearch).not.toHaveBeenCalled();
    // no filter active -> want === first (20)
    expect(mockBm25Search).toHaveBeenCalledWith(expect.anything(), 'marshal', 20);
  });

  it('reranked is only true when Upstash served AND rerank is on', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockResolvedValue([bmHit()]);
    vi.stubEnv('SEARCH_RERANK', '0');
    const body = await (await get('q=marshal')).json();
    expect(body.backend).toBe('upstash');
    expect(body.reranked).toBe(false);
  });

  it('reports matchedTerms via the shared tokenizer end-to-end', async () => {
    mockEsEnabled.mockReturnValue(false);
    const body = await (await get('q=ReadFile')).json();
    expect(body.matchedTerms).toEqual(['readfile', 'read', 'file']);
  });

  it('sets total === hits.length and a finite non-negative tookMs', async () => {
    mockEsEnabled.mockReturnValue(false);
    mockBm25Search.mockReturnValue([bmHit(), bmHit({ id: 'y' })]);
    const body = await (await get('q=marshal')).json();
    expect(body.total).toBe(body.hits.length);
    expect(body.total).toBe(2);
    expect(Number.isFinite(body.tookMs)).toBe(true);
    expect(body.tookMs).toBeGreaterThanOrEqual(0);
  });

  it('clamps first: 9999 -> 100, invalid/<=0 -> default 20', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockResolvedValue([]);
    await get('q=marshal&first=9999');
    expect(mockEsSearch).toHaveBeenCalledWith('marshal', 100, expect.anything());

    mockEsSearch.mockClear();
    await get('q=marshal&first=abc');
    expect(mockEsSearch).toHaveBeenCalledWith('marshal', 20, expect.anything());

    mockEsSearch.mockClear();
    await get('q=marshal&first=0');
    expect(mockEsSearch).toHaveBeenCalledWith('marshal', 20, expect.anything());
  });

  it('includes CORS and Cache-Control headers on the JSON response', async () => {
    const res = await get('q=marshal');
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
    expect(res.headers.get('Cache-Control')).toBeTruthy();
  });

  it('calls logSearch as best-effort analytics', async () => {
    mockEsEnabled.mockReturnValue(false);
    await get('q=marshal');
    expect(mockLogSearch).toHaveBeenCalledTimes(1);
  });

  it('bounds the query length before it reaches a backend', async () => {
    mockEsEnabled.mockReturnValue(false);
    await get(`q=${'a'.repeat(5_000)}`);
    expect(mockBm25Search).toHaveBeenCalledWith(expect.anything(), 'a'.repeat(MAX_Q_LEN), 20);
  });

  it('drops a malformed library instead of pushing it into the Upstash filter', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockResolvedValue([]);
    await get(`q=marshal&library=${encodeURIComponent('express" OR "1"="1')}`);
    expect(mockEsSearch).toHaveBeenCalledWith('marshal', 20, {
      reranking: true,
      library: undefined,
      kinds: [],
    });
  });

  it('treats a whitespace-only query as empty and short-circuits', async () => {
    const body = await (await get('q=%20%20')).json();
    expect(body.hits).toEqual([]);
    expect(mockBm25Search).not.toHaveBeenCalled();
  });

  it('returns 503 (not a 500 stack) when the corpus itself is unavailable', async () => {
    mockEsEnabled.mockReturnValue(false);
    mockGetSymbols.mockImplementation(() => {
      throw new Error('ENOENT /var/task/api/_data/symbols.json');
    });
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const res = await get('q=marshal');
    expect(res.status).toBe(503);
    const text = await res.text();
    expect(JSON.parse(text)).toMatchObject({ error: 'search_unavailable', hits: [], total: 0 });
    expect(text).not.toContain('ENOENT');
    expect(text).not.toContain('/var/task');
    expect(res.headers.get('Cache-Control')).toBe('no-store');
  });
});
