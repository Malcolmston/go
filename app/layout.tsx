import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import ClientRoot from './ClientRoot';
import '@awesome.me/kit-61a008692d/icons/css/all.min.css';
import '../ui/src/styles.css';

// Page metadata is declared through the App Router's metadata export rather
// than hand-written <title>/<meta> tags so Next owns de-duplication and each
// route can override it (see app/lib/[id]/page.tsx's generateMetadata).
export const metadata: Metadata = {
  title: {
    default: 'malcolmston/go',
    template: '%s · malcolmston/go',
  },
  description: 'The Node.js ecosystem, reimagined in Go.',
  applicationName: 'malcolmston/go',
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
