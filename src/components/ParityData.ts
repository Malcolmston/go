// ParityData — the shape of api/_data/parity.json, plus tolerant normalisers.
//
// parity.json is generated from the harnesses under parity/ (see
// parity/HARNESS.md): one entry per Go library, each carrying the pinned
// upstream oracle it was measured against, the per-case results, and the full
// upstream API inventory from COVERAGE.md. Nested ports (express/qs, chalk/
// figlet, …) are separate harnesses against a DIFFERENT upstream package and
// appear under `nested`.
//
// Everything here is defensive: the file is produced by a generator that runs
// outside this app, so a missing field renders as "unknown" rather than
// throwing during the build.

export type CaseStatus = 'match' | 'mismatch' | 'deviation' | 'unknown';
export type CoverageStatus = 'match' | 'differs' | 'missing' | 'extra' | 'untested';
export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'unknown';

export const CASE_STATUSES: CaseStatus[] = ['match', 'mismatch', 'deviation', 'unknown'];
export const COVERAGE_STATUSES: CoverageStatus[] = ['match', 'differs', 'missing', 'extra', 'untested'];

/** The exact upstream package a harness was measured against. */
export interface ParityUpstream {
  /** As pinned in the case files, e.g. "chalk@5.3.0" or "numpy==2.0.1". */
  raw: string;
  name: string;
  version: string;
  /** node | python | java | ruby | rust | elixir | c — names the runner dir. */
  ecosystem: string;
}

export interface ParityGroup {
  name: string;
  total: number;
  match: number;
  mismatch: number;
}

export interface ParityCase {
  id: string;
  group: string;
  /** The logical operation both runners were asked for. */
  fn: string;
  /** The real upstream symbol the case exercised. */
  upstreamFn: string;
  /** The real Go symbol the case exercised. */
  goFn: string;
  note: string;
  /** Non-empty when the difference is deliberate and documented. */
  deviation: string;
  status: CaseStatus;
}

export interface ParityCoverageRow {
  upstreamSymbol: string;
  goSymbol: string;
  status: CoverageStatus;
  /** Case ids that exercise this symbol; empty means `untested`. */
  cases: string[];
  note: string;
}

export interface ParityCoverageSummary {
  match: number;
  differs: number;
  missing: number;
  extra: number;
  untested: number;
  percent: number;
}

export interface ParitySecurityFinding {
  id: string;
  summary: string;
  severity: Severity;
  affectedVersions: string;
  cases: string[];
}

/** One harness: a Go module measured against one pinned upstream package. */
export interface ParityHarness {
  /** The harness's own name (library id, or nested package name). */
  library: string;
  /** Repo-relative path of the harness, e.g. "parity/express/nested/qs". */
  harnessPath: string;
  upstream: ParityUpstream;
  goModule: string;
  goVersion: string;
  /** The command/tool that produced the numbers. */
  generatedBy: string;

  total: number;
  match: number;
  mismatch: number;
  deviations: number;
  parityPercent: number;
  /** Cases that produced a comparable answer on both sides. */
  comparedTotal: number;

  groups: ParityGroup[];
  cases: ParityCase[];
  coverage: ParityCoverageRow[];
  coverageSummary: ParityCoverageSummary;
  /** The command COVERAGE.md says produced the upstream symbol inventory. */
  coverageCommand: string;
  security: ParitySecurityFinding[];
  /** Ports of other upstream packages that live inside this repo. */
  nested: Record<string, ParityHarness>;
}

export interface ParityData {
  generatedAt: string;
  libraries: Record<string, ParityHarness>;
}

/* ------------------------------------------------------------------ */
/* normalisation                                                       */
/* ------------------------------------------------------------------ */

function obj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}
function str(v: unknown, fallback = ''): string {
  return typeof v === 'string' ? v : typeof v === 'number' ? String(v) : fallback;
}
function num(v: unknown, fallback = 0): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : fallback;
}
function arr(v: unknown): unknown[] {
  return Array.isArray(v) ? v : [];
}
function strList(v: unknown): string[] {
  return arr(v).map((x) => str(x)).filter((x) => x !== '');
}

function caseStatus(v: unknown): CaseStatus {
  const s = str(v).toLowerCase();
  return (CASE_STATUSES as string[]).includes(s) ? (s as CaseStatus) : 'unknown';
}
function coverageStatus(v: unknown): CoverageStatus {
  const s = str(v).toLowerCase();
  return (COVERAGE_STATUSES as string[]).includes(s) ? (s as CoverageStatus) : 'untested';
}
function severity(v: unknown): Severity {
  const s = str(v).toLowerCase();
  return s === 'critical' || s === 'high' || s === 'medium' || s === 'low' ? s : 'unknown';
}

// The pinned spec is written differently per ecosystem ("chalk@5.3.0",
// "numpy==2.0.1", "org.example:lib:1.2.3"); split on the LAST separator so a
// scoped npm name ("@scope/pkg@1.0.0") keeps its leading @.
function splitPinned(raw: string): { name: string; version: string } {
  const at = raw.lastIndexOf('@');
  if (at > 0) return { name: raw.slice(0, at), version: raw.slice(at + 1) };
  const eq = raw.indexOf('==');
  if (eq > 0) return { name: raw.slice(0, eq), version: raw.slice(eq + 2) };
  const colon = raw.lastIndexOf(':');
  if (colon > 0) return { name: raw.slice(0, colon), version: raw.slice(colon + 1) };
  return { name: raw, version: '' };
}

function normalizeUpstream(v: unknown): ParityUpstream {
  const o = obj(v);
  const raw = str(o.raw);
  const guess = splitPinned(raw);
  return {
    raw,
    name: str(o.name, guess.name),
    version: str(o.version, guess.version),
    ecosystem: str(o.ecosystem).toLowerCase(),
  };
}

function normalizeGroups(v: unknown): ParityGroup[] {
  return arr(v).map((g) => {
    const o = obj(g);
    return {
      name: str(o.name ?? o.group, 'ungrouped'),
      total: num(o.total),
      match: num(o.match),
      mismatch: num(o.mismatch),
    };
  });
}

function normalizeCases(v: unknown): ParityCase[] {
  return arr(v).map((c) => {
    const o = obj(c);
    return {
      id: str(o.id),
      group: str(o.group),
      fn: str(o.fn),
      upstreamFn: str(o.upstreamFn),
      goFn: str(o.goFn),
      note: str(o.note),
      deviation: str(o.deviation),
      status: caseStatus(o.status),
    };
  });
}

function normalizeCoverage(v: unknown): ParityCoverageRow[] {
  return arr(v).map((r) => {
    const o = obj(r);
    return {
      upstreamSymbol: str(o.upstreamSymbol),
      goSymbol: str(o.goSymbol),
      status: coverageStatus(o.status),
      cases: strList(o.cases),
      note: str(o.note),
    };
  });
}

function normalizeCoverageSummary(v: unknown, rows: ParityCoverageRow[]): ParityCoverageSummary {
  const o = obj(v);
  const has = Object.keys(o).length > 0;
  if (has) {
    return {
      match: num(o.match),
      differs: num(o.differs),
      missing: num(o.missing),
      extra: num(o.extra),
      untested: num(o.untested),
      percent: num(o.percent),
    };
  }
  // Derive the counts when the generator omitted them, so the chips always add
  // up to the rows actually shown in the table.
  const count = (s: CoverageStatus) => rows.filter((r) => r.status === s).length;
  const match = count('match');
  const compared = match + count('differs');
  return {
    match,
    differs: count('differs'),
    missing: count('missing'),
    extra: count('extra'),
    untested: count('untested'),
    percent: compared > 0 ? Math.round((match / compared) * 1000) / 10 : 0,
  };
}

function normalizeSecurity(v: unknown): ParitySecurityFinding[] {
  return arr(v).map((f) => {
    const o = obj(f);
    return {
      id: str(o.id),
      summary: str(o.summary),
      severity: severity(o.severity),
      affectedVersions: str(o.affectedVersions),
      cases: strList(o.cases),
    };
  });
}

function normalizeHarness(key: string, v: unknown, withNested: boolean): ParityHarness {
  const o = obj(v);
  const coverage = normalizeCoverage(o.coverage);
  const cases = normalizeCases(o.cases);
  const groups = normalizeGroups(o.groups);
  const total = num(o.total, cases.length);
  const match = num(o.match);
  const nested: Record<string, ParityHarness> = {};
  if (withNested) {
    for (const [nk, nv] of Object.entries(obj(o.nested))) {
      nested[nk] = normalizeHarness(nk, nv, false);
    }
  }
  return {
    library: str(o.library, key),
    harnessPath: str(o.harnessPath),
    upstream: normalizeUpstream(o.upstream),
    goModule: str(o.goModule),
    goVersion: str(o.goVersion),
    generatedBy: str(o.generatedBy),
    total,
    match,
    mismatch: num(o.mismatch),
    deviations: num(o.deviations),
    parityPercent: num(o.parityPercent),
    comparedTotal: num(o.comparedTotal, total),
    groups,
    cases,
    coverage,
    coverageSummary: normalizeCoverageSummary(o.coverageSummary, coverage),
    coverageCommand: str(o.coverageCommand),
    security: normalizeSecurity(o.security),
    nested,
  };
}

/** Parse an unknown JSON blob into a ParityData, tolerating a missing file. */
export function normalizeParityData(parsed: unknown): ParityData {
  const root = obj(parsed);
  const libraries: Record<string, ParityHarness> = {};
  for (const [key, value] of Object.entries(obj(root.libraries))) {
    libraries[key] = normalizeHarness(key, value, true);
  }
  return { generatedAt: str(root.generatedAt), libraries };
}

/* ------------------------------------------------------------------ */
/* derived views                                                       */
/* ------------------------------------------------------------------ */

/** A row on the /parity index: everything but the per-case / per-symbol bulk. */
export interface ParitySummary {
  slug: string;
  /** Display name from the site's library data, or the harness key. */
  name: string;
  accent: string;
  upstream: ParityUpstream;
  goModule: string;
  goVersion: string;
  harnessPath: string;
  total: number;
  match: number;
  mismatch: number;
  deviations: number;
  comparedTotal: number;
  parityPercent: number;
  groupCount: number;
  symbolCount: number;
  coverageSummary: ParityCoverageSummary;
  securityCount: number;
  nested: {
    /** The harness key, i.e. the nested/<key> directory name. */
    key: string;
    name: string;
    upstream: ParityUpstream;
    goModule: string;
    total: number;
    match: number;
    mismatch: number;
    deviations: number;
    comparedTotal: number;
    parityPercent: number;
    symbolCount: number;
  }[];
}

export function summarize(
  slug: string,
  h: ParityHarness,
  name: string,
  accent: string,
): ParitySummary {
  return {
    slug,
    name,
    accent,
    upstream: h.upstream,
    goModule: h.goModule,
    goVersion: h.goVersion,
    harnessPath: h.harnessPath,
    total: h.total,
    match: h.match,
    mismatch: h.mismatch,
    deviations: h.deviations,
    comparedTotal: h.comparedTotal,
    parityPercent: h.parityPercent,
    groupCount: h.groups.length,
    symbolCount: h.coverage.length,
    coverageSummary: h.coverageSummary,
    securityCount: h.security.length,
    nested: Object.entries(h.nested)
      .map(([nk, n]) => ({
        key: nk,
        name: n.library || nk,
        upstream: n.upstream,
        goModule: n.goModule,
        total: n.total,
        match: n.match,
        mismatch: n.mismatch,
        deviations: n.deviations,
        comparedTotal: n.comparedTotal,
        parityPercent: n.parityPercent,
        symbolCount: n.coverage.length,
      }))
      .sort((a, b) => a.name.localeCompare(b.name)),
  };
}

/**
 * The denominator the harness's own parityPercent was actually computed over.
 *
 * Harnesses disagree about this. Most exclude declared deviations, so
 * comparedTotal (total - deviations) is the denominator. But some — express, for
 * one — count a declared deviation inside `mismatch` as well, so subtracting it
 * again yields a denominator that does not reproduce the published percentage:
 * express is match 182, total 184, deviations 1, parityPercent 98.91, and
 * 182/184 is 98.91% while 182/183 is 99.45%.
 *
 * Showing "182 / 183" next to "98.9%" invites the reader to check the division
 * and conclude one of the two is wrong. So pick whichever denominator actually
 * reproduces the published percentage, preferring comparedTotal when both do.
 * The deviation count is displayed on its own tile either way, so nothing is
 * hidden by this choice.
 */
export function scoredDenominator(h: { match: number; total: number; comparedTotal: number; parityPercent: number }): number {
  const candidates = [h.comparedTotal, h.total].filter((d) => d > 0);
  if (!candidates.length) return h.total;
  const closest = candidates.reduce((best, d) =>
    Math.abs((100 * h.match) / d - h.parityPercent) < Math.abs((100 * h.match) / best - h.parityPercent) ? d : best,
  );
  return closest;
}

/** Percent formatted for display; "—" when nothing comparable ran. */
export function pct(value: number, compared: number): string {
  if (compared <= 0) return '—';
  return `${(Math.round(value * 10) / 10).toFixed(1)}%`;
}

/** The install command that pins the oracle, per upstream ecosystem. */
export function installCommand(u: ParityUpstream): string {
  const { name, version, raw, ecosystem } = u;
  const pinned = raw || (version ? `${name}@${version}` : name);
  switch (ecosystem) {
    case 'node':
      return `npm install --save-exact ${pinned}`;
    case 'python':
      return `pip install ${version ? `${name}==${version}` : name}`;
    case 'ruby':
      return `gem install ${name}${version ? ` -v ${version}` : ''}`;
    case 'rust':
      return `cargo add ${name}${version ? `@${version}` : ''}`;
    case 'java':
      return `mvn dependency:get -Dartifact=${pinned}`;
    case 'elixir':
      return `mix deps.get  # {:${name}, "== ${version}"}`;
    case 'c':
      return `cc -o run run.c -l${name}${version ? `  # ${version}` : ''}`;
    default:
      return pinned ? `install ${pinned}` : 'install the pinned upstream';
  }
}

/** Filename of the upstream runner for an ecosystem. */
export function runnerFile(ecosystem: string): string {
  switch (ecosystem) {
    case 'node':
      return 'run.js';
    case 'python':
      return 'run.py';
    case 'ruby':
      return 'run.rb';
    case 'rust':
      return 'run.rs';
    case 'java':
      return 'Run.java';
    case 'elixir':
      return 'run.exs';
    case 'c':
      return 'run.c';
    default:
      return 'run';
  }
}

/** The runtime that executes the upstream runner. */
export function runtimeLabel(ecosystem: string): string {
  switch (ecosystem) {
    case 'node':
      return 'Node.js';
    case 'python':
      return 'CPython';
    case 'ruby':
      return 'CRuby';
    case 'rust':
      return 'rustc';
    case 'java':
      return 'JVM';
    case 'elixir':
      return 'BEAM';
    case 'c':
      return 'cc';
    default:
      return ecosystem || 'upstream runtime';
  }
}

/** The command COVERAGE.md documents for enumerating upstream symbols. */
export function inventoryHint(ecosystem: string): string {
  switch (ecosystem) {
    case 'node':
      return 'Object.keys over the installed package';
    case 'python':
      return 'dir() over the imported module';
    case 'java':
      return 'javap over the installed jar';
    case 'rust':
      return 'cargo doc --output-format json';
    case 'ruby':
      return 'Module#instance_methods reflection';
    case 'elixir':
      return 'Module.__info__(:functions)';
    case 'c':
      return 'nm over the built library';
    default:
      return 'reflection over the installed package';
  }
}
