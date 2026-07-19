// api/graph.ts — the frontend API client for the search + package-graph backend.
//
// Everything here DEGRADES GRACEFULLY. On Vercel the serverless functions
// (`/api/graphql`, `/api/search`, `/api/health`) are live; on GitHub Pages (or
// any static host without functions) every call quietly falls back to the
// data bundled into the site (`graph.json` + `search-index.json`). No network
// helper ever throws to the UI — failures resolve to `null` / empty results so
// callers can branch on that.

// ---------------------------------------------------------------------------
// Shared shapes — these MUST match the generated data-file shapes and the
// GraphQL SDL so the pieces interoperate.
// ---------------------------------------------------------------------------

export type EdgeKind = 'import' | 'shared-upstream' | 'reference' | 'same-library';
export type SymbolKind = 'package' | 'type' | 'interface' | 'func' | 'method' | 'const' | 'var';

export interface Library {
  id: string;
  name: string;
  packageCount: number;
  symbolCount: number;
  parityAfter: string | null;
}

export interface GraphPackage {
  id: string; // === importPath
  importPath: string;
  name: string;
  library: string;
  synopsis: string;
  symbolCount: number;
}

export interface GraphEdge {
  source: string;
  target: string;
  kind: EdgeKind;
  weight: number;
}

export interface GraphData {
  generatedAt: string;
  libraries: Library[];
  packages: GraphPackage[];
  edges: GraphEdge[];
}

/** A search-index record (search-index.json / symbols.json), without a score. */
export interface SymbolRecord {
  id: string;
  name: string;
  kind: SymbolKind;
  packageImportPath: string;
  library: string;
  signature: string;
  doc: string;
  anchor: string;
}

export interface SymbolIndex {
  generatedAt: string;
  symbols: SymbolRecord[];
}

/** A ranked search hit — a SymbolRecord plus a relevance score (SearchHit SDL). */
export interface SearchHit extends SymbolRecord {
  score: number;
}

export type SearchBackend = 'elasticsearch' | 'memory' | 'fallback';

export interface SearchResult {
  hits: SearchHit[];
  backend: SearchBackend;
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/**
 * The base origin for the serverless functions. `VITE_API_URL` lets a static
 * deploy (e.g. GitHub Pages) point at a separate Vercel origin; blank means
 * same-origin, which is the Vercel case.
 */
export function apiBase(): string {
  const env = import.meta.env as Record<string, string | undefined>;
  return env.VITE_API_URL || '';
}

/**
 * POST a GraphQL query to `/api/graphql`. Resolves to the `data` payload typed
 * as `T`, or `null` on any transport/GraphQL error so callers can fall back.
 */
export async function graphql<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T | null> {
  try {
    const res = await fetch(`${apiBase()}/api/graphql`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query, variables }),
    });
    if (!res.ok) return null;
    const json = (await res.json()) as { data?: T; errors?: unknown[] };
    if (json.errors && json.errors.length) return null;
    return (json.data ?? null) as T | null;
  } catch {
    return null;
  }
}

/**
 * GET `/api/search?q=&first=`. Resolves to the raw response
 * `{ hits, backend }` or `null` on failure.
 */
export async function apiSearch(
  q: string,
  first = 20,
): Promise<{ hits: SearchHit[]; backend: SearchBackend } | null> {
  try {
    const url = `${apiBase()}/api/search?q=${encodeURIComponent(q)}&first=${first}`;
    const res = await fetch(url);
    if (!res.ok) return null;
    const json = (await res.json()) as { hits?: SearchHit[]; backend?: SearchBackend };
    if (!json || !Array.isArray(json.hits)) return null;
    return { hits: json.hits, backend: json.backend ?? 'memory' };
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Bundled fallbacks (always available; memoized so the JSON is fetched once)
// ---------------------------------------------------------------------------

let graphCache: Promise<GraphData | null> | null = null;
let symbolsCache: Promise<SymbolIndex | null> | null = null;

/** Fetch the bundled package graph (`graph.json`), memoized. */
export function loadFallbackGraph(): Promise<GraphData | null> {
  if (!graphCache) {
    graphCache = (async () => {
      try {
        const res = await fetch(`${import.meta.env.BASE_URL}graph.json`);
        if (!res.ok) return null;
        return (await res.json()) as GraphData;
      } catch {
        return null;
      }
    })();
  }
  return graphCache;
}

/** Fetch the bundled search index (`search-index.json`), memoized. */
export function loadFallbackSymbols(): Promise<SymbolIndex | null> {
  if (!symbolsCache) {
    symbolsCache = (async () => {
      try {
        const res = await fetch(`${import.meta.env.BASE_URL}search-index.json`);
        if (!res.ok) return null;
        return (await res.json()) as SymbolIndex;
      } catch {
        return null;
      }
    })();
  }
  return symbolsCache;
}

// ---------------------------------------------------------------------------
// API availability probe (cached for the life of the page)
// ---------------------------------------------------------------------------

let apiProbe: Promise<boolean> | null = null;

/**
 * Probe `/api/health` once and cache the answer. `true` means the serverless
 * functions are live (so search/graphql should be used); `false` means we are
 * on a static host and must use the bundled fallbacks.
 */
export function hasApi(): Promise<boolean> {
  if (!apiProbe) {
    apiProbe = (async () => {
      try {
        const res = await fetch(`${apiBase()}/api/health`);
        if (!res.ok) return false;
        const json = (await res.json()) as { ok?: boolean };
        return Boolean(json && json.ok);
      } catch {
        return false;
      }
    })();
  }
  return apiProbe;
}

// ---------------------------------------------------------------------------
// Unified search — API when available, else a small client-side ranked filter
// ---------------------------------------------------------------------------

/**
 * Search the corpus. Uses the live API (`/api/search`, which is Elasticsearch-
 * or memory-BM25-backed) when the functions are available, otherwise runs a
 * lightweight ranked filter over the bundled search index. Never throws.
 */
export async function search(q: string, first = 20): Promise<SearchResult> {
  const query = q.trim();
  if (!query) return { hits: [], backend: 'fallback' };

  if (await hasApi()) {
    const api = await apiSearch(query, first);
    if (api) return { hits: api.hits, backend: api.backend };
  }

  const idx = await loadFallbackSymbols();
  return { hits: rankSymbols(idx?.symbols ?? [], query, first), backend: 'fallback' };
}

/**
 * A small client-side ranked filter over the bundled symbols — the offline
 * counterpart to the server's BM25. Weights name(^) > package > doc, is
 * case-insensitive, prefers exact/prefix matches, and requires every whitespace
 * term to match somewhere (AND semantics).
 */
export function rankSymbols(symbols: SymbolRecord[], q: string, first = 20): SearchHit[] {
  const terms = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!terms.length) return [];

  const scored: SearchHit[] = [];
  for (const s of symbols) {
    const name = s.name.toLowerCase();
    const pkg = s.packageImportPath.toLowerCase();
    const doc = s.doc ? s.doc.toLowerCase() : '';

    let score = 0;
    let matchedAll = true;
    for (const t of terms) {
      let ts = 0;
      if (name === t) ts += 100;
      else if (name.startsWith(t)) ts += 42;
      else if (name.includes(t)) ts += 18;
      if (pkg.includes(t)) ts += 8;
      if (doc.includes(t)) ts += 3;
      if (ts === 0) {
        matchedAll = false;
        break;
      }
      score += ts;
    }
    if (!matchedAll) continue;

    // Nudge packages and short, memorable names to the top.
    if (s.kind === 'package') score += 6;
    score += Math.max(0, 12 - name.length) * 0.5;

    scored.push({ ...s, score });
  }

  scored.sort((a, b) => b.score - a.score || a.name.length - b.name.length);
  return scored.slice(0, first);
}
