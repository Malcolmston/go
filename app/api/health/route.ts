// app/api/health/route.ts — Next.js Route Handler port of api/health.js.
//
// Health probe for the go aggregator, reusing the shared ESM lib in
// /workspace/go/api/_lib:
//   GET /api/health      -> { ok: true, es: <boolean> }
//   OPTIONS preflight    -> 204
//
// The frontend hits this to detect whether the API is reachable (and whether
// Upstash Search is configured) before deciding to use the live API or the
// bundled fallback data. `es` mirrors esEnabled() from es.ts: true iff the
// Upstash Search env vars are configured. CORS is permissive (Allow-Origin: *).

import { esEnabled } from '../../../api/_lib/es';
import { getGraph, getSymbols } from '../../../api/_lib/data';

// Run on the Node.js runtime (the shared lib reads process.env), and never
// pre-render / cache — every request reports live configuration state.
export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  'Access-Control-Max-Age': '86400',
};

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function GET(): Promise<Response> {
  // `ok` and `es` are the stable contract the frontend probe (src/api/graph.ts
  // hasApi()) depends on. The corpus counters below are additive diagnostics —
  // they make it possible to tell "functions are up but the data files did not
  // ship" apart from a healthy deploy, which the boolean alone cannot.
  //
  // Every read is defensive: a health probe must answer even when the data it
  // reports on is broken, and it must never echo an internal error (which would
  // disclose the deployment's filesystem layout).
  let symbols = 0;
  let packages = 0;
  let libraries = 0;
  let generatedAt = '';
  let dataOk = true;
  try {
    symbols = getSymbols().length;
    const graph = getGraph();
    packages = graph.packages.length;
    libraries = graph.libraries.length;
    generatedAt = graph.generatedAt;
  } catch (err) {
    console.error('[health] failed to read bundled data', err);
    dataOk = false;
  }

  return Response.json(
    {
      ok: true,
      es: esEnabled(),
      data: { ok: dataOk && symbols > 0 && packages > 0, symbols, packages, libraries, generatedAt },
    },
    {
      headers: {
        ...CORS_HEADERS,
        'Cache-Control': 'no-store',
      },
    }
  );
}
