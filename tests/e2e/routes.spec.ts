// Route existence, for every route the nav does NOT list.
//
// tests/e2e/site.spec.ts and tests/e2e/interactions.spec.ts both derive their
// route list from the nav (site.spec builds TAB_IDS from src/data LIBS, and
// interactions.spec reads `nav.tabs a.tab` hrefs out of the DOM), so the
// majority of the pages the build emits had no browser coverage at all: the 40
// /parity/<harness> records, their 40 /parity/<harness>/nested JSON endpoints,
// /pipeline, the 404 body, the metadata routes (sitemap, robots, manifest, the
// OG images) and /api/parity. site.spec only ever matches /parity/<lib> as a
// *link shape* (KNOWN_PATH_PATTERNS) — it never navigates to one.
//
// Two deliberate choices here:
//
//  1. These are route-existence checks, not responsive checks, so they run in a
//     SINGLE Playwright project. Multiplying 80+ route hits by the 8-project
//     device subset would only burn wall-clock for identical assertions.
//  2. Anything that is not HTML (sitemap, robots, manifest, OG images, JSON) is
//     fetched with `request.get()` — no browser, no hydration, no page errors to
//     filter — and only the HTML pages pay for a real navigation.
import { test, expect, type Page } from '@playwright/test';
import { paritySlugs } from '../../app/parity/load';
import { subsetNames } from '../../playwright.config';
import { waitForHydration } from './hydrate';

// The harness keys, read from the same loader app/parity/[lib]/page.tsx uses in
// generateStaticParams — so a harness added to the corpus is covered here
// without editing a hand-written list, and a harness whose page stops being
// emitted fails instead of silently dropping out of the sweep.
const SLUGS = paritySlugs();

// The one project these tests run in: the first entry of the default device
// subset (Desktop Chrome unless Playwright renames it). subsetNames() only ever
// returns names the installed Playwright ships, and the full E2E_FULL=1 matrix
// is a superset of the subset, so this name exists in both run modes.
const ONLY_PROJECT = subsetNames()[0];

// Deliberately declared WITHOUT the `page` fixture: most of these tests only
// need `request`, and naming `page` in a hook builds a browser page for every
// one of them.
test.beforeEach(({}, testInfo) => {
  test.skip(
    testInfo.project.name !== ONLY_PROJECT,
    `route-existence checks run once, in ${ONLY_PROJECT}`,
  );
});

/**
 * Load an HTML route and wait until the client has hydrated the shell.
 *
 * domcontentloaded rather than 'load' for the reason site.spec.ts documents (the
 * blocked external subresources), and the response status is asserted here so a
 * 500 fails as a 500 instead of as a confusing missing-element timeout.
 */
async function gotoHtml(page: Page, path: string, expectedStatus = 200) {
  // Same external-host block as site.spec.ts: font CSS and api.github.com are
  // network-blocked in CI, where a render-blocking <link> stalls first paint
  // until it times out. Aborting makes them fail immediately.
  await page.route(
    /(fonts\.googleapis\.com|fonts\.gstatic\.com|kit\.fontawesome\.com|ka-f\.fontawesome\.com|api\.github\.com)/,
    (r) => r.abort(),
  );
  // Reduced motion so the wormhole page transition resolves instantly.
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const res = await page.goto(path, { waitUntil: 'domcontentloaded' });
  expect(res, `no response for ${path}`).not.toBeNull();
  expect(res!.status(), `status for ${path}`).toBe(expectedStatus);
  await waitForHydration(page);
}

// express is the heaviest record (28 nested ports), socket.io is the only slug
// carrying a dot, algebra is a plain one — enough to cover the render path.
const RENDER_SLUGS = ['express', 'socket.io', 'algebra'];

test.describe('parity records', () => {
  test('the corpus has every harness the site advertises', () => {
    expect(SLUGS.length).toBeGreaterThanOrEqual(40);
    expect(new Set(SLUGS).size).toBe(SLUGS.length);
    // The representatives below are hand-picked, so they must still exist.
    for (const slug of RENDER_SLUGS) expect(SLUGS).toContain(slug);
  });

  // All 40 records are checked over HTTP rather than by driving a browser to
  // each one: the assertion that matters for a prerendered page is that the
  // route exists and that its own record is what the server emitted. Rendering
  // 40 of the heaviest pages on the site in a browser costs minutes and pushes
  // `next start` towards its heap limit for no extra signal — the hydration path
  // is exercised on the representatives below.
  test('every /parity/<lib> page serves its own record', async ({ request }) => {
    const bad: string[] = [];
    for (const slug of SLUGS) {
      const res = await request.get(`/parity/${slug}`);
      const type = res.headers()['content-type'] ?? '';
      if (!res.ok() || !type.includes('text/html')) {
        bad.push(`${slug}: ${res.status()} ${type}`);
        continue;
      }
      const html = await res.text();
      if (!html.includes(`id="view-parity-${slug}"`)) bad.push(`${slug}: no record section`);
    }
    expect(bad, `bad /parity/<lib> pages:\n${bad.join('\n')}`).toEqual([]);
  });

  for (const slug of RENDER_SLUGS) {
    test(`/parity/${slug} renders and hydrates in a browser`, async ({ page }) => {
      await gotoHtml(page, `/parity/${slug}`);
      // The streamed HTML of a heavy record legitimately carries TWO
      // `class="view active"` sections — `view-parity-lib-loading` (the Suspense
      // fallback from app/parity/[lib]/loading.tsx) and the record itself. Only
      // after hydration does the browser show one, so this count is asserted
      // with expect's auto-retry rather than read once.
      await expect(page.locator('.view.active')).toHaveCount(1, { timeout: 20_000 });
      // Attribute selector, not `#id`: slugs contain dots (socket.io).
      await expect(page.locator(`.view.active[id="view-parity-${slug}"]`)).toBeVisible();
      // A record that rendered its frame but none of its measured content would
      // still pass the assertions above.
      await expect(page.locator('h2').first()).not.toBeEmpty();
    });
  }

  test('every /parity/<lib>/nested endpoint answers JSON for its own library', async ({
    request,
  }) => {
    const bad: string[] = [];
    for (const slug of SLUGS) {
      const res = await request.get(`/parity/${slug}/nested`);
      const type = res.headers()['content-type'] ?? '';
      if (!res.ok() || !type.includes('application/json')) {
        bad.push(`${slug}: ${res.status()} ${type}`);
        continue;
      }
      const body = (await res.json()) as { library?: string; nested?: unknown };
      if (body.library !== slug) bad.push(`${slug}: library=${String(body.library)}`);
    }
    expect(bad, `bad /parity/<lib>/nested responses:\n${bad.join('\n')}`).toEqual([]);
  });

  test('an unknown harness answers 404 JSON', async ({ request }) => {
    const res = await request.get('/parity/definitely-not-a-harness/nested');
    expect(res.status()).toBe(404);
    expect(await res.json()).toMatchObject({ error: 'unknown library' });
  });
});

test.describe('routes outside the nav', () => {
  test('/pipeline keeps old links working by landing on /parity', async ({ page }) => {
    await gotoHtml(page, '/pipeline');
    // The redirect is a client-side router.replace (see app/pipeline/
    // PipelineClient.tsx) so the static export keeps working; the stub body is
    // what the server sends.
    await expect(page).toHaveURL(/\/parity$/, { timeout: 20_000 });
    await expect(page.locator('.view.active#view-parity')).toBeVisible();
  });

  test('an unknown path renders the not-found view', async ({ page }) => {
    await gotoHtml(page, '/nope-does-not-exist', 404);
    await expect(page.locator('.view.active#view-not-found')).toBeVisible();
    await expect(page).toHaveTitle(/Page not found/);
  });
});

test.describe('metadata routes', () => {
  test('/sitemap.xml lists the site, as XML', async ({ request }) => {
    const res = await request.get('/sitemap.xml');
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type'] ?? '').toContain('xml');
    const body = await res.text();
    expect(body).toContain('<urlset');
    expect(body).toContain('<loc>');
  });

  test('/robots.txt allows crawling and points at the sitemap', async ({ request }) => {
    const res = await request.get('/robots.txt');
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type'] ?? '').toContain('text/plain');
    const body = await res.text();
    expect(body).toContain('User-Agent');
    expect(body).toContain('Sitemap:');
  });

  test('/manifest.webmanifest is an installable manifest', async ({ request }) => {
    const res = await request.get('/manifest.webmanifest');
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type'] ?? '').toContain('manifest');
    const body = (await res.json()) as { name?: string; icons?: unknown[] };
    expect(body.name).toBeTruthy();
    expect(Array.isArray(body.icons) && body.icons.length > 0).toBe(true);
  });

  // Minimum body size per image route: an OG card is a 1200x630 render (~100 KB
  // here), the favicon is a 32x32 one, so they cannot share one threshold. The
  // point of the floor is that a truncated or error-substituted body still
  // carries image/png.
  const IMAGE_ROUTES: Array<{ path: string; minBytes: number }> = [
    { path: '/opengraph-image', minBytes: 10_000 },
    { path: '/lib/express/opengraph-image', minBytes: 10_000 },
    { path: '/icon', minBytes: 200 },
  ];

  for (const { path, minBytes } of IMAGE_ROUTES) {
    test(`${path} is a real image`, async ({ request }) => {
      const res = await request.get(path);
      expect(res.status()).toBe(200);
      expect(res.headers()['content-type'] ?? '').toContain('image/');
      expect((await res.body()).length).toBeGreaterThan(minBytes);
    });
  }
});

test.describe('api routes outside the nav', () => {
  test('/api/parity exposes the measured summary per library', async ({ request }) => {
    const res = await request.get('/api/parity');
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type'] ?? '').toContain('application/json');
    const body = (await res.json()) as {
      generatedAt?: string;
      libraries?: Record<string, { parityPercent?: number }>;
    };
    expect(body.generatedAt).toBeTruthy();
    const libraries = body.libraries ?? {};
    expect(Object.keys(libraries).length).toBeGreaterThanOrEqual(SLUGS.length);
    // The percentage the browser-side Explore reads must be a number, not a
    // string or null, or Explore falls back to the declared figure.
    expect(typeof libraries.express?.parityPercent).toBe('number');
  });
});
