import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import type { DocPackage, DocValue, DocFunc, DocType } from '../../docs/types';

export interface SymbolSearchProps {
  packages: DocPackage[];
  // Called when a symbol is chosen. anchorId is the `sym-…` id per the contract.
  onPick: (importPath: string, anchorId: string) => void;
}

// The kinds a searchable symbol can have. These drive the little kind chip shown
// on each result row (gd-search-item-kind).
type SymbolKind = 'type' | 'interface' | 'func' | 'method' | 'const' | 'var';

// Single-letter glyph shown in the kind chip, and a human label for a11y/title.
const KIND_LETTER: Record<SymbolKind, string> = {
  type: 'T',
  interface: 'I',
  func: 'F',
  method: 'M',
  const: 'C',
  var: 'V',
};
const KIND_LABEL: Record<SymbolKind, string> = {
  type: 'type',
  interface: 'interface',
  func: 'function',
  method: 'method',
  const: 'constant',
  var: 'variable',
};

// A flat, searchable entry for one exported symbol. Shape is fixed by the
// contract: { label, kind, importPath, anchorId, pkgName }.
interface IndexEntry {
  label: string;
  kind: SymbolKind;
  importPath: string;
  anchorId: string;
  pkgName: string;
}

// Anchor-id scheme (must match PackageNav / PackageView and the shared contract):
//   value (const/var group): sym-<names[0]>
//   func / constructor:       sym-<funcName>
//   type:                     sym-<TypeName>
//   method:                   sym-<recvBase>.<methodName>
//     where recvBase = recv without leading '*' and without any `[...]` params,
//     e.g. recv "*LRU[K,V]", name "Get" -> "sym-LRU.Get".
function recvBase(recv: string): string {
  return recv.replace(/^\*/, '').replace(/\[.*$/, '');
}

function valueId(v: DocValue): string {
  return `sym-${v.names[0] ?? 'value'}`;
}

function funcId(fn: DocFunc): string {
  return fn.recv ? `sym-${recvBase(fn.recv)}.${fn.name}` : `sym-${fn.name}`;
}

function typeId(t: DocType): string {
  return `sym-${t.name}`;
}

// isInterface reports whether a type declaration is an interface (chip I) rather
// than a plain type/struct (chip T), matching PackageNav's detection: look for
// the `interface` keyword in the signature.
function isInterface(t: DocType): boolean {
  return /\binterface\b/.test(t.signature);
}

// buildIndex flattens every exported symbol across all packages into a single
// searchable list. Order is stable and mirrors the sidebar: package consts,
// vars, then each type (with its scoped constructors/methods), then funcs.
function buildIndex(packages: DocPackage[]): IndexEntry[] {
  const out: IndexEntry[] = [];

  for (const pkg of packages) {
    const importPath = pkg.importPath;
    const pkgName = pkg.name;

    for (const c of pkg.consts) {
      out.push({
        label: c.names.join(', '),
        kind: 'const',
        importPath,
        anchorId: valueId(c),
        pkgName,
      });
    }
    for (const v of pkg.vars) {
      out.push({
        label: v.names.join(', '),
        kind: 'var',
        importPath,
        anchorId: valueId(v),
        pkgName,
      });
    }
    for (const t of pkg.types) {
      out.push({
        label: t.name,
        kind: isInterface(t) ? 'interface' : 'type',
        importPath,
        anchorId: typeId(t),
        pkgName,
      });
      // Type-scoped constructors return the type; index them as funcs.
      for (const fn of t.funcs) {
        out.push({
          label: fn.name,
          kind: 'func',
          importPath,
          anchorId: funcId(fn),
          pkgName,
        });
      }
      // Methods are named "<Type>.<Method>" and anchor to sym-<recvBase>.<name>.
      for (const m of t.methods) {
        out.push({
          label: `${recvBase(m.recv ?? t.name)}.${m.name}`,
          kind: 'method',
          importPath,
          anchorId: funcId(m),
          pkgName,
        });
      }
    }
    for (const fn of pkg.funcs) {
      out.push({
        label: fn.name,
        kind: 'func',
        importPath,
        anchorId: funcId(fn),
        pkgName,
      });
    }
  }

  return out;
}

const MAX_RESULTS = 12;

// rank scores a matching entry: prefix matches on the symbol name rank first,
// then any name substring match, then package-only matches. Lower is better.
function rank(entry: IndexEntry, q: string): number {
  const label = entry.label.toLowerCase();
  if (label.startsWith(q)) return 0;
  // A method's own name after the "Type." prefix — treat a prefix hit there as
  // nearly as good as a leading prefix so "Get" surfaces "LRU.Get".
  const dot = label.indexOf('.');
  if (dot >= 0 && label.slice(dot + 1).startsWith(q)) return 1;
  if (label.includes(q)) return 2;
  return 3; // matched only on the package name/path
}

// SymbolSearch is the header search box. It builds a memoized flat index of every
// exported symbol and, as the user types, shows a ranked dropdown of matches.
// Choosing one (click, or keyboard Enter) calls onPick(importPath, anchorId).
export function SymbolSearch({ packages, onPick }: SymbolSearchProps) {
  const index = useMemo(() => buildIndex(packages), [packages]);

  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);

  const blurTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);

  const q = query.trim().toLowerCase();

  const results = useMemo(() => {
    if (!q) return [] as IndexEntry[];
    const matched = index.filter(
      (e) =>
        e.label.toLowerCase().includes(q) ||
        e.pkgName.toLowerCase().includes(q) ||
        e.importPath.toLowerCase().includes(q),
    );
    matched.sort((a, b) => {
      const ra = rank(a, q);
      const rb = rank(b, q);
      if (ra !== rb) return ra - rb;
      // Shorter labels first (closer match), then alphabetical for stability.
      if (a.label.length !== b.label.length) return a.label.length - b.label.length;
      return a.label.localeCompare(b.label);
    });
    return matched.slice(0, MAX_RESULTS);
  }, [index, q]);

  // Keep the active row in bounds and reset to the top whenever results change.
  useEffect(() => {
    setActive(0);
  }, [q]);

  // Scroll the active row into view as the user arrows through the list.
  useEffect(() => {
    const menu = menuRef.current;
    if (!menu) return;
    const row = menu.querySelector<HTMLElement>('.gd-search-item.active');
    if (row) row.scrollIntoView({ block: 'nearest' });
  }, [active, open]);

  useEffect(() => {
    return () => {
      if (blurTimer.current) clearTimeout(blurTimer.current);
    };
  }, []);

  const pick = useCallback(
    (entry: IndexEntry) => {
      onPick(entry.importPath, entry.anchorId);
      setQuery('');
      setOpen(false);
      setActive(0);
    },
    [onPick],
  );

  const onKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Escape') {
        // First Escape closes the menu; if already closed, clear the query.
        if (open && results.length > 0) {
          setOpen(false);
        } else {
          setQuery('');
          setOpen(false);
        }
        return;
      }
      if (e.key === 'ArrowDown') {
        if (results.length === 0) return;
        e.preventDefault();
        setOpen(true);
        setActive((i) => (i + 1) % results.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        if (results.length === 0) return;
        e.preventDefault();
        setOpen(true);
        setActive((i) => (i - 1 + results.length) % results.length);
        return;
      }
      if (e.key === 'Enter') {
        if (open && results.length > 0) {
          e.preventDefault();
          const chosen = results[active] ?? results[0];
          if (chosen) pick(chosen);
        }
      }
    },
    [open, results, active, pick],
  );

  const onFocus = useCallback(() => {
    if (blurTimer.current) {
      clearTimeout(blurTimer.current);
      blurTimer.current = null;
    }
    if (query.trim()) setOpen(true);
  }, [query]);

  const onBlur = useCallback(() => {
    // Delay closing so a click on a result row registers before unmount.
    if (blurTimer.current) clearTimeout(blurTimer.current);
    blurTimer.current = setTimeout(() => setOpen(false), 150);
  }, []);

  const showMenu = open && q.length > 0;

  return (
    <div className="gd-search">
      <span className="gd-search-ico" aria-hidden="true">
        ⌕
      </span>
      <input
        type="search"
        className="gd-search-input"
        placeholder="Search symbols"
        value={query}
        role="combobox"
        aria-expanded={showMenu}
        aria-autocomplete="list"
        aria-controls="gd-search-menu"
        aria-label="Search symbols"
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
        }}
        onKeyDown={onKeyDown}
        onFocus={onFocus}
        onBlur={onBlur}
      />

      {showMenu ? (
        <div className="gd-search-menu" id="gd-search-menu" role="listbox" ref={menuRef}>
          {results.length > 0 ? (
            results.map((entry, i) => {
              const isActive = i === active;
              return (
                <div
                  key={`${entry.importPath}#${entry.anchorId}`}
                  className={`gd-search-item${isActive ? ' active' : ''}`}
                  role="option"
                  aria-selected={isActive}
                  // mouseDown fires before the input's blur, so the pick lands
                  // even though blur would otherwise close the menu first.
                  onMouseDown={(e) => {
                    e.preventDefault();
                    pick(entry);
                  }}
                  onMouseEnter={() => setActive(i)}
                >
                  <span
                    className="gd-search-item-kind"
                    data-kind={entry.kind}
                    title={KIND_LABEL[entry.kind]}
                    aria-label={KIND_LABEL[entry.kind]}
                  >
                    {KIND_LETTER[entry.kind]}
                  </span>
                  <span className="gd-search-item-label">{entry.label}</span>
                  <span className="gd-search-item-path">{entry.importPath}</span>
                </div>
              );
            })
          ) : (
            <div className="gd-search-empty">No symbols match “{query.trim()}”.</div>
          )}
        </div>
      ) : null}
    </div>
  );
}

export default SymbolSearch;
