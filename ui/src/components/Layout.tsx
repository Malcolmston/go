import { useEffect, useState, useCallback, useRef } from 'react';
import type { ReactNode } from 'react';
import { ThemeToggle } from './ThemeToggle';
import { Wormhole } from './Wormhole';
import type { WormholeHandle } from './Wormhole';

export interface Tab {
  id: string;
  label: string;
  dot?: string;
}

export interface Brand {
  title: string;
  sub: string;
  initials?: string;
  href?: string;
}

export interface LayoutProps {
  brand: Brand;
  tabs: Tab[];
  active: string;
  onNav: (id: string) => void;
  github?: string;
  footer?: ReactNode;
  children?: ReactNode;
}

// Layout renders the sticky glass nav (brand, tab bar, theme + GitHub buttons,
// mobile menu) and a footer. `tabs` is [{ id, label, dot? }]. `brand` is
// { title, sub, initials, href }. `github` is the primary repo URL.
export function Layout({ brand, tabs, active, onNav, github, footer, children }: LayoutProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const wormhole = useRef<WormholeHandle>(null);
  // Navigating to a different page warps through the wormhole; the actual route
  // swap happens at the throat of the tunnel (mid-animation). Re-selecting the
  // current tab, or reduced-motion, routes immediately (handled in Wormhole).
  const nav = useCallback((id: string) => {
    setMenuOpen(false);
    if (id !== active && wormhole.current) wormhole.current.warp(() => onNav(id));
    else onNav(id);
  }, [onNav, active]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setMenuOpen(false); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  return (
    <>
      <header className="nav">
        <div className="nav-inner">
          <a className="brand" href={brand.href || '#home'} onClick={(e) => { e.preventDefault(); nav(tabs[0].id); }}>
            <span className="logo">{brand.initials || 'go'}</span>
            <span className="txt">{brand.title} <span className="sub">{brand.sub}</span></span>
          </a>
          <nav className={`tabs${menuOpen ? ' open' : ''}`}>
            {tabs.map((t) => (
              <a key={t.id} className={`tab${active === t.id ? ' active' : ''}`} href={`#${t.id}`}
                 onClick={(e) => { e.preventDefault(); nav(t.id); }}>
                {t.dot && <span className="dot" style={{ background: t.dot }} />}{t.label}
              </a>
            ))}
          </nav>
          <div className="nav-actions">
            <ThemeToggle />
            {github && (
              <a className="iconbtn star" href={github} target="_blank" rel="noopener">
                <i className="fa-brands fa-github" /><span className="label">GitHub</span>
              </a>
            )}
            <button className="iconbtn menu-btn" onClick={() => setMenuOpen((o) => !o)} aria-label="Menu">
              <i className={menuOpen ? 'fa-solid fa-xmark' : 'fa-solid fa-bars'} />
            </button>
          </div>
        </div>
      </header>
      <main className="wrap">{children}</main>
      {footer && <footer className="wrap" style={{ padding: '2rem', textAlign: 'center', color: 'var(--fg-dim)' }}>{footer}</footer>}
      <Wormhole ref={wormhole} />
    </>
  );
}
