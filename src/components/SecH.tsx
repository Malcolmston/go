import type { ElementType, ReactNode, CSSProperties } from 'react';

export interface SecHProps {
  children: ReactNode;
  h?: ElementType;
  style?: CSSProperties;
  // id, when set, is placed on the wrapper so the heading is an anchor target
  // (e.g. for on-this-page navigation).
  id?: string;
}

// SecH is a section heading with the accent bar used throughout the site.
export function SecH({ children, h = 'h3', style, id }: SecHProps) {
  const H = h;
  return <div className="sec-h" id={id} style={style}><span className="bar" /><H style={{ margin: 0 }}>{children}</H></div>;
}
