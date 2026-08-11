'use client';
import { Ask } from '../../src/components/Ask';

// Client half of /ask. The route is a server component (so it can export
// `metadata`); Ask itself is interactive (it streams from /api/chat), so it stays
// behind this 'use client' boundary.
export default function AskClient() {
  return <Ask />;
}
