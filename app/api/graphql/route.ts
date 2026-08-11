// app/api/graphql/route.ts — Next.js Route Handler port of api/graphql.js.
//
// Serves the package-connection graph GraphQL API over graphql-js, reusing the
// shared ESM libs in /workspace/go/api/_lib. Mirrors the SDL/resolvers of the
// original Vercel serverless function exactly:
//   - POST /api/graphql  { query, variables, operationName }
//   - GET  /api/graphql?query=...&variables=...&operationName=...
//   - OPTIONS preflight -> 204
// Responds with { data, errors } JSON and permissive CORS. All graph reads go
// through graphstore.js; `search` uses Upstash Search (es.ts) when enabled and
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
  GraphQLError,
  parse,
  validate,
  execute as executeDocument,
  specifiedRules,
  type DocumentNode,
  type ValidationRule,
} from 'graphql';

import { depthLimitRule, MAX_QUERY_DEPTH } from './depthLimit';

import {
  pkg as storePkg,
  packages as storePackages,
  libraries as storeLibraries,
  importsOf,
  importedBy,
  related,
  subgraph,
} from '../../../api/_lib/graphstore';
import type { RelatedEdge, SubgraphEdge, SubgraphNode } from '../../../api/_lib/graphstore';

import { getSymbols } from '../../../api/_lib/data';
import type { GraphPackage } from '../../../api/_lib/data';
import { esEnabled, esSearch } from '../../../api/_lib/es';
import type { SearchHit } from '../../../api/_lib/es';
import { search as bm25Search } from '../../../api/_lib/bm25';
import type { SearchResult } from '../../../api/_lib/bm25';

// A hit as the `search` field returns it: Upstash-backed or BM25-backed. Both
// satisfy the SearchHit SDL type; the union avoids widening to `any`.
type GqlSearchHit = SearchHit | SearchResult;

// Arguments of the `packages` root field.
interface PackagesArgs {
  library?: string;
  search?: string;
  first?: number;
}

// The three fields a GraphQL-over-HTTP request body may carry.
interface GraphQLRequestBody {
  query?: unknown;
  variables?: unknown;
  operationName?: unknown;
}

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
      resolve: (p: GraphPackage) => p.symbolCount || 0,
    },
    imports: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      resolve: (p: GraphPackage) => importsOf(p.id),
    },
    importedBy: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      resolve: (p: GraphPackage) => importedBy(p.id),
    },
    related: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(EdgeType))),
      resolve: (p: GraphPackage) => related(p.id),
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
      resolve: (e: RelatedEdge) => e.weight || 0,
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
      resolve: (h: { score?: unknown }) => (typeof h.score === 'number' ? h.score : 0),
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
      resolve: (n: SubgraphNode) => n.symbolCount || 0,
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
      resolve: (e: SubgraphEdge) => e.weight || 0,
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
// search resolver: Upstash Search when enabled, BM25 fallback otherwise.
// ---------------------------------------------------------------------------

// Hard caps on caller-supplied list sizes. GraphQL `Int` accepts up to 2^31-1,
// so without these a single `search(first: 2000000000)` or
// `packages(first: 2000000000)` would ask the backend for the entire corpus.
const MAX_LIST_SIZE = 100;

// Upper bound on the GraphQL document text and on the serialized `variables`
// blob. Both are parsed before anything else runs, so they need a ceiling.
const MAX_QUERY_LENGTH = 8_000;
const MAX_VARIABLES_LENGTH = 16_000;
const MAX_BODY_LENGTH = 64_000;

// Clamp a caller-supplied `first` into [1, MAX_LIST_SIZE], defaulting when the
// value is absent or not a finite positive number.
function clampFirst(first: unknown, fallback: number): number {
  const n = typeof first === 'number' ? first : Number(first);
  if (!Number.isFinite(n) || n <= 0) return fallback;
  return Math.min(Math.floor(n), MAX_LIST_SIZE);
}

async function resolveSearch(q: unknown, first: unknown): Promise<GqlSearchHit[]> {
  const limit = clampFirst(first, 20);
  if (q == null || String(q).trim() === '') return [];

  const query = String(q);
  if (esEnabled()) {
    try {
      return await esSearch(query, limit);
    } catch {
      // Fall through to BM25 on any Upstash Search failure.
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
      resolve: (_root: unknown, { id }: { id: string }) => storePkg(id),
    },
    packages: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      args: {
        library: { type: GraphQLString },
        search: { type: GraphQLString },
        first: { type: GraphQLInt, defaultValue: 50 },
      },
      resolve: (_root: unknown, { library, search, first }: PackagesArgs) =>
        storePackages({ library, search, first: clampFirst(first, 50) }),
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
      resolve: (_root: unknown, { q, first }: { q: string; first?: number }) => resolveSearch(q, first),
    },
    graph: {
      type: new GraphQLNonNull(GraphType),
      args: { library: { type: GraphQLString } },
      resolve: (_root: unknown, { library }: { library?: string }) => subgraph(library),
    },
  }),
});

const schema = new GraphQLSchema({ query: QueryType });

// Spec validation plus the cyclic-schema depth guard (see ./depthLimit).
const VALIDATION_RULES: readonly ValidationRule[] = [
  ...specifiedRules,
  depthLimitRule(MAX_QUERY_DEPTH),
];

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

// Normalize `variables` into an object, or undefined. A string form (used by the
// GET transport) is length-bounded before parsing and must decode to a plain
// object — an array or scalar is not a valid variables map.
function parseVariables(vars: unknown): Record<string, unknown> | undefined {
  if (vars == null || vars === '') return undefined;
  if (typeof vars === 'string') {
    if (vars.length > MAX_VARIABLES_LENGTH) return undefined;
    try {
      const parsed: unknown = JSON.parse(vars);
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
        ? (parsed as Record<string, unknown>)
        : undefined;
    } catch {
      return undefined;
    }
  }
  if (typeof vars === 'object' && !Array.isArray(vars)) return vars as Record<string, unknown>;
  return undefined;
}

// Parse a POST body: JSON object, or empty -> {}. Returns null for a malformed
// or oversized body (caller returns 400). Reading is wrapped because a client
// that aborts mid-upload makes req.text() reject, which would otherwise surface
// as an unhandled rejection / 500.
async function readBody(req: Request): Promise<GraphQLRequestBody | null> {
  let raw: string;
  try {
    raw = await req.text();
  } catch {
    return null;
  }
  if (raw == null || raw.trim() === '') return {};
  if (raw.length > MAX_BODY_LENGTH) return null;
  try {
    const parsed: unknown = JSON.parse(raw);
    // Only an object body carries {query, variables, operationName}.
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
    return parsed;
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// GraphQL execution
// ---------------------------------------------------------------------------

// graphql-js reports a resolver that threw by wrapping the raw Error as
// `originalError` and surfacing its message verbatim. For an arbitrary Error
// that message is internal — it can carry a connection string, a filesystem
// path, or an upstream URL — so it is logged server-side and replaced with a
// fixed one. Errors graphql-js authored itself — no originalError (variable
// coercion, unknown operation name) or an originalError that is already a
// GraphQLError (a resolver deliberately raising a user-facing error) — describe
// the caller's own request and are returned unchanged.
//
// The `path` is always preserved: it names a field in the caller's query, so it
// discloses nothing, and clients need it to attribute the failure.
//
// Schema/data faults such as "cannot return null for a non-nullable field" are
// raised as plain Errors and so are masked too. That is deliberate: they signal
// a corpus/deploy defect, and the operator needs the log line, not the caller.
function scrubError(err: GraphQLError): { message: string; path?: readonly (string | number)[] } {
  const internal = err.originalError && !(err.originalError instanceof GraphQLError);
  if (!internal) {
    return err.path ? { message: err.message, path: err.path } : { message: err.message };
  }
  console.error('[graphql] resolver error', err.originalError);
  const message = 'Internal server error';
  return err.path ? { message, path: err.path } : { message };
}

// Run one GraphQL request: bound the source, parse, validate (spec rules plus
// the depth limit), then execute. Client-fault conditions (missing/oversized/
// unparseable/invalid query) return 400 with the GraphQL error — those messages
// describe the caller's own document and are safe to echo. An unexpected
// server-side throw returns 500 with a fixed message and is logged, so no
// internal detail (stack, env value, upstream URL) reaches the client.
async function execute(
  source: unknown,
  variableValues: Record<string, unknown> | undefined,
  operationName: unknown
): Promise<Response> {
  if (!source || String(source).trim() === '') {
    return jsonResponse(400, { errors: [{ message: 'Missing GraphQL query' }] });
  }

  const text = String(source);
  if (text.length > MAX_QUERY_LENGTH) {
    return jsonResponse(400, {
      errors: [{ message: `Query is too large (max ${MAX_QUERY_LENGTH} characters).` }],
    });
  }

  let document: DocumentNode;
  try {
    document = parse(text);
  } catch (err) {
    const message = err instanceof GraphQLError ? err.message : 'Syntax error in GraphQL query';
    return jsonResponse(400, { errors: [{ message }] });
  }

  const validationErrors = validate(schema, document, VALIDATION_RULES);
  if (validationErrors.length > 0) {
    return jsonResponse(400, { errors: validationErrors.map((e) => ({ message: e.message })) });
  }

  try {
    const result = await executeDocument({
      schema,
      document,
      variableValues,
      operationName: (operationName as string) || undefined,
    });
    if (result.errors && result.errors.length > 0) {
      return jsonResponse(200, { ...result, errors: result.errors.map(scrubError) });
    }
    return jsonResponse(200, result);
  } catch (err) {
    console.error('[graphql] execution failure', err);
    return jsonResponse(500, { errors: [{ message: 'Internal server error' }] });
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
    return jsonResponse(400, { errors: [{ message: 'Invalid or oversized JSON body' }] });
  }
  return execute(body.query, parseVariables(body.variables), body.operationName);
}
