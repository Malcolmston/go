import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  THEME_KEY,
  THEME_BOOTSTRAP,
  applyStoredTheme,
  getStoredTheme,
  resolveTheme,
  setStoredTheme,
  systemTheme,
} from 'go-ui';

// jsdom has no matchMedia by default; install a controllable stub.
function stubPrefersLight(light: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((query: string) => ({
      matches: query.includes('light') ? light : !light,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

beforeEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute('data-theme');
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('theme', () => {
  it('getStoredTheme returns null when nothing valid is stored', () => {
    expect(getStoredTheme()).toBeNull();
    localStorage.setItem(THEME_KEY, 'chartreuse');
    expect(getStoredTheme()).toBeNull();
  });

  it('getStoredTheme returns a persisted choice', () => {
    localStorage.setItem(THEME_KEY, 'light');
    expect(getStoredTheme()).toBe('light');
  });

  it('systemTheme defaults to dark when matchMedia is unavailable', () => {
    vi.stubGlobal('matchMedia', undefined);
    expect(systemTheme()).toBe('dark');
  });

  it('systemTheme honours prefers-color-scheme', () => {
    stubPrefersLight(true);
    expect(systemTheme()).toBe('light');
    stubPrefersLight(false);
    expect(systemTheme()).toBe('dark');
  });

  it('resolveTheme prefers an explicit choice over the system preference', () => {
    stubPrefersLight(true);
    expect(resolveTheme()).toBe('light');
    localStorage.setItem(THEME_KEY, 'dark');
    expect(resolveTheme()).toBe('dark');
  });

  it('applyStoredTheme writes the resolved theme onto <html>', () => {
    stubPrefersLight(true);
    applyStoredTheme();
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');

    localStorage.setItem(THEME_KEY, 'dark');
    applyStoredTheme();
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });

  it('setStoredTheme applies and persists', () => {
    setStoredTheme('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(localStorage.getItem(THEME_KEY)).toBe('light');
  });

  it('THEME_BOOTSTRAP resolves the same theme as resolveTheme()', () => {
    stubPrefersLight(true);
    // eslint-disable-next-line no-new-func
    new Function(THEME_BOOTSTRAP)();
    expect(document.documentElement.getAttribute('data-theme')).toBe(resolveTheme());

    localStorage.setItem(THEME_KEY, 'dark');
    // eslint-disable-next-line no-new-func
    new Function(THEME_BOOTSTRAP)();
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });
});
