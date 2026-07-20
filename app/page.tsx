'use client';
import dynamic from 'next/dynamic';
import { useRouter } from 'next/navigation';
import { pathForTab } from './nav';

// The aggregator is a client SPA; render the Home view client-only so it works
// both as a static export (GitHub Pages) and on Vercel without SSR-ing
// browser-only code.
const Home = dynamic(() => import('../src/components/Home').then((m) => ({ default: m.Home })), { ssr: false });

export default function Page() {
  const router = useRouter();
  // Home navigates via a `go(id)` callback; route to the matching Next path.
  return <Home go={(id) => router.push(pathForTab(id))} />;
}
