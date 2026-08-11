// Tests for the two metadata image routes (app/opengraph-image.tsx and
// app/lib/[id]/opengraph-image.tsx).
//
// The point of these cards is the TEXT they carry — a shared /lib/express link
// must preview as "Express for Go" with the MEASURED parity against the pinned
// oracle, never a hand-written figure. Rasterising is @vercel/og's job and is far
// too slow for a unit test, so 'next/og' is mocked with a stub that records the
// element tree, and the assertions walk that tree for the strings.
import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactElement, ReactNode } from 'react';
import { LIBS } from '../../../src/data';

// Captures every ImageResponse constructed during a test.
const responses: { element: ReactNode; options: unknown }[] = [];

vi.mock('next/og', () => ({
  ImageResponse: class {
    constructor(element: ReactNode, options: unknown) {
      responses.push({ element, options });
    }
  },
}));

// The per-library card reads the generated parity corpus through the same server
// loader /parity uses. api/_data/parity.json is a gitignored build artifact, so
// the loader is stubbed with a fixture: the test then asserts the card reflects
// whatever was measured, independent of whether a data build has run.
vi.mock('../../../app/parity/load', () => ({
  parityHarness: (slug: string) =>
    slug === 'express'
      ? {
          slug: 'express',
          harness: {
            upstream: { raw: 'express@4.21.2', name: 'express', version: '4.21.2', ecosystem: 'node' },
            parityPercent: 98.9,
            comparedTotal: 184,
          },
        }
      : null,
  libMeta: () => ({ name: 'Express', accent: '#00add8', repo: null }),
}));

/** Every string in a JSX tree, flattened in document order. */
function textOf(node: ReactNode): string {
  if (node === null || node === undefined || typeof node === 'boolean') return '';
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  const el = node as ReactElement<{ children?: ReactNode }>;
  if (el.props) return textOf(el.props.children);
  return '';
}

beforeEach(() => {
  responses.length = 0;
});

describe('app/opengraph-image', () => {
  it('renders a 1200x630 card with the wordmark, tagline and library count', async () => {
    const mod = await import('../../../app/opengraph-image');
    expect(mod.size).toEqual({ width: 1200, height: 630 });
    expect(mod.contentType).toBe('image/png');
    expect(mod.alt).toMatch(/malcolmston\/go/);
    // Without this the GitHub Pages export build refuses the route outright.
    expect(mod.dynamic).toBe('force-static');

    mod.default();
    expect(responses).toHaveLength(1);
    const text = textOf(responses[0].element);
    expect(text).toContain('malcolmston/go');
    expect(text).toContain('The Node.js ecosystem, reimagined in Go.');
    expect(text).toContain(String(LIBS.length));
    expect(responses[0].options).toEqual({ width: 1200, height: 630 });
  });
});

describe('app/lib/[id]/opengraph-image', () => {
  it('emits one static param per library, mirroring the page route', async () => {
    const mod = await import('../../../app/lib/[id]/opengraph-image');
    expect(mod.generateStaticParams()).toEqual(LIBS.map((lib) => ({ id: lib.id })));
    expect(mod.dynamic).toBe('force-static');
  });

  it('puts the measured parity and the pinned oracle on the card', async () => {
    const mod = await import('../../../app/lib/[id]/opengraph-image');
    await mod.default({ params: Promise.resolve({ id: 'express' }) });
    const text = textOf(responses[0].element);
    expect(text).toContain('Express for Go');
    expect(text).toContain('98.9%');
    expect(text).toContain('parity vs express@4.21.2');
    expect(text).toContain('github.com/malcolmston/express');
  });

  it('omits a score for a library with no harness instead of inventing one', async () => {
    const unmeasured = LIBS.find((lib) => lib.id !== 'express');
    expect(unmeasured).toBeDefined();
    const mod = await import('../../../app/lib/[id]/opengraph-image');
    await mod.default({ params: Promise.resolve({ id: unmeasured!.id }) });
    const text = textOf(responses[0].element);
    expect(text).toContain(`${unmeasured!.name} for Go`);
    expect(text).not.toMatch(/\d+\.\d%/);
    expect(text).toContain(`An independent Go port of ${unmeasured!.node}`);
  });

  it('does not throw on a malformed percent-escape in the id', async () => {
    const mod = await import('../../../app/lib/[id]/opengraph-image');
    await mod.default({ params: Promise.resolve({ id: '%E0' }) });
    const text = textOf(responses[0].element);
    expect(text).toContain('malcolmston/go');
  });
});
