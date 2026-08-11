// Server-side loader for the generated parity dataset.
//
// api/_data/parity.json is written by the repo's data build from the harnesses
// under parity/ (see parity/HARNESS.md). It is read here with fs at
// build/prerender time — never shipped to the browser wholesale — so the static
// export and the Vercel build both get the same numbers without a runtime API
// call. A missing file is not an error: the page renders an explanatory empty
// state instead of failing the build.

import fs from 'node:fs';
import path from 'node:path';
import { LIBS } from '../../src/data';
import { repoKey } from '../../src/parityLookup';
import type { ParityData, ParityHarness } from '../../src/components/ParityData';
import { normalizeParityData, summarize } from '../../src/components/ParityData';
import type { ParitySummary } from '../../src/components/ParityData';

// Candidate locations, most authoritative first. api/_data is the generated
// home; public/ is where the same file is mirrored for the static site.
const CANDIDATES = [
  path.join(process.cwd(), 'api', '_data', 'parity.json'),
  path.join(process.cwd(), 'public', 'parity.json'),
  path.join(process.cwd(), 'public', 'docs', 'parity.json'),
];

let cache: ParityData | null = null;

export function loadParityData(): ParityData {
  if (cache) return cache;
  let parsed: unknown = null;
  for (const file of CANDIDATES) {
    try {
      parsed = JSON.parse(fs.readFileSync(file, 'utf8'));
      break;
    } catch {
      // try the next candidate
    }
  }
  cache = normalizeParityData(parsed);
  return cache;
}

/** The site's display name + accent for a harness key (a parity/ directory). */
export function libMeta(slug: string): { name: string; accent: string; repo: string | null } {
  const lib = LIBS.find((l) => l.id === slug) ?? LIBS.find((l) => repoKey(l) === slug);
  return lib
    ? { name: lib.name, accent: lib.accent, repo: lib.repo }
    : { name: slug, accent: '#6b7280', repo: null };
}

/** The /lib/<id> route for a harness key, when the site has that library. */
export function libRoute(slug: string): string | null {
  const lib = LIBS.find((l) => l.id === slug) ?? LIBS.find((l) => repoKey(l) === slug);
  return lib ? `/lib/${encodeURIComponent(lib.id)}` : null;
}

/** Every harness key, sorted for stable static params + index order. */
export function paritySlugs(): string[] {
  return Object.keys(loadParityData().libraries).sort((a, b) => a.localeCompare(b));
}

/** One harness by key, tolerating the site slug (socketio) vs dir (socket.io). */
export function parityHarness(slug: string): { slug: string; harness: ParityHarness } | null {
  const { libraries } = loadParityData();
  if (Object.prototype.hasOwnProperty.call(libraries, slug)) {
    return { slug, harness: libraries[slug] };
  }
  const lib = LIBS.find((l) => l.id === slug);
  const alt = lib ? repoKey(lib) : '';
  if (alt && Object.prototype.hasOwnProperty.call(libraries, alt)) {
    return { slug: alt, harness: libraries[alt] };
  }
  return null;
}

/** Index rows: the whole dataset minus the per-case and per-symbol bulk. */
export function paritySummaries(): ParitySummary[] {
  const { libraries } = loadParityData();
  return Object.entries(libraries)
    .map(([slug, h]) => {
      const meta = libMeta(slug);
      return summarize(slug, h, meta.name, meta.accent);
    })
    .sort((a, b) => a.name.localeCompare(b.name));
}
