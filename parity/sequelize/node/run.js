'use strict';
// Upstream parity runner for sequelize over SQLite.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//   in : {"id":..,"fn":..,"args":[..]}
//   out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Each case runs against a freshly built, identically seeded in-memory SQLite
// database, so cases are order-independent. Never exits on a failing case; logs
// go to stderr only.

const readline = require('readline');
const { Sequelize, DataTypes, Op } = require('sequelize');

// ------------------------------------------------------------- schema + seed

async function schema() {
  const sequelize = new Sequelize({ dialect: 'sqlite', storage: ':memory:', logging: false });

  const User = sequelize.define('User', {
    id: { type: DataTypes.INTEGER, primaryKey: true, autoIncrement: true },
    name: { type: DataTypes.STRING(64), allowNull: false },
    age: { type: DataTypes.INTEGER },
    email: { type: DataTypes.STRING(128), unique: true },
    active: { type: DataTypes.BOOLEAN },
    score: { type: DataTypes.FLOAT },
  }, { timestamps: false, tableName: 'users' });

  const Post = sequelize.define('Post', {
    id: { type: DataTypes.INTEGER, primaryKey: true, autoIncrement: true },
    title: { type: DataTypes.STRING(128), allowNull: false },
    userId: { type: DataTypes.INTEGER },
  }, { timestamps: false, tableName: 'posts' });

  User.hasMany(Post, { foreignKey: 'userId', as: 'posts' });
  Post.belongsTo(User, { foreignKey: 'userId', as: 'user' });

  await sequelize.sync({ force: true });

  await User.bulkCreate([
    { id: 1, name: 'Alice', age: 30, email: 'alice@example.com', active: true, score: 4.5 },
    { id: 2, name: 'Bob', age: 25, email: 'bob@example.com', active: false, score: 3.0 },
    { id: 3, name: 'Carol', age: 35, email: 'carol@example.com', active: true, score: 5.0 },
    { id: 4, name: 'Dave', age: 25, email: null, active: false, score: 2.5 },
  ]);
  await Post.bulkCreate([
    { id: 1, title: 'Hello', userId: 1 },
    { id: 2, title: 'World', userId: 1 },
    { id: 3, title: 'Foo', userId: 2 },
    { id: 4, title: 'Bar', userId: 3 },
  ]);

  return { sequelize, User, Post };
}

// ------------------------------------------------------------- where DSL

const OPS = {
  eq: Op.eq, ne: Op.ne, gt: Op.gt, gte: Op.gte, lt: Op.lt, lte: Op.lte,
  like: Op.like, notLike: Op.notLike, in: Op.in, notIn: Op.notIn,
  between: Op.between, notBetween: Op.notBetween, is: Op.is, not: Op.not,
};

function buildWhereMap(m) {
  const out = {};
  for (const key of Object.keys(m)) {
    if (key === 'and') {
      out[Op.and] = m[key].map(buildWhereMap);
    } else if (key === 'or') {
      out[Op.or] = m[key].map(buildWhereMap);
    } else if (key === 'not') {
      out[Op.not] = buildWhereMap(m[key]);
    } else {
      out[key] = buildField(m[key]);
    }
  }
  return out;
}

function buildField(cond) {
  if (cond === null) return null;
  if (Array.isArray(cond)) return cond; // shorthand IN
  if (typeof cond === 'object') {
    const out = {};
    for (const op of Object.keys(cond)) {
      if (!(op in OPS)) throw new Error('unknown operator ' + op);
      out[OPS[op]] = cond[op];
    }
    return out;
  }
  return cond; // scalar eq
}

function buildWhere(raw) {
  if (raw === undefined || raw === null) return undefined;
  return buildWhereMap(raw);
}

function buildQuery(dsl, models) {
  const q = {};
  if (!dsl) return q;
  if (dsl.where) q.where = buildWhere(dsl.where);
  if (dsl.order) q.order = dsl.order.map((o) => [o[0], o[1] || 'ASC']);
  if (dsl.limit !== undefined && dsl.limit !== null) q.limit = dsl.limit;
  if (dsl.offset !== undefined && dsl.offset !== null) q.offset = dsl.offset;
  if (dsl.attributes) q.attributes = dsl.attributes;
  if (dsl.distinct) q.distinct = true;
  if (dsl.include) {
    q.include = dsl.include.map((name) => ({ model: models[name], as: name }));
  }
  return q;
}

// include names map to association aliases + models
function includeModels(name, User, Post) {
  if (name === 'posts') return { posts: Post };
  if (name === 'user') return { user: User };
  return {};
}

// ------------------------------------------------------------- normalisation

const TS = new Set(['createdAt', 'updatedAt', 'deletedAt']);

function normalize(v) {
  if (v === null || v === undefined) return null;
  if (Array.isArray(v)) {
    const arr = v.map(normalize);
    if (arr.length && arr[0] && typeof arr[0] === 'object' && 'id' in arr[0]) {
      arr.sort((a, b) => (a.id || 0) - (b.id || 0));
    }
    return arr;
  }
  if (typeof v === 'object') {
    const out = {};
    for (const k of Object.keys(v)) {
      if (TS.has(k)) continue;
      out[k] = normalize(v[k]);
    }
    return out;
  }
  return v;
}

function normalizeRows(rows, hasOrder) {
  const arr = rows.map((r) => normalize(r.toJSON ? r.toJSON() : r));
  if (!hasOrder) arr.sort((a, b) => (a.id || 0) - (b.id || 0));
  return arr;
}

// ------------------------------------------------------------- dispatch

async function handle(req) {
  const { User, Post, sequelize } = await schema();
  try {
    const models = { posts: Post, user: User, User, Post };
    switch (req.fn) {
      case 'createThing': {
        const Thing = sequelize.define('Thing', {
          id: { type: DataTypes.INTEGER, primaryKey: true, autoIncrement: true },
          amount: { type: DataTypes.DECIMAL(10, 2) },
        }, { timestamps: false, tableName: 'things' });
        await Thing.sync();
        const row = await Thing.create(req.args[0]);
        return normalize(row.toJSON());
      }
      case 'findAll': {
        const dsl = req.args[0] || {};
        const rows = await User.findAll(buildQuery(dsl, models));
        return normalizeRows(rows, !!(dsl.order && dsl.order.length));
      }
      case 'findAllPost': {
        const dsl = req.args[0] || {};
        const rows = await Post.findAll(buildQuery(dsl, models));
        return normalizeRows(rows, !!(dsl.order && dsl.order.length));
      }
      case 'findOne': {
        const dsl = req.args[0] || {};
        const row = await User.findOne(buildQuery(dsl, models));
        if (!row) return null;
        return normalize(row.toJSON());
      }
      case 'findByPk': {
        const row = await User.findByPk(req.args[0]);
        if (!row) return null;
        return normalize(row.toJSON());
      }
      case 'count': {
        const dsl = req.args[0] || {};
        return await User.count(buildQuery(dsl, models));
      }
      case 'sum': {
        const v = await User.sum(req.args[0], buildQuery(req.args[1] || {}, models));
        return v === null || v === undefined || Number.isNaN(v) ? null : v;
      }
      case 'max': {
        const v = await User.max(req.args[0], buildQuery(req.args[1] || {}, models));
        return v === undefined ? null : v;
      }
      case 'min': {
        const v = await User.min(req.args[0], buildQuery(req.args[1] || {}, models));
        return v === undefined ? null : v;
      }
      case 'create': {
        const row = await User.create(req.args[0]);
        return normalize(row.toJSON());
      }
      case 'bulkCreate': {
        const rows = await User.bulkCreate(req.args[0]);
        return normalizeRows(rows, false);
      }
      case 'update': {
        const [n] = await User.update(req.args[0], { where: buildWhere(req.args[1]) || {} });
        return n;
      }
      case 'destroy': {
        return await User.destroy({ where: buildWhere(req.args[0]) || {} });
      }
      case 'increment': {
        await User.increment(req.args[0], { where: buildWhere(req.args[1]) || {} });
        return await User.count({ where: buildWhere(req.args[1]) || {} });
      }
      default:
        throw new Error('unknown fn ' + req.fn);
    }
  } finally {
    await sequelize.close();
  }
}

// ------------------------------------------------------------- main loop

const rl = readline.createInterface({ input: process.stdin });
let queue = Promise.resolve();

rl.on('line', (line) => {
  line = line.trim();
  if (!line) return;
  let req;
  try {
    req = JSON.parse(line);
  } catch (e) {
    process.stderr.write('bad request: ' + e.message + '\n');
    return;
  }
  // Serialise: process one case fully before the next, preserving reply order.
  queue = queue.then(async () => {
    let resp;
    try {
      const value = await handle(req);
      resp = { id: req.id, ok: true, value };
    } catch (e) {
      resp = { id: req.id, ok: false, error: e && e.message ? e.message : String(e) };
    }
    process.stdout.write(JSON.stringify(resp) + '\n');
  });
});
