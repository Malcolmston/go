// graphql.js — Vercel serverless GraphQL API over the package-connection graph.
//
// Implements the exact SDL from the shared contract using graphql-js
// (GraphQLSchema / GraphQLObjectType) so that object-type fields such as
// Package.imports / Package.importedBy / Package.related and Edge.to get proper
// resolvers. All graph reads go through api/_lib/graphstore.js; the `search`
// field uses Elasticsearch (api/_lib/es.js) when enabled and falls back to the
// in-memory BM25 backend (api/_lib/bm25.js) over getSymbols() on any failure.
//
// Handler (Node runtime, ESM):
//   - POST /api/graphql  { query, variables, operationName }
//   - GET  /api/graphql?query=...&variables=...&operationName=...
//   - OPTIONS preflight -> 204
// Responds with { data, errors } JSON and permissive CORS.

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
} from './_lib/graphstore.js';

import { getSymbols } from './_lib/data.js';
import { esEnabled, esSearch } from './_lib/es.js';
import { search as bm25Search } from './_lib/bm25.js';

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
const PackageType = new GraphQLObjectType({
  name: 'Package',
  fields: () => ({
    id: { type: new GraphQLNonNull(GraphQLID) },
    importPath: { type: new GraphQLNonNull(GraphQLString) },
    name: { type: new GraphQLNonNull(GraphQLString) },
    library: { type: new GraphQLNonNull(GraphQLString) },
    synopsis: { type: GraphQLString },
    symbolCount: {
      type: new GraphQLNonNull(GraphQLInt),
      resolve: (p) => p.symbolCount || 0,
    },
    imports: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      resolve: (p) => importsOf(p.id),
    },
    importedBy: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      resolve: (p) => importedBy(p.id),
    },
    related: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(EdgeType))),
      resolve: (p) => related(p.id),
    },
  }),
});

// type Edge { to: Package!, kind: String!, weight: Int! }
const EdgeType = new GraphQLObjectType({
  name: 'Edge',
  fields: () => ({
    to: { type: new GraphQLNonNull(PackageType) },
    kind: { type: new GraphQLNonNull(GraphQLString) },
    weight: {
      type: new GraphQLNonNull(GraphQLInt),
      resolve: (e) => e.weight || 0,
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
      resolve: (h) => (typeof h.score === 'number' ? h.score : 0),
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
      resolve: (n) => n.symbolCount || 0,
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
      resolve: (e) => e.weight || 0,
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

async function resolveSearch(q, first) {
  const limit = Number.isFinite(first) && first > 0 ? Math.floor(first) : 20;
  if (q == null || String(q).trim() === '') return [];

  if (esEnabled()) {
    try {
      return await esSearch(q, limit);
    } catch {
      // Fall through to BM25 on any Elasticsearch failure.
    }
  }
  return bm25Search(getSymbols(), q, limit);
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
      resolve: (_root, { id }) => storePkg(id),
    },
    packages: {
      type: new GraphQLNonNull(new GraphQLList(new GraphQLNonNull(PackageType))),
      args: {
        library: { type: GraphQLString },
        search: { type: GraphQLString },
        first: { type: GraphQLInt, defaultValue: 50 },
      },
      resolve: (_root, { library, search, first }) =>
        storePackages({ library, search, first }),
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
      resolve: (_root, { q, first }) => resolveSearch(q, first),
    },
    graph: {
      type: new GraphQLNonNull(GraphType),
      args: { library: { type: GraphQLString } },
      resolve: (_root, { library }) => subgraph(library),
    },
  }),
});

const schema = new GraphQLSchema({ query: QueryType });

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

function setCors(res) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'POST, GET, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  res.setHeader('Access-Control-Max-Age', '86400');
}

function sendJson(res, status, payload) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.end(JSON.stringify(payload));
}

// Parse the request body for POST. Vercel usually pre-parses JSON into req.body,
// but fall back to reading the raw stream so the function is robust standalone.
async function readBody(req) {
  if (req.body != null) {
    if (typeof req.body === 'string') {
      if (req.body.trim() === '') return {};
      try {
        return JSON.parse(req.body);
      } catch {
        return null;
      }
    }
    if (typeof req.body === 'object') return req.body;
  }

  const chunks = [];
  for await (const chunk of req) {
    chunks.push(typeof chunk === 'string' ? Buffer.from(chunk) : chunk);
  }
  const raw = Buffer.concat(chunks).toString('utf8');
  if (raw.trim() === '') return {};
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function parseVariables(vars) {
  if (vars == null || vars === '') return undefined;
  if (typeof vars === 'string') {
    try {
      return JSON.parse(vars);
    } catch {
      return undefined;
    }
  }
  return vars;
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

export default async function handler(req, res) {
  setCors(res);

  const method = (req.method || 'GET').toUpperCase();

  if (method === 'OPTIONS') {
    res.statusCode = 204;
    res.end();
    return;
  }

  let source;
  let variableValues;
  let operationName;

  try {
    if (method === 'POST') {
      const body = await readBody(req);
      if (body == null) {
        sendJson(res, 400, { errors: [{ message: 'Invalid JSON body' }] });
        return;
      }
      source = body.query;
      variableValues = parseVariables(body.variables);
      operationName = body.operationName;
    } else if (method === 'GET') {
      const query = req.query || {};
      source = query.query;
      variableValues = parseVariables(query.variables);
      operationName = query.operationName;
    } else {
      sendJson(res, 405, { errors: [{ message: `Method ${method} not allowed` }] });
      return;
    }

    if (!source || String(source).trim() === '') {
      sendJson(res, 400, { errors: [{ message: 'Missing GraphQL query' }] });
      return;
    }

    const result = await graphql({
      schema,
      source: String(source),
      variableValues,
      operationName: operationName || undefined,
    });

    sendJson(res, 200, result);
  } catch (err) {
    sendJson(res, 500, {
      errors: [{ message: err && err.message ? err.message : 'Internal server error' }],
    });
  }
}
