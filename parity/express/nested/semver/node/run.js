// Upstream parity runner for npm/node-semver — the oracle for the
// github.com/malcolmston/express/semver port.
//
// The pinned oracle is semver@7.8.5 (npm `latest`).
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Never exits on a failing case. Logs go to stderr only.
//
// NULL MAPPING — the one convention this harness rests on:
// node-semver signals "not a version / no answer" two different ways. Some
// functions throw (`new SemVer('bogus')`, `compare('bogus','1.2.3')`), others
// return null (`valid`, `clean`, `coerce`, `parse`, `prerelease`, `inc` with an
// unusable release level, `maxSatisfying`/`minSatisfying` with no match,
// `validRange`). The Go port has a single channel for both: a non-nil error (or
// ok=false). So every null-returning upstream call is funnelled through nn()
// below, which turns null/undefined into a throw, i.e. into ok:false. The
// harness then compares "did it produce an answer, and which" — the fact that
// is genuinely comparable across the two languages. The mapping is applied
// symmetrically: the Go runner reports ok:false in exactly the same situations.

import readline from 'node:readline';
import semver from 'semver';

const { SemVer, Range } = semver;

function wantString(v) {
  if (typeof v !== 'string') throw new Error('expected a string argument, got ' + typeof v);
  return v;
}

function wantArray(v) {
  if (!Array.isArray(v)) throw new Error('expected an array argument, got ' + typeof v);
  return v;
}

// nn: "not null". See the NULL MAPPING note above.
function nn(what, v) {
  if (v === null || v === undefined) throw new Error(what + ': no result (upstream returned null)');
  return v;
}

const fns = {
  // ---------------------------------------------------------------- parsing
  // valid(v) -> the canonical version string, or a failure for invalid input.
  valid: (a) => nn('valid', semver.valid(wantString(a[0]))),
  // the same question as a boolean, which is what the Go port's Valid returns.
  isValid: (a) => semver.valid(wantString(a[0])) !== null,
  // parse(v).version — the SemVer object round-tripped back to a string.
  versionString: (a) => nn('parse', semver.parse(wantString(a[0])))?.version,
  // clean(v) is deliberately more permissive than parse: it strips leading
  // "=" and "v" characters and surrounding whitespace before parsing.
  clean: (a) => nn('clean', semver.clean(wantString(a[0]))),
  // Major.Minor.Patch with prerelease and build metadata dropped.
  core: (a) => {
    const v = new SemVer(wantString(a[0]));
    return `${v.major}.${v.minor}.${v.patch}`;
  },

  // -------------------------------------------------------------- accessors
  major: (a) => semver.major(wantString(a[0])),
  minor: (a) => semver.minor(wantString(a[0])),
  patch: (a) => semver.patch(wantString(a[0])),
  // prerelease(v) -> array of identifiers with numeric ones as numbers, or
  // null. Joined with "." so it compares against the Go port's string.
  prerelease: (a) => nn('prerelease', semver.prerelease(wantString(a[0]))).join('.'),

  // ------------------------------------------------------------- comparison
  compare: (a) => semver.compare(wantString(a[0]), wantString(a[1])),
  rcompare: (a) => semver.rcompare(wantString(a[0]), wantString(a[1])),
  // SemVer#compare, the method behind the free function.
  compareMethod: (a) => new SemVer(wantString(a[0])).compare(wantString(a[1])),
  gt: (a) => semver.gt(wantString(a[0]), wantString(a[1])),
  gte: (a) => semver.gte(wantString(a[0]), wantString(a[1])),
  lt: (a) => semver.lt(wantString(a[0]), wantString(a[1])),
  lte: (a) => semver.lte(wantString(a[0]), wantString(a[1])),
  eq: (a) => semver.eq(wantString(a[0]), wantString(a[1])),
  neq: (a) => semver.neq(wantString(a[0]), wantString(a[1])),
  // the three boolean SemVer methods the port spells LessThan/GreaterThan/Equal
  lessThan: (a) => new SemVer(wantString(a[0])).compare(wantString(a[1])) < 0,
  greaterThan: (a) => new SemVer(wantString(a[0])).compare(wantString(a[1])) > 0,
  equalVersion: (a) => new SemVer(wantString(a[0])).compare(wantString(a[1])) === 0,
  // sort(list) ascending. Upstream returns the same strings it was given, in
  // order, and breaks ties with compareBuild, so the answer is compared verbatim.
  sort: (a) => semver.sort(wantArray(a[0]).map(wantString)),

  // ------------------------------------------------------------ incrementing
  inc: (a) => nn('inc', semver.inc(wantString(a[0]), wantString(a[1]))),
  incId: (a) => nn('inc', semver.inc(wantString(a[0]), wantString(a[1]), wantString(a[2]))),
  // the SemVer#inc methods the port exposes as IncMajor/IncMinor/IncPatch
  incMajorMethod: (a) => new SemVer(wantString(a[0])).inc('major').version,
  incMinorMethod: (a) => new SemVer(wantString(a[0])).inc('minor').version,
  incPatchMethod: (a) => new SemVer(wantString(a[0])).inc('patch').version,

  // ---------------------------------------------------------------- coercion
  coerce: (a) => nn('coerce', semver.coerce(wantString(a[0]))).version,

  // ------------------------------------------------------------------ ranges
  satisfies: (a) => semver.satisfies(wantString(a[0]), wantString(a[1])),
  // Range#test, which unlike satisfies() propagates an invalid range as a throw.
  rangeTest: (a) => new Range(wantString(a[1])).test(wantString(a[0])),
  // validRange as the boolean "does this range parse at all".
  rangeValid: (a) => semver.validRange(wantString(a[0])) !== null,
  // the normalised comparator-set text of a range.
  rangeString: (a) => new Range(wantString(a[0])).range,
  maxSatisfying: (a) =>
    nn('maxSatisfying', semver.maxSatisfying(wantArray(a[0]).map(wantString), wantString(a[1]))),
  minSatisfying: (a) =>
    nn('minSatisfying', semver.minSatisfying(wantArray(a[0]).map(wantString), wantString(a[1]))),
};

function emit(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on('line', (line) => {
  line = line.trim();
  if (line === '') return;
  let req;
  try {
    req = JSON.parse(line);
  } catch (err) {
    emit({ id: null, ok: false, error: 'malformed request: ' + err.message });
    return;
  }
  const fn = fns[req.fn];
  if (!fn) {
    emit({ id: req.id, ok: false, error: 'unknown fn: ' + req.fn });
    return;
  }
  try {
    let value = fn(req.args || []);
    if (value === undefined) value = null;
    emit({ id: req.id, ok: true, value });
  } catch (err) {
    emit({ id: req.id, ok: false, error: String((err && err.message) || err) });
  }
});
