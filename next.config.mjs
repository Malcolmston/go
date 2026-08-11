import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// public/ holds generated, browser-fetched data bundles (public/docs/<lib>.json,
// search-index.json, graph.json, parity.json). Next serves everything under
// public/ with `cache-control: public, max-age=0`, so a repeat visit re-downloads
// the whole payload — /docs/algebra.json alone is ~1.05 MB gzipped. These URLs
// carry no content hash, so `immutable` is wrong; a short browser max-age plus a
// long CDN s-maxage makes repeat views free while keeping a deploy visible within
// minutes.
export const staticDataCacheControl =
  'public, max-age=300, s-maxage=86400, stale-while-revalidate=604800';

// Every file under public/docs is a JSON doc bundle, hence the catch-all.
export const staticDataSources = [
  '/docs/:path*',
  '/search-index.json',
  '/graph.json',
  '/parity.json',
];

/**
 * headers() entries for the generated data bundles.
 *
 * This used to return [] when GITHUB_PAGES=true, because `output: 'export'` has no
 * server to attach headers to and Next errors if headers() is set alongside it.
 * The Pages deployment is gone, so there is only one target now and the env
 * parameter is kept purely so the unit test can call this directly.
 * @param {Record<string, string | undefined>} _env
 */
export function staticDataHeaders(_env = process.env) {
  return staticDataSources.map((source) => ({
    source,
    headers: [{ key: 'Cache-Control', value: staticDataCacheControl }],
  }));
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Type-check and lint as part of the (Vercel) production build: a type error
  // or an ESLint error fails the deploy. ESLint lints the app + shared source
  // dirs; warnings (e.g. no-explicit-any) don't fail the build, errors do.
  typescript: { ignoreBuildErrors: false },
  eslint: { ignoreDuringBuilds: false, dirs: ['app', 'src'] },
  // The shared go-ui library is imported from ./ui/src (outside the app dir).
  experimental: { externalDir: true },
  // The Next app lives at the repo root but the /api route handlers import
  // ./api/_lib and read ./api/_data. Anchor the file tracer at the repo root
  // and force-include the JSON data the /api routes read via fs (dynamic reads
  // the tracer can't infer).
  outputFileTracingRoot: __dirname,
  outputFileTracingIncludes: {
    '/api/**': ['api/_data/**', 'api/_lib/**'],
  },
  // Vercel serves the full Next app, API routes included, at the domain root.
  // There is no second, static target any more: the GitHub Pages export (which
  // forced `output: 'export'`, a '/go' basePath and unoptimised images) has been
  // removed, so nothing here is conditional on the deploy target.
  async headers() {
    return staticDataHeaders();
  },
  webpack: (config) => {
    config.resolve.alias['go-ui'] = path.resolve(__dirname, 'ui/src/index.ts');
    // go-ui's source (ui/src) has no node_modules of its own; make webpack
    // resolve its bare imports (react, etc.) from THIS app's node_modules so
    // there is a single React copy. We deliberately do NOT hard-alias
    // `react`/`react-dom`: now that the shell (ClientRoot) server-renders, a
    // static `react` alias points Next's server/RSC build at the client React
    // build, whose hooks read a null dispatcher during prerender (usePathname →
    // `useContext` of null). Prepending the root node_modules to `modules` is
    // enough to dedupe go-ui's React while letting Next choose the correct React
    // per build layer (server vs client).
    config.resolve.modules = [path.resolve(__dirname, 'node_modules'), 'node_modules'];
    return config;
  },
};
export default nextConfig;
