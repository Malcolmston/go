'use client';
import { useEffect, useMemo, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { hx } from 'go-ui';
import type {
  CaseStatus,
  CoverageStatus,
  ParityCase,
  ParityCoverageRow,
  ParityGroup,
} from './ParityData';
import { CASE_STATUSES, COVERAGE_STATUSES, pct } from './ParityData';
import { accentVar } from './accentText';
import './Parity.css';
import './AccentText.css';

// The tables here can hold thousands of rows (16,900 cases and 9,200 upstream
// symbols across the repo), so nothing renders the full set up front: rows are
// filtered first and then windowed to PAGE at a time behind an explicit
// "show more" control. The count line always states the true total, so the
// window never hides how much data there is.
const PAGE = 100;

export function StatusTag({ status }: { status: string }) {
  return <span className={`parity-status is-${status || 'unknown'}`}>{status || 'unknown'}</span>;
}

export function ScorePill({ value, compared, accent }: { value: number; compared: number; accent: string }) {
  return (
    <span
      className="parity-pill acc-text"
      // The accent stays the border and the tint; the readable text colour is
      // derived from --acc per theme in AccentText.css.
      style={accentVar(accent, { borderColor: hx(accent, '55'), background: hx(accent, '14') })}
    >
      {pct(value, compared)}
    </span>
  );
}

export interface TileSpec {
  value: ReactNode;
  label: string;
}

export function Tiles({ tiles, accent }: { tiles: TileSpec[]; accent?: string }) {
  return (
    <div className="parity-stats">
      {tiles.map((t) => (
        <div
          key={t.label}
          className="parity-tile"
          style={accent ? ({ '--lib-accent': accent } as CSSProperties) : undefined}
        >
          <div className="parity-num">{t.value}</div>
          <div className="parity-lbl">{t.label}</div>
        </div>
      ))}
    </div>
  );
}

/** Shared "showing X of Y" footer with the windowing controls. */
function MoreBar({
  shown,
  total,
  noun,
  onMore,
  onAll,
}: {
  shown: number;
  total: number;
  noun: string;
  onMore: () => void;
  onAll: () => void;
}) {
  if (total === 0) return null;
  return (
    <div className="parity-more">
      <span className="parity-more-note">
        Showing {shown.toLocaleString()} of {total.toLocaleString()} {noun}
      </span>
      {shown < total && (
        <>
          <button type="button" onClick={onMore}>
            Show {Math.min(PAGE, total - shown).toLocaleString()} more
          </button>
          <button type="button" onClick={onAll}>
            Show all {total.toLocaleString()}
          </button>
        </>
      )}
    </div>
  );
}

function chipRow<T extends string>(
  all: readonly T[],
  counts: Record<string, number>,
  active: Set<T>,
  toggle: (s: T) => void,
) {
  return all.map((s) => (
    <button
      key={s}
      type="button"
      className="parity-chip"
      aria-pressed={active.has(s)}
      onClick={() => toggle(s)}
    >
      {s}
      <span className="parity-chip-n">{(counts[s] ?? 0).toLocaleString()}</span>
    </button>
  ));
}

function useToggleSet<T extends string>(all: readonly T[]) {
  const [active, setActive] = useState<Set<T>>(() => new Set(all));
  const toggle = (s: T) =>
    setActive((prev) => {
      // Everything is selected by default, so the first click on a chip means
      // "show me only these" — the reading a filter control invites — rather
      // than "hide this one". After that it behaves additively.
      if (prev.size === all.length) return new Set([s]);
      const next = new Set(prev);
      if (next.has(s)) next.delete(s);
      else next.add(s);
      // An empty selection would show nothing at all, which reads as a bug;
      // clearing the last chip restores "all" instead.
      return next.size === 0 ? new Set(all) : next;
    });
  return { active, toggle };
}

/* ------------------------------------------------------------------ */
/* symbol-by-symbol coverage                                          */
/* ------------------------------------------------------------------ */

export function CoverageTable({
  rows,
  idPrefix,
  command,
}: {
  rows: ParityCoverageRow[];
  idPrefix: string;
  command: string;
}) {
  const [query, setQuery] = useState('');
  const { active, toggle } = useToggleSet<CoverageStatus>(COVERAGE_STATUSES);
  const [visible, setVisible] = useState(PAGE);

  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of rows) c[r.status] = (c[r.status] ?? 0) + 1;
    return c;
  }, [rows]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return rows.filter((r) => {
      if (!active.has(r.status)) return false;
      if (q === '') return true;
      return (
        r.upstreamSymbol.toLowerCase().includes(q) ||
        r.goSymbol.toLowerCase().includes(q) ||
        r.note.toLowerCase().includes(q) ||
        r.cases.some((c) => c.toLowerCase().includes(q))
      );
    });
  }, [rows, active, query]);

  useEffect(() => { setVisible(PAGE); }, [query, active]);

  const searchId = `${idPrefix}-cov-q`;
  if (rows.length === 0) {
    return (
      <p className="parity-empty">
        No upstream API inventory has been recorded for this harness yet, so no symbol-by-symbol
        comparison can be shown.
      </p>
    );
  }

  return (
    <>
      <p className="parity-sec">
        Every exported symbol of the upstream package, and what the port offers for it. The upstream
        list is derived mechanically, never from a README:{' '}
        <code>{command || 'see COVERAGE.md for the command'}</code>. A symbol with no case is{' '}
        <b>untested</b>, never a match.
      </p>
      <div className="parity-filters">
        <label htmlFor={searchId}>Find a symbol</label>
        <input
          id={searchId}
          type="search"
          value={query}
          placeholder="upstream or Go symbol, case id, note"
          onChange={(e) => setQuery(e.target.value)}
        />
        {chipRow(COVERAGE_STATUSES, counts, active, toggle)}
        <span className="parity-count">{filtered.length.toLocaleString()} matching</span>
      </div>
      <div className="parity-table-wrap" tabIndex={0} role="group" aria-label="Symbol coverage table">
        <table className="parity-table">
          <thead>
            <tr>
              <th scope="col">Upstream symbol</th>
              <th scope="col">Go symbol</th>
              <th scope="col">Status</th>
              <th scope="col">Cases</th>
              <th scope="col">Note</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(0, visible).map((r, i) => (
              <tr key={`${r.upstreamSymbol}|${r.goSymbol}|${i}`}>
                <td className="mono">{r.upstreamSymbol || '—'}</td>
                <td className="mono">{r.goSymbol || '—'}</td>
                <td><StatusTag status={r.status} /></td>
                <td className="mono">{r.cases.length > 0 ? r.cases.join(', ') : '—'}</td>
                <td>{r.note || ''}</td>
              </tr>
            ))}
            {filtered.length === 0 && (
              <tr><td colSpan={5}>No symbol matches that filter.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      <MoreBar
        shown={Math.min(visible, filtered.length)}
        total={filtered.length}
        noun="symbols"
        onMore={() => setVisible((v) => v + PAGE)}
        onAll={() => setVisible(filtered.length)}
      />
    </>
  );
}

/* ------------------------------------------------------------------ */
/* case-by-case results                                               */
/* ------------------------------------------------------------------ */

export function CaseTable({ rows, idPrefix }: { rows: ParityCase[]; idPrefix: string }) {
  const [query, setQuery] = useState('');
  const [group, setGroup] = useState('');
  const { active, toggle } = useToggleSet<CaseStatus>(CASE_STATUSES);
  const [visible, setVisible] = useState(PAGE);

  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of rows) c[r.status] = (c[r.status] ?? 0) + 1;
    return c;
  }, [rows]);
  const groups = useMemo(
    () => Array.from(new Set(rows.map((r) => r.group).filter((g) => g !== ''))).sort(),
    [rows],
  );

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return rows.filter((r) => {
      if (!active.has(r.status)) return false;
      if (group !== '' && r.group !== group) return false;
      if (q === '') return true;
      return (
        r.id.toLowerCase().includes(q) ||
        r.fn.toLowerCase().includes(q) ||
        r.upstreamFn.toLowerCase().includes(q) ||
        r.goFn.toLowerCase().includes(q) ||
        r.note.toLowerCase().includes(q) ||
        r.deviation.toLowerCase().includes(q)
      );
    });
  }, [rows, active, group, query]);

  useEffect(() => { setVisible(PAGE); }, [query, group, active]);

  const searchId = `${idPrefix}-case-q`;
  const groupId = `${idPrefix}-case-g`;
  if (rows.length === 0) {
    return <p className="parity-empty">No individual cases were recorded for this harness.</p>;
  }

  return (
    <>
      <p className="parity-sec">
        Every case the harness streamed to both runners, with the exact upstream symbol and Go symbol
        it exercised. A deliberate, documented difference is a <b>deviation</b> and is counted apart
        from a <b>mismatch</b>.
      </p>
      <div className="parity-filters">
        <label htmlFor={searchId}>Find a case</label>
        <input
          id={searchId}
          type="search"
          value={query}
          placeholder="case id, symbol, note"
          onChange={(e) => setQuery(e.target.value)}
        />
        <label htmlFor={groupId}>Group</label>
        <select id={groupId} value={group} onChange={(e) => setGroup(e.target.value)}>
          <option value="">all groups</option>
          {groups.map((g) => (
            <option key={g} value={g}>{g}</option>
          ))}
        </select>
        {chipRow(CASE_STATUSES, counts, active, toggle)}
        <span className="parity-count">{filtered.length.toLocaleString()} matching</span>
      </div>
      <div className="parity-table-wrap" tabIndex={0} role="group" aria-label="Case results table">
        <table className="parity-table">
          <thead>
            <tr>
              <th scope="col">Case</th>
              <th scope="col">Group</th>
              <th scope="col">Upstream symbol</th>
              <th scope="col">Go symbol</th>
              <th scope="col">Status</th>
              <th scope="col">Note</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(0, visible).map((r, i) => (
              <tr key={`${r.id}|${i}`}>
                <td className="mono">{r.id || '—'}</td>
                <td className="nowrap">{r.group || '—'}</td>
                <td className="mono">{r.upstreamFn || r.fn || '—'}</td>
                <td className="mono">{r.goFn || '—'}</td>
                <td><StatusTag status={r.status} /></td>
                <td>{r.deviation || r.note || ''}</td>
              </tr>
            ))}
            {filtered.length === 0 && (
              <tr><td colSpan={6}>No case matches that filter.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      <MoreBar
        shown={Math.min(visible, filtered.length)}
        total={filtered.length}
        noun="cases"
        onMore={() => setVisible((v) => v + PAGE)}
        onAll={() => setVisible(filtered.length)}
      />
    </>
  );
}

/* ------------------------------------------------------------------ */
/* per-group breakdown                                                */
/* ------------------------------------------------------------------ */

export function GroupTable({ groups, accent }: { groups: ParityGroup[]; accent: string }) {
  if (groups.length === 0) {
    return <p className="parity-empty">This harness records no group breakdown.</p>;
  }
  const sorted = [...groups].sort((a, b) => b.total - a.total || a.name.localeCompare(b.name));
  return (
    <div className="parity-table-wrap" tabIndex={0} role="group" aria-label="Per-group breakdown">
      <table className="parity-table">
        <thead>
          <tr>
            <th scope="col">Case group</th>
            <th scope="col" className="num">Cases</th>
            <th scope="col" className="num">Match</th>
            <th scope="col" className="num">Mismatch</th>
            <th scope="col" className="num">Group parity</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((g) => (
            <tr key={g.name}>
              <td className="mono">{g.name}</td>
              <td className="num">{g.total.toLocaleString()}</td>
              <td className="num">{g.match.toLocaleString()}</td>
              <td className="num">{g.mismatch.toLocaleString()}</td>
              <td className="num">
                <ScorePill
                  value={g.total > 0 ? (g.match / g.total) * 100 : 0}
                  compared={g.total}
                  accent={accent}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
