// data.ts — loads and memoizes the generated graph + symbols data files.
//
// The data files are the single source of truth shared by the serverless
// functions and (via a public/ copy) the frontend. They are read once from
// api/_data/ using import.meta.url so paths resolve regardless of the process
// cwd Vercel runs the function under.
//
// Exports:
//   loadGraph()  / getGraph()   -> the parsed graph.json object
//                                  { generatedAt, libraries[], packages[], edges[] }
//   loadSymbols()/ getSymbols() -> the symbols array from symbols.json
//                                  [ { id, name, kind, packageImportPath, library,
//                                      signature, doc, anchor } ]
//   getParity()  / getParityFor(libId)
//                               -> the MEASURED parity corpus from parity.json,
//                                  built out of parity/**/{parity.json,
//                                  COVERAGE.md,cases/*.json,security.json}.
//                                  This — not src/parity.ts, which is a legacy
//                                  hand-written map — is the source of truth for
//                                  what the ports actually score.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// A library node in graph.json.
export interface GraphLibrary {
  id: string;
  name: string;
  packageCount?: number;
  symbolCount?: number;
  parityAfter?: string;
  [key: string]: unknown;
}

// A package node in graph.json. `id` equals `importPath`.
export interface GraphPackage {
  id: string;
  importPath: string;
  name: string;
  library: string;
  synopsis?: string;
  symbolCount?: number;
  [key: string]: unknown;
}

// An edge in graph.json.
export type EdgeKind = 'import' | 'shared-upstream' | 'reference' | 'same-library';
export interface GraphEdge {
  source: string;
  target: string;
  kind: EdgeKind | string;
  weight?: number;
  [key: string]: unknown;
}

export interface Graph {
  generatedAt: string;
  libraries: GraphLibrary[];
  packages: GraphPackage[];
  edges: GraphEdge[];
}

// A symbol record from symbols.json (the search corpus).
export interface SymbolDoc {
  id: string;
  name: string;
  kind: string;
  packageImportPath: string;
  library: string;
  signature?: string | null;
  doc?: string | null;
  anchor?: string | null;
  [key: string]: unknown;
}

// A runnable-example record from examples.json (keyed by library id).
export interface ExampleDoc {
  library: string;
  path: string;
  code: string;
  codeTruncated?: boolean;
  readme: string;
  readmeTruncated?: boolean;
  [key: string]: unknown;
}

// --- measured parity (api/_data/parity.json) -------------------------------
// Written by scripts/build-graph-data.ts, which normalises the non-uniform
// harness manifests under parity/** into this one shape. Numbers are always
// finite; `status`/`percent` fields are never NaN.

export type ParityCaseStatus = 'match' | 'mismatch' | 'deviation' | 'unknown';
export type ParityCoverageStatus = 'match' | 'differs' | 'missing' | 'extra' | 'untested';

export interface ParityUpstream {
  /** exactly as the harness pinned it, e.g. "express@4.21.2". */
  raw: string;
  name: string;
  version: string;
  /** node | python | java | ruby | rust | elixir | c ("" when undetectable). */
  ecosystem: string;
}
export interface ParityGroup {
  name: string;
  total: number;
  match: number;
  mismatch: number;
  deviations: number;
  /** the harness recorded a case count for this group but no verdicts. */
  countsOnly: boolean;
}
export interface ParityCase {
  id: string;
  group: string;
  fn: string;
  upstreamFn: string;
  goFn: string;
  note: string;
  deviation: string;
  status: ParityCaseStatus;
  /** the harness's failure list named this case, even if it is a declared deviation. */
  harnessFailed: boolean;
  file: string;
}
export interface ParityCoverageRow {
  upstreamSymbol: string;
  goSymbol: string;
  status: ParityCoverageStatus;
  cases: string[];
  note: string;
}
export interface ParityCoverageSummary {
  match: number;
  differs: number;
  missing: number;
  extra: number;
  untested: number;
  percent: number;
  total: number;
}
export interface ParitySecurityFinding {
  id: string;
  summary: string;
  severity: string;
  affectedVersions: string;
  cases: string[];
  /** repo-relative manifest holding the full markdown `description`. */
  descriptionPath: string;
  hasDescription: boolean;
}
export interface ParityLibrary {
  library: string;
  /** nested harnesses only: the upstream package this sub-port ports. */
  package?: string;
  harnessPath: string;
  upstream: ParityUpstream;
  goModule: string;
  goPackage: string;
  goVersion: string;
  goVersionRaw: string;
  generatedBy: string;
  measuredAt: string;
  total: number;
  match: number;
  mismatch: number;
  deviations: number;
  parityPercent: number;
  comparedTotal: number;
  /** false when the harness recorded no per-case verdicts (statuses are `unknown`). */
  perCaseVerdicts: boolean;
  groups: ParityGroup[];
  groupsDerived: boolean;
  cases: ParityCase[];
  coverage: ParityCoverageRow[];
  coverageSummary: ParityCoverageSummary;
  coverageCommand: string;
  security: ParitySecurityFinding[];
  /** ports of a *different* upstream package living in the same sub-repo. */
  nested?: Record<string, ParityLibrary>;
  [key: string]: unknown;
}
export interface ParityCorpus {
  generatedAt: string;
  libraries: Record<string, ParityLibrary>;
}

// ---------------------------------------------------------------------------
// The browser-reachable parity SUMMARY.
//
// parity.json (the corpus) is ~5.4 MB of per-case detail and is deliberately
// server-side only. The frontend (src/components/Explore.tsx) needs exactly one
// number per library, and refuses any parity payload over its own 512 KB guard,
// so the summary below is the browser-facing projection of the corpus: the
// headline counts and the pinned upstream, nothing else. It is produced twice
// from the same code — written to public/parity.json by
// scripts/build-graph-data.ts (which Explore fetches directly, and which is the
// only form GitHub Pages can serve) and served live by app/api/parity/route.ts.
// ---------------------------------------------------------------------------

export interface ParitySummaryEntry {
  parityPercent: number;
  total: number;
  match: number;
  mismatch: number;
  deviations: number;
  upstream: { raw: string; name: string; version: string };
}
export interface ParitySummary {
  generatedAt: string;
  libraries: Record<string, ParitySummaryEntry>;
}

// Structural input: the generator has its own (wider) corpus types and runs
// under Node's type stripping, so summarizeParity accepts anything carrying the
// fields it projects rather than the ParityCorpus nominal shape.
export interface ParitySummarySource {
  generatedAt: string;
  libraries: Record<
    string,
    {
      total: number;
      match: number;
      mismatch: number;
      deviations: number;
      parityPercent: number;
      upstream: { raw: string; name: string; version: string };
    }
  >;
}

// Project the measured corpus down to the per-library headline figures.
// Deterministic: keys are emitted in sorted order and lowercased, because
// Explore keys its lookup by the lowercased library id. Nested harnesses are
// intentionally omitted — they are not libraries in the graph, so no page can
// key on them, and including them would only grow the payload.
export function summarizeParity(corpus: ParitySummarySource): ParitySummary {
  const libraries: Record<string, ParitySummaryEntry> = {};
  const src = corpus?.libraries ?? {};
  for (const id of Object.keys(src).sort()) {
    const lib = src[id];
    if (!lib) continue;
    const key = id.trim().toLowerCase();
    if (key === '') continue;
    libraries[key] = {
      parityPercent: num(lib.parityPercent),
      total: num(lib.total),
      match: num(lib.match),
      mismatch: num(lib.mismatch),
      deviations: num(lib.deviations),
      upstream: {
        raw: String(lib.upstream?.raw ?? ''),
        name: String(lib.upstream?.name ?? ''),
        version: String(lib.upstream?.version ?? ''),
      },
    };
  }
  return { generatedAt: String(corpus?.generatedAt ?? ''), libraries };
}

let graphCache: Graph | null = null;
let symbolsCache: SymbolDoc[] | null = null;
let examplesCache: Record<string, ExampleDoc> | null = null;
let parityCache: ParityCorpus | null = null;

// Where the generated data actually is at runtime, which is NOT one fixed place.
//
// Locally, this module runs from api/_lib/ and `../_data/x.json` resolves against
// import.meta.url. In a deployed serverless function it does not: the compiled
// module is inlined into a bundle under .next/server/app/api/**, so
// `../_data/` points at a directory that does not exist. next.config.mjs traces
// api/_data into the function, but it lands relative to the tracing ROOT, not
// relative to the chunk. The old single-candidate read therefore threw on every
// deploy and a bare `catch` turned it into an empty corpus — /api/parity answered
// {"libraries":{}}, the assistant's example and security tools had nothing to
// serve, and /api/health reported symbols: 0. Nothing failed loudly, so it shipped.
//
// So: try the candidates, and if every one misses, SAY SO on stderr. A silent
// empty corpus is the failure mode that hid this.
const DATA_CANDIDATES: readonly string[] = [
  // Deployed: cwd is the traced root, so the path is as-committed.
  'api/_data',
  // Deployed from a nested cwd (belt and braces).
  '../api/_data',
  '../../api/_data',
];

let resolvedDataDir: string | null = null;
const missingWarned = new Set<string>();

function dataPathFor(fileName: string): string | null {
  if (resolvedDataDir) {
    const p = path.join(resolvedDataDir, fileName);
    if (fs.existsSync(p)) return p;
  }
  // Relative to this module — correct locally and in any unbundled runtime.
  try {
    const url = new URL(`../_data/${fileName}`, import.meta.url);
    const p = fileURLToPath(url);
    if (fs.existsSync(p)) {
      resolvedDataDir = path.dirname(p);
      return p;
    }
  } catch {
    // import.meta.url may not be a file URL in every runtime; fall through.
  }
  for (const base of DATA_CANDIDATES) {
    const p = path.resolve(process.cwd(), base, fileName);
    if (fs.existsSync(p)) {
      resolvedDataDir = path.dirname(p);
      return p;
    }
  }
  return null;
}

/**
 * Read a generated file out of api/_data by NAME, from wherever it landed.
 *
 * Exported so every loader (./security.ts included) shares one resolution
 * strategy: a second copy of this logic is a second chance to be wrong in
 * production only, which is how the empty-corpus bug survived.
 *
 * Throws if the file cannot be found, after warning once on stderr.
 */
export function readDataJson(fileName: string): unknown {
  return readJson(fileName);
}

function readJson(relativePath: string): unknown {
  // Callers pass '../_data/<file>' or a bare file name; the name is what matters.
  const fileName = relativePath.split('/').pop() as string;
  const found = dataPathFor(fileName);
  if (!found) {
    if (!missingWarned.has(fileName)) {
      missingWarned.add(fileName);
      console.warn(
        `[api/_lib/data] ${fileName} not found. Looked relative to this module and under ` +
          `${DATA_CANDIDATES.join(', ')} from cwd=${process.cwd()}. ` +
          'Generated by `pnpm build:graph` / `pnpm build:security`; in a deployment it must be ' +
          'traced into the function by outputFileTracingIncludes in next.config.mjs.',
      );
    }
    throw new Error(`data file not found: ${fileName}`);
  }
  return JSON.parse(fs.readFileSync(found, 'utf8'));
}

export function loadGraph(): Graph {
  if (graphCache) return graphCache;
  let parsed: unknown;
  try {
    parsed = readJson('../_data/graph.json');
  } catch {
    parsed = null;
  }
  graphCache = normalizeGraph(parsed);
  return graphCache;
}

export function loadSymbols(): SymbolDoc[] {
  if (symbolsCache) return symbolsCache;
  let parsed: unknown;
  try {
    parsed = readJson('../_data/symbols.json');
  } catch {
    parsed = null;
  }
  symbolsCache = normalizeSymbols(parsed);
  return symbolsCache;
}

export function loadExamples(): Record<string, ExampleDoc> {
  if (examplesCache) return examplesCache;
  let parsed: unknown;
  try {
    parsed = readJson('../_data/examples.json');
  } catch {
    parsed = null;
  }
  examplesCache = normalizeExamples(parsed);
  return examplesCache;
}

export function loadParity(): ParityCorpus {
  if (parityCache) return parityCache;
  let parsed: unknown;
  try {
    parsed = readJson('../_data/parity.json');
  } catch {
    parsed = null;
  }
  parityCache = normalizeParity(parsed);
  return parityCache;
}

// Shape guard. The file is generated, but a stale or half-written artifact must
// degrade to "no parity data" rather than throw inside a request handler, and a
// library entry that is not an object must not poison the whole map.
function normalizeParity(parsed: unknown): ParityCorpus {
  const root = parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {};
  const generatedAt = typeof root.generatedAt === 'string' ? root.generatedAt : '';
  const raw = root.libraries;
  const libraries: Record<string, ParityLibrary> = {};
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    for (const [id, value] of Object.entries(raw as Record<string, unknown>)) {
      const lib = normalizeParityLibrary(value, id);
      if (lib) libraries[id] = lib;
    }
  }
  return { generatedAt, libraries };
}

function num(v: unknown): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

function normalizeParityLibrary(value: unknown, id: string): ParityLibrary | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const v = value as Record<string, unknown>;
  const upstream = (v.upstream && typeof v.upstream === 'object' ? v.upstream : {}) as Record<string, unknown>;
  const cov = (v.coverageSummary && typeof v.coverageSummary === 'object'
    ? v.coverageSummary
    : {}) as Record<string, unknown>;
  const nestedRaw = v.nested;
  let nested: Record<string, ParityLibrary> | undefined;
  if (nestedRaw && typeof nestedRaw === 'object' && !Array.isArray(nestedRaw)) {
    nested = {};
    for (const [k, sub] of Object.entries(nestedRaw as Record<string, unknown>)) {
      const lib = normalizeParityLibrary(sub, k);
      if (lib) nested[k] = lib;
    }
  }
  const out: ParityLibrary = {
    ...v,
    library: typeof v.library === 'string' ? v.library : id,
    harnessPath: typeof v.harnessPath === 'string' ? v.harnessPath : '',
    upstream: {
      raw: typeof upstream.raw === 'string' ? upstream.raw : '',
      name: typeof upstream.name === 'string' ? upstream.name : '',
      version: typeof upstream.version === 'string' ? upstream.version : '',
      ecosystem: typeof upstream.ecosystem === 'string' ? upstream.ecosystem : '',
    },
    goModule: typeof v.goModule === 'string' ? v.goModule : '',
    goPackage: typeof v.goPackage === 'string' ? v.goPackage : '',
    goVersion: typeof v.goVersion === 'string' ? v.goVersion : '',
    goVersionRaw: typeof v.goVersionRaw === 'string' ? v.goVersionRaw : '',
    generatedBy: typeof v.generatedBy === 'string' ? v.generatedBy : '',
    measuredAt: typeof v.measuredAt === 'string' ? v.measuredAt : '',
    total: num(v.total),
    match: num(v.match),
    mismatch: num(v.mismatch),
    deviations: num(v.deviations),
    parityPercent: num(v.parityPercent),
    comparedTotal: num(v.comparedTotal),
    perCaseVerdicts: v.perCaseVerdicts === true,
    groups: Array.isArray(v.groups) ? (v.groups as ParityGroup[]) : [],
    groupsDerived: v.groupsDerived === true,
    cases: Array.isArray(v.cases) ? (v.cases as ParityCase[]) : [],
    coverage: Array.isArray(v.coverage) ? (v.coverage as ParityCoverageRow[]) : [],
    coverageSummary: {
      match: num(cov.match),
      differs: num(cov.differs),
      missing: num(cov.missing),
      extra: num(cov.extra),
      untested: num(cov.untested),
      percent: num(cov.percent),
      total: num(cov.total),
    },
    coverageCommand: typeof v.coverageCommand === 'string' ? v.coverageCommand : '',
    security: Array.isArray(v.security) ? (v.security as ParitySecurityFinding[]) : [],
  };
  if (typeof v.package === 'string') out.package = v.package;
  if (nested && Object.keys(nested).length > 0) out.nested = nested;
  else delete out.nested;
  return out;
}

function normalizeExamples(parsed: unknown): Record<string, ExampleDoc> {
  const container =
    parsed && typeof parsed === 'object' && 'examples' in (parsed as object)
      ? (parsed as { examples?: unknown }).examples
      : parsed;
  if (!container || typeof container !== 'object') return {};
  const out: Record<string, ExampleDoc> = {};
  for (const [id, value] of Object.entries(container as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue;
    const v = value as Record<string, unknown>;
    out[id] = {
      library: typeof v.library === 'string' ? v.library : id,
      path: typeof v.path === 'string' ? v.path : '',
      code: typeof v.code === 'string' ? v.code : '',
      codeTruncated: v.codeTruncated === true,
      readme: typeof v.readme === 'string' ? v.readme : '',
      readmeTruncated: v.readmeTruncated === true,
    };
  }
  return out;
}

function normalizeGraph(parsed: unknown): Graph {
  const g = (parsed && typeof parsed === 'object' ? parsed : {}) as Record<string, unknown>;
  return {
    generatedAt: typeof g.generatedAt === 'string' ? g.generatedAt : '',
    libraries: Array.isArray(g.libraries) ? (g.libraries as GraphLibrary[]) : [],
    packages: Array.isArray(g.packages) ? (g.packages as GraphPackage[]) : [],
    edges: Array.isArray(g.edges) ? (g.edges as GraphEdge[]) : [],
  };
}

function normalizeSymbols(parsed: unknown): SymbolDoc[] {
  if (Array.isArray(parsed)) return parsed as SymbolDoc[];
  if (parsed && typeof parsed === 'object' && Array.isArray((parsed as { symbols?: unknown }).symbols)) {
    return (parsed as { symbols: SymbolDoc[] }).symbols;
  }
  return [];
}

export function getGraph(): Graph {
  return loadGraph();
}

export function getSymbols(): SymbolDoc[] {
  return loadSymbols();
}

export function getExamples(): Record<string, ExampleDoc> {
  return loadExamples();
}

// The whole measured parity corpus, keyed by library id (the parity/<id>/ dir
// name, which is also the docs/library id).
export function getParity(): ParityCorpus {
  return loadParity();
}

// The browser-sized projection of the corpus (see summarizeParity). Recomputed
// per call off the memoized corpus — it is a few thousand bytes of object churn
// over 40 libraries, not worth a second cache.
export function getParitySummary(): ParitySummary {
  return summarizeParity(loadParity());
}

// One library's measured parity, or null when the id is unknown (a placeholder
// port with no harness yet included). A nested package can be addressed as
// "express/qs" or "express/nested/qs".
export function getParityFor(libId: string): ParityLibrary | null {
  const raw = String(libId ?? '').trim();
  if (raw === '') return null;
  const parts = raw.split('/').filter((p) => p !== '' && p !== 'nested');
  if (parts.length === 0) return null;
  const { libraries } = loadParity();
  const rootId = parts[0].toLowerCase();
  if (!Object.prototype.hasOwnProperty.call(libraries, rootId)) return null;
  let lib: ParityLibrary = libraries[rootId];
  for (const pkg of parts.slice(1)) {
    const nested = lib.nested;
    if (!nested || !Object.prototype.hasOwnProperty.call(nested, pkg)) return null;
    lib = nested[pkg];
  }
  return lib;
}

// Look up a single runnable example by library id. Returns null when unknown.
export function getExample(library: string): ExampleDoc | null {
  const id = String(library ?? '').trim().toLowerCase();
  if (id === '') return null;
  const ex = loadExamples();
  return Object.prototype.hasOwnProperty.call(ex, id) ? ex[id] : null;
}
