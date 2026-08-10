'use strict';
//
// Upstream oracle runner for parity/passport.
//
// Speaks JSON Lines on stdio (see ../../HARNESS.md). For each case it rebuilds a
// fresh express + passport application (so no state leaks between cases), replays
// the request(s) described by the case against it over loopback, and emits the
// authentication OUTCOME.
//
// Nothing here talks to a remote identity provider: every strategy compared is
// self-contained (local, http basic, http digest, http bearer).
//
// Determinism / normalisation, applied identically by the Go runner:
//   * the digest challenge nonce is replaced with the literal <NONCE>
//   * session ids are never emitted, only `setCookiePresent`
//   * `user` is normalised to the principal's username (upstream yields a user
//     object, the Go digest port yields a bare username string)
//   * Date/Content-Length/ETag and every other volatile header are dropped
//   * the per-case digest (nonce, nc) replay ledger is cleared before each case
//
const http = require('http');
const express = require('express');
const expressSession = require('express-session');
const passport = require('passport');
const LocalStrategy = require('passport-local').Strategy;
const { BasicStrategy, DigestStrategy } = require('passport-http');
const BearerStrategy = require('passport-http-bearer').Strategy;

// ------------------------------------------------------------ fixed fixtures

const REALM = 'Parity';
const ADMIN_REALM = 'Admin';
const SESSION_COOKIE = 'connect.sid';

// Fixed in-memory user store. Identical table in the Go runner.
const USERS = {
  alice: { id: 'u-alice', username: 'alice', password: 's3cr3t-alice' },
  bob: { id: 'u-bob', username: 'bob', password: 'hunter2' },
};
// Fixed opaque bearer tokens.
const TOKENS = { 'tok-alice': 'alice', 'tok-bob': 'bob' };

function principal(user) {
  if (user === null || user === undefined || user === false) return null;
  if (typeof user === 'string') return user;
  if (typeof user === 'object' && typeof user.username === 'string') return user.username;
  return String(user);
}

// ------------------------------------------------------------ app assembly

// buildApp returns a brand-new express app with its own session store, its own
// passport instance and its own digest replay ledger.
function buildApp() {
  const used = new Set(); // (nonce, nc) pairs already spent, for digest replay

  const pp = new passport.Passport();

  pp.serializeUser((user, done) => done(null, user.id));
  pp.deserializeUser((id, done) => {
    for (const u of Object.values(USERS)) if (u.id === id) return done(null, u);
    return done(null, false);
  });

  pp.use(
    new LocalStrategy((username, password, done) => {
      const u = USERS[username];
      if (!u || u.password !== password) return done(null, false);
      return done(null, u);
    })
  );

  const basicVerify = (username, password, done) => {
    const u = USERS[username];
    if (!u || u.password !== password) return done(null, false);
    return done(null, u);
  };
  pp.use('basic', new BasicStrategy({ realm: REALM }, basicVerify));
  pp.use('basic-admin', new BasicStrategy({ realm: ADMIN_REALM }, basicVerify));

  pp.use(
    'digest',
    new DigestStrategy(
      { realm: REALM, qop: 'auth' },
      (username, done) => {
        const u = USERS[username];
        if (!u) return done(null, false);
        return done(null, u, u.password);
      },
      (params, done) => {
        // Upstream's replay hook: reject a (nonce, nc) pair that was already
        // spent. The Go port exposes no equivalent option at all.
        const key = String(params.nonce) + ':' + String(params.nc);
        if (used.has(key)) return done(null, false);
        used.add(key);
        return done(null, true);
      }
    )
  );

  pp.use(
    'bearer',
    new BearerStrategy({ realm: REALM }, (token, done) => {
      const name = TOKENS[token];
      if (!name) return done(null, false);
      return done(null, USERS[name]);
    })
  );

  const app = express();
  app.use(express.urlencoded({ extended: false }));
  app.use(express.json());
  app.use(
    expressSession({
      name: SESSION_COOKIE,
      secret: 'parity-fixed-session-secret',
      resave: false,
      saveUninitialized: false,
    })
  );
  app.use(pp.initialize());
  app.use(pp.session());

  // Terminal handler: reports the authentication outcome of the request.
  const ok = (req, res) => {
    res.setHeader('content-type', 'application/json');
    res.end(
      JSON.stringify({
        __parity: true,
        authenticated: req.isAuthenticated ? !!req.isAuthenticated() : false,
        user: principal(req.user),
      })
    );
  };

  // --- local ---------------------------------------------------------------
  app.all('/login/local', pp.authenticate('local', { session: true }), ok);
  app.all('/login/local-nosession', pp.authenticate('local', { session: false }), ok);
  // Options with no explicit session key: upstream defaults session:true.
  app.all('/login/local-defaults', pp.authenticate('local', { successRedirect: '/ok' }), ok);
  app.all('/login/local-failredirect', pp.authenticate('local', { session: true, failureRedirect: '/denied' }), ok);

  // --- http basic ----------------------------------------------------------
  app.all('/basic', pp.authenticate('basic', { session: false }), ok);
  app.all('/basic-admin', pp.authenticate('basic-admin', { session: false }), ok);

  // --- http digest ---------------------------------------------------------
  app.all('/digest', pp.authenticate('digest', { session: false }), ok);
  app.all('/digest/other', pp.authenticate('digest', { session: false }), ok);

  // --- bearer --------------------------------------------------------------
  app.all('/bearer', pp.authenticate('bearer', { session: false }), ok);

  // --- session / route protection -----------------------------------------
  app.all('/whoami', ok);
  app.all(
    '/protected',
    (req, res, next) => {
      if (req.isAuthenticated()) return next();
      res.statusCode = 401;
      res.end(http.STATUS_CODES[401]);
    },
    ok
  );
  app.all('/logout', (req, res, next) => {
    req.logout({ keepSessionInfo: false }, (err) => (err ? next(err) : ok(req, res)));
  });
  app.all('/ok', ok);
  app.all('/denied', (req, res) => {
    res.statusCode = 403;
    ok(req, res);
  });

  return app;
}

// ------------------------------------------------------------ request replay

let currentApp = buildApp();
const server = http.createServer((req, res) => currentApp(req, res));

function cookieHeader(mode, jar) {
  if (!mode || mode === 'none') return null;
  const parts = [];
  for (const [k, v] of Object.entries(jar)) {
    let val = v;
    if (mode === 'tamper' && k === SESSION_COOKIE) {
      // Flip the final character so the id/signature no longer matches.
      val = val.slice(0, -1) + (val.slice(-1) === 'a' ? 'b' : 'a');
    }
    parts.push(k + '=' + val);
  }
  return parts.length ? parts.join('; ') : null;
}

function normaliseChallenge(v) {
  if (v === null || v === undefined) return null;
  return String(v).replace(/nonce="[^"]*"/g, 'nonce="<NONCE>"');
}

async function replay(spec, jar, base) {
  const headers = {};
  for (const [k, v] of Object.entries(spec.headers || {})) headers[k.toLowerCase()] = v;
  let body;
  if (spec.form !== undefined) {
    headers['content-type'] = 'application/x-www-form-urlencoded';
    body = new URLSearchParams(spec.form).toString();
  } else if (spec.json !== undefined) {
    headers['content-type'] = 'application/json';
    body = JSON.stringify(spec.json);
  } else if (spec.raw !== undefined) {
    body = spec.raw;
  }
  const ck = cookieHeader(spec.session, jar);
  if (ck !== null) headers['cookie'] = ck;

  const res = await fetch(base + (spec.path || '/'), {
    method: spec.method || 'GET',
    headers,
    body,
    redirect: 'manual',
  });

  let setCookiePresent = false;
  for (const raw of res.headers.getSetCookie()) {
    const eq = raw.indexOf('=');
    const name = raw.slice(0, eq);
    const value = raw.slice(eq + 1).split(';')[0];
    if (value !== '') jar[name] = value;
    else delete jar[name];
    if (name === SESSION_COOKIE && value !== '') setCookiePresent = true;
  }

  const text = await res.text();
  let authenticated = false;
  let user = null;
  try {
    const parsed = JSON.parse(text);
    if (parsed && parsed.__parity === true) {
      authenticated = !!parsed.authenticated;
      user = parsed.user === undefined ? null : parsed.user;
    }
  } catch (_) {
    /* passport wrote its own non-JSON failure body */
  }

  return {
    status: res.status,
    authenticated,
    user,
    wwwAuthenticate: normaliseChallenge(res.headers.get('www-authenticate')),
    setCookiePresent,
    location: res.headers.get('location') || null,
  };
}

// ------------------------------------------------------------ JSON Lines loop

function out(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}

async function handle(req) {
  currentApp = buildApp(); // fresh state per case: sessions + digest ledger
  const base = 'http://127.0.0.1:' + server.address().port;
  const jar = {};

  if (req.fn === 'request') {
    const spec = req.args && req.args[0];
    if (!spec) throw new Error('request: missing spec');
    return await replay(spec, jar, base);
  }
  if (req.fn === 'sequence') {
    const specs = req.args && req.args[0];
    if (!Array.isArray(specs)) throw new Error('sequence: args[0] must be an array');
    const outs = [];
    for (const spec of specs) outs.push(await replay(spec, jar, base));
    return outs;
  }
  throw new Error('unknown fn: ' + String(req.fn));
}

async function main() {
  await new Promise((res) => server.listen(0, '127.0.0.1', res));

  let buf = '';
  // A single serial promise chain: every line is appended to it, so replies come
  // out in request order and `end` can simply await the tail.
  let chain = Promise.resolve();

  function enqueue(line) {
    chain = chain.then(async () => {
      let id = null;
      try {
        const req = JSON.parse(line);
        id = req.id;
        const value = await handle(req);
        out({ id, ok: true, value });
      } catch (err) {
        // Never exit on a failing case.
        out({ id, ok: false, error: err && err.message ? err.message : String(err) });
      }
    });
  }

  process.stdin.setEncoding('utf8');
  process.stdin.on('data', (chunk) => {
    buf += chunk;
    let i;
    while ((i = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, i).trim();
      buf = buf.slice(i + 1);
      if (line) enqueue(line);
    }
  });
  process.stdin.on('end', async () => {
    if (buf.trim()) enqueue(buf.trim());
    await chain;
    server.close();
    process.exit(0);
  });
}

main().catch((e) => {
  process.stderr.write('fatal: ' + e.stack + '\n');
  process.exit(1);
});
