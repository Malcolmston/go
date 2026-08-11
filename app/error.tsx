'use client';
import Link from 'next/link';

// The App Router segment error boundary. Next renders this in place of the
// failing route while KEEPING the shared shell (app/layout.tsx + ClientRoot),
// so — exactly like app/not-found.tsx — it must not render a <main> of its own
// (the shell already owns that landmark) and it uses the same `.view` section
// wrapper so heading order and styling match every other route.
//
// It must be a client component: `reset` is a callback and the boundary needs
// to re-render on the client to retry.
//
// Only `error.digest` is shown. `error.message` is deliberately never rendered:
// in production Next replaces it with a generic string anyway, and in
// development it can carry internal paths or query fragments. The digest is the
// stable handle that also appears in the server log, so it is the useful thing
// to quote in a bug report.
export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <section className="view active" id="view-error">
      <h1>Something went wrong</h1>
      <p className="muted">
        This page failed to render. The fault is on our side, not yours — nothing you did
        caused it. Retrying often works, because most failures here are a transient problem
        talking to the data layer.
      </p>
      <p>
        <button className="btn primary" type="button" onClick={() => reset()}>
          Try again
        </button>
      </p>
      {error.digest ? (
        <p className="small dim">
          Reference for a bug report: <code className="mono">{error.digest}</code>
        </p>
      ) : null}
      <ul className="clean">
        <li><Link href="/">Home — the library grid</Link></li>
        <li><Link href="/explore">Explore — search every exported symbol</Link></li>
        <li>
          <a href="https://github.com/malcolmston" target="_blank" rel="noopener">
            Report this on GitHub
          </a>
        </li>
      </ul>
    </section>
  );
}
