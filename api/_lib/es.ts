// es.ts — optional Upstash Search backend for symbol search.
//
// Named `es` for historical reasons (it began as an Elasticsearch backend); it
// now talks to Upstash Search, provisioned via the Vercel Marketplace. The
// exported surface is unchanged so callers (search/graphql/health routes) did
// not need to change.
//
// Everything here is best-effort: if the Upstash env vars are unset, esEnabled()
// is false and callers use the in-memory BM25 backend. If any call throws
// (auth failure, service down, index errors), the error propagates so the
// caller can catch it and fall back to BM25.
//
// Exports:
//   esEnabled()          -> boolean, true iff Upstash Search is configured
//   esSearch(q, first)   -> Promise<SearchHit[]>, hybrid semantic + keyword (query only)
//   esIndexAll(onProgress?) -> Promise<number>, (re)upserts every symbol
//
// Indexing is NOT done on the search path. The symbol corpus is static (built
// at deploy time), so it is loaded into Upstash once via `pnpm index:symbols`
// (scripts/index-symbols.ts) — after initial deploy and whenever the data
// changes. esSearch only queries; a cold function never re-indexes 30k docs.
//
// Env (injected by the Upstash Search Marketplace integration; read by
// Search.fromEnv()):
//   UPSTASH_SEARCH_REST_URL     REST endpoint of the search index
//   UPSTASH_SEARCH_REST_TOKEN   read/write token

import { Search } from '@upstash/search';
import type { SymbolDoc } from './data';

const INDEX = 'symbols';

// Searchable text fields Upstash ranks over (hybrid full-text + semantic).
interface SymbolContent extends Record<string, unknown> {
  name: string;
  package: string;
  signature: string;
  doc: string;
}

// Everything needed to rebuild a SearchHit, stored alongside the document.
interface SymbolMeta extends Record<string, unknown> {
  id: string;
  name: string;
  kind: string;
  packageImportPath: string;
  library: string;
  signature: string | null;
  doc: string | null;
  anchor: string | null;
}

// A search hit returned to callers (matches the shared SearchHit contract).
export interface SearchHit {
  id: unknown;
  name: unknown;
  kind: unknown;
  packageImportPath: unknown;
  library: unknown;
  signature: unknown;
  doc: unknown;
  anchor: unknown;
  score: number;
}

export function esEnabled(): boolean {
  const url = process.env.UPSTASH_SEARCH_REST_URL;
  const token = process.env.UPSTASH_SEARCH_REST_TOKEN;
  return typeof url === 'string' && url.trim() !== '' && typeof token === 'string' && token.trim() !== '';
}

// Lazily create the client + index handle once per warm instance.
function makeIndex() {
  return Search.fromEnv().index<SymbolContent, SymbolMeta>(INDEX);
}
let indexHandle: ReturnType<typeof makeIndex> | null = null;
function getIndex(): ReturnType<typeof makeIndex> {
  if (!indexHandle) indexHandle = makeIndex();
  return indexHandle;
}

function toHit(meta: SymbolMeta | undefined, score: unknown): SearchHit {
  const s = meta || ({} as SymbolMeta);
  return {
    id: s.id,
    name: s.name,
    kind: s.kind,
    packageImportPath: s.packageImportPath,
    library: s.library,
    signature: s.signature ?? null,
    doc: s.doc ?? null,
    anchor: s.anchor ?? null,
    score: typeof score === 'number' ? score : 0,
  };
}

export async function esSearch(q: string, first = 20): Promise<SearchHit[]> {
  if (!esEnabled()) throw new Error('Upstash Search is not configured');

  const size = Number.isFinite(first) && first > 0 ? Math.floor(first) : 20;

  // Query only — the index is populated out-of-band by esIndexAll (see the
  // priming script). This keeps search latency low and off the write path.
  const results = await getIndex().search({
    query: String(q ?? ''),
    limit: size,
    reranking: true,
  });

  return results.map((r) => toHit(r.metadata, r.score));
}

// (Re)index the given symbols into Upstash. Called by scripts/index-symbols.ts,
// not on the search path. upsert is idempotent by id, so re-running is safe and
// picks up data changes. Returns the number of documents upserted. onProgress,
// if given, is called after each chunk with (done, total). The symbols are
// passed in (rather than read here) so this module has no runtime dependency on
// the data loader — keeping it importable from a plain Node script.
export async function esIndexAll(
  symbols: SymbolDoc[],
  onProgress?: (done: number, total: number) => void,
): Promise<number> {
  if (!esEnabled()) throw new Error('Upstash Search is not configured');

  const index = getIndex();
  const CHUNK = 100;

  for (let start = 0; start < symbols.length; start += CHUNK) {
    const slice = symbols.slice(start, start + CHUNK);
    const docs = slice.map((s) => {
      const content = buildContent(s);
      return {
        id: s.id,
        content,
        metadata: {
          id: s.id,
          name: s.name,
          kind: s.kind,
          packageImportPath: s.packageImportPath,
          library: s.library,
          signature: s.signature ?? null,
          // Display snippet — capped for the same reason as content.
          doc: s.doc ? clip(s.doc, DOC_META_MAX) : null,
          anchor: s.anchor ?? null,
        },
      };
    });
    await index.upsert(docs);
    onProgress?.(Math.min(start + CHUNK, symbols.length), symbols.length);
  }

  return symbols.length;
}

// Upstash Search rejects documents whose *serialized* content exceeds 4096
// chars. It measures the JSON form, so field names and newline escaping count —
// a raw character budget undercounts. We trim doc (then signature) until the
// serialized content fits CONTENT_MAX, which leaves margin under the 4096 limit.
const CONTENT_MAX = 4000;
const DOC_META_MAX = 2000;

function clip(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) : s;
}

function serializedLen(c: SymbolContent): number {
  return JSON.stringify(c).length;
}

function buildContent(s: SymbolDoc): SymbolContent {
  const content: SymbolContent = {
    name: String(s.name ?? ''),
    package: String(s.packageImportPath ?? ''),
    signature: String(s.signature ?? ''),
    doc: String(s.doc ?? ''),
  };

  // Trim doc to fit, then (rarely) signature. Converges in 1-2 passes; each
  // pass removes the exact overflow plus a small margin for escape expansion.
  for (let guard = 0; guard < 8 && serializedLen(content) > CONTENT_MAX; guard++) {
    const over = serializedLen(content) - CONTENT_MAX;
    if (content.doc.length > 0) {
      content.doc = content.doc.slice(0, Math.max(0, content.doc.length - over - 8));
    } else if (content.signature.length > 0) {
      content.signature = content.signature.slice(0, Math.max(0, content.signature.length - over - 8));
    } else {
      break;
    }
  }

  return content;
}
