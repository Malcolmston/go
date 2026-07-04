import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LibReleases } from 'go-ui';
import type { RelLib } from 'go-ui';

const lib: RelLib = {
  name: 'Express',
  icon: '<i class="fa-solid fa-route"></i>',
  accent: '#00add8',
  repo: 'express',
  url: 'https://github.com/malcolmston/express',
};

const okJson = (data: unknown) => ({ ok: true, status: 200, json: () => Promise.resolve(data) });

describe('LibReleases', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('renders the library name and a changelog link while loading', () => {
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
    render(<LibReleases lib={lib} />);
    expect(screen.getByRole('heading', { name: 'Express' })).toBeInTheDocument();
    expect(screen.getByText('Loading releases…')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Changelog/ })).toHaveAttribute(
      'href',
      'https://github.com/malcolmston/express/blob/main/CHANGELOG.md',
    );
  });

  it('renders fetched releases with tag, date and notes', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      okJson([
        {
          id: 1,
          tag_name: 'v1.4.0',
          published_at: '2025-01-15T00:00:00Z',
          html_url: 'https://github.com/malcolmston/express/releases/tag/v1.4.0',
          body: 'Fixed things. See https://example.com/notes',
        },
      ]),
    );
    render(<LibReleases lib={lib} />);
    expect(await screen.findByText('v1.4.0')).toBeInTheDocument();
    expect(screen.getByText('latest')).toBeInTheDocument();
    expect(screen.getByText(/Fixed things/)).toBeInTheDocument();
  });

  it('shows an error state when the fetch rejects', async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error('boom'));
    render(<LibReleases lib={lib} />);
    expect(await screen.findByText(/Couldn't load releases/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /releases page/ })).toBeInTheDocument();
  });

  it('shows an empty state when there are no releases', async () => {
    global.fetch = vi.fn().mockResolvedValue(okJson([]));
    render(<LibReleases lib={lib} />);
    expect(await screen.findByText('No releases yet.')).toBeInTheDocument();
  });
});
