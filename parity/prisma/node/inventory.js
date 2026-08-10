// Mechanical upstream API inventory for COVERAGE.md.
//
// Run: node inventory.js            (from parity/prisma/node)
//      node inventory.js --counts   (just the totals)
//
// Two sources, both derived from the INSTALLED, GENERATED client rather than
// from documentation:
//
//  1. Reflection over the live client and its model delegates
//     (`Object.getOwnPropertyNames`), which yields the $-prefixed client methods
//     and every method on `prisma.user` / `prisma.post`.
//  2. The generated `node_modules/.prisma/client/index.d.ts`, which is where the
//     argument vocabulary lives. Each `export type X = { ... }` block is parsed
//     for its top-level property names: the where/orderBy/args/data/having input
//     types are what a case can actually say.
'use strict';

const fs = require('fs');
const { PrismaClient } = require('@prisma/client');

const prisma = new PrismaClient({ datasources: { db: { url: 'file:./.parity/template.db' } } });

const rows = [];
const add = (s) => { if (!rows.includes(s)) rows.push(s); };

// ---------------------------------------------------------------- reflection

const objectNoise = new Set(Object.getOwnPropertyNames(Object.prototype));
objectNoise.add('constructor');

function methodsOf(obj) {
  const seen = new Set();
  for (let o = obj; o && o !== Object.prototype; o = Object.getPrototypeOf(o)) {
    for (const k of Object.getOwnPropertyNames(o)) {
      if (objectNoise.has(k) || k.startsWith('_')) continue;
      let v;
      try { v = obj[k]; } catch { continue; }
      if (typeof v === 'function') seen.add(k);
    }
  }
  return [...seen].sort();
}

for (const k of methodsOf(prisma)) add('PrismaClient#' + k);
for (const k of methodsOf(prisma.user)) add('<model>.' + k);

// -------------------------------------------------------------- generated d.ts

const dts = fs.readFileSync('node_modules/.prisma/client/index.d.ts', 'utf8');

// keysOf returns the top-level property names of `export type <name> = { ... }`.
// Generic parameters may themselves contain `=` (`<$PrismaModel = never>`), so the
// body is found by matching braces from the first `{` rather than by a regex.
function keysOf(name) {
  const at = dts.search(new RegExp('export type ' + name + '[<\\s]'));
  if (at < 0) return [];
  const open = dts.indexOf('{', at);
  if (open < 0) return [];
  let depth = 0;
  let end = open;
  for (let i = open; i < dts.length; i++) {
    if (dts[i] === '{') depth++;
    else if (dts[i] === '}') { depth--; if (depth === 0) { end = i; break; } }
  }
  const body = dts.slice(open + 1, end);
  const keys = [];
  let depth2 = 0;
  for (const line of body.split('\n')) {
    const m = depth2 === 0 && line.match(/^\s{4}([A-Za-z_$][A-Za-z0-9_]*)\??:/);
    if (m) keys.push(m[1]);
    for (const ch of line) {
      if (ch === '{') depth2++;
      else if (ch === '}') depth2--;
    }
  }
  return keys;
}

const inputs = [
  ['UserWhereInput', 'where'],
  ['StringFilter', 'where.<String>'],
  ['StringNullableFilter', 'where.<String?>'],
  ['IntFilter', 'where.<Int>'],
  ['IntNullableFilter', 'where.<Int?>'],
  ['FloatFilter', 'where.<Float>'],
  ['BoolFilter', 'where.<Boolean>'],
  ['PostListRelationFilter', 'where.<to-many>'],
  ['UserRelationFilter', 'where.<to-one>'],
  ['UserOrderByWithRelationInput', 'orderBy'],
  ['SortOrderInput', 'orderBy.<nullable>'],
  ['UserFindManyArgs', 'findMany args'],
  ['UserAggregateArgs', 'aggregate args'],
  ['UserGroupByArgs', 'groupBy args'],
  ['UserScalarWhereWithAggregatesInput', 'having'],
  ['IntWithAggregatesFilter', 'having.<Int>'],
  ['UserUpdateInput', 'update data'],
  ['IntFieldUpdateOperationsInput', 'update data.<Int>'],
  ['UserUpsertArgs', 'upsert args'],
  ['UserCreateInput', 'create data'],
  ['PostCreateNestedManyWithoutAuthorInput', 'create data.<to-many>'],
  ['PostUpdateManyWithoutAuthorNestedInput', 'update data.<to-many>'],
  ['UserCountAggregateOutputType', 'aggregate _count output'],
];

for (const [type, label] of inputs) {
  const keys = keysOf(type);
  if (!keys.length) {
    process.stderr.write('WARNING: no keys parsed for ' + type + '\n');
    continue;
  }
  for (const k of keys) add(label + ': ' + k);
}

// The schema/migration surface is not part of the client at all; it is the CLI.
// Listing it here is what makes its absence from the port auditable.
const prismaHelp = (...args) =>
  require('child_process').execFileSync('node_modules/.bin/prisma', args, {
    encoding: 'utf8',
    env: { ...process.env, PARITY_DB_URL: 'file:./.parity/template.db' },
  });
const cliHelp = prismaHelp('--help');
// The Commands block of `prisma --help` (and of each sub-help) lists one command
// per line, right-aligned in a column of variable width, description first
// capitalised word.
const cliCommands = (help, prefix) => {
  // `prisma migrate --help` splits its commands over three "Commands for ..."
  // headings, so take everything from the first Commands heading to Options.
  const from = help.search(/^\s*Commands?\b.*$/m);
  if (from < 0) return;
  const rest = help.slice(from);
  const to = rest.slice(1).search(/^\s*(Options|Flags|Examples)\s*$/m);
  const block = to < 0 ? rest : rest.slice(0, to + 1);
  for (const line of block.split('\n')) {
    const m = line.match(/^\s{2,}([a-z]+)\s{2,}[A-Z]/);
    if (m) add('prisma CLI: ' + prefix + m[1]);
  }
};
cliCommands(cliHelp, '');
for (const sub of ['db', 'migrate']) {
  cliCommands(prismaHelp(sub, '--help'), sub + ' ');
}

if (process.argv.includes('--counts')) {
  process.stdout.write('symbols: ' + rows.length + '\n');
} else {
  process.stderr.write('symbols: ' + rows.length + '\n');
  process.stdout.write(rows.join('\n') + '\n');
}
prisma.$disconnect();
