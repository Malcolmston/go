'use client';

import { useState, useEffect } from 'react';
import { THEME_KEY } from '../theme';

// ThemeToggle flips the shared light/dark choice and persists it so the
// preference follows the visitor across every malcolmston.github.io site.
export function ThemeToggle() {
  // Render 'dark' on the server (and for the first client paint) so SSR/SSG
  // markup is deterministic and hydration matches; the effect below then syncs
  // to whatever the pre-hydration inline script actually set on <html>.
  const [theme, setTheme] = useState<string>('dark');
  useEffect(() => {
    setTheme(document.documentElement.getAttribute('data-theme') || 'dark');
  }, []);
  const toggle = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    document.documentElement.setAttribute('data-theme', next);
    try { localStorage.setItem(THEME_KEY, next); } catch { /* ignore */ }
  };
  return (
    <button className="iconbtn" onClick={toggle} aria-label="Toggle colour theme" title="Toggle theme">
      <i className={theme === 'dark' ? 'fa-solid fa-moon' : 'fa-solid fa-sun'} />
    </button>
  );
}
