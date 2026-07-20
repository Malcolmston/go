// Route <-> tab-id mapping for the Next.js App Router migration.
//
// The old app hash-routed every tab (home, the top-level sections, and each
// library) through a single id. The App Router gives each id a real path:
//   home    -> '/'
//   <top>   -> '/<top>'      (parity, pipeline, explore, releases, howto, faq, ai, about)
//   <libId> -> '/lib/<libId>'  (everything else — the ecosystem libraries)
//
// pathForTab() feeds router.push() from the Layout's onNav; tabForPath()
// derives the active tab from usePathname() so the nav highlights correctly.

// The fixed, non-library sections. Any id that is neither 'home' nor one of
// these is treated as a library and routed under /lib/.
const TOP_LEVEL = new Set([
  'parity',
  'pipeline',
  'explore',
  'releases',
  'howto',
  'faq',
  'ai',
  'about',
]);

/** Map a tab id to its route path. */
export function pathForTab(id: string): string {
  if (id === 'home') return '/';
  if (TOP_LEVEL.has(id)) return `/${id}`;
  return `/lib/${encodeURIComponent(id)}`;
}

/** Map a route pathname (from usePathname, basePath already stripped) to its tab id. */
export function tabForPath(pathname: string | null | undefined): string {
  if (!pathname) return 'home';
  // Normalise a possible trailing slash from the static export.
  const clean = pathname.replace(/\/+$/, '');
  if (clean === '' || clean === '/') return 'home';
  // A library route is /lib/<id> plus optional sub-tab (/examples, /api, …);
  // the top-nav tab is the library id, so keep only the first segment.
  if (clean.startsWith('/lib/')) {
    return decodeURIComponent(clean.slice('/lib/'.length).split('/')[0]);
  }
  const seg = clean.slice(1).split('/')[0];
  if (TOP_LEVEL.has(seg)) return seg;
  return 'home';
}
