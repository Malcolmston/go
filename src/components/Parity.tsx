import Link from 'next/link';
import { hx } from 'go-ui';
import { LIBS } from '../data';
import { parityFor } from '../parityLookup';
import { SecH } from './SecH';

// Parity is the dedicated tab explaining HOW the upstream-parity score is
// calculated, with the methodology, the live pipeline it runs through, and a
// complete per-library table of the current numbers.
export function Parity() {
  const rows = LIBS.map((lib) => ({ lib, p: parityFor(lib) })).filter((r) => r.p);
  const sorted = [...rows].sort((a, b) => {
    const av = parseInt(a.p!.after), bv = parseInt(b.p!.after);
    return bv - av || b.p!.casesSynced - a.p!.casesSynced;
  });
  const totalCases = rows.reduce((s, r) => s + r.p!.casesSynced, 0);
  const totalGaps = rows.reduce((s, r) => s + r.p!.gapsClosed, 0);
  const at100 = rows.filter((r) => parseInt(r.p!.after) >= 100).length;
  const unaudited = LIBS.length - rows.length;

  const steps: [string, string][] = [
    ['Sync the upstream test suite', "Fetch the ORIGINAL library's own tests — the real vectors from expressjs/express, lodash, the RFC appendices, the official JSON-Schema / URI-Template conformance suites, etc. — and encode each as a Go input → expected-output case in a <code>*_parity_test.go</code> file (named <code>TestParity*</code>)."],
    ['Classify each vector', 'Run the synced cases against the port and bucket each one: <b>mapped-passing</b> (already correct), <b>mapped-wrong</b> (the port disagrees with upstream), or <b>missing</b> (behavior the port lacks).'],
    ['Close the gaps', 'Fix the port so every mapped-wrong case passes and add missing behavior where a small, targeted, clearly-correct change matches upstream. Gaps needing a large refactor are recorded honestly rather than hidden.'],
    ['Publish parity.json', 'Each repo writes a machine-readable <code>parity.json</code> — <code>{ upstream, parityBefore, parityAfter, upstreamCasesSynced, gapsClosed }</code> — as its parity CI artifact.'],
    ['Score, live', 'This landing regenerates every score from those <code>parity.json</code> files on each deploy (via <code>genparity</code>), so the numbers below are always current — never a hand-edited snapshot.'],
  ];

  return (
    <section className="view active" id="view-parity">
      <SecH h="h2">How upstream parity is calculated</SecH>
      <p className="muted">
        Every port is measured against the <b>original</b> library, not by eyeball. The parity score is the port's
        measured fidelity to that upstream <i>after</i> closing the behavior gaps the upstream's own test suite exposes.
        Here's the exact method, stage by stage — the same pipeline you can watch on the <Link href="/pipeline">Pipeline</Link> tab.
      </p>

      <div className="parity-stats">
        <div className="parity-tile"><div className="parity-num">{rows.length}</div><div className="parity-lbl">libraries audited vs upstream</div></div>
        <div className="parity-tile"><div className="parity-num">{totalCases.toLocaleString()}</div><div className="parity-lbl">upstream test cases synced</div></div>
        <div className="parity-tile"><div className="parity-num">{totalGaps}</div><div className="parity-lbl">behavior gaps closed</div></div>
        <div className="parity-tile"><div className="parity-num">{at100}</div><div className="parity-lbl">at 100% parity</div></div>
      </div>

      <SecH>The method</SecH>
      <ol className="parity-steps">
        {steps.map(([t, d], i) => (
          <li key={i}>
            <span className="parity-step-n">{i + 1}</span>
            <div><b>{t}.</b> <span dangerouslySetInnerHTML={{ __html: d }} /></div>
          </li>
        ))}
      </ol>
      <div className="note">
        <b>What the number means.</b> <code>after</code> is the share of the upstream's exercised behavior the port
        reproduces once gaps are closed; <code>before → after</code> shows how far the audit moved it. Multi-package
        libraries are additionally audited <b>per sub-package</b> against each sub-package's own upstream. Every repo's
        pipeline routes through one central reusable workflow,{' '}
        <a href="https://github.com/malcolmston/go/blob/main/.github/workflows/parity-reusable.yml" target="_blank" rel="noopener"><code>parity-reusable.yml</code></a>.
      </div>

      <SecH>Per-library parity</SecH>
      <div className="parity-table-wrap">
        <table className="parity-table">
          <thead>
            <tr><th>Library</th><th>Upstream</th><th className="num">Before</th><th className="num">After</th><th className="num">Cases</th><th className="num">Gaps closed</th></tr>
          </thead>
          <tbody>
            {sorted.map(({ lib, p }) => (
              <tr key={lib.id}>
                <td><Link href={`/lib/${lib.id}`} style={{ color: lib.accent, fontWeight: 600 }}>{lib.name}</Link></td>
                <td className="mono"><a href={`https://github.com/${p!.upstream}`} target="_blank" rel="noopener">{p!.upstream}</a></td>
                <td className="num mono">{p!.before}</td>
                <td className="num"><span className="parity-pill" style={{ borderColor: hx(lib.accent, '55'), color: lib.accent, background: hx(lib.accent, '14') }}>{p!.after}</span></td>
                <td className="num mono">{p!.casesSynced.toLocaleString()}</td>
                <td className="num mono">{p!.gapsClosed}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {unaudited > 0 && (
        <p className="muted small" style={{ marginTop: '.7rem' }}>
          {unaudited} librar{unaudited === 1 ? 'y is' : 'ies are'} not yet audited against an upstream test suite, so {unaudited === 1 ? 'it has' : 'they have'} no score.
        </p>
      )}
      <style>{parityCss}</style>
    </section>
  );
}

const parityCss = `
.parity-stats { display:grid; grid-template-columns:repeat(auto-fit,minmax(11rem,1fr)); gap:.7rem; margin:1rem 0 .4rem; }
.parity-steps { list-style:none; padding:0; margin:.6rem 0 1rem; display:flex; flex-direction:column; gap:.7rem; }
.parity-steps li { display:flex; gap:.8rem; align-items:flex-start; color:var(--fg-muted); line-height:1.6; }
.parity-step-n { flex:0 0 auto; width:1.7rem; height:1.7rem; border-radius:50%; display:grid; place-items:center; font-weight:700; font-size:.85rem; color:var(--fg); background:var(--glass-2); border:1px solid var(--edge); }
.parity-steps b, .note b { color:var(--fg); }
.parity-table-wrap { overflow-x:auto; border:1px solid var(--edge); border-radius:12px; }
.parity-table { width:100%; border-collapse:collapse; font-size:.88rem; }
.parity-table th, .parity-table td { text-align:left; padding:.55rem .8rem; border-bottom:1px solid var(--edge); white-space:nowrap; }
.parity-table thead th { color:var(--fg-dim); font-weight:600; font-size:.78rem; text-transform:uppercase; letter-spacing:.04em; }
.parity-table tbody tr:last-child td { border-bottom:none; }
.parity-table tbody tr:hover { background:var(--glass); }
.parity-table .num { text-align:right; }
.parity-table .mono { font-family:"SF Mono",ui-monospace,monospace; font-size:.82rem; color:var(--fg-muted); }
.parity-pill { display:inline-block; padding:.05rem .5rem; border-radius:999px; border:1px solid; font-weight:600; font-size:.8rem; }
`;
