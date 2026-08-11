'use client';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as RPointerEvent } from 'react';
import type { ParityUpstream } from './ParityData';
import { installCommand, pct, runnerFile, runtimeLabel } from './ParityData';
import './PipelineFlow.css';

// The concrete facts of one harness run, used to label every node with a real
// artefact (a path, a pinned version, a count) instead of a generic caption.
export interface PipelineRun {
  accent: string;
  /** Repo-relative harness dir, e.g. "parity/express" or "parity/express/nested/qs". */
  harnessPath: string;
  upstream: ParityUpstream;
  goModule: string;
  goVersion: string;
  caseCount: number;
  groupCount: number;
  comparedTotal: number;
  match: number;
  mismatch: number;
  deviations: number;
  parityPercent: number;
  symbolCount: number;
  generatedBy: string;
  generatedAt?: string;
}

export interface PipelineFlowProps {
  run: PipelineRun;
  /** Heading level context: the flow renders no heading of its own. */
  label: string;
}

type Status = 'pending' | 'running' | 'done';
type Kind = 'input' | 'runner' | 'compare' | 'publish';

interface Stage {
  id: string;
  code: string;
  label: string;
  sub: string;
  detail: string;
  kind: Kind;
  x: number;
  y: number;
}
interface Edge { from: string; to: string }

const NODE_W = 210;
const NODE_H = 70;

const KIND_ACCENT: Record<Kind, string> = {
  input: '#8b5cf6',
  runner: '#3b82f6',
  compare: '#f59e0b',
  publish: '#10b981',
};
const STATUS_COLOR: Record<Status, string> = {
  pending: '#94a3b8',
  running: '#3b82f6',
  done: '#10b981',
};

// The stages the harness actually executes, per parity/HARNESS.md: install the
// pinned oracle, start both runners, stream every case through both over JSON
// Lines, compare ok-then-value with a normalising deep-equal, classify the
// result, then write parity.json + COVERAGE.md.
export function pipelineStages(run: PipelineRun): Stage[] {
  const eco = run.upstream.ecosystem || 'upstream';
  const runner = `${run.harnessPath || 'parity/<lib>'}/${eco}/${runnerFile(eco)}`;
  const goRunner = `${run.harnessPath || 'parity/<lib>'}/go/run.go`;
  const cases = `${run.harnessPath || 'parity/<lib>'}/cases/*.json`;
  return [
    {
      id: 'install',
      code: 'PIN',
      label: `Install ${run.upstream.raw || 'the pinned upstream'}`,
      sub: installCommand(run.upstream),
      detail: `The oracle is the real published package, pinned to an exact version (${run.upstream.version || 'no version recorded'}). Nothing is transcribed from docs or memory.`,
      kind: 'input',
      x: 20,
      y: 24,
    },
    {
      id: 'cases',
      code: 'CASE',
      label: `${run.groupCount} case file${run.groupCount === 1 ? '' : 's'}`,
      sub: cases,
      detail: `${run.caseCount.toLocaleString()} language-agnostic cases, each naming the upstream symbol and the Go symbol it exercises. Both sides are asked the identical question.`,
      kind: 'input',
      x: 20,
      y: 168,
    },
    {
      id: 'gomod',
      code: 'MOD',
      label: run.goModule || 'the Go module',
      sub: run.goVersion ? `pinned at ${run.goVersion}` : 'measured at HEAD',
      detail: 'The port under measurement, resolved as a normal Go module so the score names an exact released version.',
      kind: 'input',
      x: 20,
      y: 312,
    },
    {
      id: 'upstream',
      code: 'UP',
      label: `${runtimeLabel(eco)} runner`,
      sub: runner,
      detail: `Started once and kept open for the whole suite. It reads one JSON object per line from stdin and writes exactly one per line to stdout — the harness never links against the library, it only talks to it.`,
      kind: 'runner',
      x: 286,
      y: 24,
    },
    {
      id: 'go',
      code: 'GO',
      label: 'Go runner',
      sub: goRunner,
      detail: 'The same JSON-Lines contract, implemented over the Go port. A failing case reports ok:false and the runner keeps reading, so one loss never cascades.',
      kind: 'runner',
      x: 286,
      y: 312,
    },
    {
      id: 'stream',
      code: 'IO',
      label: 'Stream every case to both',
      sub: `${run.caseCount.toLocaleString()} cases · one process each`,
      detail: 'Cases are streamed through the two long-lived subprocesses in order, with a per-case timeout so a hung runner fails that case rather than the suite.',
      kind: 'compare',
      x: 552,
      y: 168,
    },
    {
      id: 'compare',
      code: 'EQ',
      label: 'Compare ok, then value',
      sub: 'normalising deep-equal',
      detail: 'Failure is compared before value: returning a result where upstream throws is not parity. Values are compared with a normalising deep-equal — all JSON numbers as float64, so 1 and 1.0 agree.',
      kind: 'compare',
      x: 818,
      y: 168,
    },
    {
      id: 'classify',
      code: 'CLS',
      label: 'Classify each case',
      sub: `${run.match.toLocaleString()} match · ${run.mismatch.toLocaleString()} mismatch · ${run.deviations.toLocaleString()} deviation`,
      detail: 'A documented, deliberate difference is counted as a deviation, not silently as a pass; everything else that disagrees is a mismatch.',
      kind: 'compare',
      x: 1084,
      y: 168,
    },
    {
      id: 'publish',
      code: 'OUT',
      label: 'Write parity.json + COVERAGE.md',
      sub: `${pct(run.parityPercent, run.comparedTotal)} over ${run.comparedTotal.toLocaleString()} compared · ${run.symbolCount.toLocaleString()} symbols`,
      detail: `Regenerated by ${run.generatedBy || 'the harness test'}: the score, the named failing cases, and the full upstream API inventory. This page reads those files.`,
      kind: 'publish',
      x: 1350,
      y: 168,
    },
  ];
}

const EDGES: Edge[] = [
  { from: 'install', to: 'upstream' },
  { from: 'gomod', to: 'go' },
  { from: 'upstream', to: 'stream' },
  { from: 'cases', to: 'stream' },
  { from: 'go', to: 'stream' },
  { from: 'stream', to: 'compare' },
  { from: 'compare', to: 'classify' },
  { from: 'classify', to: 'publish' },
];
const ORDER = ['install', 'cases', 'gomod', 'upstream', 'go', 'stream', 'compare', 'classify', 'publish'];

function bezier(a: { x: number; y: number }, b: { x: number; y: number }): string {
  const dx = Math.max(45, Math.abs(b.x - a.x) * 0.5);
  return `M${a.x},${a.y} C${a.x + dx},${a.y} ${b.x - dx},${b.y} ${b.x},${b.y}`;
}

// PipelineFlow draws the harness run as a small DAG on a pannable, zoomable
// canvas: inputs on the left (the pinned oracle, the case files, the Go module),
// the two runners, then the compare/classify/publish chain. Every caption is a
// real artefact of THAT library's run. The canvas is decorative — the same
// stages are listed as text below it — so it is hidden from assistive tech and
// the list carries the content.
export function PipelineFlow({ run, label }: PipelineFlowProps) {
  const paneRef = useRef<HTMLDivElement | null>(null);
  const stages = useMemo(() => pipelineStages(run), [run]);

  const [pos, setPos] = useState<Record<string, { x: number; y: number }>>(() =>
    Object.fromEntries(stages.map((s) => [s.id, { x: s.x, y: s.y }])),
  );
  const [view, setView] = useState({ tx: 0, ty: 0, k: 0.62 });
  // Every stage starts 'done': unlike a CI graph, these stages are not being
  // polled — the numbers on the page exist BECAUSE the run completed. "Replay"
  // re-animates the order for anyone who wants to follow the flow.
  const [status, setStatus] = useState<Record<string, Status>>(() =>
    Object.fromEntries(stages.map((s) => [s.id, 'done' as Status])),
  );
  const [running, setRunning] = useState(false);
  const drag = useRef<{ mode: 'pan' | 'node'; id?: string; sx: number; sy: number; ox: number; oy: number } | null>(null);
  const runTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

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

  // Pane-relative -> world coordinates. The pane may not be mounted yet (or may
  // have unmounted mid-drag), so this returns null rather than asserting.
  const world = useCallback(
    (cx: number, cy: number): { x: number; y: number } | null => {
      const pane = paneRef.current;
      if (!pane) return null;
      const r = pane.getBoundingClientRect();
      return { x: (cx - r.left - view.tx) / view.k, y: (cy - r.top - view.ty) / view.k };
    },
    [view.tx, view.ty, view.k],
  );

  const onPaneDown = (e: RPointerEvent) => {
    drag.current = { mode: 'pan', sx: e.clientX, sy: e.clientY, ox: view.tx, oy: view.ty };
  };
  const onNodeDown = (id: string, e: RPointerEvent) => {
    e.stopPropagation();
    const w = world(e.clientX, e.clientY);
    const p = pos[id];
    if (!w || !p) return;
    drag.current = { mode: 'node', id, sx: e.clientX, sy: e.clientY, ox: w.x - p.x, oy: w.y - p.y };
  };
  useEffect(() => {
    const move = (e: PointerEvent) => {
      const d = drag.current;
      if (!d) return;
      if (d.mode === 'pan') {
        setView((v) => ({ ...v, tx: d.ox + (e.clientX - d.sx), ty: d.oy + (e.clientY - d.sy) }));
      } else if (d.mode === 'node' && d.id) {
        const id = d.id;
        const w = world(e.clientX, e.clientY);
        if (!w) return;
        setPos((p) => ({ ...p, [id]: { x: w.x - d.ox, y: w.y - d.oy } }));
      }
    };
    const up = () => { drag.current = null; };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
    return () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
    };
    // `move` closes over the viewport through world(); re-subscribe whenever
    // that identity changes so a drag started after a zoom uses the live scale.
  }, [world]);

  const onWheel = (e: React.WheelEvent) => {
    const pane = paneRef.current;
    if (!pane) return;
    e.preventDefault();
    const r = pane.getBoundingClientRect();
    const sx = e.clientX - r.left, sy = e.clientY - r.top;
    setView((v) => {
      const nk = Math.min(2, Math.max(0.3, v.k * (e.deltaY < 0 ? 1.12 : 0.89)));
      return { k: nk, tx: sx - (sx - v.tx) * (nk / v.k), ty: sy - (sy - v.ty) * (nk / v.k) };
    });
  };
  const zoom = (f: number) => {
    const pane = paneRef.current;
    if (!pane) return;
    const r = pane.getBoundingClientRect();
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
        setStatus((s) => ({ ...s, [id]: 'done' }));
        i++;
        step();
      }, 380);
    };
    runTimer.current = setTimeout(step, 120);
  };
  useEffect(() => () => clearTimeout(runTimer.current), []);

  // Positions come from drag state; fall back to the stage's declared layout so
  // a stage that has not been placed yet still renders in the right spot.
  const at = (id: string) => pos[id] ?? stages.find((s) => s.id === id) ?? { x: 0, y: 0 };
  const out = (id: string) => ({ x: at(id).x + NODE_W, y: at(id).y + NODE_H / 2 });
  const inp = (id: string) => ({ x: at(id).x, y: at(id).y + NODE_H / 2 });

  // minimap
  const mmW = 150, mmH = 92, mmPad = 8;
  const xs = stages.map((s) => at(s.id).x), ys = stages.map((s) => at(s.id).y);
  const mnX = Math.min(...xs), mxX = Math.max(...xs) + NODE_W, mnY = Math.min(...ys), mxY = Math.max(...ys) + NODE_H;
  const mSc = Math.min((mmW - mmPad * 2) / (mxX - mnX || 1), (mmH - mmPad * 2) / (mxY - mnY || 1));
  const mOx = (mmW - (mxX - mnX) * mSc) / 2, mOy = (mmH - (mxY - mnY) * mSc) / 2;

  return (
    <div className="pipeflow" style={{ '--pf-accent': run.accent } as CSSProperties}>
      <div
        ref={paneRef}
        className="pipeflow-pane"
        onPointerDown={onPaneDown}
        onWheel={onWheel}
        aria-hidden="true"
      >
        <div className="pipeflow-bg" style={{ backgroundPosition: `${view.tx}px ${view.ty}px`, backgroundSize: `${22 * view.k}px ${22 * view.k}px` }} />
        <div className="pipeflow-world" style={{ transform: `translate(${view.tx}px, ${view.ty}px) scale(${view.k})` }}>
          <svg className="pipeflow-edges">
            {EDGES.map((e) => {
              const a = status[e.from], b = status[e.to];
              const flowing = b === 'running';
              const done = a === 'done' && (b === 'done' || b === 'running');
              const fromKind = stages.find((s) => s.id === e.from)?.kind;
              const stroke = flowing ? run.accent : done && fromKind ? KIND_ACCENT[fromKind] : 'var(--edge-2)';
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
            const st = status[s.id] ?? 'done';
            const p = at(s.id);
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
                  <span style={{ color: STATUS_COLOR[st] }}>{st === 'done' ? 'ran' : st}</span>
                </div>
                <span className="pipeflow-handle in" />
                <span className="pipeflow-handle out" style={{ background: run.accent }} />
              </div>
            );
          })}
        </div>

        {/* control panel */}
        <div className="pipeflow-panel pipeflow-tl">
          <div className="pipeflow-panel-hd">
            <span className="pipeflow-panel-ico" style={{ background: run.accent }}>=</span>
            <span className="pipeflow-panel-title">{label}</span>
          </div>
          <div className="pipeflow-panel-sub pf-ok">
            <span className="pipeflow-status-dot" />{' '}
            {running ? 'replaying the run' : `${stages.length} stages · ${run.caseCount.toLocaleString()} cases`}
            {run.generatedAt ? ` · ${run.generatedAt.slice(0, 10)}` : ''}
          </div>
          <div className="pipeflow-panel-btns">
            <button type="button" className="pipeflow-run" onClick={replay} disabled={running}>
              {running ? 'Replaying' : 'Replay'}
            </button>
          </div>
          <div className="pipeflow-legend2">
            {([['input', 'Inputs'], ['runner', 'Runners'], ['compare', 'Compare'], ['publish', 'Publish']] as [Kind, string][]).map(([k, lbl]) => (
              <span key={k} className="pipeflow-leg"><span className="pipeflow-leg-sw" style={{ background: KIND_ACCENT[k] }} />{lbl}</span>
            ))}
          </div>
        </div>

        {/* zoom controls */}
        <div className="pipeflow-zoom">
          <button type="button" onClick={() => zoom(1.2)} aria-label="Zoom in">+</button>
          <button type="button" onClick={() => zoom(1 / 1.2)} aria-label="Zoom out">-</button>
          <button type="button" onClick={fit} aria-label="Fit view">fit</button>
        </div>

        {/* minimap */}
        <div className="pipeflow-minimap" style={{ width: mmW, height: mmH }}>
          {stages.map((s) => (
            <span
              key={s.id}
              className="pipeflow-mm-node"
              style={{
                left: mOx + (at(s.id).x - mnX) * mSc,
                top: mOy + (at(s.id).y - mnY) * mSc,
                width: NODE_W * mSc,
                height: NODE_H * mSc,
                background: STATUS_COLOR[status[s.id]],
              }}
            />
          ))}
          <span className="pipeflow-mm-label">minimap</span>
        </div>

        <div className="pipeflow-hint">drag nodes · drag canvas to pan · scroll to zoom · Replay re-animates the order</div>
      </div>

      {/* The same stages as text: this is the accessible version of the canvas
          above, and it carries the per-stage explanation. */}
      <ol className="pipeflow-steps">
        {stages.map((s, i) => (
          <li key={s.id}>
            <span className="pipeflow-steps-n" style={{ background: KIND_ACCENT[s.kind] }}>{i + 1}</span>
            <div>
              <b>{s.label}</b>
              <div className="pipeflow-steps-sub">{s.sub}</div>
              <div className="pipeflow-steps-detail">{s.detail}</div>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
