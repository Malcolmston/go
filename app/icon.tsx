// The browser-tab icon: /icon, linked from every page's <head> by Next's
// `icon` file convention (no edit to app/layout.tsx is needed or wanted).
//
// Same technique as app/opengraph-image.tsx: `ImageResponse` from 'next/og',
// which ships with Next 15, so this adds no dependency and needs no network
// access. `force-static` is what lets the `output: 'export'` GitHub Pages build
// emit the PNG as a file instead of refusing a dynamic metadata route.
import { ImageResponse } from 'next/og';

export const dynamic = 'force-static';

// 32x32 is the size browsers actually rasterise into the tab strip; anything
// larger is downscaled by the browser and loses the mark's edges.
export const size = { width: 32, height: 32 };
export const contentType = 'image/png';

// Palette shared with app/opengraph-image.tsx (the dark theme from
// ui/src/styles.css, which is the site's default), so the tab icon, the pinned
// shortcut and the social card all read as one mark.
const BG = '#06070a';
const GO = '#00add8';

export default function Icon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: BG,
          // Rounded rather than a full circle: at 32px a circle eats the
          // glyph's side bearings, and the tab strip already clips corners.
          borderRadius: 7,
          color: GO,
          fontFamily: 'sans-serif',
          fontSize: 22,
          fontWeight: 700,
          // Optical centring — satori places the baseline slightly low for a
          // single glyph in a flex box this small.
          lineHeight: 1,
        }}
      >
        g
      </div>
    ),
    size,
  );
}
