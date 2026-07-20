'use client';
import type { ReactNode } from 'react';
import dynamic from 'next/dynamic';
import '../ui/src/styles.css';

// The aggregator is a client SPA; render the entire interactive shell (nav +
// page) client-only so no browser-only code runs during static prerender/export.
const ClientRoot = dynamic(() => import('./ClientRoot'), { ssr: false });

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <head>
        <title>malcolmston/go</title>
        <meta name="description" content="The Node.js ecosystem, reimagined in Go." />
      </head>
      <body>
        <ClientRoot>{children}</ClientRoot>
      </body>
    </html>
  );
}
