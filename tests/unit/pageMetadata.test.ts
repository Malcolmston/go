import { describe, it, expect } from 'vitest';
import type { Metadata } from 'next';

// Every top-level route must describe itself. These pages used to be client
// components, which cannot export `metadata`, so all of them inherited the
// layout defaults and shipped the identical <title>/<meta description>. Each one
// is now a server component with its own metadata, and this test locks that in:
// a future refactor that turns one back into a client component (dropping the
// export) fails here instead of silently regressing the titles.
const ROUTES: Record<string, () => Promise<{ metadata?: Metadata }>> = {
  '/about': () => import('../../app/about/page'),
  '/ai': () => import('../../app/ai/page'),
  '/ask': () => import('../../app/ask/page'),
  '/explore': () => import('../../app/explore/page'),
  '/faq': () => import('../../app/faq/page'),
  '/howto': () => import('../../app/howto/page'),
  '/releases': () => import('../../app/releases/page'),
  '/parity': () => import('../../app/parity/page'),
  '/security': () => import('../../app/security/page'),
};

// The canonical/Open Graph assertions cover the same routes plus the home page.
// '/' is excluded from ROUTES because it legitimately keeps the layout's default
// title, which the title/description assertions below forbid — but it still owes
// a canonical and its own og:url, so it belongs here.
const CANONICAL_ROUTES: Record<string, () => Promise<{ metadata?: Metadata }>> = {
  ...ROUTES,
  '/': () => import('../../app/page'),
};

const LAYOUT_DEFAULT_TITLE = 'malcolmston/go';
const LAYOUT_DEFAULT_DESCRIPTION = 'The Node.js ecosystem, reimagined in Go.';

async function metadataFor(path: string): Promise<Metadata> {
  const mod = await CANONICAL_ROUTES[path]();
  const meta = mod.metadata;
  expect(meta, `${path} must export metadata`).toBeDefined();
  return meta as Metadata;
}

describe('top-level route metadata', () => {
  for (const path of Object.keys(ROUTES)) {
    it(`${path} exports its own title and description`, async () => {
      const meta = await metadataFor(path);
      expect(typeof meta.title).toBe('string');
      const title = meta.title as string;
      expect(title.length).toBeGreaterThan(0);
      expect(title).not.toBe(LAYOUT_DEFAULT_TITLE);
      expect(typeof meta.description).toBe('string');
      const description = meta.description as string;
      // Long enough to be a real snippet, short enough for a search result.
      expect(description.length).toBeGreaterThan(60);
      expect(description.length).toBeLessThan(320);
      expect(description).not.toBe(LAYOUT_DEFAULT_DESCRIPTION);
    });
  }

  it('gives no two routes the same title or description', async () => {
    const titles: string[] = [];
    const descriptions: string[] = [];
    for (const path of Object.keys(ROUTES)) {
      const meta = await metadataFor(path);
      titles.push(meta.title as string);
      descriptions.push(meta.description as string);
    }
    expect(new Set(titles).size).toBe(titles.length);
    expect(new Set(descriptions).size).toBe(descriptions.length);
  });

  // The regression lock for the canonical/card fix: every route must point its
  // canonical and og:url at ITSELF. Eight of these shipped no canonical at all
  // and inherited the site-wide card, so a share of /faq rendered the generic
  // home-page preview, and the Vercel and GitHub Pages copies of each page
  // competed as duplicates.
  for (const path of Object.keys(CANONICAL_ROUTES)) {
    it(`${path} declares its own canonical and Open Graph card`, async () => {
      const meta = await metadataFor(path);
      expect(meta.alternates?.canonical).toBe(path);
      expect(meta.openGraph?.url).toBe(path);
      expect(meta.openGraph?.title).toBeTruthy();
      expect(meta.openGraph?.description).toBe(meta.description);
      // Next's `twitter` field is a union whose base member has no `card`, so
      // read it through a narrow shape rather than off the union directly.
      const twitter = meta.twitter as { card?: string } | undefined;
      expect(twitter?.card).toBe('summary_large_image');
    });
  }
});
