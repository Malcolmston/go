// middleware.ts — request-time routing rules for the library routes.
//
// Now that each library is its own set of routes (/lib/<id> plus /examples,
// /api, /parity), a library can define its own routing behaviour here — the
// request-time equivalent of an Express app mounting its own middleware. This
// runs on Vercel (the app is a full Next server; there is no static export).
//
// It only touches /lib/* (see `config.matcher`); everything else passes through.

import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

// npm / dotted package names that resolve to a canonical library id. The
// socket.io port lives at /lib/socketio, but people type the package name
// "socket.io" — canonicalise it (and keep any sub-path) with a permanent
// redirect. Add an entry whenever a library's id differs from its package name.
const CANONICAL_ID: Record<string, string> = {
  'socket.io': 'socketio',
};

// Sub-path aliases that apply to every library, e.g. /lib/<id>/docs -> /api.
const GLOBAL_ALIASES: Record<string, string> = {
  docs: 'api',
};

// Per-library routing overrides, keyed by canonical id. Each library can add its
// own sub-path aliases (and, over time, richer rules) without affecting others —
// this is the "different routing rules per package" hook.
interface LibRouting {
  aliases?: Record<string, string>;
}
const LIB_ROUTING: Record<string, LibRouting> = {
  // socket.io is the multi-package port; give its docs a couple of shortcuts.
  socketio: { aliases: { reference: 'api', packages: 'api' } },
};

export function middleware(req: NextRequest): NextResponse {
  const m = req.nextUrl.pathname.match(/^\/lib\/([^/]+)(\/.*)?$/);
  if (!m) return NextResponse.next();

  const rawId = decodeURIComponent(m[1]);
  const rest = m[2] ?? ''; // '' or '/<sub>/...'

  // 1) Canonicalise an npm/dotted name to the library id (socket.io -> socketio).
  const canonical = CANONICAL_ID[rawId];
  if (canonical && canonical !== rawId) {
    const url = req.nextUrl.clone();
    url.pathname = `/lib/${canonical}${rest}`;
    return NextResponse.redirect(url, 308);
  }
  const id = canonical ?? rawId;

  // 2) Alias the first sub-segment (per-library rules win over the global ones),
  //    e.g. /lib/<id>/docs -> /lib/<id>/api.
  const segs = rest.replace(/^\//, '').split('/').filter(Boolean);
  if (segs.length) {
    const alias = LIB_ROUTING[id]?.aliases?.[segs[0]] ?? GLOBAL_ALIASES[segs[0]];
    if (alias && alias !== segs[0]) {
      const url = req.nextUrl.clone();
      segs[0] = alias;
      url.pathname = `/lib/${id}/${segs.join('/')}`;
      return NextResponse.redirect(url, 308);
    }
  }

  return NextResponse.next();
}

export const config = {
  // Only run on the library routes.
  matcher: ['/lib/:path*'],
};
