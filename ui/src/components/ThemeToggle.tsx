'use client';

import { useState } from 'react';
import { THEME_KEY } from '../theme';

// ThemeToggle flips the shared light/dark choice and persists it so the
// preference follows the visitor across every malcolmston.github.io site.
export function ThemeToggle() {
  const [theme, setTheme] = useState<string>(() =>
    document.documentElement.getAttribute('data-theme') || 'dark');
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
