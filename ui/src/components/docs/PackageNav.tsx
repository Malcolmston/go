'use client';

import { useEffect, useState } from 'react';
import type { DocPackage, DocValue, DocFunc, DocType } from '../../docs/types';

export interface PackageNavProps {
  packages: DocPackage[];
  // Import path of the currently selected package.
  active: string;
  // Called with a package import path when a nav entry is chosen.
  onSelect: (importPath: string) => void;
}

// Anchor-id scheme (must match PackageView / SymbolSearch and the shared contract):
//   value (const/var group): sym-<names[0]>
//   func / constructor:       sym-<funcName>
//   type:                     sym-<TypeName>
//   method:                   sym-<recvBase>.<methodName>  (recvBase: recv sans '*' and '[...]')
function recvName(recv: string): string {
  return recv.replace(/^\*/, '').replace(/\[.*$/, '');
}

function valueId(v: DocValue): string {
  return `sym-${v.names[0] ?? 'value'}`;
}

function funcId(fn: DocFunc): string {
  return fn.recv ? `sym-${recvName(fn.recv)}.${fn.name}` : `sym-${fn.name}`;
}

function typeId(t: DocType): string {
  return `sym-${t.name}`;
}

// Kind badge letters + their gd-badge-* modifier suffix.
type Kind = 't' | 'i' | 'f' | 'c' | 'v';

const KIND_LETTER: Record<Kind, string> = { t: 'T', i: 'I', f: 'F', c: 'C', v: 'V' };
const KIND_LABEL: Record<Kind, string> = {
  t: 'type',
  i: 'interface',
  f: 'function',
  c: 'constant',
  v: 'variable',
};

// A flattened sidebar entry for the active package's symbol list.
interface SymEntry {
  id: string;
  name: string;
  kind: Kind;
  // Nesting depth (0 = package-level, 1 = type member).
  depth: number;
}

// isInterface reports whether a type's declaration is an interface type (badge I)
// rather than a plain type/struct (badge T), per the contract: check the type
// signature text for the `interface` keyword.
function isInterface(t: DocType): boolean {
  return /\binterface\b/.test(t.signature);
}

// symbolsFor builds the ordered, flattened symbol list shown under an expanded
// package: package consts, vars, types (with their scoped constructors/methods
// indented), then package-level funcs.
function symbolsFor(pkg: DocPackage): SymEntry[] {
  const out: SymEntry[] = [];

  for (const c of pkg.consts) {
    out.push({ id: valueId(c), name: c.names.join(', '), kind: 'c', depth: 0 });
  }
  for (const v of pkg.vars) {
    out.push({ id: valueId(v), name: v.names.join(', '), kind: 'v', depth: 0 });
  }
  for (const t of pkg.types) {
    out.push({ id: typeId(t), name: t.name, kind: isInterface(t) ? 'i' : 't', depth: 0 });
    // Constructors and methods are reachable via their own sym ids; list them
    // indented so the whole type surface is navigable from the sidebar.
    for (const fn of t.funcs) {
      out.push({ id: funcId(fn), name: fn.name, kind: 'f', depth: 1 });
    }
    for (const m of t.methods) {
      out.push({ id: funcId(m), name: m.name, kind: 'f', depth: 1 });
    }
  }
  for (const fn of pkg.funcs) {
    out.push({ id: funcId(fn), name: fn.name, kind: 'f', depth: 0 });
  }

  return out;
}

// useActiveHash tracks the current location hash so the matching symbol row can
// be highlighted (gd-sym-link.active). Native in-page links change the hash
// without a React re-render, so we listen for hashchange.
function useActiveHash(): string {
  const read = () =>
    typeof window === 'undefined' ? '' : decodeURIComponent(window.location.hash.replace(/^#/, ''));
  const [hash, setHash] = useState(read);
  useEffect(() => {
    const onChange = () => setHash(read());
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);
  return hash;
}

// PackageNav is the docs sidebar: a "Packages" title, a filter box, and a
// collapsible tree of packages. The active package is expanded and lists its
// symbols as in-page #sym-… anchor links with kind badges.
export function PackageNav({ packages, active, onSelect }: PackageNavProps) {
  const [query, setQuery] = useState('');
  const activeHash = useActiveHash();

  const q = query.trim().toLowerCase();
  const shown = q
    ? packages.filter(
        (p) =>
          p.importPath.toLowerCase().includes(q) ||
          p.name.toLowerCase().includes(q) ||
          p.synopsis.toLowerCase().includes(q),
      )
    : packages;

  return (
    <nav className="docs-nav" aria-label="Packages">
      <div className="gd-sidebar-title">Packages</div>

      <div className="gd-search gd-sidebar-search">
        <span className="gd-search-ico" aria-hidden="true">
          ⌕
        </span>
        <input
          className="docs-nav-search"
          type="search"
          placeholder="Filter packages…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-label="Filter packages"
        />
      </div>

      <div className="gd-pkg-list">
        {shown.map((pkg) => {
          const isActive = pkg.importPath === active;
          return (
            <div className="gd-pkg" key={pkg.importPath}>
              <button
                type="button"
                className={`gd-pkg-header${isActive ? ' open' : ''}`}
                aria-expanded={isActive}
                title={pkg.importPath}
                onClick={() => onSelect(pkg.importPath)}
              >
                <span className="gd-pkg-icon" aria-hidden="true">
                  {isActive ? '▾' : '▸'}
                </span>
                <span className="gd-pkg-name">{pkg.name}</span>
              </button>

              {isActive ? (
                <div className="gd-pkg-syms" role="list">
                  {symbolsFor(pkg).map((s) => {
                    const rowActive = activeHash === s.id;
                    return (
                      <a
                        key={s.id}
                        role="listitem"
                        className={`gd-sym-link gd-sym-depth-${s.depth}${
                          rowActive ? ' active' : ''
                        }`}
                        href={`#${s.id}`}
                        aria-current={rowActive ? 'true' : undefined}
                        onClick={() => onSelect(pkg.importPath)}
                      >
                        <span
                          className={`gd-badge gd-badge-${s.kind}`}
                          title={KIND_LABEL[s.kind]}
                          aria-label={KIND_LABEL[s.kind]}
                        >
                          {KIND_LETTER[s.kind]}
                        </span>
                        <span className="gd-sym-name">{s.name}</span>
                      </a>
                    );
                  })}
                </div>
              ) : null}
            </div>
          );
        })}

        {shown.length === 0 ? <div className="gd-nav-empty">No packages match.</div> : null}
      </div>
    </nav>
  );
}
