import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import ClientRoot from './ClientRoot';
import { SITE_DESCRIPTION, SITE_NAME, SITE_URL } from './seo';
import '@awesome.me/kit-61a008692d/icons/css/all.min.css';
import '../ui/src/styles.css';

// Page metadata is declared through the App Router's metadata export rather
// than hand-written <title>/<meta> tags so Next owns de-duplication and each
// route can override it (see app/lib/[id]/page.tsx's generateMetadata).
//
// metadataBase is what makes every relative metadata URL — canonical, og:url,
// and the og:image the opengraph-image file convention emits — resolve to an
// absolute URL, which the Open Graph spec requires and scrapers will not guess.
//
// openGraph/twitter here are the site-wide fallback, used verbatim by any route
// that declares neither (including '/', which has no metadata of its own). Note
// that Next merges metadata shallowly per top-level key, so a route declaring
// `openGraph` replaces this whole object rather than adding to it; routes go
// through pageMetadata() in app/seo.ts, which restates the shared fields.
//
// No `alternates.canonical` at this level on purpose: a canonical is inherited
// literally, so declaring '/' here would tell crawlers that every page without
// its own canonical IS the home page. Each route supplies its own instead.
export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: SITE_NAME,
    template: `%s · ${SITE_NAME}`,
  },
  description: SITE_DESCRIPTION,
  applicationName: SITE_NAME,
  openGraph: {
    type: 'website',
    siteName: SITE_NAME,
    locale: 'en_US',
    url: '/',
    title: SITE_NAME,
    description: SITE_DESCRIPTION,
  },
  twitter: {
    card: 'summary_large_image',
    title: SITE_NAME,
    description: SITE_DESCRIPTION,
  },
};

// Pre-hydration theme script: apply the stored light/dark choice to <html>
// before first paint so server-rendered markup doesn't flash the wrong theme.
// Kept inline (and tiny) so it runs ahead of hydration; mirrors applyStoredTheme.
const THEME_BOOTSTRAP = `try{var t=localStorage.getItem('mgo-theme')||'dark';document.documentElement.setAttribute('data-theme',t);}catch(e){document.documentElement.setAttribute('data-theme','dark');}`;

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    // suppressHydrationWarning: THEME_BOOTSTRAP rewrites data-theme on <html>
    // before React hydrates, so for anyone who chose the light theme the
    // attribute legitimately differs from the server-rendered "dark". Without
    // this, React logs a hydration mismatch on every light-theme page load.
    <html lang="en" data-theme="dark" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_BOOTSTRAP }} />
      </head>
      <body>
        <ClientRoot>{children}</ClientRoot>
      </body>
    </html>
  );
}
