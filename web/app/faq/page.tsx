'use client';
import dynamic from 'next/dynamic';

// The aggregator is a client SPA; render the FAQ view client-only so it works
// both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code.
const Faq = dynamic(() => import('../../src/components/Faq').then((m) => ({ default: m.Faq })), { ssr: false });

export default function Page() {
  return <Faq />;
}
