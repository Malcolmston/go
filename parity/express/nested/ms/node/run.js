// Upstream parity runner for vercel/ms — the oracle for the
// github.com/malcolmston/express/ms port.
//
// The pinned oracle is ms@4.0.0-nightly.202508271359, the published build of
// vercel/ms main. That is the revision this port was written against (its
// week/month/year format ladder and its {parse, format} split exist only there;
// the older ms@2.1.3 on npm's `latest` tag stops at days). See COVERAGE.md.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Never exits on a failing case. Logs go to stderr only.

import readline from 'node:readline';
import { ms, parse, format, parseStrict } from 'ms';

function wantString(v) {
  if (typeof v !== 'string') throw new Error('expected a string argument, got ' + typeof v);
  return v;
}

function wantNumber(v) {
  if (typeof v !== 'number') throw new Error('expected a number argument, got ' + typeof v);
  return v;
}

// ms.parse() signals "cannot parse" two different ways: it throws for a
// non-string / empty / over-long input and returns NaN for input that does not
// match the grammar. The Go port returns an error in both situations, so
// normalise NaN into a throw here — the harness then compares "did it fail",
// which is the comparable fact.
function parsed(fn, s) {
  const v = fn(s);
  if (typeof v !== 'number' || Number.isNaN(v)) {
    throw new Error('ms.parse: invalid duration ' + JSON.stringify(s));
  }
  return v;
}

const fns = {
  // parse(str) -> milliseconds as a number
  parse: (a) => parsed(parse, wantString(a[0])),
  // parseStrict is parse with a compile-time-checked argument type; at runtime
  // it is the same function, and the port has no separate symbol for it.
  parseStrict: (a) => parsed(parseStrict, wantString(a[0])),
  // format(ms) -> short human form, e.g. "2h"
  format: (a) => format(wantNumber(a[0])),
  // format(ms, {long:true}) -> long human form, e.g. "2 hours"
  formatLong: (a) => format(wantNumber(a[0]), { long: true }),
  // the dual-dispatch default export, in each of its two directions
  msString: (a) => parsed(ms, wantString(a[0])),
  msNumber: (a) => ms(wantNumber(a[0])),
  msNumberLong: (a) => ms(wantNumber(a[0]), { long: true }),
  // roundtrip(str) -> format(parse(str))
  roundtrip: (a) => format(parsed(parse, wantString(a[0]))),
  roundtripLong: (a) => format(parsed(parse, wantString(a[0])), { long: true }),
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
