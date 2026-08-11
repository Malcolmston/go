'use client';
import type { CSSProperties } from 'react';

// The root error boundary. Unlike app/error.tsx this one catches failures in
// app/layout.tsx and ClientRoot themselves, which means it REPLACES the root
// layout: it has to render its own <html> and <body>, and the app stylesheet
// (imported by that layout) is not guaranteed to have loaded. So everything
// here is inline-styled and dependency-free — no next/link, no go-ui, no
// imports beyond React's JSX runtime — because anything it pulled in could be
// the very thing that just failed.
//
// Colours are hard-coded from the dark theme in ui/src/styles.css rather than
// read from CSS variables, which would resolve to nothing without that sheet.
const BG = '#08080a';
const FG = '#f5f5f7';
const DIM = '#86868b';
const ACCENT = '#0071e3';

const page: CSSProperties = {
  margin: 0,
  minHeight: '100vh',
  background: BG,
  color: FG,
  font: '16px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  padding: '2rem',
};

const button: CSSProperties = {
  display: 'inline-block',
  padding: '.8rem 1.6rem',
  borderRadius: 999,
  border: 'none',
  background: ACCENT,
  color: '#fff',
  font: 'inherit',
  fontWeight: 500,
  cursor: 'pointer',
};

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="en">
      <body style={page}>
        <div style={{ maxWidth: '34rem' }}>
          <p style={{ margin: '0 0 .5rem', color: DIM, fontSize: '.92rem', letterSpacing: '.08em', textTransform: 'uppercase' }}>
            malcolmston/go
          </p>
          <h1 style={{ margin: '0 0 1rem', fontSize: '2rem', lineHeight: 1.2 }}>
            Something went wrong
          </h1>
          <p style={{ margin: '0 0 1.5rem', color: DIM }}>
            The site shell itself failed to load, so this stripped-back page is all we can
            show. Reloading usually clears it.
          </p>
          <p style={{ margin: '0 0 1.5rem' }}>
            <button type="button" style={button} onClick={() => reset()}>
              Try again
            </button>
          </p>
          {error.digest ? (
            <p style={{ margin: '0 0 1.5rem', color: DIM, fontSize: '.92rem' }}>
              Reference for a bug report:{' '}
              <code style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>
                {error.digest}
              </code>
            </p>
          ) : null}
          <p style={{ margin: 0 }}>
            {/* A plain <a>, not next/link, on purpose: the root layout has
                crashed, so a client-side navigation would re-enter the broken
                shell. A full document load is the recovery path. */}
            {/* eslint-disable-next-line @next/next/no-html-link-for-pages */}
            <a href="/" style={{ color: ACCENT }}>Back to the library grid</a>
          </p>
        </div>
      </body>
    </html>
  );
}
