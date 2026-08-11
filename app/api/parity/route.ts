// app/api/parity/route.ts — the measured parity summary, live from the corpus.
//
//   GET /api/parity              -> { generatedAt, libraries: { <id>: entry } }
//   GET /api/parity?lib=express  -> { generatedAt, library: "express", ...entry }
//   GET /api/parity?lib=nope     -> 404 { error: "unknown library" }
//   OPTIONS preflight            -> 204
//
// entry = { parityPercent, total, match, mismatch, deviations,
//           upstream: { raw, name, version } }
//
// This is the same projection scripts/build-graph-data.ts writes to
// public/parity.json (both call summarizeParity), so a host that serves the
// static file and a host that serves this endpoint answer identically. The full
// per-case corpus is deliberately NOT exposed here: it is ~5.4 MB, the frontend
// rejects any parity payload over 512 KB, and the per-case detail already has a
// dedicated page at /parity/[lib].
//
// CORS is permissive (Access-Control-Allow-Origin: *).

import { getParitySummary } from '../../../api/_lib/data';

// Run on the Node.js runtime (the shared lib reads the bundled data files off
// disk), and never pre-render — the corpus is read from the deployed bundle.
export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
  'Access-Control-Max-Age': '86400',
};

// The data file is a build artefact that only changes on deploy, so it is safe
// to let the CDN hold the answer briefly while still revalidating.
const CACHE_HEADERS: Record<string, string> = {
  'Cache-Control': 'public, max-age=0, s-maxage=300, stale-while-revalidate=86400',
};

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function GET(req: Request): Promise<Response> {
  let lib = '';
  try {
    lib = (new URL(req.url).searchParams.get('lib') ?? '').trim().toLowerCase();
  } catch {
    lib = '';
  }

  let summary;
  try {
    summary = getParitySummary();
  } catch (err) {
    // A missing/corrupt api/_data/parity.json is a deploy problem, not a client
    // error. Report it as such without echoing the filesystem layout.
    console.error('[parity] failed to read the measured parity corpus', err);
    return Response.json(
      { error: 'parity data unavailable' },
      { status: 503, headers: { ...CORS_HEADERS, 'Cache-Control': 'no-store' } }
    );
  }

  if (lib !== '') {
    if (!Object.prototype.hasOwnProperty.call(summary.libraries, lib)) {
      return Response.json(
        { error: 'unknown library' },
        { status: 404, headers: { ...CORS_HEADERS, 'Cache-Control': 'no-store' } }
      );
    }
    return Response.json(
      { generatedAt: summary.generatedAt, library: lib, ...summary.libraries[lib] },
      { headers: { ...CORS_HEADERS, ...CACHE_HEADERS } }
    );
  }

  return Response.json(summary, { headers: { ...CORS_HEADERS, ...CACHE_HEADERS } });
}
