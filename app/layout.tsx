import type { ReactNode } from 'react';
import ClientRoot from './ClientRoot';
import '@fortawesome/fontawesome-free/css/all.min.css';
import '../ui/src/styles.css';

// Pre-hydration theme script: apply the stored light/dark choice to <html>
// before first paint so server-rendered markup doesn't flash the wrong theme.
// Kept inline (and tiny) so it runs ahead of hydration; mirrors applyStoredTheme.
const THEME_BOOTSTRAP = `try{var t=localStorage.getItem('mgo-theme')||'dark';document.documentElement.setAttribute('data-theme',t);}catch(e){document.documentElement.setAttribute('data-theme','dark');}`;

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" data-theme="dark">
      <head>
        <title>malcolmston/go</title>
        <meta name="description" content="The Node.js ecosystem, reimagined in Go." />
        <script dangerouslySetInnerHTML={{ __html: THEME_BOOTSTRAP }} />
      </head>
      <body>
        <ClientRoot>{children}</ClientRoot>
      </body>
    </html>
  );
}
