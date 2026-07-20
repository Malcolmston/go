// app/api/search/route.ts — Next.js Route Handler port of api/search.js.
//
// REST symbol search for the go aggregator, reusing the shared ESM libs in
// /workspace/go/api/_lib:
//   GET /api/search?q=<query>&first=<n>
//     -> { hits: SearchHit[], backend: "elasticsearch" | "memory" }
//   OPTIONS preflight -> 204
//
// SearchHit shape (shared contract):
//   { id, name, kind, packageImportPath, library, signature, doc, anchor, score }
//
// Backend selection mirrors the original serverless function exactly:
//   - If Elasticsearch is configured (esEnabled()), try esSearch(); on ANY
//     error fall back to the in-memory BM25 backend over getSymbols().
//   - Otherwise use BM25 directly.
//   - With an empty/missing query, return an empty hit list (no error).
//
// CORS is permissive (Access-Control-Allow-Origin: *).

import { esEnabled, esSearch } from '../../../api/_lib/es';
import { search as bm25Search } from '../../../api/_lib/bm25';
import { getSymbols } from '../../../api/_lib/data';

// Run on the Node.js runtime (the shared libs use node built-ins), and never
// pre-render / cache — every request executes the search live.
export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const DEFAULT_FIRST = 20;
const MAX_FIRST = 100;

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
  'Access-Control-Max-Age': '86400',
};

function firstParam(value: string | null): number {
  const n = Number.parseInt(value ?? '', 10);
  if (!Number.isFinite(n) || n <= 0) return DEFAULT_FIRST;
  return Math.min(n, MAX_FIRST);
}

async function runSearch(q: string, first: number, library: string): Promise<{ hits: any[]; backend: string }> {
  if (q.trim() === '') return { hits: [], backend: 'memory' };

  // When scoping to one library (the API-docs header search), over-fetch so the
  // per-library slice still fills `first` after filtering, then keep only that
  // library's hits. Applies to both backends without changing their signatures.
  const want = library ? Math.min(MAX_FIRST, first * 8) : first;
  const scope = (hits: any[]): any[] =>
    (library ? hits.filter((h) => h && h.library === library) : hits).slice(0, first);

  if (esEnabled()) {
    try {
      const hits = await esSearch(q, want);
      return { hits: scope(hits), backend: 'elasticsearch' };
    } catch {
      // Fall through to BM25 on any Elasticsearch failure.
    }
  }
  return { hits: scope(bm25Search(getSymbols(), q, want)), backend: 'memory' };
}

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function GET(req: Request): Promise<Response> {
  const params = new URL(req.url).searchParams;
  const q = params.get('q') ?? '';
  const first = firstParam(params.get('first'));
  const library = (params.get('library') ?? '').trim();

  const { hits, backend } = await runSearch(q, first, library);

  return Response.json(
    { hits, backend },
    {
      headers: {
        ...CORS_HEADERS,
        'Cache-Control': 'public, max-age=0, s-maxage=60, stale-while-revalidate=300',
      },
    }
  );
}
