'use strict';
// Upstream oracle runner for the express/sanitizehtml parity harness.
//
// Reads one JSON object per line from stdin ({id, fn, args}) and writes exactly
// one JSON object per line to stdout ({id, ok, value} or {id, ok:false, error}),
// in the same order. A throwing case is reported as ok:false; the process never
// exits early. Logs go to stderr only — sanitize-html console.warn()s when the
// policy allows <script> or <style>, and that must not reach stdout.

const sanitizeHtml = require('sanitize-html');

// sanitize-html warns on stderr via console.warn already, but make sure nothing
// can slip onto stdout from inside the library.
console.log = (...a) => console.error(...a);

// Named text filters, so that a case can exercise the textFilter option through
// JSON: a function cannot be sent over the wire, so both runners implement the
// same small set by name.
const TEXT_FILTERS = {
  upper: (text) => text.toUpperCase(),
  brackets: (text, tagName) => `[${tagName || 'root'}:${text}]`,
  drop: () => '',
  markup: () => '<script>alert(1)</script>',
};

function buildOptions(raw) {
  if (raw === undefined || raw === null) return undefined;
  const options = Object.assign({}, raw);
  if (options.textFilterKind !== undefined) {
    const f = TEXT_FILTERS[options.textFilterKind];
    if (!f) throw new Error(`unknown textFilterKind: ${options.textFilterKind}`);
    delete options.textFilterKind;
    options.textFilter = f;
  }
  return options;
}

const FNS = {
  // sanitizeHtml(html, options)
  sanitize: (html, options) => sanitizeHtml(html, buildOptions(options)),

  // Introspection of sanitizeHtml.defaults, so that the default POLICY itself is
  // compared and not just its effect on a handful of inputs. Lists are sorted:
  // the two implementations need not agree on declaration order.
  defaultAllowedTags: () => [...sanitizeHtml.defaults.allowedTags].sort(),
  defaultAllowedAttributes: () => {
    const out = {};
    for (const k of Object.keys(sanitizeHtml.defaults.allowedAttributes).sort()) {
      out[k] = [...sanitizeHtml.defaults.allowedAttributes[k]].sort();
    }
    return out;
  },
  defaultSelfClosing: () => [...sanitizeHtml.defaults.selfClosing].sort(),
  defaultAllowedSchemes: () => [...sanitizeHtml.defaults.allowedSchemes].sort(),
  defaultAllowedSchemesAppliedToAttributes: () =>
    [...sanitizeHtml.defaults.allowedSchemesAppliedToAttributes].sort(),
  defaultDisallowedTagsMode: () => sanitizeHtml.defaults.disallowedTagsMode,
  defaultAllowProtocolRelative: () => sanitizeHtml.defaults.allowProtocolRelative,
  // nonTextTags has no entry in defaults; the default lives inside the function.
  // It is read back out of the library source so the value stays honest.
  defaultNonTextTags: () => {
    const src = require('fs').readFileSync(require.resolve('sanitize-html'), 'utf8');
    const m = src.match(/options\.nonTextTags \|\| \[([^\]]*)\]/);
    if (!m) throw new Error('could not locate the nonTextTags default in the library source');
    return m[1]
      .split(',')
      .map((s) => s.trim().replace(/^['"]|['"]$/g, ''))
      .filter(Boolean)
      .sort();
  },
};

function handle(req) {
  const fn = FNS[req.fn];
  if (!fn) throw new Error(`unknown fn: ${req.fn}`);
  return fn(...(req.args || []));
}

let buf = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf('\n')) !== -1) {
    const line = buf.slice(0, nl);
    buf = buf.slice(nl + 1);
    if (!line.trim()) continue;
    let req;
    try {
      req = JSON.parse(line);
    } catch (err) {
      process.stdout.write(JSON.stringify({ id: null, ok: false, error: `bad request: ${err.message}` }) + '\n');
      continue;
    }
    let out;
    try {
      out = { id: req.id, ok: true, value: handle(req) };
    } catch (err) {
      out = { id: req.id, ok: false, error: String((err && err.message) || err) };
    }
    // One line per case, flushed immediately: the harness reads synchronously.
    process.stdout.write(JSON.stringify(out) + '\n');
  }
});
