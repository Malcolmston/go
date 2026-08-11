import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { render, screen } from '@testing-library/react';
import { ScorePill } from '../../../src/components/ParityTables';
import { accentVar } from '../../../src/components/accentText';
import { LIBS } from '../../../src/data';

// The accent hues are brand colours picked for a dark backdrop. Used raw as a
// text colour they failed WCAG AA badly (1.55:1 for the parity percentage on
// the light theme), so components now ship the accent as `--acc` and let
// AccentText.css derive the readable text colour per theme. Two things have to
// stay true: no component may put the raw accent back into `color`, and the
// mix percentages in AccentText.css must actually clear 4.5:1 for every accent
// and severity colour the site draws.

const CSS = readFileSync(resolve(process.cwd(), 'src/components/AccentText.css'), 'utf8');

/* ---- colour maths: oklab mixing (as color-mix(in oklab, ...) does) ---- */

const srgb = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
const lin = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const unlin = (c: number) => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055);

function toOklab([r0, g0, b0]: number[]) {
  const r = lin(r0), g = lin(g0), b = lin(b0);
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

function fromOklab([L, a, bb]: number[]) {
  const l = (L + 0.3963377774 * a + 0.2158037573 * bb) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * bb) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * bb) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ].map((v) => Math.min(1, Math.max(0, unlin(v))));
}

/** color-mix(in oklab, a, b <pct>%) */
function mix(a: string, b: string, pct: number) {
  const A = toOklab(srgb(a));
  const B = toOklab(srgb(b));
  return fromOklab(A.map((v, i) => v * (1 - pct) + B[i] * pct));
}

const relL = ([r, g, b]: number[]) => 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
function contrast(fg: number[], bg: number[]) {
  const x = relL(fg);
  const y = relL(bg);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** The mix declared in AccentText.css for a theme, read out of the stylesheet. */
function declaredMix(block: RegExp) {
  const m = CSS.match(block);
  if (!m) throw new Error('AccentText.css no longer declares that theme block');
  const toward = /--acc-text-toward:\s*(#[0-9a-f]{6})/i.exec(m[0]);
  const pct = /--acc-text-mix:\s*([\d.]+)%/.exec(m[0]);
  if (!toward || !pct) throw new Error('AccentText.css is missing --acc-text-toward / --acc-text-mix');
  return { toward: toward[1], pct: Number(pct[1]) / 100 };
}

// Every hue the site paints as accent text: the library accents plus the
// security severity/status scale (Security.tsx) — kept in sync by hand here on
// purpose, so adding a low-contrast colour there trips this test.
const SEVERITY_AND_STATUS = ['#ff5c7c', '#ff8a3d', '#e3b341', '#8b98a9', '#3fb950'];
const ACCENTS = [...new Set([...LIBS.map((l) => l.accent), ...SEVERITY_AND_STATUS])];

describe('accent text contrast', () => {
  it('clears WCAG AA (4.5:1) on the dark theme for every accent', () => {
    const { toward, pct } = declaredMix(/:root\s*\{[^}]*\}/);
    // The lightest surface an accent pill is drawn on in the dark theme.
    const bg = srgb('#1a1d24');
    const worst = Math.min(...ACCENTS.map((a) => contrast(mix(a, toward, pct), bg)));
    expect(worst).toBeGreaterThanOrEqual(4.5);
  });

  it('clears WCAG AA (4.5:1) on the light theme for every accent', () => {
    const { toward, pct } = declaredMix(/:root\[data-theme='light'\]\s*\{[^}]*\}/);
    // The lightest surface possible in the light theme: plain white.
    const bg = srgb('#ffffff');
    const worst = Math.min(...ACCENTS.map((a) => contrast(mix(a, toward, pct), bg)));
    expect(worst).toBeGreaterThanOrEqual(4.5);
  });

  it('the raw accent fails AA on the light theme, which is why the mix exists', () => {
    const bg = srgb('#ffffff');
    const failing = ACCENTS.filter((a) => contrast(srgb(a), bg) < 4.5);
    expect(failing.length).toBeGreaterThan(ACCENTS.length / 2);
  });
});

describe('accentVar', () => {
  it('hands the accent over as a custom property, never as a colour', () => {
    const style = accentVar('#f2cc60', { borderColor: '#f2cc6055' }) as Record<string, string>;
    expect(style['--acc']).toBe('#f2cc60');
    expect(style.borderColor).toBe('#f2cc6055');
    expect(style.color).toBeUndefined();
  });
});

describe('ScorePill', () => {
  it('renders the percentage as acc-text with --acc set and no inline colour', () => {
    render(<ScorePill value={90.8} compared={184} accent="#f2cc60" />);
    const pill = screen.getByText(/90\.8%/);
    expect(pill.className).toContain('parity-pill');
    expect(pill.className).toContain('acc-text');
    const inline = pill.getAttribute('style') ?? '';
    expect(inline).toContain('--acc: #f2cc60');
    // `color:` would defeat the stylesheet; border-color/background may keep it.
    expect(/(^|;)\s*color:/.test(inline)).toBe(false);
  });
});
