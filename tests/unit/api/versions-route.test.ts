// @vitest-environment node
//
// Unit tests for the release-metadata Route Handler (app/api/versions/route.ts).
// This route exists so the browser stops calling api.github.com once per version
// badge / release block: it fetches the data server-side, once, behind a CDN
// cache, and can carry a GITHUB_TOKEN.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import { GET, OPTIONS } from '../../../app/api/versions/route';
import { LIBS } from '../../../src/data';

const release = (over: Record<string, unknown> = {}) => ({
  id: 7,
  tag_name: 'v1.2.3',
  published_at: '2026-01-01T00:00:00Z',
  html_url: 'https://github.com/malcolmston/express/releases/tag/v1.2.3',
  body: 'notes',
  prerelease: false,
  ...over,
});

const ok = (data: unknown) => ({
  ok: true,
  status: 200,
  headers: new Headers(),
  json: () => Promise.resolve(data),
});
const fail = (status: number, headers: Record<string, string> = {}) => ({
  ok: false,
  status,
  headers: new Headers(headers),
  json: () => Promise.resolve({ message: 'nope' }),
});

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  delete process.env.GITHUB_TOKEN;
  delete process.env.GH_TOKEN;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const get = (url: string) => GET(new Request(url));

describe('OPTIONS /api/versions', () => {
  it('returns 204 with CORS headers', async () => {
    const res = await OPTIONS();
    expect(res.status).toBe(204);
    expect(res.headers.get('Access-Control-Allow-Origin')).toBe('*');
  });
});

describe('GET /api/versions?repo=', () => {
  it('returns the release list for a known repo, cached at the edge', async () => {
    fetchMock.mockResolvedValue(ok([release(), release({ id: 8, tag_name: 'v1.2.2' })]));
    const res = await get('http://localhost/api/versions?repo=express');
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.repo).toBe('express');
    expect(body.tag).toBe('v1.2.3');
    expect(body.prerelease).toBe(false);
    expect(body.releases.map((r: { tag_name: string }) => r.tag_name)).toEqual(['v1.2.3', 'v1.2.2']);
    expect(res.headers.get('Cache-Control')).toContain('s-maxage=3600');
    // Exactly one upstream call for one repo.
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toBe(
      'https://api.github.com/repos/malcolmston/express/releases?per_page=8',
    );
  });

  it('404s an unknown repo without touching GitHub', async () => {
    const res = await get('http://localhost/api/versions?repo=../../etc/passwd');
    expect(res.status).toBe(404);
    expect(await res.json()).toEqual({ error: 'unknown repo' });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('falls back to the newest tag when a repo has no releases', async () => {
    fetchMock
      .mockResolvedValueOnce(ok([]))
      .mockResolvedValueOnce(ok([{ name: 'v0.9.0' }]));
    const body = await (await get('http://localhost/api/versions?repo=express')).json();
    expect(body.tag).toBe('v0.9.0');
    expect(body.releases).toHaveLength(1);
    expect(String(fetchMock.mock.calls[1][0])).toContain('/tags?per_page=1');
  });

  it('degrades to an empty list with a short cache when GitHub rate limits us', async () => {
    fetchMock.mockResolvedValue(fail(403, { 'x-ratelimit-remaining': '0' }));
    const res = await get('http://localhost/api/versions?repo=express');
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.tag).toBeNull();
    expect(body.releases).toEqual([]);
    expect(res.headers.get('Cache-Control')).toContain('s-maxage=60');
  });

  it('sends an Authorization header only when a token is configured', async () => {
    fetchMock.mockResolvedValue(ok([release()]));
    await get('http://localhost/api/versions?repo=express');
    const anon = fetchMock.mock.calls[0][1] as { headers: Record<string, string> };
    expect(anon.headers.Authorization).toBeUndefined();

    fetchMock.mockClear();
    process.env.GITHUB_TOKEN = 'secret-token';
    await get('http://localhost/api/versions?repo=express');
    const authed = fetchMock.mock.calls[0][1] as { headers: Record<string, string> };
    expect(authed.headers.Authorization).toBe('Bearer secret-token');
  });
});

describe('GET /api/versions', () => {
  it('returns one small entry per repo — every library plus the meta repo', async () => {
    fetchMock.mockResolvedValue(ok([release()]));
    const res = await get('http://localhost/api/versions');
    expect(res.status).toBe(200);
    const body = await res.json();
    const repos = Object.keys(body.repos);
    expect(repos).toHaveLength(LIBS.length + 1);
    expect(repos).toContain('express');
    expect(repos).toContain('go');
    // Tags only: the summary must not carry release notes (the landing page
    // renders 38 badges and needs kilobytes, not the whole changelog).
    expect(body.repos.express).toEqual({
      tag: 'v1.2.3',
      prerelease: false,
      url: 'https://github.com/malcolmston/express/releases/tag/v1.2.3',
    });
    // One upstream call per repo per revalidation — not per visitor.
    expect(fetchMock).toHaveBeenCalledTimes(LIBS.length + 1);
  });

  it('omits repos GitHub could not answer for instead of failing the response', async () => {
    fetchMock.mockImplementation((url: string) =>
      Promise.resolve(String(url).includes('/express/') ? ok([release()]) : fail(403)),
    );
    const body = await (await get('http://localhost/api/versions')).json();
    expect(Object.keys(body.repos)).toEqual(['express']);
  });
});
