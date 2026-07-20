'use client';
import { useEffect } from 'react';
import type { ReactNode } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { Layout } from 'go-ui';
import type { Tab } from 'go-ui';
import { LIBS } from '../src/data';
import { pathForTab, tabForPath } from './nav';

const TABS: Tab[] = [
  { id: 'home', label: 'Home' },
  ...LIBS.map((l) => ({ id: l.id, label: l.name, dot: l.accent })),
  { id: 'parity', label: 'Parity' },
  { id: 'pipeline', label: 'Pipeline' },
  { id: 'explore', label: 'Explore' },
  { id: 'releases', label: 'Releases' },
  { id: 'howto', label: 'How-to' },
  { id: 'faq', label: 'FAQ' },
  { id: 'ai', label: 'AI' },
  { id: 'about', label: 'About' },
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
