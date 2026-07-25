// Parity driver for Node.js: loads the REAL npm package installed alongside it
// and answers parity cases over newline-delimited JSON on stdin/stdout.
//
// Each request is {id, src, input}. `src` is evaluated as the body of an async
// function receiving the case input plus `require` and `imp` (dynamic import,
// for ESM-only packages), both resolved against this directory's node_modules.
// Whatever it returns is the upstream answer; anything it throws is upstream's
// error behavior, which ports must reproduce too.
import { createRequire } from 'node:module';
import { createInterface } from 'node:readline';

const require = createRequire(import.meta.url);
const imp = (specifier) => import(require.resolve(specifier));
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;

// encode maps runtime values onto the canonical JSON the Go side also encodes
// to, so a comparison is about behavior and never about representation.
function encode(v, seen = new Set()) {
  if (v === undefined || v === null) return null;
  if (typeof v === 'bigint') return v.toString();
  if (typeof v === 'number') {
    if (Number.isNaN(v)) return 'NaN';
    if (v === Infinity) return 'Infinity';
    if (v === -Infinity) return '-Infinity';
    return v;
  }
  if (typeof v === 'boolean' || typeof v === 'string') return v;
  if (typeof v === 'function') return { $fn: v.name || 'anonymous' };
  if (typeof v === 'symbol') return v.toString();
  if (v instanceof Date) return v.toISOString();
  if (v instanceof RegExp) return v.toString();
  if (v instanceof Error) return { $error: v.message };
  if (ArrayBuffer.isView(v) || v instanceof ArrayBuffer) {
    // Base64, matching how Go's encoding/json renders []byte.
    return Buffer.from(v.buffer ?? v, v.byteOffset ?? 0, v.byteLength ?? undefined).toString('base64');
  }
  if (seen.has(v)) return { $cycle: true };
  seen.add(v);
  try {
    if (Array.isArray(v)) return v.map((x) => encode(x, seen));
    if (v instanceof Set) return [...v].map((x) => encode(x, seen));
    if (v instanceof Map) {
      const o = {};
      for (const [k, x] of v) o[String(k)] = encode(x, seen);
      return o;
    }
    if (typeof v.toJSON === 'function') return encode(v.toJSON(), seen);
    const o = {};
    for (const k of Object.keys(v)) o[k] = encode(v[k], seen);
    return o;
  } finally {
    seen.delete(v);
  }
}

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });
for await (const line of rl) {
  if (!line.trim()) continue;
  let req;
  try {
    req = JSON.parse(line);
  } catch (err) {
    process.stdout.write(JSON.stringify({ id: '', ok: false, error: `bad request: ${err.message}` }) + '\n');
    continue;
  }
  let reply;
  try {
    const fn = new AsyncFunction('input', 'require', 'imp', req.src);
    const value = await fn(req.input ?? null, require, imp);
    reply = { id: req.id, ok: true, value: encode(value) };
  } catch (err) {
    reply = { id: req.id, ok: false, error: String((err && err.message) || err) };
  }
  process.stdout.write(JSON.stringify(reply) + '\n');
}
