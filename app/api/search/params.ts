// app/api/search/params.ts — pure request-param helpers for the search route.
//
// These live in their own module (not route.ts) because Next.js only permits a
// fixed set of exports from a Route Handler file (the HTTP-method handlers plus
// route-segment config like `runtime`/`dynamic`). Exporting arbitrary helpers
// from route.ts fails `next build` with "does not match the required types of a
// Next.js Route" — so the reusable, unit-testable pieces live here and route.ts
// imports them.

export const DEFAULT_FIRST = 20;
export const MAX_FIRST = 100;

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

// Reranking is ON by default; only an explicit "0"/"false" (case-insensitive)
// in SEARCH_RERANK disables it.
export function rerankEnabled(): boolean {
  const v = (process.env.SEARCH_RERANK ?? '').trim().toLowerCase();
  return v !== '0' && v !== 'false';
}
