// @vitest-environment node
//
// Unit tests for the dependency-free BM25 ranking used by the search backend
// (api/_lib/bm25.ts). bm25.ts imports only `type SymbolDoc` (erased at compile
// time), so it can be imported directly without triggering data.ts fs reads.
import { describe, it, expect } from 'vitest';
import { queryTokens, search } from '../../../api/_lib/bm25';
import type { SymbolDoc } from '../../../api/_lib/data';

// Minimal SymbolDoc factory — only the fields BM25 reads matter.
function sym(over: Partial<SymbolDoc>): SymbolDoc {
  return {
    id: over.id ?? over.name ?? 'id',
    name: over.name ?? '',
    kind: over.kind ?? 'func',
    packageImportPath: over.packageImportPath ?? 'pkg',
    library: over.library ?? 'lib',
    signature: over.signature ?? null,
    doc: over.doc ?? null,
    anchor: over.anchor ?? null,
  } as SymbolDoc;
}

describe('queryTokens', () => {
  it('returns [] for empty / whitespace / nullish queries', () => {
    expect(queryTokens('')).toEqual([]);
    expect(queryTokens('   ')).toEqual([]);
    expect(queryTokens(null as unknown as string)).toEqual([]);
    expect(queryTokens(undefined as unknown as string)).toEqual([]);
  });

  it('lowercases and emits camelCase subtokens alongside the whole chunk', () => {
    const t = queryTokens('ReadFile');
    expect(t).toContain('readfile');
    expect(t).toContain('read');
    expect(t).toContain('file');
  });

  it('splits on digit boundaries', () => {
    expect(queryTokens('http2')).toEqual(['http2', 'http', '2']);
  });

  it('splits on non-alphanumeric chars with no empty strings', () => {
    const t = queryTokens('LRU.Get');
    expect(t).toContain('lru');
    expect(t).toContain('get');
    expect(t).not.toContain('');
  });

  it('de-duplicates repeated tokens (case-insensitive)', () => {
    expect(queryTokens('file File FILE')).toEqual(['file']);
  });

  it('is stable / order-deterministic for a representative input', () => {
    // whole chunk first, then its camelCase subtokens, in order.
    expect(queryTokens('parseURL')).toEqual(['parseurl', 'parse', 'url']);
  });
});

describe('search', () => {
  it('returns [] for an empty corpus', () => {
    expect(search([], 'x')).toEqual([]);
  });

  it('returns [] for a non-array symbols argument', () => {
    expect(search(null as unknown as SymbolDoc[], 'x')).toEqual([]);
  });

  it('returns [] for empty / whitespace / nullish queries', () => {
    const syms = [sym({ name: 'Marshal' })];
    expect(search(syms, '')).toEqual([]);
    expect(search(syms, '   ')).toEqual([]);
    expect(search(syms, null as unknown as string)).toEqual([]);
  });

  it('returns [] when the query tokenizes to nothing', () => {
    const syms = [sym({ name: 'Marshal' })];
    expect(search(syms, '   ---  ')).toEqual([]);
  });

  it('ranks an exact name match above a doc-only match (name^3 boost)', () => {
    const syms = [
      sym({ id: 'a', name: 'Marshal' }),
      sym({ id: 'b', name: 'Other', doc: 'this function can marshal things' }),
    ];
    const res = search(syms, 'marshal');
    expect(res.length).toBe(2);
    expect(res[0].name).toBe('Marshal');
  });

  it('matches on prefix (needs query len >= 2)', () => {
    const syms = [sym({ name: 'Marshal' }), sym({ name: 'Zebra' })];
    const res = search(syms, 'mar');
    expect(res.map((r) => r.name)).toContain('Marshal');
    expect(res.map((r) => r.name)).not.toContain('Zebra');
  });

  it('matches within a single edit for queries of length >= 3 (fuzzy)', () => {
    const syms = [sym({ name: 'Marshal' })];
    expect(search(syms, 'marshl').map((r) => r.name)).toContain('Marshal');
  });

  it('does not fuzzy-match a length-2 query', () => {
    // "xy" is 2 chars: no fuzzy pass, no prefix of "Marshal", no exact. Empty.
    const syms = [sym({ name: 'Marshal' })];
    expect(search(syms, 'xy')).toEqual([]);
  });

  it('honors the `first` limit and falls back to default 20 for invalid values', () => {
    const syms = Array.from({ length: 30 }, (_, i) => sym({ id: `s${i}`, name: `Marshal${i}` }));
    expect(search(syms, 'marshal', 3).length).toBe(3);
    // invalid first -> default 20
    expect(search(syms, 'marshal', 0).length).toBe(20);
    expect(search(syms, 'marshal', -5).length).toBe(20);
    expect(search(syms, 'marshal', NaN).length).toBe(20);
  });

  it('breaks score ties deterministically by name.localeCompare', () => {
    const syms = [
      sym({ id: 'b', name: 'Beta' }),
      sym({ id: 'a', name: 'Alpha' }),
    ];
    // both identical structure; querying a shared subtoken gives equal scores.
    const res = search(syms, 'al');
    // "Alpha" starts with "al" (prefix), "Beta" does not — so Alpha wins here.
    expect(res[0].name).toBe('Alpha');
  });

  it('matches a camelCase symbol via a lowercase joined query', () => {
    const syms = [sym({ name: 'ReadFile' })];
    expect(search(syms, 'readfile').map((r) => r.name)).toContain('ReadFile');
  });

  it('attaches a numeric score rounded to 6 decimal places', () => {
    const syms = [sym({ name: 'Marshal' })];
    const res = search(syms, 'marshal');
    expect(typeof res[0].score).toBe('number');
    // rounded to 1e6: no more than 6 decimals
    expect(res[0].score).toBe(Math.round(res[0].score * 1e6) / 1e6);
  });
});
