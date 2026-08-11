'use client';
import { useMemo, useState } from 'react';
import Link from 'next/link';
import type { ParitySummary } from './ParityData';
import { pct } from './ParityData';
import { ScorePill, Tiles } from './ParityTables';
import { SecH } from './SecH';
import './Parity.css';

export interface ParityProps {
  /** When api/_data/parity.json was generated. */
  generatedAt: string;
  /** One row per library harness, without the per-case / per-symbol bulk. */
  rows: ParitySummary[];
  /** Libraries the site ships that have no harness yet. */
  unaudited: { id: string; name: string }[];
}

// The stages every harness run executes, from parity/HARNESS.md. The per-library
// page draws these as a graph labelled with that library's real artefacts; here
// they are the method, stated once.
const STAGES: [string, string][] = [
  [
    'Install the pinned upstream oracle',
    'The harness installs the real published package at an exact version — the npm tarball, the PyPI wheel, the gem, the jar. An unpinned oracle would make the score unreproducible, so the version is written into the case files.',
  ],
  [
    'Start the upstream runner',
    'A small program in the upstream ecosystem (node/, python/, java/, ruby/, rust/, elixir/, c/) is started once and kept open, speaking JSON Lines over stdio: one JSON object in per line, exactly one out. The harness never links against the library — it only talks to it.',
  ],
  [
    'Start the Go runner',
    'go/run.go implements the same contract over the Go port. Neither runner may exit on a failing case: it reports ok:false and keeps reading, so one loss never cascades into the rest of the suite.',
  ],
  [
    'Stream every case to both',
    'Cases live in cases/*.json as language-agnostic JSON, each naming the upstream symbol and the Go symbol it exercises. Every case goes to both runners, in order, through the two long-lived processes, with a per-case timeout.',
  ],
  [
    'Compare ok, then value',
    'Failure is compared before value — returning a result where upstream throws is not parity. Values are compared with a normalising deep-equal that treats every JSON number as float64, so 1 and 1.0 agree while a real behavioural difference still shows.',
  ],
  [
    'Classify match, mismatch or deviation',
    'A documented, deliberate difference is counted as a deviation and listed in the port’s API-DEVIATIONS.md; anything else that disagrees is a mismatch, named by case id. A deviation is never quietly counted as a pass.',
  ],
  [
    'Write parity.json and COVERAGE.md',
    'The run rewrites the machine-readable score plus the full upstream API inventory: every exported upstream symbol marked match, differs, missing, extra or untested, with the command that enumerated it. This page is generated from those two files.',
  ],
];

type SortKey = 'name' | 'parity' | 'cases' | 'mismatch';

/**
 * The Parity page: how a score is calculated, and the index of every measured
 * library. The pipeline lives here too — the method as text on this page, and
 * the actual stages of a specific run on each library's own record, which also
 * carries its symbol-by-symbol and case-by-case tables.
 */
export function Parity({ generatedAt, rows, unaudited }: ParityProps) {
  const [query, setQuery] = useState('');
  const [eco, setEco] = useState('');
  const [sort, setSort] = useState<SortKey>('name');
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const ecosystems = useMemo(
    () => Array.from(new Set(rows.map((r) => r.upstream.ecosystem).filter((e) => e !== ''))).sort(),
    [rows],
  );

  const totals = useMemo(() => {
    let harnesses = 0, cases = 0, match = 0, mismatch = 0, deviations = 0, symbols = 0, nested = 0;
    for (const r of rows) {
      harnesses += 1;
      cases += r.total;
      match += r.match;
      mismatch += r.mismatch;
      deviations += r.deviations;
      symbols += r.symbolCount;
      for (const n of r.nested) {
        harnesses += 1;
        nested += 1;
        cases += n.total;
        match += n.match;
        mismatch += n.mismatch;
        deviations += n.deviations;
        symbols += n.symbolCount;
      }
    }
    return { harnesses, cases, match, mismatch, deviations, symbols, nested };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const out = rows.filter((r) => {
      if (eco !== '' && r.upstream.ecosystem !== eco) return false;
      if (q === '') return true;
      return (
        r.name.toLowerCase().includes(q) ||
        r.slug.toLowerCase().includes(q) ||
        r.upstream.raw.toLowerCase().includes(q) ||
        r.goModule.toLowerCase().includes(q) ||
        r.nested.some((n) => n.name.toLowerCase().includes(q) || n.upstream.raw.toLowerCase().includes(q))
      );
    });
    return out.sort((a, b) => {
      switch (sort) {
        case 'parity':
          return b.parityPercent - a.parityPercent || a.name.localeCompare(b.name);
        case 'cases':
          return b.total - a.total || a.name.localeCompare(b.name);
        case 'mismatch':
          return b.mismatch - a.mismatch || a.name.localeCompare(b.name);
        default:
          return a.name.localeCompare(b.name);
      }
    });
  }, [rows, query, eco, sort]);

  const toggle = (slug: string) => setExpanded((e) => ({ ...e, [slug]: !e[slug] }));

  return (
    <section className="view active" id="view-parity">
      <SecH h="h2">How every parity score is calculated</SecH>
      <p className="muted parity-lede">
        A port is only worth its claim if it does what the original does, so every score here is{' '}
        <b>measured by running the original library</b>. For each port a harness installs the real
        published package at a pinned version, starts it as a subprocess in its own runtime, asks it
        and the Go port the identical questions, and records every disagreement by name. Upstream is
        the oracle: there are no hand-written expectations to drift, and a new upstream release
        re-scores the port on its own.
      </p>
      <p className="muted parity-lede">
        Pick a library below to see its complete record — the exact upstream package and version it
        was measured against, the exact Go module measured, the stages that run executed, every
        upstream symbol compared side by side with its Go counterpart, and every individual case with
        its result.
      </p>

      <Tiles
        tiles={[
          { value: totals.harnesses.toLocaleString(), label: `harnesses, including ${totals.nested.toLocaleString()} nested package ports` },
          { value: totals.cases.toLocaleString(), label: 'cases asked of both implementations' },
          { value: totals.match.toLocaleString(), label: 'cases in agreement' },
          { value: totals.mismatch.toLocaleString(), label: 'mismatches, each named by case id' },
          { value: totals.deviations.toLocaleString(), label: 'documented deviations' },
          { value: totals.symbols.toLocaleString(), label: 'upstream symbols inventoried' },
        ]}
      />

      <SecH id="method">The stages of a run</SecH>
      <p className="parity-sec">
        Every harness executes the same seven stages, defined in{' '}
        <a href="https://github.com/malcolmston/go/blob/main/parity/HARNESS.md" target="_blank" rel="noopener">
          <code>parity/HARNESS.md</code>
        </a>
        . Each library&apos;s page draws these as a graph labelled with that run&apos;s real
        artefacts: the package it installed, the runner files it started, the case files it streamed,
        and the counts it wrote out.
      </p>
      <ol className="parity-steps">
        {STAGES.map(([title, body], i) => (
          <li key={title}>
            <span className="parity-step-n">{i + 1}</span>
            <div>
              <b>{title}.</b> <span>{body}</span>
            </div>
          </li>
        ))}
      </ol>
      <div className="note">
        <b>What the number means.</b> Measured parity is the share of <i>compared</i> cases where the
        port and the original library agree. Mismatches are counted and named, deviations are counted
        separately, and a symbol with no case counts as untested rather than as a pass — so the score
        cannot be improved by dropping a case or by leaving an API uncovered. Multi-package repos are
        audited <b>per nested package</b>, each against that package&apos;s own upstream.
        {generatedAt && <> Generated {generatedAt.slice(0, 10)}.</>}
      </div>

      <SecH id="libraries">Every measured library</SecH>
      {rows.length === 0 ? (
        <p className="parity-empty">
          No parity dataset is available in this build (<code>api/_data/parity.json</code> was not
          found), so no measured scores can be shown. Run the data build to generate it.
        </p>
      ) : (
        <>
          <div className="parity-filters">
            <label htmlFor="par-q">Find a library</label>
            <input
              id="par-q"
              type="search"
              value={query}
              placeholder="library, upstream package, Go module"
              onChange={(e) => setQuery(e.target.value)}
            />
            <label htmlFor="par-eco">Ecosystem</label>
            <select id="par-eco" value={eco} onChange={(e) => setEco(e.target.value)}>
              <option value="">all</option>
              {ecosystems.map((x) => (
                <option key={x} value={x}>{x}</option>
              ))}
            </select>
            <label htmlFor="par-sort">Sort</label>
            <select id="par-sort" value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
              <option value="name">name</option>
              <option value="parity">measured parity</option>
              <option value="cases">cases</option>
              <option value="mismatch">mismatches</option>
            </select>
            <span className="parity-count">{filtered.length.toLocaleString()} of {rows.length.toLocaleString()}</span>
          </div>

          <div className="parity-table-wrap" tabIndex={0} role="group" aria-label="Measured libraries">
            <table className="parity-table">
              <thead>
                <tr>
                  <th scope="col">Library</th>
                  <th scope="col">Upstream oracle (pinned)</th>
                  <th scope="col">Ecosystem</th>
                  <th scope="col">Go module</th>
                  <th scope="col" className="num">Cases</th>
                  <th scope="col" className="num">Mismatch</th>
                  <th scope="col" className="num">Deviations</th>
                  <th scope="col" className="num">Symbols</th>
                  <th scope="col" className="num">Parity</th>
                  <th scope="col">Nested ports</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((r) => {
                  const isOpen = expanded[r.slug] === true;
                  return [
                    <tr key={r.slug}>
                      <td>
                        <Link
                          href={`/parity/${encodeURIComponent(r.slug)}`}
                          style={{ color: r.accent, fontWeight: 600 }}
                        >
                          {r.name}
                        </Link>
                      </td>
                      <td className="mono">{r.upstream.raw || '—'}</td>
                      <td className="nowrap">{r.upstream.ecosystem || '—'}</td>
                      <td className="mono">{r.goModule || '—'}</td>
                      <td className="num">{r.total.toLocaleString()}</td>
                      <td className="num">{r.mismatch.toLocaleString()}</td>
                      <td className="num">{r.deviations.toLocaleString()}</td>
                      <td className="num">{r.symbolCount.toLocaleString()}</td>
                      <td className="num"><ScorePill value={r.parityPercent} compared={r.comparedTotal} accent={r.accent} /></td>
                      <td>
                        {r.nested.length === 0 ? (
                          <span className="mono">—</span>
                        ) : (
                          <button
                            type="button"
                            className="parity-chip"
                            aria-expanded={isOpen}
                            aria-controls={`par-nested-${r.slug}`}
                            onClick={() => toggle(r.slug)}
                          >
                            {isOpen ? 'hide' : 'show'} {r.nested.length}
                          </button>
                        )}
                      </td>
                    </tr>,
                    isOpen && (
                      <tr key={`${r.slug}-nested`} className="parity-nested-row" id={`par-nested-${r.slug}`}>
                        <td colSpan={10}>
                          <b>{r.name}</b> also ports {r.nested.length} package
                          {r.nested.length === 1 ? '' : 's'} with their own upstream:
                          <ul style={{ margin: '.4rem 0 0', paddingLeft: '1.1rem' }}>
                            {r.nested.map((n) => (
                              <li key={n.key} style={{ marginBottom: '.15rem' }}>
                                <Link href={`/parity/${encodeURIComponent(r.slug)}#${r.slug}-nested-${n.key}`}>
                                  {n.name}
                                </Link>{' '}
                                <span className="mono">ports {n.upstream.raw || 'an upstream package'}</span>
                                {' — '}
                                {pct(n.parityPercent, n.comparedTotal)} over {n.comparedTotal.toLocaleString()} compared cases,{' '}
                                {n.symbolCount.toLocaleString()} symbols inventoried
                              </li>
                            ))}
                          </ul>
                        </td>
                      </tr>
                    ),
                  ];
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {unaudited.length > 0 && (
        <p className="muted small" style={{ marginTop: '.8rem' }}>
          {unaudited.length} librar{unaudited.length === 1 ? 'y has' : 'ies have'} no harness yet, so{' '}
          {unaudited.length === 1 ? 'it carries' : 'they carry'} no measured score:{' '}
          {unaudited.map((u, i) => (
            <span key={u.id}>
              {i > 0 && ', '}
              <Link href={`/lib/${encodeURIComponent(u.id)}`}>{u.name}</Link>
            </span>
          ))}
          .
        </p>
      )}
      <style>{parityCss}</style>
    </section>
  );
}

const parityCss = `
.parity-steps { list-style:none; padding:0; margin:.6rem 0 1rem; display:flex; flex-direction:column; gap:.7rem; }
.parity-steps li { display:flex; gap:.8rem; align-items:flex-start; color:var(--fg-muted); line-height:1.6; }
.parity-steps > li > div { max-width:82ch; }
.parity-step-n { flex:0 0 auto; width:1.7rem; height:1.7rem; border-radius:50%; display:grid; place-items:center; font-weight:700; font-size:.85rem; color:var(--fg); background:var(--glass-2); border:1px solid var(--edge); }
.parity-steps b, .note b { color:var(--fg); }
`;
