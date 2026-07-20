import { useEffect, useMemo, useRef, useState } from 'react';
import type { PointerEvent as ReactPointerEvent, WheelEvent as ReactWheelEvent } from 'react';
import { useRouter } from 'next/navigation';
import { LIBS } from '../data';
import { SecH } from './SecH';
import {
  hasApi,
  loadFallbackGraph,
  search,
} from '../api/graph';
import type {
  GraphData,
  GraphEdge,
  GraphPackage,
  SearchBackend,
  SearchHit,
  SymbolKind,
} from '../api/graph';

// ---------------------------------------------------------------------------
// Tunables — keep the visualization bounded and fast.
// ---------------------------------------------------------------------------
const MAX_NODES = 60; // cap packages drawn for one library's subgraph
const RING_SIZE = 18; // packages per concentric ring
const DETAIL_CAP = 8; // neighbours listed in the detail panel

// Colour per edge kind — shared with the legend.
const KIND_COLOR: Record<GraphEdge['kind'], string> = {
  'same-library': 'var(--edge-2)',
  reference: '#a855f7',
  import: '#3b82f6',
  'shared-upstream': '#10b981',
};

// Colour per symbol kind for the search-result chips.
const SYMBOL_COLOR: Record<SymbolKind, string> = {
  package: '#2f9bff',
  type: '#a855f7',
  interface: '#8b5cf6',
  func: '#10b981',
  method: '#22c55e',
  const: '#f59e0b',
  var: '#ef4444',
};

const ACCENT_BY_LIB: Record<string, string> = Object.fromEntries(
  LIBS.map((l) => [l.id, l.accent]),
);
const accentFor = (lib: string): string => ACCENT_BY_LIB[lib] || 'var(--accent)';

interface XY {
  x: number;
  y: number;
}
interface Subgraph {
  root: GraphPackage | null;
  nodes: GraphPackage[];
  edges: GraphEdge[];
  pos: Record<string, XY>;
  total: number;
  maxR: number;
}
interface Neighbours {
  imports: GraphPackage[];
  importedBy: GraphPackage[];
  related: { pkg: GraphPackage; kind: GraphEdge['kind']; weight: number }[];
}

// The "root" package of a library is its module root: the shortest importPath,
// which the generator wires all same-library edges into (a star).
function rootOf(pkgs: GraphPackage[]): GraphPackage | null {
  if (!pkgs.length) return null;
  return pkgs.reduce((best, p) => (p.importPath.length < best.importPath.length ? p : best), pkgs[0]);
}

// Build a bounded, radially-laid-out subgraph for one library.
function buildSubgraph(graph: GraphData | null, library: string): Subgraph {
  const empty: Subgraph = { root: null, nodes: [], edges: [], pos: {}, total: 0, maxR: 0 };
  if (!graph || !library) return empty;

  const all = graph.packages.filter((p) => p.library === library);
  const total = all.length;
  const root = rootOf(all);

  // Keep the root plus the highest-symbol-count packages, capped.
  const rest = all
    .filter((p) => p.id !== root?.id)
    .sort((a, b) => b.symbolCount - a.symbolCount);
  const kept = root ? [root, ...rest.slice(0, MAX_NODES - 1)] : rest.slice(0, MAX_NODES);
  const keptIds = new Set(kept.map((p) => p.id));

  const edges = graph.edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target));

  // Radial layout centred at the origin: root at centre, others on rings.
  const pos: Record<string, XY> = {};
  let maxR = 0;
  if (root) pos[root.id] = { x: 0, y: 0 };
  const leaves = kept.filter((p) => p.id !== root?.id);
  leaves.forEach((p, i) => {
    const ring = Math.floor(i / RING_SIZE);
    const idxInRing = i % RING_SIZE;
    const countInRing = Math.min(RING_SIZE, leaves.length - ring * RING_SIZE);
    const r = 160 + ring * 135;
    const ang = (idxInRing / countInRing) * Math.PI * 2 + ring * 0.4;
    pos[p.id] = { x: r * Math.cos(ang), y: r * Math.sin(ang) };
    if (r > maxR) maxR = r;
  });

  return { root, nodes: kept, edges, pos, total, maxR };
}

// Resolve a package's neighbours from the full graph (accurate, cheap enough).
function neighboursOf(graph: GraphData | null, id: string): Neighbours {
  const empty: Neighbours = { imports: [], importedBy: [], related: [] };
  if (!graph || !id) return empty;

  const byId = new Map(graph.packages.map((p) => [p.id, p]));
  const outKinds = new Set<GraphEdge['kind']>(['import', 'reference', 'same-library']);
  const imports: GraphPackage[] = [];
  const importedBy: GraphPackage[] = [];
  const related: Neighbours['related'] = [];
  const seenImp = new Set<string>();
  const seenBy = new Set<string>();

  for (const e of graph.edges) {
    if (e.source === id) {
      const to = byId.get(e.target);
      if (to) {
        related.push({ pkg: to, kind: e.kind, weight: e.weight });
        if (outKinds.has(e.kind) && !seenImp.has(to.id)) {
          seenImp.add(to.id);
          imports.push(to);
        }
      }
    } else if (e.target === id) {
      const from = byId.get(e.source);
      if (from) {
        related.push({ pkg: from, kind: e.kind, weight: e.weight });
        if (!seenBy.has(from.id)) {
          seenBy.add(from.id);
          importedBy.push(from);
        }
      }
    }
  }

  related.sort((a, b) => b.weight - a.weight);
  return {
    imports: imports.slice(0, DETAIL_CAP),
    importedBy: importedBy.slice(0, DETAIL_CAP),
    related: related.slice(0, DETAIL_CAP),
  };
}

const shortName = (p: GraphPackage): string => p.name || p.importPath.split('/').pop() || p.importPath;

// ---------------------------------------------------------------------------
// Explore — the search + package-graph tab.
// ---------------------------------------------------------------------------
export function Explore() {
  const router = useRouter();
  const [graph, setGraph] = useState<GraphData | null>(null);
  const [live, setLive] = useState<boolean | null>(null);
  const [lib, setLib] = useState('');

  // Search state.
  const [q, setQ] = useState('');
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [backend, setBackend] = useState<SearchBackend | null>(null);

  // Graph interaction state.
  const [selected, setSelected] = useState<string | null>(null);
  const paneRef = useRef<HTMLDivElement | null>(null);
  const [view, setView] = useState({ tx: 0, ty: 0, k: 1 });
  const drag = useRef<{ sx: number; sy: number; ox: number; oy: number } | null>(null);
  const reqId = useRef(0);

  // Load bundled graph + probe the API once.
  useEffect(() => {
    let alive = true;
    loadFallbackGraph().then((g) => {
      if (!alive) return;
      setGraph(g);
      if (g && g.libraries.length) {
        const preferred = g.libraries.find((l) => l.id === 'express');
        setLib((prev) => prev || (preferred ? preferred.id : g.libraries[0].id));
      }
    });
    hasApi().then((ok) => alive && setLive(ok));
    return () => {
      alive = false;
    };
  }, []);

  // Debounced unified search.
  useEffect(() => {
    const query = q.trim();
    if (!query) {
      setHits([]);
      setBackend(null);
      setSearching(false);
      return;
    }
    setSearching(true);
    const id = ++reqId.current;
    const t = setTimeout(() => {
      search(query, 24).then((res) => {
        if (id !== reqId.current) return; // a newer query superseded this one
        setHits(res.hits);
        setBackend(res.backend);
        setSearching(false);
      });
    }, 180);
    return () => clearTimeout(t);
  }, [q]);

  const sub = useMemo(() => buildSubgraph(graph, lib), [graph, lib]);
  const neighbours = useMemo(
    () => (selected ? neighboursOf(graph, selected) : null),
    [graph, selected],
  );
  const selectedPkg = useMemo(
    () => (selected ? graph?.packages.find((p) => p.id === selected) ?? null : null),
    [selected, graph],
  );

  // Reset pan/zoom and selection whenever the library (and its layout) changes.
  useEffect(() => {
    setSelected(null);
    const pane = paneRef.current;
    const w = pane?.clientWidth ?? 720;
    const h = pane?.clientHeight ?? 420;
    const fit = sub.maxR > 0 ? Math.min(1, (Math.min(w, h) / 2 - 44) / sub.maxR) : 1;
    setView({ tx: w / 2, ty: h / 2, k: fit });
  }, [sub]);

  const accent = accentFor(lib);

  // --- pan / zoom handlers ---
  const onPaneDown = (e: ReactPointerEvent) => {
    if ((e.target as HTMLElement).closest('.xpl-node')) return; // node clicks handled separately
    drag.current = { sx: e.clientX, sy: e.clientY, ox: view.tx, oy: view.ty };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onPaneMove = (e: ReactPointerEvent) => {
    const d = drag.current;
    if (!d) return;
    setView((v) => ({ ...v, tx: d.ox + (e.clientX - d.sx), ty: d.oy + (e.clientY - d.sy) }));
  };
  const onPaneUp = (e: ReactPointerEvent) => {
    drag.current = null;
    try {
      (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    } catch {
      /* pointer already released */
    }
  };
  const onWheel = (e: ReactWheelEvent) => {
    const pane = paneRef.current;
    if (!pane) return;
    const r = pane.getBoundingClientRect();
    const mx = e.clientX - r.left;
    const my = e.clientY - r.top;
    setView((v) => {
      const k = Math.min(2.4, Math.max(0.2, v.k * (e.deltaY < 0 ? 1.1 : 1 / 1.1)));
      // zoom toward the cursor
      const tx = mx - ((mx - v.tx) * k) / v.k;
      const ty = my - ((my - v.ty) * k) / v.k;
      return { tx, ty, k };
    });
  };
  const zoom = (f: number) =>
    setView((v) => ({ ...v, k: Math.min(2.4, Math.max(0.2, v.k * f)) }));

  const goLibrary = (library: string) => {
    if (library) router.push('/lib/' + encodeURIComponent(library));
  };

  const highlight = (id: string): boolean => {
    if (!selected) return false;
    if (id === selected) return true;
    return sub.edges.some(
      (e) =>
        (e.source === selected && e.target === id) ||
        (e.target === selected && e.source === id),
    );
  };

  return (
    <section className="view active" id="view-explore">
      <SecH h="h2">Explore</SecH>
      <p className="muted">
        Search every exported symbol across all ports, and browse how each library's packages connect. Search is served
        by the live GraphQL + Elasticsearch API when it's deployed, and falls back to the data bundled with this site
        everywhere else.
      </p>

      {/* mode badge */}
      <div className="xpl-mode">
        {live === null ? (
          <span className="xpl-badge xpl-badge-idle">
            <span className="xpl-dot" /> checking API…
          </span>
        ) : live ? (
          <span className="xpl-badge xpl-badge-live">
            <span className="xpl-dot" /> Live API — GraphQL + Elasticsearch
          </span>
        ) : (
          <span className="xpl-badge xpl-badge-fallback">
            <span className="xpl-dot" /> Bundled fallback — offline index
          </span>
        )}
      </div>

      {/* search */}
      <div className="xpl-search">
        <span className="xpl-search-ico" aria-hidden>
          ⌕
        </span>
        <input
          className="xpl-search-input"
          type="search"
          placeholder="Search symbols — types, funcs, methods, packages…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          spellCheck={false}
          autoComplete="off"
        />
        {q && (
          <button className="xpl-search-clear" onClick={() => setQ('')} aria-label="Clear search">
            ×
          </button>
        )}
      </div>

      {q.trim() && (
        <div className="xpl-results">
          <div className="xpl-results-hd">
            {searching ? (
              'Searching…'
            ) : (
              <>
                {hits.length} result{hits.length === 1 ? '' : 's'}
                {backend && (
                  <span className="xpl-results-backend">
                    via {backend === 'elasticsearch' ? 'Elasticsearch' : backend === 'memory' ? 'in-memory BM25' : 'bundled index'}
                  </span>
                )}
              </>
            )}
          </div>
          {!searching && hits.length === 0 && (
            <div className="xpl-empty">No symbols match “{q.trim()}”.</div>
          )}
          <ul className="xpl-hitlist">
            {hits.map((h) => (
              <li key={h.id}>
                <button className="xpl-hit" onClick={() => goLibrary(h.library)} title={`Go to ${h.library}`}>
                  <span className="xpl-hit-name">{h.name}</span>
                  <span
                    className="xpl-chip"
                    style={{ background: (SYMBOL_COLOR[h.kind] || 'var(--fg-dim)') + '22', color: SYMBOL_COLOR[h.kind] || 'var(--fg-muted)' }}
                  >
                    {h.kind}
                  </span>
                  <span className="xpl-hit-pkg">{h.packageImportPath}</span>
                  <span className="xpl-hit-lib" style={{ color: accentFor(h.library) }}>
                    {h.library} →
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* library picker */}
      <div className="xpl-picker">
        <label htmlFor="xpl-lib">Library graph</label>
        <select id="xpl-lib" value={lib} onChange={(e) => setLib(e.target.value)}>
          {(graph?.libraries ?? []).map((l) => (
            <option key={l.id} value={l.id}>
              {l.name} — {l.packageCount} pkg{l.packageCount === 1 ? '' : 's'}
            </option>
          ))}
        </select>
        {sub.total > sub.nodes.length && (
          <span className="xpl-cap">
            showing {sub.nodes.length} of {sub.total} packages
          </span>
        )}
      </div>

      {/* graph pane */}
      <div
        className="xpl-pane"
        ref={paneRef}
        onPointerDown={onPaneDown}
        onPointerMove={onPaneMove}
        onPointerUp={onPaneUp}
        onPointerCancel={onPaneUp}
        onWheel={onWheel}
      >
        <div className="xpl-bg" />
        <div
          className="xpl-world"
          style={{ transform: `translate(${view.tx}px, ${view.ty}px) scale(${view.k})` }}
        >
          <svg className="xpl-edges" width="1" height="1">
            {sub.edges.map((e, i) => {
              const a = sub.pos[e.source];
              const b = sub.pos[e.target];
              if (!a || !b) return null;
              const on = selected && (e.source === selected || e.target === selected);
              return (
                <line
                  key={i}
                  x1={a.x}
                  y1={a.y}
                  x2={b.x}
                  y2={b.y}
                  stroke={on ? accent : KIND_COLOR[e.kind]}
                  strokeWidth={on ? 2 : 1}
                  strokeOpacity={selected ? (on ? 0.95 : 0.12) : 0.5}
                />
              );
            })}
          </svg>

          {sub.nodes.map((n) => {
            const p = sub.pos[n.id];
            if (!p) return null;
            const isRoot = n.id === sub.root?.id;
            const active = selected === n.id;
            const dim = selected && !highlight(n.id);
            return (
              <button
                key={n.id}
                className={`xpl-node${isRoot ? ' is-root' : ''}${active ? ' is-active' : ''}${dim ? ' is-dim' : ''}`}
                style={{
                  left: p.x,
                  top: p.y,
                  borderColor: active || isRoot ? accent : 'var(--edge)',
                  boxShadow: active ? `0 0 0 2px ${accent}, 0 6px 18px rgba(0,0,0,.28)` : undefined,
                }}
                onClick={() => setSelected(n.id)}
                onMouseEnter={() => setSelected(n.id)}
                title={n.importPath}
              >
                <span className="xpl-node-dot" style={{ background: accent }} />
                <span className="xpl-node-label">{shortName(n)}</span>
                {n.symbolCount > 0 && <span className="xpl-node-count">{n.symbolCount}</span>}
              </button>
            );
          })}
        </div>

        {/* detail panel */}
        {selectedPkg && neighbours && (
          <div className="xpl-detail">
            <div className="xpl-detail-hd">
              <span className="xpl-node-dot" style={{ background: accent }} />
              <span className="xpl-detail-name">{shortName(selectedPkg)}</span>
              <button className="xpl-detail-x" onClick={() => setSelected(null)} aria-label="Close">
                ×
              </button>
            </div>
            <div className="xpl-detail-path">{selectedPkg.importPath}</div>
            {selectedPkg.synopsis && <div className="xpl-detail-syn">{selectedPkg.synopsis}</div>}
            <div className="xpl-detail-meta">
              <span>{selectedPkg.symbolCount} symbols</span>
              <span>·</span>
              <button className="xpl-detail-golib" onClick={() => goLibrary(selectedPkg.library)}>
                open {selectedPkg.library} →
              </button>
            </div>
            <DetailList title="Imports / references" items={neighbours.imports} onPick={setSelected} />
            <DetailList title="Imported by" items={neighbours.importedBy} onPick={setSelected} />
          </div>
        )}

        {/* zoom controls */}
        <div className="xpl-zoom">
          <button onClick={() => zoom(1.2)} aria-label="Zoom in">
            +
          </button>
          <button onClick={() => zoom(1 / 1.2)} aria-label="Zoom out">
            −
          </button>
        </div>

        {/* legend */}
        <div className="xpl-legend">
          {(Object.keys(KIND_COLOR) as GraphEdge['kind'][]).map((k) => (
            <span key={k} className="xpl-leg">
              <span className="xpl-leg-sw" style={{ background: KIND_COLOR[k] }} />
              {k}
            </span>
          ))}
        </div>

        <div className="xpl-hint">drag to pan · scroll to zoom · hover a node for its connections</div>
      </div>

      <style>{styles}</style>
    </section>
  );
}

function DetailList({
  title,
  items,
  onPick,
}: {
  title: string;
  items: GraphPackage[];
  onPick: (id: string) => void;
}) {
  if (!items.length) return null;
  return (
    <div className="xpl-detail-sec">
      <div className="xpl-detail-sec-t">{title}</div>
      <div className="xpl-detail-chips">
        {items.map((p) => (
          <button key={p.id} className="xpl-nchip" onClick={() => onPick(p.id)} title={p.importPath}>
            {shortName(p)}
          </button>
        ))}
      </div>
    </div>
  );
}

const styles = `
#view-explore .xpl-mode { margin: .2rem 0 1rem; }
.xpl-badge {
  display: inline-flex; align-items: center; gap: .5rem;
  padding: .32rem .7rem; border-radius: 999px; font-size: .78rem; font-weight: 600;
  border: 1px solid var(--edge); background: var(--glass); backdrop-filter: var(--blur); box-shadow: var(--hi);
}
.xpl-badge .xpl-dot { width: 8px; height: 8px; border-radius: 50%; }
.xpl-badge-live { color: var(--fg); }
.xpl-badge-live .xpl-dot { background: #10b981; box-shadow: 0 0 8px #10b981; }
.xpl-badge-fallback { color: var(--fg-muted); }
.xpl-badge-fallback .xpl-dot { background: #f59e0b; box-shadow: 0 0 8px #f59e0b; }
.xpl-badge-idle { color: var(--fg-dim); }
.xpl-badge-idle .xpl-dot { background: var(--fg-dim); animation: xpl-pulse 1.1s ease-in-out infinite; }

.xpl-search {
  position: relative; display: flex; align-items: center; margin: .2rem 0 .4rem;
  border: 1px solid var(--edge); border-radius: 14px; background: var(--glass);
  backdrop-filter: var(--blur); box-shadow: var(--hi); transition: border-color .15s, box-shadow .15s;
}
.xpl-search:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 22%, transparent); }
.xpl-search-ico { padding: 0 .1rem 0 .9rem; color: var(--fg-dim); font-size: 1.15rem; }
.xpl-search-input {
  flex: 1; border: none; background: transparent; color: var(--fg);
  font: inherit; font-size: 1.02rem; padding: .85rem .7rem; outline: none;
}
.xpl-search-input::placeholder { color: var(--fg-dim); }
.xpl-search-clear {
  border: none; background: transparent; color: var(--fg-dim); cursor: pointer;
  font-size: 1.3rem; line-height: 1; padding: 0 .9rem;
}
.xpl-search-clear:hover { color: var(--fg); }

.xpl-results { margin: .3rem 0 1.4rem; }
.xpl-results-hd { font-size: .8rem; color: var(--fg-dim); margin: .2rem 0 .5rem; display: flex; gap: .5rem; align-items: baseline; }
.xpl-results-backend { color: var(--fg-muted); font-family: "SF Mono", ui-monospace, monospace; font-size: .74rem; }
.xpl-empty { color: var(--fg-muted); font-size: .9rem; padding: .4rem 0; }
.xpl-hitlist { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: .3rem; }
.xpl-hit {
  width: 100%; display: flex; align-items: center; gap: .6rem; text-align: left;
  padding: .55rem .8rem; border-radius: 11px; border: 1px solid var(--edge);
  background: var(--glass); color: var(--fg); font: inherit; cursor: pointer;
  transition: border-color .12s, background .12s, transform .12s;
}
.xpl-hit:hover { border-color: var(--edge-2); background: var(--glass-2); transform: translateX(2px); }
.xpl-hit-name { font-weight: 600; font-family: "SF Mono", ui-monospace, monospace; font-size: .92rem; flex: none; }
.xpl-chip { flex: none; padding: .1rem .5rem; border-radius: 999px; font-size: .68rem; font-weight: 700; text-transform: uppercase; letter-spacing: .03em; }
.xpl-hit-pkg { flex: 1; min-width: 0; color: var(--fg-dim); font-size: .78rem; font-family: "SF Mono", ui-monospace, monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.xpl-hit-lib { flex: none; font-size: .8rem; font-weight: 600; }

.xpl-picker { display: flex; align-items: center; gap: .6rem; margin: .4rem 0 .9rem; flex-wrap: wrap; }
.xpl-picker label { font-size: .82rem; color: var(--fg-muted); font-weight: 600; }
.xpl-picker select {
  padding: .45rem .7rem; border-radius: 10px; border: 1px solid var(--edge);
  background: var(--glass); color: var(--fg); font: inherit; font-size: .9rem;
  backdrop-filter: var(--blur); box-shadow: var(--hi); cursor: pointer;
}
.xpl-cap { font-size: .76rem; color: var(--fg-dim); font-family: "SF Mono", ui-monospace, monospace; }

.xpl-pane {
  position: relative; width: 100%; height: 460px; border-radius: 16px;
  border: 1px solid var(--edge); background: var(--code-bg); overflow: hidden;
  cursor: grab; touch-action: none; box-shadow: var(--hi);
}
.xpl-pane:active { cursor: grabbing; }
.xpl-bg {
  position: absolute; inset: 0; pointer-events: none;
  background-image: radial-gradient(circle, var(--edge-2) 1.2px, transparent 1.2px);
  background-size: 26px 26px; opacity: .5;
}
.xpl-world { position: absolute; top: 0; left: 0; transform-origin: 0 0; }
.xpl-edges { position: absolute; top: 0; left: 0; overflow: visible; pointer-events: none; }

.xpl-node {
  position: absolute; transform: translate(-50%, -50%);
  display: inline-flex; align-items: center; gap: .4rem;
  padding: .34rem .6rem; border-radius: 10px; border: 1px solid var(--edge);
  background: var(--glass-2); backdrop-filter: var(--blur); color: var(--fg);
  font: inherit; font-size: .78rem; font-weight: 600; white-space: nowrap; cursor: pointer;
  box-shadow: 0 2px 10px rgba(0,0,0,.14); transition: opacity .12s, transform .08s, box-shadow .12s;
}
.xpl-node:hover { transform: translate(-50%, -50%) scale(1.04); z-index: 3; }
.xpl-node.is-root { font-size: .86rem; padding: .45rem .75rem; border-width: 2px; z-index: 2; }
.xpl-node.is-active { z-index: 4; }
.xpl-node.is-dim { opacity: .28; }
.xpl-node-dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
.xpl-node-label { max-width: 160px; overflow: hidden; text-overflow: ellipsis; }
.xpl-node-count { font-size: .64rem; color: var(--fg-dim); font-family: "SF Mono", ui-monospace, monospace; }

.xpl-detail {
  position: absolute; top: 14px; right: 14px; z-index: 6; width: 260px; max-width: calc(100% - 28px);
  background: var(--glass); border: 1px solid var(--edge); border-radius: 12px;
  padding: 12px 14px; box-shadow: 0 6px 24px rgba(0,0,0,.2); backdrop-filter: var(--blur);
}
.xpl-detail-hd { display: flex; align-items: center; gap: .5rem; }
.xpl-detail-name { font-weight: 700; font-size: .95rem; color: var(--fg); font-family: "SF Mono", ui-monospace, monospace; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.xpl-detail-x { border: none; background: transparent; color: var(--fg-dim); font-size: 1.3rem; line-height: 1; cursor: pointer; }
.xpl-detail-x:hover { color: var(--fg); }
.xpl-detail-path { font-size: .72rem; color: var(--fg-dim); font-family: "SF Mono", ui-monospace, monospace; margin: .3rem 0; word-break: break-all; }
.xpl-detail-syn { font-size: .8rem; color: var(--fg-muted); line-height: 1.5; margin: .3rem 0 .5rem; }
.xpl-detail-meta { display: flex; align-items: center; gap: .5rem; font-size: .76rem; color: var(--fg-dim); margin-bottom: .5rem; }
.xpl-detail-golib { border: none; background: transparent; color: var(--accent); font: inherit; font-size: .76rem; font-weight: 600; cursor: pointer; padding: 0; }
.xpl-detail-golib:hover { text-decoration: underline; }
.xpl-detail-sec { margin-top: .6rem; }
.xpl-detail-sec-t { font-size: .68rem; text-transform: uppercase; letter-spacing: .05em; color: var(--fg-dim); margin-bottom: .35rem; }
.xpl-detail-chips { display: flex; flex-wrap: wrap; gap: .3rem; }
.xpl-nchip {
  border: 1px solid var(--edge); background: var(--glass-2); color: var(--fg-muted);
  border-radius: 999px; padding: .18rem .55rem; font-size: .72rem; cursor: pointer; font: inherit;
  max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.xpl-nchip:hover { color: var(--fg); border-color: var(--edge-2); }

.xpl-zoom {
  position: absolute; bottom: 14px; left: 14px; z-index: 5; display: flex; flex-direction: column;
  border: 1px solid var(--edge); border-radius: 10px; overflow: hidden; background: var(--glass);
  backdrop-filter: var(--blur); box-shadow: 0 6px 24px rgba(0,0,0,.16);
}
.xpl-zoom button { width: 32px; height: 32px; border: none; background: transparent; color: var(--fg); cursor: pointer; font-size: 16px; display: grid; place-items: center; }
.xpl-zoom button:first-child { border-bottom: 1px solid var(--edge); }
.xpl-zoom button:hover { background: var(--glass-2); }

.xpl-legend {
  position: absolute; top: 14px; left: 14px; z-index: 5; display: flex; flex-wrap: wrap; gap: .4rem .7rem;
  padding: .5rem .7rem; border-radius: 10px; border: 1px solid var(--edge); background: var(--glass);
  backdrop-filter: var(--blur); box-shadow: var(--hi); max-width: 60%;
}
.xpl-leg { display: inline-flex; align-items: center; gap: .35rem; font-size: .7rem; color: var(--fg-muted); }
.xpl-leg-sw { width: 12px; height: 3px; border-radius: 2px; }

.xpl-hint {
  position: absolute; bottom: 14px; left: 50%; transform: translateX(-50%); z-index: 5;
  font-size: .7rem; color: var(--fg-muted); font-family: "SF Mono", ui-monospace, monospace;
  background: var(--glass); border: 1px solid var(--edge); padding: 4px 11px; border-radius: 20px;
  box-shadow: 0 4px 16px rgba(0,0,0,.12); white-space: nowrap;
}
@media (max-width: 640px) { .xpl-hint, .xpl-legend { display: none; } .xpl-detail { width: 210px; } }

@keyframes xpl-pulse { 0%, 100% { opacity: 1; } 50% { opacity: .35; } }
@media (prefers-reduced-motion: reduce) { .xpl-badge-idle .xpl-dot { animation: none; } }
`;
