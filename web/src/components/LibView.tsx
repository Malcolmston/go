import type { CSSProperties } from 'react';
import { CodeBlock, CompareCard, VersionBadge, hi, hx, ghrepo } from 'go-ui';
import type { Lib } from '../data';
import { Html } from './Html';

export interface LibViewProps {
  lib: Lib;
}

// LibView renders a single library's full documentation tab.
export function LibView({ lib }: LibViewProps) {
  const idb = lib.id;
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
        <a href={`#${idb}-cmp`}>Node → Go</a>
        <a href={`#${idb}-more`}>Going further</a>
        <a href={`#${idb}-feat`}>Features</a>
      </div>

      <div className="sec-h" id={`${idb}-install`}><span className="bar" /><h3 style={{ margin: 0 }}>Install</h3></div>
      <CodeBlock lang="shell" html={`<span class="tok-c">$</span> go get ${lib.pkg}`} />

      <div className="sec-h" id={`${idb}-quick`}><span className="bar" /><h3 style={{ margin: 0 }}>Quick start</h3></div>
      <CodeBlock lang="main.go" html={hi(lib.go_code)} />

      <div className="sec-h" id={`${idb}-cmp`}><span className="bar" /><h3 style={{ margin: 0 }}>Node.js → Go</h3></div>
      <div className="compare">
        <CompareCard name="Node.js" color="var(--node)" html={hi(lib.node_code)} />
        <CompareCard name="Go" color="var(--go)" html={hi(lib.go_code)} />
      </div>

      <div className="sec-h" id={`${idb}-more`}><span className="bar" /><h3 style={{ margin: 0 }}>Going further</h3></div>
      <CodeBlock lang="go" html={lib.integrate} />

      <div className="sec-h" id={`${idb}-feat`}><span className="bar" /><h3 style={{ margin: 0 }}>Features</h3></div>
      <ul className="feat" style={{ '--lib-accent': lib.accent } as CSSProperties}>
        {lib.features.map((f, i) => <Html tag="li" html={f} key={i} />)}
      </ul>

      <div className="note">Full API reference &amp; runnable examples: <a href={lib.docs} target="_blank" rel="noopener">{lib.docs}</a></div>
    </section>
  );
}
