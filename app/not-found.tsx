import type { Metadata } from 'next';
import Link from 'next/link';
import { notFoundMetadata } from './notFoundMetadata';

// noindex, and openGraph restated rather than inherited: see app/notFoundMetadata.
export const metadata: Metadata = notFoundMetadata({
  title: 'Page not found',
  description:
    'That page does not exist on malcolmston/go. Start from the library grid, or search every exported symbol from Explore.',
});

// The App Router 404. It renders *inside* the shared shell (app/layout.tsx),
// which already provides the <main> landmark — so this must not render a <main>
// of its own (nested landmarks). It uses the same `.view` section wrapper as
// every other route so heading order and styling stay consistent.
export default function NotFound() {
  return (
    <section className="view active" id="view-not-found">
      <h1>Page not found</h1>
      <p className="muted">
        That page doesn&apos;t exist. It may have moved, or the link may be out of date.
      </p>
      <ul className="clean">
        <li><Link href="/">Home — the library grid</Link></li>
        <li><Link href="/explore">Explore — search every exported symbol</Link></li>
        <li><Link href="/howto">How-to — getting started</Link></li>
        <li><Link href="/faq">FAQ</Link></li>
      </ul>
    </section>
  );
}
