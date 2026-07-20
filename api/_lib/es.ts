// es.ts — optional Elasticsearch backend for symbol search.
//
// Everything here is best-effort: if ELASTICSEARCH_URL is unset, esEnabled() is
// false and callers use the in-memory BM25 backend. If any ES call throws (bad
// URL, auth failure, cluster down, index errors), the error propagates so the
// caller can catch it and fall back to BM25.
//
// Exports:
//   esEnabled()          -> boolean, true iff ELASTICSEARCH_URL is configured
//   esSearch(q, first)   -> Promise<SearchHit[]> via /symbols/_search
//   esIndexIfNeeded()    -> Promise<boolean>, creates + bulk-loads the index once
//
// Env:
//   ELASTICSEARCH_URL       base URL of the cluster (required to enable ES)
//   ELASTICSEARCH_API_KEY   optional; sent as `Authorization: ApiKey <key>`

import { getSymbols } from './data';

const INDEX = 'symbols';

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

function esBase(): string | null {
  const url = process.env.ELASTICSEARCH_URL;
  if (!url) return null;
  return url.replace(/\/+$/, '');
}

function authHeaders(overrides: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...overrides };
  const apiKey = process.env.ELASTICSEARCH_API_KEY;
  if (apiKey) headers.Authorization = `ApiKey ${apiKey}`;
  return headers;
}

export function esEnabled(): boolean {
  const url = process.env.ELASTICSEARCH_URL;
  return typeof url === 'string' && url.trim() !== '';
}

function toHit(source: Record<string, unknown> | undefined | null, score: unknown): SearchHit {
  const s = source || {};
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
  const base = esBase();
  if (!base) throw new Error('Elasticsearch is not configured');

  const size = Number.isFinite(first) && first > 0 ? Math.floor(first) : 20;

  // Make sure the index exists and is populated before querying.
  await esIndexIfNeeded();

  const body = {
    size,
    query: {
      multi_match: {
        query: String(q ?? ''),
        fields: ['name^3', 'package^2', 'doc'],
        fuzziness: 'AUTO',
      },
    },
  };

  const resp = await fetch(`${base}/${INDEX}/_search`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(body),
  });

  if (!resp.ok) {
    throw new Error(`Elasticsearch search failed: ${resp.status}`);
  }

  const json = (await resp.json()) as { hits?: { hits?: { _source?: Record<string, unknown>; _score?: unknown }[] } };
  const hits = json?.hits?.hits || [];
  return hits.map((h) => toHit(h._source, h._score));
}

// Guard so index creation + bulk load happens at most once per warm instance.
// On failure the promise is cleared so a later call can retry; the rejection is
// re-thrown so callers fall back to BM25.
let indexPromise: Promise<boolean> | null = null;

export function esIndexIfNeeded(): Promise<boolean> {
  if (!esEnabled()) return Promise.resolve(false);
  if (indexPromise) return indexPromise;
  indexPromise = doIndex().catch((err) => {
    indexPromise = null;
    throw err;
  });
  return indexPromise;
}

async function doIndex(): Promise<boolean> {
  const base = esBase();
  if (!base) return false;

  // If the index already exists, assume it is populated and do nothing.
  const head = await fetch(`${base}/${INDEX}`, {
    method: 'HEAD',
    headers: authHeaders(),
  });
  if (head.ok) return true;

  // Create the index with an explicit mapping. The searchable `package` field
  // mirrors packageImportPath so the multi_match `package^2` boost works.
  const mapping = {
    mappings: {
      properties: {
        id: { type: 'keyword' },
        name: { type: 'text' },
        kind: { type: 'keyword' },
        package: { type: 'text' },
        packageImportPath: { type: 'keyword' },
        library: { type: 'keyword' },
        signature: { type: 'text' },
        doc: { type: 'text' },
        anchor: { type: 'keyword' },
      },
    },
  };

  const create = await fetch(`${base}/${INDEX}`, {
    method: 'PUT',
    headers: authHeaders(),
    body: JSON.stringify(mapping),
  });
  // 400 typically means the index was created concurrently — tolerate it.
  if (!create.ok && create.status !== 400) {
    throw new Error(`Elasticsearch create index failed: ${create.status}`);
  }

  const symbols = getSymbols();
  const CHUNK = 1000;
  for (let start = 0; start < symbols.length; start += CHUNK) {
    const slice = symbols.slice(start, start + CHUNK);
    let ndjson = '';
    for (const s of slice) {
      ndjson += JSON.stringify({ index: { _index: INDEX, _id: s.id } }) + '\n';
      ndjson +=
        JSON.stringify({
          id: s.id,
          name: s.name,
          kind: s.kind,
          package: s.packageImportPath,
          packageImportPath: s.packageImportPath,
          library: s.library,
          signature: s.signature,
          doc: s.doc,
          anchor: s.anchor,
        }) + '\n';
    }

    const refresh = start + CHUNK >= symbols.length ? '?refresh=wait_for' : '';
    const resp = await fetch(`${base}/_bulk${refresh}`, {
      method: 'POST',
      headers: authHeaders({ 'Content-Type': 'application/x-ndjson' }),
      body: ndjson,
    });
    if (!resp.ok) {
      throw new Error(`Elasticsearch bulk index failed: ${resp.status}`);
    }
    const result = (await resp.json()) as { errors?: boolean };
    if (result && result.errors) {
      throw new Error('Elasticsearch bulk index reported item errors');
    }
  }

  return true;
}
