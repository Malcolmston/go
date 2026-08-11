'use client';
// /pipeline used to be its own tab. The pipeline is now part of /parity — the
// method on the index, and the actual stages of a run on each library's record —
// so this route only exists to keep old links working.
//
// The redirect is done client-side with router.replace() rather than with
// next/navigation's server redirect() or a next.config rewrite: this app is also
// built with output:'export' for GitHub Pages, where neither of those runs, and
// router.replace() correctly applies the /go basePath there.
import { useEffect } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';

export default function Page() {
  const router = useRouter();
  useEffect(() => {
    router.replace('/parity');
  }, [router]);

  return (
    <section className="view active" id="view-pipeline">
      <h2>The pipeline moved</h2>
      <p className="muted">
        The parity pipeline is now shown on the <Link href="/parity">Parity</Link> page, next to the
        scores it produces. Taking you there now.
      </p>
    </section>
  );
}
