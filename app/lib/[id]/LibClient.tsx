'use client';
import Link from 'next/link';
import { LibView } from '../../../src/components/LibView';
import { LIBS } from '../../../src/data';

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
