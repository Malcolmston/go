import { test, expect, type Page } from '@playwright/test';
import { LIBS } from '../../src/data';

// The Next.js App Router migration turned every hash tab into a real route:
//   home    -> '/'
//   <top>   -> '/<top>'         (parity, pipeline, explore, releases, howto, faq, ai, about)
//   <libId> -> '/lib/<libId>'   (every ecosystem library from src/data LIBS)
// The go-ui <Layout> still renders each nav link with an href of `#<id>`, but its
// onClick routes through the Next router, so the URL after navigation is the PATH
// (mirrors app/nav.ts pathForTab / tabForPath).
const TOP_LEVEL = [
  'parity',
  'pipeline',
  'explore',
  'releases',
  'howto',
  'faq',
  'ai',
  'about',
] as const;

// The nav, in order: home, then every library, then the top-level sections —
// exactly the tab list ClientRoot feeds to <Layout>.
const TAB_IDS: string[] = ['home', ...LIBS.map((l) => l.id), ...TOP_LEVEL];

/** Map a tab id to its route path (mirror of app/nav.ts pathForTab). */
function pathForTab(id: string): string {
  if (id === 'home') return '/';
  if ((TOP_LEVEL as readonly string[]).includes(id)) return `/${id}`;
  return `/lib/${id}`;
}

/** A regex that matches when the browser URL ends at this tab's path. */
function urlForTab(id: string): RegExp {
  const p = pathForTab(id).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(`${p}$`);
}

// Network to these hosts is blocked in the sandbox; failures there are expected
// and must not fail the page-error assertion.
const IGNORED_HOSTS = ['kit.fontawesome.com', 'api.github.com'];

// Collected uncaught page errors, checked after every test.
let pageErrors: string[] = [];

test.beforeEach(async ({ page }) => {
  pageErrors = [];
  page.on('pageerror', (err) => {
    const msg = `${err.name}: ${err.message}\n${err.stack ?? ''}`;
    // A bare Event (no name, "Event" message, no stack) is a failed subresource
    // (fontawesome kit / fonts, network-blocked in CI) surfacing as an error
    // event -- not a JS exception. Real errors carry a name + message + stack.
    const benign = (err.message === 'Event' || !err.message) && !err.stack;
    if (!benign && !IGNORED_HOSTS.some((h) => msg.includes(h))) pageErrors.push(msg);
  });
  // Reduced motion so the wormhole page transition resolves instantly.
  await page.emulateMedia({ reducedMotion: 'reduce' });
});

test.afterEach(() => {
  expect(pageErrors, `unexpected page errors:\n${pageErrors.join('\n---\n')}`).toEqual([]);
});

// Navigate to a tab's route. The app renders client-only (ssr:false), so after
// the goto we wait for the client to hydrate and paint the active view before
// asserting on it.
async function gotoTab(page: Page, id: string) {
  await page.goto(pathForTab(id));
  // Client-only render: wait atomically for THIS tab's active view to paint
  // (a single combined selector avoids a races between two separate expects).
  await expect(page.locator(`.view.active#view-${id}`)).toBeVisible({ timeout: 20_000 });
}

// Activate a nav tab link. Across 200+ device profiles the tab bar renders three
// ways — inline (desktop), an overflow-scrolled strip (narrow tablets), and a
// collapsed dropdown behind a menu button that overlays the sticky header
// (phones) — so a coordinate-based click is not reliably actionable everywhere.
// Dispatching the click event exercises the same React onClick -> router.push
// handler on every layout; genuine pointer-click coverage lives in the
// "internal navigation links actually navigate" test below. The link href is
// still `#<id>` (go-ui Layout), even though navigation lands on the real path.
async function activateTab(page: Page, id: string) {
  await page.locator(`nav.tabs a.tab[href="#${id}"]`).dispatchEvent('click');
}

test.describe('every page renders responsively', () => {
  for (const id of TAB_IDS) {
    test(`${pathForTab(id)}: single active view, visible heading, no horizontal overflow`, async ({ page }) => {
      await gotoTab(page, id);

      // Exactly one active view.
      await expect(page.locator('.view.active')).toHaveCount(1);

      // A visible heading in the active view.
      await expect(page.locator('.view.active :is(h1, h2, h3)').first()).toBeVisible();

      // Per-device responsive guarantee: no horizontal overflow.
      const { scrollWidth, innerWidth } = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
      }));
      expect(scrollWidth, `${pathForTab(id)} overflows: scrollWidth ${scrollWidth} > innerWidth ${innerWidth}`)
        .toBeLessThanOrEqual(innerWidth + 2);
    });
  }
});

test('nav tabs switch the active view and update the URL path', async ({ page }) => {
  await gotoTab(page, 'home');
  for (const id of TAB_IDS) {
    await activateTab(page, id);
    await expect(page.locator('.view.active')).toHaveAttribute('id', `view-${id}`);
    await expect(page).toHaveURL(urlForTab(id));
  }
});

test('every link is valid (internal targets exist, external links are safe)', async ({ page }) => {
  // Every route the nav can reach, as a set of concrete paths.
  const knownPaths = new Set(TAB_IDS.map(pathForTab));

  for (const id of TAB_IDS) {
    await gotoTab(page, id);
    const anchors = page.locator('a[href]');
    const count = await anchors.count();
    expect(count, `${pathForTab(id)} should have links`).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const a = anchors.nth(i);
      const href = (await a.getAttribute('href')) ?? '';

      // No dead hrefs.
      expect(href, `${pathForTab(id)} link ${i} has an empty href`).not.toBe('');
      expect(href, `${pathForTab(id)} link ${i} has a bare "#" href`).not.toBe('#');

      if (/^https?:\/\//.test(href)) {
        // External: valid absolute URL, opens in a new tab, safe rel.
        expect(() => new URL(href), `invalid external URL: ${href}`).not.toThrow();
        await expect(a, `external link ${href} must open in a new tab`).toHaveAttribute('target', '_blank');
        const rel = (await a.getAttribute('rel')) ?? '';
        expect(rel, `external link ${href} must have rel*=noopener`).toContain('noopener');
      } else if (href.startsWith('/')) {
        // Internal route path (Next <Link>): '/', '/parity', '/lib/express', …
        const clean = href.replace(/\/+$/, '') || '/';
        expect(
          knownPaths.has(clean) || knownPaths.has(href),
          `${pathForTab(id)}: internal path "${href}" maps to no known route`,
        ).toBeTruthy();
      } else if (href.startsWith('#')) {
        // Either a nav tab link (`#<id>`) or an in-page jump anchor (`#section`).
        const target = href.slice(1);
        const isTab = TAB_IDS.includes(target);
        const targetExists = (await page.locator(`[id="${target}"]`).count()) > 0;
        expect(isTab || targetExists, `${pathForTab(id)}: internal link "${href}" maps to nothing`).toBeTruthy();
      } else {
        throw new Error(`${pathForTab(id)}: unexpected non-path, non-hash, non-http href "${href}"`);
      }
    }
  }
});

test('internal navigation links actually navigate to their view', async ({ page }) => {
  await gotoTab(page, 'home');
  // The home CTA "Get started" is a Next <Link href="/howto">.
  await page.locator('.view.active a[href="/howto"]').first().click();
  await expect(page.locator('.view.active')).toHaveAttribute('id', 'view-howto');
  await expect(page).toHaveURL(urlForTab('howto'));
});
