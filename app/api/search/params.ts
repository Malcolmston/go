// app/api/search/params.ts — request-param parsing + validation for the search route.
//
// These live in their own module (not route.ts) because Next.js only permits a
// fixed set of exports from a Route Handler file (the HTTP-method handlers plus
// route-segment config like `runtime`/`dynamic`). Exporting arbitrary helpers
// from route.ts fails `next build` with "does not match the required types of a
// Next.js Route" — so the reusable, unit-testable pieces live here and route.ts
// imports them.
//
// Everything is *coerced and bounded*, never rejected: `/api/search` is a public,
// CORS-open read endpoint, and a garbage param should degrade to the default
// rather than 400. The bounds matter for more than tidiness:
//   - `first` is capped so a single request cannot ask Upstash (or the BM25
//     ranker) for an unbounded result set.
//   - `q` is length-capped because the BM25 path tokenizes the query and scans
//     the whole corpus once per unique term — an unbounded query string is a
//     cheap way to burn CPU.
//   - `library` is charset-restricted before it is interpolated into the Upstash
//     metadata filter expression, so no caller-controlled text reaches the
//     filter builder.

import { z } from 'zod';

export const DEFAULT_FIRST = 20;
export const MAX_FIRST = 100;

// Upper bound on the raw `q` string. Real symbol queries are a few words; this
// is generous while keeping the BM25 term loop bounded.
export const MAX_Q_LEN = 256;

// Upper bound on a library id. The longest real id is well under this.
export const MAX_LIBRARY_LEN = 64;

// Library ids are generated slugs: lowercase alphanumerics plus `-`, `_` and `.`
// (e.g. "express", "socket.io", "next-mdx"). Anything else is not a library id
// and is dropped rather than passed downstream.
const LIBRARY_RE = /^[a-z0-9][a-z0-9._-]*$/;

// Valid `kind` filter values (matches the Go symbol kinds surfaced in hits).
export const VALID_KINDS = new Set(['func', 'type', 'method', 'interface', 'const', 'var']);

// Parse the comma-separated `kind` param into a validated, de-duped list.
// Invalid entries are ignored; empty/absent yields [] (no kind filtering).
export function parseKinds(value: string | null): string[] {
  if (!value) return [];
  const out: string[] = [];
  for (const raw of value.split(',')) {
    const k = raw.trim().toLowerCase();
    if (k && VALID_KINDS.has(k) && !out.includes(k)) out.push(k);
  }
  return out;
}

// Clamp the `first` param to (0, MAX_FIRST], defaulting when absent/invalid.
export function firstParam(value: string | null): number {
  const n = Number.parseInt(value ?? '', 10);
  if (!Number.isFinite(n) || n <= 0) return DEFAULT_FIRST;
  return Math.min(n, MAX_FIRST);
}

// Normalize the free-text query: trim and hard-cap the length. Non-strings and
// absent values become ''. An empty result makes the route short-circuit.
export function parseQuery(value: string | null): string {
  if (typeof value !== 'string') return '';
  return value.trim().slice(0, MAX_Q_LEN);
}

// Validate the `library` scope. Returns '' (no scoping) unless the value is a
// well-formed library slug within MAX_LIBRARY_LEN. Case is normalized down,
// matching how library ids are generated.
export function parseLibrary(value: string | null): string {
  if (typeof value !== 'string') return '';
  const v = value.trim().toLowerCase();
  if (v === '' || v.length > MAX_LIBRARY_LEN) return '';
  return LIBRARY_RE.test(v) ? v : '';
}

// Reranking is ON by default; only an explicit "0"/"false" (case-insensitive)
// in SEARCH_RERANK disables it.
export function rerankEnabled(): boolean {
  const v = (process.env.SEARCH_RERANK ?? '').trim().toLowerCase();
  return v !== '0' && v !== 'false';
}

// The validated shape the route handler works with.
export interface SearchParams {
  q: string;
  first: number;
  library: string;
  kinds: string[];
}

// zod schema over the *raw* (string | null) query params. Every field has a
// catch/transform so parsing is total — `.parse()` cannot throw, which is what
// lets the route treat bad input as "use the default" instead of 400.
export const searchParamsSchema = z
  .object({
    q: z.string().nullable().transform(parseQuery),
    first: z.string().nullable().transform(firstParam),
    library: z.string().nullable().transform(parseLibrary),
    kind: z.string().nullable().transform(parseKinds),
  })
  .transform((v): SearchParams => ({ q: v.q, first: v.first, library: v.library, kinds: v.kind }));

// Parse and validate a whole URLSearchParams into the route's bounded input.
export function parseSearchParams(params: URLSearchParams): SearchParams {
  return searchParamsSchema.parse({
    q: params.get('q'),
    first: params.get('first'),
    library: params.get('library'),
    kind: params.get('kind'),
  });
}
