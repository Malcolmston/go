'use client';
import { About } from '../../src/components/About';

// Client half of /about. The route is a server component (so it can export
// `metadata`); this file carries the 'use client' boundary that the view needs.
export default function AboutClient() {
  return <About />;
}
