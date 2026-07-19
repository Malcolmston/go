#!/usr/bin/env node
// build-graph-data.mjs — generate the package-connection graph + search-symbol
// index that back the search endpoint, the GraphQL API and the frontend Explore
// tab. Run from the repo root:  node scripts/build-graph-data.mjs
//
// Reads:   web/public/docs/*.json   (DocIndex per library)
//          web/src/parity.ts        (best-effort, for parityAfter)
//          <library source dirs>     (best-effort, for real import edges)
// Writes:  api/_data/graph.json      + web/public/graph.json
//          api/_data/symbols.json    + web/public/search-index.json
//
// The output is DETERMINISTIC: everything is sorted by stable ids and no
// Date.now/Math.random is used for ordering. Only the free-form `generatedAt`
// timestamp field is time-derived (and is never used as a sort key); it can be
// pinned via the GRAPH_GENERATED_AT env var or `--generated-at <iso>` for fully
// reproducible builds.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, '..');
const DOCS_DIR = path.join(REPO_ROOT, 'web', 'public', 'docs');
const PARITY_TS = path.join(REPO_ROOT, 'web', 'src', 'parity.ts');
const OUT_DIRS = [
  path.join(REPO_ROOT, 'api', '_data'),
  path.join(REPO_ROOT, 'web', 'public'),
];

// Cap applied to reference-edge weights so a package that mentions a neighbour's
// types dozens of times does not dominate the layout.
const REFERENCE_WEIGHT_CAP = 10;

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

function resolveGeneratedAt() {
  const argv = process.argv.slice(2);
  const i = argv.indexOf('--generated-at');
  if (i !== -1 && argv[i + 1]) return argv[i + 1];
  if (process.env.GRAPH_GENERATED_AT) return process.env.GRAPH_GENERATED_AT;
  return new Date().toISOString();
}

// Anchor computation must match the docs renderer:
//   value / func  => sym-<name>
//   type          => sym-<Type>
//   method        => sym-<recvBase>.<method>
function valueAnchor(name) {
  return `sym-${name}`;
}
function typeAnchor(typeName) {
  return `sym-${typeName}`;
}
function methodAnchor(recv, methodName) {
  return `sym-${recvBase(recv)}.${methodName}`;
}
// "*Application", "Application[T]", "*Store[K, V]" => "Application" / "Store"
function recvBase(recv) {
  if (!recv) return '';
  let s = String(recv).trim();
  s = s.replace(/^[*&]+/, '');       // drop pointer/ref markers
  s = s.replace(/\[.*$/, '');         // drop generic type params
  s = s.trim();
  return s;
}

function isInterfaceSig(sig) {
  if (!sig) return false;
  // A declared interface type: `type Foo interface { ... }` (as opposed to a
  // struct/alias whose body merely mentions an interface elsewhere).
  return /^\s*type\s+\S+[^={]*\binterface\b/.test(sig) || /\binterface\s*\{/.test(sig);
}

function segmentsAfter(importPath, module) {
  if (importPath === module) return 0;
  if (module && importPath.startsWith(module + '/')) {
    return importPath.slice(module.length + 1).split('/').length;
  }
  // Fallback: total path depth.
  return importPath.split('/').length;
}

// ---------------------------------------------------------------------------
// parity (best-effort)
// ---------------------------------------------------------------------------

// Parse the PARITY map out of parity.ts with a regex; on any trouble return {}.
function loadParity() {
  const out = {};
  let src;
  try {
    src = fs.readFileSync(PARITY_TS, 'utf8');
  } catch {
    return out;
  }
  const start = src.indexOf('PARITY');
  const body = start === -1 ? src : src.slice(start);
  // Match:  key: { ... after: "100%" ... }   where key is bare or quoted.
  const entry = /(?:^|[,{]\s*)(?:'([^']+)'|"([^"]+)"|([A-Za-z0-9_.$]+))\s*:\s*\{([^}]*)\}/g;
  let m;
  while ((m = entry.exec(body)) !== null) {
    const key = m[1] || m[2] || m[3];
    const inner = m[4] || '';
    if (!key || key === 'PARITY') continue;
    const afterMatch = inner.match(/\bafter\s*:\s*['"]([^'"]*)['"]/);
    const upstreamMatch = inner.match(/\bupstream\s*:\s*['"]([^'"]*)['"]/);
    if (afterMatch || upstreamMatch) {
      out[key] = {
        after: afterMatch ? afterMatch[1] : null,
        upstream: upstreamMatch ? upstreamMatch[1] : null,
      };
    }
  }
  return out;
}

// ---------------------------------------------------------------------------
// symbol extraction
// ---------------------------------------------------------------------------

// Produce every exported search symbol for one package (excluding the package
// node itself, which is emitted separately by the caller).
function symbolsForPackage(pkg, library) {
  const importPath = pkg.importPath;
  const out = [];

  const pushValue = (group, kind) => {
    if (!group) return;
    const sig = group.signature || '';
    const doc = group.doc || '';
    for (const name of group.names || []) {
      if (!name) continue;
      out.push({
        id: `${importPath}#${valueAnchor(name)}`,
        name,
        kind,
        packageImportPath: importPath,
        library,
        signature: sig,
        doc,
        anchor: valueAnchor(name),
      });
    }
  };

  const pushFunc = (fn) => {
    if (!fn || !fn.name) return;
    out.push({
      id: `${importPath}#${valueAnchor(fn.name)}`,
      name: fn.name,
      kind: 'func',
      packageImportPath: importPath,
      library,
      signature: fn.signature || '',
      doc: fn.doc || '',
      anchor: valueAnchor(fn.name),
    });
  };

  const pushMethod = (mth) => {
    if (!mth || !mth.name) return;
    const anchor = methodAnchor(mth.recv, mth.name);
    const base = recvBase(mth.recv);
    out.push({
      id: `${importPath}#${anchor}`,
      name: base ? `${base}.${mth.name}` : mth.name,
      kind: 'method',
      packageImportPath: importPath,
      library,
      signature: mth.signature || '',
      doc: mth.doc || '',
      anchor,
    });
  };

  // Types (and interfaces), plus symbols nested under a type.
  for (const t of pkg.types || []) {
    if (t.name) {
      out.push({
        id: `${importPath}#${typeAnchor(t.name)}`,
        name: t.name,
        kind: isInterfaceSig(t.signature) ? 'interface' : 'type',
        packageImportPath: importPath,
        library,
        signature: t.signature || '',
        doc: t.doc || '',
        anchor: typeAnchor(t.name),
      });
    }
    for (const c of t.consts || []) pushValue(c, 'const');
    for (const v of t.vars || []) pushValue(v, 'var');
    for (const fn of t.funcs || []) pushFunc(fn);
    for (const mth of t.methods || []) pushMethod(mth);
  }

  // Package-level values and funcs.
  for (const c of pkg.consts || []) pushValue(c, 'const');
  for (const v of pkg.vars || []) pushValue(v, 'var');
  for (const fn of pkg.funcs || []) pushFunc(fn);

  return out;
}

// Concatenated signature text of a package, used for reference-edge detection.
function signatureBlob(pkg) {
  const parts = [];
  for (const t of pkg.types || []) {
    if (t.signature) parts.push(t.signature);
    for (const c of t.consts || []) if (c.signature) parts.push(c.signature);
    for (const v of t.vars || []) if (v.signature) parts.push(v.signature);
    for (const fn of t.funcs || []) if (fn.signature) parts.push(fn.signature);
    for (const mth of t.methods || []) if (mth.signature) parts.push(mth.signature);
  }
  for (const c of pkg.consts || []) if (c.signature) parts.push(c.signature);
  for (const v of pkg.vars || []) if (v.signature) parts.push(v.signature);
  for (const fn of pkg.funcs || []) if (fn.signature) parts.push(fn.signature);
  return parts.join('\n');
}

function exportedTypeNames(pkg) {
  const names = [];
  for (const t of pkg.types || []) if (t.name) names.push(t.name);
  return names;
}

// ---------------------------------------------------------------------------
// import-edge parsing (best-effort — only when Go source is on disk)
// ---------------------------------------------------------------------------

function goImportsForDir(dir) {
  const counts = new Map(); // importPath -> number of files importing it
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return counts;
  }
  for (const ent of entries) {
    if (!ent.isFile()) continue;
    if (!ent.name.endsWith('.go')) continue;
    if (ent.name.endsWith('_test.go')) continue;
    let src;
    try {
      src = fs.readFileSync(path.join(dir, ent.name), 'utf8');
    } catch {
      continue;
    }
    const seen = new Set();
    // Grouped import blocks: import ( "a" \n _ "b" \n alias "c" )
    const blockRe = /import\s*\(([\s\S]*?)\)/g;
    let bm;
    while ((bm = blockRe.exec(src)) !== null) {
      const inner = bm[1];
      const strRe = /"([^"]+)"/g;
      let sm;
      while ((sm = strRe.exec(inner)) !== null) seen.add(sm[1]);
    }
    // Single-line imports: import "a"  /  import alias "a"
    const singleRe = /import\s+(?:[A-Za-z0-9_.]+\s+)?"([^"]+)"/g;
    let sm2;
    while ((sm2 = singleRe.exec(src)) !== null) seen.add(sm2[1]);

    for (const p of seen) counts.set(p, (counts.get(p) || 0) + 1);
  }
  return counts;
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

function main() {
  const generatedAt = resolveGeneratedAt();
  const parity = loadParity();

  let docFiles;
  try {
    docFiles = fs.readdirSync(DOCS_DIR).filter((f) => f.endsWith('.json'));
  } catch (err) {
    console.error(`build-graph-data: cannot read docs dir ${DOCS_DIR}: ${err.message}`);
    process.exit(1);
  }
  docFiles.sort();

  const libraries = [];
  const packages = [];
  const edges = [];
  const symbols = [];

  const packageIds = new Set();          // every package importPath in the graph
  // Per-library working state we need for a second edge-building pass.
  const libState = []; // { id, module, pkgs:[{importPath,name,rawPkg}], rootId, sourceDir }

  for (const file of docFiles) {
    const library = file.replace(/\.json$/, ''); // filename stem, e.g. "socket.io"
    let doc;
    try {
      doc = JSON.parse(fs.readFileSync(path.join(DOCS_DIR, file), 'utf8'));
    } catch (err) {
      console.error(`build-graph-data: skipping ${file}: ${err.message}`);
      continue;
    }
    const module = doc.module || '';
    const pkgs = Array.isArray(doc.packages) ? doc.packages : [];

    // Determine the library "root" package: importPath === module, else the
    // package with the fewest segments beyond the module (lexical tie-break).
    let rootPkg = null;
    for (const p of pkgs) {
      if (!p || !p.importPath) continue;
      if (p.importPath === module) { rootPkg = p; break; }
    }
    if (!rootPkg) {
      for (const p of pkgs) {
        if (!p || !p.importPath) continue;
        if (
          rootPkg === null ||
          segmentsAfter(p.importPath, module) < segmentsAfter(rootPkg.importPath, module) ||
          (segmentsAfter(p.importPath, module) === segmentsAfter(rootPkg.importPath, module) &&
            p.importPath < rootPkg.importPath)
        ) {
          rootPkg = p;
        }
      }
    }
    const rootId = rootPkg ? rootPkg.importPath : null;

    const state = {
      id: library,
      module,
      pkgs: [],
      rootId,
      sourceDir: path.join(REPO_ROOT, library), // best-effort Go source location
    };

    let librarySymbolCount = 0;

    for (const p of pkgs) {
      if (!p || !p.importPath) continue;
      const importPath = p.importPath;
      packageIds.add(importPath);

      // Package node = also a search symbol of kind "package".
      symbols.push({
        id: importPath,
        name: p.name || importPath,
        kind: 'package',
        packageImportPath: importPath,
        library,
        signature: '',
        doc: p.synopsis || p.doc || '',
        anchor: '',
      });

      const memberSyms = symbolsForPackage(p, library);
      for (const s of memberSyms) symbols.push(s);

      const symbolCount = memberSyms.length;
      librarySymbolCount += symbolCount;

      packages.push({
        id: importPath,
        importPath,
        name: p.name || importPath,
        library,
        synopsis: p.synopsis || '',
        symbolCount,
      });

      state.pkgs.push({
        importPath,
        name: p.name || '',
        blob: signatureBlob(p),
        typeNames: exportedTypeNames(p),
      });
    }

    const parEntry = parity[library] || null;
    libraries.push({
      id: library,
      name: library,
      packageCount: state.pkgs.length,
      symbolCount: librarySymbolCount,
      parityAfter: parEntry && parEntry.after ? parEntry.after : null,
    });

    libState.push(state);
  }

  // -------- edges: same-library star --------
  for (const st of libState) {
    if (!st.rootId) continue;
    for (const p of st.pkgs) {
      if (p.importPath === st.rootId) continue;
      edges.push({ source: p.importPath, target: st.rootId, kind: 'same-library', weight: 1 });
    }
  }

  // -------- edges: reference (same-library type usage) --------
  for (const st of libState) {
    for (const a of st.pkgs) {
      if (!a.blob) continue;
      for (const b of st.pkgs) {
        if (a.importPath === b.importPath) continue;
        if (!b.name || b.typeNames.length === 0) continue;
        let weight = 0;
        for (const tn of b.typeNames) {
          const token = `${b.name}.${tn}`;
          let idx = a.blob.indexOf(token);
          while (idx !== -1) {
            // Ensure the char before is not an identifier char (so `xyzb.T`
            // does not match `b.T`).
            const prev = idx > 0 ? a.blob[idx - 1] : '';
            if (!/[A-Za-z0-9_.]/.test(prev)) weight++;
            idx = a.blob.indexOf(token, idx + token.length);
          }
        }
        if (weight > 0) {
          edges.push({
            source: a.importPath,
            target: b.importPath,
            kind: 'reference',
            weight: Math.min(weight, REFERENCE_WEIGHT_CAP),
          });
        }
      }
    }
  }

  // -------- edges: import (best-effort, from Go source) --------
  let importEdgeCount = 0;
  for (const st of libState) {
    for (const p of st.pkgs) {
      // Map importPath -> on-disk directory under the library source dir.
      let sub = '';
      if (st.module && p.importPath === st.module) sub = '';
      else if (st.module && p.importPath.startsWith(st.module + '/')) sub = p.importPath.slice(st.module.length + 1);
      else continue; // cannot map -> skip
      const dir = sub ? path.join(st.sourceDir, sub) : st.sourceDir;
      const counts = goImportsForDir(dir);
      for (const [target, n] of counts) {
        if (target === p.importPath) continue;
        if (!packageIds.has(target)) continue; // only edges to known graph nodes
        edges.push({ source: p.importPath, target, kind: 'import', weight: n });
        importEdgeCount++;
      }
    }
  }

  // -------- edges: shared-upstream (same upstream org) --------
  const byOrg = new Map(); // org -> [rootId,...]
  for (const st of libState) {
    if (!st.rootId) continue;
    const par = parity[st.id];
    if (!par || !par.upstream) continue;
    const org = String(par.upstream).split('/')[0];
    if (!org) continue;
    if (!byOrg.has(org)) byOrg.set(org, []);
    byOrg.get(org).push(st.rootId);
  }
  for (const roots of byOrg.values()) {
    if (roots.length < 2) continue;
    const uniq = Array.from(new Set(roots)).sort();
    for (const source of uniq) {
      for (const target of uniq) {
        if (source === target) continue;
        edges.push({ source, target, kind: 'shared-upstream', weight: 1 });
      }
    }
  }

  // -------- dedup + deterministic ordering --------
  // Collapse duplicate edges (same source/target/kind), summing weight.
  const edgeMap = new Map();
  for (const e of edges) {
    const key = `${e.source} ${e.target} ${e.kind}`;
    const prev = edgeMap.get(key);
    if (prev) prev.weight += e.weight;
    else edgeMap.set(key, { ...e });
  }
  const finalEdges = Array.from(edgeMap.values()).sort(
    (x, y) =>
      x.source < y.source ? -1 : x.source > y.source ? 1 :
      x.target < y.target ? -1 : x.target > y.target ? 1 :
      x.kind < y.kind ? -1 : x.kind > y.kind ? 1 : 0
  );

  // Dedup symbols by id (defensive) and sort by id.
  const symMap = new Map();
  for (const s of symbols) if (!symMap.has(s.id)) symMap.set(s.id, s);
  const finalSymbols = Array.from(symMap.values()).sort(
    (a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
  );

  const finalPackages = packages.slice().sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  const finalLibraries = libraries.slice().sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));

  const graph = {
    generatedAt,
    libraries: finalLibraries,
    packages: finalPackages,
    edges: finalEdges,
  };
  const symbolIndex = {
    generatedAt,
    symbols: finalSymbols,
  };

  // -------- write to both destinations --------
  for (const dir of OUT_DIRS) {
    fs.mkdirSync(dir, { recursive: true });
  }
  const graphJson = JSON.stringify(graph, null, 2) + '\n';
  const symbolsJson = JSON.stringify(symbolIndex, null, 2) + '\n';

  fs.writeFileSync(path.join(OUT_DIRS[0], 'graph.json'), graphJson);
  fs.writeFileSync(path.join(OUT_DIRS[0], 'symbols.json'), symbolsJson);
  // web/public copies use the frontend-fallback filenames.
  fs.writeFileSync(path.join(OUT_DIRS[1], 'graph.json'), graphJson);
  fs.writeFileSync(path.join(OUT_DIRS[1], 'search-index.json'), symbolsJson);

  const kindCounts = {};
  for (const e of finalEdges) kindCounts[e.kind] = (kindCounts[e.kind] || 0) + 1;
  const kindStr = ['import', 'reference', 'same-library', 'shared-upstream']
    .map((k) => `${k}=${kindCounts[k] || 0}`)
    .join(' ');
  console.log(
    `build-graph-data: ${finalLibraries.length} libraries, ${finalPackages.length} packages, ` +
    `${finalSymbols.length} symbols, ${finalEdges.length} edges (${kindStr}; ` +
    `${importEdgeCount} raw import edges from source). Wrote graph.json + symbols.json ` +
    `to api/_data and web/public.`
  );
}

main();
