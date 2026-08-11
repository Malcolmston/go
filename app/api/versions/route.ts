// app/api/versions/route.ts — release metadata for every ported repo, fetched
// once on the server instead of once per browser.
//
//   GET /api/versions               -> { generatedAt, repos: { <repo>: { tag, prerelease, url } } }
//   GET /api/versions?repo=express  -> { repo, tag, prerelease, releases: [ ... up to 8 ] }
//   GET /api/versions?repo=nope     -> 404 { error: "unknown repo" }
//   OPTIONS preflight               -> 204
//
// Why this exists: VersionBadge is rendered once per library card (38 on the
// landing page) and LibReleases once per library on /releases (39), and both
// used to call api.github.com straight from the browser with no credentials.
// GitHub's unauthenticated limit is 60 requests/hour per client IP, so a single
// page view burned the whole budget and every subsequent request came back 403
// with x-ratelimit-remaining: 0 — the version pills silently disappeared and
// /releases rendered 39 copies of its error state. The data is public and
// identical for every visitor, so it belongs on the server: this route turns
// "39 calls per visitor" into "39 calls per hour per deployment", and lifts the
// ceiling to 5000/hour whenever GITHUB_TOKEN is set.
//
// The clients (ui/src/components/VersionBadge.tsx, LibReleases.tsx) keep their
// direct-to-GitHub path as a fallback for whenever this route is absent — the
// per-library sites and the GitHub Pages static export have no functions.
//
// CORS is permissive (Access-Control-Allow-Origin: *) so those static exports
// can use it too.

import { LIBS } from '../../../src/data';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const OWNER = 'malcolmston';
const GH = 'https://api.github.com';
const PER_REPO = 8;
// One hour: releases move on a VERSION bump, minutes-old data is never wrong in
// a way a visitor can notice, and it keeps the upstream budget at 39/hour.
const TTL_SECONDS = 3600;
// Cap the fan-out so a cold request cannot open 40 sockets at once.
const CONCURRENCY = 6;
const TIMEOUT_MS = 8000;

// The repos this route will talk to: the ported libraries plus the aggregate
// meta repo (`malcolmston/go`), mirroring RELEASE_LIBS in
// src/components/Releases.tsx. It doubles as an allowlist — `?repo=` is only
// ever interpolated into a URL after a lookup here, so no caller-controlled
// path can reach api.github.com.
const REPOS: readonly string[] = Array.from(
  new Set(
    LIBS.map((l) => String(l.repo ?? '').split('/').filter(Boolean).pop() ?? '')
      .map((r) => r.replace(/\.git$/, ''))
      .filter((r) => r !== '')
      .concat(['go'])
  )
);

export interface ReleaseRecord {
  id: number;
  tag_name: string;
  published_at: string;
  html_url: string;
  body: string;
  prerelease: boolean;
}

export interface RepoVersion {
  tag: string;
  prerelease: boolean;
  url: string;
}

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
  'Access-Control-Max-Age': '86400',
};

const CACHE_HEADERS: Record<string, string> = {
  'Cache-Control': `public, max-age=0, s-maxage=${TTL_SECONDS}, stale-while-revalidate=86400`,
};

// A budget-exhausted or unreachable GitHub is not cacheable for an hour: the
// clients fall back to their own direct call, and the next visitor should get a
// fresh attempt reasonably soon.
const SOFT_CACHE_HEADERS: Record<string, string> = {
  'Cache-Control': 'public, max-age=0, s-maxage=60, stale-while-revalidate=600',
};

function ghHeaders(): Record<string, string> {
  const h: Record<string, string> = {
    Accept: 'application/vnd.github+json',
    'X-GitHub-Api-Version': '2022-11-28',
    'User-Agent': 'malcolmston-go-aggregator',
  };
  const token = process.env.GITHUB_TOKEN ?? process.env.GH_TOKEN ?? '';
  if (token !== '') h.Authorization = `Bearer ${token}`;
  return h;
}

// ghJson fetches one GitHub endpoint. It resolves to null on any failure
// (non-2xx, timeout, malformed JSON) — a missing repo must never fail the whole
// response, because the caller degrades per repo.
async function ghJson(path: string): Promise<unknown | null> {
  try {
    const res = await fetch(`${GH}${path}`, {
      headers: ghHeaders(),
      signal: AbortSignal.timeout(TIMEOUT_MS),
      // Let the framework's data cache hold the upstream answer, so warm
      // invocations in the same deployment do not re-ask GitHub either.
      next: { revalidate: TTL_SECONDS },
    });
    if (!res.ok) {
      if (res.status === 403 || res.status === 429) {
        console.warn(
          `[versions] GitHub rate limited ${path}`,
          `remaining=${res.headers.get('x-ratelimit-remaining') ?? '?'}`
        );
      }
      return null;
    }
    return (await res.json()) as unknown;
  } catch (err) {
    console.warn(`[versions] GitHub request failed ${path}`, err);
    return null;
  }
}

const str = (v: unknown): string => (typeof v === 'string' ? v : '');
const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v);

function toRelease(raw: unknown): ReleaseRecord | null {
  if (!isObj(raw)) return null;
  const tag = str(raw.tag_name);
  if (tag === '') return null;
  return {
    id: typeof raw.id === 'number' ? raw.id : 0,
    tag_name: tag,
    published_at: str(raw.published_at) || str(raw.created_at),
    html_url: str(raw.html_url),
    body: str(raw.body),
    prerelease: raw.prerelease === true,
  };
}

// releasesFor returns the newest releases for one repo, falling back to the
// newest tag (as a synthetic single-entry list) for a repo that has tags but no
// GitHub Releases — the same two-step VersionBadge used to do client-side.
async function releasesFor(repo: string): Promise<ReleaseRecord[]> {
  const list = await ghJson(`/repos/${OWNER}/${repo}/releases?per_page=${PER_REPO}`);
  if (Array.isArray(list)) {
    const rels = list.map(toRelease).filter((r): r is ReleaseRecord => r !== null);
    if (rels.length > 0) return rels;
  }
  const tags = await ghJson(`/repos/${OWNER}/${repo}/tags?per_page=1`);
  if (Array.isArray(tags) && tags.length > 0 && isObj(tags[0])) {
    const name = str(tags[0].name);
    if (name !== '') {
      return [{
        id: 0,
        tag_name: name,
        published_at: '',
        html_url: `https://github.com/${OWNER}/${repo}/releases/tag/${encodeURIComponent(name)}`,
        body: '',
        prerelease: false,
      }];
    }
  }
  return [];
}

// mapLimit runs `fn` over `items` with at most `limit` in flight.
async function mapLimit<T, R>(items: readonly T[], limit: number, fn: (item: T) => Promise<R>): Promise<R[]> {
  const out: R[] = new Array(items.length);
  let next = 0;
  const workers = new Array(Math.min(limit, items.length)).fill(0).map(async () => {
    for (let i = next++; i < items.length; i = next++) out[i] = await fn(items[i]);
  });
  await Promise.all(workers);
  return out;
}

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function GET(req: Request): Promise<Response> {
  let repo = '';
  try {
    repo = (new URL(req.url).searchParams.get('repo') ?? '').trim();
  } catch {
    repo = '';
  }

  if (repo !== '') {
    // Case-insensitive lookup against the allowlist; anything else is a 404
    // rather than a proxied request.
    const known = REPOS.find((r) => r.toLowerCase() === repo.toLowerCase());
    if (known === undefined) {
      return Response.json(
        { error: 'unknown repo' },
        { status: 404, headers: { ...CORS_HEADERS, 'Cache-Control': 'no-store' } }
      );
    }
    const releases = await releasesFor(known);
    const latest = releases[0];
    return Response.json(
      {
        generatedAt: new Date().toISOString(),
        repo: known,
        tag: latest ? latest.tag_name : null,
        prerelease: latest ? latest.prerelease : false,
        releases,
      },
      { headers: { ...CORS_HEADERS, ...(releases.length > 0 ? CACHE_HEADERS : SOFT_CACHE_HEADERS) } }
    );
  }

  // The summary: just enough for the version pills, so the landing page pulls
  // a couple of KB rather than every release note.
  const pairs = await mapLimit(REPOS, CONCURRENCY, async (r) => {
    const rels = await releasesFor(r);
    const latest = rels[0];
    if (!latest) return null;
    const entry: RepoVersion = {
      tag: latest.tag_name,
      prerelease: latest.prerelease,
      url: latest.html_url || `https://github.com/${OWNER}/${r}/releases`,
    };
    return [r, entry] as const;
  });

  const repos: Record<string, RepoVersion> = {};
  for (const p of pairs) if (p) repos[p[0]] = p[1];

  return Response.json(
    { generatedAt: new Date().toISOString(), repos },
    {
      headers: {
        ...CORS_HEADERS,
        ...(Object.keys(repos).length > 0 ? CACHE_HEADERS : SOFT_CACHE_HEADERS),
      },
    }
  );
}
