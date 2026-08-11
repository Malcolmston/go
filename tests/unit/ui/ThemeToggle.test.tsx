import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ThemeToggle } from 'go-ui';

describe('ThemeToggle', () => {
  beforeEach(() => {
    document.documentElement.setAttribute('data-theme', 'dark');
    localStorage.clear();
  });

  it('renders a toggle button', () => {
    render(<ThemeToggle />);
    expect(screen.getByRole('button', { name: /Toggle colour theme/ })).toBeInTheDocument();
  });

  it('toggles data-theme on the document element and persists to localStorage', async () => {
    render(<ThemeToggle />);
    const btn = screen.getByRole('button', { name: /Toggle colour theme/ });

    await userEvent.click(btn);
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(localStorage.getItem('mgo-theme')).toBe('light');

    await userEvent.click(btn);
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(localStorage.getItem('mgo-theme')).toBe('dark');
  });

  it('adopts the theme the pre-hydration script already put on <html>', () => {
    document.documentElement.setAttribute('data-theme', 'light');
    render(<ThemeToggle />);
    expect(document.querySelector('.fa-sun')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /currently light/ })).toBeInTheDocument();
  });

  it('exposes the current state and the next action to assistive tech', () => {
    render(<ThemeToggle />);
    const btn = screen.getByRole('button', { name: /Toggle colour theme/ });
    expect(btn).toHaveAttribute('type', 'button');
    expect(btn.getAttribute('aria-label')).toMatch(/currently dark/);
    expect(btn).toHaveAttribute('title', 'Switch to light theme');
    // The icon is decorative — the label carries the meaning.
    expect(btn.querySelector('i')).toHaveAttribute('aria-hidden', 'true');
  });

  it('follows a theme change made in another tab', async () => {
    render(<ThemeToggle />);
    localStorage.setItem('mgo-theme', 'light');
    await act(async () => {
      window.dispatchEvent(new StorageEvent('storage', { key: 'mgo-theme', newValue: 'light' }));
    });
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(screen.getByRole('button', { name: /currently light/ })).toBeInTheDocument();
  });

  it('ignores storage events for unrelated keys', async () => {
    render(<ThemeToggle />);
    localStorage.setItem('something-else', 'light');
    await act(async () => {
      window.dispatchEvent(new StorageEvent('storage', { key: 'something-else', newValue: 'x' }));
    });
    expect(screen.getByRole('button', { name: /currently dark/ })).toBeInTheDocument();
  });
});
