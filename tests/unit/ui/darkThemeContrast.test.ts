import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// The dark theme is the default: the bare `:root` block in ui/src/styles.css ships
// unless the user opts into data-theme="light". Its tokens are plain custom
// properties, so the contrast contract can be checked statically the same way the
// light theme is (see lightThemeContrast.test.ts). This is the guard for the
// regression where --fg-dim was #727a88 — 4.22:1 over the --glass surface and
// 3.32:1 over --glass-hi, i.e. below AA on the stat-tile labels, table headers and
// package paths that carry it (326 axe-core nodes across /, /security, /parity).

// vitest runs from the repo root (vitest.config.ts lives there).
const CSS = readFileSync(resolve(process.cwd(), 'ui/src/styles.css'), 'utf8');
const CSS_BARE = CSS.replace(/\/\*[\s\S]*?\*\//g, '');

function block(selector: string): Record<string, string> {
  const at = CSS_BARE.indexOf(selector + '{');
  if (at < 0) throw new Error(`no ${selector} block in styles.css`);
  const body = CSS_BARE.slice(at + selector.length + 1, CSS_BARE.indexOf('}', at));
  const out: Record<string, string> = {};
  for (const decl of body.split(';')) {
    const [prop, ...rest] = decl.split(':');
    if (!prop?.trim().startsWith('--')) continue;
    out[prop.trim()] = rest.join(':').trim();
  }
  return out;
}

type RGB = [number, number, number];

function parse(value: string): { rgb: RGB; alpha: number } {
  const hex = /^#([0-9a-f]{6})$/i.exec(value);
  if (hex) {
    const n = parseInt(hex[1], 16);
    return { rgb: [(n >> 16) & 255, (n >> 8) & 255, n & 255], alpha: 1 };
  }
  const fn = /^rgba?\(([^)]+)\)$/i.exec(value);
  if (!fn) throw new Error(`unsupported colour: ${value}`);
  const parts = fn[1].split(',').map((p) => Number(p.trim()));
  return { rgb: [parts[0], parts[1], parts[2]] as RGB, alpha: parts.length > 3 ? parts[3] : 1 };
}

const over = (fg: string, bg: RGB): RGB => {
  const { rgb, alpha } = parse(fg);
  return rgb.map((c, i) => Math.round(c * alpha + bg[i] * (1 - alpha))) as RGB;
};

function luminance([r, g, b]: RGB): number {
  const lin = (c: number) => {
    const s = c / 255;
    return s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function contrast(a: RGB, b: RGB): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe('dark theme token contrast (the default theme)', () => {
  const dark = block(':root');
  const bg = parse(dark['--bg']).rgb;
  // surfaces text actually sits on: the page and the three glass tints over it
  const surfaces: Array<[string, RGB]> = [
    ['--bg', bg],
    ['--glass', over(dark['--glass'], bg)],
    ['--glass-2', over(dark['--glass-2'], bg)],
    ['--glass-hi', over(dark['--glass-hi'], bg)],
    ['--code-bg', over(dark['--code-bg'], bg)],
  ];

  for (const token of ['--fg', '--fg-muted', '--fg-dim', '--link']) {
    for (const [name, surface] of surfaces) {
      it(`${token} clears 4.5:1 on ${name}`, () => {
        expect(contrast(parse(dark[token]).rgb, surface)).toBeGreaterThanOrEqual(4.5);
      });
    }
  }

  it('the parsed block really is the dark default, not the light override', () => {
    expect(luminance(bg)).toBeLessThan(0.05);
  });
});
