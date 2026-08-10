'use strict';
// Upstream (oracle) runner for the handlebars parity harness.
// Reads one JSON object per line on stdin, writes one JSON object per line on
// stdout. Never exits on a failing case: a throw becomes {ok:false,error}.
// Logs go to stderr only.

const readline = require('readline');
const Handlebars = require('handlebars');

// ---------------------------------------------------------------------------
// Shared custom-helper registry. The Go runner defines the same set under the
// same names; cases name the helpers they want registered.
// ---------------------------------------------------------------------------
function str(v) {
  return v === null || v === undefined ? '' : String(v);
}

const HELPERS = {
  // inline, escaped result
  shout: function (v) {
    return str(v).toUpperCase();
  },
  // inline, SafeString result (must not be HTML-escaped)
  rawHtml: function (v) {
    return new Handlebars.SafeString('<i>' + str(v) + '</i>');
  },
  // inline, numeric result
  sum: function (a, b) {
    return a + b;
  },
  // inline, variadic + hash argument
  join: function () {
    const options = arguments[arguments.length - 1];
    const args = Array.prototype.slice.call(arguments, 0, arguments.length - 1);
    const sep = options.hash && options.hash.sep !== undefined ? String(options.hash.sep) : ',';
    return args.map(str).join(sep);
  },
  // block helper: wraps its body, body rendered against the current context
  bold: function (options) {
    return '<b>' + options.fn(this) + '</b>';
  },
  // block helper with an inverse section
  hasIt: function (v, options) {
    return v ? options.fn(this) : options.inverse(this);
  },
  // block helper yielding block params
  pair: function (a, b, options) {
    return options.fn(this, { blockParams: [a, b] });
  },
  // hook: unknown helper / unknown ambiguous name
  helperMissing: function () {
    const options = arguments[arguments.length - 1];
    return 'MISSING:' + options.name;
  },
  // hook: unknown block helper
  blockHelperMissing: function (context, options) {
    return 'BHM:' + options.name + ':' + str(context);
  },
};

const DECORATORS = {
  // decorator that rewrites the program's output; the closest thing both
  // implementations can express with their own decorator API.
  noop: function () {},
};

// ---------------------------------------------------------------------------
function makeEnv(spec) {
  const env = Handlebars.create();
  // keep {{log}} off stdout and deterministic
  env.log = function () {};
  env.logger.log = function () {};

  (spec.helpers || []).forEach(function (name) {
    if (!HELPERS[name]) throw new Error('unknown test helper: ' + name);
    env.registerHelper(name, HELPERS[name]);
  });
  (spec.decorators || []).forEach(function (name) {
    if (!DECORATORS[name]) throw new Error('unknown test decorator: ' + name);
    env.registerDecorator(name, DECORATORS[name]);
  });
  const partials = spec.partials || {};
  Object.keys(partials)
    .sort()
    .forEach(function (name) {
      env.registerPartial(name, partials[name]);
    });
  return env;
}

function compileOptions(spec) {
  const o = {};
  if (spec.noEscape) o.noEscape = true;
  if (spec.strict) o.strict = true;
  if (spec.compat) o.compat = true;
  if (spec.noData) o.data = false;
  if (spec.knownHelpersOnly) o.knownHelpersOnly = true;
  if (spec.knownHelpers) {
    o.knownHelpers = {};
    spec.knownHelpers.forEach(function (n) {
      o.knownHelpers[n] = true;
    });
  }
  return o;
}

function sortObject(v) {
  if (Array.isArray(v)) return v.map(sortObject);
  if (v && typeof v === 'object') {
    const out = {};
    Object.keys(v)
      .sort()
      .forEach(function (k) {
        out[k] = sortObject(v[k]);
      });
    return out;
  }
  return v;
}

function dispatch(req) {
  const fn = req.fn;
  const args = req.args || [];
  switch (fn) {
    case 'render': {
      const template = args[0];
      const context = args.length > 1 ? args[1] : undefined;
      const spec = args.length > 2 && args[2] ? args[2] : {};
      const env = makeEnv(spec);
      const tpl = env.compile(template, compileOptions(spec));
      return tpl(context);
    }
    case 'compile': {
      // parse/compile only: value is true when the template is accepted
      // compile() is lazy in handlebars.js, so precompile() is what actually
      // forces the parse — matching Go's eager handlebars.Compile.
      const env = makeEnv(args[1] && args[1] !== null ? args[1] : {});
      env.precompile(args[0], compileOptions(args[1] || {}));
      return true;
    }
    case 'escapeExpression':
      return Handlebars.escapeExpression(args[0]);
    case 'isEmpty':
      return Handlebars.Utils.isEmpty(args[0]);
    case 'isArray':
      return Handlebars.Utils.isArray(args[0]);
    case 'extend':
      return sortObject(
        Handlebars.Utils.extend.apply(null, [Object.assign({}, args[0])].concat(args.slice(1)))
      );
    case 'createFrame': {
      const f = Handlebars.createFrame(args[0]);
      // _parent is the source object; compare it structurally
      return sortObject(JSON.parse(JSON.stringify(f)));
    }
    case 'safeString':
      return String(new Handlebars.SafeString(args[0]));
    default:
      throw new Error('unknown fn: ' + fn);
  }
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', function (line) {
  line = line.trim();
  if (!line) return;
  let req;
  try {
    req = JSON.parse(line);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: '', ok: false, error: 'bad request: ' + e.message }) + '\n');
    return;
  }
  let out;
  try {
    const v = dispatch(req);
    out = { id: req.id, ok: true, value: v === undefined ? null : v };
  } catch (e) {
    out = { id: req.id, ok: false, error: (e && e.message) || String(e) };
  }
  process.stdout.write(JSON.stringify(out) + '\n');
});
