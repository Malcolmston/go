// Dynamic per-library route: /lib/<id> (e.g. /lib/express, /lib/socketio).
//
// This is a SERVER component so it can export generateStaticParams(), which the
// static export (output:'export' on GitHub Pages) needs to know every library
// page to emit. It resolves the route id and hands it to a small client child;
// the actual view (LibView, which pulls in browser-only doc rendering) is loaded
// client-only there, matching the aggregator's client-SPA model.
import type { Metadata } from 'next';
import { LIBS } from '../../../src/data';
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
// that library rather than as the generic site title. Unknown ids get the
// not-found title instead of silently inheriting the default.
export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  const lib = LIBS.find((l) => l.id === safeDecode(id));
  if (!lib) return { title: 'Library not found' };
  return {
    title: `${lib.name} for Go`,
    description: `${lib.tagline} ${lib.pkg} — an independent Go port of ${lib.node}.`,
  };
}

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <LibClient id={safeDecode(id)} />;
}
