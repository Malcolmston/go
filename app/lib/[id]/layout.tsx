// Per-library layout: the shell shared by every sub-page of /lib/<id>
// (Overview / Examples / API / Parity). It renders the persistent hero + the
// sub-page nav once and keeps them mounted while `children` — the active
// sub-page — swaps, so each concern is its own route/URL instead of one
// tabbed page.
//
// This is a SERVER component so it can export generateStaticParams(); the
// static export (output:'export' on GitHub Pages) emits every /lib/<id>* page
// from it. Unknown ids are never generated, so a bad id 404s.
import type { ReactNode } from 'react';
import { notFound } from 'next/navigation';
import { LIBS } from '../../../src/data';
import { LibHero } from '../../../src/components/lib/LibHero';
import { LibNav } from '../../../src/components/lib/LibNav';

// Emit one set of static pages per library so the export writes every
// /lib/<id>* route. Covers plain ids (express) and dotted ones (socket.io).
export function generateStaticParams() {
  return LIBS.map((lib) => ({ id: lib.id }));
}

export default async function LibLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const lib = LIBS.find((l) => l.id === decodeURIComponent(id));
  if (!lib) notFound();

  return (
    <section className="view active" id={`view-${lib.id}`}>
      <LibHero lib={lib} />
      <LibNav libId={lib.id} libName={lib.name} accent={lib.accent} />
      {children}
      <style>{libTabsCss}</style>
    </section>
  );
}

const libTabsCss = `
.libtabs { display:flex; flex-wrap:wrap; gap:.4rem; margin:1rem 0 1.4rem; border-bottom:1px solid var(--edge); padding-bottom:.6rem; }
.libtab {
  font: inherit; font-size:.92rem; font-weight:600; cursor:pointer; text-decoration:none;
  padding:.4rem .85rem; border-radius:999px; border:1px solid transparent;
  background:transparent; color:var(--fg-muted); transition:color .12s, background .12s, border-color .12s;
}
.libtab:hover { color:var(--fg); background:var(--glass); }
.libtab.active {
  color:var(--fg); background:var(--glass-2); box-shadow:var(--hi);
  border-color:var(--edge); border-bottom:2px solid var(--lib-accent, var(--accent));
}
.libpanel { animation:libpanel-in .18s ease both; }
@keyframes libpanel-in { from { opacity:0; transform:translateY(4px); } to { opacity:1; transform:none; } }
@media (prefers-reduced-motion: reduce) { .libpanel { animation:none; } }
`;
