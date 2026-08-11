// Route <-> tab-id mapping for the Next.js App Router migration.
//
// The old app hash-routed every tab (home, the top-level sections, and each
// library) through a single id. The App Router gives each id a real path:
//   home    -> '/'
//   <top>   -> '/<top>'      (parity, explore, releases, howto, faq, ai, about)
//   <libId> -> '/lib/<libId>'  (everything else — the ecosystem libraries)
//
// pathForTab() feeds router.push() from the Layout's onNav; tabForPath()
// derives the active tab from usePathname() so the nav highlights correctly.

import { LIBS } from '../src/data';

// The fixed, non-library sections, with the human label used for the route
// announcement. Any id that is neither 'home' nor one of these is treated as a
// library and routed under /lib/.
const TOP_LEVEL_LABELS: Record<string, string> = {
  parity: 'Parity',
  security: 'Security',
  explore: 'Explore',
  releases: 'Releases',
  howto: 'How-to',
  faq: 'FAQ',
  ai: 'AI',
  ask: 'Ask',
  about: 'About',
};

const TOP_LEVEL = new Set(Object.keys(TOP_LEVEL_LABELS));

// Retired routes that still resolve, so an old link keeps highlighting a real
// tab. /pipeline was folded into /parity — the pipeline is now shown next to the
// scores it produces — and the route itself redirects there.
const ROUTE_ALIASES: Record<string, string> = {
  pipeline: 'parity',
};

/** True when `id` names one of the fixed, non-library sections. */
export function isTopLevel(id: string): boolean {
  return TOP_LEVEL.has(id);
}

/** Map a tab id to its route path. */
export function pathForTab(id: string): string {
  if (id === 'home') return '/';
  if (TOP_LEVEL.has(id)) return `/${id}`;
  const alias = ROUTE_ALIASES[id];
  if (alias && TOP_LEVEL.has(alias)) return `/${alias}`;
  return `/lib/${encodeURIComponent(id)}`;
}

/** Map a route pathname (from usePathname, basePath already stripped) to its tab id. */
export function tabForPath(pathname: string | null | undefined): string {
  if (!pathname) return 'home';
  // Normalise a possible trailing slash from the static export.
  const clean = pathname.replace(/\/+$/, '');
  if (clean === '' || clean === '/') return 'home';
  if (clean.startsWith('/lib/')) return decodeURIComponent(clean.slice('/lib/'.length));
  const seg = clean.slice(1).split('/')[0];
  if (TOP_LEVEL.has(seg)) return seg;
  // A per-library parity record (/parity/<lib>) and the retired /pipeline route
  // both belong to the Parity tab.
  const alias = ROUTE_ALIASES[seg];
  if (alias && TOP_LEVEL.has(alias)) return alias;
  return 'home';
}

/**
 * A human-readable name for a tab id, used for the route-change announcement
 * and per-library page titles. Unknown ids fall back to the id itself so a
 * stale /lib/<id> link still announces something meaningful.
 */
export function labelForTab(id: string): string {
  if (id === 'home') return 'Home';
  const top = TOP_LEVEL_LABELS[id];
  if (top) return top;
  return LIBS.find((l) => l.id === id)?.name ?? id;
}
