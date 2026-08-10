'use strict';
// Upstream oracle runner for the express parity harness.
//
// Protocol: one JSON object per line on stdin, one JSON object per line on
// stdout (see ../../HARNESS.md).
//
//   in  {"id":"...","fn":"<appName>","args":[{"method":"GET","path":"/a"}]}
//   out {"id":"...","ok":true,"value":{"status":200,"headers":[[k,v],...],"body":"..."}}
//
// Each `fn` names an app fixture built below. The fixture is instantiated once,
// listened on a random loopback port, and every case for it replays a request
// against that port. Nothing ever leaves localhost.

const express = require('express');
const http = require('http');
const path = require('path');
const readline = require('readline');

const FIXTURES = process.env.PARITY_FIXTURES || path.join(__dirname, '..', 'fixtures');
const PUB = path.join(FIXTURES, 'pub');

// ---------------------------------------------------------------------------
// response normalisation (must match go/run.go exactly)
// ---------------------------------------------------------------------------

// Dropped outright: non-deterministic or framing-dependent.
const DROP = new Set([
  'date',
  'connection',
  'keep-alive',
  'transfer-encoding',
  'content-length',
  'x-powered-by',
]);
// Validator headers: their values are implementation-defined (hash algorithm,
// file clock), so they are dropped here and conditional-request behaviour is
// compared through status codes instead (see cases/conditional.json).
const DROP_VALIDATORS = new Set(['etag', 'last-modified']);

// Set-Cookie Expires timestamps are derived from the wall clock; keep the
// parameter but hide its value.
function canonCookie(value) {
  return value.replace(/Expires=[^;]*/gi, 'Expires=<opaque>');
}

function canonType(value) {
  // charset tokens are case-insensitive; normalise so utf-8 == UTF-8.
  return value.replace(/charset=([^;\s]+)/gi, (m, c) => 'charset=' + c.toLowerCase());
}

function normaliseHeaders(raw) {
  const out = [];
  for (let i = 0; i < raw.length; i += 2) {
    const name = String(raw[i]).toLowerCase();
    if (DROP.has(name) || DROP_VALIDATORS.has(name)) continue;
    let value = String(raw[i + 1]);
    if (name === 'content-type') value = canonType(value);
    if (name === 'set-cookie') value = canonCookie(value);
    out.push([name, value]);
  }
  out.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : a[1] < b[1] ? -1 : a[1] > b[1] ? 1 : 0));
  return out;
}

// ---------------------------------------------------------------------------
// app fixtures
// ---------------------------------------------------------------------------

const send = (s) => (req, res) => res.send(s);
const params = (req, res) => res.json(sortObj(req.params));

function sortObj(o) {
  const out = {};
  for (const k of Object.keys(o).sort()) out[k] = o[k];
  return out;
}

const apps = {};

// --- routing: static paths, verbs, precedence, fallthrough --------------
apps['route.basic'] = () => {
  const app = express();
  app.get('/', send('root'));
  app.get('/a', send('a'));
  app.get('/a/b', send('a/b'));
  app.get('/a/b/c', send('a/b/c'));
  app.post('/a', send('post-a'));
  app.put('/a', send('put-a'));
  app.delete('/a', send('delete-a'));
  app.patch('/a', send('patch-a'));
  app.options('/opt', send('opt'));
  app.head('/headonly', (req, res) => res.set('X-Head', 'yes').end());
  app.all('/all', (req, res) => res.send('all-' + req.method));
  // first registration wins
  app.get('/dup', send('first'));
  app.get('/dup', send('second'));
  // registration order decides param-vs-static
  app.get('/order/:x', (req, res) => res.send('param-' + req.params.x));
  app.get('/order/static', send('static'));
  app.get('/order2/static', send('static2'));
  app.get('/order2/:x', (req, res) => res.send('param2-' + req.params.x));
  // multiple handlers on one route
  app.get('/chain', (req, res, next) => { res.set('X-One', '1'); next(); }, send('chain-done'));
  // next() past the only handler -> 404
  app.get('/fall', (req, res, next) => next());
  app.get('/trail', send('trail'));
  app.get('/Case', send('cased'));
  // isolate routing from the framework default 404 page (compared separately
  // in the route.default404 fixture)
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};

// --- routing: parameters ------------------------------------------------
apps['route.params'] = () => {
  const app = express();
  app.get('/u/:id', params);
  app.get('/u/:id/p/:pid', params);
  app.get('/opt/:id?', params);
  app.get('/num/:id(\\d+)', params);
  app.get('/word/:w([a-z]+)', params);
  app.get('/mix/:from-:to', params);
  app.get('/star/*', params);
  app.get('/pre/*/post', params);
  app.get('/many/:a/:b/:c', params);
  // isolate routing from the framework default 404 page (compared separately
  // in the route.default404 fixture)
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};

// --- routing: mounted / nested routers ----------------------------------
apps['route.mount'] = () => {
  const app = express();

  const r1 = express.Router();
  r1.get('/', send('r1-root'));
  r1.get('/x', send('r1-x'));
  r1.get('/p/:id', params);
  app.use('/m', r1);

  const merged = express.Router({ mergeParams: true });
  merged.get('/who', params);
  app.use('/u/:uid', merged);

  const unmerged = express.Router({ mergeParams: false });
  unmerged.get('/who', params);
  app.use('/n/:nid', unmerged);

  // two levels of nesting
  const inner = express.Router();
  inner.get('/leaf', send('leaf'));
  const outer = express.Router();
  outer.use('/in', inner);
  outer.get('/own', send('outer-own'));
  app.use('/out', outer);

  // router-scoped middleware
  const mw = express.Router();
  mw.use((req, res, next) => { res.set('X-Router-MW', '1'); next(); });
  mw.get('/hit', send('mw-hit'));
  app.use('/mw', mw);

  // path-scoped middleware via app.use with a prefix
  app.use('/pfx', (req, res) => res.send('pfx:' + req.url));

  // location reporting
  const loc = express.Router();
  loc.get('/where', (req, res) => res.json(sortObj({
    baseUrl: req.baseUrl, originalUrl: req.originalUrl, path: req.path, url: req.url,
  })));
  app.use('/loc', loc);
  // isolate routing from the framework default 404 page (compared separately
  // in the route.default404 fixture)
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};

// --- routing: strict / case-sensitive settings ---------------------------
apps['route.strict'] = () => {
  const app = express();
  app.set('strict routing', true);
  app.get('/s', send('s'));
  app.get('/t/', send('t-slash'));
  // isolate routing from the framework default 404 page (compared separately
  // in the route.default404 fixture)
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};
apps['route.case'] = () => {
  const app = express();
  app.set('case sensitive routing', true);
  app.get('/Foo', send('Foo'));
  app.get('/bar', send('bar'));
  // isolate routing from the framework default 404 page (compared separately
  // in the route.default404 fixture)
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};

// --- routing: app.param callbacks ---------------------------------------
apps['route.param'] = () => {
  const app = express();
  app.param('id', (req, res, next, id) => { req.seen = (req.seen || []).concat('id:' + id); next(); });
  app.get('/p/:id', (req, res) => res.json(sortObj({ seen: req.seen || [], id: req.params.id })));
  app.get('/q/:other', (req, res) => res.json(sortObj({ seen: req.seen || [], other: req.params.other })));
  app.param('bad', (req, res, next) => next(new Error('bad param')));
  app.get('/b/:bad', send('should-not-run'));
  app.use((err, req, res, next) => res.status(500).send('perr'));
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};

// --- routing: strict / case sensitivity via app.set ----------------------
apps['route.appset'] = () => {
  const app = express();
  app.set('strict routing', true);
  app.set('case sensitive routing', true);
  app.get('/s', send('s'));
  app.get('/Foo', send('Foo'));
  app.use((req, res) => res.status(404).send('NOMATCH'));
  return app;
};

// the framework's own unmatched-route response, in isolation
apps['route.default404'] = () => {
  const app = express();
  app.get('/x', send('x'));
  return app;
};

// --- body parsing --------------------------------------------------------
apps['body'] = () => {
  const app = express();
  app.use(express.json());
  app.use(express.urlencoded({ extended: false }));
  app.post('/echo', (req, res) => res.json({ body: req.body === undefined ? null : req.body }));
  app.use((err, req, res, next) => {
    res.status(err.status || 500).json({ parseError: true });
  });
  return app;
};

// --- response helpers ----------------------------------------------------
apps['res'] = () => {
  const app = express();
  app.get('/send-string', send('plain string'));
  app.get('/send-obj', (req, res) => res.send({ a: 1, b: 'two' }));
  app.get('/send-buf', (req, res) => res.send(Buffer.from('raw bytes')));
  app.get('/json', (req, res) => res.json({ a: [1, 2, 3], z: 1 }));
  app.get('/json-key-order', (req, res) => res.json({ z: 1, m: 2, a: 3 }));
  app.get('/json-null', (req, res) => res.json(null));
  app.get('/status', (req, res) => res.status(201).send('created'));
  app.get('/send-status', (req, res) => res.sendStatus(404));
  app.get('/send-status-418', (req, res) => res.sendStatus(418));
  app.get('/redirect', (req, res) => res.redirect('/target'));
  app.get('/redirect-301', (req, res) => res.redirect(301, '/target'));
  app.get('/type-json', (req, res) => res.type('json').send('{"x":1}'));
  app.get('/type-html', (req, res) => res.type('html').send('<b>hi</b>'));
  app.get('/type-full', (req, res) => res.type('application/xml').send('<x/>'));
  app.get('/set', (req, res) => res.set('X-Custom', 'v1').set('X-Other', 'v2').send('set'));
  app.get('/append', (req, res) => { res.append('X-Multi', 'a'); res.append('X-Multi', 'b'); res.send('append'); });
  app.get('/cookie', (req, res) => res.cookie('c', 'v', { path: '/', httpOnly: true, maxAge: 60000 }).send('cookie'));
  app.get('/cookie-plain', (req, res) => res.cookie('p', 'q').send('cookie-plain'));
  app.get('/clear-cookie', (req, res) => res.clearCookie('c').send('cleared'));
  app.get('/location', (req, res) => res.location('/somewhere').send('loc'));
  app.get('/links', (req, res) => res.links({ next: 'http://x/?p=2' }).send('links'));
  app.get('/vary', (req, res) => res.vary('Accept-Encoding').send('vary'));
  app.get('/jsonp', (req, res) => res.jsonp({ a: 1 }));
  app.get('/end', (req, res) => res.status(204).end());
  app.get('/read-cookie', (req, res) => res.send('cookie=' + (req.cookies ? req.cookies.c : undefined)));
  app.get('/req-info', (req, res) => res.json(sortObj({
    method: req.method, path: req.path, protocol: req.protocol, secure: req.secure,
    xhr: req.xhr, contentType: req.get('content-type') || '', hostSeen: !!req.hostname,
  })));
  app.get('/query', (req, res) => res.json({ a: req.query.a === undefined ? null : req.query.a, b: req.query.b === undefined ? null : req.query.b }));
  return app;
};

// --- content negotiation -------------------------------------------------
apps['negotiate'] = () => {
  const app = express();
  app.get('/format', (req, res) => {
    res.format({
      'text/plain': () => res.send('as text'),
      'text/html': () => res.send('<p>as html</p>'),
      'application/json': () => res.json({ as: 'json' }),
    });
  });
  // A single-entry map so the "nothing matched" fallback is well defined on
  // both sides (the Go port takes an unordered Go map, so a multi-entry
  // fallback has no defined winner there).
  app.get('/format-one', (req, res) => {
    res.format({ 'text/plain': () => res.send('only text') });
  });
  app.get('/format-nodefault', (req, res) => {
    res.format({ 'application/json': () => res.json({ only: 'json' }) });
  });
  // keep the 406 comparison portable: upstream's default error page embeds
  // absolute file paths and a stack trace
  app.use((err, req, res, next) => res.status(err.status || 500).send('NOTACCEPTABLE'));
  app.get('/accepts', (req, res) => res.json(sortObj({
    best: req.accepts(['html', 'json']) || null,
    jsonOnly: req.accepts('json') || null,
    lang: req.acceptsLanguages('en', 'fr') || null,
    enc: req.acceptsEncodings('gzip', 'identity') || null,
    charset: req.acceptsCharsets('utf-8', 'iso-8859-1') || null,
  })));
  return app;
};

// --- conditional GET / etag ---------------------------------------------
apps['etag'] = () => {
  const app = express();
  app.get('/body', send('etag me please'));
  app.get('/json', (req, res) => res.json({ etag: true }));
  app.get('/manual', (req, res) => {
    res.set('ETag', '"fixed-tag"');
    if (req.headers['if-none-match'] === '"fixed-tag"') { res.status(304).end(); return; }
    res.send('manual etag');
  });
  return app;
};

// --- static file serving -------------------------------------------------
apps['static'] = () => {
  const app = express();
  app.use(express.static(PUB));
  app.use('/assets', express.static(PUB));
  app.use((req, res) => res.status(404).send('no-file'));
  return app;
};

// --- error handling ------------------------------------------------------
apps['errors'] = () => {
  const app = express();

  app.get('/next-err', (req, res, next) => next(new Error('boom')));
  app.get('/throw', () => { throw new Error('thrown'); });
  app.get('/ok', send('ok'));

  // an error raised BEFORE a mounted sub-router must skip the sub-router
  const sub = express.Router();
  sub.get('/inside', send('should-not-run'));
  app.get('/pre-sub', (req, res, next) => next(new Error('before-sub')));
  app.use('/sub', (req, res, next) => next(new Error('sub-mw-error')), sub);

  // a router-local error handler wins over the app-level one
  const local = express.Router();
  local.get('/fail', (req, res, next) => next(new Error('local')));
  local.use((err, req, res, next) => res.status(500).send('local-handler:' + err.message));
  app.use('/local', local);

  app.use((req, res) => res.status(404).send('custom-404'));
  app.use((err, req, res, next) => res.status(500).send('app-handler:' + err.message));
  return app;
};

// ---------------------------------------------------------------------------
// server management + request replay
// ---------------------------------------------------------------------------

const servers = new Map();

function serverFor(name) {
  if (servers.has(name)) return servers.get(name);
  const factory = apps[name];
  if (!factory) return Promise.reject(new Error('unknown app fixture: ' + name));
  const p = new Promise((resolve, reject) => {
    let app;
    try { app = factory(); } catch (e) { reject(e); return; }
    const srv = http.createServer(app);
    srv.on('error', reject);
    srv.listen(0, '127.0.0.1', () => resolve({ srv, port: srv.address().port }));
  });
  servers.set(name, p);
  return p;
}

function doRequest(port, spec) {
  return new Promise((resolve, reject) => {
    const opts = {
      host: '127.0.0.1',
      port,
      method: spec.method || 'GET',
      path: spec.path,
      headers: Object.assign({}, spec.headers || {}),
      agent: false,
    };
    const req = http.request(opts, (res) => {
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => resolve({
        status: res.statusCode,
        rawHeaders: res.rawHeaders,
        body: Buffer.concat(chunks).toString('utf8'),
      }));
    });
    req.on('error', reject);
    if (spec.body !== undefined && spec.body !== null) req.write(spec.body);
    req.end();
  });
}

async function replay(name, spec) {
  const { port } = await serverFor(name);
  let raw = await doRequest(port, spec);
  if (spec.revalidate) {
    // Second pass: re-issue with the validator the first response produced, so
    // 304 handling is compared without depending on the ETag algorithm.
    const hdrs = Object.assign({}, spec.headers || {});
    const idx = raw.rawHeaders.findIndex((h, i) => i % 2 === 0 && String(h).toLowerCase() === (spec.revalidate === 'lastModified' ? 'last-modified' : 'etag'));
    if (idx >= 0) {
      hdrs[spec.revalidate === 'lastModified' ? 'If-Modified-Since' : 'If-None-Match'] = raw.rawHeaders[idx + 1];
    } else {
      return { status: 0, headers: [], body: '', validator: 'absent' };
    }
    raw = await doRequest(port, Object.assign({}, spec, { headers: hdrs }));
  }
  return {
    status: raw.status,
    headers: normaliseHeaders(raw.rawHeaders),
    body: raw.body,
  };
}

// ---------------------------------------------------------------------------
// main loop: strictly sequential, never exits on a failing case
// ---------------------------------------------------------------------------

const rl = readline.createInterface({ input: process.stdin });
let chain = Promise.resolve();

rl.on('line', (line) => {
  if (!line.trim()) return;
  chain = chain.then(async () => {
    let c;
    try {
      c = JSON.parse(line);
    } catch (e) {
      process.stdout.write(JSON.stringify({ id: '?', ok: false, error: 'bad input: ' + e.message }) + '\n');
      return;
    }
    try {
      const value = await replay(c.fn, (c.args && c.args[0]) || {});
      process.stdout.write(JSON.stringify({ id: c.id, ok: true, value }) + '\n');
    } catch (e) {
      process.stdout.write(JSON.stringify({ id: c.id, ok: false, error: String((e && e.message) || e) }) + '\n');
    }
  });
});

rl.on('close', () => {
  chain.then(() => {
    for (const p of servers.values()) p.then(({ srv }) => srv.close()).catch(() => {});
  });
});
