'use client';

import type { CSSProperties } from 'react';
import Link from 'next/link';
import type { Lib } from '../../data';
import { parityFor } from '../../parityLookup';

// ParityPanel is the library's Parity sub-page: the live upstream-parity score
// tiles, or a friendly note when the library has no audited score yet. Rendered
// at /lib/<id>/parity.
export function ParityPanel({ lib }: { lib: Lib }) {
  const idb = lib.id;
  const parity = parityFor(lib);
  return (
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
  );
}