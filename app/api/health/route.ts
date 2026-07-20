// app/api/health/route.ts — Next.js Route Handler port of api/health.js.
//
// Health probe for the go aggregator, reusing the shared ESM lib in
// /workspace/go/api/_lib:
//   GET /api/health      -> { ok: true, es: <boolean> }
//   OPTIONS preflight    -> 204
//
// The frontend hits this to detect whether the API is reachable (and whether
// Elasticsearch is configured) before deciding to use the live API or the
// bundled fallback data. `es` mirrors esEnabled() from es.js: true iff
// ELASTICSEARCH_URL is configured. CORS is permissive (Allow-Origin: *).

import { esEnabled } from '../../../api/_lib/es.js';

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
  return Response.json(
    { ok: true, es: esEnabled() },
    {
      headers: {
        ...CORS_HEADERS,
        'Cache-Control': 'no-store',
      },
    }
  );
}
