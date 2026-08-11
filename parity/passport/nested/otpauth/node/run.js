// Upstream oracle runner for parity/passport/nested/otpauth.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one reply
// object per line out, in the same order. The oracle is otpauth@9.5.1, the npm
// HOTP/TOTP library that also owns the "otpauth://" key-URI format the Go
// package is named after.
//
// Conventions, mirrored by go/run.go:
//   * A key is the object {type, issuer, account, secret, algorithm, digits,
//     period, counter}; `secret` is base32 without padding, as the Key Uri
//     Format specifies. Raw bytes (decodeSecret) are lowercase hex.
//   * Timestamps are explicit milliseconds since the epoch. Nothing here reads
//     the clock, so every code and every verification verdict is reproducible.
//   * `url` answers a STRUCTURED rendering — {host, path, params} with the query
//     as an object — because RFC-free parameter order is not a parity question;
//     percent-encoding still is, so `path` is the raw encoded path.
//   * `verify` answers {ok, delta} and never an error: an unusable key cannot
//     verify a code. `code`/`codeAtCounter` do propagate the error, since there
//     the answer is a value and there is none.
//
// Never exits on a failing case: the throw is reported as {ok:false,error}.
import readline from 'node:readline';
import { HOTP, Secret, TOTP, URI } from 'otpauth';

function build(k) {
  const common = {
    issuer: k.issuer ?? '',
    label: k.account ?? '',
    secret: Secret.fromBase32(k.secret ?? ''),
    algorithm: k.algorithm || 'SHA1',
    digits: k.digits || 6,
  };
  if ((k.type ?? 'totp') === 'hotp') {
    return new HOTP({ ...common, counter: k.counter ?? 0 });
  }
  return new TOTP({ ...common, period: k.period || 30 });
}

// normalise projects an upstream TOTP/HOTP instance onto the field set both
// sides emit, so a parameter that defaults on one side and is explicit on the
// other still compares equal.
function normalise(otp) {
  const isHOTP = otp instanceof HOTP;
  return {
    type: isHOTP ? 'hotp' : 'totp',
    issuer: otp.issuer ?? '',
    account: otp.label ?? '',
    secret: otp.secret.base32,
    algorithm: otp.algorithm,
    digits: otp.digits,
    period: isHOTP ? 0 : otp.period,
    counter: isHOTP ? otp.counter : 0,
  };
}

function structuredURI(uriString) {
  const u = new URL(uriString);
  const params = {};
  for (const [k, v] of [...u.searchParams.entries()].sort()) params[k] = v;
  return { host: u.host, path: u.pathname, params };
}

const ops = {
  parse([uri]) {
    return normalise(URI.parse(uri));
  },

  url([key]) {
    return structuredURI(build(key).toString());
  },

  decodeSecret([secret]) {
    return Secret.fromBase32(secret).hex.toLowerCase();
  },

  usable([key]) {
    try {
      const otp = build(key);
      otp.generate(otp instanceof HOTP ? { counter: 0 } : { timestamp: 0 });
      return true;
    } catch {
      return false;
    }
  },

  code([key, timestampMs]) {
    return build(key).generate({ timestamp: timestampMs });
  },

  codeAtCounter([key, counter]) {
    return build(key).generate({ counter });
  },

  movingFactor([key, timestampMs]) {
    return build(key).counter({ timestamp: timestampMs });
  },

  verify([key, code, timestampMs, window]) {
    let delta;
    try {
      const otp = build(key);
      delta =
        otp instanceof HOTP
          ? otp.validate({ token: code, counter: otp.counter, window })
          : otp.validate({ token: code, timestamp: timestampMs, window });
    } catch {
      return { ok: false, delta: 0 };
    }
    return delta === null || delta === undefined ? { ok: false, delta: 0 } : { ok: true, delta };
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
