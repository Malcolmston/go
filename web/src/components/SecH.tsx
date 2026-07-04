import type { ElementType, ReactNode, CSSProperties } from 'react';

export interface SecHProps {
  children: ReactNode;
  h?: ElementType;
  style?: CSSProperties;
}

// SecH is a section heading with the accent bar used throughout the site.
export function SecH({ children, h = 'h3', style }: SecHProps) {
  const H = h;
  return <div className="sec-h" style={style}><span className="bar" /><H style={{ margin: 0 }}>{children}</H></div>;
}
