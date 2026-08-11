'use client';
import type { ElementType } from 'react';
import type { ParityHarness } from './ParityData';
import { inventoryHint, pct, runnerFile, runtimeLabel } from './ParityData';
import { PipelineFlow } from './PipelineFlow';
import type { PipelineRun } from './PipelineFlow';
import { CaseTable, CoverageTable, GroupTable, ScorePill, Tiles } from './ParityTables';
import { SecH } from './SecH';
import './Parity.css';

export interface ParityHarnessViewProps {
  harness: ParityHarness;
  /** Accent used for score pills and the pipeline canvas. */
  accent: string;
  /** Unique prefix for element ids and section anchors. */
  idPrefix: string;
  /** Heading element for this harness's own sections. */
  h?: ElementType;
  generatedAt: string;
  /** Human label for the pipeline panel, e.g. "express parity run". */
  runLabel: string;
}

export function pipelineRunFor(h: ParityHarness, accent: string, generatedAt: string): PipelineRun {
  return {
    accent,
    harnessPath: h.harnessPath,
    upstream: h.upstream,
    goModule: h.goModule,
    goVersion: h.goVersion,
    caseCount: h.total,
    groupCount: h.groups.length,
    comparedTotal: h.comparedTotal,
    match: h.match,
    mismatch: h.mismatch,
    deviations: h.deviations,
    parityPercent: h.parityPercent,
    symbolCount: h.coverage.length,
    generatedBy: h.generatedBy,
    generatedAt,
  };
}

/**
 * The full, auditable record of ONE harness: which upstream package answered,
 * which Go module was asked, the stages the run executed, the per-group split,
 * the symbol-by-symbol inventory and every individual case. Used for a library
 * and, unchanged, for each of its nested package ports.
 */
export function ParityHarnessView({
  harness,
  accent,
  idPrefix,
  h = 'h3',
  generatedAt,
  runLabel,
}: ParityHarnessViewProps) {
  const eco = harness.upstream.ecosystem || 'upstream';
  const dir = harness.harnessPath || 'parity/<lib>';
  const cs = harness.coverageSummary;
  const symbolsCompared = cs.match + cs.differs;

  return (
    <div className="parity-harness">
      <div className="parity-idgrid">
        <div className="parity-idcard">
          <h4>Upstream oracle — the library being copied</h4>
          <dl className="parity-dl">
            <dt>Package</dt>
            <dd>{harness.upstream.name || 'unrecorded'}</dd>
            <dt>Pinned version</dt>
            <dd>{harness.upstream.version || 'unpinned'}</dd>
            <dt>Ecosystem</dt>
            <dd>{eco}</dd>
            <dt>Runtime</dt>
            <dd>{runtimeLabel(eco)}</dd>
            <dt>Runner</dt>
            <dd>{`${dir}/${eco}/${runnerFile(eco)}`}</dd>
          </dl>
        </div>
        <div className="parity-idcard">
          <h4>Go port — the code under measurement</h4>
          <dl className="parity-dl">
            <dt>Module</dt>
            <dd>{harness.goModule || 'unrecorded'}</dd>
            <dt>Version</dt>
            <dd>{harness.goVersion || 'HEAD'}</dd>
            <dt>Runner</dt>
            <dd>{`${dir}/go/run.go`}</dd>
            <dt>Cases</dt>
            <dd>{`${dir}/cases/*.json`}</dd>
            <dt>Scored by</dt>
            <dd>{harness.generatedBy || 'the harness test'}</dd>
          </dl>
        </div>
      </div>

      <Tiles
        accent={accent}
        tiles={[
          { value: <ScorePill value={harness.parityPercent} compared={harness.comparedTotal} accent={accent} />, label: 'measured parity' },
          { value: `${harness.match.toLocaleString()} / ${harness.comparedTotal.toLocaleString()}`, label: 'cases in agreement' },
          { value: harness.mismatch.toLocaleString(), label: 'mismatches' },
          { value: harness.deviations.toLocaleString(), label: 'documented deviations' },
          { value: harness.total.toLocaleString(), label: 'cases streamed to both' },
          { value: harness.coverage.length.toLocaleString(), label: 'upstream symbols inventoried' },
        ]}
      />

      <SecH h={h} id={`${idPrefix}-pipeline`}>The stages this run executed</SecH>
      <p className="parity-sec">
        Not a generic diagram: every node below names an artefact of this harness — the pinned
        package it installed, the runner files it started, the case files it streamed, and the
        counts it wrote out.
      </p>
      <PipelineFlow run={pipelineRunFor(harness, accent, generatedAt)} label={runLabel} />

      <SecH h={h} id={`${idPrefix}-groups`}>Per-group breakdown</SecH>
      <GroupTable groups={harness.groups} accent={accent} />

      <SecH h={h} id={`${idPrefix}-symbols`}>Symbol-by-symbol comparison</SecH>
      <Tiles
        tiles={[
          { value: cs.match.toLocaleString(), label: 'symbols that match' },
          { value: cs.differs.toLocaleString(), label: 'symbols that differ' },
          { value: cs.missing.toLocaleString(), label: 'upstream symbols not ported' },
          { value: cs.extra.toLocaleString(), label: 'Go-only symbols' },
          { value: cs.untested.toLocaleString(), label: 'symbols with no case' },
          { value: pct(cs.percent, symbolsCompared), label: 'parity over compared symbols' },
        ]}
      />
      <CoverageTable
        rows={harness.coverage}
        idPrefix={idPrefix}
        command={harness.coverageCommand || inventoryHint(eco)}
      />

      <SecH h={h} id={`${idPrefix}-cases`}>Every case, one row each</SecH>
      <CaseTable rows={harness.cases} idPrefix={idPrefix} />

      {harness.security.length > 0 && (
        <>
          <SecH h={h} id={`${idPrefix}-security`}>Security findings raised by these cases</SecH>
          <p className="parity-sec">
            A case can show the port is less safe than the library it ports. Those findings are
            written into <code>security.json</code> and filed as <b>draft</b> advisories, never as
            public issues with a working repro.
          </p>
          <div className="parity-table-wrap" tabIndex={0} role="group" aria-label="Security findings">
            <table className="parity-table">
              <thead>
                <tr>
                  <th scope="col">Finding</th>
                  <th scope="col">Severity</th>
                  <th scope="col">Affected</th>
                  <th scope="col">Cases</th>
                  <th scope="col">Summary</th>
                </tr>
              </thead>
              <tbody>
                {harness.security.map((f) => (
                  <tr key={f.id}>
                    <td className="mono">{f.id}</td>
                    <td><span className={`parity-status is-${f.severity === 'low' ? 'untested' : 'mismatch'}`}>{f.severity}</span></td>
                    <td className="mono">{f.affectedVersions || '—'}</td>
                    <td className="mono">{f.cases.length > 0 ? f.cases.join(', ') : '—'}</td>
                    <td>{f.summary}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
