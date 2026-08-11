'use client';
import { useRouter } from 'next/navigation';
import { Home } from '../src/components/Home';
import { pathForTab } from './nav';

// Client half of the '/' route. The route itself is a server component so it can
// export `metadata`; everything interactive (the router push behind Home's
// `go(id)` callback) lives here.
export default function HomeClient() {
  const router = useRouter();
  // Home navigates via a `go(id)` callback; route to the matching Next path.
  return <Home go={(id) => router.push(pathForTab(id))} />;
}
