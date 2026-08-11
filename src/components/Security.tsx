'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { hx } from 'go-ui';
import { LIBS } from '../data';
import { repoKey } from '../parityLookup';
import { SecH } from './SecH';

// Security — the disclosure record for the ports themselves.
//
// Every incident here came out of the parity harness: running the original
// library and the Go port over the same inputs occasionally shows the port is
// less SAFE than the thing it ports. Those findings are written into
// parity/<lib>[/nested/<pkg>]/security.json and turned into PRIVATE DRAFT GitHub
// Security Advisories; publishing a draft is always a human decision. This page
// renders those manifests, generated into api/_data/security.json by
// scripts/build-security-data.ts.
//
// The fixed/unfixed column is DERIVED by that script (affected range vs the
// library's VERSION file) and carries its own derivation sentence per finding —
// this view never asserts a status of its own, and never widens an affected
// range beyond what the manifest states.

export type SecuritySeverity = 'critical' | 'high' | 'medium' | 'low';
export type SecurityStatus = 'fixed' | 'unfixed' | 'unknown';

export interface SecurityFinding {
  id: string;
  library: string;
  nestedPackage: string | null;
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
  status: SecurityStatus;
  fixedIn: string | null;
  statusReason: string;
}

export interface SecurityData {
  generatedAt: string;
  statusDerivation: string;
  counts: {
    total: number;
    bySeverity: Record<string, number>;
    fixed: number;
    unfixed: number;
    unknown: number;
    libraries: number;
  };
  findings: SecurityFinding[];
}

const SEVERITY_ORDER: SecuritySeverity[] = ['critical', 'high', 'medium', 'low'];
const SEVERITY_RANK: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 };

// Severity colours are the site's own, not a vendor scale: red / amber / yellow /
// slate, tinted the same way the parity pills are.
const SEVERITY_COLOR: Record<SecuritySeverity, string> = {
  critical: '#ff5c7c',
  high: '#ff8a3d',
  medium: '#e3b341',
  low: '#8b98a9',
};

const STATUS_LABEL: Record<SecurityStatus, string> = {
  fixed: 'fixed',
  unfixed: 'unfixed in the latest release',
  unknown: 'status not derivable',
};

// The site slug for a parity directory name. parity/ dirs are repo names
// (socket.io), while the landing's ids are its own slugs (socketio) — the same
// mismatch parityFor() handles, resolved the same way.
function libForDir(dir: string) {
  return LIBS.find((l) => l.id === dir) ?? LIBS.find((l) => repoKey(l) === dir);
}

function Markdown({ text }: { text: string }) {
  return (
    <div className="ask-md sec-md">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          // Manifest prose is authored in this repo, but it is still rendered
          // content: send every outbound link out of the tab safely, and never
          // let it inherit the page's own link styling silently.
          a: ({ href, children }) => {
            const url = href ?? '';
            const external = /^https?:\/\//.test(url);
            return (
              <a
                className="ask-link"
                href={url}
                {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
              >
                {children}
              </a>
            );
          },
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}

export function Security({ data }: { data: SecurityData }) {
  const findings = data.findings;

  const [severity, setSeverity] = useState<'all' | SecuritySeverity>('all');
  const [library, setLibrary] = useState<'all' | string>('all');
  const [status, setStatus] = useState<'all' | SecurityStatus>('all');

  // Counts come from the data, never from prose: the strip below is arithmetic
  // over exactly the findings this page renders.
  const counts = useMemo(() => {
    const bySeverity: Record<string, number> = {};
    for (const s of SEVERITY_ORDER) bySeverity[s] = findings.filter((f) => f.severity === s).length;
    return {
      total: findings.length,
      bySeverity,
      fixed: findings.filter((f) => f.status === 'fixed').length,
      unfixed: findings.filter((f) => f.status === 'unfixed').length,
      unknown: findings.filter((f) => f.status === 'unknown').length,
      ports: new Set(findings.map((f) => f.port)).size,
    };
  }, [findings]);

  const libraryOptions = useMemo(() => {
    const seen = new Map<string, number>();
    for (const f of findings) seen.set(f.library, (seen.get(f.library) ?? 0) + 1);
    return Array.from(seen.entries()).sort((a, b) => (a[0] < b[0] ? -1 : 1));
  }, [findings]);

  const shown = useMemo(
    () =>
      findings
        .filter((f) => severity === 'all' || f.severity === severity)
        .filter((f) => library === 'all' || f.library === library)
        .filter((f) => status === 'all' || f.status === status)
        // Severity first, then library — the requested grouping, applied here so
        // the order is the same whatever the generator emitted.
        .sort(
          (a, b) =>
            (SEVERITY_RANK[a.severity] ?? 99) - (SEVERITY_RANK[b.severity] ?? 99) ||
            (a.port < b.port ? -1 : a.port > b.port ? 1 : 0) ||
            (a.id < b.id ? -1 : a.id > b.id ? 1 : 0),
        ),
    [findings, severity, library, status],
  );

  // Group the visible rows by severity so each block gets one heading.
  const groups = useMemo(() => {
    const out: { severity: SecuritySeverity; items: SecurityFinding[] }[] = [];
    for (const s of SEVERITY_ORDER) {
      const items = shown.filter((f) => f.severity === s);
      if (items.length > 0) out.push({ severity: s, items });
    }
    return out;
  }, [shown]);

  const filtered = shown.length !== findings.length;

  return (
    <section className="view active" id="view-security">
      <SecH h="h2">Security findings in the ports</SecH>
      <p className="muted">
        The parity harness runs the original library and the Go port over the same inputs. Most disagreements are
        behaviour gaps; a few mean the port is <b>less safe</b> than the library it ports — it accepts a token upstream
        rejects, admits an unauthenticated socket, reflects a request path unescaped. Those are recorded as findings
        rather than filed as public issues, and this page is the record. Every field below is read from{' '}
        <code>parity/&lt;lib&gt;/security.json</code>; nothing here is hand-written prose about a version.
      </p>

      <div className="sec-stats">
        <div className="sec-tile">
          <div className="sec-num">{counts.total}</div>
          <div className="sec-lbl">recorded findings</div>
        </div>
        <div className="sec-tile">
          <div className="sec-num">{counts.ports}</div>
          <div className="sec-lbl">ports affected</div>
        </div>
        <div className="sec-tile">
          <div className="sec-num">{counts.fixed}</div>
          <div className="sec-lbl">fixed in the latest release</div>
        </div>
        <div className="sec-tile">
          <div className="sec-num">{counts.unfixed}</div>
          <div className="sec-lbl">unfixed in the latest release</div>
        </div>
        {counts.unknown > 0 && (
          <div className="sec-tile">
            <div className="sec-num">{counts.unknown}</div>
            <div className="sec-lbl">status not derivable</div>
          </div>
        )}
      </div>
      <div className="sec-sev-strip">
        {SEVERITY_ORDER.map((s) => (
          <span
            key={s}
            className="sec-pill"
            style={{
              borderColor: hx(SEVERITY_COLOR[s], '66'),
              color: SEVERITY_COLOR[s],
              background: hx(SEVERITY_COLOR[s], '16'),
            }}
          >
            {counts.bySeverity[s] ?? 0} {s}
          </span>
        ))}
      </div>

      <SecH>How a finding gets here</SecH>
      <div className="note">
        <b>Manifest → private draft advisory → a human decides.</b> A parity case that shows the port is less safe than
        upstream is never opened as a public issue: a public issue with a working repro discloses an exploitable
        vulnerability in a shipped library before a fix exists. The finding is written into{' '}
        <code>parity/&lt;lib&gt;/security.json</code>, and the{' '}
        <a
          href="https://github.com/malcolmston/go/blob/main/.github/workflows/parity-advisories.yml"
          target="_blank"
          rel="noopener"
        >
          <code>parity-advisories</code>
        </a>{' '}
        workflow turns each one into a <b>draft</b> — i.e. private — GitHub Security Advisory in that library's own repo,
        idempotently, never on push. Publishing a draft is always a manual decision made once a fix is ready. That is
        why a finding on this page may have no public advisory to link to: the record exists, the disclosure timing
        belongs to the maintainer. A finding is disclosed because someone wrote it down, never because a test went red.
      </div>
      <div className="note">
        <b>How the fixed/unfixed column is derived.</b> {data.statusDerivation} Nothing on this page declares a status
        by hand, and each finding carries the exact sentence its own status came from. Where a finding&apos;s note says
        &ldquo;no fix released&rdquo; but the derived status says fixed, the note is the text as written when the
        finding was recorded and the status reflects a release that landed afterwards — the derivation, not the prose,
        is the current answer.
      </div>
      <div className="note">
        <b>Findings that were withdrawn are kept.</b> Several proposed findings turned out to be wrong about released
        code once they were reproduced against the actually-published module, and were dropped or corrected. Where a
        manifest records that, it appears below as a <b>scope note</b> on the finding it sits next to. It is kept
        deliberately: a disclosure record that only lists the claims that survived is not auditable.
      </div>

      <SecH>Findings</SecH>
      <div className="sec-filters">
        <label>
          <span className="muted small">Severity</span>
          <select value={severity} onChange={(e) => setSeverity(e.target.value as 'all' | SecuritySeverity)}>
            <option value="all">All severities ({counts.total})</option>
            {SEVERITY_ORDER.map((s) => (
              <option key={s} value={s}>
                {s} ({counts.bySeverity[s] ?? 0})
              </option>
            ))}
          </select>
        </label>
        <label>
          <span className="muted small">Library</span>
          <select value={library} onChange={(e) => setLibrary(e.target.value)}>
            <option value="all">All libraries ({libraryOptions.length})</option>
            {libraryOptions.map(([id, n]) => (
              <option key={id} value={id}>
                {id} ({n})
              </option>
            ))}
          </select>
        </label>
        <label>
          <span className="muted small">Status</span>
          <select value={status} onChange={(e) => setStatus(e.target.value as 'all' | SecurityStatus)}>
            <option value="all">Fixed and unfixed ({counts.total})</option>
            <option value="fixed">Fixed in the latest release ({counts.fixed})</option>
            <option value="unfixed">Unfixed in the latest release ({counts.unfixed})</option>
            {counts.unknown > 0 && <option value="unknown">Status not derivable ({counts.unknown})</option>}
          </select>
        </label>
        {filtered && (
          <button
            type="button"
            className="pill"
            onClick={() => {
              setSeverity('all');
              setLibrary('all');
              setStatus('all');
            }}
          >
            Clear filters
          </button>
        )}
        <span className="muted small sec-count">
          {shown.length} of {counts.total} shown
        </span>
      </div>

      {shown.length === 0 && (
        <p className="muted">
          No recorded finding matches this filter. That is the filter, not a clean bill of health — widen it to see the
          rest of the record.
        </p>
      )}

      {groups.map((g) => (
        <div key={g.severity} className="sec-group">
          <h3 className="sec-group-h">
            <span
              className="sec-pill"
              style={{
                borderColor: hx(SEVERITY_COLOR[g.severity], '66'),
                color: SEVERITY_COLOR[g.severity],
                background: hx(SEVERITY_COLOR[g.severity], '16'),
              }}
            >
              {g.severity}
            </span>
            <span className="muted small">
              {g.items.length} finding{g.items.length === 1 ? '' : 's'}
            </span>
          </h3>
          {g.items.map((f) => (
            <Finding key={`${f.manifestPath}#${f.id}`} f={f} />
          ))}
        </div>
      ))}

      <p className="muted small" style={{ marginTop: '1rem' }}>
        Generated from the manifests on the last deploy{data.generatedAt ? ` (${data.generatedAt})` : ''} by{' '}
        <code>scripts/build-security-data.ts</code>. To report something that is not listed here, use the repo&apos;s{' '}
        <a href="https://github.com/malcolmston/go/blob/main/SECURITY.md" target="_blank" rel="noopener">
          SECURITY.md
        </a>{' '}
        — please do not open a public issue with a working repro.
      </p>
      <style>{securityCss}</style>
    </section>
  );
}

function Finding({ f }: { f: SecurityFinding }) {
  const lib = libForDir(f.library);
  const accent = lib?.accent ?? '#8b98a9';
  const sevColor = SEVERITY_COLOR[f.severity];
  const statusColor = f.status === 'unfixed' ? SEVERITY_COLOR.critical : f.status === 'fixed' ? '#3fb950' : '#8b98a9';

  return (
    <article className="sec-card card">
      <header className="sec-card-h">
        <div className="sec-card-title">
          {lib ? (
            <Link href={`/lib/${lib.id}`} className="sec-lib" style={{ color: accent }}>
              {f.port}
            </Link>
          ) : (
            <span className="sec-lib" style={{ color: accent }}>
              {f.port}
            </span>
          )}
          <span
            className="sec-pill"
            style={{ borderColor: hx(sevColor, '66'), color: sevColor, background: hx(sevColor, '16') }}
          >
            {f.severity}
          </span>
          <span
            className="sec-pill"
            style={{ borderColor: hx(statusColor, '66'), color: statusColor, background: hx(statusColor, '16') }}
            title={f.statusReason}
          >
            {f.status === 'fixed' && f.fixedIn ? `fixed in ${f.fixedIn}` : STATUS_LABEL[f.status]}
          </span>
        </div>
        <p className="sec-summary">{f.summary}</p>
        {f.nestedPackage && (
          <p className="muted small">
            A <b>nested port</b>: <code>{f.nestedPackage}</code> lives inside the {f.library} repo but ports a different
            upstream package of its own
            {f.upstream ? (
              <>
                {' '}
                — {f.upstreamEcosystem ?? 'upstream'} <code>{f.upstream}</code>
              </>
            ) : null}
            . It is measured against that package, not against {f.library}.
          </p>
        )}
      </header>

      <dl className="sec-meta">
        <div>
          <dt>Finding id</dt>
          <dd className="mono">{f.id}</dd>
        </div>
        <div>
          <dt>Module</dt>
          <dd className="mono">{f.module}</dd>
        </div>
        <div>
          <dt>Affected versions</dt>
          <dd className="mono">{f.affectedVersions}</dd>
        </div>
        <div>
          <dt>Released version</dt>
          <dd className="mono">{f.releasedVersion ?? 'unknown'}</dd>
        </div>
        {!f.nestedPackage && f.upstream && (
          <div>
            <dt>Upstream oracle</dt>
            <dd className="mono">
              {f.upstreamEcosystem ? `${f.upstreamEcosystem} ` : ''}
              {f.upstream}
            </dd>
          </div>
        )}
        <div>
          <dt>Parity cases that detected it</dt>
          <dd className="mono">
            {f.cases.length > 0 ? (
              f.cases.join(', ')
            ) : (
              <span className="muted">named in the finding note below</span>
            )}
          </dd>
        </div>
        <div>
          <dt>Manifest</dt>
          <dd className="mono">{f.manifestPath}</dd>
        </div>
      </dl>

      <p className="sec-derived muted small">
        <b>Status derivation.</b> {f.statusReason}
      </p>

      {f.scopeNote && (
        <div className="note sec-scope">
          <b>Scope note from the manifest.</b>
          <Markdown text={f.scopeNote} />
        </div>
      )}

      <details className="sec-desc">
        <summary>Full finding note (as written in the manifest)</summary>
        <Markdown text={f.description} />
      </details>
    </article>
  );
}

const securityCss = `
.sec-stats { display:grid; grid-template-columns:repeat(auto-fit,minmax(10.5rem,1fr)); gap:.7rem; margin:1rem 0 .4rem; }
.sec-tile { padding:.85rem 1rem; border-radius:14px; border:1px solid var(--edge); border-top:3px solid var(--edge-2); background:var(--glass); backdrop-filter:var(--blur); box-shadow:var(--hi); }
.sec-num { font-size:1.5rem; font-weight:700; letter-spacing:-.02em; color:var(--fg); }
.sec-lbl { margin-top:.15rem; font-size:.76rem; color:var(--fg-dim); }
.sec-sev-strip { display:flex; flex-wrap:wrap; gap:.4rem; margin:.5rem 0 1rem; }
.sec-pill { display:inline-block; padding:.05rem .55rem; border-radius:999px; border:1px solid; font-weight:600; font-size:.78rem; text-transform:lowercase; letter-spacing:.01em; }
.sec-filters { display:flex; flex-wrap:wrap; gap:.7rem; align-items:flex-end; margin:.7rem 0 1rem; }
.sec-filters label { display:flex; flex-direction:column; gap:.2rem; }
.sec-filters select { background:var(--glass-2); color:var(--fg); border:1px solid var(--edge); border-radius:8px; padding:.35rem .5rem; font:inherit; font-size:.85rem; }
.sec-count { margin-left:auto; }
.sec-group { margin:1.2rem 0; }
.sec-group-h { display:flex; align-items:center; gap:.6rem; margin:.2rem 0 .6rem; font-size:.95rem; font-weight:600; }
.sec-card { padding:1rem 1.1rem; margin:.7rem 0; }
.sec-card-h { display:flex; flex-direction:column; gap:.35rem; }
.sec-card-title { display:flex; flex-wrap:wrap; align-items:center; gap:.5rem; }
.sec-lib { font-weight:700; font-size:1rem; text-decoration:none; }
.sec-summary { margin:0; color:var(--fg); line-height:1.55; }
.sec-meta { display:grid; grid-template-columns:repeat(auto-fit,minmax(15rem,1fr)); gap:.5rem .9rem; margin:.9rem 0 .4rem; }
.sec-meta dt { color:var(--fg-dim); font-size:.72rem; text-transform:uppercase; letter-spacing:.04em; }
.sec-meta dd { margin:.1rem 0 0; overflow-wrap:anywhere; }
.sec-meta .mono, .sec-card .mono { font-family:"SF Mono",ui-monospace,monospace; font-size:.82rem; color:var(--fg-muted); }
.sec-derived { margin:.5rem 0 .2rem; line-height:1.5; }
.sec-scope { margin:.7rem 0 .2rem; }
.sec-scope .sec-md > :first-child { margin-top:.3rem; }
.sec-desc { margin-top:.7rem; border-top:1px solid var(--edge); padding-top:.6rem; }
.sec-desc > summary { cursor:pointer; color:var(--fg-muted); font-size:.85rem; }
.sec-md { margin-top:.6rem; line-height:1.6; }
.sec-md table { border-collapse:collapse; font-size:.85rem; }
.sec-md th, .sec-md td { border:1px solid var(--edge); padding:.3rem .55rem; text-align:left; }
.sec-md pre { overflow-x:auto; }
`;
