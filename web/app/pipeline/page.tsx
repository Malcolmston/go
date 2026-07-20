'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the Pipeline view client-only so it
// works both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code (the interactive parity-pipeline diagram uses the DOM).
const Pipeline = dynamic(() => import('../../src/components/Pipeline').then((m) => ({ default: m.Pipeline })), { ssr: false });

export default function Page() {
  return <Pipeline />;
}
