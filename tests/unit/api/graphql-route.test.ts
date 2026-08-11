// @vitest-environment node
//
// Unit tests for the GraphQL Route Handler (app/api/graphql/route.ts) and its
// depth-limit validation rule (app/api/graphql/depthLimit.ts).
//
// graphstore / data / es / bm25 are mocked with a tiny two-package graph so the
// resolvers are deterministic and no data files are read. The focus is the
// route's *hardening*: bounded list sizes, bounded document/variable/body size,
// depth limiting over the cyclic Package type, correct 400-vs-500 split, and no
// internal error detail in responses.
import { describe, it, expect, vi, beforeEach } from 'vitest';

const {
  mockPkg,
  mockPackages,
  mockLibraries,
  mockImportsOf,
  mockImportedBy,
  mockRelated,
  mockSubgraph,
  mockGetSymbols,
  mockEsEnabled,
  mockEsSearch,
  mockBm25Search,
} = vi.hoisted(() => ({
  mockPkg: vi.fn(),
  mockPackages: vi.fn(),
  mockLibraries: vi.fn(),
  mockImportsOf: vi.fn(),
  mockImportedBy: vi.fn(),
  mockRelated: vi.fn(),
  mockSubgraph: vi.fn(),
  mockGetSymbols: vi.fn(),
  mockEsEnabled: vi.fn(),
  mockEsSearch: vi.fn(),
  mockBm25Search: vi.fn(),
}));

vi.mock('../../../api/_lib/graphstore', () => ({
  pkg: mockPkg,
  packages: mockPackages,
  libraries: mockLibraries,
  importsOf: mockImportsOf,
  importedBy: mockImportedBy,
  related: mockRelated,
  subgraph: mockSubgraph,
}));
vi.mock('../../../api/_lib/data', () => ({ getSymbols: mockGetSymbols }));
vi.mock('../../../api/_lib/es', () => ({ esEnabled: mockEsEnabled, esSearch: mockEsSearch }));
vi.mock('../../../api/_lib/bm25', () => ({ search: mockBm25Search }));

import { GET, POST, OPTIONS } from '../../../app/api/graphql/route';
import { depthLimitRule, MAX_QUERY_DEPTH } from '../../../app/api/graphql/depthLimit';

const PKG_A = {
  id: 'github.com/malcolmston/express/router',
  importPath: 'github.com/malcolmston/express/router',
  name: 'router',
  library: 'express',
  synopsis: 'HTTP routing',
  symbolCount: 12,
};
const PKG_B = { ...PKG_A, id: 'x/b', importPath: 'x/b', name: 'b', symbolCount: 3 };

const hit = () => ({
  id: 's1',
  name: 'Listen',
  kind: 'func',
  packageImportPath: 'x/b',
  library: 'express',
  signature: 'func Listen()',
  doc: 'Listens.',
  anchor: 'sym-Listen',
  score: 1.5,
});

function post(body: unknown, raw?: string): Promise<Response> {
  return POST(
    new Request('http://x/api/graphql', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: raw ?? JSON.stringify(body),
    }),
  );
}
function get(qs: string): Promise<Response> {
  return GET(new Request(`http://x/api/graphql?${qs}`));
}

beforeEach(() => {
  mockPkg.mockReset().mockReturnValue(PKG_A);
  mockPackages.mockReset().mockReturnValue([PKG_A, PKG_B]);
  mockLibraries.mockReset().mockReturnValue([
    { id: 'express', name: 'express', packageCount: 2, symbolCount: 15, parityAfter: '98%' },
  ]);
  // A cycle: A imports B, B is imported by A — enough to nest indefinitely.
  mockImportsOf.mockReset().mockReturnValue([PKG_B]);
  mockImportedBy.mockReset().mockReturnValue([PKG_A]);
  mockRelated.mockReset().mockReturnValue([{ to: PKG_B, kind: 'import', weight: 2 }]);
  mockSubgraph
    .mockReset()
    .mockReturnValue({ nodes: [{ id: 'x/b', label: 'b', library: 'express', symbolCount: 3 }], edges: [] });
  mockGetSymbols.mockReset().mockReturnValue([]);
  mockEsEnabled.mockReset().mockReturnValue(false);
  mockEsSearch.mockReset().mockResolvedValue([]);
  mockBm25Search.mockReset().mockReturnValue([hit()]);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

describe('OPTIONS', () => {
  it('returns 204 with CORS headers', async () => {
    const res = await OPTIONS();
    expect(res.status).toBe(204);
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
    expect(res.headers.get('Access-Control-Allow-Methods')).toContain('POST');
  });
});

describe('query execution', () => {
  it('resolves libraries', async () => {
    const body = await (await post({ query: '{ libraries { id name packageCount } }' })).json();
    expect(body.errors).toBeUndefined();
    expect(body.data.libraries[0]).toMatchObject({ id: 'express', packageCount: 2 });
  });

  it('resolves a package and its nested edges', async () => {
    const body = await (
      await post({
        query: '{ package(id: "x/b") { importPath imports { name } related { kind weight } } }',
      })
    ).json();
    expect(body.errors).toBeUndefined();
    expect(body.data.package.imports).toEqual([{ name: 'b' }]);
    expect(body.data.package.related).toEqual([{ kind: 'import', weight: 2 }]);
  });

  it('resolves search through BM25 when Upstash is off', async () => {
    const body = await (await post({ query: '{ search(q: "listen") { name score } }' })).json();
    expect(body.data.search).toEqual([{ name: 'Listen', score: 1.5 }]);
    expect(mockBm25Search).toHaveBeenCalledWith([], 'listen', 20);
  });

  it('falls back to BM25 when Upstash rejects', async () => {
    mockEsEnabled.mockReturnValue(true);
    mockEsSearch.mockRejectedValue(new Error('upstash down'));
    const body = await (await post({ query: '{ search(q: "listen") { name } }' })).json();
    expect(body.data.search).toEqual([{ name: 'Listen' }]);
    expect(mockBm25Search).toHaveBeenCalled();
  });

  it('supports variables and operationName', async () => {
    const body = await (
      await post({
        query: 'query A($id: ID!) { package(id: $id) { name } } query B { libraries { id } }',
        variables: { id: 'x/b' },
        operationName: 'A',
      })
    ).json();
    expect(body.errors).toBeUndefined();
    expect(body.data.package.name).toBe('router');
  });

  it('serves the same schema over GET', async () => {
    const body = await (await get('query=' + encodeURIComponent('{ libraries { id } }'))).json();
    expect(body.data.libraries[0].id).toBe('express');
  });

  it('parses GET variables from the JSON-encoded query param', async () => {
    const body = await (
      await get(
        'query=' +
          encodeURIComponent('query A($id: ID!) { package(id: $id) { name } }') +
          '&variables=' +
          encodeURIComponent('{"id":"x/b"}'),
      )
    ).json();
    expect(body.errors).toBeUndefined();
    expect(body.data.package.name).toBe('router');
  });
});

describe('list-size bounds', () => {
  it('clamps packages(first:) to 100', async () => {
    await post({ query: '{ packages(first: 2000000000) { id } }' });
    expect(mockPackages).toHaveBeenCalledWith(expect.objectContaining({ first: 100 }));
  });

  it('keeps the packages default of 50 when first is omitted', async () => {
    await post({ query: '{ packages { id } }' });
    expect(mockPackages).toHaveBeenCalledWith(expect.objectContaining({ first: 50 }));
  });

  it('clamps search(first:) to 100', async () => {
    await post({ query: '{ search(q: "listen", first: 999999) { name } }' });
    expect(mockBm25Search).toHaveBeenCalledWith([], 'listen', 100);
  });

  it('falls back to the search default for a non-positive first', async () => {
    await post({ query: '{ search(q: "listen", first: 0) { name } }' });
    expect(mockBm25Search).toHaveBeenCalledWith([], 'listen', 20);
  });

  it('short-circuits a blank search query without hitting a backend', async () => {
    const body = await (await post({ query: '{ search(q: "   ") { name } }' })).json();
    expect(body.data.search).toEqual([]);
    expect(mockBm25Search).not.toHaveBeenCalled();
  });
});

describe('depth limiting', () => {
  // Package.imports returns Package, so the schema is cyclic; each extra level
  // re-scans every graph edge. Build a query nested `levels` deep.
  const nested = (levels: number): string => {
    let inner = 'name';
    for (let i = 0; i < levels; i++) inner = `imports { ${inner} }`;
    return `{ package(id: "x/b") { ${inner} } }`;
  };

  it('accepts a query at the depth limit', async () => {
    // `{ package { imports*(n) { name } } }` == 2 + n field levels.
    const res = await post({ query: nested(MAX_QUERY_DEPTH - 2) });
    expect(res.status).toBe(200);
    expect((await res.json()).errors).toBeUndefined();
  });

  it('rejects a query past the depth limit with 400, before any resolver runs', async () => {
    const res = await post({ query: nested(MAX_QUERY_DEPTH + 5) });
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/too deep/);
    expect(mockPkg).not.toHaveBeenCalled();
  });

  it('cannot be sidestepped by hiding the nesting in a fragment', async () => {
    const query = `
      { package(id: "x/b") { ...Deep } }
      fragment Deep on Package { imports { importedBy { imports { importedBy { imports { importedBy { imports { importedBy { imports { name } } } } } } } } } }
    `;
    const res = await post({ query });
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/too deep/);
  });

  it('does not count an inline fragment as a level of its own', async () => {
    const shallow = '{ package(id: "x/b") { ... on Package { name } } }';
    const res = await post({ query: shallow });
    expect(res.status).toBe(200);
    expect((await res.json()).data.package.name).toBe('router');
  });

  it('depthLimitRule is configurable and reports the measured depth', async () => {
    expect(typeof depthLimitRule(1)).toBe('function');
    const res = await post({ query: nested(MAX_QUERY_DEPTH + 1) });
    expect((await res.json()).errors[0].message).toContain(String(MAX_QUERY_DEPTH));
  });
});

describe('input bounds and error handling', () => {
  it('rejects a missing query with 400', async () => {
    const res = await post({});
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/Missing GraphQL query/);
  });

  it('rejects an oversized query document with 400', async () => {
    const res = await post({ query: '{ libraries { ' + 'id '.repeat(4000) + '} }' });
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/too large/);
  });

  it('rejects a syntax error with 400 rather than 200/500', async () => {
    const res = await post({ query: '{ libraries { id ' });
    expect(res.status).toBe(400);
    expect((await res.json()).errors.length).toBeGreaterThan(0);
  });

  it('rejects an unknown field with 400 (validation, not execution)', async () => {
    const res = await post({ query: '{ nopeNotAField }' });
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/nopeNotAField/);
    expect(mockLibraries).not.toHaveBeenCalled();
  });

  it('rejects a malformed JSON body with 400', async () => {
    const res = await post(null, '{not json');
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/Invalid or oversized/);
  });

  it('rejects a non-object JSON body with 400', async () => {
    const res = await post(null, '[1,2,3]');
    expect(res.status).toBe(400);
  });

  it('rejects an oversized request body with 400', async () => {
    const res = await post(null, JSON.stringify({ query: '{ libraries { id } }', pad: 'x'.repeat(70_000) }));
    expect(res.status).toBe(400);
  });

  it('treats an empty body as a missing query (400, not a crash)', async () => {
    const res = await post(null, '');
    expect(res.status).toBe(400);
    expect((await res.json()).errors[0].message).toMatch(/Missing GraphQL query/);
  });

  it('ignores oversized or non-object variables instead of parsing them', async () => {
    const res = await get(
      'query=' + encodeURIComponent('{ libraries { id } }') + '&variables=' + encodeURIComponent('[1,2]'),
    );
    expect(res.status).toBe(200);
    expect((await res.json()).errors).toBeUndefined();
  });

  it('scrubs a thrown resolver error instead of echoing its message', async () => {
    mockLibraries.mockImplementation(() => {
      throw new Error('redis://user:pa55w0rd@internal-host:6379');
    });
    const res = await post({ query: '{ libraries { id } }' });
    const text = await res.text();
    const body = JSON.parse(text);
    expect(body.errors[0].message).toBe('Internal server error');
    expect(body.errors[0].path).toEqual(['libraries']);
    expect(text).not.toContain('pa55w0rd');
    expect(text).not.toContain('internal-host');
  });

  it('masks a schema/data fault (null in a non-nullable field) as well', async () => {
    mockPackages.mockReturnValue([{ ...PKG_A, name: null }]);
    const body = await (await post({ query: '{ packages { name } }' })).json();
    expect(body.errors[0].message).toBe('Internal server error');
  });

  it('keeps graphql-js-authored errors (which describe the caller document) intact', async () => {
    // Variable coercion failures carry no originalError — their message helps
    // the caller fix their own request and must survive scrubbing.
    const body = await (
      await post({ query: 'query A($id: ID!) { package(id: $id) { name } }', variables: {} })
    ).json();
    expect(body.errors[0].message).toMatch(/\$id.*required type/i);
  });

  it('always answers JSON with CORS headers', async () => {
    for (const res of [await post({ query: '{ libraries { id } }' }), await post({})]) {
      expect(res.headers.get('Content-Type')).toContain('application/json');
      expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
    }
  });
});
