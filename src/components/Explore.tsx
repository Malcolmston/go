import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent, WheelEvent as ReactWheelEvent } from 'react';
import { useRouter } from 'next/navigation';
import { LIBS } from '../data';
import { withBase } from '../basePath';
import { SecH } from './SecH';
import {
  hasApi,
  loadFallbackGraph,
  search,
} from '../api/graph';
import type {
  EdgeKind,
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
const NEAR_CAP = 10; // neighbours listed in the left panel
const HIT_CAP = 24; // search hits requested / rendered
const RING_CAP = 3; // hop rings that get their own shading; beyond this a node dims
const PARITY_MAX_BYTES = 512 * 1024; // refuse an oversized parity payload (see below)

// Layout of the single all-libraries graph. Each library becomes a cluster:
// its module root sits at the cluster centre with the rest of its packages on
// concentric rings, and the clusters themselves are packed into rings around
// the origin, largest first. Ring capacity grows with the ring index so a big
// library stays compact instead of stretching into one enormous circle.
const NODE_RING_GAP = 48; // distance between concentric rings inside a cluster
const NODE_RING_BASE = 6; // packages on the innermost ring; ring k holds k*this
const CLUSTER_GAP = 56; // clear space between neighbouring clusters
const LABEL_ZOOM = 0.55; // below this scale nodes render as bare dots

// Colour per edge kind — shared with the legend.
const KIND_COLOR: Record<EdgeKind, string> = {
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

// Shading per hop ring, used by both the nodes and the distance legend.
const RING_COLOR = ['var(--accent)', '#f59e0b', '#8b5cf6', '#64748b'];

// Which edge kinds each distance mode walks. This is the whole definition of
// "distance" in this view, and the UI states it verbatim next to the number.
const DEP_KINDS: EdgeKind[] = ['import', 'reference', 'shared-upstream'];
const ALL_KINDS: EdgeKind[] = ['same-library', ...DEP_KINDS];
type DistMode = 'all' | 'deps';
const MODE_KINDS: Record<DistMode, EdgeKind[]> = { all: ALL_KINDS, deps: DEP_KINDS };
const MODE_LABEL: Record<DistMode, string> = { all: 'all edges', deps: 'dependency edges' };

const ACCENT_BY_LIB: Record<string, string> = Object.fromEntries(
  LIBS.map((l) => [l.id, l.accent]),
);
const accentFor = (lib: string): string => ACCENT_BY_LIB[lib] || 'var(--accent)';

interface XY {
  x: number;
  y: number;
}
interface Cluster {
  id: string; // library id
  x: number;
  y: number;
  r: number;
  count: number;
}
interface FullGraph {
  nodes: GraphPackage[];
  edges: GraphEdge[];
  pos: Record<string, XY>;
  roots: Set<string>;
  clusters: Cluster[];
  clusterById: Map<string, Cluster>;
  byId: Map<string, GraphPackage>;
  extent: number; // furthest node from the origin, for fit-to-view
}
/** One undirected adjacency entry. `dir` records the original edge direction. */
interface Link {
  id: string;
  kind: EdgeKind;
  weight: number;
  dir: 'out' | 'in';
}
/** A neighbour of the selection, annotated with its hop distance. */
interface NearNode {
  pkg: GraphPackage;
  hops: number;
  kind: EdgeKind | null; // the edge kind at the first hop, when adjacent
  dir: 'out' | 'in' | null;
}

// The "root" package of a library is its module root: the shortest importPath,
// which the generator wires all same-library edges into (a star).
function rootOf(pkgs: GraphPackage[]): GraphPackage | null {
  if (!pkgs.length) return null;
  return pkgs.reduce((best, p) => (p.importPath.length < best.importPath.length ? p : best), pkgs[0]);
}

// Lay a library's packages out around its own centre: root in the middle, the
// rest on concentric rings. Returns local offsets and the radius they occupy.
function layoutCluster(pkgs: GraphPackage[]): { local: Record<string, XY>; r: number } {
  const local: Record<string, XY> = {};
  const root = rootOf(pkgs);
  if (root) local[root.id] = { x: 0, y: 0 };
  const leaves = pkgs.filter((p) => p.id !== root?.id);

  let i = 0;
  let ring = 1;
  let r = 0;
  while (i < leaves.length) {
    const capacity = ring * NODE_RING_BASE;
    const inThisRing = Math.min(capacity, leaves.length - i);
    r = ring * NODE_RING_GAP;
    for (let j = 0; j < inThisRing; j++) {
      const ang = (j / inThisRing) * Math.PI * 2;
      local[leaves[i + j].id] = { x: r * Math.cos(ang), y: r * Math.sin(ang) };
    }
    i += inThisRing;
    ring++;
  }
  // Even a single-package library needs enough room for its label.
  return { local, r: Math.max(r, NODE_RING_GAP) };
}

// Pack the clusters into concentric rings around the origin, largest first, so
// the biggest libraries sit in the middle and nothing overlaps.
function packClusters(sized: { id: string; r: number; count: number }[]): Cluster[] {
  const out: Cluster[] = [];
  if (!sized.length) return out;

  out.push({ ...sized[0], x: 0, y: 0 });
  let inner = sized[0].r;
  let i = 1;

  while (i < sized.length) {
    // Ring radius is set by the first (largest remaining) cluster on it.
    const ringR = inner + CLUSTER_GAP + sized[i].r;
    const batch: typeof sized = [];
    let used = 0;
    let widest = 0;
    while (i < sized.length) {
      // Angular width this cluster needs at this ring radius.
      const need = 2 * Math.asin(Math.min(1, (sized[i].r + CLUSTER_GAP / 2) / ringR));
      if (batch.length && used + need > Math.PI * 2) break;
      batch.push(sized[i]);
      used += need;
      widest = Math.max(widest, sized[i].r);
      i++;
    }
    // Distribute the leftover angle evenly so the ring looks deliberate.
    const slack = Math.max(0, Math.PI * 2 - used) / batch.length;
    let a = 0;
    for (const c of batch) {
      const need = 2 * Math.asin(Math.min(1, (c.r + CLUSTER_GAP / 2) / ringR));
      a += need / 2;
      out.push({ ...c, x: ringR * Math.cos(a), y: ringR * Math.sin(a) });
      a += need / 2 + slack;
    }
    inner = ringR + widest;
  }
  return out;
}

// Build the single graph containing every package of every library. This is the
// expensive step (layout for 600+ nodes) and is memoised on the graph data, so
// it runs once per load and never on a keystroke or a selection.
function buildFullGraph(graph: GraphData | null): FullGraph {
  const empty: FullGraph = {
    nodes: [],
    edges: [],
    pos: {},
    roots: new Set(),
    clusters: [],
    clusterById: new Map(),
    byId: new Map(),
    extent: 0,
  };
  if (!graph || !graph.packages.length) return empty;

  const byLib = new Map<string, GraphPackage[]>();
  for (const p of graph.packages) {
    const cur = byLib.get(p.library);
    if (cur) cur.push(p);
    else byLib.set(p.library, [p]);
  }

  const laid = [...byLib.entries()]
    .map(([id, pkgs]) => ({ id, pkgs, ...layoutCluster(pkgs) }))
    .sort((a, b) => b.r - a.r || b.pkgs.length - a.pkgs.length);

  const placed = packClusters(laid.map((l) => ({ id: l.id, r: l.r, count: l.pkgs.length })));
  const centre = new Map(placed.map((c) => [c.id, c]));

  const pos: Record<string, XY> = {};
  const roots = new Set<string>();
  let extent = 0;
  for (const l of laid) {
    const c = centre.get(l.id);
    if (!c) continue;
    const root = rootOf(l.pkgs);
    if (root) roots.add(root.id);
    for (const p of l.pkgs) {
      const o = l.local[p.id];
      if (!o) continue;
      const x = c.x + o.x;
      const y = c.y + o.y;
      pos[p.id] = { x, y };
      extent = Math.max(extent, Math.hypot(x, y));
    }
  }

  return {
    nodes: graph.packages,
    edges: graph.edges,
    pos,
    roots,
    clusters: placed,
    clusterById: centre,
    byId: new Map(graph.packages.map((p) => [p.id, p])),
    extent,
  };
}

/**
 * Undirected adjacency list, built once per graph. Every later distance query is
 * a BFS over this map, so no part of the UI ever scans the edge array again.
 */
function buildAdjacency(edges: GraphEdge[]): Map<string, Link[]> {
  const adj = new Map<string, Link[]>();
  const push = (from: string, link: Link) => {
    const cur = adj.get(from);
    if (cur) cur.push(link);
    else adj.set(from, [link]);
  };
  for (const e of edges) {
    if (!e.source || !e.target || e.source === e.target) continue;
    const kind = e.kind as EdgeKind;
    const weight = typeof e.weight === 'number' ? e.weight : 1;
    push(e.source, { id: e.target, kind, weight, dir: 'out' });
    push(e.target, { id: e.source, kind, weight, dir: 'in' });
  }
  return adj;
}

/**
 * Breadth-first hop distance from `start` over the allowed edge kinds. The
 * result is a Map of package id -> hop count (0 for the start itself); ids
 * absent from the map are unreachable under that edge filter.
 */
function bfsHops(adj: Map<string, Link[]>, start: string, kinds: Set<EdgeKind>): Map<string, number> {
  const dist = new Map<string, number>();
  if (!start) return dist;
  dist.set(start, 0);
  let frontier = [start];
  let d = 0;
  while (frontier.length) {
    d += 1;
    const next: string[] = [];
    for (const id of frontier) {
      const links = adj.get(id);
      if (!links) continue;
      for (const l of links) {
        if (!kinds.has(l.kind) || dist.has(l.id)) continue;
        dist.set(l.id, d);
        next.push(l.id);
      }
    }
    frontier = next;
  }
  return dist;
}

const shortName = (p: GraphPackage): string => p.name || p.importPath.split('/').pop() || p.importPath;

// ---------------------------------------------------------------------------
// Parity — prefer the measured figures in parity.json when the build produced
// them, otherwise fall back to whatever the graph's library records carry. The
// shape is read defensively so a change on the generator side degrades to the
// fallback instead of throwing.
// ---------------------------------------------------------------------------
type ParityMap = Record<string, string>;

function parityValue(v: unknown): string | null {
  if (typeof v === 'number' && Number.isFinite(v)) {
    const pct = v > 0 && v <= 1 ? v * 100 : v;
    return `${Math.round(pct * 10) / 10}%`;
  }
  if (typeof v === 'string' && v.trim()) return v.trim();
  return null;
}

function parityOf(entry: unknown): string | null {
  if (!entry || typeof entry !== 'object') return parityValue(entry);
  const rec = entry as Record<string, unknown>;
  for (const key of ['parityPercent', 'percent', 'parityAfter', 'parity', 'after', 'score']) {
    const got = parityValue(rec[key]);
    if (got) return got;
  }
  return null;
}

function normalizeParity(raw: unknown): ParityMap {
  const out: ParityMap = {};
  if (!raw || typeof raw !== 'object') return out;
  const container = (raw as { libraries?: unknown }).libraries ?? raw;
  const add = (id: unknown, entry: unknown) => {
    const key = typeof id === 'string' ? id.trim().toLowerCase() : '';
    if (!key) return;
    const value = parityOf(entry);
    if (value) out[key] = value;
  };
  if (Array.isArray(container)) {
    for (const item of container) {
      if (item && typeof item === 'object') add((item as { id?: unknown }).id, item);
    }
  } else if (typeof container === 'object') {
    for (const [id, entry] of Object.entries(container as Record<string, unknown>)) add(id, entry);
  }
  return out;
}

// ---------------------------------------------------------------------------
// Explore — search + the one all-libraries package graph.
// ---------------------------------------------------------------------------
export function Explore() {
  const router = useRouter();
  const [graph, setGraph] = useState<GraphData | null>(null);
  const [graphLoaded, setGraphLoaded] = useState(false);
  const [live, setLive] = useState<boolean | null>(null);
  const [measuredParity, setMeasuredParity] = useState<ParityMap | null>(null);

  // Search state. `q` is what the user typed; `dq` is the debounced copy that
  // drives every derived computation, so typing never re-walks 600+ nodes.
  const [q, setQ] = useState('');
  const [dq, setDq] = useState('');
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [backend, setBackend] = useState<SearchBackend | null>(null);
  const [cursor, setCursor] = useState(-1); // keyboard position in the hit list

  // Graph interaction state.
  const [selected, setSelected] = useState<string | null>(null);
  const [mode, setMode] = useState<DistMode>('all');
  const paneRef = useRef<HTMLDivElement | null>(null);
  const panelRef = useRef<HTMLElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [view, setView] = useState({ tx: 0, ty: 0, k: 1 });
  const drag = useRef<{ sx: number; sy: number; ox: number; oy: number } | null>(null);
  const reqId = useRef(0);

  // Load bundled graph + probe the API once. Both helpers are documented to
  // resolve rather than reject, but a .catch keeps a future regression from
  // leaving the pane stuck on "loading" with an unhandled rejection.
  useEffect(() => {
    let alive = true;
    loadFallbackGraph()
      .then((g) => {
        if (!alive) return;
        setGraph(g);
        setGraphLoaded(true);
      })
      .catch(() => {
        if (alive) setGraphLoaded(true);
      });
    hasApi()
      .then((ok) => alive && setLive(ok))
      .catch(() => alive && setLive(false));
    // Measured parity is optional: the file only exists once the generator has
    // published a browser-reachable copy. A miss is normal and falls back to the
    // parity figures the graph's library records already carry. The size guard
    // matters: the generator's full parity.json is megabytes of per-case detail,
    // which must never be pulled into the page just for one percentage.
    fetch(withBase('parity.json'))
      .then((res) => {
        const len = Number(res.headers.get('content-length') ?? 0);
        if (!res.ok || len > PARITY_MAX_BYTES) {
          void res.body?.cancel();
          return null;
        }
        return res.json();
      })
      .then((raw) => alive && setMeasuredParity(normalizeParity(raw)))
      .catch(() => alive && setMeasuredParity({}));
    return () => {
      alive = false;
    };
  }, []);

  // Debounce the query once; both the network search and the local highlight
  // computation hang off the debounced value.
  useEffect(() => {
    if (!q.trim()) {
      setDq('');
      return;
    }
    const t = setTimeout(() => setDq(q.trim()), 160);
    return () => clearTimeout(t);
  }, [q]);

  // Symbol search against the API (or the bundled index).
  useEffect(() => {
    if (!dq) {
      setHits([]);
      setBackend(null);
      setSearching(false);
      return;
    }
    setSearching(true);
    const id = ++reqId.current;
    search(dq, HIT_CAP)
      .then((res) => {
        if (id !== reqId.current) return; // a newer query superseded this one
        setHits(res.hits);
        setBackend(res.backend);
        setSearching(false);
      })
      .catch(() => {
        // Without this the spinner would latch on forever if search() ever
        // rejects (it is documented not to, but the UI must not depend on it).
        if (id !== reqId.current) return;
        setHits([]);
        setBackend(null);
        setSearching(false);
      });
  }, [dq]);

  // Typing invalidates the keyboard position in the results list.
  useEffect(() => setCursor(-1), [dq]);
  // "Searching…" is only honest while a query is in flight or pending.
  const pending = searching || (q.trim() !== '' && dq !== q.trim());

  const sub = useMemo(() => buildFullGraph(graph), [graph]);
  const adj = useMemo(() => buildAdjacency(sub.edges), [sub.edges]);
  const selectedPkg = useMemo(
    () => (selected ? sub.byId.get(selected) ?? null : null),
    [selected, sub.byId],
  );

  // Hop distance from the selection, computed once per (selection, mode) and
  // reused by the nodes, the edges, the legend and the panel.
  const kinds = useMemo(() => new Set(MODE_KINDS[mode]), [mode]);
  const dist = useMemo(
    () => (selected ? bfsHops(adj, selected, kinds) : new Map<string, number>()),
    [adj, selected, kinds],
  );

  // Nearest packages, ranked by hop distance then by the strength of the edge
  // that reaches them.
  const near = useMemo<NearNode[]>(() => {
    if (!selected) return [];
    const firstHop = new Map<string, Link>();
    for (const l of adj.get(selected) ?? []) {
      if (!kinds.has(l.kind)) continue;
      const cur = firstHop.get(l.id);
      if (!cur || l.weight > cur.weight) firstHop.set(l.id, l);
    }
    const out: NearNode[] = [];
    for (const [id, hops] of dist) {
      if (hops === 0) continue;
      const pkg = sub.byId.get(id);
      if (!pkg) continue;
      const link = firstHop.get(id);
      out.push({ pkg, hops, kind: link?.kind ?? null, dir: link?.dir ?? null });
    }
    out.sort((a, b) => {
      if (a.hops !== b.hops) return a.hops - b.hops;
      const aw = firstHop.get(a.pkg.id)?.weight ?? 0;
      const bw = firstHop.get(b.pkg.id)?.weight ?? 0;
      if (aw !== bw) return bw - aw;
      return shortName(a.pkg).localeCompare(shortName(b.pkg));
    });
    return out.slice(0, NEAR_CAP);
  }, [adj, dist, kinds, selected, sub.byId]);

  // How many packages sit on each hop ring, plus how much of the corpus the
  // selection can reach at all under the current edge filter.
  const rings = useMemo(() => {
    const counts = [0, 0, 0];
    let reachable = 0;
    for (const [, d] of dist) {
      if (d === 0) continue;
      reachable += 1;
      if (d <= RING_CAP) counts[d - 1] += 1;
    }
    return { counts, reachable };
  }, [dist]);

  // Where the selection sits inside its own library cluster: which concentric
  // layout ring it occupies, out of how many the cluster has.
  const clusterPos = useMemo(() => {
    if (!selectedPkg) return null;
    const c = sub.clusterById.get(selectedPkg.library);
    const p = sub.pos[selectedPkg.id];
    if (!c || !p) return null;
    const radius = Math.hypot(p.x - c.x, p.y - c.y);
    return {
      ring: Math.round(radius / NODE_RING_GAP),
      total: Math.max(1, Math.round(c.r / NODE_RING_GAP)),
      size: c.count,
      isRoot: sub.roots.has(selectedPkg.id),
    };
  }, [selectedPkg, sub.clusterById, sub.pos, sub.roots]);

  // Packages the query highlights: any package holding a matching symbol, plus
  // any package whose own name or import path matches. Joined on importPath,
  // which is what identifies a node in the graph.
  const matched = useMemo(() => {
    const ids = new Set<string>();
    if (!dq) return ids;
    const needle = dq.toLowerCase();
    const paths = new Set(hits.map((h) => h.packageImportPath));
    for (const p of sub.nodes) {
      if (paths.has(p.importPath) || p.importPath.toLowerCase().includes(needle) || p.name.toLowerCase().includes(needle)) {
        ids.add(p.id);
      }
    }
    return ids;
  }, [dq, hits, sub.nodes]);

  const parityFor = useCallback(
    (library: string): { value: string; measured: boolean } | null => {
      const id = library.trim().toLowerCase();
      const m = measuredParity?.[id];
      if (m) return { value: m, measured: true };
      const lib = graph?.libraries.find((l) => l.id === library);
      const declared = lib?.parityAfter;
      return declared ? { value: declared, measured: false } : null;
    },
    [graph, measuredParity],
  );

  // Fit the whole graph on first load. This runs on `sub`, which changes only
  // when the graph data itself does, so it does not fight the user's panning.
  useEffect(() => {
    const pane = paneRef.current;
    const w = pane?.clientWidth ?? 720;
    const h = pane?.clientHeight ?? 420;
    const fit = sub.extent > 0 ? Math.min(1, (Math.min(w, h) / 2 - 30) / sub.extent) : 1;
    setView({ tx: w / 2, ty: h / 2, k: Math.max(0.08, fit) });
  }, [sub]);

  /** Pan (and optionally zoom) the view so a package sits in the middle. */
  const centreOn = useCallback((id: string, zoomTo?: number) => {
    const p = sub.pos[id];
    const pane = paneRef.current;
    if (!p || !pane) return;
    const w = pane.clientWidth;
    const h = pane.clientHeight;
    setView((v) => {
      const k = zoomTo ? Math.max(v.k, zoomTo) : v.k;
      return { tx: w / 2 - p.x * k, ty: h / 2 - p.y * k, k };
    });
  }, [sub.pos]);

  // Centre the view on the best match when a search resolves, so a hit in a
  // distant cluster is not left off-screen.
  useEffect(() => {
    if (!matched.size) return;
    const first = sub.nodes.find((n) => matched.has(n.id));
    if (first) centreOn(first.id);
    // Only react to a new set of matches, not to panning.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [matched]);

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

  const accent = accentFor(selectedPkg?.library ?? '');

  const goLibrary = (library: string) => {
    if (library) router.push('/lib/' + encodeURIComponent(library));
  };

  /** Select a package and bring it into view; used by nodes, hits and chips. */
  const pick = useCallback(
    (id: string, opts?: { centre?: boolean; focusPanel?: boolean }) => {
      setSelected(id);
      if (opts?.centre) centreOn(id, LABEL_ZOOM + 0.2);
      if (opts?.focusPanel) requestAnimationFrame(() => panelRef.current?.focus());
    },
    [centreOn],
  );
  const onNodeSelect = useCallback((id: string) => pick(id), [pick]);

  // Clicking a search result highlights and focuses that package in the graph
  // rather than navigating away — the point of the search is to find it here.
  const focusHit = useCallback(
    (h: SearchHit, focusPanel = false) => {
      const node = sub.nodes.find((n) => n.importPath === h.packageImportPath);
      if (!node) {
        goLibrary(h.library); // package not in the graph (e.g. stale index)
        return;
      }
      pick(node.id, { centre: true, focusPanel });
    },
    // goLibrary closes over a stable router
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [pick, sub.nodes],
  );

  // Keyboard driving of the results list from the search field.
  const onSearchKey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (!hits.length) {
      if (e.key === 'Escape' && q) {
        setQ('');
        e.preventDefault();
      }
      return;
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const next = e.key === 'ArrowDown'
        ? Math.min(hits.length - 1, cursor + 1)
        : Math.max(0, cursor - 1);
      setCursor(next);
      focusHit(hits[next]);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      focusHit(hits[Math.max(0, cursor)], true);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setQ('');
      setCursor(-1);
    }
  };

  // Arrow keys walk the graph without putting 600+ tab stops in the document:
  // the pane is one tab stop and moves the selection.
  const onPaneKey = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (!sub.nodes.length) return;
    const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
    if (step) {
      e.preventDefault();
      const at = selected ? sub.nodes.findIndex((n) => n.id === selected) : -1;
      const next = sub.nodes[(at + step + sub.nodes.length) % sub.nodes.length];
      if (next) pick(next.id, { centre: true });
    } else if (e.key === 'Enter' && selected) {
      e.preventDefault();
      panelRef.current?.focus();
    } else if (e.key === 'Escape' && selected) {
      e.preventDefault();
      setSelected(null);
    }
  };

  const closePanel = useCallback(() => {
    setSelected(null);
    paneRef.current?.focus();
  }, []);

  const parity = selectedPkg ? parityFor(selectedPkg.library) : null;
  const far = view.k < LABEL_ZOOM;

  // Edge rendering, recomputed only when the graph, the selection or the match
  // set changes — never while panning or zooming (that is a CSS transform).
  const edgeEls = useMemo(
    () =>
      sub.edges.map((e, i) => {
        const a = sub.pos[e.source];
        const b = sub.pos[e.target];
        if (!a || !b) return null;
        const da = dist.get(e.source);
        const db = dist.get(e.target);
        // An edge is "on" when it is a step outward along the BFS front from the
        // selection, i.e. it is one of the hops the distance readout counts.
        const step = da !== undefined && db !== undefined ? Math.min(da, db) : -1;
        const on =
          da !== undefined && db !== undefined && Math.abs(da - db) === 1 && step < RING_CAP;
        const hit = matched.size > 0 && (matched.has(e.source) || matched.has(e.target));
        const muted = (matched.size > 0 && !hit && !on) || (selected !== null && !on && !hit);
        return (
          <line
            key={i}
            x1={a.x}
            y1={a.y}
            x2={b.x}
            y2={b.y}
            stroke={on ? RING_COLOR[step] ?? accent : KIND_COLOR[e.kind]}
            strokeWidth={on ? 2 : 1}
            strokeOpacity={on ? 0.9 : muted ? 0.05 : 0.4}
          />
        );
      }),
    [sub.edges, sub.pos, dist, matched, selected, accent],
  );

  return (
    <section className="view active" id="view-explore">
      <SecH h="h2">Explore</SecH>
      <p className="muted">
        Search every exported symbol across all ports, and browse how every library's packages connect — one graph, the
        whole ecosystem. Search is served by the live GraphQL + Upstash Search API when it's deployed, and falls back to
        the data bundled with this site everywhere else.
      </p>

      {/* mode badge */}
      <div className="xpl-mode">
        {live === null ? (
          <span className="xpl-badge xpl-badge-idle">
            <span className="xpl-dot" /> checking API…
          </span>
        ) : live ? (
          <span className="xpl-badge xpl-badge-live">
            <span className="xpl-dot" /> Live API — GraphQL + Upstash Search
          </span>
        ) : (
          <span className="xpl-badge xpl-badge-fallback">
            <span className="xpl-dot" /> Bundled fallback — offline index
          </span>
        )}
      </div>

      {/* search — highlights matches in place, never filters the graph down */}
      <div className="xpl-search">
        <span className="xpl-search-ico" aria-hidden>
          ⌕
        </span>
        <label className="sr-only" htmlFor="xpl-q">Search symbols and packages</label>
        <input
          id="xpl-q"
          ref={inputRef}
          className="xpl-search-input"
          type="search"
          placeholder="Search symbols and packages — highlights them on the graph…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={onSearchKey}
          spellCheck={false}
          autoComplete="off"
          aria-expanded={hits.length > 0}
          aria-controls="xpl-hitlist"
          aria-autocomplete="list"
          aria-activedescendant={cursor >= 0 && hits[cursor] ? `xpl-hit-${cursor}` : undefined}
          aria-describedby="xpl-search-help"
        />
        {q && (
          <button type="button" className="xpl-search-clear" onClick={() => setQ('')} aria-label="Clear search">
            ×
          </button>
        )}
      </div>
      <p className="xpl-search-help" id="xpl-search-help">
        Matches stay on the map: hits light up, everything else recedes. Use the up and down arrows to step through
        results, Enter to open the details panel.
      </p>

      {q.trim() && (
        <div className="xpl-results">
          <div className="xpl-results-hd" role="status" aria-live="polite">
            {pending ? (
              'Searching…'
            ) : (
              <>
                {hits.length} result{hits.length === 1 ? '' : 's'}
                {matched.size > 0 && (
                  <span className="xpl-results-hl">
                    {matched.size} package{matched.size === 1 ? '' : 's'} highlighted
                  </span>
                )}
                {backend && (
                  <span className="xpl-results-backend">
                    via {backend === 'upstash' ? 'Upstash Search' : backend === 'memory' ? 'in-memory BM25' : 'bundled index'}
                  </span>
                )}
              </>
            )}
          </div>
          {!pending && hits.length === 0 && (
            <div className="xpl-empty">No symbols match “{q.trim()}”.</div>
          )}
          <ul className="xpl-hitlist" id="xpl-hitlist" role="listbox" aria-label="Search results">
            {hits.map((h, i) => (
              <li key={h.id} role="presentation">
                <button
                  type="button"
                  id={`xpl-hit-${i}`}
                  role="option"
                  aria-selected={i === cursor}
                  className={`xpl-hit${i === cursor ? ' is-cursor' : ''}`}
                  onClick={() => {
                    setCursor(i);
                    focusHit(h, true);
                  }}
                  title={`Highlight ${h.packageImportPath} on the graph`}
                >
                  <span className="xpl-hit-name">{h.name}</span>
                  <span
                    className="xpl-chip"
                    style={{ background: (SYMBOL_COLOR[h.kind] || 'var(--fg-dim)') + '22', color: SYMBOL_COLOR[h.kind] || 'var(--fg-muted)' }}
                  >
                    {h.kind}
                  </span>
                  <span className="xpl-hit-pkg">{h.packageImportPath}</span>
                  <span className="xpl-hit-lib" style={{ color: accentFor(h.library) }}>
                    {h.library}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* graph summary — one graph, every package, clustered by library */}
      <div className="xpl-picker">
        <span className="xpl-graph-title">Package graph</span>
        {sub.nodes.length > 0 ? (
          <span className="xpl-cap">
            {sub.nodes.length.toLocaleString()} packages across {sub.clusters.length} libraries
            {matched.size > 0 && (
              <>
                {' · '}
                <b style={{ color: 'var(--accent)' }}>
                  {matched.size} highlighted
                </b>
              </>
            )}
          </span>
        ) : (
          <span className="xpl-cap">{graphLoaded ? 'no graph data available' : 'loading…'}</span>
        )}
      </div>

      {/* announce the selection for assistive tech (not role=status: the results
          header already owns that role) */}
      <div className="sr-only" aria-live="polite" aria-atomic="true">
        {selectedPkg
          ? `Selected ${selectedPkg.importPath}. ${rings.counts[0]} packages one hop away, ${rings.reachable} reachable over ${MODE_LABEL[mode]}.`
          : ''}
      </div>

      {/* stage: the left panel is a sibling of the pane so it can overlay on
          wide screens and stack below the graph on narrow ones */}
      <div className={`xpl-stage${selectedPkg ? ' has-panel' : ''}`}>
        <div
          className="xpl-pane"
          ref={paneRef}
          tabIndex={0}
          role="group"
          aria-label="Package graph — arrow keys move the selection"
          onPointerDown={onPaneDown}
          onPointerMove={onPaneMove}
          onPointerUp={onPaneUp}
          onPointerCancel={onPaneUp}
          onWheel={onWheel}
          onKeyDown={onPaneKey}
        >
          <div className="xpl-bg" />
          <div
            className="xpl-world"
            style={{ transform: `translate(${view.tx}px, ${view.ty}px) scale(${view.k})` }}
          >
            <svg className="xpl-edges" width="1" height="1">
              {edgeEls}
            </svg>

            {/* one label per cluster, so the map reads as libraries not a blob */}
            {sub.clusters.map((c) => (
              <span
                key={c.id}
                className="xpl-cluster-label"
                style={{ left: c.x, top: c.y - c.r - 18, color: accentFor(c.id) }}
                aria-hidden
              >
                {c.id}
              </span>
            ))}

            {sub.nodes.map((n) => {
              const p = sub.pos[n.id];
              if (!p) return null;
              const hops = dist.get(n.id);
              const active = selected === n.id;
              const isHit = matched.has(n.id);
              const state: NodeState = active
                ? 'active'
                : isHit
                  ? 'hit'
                  : matched.size > 0
                    ? 'dim'
                    : selected === null
                      ? 'base'
                      : hops !== undefined && hops <= RING_CAP
                        ? (`n${hops}` as NodeState)
                        : 'dim';
              return (
                <GraphNode
                  key={n.id}
                  id={n.id}
                  x={p.x}
                  y={p.y}
                  label={shortName(n)}
                  path={n.importPath}
                  count={n.symbolCount}
                  accent={accentFor(n.library)}
                  isRoot={sub.roots.has(n.id)}
                  state={state}
                  hops={state.startsWith('n') ? hops ?? null : null}
                  far={far}
                  onSelect={onNodeSelect}
                />
              );
            })}
          </div>

          {/* zoom controls */}
          <div className="xpl-zoom">
            <button type="button" onClick={() => zoom(1.2)} aria-label="Zoom in">
              +
            </button>
            <button type="button" onClick={() => zoom(1 / 1.2)} aria-label="Zoom out">
              −
            </button>
          </div>

          {/* legend: edge kinds, or hop rings once something is selected */}
          <div className="xpl-legend">
            {selectedPkg
              ? [0, 1, 2].map((i) => (
                  <span key={i} className="xpl-leg">
                    <span className="xpl-leg-dot" style={{ background: RING_COLOR[i] }} />
                    {i + 1} hop{i ? 's' : ''} ({rings.counts[i]})
                  </span>
                ))
              : (Object.keys(KIND_COLOR) as EdgeKind[]).map((k) => (
                  <span key={k} className="xpl-leg">
                    <span className="xpl-leg-sw" style={{ background: KIND_COLOR[k] }} />
                    {k}
                  </span>
                ))}
          </div>

          <div className="xpl-hint">drag to pan · scroll to zoom · click a node for its cluster distance</div>
        </div>

        {/* left info panel for the selected package */}
        {selectedPkg && (
          <aside
            className="xpl-panel"
            ref={panelRef}
            tabIndex={-1}
            aria-label={`Details for ${selectedPkg.importPath}`}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault();
                closePanel();
              }
            }}
          >
            <div className="xpl-panel-hd">
              <span className="xpl-node-dot" style={{ background: accent }} />
              <h3 className="xpl-panel-name">{shortName(selectedPkg)}</h3>
              <button type="button" className="xpl-panel-x" onClick={closePanel} aria-label="Close package details">
                ×
              </button>
            </div>
            <button
              type="button"
              className="xpl-panel-lib"
              style={{ color: accentFor(selectedPkg.library) }}
              onClick={() => goLibrary(selectedPkg.library)}
            >
              {selectedPkg.library} →
            </button>
            <div className="xpl-panel-path">{selectedPkg.importPath}</div>
            {selectedPkg.synopsis && <p className="xpl-panel-syn">{selectedPkg.synopsis}</p>}

            <dl className="xpl-panel-facts">
              <div>
                <dt>Symbols</dt>
                <dd>{selectedPkg.symbolCount.toLocaleString()}</dd>
              </div>
              <div>
                <dt>{parity?.measured ? 'Parity (measured)' : 'Parity (declared)'}</dt>
                <dd>{parity ? parity.value : 'not reported'}</dd>
              </div>
              {clusterPos && (
                <div>
                  <dt>Cluster</dt>
                  <dd>
                    {clusterPos.isRoot ? 'module root' : `ring ${clusterPos.ring} of ${clusterPos.total}`} ·{' '}
                    {clusterPos.size} pkgs
                  </dd>
                </div>
              )}
            </dl>

            <div className="xpl-panel-sec">
              <div className="xpl-panel-sec-t">Cluster distance</div>
              <div className="xpl-modes" role="group" aria-label="Edges counted as one hop">
                {(['all', 'deps'] as DistMode[]).map((m) => (
                  <button
                    key={m}
                    type="button"
                    className={`xpl-modebtn${mode === m ? ' is-on' : ''}`}
                    aria-pressed={mode === m}
                    onClick={() => setMode(m)}
                  >
                    {MODE_LABEL[m]}
                  </button>
                ))}
              </div>
              <p className="xpl-panel-note">
                Hop count = shortest path over{' '}
                {mode === 'all' ? 'same-library, import, reference and shared-upstream edges' : 'import, reference and shared-upstream edges (same-library links excluded)'}
                .
              </p>
              <div className="xpl-distbar">
                {[0, 1, 2].map((i) => (
                  <span key={i} className="xpl-distcell">
                    <span className="xpl-distnum" style={{ color: RING_COLOR[i] }}>
                      {rings.counts[i]}
                    </span>
                    <span className="xpl-distlab">{i + 1} hop{i ? 's' : ''}</span>
                  </span>
                ))}
              </div>
              <p className="xpl-panel-note">
                {rings.reachable.toLocaleString()} of {Math.max(0, sub.nodes.length - 1).toLocaleString()} packages
                reachable from here.
              </p>
            </div>

            {near.length > 0 && (
              <div className="xpl-panel-sec">
                <div className="xpl-panel-sec-t">Nearest packages</div>
                <ul className="xpl-nearlist">
                  {near.map((n) => (
                    <li key={n.pkg.id}>
                      <button type="button" className="xpl-near" onClick={() => pick(n.pkg.id, { centre: true })} title={n.pkg.importPath}>
                        <span
                          className="xpl-near-hops"
                          style={{ color: RING_COLOR[Math.min(n.hops - 1, RING_COLOR.length - 1)] }}
                        >
                          {n.hops}
                        </span>
                        <span className="xpl-near-name">{shortName(n.pkg)}</span>
                        {n.kind && (
                          <span className="xpl-near-kind" style={{ color: KIND_COLOR[n.kind] }}>
                            {n.dir === 'in' ? '←' : '→'} {n.kind}
                          </span>
                        )}
                        <span className="xpl-near-lib" style={{ color: accentFor(n.pkg.library) }}>
                          {n.pkg.library}
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </aside>
        )}
      </div>

      <style>{styles}</style>
    </section>
  );
}

// ---------------------------------------------------------------------------
// One package node. Memoised on primitive props so a selection change only
// re-renders the handful of nodes whose state actually changed, not all 600.
// ---------------------------------------------------------------------------
type NodeState = 'base' | 'active' | 'hit' | 'dim' | 'n1' | 'n2' | 'n3';

interface GraphNodeProps {
  id: string;
  x: number;
  y: number;
  label: string;
  path: string;
  count: number;
  accent: string;
  isRoot: boolean;
  state: NodeState;
  hops: number | null;
  far: boolean;
  onSelect: (id: string) => void;
}

const GraphNode = memo(function GraphNode({
  id,
  x,
  y,
  label,
  path,
  count,
  accent,
  isRoot,
  state,
  hops,
  far,
  onSelect,
}: GraphNodeProps) {
  const active = state === 'active';
  const hit = state === 'hit';
  return (
    <button
      className={`xpl-node is-${state}${isRoot ? ' is-root' : ''}${far ? ' is-far' : ''}`}
      style={{
        left: x,
        top: y,
        borderColor: hit ? 'var(--accent)' : active || isRoot ? accent : 'var(--edge)',
        boxShadow: hit
          ? '0 0 0 3px var(--accent), 0 0 18px 2px color-mix(in srgb, var(--accent) 60%, transparent)'
          : active
            ? `0 0 0 2px ${accent}, 0 6px 18px rgba(0,0,0,.28)`
            : undefined,
      }}
      type="button"
      // The graph is one tab stop (the pane); nodes are reached with the arrow
      // keys or the search results, so 600 buttons do not flood the tab order.
      tabIndex={-1}
      onClick={() => onSelect(id)}
      title={path}
      aria-current={active ? 'true' : undefined}
    >
      <span className="xpl-node-dot" style={{ background: accent }} />
      <span className="xpl-node-label">{label}</span>
      {hops !== null && !far && <span className="xpl-node-hop">{hops}</span>}
      {hops === null && count > 0 && <span className="xpl-node-count">{count}</span>}
    </button>
  );
});

const styles = `
#view-explore .sr-only {
  position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px;
  overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0;
}
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
  flex: 1; min-width: 0; border: none; background: transparent; color: var(--fg);
  font: inherit; font-size: 1.02rem; padding: .85rem .7rem; outline: none;
}
.xpl-search-input::placeholder { color: var(--fg-dim); }
.xpl-search-clear {
  border: none; background: transparent; color: var(--fg-dim); cursor: pointer;
  font-size: 1.3rem; line-height: 1; padding: 0 .9rem;
}
.xpl-search-clear:hover { color: var(--fg); }
.xpl-search-help { font-size: .74rem; color: var(--fg-dim); margin: .1rem 0 .2rem; }

.xpl-results { margin: .3rem 0 1.4rem; }
.xpl-results-hd { font-size: .8rem; color: var(--fg-dim); margin: .2rem 0 .5rem; display: flex; gap: .5rem; align-items: baseline; flex-wrap: wrap; }
.xpl-results-hl { color: var(--accent); font-weight: 600; }
.xpl-results-backend { color: var(--fg-muted); font-family: "SF Mono", ui-monospace, monospace; font-size: .74rem; }
.xpl-empty { color: var(--fg-muted); font-size: .9rem; padding: .4rem 0; }
.xpl-hitlist { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: .3rem; max-height: 15rem; overflow-y: auto; }
.xpl-hit {
  width: 100%; display: flex; align-items: center; gap: .6rem; text-align: left;
  padding: .55rem .8rem; border-radius: 11px; border: 1px solid var(--edge);
  background: var(--glass); color: var(--fg); font: inherit; cursor: pointer;
  transition: border-color .12s, background .12s, transform .12s;
}
.xpl-hit:hover { border-color: var(--edge-2); background: var(--glass-2); transform: translateX(2px); }
.xpl-hit.is-cursor { border-color: var(--accent); background: var(--glass-2); }
.xpl-hit-name { font-weight: 600; font-family: "SF Mono", ui-monospace, monospace; font-size: .92rem; flex: none; }
.xpl-chip { flex: none; padding: .1rem .5rem; border-radius: 999px; font-size: .68rem; font-weight: 700; text-transform: uppercase; letter-spacing: .03em; }
.xpl-hit-pkg { flex: 1; min-width: 0; color: var(--fg-dim); font-size: .78rem; font-family: "SF Mono", ui-monospace, monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.xpl-hit-lib { flex: none; font-size: .8rem; font-weight: 600; }

.xpl-picker { display: flex; align-items: center; gap: .6rem; margin: .4rem 0 .9rem; flex-wrap: wrap; }
.xpl-cap { font-size: .76rem; color: var(--fg-dim); font-family: "SF Mono", ui-monospace, monospace; }
.xpl-graph-title { font-size: .82rem; color: var(--fg-muted); font-weight: 600; }

/* Stage — pane plus the left-hand detail panel. */
.xpl-stage { position: relative; display: flex; flex-direction: column; gap: .8rem; }
.xpl-pane {
  position: relative; width: 100%; height: 520px; border-radius: 16px;
  border: 1px solid var(--edge); background: var(--code-bg); overflow: hidden;
  cursor: grab; touch-action: none; box-shadow: var(--hi);
}
.xpl-pane:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.xpl-pane:active { cursor: grabbing; }
.xpl-bg {
  position: absolute; inset: 0; pointer-events: none;
  background-image: radial-gradient(circle, var(--edge-2) 1.2px, transparent 1.2px);
  background-size: 26px 26px; opacity: .5;
}
.xpl-world { position: absolute; top: 0; left: 0; transform-origin: 0 0; will-change: transform; }
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
.xpl-node.is-active { z-index: 6; opacity: 1; }
.xpl-node.is-dim { opacity: .1; }
/* Hop shading: the nearer the selection, the more present the node. */
.xpl-node.is-n1 { opacity: 1; z-index: 5; }
.xpl-node.is-n2 { opacity: .68; z-index: 4; }
.xpl-node.is-n3 { opacity: .42; }
/* A search hit stays fully opaque and sits above everything else. */
.xpl-node.is-hit { opacity: 1; z-index: 5; }
/* Zoomed far enough out that labels are unreadable, nodes become bare dots.
   With 600+ packages on screen this is the difference between a map and a
   wall of overlapping text. */
.xpl-node.is-far { padding: 0; border-radius: 50%; width: 11px; height: 11px; gap: 0; box-shadow: none; }
.xpl-node.is-far .xpl-node-label,
.xpl-node.is-far .xpl-node-count { display: none; }
.xpl-node.is-far .xpl-node-dot { width: 7px; height: 7px; }
.xpl-node.is-far.is-hit, .xpl-node.is-far.is-active { width: 15px; height: 15px; }
.xpl-node-dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
.xpl-node-label { max-width: 160px; overflow: hidden; text-overflow: ellipsis; }
.xpl-node-count { font-size: .64rem; color: var(--fg-dim); font-family: "SF Mono", ui-monospace, monospace; }
.xpl-node-hop {
  font-size: .62rem; font-weight: 800; line-height: 1; padding: .12rem .3rem;
  border-radius: 999px; background: var(--glass); color: var(--fg-muted);
  border: 1px solid var(--edge); font-family: "SF Mono", ui-monospace, monospace;
}

.xpl-cluster-label {
  position: absolute; transform: translate(-50%, -50%);
  font-size: .8rem; font-weight: 700; letter-spacing: .04em; text-transform: lowercase;
  opacity: .55; pointer-events: none; white-space: nowrap; z-index: 1;
}

/* The detail panel overlays the left edge of the graph on wide screens. */
.xpl-panel {
  position: absolute; top: 14px; left: 14px; z-index: 7; width: 290px;
  max-height: calc(520px - 28px); overflow-y: auto;
  background: var(--glass); border: 1px solid var(--edge); border-radius: 14px;
  padding: 12px 14px; box-shadow: 0 10px 30px rgba(0,0,0,.24); backdrop-filter: var(--blur);
}
.xpl-panel:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.xpl-panel-hd { display: flex; align-items: center; gap: .5rem; }
.xpl-panel-name {
  margin: 0; font-weight: 700; font-size: .98rem; color: var(--fg);
  font-family: "SF Mono", ui-monospace, monospace; flex: 1; min-width: 0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.xpl-panel-x { border: none; background: transparent; color: var(--fg-dim); font-size: 1.3rem; line-height: 1; cursor: pointer; padding: 0 .2rem; }
.xpl-panel-x:hover { color: var(--fg); }
.xpl-panel-lib { border: none; background: transparent; font: inherit; font-size: .78rem; font-weight: 700; cursor: pointer; padding: 0; margin: .2rem 0 0; }
.xpl-panel-lib:hover { text-decoration: underline; }
.xpl-panel-path { font-size: .72rem; color: var(--fg-dim); font-family: "SF Mono", ui-monospace, monospace; margin: .3rem 0; word-break: break-all; }
.xpl-panel-syn { font-size: .8rem; color: var(--fg-muted); line-height: 1.5; margin: .35rem 0 .55rem; }
.xpl-panel-facts { display: grid; grid-template-columns: 1fr 1fr; gap: .45rem .6rem; margin: .1rem 0 .2rem; }
.xpl-panel-facts dt { font-size: .62rem; text-transform: uppercase; letter-spacing: .05em; color: var(--fg-dim); }
.xpl-panel-facts dd { margin: .1rem 0 0; font-size: .8rem; font-weight: 600; color: var(--fg); }
.xpl-panel-sec { margin-top: .75rem; border-top: 1px solid var(--edge); padding-top: .6rem; }
.xpl-panel-sec-t { font-size: .66rem; text-transform: uppercase; letter-spacing: .05em; color: var(--fg-dim); margin-bottom: .4rem; }
.xpl-panel-note { font-size: .68rem; color: var(--fg-dim); line-height: 1.45; margin: .35rem 0 0; }
.xpl-modes { display: inline-flex; border: 1px solid var(--edge); border-radius: 999px; overflow: hidden; }
.xpl-modebtn { border: none; background: transparent; color: var(--fg-dim); font: inherit; font-size: .7rem; font-weight: 600; padding: .22rem .6rem; cursor: pointer; }
.xpl-modebtn.is-on { background: var(--glass-2); color: var(--fg); }
.xpl-distbar { display: flex; gap: .5rem; margin-top: .5rem; }
.xpl-distcell { flex: 1; display: flex; flex-direction: column; gap: .1rem; padding: .35rem .4rem; border: 1px solid var(--edge); border-radius: 10px; background: var(--glass-2); }
.xpl-distnum { font-size: .95rem; font-weight: 800; font-family: "SF Mono", ui-monospace, monospace; }
.xpl-distlab { font-size: .62rem; color: var(--fg-dim); }
.xpl-nearlist { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: .2rem; }
.xpl-near {
  width: 100%; display: flex; align-items: center; gap: .4rem; text-align: left;
  border: 1px solid transparent; background: transparent; color: var(--fg-muted);
  font: inherit; font-size: .74rem; border-radius: 8px; padding: .25rem .35rem; cursor: pointer;
}
.xpl-near:hover, .xpl-near:focus-visible { border-color: var(--edge-2); background: var(--glass-2); color: var(--fg); }
.xpl-near-hops { flex: none; width: 1.1rem; font-weight: 800; font-family: "SF Mono", ui-monospace, monospace; }
.xpl-near-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
.xpl-near-kind { flex: none; font-size: .62rem; }
.xpl-near-lib { flex: none; font-size: .66rem; font-weight: 700; }

.xpl-zoom {
  position: absolute; bottom: 14px; right: 14px; z-index: 5; display: flex; flex-direction: column;
  border: 1px solid var(--edge); border-radius: 10px; overflow: hidden; background: var(--glass);
  backdrop-filter: var(--blur); box-shadow: 0 6px 24px rgba(0,0,0,.16);
}
.xpl-zoom button { width: 32px; height: 32px; border: none; background: transparent; color: var(--fg); cursor: pointer; font-size: 16px; display: grid; place-items: center; }
.xpl-zoom button:first-child { border-bottom: 1px solid var(--edge); }
.xpl-zoom button:hover { background: var(--glass-2); }

.xpl-legend {
  position: absolute; top: 14px; right: 14px; z-index: 5; display: flex; flex-wrap: wrap; gap: .4rem .7rem;
  padding: .5rem .7rem; border-radius: 10px; border: 1px solid var(--edge); background: var(--glass);
  backdrop-filter: var(--blur); box-shadow: var(--hi); max-width: 46%;
}
.xpl-leg { display: inline-flex; align-items: center; gap: .35rem; font-size: .7rem; color: var(--fg-muted); }
.xpl-leg-sw { width: 12px; height: 3px; border-radius: 2px; }
.xpl-leg-dot { width: 9px; height: 9px; border-radius: 50%; }

.xpl-hint {
  position: absolute; bottom: 14px; left: 50%; transform: translateX(-50%); z-index: 4;
  font-size: .7rem; color: var(--fg-muted); font-family: "SF Mono", ui-monospace, monospace;
  background: var(--glass); border: 1px solid var(--edge); padding: 4px 11px; border-radius: 20px;
  box-shadow: 0 4px 16px rgba(0,0,0,.12); white-space: nowrap;
}

/* Narrow screens: the panel must not sit on top of the graph, so it stops
   being an overlay and stacks underneath it instead. */
@media (max-width: 900px) {
  .xpl-panel {
    position: static; width: auto; max-width: none; max-height: none;
    order: 2; box-shadow: var(--hi);
  }
  .xpl-stage.has-panel .xpl-pane { order: 1; }
}
@media (max-width: 640px) {
  .xpl-hint { display: none; }
  .xpl-pane { height: 400px; }
  .xpl-panel-facts { grid-template-columns: 1fr; }
}

@keyframes xpl-pulse { 0%, 100% { opacity: 1; } 50% { opacity: .35; } }
@media (prefers-reduced-motion: reduce) {
  .xpl-badge-idle .xpl-dot { animation: none; }
  .xpl-node, .xpl-hit { transition: none; }
}
`;
