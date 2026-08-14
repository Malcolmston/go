// scripts/subrepo/gen-passport-manifest.ts — generate passport/strategies.json.
//
// The manifest is the map for the strategy subrepo split: every strategy under
// passport/strategies/<name> becomes its own repository, github.com/malcolmston/
// passport-<name>, held by the passport repo as a git submodule at its original
// path. Naming follows the convention the parity harness already uses for split
// subpackages (go-parity/passport-httpauth, go-parity/express-nanoid).
//
// This is GENERATED, never hand-edited. Five separate hand-maintained copies of
// "what is in this family" (.gitmodules, go.work, library-tests.yml,
// sync-submodules.yml, .vercelignore) have already drifted apart in this repo —
// a sixth, listing 154 entries, would drift within a week. Regenerate with:
//
//   node --experimental-strip-types scripts/subrepo/gen-passport-manifest.ts
//
// It reads the working tree, so the passport submodule must be checked out
// (`git submodule update --init passport`).
//
// WHY THE MANIFEST MATTERS BEYOND THE MIGRATION
// --------------------------------------------
// Go resolves module paths from the repository URL, so once a strategy lives in
// its own repo its import path necessarily changes:
//
//   github.com/malcolmston/passport/strategies/github   (before)
//   github.com/malcolmston/passport-github              (after)
//
// Git submodules are invisible to the Go module resolver, and the module zip the
// proxy builds for passport excludes both gitlink contents and any subdirectory
// carrying its own go.mod. So after the split `go get github.com/malcolmston/
// passport` no longer ships strategies at all — each is fetched on its own. That
// makes an explicit, machine-readable map of name -> repo -> import path the only
// way the set stays discoverable. `legacyImport` is recorded on every entry so
// consumers (and the parity harness) can mechanically rewrite their imports.

import fs from 'node:fs';
import path from 'node:path';

const REPO_ROOT = path.resolve(import.meta.dirname, '..', '..');
const PASSPORT = path.join(REPO_ROOT, 'passport');
const STRATEGIES = path.join(PASSPORT, 'strategies');
const OUT = path.join(PASSPORT, 'strategies.json');

const OWNER = 'malcolmston';
const CORE_MODULE = `github.com/${OWNER}/passport`;
/** Import prefix that identifies a sibling strategy in the pre-split tree. */
const LEGACY_PREFIX = `${CORE_MODULE}/strategies/`;

/** `passport-github`, matching the go-parity/<lib>-<sub> convention. */
const repoName = (strategy: string) => `passport-${strategy}`;
const modulePath = (strategy: string) => `github.com/${OWNER}/${repoName(strategy)}`;

export interface StrategyEntry {
  name: string;
  repo: string;
  module: string;
  legacyImport: string;
  submodulePath: string;
  /** 1 = depends only on passport core; 2 = also depends on sibling strategies. */
  layer: 1 | 2;
  /** Module paths this strategy must `require`, core first then siblings. */
  requires: string[];
  goFiles: number;
  lines: number;
}

/** Strategy directories, excluding the loose *_test.go files at strategies/ root. */
export function readStrategyNames(dir: string = STRATEGIES): string[] {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => e.name)
    .sort();
}

/**
 * Sibling strategies imported by `name`, read from its Go source.
 *
 * Deliberately a source scan rather than `go list`: this has to run without a Go
 * toolchain (and before the split, when the module graph is still one module), and
 * an import line is unambiguous enough that a parser buys nothing here.
 */
export function siblingDeps(name: string, dir: string = STRATEGIES): string[] {
  const strategyDir = path.join(dir, name);
  const deps = new Set<string>();
  for (const file of fs.readdirSync(strategyDir)) {
    if (!file.endsWith('.go')) continue;
    const src = fs.readFileSync(path.join(strategyDir, file), 'utf8');
    for (const m of src.matchAll(/"(?:[a-z0-9./-]+)"/g)) {
      const imported = m[0].slice(1, -1);
      if (!imported.startsWith(LEGACY_PREFIX)) continue;
      const sibling = imported.slice(LEGACY_PREFIX.length).split('/')[0];
      if (sibling && sibling !== name) deps.add(sibling);
    }
  }
  return [...deps].sort();
}

function countSource(name: string, dir: string = STRATEGIES) {
  const files = fs.readdirSync(path.join(dir, name)).filter((f) => f.endsWith('.go'));
  const lines = files.reduce(
    (n, f) => n + fs.readFileSync(path.join(dir, name, f), 'utf8').split('\n').length,
    0,
  );
  return { goFiles: files.length, lines };
}

export function buildEntries(dir: string = STRATEGIES): StrategyEntry[] {
  return readStrategyNames(dir).map((name) => {
    const deps = siblingDeps(name, dir);
    return {
      name,
      repo: `${OWNER}/${repoName(name)}`,
      module: modulePath(name),
      legacyImport: `${LEGACY_PREFIX}${name}`,
      submodulePath: `strategies/${name}`,
      layer: deps.length === 0 ? 1 : 2,
      requires: [CORE_MODULE, ...deps.map(modulePath)],
      ...countSource(name, dir),
    };
  });
}

/**
 * Creation order. Layer 1 carries no sibling edges and must exist and be tagged
 * before layer 2 can pin it, so the split is a two-wave rollout, not 154
 * independent pushes.
 */
export function creationOrder(entries: StrategyEntry[]): string[][] {
  return [
    entries.filter((e) => e.layer === 1).map((e) => e.name),
    entries.filter((e) => e.layer === 2).map((e) => e.name),
  ];
}

function main(): void {
  if (!fs.existsSync(STRATEGIES)) {
    console.error(
      `passport/strategies not found at ${STRATEGIES}\n` +
        'The passport submodule is not checked out. Run:\n' +
        '  git submodule update --init passport',
    );
    process.exit(1);
  }

  const entries = buildEntries();
  const [layer1, layer2] = creationOrder(entries);
  const coreVersion = fs.readFileSync(path.join(PASSPORT, 'VERSION'), 'utf8').trim();

  const manifest = {
    // Generated — see scripts/subrepo/gen-passport-manifest.ts in malcolmston/go.
    generator: 'malcolmston/go: scripts/subrepo/gen-passport-manifest.ts',
    core: { module: CORE_MODULE, version: `v${coreVersion}` },
    convention: {
      repo: `${OWNER}/passport-<strategy>`,
      module: `github.com/${OWNER}/passport-<strategy>`,
      submodulePath: 'strategies/<strategy>',
      note:
        'Import paths change with the split: a separate repository cannot serve ' +
        `${LEGACY_PREFIX}<strategy>. Every entry records legacyImport for mechanical rewrites.`,
    },
    counts: { total: entries.length, layer1: layer1.length, layer2: layer2.length },
    creationOrder: { layer1, layer2 },
    strategies: entries,
  };

  fs.writeFileSync(OUT, `${JSON.stringify(manifest, null, 2)}\n`);
  console.log(
    `wrote ${path.relative(REPO_ROOT, OUT)}: ${entries.length} strategies ` +
      `(layer 1: ${layer1.length}, layer 2: ${layer2.length}), core ${manifest.core.version}`,
  );
}

if (process.argv[1] === import.meta.filename) main();
