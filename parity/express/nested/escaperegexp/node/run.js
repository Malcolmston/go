// Upstream oracle runner for the express/escaperegexp parity harness.
//
// escape-string-regexp 5.x is pure ESM, so this runner is an ES module
// ("type": "module" in package.json).
//
// Reads one JSON object per line from stdin ({id, fn, args}) and writes exactly
// one JSON object per line to stdout ({id, ok, value} or {id, ok:false, error}),
// in the same order. A throwing case is reported as ok:false; the process never
// exits early. Logs go to stderr only.

import escapeStringRegexp from 'escape-string-regexp';

const FNS = {
  // escape-string-regexp has a single default export.
  escapeRegExp: (s) => escapeStringRegexp(s),

  // roundTrip escapes the needle, compiles it as a whole pattern and reports
  // whether it matches the needle literally and how many matches it finds in a
  // haystack. This checks the *purpose* of the function, not just its bytes.
  roundTrip: (needle, haystack) => {
    const re = new RegExp(escapeStringRegexp(needle), 'g');
    return {
      matchesSelf: new RegExp('^' + escapeStringRegexp(needle) + '$').test(needle),
      count: (String(haystack).match(re) || []).length,
    };
  },

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
