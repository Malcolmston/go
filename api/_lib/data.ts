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

import fs from 'node:fs';

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

let graphCache: Graph | null = null;
let symbolsCache: SymbolDoc[] | null = null;
let examplesCache: Record<string, ExampleDoc> | null = null;

function readJson(relativePath: string): unknown {
  const url = new URL(relativePath, import.meta.url);
  const raw = fs.readFileSync(url, 'utf8');
  return JSON.parse(raw);
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

// Look up a single runnable example by library id. Returns null when unknown.
export function getExample(library: string): ExampleDoc | null {
  const id = String(library ?? '').trim().toLowerCase();
  if (id === '') return null;
  const ex = loadExamples();
  return Object.prototype.hasOwnProperty.call(ex, id) ? ex[id] : null;
}
