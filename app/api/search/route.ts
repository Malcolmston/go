// app/api/search/route.ts — Next.js Route Handler port of api/search.js.
//
// REST symbol search for the go aggregator, reusing the shared ESM libs in
// /workspace/go/api/_lib:
//   GET /api/search?q=<query>&first=<n>
//     -> { hits: SearchHit[], backend: "upstash" | "memory" }
//   OPTIONS preflight -> 204
//
// SearchHit shape (shared contract):
//   { id, name, kind, packageImportPath, library, signature, doc, anchor, score }
//
// Backend selection mirrors the original serverless function exactly:
//   - If Upstash Search is configured (esEnabled()), try esSearch(); on ANY
//     error fall back to the in-memory BM25 backend over getSymbols().
//   - Otherwise use BM25 directly.
//   - With an empty/missing query, return an empty hit list (no error).
//
// CORS is permissive (Access-Control-Allow-Origin: *).

import { esEnabled, esSearch } from '../../../api/_lib/es';
import { search as bm25Search, queryTokens } from '../../../api/_lib/bm25';
import { getSymbols } from '../../../api/_lib/data';
import { logSearch } from '../../../api/_lib/analytics';
import { MAX_FIRST, firstParam, parseKinds, rerankEnabled } from './params';

// Run on the Node.js runtime (the shared libs use node built-ins), and never
// pre-render / cache — every request executes the search live.
export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
  'Access-Control-Max-Age': '86400',
};

async function runSearch(
  q: string,
  first: number,
  library: string,
  kinds: string[],
  reranking: boolean,
): Promise<{ hits: any[]; backend: string; upstashUsed: boolean }> {
  if (q.trim() === '') return { hits: [], backend: 'memory', upstashUsed: false };

  if (esEnabled()) {
    try {
      // Push library/kind scoping down to Upstash as a metadata filter, so we
      // request exactly `first` results and never drop in-scope hits that rank
      // below an over-fetch cap. No app-side re-filtering needed here.
      const hits = await esSearch(q, first, {
        reranking,
        library: library || undefined,
        kinds,
      });
      return { hits, backend: 'upstash', upstashUsed: true };
    } catch {
      // Fall through to BM25 on any Upstash Search failure.
    }
  }

  // BM25 fallback: filtering is done in-app, so when any filter is active
  // over-fetch first and slice after, so the post-filter result still fills
  // `first`.
  const filtered = library !== '' || kinds.length > 0;
  const kindSet = kinds.length > 0 ? new Set(kinds) : null;
  const want = filtered ? Math.min(MAX_FIRST, first * 8) : first;
  const scope = (hits: any[]): any[] =>
    hits
      .filter(
        (h) =>
          h &&
          (library === '' || h.library === library) &&
          (kindSet === null || kindSet.has(h.kind)),
      )
      .slice(0, first);
  return { hits: scope(bm25Search(getSymbols(), q, want)), backend: 'memory', upstashUsed: false };
}

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function GET(req: Request): Promise<Response> {
  const params = new URL(req.url).searchParams;
  const q = params.get('q') ?? '';
  const first = firstParam(params.get('first'));
  const library = (params.get('library') ?? '').trim();
  const kinds = parseKinds(params.get('kind'));
  const rerankOn = rerankEnabled();

  const startedAt = Date.now();
  const { hits, backend, upstashUsed } = await runSearch(q, first, library, kinds, rerankOn);
  const tookMs = Math.round(Date.now() - startedAt);

  // reranking was actually applied only when Upstash served the query AND it
  // was enabled for this request.
  const reranked = upstashUsed && rerankOn;
  // Lowercased query tokens from the same tokenizer BM25 uses, so client-side
  // highlighting lines up with ranking. Never server-render highlight HTML.
  const matchedTerms = queryTokens(q);

  // Best-effort structured analytics line (Vercel logs); never throws/blocks.
  logSearch({
    q,
    first,
    library,
    kind: kinds,
    hits: hits.length,
    backend,
    tookMs,
    reranked,
  });

  return Response.json(
    {
      hits,
      backend,
      total: hits.length,
      tookMs,
      reranked,
      matchedTerms,
    },
    {
      headers: {
        ...CORS_HEADERS,
        'Cache-Control': 'public, max-age=0, s-maxage=60, stale-while-revalidate=300',
      },
    }
  );
}
