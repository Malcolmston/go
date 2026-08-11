'use client';
import { AI } from '../../src/components/AI';

// Client half of /ai. The route is a server component (so it can export
// `metadata`); this file carries the 'use client' boundary that the view needs.
export default function AiClient() {
  return <AI />;
}
