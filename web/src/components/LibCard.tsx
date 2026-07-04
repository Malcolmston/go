import type { CSSProperties } from 'react';
import { VersionBadge, hx, ghrepo } from 'go-ui';
import type { Lib } from '../data';
import { Html } from './Html';

export interface LibCardProps {
  lib: Lib;
  onOpen: (id: string) => void;
}

// LibCard is the glass card for a single library on the home grid.
export function LibCard({ lib, onOpen }: LibCardProps) {
  return (
    <div className="card lib" style={{ '--glow': lib.accent } as CSSProperties} onClick={() => onOpen(lib.id)}>
      <span className="arrow">↗</span>
      <VersionBadge repo={ghrepo(lib)} />
      <Html html={lib.icon} className="ico" />
      <h3>{lib.name} <span className="tag" style={{ borderColor: hx(lib.accent, '55'), color: lib.accent, background: hx(lib.accent, '12') }}>Go</span></h3>
      <div className="mono pkg">{lib.pkg}</div>
      <p className="muted small" style={{ margin: '.7rem 0 .5rem' }}>{lib.tagline}</p>
      <div>{lib.tags.slice(0, 4).map((t) => <span className="tag" key={t}>{t}</span>)}</div>
    </div>
  );
}
