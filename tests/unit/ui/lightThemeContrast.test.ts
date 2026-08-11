import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// The light theme is a plain block of custom properties in ui/src/styles.css, so
// the contrast contract can be checked statically: parse the tokens out of the
// `:root[data-theme="light"]` block and compute WCAG ratios. This is the guard
// for the regression where --fg-dim was #86868b (3.33:1 on --bg, below AA) and
// --edge was a white hairline on an off-white page (1.06:1, invisible).

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

describe('light theme token contrast', () => {
  const light = block(':root[data-theme="light"]');
  const bg = parse(light['--bg']).rgb;
  // surfaces text actually sits on: the page and the three glass tints over it
  const surfaces: Array<[string, RGB]> = [
    ['--bg', bg],
    ['--glass', over(light['--glass'], bg)],
    ['--glass-2', over(light['--glass-2'], bg)],
    ['--glass-hi', over(light['--glass-hi'], bg)],
    ['--code-bg', over(light['--code-bg'], bg)],
  ];

  for (const token of ['--fg', '--fg-muted', '--fg-dim', '--link']) {
    for (const [name, surface] of surfaces) {
      it(`${token} clears 4.5:1 on ${name}`, () => {
        expect(contrast(parse(light[token]).rgb, surface)).toBeGreaterThanOrEqual(4.5);
      });
    }
  }

  // syntax tokens only ever render inside a code block, so --code-bg is the
  // only surface that matters for them; the comment colour is the dimmest one.
  for (const token of ['--tok-c', '--tok-k', '--tok-s', '--tok-f', '--tok-n', '--tok-t']) {
    it(`${token} clears 4.5:1 on --code-bg`, () => {
      const codeBg = over(light['--code-bg'], bg);
      expect(contrast(parse(light[token]).rgb, codeBg)).toBeGreaterThanOrEqual(4.5);
    });
  }

  it('--edge is a visible boundary, not a white-on-white hairline', () => {
    expect(contrast(over(light['--edge'], bg), bg)).toBeGreaterThan(1.5);
  });

  it('--edge-2 clears 3:1 on the page, so light-mode controls have a perceivable edge', () => {
    expect(contrast(over(light['--edge-2'], bg), bg)).toBeGreaterThanOrEqual(3);
  });

  it('light-mode form controls are bound to --edge-2', () => {
    expect(CSS).toContain(
      ':root[data-theme="light"] :is(input,select,textarea,button):not(:focus){border-color:var(--edge-2)}',
    );
  });
});
