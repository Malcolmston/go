// bm25.js — dependency-free BM25 ranking over the symbols corpus.
//
// search(symbols, q, first) returns the top `first` symbols ranked by a real
// BM25 score, combining three fields with the boosts required by the contract:
//   name    ^3
//   package ^2   (packageImportPath + library)
//   doc     ^1   (doc + signature)
//
// Matching is case-insensitive and tolerant of prefixes and single-edit typos
// (fuzziness of 1 edit). Identifiers are additionally split on camelCase and
// digit boundaries so e.g. "readfile" matches the token "ReadFile".
//
// Pure and self-contained: no external dependencies, no I/O.

const K1 = 1.5;
const B = 0.75;

// Match-strength weights applied to a token's term frequency.
const W_EXACT = 1.0;
const W_PREFIX = 0.85;
const W_FUZZY = 0.6;

const FIELDS = [
  { key: 'name', weight: 3 },
  { key: 'package', weight: 2 },
  { key: 'doc', weight: 1 },
];

function fieldText(sym, key) {
  switch (key) {
    case 'name':
      return sym.name || '';
    case 'package':
      return `${sym.packageImportPath || ''} ${sym.library || ''}`;
    case 'doc':
      return `${sym.doc || ''} ${sym.signature || ''}`;
    default:
      return '';
  }
}

// Tokenize into lowercase alphanumeric tokens, additionally emitting the
// camelCase / digit-boundary subtokens of each chunk.
function tokenize(text) {
  const tokens = [];
  const chunks = String(text).split(/[^A-Za-z0-9]+/);
  for (const chunk of chunks) {
    if (!chunk) continue;
    tokens.push(chunk.toLowerCase());
    const split = chunk
      .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
      .replace(/([A-Za-z])([0-9])/g, '$1 $2')
      .replace(/([0-9])([A-Za-z])/g, '$1 $2')
      .split(/\s+/);
    if (split.length > 1) {
      for (const part of split) {
        if (part) tokens.push(part.toLowerCase());
      }
    }
  }
  return tokens;
}

function uniq(arr) {
  return Array.from(new Set(arr));
}

// True iff the Levenshtein distance between a and b is <= 1.
function within1Edit(a, b) {
  if (a === b) return true;
  let la = a.length;
  let lb = b.length;
  if (Math.abs(la - lb) > 1) return false;
  // Make `a` the shorter (or equal) string.
  if (la > lb) {
    const t = a;
    a = b;
    b = t;
    const tl = la;
    la = lb;
    lb = tl;
  }
  let i = 0;
  let j = 0;
  let edits = 0;
  while (i < la && j < lb) {
    if (a[i] === b[j]) {
      i++;
      j++;
    } else {
      edits++;
      if (edits > 1) return false;
      if (la === lb) {
        // substitution
        i++;
        j++;
      } else {
        // deletion from the longer string
        j++;
      }
    }
  }
  // Trailing unmatched char in the longer string counts as one edit.
  if (j < lb) edits += lb - j;
  return edits <= 1;
}

function matchWeight(queryTerm, token) {
  if (token === queryTerm) return W_EXACT;
  if (queryTerm.length >= 2 && token.startsWith(queryTerm)) return W_PREFIX;
  if (queryTerm.length >= 3 && within1Edit(token, queryTerm)) return W_FUZZY;
  return 0;
}

// Effective (fuzzy) term frequency of a query term within one document field.
function effectiveTf(tfMap, queryTerm) {
  let tf = 0;
  for (const [token, count] of tfMap) {
    const w = matchWeight(queryTerm, token);
    if (w > 0) tf += w * count;
  }
  return tf;
}

// Build (and memoize on the symbols array identity) the per-field term-frequency
// maps, document lengths and average document lengths used by BM25.
const indexCache = new WeakMap();

function buildIndex(symbols) {
  const cached = indexCache.get(symbols);
  if (cached) return cached;

  const N = symbols.length;
  const fields = {};
  for (const f of FIELDS) {
    const tf = new Array(N);
    const len = new Float64Array(N);
    let total = 0;
    for (let i = 0; i < N; i++) {
      const toks = tokenize(fieldText(symbols[i], f.key));
      const m = new Map();
      for (const t of toks) m.set(t, (m.get(t) || 0) + 1);
      tf[i] = m;
      len[i] = toks.length;
      total += toks.length;
    }
    fields[f.key] = { tf, len, avgdl: N ? total / N : 0 };
  }

  const idx = { N, fields };
  indexCache.set(symbols, idx);
  return idx;
}

export function search(symbols, q, first = 20) {
  if (!Array.isArray(symbols) || symbols.length === 0) return [];
  if (q == null || String(q).trim() === '') return [];

  const limit = Number.isFinite(first) && first > 0 ? Math.floor(first) : 20;
  const queryTerms = uniq(tokenize(q));
  if (queryTerms.length === 0) return [];

  const idx = buildIndex(symbols);
  const N = idx.N;
  const scores = new Float64Array(N);

  for (const f of FIELDS) {
    const { tf: tfArr, len, avgdl } = idx.fields[f.key];
    const denomAvg = avgdl || 1;
    for (const term of queryTerms) {
      // First pass: effective tf per doc + document frequency (for IDF).
      const tfs = new Float64Array(N);
      let df = 0;
      for (let i = 0; i < N; i++) {
        const t = effectiveTf(tfArr[i], term);
        tfs[i] = t;
        if (t > 0) df++;
      }
      if (df === 0) continue;

      const idf = Math.log(1 + (N - df + 0.5) / (df + 0.5));
      for (let i = 0; i < N; i++) {
        const t = tfs[i];
        if (t <= 0) continue;
        const denom = t + K1 * (1 - B + B * (len[i] / denomAvg));
        const s = idf * ((t * (K1 + 1)) / (denom || 1));
        scores[i] += f.weight * s;
      }
    }
  }

  const ranked = [];
  for (let i = 0; i < N; i++) {
    if (scores[i] > 0) ranked.push(i);
  }
  ranked.sort((a, b) => {
    if (scores[b] !== scores[a]) return scores[b] - scores[a];
    return String(symbols[a].name || '').localeCompare(String(symbols[b].name || ''));
  });

  return ranked.slice(0, limit).map((i) => ({
    ...symbols[i],
    score: Math.round(scores[i] * 1e6) / 1e6,
  }));
}
