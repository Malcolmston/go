'use strict';
// Upstream oracle runner for the express/escapehtml parity harness.
//
// Reads one JSON object per line from stdin ({id, fn, args}) and writes exactly
// one JSON object per line to stdout ({id, ok, value} or {id, ok:false, error}),
// in the same order. A throwing case is reported as ok:false; the process never
// exits early. Logs go to stderr only.

const escapeHtml = require('escape-html');

const FNS = {
  // escape-html exports a single function.
  escape: (s) => escapeHtml(s),
};

function handle(req) {
  const fn = FNS[req.fn];
  if (!fn) throw new Error(`unknown fn: ${req.fn}`);
  return fn(...(req.args || []));
}

let buf = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf('\n')) !== -1) {
    const line = buf.slice(0, nl);
    buf = buf.slice(nl + 1);
    if (!line.trim()) continue;
    let req;
    try {
      req = JSON.parse(line);
    } catch (err) {
      process.stdout.write(JSON.stringify({ id: null, ok: false, error: `bad request: ${err.message}` }) + '\n');
      continue;
    }
    let out;
    try {
      out = { id: req.id, ok: true, value: handle(req) };
    } catch (err) {
      out = { id: req.id, ok: false, error: String((err && err.message) || err) };
    }
    // One line per case, flushed immediately: the harness reads synchronously.
    process.stdout.write(JSON.stringify(out) + '\n');
  }
});
