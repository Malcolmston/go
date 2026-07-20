'use client';

import Link from 'next/link';
import { CodeBlock, CompareCard, hi } from 'go-ui';
import type { Lib } from '../../data';

// ExamplesPanel is the library's Examples sub-page: a side-by-side Node.js → Go
// comparison and a fuller "going further" snippet. Rendered at
// /lib/<id>/examples.
export function ExamplesPanel({ lib }: { lib: Lib }) {
  const idb = lib.id;
  const source = lib.source ?? 'Node.js';
  return (
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
        More runnable, per-package examples are in the <Link href={`/lib/${lib.id}/api`}>API reference</Link> — every function and type documents its own usage.
      </div>
    </div>
  );
}