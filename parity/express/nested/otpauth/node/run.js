'use strict';
// Upstream parity runner for otpauth@9.5.1 (hectorm/otpauth), the HOTP/TOTP
// one-time-password library.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[<config>, <options>]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Nothing here reads the clock: every TOTP case passes an explicit timestamp (in
// milliseconds, as upstream expects) and every HOTP case an explicit counter.
// Secret bytes are reported as lower-case hex. Never exits on a failing case;
// logs go to stderr only.

const readline = require('readline');
const { HOTP, TOTP, Secret, URI } = require('otpauth');

// config: {type, issuer, label, secret (base32), algorithm, digits, period,
//          counter, omitIssuerFromLabel}
// Only the keys the case supplies are forwarded, so upstream applies its own
// defaults for the rest.
function build(cfg) {
  if (!cfg || typeof cfg !== 'object') throw new Error('missing config');
  const out = { secret: Secret.fromBase32(cfg.secret === undefined ? '' : cfg.secret) };
  for (const k of ['issuer', 'label', 'algorithm', 'digits']) {
    if (cfg[k] !== undefined) out[k] = cfg[k];
  }
  if (cfg.omitIssuerFromLabel === true) out.issuerInLabel = false;
  const type = (cfg.type || 'totp').toLowerCase();
  if (type === 'hotp') {
    if (cfg.counter !== undefined) out.counter = cfg.counter;
    return new HOTP(out);
  }
  if (type === 'totp') {
    if (cfg.period !== undefined) out.period = cfg.period;
    return new TOTP(out);
  }
  throw new Error('unknown otp type: ' + cfg.type);
}

function isHOTP(otp) {
  return otp instanceof HOTP && !(otp instanceof TOTP);
}

// describe renders an HOTP/TOTP object as a plain object both runners can emit.
function describe(otp) {
  const out = {
    type: isHOTP(otp) ? 'hotp' : 'totp',
    issuer: otp.issuer,
    label: otp.label,
    secret: otp.secret.base32,
    algorithm: otp.algorithm,
    digits: otp.digits,
    omitIssuerFromLabel: otp.issuerInLabel === false,
  };
  if (isHOTP(otp)) out.counter = otp.counter;
  else out.period = otp.period;
  return out;
}

function call(fn, args) {
  const cfg = args[0];
  const opts = args[1] || {};
  switch (fn) {
    case 'generate': {
      const otp = build(cfg);
      if (isHOTP(otp)) {
        return otp.generate({ counter: opts.counter !== undefined ? opts.counter : otp.counter });
      }
      if (opts.timestamp === undefined) throw new Error('totp generate needs a timestamp');
      return otp.generate({ timestamp: opts.timestamp });
    }
    case 'validate': {
      const otp = build(cfg);
      const window = opts.window === undefined ? 1 : opts.window;
      const delta = isHOTP(otp)
        ? otp.validate({ token: opts.token, counter: opts.counter !== undefined ? opts.counter : otp.counter, window })
        : otp.validate({ token: opts.token, timestamp: opts.timestamp, window });
      // null means "not in the window"; keep it as null so both runners agree.
      return delta === null || delta === undefined ? null : delta;
    }
    case 'counter': {
      // The RFC 6238 time step for a timestamp, TOTP.counter({period,timestamp}).
      const period = cfg.period === undefined ? TOTP.defaults.period : cfg.period;
      return TOTP.counter({ period, timestamp: opts.timestamp });
    }
    case 'uri':
      return build(cfg).toString();
    case 'stringify':
      return URI.stringify(build(cfg));
    case 'parse':
      return describe(URI.parse(cfg));
    case 'parseThenUri':
      return URI.parse(cfg).toString();
    case 'secretHex':
      return Secret.fromBase32(cfg).hex.toLowerCase();
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
