import { useState, useEffect, useCallback } from 'react';

// useHashTab wires hash-based routing to a tab id. Returns [active, setActive]
// where setActive also updates the URL hash.
export function useHashTab(ids: string[], fallback: string): [string, (id: string) => void] {
  const read = (): string => {
    const h = (typeof location !== 'undefined' ? location.hash : '').replace(/^#/, '');
    return ids.includes(h) ? h : fallback;
  };
  const [active, setActive] = useState<string>(read);
  useEffect(() => {
    const onHash = () => {
      const h = (typeof location !== 'undefined' ? location.hash : '').replace(/^#/, '');
      // Only switch tabs for a recognised tab id. Any other hash is an in-page
      // anchor (e.g. "express-install") or a nested route owned by another
      // component (e.g. the docs "pkg/<path>" routes) — leave the active tab
      // unchanged so the browser can scroll to the anchor without the whole
      // view jumping back to the fallback tab.
      if (ids.includes(h)) setActive(h);
    };
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const go = useCallback((id: string) => {
    if (location.hash.replace(/^#/, '') !== id) location.hash = id;
    else setActive(id);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }, []);
  return [active, go];
}
