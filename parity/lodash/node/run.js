'use strict';
// Upstream oracle runner for the lodash parity harness.
//
// Protocol: one JSON object per line on stdin -> exactly one JSON object per
// line on stdout, same order.
//   in : {"id":"...","fn":"chunk","args":[[1,2,3],2]}
//   out: {"id":"...","ok":true,"value":[[1,2],[3]]}
//   out: {"id":"...","ok":false,"error":"..."}
// Never exits on a failing case. Logs go to stderr only.

const _ = require('lodash');
const readline = require('readline');

// ---------------------------------------------------------------------------
// named function registry -- args may carry {"$fn":"name"} because JSON cannot
// carry callables. Every entry is a *factory* so each case gets a fresh
// closure (some are deliberately stateful, e.g. `counter`).
// ---------------------------------------------------------------------------
const num = (v) => (typeof v === 'number' ? v : Number(v));
const FNS = {
  // no-arg
  counter: () => { let c = 0; return () => ++c; },
  const42: () => () => 42,
  // predicates
  isEven: () => (v) => num(v) % 2 === 0,
  isOdd: () => (v) => Math.abs(num(v) % 2) === 1,
  gt2: () => (v) => num(v) > 2,
  lt3: () => (v) => num(v) < 3,
  truthy: () => (v) => !!v,
  isNeg: () => (v) => num(v) < 0,
  eqA: () => (v) => v === 'a',
  hasFlag: () => (v) => !!(v && v.flag),
  isNumType: () => (v) => typeof v === 'number',
  alwaysTrue: () => () => true,
  alwaysFalse: () => () => false,
  isEmptyStr: () => (v) => v === '',
  // unary any -> any
  identity: () => (v) => v,
  double: () => (v) => num(v) * 2,
  square: () => (v) => num(v) * num(v),
  mod3: () => (v) => num(v) % 3,
  absv: () => (v) => Math.abs(num(v)),
  floorv: () => (v) => Math.floor(num(v)),
  lenOf: () => (v) => (v == null ? 0 : (v.length === undefined ? 0 : v.length)),
  numOf: () => (v) => num(v),
  toStr: () => (v) => String(v),
  upper: () => (v) => String(v).toUpperCase(),
  firstChar: () => (v) => String(v).slice(0, 1),
  propN: () => (v) => (v == null ? undefined : v.n),
  propAge: () => (v) => (v == null ? undefined : v.age),
  propName: () => (v) => (v == null ? undefined : v.name),
  wrapArr: () => (v) => [v],
  dupArr: () => (v) => [v, v],
  // binary (also used as reducers: (acc, cur))
  add2: () => (a, b) => num(a) + num(b),
  sub2: () => (a, b) => num(a) - num(b),
  mulPair: () => (a, b) => num(a) * num(b),
  concat2: () => (a, b) => String(a) + String(b),
  // comparators (a, b) -> bool
  eqStrict: () => (a, b) => a === b,
  eqNum: () => (a, b) => num(a) === num(b),
  eqMod3: () => (a, b) => num(a) % 3 === num(b) % 3,
  // (value, index)
  mulIdx: () => (v, i) => num(v) * num(i),
  pairIdx: () => (v, i) => [v, i],
};

// a few keys are legitimately usable as numeric/string key functions; the Go
// runner splits them into typed registries, node needs no such distinction.

function resolve(a) {
  if (Array.isArray(a)) return a.map(resolve);
  if (a !== null && typeof a === 'object') {
    if (Object.prototype.hasOwnProperty.call(a, '$fn')) {
      const f = FNS[a.$fn];
      if (!f) throw new Error('unknown $fn: ' + a.$fn);
      return f();
    }
    if (Object.prototype.hasOwnProperty.call(a, '$undefined')) return undefined;
    if (Object.prototype.hasOwnProperty.call(a, '$nan')) return NaN;
    if (Object.prototype.hasOwnProperty.call(a, '$inf')) return a.$inf > 0 ? Infinity : -Infinity;
    if (Object.prototype.hasOwnProperty.call(a, '$regexp')) return new RegExp(a.$regexp);
    if (Object.prototype.hasOwnProperty.call(a, '$date')) return new Date(a.$date);
    if (Object.prototype.hasOwnProperty.call(a, '$sparse')) {
      // sparse/holey array: {"$sparse": n, "set": {"0": v, ...}}
      const arr = new Array(a.$sparse);
      for (const k of Object.keys(a.set || {})) arr[Number(k)] = resolve(a.set[k]);
      return arr;
    }
    const o = {};
    for (const k of Object.keys(a)) o[k] = resolve(a[k]);
    return o;
  }
  return a;
}

// ---------------------------------------------------------------------------
// normalisation: JSON-comparable, deterministic, object keys sorted.
// ---------------------------------------------------------------------------
function norm(v) {
  if (v === undefined) return { $undefined: true };
  if (v === null) return null;
  const t = typeof v;
  if (t === 'number') {
    if (Number.isNaN(v)) return { $nan: true };
    if (!Number.isFinite(v)) return { $inf: v > 0 ? 1 : -1 };
    return v;
  }
  if (t === 'function') return { $function: true };
  if (t === 'symbol') return { $symbol: String(v) };
  if (t === 'string' || t === 'boolean') return v;
  if (Array.isArray(v)) {
    const out = new Array(v.length);
    for (let i = 0; i < v.length; i++) out[i] = norm(v[i]);
    return out;
  }
  if (v instanceof Date) return { $date: v.toISOString() };
  if (v instanceof RegExp) return { $regexp: v.source };
  if (v instanceof Error) return { $errorValue: true };
  if (v instanceof Map) return { $map: norm([...v.entries()]) };
  if (v instanceof Set) return { $set: norm([...v.values()]) };
  const out = {};
  for (const k of Object.keys(v).sort()) out[k] = norm(v[k]);
  return out;
}

function sortNormed(arr) {
  return arr
    .map((x) => [JSON.stringify(x), x])
    .sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0))
    .map((p) => p[1]);
}

// Results whose order is not meaningful on the Go side (Go map iteration is
// randomised) are sorted on BOTH sides before comparison.
const SORT_ALWAYS = new Set([
  'keys', 'values', 'toPairs', 'entries', 'sortedKeys',
  'mapToSlice', 'filterMap', 'rejectMap', 'forOwn-collect',
]);
// Sorted only when the collection argument was a plain object.
const SORT_IF_OBJECT = new Set([]);

function isPlainObj(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

// ---------------------------------------------------------------------------
// composite ops -- lodash functions that *return* functions cannot be compared
// directly, so the case supplies the calls to make. Convention: the final
// argument is a list of argument-lists; the op returns the array of results.
// ---------------------------------------------------------------------------
const applyEach = (f, calls) => (calls || []).map((c) => f.apply(null, c));

const OPS = {
  // ---- array ----
  flatten: (a) => _.flatten(a),
  fill: (a, v, s, e) => _.fill(_.clone(a), v, s, e),
  pull: (a, ...vs) => _.pull(_.clone(a), ...vs),
  pullAll: (a, vs) => _.pullAll(_.clone(a), vs),
  pullAllBy: (a, vs, it) => _.pullAllBy(_.clone(a), vs, it),
  pullAllWith: (a, vs, cmp) => _.pullAllWith(_.clone(a), vs, cmp),
  pullAt: (a, ...idx) => { const c = _.clone(a); _.pullAt(c, ...idx); return c; },
  remove: (a, p) => { const c = _.clone(a); _.remove(c, p); return c; },
  reverse: (a) => _.reverse(_.clone(a)),
  zip: (a, b) => _.zip(a, b),
  unzip: (a) => _.unzip(a),

  // ---- object ----
  values: (o) => (isPlainObj(o) ? Object.keys(o).sort().map((k) => o[k]) : _.values(o)),
  set: (o, p, v) => _.set(_.cloneDeep(o), p, v),
  setWith: (o, p, v) => _.setWith(_.cloneDeep(o), p, v, Object),
  unset: (o, p) => { const c = _.cloneDeep(o); const ok = _.unset(c, p); return { ok, object: c }; },
  update: (o, p, u) => _.update(_.cloneDeep(o), p, u),
  updateWith: (o, p, u) => _.updateWith(_.cloneDeep(o), p, u, Object),
  parseFloat: (str) => Number.parseFloat(str),
  assign: (o, ...src) => _.assign(_.cloneDeep(o), ...src),
  assignWith: (o, src, cust) => _.assignWith(_.cloneDeep(o), src, cust),
  defaults: (o, ...src) => _.defaults(_.cloneDeep(o), ...src),
  defaultsDeep: (o, ...src) => _.defaultsDeep(_.cloneDeep(o), ...src),
  merge: (o, ...src) => _.merge(_.cloneDeep(o), ...src),
  mergeWith: (o, src, cust) => _.mergeWith(_.cloneDeep(o), src, cust),
  transform: (o, f, init) => _.transform(o, (acc, v) => f(acc, v), init),

  // ---- map/object collection variants (Go has *Map twins) ----
  mapToSlice: (o, f) => _.map(o, f),
  filterMap: (o, p) => _.filter(o, p),
  rejectMap: (o, p) => _.reject(o, p),
  everyMap: (o, p) => _.every(o, p),
  someMap: (o, p) => _.some(o, p),
  noneMap: (o, p) => !_.some(o, p),
  findMap: (o, p) => _.find(o, p),
  partitionMap: (o, p) => _.partition(o, p).map((h) => sortNormed(norm(h))),
  groupByMap: (o, f) => _.mapValues(_.groupBy(o, f), (h) => sortNormed(norm(h))),
  countByMap: (o, f) => _.countBy(o, f),
  maxByMap: (o, f) => _.maxBy(_.values(o), f),
  minByMap: (o, f) => _.minBy(_.values(o), f),
  reduceMap: (o, f, init) => _.reduce(_.keys(o).sort().map((k) => o[k]), (acc, v) => f(acc, v), init),
  includesValue: (o, v) => _.includes(o, v),
  invertByKeys: (o, f) => _.mapValues(_.invertBy(o, f), (h) => h.slice().sort()),
  sortedKeys: (o) => _.keys(o),
  'forOwn-collect': (o, f) => { const out = []; _.forOwn(o, (v, k) => out.push([k, f(v)])); return out; },

  // ---- string ----
  truncate: (s, len, om) => _.truncate(s, { length: len, omission: om }),
  template: (tmpl, data) => _.template(tmpl)(data),
  parseInt: (s, radix) => _.parseInt(s, radix),
  split: (s, sep, limit) => (limit === undefined ? _.split(s, sep) : _.split(s, sep, limit)),

  // ---- collection ----
  'forEach-collect': (c, f) => { const out = []; _.forEach(c, (v) => out.push(f(v))); return out; },
  'forEachRight-collect': (c, f) => { const out = []; _.forEachRight(c, (v) => out.push(f(v))); return out; },
  'mapI': (a, f) => _.map(a, (v, i) => f(v, i)),
  'forEachI-collect': (a, f) => { const out = []; _.forEach(a, (v, i) => out.push(f(v, i))); return out; },
  'orderBy': (a, keys, orders) => _.orderBy(a, keys, orders),
  'partition': (c, p) => _.partition(c, p),

  // ---- function / combinator (composite) ----
  after: (n, calls) => { const f = FNS.counter(); const g = _.after(n, f); return applyEach(g, calls); },
  before: (n, calls) => { const f = FNS.counter(); const g = _.before(n, f); return applyEach(g, calls); },
  once: (calls) => { const f = FNS.counter(); const g = _.once(f); return applyEach(g, calls); },
  memoize: (calls) => { const f = FNS.counter(); const g = _.memoize((k) => f() + ':' + String(k)); return applyEach(g, calls); },
  negate: (p, calls) => applyEach(_.negate(p), calls),
  unary: (f, calls) => applyEach(_.unary(f), calls),
  ary: (f, n, calls) => applyEach(_.ary(f, n), calls),
  flip: (f, calls) => applyEach(_.flip(f), calls),
  partial: (f, a, calls) => applyEach(_.partial(f, a), calls),
  partialRight: (f, a, calls) => applyEach(_.partialRight(f, a), calls),
  rearg: (f, idx, calls) => applyEach(_.rearg(f, idx), calls),
  spread: (f, calls) => applyEach(_.spread(f), calls),
  rest: (f, calls) => applyEach(_.rest(f), calls),
  overArgs: (f, ts, calls) => applyEach(_.overArgs(f, ts), calls),
  wrap: (v, w, calls) => applyEach(_.wrap(v, w), calls),
  curry2: (f, a, b) => _.curry(f)(a)(b),
  curryRight2: (f, a, b) => _.curryRight(f)(a)(b),
  flow: (fs, calls) => applyEach(_.flow(fs), calls),
  flowRight: (fs, calls) => applyEach(_.flowRight(fs), calls),
  over: (fs, calls) => applyEach(_.over(fs), calls),
  overEvery: (ps, calls) => applyEach(_.overEvery(ps), calls),
  overSome: (ps, calls) => applyEach(_.overSome(ps), calls),
  cond: (pairs, calls) => applyEach(_.cond(pairs), calls),
  constant: (v, calls) => applyEach(_.constant(v), calls),
  identityFn: (calls) => applyEach(_.identity, calls),
  nthArg: (n, calls) => applyEach(_.nthArg(n), calls),
  property: (path, calls) => applyEach(_.property(path), calls),
  propertyOf: (obj, calls) => applyEach(_.propertyOf(obj), calls),
  matches: (src, calls) => applyEach(_.matches(src), calls),
  matchesProperty: (p, v, calls) => applyEach(_.matchesProperty(p, v), calls),
  conforms: (spec, calls) => applyEach(_.conforms(spec), calls),
  times: (n, f) => _.times(n, f),
  noop: () => _.noop(),
  stubArray: () => _.stubArray(),
  stubObject: () => _.stubObject(),
  stubString: () => _.stubString(),
  stubTrue: () => _.stubTrue(),
  stubFalse: () => _.stubFalse(),
  attempt: (mode) => {
    const r = _.attempt(() => { if (mode === 'throw') throw new Error('boom'); return 7; });
    return _.isError(r) ? { $threw: true } : r;
  },
  uniqueId: (prefix) => {
    // deterministic: report only the shape, since the counter is global
    const a = _.uniqueId(prefix); const b = _.uniqueId(prefix);
    const strip = (s) => s.replace(/\d+$/, '#');
    return [strip(a), strip(b), Number(b.replace(/^\D*/, '')) - Number(a.replace(/^\D*/, ''))];
  },

  // ---- non-deterministic upstream: compare an invariant, not the draw ----
  sample: (a) => { const v = _.sample(a); return a.length === 0 ? false : _.includes(a, v); },
  sampleSize: (a, n) => { const got = _.sampleSize(a, n); return [got.length, got.every((v) => _.includes(a, v))]; },
  shuffle: (a) => sortNormed(norm(_.shuffle(a))),
  random: (lo_, hi) => { const v = _.random(lo_, hi); return v >= lo_ && v <= hi; },

  // ---- Go-only helpers, compared against the closest lodash spelling ----
  pascalCase: (s2) => _.upperFirst(_.camelCase(s2)),
  none: (a, p) => !_.some(a, p),
  getOr: (o, p, d) => _.get(o, p, d),
  fromEntries: (e) => _.fromPairs(e),

  // ---- seq / chain pipeline ----
  seq: (input, steps) => runSeq(input, steps),
  seqSet: (input, steps) => runSeq(input, steps),
  chainValue: (v) => _.chain(v).value(),
  thru: (v, f) => _.chain(v).thru(f).value(),
  tap: (a, f) => { const seen = []; const out = _.chain(a).tap((x) => { seen.push(_.clone(x)); f(x); }).value(); return { seen: norm(seen), value: norm(out) }; },
};

// aliases: lodash spells several of these more than one way
OPS.each = OPS['forEach-collect'];
OPS.eachRight = OPS['forEachRight-collect'];
OPS.forOwn = OPS['forOwn-collect'];
OPS.forIn = OPS['forOwn-collect'];
OPS.extend = OPS.assign;
OPS.assignIn = OPS.assign;
OPS.extendWith = OPS.assignWith;
OPS.assignInWith = OPS.assignWith;
OPS.keysIn = OPS.sortedKeys;
OPS.valuesIn = OPS.values;
OPS.toPairsIn = (o) => _.toPairs(o);
OPS.entriesIn = (o) => _.entries(o);
OPS.forOwnRight = OPS['forOwn-collect'];
OPS.forInRight = OPS['forOwn-collect'];
OPS.findLastKey = (o, p2) => _.findLastKey(o, p2);
OPS.isMatchWith = (o, src, cust) => _.isMatchWith(o, src, cust);
for (const k of ['keysIn', 'valuesIn', 'toPairsIn', 'entriesIn', 'forOwnRight', 'forInRight']) SORT_ALWAYS.add(k);
SORT_ALWAYS.add('forOwn');
SORT_ALWAYS.add('forIn');

function runSeq(input, steps) {
  let cur = input;
  for (const step of steps) {
    const [name, ...rest] = step;
    switch (name) {
      case 'filter': cur = _.filter(cur, rest[0]); break;
      case 'reject': cur = _.reject(cur, rest[0]); break;
      case 'take': cur = _.take(cur, rest[0]); break;
      case 'takeRight': cur = _.takeRight(cur, rest[0]); break;
      case 'drop': cur = _.drop(cur, rest[0]); break;
      case 'dropRight': cur = _.dropRight(cur, rest[0]); break;
      case 'takeWhile': cur = _.takeWhile(cur, rest[0]); break;
      case 'dropWhile': cur = _.dropWhile(cur, rest[0]); break;
      case 'reverse': cur = _.reverse(_.clone(cur)); break;
      case 'tail': cur = _.tail(cur); break;
      case 'initial': cur = _.initial(cur); break;
      case 'slice': cur = _.slice(cur, rest[0], rest[1]); break;
      case 'concat': cur = _.concat(cur, rest[0]); break;
      case 'uniq': cur = _.uniq(cur); break;
      case 'compact': cur = _.compact(cur); break;
      case 'without': cur = _.without(cur, ...rest[0]); break;
      case 'union': cur = _.union(cur, rest[0]); break;
      case 'intersection': cur = _.intersection(cur, rest[0]); break;
      case 'difference': cur = _.difference(cur, rest[0]); break;
      case 'chunk': return _.chunk(cur, rest[0]);
      // terminals
      case 'value': return cur;
      case 'head': return _.head(cur);
      case 'last': return _.last(cur);
      case 'nth': return _.nth(cur, rest[0]);
      case 'join': return _.join(cur, rest[0]);
      case 'size': return _.size(cur);
      case 'isEmpty': return _.isEmpty(cur);
      case 'includes': return _.includes(cur, rest[0]);
      case 'indexOf': return _.indexOf(cur, rest[0]);
      default: throw new Error('unknown seq step: ' + name);
    }
  }
  return cur;
}

function call(fn, args) {
  if (Object.prototype.hasOwnProperty.call(OPS, fn)) return OPS[fn].apply(null, args);
  const f = _[fn];
  if (typeof f !== 'function') throw new Error('unknown lodash export: ' + fn);
  return f.apply(_, args);
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  const t = line.trim();
  if (!t) return;
  let msg;
  try {
    msg = JSON.parse(t);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: 'bad request json: ' + e.message }) + '\n');
    return;
  }
  let out;
  try {
    const args = (msg.args || []).map(resolve);
    let v = call(msg.fn, args);
    v = norm(v);
    if (SORT_ALWAYS.has(msg.fn) || (SORT_IF_OBJECT.has(msg.fn) && isPlainObj((msg.args || [])[0]) && Array.isArray(v))) {
      v = sortNormed(v);
    }
    out = { id: msg.id, ok: true, value: v };
  } catch (e) {
    out = { id: msg.id, ok: false, error: String((e && e.message) || e) };
  }
  process.stdout.write(JSON.stringify(out) + '\n');
});
