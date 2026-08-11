import type { Page } from '@playwright/test';

// Wait until React has hydrated the app shell.
//
// This used to be implicit: the site rendered client-only, so the nav and the
// active view simply did not exist in the HTML, and waiting for
// `.view.active#view-<id>` to be visible was the same thing as waiting for
// hydration. The App Router pages now render on the server, so the markup —
// nav links included — is in the first response. A click dispatched before
// hydration hits an anchor whose React onClick is not attached yet, so the
// browser follows the literal `href="#<id>"`, the URL becomes `/#<id>`, and no
// routing happens. That is a test race, not a product bug: the same click after
// hydration routes correctly.
//
// react-dom stamps its internal props/fiber keys onto a host element when it
// hydrates that element, so the presence of one on a nav tab link is a direct
// signal that this element's event handlers are live.
export async function waitForHydration(page: Page): Promise<void> {
  await page.waitForFunction(
    () => {
      const el = document.querySelector('nav.tabs a.tab');
      if (!el) return false;
      return Object.keys(el).some(
        (k) => k.startsWith('__reactProps$') || k.startsWith('__reactFiber$'),
      );
    },
    undefined,
    { timeout: 20_000 },
  );
}
