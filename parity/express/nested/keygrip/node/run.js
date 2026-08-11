'use strict';
// Upstream parity runner for keygrip@1.1.0 (crypto-utils/keygrip), the rotating
// signing-key helper Express and Koa use for signed cookies.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[{"keys":[..],"algorithm":..,"encoding":..}, ..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// The first argument is always the constructor spec, so a construction failure
// (empty key list, unknown algorithm/encoding) surfaces as ok:false on the same
// case that exercises the method. Never exits on a failing case; logs go to
// stderr only.

const readline = require('readline');
const Keygrip = require('keygrip');

function build(spec) {
  if (spec === null || spec === undefined) throw new Error('missing keygrip spec');
  // Node's keygrip(keys, algorithm, encoding) applies its own defaults for
  // undefined algorithm/encoding ("sha1"/"base64").
  return new Keygrip(spec.keys, spec.algorithm, spec.encoding);
}

function call(fn, args) {
  switch (fn) {
    case 'new':
      build(args[0]);
      return true;
    case 'sign':
      return build(args[0]).sign(args[1]);
    case 'verify':
      return build(args[0]).verify(args[1], args[2]);
    case 'index':
      return build(args[0]).index(args[1], args[2]);
    // signAll re-signs the same data under every key in the list, which is how
    // a rotation corpus is built: it shows what each historical key produces.
    case 'signAll': {
      const spec = args[0];
      return spec.keys.map((k) =>
        new Keygrip([k], spec.algorithm, spec.encoding).sign(args[1])
      );
    }
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
