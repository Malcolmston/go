// Per-library social card: /lib/<id>/opengraph-image.
//
// The highest-leverage sharing asset the site has — a shared /lib/express link
// previews as "Express for Go, 98.9% parity vs express@4.21.2" instead of the
// generic site card. Rendered with `ImageResponse` from 'next/og' (bundled with
// Next 15, so no new dependency) using only inline styles and the bundled
// default font: no network fetch, which is what lets the same file work in the
// Vercel build and in the static `output: 'export'` GitHub Pages build.
//
// Numbers come from the MEASURED corpus (api/_data/parity.json) through the same
// server-side loader the /parity pages use — never from src/parity.ts, the legacy
// hand-written map. A library with no harness simply gets the card without a
// score rather than an invented one.
import { ImageResponse } from 'next/og';
import { LIBS } from '../../../src/data';
import { pct } from '../../../src/components/ParityData';
import { libMeta, parityHarness } from '../../parity/load';

// force-static: `output: 'export'` (GitHub Pages) rejects a metadata image
// route that could render per request. Everything here is read at build time,
// so the whole set of per-library PNGs is emitted into out/.
export const dynamic = 'force-static';

export const alt = 'A malcolmston/go library card: the Go port, its tagline and its measured parity';
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

// Mirrors the route's own generateStaticParams so the static export emits one
// PNG per library page. Without this, `output: 'export'` has no id list for the
// dynamic segment and the build fails.
export function generateStaticParams() {
  return LIBS.map((lib) => ({ id: lib.id }));
}

// Same tolerance as app/lib/[id]/page.tsx: decodeURIComponent throws on a
// malformed escape, and a bad URL must still produce an image.
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

// Dark-theme palette from ui/src/styles.css, matching the page being shared.
const BG = '#06070a';
const FG = '#f5f6f8';
const MUTED = '#a6adba';
const GO = '#00add8';

export default async function LibOpengraphImage({
  params,
}: {
  // Next 15 passes params as a promise to metadata image routes; awaiting a
  // plain object is harmless, so this stays correct either way.
  params: Promise<{ id: string }> | { id: string };
}) {
  const { id } = await params;
  const slug = safeDecode(id);
  const lib = LIBS.find((l) => l.id === slug);
  const found = parityHarness(slug);
  const harness = found?.harness ?? null;
  // parityPercent is only meaningful when cases actually compared; pct() renders
  // the em dash otherwise, which is not worth putting on a card.
  const percent =
    harness && harness.comparedTotal > 0 ? pct(harness.parityPercent, harness.comparedTotal) : null;
  const upstream = harness?.upstream.raw || lib?.node || '';
  const accent = lib ? libMeta(lib.id).accent : GO;

  const title = lib ? `${lib.name} for Go` : 'malcolmston/go';
  const tagline = lib ? lib.tagline : 'The Node.js ecosystem, reimagined in Go.';
  const pkg = lib ? lib.pkg : 'github.com/malcolmston';

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          padding: '76px 96px',
          background: BG,
          backgroundImage: `radial-gradient(900px 520px at 92% -12%, ${accent}33, rgba(6,7,10,0)), radial-gradient(700px 460px at 0% 110%, rgba(91,140,255,0.16), rgba(6,7,10,0))`,
          color: FG,
          fontFamily: 'sans-serif',
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', fontSize: 30, color: MUTED, letterSpacing: '0.04em' }}>
            malcolmston/go
          </div>
          <div
            style={{
              display: 'flex',
              marginTop: 26,
              fontSize: 90,
              fontWeight: 700,
              letterSpacing: '-0.03em',
              color: accent,
            }}
          >
            {title}
          </div>
          <div style={{ display: 'flex', marginTop: 20, fontSize: 40, color: MUTED }}>
            {tagline}
          </div>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column' }}>
          {percent ? (
            <div style={{ display: 'flex', alignItems: 'baseline' }}>
              <span style={{ fontSize: 84, fontWeight: 700 }}>{percent}</span>
              <span style={{ fontSize: 34, color: MUTED, marginLeft: 20 }}>
                {upstream ? `parity vs ${upstream}` : 'measured parity'}
              </span>
            </div>
          ) : (
            <div style={{ display: 'flex', fontSize: 34, color: MUTED }}>
              {upstream ? `An independent Go port of ${upstream}` : 'An independent Go port'}
            </div>
          )}
          <div style={{ display: 'flex', marginTop: 26, fontSize: 30, color: GO }}>{pkg}</div>
        </div>
      </div>
    ),
    size,
  );
}
