'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the Parity view client-only so it
// works both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code.
const Parity = dynamic(() => import('../../src/components/Parity').then((m) => ({ default: m.Parity })), { ssr: false });

export default function Page() {
  return <Parity />;
}
