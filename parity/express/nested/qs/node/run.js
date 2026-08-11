// Upstream parity runner for ljharb/qs — the oracle for the
// github.com/malcolmston/express/qs port.
//
// Pinned oracle: qs@6.15.3 (npm `latest` at the time this harness was written;
// see node/package.json and COVERAGE.md).
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[input, opts?]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Determinism, and why it needs help here:
//
//   * parse() returns a JavaScript object whose key order is insertion order.
//     Go maps have no order at all, so every emitted object is rebuilt with its
//     keys deep-sorted before it is serialised. Array order is meaningful in
//     both languages and is preserved untouched.
//   * stringify() emits keys in insertion order, and there the order is part of
//     the output *string*, so sorting after the fact is impossible. The harness
//     therefore always hands upstream qs its own `sort` option with a
//     code-unit comparator — the same order Go's sort.Strings produces — unless
//     a case opts out with "sort": false. Array indices are keys too, so
//     stringify cases keep arrays under ten elements, where "0".."9" sort the
//     same way numerically and lexicographically.
//
// Never exits on a failing case. Logs go to stderr only.

'use strict';

const readline = require('node:readline');
const qs = require('qs');

// A code-unit comparator: identical to Go's sort.Strings for the ASCII keys
// these cases use.
const codeUnitSort = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

// "Infinity" / "-Infinity" appear as strings in case files so the same JSON can
// also feed the Go runner, which spells "no limit" as a negative int.
function fixNumbers(opts) {
  const out = {};
  for (const k of Object.keys(opts)) {
    const v = opts[k];
    if (v === 'Infinity') {
      out[k] = Infinity;
    } else if (v === '-Infinity') {
      out[k] = -Infinity;
    } else {
      out[k] = v;
    }
  }
  return out;
}

function parseOpts(a, i) {
  const raw = a[i];
  if (raw === undefined || raw === null) return undefined;
  if (typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error('opts argument ' + i + ' must be an object');
  }
  return fixNumbers(raw);
}

function stringifyOpts(a, i) {
  const o = parseOpts(a, i) || {};
  if (o.sort === false) {
    // The case deliberately wants upstream insertion order.
    delete o.sort;
    return o;
  }
  o.sort = codeUnitSort;
  return o;
}

function wantString(v) {
  if (typeof v !== 'string') throw new Error('expected a string input, got ' + typeof v);
  return v;
}

function wantObject(v) {
  if (typeof v !== 'object' || v === null) throw new Error('expected an object input');
  return v;
}

// normalize rebuilds a parse result with object keys sorted, so the JSON the
// harness compares does not depend on JavaScript insertion order. It also turns
// the holes of a sparse array (allowSparse) into explicit nulls, which is what
// JSON.stringify would do anyway, and what the Go side emits.
function normalize(v) {
  if (Array.isArray(v)) {
    const out = new Array(v.length);
    for (let i = 0; i < v.length; i += 1) {
      out[i] = i in v ? normalize(v[i]) : null;
    }
    return out;
  }
  if (v === null || typeof v !== 'object') {
    return v === undefined ? null : v;
  }
  const out = {};
  for (const k of Object.keys(v).sort(codeUnitSort)) {
    out[k] = normalize(v[k]);
  }
  return out;
}

const fns = {
  // parse(str, opts) -> nested object, keys deep-sorted
  parse: (a) => normalize(qs.parse(wantString(a[0]), parseOpts(a, 1))),

  // stringify(obj, opts) -> query string
  stringify: (a) => qs.stringify(wantObject(a[0]), stringifyOpts(a, 1)),

  // stringify(parse(str)) -> query string: does the port re-serialise what it
  // parsed the way upstream does?
  parseStringify: (a) => qs.stringify(qs.parse(wantString(a[0]), parseOpts(a, 1)), stringifyOpts(a, 2)),

  // parse(stringify(obj)) -> object: is the serialisation reversible?
  stringifyParse: (a) => normalize(qs.parse(qs.stringify(wantObject(a[0]), stringifyOpts(a, 1)), parseOpts(a, 2))),

  // qs.formats, the third exported symbol.
  formats: () => ({
    default: qs.formats.default,
    RFC1738: qs.formats.RFC1738,
    RFC3986: qs.formats.RFC3986,
  }),

  // The names qs exports, so the port's surface can be checked against them.
  exports: () => Object.keys(qs).sort(codeUnitSort),
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
