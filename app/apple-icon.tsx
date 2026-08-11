// The iOS home-screen icon: /apple-icon, linked as <link rel="apple-touch-icon">
// by Next's `apple-icon` file convention. Without it, "Add to Home Screen" on
// iOS saves a screenshot of the page instead of a mark.
//
// Built the same way as app/icon.tsx and app/opengraph-image.tsx — `next/og`'s
// `ImageResponse`, no new dependency — and pinned static so the
// `output: 'export'` GitHub Pages build writes the PNG out at build time.
import { ImageResponse } from 'next/og';

export const dynamic = 'force-static';

// 180x180 is the size iOS asks for; it downscales for the smaller slots.
export const size = { width: 180, height: 180 };
export const contentType = 'image/png';

// Same palette as app/icon.tsx and the social card.
const BG = '#06070a';
const FG = '#f5f6f8';
const GO = '#00add8';

export default function AppleIcon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          // iOS applies its own corner mask, so the artwork is a full bleed
          // square and the padding below keeps the mark clear of the mask.
          background: BG,
          backgroundImage:
            'radial-gradient(180px 140px at 80% -10%, rgba(47,155,255,0.30), rgba(6,7,10,0))',
          color: GO,
          fontFamily: 'sans-serif',
        }}
      >
        <div style={{ display: 'flex', fontSize: 96, fontWeight: 700, lineHeight: 1 }}>go</div>
        <div
          style={{
            display: 'flex',
            marginTop: 10,
            fontSize: 20,
            fontWeight: 600,
            letterSpacing: '0.08em',
            color: FG,
          }}
        >
          malcolmston
        </div>
      </div>
    ),
    size,
  );
}
