// app/api/graphql/route.ts — Next.js Route Handler port of api/graphql.js.
//
// Serves the package-connection graph GraphQL API over graphql-js, reusing the
// shared ESM libs in /workspace/go/api/_lib. Mirrors the SDL/resolvers of the
// original Vercel serverless function exactly:
//   - POST /api/graphql  { query, variables, operationName }
//   - GET  /api/graphql?query=...&variables=...&operationName=...
//   - OPTIONS preflight -> 204
// Responds with { data, errors } JSON and permissive CORS. All graph reads go
// through graphstore.js; `search` uses Elasticsearch (es.js) when enabled and
// falls back to the in-memory BM25 backend (bm25.js) over getSymbols().

import {
  GraphQLSchema,
  GraphQLObjectType,
  GraphQLNonNull,
  GraphQLList,
  GraphQLID,
  GraphQLString,
  GraphQLInt,
  GraphQLFloat,
  graphql,
} from 'graphql';

import {
  pkg as storePkg,
  packages as storePackages,
  libraries as storeLibraries,
  importsOf,
  importedBy,
  related,
  subgraph,
} from '../../../api/_lib/graphstore';

import { getSymbols } from '../../../api/_lib/data';
import { esEnabled, esSearch } from '../../../api/_lib/es';
import { search as bm25Search } from '../../../api/_lib/bm25';

// Run on the Node.js runtime (the shared libs use node built-ins), and never
// pre-render / cache — every request executes the resolvers live.
export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

// ---------------------------------------------------------------------------
// Object types
// ---------------------------------------------------------------------------

// type Library { id: ID!, name: String!, packageCount: Int!, symbolCount: Int!, parityAfter: String }
const LibraryType = new GraphQLObjectType({
  name: 'Library',
  fields: () => ({
    id: { type: new GraphQLNonNull(GraphQLID) },
    name: { type: new GraphQLNonNull(GraphQLString) },
    packageCount: { type: new GraphQLNonNull(GraphQLInt) },
    symbolCount: { type: new GraphQLNonNull(GraphQLInt) },
    parityAfter: { type: GraphQLString },
  }),
});

// type Package { id, importPath, name, library, synopsis, symbolCount,
//                imports: [Package!]!, importedBy: [Package!]!, related: [Edge!]! }
const PackageType: GraphQLObjectType = new GraphQLObjectType({
  name: 'Package',
  fields: () => ({
    id: { type: new GraphQLNonNull(GraphQLID) },
    importPath: { type: new GraphQLNonNull(GraphQLString) },
    name: { type: new GraphQLNonNull(GraphQLString) },
    library: { type: new GraphQLNonNull(GraphQLString) },
    synopsis: { type: GraphQLString },
    symbolCount: {
      type: new GraphQLNonNull(GraphQLInt),
      resolve: (p: any) => p.symbolCount || 0,
    },
    imports: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      resolve: (p: any) => importsOf(p.id),
    },
    importedBy: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      resolve: (p: any) => importedBy(p.id),
    },
    related: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(EdgeType))),
      resolve: (p: any) => related(p.id),
    },
  }),
});

// type Edge { to: Package!, kind: String!, weight: Int! }
const EdgeType: GraphQLObjectType = new GraphQLObjectType({
  name: 'Edge',
  fields: () => ({
    to: { type: new GraphQLNonNull(PackageType) },
    kind: { type: new GraphQLNonNull(GraphQLString) },
    weight: {
      type: new GraphQLNonNull(GraphQLInt),
      resolve: (e: any) => e.weight || 0,
    },
  }),
});

// type SearchHit { id, name, kind, packageImportPath, library, signature, doc, anchor, score: Float! }
const SearchHitType = new GraphQLObjectType({
  name: 'SearchHit',
  fields: () => ({
    id: { type: new GraphQLNonNull(GraphQLID) },
    name: { type: new GraphQLNonNull(GraphQLString) },
    kind: { type: new GraphQLNonNull(GraphQLString) },
    packageImportPath: { type: new GraphQLNonNull(GraphQLString) },
    library: { type: new GraphQLNonNull(GraphQLString) },
    signature: { type: GraphQLString },
    doc: { type: GraphQLString },
    anchor: { type: GraphQLString },
    score: {
      type: new GraphQLNonNull(GraphQLFloat),
      resolve: (h: any) => (typeof h.score === 'number' ? h.score : 0),
    },
  }),
});

// type GraphNode { id: ID!, label: String!, library: String!, symbolCount: Int! }
const GraphNodeType = new GraphQLObjectType({
  name: 'GraphNode',
  fields: () => ({
    id: { type: new GraphQLNonNull(GraphQLID) },
    label: { type: new GraphQLNonNull(GraphQLString) },
    library: { type: new GraphQLNonNull(GraphQLString) },
    symbolCount: {
      type: new GraphQLNonNull(GraphQLInt),
      resolve: (n: any) => n.symbolCount || 0,
    },
  }),
});

// type GraphEdge { source: ID!, target: ID!, kind: String!, weight: Int! }
const GraphEdgeType = new GraphQLObjectType({
  name: 'GraphEdge',
  fields: () => ({
    source: { type: new GraphQLNonNull(GraphQLID) },
    target: { type: new GraphQLNonNull(GraphQLID) },
    kind: { type: new GraphQLNonNull(GraphQLString) },
    weight: {
      type: new GraphQLNonNull(GraphQLInt),
      resolve: (e: any) => e.weight || 0,
    },
  }),
});

// type Graph { nodes: [GraphNode!]!, edges: [GraphEdge!]! }
const GraphType = new GraphQLObjectType({
  name: 'Graph',
  fields: () => ({
    nodes: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(GraphNodeType))),
    },
    edges: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(GraphEdgeType))),
    },
  }),
});

// ---------------------------------------------------------------------------
// search resolver: Elasticsearch when enabled, BM25 fallback otherwise.
// ---------------------------------------------------------------------------

async function resolveSearch(q: unknown, first: unknown): Promise<any[]> {
  const firstNum = typeof first === 'number' ? first : Number(first);
  const limit = Number.isFinite(firstNum) && firstNum > 0 ? Math.floor(firstNum) : 20;
  if (q == null || String(q).trim() === '') return [];

  const query = String(q);
  if (esEnabled()) {
    try {
      return await esSearch(query, limit);
    } catch {
      // Fall through to BM25 on any Elasticsearch failure.
    }
  }
  return bm25Search(getSymbols(), query, limit);
}

// ---------------------------------------------------------------------------
// Root Query
// ---------------------------------------------------------------------------

const QueryType = new GraphQLObjectType({
  name: 'Query',
  fields: () => ({
    package: {
      type: PackageType,
      args: { id: { type: new GraphQLNonNull(GraphQLID) } },
      resolve: (_root: unknown, { id }: any) => storePkg(id),
    },
    packages: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      args: {
        library: { type: GraphQLString },
        search: { type: GraphQLString },
        first: { type: GraphQLInt, defaultValue: 50 },
      },
      resolve: (_root: unknown, { library, search, first }: any) =>
        (storePackages as (o: { library?: string; search?: string; first?: number }) => unknown)({ library, search, first }),
    },
    libraries: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(LibraryType))),
      resolve: () => storeLibraries(),
    },
    search: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(SearchHitType))),
      args: {
        q: { type: new GraphQLNonNull(GraphQLString) },
        first: { type: GraphQLInt, defaultValue: 20 },
      },
      resolve: (_root: unknown, { q, first }: any) => resolveSearch(q, first),
    },
    graph: {
      type: new GraphQLNonNull(GraphType),
      args: { library: { type: GraphQLString } },
      resolve: (_root: unknown, { library }: any) => subgraph(library),
    },
  }),
});

const schema = new GraphQLSchema({ query: QueryType });

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'POST, GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  'Access-Control-Max-Age': '86400',
};

function jsonResponse(status: number, payload: unknown): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      ...CORS_HEADERS,
      'Content-Type': 'application/json; charset=utf-8',
    },
  });
}

function parseVariables(vars: unknown): Record<string, unknown> | undefined {
  if (vars == null || vars === '') return undefined;
  if (typeof vars === 'string') {
    try {
      return JSON.parse(vars);
    } catch {
      return undefined;
    }
  }
  if (typeof vars === 'object') return vars as Record<string, unknown>;
  return undefined;
}

// Parse a POST body: JSON object, JSON string, or empty -> {}. null signals
// a malformed JSON body (caller returns 400).
async function readBody(req: Request): Promise<any> {
  const raw = await req.text();
  if (raw == null || raw.trim() === '') return {};
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// GraphQL execution
// ---------------------------------------------------------------------------

async function execute(
  source: unknown,
  variableValues: Record<string, unknown> | undefined,
  operationName: unknown
): Promise<Response> {
  if (!source || String(source).trim() === '') {
    return jsonResponse(400, { errors: [{ message: 'Missing GraphQL query' }] });
  }

  try {
    const result = await graphql({
      schema,
      source: String(source),
      variableValues,
      operationName: (operationName as string) || undefined,
    });
    return jsonResponse(200, result);
  } catch (err: any) {
    return jsonResponse(500, {
      errors: [{ message: err && err.message ? err.message : 'Internal server error' }],
    });
  }
}

// ---------------------------------------------------------------------------
// Route Handlers
// ---------------------------------------------------------------------------

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function GET(req: Request): Promise<Response> {
  const params = new URL(req.url).searchParams;
  return execute(
    params.get('query'),
    parseVariables(params.get('variables')),
    params.get('operationName')
  );
}

export async function POST(req: Request): Promise<Response> {
  const body = await readBody(req);
  if (body == null) {
    return jsonResponse(400, { errors: [{ message: 'Invalid JSON body' }] });
  }
  return execute(body.query, parseVariables(body.variables), body.operationName);
}
