// Test-only stub for `next/link`.
//
// The view components now navigate with the App Router's <Link> (e.g. Home's
// "Get started" CTA, LibView's cross-tab links). Under vitest/jsdom there is no
// AppRouterContext, so the real next/link — which reads the router for
// prefetching — throws. This stub renders a plain <a> with the resolved href,
// forwarding className/onClick/target/rel/etc. and dropping the Next-only props,
// which is all the unit tests need (they assert on role="link" + href).
//
// Wired in via `resolve.alias` in vitest.config.ts; it never ships in a build.
import * as React from 'react';

type UrlObject = {
  pathname?: string;
  query?: Record<string, string | number | undefined> | null;
  hash?: string;
};
type Href = string | UrlObject;

function hrefToString(href: Href): string {
  if (typeof href === 'string') return href;
  if (!href) return '';
  let out = href.pathname ?? '';
  if (href.query) {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(href.query)) {
      if (v !== undefined && v !== null) params.append(k, String(v));
    }
    const qs = params.toString();
    if (qs) out += `?${qs}`;
  }
  if (href.hash) out += href.hash.startsWith('#') ? href.hash : `#${href.hash}`;
  return out;
}

export interface LinkProps
  extends Omit<React.AnchorHTMLAttributes<HTMLAnchorElement>, 'href'> {
  href: Href;
  // Next-only props, accepted and ignored by the stub.
  prefetch?: boolean | null;
  replace?: boolean;
  scroll?: boolean;
  shallow?: boolean;
  passHref?: boolean;
  legacyBehavior?: boolean;
  locale?: string | false;
}

const Link = React.forwardRef<HTMLAnchorElement, LinkProps>(function Link(
  {
    href,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    prefetch,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    replace,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    scroll,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    shallow,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    passHref,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    legacyBehavior,
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    locale,
    children,
    ...rest
  },
  ref,
) {
  return (
    <a ref={ref} href={hrefToString(href)} {...rest}>
      {children}
    </a>
  );
});

export default Link;
