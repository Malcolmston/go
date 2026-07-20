import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { VersionBadge } from 'go-ui';

const okJson = (data: unknown) => ({ ok: true, status: 200, json: () => Promise.resolve(data) });

describe('VersionBadge', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the latest release tag as a stable pill', async () => {
    global.fetch = vi.fn().mockResolvedValue(okJson({ tag_name: 'v1.2.3', prerelease: false }));
    render(<VersionBadge repo="express" />);
    expect(await screen.findByText('v1.2.3')).toBeInTheDocument();
    expect(screen.getByText(/stable/)).toBeInTheDocument();
  });

  it('marks a pre-release as pre', async () => {
    global.fetch = vi.fn().mockResolvedValue(okJson({ tag_name: 'v2.0.0-rc1', prerelease: true }));
    render(<VersionBadge repo="passport" href="https://example.com/releases" />);
    const badge = await screen.findByText('v2.0.0-rc1');
    expect(badge).toBeInTheDocument();
    expect(screen.getByText(/pre/)).toBeInTheDocument();
    expect(badge.closest('a')).toHaveAttribute('href', 'https://example.com/releases');
  });

  it('falls back to the newest tag when no release exists', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 404, json: () => Promise.resolve({}) })
      .mockResolvedValueOnce(okJson([{ name: 'v0.9.0' }]));
    render(<VersionBadge repo="chalk" />);
    expect(await screen.findByText('v0.9.0')).toBeInTheDocument();
  });

  it('renders nothing (graceful) when both fetches reject', async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error('network down'));
    const { container } = render(<VersionBadge repo="morgan" />);
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
    // Give the fallback chain a microtask to settle, then assert empty output.
    await Promise.resolve();
    expect(container.textContent).toBe('');
  });
});
