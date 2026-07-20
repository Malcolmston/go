#!/usr/bin/env node
// es-reindex.ts — (re)seed the Elasticsearch `symbols` index that backs the docs
// search, from the generated api/_data/symbols.json. Run once after pointing the
// app at a cluster (or after the docs change) so search is ready without paying
// the lazy first-query index build:
//
//   ELASTICSEARCH_URL=http://localhost:9200 pnpm es:reindex
//   ELASTICSEARCH_URL=... ELASTICSEARCH_API_KEY=... pnpm es:reindex   (secured)
//
// It mirrors the mapping + bulk load in api/_lib/es.ts (doIndex) but deletes and
// rebuilds the index for a clean reseed. Self-contained (only node builtins +
// fetch) so it runs under `node --experimental-strip-types`. Run `pnpm
// build:graph` first if api/_data/symbols.json is missing.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SYMBOLS = path.join(__dirname, '..', 'api', '_data', 'symbols.json');
const INDEX = 'symbols';
const CHUNK = 1000;

interface SymbolDoc {
  id: string;
  name?: string;
  kind?: string;
  packageImportPath?: string;
  library?: string;
  signature?: string | null;
  doc?: string | null;
  anchor?: string | null;
}

function base(): string {
  const url = process.env.ELASTICSEARCH_URL;
  if (!url || url.trim() === '') {
    console.error('es-reindex: ELASTICSEARCH_URL is not set; nothing to do.');
    process.exit(1);
  }
  return url.replace(/\/+$/, '');
}

function headers(overrides: Record<string, string> = {}): Record<string, string> {
  const h: Record<string, string> = { 'Content-Type': 'application/json', ...overrides };
  const key = process.env.ELASTICSEARCH_API_KEY;
  if (key) h.Authorization = `ApiKey ${key}`;
  return h;
}

function loadSymbols(): SymbolDoc[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(SYMBOLS, 'utf8'));
  } catch (err) {
    console.error(`es-reindex: cannot read ${SYMBOLS}: ${(err as Error).message}`);
    console.error('es-reindex: run `pnpm build:graph` first.');
    process.exit(1);
  }
  if (Array.isArray(parsed)) return parsed as SymbolDoc[];
  if (parsed && typeof parsed === 'object' && Array.isArray((parsed as { symbols?: unknown }).symbols)) {
    return (parsed as { symbols: SymbolDoc[] }).symbols;
  }
  return [];
}

const MAPPING = {
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

async function main(): Promise<void> {
  const b = base();
  const symbols = loadSymbols();
  if (symbols.length === 0) {
    console.error('es-reindex: symbols.json is empty; run `pnpm build:graph`.');
    process.exit(1);
  }

  // Clean reseed: drop the index if it exists, then recreate with the mapping.
  await fetch(`${b}/${INDEX}`, { method: 'DELETE', headers: headers() }).catch(() => {});
  const create = await fetch(`${b}/${INDEX}`, {
    method: 'PUT',
    headers: headers(),
    body: JSON.stringify(MAPPING),
  });
  if (!create.ok && create.status !== 400) {
    throw new Error(`create index failed: ${create.status} ${await create.text()}`);
  }

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
    const resp = await fetch(`${b}/_bulk${refresh}`, {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/x-ndjson' }),
      body: ndjson,
    });
    if (!resp.ok) throw new Error(`bulk index failed: ${resp.status} ${await resp.text()}`);
    const result = (await resp.json()) as { errors?: boolean };
    if (result && result.errors) throw new Error('bulk index reported item errors');
  }
  console.log(`es-reindex: loaded ${symbols.length} symbols into "${INDEX}" at ${b}`);
}

main().catch((err) => {
  console.error(`es-reindex failed: ${(err as Error).message}`);
  process.exit(1);
});
