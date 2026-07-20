'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the About view client-only so it works
// both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code.
const About = dynamic(() => import('../../src/components/About').then((m) => ({ default: m.About })), { ssr: false });

export default function Page() {
  return <About />;
}
