'use client';
import { HowTo } from '../../src/components/HowTo';

// Client half of /howto. The route is a server component (so it can export
// `metadata`); this file carries the 'use client' boundary that the view needs.
export default function HowtoClient() {
  return <HowTo />;
}
