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
    // there is a single React copy — without hard-aliasing React (which breaks
    // Next's server/RSC React handling).
    config.resolve.modules = [path.resolve(__dirname, 'node_modules'), 'node_modules'];
    const R = (m) => path.resolve(__dirname, 'node_modules', m);
    config.resolve.alias['react'] = R('react');
    config.resolve.alias['react-dom'] = R('react-dom');
    config.resolve.alias['react/jsx-runtime'] = R('react/jsx-runtime');
    config.resolve.alias['react/jsx-dev-runtime'] = R('react/jsx-dev-runtime');
    return config;
  },
};
export default nextConfig;
