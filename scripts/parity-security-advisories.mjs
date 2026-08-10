#!/usr/bin/env node
// parity-security-advisories.mjs
//
// Turns declared parity security findings into DRAFT GitHub Security Advisories,
// one per finding, in the library's own repo. It never publishes: a draft is
// private and a human decides whether it becomes public. Auto-*publishing* a
// vulnerability is a coordinated-disclosure decision no CI job should make.
//
// The pipeline reads ONLY an explicit, opt-in manifest — parity/<lib>/security.json
// — never heuristics over parity.json. A harness with no manifest produces no
// advisory, so a failing case can never become a spurious public-facing draft.
// A finding is disclosed because a human wrote it down, not because a test went
// red.
//
// Manifest schema (parity/<lib>/security.json):
//   {
//     "repo": "socketio",              // GitHub repo under malcolmston/ (optional;
//                                       //   defaults to the directory name)
//     "module": "github.com/malcolmston/socketio",
//     "findings": [
//       {
//         "id": "namespace-middleware-silent",   // stable key, used for dedupe
//         "summary": "Namespace middleware that never calls next() connects the socket anyway",
//         "severity": "critical",       // critical | high | medium | low
//         "cases": ["namespace-middleware-silent"],
//         "affectedVersions": "<= 0.4.0",
//         "description": "full markdown: upstream vs port, impact, repro, fix"
//       }
//     ]
//   }
//
// Safety properties:
//   * DRAFT only. The API call creates state:"draft"; nothing is ever published.
//   * Idempotent. Existing advisories are listed first; a finding whose summary
//     already exists (case-insensitively) is skipped, so re-runs never duplicate.
//   * Dry-run by default. Without --apply it prints what it would create and
//     exits 0. Without a token it is forced into dry-run.
//   * Never fails the build. Missing token, missing manifest, and API errors are
//     reported and the process still exits 0, because a security-advisory
//     pipeline must not gate merges.
//
// Usage:
//   node scripts/parity-security-advisories.mjs            # dry run, all libs
//   node scripts/parity-security-advisories.mjs --apply    # create drafts
//   node scripts/parity-security-advisories.mjs --lib jose # one library
//
// Env:
//   ADVISORY_TOKEN  a PAT with security-advisory write on the target repos.
//                   The default Actions GITHUB_TOKEN usually cannot create
//                   advisories, so this is separate on purpose.

import { readdir, readFile } from 'node:fs/promises';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const OWNER = 'malcolmston';
const APPLY = process.argv.includes('--apply');
const ONE = (() => {
  const i = process.argv.indexOf('--lib');
  return i >= 0 ? process.argv[i + 1] : null;
})();
const TOKEN = process.env.ADVISORY_TOKEN || '';
const SEVERITIES = new Set(['critical', 'high', 'medium', 'low']);

function log(...a) { console.log(...a); }

async function readManifests() {
  const parityDir = join(ROOT, 'parity');
  let entries;
  try {
    entries = await readdir(parityDir, { withFileTypes: true });
  } catch {
    log('no parity/ directory; nothing to do');
    return [];
  }
  const out = [];
  for (const e of entries) {
    if (!e.isDirectory()) continue;
    if (ONE && e.name !== ONE) continue;
    const path = join(parityDir, e.name, 'security.json');
    let raw;
    try { raw = await readFile(path, 'utf8'); } catch { continue; }
    let m;
    try { m = JSON.parse(raw); } catch (err) {
      log(`! ${e.name}/security.json is not valid JSON: ${err.message} — skipped`);
      continue;
    }
    m._dir = e.name;
    m.repo = m.repo || e.name;
    out.push(m);
  }
  return out;
}

async function gh(method, path, body) {
  const res = await fetch(`https://api.github.com${path}`, {
    method,
    headers: {
      authorization: `Bearer ${TOKEN}`,
      accept: 'application/vnd.github+json',
      'x-github-api-version': '2022-11-28',
      'content-type': 'application/json',
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json;
  try { json = text ? JSON.parse(text) : null; } catch { json = { raw: text }; }
  return { ok: res.ok, status: res.status, json };
}

async function existingSummaries(repo) {
  // Draft + published; dedupe against both.
  const seen = new Set();
  for (const state of ['draft', 'triage', 'published']) {
    const r = await gh('GET', `/repos/${OWNER}/${repo}/security-advisories?per_page=100&state=${state}`);
    if (r.ok && Array.isArray(r.json)) {
      for (const a of r.json) if (a.summary) seen.add(a.summary.trim().toLowerCase());
    }
  }
  return seen;
}

function validateFinding(f) {
  const problems = [];
  if (!f.summary) problems.push('missing summary');
  if (f.summary && f.summary.length > 1024) problems.push('summary too long');
  if (!SEVERITIES.has(f.severity)) problems.push(`severity must be one of ${[...SEVERITIES].join('/')}`);
  if (!f.description) problems.push('missing description');
  if (!f.affectedVersions) problems.push('missing affectedVersions');
  return problems;
}

async function main() {
  const manifests = await readManifests();
  if (!manifests.length) { log('no security.json manifests found — nothing to disclose'); return; }
  if (!TOKEN) log('ADVISORY_TOKEN not set — DRY RUN (nothing will be created)');
  else if (!APPLY) log('--apply not passed — DRY RUN (nothing will be created)');

  let created = 0, skipped = 0, wouldCreate = 0, invalid = 0;
  const canWrite = TOKEN && APPLY;

  for (const m of manifests) {
    const repo = m.repo;
    const module = m.module || `github.com/${OWNER}/${repo}`;
    const findings = Array.isArray(m.findings) ? m.findings : [];
    if (!findings.length) continue;

    const seen = canWrite || TOKEN ? await existingSummaries(repo) : new Set();
    log(`\n== ${m._dir} -> ${OWNER}/${repo} (${findings.length} finding${findings.length === 1 ? '' : 's'}) ==`);

    for (const f of findings) {
      const problems = validateFinding(f);
      if (problems.length) { log(`  ✗ ${f.id || f.summary || '<unnamed>'}: ${problems.join('; ')}`); invalid++; continue; }
      const key = f.summary.trim().toLowerCase();
      if (seen.has(key)) { log(`  = skip (exists): ${f.summary}`); skipped++; continue; }

      if (!canWrite) { log(`  + would create [${f.severity}]: ${f.summary}`); wouldCreate++; continue; }

      const payload = {
        summary: f.summary,
        description: f.description,
        severity: f.severity,
        vulnerabilities: [{
          package: { ecosystem: 'go', name: module },
          vulnerable_version_range: f.affectedVersions,
          patched_versions: f.patchedVersions || null,
          vulnerable_functions: f.vulnerableFunctions || [],
        }],
      };
      const r = await gh('POST', `/repos/${OWNER}/${repo}/security-advisories`, payload);
      if (r.ok) { log(`  + created ${r.json.ghsa_id} (draft) [${f.severity}]: ${f.summary}`); created++; seen.add(key); }
      else { log(`  ! failed (${r.status}) for "${f.summary}": ${JSON.stringify(r.json).slice(0, 200)}`); }
    }
  }

  log(`\nsummary: ${created} created, ${wouldCreate} would-create, ${skipped} skipped (exist), ${invalid} invalid`);
  log('every advisory is a DRAFT; publishing is always a manual decision.');
}

main().catch((e) => { log(`unexpected error (not failing the build): ${e.stack || e}`); });
