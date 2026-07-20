'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the HowTo getting-started guide
// client-only so it works both as a static export (GitHub Pages) and on Vercel
// without SSR-ing browser-only code (CodeBlock/highlight, in-page anchors).
const HowTo = dynamic(() => import('../../src/components/HowTo').then((m) => ({ default: m.HowTo })), { ssr: false });

export default function Page() {
  return <HowTo />;
}
