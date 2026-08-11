// Upstream oracle runner for parity/passport/nested/pwhash.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one reply
// object per line out, in the same order. The oracle is pbkdf2@3.1.6, the npm
// implementation of PBKDF2 (RFC 2898 / PKCS #5), which is the primitive the Go
// pwhash package is built on.
//
// Conventions, mirrored by go/run.go:
//   * Passwords and salts arrive as strings and are hashed as their UTF-8 bytes
//     on both sides. Derived keys are lowercase hex.
//   * Every parameter is explicit: a fixed salt, a fixed iteration count, a
//     fixed key length. Nothing is generated, so every answer is reproducible.
//   * `key` propagates the error, because a rejected parameter set has no
//     derived key to report. `verifyKnownKey` answers a verdict (true/false),
//     because there the question is whether a password matches a stored hash,
//     and "these parameters are unusable" is a rejection.
//   * MAX_ITERATIONS mirrors the port's pwhash.MaxIterations. It is NOT an
//     upstream behaviour — upstream has no bound — it is this runner's own guard
//     so a case asking for a hundred million iterations cannot wedge the
//     harness. The single case that crosses it is marked a deviation.
//
// Never exits on a failing case: the throw is reported as {ok:false,error}.
import readline from 'node:readline';
import crypto from 'node:crypto';
import { pbkdf2Sync } from 'pbkdf2';

const MAX_ITERATIONS = 100_000_000;

function derive({ password, salt, iterations, keyLength, algorithm }) {
  if (!Number.isInteger(iterations) || iterations > MAX_ITERATIONS) {
    throw new Error(`iterations out of the harness guard range: ${iterations}`);
  }
  return pbkdf2Sync(
    Buffer.from(String(password), 'utf8'),
    Buffer.from(String(salt), 'utf8'),
    iterations,
    keyLength,
    String(algorithm),
  ).toString('hex');
}

const ops = {
  key([password, salt, iterations, keyLength, algorithm]) {
    return derive({ password, salt, iterations, keyLength, algorithm });
  },

  // verifyKnownKey answers the question a login asks: does this password
  // reproduce the stored derived key? The stored key travels as hex, and the
  // comparison is constant-time, as it must be on both sides.
  verifyKnownKey([password, salt, iterations, algorithm, expectedHex]) {
    if (typeof expectedHex !== 'string' || expectedHex.length === 0 || expectedHex.length % 2 !== 0) {
      return false;
    }
    let expected;
    try {
      expected = Buffer.from(expectedHex, 'hex');
    } catch {
      return false;
    }
    if (expected.length * 2 !== expectedHex.length) return false;
    let actual;
    try {
      actual = Buffer.from(derive({ password, salt, iterations, keyLength: expected.length, algorithm }), 'hex');
    } catch {
      return false;
    }
    return actual.length === expected.length && crypto.timingSafeEqual(actual, expected);
  },
};

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on('line', (line) => {
  const text = line.trim();
  if (!text) return;
  let req;
  try {
    req = JSON.parse(text);
  } catch (err) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: `bad request: ${err.message}` }) + '\n');
    return;
  }
  let out;
  try {
    const op = ops[req.fn];
    if (!op) throw new Error(`unknown fn ${req.fn}`);
    out = { id: req.id, ok: true, value: op(req.args ?? []) };
  } catch (err) {
    out = { id: req.id, ok: false, error: String((err && err.message) || err) };
  }
  process.stdout.write(JSON.stringify(out) + '\n');
});
