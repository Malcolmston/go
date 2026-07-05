import { useState } from 'react';
import type { DocPackage } from '../../docs/types';

export interface PackageNavProps {
  packages: DocPackage[];
  // Import path of the currently selected package.
  active: string;
  // Called with a package import path when a nav entry is chosen.
  onSelect: (importPath: string) => void;
}

// PackageNav is the docs sidebar: a filterable list of packages. Selecting an
// entry calls onSelect with its import path (the caller updates the hash route).
export function PackageNav({ packages, active, onSelect }: PackageNavProps) {
  const [query, setQuery] = useState('');
  const q = query.trim().toLowerCase();
  const shown = q
    ? packages.filter(
        (p) => p.importPath.toLowerCase().includes(q) || p.synopsis.toLowerCase().includes(q),
      )
    : packages;

  return (
    <nav className="docs-nav" aria-label="Packages">
      <input
        className="docs-nav-search"
        type="search"
        placeholder="Filter packages…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        aria-label="Filter packages"
      />
      <ul className="docs-pkg-list">
        {shown.map((p) => (
          <li key={p.importPath}>
            <a
              className={`docs-pkg-link${p.importPath === active ? ' active' : ''}`}
              href={`#pkg/${p.importPath}`}
              onClick={(e) => {
                e.preventDefault();
                onSelect(p.importPath);
              }}
            >
              <span className="docs-pkg-name">{p.name}</span>
              <span className="docs-pkg-path">{p.importPath}</span>
            </a>
          </li>
        ))}
        {shown.length === 0 ? <li className="docs-nav-empty">No packages match.</li> : null}
      </ul>
    </nav>
  );
}
