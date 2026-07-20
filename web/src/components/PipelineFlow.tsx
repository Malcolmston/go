import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as RPointerEvent } from 'react';
import type { Lib } from '../data';
import type { Parity } from '../parity';
import { repoKey } from '../parityLookup';
import './PipelineFlow.css';

export interface PipelineFlowProps {
  lib: Lib;
  parity?: Parity;
}

interface LatestRun {
  status: string;
  conclusion: string | null;
  html_url: string;
  created_at: string;
}

type Status = 'pending' | 'running' | 'success' | 'failed';
type Kind = 'trigger' | 'build' | 'test' | 'deploy';

interface Stage {
  id: string;
  code: string;
  label: string;
  sub: string;
  kind: Kind;
  x: number;
  y: number;
}
interface Edge { from: string; to: string }

const NODE_W = 200;
const NODE_H = 66;

const KIND_ACCENT: Record<Kind, string> = {
  trigger: '#8b5cf6',
  build: '#3b82f6',
  test: '#f59e0b',
  deploy: '#10b981',
};
const STATUS_COLOR: Record<Status, string> = {
  pending: '#94a3b8',
  running: '#3b82f6',
  success: '#10b981',
  failed: '#ef4444',
};

// The real stages of the parity pipeline (parity-reusable.yml), laid out as a
// small DAG: the push/PR trigger routes into the central reusable workflow,
// which checks out + sets up Go, builds, runs the synced parity tests,
// summarizes the before→after score, and publishes parity.json.
function stagesFor(parity?: Parity): Stage[] {
  return [
    { id: 'push', code: 'PUSH', label: 'Push · PR', sub: 'webhook', kind: 'trigger', x: 24, y: 150 },
    { id: 'reusable', code: 'GO', label: 'parity-reusable.yml', sub: 'workflow_call', kind: 'build', x: 268, y: 150 },
    { id: 'checkout', code: 'CHK', label: 'Checkout', sub: 'actions/checkout', kind: 'build', x: 512, y: 40 },
    { id: 'setup', code: 'GO', label: 'Setup Go', sub: 'setup-go@v5', kind: 'build', x: 512, y: 260 },
    { id: 'build', code: 'BLD', label: 'Build', sub: 'go build ./…', kind: 'build', x: 756, y: 150 },
    { id: 'test', code: 'TEST', label: 'Parity tests', sub: parity ? `${parity.casesSynced.toLocaleString()} cases` : 'go test -run Parity', kind: 'test', x: 1000, y: 150 },
    { id: 'summary', code: 'SUM', label: 'Summary', sub: parity ? `${parity.before} → ${parity.after}` : 'before → after', kind: 'test', x: 1244, y: 150 },
    { id: 'publish', code: 'PUB', label: 'Publish', sub: parity ? `${parity.gapsClosed} gaps closed` : 'parity.json', kind: 'deploy', x: 1488, y: 150 },
  ];
}
const EDGES: Edge[] = [
  { from: 'push', to: 'reusable' },
  { from: 'reusable', to: 'checkout' },
  { from: 'reusable', to: 'setup' },
  { from: 'checkout', to: 'build' },
  { from: 'setup', to: 'build' },
  { from: 'build', to: 'test' },
  { from: 'test', to: 'summary' },
  { from: 'summary', to: 'publish' },
];
const ORDER = ['push', 'reusable', 'checkout', 'setup', 'build', 'test', 'summary', 'publish'];

function bezier(a: { x: number; y: number }, b: { x: number; y: number }): string {
  const dx = Math.max(45, Math.abs(b.x - a.x) * 0.5);
  return `M${a.x},${a.y} C${a.x + dx},${a.y} ${b.x - dx},${b.y} ${b.x},${b.y}`;
}
function timeAgo(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return '';
  const s = Math.max(0, (Date.now() - then) / 1000);
  if (s < 90) return 'just now';
  const m = s / 60;
  if (m < 90) return `${Math.round(m)}m ago`;
  const h = m / 60;
  return h < 36 ? `${Math.round(h)}h ago` : `${Math.round(h / 24)}d ago`;
}

// PipelineFlow renders the parity pipeline as an interactive React-Flow-style
// node graph (imported from the "CI/CD Pipeline Visualizer" design): a dotted
// canvas of code-badged stage cards wired by animated bezier edges, with a
// control panel, legend and minimap. Node statuses reflect the live GitHub
// Actions run (best-effort); "Replay" animates the run in topological order.
export function PipelineFlow({ lib, parity }: PipelineFlowProps) {
  const repo = repoKey(lib);
  const paneRef = useRef<HTMLDivElement | null>(null);
  const stages = useMemo(() => stagesFor(parity), [parity]);

  const [pos, setPos] = useState<Record<string, { x: number; y: number }>>(() =>
    Object.fromEntries(stages.map((s) => [s.id, { x: s.x, y: s.y }])),
  );
  const [view, setView] = useState({ tx: 0, ty: 0, k: 0.62 });
  const [status, setStatus] = useState<Record<string, Status>>(() =>
    Object.fromEntries(stages.map((s) => [s.id, 'success'])),
  );
  const [run, setRun] = useState<LatestRun | null>(null);
  const [running, setRunning] = useState(false);
  const drag = useRef<{ mode: 'pan' | 'node'; id?: string; sx: number; sy: number; ox: number; oy: number } | null>(null);
  const runTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  // Live status from the GitHub Actions API (best-effort). The fetch is bounded
  // by an AbortController timeout: the graph renders from static parity data on
  // its own, so a slow/blocked GitHub request (e.g. behind a proxy that never
  // returns) must never keep the page's network in flight — it just falls back
  // to the "N/N stages" label.
  useEffect(() => {
    let cancelled = false;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 8000);
    const url = `https://api.github.com/repos/malcolmston/${repo}/actions/workflows/parity.yml/runs?per_page=1`;
    fetch(url, { headers: { Accept: 'application/vnd.github+json' }, signal: ctrl.signal })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((j: { workflow_runs?: LatestRun[] }) => {
        if (cancelled) return;
        const r = j.workflow_runs && j.workflow_runs[0];
        setRun(r || null);
        if (r) {
          const exec: Status = r.status !== 'completed' ? 'running' : r.conclusion === 'success' ? 'success' : r.conclusion === 'failure' ? 'failed' : 'pending';
          setStatus((prev) => {
            const next = { ...prev };
            for (const s of stages) next[s.id] = s.kind === 'trigger' ? 'success' : exec;
            return next;
          });
        }
      })
      .catch(() => {});
    return () => { cancelled = true; clearTimeout(timer); ctrl.abort(); };
  }, [repo, stages]);

  const fit = useCallback(() => {
    const pane = paneRef.current;
    if (!pane) return;
    const r = pane.getBoundingClientRect();
    const xs = stages.map((s) => pos[s.id]?.x ?? s.x);
    const ys = stages.map((s) => pos[s.id]?.y ?? s.y);
    const minX = Math.min(...xs), maxX = Math.max(...xs) + NODE_W;
    const minY = Math.min(...ys), maxY = Math.max(...ys) + NODE_H;
    const pad = 48;
    const k = Math.min(1.1, Math.min((r.width - pad) / (maxX - minX), (r.height - pad) / (maxY - minY)));
    setView({ k, tx: (r.width - (maxX - minX) * k) / 2 - minX * k, ty: (r.height - (maxY - minY) * k) / 2 - minY * k });
  }, [pos, stages]);

  useEffect(() => { const t = setTimeout(fit, 30); return () => clearTimeout(t); }, [fit]);

  const world = (cx: number, cy: number) => {
    const r = paneRef.current!.getBoundingClientRect();
    return { x: (cx - r.left - view.tx) / view.k, y: (cy - r.top - view.ty) / view.k };
  };

  const onPaneDown = (e: RPointerEvent) => {
    drag.current = { mode: 'pan', sx: e.clientX, sy: e.clientY, ox: view.tx, oy: view.ty };
  };
  const onNodeDown = (id: string, e: RPointerEvent) => {
    e.stopPropagation();
    const w = world(e.clientX, e.clientY);
    const p = pos[id];
    drag.current = { mode: 'node', id, sx: e.clientX, sy: e.clientY, ox: w.x - p.x, oy: w.y - p.y };
  };
  useEffect(() => {
    const move = (e: PointerEvent) => {
      const d = drag.current;
      if (!d) return;
      if (d.mode === 'pan') {
        setView((v) => ({ ...v, tx: d.ox + (e.clientX - d.sx), ty: d.oy + (e.clientY - d.sy) }));
      } else if (d.mode === 'node' && d.id) {
        const w = world(e.clientX, e.clientY);
        setPos((p) => ({ ...p, [d.id!]: { x: w.x - d.ox, y: w.y - d.oy } }));
      }
    };
    const up = () => { drag.current = null; };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    return () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); };
  });

  const onWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    const r = paneRef.current!.getBoundingClientRect();
    const sx = e.clientX - r.left, sy = e.clientY - r.top;
    setView((v) => {
      const nk = Math.min(2, Math.max(0.3, v.k * (e.deltaY < 0 ? 1.12 : 0.89)));
      return { k: nk, tx: sx - (sx - v.tx) * (nk / v.k), ty: sy - (sy - v.ty) * (nk / v.k) };
    });
  };
  const zoom = (f: number) => {
    const r = paneRef.current!.getBoundingClientRect();
    const sx = r.width / 2, sy = r.height / 2;
    setView((v) => {
      const nk = Math.min(2, Math.max(0.3, v.k * f));
      return { k: nk, tx: sx - (sx - v.tx) * (nk / v.k), ty: sy - (sy - v.ty) * (nk / v.k) };
    });
  };

  const replay = () => {
    if (running) return;
    clearTimeout(runTimer.current);
    setRunning(true);
    setStatus(Object.fromEntries(stages.map((s) => [s.id, 'pending'])) as Record<string, Status>);
    let i = 0;
    const step = () => {
      if (i >= ORDER.length) { setRunning(false); return; }
      const id = ORDER[i];
      setStatus((s) => ({ ...s, [id]: 'running' }));
      runTimer.current = setTimeout(() => {
        setStatus((s) => ({ ...s, [id]: 'success' }));
        i++;
        step();
      }, 420);
    };
    runTimer.current = setTimeout(step, 150);
  };
  useEffect(() => () => clearTimeout(runTimer.current), []);

  const out = (id: string) => ({ x: pos[id].x + NODE_W, y: pos[id].y + NODE_H / 2 });
  const inp = (id: string) => ({ x: pos[id].x, y: pos[id].y + NODE_H / 2 });

  const okCount = stages.filter((s) => status[s.id] === 'success').length;
  const statusLabel = running
    ? 'replaying…'
    : !run
      ? `${okCount}/${stages.length} stages`
      : run.status !== 'completed'
        ? 'running'
        : run.conclusion === 'success'
          ? 'passing'
          : run.conclusion ?? 'unknown';

  // minimap
  const mmW = 150, mmH = 92, mmPad = 8;
  const xs = stages.map((s) => pos[s.id].x), ys = stages.map((s) => pos[s.id].y);
  const mnX = Math.min(...xs), mxX = Math.max(...xs) + NODE_W, mnY = Math.min(...ys), mxY = Math.max(...ys) + NODE_H;
  const mSc = Math.min((mmW - mmPad * 2) / (mxX - mnX || 1), (mmH - mmPad * 2) / (mxY - mnY || 1));
  const mOx = (mmW - (mxX - mnX) * mSc) / 2, mOy = (mmH - (mxY - mnY) * mSc) / 2;

  return (
    <div className="pipeflow" style={{ '--pf-accent': lib.accent } as CSSProperties}>
      <div
        ref={paneRef}
        className="pipeflow-pane"
        onPointerDown={onPaneDown}
        onWheel={onWheel}
      >
        <div className="pipeflow-bg" style={{ backgroundPosition: `${view.tx}px ${view.ty}px`, backgroundSize: `${22 * view.k}px ${22 * view.k}px` }} />
        <div className="pipeflow-world" style={{ transform: `translate(${view.tx}px, ${view.ty}px) scale(${view.k})` }}>
          <svg className="pipeflow-edges" aria-hidden>
            {EDGES.map((e) => {
              const a = status[e.from], b = status[e.to];
              const flowing = b === 'running';
              const done = a === 'success' && (b === 'success' || b === 'running');
              const stroke = flowing ? lib.accent : done ? KIND_ACCENT[stages.find((s) => s.id === e.from)!.kind] : 'var(--edge-2)';
              return (
                <path
                  key={`${e.from}-${e.to}`}
                  d={bezier(out(e.from), inp(e.to))}
                  fill="none"
                  stroke={stroke}
                  strokeWidth={flowing ? 2.6 : done ? 2 : 1.6}
                  strokeLinecap="round"
                  className={flowing ? 'pipeflow-edge-flow' : ''}
                />
              );
            })}
          </svg>
          {stages.map((s) => {
            const st = status[s.id];
            const p = pos[s.id];
            return (
              <div
                key={s.id}
                className="pipeflow-card"
                style={{ left: p.x, top: p.y, width: NODE_W, height: NODE_H }}
                onPointerDown={(e) => onNodeDown(s.id, e)}
              >
                <div className="pipeflow-row">
                  <span className="pipeflow-code" style={{ background: KIND_ACCENT[s.kind] }}>{s.code}</span>
                  <span className="pipeflow-meta">
                    <span className="pipeflow-label">{s.label}</span>
                    <span className="pipeflow-dur">{s.sub}</span>
                  </span>
                  <span
                    className={`pipeflow-dot${st === 'running' ? ' is-pulse' : ''}`}
                    style={{ background: STATUS_COLOR[st] }}
                  />
                </div>
                <div className="pipeflow-kindrow">
                  <span className="pipeflow-kind">{s.kind}</span>
                  <span className="pipeflow-sep" />
                  <span style={{ color: STATUS_COLOR[st] }}>{st}</span>
                </div>
                <span className="pipeflow-handle in" />
                <span className="pipeflow-handle out" style={{ background: lib.accent }} />
              </div>
            );
          })}
        </div>

        {/* control panel */}
        <div className="pipeflow-panel pipeflow-tl">
          <div className="pipeflow-panel-hd">
            <span className="pipeflow-panel-ico" style={{ background: lib.accent }}>⟳</span>
            <span className="pipeflow-panel-title">Parity pipeline</span>
          </div>
          <div className={`pipeflow-panel-sub pf-${run ? (run.status !== 'completed' ? 'run' : run.conclusion === 'success' ? 'ok' : 'fail') : 'ok'}`}>
            <span className="pipeflow-status-dot" /> {statusLabel}
            {run?.created_at ? ` · ${timeAgo(run.created_at)}` : ''}
          </div>
          <div className="pipeflow-panel-btns">
            <button type="button" className="pipeflow-run" onClick={replay} disabled={running}>▶ {running ? 'Replaying…' : 'Replay'}</button>
            {run ? (
              <a className="pipeflow-ghlink" href={run.html_url} target="_blank" rel="noopener">run ↗</a>
            ) : (
              <a className="pipeflow-ghlink" href={`${lib.repo}/actions/workflows/parity.yml`} target="_blank" rel="noopener">GitHub ↗</a>
            )}
          </div>
          <div className="pipeflow-legend2">
            {([['trigger', 'Trigger'], ['build', 'Build'], ['test', 'Tests'], ['deploy', 'Publish']] as [Kind, string][]).map(([k, lbl]) => (
              <span key={k} className="pipeflow-leg"><span className="pipeflow-leg-sw" style={{ background: KIND_ACCENT[k] }} />{lbl}</span>
            ))}
          </div>
        </div>

        {/* zoom controls */}
        <div className="pipeflow-zoom">
          <button type="button" onClick={() => zoom(1.2)} aria-label="Zoom in">+</button>
          <button type="button" onClick={() => zoom(1 / 1.2)} aria-label="Zoom out">−</button>
          <button type="button" onClick={fit} aria-label="Fit view">⤢</button>
        </div>

        {/* minimap */}
        <div className="pipeflow-minimap" style={{ width: mmW, height: mmH }}>
          {stages.map((s) => (
            <span
              key={s.id}
              className="pipeflow-mm-node"
              style={{
                left: mOx + (pos[s.id].x - mnX) * mSc,
                top: mOy + (pos[s.id].y - mnY) * mSc,
                width: NODE_W * mSc,
                height: NODE_H * mSc,
                background: STATUS_COLOR[status[s.id]],
              }}
            />
          ))}
          <span className="pipeflow-mm-label">minimap</span>
        </div>

        <div className="pipeflow-hint">drag nodes · drag canvas to pan · scroll to zoom · ▶ replays the run</div>
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
