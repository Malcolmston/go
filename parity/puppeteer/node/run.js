'use strict';
// Upstream (Node) parity runner for the *real* surface of
// github.com/malcolmston/puppeteer: HTML parsing, CSS selector semantics and
// net/http navigation + form serialisation.
//
// The oracle is deliberately NOT puppeteer: the Go port contains no CDP, no
// websocket, no Evaluate and no Screenshot, so puppeteer's browser-automation
// surface has nothing to compare against (see ../COVERAGE.md). The oracles are:
//   * cheerio@1.0.0 (parse5 + css-select) — HTML tree construction, serialisation
//     and CSS selector matching, in fragment mode so no html/head/body is
//     synthesised (the Go parser does not synthesise them either).
//   * tough-cookie@5.1.2 — RFC 6265 cookie jar.
//   * node's built-in WHATWG fetch/undici + URL — status, redirect following and
//     relative URL resolution.
// HTTP cases run against an in-process loopback fixture server; nothing is
// downloaded and no browser is launched.
//
// Protocol: one JSON object per line on stdin, exactly one JSON object per line
// on stdout, same order. Never exits on a failing case: every throw is reported
// as {"ok":false,"error":"..."}.
//
// Usage: node run.js [path/to/fixtures.json]

const fs = require('fs');
const http = require('http');
const path = require('path');
const cheerio = require('cheerio');
const { CookieJar } = require('tough-cookie');

const fixturesPath = process.argv[2] || path.join(__dirname, '..', 'fixtures', 'fixtures.json');
const FIXTURES = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'));

// ---------------------------------------------------------------- canonical HTML
//
// Attribute order and tag/attribute case are normalised identically by both
// runners so a pure serialisation-order difference is not reported as a
// divergence. See COVERAGE.md ("Normalisation") for the exact rules.

const RAW_TEXT = new Set(['script', 'style', 'xmp', 'iframe', 'noembed', 'noframes', 'plaintext']);

function isAlpha(c) {
  return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z');
}
function isSpace(c) {
  return c === ' ' || c === '\t' || c === '\n' || c === '\r' || c === '\f';
}

function canon(html) {
  if (typeof html !== 'string') return html;
  let out = '';
  let i = 0;
  const n = html.length;
  while (i < n) {
    const c = html[i];
    if (c !== '<') { out += c; i++; continue; }
    if (html.startsWith('<!--', i)) {
      const end = html.indexOf('-->', i + 4);
      if (end < 0) { out += html.slice(i); break; }
      out += html.slice(i, end + 3);
      i = end + 3;
      continue;
    }
    if (html.startsWith('<!', i)) { // doctype / declaration
      const end = html.indexOf('>', i);
      if (end < 0) { out += html.slice(i); break; }
      out += html.slice(i, end + 1).toLowerCase();
      i = end + 1;
      continue;
    }
    if (html.startsWith('</', i) && isAlpha(html[i + 2] || '')) {
      let j = i + 2;
      while (j < n && html[j] !== '>') j++;
      out += '</' + html.slice(i + 2, j).trim().toLowerCase() + '>';
      i = Math.min(j + 1, n);
      continue;
    }
    if (!isAlpha(html[i + 1] || '')) { out += c; i++; continue; }
    // start tag
    let j = i + 1;
    while (j < n && !isSpace(html[j]) && html[j] !== '>' && html[j] !== '/') j++;
    const name = html.slice(i + 1, j).toLowerCase();
    const attrs = [];
    while (j < n) {
      while (j < n && isSpace(html[j])) j++;
      if (j >= n) break;
      if (html[j] === '>') { j++; break; }
      if (html[j] === '/') { j++; continue; }
      let k = j;
      while (k < n && !isSpace(html[k]) && html[k] !== '=' && html[k] !== '>' && html[k] !== '/') k++;
      const key = html.slice(j, k).toLowerCase();
      let val = '';
      j = k;
      while (j < n && isSpace(html[j])) j++;
      if (j < n && html[j] === '=') {
        j++;
        while (j < n && isSpace(html[j])) j++;
        if (j < n && (html[j] === '"' || html[j] === "'")) {
          const q = html[j];
          const end = html.indexOf(q, j + 1);
          val = end < 0 ? html.slice(j + 1) : html.slice(j + 1, end);
          j = end < 0 ? n : end + 1;
        } else {
          const s = j;
          while (j < n && !isSpace(html[j]) && html[j] !== '>') j++;
          val = html.slice(s, j);
        }
      }
      if (key !== '') attrs.push([key, val]);
    }
    attrs.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
    out += '<' + name;
    for (const [k, v] of attrs) out += ' ' + k + '="' + v + '"';
    out += '>';
    i = j;
    if (RAW_TEXT.has(name)) {
      const close = html.toLowerCase().indexOf('</' + name, i);
      if (close < 0) { out += html.slice(i); i = n; } else { out += html.slice(i, close); i = close; }
    }
  }
  return out;
}

// ---------------------------------------------------------------- helpers

function fixture(name) {
  if (!Object.prototype.hasOwnProperty.call(FIXTURES, name)) {
    throw new Error('unknown fixture: ' + name);
  }
  return FIXTURES[name];
}

// A fixture that declares itself a document (leading <!doctype> or <html>) is
// loaded in document mode; anything else is loaded as a fragment so parse5 does
// not synthesise an html/head/body shell the Go port would not have produced
// either. The Go runner applies the mirror-image rule: Page.HTML() for fragments,
// Page.NormalizedHTML() (the port's documented page.content() equivalent) for
// documents. See COVERAGE.md ("Normalisation").
function isDocumentSource(html) {
  return /^\s*(<!doctype|<html[\s>])/i.test(html);
}
function load(html) {
  return cheerio.load(html, null, isDocumentSource(html));
}

function nodeName(el) {
  if (!el) return '';
  switch (el.type) {
    case 'tag': case 'script': case 'style': return (el.name || '').toLowerCase();
    case 'text': return '#text';
    case 'comment': return '#comment';
    case 'directive': return '#doctype';
    case 'root': return '#document';
    default: return '#' + el.type;
  }
}

// The Go port is typed string-only: a missing attribute yields "" there, so the
// oracle collapses undefined/null to "" as well. Documented in COVERAGE.md.
function attrOr(el, name) {
  const v = el.attribs ? el.attribs[name] : undefined;
  return v === undefined || v === null ? '' : v;
}

function collapse(s) {
  return String(s).replace(/\s+/g, ' ').trim();
}

function outValue($, els, kind) {
  switch (kind || 'count') {
    case 'count': return els.length;
    case 'tags': return els.map(nodeName);
    case 'ids': return els.map((el) => attrOr(el, 'id'));
    case 'texts': return els.map((el) => $(el).text());
    case 'innerTexts': return els.map((el) => collapse($(el).text()));
    case 'outerAll': return els.map((el) => canon($.html(el)));
    case 'innerAll': return els.map((el) => canon($(el).html() || ''));
    case 'classes': return els.map((el) => attrOr(el, 'class').split(/\s+/).filter(Boolean));
    default: throw new Error('unknown out kind: ' + kind);
  }
}

// ---------------------------------------------------------------- fixture server
//
// Identical routes are served by the Go runner's httptest server. Only loopback
// is used and the port is chosen by the OS, so URLs are normalised to SRV before
// comparison.

let server = null;
let base = '';

function html(body) {
  return body;
}

function routes(req, res) {
  const u = new URL(req.url, 'http://127.0.0.1');
  const p = u.pathname;
  const send = (code, body, headers) => {
    res.writeHead(code, Object.assign({ 'content-type': 'text/html; charset=utf-8' }, headers || {}));
    res.end(body === null ? '' : body);
  };

  if (p.startsWith('/f/')) {
    return send(200, html(fixture(decodeURIComponent(p.slice(3)))));
  }
  if (p.startsWith('/status/')) {
    const code = parseInt(p.slice('/status/'.length), 10);
    if (code === 204 || code === 304) { res.writeHead(code); return res.end(); }
    return send(code, '<title>s' + code + '</title><p id="s">status ' + code + '</p>');
  }
  switch (p) {
    case '/redirect/absolute':
      return send(302, '', { location: base + '/final' });
    case '/redirect/relative':
      return send(302, '', { location: '../final' });
    case '/redirect/pathonly':
      return send(302, '', { location: '/final' });
    case '/redirect/chain/1':
      return send(302, '', { location: '/redirect/chain/2' });
    case '/redirect/chain/2':
      return send(302, '', { location: '/redirect/chain/3' });
    case '/redirect/chain/3':
      return send(302, '', { location: '/final' });
    case '/redirect/permanent':
      return send(301, '', { location: '/final' });
    case '/final':
      return send(200, '<title>final</title><p id="f">final page</p>');
    case '/deep/a/b/page':
      return send(200,
        '<title>page</title><link rel="stylesheet" href="/css/site.css">' +
        '<a href="sibling">s</a><a href="../other">o</a><a href="/final">r</a>' +
        '<a href="https://example.com/w">w</a><a href="sibling">dup</a>' +
        '<map><area href="/area-target"></map>');
    case '/deep/a/b/sibling':
      return send(200, '<title>sibling</title>');
    case '/deep/a/other':
      return send(200, '<title>other</title>');
    case '/cookie/set':
      return send(200, '<title>set</title>', { 'set-cookie': 'sid=abc123; Path=/' });
    case '/cookie/set-scoped':
      return send(200, '<title>set-scoped</title>', { 'set-cookie': 'scoped=zzz; Path=/scoped' });
    case '/cookie/set-two':
      res.writeHead(200, {
        'content-type': 'text/html; charset=utf-8',
        'set-cookie': ['one=1; Path=/', 'two=2; Path=/'],
      });
      return res.end('<title>set-two</title>');
    case '/cookie/echo':
    case '/scoped/echo':
      return send(200, '<p id="c">' + (req.headers.cookie || '') + '</p>');
    case '/echo': {
      let body = '';
      req.on('data', (d) => { body += d; });
      return req.on('end', () => {
        send(200,
          '<p id="m">' + req.method + '</p>' +
          '<p id="ct">' + (req.headers['content-type'] || '') + '</p>' +
          '<p id="q">' + u.search.replace(/^\?/, '') + '</p>' +
          '<p id="b">' + body.replace(/&/g, '&amp;').replace(/</g, '&lt;') + '</p>');
      });
    }
    default:
      return send(404, '<title>404</title><p id="s">not found</p>');
  }
}

function startServer() {
  return new Promise((resolve) => {
    server = http.createServer(routes);
    server.listen(0, '127.0.0.1', () => {
      base = 'http://127.0.0.1:' + server.address().port;
      resolve();
    });
  });
}

function norm(s) {
  return String(s).split(base).join('SRV');
}

// ---------------------------------------------------------------- navigation
//
// A minimal client mirroring what the Go port's Browser does: one cookie jar
// (tough-cookie, the RFC 6265 reference implementation) shared across
// navigations, redirects followed by undici, relative URLs resolved by WHATWG
// URL. State is reset per case so the run is order-independent.

let jar = new CookieJar();
let current = null; // last resolved absolute URL

function resetNav() {
  jar = new CookieJar();
  current = null;
}

async function nav(target, opts) {
  const o = opts || {};
  const abs = current ? new URL(target, current).toString() : new URL(target, base).toString();
  const headers = {};
  const cookie = await jar.getCookieString(abs);
  if (cookie) headers.cookie = cookie;
  const res = await fetch(abs, {
    method: o.method || 'GET',
    headers: Object.assign(headers, o.headers || {}),
    body: o.body,
    redirect: o.redirect || 'follow',
  });
  const setCookies = res.headers.getSetCookie ? res.headers.getSetCookie() : [];
  for (const sc of setCookies) await jar.setCookie(sc, res.url || abs);
  const text = await res.text();
  if (res.url) current = res.url;
  return { res, text, requested: abs };
}

function $$(text) {
  return load(text);
}

function title($) {
  const t = $('title').first();
  return t.length ? collapse(t.text()) : '';
}

// document.links semantics: <a href> and <area href>, in document order,
// resolved against the document URL, de-duplicated first-seen.
function links($, docURL) {
  const out = [];
  const seen = new Set();
  $('a[href], area[href]').each((_, el) => {
    let abs;
    try { abs = new URL(attrOr(el, 'href'), docURL).toString(); } catch { return; }
    if (!seen.has(abs)) { seen.add(abs); out.push(abs); }
  });
  return out;
}

// ---------------------------------------------------------------- form oracle
//
// cheerio's .serialize()/.serializeArray() implement the HTML form
// serialisation algorithm (document order, submit buttons excluded, disabled
// controls excluded, <select multiple> contributing one pair per selected
// option). The request shape follows the HTML spec: GET puts the pairs in the
// query string, POST uses the declared enctype.
function formOf($, selector) {
  const f = $(selector).first();
  if (!f.length) throw new Error('no form matches ' + selector);
  return f;
}

function formMethod(f) {
  const m = (f.attr('method') || 'GET').trim().toUpperCase();
  return m === 'POST' ? 'POST' : 'GET';
}

function formEnctype(f) {
  return (f.attr('enctype') || 'application/x-www-form-urlencoded').trim();
}

function sortedPairs(arr) {
  return arr.slice().sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : a.value < b.value ? -1 : a.value > b.value ? 1 : 0));
}

function formRequest($, f, docURL) {
  const method = formMethod(f);
  const action = f.attr('action');
  const actionURL = new URL(action === undefined || action === '' ? docURL : action, docURL);
  const pairs = f.serializeArray().map((x) => ({ name: x.name, value: x.value }));
  const enc = f.serialize();
  if (method === 'GET') {
    const u = new URL(actionURL.toString());
    const q = u.searchParams;
    for (const { name, value } of pairs) q.append(name, value);
    u.search = q.toString();
    return { method, path: u.pathname, query: u.searchParams.toString(), contentType: '', body: '' };
  }
  return {
    method,
    path: actionURL.pathname,
    query: actionURL.searchParams.toString(),
    contentType: formEnctype(f),
    body: enc,
  };
}

// ---------------------------------------------------------------- dispatch

async function handle(req) {
  const fn = req.fn;
  const args = req.args || [];
  switch (fn) {
    // ---- parsing
    case 'parse': {
      const $ = load(fixture(args[0]));
      return canon($.html());
    }
    case 'parseText': {
      const $ = load(fixture(args[0]));
      return $.root().text();
    }
    case 'tree': { // structural view: tag names of the children of a selector
      const $ = load(fixture(args[0]));
      const root = args[1] ? $(args[1]).first() : $.root();
      return root.contents().toArray().map(nodeName);
    }
    case 'title': {
      const $ = load(fixture(args[0]));
      return title($);
    }
    case 'text': {
      const $ = load(fixture(args[0]));
      const el = $(args[1]).first();
      if (!el.length) throw new Error('no element matches ' + args[1]);
      return el.text();
    }

    // ---- selectors (served over HTTP by the Go side; cheerio can query directly)
    case 'select': {
      const $ = load(fixture(args[0]));
      return outValue($, $(args[1]).toArray(), args[2]);
    }
    case 'matches': { // does the element at args[1] match selector args[2]?
      const $ = load(fixture(args[0]));
      const el = $(args[1]).first();
      if (!el.length) throw new Error('no element matches ' + args[1]);
      return el.is(args[2]);
    }
    case 'closest': {
      const $ = load(fixture(args[0]));
      const el = $(args[1]).first();
      if (!el.length) throw new Error('no element matches ' + args[1]);
      const c = el.closest(args[2]);
      return c.length ? attrOr(c[0], 'id') : null;
    }
    case 'compile': { // does the selector parse/run at all?
      const $ = load('<div id="d"><p>x</p></div>');
      $(args[0]).length;
      return true;
    }

    // ---- HTTP / navigation
    case 'http': return await httpScenario(args[0]);

    // ---- forms
    case 'form.serialize': {
      const { text, requested } = await nav('/f/' + args[0]);
      const $ = $$(text);
      return formOf($, args[1] || '#f').serialize();
    }
    case 'form.fields': {
      const { text } = await nav('/f/' + args[0]);
      const $ = $$(text);
      return sortedPairs(formOf($, args[1] || '#f').serializeArray().map((x) => ({ name: x.name, value: x.value })));
    }
    case 'form.request': {
      const { text, requested } = await nav('/f/' + args[0]);
      const $ = $$(text);
      return formRequest($, formOf($, args[1] || '#f'), requested);
    }
    case 'form.submit': {
      const { text, requested } = await nav('/f/' + args[0]);
      const $ = $$(text);
      const f = formOf($, args[1] || '#f');
      const rq = formRequest($, f, requested);
      const target = new URL(rq.path + (rq.query ? '?' + rq.query : ''), requested).toString();
      const r = rq.method === 'GET'
        ? await nav(target)
        : await nav(target, { method: 'POST', headers: { 'content-type': rq.contentType }, body: rq.body });
      const e = $$(r.text);
      return {
        status: r.res.status,
        method: e('#m').text(),
        contentType: e('#ct').text(),
        query: e('#q').text(),
        body: e('#b').text(),
      };
    }

    default:
      throw new Error('unknown fn: ' + fn);
  }
}

async function httpScenario(name) {
  resetNav();
  switch (name) {
    case 'status-200': case 'status-201': case 'status-404': case 'status-500': case 'status-204': {
      const code = name.slice('status-'.length);
      const { res, text } = await nav('/status/' + code);
      const $ = $$(text);
      return { status: res.status, ok: res.status >= 200 && res.status < 300, title: title($), path: new URL(res.url).pathname };
    }
    case 'redirect-absolute': case 'redirect-relative': case 'redirect-pathonly': case 'redirect-permanent': {
      const { res, text } = await nav('/' + name.replace('redirect-', 'redirect/'));
      const $ = $$(text);
      return { status: res.status, path: new URL(res.url).pathname, title: title($) };
    }
    case 'redirect-chain': {
      const { res, text } = await nav('/redirect/chain/1');
      const $ = $$(text);
      return { status: res.status, path: new URL(res.url).pathname, title: title($) };
    }
    case 'redirect-no-follow': {
      const { res } = await nav('/redirect/pathonly', { redirect: 'manual' });
      return { status: res.status, location: norm(res.headers.get('location') || '') };
    }
    case 'relative-goto-dotdot': {
      await nav('/deep/a/b/page');
      const { res, text } = await nav('../other');
      return { status: res.status, path: new URL(res.url).pathname, title: title($$(text)) };
    }
    case 'relative-goto-sibling': {
      await nav('/deep/a/b/page');
      const { res, text } = await nav('sibling');
      return { status: res.status, path: new URL(res.url).pathname, title: title($$(text)) };
    }
    case 'relative-goto-absolute-path': {
      await nav('/deep/a/b/page');
      const { res, text } = await nav('/final');
      return { status: res.status, path: new URL(res.url).pathname, title: title($$(text)) };
    }
    case 'links-resolved': {
      const { res, text } = await nav('/deep/a/b/page');
      return links($$(text), res.url).map(norm);
    }
    case 'cookie-roundtrip': {
      await nav('/cookie/set');
      const { text } = await nav('/cookie/echo');
      return $$(text)('#c').text();
    }
    case 'cookie-two': {
      await nav('/cookie/set-two');
      const { text } = await nav('/cookie/echo');
      return $$(text)('#c').text().split('; ').filter(Boolean).sort().join('; ');
    }
    case 'cookie-path-excluded': {
      await nav('/cookie/set-scoped');
      const { text } = await nav('/cookie/echo');
      return $$(text)('#c').text();
    }
    case 'cookie-path-included': {
      await nav('/cookie/set-scoped');
      const { text } = await nav('/scoped/echo');
      return $$(text)('#c').text();
    }
    case 'cookie-not-leaked-before-set': {
      const { text } = await nav('/cookie/echo');
      return $$(text)('#c').text();
    }
    case 'echo-get-query': {
      const { text } = await nav('/echo?a=1&b=2');
      const $ = $$(text);
      return { method: $('#m').text(), query: $('#q').text(), body: $('#b').text() };
    }
    case 'page-title': {
      const { text } = await nav('/f/full-document');
      return title($$(text));
    }
    case 'page-content': {
      const { text } = await nav('/f/full-document');
      return canon(text);
    }
    case 'page-html': {
      const { text } = await nav('/f/full-document');
      return canon($$(text).html());
    }
    default:
      throw new Error('unknown http scenario: ' + name);
  }
}

// ---------------------------------------------------------------- main loop

async function main() {
  await startServer();
  const rl = require('readline').createInterface({ input: process.stdin });
  for await (const line of rl) {
    if (line.trim() === '') continue;
    let req = null;
    let res;
    try {
      req = JSON.parse(line);
      let value = await handle(req);
      if (value === undefined) value = null;
      res = { id: req.id, ok: true, value };
    } catch (err) {
      res = { id: req && req.id ? req.id : null, ok: false, error: String((err && err.message) || err) };
    }
    process.stdout.write(JSON.stringify(res) + '\n');
  }
  server.close();
  process.exit(0);
}

main().catch((e) => { process.stderr.write('fatal: ' + e.stack + '\n'); process.exit(1); });
