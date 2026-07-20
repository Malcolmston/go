import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
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
});
