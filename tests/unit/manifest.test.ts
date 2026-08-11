import { describe, it, expect } from 'vitest';
import manifest, { manifestValue } from '../../app/manifest';
import { SITE_DESCRIPTION, SITE_NAME } from '../../app/seo';

// app/manifest.ts is a file-convention route, so nothing else in the app
// imports it — this is the only guard that the installed-shortcut identity
// stays derived from app/seo.ts and that every URL in it is base-path aware
// (the GitHub Pages export serves the site under /go).
describe('manifest', () => {
  it('takes its identity from the shared SEO constants', () => {
    const m = manifest();
    expect(m.name).toBe(SITE_NAME);
    expect(m.short_name).toBe(SITE_NAME);
    expect(m.description).toBe(SITE_DESCRIPTION);
  });

  it('is installable: standalone display, start_url, colours', () => {
    const m = manifestValue();
    expect(m.display).toBe('standalone');
    expect(m.start_url).toBe('/');
    expect(m.scope).toBe('/');
    expect(m.background_color).toMatch(/^#[0-9a-f]{6}$/i);
    expect(m.theme_color).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it('points at the two ImageResponse icon routes, including a maskable one', () => {
    const icons = manifestValue().icons ?? [];
    expect(icons.length).toBeGreaterThanOrEqual(2);
    for (const icon of icons) {
      expect(icon.type).toBe('image/png');
      // Root-relative, never absolute: the manifest is served from the same
      // origin as the pages under both deployments.
      expect(icon.src.startsWith('/')).toBe(true);
    }
    expect(icons.map((i) => i.src)).toContain('/icon');
    expect(icons.map((i) => i.src)).toContain('/apple-icon');
    expect(icons.some((i) => i.purpose === 'maskable')).toBe(true);
  });
});
