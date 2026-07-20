'use client';

import type { CSSProperties } from 'react';
import Link from 'next/link';
import { VersionBadge, hx, ghrepo } from 'go-ui';
import type { Lib } from '../../data';
import { Html } from '../Html';

export interface LibHeroProps {
  lib: Lib;
}

// LibHero is the persistent header shared by every sub-page of a library
// (Overview / Examples / API / Parity): the icon, name, package path, tagline,
// action pills and the one-line blurb. It lives in the library layout so it
// stays mounted as the reader moves between the sub-pages.
export function LibHero({ lib }: LibHeroProps) {
  return (
    <>
      <div className="libhero" style={{ '--lib-soft': hx(lib.accent, '1f'), '--lib-accent': lib.accent } as CSSProperties}>
        <div className="row">
          <Html html={lib.icon} className="mono" />
          <div style={{ flex: 1, minWidth: 220 }}>
            <h2>{lib.name} <span className="muted" style={{ fontWeight: 400, fontSize: '1rem' }}>for Go</span></h2>
            <div className="pkg mono">{lib.pkg}</div>
            <p className="tagline">{lib.tagline}</p>
          </div>
        </div>
        <div className="actions">
          <a className="pill b" href={lib.repo} target="_blank" rel="noopener"><i className="fa-brands fa-github" />&nbsp;GitHub</a>
          {/* The docs are self-contained: "API docs" links to this library's API
              sub-page rather than the external github.io docs site. Source ≠ docs,
              so the GitHub pill above still points at the repository. */}
          <Link className="pill b" href={`/lib/${lib.id}/api`}><i className="fa-solid fa-book" /> API docs</Link>
          <VersionBadge repo={ghrepo(lib)} href={`${lib.repo}/releases`} />
          <span className="pill">ports <b style={{ color: 'var(--fg)', marginLeft: '.25rem' }}>{lib.node}</b></span>
        </div>
      </div>

      <p className="muted">{lib.blurb}</p>
    </>
  );
}