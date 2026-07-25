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
    ['Install the original library', "The port's parity suite pins the upstream package it mirrors — <code>chalk@5.3.0</code>, <code>numpy==2.0.1</code>, the real gem — and installs it. Nothing is transcribed from memory or from docs; the library itself is fetched."],
    ['Run it', 'CI starts the upstream runtime (Node, CPython, CRuby) and keeps it open for the whole suite. Every case is a question put to the original library in its own ecosystem, and its answer is the expectation the port is measured against.'],
    ['Ask the port the same question', 'The Go port answers the identical case. Both values are normalized to one canonical JSON shape — numbers as numbers, bytes as base64, time as RFC 3339 — so a difference is a difference in behavior, not in how two ecosystems spell the same value. Errors are compared too: returning a value where upstream throws is a gap.'],
    ['Cross-compile', 'The same run builds the port for every target the family claims — linux, darwin and windows on amd64/arm64, <code>CGO_ENABLED=0</code>. A behavior score for code that will not build where it ships is not a score.'],
    ['Publish and score, live', "Each repo writes a measured <code>parity.json</code> — counts, named failing cases, upstream version, build targets. This landing regenerates the table below from those files on each deploy (via <code>genparity</code>), so a new upstream release re-scores every port on its own."],
  ];

  const measured = rows.filter((r) => r.p!.mode === 'live' || r.p!.mode === 'mixed').length;

  return (
    <section className="view active" id="view-parity">
      <SecH h="h2">How upstream parity is calculated</SecH>
      <p className="muted">
        Every port is measured by <b>running the original library</b>. CI installs the real npm / PyPI / RubyGems
        package, starts it in its own runtime, asks it and the Go port the same questions, and records every
        disagreement. Nothing below is a hand-written expectation that can drift: when upstream ships a new release,
        it re-scores the ports itself. Here's the method, stage by stage — the same pipeline you can watch on the{' '}
        <Link href="/pipeline">Pipeline</Link> tab.
      </p>

      <div className="parity-stats">
        <div className="parity-tile"><div className="parity-num">{rows.length}</div><div className="parity-lbl">libraries audited vs upstream</div></div>
        <div className="parity-tile"><div className="parity-num">{totalCases.toLocaleString()}</div><div className="parity-lbl">cases asked of both implementations</div></div>
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
        <b>What the number means.</b> <code>after</code> is the share of cases where the port and the original library
        agree, measured on this run; <code>before</code> scores the <i>same</i> cases using the previous run's failures,
        so <code>before → after</code> is movement the port earned and cannot be improved by dropping a case. Failing
        cases are named in <code>parity.json</code>, never rounded away, and a deliberate divergence is recorded as a
        gap rather than a pass. {measured > 0 && <>Rows marked <span className="parity-mode">live</span> were answered by the upstream
        library during that run; the rest replay upstream's own recorded answers where its runtime could not be installed. </>}
        Multi-package libraries are additionally audited <b>per sub-package</b> against each sub-package's own upstream.
        Every repo's pipeline routes through one central reusable workflow,{' '}
        <a href="https://github.com/malcolmston/go/blob/main/.github/workflows/parity-reusable.yml" target="_blank" rel="noopener"><code>parity-reusable.yml</code></a>.
      </div>

      <SecH>Per-library parity</SecH>
      <div className="parity-table-wrap">
        <table className="parity-table">
          <thead>
            <tr><th>Library</th><th>Upstream</th><th>Ran as</th><th className="num">Before</th><th className="num">After</th><th className="num">Cases</th><th className="num">Gaps closed</th></tr>
          </thead>
          <tbody>
            {sorted.map(({ lib, p }) => (
              <tr key={lib.id}>
                <td><Link href={`/lib/${lib.id}`} style={{ color: lib.accent, fontWeight: 600 }}>{lib.name}</Link></td>
                <td className="mono"><a href={`https://github.com/${p!.upstream}`} target="_blank" rel="noopener">{p!.upstream}</a></td>
                <td className="mono">
                  {p!.upstreamPkg ? (
                    <>
                      <span className="parity-mode" title={`Answered by ${p!.upstreamPkg} running under ${p!.runtime}`}>{p!.mode}</span>{' '}
                      {p!.upstreamPkg}
                    </>
                  ) : (
                    <span className="muted">—</span>
                  )}
                </td>
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
.parity-mode { display:inline-block; padding:.02rem .35rem; border-radius:5px; border:1px solid var(--edge); background:var(--glass-2); color:var(--fg-dim); font-size:.7rem; text-transform:uppercase; letter-spacing:.04em; }
`;
