import type { CSSProperties, KeyboardEvent } from 'react';
import { VersionBadge, hx, ghrepo } from 'go-ui';
import type { Lib } from '../data';
import { parityFor } from '../parityLookup';
import { Html } from './Html';

export interface LibCardProps {
  lib: Lib;
  onOpen: (id: string) => void;
}

// LibCard is the glass card for a single library on the home grid.
//
// The whole card is the click target, so it also has to be a keyboard target:
// it carries role="button" + tabIndex and handles Enter/Space, matching the
// ARIA button pattern. (A real <button> can't be used — the card contains a
// heading and other flow content, which is invalid inside <button>.)
export function LibCard({ lib, onOpen }: LibCardProps) {
  const parity = parityFor(lib);
  const open = () => onOpen(lib.id);
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar') return;
    e.preventDefault(); // Space would otherwise scroll the page
    open();
  };
  return (
    <div
      className="card lib"
      style={{ '--glow': lib.accent } as CSSProperties}
      role="button"
      tabIndex={0}
      aria-label={`${lib.name} — ${lib.tagline}`}
      onClick={open}
      onKeyDown={onKeyDown}
    >
      <span className="arrow" aria-hidden="true">↗</span>
      <VersionBadge repo={ghrepo(lib)} />
      <Html html={lib.icon} className="ico" />
      <h3>{lib.name} <span className="tag" style={{ borderColor: hx(lib.accent, '55'), color: lib.accent, background: hx(lib.accent, '12') }}>Go</span></h3>
      <div className="mono pkg">{lib.pkg}</div>
      <p className="muted small" style={{ margin: '.7rem 0 .5rem' }}>{lib.tagline}</p>
      <div>{lib.tags.slice(0, 4).map((t) => <span className="tag" key={t}>{t}</span>)}</div>
      {parity && (
        <div
          className="tag"
          title={`Measured against ${parity.upstreamPkg ?? parity.upstream}: ${parity.casesSynced} cases asked of both implementations, ${parity.gapsClosed} gaps closed`}
          style={{ marginTop: '.6rem', borderColor: hx(lib.accent, '55'), color: lib.accent, background: hx(lib.accent, '12') }}
        >
          ● {parity.after} upstream parity
        </div>
      )}
    </div>
  );
}
