// search.js — REST symbol search for the go aggregator.
//
//   GET /api/search?q=<query>&first=<n>
//     -> { hits: SearchHit[], backend: "elasticsearch" | "memory" }
//
// SearchHit shape (matches the shared contract):
//   { id, name, kind, packageImportPath, library, signature, doc, anchor, score }
//
// Backend selection:
//   - If Elasticsearch is configured (esEnabled()), try esSearch(); on ANY error
//     fall back to the in-memory BM25 backend over getSymbols().
//   - Otherwise use BM25 directly.
//
// CORS is permissive (Access-Control-Allow-Origin: *) and OPTIONS preflight is
// handled. The endpoint degrades gracefully: with no ES and an empty/missing
// query it returns an empty hit list rather than erroring.

import { esEnabled, esSearch } from './_lib/es.js';
import { search as bm25Search } from './_lib/bm25.js';
import { getSymbols } from './_lib/data.js';

const DEFAULT_FIRST = 20;
const MAX_FIRST = 100;

function setCors(res) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
  res.setHeader('Access-Control-Max-Age', '86400');
}

// Vercel passes req.query, but fall back to parsing the URL for robustness.
function readQuery(req) {
  const q = req && req.query;
  if (q && typeof q === 'object') return q;
  try {
    const url = new URL(req.url, 'http://localhost');
    const out = {};
    for (const [k, v] of url.searchParams.entries()) out[k] = v;
    return out;
  } catch {
    return {};
  }
}

function firstParam(value) {
  if (Array.isArray(value)) value = value[0];
  const n = Number.parseInt(value, 10);
  if (!Number.isFinite(n) || n <= 0) return DEFAULT_FIRST;
  return Math.min(n, MAX_FIRST);
}

function queryParam(value) {
  if (Array.isArray(value)) value = value[0];
  return typeof value === 'string' ? value : '';
}

export default async function handler(req, res) {
  setCors(res);

  if (req.method === 'OPTIONS') {
    res.statusCode = 204;
    res.end();
    return;
  }

  if (req.method && req.method !== 'GET') {
    res.setHeader('Allow', 'GET, OPTIONS');
    res.statusCode = 405;
    res.setHeader('Content-Type', 'application/json; charset=utf-8');
    res.end(JSON.stringify({ error: 'Method Not Allowed' }));
    return;
  }

  const params = readQuery(req);
  const q = queryParam(params.q);
  const first = firstParam(params.first);

  let hits = [];
  let backend = 'memory';

  if (q.trim() !== '') {
    if (esEnabled()) {
      try {
        hits = await esSearch(q, first);
        backend = 'elasticsearch';
      } catch {
        hits = bm25Search(getSymbols(), q, first);
        backend = 'memory';
      }
    } else {
      hits = bm25Search(getSymbols(), q, first);
      backend = 'memory';
    }
  }

  res.statusCode = 200;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.setHeader('Cache-Control', 'public, max-age=0, s-maxage=60, stale-while-revalidate=300');
  res.end(JSON.stringify({ hits, backend }));
}
