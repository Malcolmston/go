// @vitest-environment node
//
// Unit tests for the Upstash Search backend (api/_lib/es.ts). buildFilter and
// esEnabled are pure; esSearch is tested against a mocked @upstash/search SDK so
// no network / real credentials are involved.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// A single shared spy the mocked SDK's index.search() delegates to. Created via
// vi.hoisted so it exists before the (hoisted) vi.mock factory references it.
const { mockSearch } = vi.hoisted(() => ({ mockSearch: vi.fn() }));

vi.mock('@upstash/search', () => ({
  Search: {
    fromEnv: () => ({
      index: () => ({ search: mockSearch }),
    }),
  },
}));

import { buildFilter, esEnabled, esSearch } from '@/api/_lib/es';

beforeEach(() => {
  mockSearch.mockReset();
  mockSearch.mockResolvedValue([]);
});

afterEach(() => {
  vi.unstubAllEnvs();
});

describe('buildFilter', () => {
  it('returns undefined when nothing is scoped', () => {
    expect(buildFilter(undefined, undefined)).toBeUndefined();
    expect(buildFilter('', [])).toBeUndefined();
    // whitespace-only library is trimmed away
    expect(buildFilter('  ', [])).toBeUndefined();
  });

  it('emits a bare library clause when only library is set', () => {
    expect(buildFilter('express', undefined)).toEqual({
      '@metadata.library': { equals: 'express' },
    });
  });

  it('emits a bare kind clause when only kinds are set', () => {
    expect(buildFilter(undefined, ['func', 'type'])).toEqual({
      '@metadata.kind': { in: ['func', 'type'] },
    });
  });

  it('AND-combines both clauses (library first) when both are set', () => {
    expect(buildFilter('express', ['func'])).toEqual({
      AND: [
        { '@metadata.library': { equals: 'express' } },
        { '@metadata.kind': { in: ['func'] } },
      ],
    });
  });

  it('treats an empty kinds array as absent', () => {
    expect(buildFilter('express', [])).toEqual({
      '@metadata.library': { equals: 'express' },
    });
  });
});

describe('esEnabled', () => {
  it('is true only when both env vars are non-empty', () => {
    vi.stubEnv('UPSTASH_SEARCH_REST_URL', 'https://x');
    vi.stubEnv('UPSTASH_SEARCH_REST_TOKEN', 'tok');
    expect(esEnabled()).toBe(true);
  });

  it('is false when either is missing, empty, or whitespace', () => {
    vi.stubEnv('UPSTASH_SEARCH_REST_URL', 'https://x');
    vi.stubEnv('UPSTASH_SEARCH_REST_TOKEN', '');
    expect(esEnabled()).toBe(false);

    vi.stubEnv('UPSTASH_SEARCH_REST_URL', '');
    vi.stubEnv('UPSTASH_SEARCH_REST_TOKEN', 'tok');
    expect(esEnabled()).toBe(false);

    vi.stubEnv('UPSTASH_SEARCH_REST_URL', '   ');
    vi.stubEnv('UPSTASH_SEARCH_REST_TOKEN', '   ');
    expect(esEnabled()).toBe(false);
  });
});

describe('esSearch', () => {
  beforeEach(() => {
    vi.stubEnv('UPSTASH_SEARCH_REST_URL', 'https://x');
    vi.stubEnv('UPSTASH_SEARCH_REST_TOKEN', 'tok');
  });

  it('throws when Upstash is not configured', async () => {
    vi.stubEnv('UPSTASH_SEARCH_REST_URL', '');
    vi.stubEnv('UPSTASH_SEARCH_REST_TOKEN', '');
    await expect(esSearch('q')).rejects.toThrow('Upstash Search is not configured');
  });

  it('passes query, limit and reranking:true by default', async () => {
    await esSearch('hello', 20);
    expect(mockSearch).toHaveBeenCalledWith(
      expect.objectContaining({ query: 'hello', limit: 20, reranking: true }),
    );
  });

  it('disables reranking only when opts.reranking === false', async () => {
    await esSearch('hello', 20, { reranking: false });
    expect(mockSearch).toHaveBeenCalledWith(expect.objectContaining({ reranking: false }));

    mockSearch.mockClear();
    await esSearch('hello', 20, { reranking: undefined });
    expect(mockSearch).toHaveBeenCalledWith(expect.objectContaining({ reranking: true }));
  });

  it('sanitizes `first`: non-finite/<=0 -> 20, floats floored', async () => {
    await esSearch('q', 0);
    expect(mockSearch).toHaveBeenCalledWith(expect.objectContaining({ limit: 20 }));

    mockSearch.mockClear();
    await esSearch('q', Number.NaN);
    expect(mockSearch).toHaveBeenCalledWith(expect.objectContaining({ limit: 20 }));

    mockSearch.mockClear();
    await esSearch('q', 5.9);
    expect(mockSearch).toHaveBeenCalledWith(expect.objectContaining({ limit: 5 }));
  });

  it('includes a filter only when a scope is active', async () => {
    await esSearch('q', 20, {});
    expect(mockSearch.mock.calls[0][0]).not.toHaveProperty('filter');

    mockSearch.mockClear();
    await esSearch('q', 20, { library: 'express', kinds: ['func'] });
    expect(mockSearch.mock.calls[0][0]).toHaveProperty('filter', {
      AND: [
        { '@metadata.library': { equals: 'express' } },
        { '@metadata.kind': { in: ['func'] } },
      ],
    });
  });

  it('maps SDK results to SearchHit, defaulting score and null-ish meta', async () => {
    mockSearch.mockResolvedValueOnce([
      {
        score: 0.5,
        metadata: {
          id: '1',
          name: 'Marshal',
          kind: 'func',
          packageImportPath: 'encoding/json',
          library: 'json',
          signature: null,
          doc: null,
          anchor: null,
        },
      },
      // missing metadata + non-numeric score -> score 0, null-ish fields
      { score: 'nope' },
    ]);
    const hits = await esSearch('marshal');
    expect(hits[0]).toMatchObject({ name: 'Marshal', score: 0.5, signature: null, doc: null });
    expect(hits[1].score).toBe(0);
    expect(hits[1].signature).toBeNull();
    expect(hits[1].doc).toBeNull();
    expect(hits[1].anchor).toBeNull();
  });
});
