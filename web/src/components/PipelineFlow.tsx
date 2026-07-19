import { useEffect, useMemo, useState } from 'react';
import type { CSSProperties } from 'react';
import type { Lib } from '../data';
import type { Parity } from '../parity';
import { repoKey } from '../parityLookup';
import './PipelineFlow.css';

export interface PipelineFlowProps {
  lib: Lib;
  parity?: Parity;
}

// LatestRun is the slice of the GitHub Actions "list workflow runs" response we
// use to show the parity pipeline's live status.
interface LatestRun {
  status: string; // queued | in_progress | completed
  conclusion: string | null; // success | failure | cancelled | ...
  html_url: string;
  created_at: string;
}

type NodeState = 'idle' | 'run' | 'ok' | 'fail';

interface Stage {
  id: string;
  title: string;
  sub: string;
  icon: string;
  metric?: string;
  hub?: boolean; // the central reusable-workflow node
  exec?: boolean; // a real CI step whose state tracks the run conclusion
}

function stagesFor(parity?: Parity): Stage[] {
  return [
    { id: 'trigger', title: 'Trigger', sub: 'push · pull_request', icon: '⎇' },
    { id: 'reusable', title: 'parity-reusable.yml', sub: 'go · workflow_call', icon: '⟳', hub: true },
    { id: 'checkout', title: 'Checkout', sub: 'actions/checkout', icon: '⤓', exec: true },
    { id: 'setup', title: 'Setup Go', sub: 'setup-go@v5', icon: 'go', exec: true },
    { id: 'build', title: 'Build', sub: 'go build ./…', icon: '⚙', exec: true },
    {
      id: 'test', title: 'Parity tests', sub: 'go test -run Parity', icon: '✓', exec: true,
      metric: parity ? `${parity.casesSynced.toLocaleString()} upstream cases` : undefined,
    },
    {
      id: 'summary', title: 'Summary', sub: 'before → after', icon: 'Σ', exec: true,
      metric: parity ? `${parity.before} → ${parity.after}` : undefined,
    },
    {
      id: 'publish', title: 'Publish', sub: 'parity.json artifact', icon: '⬆', exec: true,
      metric: parity ? `${parity.gapsClosed} gaps closed` : undefined,
    },
  ];
}

function timeAgo(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return '';
  const s = Math.max(0, (Date.now() - then) / 1000);
  if (s < 90) return 'just now';
  const m = s / 60;
  if (m < 90) return `${Math.round(m)}m ago`;
  const h = m / 60;
  if (h < 36) return `${Math.round(h)}h ago`;
  return `${Math.round(h / 24)}d ago`;
}

// PipelineFlow renders the real stages of the upstream-parity CI pipeline as a
// React-Flow-style node graph: the push/PR trigger routes into the central
// reusable workflow in `go`, which checks out, builds, runs the synced parity
// tests, summarizes the before→after score, and publishes parity.json. The
// library's own metrics annotate the relevant nodes, and the pipeline's live
// status is fetched from the GitHub Actions API (best-effort; the diagram still
// renders with metrics if the fetch is unavailable).
export function PipelineFlow({ lib, parity }: PipelineFlowProps) {
  const repo = repoKey(lib);
  const [run, setRun] = useState<LatestRun | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setRun(null);
    setLoaded(false);
    const url = `https://api.github.com/repos/malcolmston/${repo}/actions/workflows/parity.yml/runs?per_page=1`;
    fetch(url, { headers: { Accept: 'application/vnd.github+json' } })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((j: { workflow_runs?: LatestRun[] }) => {
        if (cancelled) return;
        setRun(j.workflow_runs && j.workflow_runs[0] ? j.workflow_runs[0] : null);
        setLoaded(true);
      })
      .catch(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, [repo]);

  // Derive the state applied to executable nodes from the live run.
  const execState: NodeState = useMemo(() => {
    if (!run) return 'idle';
    if (run.status !== 'completed') return 'run';
    return run.conclusion === 'success' ? 'ok' : run.conclusion === 'failure' ? 'fail' : 'idle';
  }, [run]);

  const stages = stagesFor(parity);
  const statusLabel = !loaded
    ? 'checking…'
    : !run
      ? 'no runs yet'
      : run.status !== 'completed'
        ? 'running'
        : run.conclusion === 'success'
          ? 'passing'
          : run.conclusion ?? 'unknown';
  const statusKind: NodeState =
    !run ? 'idle' : run.status !== 'completed' ? 'run' : run.conclusion === 'success' ? 'ok' : 'fail';

  return (
    <div className="pipeflow" style={{ '--pf-accent': lib.accent } as CSSProperties}>
      <div className="pipeflow-head">
        <span className={`pipeflow-status pf-${statusKind}`}>
          <span className="pipeflow-dot" /> parity pipeline · {statusLabel}
        </span>
        {run ? (
          <a className="pipeflow-link" href={run.html_url} target="_blank" rel="noopener">
            latest run{run.created_at ? ` · ${timeAgo(run.created_at)}` : ''} ↗
          </a>
        ) : (
          <a
            className="pipeflow-link"
            href={`${lib.repo}/actions/workflows/parity.yml`}
            target="_blank"
            rel="noopener"
          >
            view on GitHub ↗
          </a>
        )}
      </div>

      <div className="pipeflow-rail" role="list" aria-label="Parity pipeline stages">
        {stages.map((s, i) => {
          const state: NodeState = s.hub || !s.exec ? 'idle' : execState;
          return (
            <div className="pipeflow-cell" key={s.id}>
              <div
                role="listitem"
                className={`pipeflow-node ${s.hub ? 'is-hub' : ''} pf-${state}`}
                title={`${s.title} — ${s.sub}`}
              >
                <span className="pipeflow-icon" aria-hidden>{s.icon}</span>
                <div className="pipeflow-body">
                  <div className="pipeflow-title">{s.title}</div>
                  <div className="pipeflow-sub">{s.sub}</div>
                  {s.metric ? <div className="pipeflow-metric">{s.metric}</div> : null}
                </div>
                {s.exec ? <span className="pipeflow-nodedot" /> : null}
              </div>
              {i < stages.length - 1 ? (
                <svg className="pipeflow-edge" viewBox="0 0 48 24" aria-hidden preserveAspectRatio="none">
                  <path d="M0 12 H40" className="pipeflow-edgeline" />
                  <path d="M36 6 L44 12 L36 18" className="pipeflow-edgehead" fill="none" />
                </svg>
              ) : null}
            </div>
          );
        })}
      </div>

      {parity ? (
        <div className="pipeflow-legend">
          Verified against <a href={`https://github.com/${parity.upstream}`} target="_blank" rel="noopener">{parity.upstream}</a>:
          {' '}the port syncs <b>{parity.casesSynced.toLocaleString()}</b> of the upstream library's own test vectors, which raised
          measured parity from <b>{parity.before}</b> to <b>{parity.after}</b> by closing <b>{parity.gapsClosed}</b> behavior gaps.
        </div>
      ) : (
        <div className="pipeflow-legend">This library is not yet audited against an upstream test suite.</div>
      )}
    </div>
  );
}
