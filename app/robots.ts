// /robots.txt — crawl directives plus a pointer at the sitemap.
//
// The App Router's `robots` file convention, so this is rendered as a static
// file at build time and works both on Vercel (/robots.txt) and in the
// `output: 'export'` GitHub Pages build (out/robots.txt).
//
// Everything user-facing is crawlable; only the JSON/streaming endpoints under
// /api/ are disallowed (they are machine surfaces with no indexable content, and
// /api/chat is a paid LLM call — no reason to let a crawler spend it). The
// sitemap and host are absolute so both deployments name the same canonical
// origin rather than competing as duplicate content.
import type { MetadataRoute } from 'next';
import { siteUrl } from './sitemap';

export default function robots(): MetadataRoute.Robots {
  const base = siteUrl();
  return {
    rules: [
      {
        userAgent: '*',
        allow: '/',
        disallow: ['/api/'],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
    host: base,
  };
}
