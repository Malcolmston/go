import { describe, it, expect } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { subsetNames } from '../../playwright.config';

// tests/e2e/routes.spec.ts is the only browser coverage the non-nav routes have:
// the 40 /parity/<harness> pages, their /nested JSON endpoints, /pipeline, the
// 404 body, the metadata routes and /api/parity. The two older specs derive
// their route list from the nav, so nothing else visits any of these.
//
// Playwright specs are not run by vitest, so this file is the unit-level guard
// that the coverage list itself does not quietly shrink — and that the two
// mechanics the spec depends on stay in place: the harness list is DERIVED from
// app/parity/load (not hand-written, so a new harness is covered for free), and
// the whole file is pinned to one device project so 50-odd route checks are not
// multiplied by the 8-project subset.
const SPEC = path.join(process.cwd(), 'tests', 'e2e', 'routes.spec.ts');
const source = fs.readFileSync(SPEC, 'utf8');

describe('tests/e2e/routes.spec.ts', () => {
  it('derives the harness list from the same loader the pages use', () => {
    expect(source).toMatch(/import \{ paritySlugs \} from '\.\.\/\.\.\/app\/parity\/load'/);
    expect(source).toContain('paritySlugs()');
  });

  it('runs in a single device project taken from the configured subset', () => {
    expect(source).toContain('subsetNames()[0]');
    expect(source).toMatch(/testInfo\.project\.name !== ONLY_PROJECT/);
    // The name the guard resolves to must be a project the default run has.
    expect(subsetNames()).toContain(subsetNames()[0]);
  });

  it('covers every route family the nav-driven specs miss', () => {
    for (const route of [
      '/parity/${slug}',
      '/parity/${slug}/nested',
      '/pipeline',
      '/sitemap.xml',
      '/robots.txt',
      '/manifest.webmanifest',
      '/opengraph-image',
      '/lib/express/opengraph-image',
      '/icon',
      '/api/parity',
    ]) {
      expect(source, `missing coverage for ${route}`).toContain(route);
    }
    // The 404 body, asserted by its view id rather than by a path.
    expect(source).toContain('view-not-found');
  });

  it('waits for hydration before counting the single active view', () => {
    // /parity/<lib> streams TWO `.view.active` sections (the loading fallback
    // plus the record); only a hydrated browser shows one, so the count
    // assertion is worthless without the hydration wait.
    expect(source).toMatch(/import \{ waitForHydration \} from '\.\/hydrate'/);
    expect(source).toContain("expect(page.locator('.view.active')).toHaveCount(1");
  });
});
