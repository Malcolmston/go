// @vitest-environment node
//
// Unit tests for the Vercel Cron reindex Route Handler
// (app/api/cron/reindex/route.ts). This endpoint rebuilds the Upstash Search
// index — an expensive, ~300s job — so its authorization is the security-
// critical part and gets the most coverage here. node:fs and the es lib are
// mocked so nothing is read from disk and nothing is indexed for real.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const { mockReadFileSync, mockEsEnabled, mockEsIndexAll } = vi.hoisted(() => ({
  mockReadFileSync: vi.fn(),
  mockEsEnabled: vi.fn(),
  mockEsIndexAll: vi.fn(),
}));

vi.mock('node:fs', () => ({
  default: { readFileSync: mockReadFileSync },
  readFileSync: mockReadFileSync,
}));
vi.mock('../../../api/_lib/es', () => ({
  esEnabled: mockEsEnabled,
  esIndexAll: mockEsIndexAll,
}));

import { GET } from '../../../app/api/cron/reindex/route';

const SECRET = 'super-secret-cron-token';

function req(headers: Record<string, string> = {}): Request {
  return new Request('http://x/api/cron/reindex', { headers });
}
const authed = () => req({ authorization: `Bearer ${SECRET}` });

beforeEach(() => {
  vi.stubEnv('CRON_SECRET', SECRET);
  mockEsEnabled.mockReset().mockReturnValue(true);
  mockEsIndexAll.mockReset().mockResolvedValue(3);
  mockReadFileSync
    .mockReset()
    .mockReturnValue(JSON.stringify({ symbols: [{ id: 'a' }, { id: 'b' }, { id: 'c' }] }));
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.unstubAllEnvs();
});

describe('authorization', () => {
  it('refuses with 500 when CRON_SECRET is not configured (never runs open)', async () => {
    vi.stubEnv('CRON_SECRET', '');
    const res = await GET(authed());
    expect(res.status).toBe(500);
    expect((await res.json()).reason).toBe('CRON_SECRET not configured');
    expect(mockEsIndexAll).not.toHaveBeenCalled();
  });

  it('refuses with 500 when CRON_SECRET is whitespace only', async () => {
    vi.stubEnv('CRON_SECRET', '   ');
    expect((await GET(authed())).status).toBe(500);
    expect(mockEsIndexAll).not.toHaveBeenCalled();
  });

  it('rejects an unauthenticated request with 401', async () => {
    const res = await GET(req());
    expect(res.status).toBe(401);
    expect((await res.json()).reason).toBe('unauthorized');
    expect(mockEsIndexAll).not.toHaveBeenCalled();
  });

  it('rejects a wrong bearer token with 401', async () => {
    const res = await GET(req({ authorization: `Bearer ${'x'.repeat(SECRET.length)}` }));
    expect(res.status).toBe(401);
    expect(mockEsIndexAll).not.toHaveBeenCalled();
  });

  it('rejects a token of a different length with 401 (no timingSafeEqual throw)', async () => {
    const res = await GET(req({ authorization: 'Bearer short' }));
    expect(res.status).toBe(401);
  });

  it('rejects a non-Bearer scheme with 401', async () => {
    const res = await GET(req({ authorization: `Basic ${SECRET}` }));
    expect(res.status).toBe(401);
  });

  it('rejects the bare secret without the Bearer prefix with 401', async () => {
    const res = await GET(req({ authorization: SECRET }));
    expect(res.status).toBe(401);
  });

  it('never echoes the configured secret in any response body', async () => {
    for (const r of [req(), req({ authorization: 'Bearer nope' })]) {
      expect(await (await GET(r)).text()).not.toContain(SECRET);
    }
  });
});

describe('reindex', () => {
  it('indexes the bundled corpus and reports the count on success', async () => {
    const res = await GET(authed());
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.ok).toBe(true);
    expect(body.indexed).toBe(3);
    expect(typeof body.tookMs).toBe('number');
    expect(mockEsIndexAll).toHaveBeenCalledWith([{ id: 'a' }, { id: 'b' }, { id: 'c' }]);
  });

  it('is a benign 200 no-op when Upstash is not configured', async () => {
    mockEsEnabled.mockReturnValue(false);
    const res = await GET(authed());
    expect(res.status).toBe(200);
    expect(await res.json()).toMatchObject({ ok: false, reason: 'upstash not configured' });
    expect(mockEsIndexAll).not.toHaveBeenCalled();
  });

  it('returns 500 with a fixed reason when the corpus file is unreadable', async () => {
    mockReadFileSync.mockImplementation(() => {
      throw new Error('ENOENT: no such file /var/task/api/_data/symbols.json');
    });
    const res = await GET(authed());
    expect(res.status).toBe(500);
    const text = await res.text();
    expect(JSON.parse(text).reason).toBe('corpus unavailable');
    // The raw fs error, which discloses the deployment layout, is not echoed.
    expect(text).not.toContain('ENOENT');
    expect(text).not.toContain('/var/task');
  });

  it('returns 500 when the corpus file is not valid JSON', async () => {
    mockReadFileSync.mockReturnValue('{not json');
    expect((await GET(authed())).status).toBe(500);
    expect(mockEsIndexAll).not.toHaveBeenCalled();
  });

  it('treats a corpus without a symbols array as empty rather than crashing', async () => {
    mockReadFileSync.mockReturnValue(JSON.stringify({ symbols: 'nope' }));
    mockEsIndexAll.mockResolvedValue(0);
    const res = await GET(authed());
    expect(res.status).toBe(200);
    expect(mockEsIndexAll).toHaveBeenCalledWith([]);
  });

  it('returns 502 (not an unhandled rejection) when indexing fails part-way', async () => {
    mockEsIndexAll.mockRejectedValue(new Error('UPSTASH 429 token=abc123'));
    const res = await GET(authed());
    expect(res.status).toBe(502);
    const text = await res.text();
    expect(JSON.parse(text).reason).toBe('indexing failed');
    expect(text).not.toContain('abc123');
  });

  it('marks every response no-store', async () => {
    expect((await GET(authed())).headers.get('Cache-Control')).toBe('no-store');
    expect((await GET(req())).headers.get('Cache-Control')).toBe('no-store');
  });
});
