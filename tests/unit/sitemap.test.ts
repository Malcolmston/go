import { describe, it, expect, afterEach } from 'vitest';
import { entries, siteUrl } from '../../app/sitemap';
import robots from '../../app/robots';
import { LIBS } from '../../src/data';
import { paritySlugs } from '../../app/parity/load';
import { SITE_URL } from '../../app/seo';

// app/sitemap.ts and app/robots.ts are the crawl entry point for ~90 static
// URLs. They are file-convention routes, so nothing else in the app imports
// them — these tests are the only guard that the URL list stays derived from
// LIBS + the generated parity dataset and stays absolute against one origin.
const ORIGIN = SITE_URL.replace(/\/+$/, '');

afterEach(() => {
  delete process.env.NEXT_PUBLIC_SITE_URL;
});

describe('siteUrl', () => {
  it('falls back to the site-wide canonical origin', () => {
    delete process.env.NEXT_PUBLIC_SITE_URL;
    expect(siteUrl()).toBe(ORIGIN);
  });

  it('honours NEXT_PUBLIC_SITE_URL and strips a trailing slash', () => {
    process.env.NEXT_PUBLIC_SITE_URL = 'https://example.test/';
    expect(siteUrl()).toBe('https://example.test');
  });

  it('ignores a blank override', () => {
    process.env.NEXT_PUBLIC_SITE_URL = '   ';
    expect(siteUrl()).toBe(ORIGIN);
  });
});

describe('sitemap entries', () => {
  it('lists the home page and every fixed section, absolute', () => {
    const urls = entries().map((e) => e.url);
    for (const path of [
      '',
      '/parity',
      '/security',
      '/explore',
      '/releases',
      '/howto',
      '/faq',
      '/ai',
      '/ask',
      '/about',
    ]) {
      expect(urls).toContain(`${ORIGIN}${path}`);
    }
  });

  // /pipeline is a redirect stub that declares robots noindex and canonicalises
  // at /parity (app/pipeline/page.tsx), so advertising it here would invite
  // crawlers to a URL the page itself asks them to drop.
  it('omits the noindex /pipeline redirect stub', () => {
    expect(entries().map((e) => e.url)).not.toContain(`${ORIGIN}/pipeline`);
  });

  it('lists one /lib/<id> entry per library', () => {
    const urls = new Set(entries().map((e) => e.url));
    expect(LIBS.length).toBeGreaterThan(30);
    for (const lib of LIBS) {
      expect(urls.has(`${ORIGIN}/lib/${encodeURIComponent(lib.id)}`)).toBe(true);
    }
  });

  it('lists one /parity/<slug> entry per measured harness', () => {
    const urls = new Set(entries().map((e) => e.url));
    const slugs = paritySlugs();
    // The generated dataset is a build artefact; when it is absent there are no
    // harness pages to list either, so the invariant is "one entry per slug".
    for (const slug of slugs) {
      expect(urls.has(`${ORIGIN}/parity/${encodeURIComponent(slug)}`)).toBe(true);
    }
    expect(entries()).toHaveLength(10 + LIBS.length + slugs.length);
  });

  it('emits unique, absolute URLs with a valid priority and a real lastModified', () => {
    const all = entries();
    const urls = all.map((e) => e.url);
    expect(new Set(urls).size).toBe(urls.length);
    for (const e of all) {
      expect(e.url.startsWith(`${ORIGIN}/`) || e.url === ORIGIN).toBe(true);
      expect(e.priority).toBeGreaterThan(0);
      expect(e.priority).toBeLessThanOrEqual(1);
      expect(Number.isNaN(new Date(e.lastModified as Date).getTime())).toBe(false);
    }
  });

  it('rebases every URL on the configured origin', () => {
    process.env.NEXT_PUBLIC_SITE_URL = 'https://example.test';
    for (const e of entries()) {
      expect(e.url.startsWith('https://example.test')).toBe(true);
    }
  });
});

describe('robots', () => {
  it('allows the site, disallows the API surface and points at the sitemap', () => {
    const r = robots();
    const rules = Array.isArray(r.rules) ? r.rules : [r.rules!];
    expect(rules[0].userAgent).toBe('*');
    expect(rules[0].allow).toBe('/');
    expect(rules[0].disallow).toEqual(['/api/']);
    expect(r.sitemap).toBe(`${ORIGIN}/sitemap.xml`);
    expect(r.host).toBe(ORIGIN);
  });

  it('tracks NEXT_PUBLIC_SITE_URL', () => {
    process.env.NEXT_PUBLIC_SITE_URL = 'https://example.test';
    expect(robots().sitemap).toBe('https://example.test/sitemap.xml');
  });
});
