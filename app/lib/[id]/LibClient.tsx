'use client';
import Link from 'next/link';
import { LibView } from '../../../src/components/LibView';
import { LIBS } from '../../../src/data';

// Suggest libraries whose id or name overlaps the bad id, so a typo or an old
// slug (e.g. /lib/socket.io, which is now /lib/socketio) still lands the reader
// somewhere useful instead of on a dead end.
function suggestionsFor(id: string) {
  const needle = id.toLowerCase().replace(/[^a-z0-9]/g, '');
  if (!needle) return [];
  return LIBS.filter((l) => {
    const hay = `${l.id}${l.name}`.toLowerCase().replace(/[^a-z0-9]/g, '');
    return hay.includes(needle) || needle.includes(l.id.toLowerCase());
  }).slice(0, 5);
}

// LibClient receives the route id from the server page and looks the library up
// in LIBS. An unknown id (e.g. a stale link) renders a friendly fallback rather
// than crashing the client render. It deliberately does NOT call notFound():
// the GitHub Pages build is a static export where only the pre-rendered
// /lib/<id> pages exist, so this fallback is what an unknown id has to show.
export default function LibClient({ id }: { id: string }) {
  const lib = LIBS.find((l) => l.id === id);

  if (!lib) {
    const near = suggestionsFor(id);
    return (
      <section className="view active" id="view-lib-missing">
        <h2>Library not found</h2>
        <p className="muted">
          There&apos;s no library called <code>{id}</code> in the go aggregator.{' '}
          <Link href="/explore">Explore all libraries</Link>.
        </p>
        {near.length > 0 && (
          <>
            <p className="muted">Did you mean:</p>
            <ul className="clean">
              {near.map((l) => (
                <li key={l.id}>
                  <Link href={`/lib/${l.id}`}>{l.name}</Link> — {l.tagline}
                </li>
              ))}
            </ul>
          </>
        )}
      </section>
    );
  }

  return <LibView lib={lib} key={lib.id} />;
}
