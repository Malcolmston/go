import { describe, expect, it } from 'vitest';

import config, {
  staticDataCacheControl,
  staticDataHeaders,
  staticDataSources,
} from '../../next.config.mjs';

describe('next config: cache headers for generated data bundles', () => {
  it('covers every browser-fetched file in public/', () => {
    expect(staticDataSources).toEqual([
      '/docs/:path*',
      '/search-index.json',
      '/graph.json',
      '/parity.json',
    ]);
  });

  it('lets browsers and the CDN reuse the bundles instead of max-age=0', () => {
    expect(staticDataCacheControl).toContain('public');
    expect(staticDataCacheControl).not.toContain('max-age=0');
    // Unhashed URLs: a deploy has to become visible, so never immutable.
    expect(staticDataCacheControl).not.toContain('immutable');
    const maxAge = /(?:^|[ ,])max-age=(\d+)/.exec(staticDataCacheControl)?.[1];
    const sMaxAge = /s-maxage=(\d+)/.exec(staticDataCacheControl)?.[1];
    expect(Number(maxAge)).toBeGreaterThan(0);
    expect(Number(sMaxAge)).toBeGreaterThan(Number(maxAge));
  });

  it('attaches the Cache-Control header to each source', () => {
    const entries = staticDataHeaders({});
    expect(entries.map((e) => e.source)).toEqual(staticDataSources);
    for (const entry of entries) {
      expect(entry.headers).toEqual([{ key: 'Cache-Control', value: staticDataCacheControl }]);
    }
  });

  // The GitHub Pages export has been removed, so headers() is no longer
  // conditional on the deploy target. It used to return [] for GITHUB_PAGES=true
  // because output:'export' has no server to attach headers to; that env var now
  // means nothing, and asserting so keeps a stale branch from creeping back.
  it('emits the same headers regardless of the environment', () => {
    expect(staticDataHeaders({ GITHUB_PAGES: 'true' })).toEqual(staticDataHeaders({}));
    expect(staticDataHeaders({}).length).toBe(staticDataSources.length);
  });

  it('wires headers() into the config and configures no static export', async () => {
    expect(typeof config.headers).toBe('function');
    await expect(config.headers?.()).resolves.toEqual(staticDataHeaders({}));
    // A static export would break the API routes and the parity/security pages.
    expect(config.output).toBeUndefined();
    expect(config.basePath).toBeUndefined();
  });
});
