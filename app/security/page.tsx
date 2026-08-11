// /security — the disclosure record for the ports.
//
// A SERVER component: it reads the generated api/_data/security.json from disk
// (the file carries the full markdown note per finding — far too large to ship to
// the browser as a fetched asset) and hands the parsed data to the client view,
// which owns the filters. The data file is produced by
// scripts/build-security-data.ts, which `prebuild` runs on every deploy.
//
// Two readers, because the file has to be found in two different environments:
// the shared api/_lib loader resolves it relative to its own module (how the
// serverless functions read it), and the cwd read is the plain repo-root path
// that works while this page is prerendered at build time. Either is enough; a
// file found by neither renders an empty record with an explanation — never a
// build failure, and never a page that implies "no findings".
import fs from 'node:fs';
import path from 'node:path';
import type { Metadata } from 'next';
import { loadSecurity } from '../../api/_lib/security';
import { Security, type SecurityData, type SecurityFinding } from '../../src/components/Security';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'Security findings',
  description:
    'Security findings in the Go ports, recorded from the parity harness: severity, affected version range, the parity cases that detected each one, and whether the latest release is still affected.',
  path: '/security',
});

const DEFAULT_DERIVATION =
  'Fixed status is derived, not declared: the affected range recorded in each manifest is ' +
  "evaluated against the version in the library's own VERSION file.";

const EMPTY: SecurityData = {
  generatedAt: '',
  statusDerivation: DEFAULT_DERIVATION,
  counts: { total: 0, bySeverity: {}, fixed: 0, unfixed: 0, unknown: 0, libraries: 0 },
  findings: [],
};

// The data file is generated, but it is still parsed rather than trusted: a
// half-written or stale file must not throw out of the route render.
function readFromCwd(): unknown {
  try {
    return JSON.parse(fs.readFileSync(path.join(process.cwd(), 'api', '_data', 'security.json'), 'utf8'));
  } catch {
    return null;
  }
}

function normalize(parsed: unknown, findings: SecurityFinding[]): SecurityData {
  const d = (parsed && typeof parsed === 'object' ? parsed : {}) as Record<string, unknown>;
  const counts = (d.counts && typeof d.counts === 'object' ? d.counts : {}) as Partial<SecurityData['counts']>;
  return {
    generatedAt: typeof d.generatedAt === 'string' ? d.generatedAt : '',
    statusDerivation:
      typeof d.statusDerivation === 'string' && d.statusDerivation !== '' ? d.statusDerivation : DEFAULT_DERIVATION,
    counts: {
      // The view recomputes every number it shows from `findings`; these are
      // only the generator's own totals, kept for cross-checking.
      total: Number(counts.total ?? findings.length) || findings.length,
      bySeverity:
        counts.bySeverity && typeof counts.bySeverity === 'object'
          ? (counts.bySeverity as Record<string, number>)
          : {},
      fixed: Number(counts.fixed ?? 0) || 0,
      unfixed: Number(counts.unfixed ?? 0) || 0,
      unknown: Number(counts.unknown ?? 0) || 0,
      libraries: Number(counts.libraries ?? 0) || 0,
    },
    findings,
  };
}

function loadSecurityData(): SecurityData {
  try {
    const viaLib = loadSecurity();
    if (viaLib.findings.length > 0) {
      return normalize(viaLib as unknown, viaLib.findings as unknown as SecurityFinding[]);
    }
  } catch {
    // fall through to the cwd read
  }
  const parsed = readFromCwd();
  if (!parsed || typeof parsed !== 'object') return EMPTY;
  const d = parsed as Record<string, unknown>;
  const findings = Array.isArray(d.findings)
    ? (d.findings.filter((f) => f && typeof f === 'object') as SecurityFinding[])
    : [];
  return normalize(parsed, findings);
}

export default function Page() {
  return <Security data={loadSecurityData()} />;
}
