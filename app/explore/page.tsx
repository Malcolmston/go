// /explore — search across every library and exported symbol.
import type { Metadata } from 'next';
import ExploreClient from './ExploreClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'Explore every library and symbol',
  description:
    'Search the whole suite at once: filter the Go ports by the Node.js package they replace, and look up any exported symbol to see which module provides it.',
  path: '/explore',
});

export default function Page() {
  return <ExploreClient />;
}
