'use client';
import { Explore } from '../../src/components/Explore';

// Client half of /explore. The route is a server component (so it can export
// `metadata`); Explore is fully interactive (search, filters, keyboard nav), so
// it stays behind this 'use client' boundary.
export default function ExploreClient() {
  return <Explore />;
}
