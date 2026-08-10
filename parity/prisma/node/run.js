// Upstream oracle runner for the prisma parity harness.
//
// Speaks the JSON-Lines protocol described in parity/HARNESS.md: one request
// object per line on stdin, exactly one reply object per line on stdout, never
// exiting on a failing case.
//
// Every case is a SCRIPT: a list of steps run in order against one SQLite
// database. Step 1 of every script is a `reset` (every table truncated and the
// AUTOINCREMENT sequence cleared) so each case starts from an empty schema, and
// step 2 is normally a `seed` of the fixed dataset below. Each step produces its
// own {ok, code?, value?} object, and the script never aborts: a step that
// throws is recorded and the following steps still run, which is what lets a
// must-fail case be followed by a `dump` proving nothing was written.
//
// Error MESSAGES are never emitted, only Prisma's error `code` (P2002, P2025,
// ...) — the harness compares codes, not prose. Messages go to stderr.
'use strict';

const fs = require('fs');
const os = require('os');
const path = require('path');
const readline = require('readline');
const { PrismaClient } = require('@prisma/client');

// ----------------------------------------------------------------- the seed
//
// The dataset is hard-coded identically in this file and in ../go/run.go rather
// than repeated in every case file. `seed-dump` in cases/basics.json dumps both
// tables straight after seeding, so a divergence in the seed itself is scored
// like any other case instead of silently skewing the rest of the suite.
//
// ids are consecutive from 1 because the Go port always treats an integer
// primary key as database-generated (see cases/errors.json:create-explicit-id),
// so it can only reproduce these ids by inserting in this order.
const SEED = {
  base: {
    users: [
      { id: 1, name: 'Ada', email: 'ada@x.io', city: 'Paris', age: 36, score: 10 },
      { id: 2, name: 'Grace', email: 'grace@x.io', city: 'Paris', age: 44, score: 20 },
      { id: 3, name: 'Alan', email: 'alan@x.io', city: 'London', age: 40, score: 30 },
      { id: 4, name: 'Edsger', email: 'edsger@x.io', city: null, age: 30, score: 40 },
      { id: 5, name: 'Barbara', email: 'barbara@x.io', city: 'London', age: null, score: 50 },
      { id: 6, name: 'Linus', email: 'linus@x.io', city: 'Berlin', age: 24, score: 60 },
      { id: 7, name: 'Ken', email: 'ken@x.io', city: null, age: null, score: 70 },
      { id: 8, name: 'Donald', email: 'donald@x.io', city: 'Berlin', age: 60, score: 80 },
    ],
    posts: [
      { id: 1, title: 'Alpha', views: 10, published: true, author_id: 1 },
      { id: 2, title: 'Beta', views: 20, published: false, author_id: 1 },
      { id: 3, title: 'Gamma', views: 30, published: true, author_id: 2 },
      { id: 4, title: 'Delta', views: 40, published: true, author_id: 2 },
      { id: 5, title: 'Epsilon', views: 50, published: false, author_id: 3 },
      { id: 6, title: 'Zeta', views: 60, published: true, author_id: 3 },
      { id: 7, title: 'Eta', views: 70, published: false, author_id: 4 },
      { id: 8, title: 'Theta', views: 80, published: true, author_id: 6 },
      { id: 9, title: 'Iota', views: 90, published: true, author_id: 6 },
      { id: 10, title: 'Kappa', views: 100, published: false, author_id: 6 },
      { id: 11, title: '50% off', views: 5, published: true, author_id: 8 },
      { id: 12, title: 'beta_two', views: 15, published: false, author_id: 8 },
    ],
  },
};

// --------------------------------------------------------------- canonicalise

// canon renders a Prisma result as a JSON-comparable value: BigInt becomes a
// number, Date becomes an RFC3339 string, undefined becomes null. Object key
// order is irrelevant — the harness deep-equals decoded values.
function canon(v) {
  if (v === undefined) return null;
  if (v === null) return null;
  if (typeof v === 'bigint') return Number(v);
  if (v instanceof Date) return v.toISOString();
  if (Array.isArray(v)) return v.map(canon);
  if (typeof v === 'object') {
    const out = {};
    for (const k of Object.keys(v).sort()) out[k] = canon(v[k]);
    return out;
  }
  return v;
}

// errCode extracts the Prisma error code, or "" for errors that carry none
// (PrismaClientValidationError, raw SQL failures). The harness only compares
// codes when both sides expose one.
function errCode(e) {
  if (e && typeof e.code === 'string' && /^P\d{4}$/.test(e.code)) return e.code;
  return '';
}

// ------------------------------------------------------------------- database

const dbFile =
  process.env.PARITY_DB_FILE ||
  path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'prisma-parity-node-')), 'parity.db');

// The template database is the one `prisma db push` created from schema.prisma:
// copying it means the oracle and the port run against byte-identical DDL that
// neither runner hand-wrote.
const template = process.env.PARITY_TEMPLATE_DB;
if (template) fs.copyFileSync(template, dbFile);

const prisma = new PrismaClient({ datasources: { db: { url: 'file:' + dbFile } } });

// TABLES is in delete order: children before parents, so a reset never trips the
// posts -> users foreign key.
const TABLES = ['posts', 'users'];

async function reset() {
  for (const t of TABLES) await prisma.$executeRawUnsafe(`DELETE FROM "${t}"`);
  // Prisma's SQLite DDL makes every `Int @id` AUTOINCREMENT, so the rowid
  // counter survives a DELETE. Clearing sqlite_sequence is what makes ids
  // restart at 1 for every case regardless of which cases ran before.
  await prisma.$executeRawUnsafe(`DELETE FROM sqlite_sequence`);
  return null;
}

async function seed(name) {
  const data = SEED[name];
  if (!data) throw new Error(`unknown dataset ${name}`);
  await prisma.user.createMany({ data: data.users });
  await prisma.post.createMany({ data: data.posts });
  return { users: data.users.length, posts: data.posts.length };
}

// ---------------------------------------------------------------- query args

// buildFindArgs turns the case's step into Prisma's argument object. The case
// files are written in Prisma's own vocabulary, so upstream needs almost no
// translation; ../go/run.go translates the same keys into the port's builder.
function buildFindArgs(s) {
  const args = {};
  if (s.where !== undefined) args.where = s.where;
  if (s.select !== undefined) {
    args.select = {};
    for (const f of s.select) args.select[f] = true;
  }
  if (s.include !== undefined) {
    args.include = {};
    for (const r of s.include) args.include[r] = true;
  }
  if (s.orderBy !== undefined) args.orderBy = s.orderBy;
  if (s.distinct !== undefined) args.distinct = s.distinct;
  if (s.take !== undefined) args.take = s.take;
  if (s.skip !== undefined) args.skip = s.skip;
  if (s.cursor !== undefined) args.cursor = s.cursor;
  return args;
}

// writeArgs collects the pagination/ordering arguments a case asked for on a
// write. Prisma has no such arguments on update/delete, so anything here makes
// upstream fail — which is the point.
function writeArgs(s) {
  const args = {};
  if (s.orderBy !== undefined) args.orderBy = s.orderBy;
  if (s.take !== undefined) args.take = s.take;
  if (s.skip !== undefined) args.skip = s.skip;
  return args;
}

function aggArgs(s) {
  const args = {};
  if (s.where !== undefined) args.where = s.where;
  if (s._count) args._count = true;
  for (const fn of ['_sum', '_avg', '_min', '_max']) {
    if (s[fn] !== undefined) {
      args[fn] = {};
      for (const c of s[fn]) args[fn][c] = true;
    }
  }
  return args;
}

// ---------------------------------------------------------------- operations

async function runStep(s) {
  const model = s.model === 'post' ? prisma.post : prisma.user;

  switch (s.op) {
    case 'reset':
      return await reset();
    case 'seed':
      return await seed(s.dataset || 'base');

    case 'dump': {
      const rows = await (s.model === 'post' ? prisma.post : prisma.user).findMany({
        orderBy: { id: 'asc' },
      });
      return rows;
    }

    case 'create':
      return await model.create({ data: s.data });
    case 'createMany': {
      const r = await model.createMany({ data: s.rows });
      return { count: r.count };
    }
    case 'nestedCreate':
      // s.data carries a nested `posts: { create: [...] }` block.
      return await model.create({ data: s.data });

    case 'findMany':
      return await model.findMany(buildFindArgs(s));
    case 'findFirst':
      return await model.findFirst(buildFindArgs(s));
    case 'findFirstOrThrow':
      return await model.findFirstOrThrow(buildFindArgs(s));
    case 'findUnique':
      return await model.findUnique(buildFindArgs(s));
    case 'findUniqueOrThrow':
      return await model.findUniqueOrThrow(buildFindArgs(s));

    // update/updateMany/delete/deleteMany forward take/skip/orderBy when the
    // case supplies them. Prisma's write API has no such arguments and rejects
    // them; the port silently accepts and ignores them, which is exactly the
    // difference the pagination-on-writes cases are there to catch.
    case 'update':
      await model.update({ ...writeArgs(s), where: s.where, data: s.data });
      return { count: 1 };
    case 'updateMany': {
      const r = await model.updateMany({ ...writeArgs(s), where: s.where, data: s.data });
      return { count: r.count };
    }
    case 'upsert':
      await model.upsert({ where: s.where, create: s.create, update: s.update });
      return { count: 1 };

    case 'delete':
      await model.delete({ ...writeArgs(s), where: s.where });
      return { count: 1 };
    case 'deleteMany': {
      const r = await model.deleteMany({ ...writeArgs(s), where: s.where });
      return { count: r.count };
    }

    case 'count':
      return await model.count({ where: s.where });

    case 'aggregate':
      return await model.aggregate(aggArgs(s));

    case 'groupBy': {
      const args = { by: s.by };
      if (s.where !== undefined) args.where = s.where;
      if (s.orderBy !== undefined) args.orderBy = s.orderBy;
      if (s.having !== undefined) args.having = s.having;
      if (s._count) args._count = true;
      for (const fn of ['_sum', '_avg', '_min', '_max']) {
        if (s[fn] !== undefined) {
          args[fn] = {};
          for (const c of s[fn]) args[fn][c] = true;
        }
      }
      return await model.groupBy(args);
    }

    default:
      throw new Error(`unknown op ${s.op}`);
  }
}

async function runScript(steps) {
  const out = [];
  for (const s of steps) {
    try {
      const value = await runStep(s);
      out.push({ ok: true, value: canon(value) });
    } catch (e) {
      process.stderr.write(`step ${s.op}: ${e && e.message}\n`);
      // `value: null` and an always-present `code` keep the failure shape
      // identical to the Go runner's, so a deep-equal never trips on a key that
      // one side merely omitted.
      out.push({ ok: false, code: errCode(e), value: null });
    }
  }
  return { steps: out };
}

// -------------------------------------------------------------------- driver

async function main() {
  const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
  for await (const line of rl) {
    if (!line.trim()) continue;
    let req;
    try {
      req = JSON.parse(line);
    } catch (e) {
      process.stdout.write(JSON.stringify({ id: '', ok: false, error: 'bad request' }) + '\n');
      continue;
    }
    let reply;
    try {
      if (req.fn !== 'script') throw new Error(`unknown fn ${req.fn}`);
      reply = { id: req.id, ok: true, value: await runScript(req.args[0]) };
    } catch (e) {
      reply = { id: req.id, ok: false, error: String((e && e.message) || e) };
    }
    process.stdout.write(JSON.stringify(reply) + '\n');
  }
  await prisma.$disconnect();
}

main().catch((e) => {
  process.stderr.write('runner failed: ' + (e && e.stack) + '\n');
  process.exit(1);
});
