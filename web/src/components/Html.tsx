import type { ElementType } from 'react';

export interface HtmlProps {
  tag?: ElementType;
  html: string;
  [key: string]: unknown;
}

// Html renders raw pre-built markup (icons, feature bullets, release notes)
// into an arbitrary element via dangerouslySetInnerHTML.
export function Html({ tag = 'span', html, ...rest }: HtmlProps) {
  const T = tag;
  return <T {...rest} dangerouslySetInnerHTML={{ __html: html }} />;
}
