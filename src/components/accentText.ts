import type { CSSProperties } from 'react';

// Accent colours reach the DOM as a custom property, never as a text colour:
// AccentText.css derives the per-theme readable text colour from `--acc`, so a
// component that hands the raw accent to `color` would fail WCAG AA on the
// light theme (as low as 1.55:1). Pair this with className="acc-text".
//
// `extra` carries the declarations that legitimately want the RAW accent —
// borders, dots, tinted fills — where the bar is 3:1 non-text contrast.
export function accentVar(accent: string, extra?: CSSProperties): CSSProperties {
  return { ...extra, '--acc': accent } as CSSProperties;
}
