// Upstream parity runner for visionmedia/bytes.js — the oracle for the
// github.com/malcolmston/express/bytes port.
//
// The pinned oracle is bytes@3.1.2, npm's `latest`, and the revision the port
// was written against (express/bytes/bytes_parity_test.go cites that tag's
// index.js, test/bytes.js and Readme.md). See COVERAGE.md.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Never exits on a failing case. Logs go to stderr only.

'use strict';

const readline = require('node:readline');
const bytes = require('bytes');

function wantString(v) {
  if (typeof v !== 'string') throw new Error('expected a string argument, got ' + typeof v);
  return v;
}

function wantNumber(v) {
  if (typeof v !== 'number') throw new Error('expected a number argument, got ' + typeof v);
  return v;
}

function wantOptions(v) {
  if (v === undefined) return undefined;
  if (v === null || typeof v !== 'object' || Array.isArray(v)) {
    throw new Error('expected an options object, got ' + JSON.stringify(v));
  }
  return v;
}

// bytes.parse() and the default export signal "cannot parse" by returning null
// rather than throwing; bytes.format() returns null for a non-finite value. The
// Go port has no null, it returns an error, so normalise null into a throw here
// and let the harness compare "did it fail" — the comparable fact.
function notNull(what, v) {
  if (v === null || v === undefined) {
    throw new Error(what + ': returned null');
  }
  return v;
}

const fns = {
  // bytes.parse(string) -> number of bytes
  parse: (a) => notNull('bytes.parse', bytes.parse(wantString(a[0]))),
  // bytes.parse(number) -> the same number, straight back out
  parseNum: (a) => notNull('bytes.parse', bytes.parse(wantNumber(a[0]))),
  // bytes.parse(NaN) -> null. NaN cannot travel through JSON, so it is a
  // zero-argument case instead.
  parseNaN: () => notNull('bytes.parse', bytes.parse(NaN)),

  // bytes.format(number) -> string, default options
  format: (a) => notNull('bytes.format', bytes.format(wantNumber(a[0]))),
  // bytes.format(number, options) -> string
  formatOpts: (a) => notNull('bytes.format', bytes.format(wantNumber(a[0]), wantOptions(a[1]))),

  // the dual-dispatch default export, in each of its directions
  bytesString: (a) => notNull('bytes()', bytes(wantString(a[0]))),
  bytesNumber: (a) => notNull('bytes()', bytes(wantNumber(a[0]))),
  bytesNumberOpts: (a) => notNull('bytes()', bytes(wantNumber(a[0]), wantOptions(a[1]))),
  // anything that is neither a string nor a number: returns null
  bytesOther: (a) => notNull('bytes()', bytes(a[0])),

  // format(parse(string)) and parse(format(number))
  roundtrip: (a) => notNull('bytes.format', bytes.format(notNull('bytes.parse', bytes.parse(wantString(a[0]))))),
  roundtripParse: (a) => notNull('bytes.parse', bytes.parse(notNull('bytes.format', bytes.format(wantNumber(a[0]))))),
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
