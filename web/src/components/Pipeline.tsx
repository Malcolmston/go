import { useState } from 'react';
import type { ChangeEvent } from 'react';
import { LIBS } from '../data';
import { parityFor } from '../parityLookup';
import { PipelineFlow } from './PipelineFlow';
import { SecH } from './SecH';

const STAGE_DOCS: [string, string][] = [
  ['Push · PR', 'A push or pull request to a library repo triggers its parity.yml.'],
  ['parity-reusable.yml', 'Every library routes through one central reusable workflow in malcolmston/go, so all parity pipelines are identical and maintained in one place.'],
  ['Checkout', 'actions/checkout pulls the repo (and submodules) at the commit under test.'],
  ['Setup Go', 'actions/setup-go installs the toolchain and warms the module cache.'],
  ['Build', 'go build ./… compiles every package — the port must build stdlib-only, with no third-party deps.'],
  ['Parity tests', 'go test -run Parity runs the synced upstream vectors — the real test cases from the original library — and fails on any behavior gap.'],
  ['Summary', 'The run summarizes the measured parity (before → after) from the port’s parity.json.'],
  ['Publish', 'parity.json is published as a build artifact; the /go landing regenerates the live score from it on deploy.'],
];

// Pipeline is the dedicated tab for the interactive parity-pipeline diagram.
// Pick any library to watch its pipeline, with live GitHub Actions status.
export function Pipeline() {
  const withParity = LIBS.filter((l) => parityFor(l));
  const [id, setId] = useState(withParity[0]?.id ?? LIBS[0].id);
  const lib = LIBS.find((l) => l.id === id) ?? LIBS[0];
  const parity = parityFor(lib);
  const onChange = (e: ChangeEvent<HTMLSelectElement>) => setId(e.target.value);

  return (
    <section className="view active" id="view-pipeline">
      <SecH h="h2">The parity pipeline</SecH>
      <p className="muted">
        Every library's upstream-parity CI runs the same stages, wired as the graph below. Node statuses reflect the
        <b> live</b> GitHub Actions run; drag nodes, pan/zoom the canvas, or hit <b>Replay</b> to watch it execute. See
        the <a href="#parity">Parity</a> tab for how the score itself is calculated.
      </p>

      <div className="pipeline-picker">
        <label htmlFor="pl-lib">Library</label>
        <select id="pl-lib" value={id} onChange={onChange}>
          {LIBS.map((l) => (
            <option key={l.id} value={l.id}>{l.name}{parityFor(l) ? ` — ${parityFor(l)!.after}` : ' — not audited'}</option>
          ))}
        </select>
      </div>

      <PipelineFlow lib={lib} parity={parity} key={lib.id} />

      <SecH>What each stage does</SecH>
      <ol className="pipeline-stages">
        {STAGE_DOCS.map(([t, d], i) => (
          <li key={i}><b>{t}</b> — <span className="muted">{d}</span></li>
        ))}
      </ol>
      <style>{pipelineCss}</style>
    </section>
  );
}

const pipelineCss = `
.pipeline-picker { display:flex; align-items:center; gap:.6rem; margin:.4rem 0 1rem; }
.pipeline-picker label { font-size:.85rem; color:var(--fg-dim); font-weight:600; }
.pipeline-picker select {
  font: inherit; font-size:.9rem; padding:.45rem .8rem; border-radius:9px;
  border:1px solid var(--edge); background:var(--glass); color:var(--fg); cursor:pointer;
  backdrop-filter:var(--blur); min-width:14rem;
}
.pipeline-stages { margin:.4rem 0 0; padding-left:1.2rem; display:flex; flex-direction:column; gap:.45rem; line-height:1.55; }
.pipeline-stages b { color:var(--fg); font-family:"SF Mono",ui-monospace,monospace; font-size:.86rem; }
`;
