#!/usr/bin/env node
// build-security-data.ts — collect the declared parity security findings into a
// single data file the site and the /ask assistant can read. Run from the repo
// root (Node 22.6+, via native type stripping):
//   node --experimental-strip-types scripts/build-security-data.ts
//
// Reads:   parity/<lib>/security.json
//          parity/<lib>/nested/<pkg>/security.json      (nested ports)
//          parity/<lib>[/nested/<pkg>]/parity.json      (best-effort, pinned upstream)
//          <lib>/VERSION                                (best-effort, released version)
// Writes:  api/_data/security.json
//
// Why a separate file from graph.json: the findings carry their FULL markdown
// description (thousands of characters each), which has no business in the
// package graph the Explore tab downloads. Only api/_data gets this file — it is
// read server-side (the /security page renderer and the chat tool) and embedded
// in the function bundle.
//
// FIXED-STATUS IS DERIVED, NEVER DECLARED. A manifest states the affected range
// ("<= 0.4.0"); the library's VERSION file states what is actually released. If
// the released version falls outside the affected range the finding is reported
// fixed as of that release, otherwise it is reported unfixed in the latest
// release. A range this script cannot parse, or a missing VERSION file, yields
// status "unknown" — never an optimistic guess. Every record carries the
// sentence explaining how its own status was reached, so the UI can show the
// derivation instead of asserting it.
//
// The output is DETERMINISTIC: findings are sorted by (severity, library,
// nested package, id) and nothing but the free-form `generatedAt` field is
// time-derived. Pin it with SECURITY_GENERATED_AT or `--generated-at <iso>` for
// reproducible builds.
//
// Malformed input never fails the build: an unreadable or invalid manifest is
// warned about and skipped, and the process still exits 0. A security page that
// disappears is better than a deploy that dies over one bad JSON file.
//
// This is developer tooling that runs under Node directly (not webpack), so it
// only uses erasable TypeScript (type annotations, interfaces, `as`) that Node's
// --experimental-strip-types removes at load time.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, '..');
const PARITY_DIR = path.join(REPO_ROOT, 'parity');
const OUT_DIR = path.join(REPO_ROOT, 'api', '_data');
const OUT_FILE = path.join(OUT_DIR, 'security.json');

// Directory names are repo paths; guard the listing against anything that is not
// a plausible library/package directory before it becomes part of an id.
const DIR_RE = /^[a-z0-9][a-z0-9._-]{0,63}$/;

// The upstream runner directory names the harness contract allows. Which one
// exists tells us the ecosystem the port is measured against.
const RUNNER_DIRS: [string, string][] = [
  ['node', 'npm'],
  ['python', 'PyPI'],
  ['java', 'Maven'],
  ['ruby', 'RubyGems'],
  ['rust', 'crates.io'],
  ['elixir', 'Hex'],
  ['c', 'C'],
];

const SEVERITIES = ['critical', 'high', 'medium', 'low'] as const;
type Severity = (typeof SEVERITIES)[number];
const SEVERITY_RANK: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 };

type Status = 'fixed' | 'unfixed' | 'unknown';

interface FindingOut {
  // identity
  id: string;
  library: string; // parity/<lib> directory name
  nestedPackage: string | null; // parity/<lib>/nested/<pkg> directory name
  /** "express" or "express/sanitizehtml" — how the port is named in prose. */
  port: string;
  repo: string;
  module: string; // Go module path the finding is against
  manifestPath: string; // repo-relative, for auditing
  // the finding, verbatim from the manifest
  severity: Severity;
  summary: string;
  affectedVersions: string;
  cases: string[];
  ghsa: string | null;
  description: string;
  /**
   * The manifest's "## Scope note" section, when it has one. These record
   * claims that were dropped or corrected after being reproduced against the
   * released module — the honest part of the record, surfaced rather than
   * buried in the middle of the description.
   */
  scopeNote: string | null;
  // upstream oracle (best-effort, from parity.json + the runner directory)
  upstream: string | null;
  upstreamEcosystem: string | null;
  // derived status
  releasedVersion: string | null;
  status: Status;
  fixedIn: string | null;
  statusReason: string;
}

interface LibraryOut {
  id: string;
  releasedVersion: string | null;
  findingCount: number;
}

// ---------------------------------------------------------------------------
// version + range arithmetic
// ---------------------------------------------------------------------------

interface Ver {
  parts: number[];
  pre: string;
}

// parseVersion accepts "0.4.0", "v0.4.0", "1.2", "0.4.0-rc.1". Returns null for
// anything else, which propagates to status "unknown".
function parseVersion(raw: string): Ver | null {
  const s = String(raw ?? '').trim().replace(/^v/i, '');
  const m = /^(\d+(?:\.\d+)*)(?:[-+]([0-9A-Za-z.-]+))?$/.exec(s);
  if (!m) return null;
  const parts = m[1].split('.').map((p) => Number(p));
  if (parts.some((n) => !Number.isFinite(n))) return null;
  return { parts, pre: m[2] ?? '' };
}

function cmpVersion(a: Ver, b: Ver): number {
  const n = Math.max(a.parts.length, b.parts.length);
  for (let i = 0; i < n; i++) {
    const x = a.parts[i] ?? 0;
    const y = b.parts[i] ?? 0;
    if (x !== y) return x < y ? -1 : 1;
  }
  // A prerelease sorts below the release it precedes (0.4.0-rc.1 < 0.4.0).
  if (a.pre === b.pre) return 0;
  if (a.pre === '') return 1;
  if (b.pre === '') return -1;
  return a.pre < b.pre ? -1 : 1;
}

// satisfies reports whether `version` falls inside `range`. The range is a
// comma-separated conjunction of comparator clauses, matching what the
// advisory pipeline sends GitHub as vulnerable_version_range:
//   "<= 0.4.0"        "< 1.0.0"       ">= 0.2.0, < 0.4.0"       "= 0.3.1"
// Returns null when any clause is not understood — the caller must then report
// "unknown" rather than assume either answer.
function satisfies(version: Ver, range: string): boolean | null {
  const clauses = String(range ?? '')
    .split(',')
    .map((c) => c.trim())
    .filter((c) => c !== '');
  if (clauses.length === 0) return null;
  for (const clause of clauses) {
    const m = /^(<=|>=|<|>|=|==)?\s*(.+)$/.exec(clause);
    if (!m) return null;
    const op = m[1] ?? '=';
    const bound = parseVersion(m[2]);
    if (!bound) return null;
    const c = cmpVersion(version, bound);
    const ok =
      op === '<=' ? c <= 0 :
      op === '>=' ? c >= 0 :
      op === '<' ? c < 0 :
      op === '>' ? c > 0 :
      c === 0;
    if (!ok) return false;
  }
  return true;
}

// ---------------------------------------------------------------------------
// repo reading (every read is best-effort)
// ---------------------------------------------------------------------------

function readTextOrNull(file: string): string | null {
  try {
    return fs.readFileSync(file, 'utf8');
  } catch {
    return null;
  }
}

function readJsonOrNull(file: string): unknown {
  const raw = readTextOrNull(file);
  if (raw === null) return null;
  try {
    return JSON.parse(raw);
  } catch (err) {
    console.warn(
      `build-security-data: ${path.relative(REPO_ROOT, file)} is not valid JSON ` +
      `(${err instanceof Error ? err.message : String(err)}) — skipped.`
    );
    return null;
  }
}

// The library's currently released version, from its VERSION file. This is the
// whole basis of the fixed/unfixed call, so it is read from the library itself
// and never from the manifest that declares the finding.
function releasedVersionFor(libDir: string): string | null {
  const raw = readTextOrNull(path.join(REPO_ROOT, libDir, 'VERSION'));
  if (raw === null) return null;
  const v = raw.trim().split(/\s+/)[0] ?? '';
  return v === '' ? null : v;
}

function upstreamFor(harnessDir: string): { upstream: string | null; ecosystem: string | null } {
  const parity = readJsonOrNull(path.join(harnessDir, 'parity.json'));
  let upstream: string | null = null;
  if (parity && typeof parity === 'object') {
    const u = (parity as Record<string, unknown>).upstream;
    if (typeof u === 'string' && u.trim() !== '') upstream = u.trim();
  }
  let ecosystem: string | null = null;
  for (const [dir, name] of RUNNER_DIRS) {
    if (fs.existsSync(path.join(harnessDir, dir))) {
      ecosystem = name;
      break;
    }
  }
  return { upstream, ecosystem };
}

// Pull a "## Scope note" (or "### Scope note") section out of a description,
// up to the next heading of the same-or-higher level.
function extractScopeNote(description: string): string | null {
  const m = /^(#{2,3})\s*Scope note\s*$/im.exec(description);
  if (!m) return null;
  const start = m.index + m[0].length;
  const rest = description.slice(start);
  const next = /^#{1,3}\s+\S/m.exec(rest);
  const body = (next ? rest.slice(0, next.index) : rest).trim();
  return body === '' ? null : body;
}

interface Manifest {
  harnessDir: string; // absolute
  manifestPath: string; // repo-relative
  library: string; // parity/<lib>
  nestedPackage: string | null;
  raw: Record<string, unknown>;
}

// Every security.json in the parity tree: one per library harness, plus one per
// nested-package harness. Sorted so the output ordering never depends on the
// filesystem's.
function findManifests(): Manifest[] {
  const out: Manifest[] = [];
  let libs: fs.Dirent[];
  try {
    libs = fs.readdirSync(PARITY_DIR, { withFileTypes: true });
  } catch {
    console.warn('build-security-data: no parity/ directory — nothing to collect.');
    return out;
  }
  const libNames = libs
    .filter((e) => e.isDirectory() && DIR_RE.test(e.name))
    .map((e) => e.name)
    .sort();

  for (const lib of libNames) {
    const libHarness = path.join(PARITY_DIR, lib);
    const own = readJsonOrNull(path.join(libHarness, 'security.json'));
    if (own && typeof own === 'object' && !Array.isArray(own)) {
      out.push({
        harnessDir: libHarness,
        manifestPath: path.posix.join('parity', lib, 'security.json'),
        library: lib,
        nestedPackage: null,
        raw: own as Record<string, unknown>,
      });
    }

    const nestedRoot = path.join(libHarness, 'nested');
    let nested: fs.Dirent[] = [];
    try {
      nested = fs.readdirSync(nestedRoot, { withFileTypes: true });
    } catch {
      continue; // no nested harnesses for this library
    }
    const pkgNames = nested
      .filter((e) => e.isDirectory() && DIR_RE.test(e.name))
      .map((e) => e.name)
      .sort();
    for (const pkg of pkgNames) {
      const dir = path.join(nestedRoot, pkg);
      const m = readJsonOrNull(path.join(dir, 'security.json'));
      if (!m || typeof m !== 'object' || Array.isArray(m)) continue;
      out.push({
        harnessDir: dir,
        manifestPath: path.posix.join('parity', lib, 'nested', pkg, 'security.json'),
        library: lib,
        nestedPackage: pkg,
        raw: m as Record<string, unknown>,
      });
    }
  }
  return out;
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

function generatedAtStamp(): string {
  const i = process.argv.indexOf('--generated-at');
  const pinned = (i >= 0 ? process.argv[i + 1] : undefined) ?? process.env.SECURITY_GENERATED_AT;
  return pinned && String(pinned).trim() !== '' ? String(pinned).trim() : new Date().toISOString();
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

function main(): void {
  const generatedAt = generatedAtStamp();
  const manifests = findManifests();

  const findings: FindingOut[] = [];
  const versionByLib = new Map<string, string | null>();
  const skipped: string[] = [];

  for (const m of manifests) {
    const rawFindings = Array.isArray(m.raw.findings) ? m.raw.findings : [];
    if (rawFindings.length === 0) {
      console.warn(`build-security-data: ${m.manifestPath} declares no findings — skipped.`);
      continue;
    }
    const repo = str(m.raw.repo) || m.library;
    // `module` is the Go module the finding is against: for a nested port the
    // manifest's `package` (github.com/malcolmston/express/sanitizehtml) is more
    // precise than the parent module.
    const module = str(m.raw.package) || str(m.raw.module) || `github.com/malcolmston/${repo}`;
    if (!versionByLib.has(m.library)) versionByLib.set(m.library, releasedVersionFor(m.library));
    const released = versionByLib.get(m.library) ?? null;
    const { upstream, ecosystem } = upstreamFor(m.harnessDir);
    const port = m.nestedPackage ? `${m.library}/${m.nestedPackage}` : m.library;

    for (const raw of rawFindings) {
      if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
        skipped.push(`${m.manifestPath}: a finding is not an object`);
        continue;
      }
      const f = raw as Record<string, unknown>;
      const id = str(f.id).trim();
      const summary = str(f.summary).trim();
      const severityRaw = str(f.severity).trim().toLowerCase();
      const affectedVersions = str(f.affectedVersions).trim();
      const description = str(f.description);
      // The same fields the advisory pipeline validates. A finding missing any
      // of them cannot be rendered honestly, so it is reported and dropped.
      const problems: string[] = [];
      if (id === '') problems.push('missing id');
      if (summary === '') problems.push('missing summary');
      if (!(SEVERITIES as readonly string[]).includes(severityRaw)) problems.push(`bad severity "${severityRaw}"`);
      if (affectedVersions === '') problems.push('missing affectedVersions');
      if (description.trim() === '') problems.push('missing description');
      if (problems.length > 0) {
        skipped.push(`${m.manifestPath}: ${id || summary || '<unnamed>'} (${problems.join('; ')})`);
        continue;
      }

      // --- derive the fixed status --------------------------------------
      let status: Status = 'unknown';
      let fixedIn: string | null = null;
      let statusReason: string;
      if (released === null) {
        statusReason =
          `No ${m.library}/VERSION file on disk, so the released version is unknown and ` +
          `this finding's status against the latest release cannot be derived.`;
      } else {
        const rv = parseVersion(released);
        const hit = rv ? satisfies(rv, affectedVersions) : null;
        if (rv === null) {
          statusReason =
            `${m.library}/VERSION reads "${released}", which is not a version this script ` +
            `can compare, so the status against the latest release cannot be derived.`;
        } else if (hit === null) {
          statusReason =
            `The affected range "${affectedVersions}" is not a comparator range this script ` +
            `can evaluate, so the status of released ${port} ${released} cannot be derived.`;
        } else if (hit) {
          status = 'unfixed';
          statusReason =
            `The released version of ${port} is ${released} (${m.library}/VERSION), which is ` +
            `inside the affected range "${affectedVersions}" — so the latest release is still affected.`;
        } else {
          status = 'fixed';
          fixedIn = released;
          statusReason =
            `The released version of ${port} is ${released} (${m.library}/VERSION), which is ` +
            `outside the affected range "${affectedVersions}" — so the finding is fixed as of ${released}.`;
        }
      }

      const cases = Array.isArray(f.cases)
        ? f.cases.filter((c): c is string => typeof c === 'string' && c.trim() !== '').map((c) => c.trim())
        : [];

      findings.push({
        id,
        library: m.library,
        nestedPackage: m.nestedPackage,
        port,
        repo,
        module,
        manifestPath: m.manifestPath,
        severity: severityRaw as Severity,
        summary,
        affectedVersions,
        cases,
        ghsa: str(f.ghsa).trim() || (/^GHSA-/i.test(id) ? id : null),
        description,
        scopeNote: extractScopeNote(description),
        upstream,
        upstreamEcosystem: ecosystem,
        releasedVersion: released,
        status,
        fixedIn,
        statusReason,
      });
    }
  }

  // Deterministic order: severity (critical first), then port, then id.
  findings.sort(
    (a, b) =>
      (SEVERITY_RANK[a.severity] ?? 99) - (SEVERITY_RANK[b.severity] ?? 99) ||
      (a.port < b.port ? -1 : a.port > b.port ? 1 : 0) ||
      (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
  );

  const libraries: LibraryOut[] = Array.from(versionByLib.keys())
    .sort()
    .map((id) => ({
      id,
      releasedVersion: versionByLib.get(id) ?? null,
      findingCount: findings.filter((f) => f.library === id).length,
    }))
    .filter((l) => l.findingCount > 0);

  const bySeverity: Record<string, number> = {};
  for (const s of SEVERITIES) bySeverity[s] = findings.filter((f) => f.severity === s).length;
  const counts = {
    total: findings.length,
    bySeverity,
    fixed: findings.filter((f) => f.status === 'fixed').length,
    unfixed: findings.filter((f) => f.status === 'unfixed').length,
    unknown: findings.filter((f) => f.status === 'unknown').length,
    libraries: libraries.length,
  };

  const out = {
    generatedAt,
    // How the status column was reached, carried with the data so the UI can
    // state its own derivation instead of inventing one.
    statusDerivation:
      'Fixed status is derived, not declared: the affected range in ' +
      'parity/<lib>[/nested/<pkg>]/security.json is evaluated against the version in the ' +
      "library's own VERSION file. A released version outside the affected range means fixed " +
      'as of that release; inside it means unfixed in the latest release; an unparseable range ' +
      'or a missing VERSION file means unknown.',
    counts,
    libraries,
    findings,
  };

  fs.mkdirSync(OUT_DIR, { recursive: true });
  fs.writeFileSync(OUT_FILE, JSON.stringify(out, null, 2) + '\n');

  const sevStr = SEVERITIES.map((s) => `${s}=${bySeverity[s]}`).join(' ');
  console.log(
    `build-security-data: ${counts.total} findings across ${counts.libraries} librar` +
    `${counts.libraries === 1 ? 'y' : 'ies'} from ${manifests.length} manifest` +
    `${manifests.length === 1 ? '' : 's'} (${sevStr}; fixed=${counts.fixed} ` +
    `unfixed=${counts.unfixed} unknown=${counts.unknown}). Wrote security.json to api/_data.`
  );
  if (skipped.length > 0) {
    console.warn(
      `build-security-data: skipped ${skipped.length} malformed finding` +
      `${skipped.length === 1 ? '' : 's'}:\n  ${skipped.join('\n  ')}`
    );
  }
}

main();
