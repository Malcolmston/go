// The react parity gate.
//
//   cd parity/react/node && npx vitest run
//
// What it does, in order:
//
//   1. loads every case in ../cases/*.json;
//   2. builds ../go (the port's runner) once, with GOWORK=off so the harness
//      measures the local ../../react tree through the replace directive rather
//      than a published version;
//   3. starts that runner once and streams all cases through it over the
//      JSON-Lines contract in parity/HARNESS.md;
//   4. renders the same cases here with real react-dom/server — this process *is*
//      the upstream runner, no subprocess needed;
//   5. asserts one test per case, and writes ../parity.json with the totals and
//      per-group counts.
//
// No expectation is written down anywhere: React's answer is the expectation.

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { readFileSync, readdirSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { spawn, spawnSync } from 'node:child_process';
import { createInterface } from 'node:readline';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { answer, VERSION } from './render.mjs';

const HARNESS = resolve(import.meta.dirname, '..');
const CASES = join(HARNESS, 'cases');

// ---------------------------------------------------------------- case files

function loadCases() {
  const files = readdirSync(CASES).filter((f) => f.endsWith('.json')).sort();
  if (files.length === 0) throw new Error('no case files in cases/');
  const all = [];
  const seen = new Map();
  let upstream = '';
  for (const file of files) {
    const cf = JSON.parse(readFileSync(join(CASES, file), 'utf8'));
    if (!cf.upstream) throw new Error(`${file}: missing pinned "upstream" version`);
    if (!upstream) upstream = cf.upstream;
    else if (upstream !== cf.upstream) {
      throw new Error(`${file} pins ${cf.upstream} but another file pins ${upstream}`);
    }
    for (const c of cf.cases) {
      if (seen.has(c.id)) throw new Error(`duplicate case id ${c.id} (${seen.get(c.id)}, ${file})`);
      seen.set(c.id, file);
      all.push({ ...c, group: cf.group });
    }
  }
  return { cases: all, upstream };
}

const { cases, upstream } = loadCases();

// The oracle must be the version the cases pin, or the score is not reproducible.
const pinned = `react@${JSON.parse(readFileSync(join(HARNESS, 'node', 'package.json'), 'utf8')).dependencies.react}`;

// ---------------------------------------------------------------- go runner

/** Streams every case through one long-lived Go subprocess. */
async function askGoRunner(list) {
  const dir = mkdtempSync(join(tmpdir(), 'react-parity-'));
  const bin = join(dir, 'react-parity-runner');
  const build = spawnSync('go', ['build', '-o', bin, './go'], {
    cwd: HARNESS,
    env: { ...process.env, GOWORK: 'off' },
    encoding: 'utf8',
  });
  if (build.status !== 0) {
    rmSync(dir, { recursive: true, force: true });
    throw new Error(`go build ./go failed:\n${build.stderr || build.stdout || build.error}`);
  }

  const child = spawn(bin, { stdio: ['pipe', 'pipe', 'inherit'] });
  const replies = new Map();
  const done = (async () => {
    for await (const line of createInterface({ input: child.stdout, crlfDelay: Infinity })) {
      const text = line.trim();
      if (!text) continue;
      const rep = JSON.parse(text);
      replies.set(rep.id, rep);
    }
  })();

  for (const c of list) {
    child.stdin.write(JSON.stringify({ id: c.id, fn: c.fn, args: c.args }) + '\n');
  }
  child.stdin.end();

  const exit = new Promise((ok, bad) => {
    child.on('error', bad);
    child.on('close', ok);
  });
  const timeout = new Promise((_, bad) =>
    setTimeout(() => bad(new Error('go runner did not finish within 120s')), 120_000).unref(),
  );
  await Promise.race([Promise.all([done, exit]), timeout]);
  rmSync(dir, { recursive: true, force: true });

  const missing = list.filter((c) => !replies.has(c.id));
  if (missing.length) {
    throw new Error(`go runner answered ${replies.size}/${list.length}; missing ${missing.slice(0, 5).map((c) => c.id).join(', ')}`);
  }
  return replies;
}

// ------------------------------------------------------------------- scoring

const goReplies = new Map();
const result = {
  total: 0,
  match: 0,
  mismatch: 0,
  deviations: 0,
  groups: new Map(),
  mismatched: [],
  deviationIDs: [],
};

function bump(group, field) {
  if (!result.groups.has(group)) {
    result.groups.set(group, { group, total: 0, match: 0, mismatch: 0, deviations: 0 });
  }
  result.groups.get(group)[field] += 1;
}

beforeAll(async () => {
  if (pinned !== upstream.split(' ')[0]) {
    throw new Error(`node/package.json installs ${pinned} but the cases pin ${upstream}`);
  }
  for (const [id, rep] of await askGoRunner(cases)) goReplies.set(id, rep);
});

afterAll(() => {
  const compared = result.match + result.mismatch;
  const goMod = readFileSync(join(HARNESS, 'go.mod'), 'utf8');
  const version = (goMod.match(/github\.com\/malcolmston\/react\s+(v\S+)/) || [, 'unknown'])[1];
  const score = {
    library: 'react',
    goModule: `github.com/malcolmston/react@${version}`,
    upstream,
    generated: new Date().toISOString().replace(/\.\d+Z$/, 'Z'),
    total: result.total,
    match: result.match,
    mismatch: result.mismatch,
    deviations: result.deviations,
    parityPct: compared ? Math.round((result.match / compared) * 10000) / 100 : 0,
    groups: [...result.groups.values()].sort((a, b) => a.group.localeCompare(b.group)),
    mismatchedCases: result.mismatched,
    deviationCases: result.deviationIDs,
  };
  writeFileSync(join(HARNESS, 'parity.json'), JSON.stringify(score, null, 2) + '\n');
  // eslint-disable-next-line no-console
  console.log(
    `react parity: ${score.match}/${compared} cases match (${score.parityPct}%), ` +
      `${score.deviations} documented deviations, oracle ${VERSION}`,
  );
});

// --------------------------------------------------------------------- tests

const byGroup = new Map();
for (const c of cases) {
  if (!byGroup.has(c.group)) byGroup.set(c.group, []);
  byGroup.get(c.group).push(c);
}

function show(rep) {
  return rep.ok ? JSON.stringify(rep.value) : `FAIL(${rep.error})`;
}

for (const [group, list] of [...byGroup.entries()].sort()) {
  describe(group, () => {
    for (const c of list) {
      it(c.id, () => {
        const up = answer(c);
        const go = goReplies.get(c.id);
        const same = up.ok === go.ok && (!up.ok || up.value === go.value);

        result.total += 1;
        if (c.deviation) {
          result.deviations += 1;
          result.deviationIDs.push(c.id);
          bump(group, 'total');
          bump(group, 'deviations');
          // A documented, deliberate difference is reported, never a failure —
          // but it stops being a deviation the moment the two agree.
          if (same) {
            throw new Error(
              `case ${c.id} is marked as a deviation but upstream and the port now agree; ` +
                `remove the deviation marker.\n  ${show(up)}`,
            );
          }
          return;
        }

        bump(group, 'total');
        if (same) {
          result.match += 1;
          bump(group, 'match');
          return;
        }
        result.mismatch += 1;
        result.mismatched.push(c.id);
        bump(group, 'mismatch');
        expect.fail(
          `case ${c.id} (${c.feature || group})\n` +
            `  fn:       ${c.fn}\n` +
            `  upstream: ${show(up)}\n` +
            `  port:     ${show(go)}` +
            (c.note ? `\n  note:     ${c.note}` : ''),
        );
      });
    }
  });
}
