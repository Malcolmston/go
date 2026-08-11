// security.ts — loads and memoizes the generated security-findings data file.
//
// api/_data/security.json is produced by scripts/build-security-data.ts from the
// parity security manifests (parity/<lib>[/nested/<pkg>]/security.json). It is the
// same record the /security page renders, and it is what the /ask assistant's
// getSecurityNotes tool answers from.
//
// Read once from api/_data/ using import.meta.url so the path resolves regardless
// of the process cwd the serverless function runs under — the same convention as
// ./data.ts. Every read is best-effort: a missing or malformed file yields an
// empty record, never a throw, because a chat tool that throws aborts the stream.

import { readDataJson } from './data';

export type SecuritySeverity = 'critical' | 'high' | 'medium' | 'low';
export type SecurityStatus = 'fixed' | 'unfixed' | 'unknown';

// One finding, exactly as the generator emits it.
export interface SecurityFinding {
  id: string;
  /** parity/<lib> directory name, e.g. "express", "socket.io". */
  library: string;
  /** parity/<lib>/nested/<pkg> directory name, when this is a nested port. */
  nestedPackage: string | null;
  /** "express" or "express/sanitizehtml" — how the port is named in prose. */
  port: string;
  repo: string;
  module: string;
  manifestPath: string;
  severity: SecuritySeverity;
  summary: string;
  affectedVersions: string;
  cases: string[];
  ghsa: string | null;
  description: string;
  scopeNote: string | null;
  upstream: string | null;
  upstreamEcosystem: string | null;
  releasedVersion: string | null;
  /** DERIVED by the generator from affectedVersions vs the library's VERSION. */
  status: SecurityStatus;
  fixedIn: string | null;
  statusReason: string;
  [key: string]: unknown;
}

export interface SecurityIndex {
  generatedAt: string;
  statusDerivation: string;
  findings: SecurityFinding[];
}

const EMPTY: SecurityIndex = { generatedAt: '', statusDerivation: '', findings: [] };

let cache: SecurityIndex | null = null;

// Resolution is delegated to ./data.ts so exactly ONE place knows where the
// generated files live at runtime. Two things make that worth centralising:
//
//  * The path differs by environment. Locally these sit next to this module, so
//    `new URL('../_data/x.json', import.meta.url)` finds them. In a deployed
//    function the compiled module is inlined into a bundle under
//    .next/server/app/api/**, and the data is traced in relative to the tracing
//    root instead — so that single candidate misses, and because the caller
//    swallows the error it degraded to EMPTY. That is how /api/health came to
//    report zero findings while /security rendered all of them.
//  * The specifier must stay dynamic. webpack statically analyses a literal
//    `new URL('./file.json', import.meta.url)` and rewrites it into an asset
//    reference (`__webpack_public_path__ + "static/media/security.<hash>.json"`),
//    a URL string rather than a readable path. Passing the name through a
//    parameter defeats that analysis, which is why readDataJson takes an argument.
function readJson(relativePath: string): unknown {
  return readDataJson(relativePath.split('/').pop() as string);
}

export function loadSecurity(): SecurityIndex {
  if (cache) return cache;
  let parsed: unknown;
  try {
    parsed = readJson('../_data/security.json');
  } catch {
    parsed = null;
  }
  cache = normalize(parsed);
  return cache;
}

function normalize(parsed: unknown): SecurityIndex {
  if (!parsed || typeof parsed !== 'object') return EMPTY;
  const d = parsed as Record<string, unknown>;
  const findings = Array.isArray(d.findings)
    ? (d.findings.filter((f) => f && typeof f === 'object') as SecurityFinding[])
    : [];
  return {
    generatedAt: typeof d.generatedAt === 'string' ? d.generatedAt : '',
    statusDerivation: typeof d.statusDerivation === 'string' ? d.statusDerivation : '',
    findings,
  };
}

export function getSecurityFindings(): SecurityFinding[] {
  return loadSecurity().findings;
}

/**
 * Filter the recorded findings. All three filters are optional and are ANDed.
 *
 *   library  matches the parity directory name, the site slug shape, or the
 *            nested package name ("express", "socket.io", "sanitizehtml").
 *   severity one of critical/high/medium/low; anything else is ignored.
 *   query    a plain substring match (case-insensitive) over the summary, id,
 *            description, affected range and case ids.
 *
 * Findings come back in the generator's order (severity, then port, then id), so
 * the most serious matches are first. Never throws.
 */
export function findSecurityFindings(opts: {
  library?: string;
  severity?: string;
  query?: string;
  limit?: number;
}): SecurityFinding[] {
  let out: SecurityFinding[];
  try {
    out = getSecurityFindings();
  } catch {
    return [];
  }
  const lib = String(opts.library ?? '').trim().toLowerCase();
  if (lib !== '') {
    // "socketio" (the site slug) and "socket.io" (the repo/parity dir) must both
    // find the socket.io findings, so compare with dots and dashes removed too.
    const loose = lib.replace(/[.\-_]/g, '');
    out = out.filter((f) => {
      const cands = [f.library, f.nestedPackage ?? '', f.port, f.repo].map((s) => String(s).toLowerCase());
      return cands.some((c) => c === lib || c.replace(/[.\-_]/g, '') === loose);
    });
  }
  const sev = String(opts.severity ?? '').trim().toLowerCase();
  if (sev !== '' && ['critical', 'high', 'medium', 'low'].includes(sev)) {
    out = out.filter((f) => f.severity === sev);
  }
  const q = String(opts.query ?? '').trim().toLowerCase();
  if (q !== '') {
    out = out.filter((f) =>
      [f.summary, f.id, f.description, f.affectedVersions, f.module, (f.cases ?? []).join(' ')]
        .join('\n')
        .toLowerCase()
        .includes(q),
    );
  }
  const limit = Number.isFinite(opts.limit) ? Math.max(1, Math.min(50, Number(opts.limit))) : out.length;
  return out.slice(0, limit);
}
