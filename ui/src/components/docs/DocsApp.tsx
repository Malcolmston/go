'use client';

import { useEffect, useMemo, useState, useCallback } from 'react';
import type { MouseEvent } from 'react';
import type { DocIndex } from '../../docs/types';
import { PackageNav } from './PackageNav';
import { PackageView } from './PackageView';
import { SymbolSearch } from './SymbolSearch';
import './godoc.css';

export interface DocsAppProps {
  // Provide the docs data directly…
  index?: DocIndex;
  // …or a URL to fetch a doc.json from (used when index is omitted).
  url?: string;
  // Optional title shown as the header brand (defaults to the module short name).
  title?: string;
  // Whether package selection is reflected in location.hash (#pkg/<importPath>).
  // Defaults to true (the standalone per-library docs sites are hash-routable).
  // A host that already owns the hash — e.g. the /go aggregator, whose tabs are
  // hash-routed — passes false so the renderer keeps package selection in local
  // state and does not fight the host router.
  hashRouting?: boolean;
}

const PKG_PREFIX = 'pkg/';

function readHashPkg(): string | null {
  if (typeof location === 'undefined') return null;
  const h = location.hash.replace(/^#/, '');
  return h.startsWith(PKG_PREFIX) ? decodeURIComponent(h.slice(PKG_PREFIX.length)) : null;
}

// The header brand shows the module's short name (its final path segment), e.g.
// "github.com/acme/cache" -> "cache". Falls back to the whole string if unsplit.
function shortModule(m: string): string {
  if (!m) return 'docs';
  const parts = m.split('/').filter(Boolean);
  return parts[parts.length - 1] || m;
}

// DocsApp is the top-level API-docs renderer. It renders the Javadoc-style
// shell — a blue header bar (brand + nav + symbol search), a breadcrumb, and a
// two-column body (package sidebar + package documentation main). Selection is
// hash-routable by import path (#pkg/<importPath>) when hashRouting is on.
// Pass `index` directly, or a `url` to fetch a doc.json.
export function DocsApp({ index, url, title, hashRouting = true }: DocsAppProps) {
  const [data, setData] = useState<DocIndex | null>(index ?? null);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState<string>('');
  // The symbol anchor the breadcrumb should reflect when hashRouting is off (when
  // it is on, the location hash is authoritative). Set by search/pick.
  const [activeSym, setActiveSym] = useState<string | null>(null);
  // A pending `sym-…` element id to scroll into view after the next render.
  const [pendingScroll, setPendingScroll] = useState<string | null>(null);
  // Header nav toggles: Index (jump-list of every package) and Help.
  const [indexOpen, setIndexOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  // Mirror of location.hash so the breadcrumb can reflect a `#sym-…` anchor.
  const [hash, setHash] = useState<string>(() =>
    typeof location !== 'undefined' ? location.hash : '',
  );

  useEffect(() => {
    if (index) {
      setData(index);
      return;
    }
    if (!url) return;
    let cancelled = false;
    setData(null);
    setError(null);
    fetch(url)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then((json: DocIndex) => {
        if (!cancelled) setData(json);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e?.message ?? e));
      });
    return () => {
      cancelled = true;
    };
  }, [index, url]);

  const packages = data?.packages ?? [];

  // Keep the active package in sync. When hashRouting is on, the location hash
  // (#pkg/<importPath>) is the source of truth; otherwise selection lives purely
  // in local state and we just default to the first package once data arrives.
  useEffect(() => {
    if (packages.length === 0) return;
    if (!hashRouting) {
      setActive((cur) => (cur && packages.some((p) => p.importPath === cur) ? cur : packages[0].importPath));
      return;
    }
    const sync = () => {
      const fromHash = readHashPkg();
      if (fromHash && packages.some((p) => p.importPath === fromHash)) {
        setActive(fromHash);
      } else {
        setActive((cur) => (cur && packages.some((p) => p.importPath === cur) ? cur : packages[0].importPath));
      }
    };
    sync();
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, hashRouting]);

  // Track the raw hash so the breadcrumb can show a `#sym-…` anchor segment.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const on = () => setHash(location.hash);
    window.addEventListener('hashchange', on);
    return () => window.removeEventListener('hashchange', on);
  }, []);

  // Scroll a freshly-selected symbol into view once its package has rendered.
  // Two rAFs so the new <main> subtree is laid out before we measure it.
  useEffect(() => {
    if (!pendingScroll || typeof document === 'undefined') return;
    let raf2 = 0;
    const raf1 = requestAnimationFrame(() => {
      raf2 = requestAnimationFrame(() => {
        const el = document.getElementById(pendingScroll);
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
        setPendingScroll(null);
      });
    });
    return () => {
      cancelAnimationFrame(raf1);
      if (raf2) cancelAnimationFrame(raf2);
    };
  }, [pendingScroll, active]);

  // Select a package. With hashRouting on the hash is authoritative; otherwise we
  // update local state directly.
  const select = useCallback((importPath: string) => {
    if (hashRouting && typeof location !== 'undefined') {
      location.hash = PKG_PREFIX + importPath;
    }
    setActive(importPath);
  }, [hashRouting]);

  // Package selection from the sidebar / index list also clears any active symbol
  // so the breadcrumb collapses back to module › package.
  const selectPkg = useCallback((importPath: string) => {
    setActiveSym(null);
    select(importPath);
  }, [select]);

  // Called by SymbolSearch: jump to a symbol in some package, then scroll to it.
  const onPick = useCallback((importPath: string, anchorId: string) => {
    setActive(importPath);
    if (hashRouting && typeof location !== 'undefined') {
      // Put the symbol anchor in the hash: the breadcrumb reflects it and the
      // package-sync effect keeps the (now-updated) active package because the
      // hash is not a #pkg/ route.
      location.hash = anchorId;
    } else {
      setActiveSym(anchorId);
    }
    setIndexOpen(false);
    setHelpOpen(false);
    setPendingScroll(anchorId);
  }, [hashRouting]);

  const current = useMemo(
    () => packages.find((p) => p.importPath === active) ?? packages[0],
    [packages, active],
  );

  // The symbol segment for the breadcrumb: from the hash when hashRouting, else
  // from local state. anchorId is `sym-<name>`; strip the prefix for display.
  const symId = useMemo(() => {
    const raw = hash.replace(/^#/, '');
    if (raw.startsWith('sym-')) return raw;
    if (!hashRouting && activeSym) return activeSym;
    return null;
  }, [hash, hashRouting, activeSym]);
  const symLabel = symId ? symId.replace(/^sym-/, '') : null;

  if (error) {
    return (
      <div className="gd-root docs-shell">
        <p className="docs-error" role="alert">
          Failed to load documentation: {error}
        </p>
      </div>
    );
  }
  if (!data) {
    return (
      <div className="gd-root docs-shell">
        <p className="docs-loading">Loading documentation…</p>
      </div>
    );
  }

  const brand = title ?? shortModule(data.module);
  const firstPkg = packages[0];

  const onOverview = (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    setIndexOpen(false);
    setHelpOpen(false);
    setActiveSym(null);
    if (firstPkg) selectPkg(firstPkg.importPath);
    if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const onPackage = (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    setIndexOpen(false);
    setHelpOpen(false);
    setActiveSym(null);
    if (current) selectPkg(current.importPath);
    if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const onIndex = (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    setHelpOpen(false);
    setIndexOpen((v) => !v);
  };

  const onHelp = (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    setIndexOpen(false);
    setHelpOpen((v) => !v);
  };

  return (
    <div className="gd-root docs-shell">
      <header className="gd-header">
        <div className="gd-header-brand">
          {brand}
          <span className="gd-header-ver"> API Reference</span>
        </div>
        <nav className="gd-header-nav" aria-label="Primary">
          <a
            className="gd-header-link"
            href={firstPkg ? `#${PKG_PREFIX}${firstPkg.importPath}` : '#'}
            onClick={onOverview}
          >
            Overview
          </a>
          <a
            className="gd-header-link active"
            href={current ? `#${PKG_PREFIX}${current.importPath}` : '#'}
            onClick={onPackage}
          >
            Package
          </a>
          <a
            className={`gd-header-link${indexOpen ? ' active' : ''}`}
            href="#index"
            aria-expanded={indexOpen}
            onClick={onIndex}
          >
            Index
          </a>
          <a
            className={`gd-header-link${helpOpen ? ' active' : ''}`}
            href="#help"
            aria-expanded={helpOpen}
            onClick={onHelp}
          >
            Help
          </a>
        </nav>
        <SymbolSearch packages={packages} onPick={onPick} />
      </header>

      {indexOpen ? (
        <div className="gd-index-panel">
          <ul className="gd-index-list">
            {packages.map((p) => (
              <li key={p.importPath}>
                <a
                  className={`gd-index-item${p.importPath === current?.importPath ? ' active' : ''}`}
                  href={`#${PKG_PREFIX}${p.importPath}`}
                  onClick={(e) => {
                    e.preventDefault();
                    setIndexOpen(false);
                    selectPkg(p.importPath);
                    if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' });
                  }}
                >
                  <span className="gd-index-name">{p.name}</span>
                  <span className="gd-index-path">{p.importPath}</span>
                </a>
              </li>
            ))}
            {packages.length === 0 ? <li className="gd-search-empty">No packages.</li> : null}
          </ul>
        </div>
      ) : null}

      {helpOpen ? (
        <div className="gd-help-panel">
          <p>
            Browse packages in the left sidebar; open one to reveal its exported
            symbols. Use <b>Search symbols</b> in the header to jump straight to a
            type, function, method, constant, or variable — arrow keys move through
            matches, <b>Enter</b> opens one, <b>Esc</b> dismisses. <b>Overview</b>
            {' '}returns to the first package, <b>Index</b> lists every package, and
            symbol links are shareable in-page anchors.
          </p>
        </div>
      ) : null}

      <div className="gd-breadcrumb">
        <span className="gd-crumb">{data.module}</span>
        {current ? (
          <>
            <span className="gd-crumb-sep">›</span>
            <span className={symLabel ? 'gd-crumb' : 'gd-crumb-cur'}>{current.name}</span>
          </>
        ) : null}
        {symLabel ? (
          <>
            <span className="gd-crumb-sep">›</span>
            <span className="gd-crumb-cur">{symLabel}</span>
          </>
        ) : null}
      </div>

      <div className="gd-body">
        <aside className="gd-sidebar">
          <div className="gd-sidebar-title">Packages</div>
          <PackageNav packages={packages} active={current?.importPath ?? ''} onSelect={selectPkg} />
        </aside>
        <main className="gd-main">
          {current ? <PackageView pkg={current} /> : <p className="muted">No packages.</p>}
        </main>
      </div>
    </div>
  );
}
