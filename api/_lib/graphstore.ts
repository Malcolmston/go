// graphstore.ts — pure query functions over the loaded package-connection graph.
//
// All reads go through data.getGraph() (memoized), so these functions have no
// side effects and always operate on the same in-memory graph.
//
// Graph shape (see contract DATA FILES):
//   libraries: [ { id, name, packageCount, symbolCount, parityAfter } ]
//   packages:  [ { id (=importPath), importPath, name, library, synopsis, symbolCount } ]
//   edges:     [ { source, target, kind: import|shared-upstream|reference|same-library, weight } ]

import { getGraph } from './data';
import type { Graph, GraphLibrary, GraphPackage } from './data';

// Edge kinds that count as an outgoing "import-like" dependency for importsOf().
const IMPORT_KINDS = new Set(['import', 'reference', 'same-library']);

// Per-graph package index, memoized on the graph object identity so repeated
// resolver calls within one invocation don't rebuild the Map.
const indexCache = new WeakMap<Graph, Map<string, GraphPackage>>();

function packageIndex(graph: Graph): Map<string, GraphPackage> {
  let idx = indexCache.get(graph);
  if (!idx) {
    idx = new Map<string, GraphPackage>();
    for (const p of graph.packages) idx.set(p.id, p);
    indexCache.set(graph, idx);
  }
  return idx;
}

// package(id) -> the package node, or null.
export function pkg(id: string | null | undefined): GraphPackage | null {
  if (id == null) return null;
  const graph = getGraph();
  return packageIndex(graph).get(id) || null;
}

export interface PackagesQuery {
  library?: string;
  search?: string;
  first?: number;
}

// packages({library, search, first}) -> filtered, capped list of package nodes.
export function packages({ library, search, first = 50 }: PackagesQuery = {}): GraphPackage[] {
  const graph = getGraph();
  let list = graph.packages;

  if (library) {
    list = list.filter((p) => p.library === library);
  }

  if (search && String(search).trim() !== '') {
    const q = String(search).toLowerCase();
    list = list.filter(
      (p) =>
        (p.importPath || '').toLowerCase().includes(q) ||
        (p.name || '').toLowerCase().includes(q) ||
        (p.synopsis || '').toLowerCase().includes(q)
    );
  }

  const limit = Number.isFinite(first) ? Math.max(0, Math.floor(first)) : 50;
  return list.slice(0, limit);
}

// libraries() -> all library nodes.
export function libraries(): GraphLibrary[] {
  return getGraph().libraries;
}

// importsOf(id) -> package nodes this package depends on
// (targets of import|reference|same-library edges leaving id), deduplicated.
export function importsOf(id: string): GraphPackage[] {
  const graph = getGraph();
  const index = packageIndex(graph);
  const seen = new Set<string>();
  const out: GraphPackage[] = [];
  for (const e of graph.edges) {
    if (e.source === id && IMPORT_KINDS.has(e.kind)) {
      const target = index.get(e.target);
      if (target && !seen.has(target.id)) {
        seen.add(target.id);
        out.push(target);
      }
    }
  }
  return out;
}

// importedBy(id) -> package nodes that point at id (sources of edges into id),
// deduplicated.
export function importedBy(id: string): GraphPackage[] {
  const graph = getGraph();
  const index = packageIndex(graph);
  const seen = new Set<string>();
  const out: GraphPackage[] = [];
  for (const e of graph.edges) {
    if (e.target === id) {
      const source = index.get(e.source);
      if (source && !seen.has(source.id)) {
        seen.add(source.id);
        out.push(source);
      }
    }
  }
  return out;
}

export interface RelatedEdge {
  to: GraphPackage;
  kind: string;
  weight: number | undefined;
}

// related(id) -> every neighbor edge touching id as { to, kind, weight },
// where `to` is the package node on the other end of the edge.
export function related(id: string): RelatedEdge[] {
  const graph = getGraph();
  const index = packageIndex(graph);
  const out: RelatedEdge[] = [];
  for (const e of graph.edges) {
    let otherId: string | null = null;
    if (e.source === id) otherId = e.target;
    else if (e.target === id) otherId = e.source;
    else continue;
    const to = index.get(otherId);
    if (to) out.push({ to, kind: e.kind, weight: e.weight });
  }
  return out;
}

export interface SubgraphNode {
  id: string;
  label: string;
  library: string;
  symbolCount: number;
}
export interface SubgraphEdge {
  source: string;
  target: string;
  kind: string;
  weight: number;
}
export interface Subgraph {
  nodes: SubgraphNode[];
  edges: SubgraphEdge[];
}

// subgraph(library) -> { nodes, edges } for the GraphQL Graph type.
// When library is falsy, the whole graph is returned. Nodes are mapped to the
// GraphNode shape { id, label, library, symbolCount }; edges to the GraphEdge
// shape { source, target, kind, weight } and restricted to those whose both
// endpoints are within the selected node set.
export function subgraph(library?: string): Subgraph {
  const graph = getGraph();
  let nodes = graph.packages;
  if (library) {
    nodes = nodes.filter((p) => p.library === library);
  }
  const ids = new Set(nodes.map((p) => p.id));
  const edges = graph.edges.filter((e) => ids.has(e.source) && ids.has(e.target));

  return {
    nodes: nodes.map((p) => ({
      id: p.id,
      label: p.name,
      library: p.library,
      symbolCount: p.symbolCount || 0,
    })),
    edges: edges.map((e) => ({
      source: e.source,
      target: e.target,
      kind: e.kind,
      weight: e.weight || 1,
    })),
  };
}
