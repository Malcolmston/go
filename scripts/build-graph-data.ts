#!/usr/bin/env node
// build-graph-data.ts — generate the package-connection graph + search-symbol
// index that back the search endpoint, the GraphQL API and the frontend Explore
// tab. Run from the repo root (Node 22.6+, via native type stripping):
//   node --experimental-strip-types scripts/build-graph-data.ts
//
// Reads:   public/docs/*.json    (DocIndex per library)
//          src/parity.ts         (best-effort, for parityAfter)
//          <library source dirs>  (best-effort, for real import edges)
//          parity/**/{parity.json,COVERAGE.md,cases/*.json,security.json}
// Writes:  api/_data/graph.json      + public/graph.json
//          api/_data/symbols.json    (full-fat, server-side ranking)
//          public/search-index.json  (columnar browser projection of it —
//                                     name/kind/package/library/anchor only)
//          api/_data/parity.json     (measured parity corpus — see below)
//          public/parity.json        (the browser-sized summary of that corpus,
//                                     fetched by Explore)
//
// The output is DETERMINISTIC: everything is sorted by stable ids and no
// Date.now/Math.random is used for ordering. Only the free-form `generatedAt`
// timestamp field is time-derived (and is never used as a sort key); it can be
// pinned via the GRAPH_GENERATED_AT env var or `--generated-at <iso>` for fully
// reproducible builds.
//
// This is developer tooling that runs under Node directly (not webpack), so it
// only uses erasable TypeScript (type annotations, interfaces, `as`) that Node's
// --experimental-strip-types removes at load time.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
// The one browser-facing projection of the parity corpus, shared with the live
// /api/parity route so the static file and the endpoint can never disagree.
// api/_lib/data.ts is itself erasable-only TypeScript, so Node strips it too.
import { summarizeParity } from '../api/_lib/data.ts';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, '..');
const DOCS_DIR = path.join(REPO_ROOT, 'public', 'docs');
const EXAMPLES_DIR = path.join(REPO_ROOT, 'examples');
const PARITY_TS = path.join(REPO_ROOT, 'src', 'parity.ts');
const PARITY_DIR = path.join(REPO_ROOT, 'parity');
const OUT_DIRS = [
  path.join(REPO_ROOT, 'api', '_data'),
  path.join(REPO_ROOT, 'public'),
];

// Caps for the embedded runnable-example corpus (examples.json). The example
// programs are real, complete main.go files (some ~50 KB); we embed them so the
// /ask tool needs no runtime fetch, but clip very large ones to keep the data
// file — which ships in the serverless bundle — to a sane size. The chat tool
// clips again before handing text to the model.
const EXAMPLE_CODE_MAX = 20_000; // chars of main.go kept per library
const EXAMPLE_README_MAX = 2_000; // chars of README kept per library

// Library ids are generated slugs; the examples/<id>/ dir name is the id. Guard
// the directory listing against anything that is not a plausible id.
const EXAMPLE_ID_RE = /^[a-z0-9][a-z0-9._-]{0,63}$/;

// Cap applied to reference-edge weights so a package that mentions a neighbour's
// types dozens of times does not dominate the layout.
const REFERENCE_WEIGHT_CAP = 10;

// ---------------------------------------------------------------------------
// input/output shapes (the DocIndex JSON is read untyped, then narrowed)
// ---------------------------------------------------------------------------

interface DocGroup {
  signature?: string;
  doc?: string;
  names?: string[];
}
interface DocFunc {
  name?: string;
  signature?: string;
  doc?: string;
}
interface DocMethod {
  name?: string;
  recv?: string;
  signature?: string;
  doc?: string;
}
interface DocType {
  name?: string;
  signature?: string;
  doc?: string;
  consts?: DocGroup[];
  vars?: DocGroup[];
  funcs?: DocFunc[];
  methods?: DocMethod[];
}
interface DocPackage {
  importPath: string;
  name?: string;
  synopsis?: string;
  doc?: string;
  types?: DocType[];
  consts?: DocGroup[];
  vars?: DocGroup[];
  funcs?: DocFunc[];
}
interface DocIndex {
  module?: string;
  packages?: DocPackage[];
}

interface SymbolOut {
  id: string;
  name: string;
  kind: string;
  packageImportPath: string;
  library: string;
  signature: string;
  doc: string;
  anchor: string;
}
interface EdgeOut {
  source: string;
  target: string;
  kind: string;
  weight: number;
}
interface PackageOut {
  id: string;
  importPath: string;
  name: string;
  library: string;
  synopsis: string;
  symbolCount: number;
}
interface LibraryOut {
  id: string;
  name: string;
  packageCount: number;
  symbolCount: number;
  parityAfter: string | null;
}
interface ParityEntry {
  after: string | null;
  upstream: string | null;
}

// One embedded runnable example, keyed in examples.json by library id.
interface ExampleOut {
  library: string;
  path: string;          // repo-relative path to the source, e.g. examples/express/main.go
  code: string;          // the main.go source (clipped)
  codeTruncated: boolean;
  readme: string;        // README text (clipped/summarized)
  readmeTruncated: boolean;
}

interface PkgState {
  importPath: string;
  name: string;
  blob: string;
  typeNames: string[];
}
interface LibStateEntry {
  id: string;
  module: string;
  pkgs: PkgState[];
  rootId: string | null;
  sourceDir: string;
}

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

function resolveGeneratedAt(): string {
  const argv = process.argv.slice(2);
  const i = argv.indexOf('--generated-at');
  if (i !== -1 && argv[i + 1]) return argv[i + 1];
  if (process.env.GRAPH_GENERATED_AT) return process.env.GRAPH_GENERATED_AT;
  return new Date().toISOString();
}

// Anchor computation must match the docs renderer:
//   value / func  => sym-<name>
//   type          => sym-<Type>
//   method        => sym-<recvBase>.<method>
const ANCHOR_PREFIX = 'sym-';
function valueAnchor(name: string): string {
  return `${ANCHOR_PREFIX}${name}`;
}
function typeAnchor(typeName: string): string {
  return `${ANCHOR_PREFIX}${typeName}`;
}
function methodAnchor(recv: string | undefined, methodName: string): string {
  return `${ANCHOR_PREFIX}${recvBase(recv)}.${methodName}`;
}
// "*Application", "Application[T]", "*Store[K, V]" => "Application" / "Store"
function recvBase(recv: string | undefined): string {
  if (!recv) return '';
  let s = String(recv).trim();
  s = s.replace(/^[*&]+/, '');       // drop pointer/ref markers
  s = s.replace(/\[.*$/, '');         // drop generic type params
  s = s.trim();
  return s;
}

function isInterfaceSig(sig: string | undefined): boolean {
  if (!sig) return false;
  // A declared interface type: `type Foo interface { ... }` (as opposed to a
  // struct/alias whose body merely mentions an interface elsewhere).
  return /^\s*type\s+\S+[^={]*\binterface\b/.test(sig) || /\binterface\s*\{/.test(sig);
}

function segmentsAfter(importPath: string, module: string): number {
  if (importPath === module) return 0;
  if (module && importPath.startsWith(module + '/')) {
    return importPath.slice(module.length + 1).split('/').length;
  }
  // Fallback: total path depth.
  return importPath.split('/').length;
}

// ---------------------------------------------------------------------------
// parity (best-effort)
// ---------------------------------------------------------------------------

// Parse the PARITY map out of parity.ts with a regex; on any trouble return {}.
function loadParity(): Record<string, ParityEntry> {
  const out: Record<string, ParityEntry> = {};
  let src: string;
  try {
    src = fs.readFileSync(PARITY_TS, 'utf8');
  } catch {
    return out;
  }
  const start = src.indexOf('PARITY');
  const body = start === -1 ? src : src.slice(start);
  // Match:  key: { ... after: "100%" ... }   where key is bare or quoted.
  const entry = /(?:^|[,{]\s*)(?:'([^']+)'|"([^"]+)"|([A-Za-z0-9_.$]+))\s*:\s*\{([^}]*)\}/g;
  let m: RegExpExecArray | null;
  while ((m = entry.exec(body)) !== null) {
    const key = m[1] || m[2] || m[3];
    const inner = m[4] || '';
    if (!key || key === 'PARITY') continue;
    const afterMatch = inner.match(/\bafter\s*:\s*['"]([^'"]*)['"]/);
    const upstreamMatch = inner.match(/\bupstream\s*:\s*['"]([^'"]*)['"]/);
    if (afterMatch || upstreamMatch) {
      out[key] = {
        after: afterMatch ? afterMatch[1] : null,
        upstream: upstreamMatch ? upstreamMatch[1] : null,
      };
    }
  }
  return out;
}

// ---------------------------------------------------------------------------
// symbol extraction
// ---------------------------------------------------------------------------

// Produce every exported search symbol for one package (excluding the package
// node itself, which is emitted separately by the caller).
function symbolsForPackage(pkg: DocPackage, library: string): SymbolOut[] {
  const importPath = pkg.importPath;
  const out: SymbolOut[] = [];

  const pushValue = (group: DocGroup | undefined, kind: string): void => {
    if (!group) return;
    const sig = group.signature || '';
    const doc = group.doc || '';
    for (const name of group.names || []) {
      if (!name) continue;
      out.push({
        id: `${importPath}#${valueAnchor(name)}`,
        name,
        kind,
        packageImportPath: importPath,
        library,
        signature: sig,
        doc,
        anchor: valueAnchor(name),
      });
    }
  };

  const pushFunc = (fn: DocFunc | undefined): void => {
    if (!fn || !fn.name) return;
    out.push({
      id: `${importPath}#${valueAnchor(fn.name)}`,
      name: fn.name,
      kind: 'func',
      packageImportPath: importPath,
      library,
      signature: fn.signature || '',
      doc: fn.doc || '',
      anchor: valueAnchor(fn.name),
    });
  };

  const pushMethod = (mth: DocMethod | undefined): void => {
    if (!mth || !mth.name) return;
    const anchor = methodAnchor(mth.recv, mth.name);
    const base = recvBase(mth.recv);
    out.push({
      id: `${importPath}#${anchor}`,
      name: base ? `${base}.${mth.name}` : mth.name,
      kind: 'method',
      packageImportPath: importPath,
      library,
      signature: mth.signature || '',
      doc: mth.doc || '',
      anchor,
    });
  };

  // Types (and interfaces), plus symbols nested under a type.
  for (const t of pkg.types || []) {
    if (t.name) {
      out.push({
        id: `${importPath}#${typeAnchor(t.name)}`,
        name: t.name,
        kind: isInterfaceSig(t.signature) ? 'interface' : 'type',
        packageImportPath: importPath,
        library,
        signature: t.signature || '',
        doc: t.doc || '',
        anchor: typeAnchor(t.name),
      });
    }
    for (const c of t.consts || []) pushValue(c, 'const');
    for (const v of t.vars || []) pushValue(v, 'var');
    for (const fn of t.funcs || []) pushFunc(fn);
    for (const mth of t.methods || []) pushMethod(mth);
  }

  // Package-level values and funcs.
  for (const c of pkg.consts || []) pushValue(c, 'const');
  for (const v of pkg.vars || []) pushValue(v, 'var');
  for (const fn of pkg.funcs || []) pushFunc(fn);

  return out;
}

// Concatenated signature text of a package, used for reference-edge detection.
function signatureBlob(pkg: DocPackage): string {
  const parts: string[] = [];
  for (const t of pkg.types || []) {
    if (t.signature) parts.push(t.signature);
    for (const c of t.consts || []) if (c.signature) parts.push(c.signature);
    for (const v of t.vars || []) if (v.signature) parts.push(v.signature);
    for (const fn of t.funcs || []) if (fn.signature) parts.push(fn.signature);
    for (const mth of t.methods || []) if (mth.signature) parts.push(mth.signature);
  }
  for (const c of pkg.consts || []) if (c.signature) parts.push(c.signature);
  for (const v of pkg.vars || []) if (v.signature) parts.push(v.signature);
  for (const fn of pkg.funcs || []) if (fn.signature) parts.push(fn.signature);
  return parts.join('\n');
}

function exportedTypeNames(pkg: DocPackage): string[] {
  const names: string[] = [];
  for (const t of pkg.types || []) if (t.name) names.push(t.name);
  return names;
}

// ---------------------------------------------------------------------------
// import-edge parsing (best-effort — only when Go source is on disk)
// ---------------------------------------------------------------------------

function goImportsForDir(dir: string): Map<string, number> {
  const counts = new Map<string, number>(); // importPath -> number of files importing it
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return counts;
  }
  for (const ent of entries) {
    if (!ent.isFile()) continue;
    if (!ent.name.endsWith('.go')) continue;
    if (ent.name.endsWith('_test.go')) continue;
    let src: string;
    try {
      src = fs.readFileSync(path.join(dir, ent.name), 'utf8');
    } catch {
      continue;
    }
    const seen = new Set<string>();
    // Grouped import blocks: import ( "a" \n _ "b" \n alias "c" )
    const blockRe = /import\s*\(([\s\S]*?)\)/g;
    let bm: RegExpExecArray | null;
    while ((bm = blockRe.exec(src)) !== null) {
      const inner = bm[1];
      const strRe = /"([^"]+)"/g;
      let sm: RegExpExecArray | null;
      while ((sm = strRe.exec(inner)) !== null) seen.add(sm[1]);
    }
    // Single-line imports: import "a"  /  import alias "a"
    const singleRe = /import\s+(?:[A-Za-z0-9_.]+\s+)?"([^"]+)"/g;
    let sm2: RegExpExecArray | null;
    while ((sm2 = singleRe.exec(src)) !== null) seen.add(sm2[1]);

    for (const p of seen) counts.set(p, (counts.get(p) || 0) + 1);
  }
  return counts;
}

// ---------------------------------------------------------------------------
// runnable examples (examples/<id>/main.go + README.md)
// ---------------------------------------------------------------------------

// Clip a string to `max` chars, appending a marker line when truncated. Returns
// the (possibly clipped) text and whether clipping happened.
function clipText(s: string, max: number): { text: string; truncated: boolean } {
  if (s.length <= max) return { text: s, truncated: false };
  return { text: s.slice(0, max).trimEnd() + '\n…\n', truncated: true };
}

// Read every examples/<id>/ dir that has a main.go and embed its source +
// README (clipped). `knownLibraries` is the set of library ids the docs pass
// produced; an example without a matching library is still emitted (it maps to
// a valid /lib/<id> page) but this lets us warn about drift. Deterministic:
// directories are processed in sorted order.
function buildExamples(knownLibraries: Set<string>): {
  examples: Record<string, ExampleOut>;
  missing: string[];
} {
  const examples: Record<string, ExampleOut> = {};
  const missing: string[] = []; // library ids with docs but no runnable example
  let dirs: fs.Dirent[] = [];
  try {
    dirs = fs.readdirSync(EXAMPLES_DIR, { withFileTypes: true });
  } catch {
    // No examples/ tree checked out — emit an empty-but-valid corpus.
    for (const id of Array.from(knownLibraries).sort()) missing.push(id);
    return { examples, missing };
  }

  const ids = dirs
    .filter((d) => d.isDirectory() && EXAMPLE_ID_RE.test(d.name))
    .map((d) => d.name)
    .sort();

  const seen = new Set<string>();
  for (const id of ids) {
    const mainPath = path.join(EXAMPLES_DIR, id, 'main.go');
    let code: string;
    try {
      code = fs.readFileSync(mainPath, 'utf8');
    } catch {
      continue; // no runnable program here (placeholder dir) — skip
    }
    let readme = '';
    try {
      readme = fs.readFileSync(path.join(EXAMPLES_DIR, id, 'README.md'), 'utf8');
    } catch {
      readme = '';
    }
    const clippedCode = clipText(code, EXAMPLE_CODE_MAX);
    const clippedReadme = clipText(readme, EXAMPLE_README_MAX);
    examples[id] = {
      library: id,
      path: `examples/${id}/main.go`,
      code: clippedCode.text,
      codeTruncated: clippedCode.truncated,
      readme: clippedReadme.text,
      readmeTruncated: clippedReadme.truncated,
    };
    seen.add(id);
  }

  for (const id of Array.from(knownLibraries).sort()) {
    if (!seen.has(id)) missing.push(id);
  }
  return { examples, missing };
}

// ---------------------------------------------------------------------------
// measured parity corpus (api/_data/parity.json)
// ---------------------------------------------------------------------------
//
// Every `malcolmston/*` port has a harness under parity/<lib>/ (and, for ports
// of a *different* upstream package that lives inside a larger repo, under
// parity/<lib>/nested/<pkg>/ — see parity/HARNESS.md "Nested packages"). The
// harness runs the real upstream package and the Go port over the same cases
// and rewrites parity.json with the totals. That file — not src/parity.ts — is
// the measured truth, and this pass turns the whole tree into one corpus the
// site can render.
//
// The harnesses are NOT uniform. They were written at different times by
// different passes and every count has two or three spellings on disk:
//
//   total      total | cases | specCases | compared(number)   | match+mismatch
//   match      match | passed | matched | parityMatch | specMatch
//   mismatch   mismatch | mismatched | failed(number) | differs | differ
//              | parityMismatch | specMismatch
//   deviations deviations | declaredDeviations | deviation | #deviationCases
//   percent    parityPercent | parityPct | casePercent | percent | parity
//              | casePassRate | conformancePercent — as a number OR "91.7%"
//   groups     groups{name:{…}} | groups[{group,…}] | byGroup(either)
//              | casesByGroup{name:count}
//   failures   failingCases | failedCases | mismatchedCases | mismatches
//              | mismatchIDs | divergedCases | failed(array) | parityMismatches
//
// So: read defensively, normalise into one shape, and NEVER emit NaN — a NaN
// serialises as `null` and silently poisons every average downstream. Every
// numeric field below goes through finite() before it is written, and a single
// malformed file only ever costs us a console.warn.

// A runner directory names the upstream ecosystem, not the package
// (parity/HARNESS.md, "Layout"). `go` is the port's own runner, never upstream.
const ECOSYSTEM_DIRS = ['node', 'python', 'java', 'ruby', 'rust', 'elixir', 'c'];

const COVERAGE_STATUSES = ['match', 'differs', 'missing', 'extra', 'untested'] as const;
type CoverageStatus = (typeof COVERAGE_STATUSES)[number];

type CaseStatus = 'match' | 'mismatch' | 'deviation' | 'unknown';

interface ParityUpstreamOut {
  raw: string;
  name: string;
  version: string;
  ecosystem: string;
}
interface ParityGroupOut {
  name: string;
  total: number;
  match: number;
  mismatch: number;
  deviations: number;
  /** true when the harness only recorded a case count for this group, no verdicts. */
  countsOnly: boolean;
}
interface ParityCaseOut {
  id: string;
  group: string;
  fn: string;
  upstreamFn: string;
  goFn: string;
  note: string;
  deviation: string;
  status: CaseStatus;
  /**
   * true when the harness's own failure list names this case. A case that also
   * declares a `deviation` is reported as `deviation` (a deliberate difference,
   * per HARNESS.md) — this flag keeps the fact that it still failed visible
   * instead of hiding it behind the nicer word.
   */
  harnessFailed: boolean;
  /** repo-relative case file this case was declared in. */
  file: string;
}
interface CoverageRowOut {
  upstreamSymbol: string;
  goSymbol: string;
  status: CoverageStatus;
  cases: string[];
  note: string;
}
interface CoverageSummaryOut {
  match: number;
  differs: number;
  missing: number;
  extra: number;
  untested: number;
  /** match / (match + differs) — parity over the symbols actually compared. */
  percent: number;
  /** every row, including missing/extra/untested. */
  total: number;
}
interface SecurityFindingOut {
  id: string;
  summary: string;
  severity: string;
  affectedVersions: string;
  cases: string[];
  /** where the full `description` markdown lives; deliberately not inlined. */
  descriptionPath: string;
  hasDescription: boolean;
}
interface ParityLibraryOut {
  library: string;
  /** nested harnesses only: the upstream package this sub-port ports. */
  package?: string;
  harnessPath: string;
  upstream: ParityUpstreamOut;
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
  groups: ParityGroupOut[];
  /** true when `groups` was aggregated from `cases` because the harness had none. */
  groupsDerived: boolean;
  cases: ParityCaseOut[];
  coverage: CoverageRowOut[];
  coverageSummary: CoverageSummaryOut;
  coverageCommand: string;
  security: SecurityFindingOut[];
  nested?: Record<string, ParityLibraryOut>;
}

function finite(n: unknown): number | null {
  const v = typeof n === 'number' ? n : NaN;
  return Number.isFinite(v) ? v : null;
}
function round2(n: number): number {
  const v = Math.round(n * 100) / 100;
  return Number.isFinite(v) ? v : 0;
}
function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}
function isRecord(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && !Array.isArray(v);
}
// First key that holds a finite number. Keys that hold an array or a prose
// string (`compared` is a number in some harnesses and an English sentence in
// others; `failed` is a count in one and a list of ids in another) are skipped.
function numField(obj: Record<string, unknown>, keys: string[]): number | null {
  for (const k of keys) {
    const v = finite(obj[k]);
    if (v !== null) return v;
  }
  return null;
}
// Accepts 91.7, "91.7", "91.7%", "**91.7 %**".
function percentField(obj: Record<string, unknown>, keys: string[]): number | null {
  for (const k of keys) {
    const raw = obj[k];
    const n = finite(raw);
    if (n !== null) return n;
    if (typeof raw === 'string') {
      const m = raw.match(/-?\d+(?:\.\d+)?/);
      if (m) {
        const parsed = Number(m[0]);
        if (Number.isFinite(parsed)) return parsed;
      }
    }
  }
  return null;
}
// Case-id lists appear as ["id", …] or [{id, …}, …], and as null when empty.
function idList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const out: string[] = [];
  for (const v of value) {
    if (typeof v === 'string' && v !== '') out.push(v);
    else if (isRecord(v) && typeof v.id === 'string' && v.id !== '') out.push(v.id);
  }
  return out;
}

const TOTAL_KEYS = ['total', 'cases', 'specCases'];
const MATCH_KEYS = ['match', 'passed', 'matched', 'parityMatch', 'specMatch'];
const MISMATCH_KEYS = [
  'mismatch', 'mismatched', 'failed', 'differs', 'differ',
  'parityMismatch', 'specMismatch', 'diverged',
];
const DEVIATION_KEYS = ['deviations', 'declaredDeviations', 'deviation'];
const PERCENT_KEYS = [
  'parityPercent', 'parityPct', 'casePercent', 'percent', 'parity',
  'casePassRate', 'conformancePercent',
];
const MISMATCH_LIST_KEYS = [
  'failingCases', 'failedCases', 'mismatchedCases', 'mismatches', 'mismatchIDs',
  'mismatchIds', 'differingCases', 'divergedCases', 'failed', 'parityMismatches',
  'failures',
];
const DEVIATION_LIST_KEYS = ['deviationCases', 'deviationIDs', 'deviationIds', 'declaredDeviations'];
const GROUP_KEYS = ['groups', 'byGroup', 'casesByGroup'];

// "express@4.21.2" | "pandas==2.3.3" | "@gltf-transform/core@4.2.1" |
// "org.quartz-scheduler:quartz@2.5.0" | "phoenixframework/phoenix_live_view"
// (slug only — the version then lives in `upstreamVersion`) | multi-package
// strings joined by "+", ";" or " + ".
function parseUpstream(raw: string): { name: string; version: string } {
  const first = raw.split(/\s*[;+]\s*/)[0]?.trim() ?? '';
  // A whitespace-separated token that carries a version, e.g.
  // "rails/rails activerecord@8.0.2 (…)" -> "activerecord@8.0.2".
  const tokens = first.split(/\s+/).filter(Boolean);
  let token = tokens.find((t) => /[^@]@[^@]/.test(t) || t.includes('=='));
  if (!token) token = tokens[0] ?? '';
  if (token.includes('==')) {
    const [n, v] = token.split('==');
    return { name: n ?? '', version: v ?? '' };
  }
  // Scoped npm names start with '@' — split on the LAST '@'.
  const at = token.lastIndexOf('@');
  if (at > 0) return { name: token.slice(0, at), version: token.slice(at + 1) };
  return { name: token, version: '' };
}

function detectEcosystem(dir: string, manifest: Record<string, unknown>): string {
  for (const eco of ECOSYSTEM_DIRS) {
    try {
      if (fs.statSync(path.join(dir, eco)).isDirectory()) return eco;
    } catch {
      /* not this one */
    }
  }
  const declared = str(manifest.upstreamEcosystem) || str(manifest.ecosystem);
  if (declared) return declared;
  const raw = upstreamRaw(manifest);
  if (raw.includes('==')) return 'python';
  if (/^[a-z0-9.\-]+:[a-z0-9.\-]+@/i.test(raw)) return 'java';
  if (/@\d/.test(raw)) return 'node';
  return '';
}

// `upstream` is normally a string; parity/jest/parity.json makes it a
// {group: "expect@29.7.0"} map, and liveview/oban/quartz put a GitHub slug there
// with the pinned package elsewhere.
function upstreamRaw(manifest: Record<string, unknown>): string {
  const u = manifest.upstream;
  if (typeof u === 'string' && u.trim() !== '') {
    const versioned = str(manifest.upstreamPackage) || str(manifest.upstreamVersion);
    // A bare "org/repo" slug is not a pinned oracle — prefer the pinned field.
    if (versioned && !/@|==/.test(u)) return versioned;
    return u.trim();
  }
  if (isRecord(u)) {
    const vals = Array.from(new Set(Object.values(u).filter((v): v is string => typeof v === 'string')));
    vals.sort();
    if (vals.length > 0) return vals.join(' + ');
  }
  return str(manifest.upstreamPackage) || str(manifest.upstreamVersion) || str(manifest.upstreamTool);
}

function normaliseGroups(manifest: Record<string, unknown>): ParityGroupOut[] {
  let raw: unknown = null;
  for (const k of GROUP_KEYS) {
    if (manifest[k] !== undefined && manifest[k] !== null) { raw = manifest[k]; break; }
  }
  const out: ParityGroupOut[] = [];
  const push = (name: string, value: unknown): void => {
    if (!name) return;
    const n = finite(value);
    if (n !== null) {
      // casesByGroup / byGroup as {name: count}: a case count and nothing else.
      out.push({ name, total: n, match: 0, mismatch: 0, deviations: 0, countsOnly: true });
      return;
    }
    if (!isRecord(value)) return;
    const total = numField(value, TOTAL_KEYS);
    const match = numField(value, MATCH_KEYS);
    const mismatch = numField(value, MISMATCH_KEYS);
    const deviations = numField(value, DEVIATION_KEYS);
    out.push({
      name,
      total: total ?? (match ?? 0) + (mismatch ?? 0) + (deviations ?? 0),
      match: match ?? 0,
      mismatch: mismatch ?? 0,
      deviations: deviations ?? 0,
      countsOnly: match === null && mismatch === null,
    });
  };
  if (Array.isArray(raw)) {
    for (const g of raw) {
      if (!isRecord(g)) continue;
      push(str(g.group) || str(g.name), g);
    }
  } else if (isRecord(raw)) {
    for (const name of Object.keys(raw)) push(name, raw[name]);
  }
  out.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  return out;
}

// --- cases/*.json ----------------------------------------------------------

function readCases(harnessDir: string, harnessPath: string): { cases: ParityCaseOut[]; skipped: number } {
  const dir = path.join(harnessDir, 'cases');
  let files: string[] = [];
  try {
    files = fs.readdirSync(dir).filter((f) => f.endsWith('.json')).sort();
  } catch {
    return { cases: [], skipped: 0 };
  }
  const out: ParityCaseOut[] = [];
  let skipped = 0;
  for (const file of files) {
    const rel = `${harnessPath}/cases/${file}`;
    let parsed: unknown;
    try {
      parsed = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
    } catch (err) {
      console.warn(`build-graph-data: parity: skipping ${rel}: ${(err as Error).message}`);
      continue;
    }
    const fileGroup = isRecord(parsed) ? str(parsed.group) : '';
    const arr = Array.isArray(parsed)
      ? parsed
      : isRecord(parsed) && Array.isArray(parsed.cases)
        ? parsed.cases
        : null;
    if (!arr) {
      console.warn(`build-graph-data: parity: ${rel} has no cases array — skipped.`);
      continue;
    }
    for (const c of arr) {
      if (!isRecord(c)) { skipped++; continue; }
      const id = str(c.id);
      // Some cases/ dirs also hold raw upstream fixtures (markdown/cases/spec.json
      // is the CommonMark spec itself). Those have no case id; they are inputs to
      // the generated spec-NN-*.json case files, not cases.
      if (id === '') { skipped++; continue; }
      out.push({
        id,
        group: str(c.group) || fileGroup || file.replace(/\.json$/, ''),
        fn: str(c.fn),
        upstreamFn: str(c.upstreamFn),
        goFn: str(c.goFn),
        note: str(c.note),
        deviation: str(c.deviation),
        status: 'unknown',
        harnessFailed: false,
        file: rel,
      });
    }
  }
  return { cases: out, skipped };
}

// --- COVERAGE.md -----------------------------------------------------------

function cleanCell(cell: string): string {
  let s = cell.replace(/\\\|/g, '|').trim();
  s = s.replace(/^\*\*|\*\*$/g, '').trim();
  s = s.replace(/`/g, '').trim();
  // An em-dash (or a bare hyphen) is the table's way of writing "nothing".
  if (s === '—' || s === '–' || s === '-' || s === '') return '';
  return s;
}
function splitTableRow(line: string): string[] {
  let s = line.trim();
  if (s.startsWith('|')) s = s.slice(1);
  if (s.endsWith('|')) s = s.slice(0, -1);
  // Protect escaped pipes so a cell containing `a\|b` stays one cell.
  const SENTINEL = '\u0001';
  return s
    .replace(/\\\|/g, SENTINEL)
    .split('|')
    .map((c) => c.split(SENTINEL).join('\\|'));
}
function isDelimiterRow(line: string): boolean {
  return /^\s*\|?[\s:|-]*-[\s:|-]*\|?\s*$/.test(line) && line.includes('-');
}
function coverageStatus(cell: string): CoverageStatus | null {
  const s = cleanCell(cell).toLowerCase();
  if (s === '') return null;
  // "match / differs" (one symbol whose cases split both ways) is reported as
  // the harsher verdict — a symbol that differs anywhere is not at parity.
  if (/\bmissing\b/.test(s)) return 'missing';
  if (/\bdiffers?\b|\bpartial\b/.test(s)) return 'differs';
  if (/\bextra\b/.test(s)) return 'extra';
  if (/\buntested\b/.test(s)) return 'untested';
  if (/\bmatch(es|ed)?\b/.test(s)) return 'match';
  return null;
}
function coverageCaseIds(cell: string): string[] {
  const s = cleanCell(cell);
  if (s === '') return [];
  return s
    .split(/[,;]/)
    .map((t) => t.replace(/\(\+\s*\d+\s*more\)/gi, '').replace(/`/g, '').trim())
    .filter((t) => t !== '' && t !== '—' && t !== '-')
    .map((t) => (t.length > 120 ? t.slice(0, 120) : t));
}

// Parse every coverage table in a COVERAGE.md. A table qualifies when its
// header row has a column literally named "status" that is not the first column
// (`| status | symbols |` and `| status | count |` are summary tables, not the
// inventory). The upstream/Go columns are found by name, because the headers are
// spelled a dozen ways: "upstream symbol", "upstream `Model.*`", "upstream
// option", "upstream command", "glTF 2.0 object", "Go symbol", "Go field",
// "Go counterpart", "port models it". Malformed rows are skipped, never guessed.
function parseCoverage(md: string): CoverageRowOut[] {
  const lines = md.split(/\r?\n/);
  const rows: CoverageRowOut[] = [];
  const seen = new Set<string>();
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (!line.trim().startsWith('|')) continue;
    if (i + 1 >= lines.length || !isDelimiterRow(lines[i + 1])) continue;
    const header = splitTableRow(line).map((c) => cleanCell(c).toLowerCase());
    const statusIdx = header.findIndex((h) => h === 'status');
    // Advance past this table regardless of whether we use it.
    let end = i + 2;
    while (end < lines.length && lines[end].trim().startsWith('|')) end++;
    if (statusIdx >= 1) {
      let upstreamIdx = header.findIndex(
        (h, idx) => idx < statusIdx && /upstream|gltf|spec|sql surface|syntax/.test(h)
      );
      let goIdx = header.findIndex(
        (h, idx) => idx < statusIdx && /\bgo\b|port|counterpart|equivalent/.test(h)
      );
      if (upstreamIdx === -1 && goIdx !== 0) upstreamIdx = 0;
      // No "guess the column before status" fallback: liveview's inventory is
      // `| upstream module | exports | status | note |`, and guessing would have
      // published the export *count* as the Go symbol.
      if (upstreamIdx !== -1 || goIdx !== -1) {
        let casesIdx = header.findIndex((h, idx) => idx > statusIdx && h === 'cases');
        if (casesIdx === -1) casesIdx = header.findIndex((h, idx) => idx > statusIdx && /case/.test(h));
        const noteIdx = header.findIndex((h, idx) => idx > statusIdx && /note|comment|why/.test(h));
        for (let r = i + 2; r < end; r++) {
          const cells = splitTableRow(lines[r]);
          if (cells.length <= statusIdx) continue;         // ragged row — skip
          const status = coverageStatus(cells[statusIdx]);
          if (!status) continue;                            // not an inventory row
          const upstreamSymbol = upstreamIdx === -1 ? '' : cleanCell(cells[upstreamIdx] ?? '');
          const goSymbol = goIdx === -1 ? '' : cleanCell(cells[goIdx] ?? '');
          if (upstreamSymbol === '' && goSymbol === '') continue;
          const caseIds = casesIdx === -1 ? [] : coverageCaseIds(cells[casesIdx] ?? '');
          const note = noteIdx === -1 ? '' : cleanCell(cells[noteIdx] ?? '');
          const key = `${upstreamSymbol}${goSymbol}${status}${caseIds.join(',')}`;
          if (seen.has(key)) continue;
          seen.add(key);
          rows.push({ upstreamSymbol, goSymbol, status, cases: caseIds, note });
        }
      }
    }
    i = end - 1;
  }
  return rows;
}

function summariseCoverage(rows: CoverageRowOut[]): CoverageSummaryOut {
  const counts: Record<CoverageStatus, number> = {
    match: 0, differs: 0, missing: 0, extra: 0, untested: 0,
  };
  for (const r of rows) counts[r.status]++;
  const compared = counts.match + counts.differs;
  return {
    ...counts,
    percent: compared > 0 ? round2((counts.match / compared) * 100) : 0,
    total: rows.length,
  };
}

// The command that produced the upstream inventory. HARNESS.md requires
// COVERAGE.md to say it; in practice it is either a fenced block under a
// "How the upstream inventory was derived" heading, or a prose sentence.
function parseCoverageCommand(md: string): string {
  const lines = md.split(/\r?\n/);
  const headingRe = /^(#+)\s*(.*)$/;
  const DERIVED = /deriv|produc|enumerat/i;

  // Read the fenced block that starts at `start`, as a command.
  const fenceAt = (start: number): string => {
    const body: string[] = [];
    for (let k = start + 1; k < lines.length && !/^\s*```/.test(lines[k]); k++) body.push(lines[k]);
    const cmd = body
      .map((l) => l.replace(/^\s*(?:\$|>>>|#)\s+/, '').trimEnd())
      .filter((l) => l.trim() !== '')
      .join('\n')
      .trim();
    if (cmd === '') return '';
    return cmd.length > 600 ? cmd.slice(0, 600) + '…' : cmd;
  };

  // 1. The canonical layout: a "How the upstream inventory was derived" heading
  //    followed by the command in a fenced block.
  for (let i = 0; i < lines.length; i++) {
    const h = lines[i].match(headingRe);
    if (!h) continue;
    if (!DERIVED.test(h[2])) continue;
    if (!/upstream|inventor|symbol|list|coverage/i.test(h[2])) continue;
    const level = h[1].length;
    for (let j = i + 1; j < lines.length; j++) {
      const nh = lines[j].match(headingRe);
      if (nh && nh[1].length <= level) break;
      if (!/^\s*```/.test(lines[j])) continue;
      const cmd = fenceAt(j);
      if (cmd !== '') return cmd;
      break;
    }
  }
  // 2. Otherwise: the sentence that names the command inline (express says
  //    "Enumerated with `Object.getOwnPropertyNames(require('express'))` on the
  //    installed package"). Table rows are excluded — express's header
  //    normalisation table says "derived from the wall clock" and would win —
  //    and so are "Reproducing" sections, whose command re-runs the *test*, not
  //    the inventory. A sentence whose inline code looks like a real call is
  //    preferred over a vaguer one.
  const REPRO = /reproduc|running|re-?run|how to run/i;
  const CALLISH = /\(|javap|go doc|npm |node |python|dir\b|reflect|Object\./;
  let vague = '';
  let sectionExcluded = false;
  const prose: string[] = [];
  for (const l of lines) {
    const h = l.match(headingRe);
    if (h) { sectionExcluded = REPRO.test(h[2]); continue; }
    if (sectionExcluded) continue;
    if (l.trim().startsWith('|') || /^\s*```/.test(l)) continue;
    prose.push(l);
  }
  for (const s of prose.join(' ').split(/(?<=\.)\s+/)) {
    if (!DERIVED.test(s) || !s.includes('`')) continue;
    const t = s.trim().replace(/\s+/g, ' ');
    const clipped = t.length > 400 ? t.slice(0, 400) + '…' : t;
    const code = t.match(/`([^`]+)`/);
    if (code && CALLISH.test(code[1])) return clipped;
    if (vague === '') vague = clipped;
  }
  // 3. Otherwise: a "derived mechanically" claim with a fenced block just below,
  //    outside any Reproducing section.
  sectionExcluded = false;
  for (let i = 0; i < lines.length; i++) {
    const h = lines[i].match(headingRe);
    if (h) { sectionExcluded = REPRO.test(h[2]); continue; }
    if (sectionExcluded) continue;
    if (lines[i].trim().startsWith('|')) continue;
    if (!DERIVED.test(lines[i])) continue;
    for (let j = i; j < Math.min(i + 8, lines.length); j++) {
      if (!/^\s*```/.test(lines[j])) continue;
      const cmd = fenceAt(j);
      if (cmd !== '') return cmd;
      break;
    }
  }
  return vague;
}

// --- security.json ---------------------------------------------------------

function readSecurity(harnessDir: string, harnessPath: string): SecurityFindingOut[] {
  const file = path.join(harnessDir, 'security.json');
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (err) {
    const e = err as NodeJS.ErrnoException;
    if (e && e.code !== 'ENOENT') {
      console.warn(`build-graph-data: parity: skipping ${harnessPath}/security.json: ${e.message}`);
    }
    return [];
  }
  const findings = isRecord(parsed) && Array.isArray(parsed.findings) ? parsed.findings : [];
  const out: SecurityFindingOut[] = [];
  for (const f of findings) {
    if (!isRecord(f)) continue;
    const id = str(f.id) || str(f.ghsa);
    const summary = str(f.summary);
    if (id === '' && summary === '') continue;
    out.push({
      id,
      summary,
      severity: str(f.severity),
      affectedVersions: str(f.affectedVersions),
      cases: idList(f.cases),
      // The `description` markdown is deliberately NOT inlined: several are
      // multi-KB advisories. The UI fetches detail from the manifest.
      descriptionPath: `${harnessPath}/security.json`,
      hasDescription: str(f.description) !== '',
    });
  }
  return out;
}

// --- one harness -----------------------------------------------------------

function buildHarness(
  harnessDir: string,
  harnessPath: string,
  fallbackName: string,
  nestedPackage: string | null
): ParityLibraryOut | null {
  let manifest: Record<string, unknown>;
  try {
    const parsed = JSON.parse(fs.readFileSync(path.join(harnessDir, 'parity.json'), 'utf8'));
    if (!isRecord(parsed)) throw new Error('parity.json is not an object');
    manifest = parsed;
  } catch (err) {
    console.warn(`build-graph-data: parity: skipping ${harnessPath}: ${(err as Error).message}`);
    return null;
  }

  // ---- counts, with every spelling folded together ----
  let total = numField(manifest, TOTAL_KEYS);
  let match = numField(manifest, MATCH_KEYS);
  let mismatch = numField(manifest, MISMATCH_KEYS);
  const deviations = numField(manifest, DEVIATION_KEYS) ?? idList(manifest.deviationCases).length;

  // Per-case verdict lists (also used to derive counts a harness omitted).
  const mismatchIds = new Set<string>();
  let mismatchListPresent = false;
  for (const k of MISMATCH_LIST_KEYS) {
    if (Array.isArray(manifest[k])) {
      mismatchListPresent = true;
      for (const id of idList(manifest[k])) mismatchIds.add(id);
    }
  }
  const deviationIds = new Set<string>();
  for (const k of DEVIATION_LIST_KEYS) {
    for (const id of idList(manifest[k])) deviationIds.add(id);
  }
  // parity/pandas records a full per-case verdict table; use it when present.
  const caseStatuses = new Map<string, CaseStatus>();
  if (Array.isArray(manifest.caseResults)) {
    for (const c of manifest.caseResults) {
      if (!isRecord(c)) continue;
      const id = str(c.id);
      if (id === '') continue;
      const s = str(c.status).toLowerCase();
      caseStatuses.set(
        id,
        s === 'match' ? 'match'
          : s === 'deviation' ? 'deviation'
            : s === 'differs' || s === 'mismatch' || s === 'differ' || s === 'failed' ? 'mismatch'
              : 'unknown'
      );
    }
  }

  if (mismatch === null && mismatchListPresent) mismatch = mismatchIds.size;
  if (total === null) total = (match ?? 0) + (mismatch ?? 0) + deviations;
  if (match === null) match = Math.max(0, total - (mismatch ?? 0) - deviations);
  if (mismatch === null) mismatch = Math.max(0, total - match - deviations);

  const comparedRaw = finite(manifest.compared) ?? finite(manifest.parityDenominator);
  const comparedTotal = comparedRaw ?? Math.max(0, total - deviations);

  let parityPercent = percentField(manifest, PERCENT_KEYS);
  if (parityPercent === null) {
    const denom = comparedTotal > 0 ? comparedTotal : total;
    parityPercent = denom > 0 ? (match / denom) * 100 : 0;
  }
  parityPercent = round2(Math.max(0, Math.min(100, parityPercent)));

  // ---- upstream ----
  const raw = upstreamRaw(manifest);
  const { name, version } = parseUpstream(raw);
  const upstream: ParityUpstreamOut = {
    raw,
    name,
    version,
    ecosystem: detectEcosystem(harnessDir, manifest),
  };

  // ---- go module / toolchain ----
  const goModuleBase = str(manifest.goModule);
  const goModuleVersion = str(manifest.goModuleVersion);
  const goModule = goModuleVersion && !goModuleBase.includes(goModuleVersion)
    ? `${goModuleBase} ${goModuleVersion}`.trim()
    : goModuleBase;
  const goVersionRaw = str(manifest.goVersion);
  const goVersion = (goVersionRaw.match(/go\d+(?:\.\d+)*/) ?? [goVersionRaw])[0];

  // ---- cases ----
  const { cases } = readCases(harnessDir, harnessPath);
  // A harness with no failure list at all cannot tell us which cases matched, so
  // its cases stay `unknown` rather than being optimistically marked `match`.
  const perCaseVerdicts =
    mismatchListPresent || deviationIds.size > 0 || caseStatuses.size > 0 || mismatch === 0;
  for (const c of cases) {
    c.harnessFailed = mismatchIds.has(c.id);
    const recorded = caseStatuses.get(c.id);
    if (recorded) { c.status = recorded; continue; }
    if (c.deviation !== '' || deviationIds.has(c.id)) { c.status = 'deviation'; continue; }
    if (mismatchIds.has(c.id)) { c.status = 'mismatch'; continue; }
    c.status = perCaseVerdicts ? 'match' : 'unknown';
  }

  // ---- COVERAGE.md ----
  let coverage: CoverageRowOut[] = [];
  let coverageCommand = '';
  try {
    const md = fs.readFileSync(path.join(harnessDir, 'COVERAGE.md'), 'utf8');
    coverage = parseCoverage(md);
    coverageCommand = parseCoverageCommand(md);
  } catch (err) {
    const e = err as NodeJS.ErrnoException;
    if (e && e.code !== 'ENOENT') {
      console.warn(`build-graph-data: parity: cannot read ${harnessPath}/COVERAGE.md: ${e.message}`);
    }
  }

  // A few harnesses (pandas) record no group table at all. When we have per-case
  // verdicts we can group them ourselves — that is aggregation of measured data,
  // not invention — and flag that we did.
  let groups = normaliseGroups(manifest);
  let groupsDerived = false;
  if (groups.length === 0 && cases.length > 0) {
    const byName = new Map<string, ParityGroupOut>();
    for (const c of cases) {
      const name = c.group || '(ungrouped)';
      let g = byName.get(name);
      if (!g) {
        g = { name, total: 0, match: 0, mismatch: 0, deviations: 0, countsOnly: !perCaseVerdicts };
        byName.set(name, g);
      }
      g.total++;
      if (c.status === 'match') g.match++;
      else if (c.status === 'mismatch') g.mismatch++;
      else if (c.status === 'deviation') g.deviations++;
    }
    groups = Array.from(byName.values()).sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
    groupsDerived = groups.length > 0;
  }

  const out: ParityLibraryOut = {
    library: str(manifest.library) || str(manifest.repo) || fallbackName,
    harnessPath,
    upstream,
    goModule,
    goPackage: str(manifest.goPackage),
    goVersion,
    goVersionRaw,
    generatedBy: str(manifest.generatedBy),
    measuredAt: str(manifest.generated) || str(manifest.generatedAt) || str(manifest.measuredAt),
    total,
    match,
    mismatch,
    deviations,
    parityPercent,
    comparedTotal,
    perCaseVerdicts,
    groups,
    groupsDerived,
    cases,
    coverage,
    coverageSummary: summariseCoverage(coverage),
    coverageCommand,
    security: readSecurity(harnessDir, harnessPath),
  };
  if (nestedPackage) {
    out.package = str(manifest.package) || nestedPackage;
  }
  return out;
}

interface ParityCorpus {
  generatedAt: string;
  libraries: Record<string, ParityLibraryOut>;
}

// Walk parity/*/ (and parity/*/nested/*/) and build the whole corpus. Every
// failure here is a warning: `pnpm build:graph` is the prebuild hook, and a
// half-written harness must not take a deploy down.
function buildParityCorpus(generatedAt: string): {
  corpus: ParityCorpus;
  stats: { libraries: number; nested: number; cases: number; coverageRows: number; findings: number };
} {
  const libraries: Record<string, ParityLibraryOut> = {};
  const stats = { libraries: 0, nested: 0, cases: 0, coverageRows: 0, findings: 0 };
  let dirs: string[] = [];
  try {
    dirs = fs
      .readdirSync(PARITY_DIR, { withFileTypes: true })
      .filter((d) => d.isDirectory() && EXAMPLE_ID_RE.test(d.name))
      .map((d) => d.name)
      .sort();
  } catch (err) {
    console.warn(`build-graph-data: parity: cannot read ${PARITY_DIR}: ${(err as Error).message}`);
    return { corpus: { generatedAt, libraries }, stats };
  }

  const tally = (lib: ParityLibraryOut): void => {
    stats.cases += lib.cases.length;
    stats.coverageRows += lib.coverage.length;
    stats.findings += lib.security.length;
  };

  for (const id of dirs) {
    const dir = path.join(PARITY_DIR, id);
    if (!fs.existsSync(path.join(dir, 'parity.json'))) continue;
    const lib = buildHarness(dir, `parity/${id}`, id, null);
    if (!lib) continue;
    stats.libraries++;
    tally(lib);

    // Nested harnesses: a peer of the top-level score, never a replacement.
    const nestedDir = path.join(dir, 'nested');
    let nestedNames: string[] = [];
    try {
      nestedNames = fs
        .readdirSync(nestedDir, { withFileTypes: true })
        .filter((d) => d.isDirectory() && EXAMPLE_ID_RE.test(d.name))
        .map((d) => d.name)
        .sort();
    } catch {
      nestedNames = [];
    }
    const nested: Record<string, ParityLibraryOut> = {};
    for (const pkg of nestedNames) {
      const pkgDir = path.join(nestedDir, pkg);
      if (!fs.existsSync(path.join(pkgDir, 'parity.json'))) continue;
      const sub = buildHarness(pkgDir, `parity/${id}/nested/${pkg}`, `${id}/${pkg}`, pkg);
      if (!sub) continue;
      delete sub.nested; // nested harnesses never nest further
      nested[pkg] = sub;
      stats.nested++;
      tally(sub);
    }
    if (Object.keys(nested).length > 0) lib.nested = nested;
    libraries[id] = lib;
  }

  // Deterministic key order.
  const sorted: Record<string, ParityLibraryOut> = {};
  for (const k of Object.keys(libraries).sort()) sorted[k] = libraries[k];
  return { corpus: { generatedAt, libraries: sorted }, stats };
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Browser search index (public/search-index.json)
// ---------------------------------------------------------------------------

/**
 * Project the full symbol index down to the columnar shape the browser
 * fallback actually needs. api/_data/symbols.json stays full-fat (the server
 * BM25 route ranks over `doc` and renders `signature`); the public copy is
 * fetched over the wire by every static-host search, so it carries only the
 * four fields the hit list renders plus the anchor needed to rebuild `id`.
 *
 * Shape (format "columns/1", decoded by src/api/graph.ts):
 *   libraries: string[]                       — interned library ids
 *   kinds:     string[]                       — interned symbol kinds
 *   packages:  [importPath, libraryIndex][]   — interned package paths
 *   symbols:   [name, kindIndex, packageIndex, anchor][]
 * where `anchor` is the literal anchor string, or the number 0 meaning
 * `anchorPrefix + name` — which is how every non-package anchor is derived
 * (see valueAnchor/typeAnchor/methodAnchor), so the column costs almost
 * nothing. `signature` and `doc` are dropped entirely: nothing in the UI
 * renders them, and together they are roughly half the payload.
 */
function browserSearchIndex(generatedAt: string, symbols: SymbolOut[]) {
  const libraries: string[] = [];
  const libraryIndex = new Map<string, number>();
  const kinds: string[] = [];
  const kindIndex = new Map<string, number>();
  const packages: [string, number][] = [];
  const packageIndex = new Map<string, number>();
  const intern = (value: string, list: string[], seen: Map<string, number>): number => {
    const hit = seen.get(value);
    if (hit !== undefined) return hit;
    const next = list.length;
    list.push(value);
    seen.set(value, next);
    return next;
  };

  const rows: [string, number, number, string | 0][] = symbols.map((s) => {
    const lib = intern(s.library, libraries, libraryIndex);
    let pkg = packageIndex.get(s.packageImportPath);
    if (pkg === undefined) {
      pkg = packages.length;
      packages.push([s.packageImportPath, lib]);
      packageIndex.set(s.packageImportPath, pkg);
    }
    const anchor: string | 0 = s.anchor === `${ANCHOR_PREFIX}${s.name}` ? 0 : s.anchor;
    return [s.name, intern(s.kind, kinds, kindIndex), pkg, anchor];
  });

  return {
    generatedAt,
    format: 'columns/1',
    anchorPrefix: ANCHOR_PREFIX,
    libraries,
    kinds,
    packages,
    symbols: rows,
  };
}

function main(): void {
  const generatedAt = resolveGeneratedAt();
  const parity = loadParity();

  // This script is the `prebuild` hook, so anything it throws fails `pnpm build`
  // outright. A missing/empty public/docs is a *data* problem (the gendocs pass
  // has not run, or submodules were never checked out) and the app has bundled
  // fallbacks for it — so degrade to an empty-but-valid graph instead of taking
  // the whole build down. Set GRAPH_STRICT=1 (or pass --strict) in a pipeline
  // that genuinely requires the data, e.g. the Pages deploy.
  const strict = process.argv.includes('--strict') || process.env.GRAPH_STRICT === '1';
  let docFiles: string[] = [];
  try {
    // gendocs also writes an index.json alongside the per-library files. It is
    // a manifest, not a library, so exclude it — otherwise it is counted as a
    // 39th library and reported as "no go.mod on disk", which reads as a real
    // missing port.
    docFiles = fs
      .readdirSync(DOCS_DIR)
      .filter((f) => f.endsWith('.json') && f !== 'index.json');
  } catch (err) {
    const msg = `build-graph-data: cannot read docs dir ${DOCS_DIR}: ${(err as Error).message}`;
    if (strict) {
      console.error(`${msg} (GRAPH_STRICT is set — failing)`);
      process.exit(1);
    }
    console.warn(`${msg}\nbuild-graph-data: continuing with an empty graph/symbol index.`);
  }
  if (docFiles.length === 0) {
    const msg = `build-graph-data: no DocIndex JSON found in ${DOCS_DIR}. Run gendocs (see .github/workflows/pages.yml) after \`git submodule update --init --recursive\`.`;
    if (strict) {
      console.error(`${msg} (GRAPH_STRICT is set — failing)`);
      process.exit(1);
    }
    console.warn(msg);
  }
  docFiles.sort();

  const libraries: LibraryOut[] = [];
  const packages: PackageOut[] = [];
  const edges: EdgeOut[] = [];
  const symbols: SymbolOut[] = [];

  const packageIds = new Set<string>();  // every package importPath in the graph
  // Per-library working state we need for a second edge-building pass.
  const libState: LibStateEntry[] = [];

  for (const file of docFiles) {
    const library = file.replace(/\.json$/, ''); // filename stem, e.g. "socket.io"
    let doc: DocIndex;
    try {
      doc = JSON.parse(fs.readFileSync(path.join(DOCS_DIR, file), 'utf8')) as DocIndex;
    } catch (err) {
      console.error(`build-graph-data: skipping ${file}: ${(err as Error).message}`);
      continue;
    }
    const module = doc.module || '';
    const pkgs = Array.isArray(doc.packages) ? doc.packages : [];

    // Determine the library "root" package: importPath === module, else the
    // package with the fewest segments beyond the module (lexical tie-break).
    let rootPkg: DocPackage | null = null;
    for (const p of pkgs) {
      if (!p || !p.importPath) continue;
      if (p.importPath === module) { rootPkg = p; break; }
    }
    if (!rootPkg) {
      for (const p of pkgs) {
        if (!p || !p.importPath) continue;
        if (
          rootPkg === null ||
          segmentsAfter(p.importPath, module) < segmentsAfter(rootPkg.importPath, module) ||
          (segmentsAfter(p.importPath, module) === segmentsAfter(rootPkg.importPath, module) &&
            p.importPath < rootPkg.importPath)
        ) {
          rootPkg = p;
        }
      }
    }
    const rootId = rootPkg ? rootPkg.importPath : null;

    const state: LibStateEntry = {
      id: library,
      module,
      pkgs: [],
      rootId,
      sourceDir: path.join(REPO_ROOT, library), // best-effort Go source location
    };

    let librarySymbolCount = 0;

    for (const p of pkgs) {
      if (!p || !p.importPath) continue;
      const importPath = p.importPath;
      packageIds.add(importPath);

      // Package node = also a search symbol of kind "package".
      symbols.push({
        id: importPath,
        name: p.name || importPath,
        kind: 'package',
        packageImportPath: importPath,
        library,
        signature: '',
        doc: p.synopsis || p.doc || '',
        anchor: '',
      });

      const memberSyms = symbolsForPackage(p, library);
      for (const s of memberSyms) symbols.push(s);

      const symbolCount = memberSyms.length;
      librarySymbolCount += symbolCount;

      packages.push({
        id: importPath,
        importPath,
        name: p.name || importPath,
        library,
        synopsis: p.synopsis || '',
        symbolCount,
      });

      state.pkgs.push({
        importPath,
        name: p.name || '',
        blob: signatureBlob(p),
        typeNames: exportedTypeNames(p),
      });
    }

    const parEntry = parity[library] || null;
    libraries.push({
      id: library,
      name: library,
      packageCount: state.pkgs.length,
      symbolCount: librarySymbolCount,
      parityAfter: parEntry && parEntry.after ? parEntry.after : null,
    });

    libState.push(state);
  }

  // -------- edges: same-library star --------
  for (const st of libState) {
    if (!st.rootId) continue;
    for (const p of st.pkgs) {
      if (p.importPath === st.rootId) continue;
      edges.push({ source: p.importPath, target: st.rootId, kind: 'same-library', weight: 1 });
    }
  }

  // -------- edges: reference (same-library type usage) --------
  for (const st of libState) {
    for (const a of st.pkgs) {
      if (!a.blob) continue;
      for (const b of st.pkgs) {
        if (a.importPath === b.importPath) continue;
        if (!b.name || b.typeNames.length === 0) continue;
        let weight = 0;
        for (const tn of b.typeNames) {
          const token = `${b.name}.${tn}`;
          let idx = a.blob.indexOf(token);
          while (idx !== -1) {
            // Ensure the char before is not an identifier char (so `xyzb.T`
            // does not match `b.T`).
            const prev = idx > 0 ? a.blob[idx - 1] : '';
            if (!/[A-Za-z0-9_.]/.test(prev)) weight++;
            idx = a.blob.indexOf(token, idx + token.length);
          }
        }
        if (weight > 0) {
          edges.push({
            source: a.importPath,
            target: b.importPath,
            kind: 'reference',
            weight: Math.min(weight, REFERENCE_WEIGHT_CAP),
          });
        }
      }
    }
  }

  // -------- edges: import (best-effort, from Go source) --------
  // A library whose submodule is unchecked-out, empty, or still a placeholder
  // simply contributes no import edges (goImportsForDir swallows the ENOENT).
  // Name those libraries so a graph with few import edges is diagnosable
  // instead of mysterious.
  let importEdgeCount = 0;
  const missingSources: string[] = [];
  for (const st of libState) {
    if (!fs.existsSync(path.join(st.sourceDir, 'go.mod'))) missingSources.push(st.id);
    for (const p of st.pkgs) {
      // Map importPath -> on-disk directory under the library source dir.
      let sub = '';
      if (st.module && p.importPath === st.module) sub = '';
      else if (st.module && p.importPath.startsWith(st.module + '/')) sub = p.importPath.slice(st.module.length + 1);
      else continue; // cannot map -> skip
      const dir = sub ? path.join(st.sourceDir, sub) : st.sourceDir;
      const counts = goImportsForDir(dir);
      for (const [target, n] of counts) {
        if (target === p.importPath) continue;
        if (!packageIds.has(target)) continue; // only edges to known graph nodes
        edges.push({ source: p.importPath, target, kind: 'import', weight: n });
        importEdgeCount++;
      }
    }
  }

  // -------- edges: shared-upstream (same upstream org) --------
  const byOrg = new Map<string, string[]>(); // org -> [rootId,...]
  for (const st of libState) {
    if (!st.rootId) continue;
    const par = parity[st.id];
    if (!par || !par.upstream) continue;
    const org = String(par.upstream).split('/')[0];
    if (!org) continue;
    if (!byOrg.has(org)) byOrg.set(org, []);
    byOrg.get(org)!.push(st.rootId);
  }
  for (const roots of byOrg.values()) {
    if (roots.length < 2) continue;
    const uniq = Array.from(new Set(roots)).sort();
    for (const source of uniq) {
      for (const target of uniq) {
        if (source === target) continue;
        edges.push({ source, target, kind: 'shared-upstream', weight: 1 });
      }
    }
  }

  // -------- dedup + deterministic ordering --------
  // Collapse duplicate edges (same source/target/kind), summing weight.
  const edgeMap = new Map<string, EdgeOut>();
  for (const e of edges) {
    const key = `${e.source} ${e.target} ${e.kind}`;
    const prev = edgeMap.get(key);
    if (prev) prev.weight += e.weight;
    else edgeMap.set(key, { ...e });
  }
  const finalEdges = Array.from(edgeMap.values()).sort(
    (x, y) =>
      x.source < y.source ? -1 : x.source > y.source ? 1 :
      x.target < y.target ? -1 : x.target > y.target ? 1 :
      x.kind < y.kind ? -1 : x.kind > y.kind ? 1 : 0
  );

  // Dedup symbols by id (defensive) and sort by id.
  const symMap = new Map<string, SymbolOut>();
  for (const s of symbols) if (!symMap.has(s.id)) symMap.set(s.id, s);
  const finalSymbols = Array.from(symMap.values()).sort(
    (a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
  );

  const finalPackages = packages.slice().sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  const finalLibraries = libraries.slice().sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));

  const graph = {
    generatedAt,
    libraries: finalLibraries,
    packages: finalPackages,
    edges: finalEdges,
  };
  const symbolIndex = {
    generatedAt,
    symbols: finalSymbols,
  };
  const browserIndex = browserSearchIndex(generatedAt, finalSymbols);

  // -------- write to both destinations --------
  for (const dir of OUT_DIRS) {
    fs.mkdirSync(dir, { recursive: true });
  }
  // -------- runnable examples corpus --------
  // Deterministic: keyed and iterated by sorted library id. Only api/_data gets
  // this file — it is read server-side by the /ask tool and embedded in the
  // function bundle; the frontend never fetches it, so no public/ copy.
  const knownLibraries = new Set(finalLibraries.map((l) => l.id));
  const { examples: exampleMap, missing: missingExamples } = buildExamples(knownLibraries);
  const sortedExamples: Record<string, ExampleOut> = {};
  for (const id of Object.keys(exampleMap).sort()) sortedExamples[id] = exampleMap[id];
  const examplesOut = {
    generatedAt,
    examples: sortedExamples,
  };

  const graphJson = JSON.stringify(graph, null, 2) + '\n';
  const symbolsJson = JSON.stringify(symbolIndex, null, 2) + '\n';
  // Not pretty-printed, and deliberately not `symbolsJson`: this copy is
  // downloaded by the browser on every static-host search (see
  // browserSearchIndex above).
  const browserIndexJson = JSON.stringify(browserIndex) + '\n';
  const examplesJson = JSON.stringify(examplesOut, null, 2) + '\n';

  // -------- measured parity corpus --------
  // Only api/_data gets the full corpus: it is read server-side (api/_lib/data.ts,
  // getParity()) and is far too large to ship to the browser wholesale. The
  // browser gets the summary written to public/parity.json below instead.
  const { corpus: parityCorpus, stats: parityStats } = buildParityCorpus(generatedAt);
  // Not pretty-printed: 17k case records and 6.7k coverage rows cost ~5 MB of
  // pure indentation, and this file is bundled into every serverless function.
  const parityJson = JSON.stringify(parityCorpus) + '\n';
  // ...and the browser-sized projection of it. src/components/Explore.tsx
  // fetches `parity.json` relative to the base path and rejects any payload over
  // 512 KB, so public/parity.json carries only the headline figures per library
  // (a few KB). Same filename as the corpus, different directory: the corpus is
  // server-side (api/_data), the summary is public. app/api/parity/route.ts
  // serves the identical shape from the same summarizeParity().
  const paritySummary = summarizeParity(parityCorpus);
  const paritySummaryJson = JSON.stringify(paritySummary, null, 2) + '\n';

  fs.writeFileSync(path.join(OUT_DIRS[0], 'graph.json'), graphJson);
  fs.writeFileSync(path.join(OUT_DIRS[0], 'symbols.json'), symbolsJson);
  fs.writeFileSync(path.join(OUT_DIRS[0], 'examples.json'), examplesJson);
  fs.writeFileSync(path.join(OUT_DIRS[0], 'parity.json'), parityJson);
  // public copies use the frontend-fallback filenames.
  fs.writeFileSync(path.join(OUT_DIRS[1], 'graph.json'), graphJson);
  fs.writeFileSync(path.join(OUT_DIRS[1], 'search-index.json'), browserIndexJson);
  fs.writeFileSync(path.join(OUT_DIRS[1], 'parity.json'), paritySummaryJson);

  const kindCounts: Record<string, number> = {};
  for (const e of finalEdges) kindCounts[e.kind] = (kindCounts[e.kind] || 0) + 1;
  const kindStr = ['import', 'reference', 'same-library', 'shared-upstream']
    .map((k) => `${k}=${kindCounts[k] || 0}`)
    .join(' ');
  console.log(
    `build-graph-data: ${finalLibraries.length} libraries, ${finalPackages.length} packages, ` +
    `${finalSymbols.length} symbols, ${finalEdges.length} edges (${kindStr}; ` +
    `${importEdgeCount} raw import edges from source). Wrote graph.json + symbols.json ` +
    `to api/_data and public.`
  );
  console.log(
    `build-graph-data: browser search index: ${browserIndex.symbols.length} symbols over ` +
    `${browserIndex.packages.length} packages ` +
    `(${(browserIndexJson.length / 1024 / 1024).toFixed(2)} MB, down from ` +
    `${(symbolsJson.length / 1024 / 1024).toFixed(2)} MB). Wrote search-index.json to public.`
  );
  console.log(
    `build-graph-data: ${Object.keys(sortedExamples).length} runnable examples ` +
    `(${(examplesJson.length / 1024).toFixed(1)} KB). Wrote examples.json to api/_data.`
  );
  console.log(
    `build-graph-data: parity: ${parityStats.libraries} harnesses + ${parityStats.nested} nested ` +
    `packages, ${parityStats.cases} cases, ${parityStats.coverageRows} coverage rows, ` +
    `${parityStats.findings} security findings ` +
    `(${(parityJson.length / 1024).toFixed(1)} KB). Wrote parity.json to api/_data, ` +
    `plus a ${(paritySummaryJson.length / 1024).toFixed(1)} KB summary of ` +
    `${Object.keys(paritySummary.libraries).length} libraries to public/parity.json.`
  );
  if (missingExamples.length > 0) {
    console.warn(
      `build-graph-data: no runnable example for ${missingExamples.length} librar` +
      `${missingExamples.length === 1 ? 'y' : 'ies'} (${missingExamples.join(', ')}).`
    );
  }
  if (missingSources.length > 0) {
    console.warn(
      `build-graph-data: no go.mod on disk for ${missingSources.length} librar` +
      `${missingSources.length === 1 ? 'y' : 'ies'} (${missingSources.join(', ')}) — ` +
      `import edges for those are omitted. These are either placeholder ports ` +
      `(README → "Planned ports") or unfetched submodules ` +
      `(\`git submodule update --init --recursive\`).`
    );
  }
}

main();
