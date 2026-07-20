'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the Explore view client-only so it
// works both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code (pointer/wheel handlers, the fallback graph fetch).
const Explore = dynamic(() => import('../../src/components/Explore').then((m) => ({ default: m.Explore })), { ssr: false });

export default function Page() {
  return <Explore />;
}
