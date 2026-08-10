'use strict';
// Upstream (Node) parity runner for cheerio@1.0.0.
//
// Protocol: one JSON object per line on stdin, exactly one JSON object per line
// on stdout, same order. Never exits on a failing case: every throw is reported
// as {"ok":false,"error":"..."}.
//
// Usage: node run.js [path/to/fixtures.json]

const fs = require('fs');
const path = require('path');
const cheerio = require('cheerio');

const fixturesPath = process.argv[2] || path.join(__dirname, '..', 'fixtures', 'fixtures.json');
const FIXTURES = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'));

// ---------------------------------------------------------------- canonical HTML
//
// Attribute order and tag spelling are normalised identically by both runners so
// that a pure serialisation-order difference is not reported as a divergence.
// See COVERAGE.md ("Normalisation") for the exact rules.

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

// cheerio returns rich JS values for .data()/.prop(); the Go port is typed
// string-only. Stringify so the comparison is about the value, not the type.
function scalar(v) {
  if (v === undefined || v === null) return null;
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}
function scalarMap(o) {
  const out = {};
  if (!o) return out;
  for (const k of Object.keys(o).sort()) out[k] = scalar(o[k]);
  return out;
}

function outValue($, sel, kind) {
  switch (kind || 'outerAll') {
    case 'docHtml': return canon($.html());
    case 'docText': return $.root().text();
    case 'outerAll': return sel.toArray().map((el) => canon($.html(el)));
    case 'outerFirst': return sel.length ? canon($.html(sel[0])) : null;
    case 'innerAll': return sel.toArray().map((el) => canon($(el).html()));
    case 'tags': return sel.toArray().map(nodeName);
    case 'texts': return sel.toArray().map((el) => $(el).text());
    case 'text': return sel.text();
    case 'count': return sel.length;
    case 'ids': return sel.toArray().map((el) => scalar($(el).attr('id')));
    default: throw new Error('unknown out kind: ' + kind);
  }
}

// ---------------------------------------------------------------- step dispatch

function applyStep($, state, step) {
  const m = step[0];
  const a = step.slice(1);
  let sel = state.sel;
  const wasRoot = state.isRoot;
  state.isRoot = false;

  switch (m) {
    // ---- traversal
    case 'find': return { sel: wasRoot ? $(a[0]) : sel.find(a[0]) };
    case 'children': return { sel: a.length ? sel.children(a[0]) : sel.children() };
    case 'contents': return { sel: sel.contents() };
    case 'parent': return { sel: a.length ? sel.parent(a[0]) : sel.parent() };
    case 'parents': return { sel: a.length ? sel.parents(a[0]) : sel.parents() };
    case 'parentsUntil': return { sel: sel.parentsUntil(a[0], a[1]) };
    case 'closest': return { sel: sel.closest(a[0]) };
    case 'siblings': return { sel: a.length ? sel.siblings(a[0]) : sel.siblings() };
    case 'next': return { sel: a.length ? sel.next(a[0]) : sel.next() };
    case 'prev': return { sel: a.length ? sel.prev(a[0]) : sel.prev() };
    case 'nextAll': return { sel: a.length ? sel.nextAll(a[0]) : sel.nextAll() };
    case 'prevAll': return { sel: a.length ? sel.prevAll(a[0]) : sel.prevAll() };
    case 'nextUntil': return { sel: sel.nextUntil(a[0], a[1]) };
    case 'prevUntil': return { sel: sel.prevUntil(a[0], a[1]) };
    case 'filter': return { sel: sel.filter(a[0]) };
    case 'not': return { sel: sel.not(a[0]) };
    case 'has': return { sel: sel.has(a[0]) };
    case 'add': return { sel: sel.add(a[0]) };
    case 'addSelection': return { sel: sel.add($(a[0])) };
    case 'addBack': return { sel: a.length ? sel.addBack(a[0]) : sel.addBack() };
    case 'end': return { sel: sel.end() };
    case 'eq': return { sel: sel.eq(a[0]) };
    case 'first': return { sel: sel.first() };
    case 'last': return { sel: sel.last() };
    case 'slice': return { sel: a.length > 1 ? sel.slice(a[0], a[1]) : sel.slice(a[0]) };
    case 'clone': return { sel: sel.clone() };

    // ---- accessors (terminal)
    case 'attr':
      if (a.length === 0) return { value: scalarMap(sel.attr()) };
      if (a.length === 1) return { value: scalar(sel.attr(a[0])) };
      return { sel: sel.attr(a[0], a[1]) };
    case 'removeAttr': return { sel: sel.removeAttr(a[0]) };
    case 'prop':
      if (a.length === 1) return { value: scalar(sel.prop(a[0])) };
      return { sel: sel.prop(a[0], a[1]) };
    case 'data':
      if (a.length === 0) return { value: scalarMap(sel.data()) };
      if (a.length === 1) return { value: scalar(sel.data(a[0])) };
      return { sel: sel.data(a[0], a[1]) };
    case 'removeData':
      if (typeof sel.removeData !== 'function') throw new Error('removeData is not part of Cheerio.prototype in cheerio@1.0.0');
      return { sel: sel.removeData(...a) };
    case 'val':
      if (a.length === 0) return { value: scalar(sel.val()) };
      return { sel: sel.val(a[0]) };
    case 'text':
      if (a.length === 0) return { value: sel.text() };
      return { sel: sel.text(a[0]) };
    case 'html':
      if (a.length === 0) return { value: sel.length ? canon(sel.html()) : null };
      return { sel: sel.html(a[0]) };
    case 'toString': return { value: canon(sel.toString()) };
    case 'outerHtml': return { value: sel.length ? canon($.html(sel[0])) : '' };
    case 'hasClass': return { value: sel.hasClass(a[0]) };
    case 'css':
      if (a.length === 1) { const v = sel.css(a[0]); return { value: v === undefined ? null : v }; }
      return { sel: sel.css(a[0], a[1]) };
    case 'tagName': return { value: sel.length ? nodeName(sel[0]) : '' };
    case 'length': return { value: sel.length };
    case 'index': return { value: a.length ? sel.index(a[0]) : sel.index() };
    case 'is': return { value: sel.is(a[0]) };
    case 'serialize': return { value: sel.serialize() };
    case 'serializeArray': return { value: sel.serializeArray().map((f) => ({ name: f.name, value: f.value })) };

    // ---- class / manipulation
    case 'addClass': return { sel: sel.addClass(a[0]) };
    case 'removeClass': return { sel: a.length ? sel.removeClass(a[0]) : sel.removeClass() };
    case 'toggleClass': return { sel: sel.toggleClass(a[0]) };
    case 'append': return { sel: sel.append(...a) };
    case 'prepend': return { sel: sel.prepend(...a) };
    case 'before': return { sel: sel.before(...a) };
    case 'after': return { sel: sel.after(...a) };
    case 'appendTo': return { sel: sel.appendTo(a[0]) };
    case 'prependTo': return { sel: sel.prependTo(a[0]) };
    case 'insertBefore': return { sel: sel.insertBefore(a[0]) };
    case 'insertAfter': return { sel: sel.insertAfter(a[0]) };
    case 'replaceWith': return { sel: sel.replaceWith(a[0]) };
    case 'remove': return { sel: a.length ? sel.remove(a[0]) : sel.remove() };
    case 'empty': return { sel: sel.empty() };
    case 'wrap': return { sel: sel.wrap(a[0]) };
    case 'wrapAll': return { sel: sel.wrapAll(a[0]) };
    case 'wrapInner': return { sel: sel.wrapInner(a[0]) };
    case 'unwrap': return { sel: a.length ? sel.unwrap(a[0]) : sel.unwrap() };

    default: throw new Error('unknown step: ' + m);
  }
}

function fixture(name) {
  if (!Object.prototype.hasOwnProperty.call(FIXTURES, name)) {
    throw new Error('unknown fixture: ' + name);
  }
  return FIXTURES[name];
}

// isDocument=false keeps parse5 in fragment mode, so no html/head/body wrapper
// is synthesised — that matches the Go port's fragment-oriented Load.
function load(html) {
  return cheerio.load(html, null, false);
}

function selectorNodes($, selector) {
  return $(selector).toArray();
}

function handle(req) {
  const fn = req.fn;
  const args = req.args || [];
  switch (fn) {
    case 'chain': {
      const [fixName, steps, out] = args;
      const $ = load(fixture(fixName));
      const state = { sel: $.root(), isRoot: true };
      let value;
      let terminated = false;
      for (const step of steps || []) {
        if (terminated) throw new Error('step after terminal accessor');
        const r = applyStep($, state, step);
        if ('value' in r) { value = r.value; terminated = true; } else { state.sel = r.sel; }
      }
      if (terminated) return value;
      return outValue($, state.sel, out);
    }
    case 'parse': { // canonical serialisation of a whole fixture
      const $ = load(fixture(args[0]));
      return canon($.html());
    }
    case 'parseText': {
      const $ = load(fixture(args[0]));
      return $.root().text();
    }
    case 'static.text': {
      const $ = load(fixture(args[0]));
      return $.text(selectorNodes($, args[1]));
    }
    case 'static.html': {
      const $ = load(fixture(args[0]));
      return canon($.html(selectorNodes($, args[1])));
    }
    case 'static.contains': {
      const $ = load(fixture(args[0]));
      const a = $(args[1])[0];
      const b = $(args[2])[0];
      return cheerio.contains(a, b);
    }
    case 'static.merge': {
      const $ = load(fixture(args[0]));
      const merged = cheerio.merge($(args[1]).toArray(), $(args[2]).toArray());
      return { length: merged.length, tags: Array.from(merged).map(nodeName) };
    }
    case 'static.parseHTML': {
      const $ = load('');
      const nodes = $.parseHTML(args[0]);
      if (nodes === null || nodes === undefined) return null; // upstream returns null for empty input
      return { length: nodes.length, html: canon($.html(nodes)), tags: nodes.map(nodeName) };
    }
    case 'compile': { // does the selector parse at all?
      const $ = load('<div></div>');
      $(args[0]).length;
      return true;
    }
    default:
      throw new Error('unknown fn: ' + fn);
  }
}

// ---------------------------------------------------------------- main loop

let buf = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, nl);
    buf = buf.slice(nl + 1);
    if (line.trim() === '') continue;
    let req = null;
    let res;
    try {
      req = JSON.parse(line);
      res = { id: req.id, ok: true, value: handle(req) };
      if (res.value === undefined) res.value = null;
    } catch (err) {
      res = { id: req && req.id ? req.id : null, ok: false, error: String((err && err.message) || err) };
    }
    process.stdout.write(JSON.stringify(res) + '\n');
  }
});
process.stdin.on('end', () => process.exit(0));
