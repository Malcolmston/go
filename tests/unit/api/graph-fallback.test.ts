// @vitest-environment node
//
// Unit tests for the browser search-index fallback in src/api/graph.ts: the
// columnar decoder (expandSymbolIndex) that mirrors browserSearchIndex() in
// scripts/build-graph-data.ts, and the size guard in loadFallbackSymbols().
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import { expandSymbolIndex, rankSymbols, SEARCH_INDEX_MAX_BYTES } from '../../../src/api/graph';

// The exact shape scripts/build-graph-data.ts writes to public/search-index.json.
const columnar = () => ({
  generatedAt: '2026-01-01T00:00:00.000Z',
  format: 'columns/1',
  anchorPrefix: 'sym-',
  libraries: ['express', 'lodash'],
  kinds: ['package', 'func'],
  packages: [
    ['github.com/example/express/router', 0],
    ['github.com/example/lodash/chunk', 1],
  ],
  symbols: [
    ['router', 0, 0, ''],
    ['NewRouter', 1, 0, 0],
    ['Chunk', 1, 1, 'sym-Chunk'],
  ],
});

describe('expandSymbolIndex', () => {
  it('expands columnar rows into full SymbolRecords', () => {
    const idx = expandSymbolIndex(columnar());
    expect(idx?.generatedAt).toBe('2026-01-01T00:00:00.000Z');
    expect(idx?.symbols).toEqual([
      {
        id: 'github.com/example/express/router',
        name: 'router',
        kind: 'package',
        packageImportPath: 'github.com/example/express/router',
        library: 'express',
        signature: '',
        doc: '',
        anchor: '',
      },
      {
        // anchor 0 is the "anchorPrefix + name" sentinel; id is rebuilt from it.
        id: 'github.com/example/express/router#sym-NewRouter',
        name: 'NewRouter',
        kind: 'func',
        packageImportPath: 'github.com/example/express/router',
        library: 'express',
        signature: '',
        doc: '',
        anchor: 'sym-NewRouter',
      },
      {
        // A literal anchor string is used verbatim.
        id: 'github.com/example/lodash/chunk#sym-Chunk',
        name: 'Chunk',
        kind: 'func',
        packageImportPath: 'github.com/example/lodash/chunk',
        library: 'lodash',
        signature: '',
        doc: '',
        anchor: 'sym-Chunk',
      },
    ]);
  });

  it('keeps rankSymbols working over the expanded records', () => {
    const idx = expandSymbolIndex(columnar());
    const hits = rankSymbols(idx?.symbols ?? [], 'router');
    expect(hits[0].name).toBe('router');
    expect(hits[0].kind).toBe('package');
    expect(hits.map((h) => h.name)).toContain('NewRouter');
  });

  it('passes through the legacy full-record shape unchanged', () => {
    const legacy = {
      generatedAt: 'x',
      symbols: [
        {
          id: 'p#F',
          name: 'F',
          kind: 'func',
          packageImportPath: 'p',
          library: 'l',
          signature: 'func F()',
          doc: 'doc',
          anchor: 'F',
        },
      ],
    };
    expect(expandSymbolIndex(legacy)).toEqual(legacy);
  });

  it('rejects junk and skips malformed rows', () => {
    expect(expandSymbolIndex(null)).toBeNull();
    expect(expandSymbolIndex({ generatedAt: 'x' })).toBeNull();
    const broken = columnar();
    // A row pointing at a package index that does not exist is dropped.
    broken.symbols.push(['Ghost', 1, 99, 0] as never);
    expect(expandSymbolIndex(broken)?.symbols).toHaveLength(3);
  });
});

describe('loadFallbackSymbols', () => {
  const realFetch = global.fetch;
  afterEach(() => {
    global.fetch = realFetch;
  });
  beforeEach(() => {
    vi.resetModules();
  });

  const respond = (body: unknown, contentLength: number) =>
    ({
      ok: true,
      status: 200,
      headers: { get: (h: string) => (h === 'content-length' ? String(contentLength) : null) },
      body: { cancel: vi.fn() },
      json: () => Promise.resolve(body),
    }) as unknown as Response;

  it('decodes the columnar index served over the wire', async () => {
    global.fetch = vi.fn(() => Promise.resolve(respond(columnar(), 512))) as unknown as typeof fetch;
    const mod = await import('../../../src/api/graph');
    const idx = await mod.loadFallbackSymbols();
    expect(idx?.symbols).toHaveLength(3);
    expect(idx?.symbols[1].id).toBe('github.com/example/express/router#sym-NewRouter');
  });

  it('refuses a payload larger than the size guard instead of parsing it', async () => {
    const json = vi.fn(() => Promise.resolve(columnar()));
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => String(SEARCH_INDEX_MAX_BYTES + 1) },
        body: { cancel: vi.fn() },
        json,
      } as unknown as Response),
    ) as unknown as typeof fetch;
    const mod = await import('../../../src/api/graph');
    expect(await mod.loadFallbackSymbols()).toBeNull();
    expect(json).not.toHaveBeenCalled();
  });
});
