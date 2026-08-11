import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { isParityCorpus } from '../../app/parity/load';

// app/parity/load.ts is the only door between the generated parity corpus
// (api/_data/parity.json) and every /parity page. public/parity.json is a
// different, much smaller schema — the browser summary, percentages only — and
// if the loader ever accepts it the site prints headline "measured" parity with
// empty case tables behind it. isParityCorpus is the guard; these tests pin the
// discrimination against both real files on disk plus the minimal shapes.

const SUMMARY = path.join(process.cwd(), 'public', 'parity.json');
const CORPUS = path.join(process.cwd(), 'api', '_data', 'parity.json');

function readJson(file: string): unknown | undefined {
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8')) as unknown;
  } catch {
    return undefined;
  }
}

describe('isParityCorpus', () => {
  it('rejects a missing or unparsable file', () => {
    expect(isParityCorpus(null)).toBe(false);
    expect(isParityCorpus(undefined)).toBe(false);
    expect(isParityCorpus('{}')).toBe(false);
    expect(isParityCorpus(42)).toBe(false);
  });

  it('rejects an object with no libraries map', () => {
    expect(isParityCorpus({})).toBe(false);
    expect(isParityCorpus({ libraries: null })).toBe(false);
    expect(isParityCorpus({ libraries: [] })).toBe(false);
    expect(isParityCorpus({ libraries: {} })).toBe(false);
  });

  it('rejects the browser summary shape: percentages with no evidence', () => {
    expect(
      isParityCorpus({
        libraries: {
          algebra: {
            parityPercent: 90.8,
            total: 292,
            match: 265,
            mismatch: 27,
            deviations: 0,
            upstream: { raw: 'algebra.js@0.2.6' },
          },
        },
      }),
    ).toBe(false);
  });

  it('accepts a corpus entry carrying cases, coverage or a harnessPath', () => {
    expect(isParityCorpus({ libraries: { a: { cases: [] } } })).toBe(true);
    expect(isParityCorpus({ libraries: { a: { coverage: [] } } })).toBe(true);
    expect(isParityCorpus({ libraries: { a: { harnessPath: 'parity/a' } } })).toBe(true);
    // an empty harnessPath is not evidence
    expect(isParityCorpus({ libraries: { a: { harnessPath: '' } } })).toBe(false);
  });

  it('rejects the real public/parity.json summary when it is present', () => {
    const summary = readJson(SUMMARY);
    if (summary === undefined) return;
    expect(isParityCorpus(summary)).toBe(false);
  });

  it('accepts the real generated corpus when it is present', () => {
    const corpus = readJson(CORPUS);
    if (corpus === undefined) return;
    expect(isParityCorpus(corpus)).toBe(true);
  });
});

// loadParityData caches per module instance, so each case re-imports the module
// with node:fs stubbed to hand back a chosen payload — the same thing that
// happens on a build where the generator was skipped.
describe('loadParityData', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.doUnmock('node:fs');
    vi.restoreAllMocks();
  });

  async function loadWith(payload: string | null) {
    vi.doMock('node:fs', () => ({
      default: {
        readFileSync: () => {
          if (payload === null) throw new Error('ENOENT');
          return payload;
        },
      },
    }));
    return await import('../../app/parity/load');
  }

  it('reads the generated corpus and exposes every harness', async () => {
    const corpus = readJson(CORPUS);
    if (corpus === undefined) return;
    const mod = await loadWith(JSON.stringify(corpus));
    expect(mod.paritySlugs().length).toBeGreaterThan(30);
    const express = mod.parityHarness('express');
    expect(express?.harness.cases.length).toBeGreaterThan(0);
    expect(express?.harness.harnessPath).not.toBe('');
  });

  it('renders the empty state when the corpus is absent', async () => {
    const mod = await loadWith(null);
    expect(mod.paritySlugs()).toEqual([]);
    expect(mod.parityHarness('express')).toBeNull();
  });

  it('refuses the browser summary rather than publishing unbacked numbers', async () => {
    const summary = readJson(SUMMARY);
    if (summary === undefined) return;
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const mod = await loadWith(JSON.stringify(summary));
    expect(mod.paritySlugs()).toEqual([]);
    expect(mod.parityHarness('express')).toBeNull();
    expect(mod.paritySummaries()).toEqual([]);
    expect(warn).toHaveBeenCalled();
  });
});
