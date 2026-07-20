'use client';
import { useRouter } from 'next/navigation';
import { Home } from '../src/components/Home';
import { pathForTab } from './nav';

export default function Page() {
  const router = useRouter();
  // Home navigates via a `go(id)` callback; route to the matching Next path.
  return <Home go={(id) => router.push(pathForTab(id))} />;
}
