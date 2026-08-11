// Upstream oracle runner for parity/passport/nested/httpauth.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one reply
// object per line out, in the same order. The oracle is http-auth-utils@7.0.1,
// the npm package that parses and builds HTTP Authorization and
// WWW-Authenticate headers for the Basic, Bearer and Digest schemes.
//
// Conventions, all mirrored by go/run.go:
//   * `bodyHex` arguments are lowercase hex; there is no other byte encoding.
//   * Digest algorithm names arrive in their RFC 7616 spelling ("MD5",
//     "SHA-256", "SHA-512-256", "" for the default) and are translated to the
//     node crypto digest name before reaching upstream, which passes the value
//     straight to crypto.createHash. Anything unrecognised is passed through so
//     createHash throws, which is the upstream answer for an unknown algorithm.
//   * Authorization headers are parsed non-strictly ({strict: false}) so the
//     scheme token is matched case-insensitively, as RFC 7235 section 2.1
//     requires. This is upstream's own opt-in for that behaviour.
//   * `schemeOf`/`hasScheme` map upstream's E_UNKNOWN_AUTH_MECHANISM to "no
//     known scheme" ("" / false) rather than an error, because the semantics
//     under comparison are "which scheme does this header name", whose negative
//     answer is a value on the Go side.
//
// Never exits on a failing case: the throw is reported as {ok:false,error}.
import readline from 'node:readline';
import crypto from 'node:crypto';
import {
  BASIC,
  BEARER,
  DIGEST,
  buildAuthorizationHeader,
  buildWWWAuthenticateHeader,
  parseAuthorizationHeader,
} from 'http-auth-utils';

const NON_STRICT = { strict: false };

function parseAuth(header, mechanism) {
  return parseAuthorizationHeader(header, mechanism ? [mechanism] : undefined, NON_STRICT);
}

function nodeDigestName(algorithm) {
  switch (String(algorithm ?? '')) {
    case '':
    case 'MD5':
      return 'md5';
    case 'SHA-256':
      return 'sha256';
    case 'SHA-512-256':
      return 'sha512-256';
    default:
      // Unknown to RFC 7616 too: hand it to createHash and let it throw.
      return String(algorithm);
  }
}

function digestResponse(p) {
  const algorithm = nodeDigestName(p.algorithm);
  // Fail loudly for an unusable algorithm, exactly as computeHash would.
  crypto.createHash(algorithm);
  return DIGEST.computeHash({
    algorithm,
    username: p.username ?? '',
    realm: p.realm ?? '',
    password: p.password ?? '',
    method: p.method ?? '',
    uri: p.uri ?? '',
    nonce: p.nonce ?? '',
    nc: p.nc ?? '',
    cnonce: p.cnonce ?? '',
    qop: p.qop ?? '',
  });
}

// normaliseDigest projects an upstream parsed Digest credential onto the fixed
// field set both sides emit, so a field absent upstream and an empty field in Go
// compare equal.
function normaliseDigest(d) {
  return {
    username: d.username ?? '',
    realm: d.realm ?? '',
    nonce: d.nonce ?? '',
    uri: d.uri ?? '',
    response: d.response ?? '',
    algorithm: d.algorithm ?? '',
    cnonce: d.cnonce ?? '',
    opaque: d.opaque ?? '',
    qop: d.qop ?? '',
    nc: d.nc ?? '',
  };
}

// definedOnly drops undefined/empty-string entries so upstream's build helpers,
// which emit any key that is not `undefined`, do not render empty parameters the
// Go side omits.
function definedOnly(obj) {
  const out = {};
  for (const [k, v] of Object.entries(obj)) {
    if (v !== undefined && v !== null && v !== '' && v !== false) out[k] = v;
  }
  return out;
}

const ops = {
  schemeOf([header]) {
    try {
      return parseAuth(header).type.toLowerCase();
    } catch (err) {
      if (err.code === 'E_UNKNOWN_AUTH_MECHANISM') return '';
      throw err;
    }
  },

  hasScheme([header, scheme]) {
    try {
      return parseAuth(header).type.toLowerCase() === String(scheme).toLowerCase();
    } catch (err) {
      if (err.code === 'E_UNKNOWN_AUTH_MECHANISM') return false;
      throw err;
    }
  },

  encodeBasic([username, password]) {
    return buildAuthorizationHeader(BASIC, { hash: BASIC.computeHash({ username, password }) });
  },

  parseBasic([header]) {
    const { username, password } = parseAuth(header, BASIC).data;
    return { username, password };
  },

  basicMatches([header, username, password]) {
    const d = parseAuth(header, BASIC).data;
    return d.username === username && d.password === password;
  },

  basicChallenge([realm]) {
    // realm is always passed, even empty: RFC 7617 makes it mandatory, and the
    // Go side likewise always emits it.
    return buildWWWAuthenticateHeader(BASIC, { realm });
  },

  encodeBearer([token]) {
    return buildAuthorizationHeader(BEARER, { hash: token });
  },

  parseBearer([header]) {
    return parseAuth(header, BEARER).data.hash;
  },

  bearerChallenge([realm, errCode]) {
    return buildWWWAuthenticateHeader(BEARER, definedOnly({ realm, error: errCode }));
  },

  parseDigest([header]) {
    return normaliseDigest(parseAuth(header, DIGEST).data);
  },

  digestChallenge([p]) {
    return buildWWWAuthenticateHeader(
      DIGEST,
      definedOnly({
        realm: p.realm,
        nonce: p.nonce,
        domain: p.domain,
        opaque: p.opaque,
        stale: p.stale ? 'true' : undefined,
        algorithm: p.algorithm,
        qop: p.qop,
        charset: p.charset,
        userhash: p.userhash ? 'true' : undefined,
      }),
    );
  },

  digestResponse([p]) {
    return digestResponse(p);
  },

  // verifyDigest composes upstream's parser and hash the way a server would. A
  // parse failure propagates (both sides fail), but an algorithm no hash
  // implementation supports is a rejection rather than an error, matching the Go
  // side's documented "report false for an unsupported algorithm".
  verifyDigest([header, method, password, bodyHex]) {
    const d = parseAuth(header, DIGEST).data;
    let expected;
    try {
      expected = digestResponse({
        method,
        uri: d.uri,
        username: d.username,
        realm: d.realm,
        password,
        nonce: d.nonce,
        nc: d.nc ?? '',
        cnonce: d.cnonce ?? '',
        qop: d.qop ?? '',
        algorithm: d.algorithm ?? '',
        bodyHex,
      });
    } catch (err) {
      if (err.code === 'ERR_OSSL_EVP_UNSUPPORTED' || /digital envelope|unsupported|Unknown|not supported/i.test(String(err.message))) {
        return false;
      }
      throw err;
    }
    return expected === (d.response ?? '');
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
