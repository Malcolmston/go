// /sitemap.xml — the machine-readable index of every indexable URL.
//
// This is the App Router's `sitemap` file convention, so Next renders it as a
// static file at build time: on Vercel it is served at /sitemap.xml, and under
// `output: 'export'` (the GitHub Pages build) it is written to out/sitemap.xml.
// Every URL is emitted ABSOLUTE against NEXT_PUBLIC_SITE_URL so the canonical
// host is unambiguous and the Pages basePath (/go) never leaks into the entries
// — the two deployments would otherwise compete as duplicate content.
//
// The route list is derived, not hand-written: the fixed sections come from the
// directories under app/, the library pages from LIBS (src/data.ts) and the
// per-harness parity pages from the generated dataset (app/parity/load.ts), so
// adding a library or a harness updates the sitemap with no edit here.
import type { MetadataRoute } from 'next';
import { LIBS } from '../src/data';
import { loadParityData, paritySlugs } from './parity/load';
import { SITE_URL } from './seo';

/**
 * The canonical origin, without a trailing slash.
 *
 * The origin itself comes from app/seo.ts (SITE_URL), which is what the routes'
 * metadataBase and canonicals already resolve against — one source of truth, so
 * a sitemap entry can never disagree with the canonical of the page it points
 * at. NEXT_PUBLIC_BASE_PATH is deliberately not involved: that is a path prefix
 * for the /go Pages export, and these URLs must name a host.
 */
export function siteUrl(): string {
  const raw = process.env.NEXT_PUBLIC_SITE_URL?.trim();
  return (raw && raw.length > 0 ? raw : SITE_URL).replace(/\/+$/, '');
}

// The fixed, non-dynamic routes, most important first. '' is the home page.
// /pipeline is deliberately absent: it still resolves, but it is a redirect stub
// to /parity that declares robots noindex and canonicalises at /parity, and a
// sitemap must only advertise URLs that are meant to be indexed.
type Freq = NonNullable<MetadataRoute.Sitemap[number]['changeFrequency']>;

const STATIC_PATHS: { path: string; priority: number; changeFrequency: Freq }[] = [
  { path: '', priority: 1, changeFrequency: 'weekly' },
  { path: '/parity', priority: 0.9, changeFrequency: 'daily' },
  { path: '/security', priority: 0.9, changeFrequency: 'daily' },
  { path: '/explore', priority: 0.7, changeFrequency: 'weekly' },
  { path: '/releases', priority: 0.7, changeFrequency: 'weekly' },
  { path: '/howto', priority: 0.6, changeFrequency: 'monthly' },
  { path: '/faq', priority: 0.6, changeFrequency: 'monthly' },
  { path: '/ai', priority: 0.5, changeFrequency: 'monthly' },
  { path: '/ask', priority: 0.5, changeFrequency: 'monthly' },
  { path: '/about', priority: 0.5, changeFrequency: 'monthly' },
];

/**
 * Every sitemap entry. Exported (and pure) so a unit test can assert the shape
 * without going through a build; `sitemap()` below is what Next calls.
 */
export function entries(): MetadataRoute.Sitemap {
  const base = siteUrl();
  // The dataset's generatedAt is the closest thing the site has to a real
  // content timestamp: the measured numbers are what change between deploys.
  const stamp = loadParityData().generatedAt;
  const lastModified = stamp && !Number.isNaN(Date.parse(stamp)) ? new Date(stamp) : new Date();

  const statics = STATIC_PATHS.map(({ path, priority, changeFrequency }) => ({
    url: `${base}${path}`,
    lastModified,
    changeFrequency,
    priority,
  }));

  const libs = LIBS.map((lib) => ({
    url: `${base}/lib/${encodeURIComponent(lib.id)}`,
    lastModified,
    changeFrequency: 'weekly' as Freq,
    priority: 0.8,
  }));

  const parity = paritySlugs().map((slug) => ({
    url: `${base}/parity/${encodeURIComponent(slug)}`,
    lastModified,
    changeFrequency: 'daily' as Freq,
    priority: 0.6,
  }));

  return [...statics, ...libs, ...parity];
}

export default function sitemap(): MetadataRoute.Sitemap {
  return entries();
}
