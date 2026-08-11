'use client';
import { Releases } from '../../src/components/Releases';

// Client half of /releases. The route is a server component (so it can export
// `metadata`); the release list fetches from GitHub in the browser, so it stays
// behind this 'use client' boundary.
export default function ReleasesClient() {
  return <Releases />;
}
