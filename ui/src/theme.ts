// Shared theme key across every malcolmston.github.io site, so the light/dark
// choice follows the visitor from one library's page to the next.
export const THEME_KEY = 'mgo-theme';

export function applyStoredTheme(): void {
  try {
    const t = localStorage.getItem(THEME_KEY) || 'dark';
    document.documentElement.setAttribute('data-theme', t);
  } catch {
    document.documentElement.setAttribute('data-theme', 'dark');
  }
}
