import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { LibReleases, VersionBadge } from 'go-ui';
import type { RelLib } from 'go-ui';

// These tests pin the reason /api/versions exists: the browser must not call
// api.github.com once per badge / release block. They live in their own file
// because VersionBadge memoises the shared request at module scope, and vitest
// gives each test file its own module registry.

const okJson = (data: unknown) => ({ ok: true, status: 200, json: () => Promise.resolve(data) });
const notFound = { ok: false, status: 404, json: () => Promise.resolve({}) };

const lib = (repo: string): RelLib => ({
  name: repo,
  icon: '<i class="fa-solid fa-route"></i>',
  accent: '#00add8',
  repo,
  url: `https://github.com/malcolmston/${repo}`,
});

const ghCalls = (fetchMock: ReturnType<typeof vi.fn>) =>
  fetchMock.mock.calls.filter((c) => String(c[0]).startsWith('https://api.github.com'));

describe('version data via the server route', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('renders many badges from a single /api/versions request, with no GitHub traffic', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      okJson({
        repos: {
          express: { tag: 'v1.2.3', prerelease: false, url: 'https://example.com/1' },
          lodash: { tag: 'v2.0.0-rc1', prerelease: true, url: 'https://example.com/2' },
        },
      }),
    );
    global.fetch = fetchMock;

    render(
      <>
        <VersionBadge repo="express" />
        <VersionBadge repo="express" />
        <VersionBadge repo="lodash" />
      </>,
    );

    expect(await screen.findByText('v2.0.0-rc1')).toBeInTheDocument();
    expect(screen.getAllByText('v1.2.3')).toHaveLength(2);
    // Three badges, one request — and none of them to GitHub.
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/versions');
    expect(ghCalls(fetchMock)).toHaveLength(0);
  });

  it('falls back to GitHub for a repo the route does not know', async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation((url: string) =>
        String(url).startsWith('/api/versions')
          ? Promise.resolve(okJson({ repos: {} }))
          : Promise.resolve(okJson({ tag_name: 'v9.9.9', prerelease: false })),
      );
    global.fetch = fetchMock;

    render(<VersionBadge repo="morgan" />);
    expect(await screen.findByText('v9.9.9')).toBeInTheDocument();
    expect(String(ghCalls(fetchMock)[0][0])).toBe(
      'https://api.github.com/repos/malcolmston/morgan/releases/latest',
    );
  });

  it('LibReleases reads the server route and never touches GitHub when it answers', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) =>
      String(url).startsWith('/api/versions')
        ? Promise.resolve(
            okJson({
              repo: 'express',
              tag: 'v1.4.0',
              releases: [
                {
                  id: 1,
                  tag_name: 'v1.4.0',
                  published_at: '2026-01-15T00:00:00Z',
                  html_url: 'https://github.com/malcolmston/express/releases/tag/v1.4.0',
                  body: 'Fixed things.',
                },
              ],
            }),
          )
        : Promise.reject(new Error('should not reach GitHub')),
    );
    global.fetch = fetchMock;

    render(<LibReleases lib={lib('express')} />);
    expect(await screen.findByText('v1.4.0')).toBeInTheDocument();
    expect(screen.getByText(/Fixed things/)).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/versions?repo=express');
    expect(ghCalls(fetchMock)).toHaveLength(0);
  });

  it('LibReleases trusts an empty server answer as "no releases yet"', async () => {
    const fetchMock = vi.fn().mockResolvedValue(okJson({ repo: 'gltf', tag: null, releases: [] }));
    global.fetch = fetchMock;
    render(<LibReleases lib={lib('gltf')} />);
    expect(await screen.findByText('No releases yet.')).toBeInTheDocument();
    expect(ghCalls(fetchMock)).toHaveLength(0);
  });

  it('LibReleases falls back to GitHub on a host without the route (404)', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) =>
      String(url).startsWith('/api/versions')
        ? Promise.resolve(notFound)
        : Promise.resolve(
            okJson([
              {
                id: 2,
                tag_name: 'v0.1.0',
                published_at: '2026-02-01T00:00:00Z',
                html_url: 'https://github.com/malcolmston/gltf/releases/tag/v0.1.0',
                body: '',
              },
            ]),
          ),
    );
    global.fetch = fetchMock;

    render(<LibReleases lib={lib('gltf')} />);
    expect(await screen.findByText('v0.1.0')).toBeInTheDocument();
    await waitFor(() => expect(ghCalls(fetchMock)).toHaveLength(1));
    expect(String(ghCalls(fetchMock)[0][0])).toBe(
      'https://api.github.com/repos/malcolmston/gltf/releases?per_page=8',
    );
  });
});
