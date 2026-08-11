'use client';
import { useEffect, useRef } from 'react';
import type { ReactNode } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { Layout } from 'go-ui';
import type { Tab } from 'go-ui';
import { LIBS } from '../src/data';
import { labelForTab, pathForTab, tabForPath } from './nav';

const TABS: Tab[] = [
  { id: 'home', label: 'Home', icon: 'fa-solid fa-house' },
  ...LIBS.map((l) => ({ id: l.id, label: l.name, dot: l.accent })),
  { id: 'parity', label: 'Parity', icon: 'fa-solid fa-scale-balanced' },
  { id: 'security', label: 'Security', icon: 'fa-solid fa-shield-halved' },
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

// The shared go-ui shell wired to the Next router. It is a client component
// because the Layout is interactive (sidebar, theme toggle) and reads the
// router; the routed content it wraps comes from the individual page files.
export default function ClientRoot({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const active = tabForPath(pathname);
  const mainRef = useRef<HTMLDivElement | null>(null);
  // Skip the focus move on first paint: focusing on load would steal focus from
  // whatever the browser restored and scroll past the hero.
  const firstRender = useRef(true);

  useEffect(() => {
    const t = setTimeout(
      () => document.querySelectorAll('.reveal').forEach((el) => el.classList.add('in')),
      30,
    );
    return () => clearTimeout(t);
  }, [pathname]);

  // Client-side navigation swaps the document content without moving focus,
  // which strands keyboard and screen-reader users at the previous position.
  // Move focus to the content container on every route change instead.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    mainRef.current?.focus();
  }, [pathname]);

  return (
    <>
      <a className="skip-link" href="#main-content">Skip to main content</a>
      <Layout
        brand={BRAND}
        tabs={TABS}
        active={active}
        onNav={(id) => router.push(pathForTab(id))}
        github="https://github.com/malcolmston"
        footer={FOOTER}
      >
        <div id="main-content" ref={mainRef} tabIndex={-1} className="route-content">
          {children}
        </div>
      </Layout>
      {/* Announce the new route to assistive tech: a client-side navigation is
          otherwise silent because the document never reloads. */}
      <div className="sr-only" role="status" aria-live="polite">
        {labelForTab(active)}
      </div>
      <style>{shellCss}</style>
    </>
  );
}

const shellCss = `
.skip-link {
  position: absolute; left: -9999px; top: 0; z-index: 200;
  padding: .6rem 1rem; border-radius: 0 0 10px 0;
  background: var(--glass-2); color: var(--fg);
  border: 1px solid var(--edge); font-weight: 600; text-decoration: none;
}
.skip-link:focus { left: 0; }
.route-content:focus { outline: none; }
.sr-only {
  position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px;
  overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0;
}
`;
