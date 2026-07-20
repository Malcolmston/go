import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ReleaseList } from 'go-ui';
import type { RelLib } from 'go-ui';

const libs: RelLib[] = [
  { name: 'Express', icon: '<i></i>', accent: '#00add8', repo: 'express', url: 'https://github.com/malcolmston/express' },
  { name: 'morgan', icon: '<i></i>', accent: '#f778ba', repo: 'morgan', url: 'https://github.com/malcolmston/morgan' },
];

const okJson = (data: unknown) => ({ ok: true, status: 200, json: () => Promise.resolve(data) });

describe('ReleaseList', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('renders one release block per library', async () => {
    global.fetch = vi.fn().mockResolvedValue(okJson([]));
    render(<ReleaseList libs={libs} />);
    expect(screen.getByRole('heading', { name: 'Express' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'morgan' })).toBeInTheDocument();
    // Both libraries fetch their releases.
    expect(global.fetch).toHaveBeenCalledTimes(2);
    expect(await screen.findAllByText('No releases yet.')).toHaveLength(2);
  });

  it('renders tags fetched for each library', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      okJson([{ id: 1, tag_name: 'v0.1.0', published_at: '2025-02-01T00:00:00Z', html_url: '#', body: '' }]),
    );
    render(<ReleaseList libs={libs} />);
    expect(await screen.findAllByText('v0.1.0')).toHaveLength(2);
  });
});
