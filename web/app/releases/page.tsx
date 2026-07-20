'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the Releases view client-only so it
// works both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code (ReleaseList fetches the live GitHub Releases API).
const Releases = dynamic(() => import('../../src/components/Releases').then((m) => ({ default: m.Releases })), { ssr: false });

export default function Page() {
  return <Releases />;
}
