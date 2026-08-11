// Server-side loader for the generated parity dataset.
//
// api/_data/parity.json is written by the repo's data build from the harnesses
// under parity/ (see parity/HARNESS.md). It is read here with fs at
// build/prerender time — never shipped to the browser wholesale — so the static
// export and the Vercel build both get the same numbers without a runtime API
// call. A missing file is not an error: the page renders an explanatory empty
// state instead of failing the build.
//
// The corpus is the ONLY accepted source. public/parity.json looks similar but
// is the small browser summary (percentages only, no cases/coverage), and
// feeding it in here would build pages that print headline "measured" numbers
// with no evidence behind them — worse than an empty state. isParityCorpus
// below rejects anything that is not the full corpus.

import fs from 'node:fs';
import path from 'node:path';
import { LIBS } from '../../src/data';
import { repoKey } from '../../src/parityLookup';
import type { ParityData, ParityHarness } from '../../src/components/ParityData';
import { normalizeParityData, summarize } from '../../src/components/ParityData';
import type { ParitySummary } from '../../src/components/ParityData';

// The generated corpus. There is deliberately no fallback: see the note above.
const CORPUS = path.join(process.cwd(), 'api', '_data', 'parity.json');

/**
 * Does this parsed blob look like the full parity corpus, as opposed to the
 * browser summary (or any other `libraries` map)? A corpus harness carries
 * per-case evidence: `cases` / `coverage` arrays and a `harnessPath`. The
 * summary carries none of those, so it is rejected and the pages fall back to
 * their empty state rather than showing unbacked percentages.
 */
export function isParityCorpus(parsed: unknown): boolean {
  if (!parsed || typeof parsed !== 'object') return false;
  const libraries = (parsed as { libraries?: unknown }).libraries;
  if (!libraries || typeof libraries !== 'object') return false;
  return Object.values(libraries as Record<string, unknown>).some((entry) => {
    if (!entry || typeof entry !== 'object') return false;
    const h = entry as { cases?: unknown; coverage?: unknown; harnessPath?: unknown };
    return (
      Array.isArray(h.cases) ||
      Array.isArray(h.coverage) ||
      (typeof h.harnessPath === 'string' && h.harnessPath.length > 0)
    );
  });
}

let cache: ParityData | null = null;

export function loadParityData(): ParityData {
  if (cache) return cache;
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(fs.readFileSync(CORPUS, 'utf8'));
  } catch {
    // absent or unreadable: the empty state below is the honest answer
  }
  if (!isParityCorpus(parsed)) {
    if (parsed !== null) {
      console.warn(
        `parity: ${CORPUS} is not the generated corpus (no cases/coverage/harnessPath); rendering the empty state`,
      );
    }
    parsed = null;
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
