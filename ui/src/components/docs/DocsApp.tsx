import { useEffect, useMemo, useState, useCallback } from 'react';
import type { DocIndex } from '../../docs/types';
import { PackageNav } from './PackageNav';
import { PackageView } from './PackageView';

export interface DocsAppProps {
  // Provide the docs data directly…
  index?: DocIndex;
  // …or a URL to fetch a doc.json from (used when index is omitted).
  url?: string;
  // Optional title shown above the package list (defaults to the module path).
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

// DocsApp is the top-level API-docs renderer. It shows a package sidebar plus
// the selected package's documentation, and is hash-routable by import path
// (#pkg/<importPath>). Pass `index` directly, or a `url` to fetch a doc.json.
export function DocsApp({ index, url, title, hashRouting = true }: DocsAppProps) {
  const [data, setData] = useState<DocIndex | null>(index ?? null);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState<string>('');

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

  const select = useCallback((importPath: string) => {
    if (hashRouting && typeof location !== 'undefined') {
      location.hash = PKG_PREFIX + importPath;
    }
    setActive(importPath);
  }, [hashRouting]);

  const current = useMemo(
    () => packages.find((p) => p.importPath === active) ?? packages[0],
    [packages, active],
  );

  if (error) {
    return (
      <div className="docs-shell">
        <p className="docs-error" role="alert">
          Failed to load documentation: {error}
        </p>
      </div>
    );
  }
  if (!data) {
    return (
      <div className="docs-shell">
        <p className="docs-loading">Loading documentation…</p>
      </div>
    );
  }

  return (
    <div className="docs-shell">
      <aside className="docs-sidebar">
        <div className="docs-module">{title ?? data.module}</div>
        <PackageNav packages={packages} active={current?.importPath ?? ''} onSelect={select} />
      </aside>
      <div className="docs-main">{current ? <PackageView pkg={current} /> : <p className="muted">No packages.</p>}</div>
    </div>
  );
}
