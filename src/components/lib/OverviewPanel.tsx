'use client';

import type { CSSProperties } from 'react';
import { CodeBlock, hi } from 'go-ui';
import type { Lib } from '../../data';
import { Html } from '../Html';

// OverviewPanel is the library's Overview sub-page: install, quick start, and
// the feature list. Rendered at /lib/<id>.
export function OverviewPanel({ lib }: { lib: Lib }) {
  const idb = lib.id;
  return (
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
  );
}