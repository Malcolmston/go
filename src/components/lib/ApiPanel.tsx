'use client';

import { DocsApp } from 'go-ui';
import type { Lib } from '../../data';
import { withBase } from '../../basePath';

// docKey derives the bundled doc-index filename for a library from its GitHub
// repo URL (its last path segment), matching what gendocs writes into
// public/docs/<repo>.json — e.g. ".../socket.io" -> "socket.io".
function docKey(lib: Lib): string {
  return lib.repo.replace(/\/+$/, '').split('/').pop() ?? lib.id;
}

// ApiPanel is the library's API sub-page: the full package-by-package Go API
// reference rendered inline by the shared DocsApp. Rendered at /lib/<id>/api.
export function ApiPanel({ lib }: { lib: Lib }) {
  const idb = lib.id;
  return (
    <div className="libpanel" role="tabpanel">
      <div className="sec-h" id={`${idb}-api`}><span className="bar" /><h3 style={{ margin: 0 }}>API reference</h3></div>
      <p className="muted">The complete package-by-package Go API reference, generated from source — every exported type, function and method, with signatures, doc comments and runnable examples.</p>
      {/* Full Javadoc-style reference rendered inline. hashRouting is off so the
          renderer does not fight the aggregator's router; `embedded` keeps its
          sticky chrome below the site nav. The doc index is the per-library file
          bundled by gendocs at build time; the header symbol search is backed by
          the Elasticsearch /api/search endpoint scoped to this library, falling
          back to the local index on GitHub Pages / any failure. */}
      <DocsApp
        url={withBase(`docs/${docKey(lib)}.json`)}
        title={lib.pkg}
        hashRouting={false}
        searchEndpoint={withBase('api/search')}
        library={docKey(lib)}
        embedded
      />
    </div>
  );
}