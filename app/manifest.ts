// /manifest.webmanifest — the web app manifest, linked from every page's <head>
// by Next's `manifest` file convention (so app/layout.tsx stays untouched).
//
// Without it, an installed or pinned shortcut on Android/Chrome falls back to
// the document title and a screenshot. The name, short name and description all
// come from app/seo.ts, the same constants the <title> and og:description are
// built from, so the installed shortcut can never disagree with the pages.
//
// `force-static` for the same reason as the metadata images: the manifest
// depends on nothing request-scoped, and pinning it static is what lets the
// `output: 'export'` GitHub Pages build write out/manifest.webmanifest.
import type { MetadataRoute } from 'next';
import { withBase } from '../src/basePath';
import { SITE_DESCRIPTION, SITE_NAME } from './seo';

export const dynamic = 'force-static';

// The dark theme from ui/src/styles.css, which is the site's default and what
// the icons and the social card are drawn on. theme-color tints the Android
// task switcher and the iOS/Chrome address bar; background_color is the splash
// screen shown before the first paint, so the two match to avoid a flash.
const BACKGROUND = '#06070a';
const THEME = '#00add8';

/**
 * The manifest as a plain value, exported for the unit test.
 *
 * Every URL is run through withBase() rather than hard-coded at the root: the
 * GitHub Pages export serves the app under /go, and a start_url or icon src
 * that names the domain root there would install a shortcut to a 404. Next
 * prefixes the basePath onto the <link rel="manifest"> href itself, but not
 * onto the JSON this function returns.
 */
export function manifestValue(): MetadataRoute.Manifest {
  return {
    name: SITE_NAME,
    short_name: SITE_NAME,
    description: SITE_DESCRIPTION,
    start_url: withBase('/'),
    scope: withBase('/'),
    display: 'standalone',
    background_color: BACKGROUND,
    theme_color: THEME,
    icons: [
      // The two ImageResponse routes next door (app/icon.tsx,
      // app/apple-icon.tsx). 'any' covers the tab/launcher slot; 'maskable'
      // declares that the 180px art tolerates Android's adaptive-icon crop,
      // which it does — the mark sits well inside the safe zone.
      { src: withBase('/icon'), sizes: '32x32', type: 'image/png', purpose: 'any' },
      { src: withBase('/apple-icon'), sizes: '180x180', type: 'image/png', purpose: 'any' },
      { src: withBase('/apple-icon'), sizes: '180x180', type: 'image/png', purpose: 'maskable' },
    ],
  };
}

export default function manifest(): MetadataRoute.Manifest {
  return manifestValue();
}
