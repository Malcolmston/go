// Upstream oracle runner for the jest parity harness.
//
// Speaks the JSON-Lines contract from parity/HARNESS.md: one case object per
// line on stdin, one result object per line on stdout, same order.
//
// The comparable artefact is a single boolean: did the assertion hold?
//   { "id": ..., "ok": true, "value": { "pass": true|false } }
// A thrown JestAssertionError means the assertion did not hold (pass:false) --
// it is NOT a runner error. Anything else (a matcher used against a value it
// cannot handle, an unknown matcher) is reported as ok:false with a message.
//
// Oracle: the `expect` package, which is jest's assertion engine published
// standalone. See COVERAGE.md for why that, and not the jest CLI.

'use strict';

const readline = require('readline');
const { expect } = require('expect');
const { ModuleMocker } = require('jest-mock');

const mocker = new ModuleMocker(global);

// ---------------------------------------------------------------------------
// value specs
// ---------------------------------------------------------------------------

const CTORS = {
  Error: Error,
  TypeError: TypeError,
  RangeError: RangeError,
  String: String,
  Number: Number,
  Boolean: Boolean,
  Object: Object,
  Array: Array,
  Function: Function,
  Date: Date,
  RegExp: RegExp,
  Map: Map,
  Set: Set,
};

const ABSENT = Symbol('absent');

function decodeObjectEntries(entries) {
  const o = {};
  for (const [k, spec] of entries) {
    const v = decode(spec);
    if (v === ABSENT) continue;
    o[k] = v;
  }
  return o;
}

function decode(spec) {
  if (spec === null) return null;
  if (Array.isArray(spec)) return spec.map(decode);
  if (typeof spec !== 'object') return spec;
  if (!Object.prototype.hasOwnProperty.call(spec, '$')) {
    const o = {};
    for (const k of Object.keys(spec)) {
      const v = decode(spec[k]);
      if (v === ABSENT) continue;
      o[k] = v;
    }
    return o;
  }
  switch (spec.$) {
    case 'undefined': return undefined;
    case 'absent': return ABSENT;
    case 'nan': return NaN;
    case 'negzero': return -0;
    case 'inf': return Infinity;
    case 'ninf': return -Infinity;
    case 'int': return spec.v;
    case 'obj': return decodeObjectEntries(spec.entries);
    case 'map': return new Map((spec.entries || []).map(([k, v]) => [decode(k), decode(v)]));
    case 'set': return new Set((spec.values || []).map(decode));
    case 'date': return new Date(spec.iso);
    case 'regexp': return new RegExp(spec.source, spec.flags || '');
    case 'error': {
      const C = CTORS[spec.ctor || 'Error'];
      return new C(spec.message === undefined ? '' : spec.message);
    }
    case 'class': {
      const C = CTORS[spec.name];
      if (!C) throw new Error('unknown class: ' + spec.name);
      return C;
    }
    case 'sparse': {
      const a = [];
      a.length = spec.length;
      for (const k of Object.keys(spec.values || {})) a[Number(k)] = decode(spec.values[k]);
      return a;
    }
    case 'cyclic': {
      const o = decodeObjectEntries(spec.entries || []);
      o[spec.selfKey || 'self'] = o;
      return o;
    }
    case 'fn': return function noop() { return spec.returns === undefined ? undefined : decode(spec.returns); };
    case 'throws': {
      const kind = spec.ctor || 'Error';
      const msg = spec.message === undefined ? '' : spec.message;
      if (kind === 'string') return function () { throw msg; };
      if (kind === 'none') return function () { return undefined; };
      const C = CTORS[kind];
      if (!C) throw new Error('unknown throw ctor: ' + kind);
      return function () { throw new C(msg); };
    }
    case 'mock': return buildMock(spec);
    // asymmetric matchers
    case 'any': return expect.any(CTORS[spec.type]);
    case 'anything': return expect.anything();
    case 'arrayContaining': return expect.arrayContaining((spec.values || []).map(decode));
    case 'objectContaining': return expect.objectContaining(decodeObjectEntries(spec.entries || []));
    case 'stringContaining': return expect.stringContaining(spec.value);
    case 'stringMatching': return expect.stringMatching(spec.flags ? new RegExp(spec.value, spec.flags) : spec.value);
    case 'closeTo': return spec.precision === undefined
      ? expect.closeTo(spec.value)
      : expect.closeTo(spec.value, spec.precision);
    case 'notArrayContaining': return expect.not.arrayContaining((spec.values || []).map(decode));
    case 'notObjectContaining': return expect.not.objectContaining(decodeObjectEntries(spec.entries || []));
    case 'notStringContaining': return expect.not.stringContaining(spec.value);
    case 'notStringMatching': return expect.not.stringMatching(spec.value);
    case 'notCloseTo': return spec.precision === undefined
      ? expect.not.closeTo(spec.value)
      : expect.not.closeTo(spec.value, spec.precision);
    default:
      throw new Error('unknown spec tag: ' + spec.$);
  }
}

// buildMock creates a jest-mock function, primes its return values and replays
// the recorded call list, so the call/return matchers have something to inspect.
function buildMock(spec) {
  const fn = mocker.fn();
  if (spec.name) fn.mockName(spec.name);
  if (spec.returns !== undefined) fn.mockReturnValue(decode(spec.returns));
  for (const argSpecs of spec.calls || []) {
    const args = argSpecs.map(decode);
    if (spec.throwsOnCall) {
      try { fn.mockImplementationOnce(() => { throw new Error(spec.throwsOnCall); }); fn(...args); } catch (e) { /* recorded */ }
    } else {
      fn(...args);
    }
  }
  return fn;
}

// ---------------------------------------------------------------------------
// dispatch
// ---------------------------------------------------------------------------

function isAssertionFailure(err) {
  return !!(err && typeof err === 'object' && err.matcherResult !== undefined);
}

function runCase(c) {
  let actual, args;
  try {
    actual = decode(c.actual);
    args = (c.args || []).map(decode);
  } catch (e) {
    return { ok: false, error: 'decode: ' + (e && e.message ? e.message : String(e)) };
  }

  let target;
  try {
    target = expect(actual);
    if (c.not) target = target.not;
  } catch (e) {
    return { ok: false, error: 'expect(): ' + (e && e.message ? e.message : String(e)) };
  }

  const m = target[c.fn];
  if (typeof m !== 'function') return { ok: false, error: 'no such matcher: ' + c.fn };

  try {
    m.apply(target, args);
    return { ok: true, value: { pass: true } };
  } catch (e) {
    if (isAssertionFailure(e)) return { ok: true, value: { pass: false } };
    return { ok: false, error: (e && e.message ? e.message : String(e)) };
  }
}

// ---------------------------------------------------------------------------
// stdio loop
// ---------------------------------------------------------------------------

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  line = line.trim();
  if (!line) return;
  let c;
  try {
    c = JSON.parse(line);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: '?', ok: false, error: 'bad json: ' + e.message }) + '\n');
    return;
  }
  let out;
  try {
    out = runCase(c);
  } catch (e) {
    out = { ok: false, error: 'runner: ' + (e && e.message ? e.message : String(e)) };
  }
  out.id = c.id;
  process.stdout.write(JSON.stringify({ id: out.id, ok: out.ok, value: out.value, error: out.error }) + '\n');
});
