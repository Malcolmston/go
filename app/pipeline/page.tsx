// /pipeline used to be its own tab. The pipeline is now part of /parity — the
// method on the index, and the actual stages of a run on each library's record —
// so this route only exists to keep old links working. The redirect lives in
// PipelineClient (it needs hooks); this file stays a server component purely so
// it can declare metadata, which a 'use client' module cannot export.
//
// The metadata deliberately disagrees with the route it is served from:
//   - the canonical and og:url name /parity, the destination, so any equity an
//     old inbound link carries lands on the page that replaced this one;
//   - robots is noindex (follow), so the 30-word stub can never compete with
//     /parity in the index — without it the stub duplicated the home page's
//     <title> exactly, the strongest duplicate-content signal a small site can
//     emit. For the same reason /pipeline is no longer listed in app/sitemap.ts:
//     a noindex URL must not be advertised in a sitemap.
import type { Metadata } from 'next';
import PipelineClient from './PipelineClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = {
  ...pageMetadata({
    title: 'The pipeline moved',
    description:
      'The parity pipeline is now shown on the Parity page, next to the measured scores it produces.',
    path: '/parity',
  }),
  robots: { index: false, follow: true },
};

export default function Page() {
  return <PipelineClient />;
}
