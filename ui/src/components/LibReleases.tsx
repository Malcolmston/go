import { useEffect, useState } from 'react';
import { esc } from '../highlight';
import type { RelLib } from './ReleaseList';

const OWNER = 'malcolmston';
const GH: RequestInit = { headers: { Accept: 'application/vnd.github+json' } };

const linkify = (s: string): string =>
  esc(s).replace(/https?:\/\/[^\s)]+/g, (u) => `<a href="${u}" target="_blank" rel="noopener">${u}</a>`);
const fmtDate = (iso: string): string => {
  const d = new Date(iso);
  return isNaN(d.getTime()) ? iso : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
};

interface Release {
  id: number;
  tag_name: string;
  published_at: string;
  html_url: string;
  body?: string;
}

interface State {
  loading: boolean;
  rels: Release[];
  err: boolean;
}

export interface LibReleasesProps {
  lib: RelLib;
}

// LibReleases renders the live release history + notes for a single library,
// read from the GitHub Releases API.
export function LibReleases({ lib }: LibReleasesProps) {
  const [state, setState] = useState<State>({ loading: true, rels: [], err: false });
  useEffect(() => {
    let alive = true;
    fetch(`https://api.github.com/repos/${OWNER}/${lib.repo}/releases?per_page=8`, GH)
      .then((x) => (x.ok ? x.json() : Promise.reject(x.status)))
      .then((rels: Release[]) => alive && setState({ loading: false, rels, err: false }))
      .catch(() => alive && setState({ loading: false, rels: [], err: true }));
    return () => { alive = false; };
  }, [lib.repo]);

  return (
    <div className="rel-lib">
      <div className="rel-head">
        <span className="ico" style={{ color: lib.accent }} dangerouslySetInnerHTML={{ __html: lib.icon }} />
        <h3>{lib.name}</h3>
        <a className="cl pill b" href={`${lib.url}/blob/main/CHANGELOG.md`} target="_blank" rel="noopener">
          <i className="fa-solid fa-file-lines" />&nbsp;Changelog
        </a>
      </div>
      <div className="rel-items">
        {state.loading && <div className="rel-empty">Loading releases…</div>}
        {!state.loading && state.err && (
          <div className="rel-empty">Couldn't load releases right now — see the{' '}
            <a href={`${lib.url}/releases`} target="_blank" rel="noopener">releases page</a>.</div>
        )}
        {!state.loading && !state.err && state.rels.length === 0 && (
          <div className="rel-empty">No releases yet.</div>
        )}
        {!state.loading && !state.err && state.rels.map((rel, i) => {
          const notes = (rel.body || '').trim();
          return (
            <details className="rel-item" key={rel.id} open={i === 0}>
              <summary>
                <span className="tagn">{rel.tag_name}</span>
                {i === 0 && <span className="latest">latest</span>}
                <span className="when">{fmtDate(rel.published_at)}</span>
                <a className="when" href={rel.html_url} target="_blank" rel="noopener" title="Open on GitHub" style={{ marginLeft: '.2rem' }}>
                  <i className="fa-solid fa-arrow-up-right-from-square" />
                </a>
                <span className="chev">▸</span>
              </summary>
              {notes
                ? <div className="rel-notes" dangerouslySetInnerHTML={{ __html: linkify(notes) }} />
                : <div className="rel-empty">No release notes.</div>}
            </details>
          );
        })}
      </div>
    </div>
  );
}
