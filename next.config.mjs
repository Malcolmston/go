import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const isPages = process.env.GITHUB_PAGES === 'true';

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
  // GitHub Pages is a static, API-less project site under /go; Vercel serves
  // the full Next app (with API routes) at the domain root.
  ...(isPages ? { output: 'export', basePath: '/go', images: { unoptimized: true } } : {}),
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
