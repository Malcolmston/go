'use client';

import type { CSSProperties } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';

export interface LibNavProps {
  libId: string;
  libName: string;
  accent: string;
}

// The four library sub-pages, in order. `seg` is the trailing path segment
// under /lib/<id> ('' is the Overview index route).
const SUBS: { seg: string; label: string }[] = [
  { seg: '', label: 'Overview' },
  { seg: 'examples', label: 'Examples' },
  { seg: 'api', label: 'API' },
  { seg: 'parity', label: 'Parity' },
];

// LibNav is the library's sub-page switcher. Each tab is a real route
// (/lib/<id>, /lib/<id>/examples, /lib/<id>/api, /lib/<id>/parity), so a page is
// its own URL — shareable, back-button-friendly, individually server-rendered —
// rather than in-page state. The active tab is derived from the pathname.
export function LibNav({ libId, libName, accent }: LibNavProps) {
  const pathname = usePathname() ?? '';
  const base = `/lib/${libId}`;
  // Strip a possible /go basePath and trailing slash, then read the segment
  // after the library id ('' for the Overview index route).
  const rel = pathname.replace(/\/+$/, '');
  const afterBase = rel.slice(rel.indexOf(base) + base.length).replace(/^\//, '');
  const activeSeg = afterBase.split('/')[0] ?? '';

  return (
    <div className="libtabs" role="tablist" aria-label={`${libName} sections`}>
      {SUBS.map((t) => {
        const active = t.seg === activeSeg;
        return (
          <Link
            key={t.seg || 'overview'}
            role="tab"
            aria-selected={active}
            className={`libtab${active ? ' active' : ''}`}
            style={active ? ({ '--lib-accent': accent } as CSSProperties) : undefined}
            href={t.seg ? `${base}/${t.seg}` : base}
          >
            {t.label}
          </Link>
        );
      })}
    </div>
  );
}
