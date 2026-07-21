'use client';
import { useEffect } from 'react';
import type { ReactNode } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { Layout } from 'go-ui';
import type { Tab } from 'go-ui';
import { LIBS } from '../src/data';
import { pathForTab, tabForPath } from './nav';

const TABS: Tab[] = [
  { id: 'home', label: 'Home', icon: 'fa-solid fa-house' },
  ...LIBS.map((l) => ({ id: l.id, label: l.name, dot: l.accent })),
  { id: 'parity', label: 'Parity', icon: 'fa-solid fa-scale-balanced' },
  { id: 'pipeline', label: 'Pipeline', icon: 'fa-solid fa-diagram-project' },
  { id: 'explore', label: 'Explore', icon: 'fa-solid fa-compass' },
  { id: 'releases', label: 'Releases', icon: 'fa-solid fa-tag' },
  { id: 'howto', label: 'How-to', icon: 'fa-solid fa-book-open' },
  { id: 'faq', label: 'FAQ', icon: 'fa-solid fa-circle-question' },
  { id: 'ai', label: 'AI', icon: 'fa-solid fa-robot' },
  { id: 'ask', label: 'Ask', icon: 'fa-solid fa-comments' },
  { id: 'about', label: 'About', icon: 'fa-solid fa-circle-info' },
];

const BRAND = { title: 'malcolmston', sub: '/go', initials: 'go', href: '/' };

const FOOTER: ReactNode = (
  <div>
    <span className="grad-text" style={{ fontWeight: 700 }}>malcolmston/go</span> — the Node.js ecosystem, reimagined in Go.
    <div className="small dim" style={{ marginTop: '.4rem' }}>MIT licensed · independent re-implementations</div>
  </div>
);

// The shared go-ui shell wired to the Next router. Rendered client-only (via the
// ssr:false dynamic in layout.tsx) so the interactive chrome never prerenders.
export default function ClientRoot({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const active = tabForPath(pathname);

  useEffect(() => {
    const t = setTimeout(
      () => document.querySelectorAll('.reveal').forEach((el) => el.classList.add('in')),
      30,
    );
    return () => clearTimeout(t);
  }, [pathname]);

  return (
    <Layout
      brand={BRAND}
      tabs={TABS}
      active={active}
      onNav={(id) => router.push(pathForTab(id))}
      github="https://github.com/malcolmston"
      footer={FOOTER}
    >
      {children}
    </Layout>
  );
}
