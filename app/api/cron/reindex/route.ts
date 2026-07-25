// app/api/cron/reindex/route.ts — Vercel Cron reindex for symbol search.
//
// Rebuilds the Upstash Search index from the static symbol corpus, the same
// work `pnpm index:symbols` (scripts/index-symbols.ts) does, but triggered by
// Vercel Cron instead of CI:
//   GET /api/cron/reindex   -> { ok, indexed, tookMs }
//
// This is a SAFETY-NET, not the primary freshness mechanism. The symbol corpus
// (api/_data/symbols.json) only changes at build time, so the authoritative
// reindex is the post-deploy step in .github/workflows/vercel.yml (no duration
// cap). The corpus is ~30.7k symbols and esIndexAll upserts serially in chunks
// of 100 (~308 round-trips), which can take tens of seconds — hence
// maxDuration below and the Pro/Fluid Compute requirement to run it to
// completion. See the INFRA notes.
//
// Auth: Vercel Cron sends `Authorization: Bearer $CRON_SECRET`. We require it
// and refuse (500) if CRON_SECRET is unset, so the route never runs unprotected.
//
// The corpus is read from the bundled api/_data/symbols.json (force-included
// into the /api function via next.config.mjs outputFileTracingIncludes). The
// Upstash env vars are injected by the Marketplace integration — no .env
// loading, unlike the CLI script.

import fs from 'node:fs';
import path from 'node:path';
import { esEnabled, esIndexAll } from '@/api/_lib/es';
import type { SymbolDoc } from '@/api/_lib/data';

// Run on the Node.js runtime (fs + the shared lib read process.env), never
// cache, and allow up to 5 minutes for the full reindex (Pro/Fluid Compute).
export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';
export const maxDuration = 300;

export async function GET(request: Request): Promise<Response> {
  // Authorize via the Vercel Cron bearer secret. If CRON_SECRET is unset we
  // refuse rather than run unprotected — a misconfiguration, not a public route.
  const secret = process.env.CRON_SECRET;
  if (!secret || secret.trim() === '') {
    return Response.json(
      { ok: false, reason: 'CRON_SECRET not configured' },
      { status: 500, headers: { 'Cache-Control': 'no-store' } }
    );
  }
  const auth = request.headers.get('authorization');
  if (auth !== `Bearer ${secret}`) {
    return Response.json(
      { ok: false, reason: 'unauthorized' },
      { status: 401, headers: { 'Cache-Control': 'no-store' } }
    );
  }

  // If Upstash Search isn't configured, there's nothing to index. Return 200 so
  // Vercel Cron doesn't alarm on an expected, benign no-op.
  if (!esEnabled()) {
    return Response.json(
      { ok: false, reason: 'upstash not configured' },
      { status: 200, headers: { 'Cache-Control': 'no-store' } }
    );
  }

  // Load the symbol corpus the same way scripts/index-symbols.ts does (the file
  // is bundled into this function by next.config.mjs outputFileTracingIncludes).
  const symbolsPath = path.join(process.cwd(), 'api', '_data', 'symbols.json');
  const parsed = JSON.parse(fs.readFileSync(symbolsPath, 'utf8')) as { symbols?: SymbolDoc[] };
  const symbols = parsed.symbols ?? [];

  const started = Date.now();
  const indexed = await esIndexAll(symbols);
  const tookMs = Date.now() - started;

  return Response.json(
    { ok: true, indexed, tookMs },
    { headers: { 'Cache-Control': 'no-store' } }
  );
}
