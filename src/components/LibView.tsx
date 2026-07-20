import { useState } from 'react';
import type { CSSProperties } from 'react';
import Link from 'next/link';
import { CodeBlock, CompareCard, DocsApp, VersionBadge, hi, hx, ghrepo } from 'go-ui';
import type { Lib } from '../data';
import { parityFor } from '../parityLookup';
import { withBase } from '../basePath';
import { Html } from './Html';

export interface LibViewProps {
  lib: Lib;
}

// docKey derives the bundled doc-index filename for a library from its GitHub
// repo URL (its last path segment), matching what gendocs writes into
// web/public/docs/<repo>.json — e.g. ".../socket.io" -> "socket.io".
function docKey(lib: Lib): string {
  return lib.repo.replace(/\/+$/, '').split('/').pop() ?? lib.id;
}

type Sub = 'overview' | 'examples' | 'api' | 'parity';

// LibView renders a single library's page. The content is split into sub-tabs
// (Overview / Examples / API / Parity) under a persistent hero, so each concern
// is its own view instead of one long scroll.
export function LibView({ lib }: LibViewProps) {
  const idb = lib.id;
  const source = lib.source ?? 'Node.js';
  const parity = parityFor(lib);
  const [sub, setSub] = useState<Sub>('overview');

  const tabs: { id: Sub; label: string }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'examples', label: 'Examples' },
    { id: 'api', label: 'API' },
    { id: 'parity', label: 'Parity' },
  ];

  return (
    <section className="view active" id={`view-${lib.id}`}>
      <div className="libhero" style={{ '--lib-soft': hx(lib.accent, '1f'), '--lib-accent': lib.accent } as CSSProperties}>
        <div className="row">
          <Html html={lib.icon} className="mono" />
          <div style={{ flex: 1, minWidth: 220 }}>
            <h2>{lib.name} <span className="muted" style={{ fontWeight: 400, fontSize: '1rem' }}>for Go</span></h2>
            <div className="pkg mono">{lib.pkg}</div>
            <p className="tagline">{lib.tagline}</p>
          </div>
        </div>
        <div className="actions">
          <a className="pill b" href={lib.repo} target="_blank" rel="noopener"><i className="fa-brands fa-github" />&nbsp;GitHub</a>
          {/* Docs are self-contained: the "API docs" pill switches to this page's
              API sub-tab (rendered inline) rather than sending the reader off to
              the external github.io docs site. Source ≠ docs, so the GitHub pill
              above still points at the repository. */}
          <button type="button" className="pill b" onClick={() => setSub('api')}><i className="fa-solid fa-book" /> API docs</button>
          <VersionBadge repo={ghrepo(lib)} href={`${lib.repo}/releases`} />
          <span className="pill">ports <b style={{ color: 'var(--fg)', marginLeft: '.25rem' }}>{lib.node}</b></span>
        </div>
      </div>

      <p className="muted">{lib.blurb}</p>

      <div className="libtabs" role="tablist" aria-label={`${lib.name} sections`}>
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={sub === t.id}
            className={`libtab${sub === t.id ? ' active' : ''}`}
            style={sub === t.id ? ({ '--lib-accent': lib.accent } as CSSProperties) : undefined}
            onClick={() => setSub(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {sub === 'overview' && (
        <div className="libpanel" role="tabpanel">
          <div className="sec-h" id={`${idb}-install`}><span className="bar" /><h3 style={{ margin: 0 }}>Install</h3></div>
          <CodeBlock lang="shell" html={`<span class="tok-c">$</span> go get ${lib.pkg}`} />

          <div className="sec-h" id={`${idb}-quick`}><span className="bar" /><h3 style={{ margin: 0 }}>Quick start</h3></div>
          <CodeBlock lang="main.go" html={hi(lib.go_code)} />

          <div className="sec-h" id={`${idb}-feat`}><span className="bar" /><h3 style={{ margin: 0 }}>Features</h3></div>
          <ul className="feat" style={{ '--lib-accent': lib.accent } as CSSProperties}>
            {lib.features.map((f, i) => <Html tag="li" html={f} key={i} />)}
          </ul>
        </div>
      )}

      {sub === 'examples' && (
        <div className="libpanel" role="tabpanel">
          <div className="sec-h" id={`${idb}-cmp`}><span className="bar" /><h3 style={{ margin: 0 }}>{source} → Go</h3></div>
          <p className="muted">The same program, written in {source} and in its Go port — the API is designed to read the same way.</p>
          <div className="compare">
            <CompareCard name={source} color="var(--node)" html={hi(lib.node_code)} />
            <CompareCard name="Go" color="var(--go)" html={hi(lib.go_code)} />
          </div>

          <div className="sec-h" id={`${idb}-more`}><span className="bar" /><h3 style={{ margin: 0 }}>Going further</h3></div>
          <p className="muted">A fuller example wiring {lib.name} into a real program.</p>
          <CodeBlock lang="go" html={lib.integrate} />

          <div className="note">
            More runnable, per-package examples are in the <button type="button" className="linklike" onClick={() => setSub('api')}>API reference</button> — every function and type documents its own usage.
          </div>
        </div>
      )}

      {sub === 'api' && (
        <div className="libpanel" role="tabpanel">
          <div className="sec-h" id={`${idb}-api`}><span className="bar" /><h3 style={{ margin: 0 }}>API reference</h3></div>
          <p className="muted">The complete package-by-package Go API reference, generated from source — every exported type, function and method, with signatures, doc comments and runnable examples.</p>
          {/* Full Javadoc-style reference rendered inline. hashRouting is off so the
              renderer does not fight the aggregator's hash-based tab router. The
              doc index is the per-library file bundled by gendocs at build time.
              The header symbol search is backed by the Elasticsearch /api/search
              endpoint scoped to this library (docKey mirrors symbols.library),
              falling back to the local index on GitHub Pages / any failure. */}
          <DocsApp
            url={withBase(`docs/${docKey(lib)}.json`)}
            title={lib.pkg}
            hashRouting={false}
            searchEndpoint={withBase('api/search')}
            library={docKey(lib)}
          />
        </div>
      )}

      {sub === 'parity' && (
        <div className="libpanel" role="tabpanel">
          <div className="sec-h" id={`${idb}-parity`}><span className="bar" /><h3 style={{ margin: 0 }}>Upstream parity</h3></div>
          {parity ? (
            <>
              <p className="muted">
                This score is <b>live</b> — regenerated on every deploy from{' '}
                <a href={`${lib.repo}/blob/main/parity.json`} target="_blank" rel="noopener"><code>parity.json</code></a>,
                which the port's parity CI pipeline publishes. It measures the Go port against the original library by syncing
                that library's own test suite and closing the gaps those tests expose. See the{' '}
                <Link href="/parity">Parity</Link> tab for exactly how the score is calculated, and the{' '}
                <Link href="/pipeline">Pipeline</Link> tab to watch {lib.name}'s CI run stage by stage.
              </p>
              <div className="parity-tiles">
                <div className="parity-tile" style={{ '--lib-accent': lib.accent } as CSSProperties}>
                  <div className="parity-num">{parity.after}</div><div className="parity-lbl">measured parity</div>
                </div>
                <div className="parity-tile">
                  <div className="parity-num">{parity.before} → {parity.after}</div><div className="parity-lbl">before → after audit</div>
                </div>
                <div className="parity-tile">
                  <div className="parity-num">{parity.casesSynced.toLocaleString()}</div><div className="parity-lbl">upstream test cases synced</div>
                </div>
                <div className="parity-tile">
                  <div className="parity-num">{parity.gapsClosed}</div><div className="parity-lbl">behavior gaps closed</div>
                </div>
              </div>
            </>
          ) : (
            <p className="muted">
              {lib.name} is not yet audited against an upstream test suite, so it has no live parity score. Its pipeline still
              builds and tests the port on every push — see the <Link href="/pipeline">Pipeline</Link> tab, or{' '}
              <a href={`${lib.repo}/actions`} target="_blank" rel="noopener">view CI ↗</a>.
            </p>
          )}
        </div>
      )}

      <style>{libTabsCss}</style>
    </section>
  );
}

const libTabsCss = `
.libtabs { display:flex; flex-wrap:wrap; gap:.4rem; margin:1rem 0 1.4rem; border-bottom:1px solid var(--edge); padding-bottom:.6rem; }
.libtab {
  font: inherit; font-size:.92rem; font-weight:600; cursor:pointer;
  padding:.4rem .85rem; border-radius:999px; border:1px solid transparent;
  background:transparent; color:var(--fg-muted); transition:color .12s, background .12s, border-color .12s;
}
.libtab:hover { color:var(--fg); background:var(--glass); }
.libtab.active {
  color:var(--fg); background:var(--glass-2); box-shadow:var(--hi);
  border-color:var(--edge); border-bottom:2px solid var(--lib-accent, var(--accent));
}
.libpanel { animation:libpanel-in .18s ease both; }
@keyframes libpanel-in { from { opacity:0; transform:translateY(4px); } to { opacity:1; transform:none; } }
.linklike { background:none; border:none; padding:0; font:inherit; color:var(--link); cursor:pointer; text-decoration:underline; }
@media (prefers-reduced-motion: reduce) { .libpanel { animation:none; } }
`;
