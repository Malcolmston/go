import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DocsApp } from 'go-ui';
import type { DocIndex } from 'go-ui';
import sampleJson from '../../../../ui/src/docs/sample.doc.json';

const sample = sampleJson as unknown as DocIndex;

afterEach(() => {
  window.location.hash = '';
});

describe('DocsApp', () => {
  it('renders the sidebar and the first package by default', () => {
    render(<DocsApp index={sample} />);
    expect(screen.getByRole('searchbox')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: /package express/ })).toBeInTheDocument();
  });

  it('selects a package from the hash route (#pkg/<importPath>)', async () => {
    window.location.hash = '#pkg/github.com/malcolmston/express/middleware';
    render(<DocsApp index={sample} />);
    await act(async () => {
      window.dispatchEvent(new Event('hashchange'));
    });
    expect(screen.getByRole('heading', { level: 1, name: /package middleware/ })).toBeInTheDocument();
  });

  it('updates the hash when a package is chosen in the nav', async () => {
    render(<DocsApp index={sample} />);
    await userEvent.click(screen.getByText('middleware'));
    expect(window.location.hash).toBe('#pkg/github.com/malcolmston/express/middleware');
    expect(screen.getByRole('heading', { level: 1, name: /package middleware/ })).toBeInTheDocument();
  });

  it('fetches doc.json from a url when no index is given', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => sample });
    vi.stubGlobal('fetch', fetchMock);
    render(<DocsApp url="/doc.json" />);
    expect(await screen.findByRole('heading', { level: 1, name: /package express/ })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith('/doc.json');
  });

  it('shows an error message when the fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404 }));
    render(<DocsApp url="/missing.json" />);
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/Failed to load/));
  });

  it('renders an empty state — not a spinner — when given neither index nor url', () => {
    render(<DocsApp />);
    expect(screen.getByText('No documentation available.')).toBeInTheDocument();
  });

  it('renders an empty state for a doc index with no packages', () => {
    render(<DocsApp index={{ module: 'github.com/x/y', packages: [] }} />);
    expect(screen.getAllByText('No packages.').length).toBeGreaterThan(0);
    expect(screen.getByText('github.com/x/y')).toBeInTheDocument();
  });

  it('does not crash on a malformed doc index (missing/!array packages)', () => {
    const bad = { module: 'github.com/x/y' } as unknown as DocIndex;
    expect(() => render(<DocsApp index={bad} />)).not.toThrow();
    expect(screen.getAllByText('No packages.').length).toBeGreaterThan(0);
  });

  it('errors rather than hanging when the fetched JSON is not an object', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => null }));
    render(<DocsApp url="/doc.json" />);
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/malformed/));
  });

  it('renders the sidebar "Packages" title exactly once', () => {
    render(<DocsApp index={sample} />);
    expect(screen.getAllByText('Packages')).toHaveLength(1);
  });

  it('toggles the Index panel from the header nav', async () => {
    render(<DocsApp index={sample} />);
    const indexLink = screen.getByRole('link', { name: 'Index' });
    expect(indexLink).toHaveAttribute('aria-expanded', 'false');
    await userEvent.click(indexLink);
    expect(indexLink).toHaveAttribute('aria-expanded', 'true');
    expect(document.getElementById('gd-index-panel')).toBeTruthy();
  });

  it('toggles the Help panel and closes the Index panel', async () => {
    render(<DocsApp index={sample} />);
    await userEvent.click(screen.getByRole('link', { name: 'Index' }));
    await userEvent.click(screen.getByRole('link', { name: 'Help' }));
    expect(document.getElementById('gd-index-panel')).toBeNull();
    expect(document.getElementById('gd-help-panel')).toBeTruthy();
  });

  it('exposes a labelled breadcrumb showing module › package', () => {
    render(<DocsApp index={sample} />);
    const crumb = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(crumb).toHaveTextContent(sample.module);
    expect(crumb).toHaveTextContent('express');
  });
});
