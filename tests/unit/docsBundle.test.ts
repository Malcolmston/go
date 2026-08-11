import { describe, it, expect } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { LIBS } from '../../src/data';

// public/docs/<repo>.json is the gendocs output bundled into the site. It is an
// INPUT to the web build, not an output of it: scripts/build-graph-data.ts reads
// the directory to derive the whole corpus (graph, search index, symbols), and
// src/components/LibView.tsx fetches `docs/<repo>.json` straight from the
// browser for the API-reference tab. Vercel has neither Go nor the library
// submodules (.vercelignore drops them), so it cannot regenerate the directory —
// whatever is committed is all the deployment ever sees.
//
// That is why this file is checked: five libraries (jose, jq, markdown, rrule,
// yaml) once existed on disk but were never `git add`ed, so a fresh checkout
// carried 33 of the 38 libraries and the Vercel build silently shipped a site
// that under-reported its own contents. These assertions run against the real
// working tree, so on a checkout that is missing a doc bundle they fail.
const DOCS_DIR = path.join(__dirname, '..', '..', 'public', 'docs');

// Mirrors docKey() in src/components/LibView.tsx: the bundle is named after the
// last segment of the repo URL, which is not always the library id (socketio ->
// socket.io).
const docKey = (repo: string): string => repo.replace(/\/+$/, '').split('/').pop() ?? '';

const bundles = fs
  .readdirSync(DOCS_DIR)
  // gendocs also writes an index.json summary alongside the per-library files;
  // build-graph-data.ts skips it and no component fetches it.
  .filter((f) => f.endsWith('.json') && f !== 'index.json')
  .map((f) => f.slice(0, -'.json'.length))
  .sort();

describe('public/docs bundles', () => {
  it('ships one doc bundle per library in LIBS', () => {
    const expected = LIBS.map((lib) => docKey(lib.repo)).sort();
    expect(bundles).toEqual(expected);
  });

  it('covers the five libraries that were missing from the committed tree', () => {
    for (const lib of ['jose', 'jq', 'markdown', 'rrule', 'yaml']) {
      expect(bundles).toContain(lib);
    }
  });

  it.each(LIBS.map((lib) => [lib.id, docKey(lib.repo)] as const))(
    '%s has a non-empty gendocs bundle',
    (_id, key) => {
      const raw = fs.readFileSync(path.join(DOCS_DIR, `${key}.json`), 'utf8');
      const doc = JSON.parse(raw) as { module?: unknown; packages?: unknown };
      expect(typeof doc.module).toBe('string');
      expect(Array.isArray(doc.packages)).toBe(true);
      expect((doc.packages as unknown[]).length).toBeGreaterThan(0);
    },
  );
});
