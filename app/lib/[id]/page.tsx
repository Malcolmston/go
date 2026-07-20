// Dynamic per-library route: /lib/<id> (e.g. /lib/express, /lib/socket.io).
//
// This is a SERVER component so it can export generateStaticParams(), which the
// static export (output:'export' on GitHub Pages) needs to know every library
// page to emit. It resolves the route id and hands it to a small client child;
// the actual view (LibView, which pulls in browser-only doc rendering) is loaded
// client-only there, matching the aggregator's client-SPA model.
import { LIBS } from '../../../src/data';
import LibClient from './LibClient';

// Emit one static page per library so `next build` with output:'export' writes
// every /lib/<id>.html. Covers both plain ids (express) and dotted ones
// (socket.io) — Next handles the segment encoding.
export function generateStaticParams() {
  return LIBS.map((lib) => ({ id: lib.id }));
}

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <LibClient id={decodeURIComponent(id)} />;
}
