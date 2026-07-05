import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DocsApp } from 'go-ui';
import type { DocIndex } from 'go-ui';
import sampleJson from '../../../../../ui/src/docs/sample.doc.json';

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
});
