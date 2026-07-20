'use client';
import dynamic from 'next/dynamic';
import Link from 'next/link';
import { LIBS } from '../../../src/data';

// LibView renders a library's hero, sub-tabs and inline API docs. It's loaded
// client-only (ssr:false) because the API sub-tab mounts DocsApp, which fetches
// and touches browser APIs — the same client-SPA pattern every other route uses,
// so it works identically under the static export and on Vercel.
const LibView = dynamic(
  () => import('../../../src/components/LibView').then((m) => ({ default: m.LibView })),
  { ssr: false },
);

// LibClient receives the route id from the server page and looks the library up
// in LIBS. An unknown id (e.g. a stale link) renders a friendly fallback rather
// than crashing the client render.
export default function LibClient({ id }: { id: string }) {
  const lib = LIBS.find((l) => l.id === id);

  if (!lib) {
    return (
      <section className="view active" id="view-lib-missing">
        <h2>Library not found</h2>
        <p className="muted">
          There's no library called <code>{id}</code> in the go aggregator.{' '}
          <Link href="/explore">Explore all libraries</Link>.
        </p>
      </section>
    );
  }

  return <LibView lib={lib} key={lib.id} />;
}
