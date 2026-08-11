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
import { timingSafeEqual } from 'node:crypto';
import { esEnabled, esIndexAll } from '../../../../api/_lib/es';
import type { SymbolDoc } from '../../../../api/_lib/data';

// Compare the presented bearer token against CRON_SECRET without leaking the
// secret's *contents* through response timing. (The length check short-circuits,
// so a wrong length is distinguishable — that is accepted; timingSafeEqual
// requires equal-length buffers, and secret length is not the sensitive part.)
function bearerMatches(header: string | null, secret: string): boolean {
  if (typeof header !== 'string') return false;
  const prefix = 'Bearer ';
  if (!header.startsWith(prefix)) return false;
  const presented = Buffer.from(header.slice(prefix.length));
  const expected = Buffer.from(secret);
  if (presented.length !== expected.length) return false;
  return timingSafeEqual(presented, expected);
}

const NO_STORE = { 'Cache-Control': 'no-store' } as const;

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
      { status: 500, headers: NO_STORE }
    );
  }
  if (!bearerMatches(request.headers.get('authorization'), secret)) {
    return Response.json({ ok: false, reason: 'unauthorized' }, { status: 401, headers: NO_STORE });
  }

  // If Upstash Search isn't configured, there's nothing to index. Return 200 so
  // Vercel Cron doesn't alarm on an expected, benign no-op.
  if (!esEnabled()) {
    return Response.json(
      { ok: false, reason: 'upstash not configured' },
      { status: 200, headers: NO_STORE }
    );
  }

  // Load the symbol corpus the same way scripts/index-symbols.ts does (the file
  // is bundled into this function by next.config.mjs outputFileTracingIncludes).
  // A missing/corrupt corpus is a deploy problem, not a caller problem — report
  // it as 500 with a fixed reason, never the raw fs/JSON error (which would
  // disclose the deployment's filesystem layout).
  let symbols: SymbolDoc[];
  try {
    const symbolsPath = path.join(process.cwd(), 'api', '_data', 'symbols.json');
    const parsed = JSON.parse(fs.readFileSync(symbolsPath, 'utf8')) as { symbols?: SymbolDoc[] };
    symbols = Array.isArray(parsed?.symbols) ? parsed.symbols : [];
  } catch (err) {
    console.error('[cron/reindex] failed to read symbol corpus', err);
    return Response.json(
      { ok: false, reason: 'corpus unavailable' },
      { status: 500, headers: NO_STORE }
    );
  }

  // Upstash can rate-limit or fail part-way through ~308 chunked upserts. Surface
  // that as 502 (upstream dependency failed) so Vercel Cron retries/alarms on a
  // meaningful status instead of an unhandled rejection.
  const started = Date.now();
  try {
    const indexed = await esIndexAll(symbols);
    return Response.json(
      { ok: true, indexed, tookMs: Date.now() - started },
      { headers: NO_STORE }
    );
  } catch (err) {
    console.error('[cron/reindex] indexing failed', err);
    return Response.json(
      { ok: false, reason: 'indexing failed', tookMs: Date.now() - started },
      { status: 502, headers: NO_STORE }
    );
  }
}
