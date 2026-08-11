// Site-wide social card: /opengraph-image (and, via the metadata layer, the
// twitter:image fallback too).
//
// Next's file-convention metadata image. Rendering happens through
// `ImageResponse` from 'next/og', which ships with Next 15 — no new dependency,
// and no network access: the layout is plain inline-styled elements and the
// bundled default font, so the exact same PNG is produced by the Vercel build
// and by the `output: 'export'` GitHub Pages build.
//
// Everything here runs at build time only. It deliberately reads nothing but
// src/data (the library list) so the card can never fail on a missing generated
// artifact — the per-library card in app/lib/[id]/opengraph-image.tsx is the one
// that carries measured numbers.
import { ImageResponse } from 'next/og';
import { LIBS } from '../src/data';

// force-static: under `output: 'export'` Next refuses a metadata image route
// that has not opted out of dynamic rendering. The card depends on nothing
// request-scoped, so pinning it static is what makes the GitHub Pages export
// emit opengraph-image.png instead of failing the build.
export const dynamic = 'force-static';

// Alt text lands in the og:image:alt tag Next emits alongside the image.
export const alt = 'malcolmston/go — the Node.js ecosystem, reimagined in Go';
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

// Palette lifted from ui/src/styles.css (the dark theme, which is the site's
// default) so a shared link looks like the page it points at.
const BG = '#06070a';
const FG = '#f5f6f8';
const MUTED = '#a6adba';
const GO = '#00add8';
const NODE = '#8cc84b';

export default function OpengraphImage() {
  // The card states the size of the ecosystem rather than hard-coding a number
  // that would drift every time a port is added.
  const libCount = LIBS.length;

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          padding: '84px 96px',
          background: BG,
          // Satori has no support for CSS variables or gradients on text, so
          // the accent wash is a plain radial background layer.
          backgroundImage:
            'radial-gradient(900px 520px at 88% -10%, rgba(47,155,255,0.22), rgba(6,7,10,0)), radial-gradient(700px 480px at 4% 108%, rgba(181,123,255,0.18), rgba(6,7,10,0))',
          color: FG,
          fontFamily: 'sans-serif',
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', alignItems: 'center', fontSize: 34, color: MUTED }}>
            <span style={{ color: NODE }}>Node.js</span>
            <span style={{ margin: '0 18px' }}>to</span>
            <span style={{ color: GO }}>Go</span>
          </div>
          <div
            style={{
              display: 'flex',
              marginTop: 28,
              fontSize: 96,
              fontWeight: 700,
              letterSpacing: '-0.03em',
            }}
          >
            malcolmston/go
          </div>
          <div style={{ display: 'flex', marginTop: 24, fontSize: 44, color: MUTED }}>
            The Node.js ecosystem, reimagined in Go.
          </div>
        </div>

        {/* Stacked rather than a two-column footer: satori has no text
            shrink-to-fit, so a side-by-side badge either wraps into the count
            line or overflows the canvas depending on the count's width. */}
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', alignItems: 'baseline' }}>
            <span style={{ fontSize: 72, fontWeight: 700, color: GO }}>{libCount}</span>
            <span style={{ fontSize: 34, color: MUTED, marginLeft: 18 }}>
              libraries, measured against the originals
            </span>
          </div>
          <div style={{ display: 'flex', marginTop: 18, fontSize: 28, color: MUTED }}>
            malcolmston.github.io/go
          </div>
        </div>
      </div>
    ),
    size,
  );
}
