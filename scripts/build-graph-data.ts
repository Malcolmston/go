#!/usr/bin/env node
// build-graph-data.ts — generate the package-connection graph + search-symbol
// index that back the search endpoint, the GraphQL API and the frontend Explore
// tab. Run from the repo root (Node 22.6+, via native type stripping):
//   node --experimental-strip-types scripts/build-graph-data.ts
//
// Reads:   public/docs/*.json    (DocIndex per library)
//          src/parity.ts         (best-effort, for parityAfter)
//          <library source dirs>  (best-effort, for real import edges)
// Writes:  api/_data/graph.json      + public/graph.json
//          api/_data/symbols.json    + public/search-index.json
//
// The output is DETERMINISTIC: everything is sorted by stable ids and no
// Date.now/Math.random is used for ordering. Only the free-form `generatedAt`
// timestamp field is time-derived (and is never used as a sort key); it can be
// pinned via the GRAPH_GENERATED_AT env var or `--generated-at <iso>` for fully
// reproducible builds.
//
// This is developer tooling that runs under Node directly (not webpack), so it
// only uses erasable TypeScript (type annotations, interfaces, `as`) that Node's
// --experimental-strip-types removes at load time.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, '..');
const DOCS_DIR = path.join(REPO_ROOT, 'public', 'docs');
const EXAMPLES_DIR = path.join(REPO_ROOT, 'examples');
const PARITY_TS = path.join(REPO_ROOT, 'src', 'parity.ts');
const OUT_DIRS = [
  path.join(REPO_ROOT, 'api', '_data'),
  path.join(REPO_ROOT, 'public'),
];

// Caps for the embedded runnable-example corpus (examples.json). The example
// programs are real, complete main.go files (some ~50 KB); we embed them so the
// /ask tool needs no runtime fetch, but clip very large ones to keep the data
// file — which ships in the serverless bundle — to a sane size. The chat tool
// clips again before handing text to the model.
const EXAMPLE_CODE_MAX = 20_000; // chars of main.go kept per library
const EXAMPLE_README_MAX = 2_000; // chars of README kept per library

// Library ids are generated slugs; the examples/<id>/ dir name is the id. Guard
// the directory listing against anything that is not a plausible id.
const EXAMPLE_ID_RE = /^[a-z0-9][a-z0-9._-]{0,63}$/;

// Cap applied to reference-edge weights so a package that mentions a neighbour's
// types dozens of times does not dominate the layout.
const REFERENCE_WEIGHT_CAP = 10;

// ---------------------------------------------------------------------------
// input/output shapes (the DocIndex JSON is read untyped, then narrowed)
// ---------------------------------------------------------------------------

interface DocGroup {
  signature?: string;
  doc?: string;
  names?: string[];
}
interface DocFunc {
  name?: string;
  signature?: string;
  doc?: string;
}
interface DocMethod {
  name?: string;
  recv?: string;
  signature?: string;
  doc?: string;
}
interface DocType {
  name?: string;
  signature?: string;
  doc?: string;
  consts?: DocGroup[];
  vars?: DocGroup[];
  funcs?: DocFunc[];
  methods?: DocMethod[];
}
interface DocPackage {
  importPath: string;
  name?: string;
  synopsis?: string;
  doc?: string;
  types?: DocType[];
  consts?: DocGroup[];
  vars?: DocGroup[];
  funcs?: DocFunc[];
}
interface DocIndex {
  module?: string;
  packages?: DocPackage[];
}

interface SymbolOut {
  id: string;
  name: string;
  kind: string;
  packageImportPath: string;
  library: string;
  signature: string;
  doc: string;
  anchor: string;
}
interface EdgeOut {
  source: string;
  target: string;
  kind: string;
  weight: number;
}
interface PackageOut {
  id: string;
  importPath: string;
  name: string;
  library: string;
  synopsis: string;
  symbolCount: number;
}
interface LibraryOut {
  id: string;
  name: string;
  packageCount: number;
  symbolCount: number;
  parityAfter: string | null;
}
interface ParityEntry {
  after: string | null;
  upstream: string | null;
}

// One embedded runnable example, keyed in examples.json by library id.
interface ExampleOut {
  library: string;
  path: string;          // repo-relative path to the source, e.g. examples/express/main.go
  code: string;          // the main.go source (clipped)
  codeTruncated: boolean;
  readme: string;        // README text (clipped/summarized)
  readmeTruncated: boolean;
}

interface PkgState {
  importPath: string;
  name: string;
  blob: string;
  typeNames: string[];
}
interface LibStateEntry {
  id: string;
  module: string;
  pkgs: PkgState[];
  rootId: string | null;
  sourceDir: string;
}

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

function resolveGeneratedAt(): string {
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
function valueAnchor(name: string): string {
  return `sym-${name}`;
}
function typeAnchor(typeName: string): string {
  return `sym-${typeName}`;
}
function methodAnchor(recv: string | undefined, methodName: string): string {
  return `sym-${recvBase(recv)}.${methodName}`;
}
// "*Application", "Application[T]", "*Store[K, V]" => "Application" / "Store"
function recvBase(recv: string | undefined): string {
  if (!recv) return '';
  let s = String(recv).trim();
  s = s.replace(/^[*&]+/, '');       // drop pointer/ref markers
  s = s.replace(/\[.*$/, '');         // drop generic type params
  s = s.trim();
  return s;
}

function isInterfaceSig(sig: string | undefined): boolean {
  if (!sig) return false;
  // A declared interface type: `type Foo interface { ... }` (as opposed to a
  // struct/alias whose body merely mentions an interface elsewhere).
  return /^\s*type\s+\S+[^={]*\binterface\b/.test(sig) || /\binterface\s*\{/.test(sig);
}

function segmentsAfter(importPath: string, module: string): number {
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
function loadParity(): Record<string, ParityEntry> {
  const out: Record<string, ParityEntry> = {};
  let src: string;
  try {
    src = fs.readFileSync(PARITY_TS, 'utf8');
  } catch {
    return out;
  }
  const start = src.indexOf('PARITY');
  const body = start === -1 ? src : src.slice(start);
  // Match:  key: { ... after: "100%" ... }   where key is bare or quoted.
  const entry = /(?:^|[,{]\s*)(?:'([^']+)'|"([^"]+)"|([A-Za-z0-9_.$]+))\s*:\s*\{([^}]*)\}/g;
  let m: RegExpExecArray | null;
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
function symbolsForPackage(pkg: DocPackage, library: string): SymbolOut[] {
  const importPath = pkg.importPath;
  const out: SymbolOut[] = [];

  const pushValue = (group: DocGroup | undefined, kind: string): void => {
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

  const pushFunc = (fn: DocFunc | undefined): void => {
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

  const pushMethod = (mth: DocMethod | undefined): void => {
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
function signatureBlob(pkg: DocPackage): string {
  const parts: string[] = [];
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

function exportedTypeNames(pkg: DocPackage): string[] {
  const names: string[] = [];
  for (const t of pkg.types || []) if (t.name) names.push(t.name);
  return names;
}

// ---------------------------------------------------------------------------
// import-edge parsing (best-effort — only when Go source is on disk)
// ---------------------------------------------------------------------------

function goImportsForDir(dir: string): Map<string, number> {
  const counts = new Map<string, number>(); // importPath -> number of files importing it
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return counts;
  }
  for (const ent of entries) {
    if (!ent.isFile()) continue;
    if (!ent.name.endsWith('.go')) continue;
    if (ent.name.endsWith('_test.go')) continue;
    let src: string;
    try {
      src = fs.readFileSync(path.join(dir, ent.name), 'utf8');
    } catch {
      continue;
    }
    const seen = new Set<string>();
    // Grouped import blocks: import ( "a" \n _ "b" \n alias "c" )
    const blockRe = /import\s*\(([\s\S]*?)\)/g;
    let bm: RegExpExecArray | null;
    while ((bm = blockRe.exec(src)) !== null) {
      const inner = bm[1];
      const strRe = /"([^"]+)"/g;
      let sm: RegExpExecArray | null;
      while ((sm = strRe.exec(inner)) !== null) seen.add(sm[1]);
    }
    // Single-line imports: import "a"  /  import alias "a"
    const singleRe = /import\s+(?:[A-Za-z0-9_.]+\s+)?"([^"]+)"/g;
    let sm2: RegExpExecArray | null;
    while ((sm2 = singleRe.exec(src)) !== null) seen.add(sm2[1]);

    for (const p of seen) counts.set(p, (counts.get(p) || 0) + 1);
  }
  return counts;
}

// ---------------------------------------------------------------------------
// runnable examples (examples/<id>/main.go + README.md)
// ---------------------------------------------------------------------------

// Clip a string to `max` chars, appending a marker line when truncated. Returns
// the (possibly clipped) text and whether clipping happened.
function clipText(s: string, max: number): { text: string; truncated: boolean } {
  if (s.length <= max) return { text: s, truncated: false };
  return { text: s.slice(0, max).trimEnd() + '\n…\n', truncated: true };
}

// Read every examples/<id>/ dir that has a main.go and embed its source +
// README (clipped). `knownLibraries` is the set of library ids the docs pass
// produced; an example without a matching library is still emitted (it maps to
// a valid /lib/<id> page) but this lets us warn about drift. Deterministic:
// directories are processed in sorted order.
function buildExamples(knownLibraries: Set<string>): {
  examples: Record<string, ExampleOut>;
  missing: string[];
} {
  const examples: Record<string, ExampleOut> = {};
  const missing: string[] = []; // library ids with docs but no runnable example
  let dirs: fs.Dirent[] = [];
  try {
    dirs = fs.readdirSync(EXAMPLES_DIR, { withFileTypes: true });
  } catch {
    // No examples/ tree checked out — emit an empty-but-valid corpus.
    for (const id of Array.from(knownLibraries).sort()) missing.push(id);
    return { examples, missing };
  }

  const ids = dirs
    .filter((d) => d.isDirectory() && EXAMPLE_ID_RE.test(d.name))
    .map((d) => d.name)
    .sort();

  const seen = new Set<string>();
  for (const id of ids) {
    const mainPath = path.join(EXAMPLES_DIR, id, 'main.go');
    let code: string;
    try {
      code = fs.readFileSync(mainPath, 'utf8');
    } catch {
      continue; // no runnable program here (placeholder dir) — skip
    }
    let readme = '';
    try {
      readme = fs.readFileSync(path.join(EXAMPLES_DIR, id, 'README.md'), 'utf8');
    } catch {
      readme = '';
    }
    const clippedCode = clipText(code, EXAMPLE_CODE_MAX);
    const clippedReadme = clipText(readme, EXAMPLE_README_MAX);
    examples[id] = {
      library: id,
      path: `examples/${id}/main.go`,
      code: clippedCode.text,
      codeTruncated: clippedCode.truncated,
      readme: clippedReadme.text,
      readmeTruncated: clippedReadme.truncated,
    };
    seen.add(id);
  }

  for (const id of Array.from(knownLibraries).sort()) {
    if (!seen.has(id)) missing.push(id);
  }
  return { examples, missing };
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

function main(): void {
  const generatedAt = resolveGeneratedAt();
  const parity = loadParity();

  // This script is the `prebuild` hook, so anything it throws fails `pnpm build`
  // outright. A missing/empty public/docs is a *data* problem (the gendocs pass
  // has not run, or submodules were never checked out) and the app has bundled
  // fallbacks for it — so degrade to an empty-but-valid graph instead of taking
  // the whole build down. Set GRAPH_STRICT=1 (or pass --strict) in a pipeline
  // that genuinely requires the data, e.g. the Pages deploy.
  const strict = process.argv.includes('--strict') || process.env.GRAPH_STRICT === '1';
  let docFiles: string[] = [];
  try {
    // gendocs also writes an index.json alongside the per-library files. It is
    // a manifest, not a library, so exclude it — otherwise it is counted as a
    // 39th library and reported as "no go.mod on disk", which reads as a real
    // missing port.
    docFiles = fs
      .readdirSync(DOCS_DIR)
      .filter((f) => f.endsWith('.json') && f !== 'index.json');
  } catch (err) {
    const msg = `build-graph-data: cannot read docs dir ${DOCS_DIR}: ${(err as Error).message}`;
    if (strict) {
      console.error(`${msg} (GRAPH_STRICT is set — failing)`);
      process.exit(1);
    }
    console.warn(`${msg}\nbuild-graph-data: continuing with an empty graph/symbol index.`);
  }
  if (docFiles.length === 0) {
    const msg = `build-graph-data: no DocIndex JSON found in ${DOCS_DIR}. Run gendocs (see .github/workflows/pages.yml) after \`git submodule update --init --recursive\`.`;
    if (strict) {
      console.error(`${msg} (GRAPH_STRICT is set — failing)`);
      process.exit(1);
    }
    console.warn(msg);
  }
  docFiles.sort();

  const libraries: LibraryOut[] = [];
  const packages: PackageOut[] = [];
  const edges: EdgeOut[] = [];
  const symbols: SymbolOut[] = [];

  const packageIds = new Set<string>();  // every package importPath in the graph
  // Per-library working state we need for a second edge-building pass.
  const libState: LibStateEntry[] = [];

  for (const file of docFiles) {
    const library = file.replace(/\.json$/, ''); // filename stem, e.g. "socket.io"
    let doc: DocIndex;
    try {
      doc = JSON.parse(fs.readFileSync(path.join(DOCS_DIR, file), 'utf8')) as DocIndex;
    } catch (err) {
      console.error(`build-graph-data: skipping ${file}: ${(err as Error).message}`);
      continue;
    }
    const module = doc.module || '';
    const pkgs = Array.isArray(doc.packages) ? doc.packages : [];

    // Determine the library "root" package: importPath === module, else the
    // package with the fewest segments beyond the module (lexical tie-break).
    let rootPkg: DocPackage | null = null;
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

    const state: LibStateEntry = {
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
  // A library whose submodule is unchecked-out, empty, or still a placeholder
  // simply contributes no import edges (goImportsForDir swallows the ENOENT).
  // Name those libraries so a graph with few import edges is diagnosable
  // instead of mysterious.
  let importEdgeCount = 0;
  const missingSources: string[] = [];
  for (const st of libState) {
    if (!fs.existsSync(path.join(st.sourceDir, 'go.mod'))) missingSources.push(st.id);
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
  const byOrg = new Map<string, string[]>(); // org -> [rootId,...]
  for (const st of libState) {
    if (!st.rootId) continue;
    const par = parity[st.id];
    if (!par || !par.upstream) continue;
    const org = String(par.upstream).split('/')[0];
    if (!org) continue;
    if (!byOrg.has(org)) byOrg.set(org, []);
    byOrg.get(org)!.push(st.rootId);
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
  const edgeMap = new Map<string, EdgeOut>();
  for (const e of edges) {
    const key = `${e.source} ${e.target} ${e.kind}`;
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
  const symMap = new Map<string, SymbolOut>();
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
  // -------- runnable examples corpus --------
  // Deterministic: keyed and iterated by sorted library id. Only api/_data gets
  // this file — it is read server-side by the /ask tool and embedded in the
  // function bundle; the frontend never fetches it, so no public/ copy.
  const knownLibraries = new Set(finalLibraries.map((l) => l.id));
  const { examples: exampleMap, missing: missingExamples } = buildExamples(knownLibraries);
  const sortedExamples: Record<string, ExampleOut> = {};
  for (const id of Object.keys(exampleMap).sort()) sortedExamples[id] = exampleMap[id];
  const examplesOut = {
    generatedAt,
    examples: sortedExamples,
  };

  const graphJson = JSON.stringify(graph, null, 2) + '\n';
  const symbolsJson = JSON.stringify(symbolIndex, null, 2) + '\n';
  const examplesJson = JSON.stringify(examplesOut, null, 2) + '\n';

  fs.writeFileSync(path.join(OUT_DIRS[0], 'graph.json'), graphJson);
  fs.writeFileSync(path.join(OUT_DIRS[0], 'symbols.json'), symbolsJson);
  fs.writeFileSync(path.join(OUT_DIRS[0], 'examples.json'), examplesJson);
  // public copies use the frontend-fallback filenames.
  fs.writeFileSync(path.join(OUT_DIRS[1], 'graph.json'), graphJson);
  fs.writeFileSync(path.join(OUT_DIRS[1], 'search-index.json'), symbolsJson);

  const kindCounts: Record<string, number> = {};
  for (const e of finalEdges) kindCounts[e.kind] = (kindCounts[e.kind] || 0) + 1;
  const kindStr = ['import', 'reference', 'same-library', 'shared-upstream']
    .map((k) => `${k}=${kindCounts[k] || 0}`)
    .join(' ');
  console.log(
    `build-graph-data: ${finalLibraries.length} libraries, ${finalPackages.length} packages, ` +
    `${finalSymbols.length} symbols, ${finalEdges.length} edges (${kindStr}; ` +
    `${importEdgeCount} raw import edges from source). Wrote graph.json + symbols.json ` +
    `to api/_data and public.`
  );
  console.log(
    `build-graph-data: ${Object.keys(sortedExamples).length} runnable examples ` +
    `(${(examplesJson.length / 1024).toFixed(1)} KB). Wrote examples.json to api/_data.`
  );
  if (missingExamples.length > 0) {
    console.warn(
      `build-graph-data: no runnable example for ${missingExamples.length} librar` +
      `${missingExamples.length === 1 ? 'y' : 'ies'} (${missingExamples.join(', ')}).`
    );
  }
  if (missingSources.length > 0) {
    console.warn(
      `build-graph-data: no go.mod on disk for ${missingSources.length} librar` +
      `${missingSources.length === 1 ? 'y' : 'ies'} (${missingSources.join(', ')}) — ` +
      `import edges for those are omitted. These are either placeholder ports ` +
      `(README → "Planned ports") or unfetched submodules ` +
      `(\`git submodule update --init --recursive\`).`
    );
  }
}

main();
