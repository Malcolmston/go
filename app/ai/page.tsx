'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the AI view client-only so it works
// both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code.
const AI = dynamic(() => import('../../src/components/AI').then((m) => ({ default: m.AI })), { ssr: false });

export default function Page() {
  return <AI />;
}
