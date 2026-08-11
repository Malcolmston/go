import { describe, expect, it } from 'vitest';
import { scoredDenominator } from '../../../src/components/ParityData';

// The displayed ratio and the displayed percentage have to divide out to the same
// number. They did not: /parity/express showed "182 / 183 cases in agreement"
// beside a "98.9% measured parity" headline, and 182/183 is 99.45%. A reader who
// checks the arithmetic finds a contradiction and stops trusting the page, which
// is the opposite of what a parity report is for.
describe('scoredDenominator', () => {
  it('uses total when the harness counts a deviation inside mismatch (the express case)', () => {
    // Real numbers from api/_data/parity.json: express publishes 98.91, which is
    // 182/184. comparedTotal is 183 because it subtracts the 1 deviation, but that
    // deviation is already one of the 2 mismatches, so subtracting double-counts.
    const express = { match: 182, total: 184, comparedTotal: 183, parityPercent: 98.91 };
    expect(scoredDenominator(express)).toBe(184);
    expect((100 * express.match) / scoredDenominator(express)).toBeCloseTo(express.parityPercent, 1);
  });

  it('uses comparedTotal when deviations are genuinely excluded from the score', () => {
    // The common shape: 100% of compared cases, with deviations held out.
    const qs = { match: 268, total: 271, comparedTotal: 268, parityPercent: 100 };
    expect(scoredDenominator(qs)).toBe(268);
    expect((100 * qs.match) / scoredDenominator(qs)).toBeCloseTo(100, 1);
  });

  it('prefers comparedTotal when both denominators reproduce the percentage', () => {
    const clean = { match: 50, total: 50, comparedTotal: 50, parityPercent: 100 };
    expect(scoredDenominator(clean)).toBe(50);
  });

  it('falls back to total rather than dividing by zero', () => {
    const empty = { match: 0, total: 0, comparedTotal: 0, parityPercent: 0 };
    expect(scoredDenominator(empty)).toBe(0);
  });

  // Guard the invariant across the whole shipped dataset, so a future harness that
  // invents a third convention is caught here rather than on the page.
  it('reproduces the published percentage for every library in the dataset', async () => {
    const { readFileSync, existsSync } = await import('node:fs');
    const path = new URL('../../../api/_data/parity.json', import.meta.url);
    if (!existsSync(path)) return; // artefact not built in this environment
    const data = JSON.parse(readFileSync(path, 'utf8')) as {
      libraries: Record<string, { match: number; total: number; comparedTotal: number; parityPercent: number }>;
    };
    const offenders: string[] = [];
    for (const [id, h] of Object.entries(data.libraries ?? {})) {
      const d = scoredDenominator(h);
      if (d <= 0) continue;
      const shown = (100 * h.match) / d;
      // 0.6 of a point of slack: harnesses round their published percentage.
      if (Math.abs(shown - h.parityPercent) > 0.6) {
        offenders.push(`${id}: ${h.match}/${d} = ${shown.toFixed(2)}% but publishes ${h.parityPercent}%`);
      }
    }
    expect(offenders).toEqual([]);
  });
});
