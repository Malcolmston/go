#!/usr/bin/env node
// index-symbols.ts — (re)build the Upstash Search index for symbol search.
//
// The symbol corpus (api/_data/symbols.json) is static, generated at build
// time by build-graph-data.ts. Rather than index it lazily on the first search
// request (slow cold starts, redundant writes under serverless isolation), we
// load it into Upstash once here — after the initial deploy and whenever the
// symbol data changes.
//
//   pnpm index:symbols
//   # or: node --experimental-strip-types scripts/index-symbols.ts
//
// Env: reads UPSTASH_SEARCH_REST_URL / UPSTASH_SEARCH_REST_TOKEN from the
// process env, or from .env.local / .env in the repo root (run `vercel env
// pull` first to fetch them locally). In CI, set them as job env vars.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { esEnabled, esIndexAll } from '../api/_lib/es.ts';
import type { SymbolDoc } from '../api/_lib/data.ts';

// Minimal .env loader (no dotenv dependency). Already-set env vars win; then
// .env.local, then .env. Strips surrounding quotes that `vercel env pull` adds.
function loadEnvFile(file: string): void {
  if (!fs.existsSync(file)) return;
  for (const raw of fs.readFileSync(file, 'utf8').split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq === -1) continue;
    const key = line.slice(0, eq).trim();
    if (process.env[key] !== undefined) continue;
    let val = line.slice(eq + 1).trim();
    val = val.replace(/^"(.*)"$/, '$1').replace(/^'(.*)'$/, '$1');
    process.env[key] = val;
  }
}

async function main(): Promise<void> {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  loadEnvFile(path.join(root, '.env.local'));
  loadEnvFile(path.join(root, '.env'));

  if (!esEnabled()) {
    console.error(
      'Upstash Search is not configured. Set UPSTASH_SEARCH_REST_URL and\n' +
        'UPSTASH_SEARCH_REST_TOKEN (run `vercel env pull` to fetch them locally).',
    );
    process.exit(1);
  }

  const symbolsPath = path.join(root, 'api', '_data', 'symbols.json');
  const parsed = JSON.parse(fs.readFileSync(symbolsPath, 'utf8')) as { symbols?: SymbolDoc[] };
  const symbols = parsed.symbols ?? [];
  if (symbols.length === 0) {
    console.error(`No symbols found in ${symbolsPath}. Run \`pnpm build:graph\` first.`);
    process.exit(1);
  }

  const started = Date.now();
  console.log(`Indexing ${symbols.length} symbols into Upstash Search…`);
  const total = await esIndexAll(symbols, (done, all) => {
    if (done % 1000 === 0 || done === all) {
      process.stdout.write(`\r  ${done}/${all}`);
    }
  });
  const secs = ((Date.now() - started) / 1000).toFixed(1);
  console.log(`\n✓ Upserted ${total} symbols in ${secs}s.`);
  console.log('Note: Upstash documentCount is eventually consistent and may take a moment to catch up.');
}

main().catch((err) => {
  console.error('\nIndexing failed:', err);
  process.exit(1);
});
