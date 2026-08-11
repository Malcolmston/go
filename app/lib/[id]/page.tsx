// Dynamic per-library route: /lib/<id> (e.g. /lib/express, /lib/socketio).
//
// This is a SERVER component so it can export generateStaticParams(), which the
// static export (output:'export' on GitHub Pages) needs to know every library
// page to emit. It resolves the route id and hands it to a small client child;
// the actual view (LibView, which pulls in browser-only doc rendering) is loaded
// client-only there, matching the aggregator's client-SPA model.
import type { Metadata } from 'next';
import { permanentRedirect, notFound } from 'next/navigation';
import { LIBS } from '../../../src/data';
import { notFoundMetadata } from '../../notFoundMetadata';
import { pageMetadata } from '../../seo';
import { aliasFor } from './alias';
import LibClient from './LibClient';

// Emit one static page per library so `next build` with output:'export' writes
// every /lib/<id>.html. Covers both plain ids (express) and dotted ones
// (socket.io) — Next handles the segment encoding.
export function generateStaticParams() {
  return LIBS.map((lib) => ({ id: lib.id }));
}

// decodeURIComponent throws on a malformed escape (e.g. /lib/%E0). A bad URL
// must render the "library not found" body, never throw out of the route.
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

// Per-library <title>/<meta description>, so a shared /lib/<id> link previews as
// that library rather than as the generic site title. An id that resolves to
// nothing is a 404 (see the page below), and gets noindex metadata that does not
// inherit the root layout's og:url — otherwise a typo'd link previews as the
// home page.
export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  const decoded = safeDecode(id);
  const lib = LIBS.find((l) => l.id === decoded);
  if (!lib) {
    return notFoundMetadata({
      title: 'Library not found',
      // Not the requested id: this string is what a crawler or a social card
      // quotes, and reflecting arbitrary URL text into it invites nonsense.
      description:
        'There is no library by that name in the go aggregator. Explore every Go port from the library index.',
    });
  }
  // Same strings drive <title>, the description, the Open Graph/Twitter card and
  // the canonical, so a shared /lib/<id> link never previews as the generic site.
  return pageMetadata({
    title: `${lib.name} for Go`,
    description: `${lib.tagline} ${lib.pkg} — an independent Go port of ${lib.node}.`,
    path: `/lib/${lib.id}`,
    // This segment has its own opengraph-image.tsx (a per-library card); leave
    // og:image to the file convention rather than pointing at the site card.
    image: null,
  });
}

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const decoded = safeDecode(id);
  const alias = aliasFor(decoded);
  // /lib/socket.io -> 308 /lib/socketio, so the dotted spelling people write
  // lands on the real page instead of a dead end.
  if (alias) permanentRedirect(`/lib/${encodeURIComponent(alias)}`);
  // Anything else that does not resolve is a genuine 404. Returning 200 with a
  // "not found" body is a soft 404: Google indexes it, and every typo'd inbound
  // link becomes a thin page. LibClient keeps its own fallback body for the
  // static export, where an unknown id can only be served by the host's 404.
  if (!LIBS.some((l) => l.id === decoded)) notFound();
  return <LibClient id={decoded} />;
}
