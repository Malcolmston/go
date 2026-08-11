'use client';
import { useRef, useState } from 'react';
import Link from 'next/link';
import { withBase } from '../basePath';
import type { ParityHarness } from './ParityData';
import { normalizeParityData, pct } from './ParityData';
import type { ParityNestedSummary } from './ParityNested';
import { nestedSummaries } from './ParityNested';
import { ParityHarnessView } from './ParityHarnessView';
import { ScorePill } from './ParityTables';
import { SecH } from './SecH';
import './Parity.css';

// A nested map is big by design (express: ~1.2 MB of harnesses) but it is still
// one library's slice, not the whole corpus. The guard is here so a
// misconfigured host that answers this route with the full parity.json (5.6 MB)
// or an HTML error page cannot be pulled into the page.
const NESTED_MAX_BYTES = 4 * 1024 * 1024;

export interface ParityLibraryProps {
  /** The harness key, i.e. the parity/<slug> directory name. */
  slug: string;
  name: string;
  accent: string;
  /** GitHub repo of the port, when the site knows it. */
  repo: string | null;
  /** /lib/<id> route of the port, when the site has that page. */
  libRoute: string | null;
  generatedAt: string;
  /**
   * The top-level harness. Its `nested` map may be empty: the page strips it to
   * keep it out of the RSC payload. When it IS populated (a test, or a caller
   * that already holds the data) the disclosures open without a fetch.
   */
  harness: ParityHarness;
  /**
   * Collapsed-row data for the nested ports. Defaults to whatever `harness.nested`
   * carries, so a caller with the full harness needs to pass nothing.
   */
  nested?: ParityNestedSummary[];
}

/**
 * One library's complete parity record: what it was measured against, the
 * stages of the run that measured it, every symbol, every case — and the same
 * again for each nested package port, which is a port of a DIFFERENT upstream
 * package that happens to live in the same repo.
 */
export function ParityLibrary({
  slug,
  name,
  accent,
  repo,
  libRoute,
  generatedAt,
  harness,
  nested: nestedProp,
}: ParityLibraryProps) {
  const nested = nestedProp ?? nestedSummaries(harness.nested);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  // Nested harness bodies, keyed like `nested`. Seeded from the harness when the
  // caller passed them inline; otherwise filled by one fetch of this library's
  // nested route the first time a disclosure is opened.
  const [bodies, setBodies] = useState<Record<string, ParityHarness>>(
    () => harness.nested ?? {}
  );
  const [loadState, setLoadState] = useState<'idle' | 'loading' | 'error'>('idle');
  const requested = useRef(false);

  function loadBodies() {
    if (requested.current) return;
    requested.current = true;
    setLoadState('loading');
    fetch(withBase(`parity/${encodeURIComponent(slug)}/nested`))
      .then((res) => {
        const len = Number(res.headers.get('content-length') ?? 0);
        if (!res.ok || len > NESTED_MAX_BYTES) {
          void res.body?.cancel();
          throw new Error('nested harnesses unavailable');
        }
        return res.json();
      })
      .then((raw: unknown) => {
        const libraries =
          raw && typeof raw === 'object'
            ? (raw as { nested?: unknown }).nested
            : null;
        setBodies(normalizeParityData({ libraries }).libraries);
        setLoadState('idle');
      })
      .catch(() => {
        // Allow a retry: the next open re-requests rather than staying broken.
        requested.current = false;
        setLoadState('error');
      });
  }

  const toggle = (key: string) => {
    setOpen((o) => ({ ...o, [key]: !o[key] }));
    if (!open[key] && !bodies[key]) loadBodies();
  };

  const nestedCases = nested.reduce((s, n) => s + n.total, 0);
  const nestedSymbols = nested.reduce((s, n) => s + n.symbols, 0);

  return (
    <section className="view active" id={`view-parity-${slug}`}>
      <p className="parity-back">
        <Link href="/parity">Parity</Link> / {name}
      </p>
      <SecH h="h2">
        {name} measured against {harness.upstream.raw || 'its upstream'}
      </SecH>
      <p className="muted parity-lede">
        Every number on this page was produced by running both implementations over the same cases:
        the real {harness.upstream.name || 'upstream'} package{' '}
        {harness.upstream.version ? <>pinned at <code>{harness.upstream.version}</code></> : 'as published'} answers first,
        and its answer is the expectation the Go port is held to. Nothing is a hand-written
        expectation, so a new upstream release re-scores the port on its own.
        {libRoute && <> See <Link href={libRoute}>{name}</Link> for the port's own documentation.</>}
        {repo && <> Source: <a href={repo} target="_blank" rel="noopener">{repo.replace('https://', '')}</a>.</>}
      </p>

      <nav className="parity-onthis" aria-label={`Sections of the ${name} parity record`}>
        <a href={`#${slug}-pipeline`}>Stages of the run</a>
        <a href={`#${slug}-groups`}>Groups</a>
        <a href={`#${slug}-symbols`}>Symbols</a>
        <a href={`#${slug}-cases`}>Cases</a>
        {harness.security.length > 0 && <a href={`#${slug}-security`}>Security</a>}
        {nested.length > 0 && <a href={`#${slug}-nested`}>Nested ports ({nested.length})</a>}
      </nav>

      <ParityHarnessView
        harness={harness}
        accent={accent}
        idPrefix={slug}
        h="h3"
        generatedAt={generatedAt}
        runLabel={`${name} parity run`}
      />

      {nested.length > 0 && (
        <>
          <SecH h="h3" id={`${slug}-nested`}>Nested package ports ({nested.length})</SecH>
          <p className="parity-sec">
            {name}&apos;s repo also ports {nested.length} package{nested.length === 1 ? '' : 's'} that
            each have their <b>own</b> upstream — for example a <code>qs</code> port inside the
            express repo is measured against npm <code>qs</code>, not against express. Each has a
            separate harness under <code>{harness.harnessPath || `parity/${slug}`}/nested/</code>, its
            own pinned oracle, and its own score; none of them changes the {name} score above.
            Together they add {nestedCases.toLocaleString()} cases and{' '}
            {nestedSymbols.toLocaleString()} inventoried symbols.
          </p>
          <div className="parity-table-wrap" tabIndex={0} role="group" aria-label="Nested package ports">
            <table className="parity-table">
              <thead>
                <tr>
                  <th scope="col">Nested port</th>
                  <th scope="col">Its own upstream</th>
                  <th scope="col">Go module</th>
                  <th scope="col" className="num">Cases</th>
                  <th scope="col" className="num">Mismatch</th>
                  <th scope="col" className="num">Deviations</th>
                  <th scope="col" className="num">Symbols</th>
                  <th scope="col" className="num">Parity</th>
                </tr>
              </thead>
              <tbody>
                {nested.map((n) => (
                  <tr key={n.key}>
                    <td className="mono"><a href={`#${slug}-nested-${n.key}`}>{n.library || n.key}</a></td>
                    <td className="mono">{n.upstream || '—'}</td>
                    <td className="mono">{n.goModule || '—'}</td>
                    <td className="num">{n.total.toLocaleString()}</td>
                    <td className="num">{n.mismatch.toLocaleString()}</td>
                    <td className="num">{n.deviations.toLocaleString()}</td>
                    <td className="num">{n.symbols.toLocaleString()}</td>
                    <td className="num">{pct(n.parityPercent, n.comparedTotal)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="parity-nested">
            {nested.map((n) => {
              const key = n.key;
              const id = `${slug}-nested-${key}`;
              const isOpen = open[key] === true;
              const body = bodies[key];
              return (
                <div className="parity-nested-item" key={key} id={id}>
                  <h4 style={{ margin: 0 }}>
                    <button
                      type="button"
                      className="parity-disc"
                      aria-expanded={isOpen}
                      aria-controls={`${id}-body`}
                      onClick={() => toggle(key)}
                    >
                      <span className="parity-disc-mark" aria-hidden="true">{isOpen ? '[-]' : '[+]'}</span>
                      <span className="parity-disc-title">{n.library || key}</span>
                      <span className="parity-disc-sub">
                        ports {n.upstream || 'an upstream package'} · {n.total.toLocaleString()} cases ·{' '}
                        {n.symbols.toLocaleString()} symbols
                      </span>
                      <ScorePill value={n.parityPercent} compared={n.comparedTotal} accent={accent} />
                    </button>
                  </h4>
                  {/* Mounted only when opened, and its harness is fetched only
                      then too: a repo like express carries 28 nested harnesses,
                      which are megabytes of cases and would build tens of
                      thousands of rows nobody asked for. */}
                  {isOpen && (
                    <div className="parity-disc-body" id={`${id}-body`}>
                      {body ? (
                        <ParityHarnessView
                          harness={body}
                          accent={accent}
                          idPrefix={id}
                          h="h5"
                          generatedAt={generatedAt}
                          runLabel={`${n.library || key} parity run`}
                        />
                      ) : (
                        <p className="muted" role="status">
                          {loadState === 'error'
                            ? `The ${n.library || key} harness could not be loaded. Open it again to retry.`
                            : `Loading the ${n.library || key} harness…`}
                        </p>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </>
      )}
    </section>
  );
}
