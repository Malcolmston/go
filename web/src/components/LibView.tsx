import type { CSSProperties } from 'react';
import { CodeBlock, CompareCard, DocsApp, VersionBadge, hi, hx, ghrepo } from 'go-ui';
import type { Lib } from '../data';
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

// LibView renders a single library's full documentation tab.
export function LibView({ lib }: LibViewProps) {
  const idb = lib.id;
  const source = lib.source ?? 'Node.js';
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
          <a className="pill b" href={lib.docs} target="_blank" rel="noopener"><i className="fa-solid fa-book" /> API docs</a>
          <VersionBadge repo={ghrepo(lib)} href={`${lib.repo}/releases`} />
          <span className="pill">ports <b style={{ color: 'var(--fg)', marginLeft: '.25rem' }}>{lib.node}</b></span>
        </div>
      </div>

      <p className="muted">{lib.blurb}</p>
      <div className="onthispage">
        <a href={`#${idb}-install`}>Install</a>
        <a href={`#${idb}-quick`}>Quick start</a>
        <a href={`#${idb}-cmp`}>{source} → Go</a>
        <a href={`#${idb}-more`}>Going further</a>
        <a href={`#${idb}-feat`}>Features</a>
        <a href={`#${idb}-api`}>API reference</a>
      </div>

      <div className="sec-h" id={`${idb}-install`}><span className="bar" /><h3 style={{ margin: 0 }}>Install</h3></div>
      <CodeBlock lang="shell" html={`<span class="tok-c">$</span> go get ${lib.pkg}`} />

      <div className="sec-h" id={`${idb}-quick`}><span className="bar" /><h3 style={{ margin: 0 }}>Quick start</h3></div>
      <CodeBlock lang="main.go" html={hi(lib.go_code)} />

      <div className="sec-h" id={`${idb}-cmp`}><span className="bar" /><h3 style={{ margin: 0 }}>{source} → Go</h3></div>
      <div className="compare">
        <CompareCard name={source} color="var(--node)" html={hi(lib.node_code)} />
        <CompareCard name="Go" color="var(--go)" html={hi(lib.go_code)} />
      </div>

      <div className="sec-h" id={`${idb}-more`}><span className="bar" /><h3 style={{ margin: 0 }}>Going further</h3></div>
      <CodeBlock lang="go" html={lib.integrate} />

      <div className="sec-h" id={`${idb}-feat`}><span className="bar" /><h3 style={{ margin: 0 }}>Features</h3></div>
      <ul className="feat" style={{ '--lib-accent': lib.accent } as CSSProperties}>
        {lib.features.map((f, i) => <Html tag="li" html={f} key={i} />)}
      </ul>

      <div className="sec-h" id={`${idb}-api`}><span className="bar" /><h3 style={{ margin: 0 }}>API reference</h3></div>
      <p className="muted">The complete package-by-package Go API reference, generated from source — every exported type, function and method, with signatures, doc comments and runnable examples. Rendered inline below; also browsable on the dedicated docs site.</p>
      {/* Full Javadoc-style reference rendered inline. hashRouting is off so the
          renderer does not fight the aggregator's hash-based tab router. The
          doc index is the per-library file bundled by gendocs at build time. */}
      <DocsApp url={`${import.meta.env.BASE_URL}docs/${docKey(lib)}.json`} title={lib.pkg} hashRouting={false} />

      <div className="note">Standalone API docs &amp; runnable examples: <a href={lib.docs} target="_blank" rel="noopener">{lib.docs}</a></div>
    </section>
  );
}
