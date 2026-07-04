import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { App } from '../../src/App';

describe('App', () => {
  beforeEach(() => {
    // Components mounted here (VersionBadge/ReleaseList) fetch on mount.
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
    window.location.hash = '';
  });

  afterEach(() => {
    window.location.hash = '';
  });

  it('renders the nav tabs and the home view by default', () => {
    render(<App />);
    expect(screen.getByRole('link', { name: 'Home' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Express' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'FAQ' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'About' })).toBeInTheDocument();
    // Home hero heading.
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/reimagined/i);
  });

  it('switches the visible view when location.hash changes', () => {
    render(<App />);
    // Move to the FAQ tab via a hash change.
    window.location.hash = '#faq';
    fireEvent(window, new Event('hashchange'));
    expect(screen.getByRole('heading', { name: /Frequently asked questions/ })).toBeInTheDocument();

    // And to a library view.
    window.location.hash = '#express';
    fireEvent(window, new Event('hashchange'));
    expect(screen.getByRole('heading', { level: 2, name: /Express/ })).toBeInTheDocument();
  });

  it('navigates when a nav tab is clicked', () => {
    render(<App />);
    fireEvent.click(screen.getByRole('link', { name: 'About' }));
    fireEvent(window, new Event('hashchange'));
    expect(screen.getByRole('heading', { level: 2, name: 'About' })).toBeInTheDocument();
  });
});
