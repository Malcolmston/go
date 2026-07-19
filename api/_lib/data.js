// data.js — loads and memoizes the generated graph + symbols data files.
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

let graphCache = null;
let symbolsCache = null;

function readJson(relativePath) {
  const url = new URL(relativePath, import.meta.url);
  const raw = fs.readFileSync(url, 'utf8');
  return JSON.parse(raw);
}

export function loadGraph() {
  if (graphCache) return graphCache;
  let parsed;
  try {
    parsed = readJson('../_data/graph.json');
  } catch {
    parsed = null;
  }
  graphCache = normalizeGraph(parsed);
  return graphCache;
}

export function loadSymbols() {
  if (symbolsCache) return symbolsCache;
  let parsed;
  try {
    parsed = readJson('../_data/symbols.json');
  } catch {
    parsed = null;
  }
  symbolsCache = normalizeSymbols(parsed);
  return symbolsCache;
}

function normalizeGraph(parsed) {
  const g = parsed && typeof parsed === 'object' ? parsed : {};
  return {
    generatedAt: typeof g.generatedAt === 'string' ? g.generatedAt : '',
    libraries: Array.isArray(g.libraries) ? g.libraries : [],
    packages: Array.isArray(g.packages) ? g.packages : [],
    edges: Array.isArray(g.edges) ? g.edges : [],
  };
}

function normalizeSymbols(parsed) {
  if (Array.isArray(parsed)) return parsed;
  if (parsed && Array.isArray(parsed.symbols)) return parsed.symbols;
  return [];
}

export function getGraph() {
  return loadGraph();
}

export function getSymbols() {
  return loadSymbols();
}
