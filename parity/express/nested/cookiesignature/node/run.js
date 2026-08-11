'use strict';
// Upstream parity runner for cookie-signature@1.2.2 (the npm package express and
// cookie-parser use to sign cookies).
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Never exits on a failing case. Logs go to stderr only.

const readline = require('readline');
const cookieSignature = require('cookie-signature');

// unsign returns the value on success and the boolean false on failure; the Go
// port returns (value, ok). Both runners emit the same shape: the string, or
// JSON false.
function call(fn, args) {
  switch (fn) {
    case 'sign':
      return cookieSignature.sign(args[0], args[1]);
    case 'unsign':
      return cookieSignature.unsign(args[0], args[1]);
    default:
      throw new Error('unknown fn: ' + fn);
  }
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  line = line.trim();
  if (!line) return;
  let req;
  try {
    req = JSON.parse(line);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: 'bad request: ' + e.message }) + '\n');
    return;
  }
  let out;
  try {
    out = { id: req.id, ok: true, value: call(req.fn, req.args || []) };
  } catch (e) {
    out = { id: req.id, ok: false, error: String((e && e.message) || e) };
  }
  process.stdout.write(JSON.stringify(out) + '\n');
});
