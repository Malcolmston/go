import type { AriaAttributes, CSSProperties, ElementType } from 'react';

export interface HtmlProps extends AriaAttributes {
  /** The element to render; defaults to a <span>. */
  tag?: ElementType;
  /** Pre-built, trusted markup from src/data.ts (icons, feature bullets). */
  html: string;
  className?: string;
  style?: CSSProperties;
  id?: string;
  title?: string;
  role?: string;
}

// Html renders raw pre-built markup (icons, feature bullets, release notes)
// into an arbitrary element via dangerouslySetInnerHTML.
//
// The prop list is explicit rather than an `[key: string]: unknown` index
// signature: the index signature accepted anything — including a second
// `dangerouslySetInnerHTML` that would silently override the `html` prop — and
// erased typo checking on every forwarded attribute.
//
// SAFETY: `html` must only ever be authored content from this repo (src/data.ts
// and the generated release notes). It is injected without sanitising, so it
// must never be wired to user input.
export function Html({ tag = 'span', html, ...rest }: HtmlProps) {
  const T = tag;
  return <T {...rest} dangerouslySetInnerHTML={{ __html: html }} />;
}
